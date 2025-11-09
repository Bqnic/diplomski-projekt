package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	libp2p "github.com/libp2p/go-libp2p"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	peer "github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/discovery/routing"
	discUtil "github.com/libp2p/go-libp2p/p2p/discovery/util"
	"github.com/multiformats/go-multiaddr"
)

func NewKDHT(ctx context.Context, host host.Host, bootstrapPeers []multiaddr.Multiaddr) (*routing.RoutingDiscovery, error) {
	var options []dht.Option

	if len(bootstrapPeers) == 0 {
		options = append(options, dht.Mode(dht.ModeServer))
	}

	kdht, err := dht.New(ctx, host, options...)
	if err != nil {
		return nil, err
	}

	if err = kdht.Bootstrap(ctx); err != nil {
		return nil, err
	}
	
	for _, peerAddr := range bootstrapPeers {
		peerinfo, _ := peer.AddrInfoFromP2pAddr(peerAddr)

		go func(peerinfo peer.AddrInfo) {
			if err := host.Connect(ctx, peerinfo); err != nil {
				log.Printf("Error connecting to %v: %v", peerinfo, err)
			} else {
				log.Printf("Connected to bootstrap node: %v", peerinfo)
			}
		}(*peerinfo)
	}

	return routing.NewRoutingDiscovery(kdht), nil
}

func Discover(ctx context.Context, h host.Host, dht *routing.RoutingDiscovery, rendezvous string) {
	var routingDiscovery = routing.NewRoutingDiscovery(dht)
	discUtil.Advertise(ctx, routingDiscovery, rendezvous)

	ticker := time.NewTicker(time.Second * 1)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:

			peers, err := discUtil.FindPeers(ctx, routingDiscovery, rendezvous)
			if err != nil {
				log.Fatal(err)
			}

			for _, p := range peers {
				if p.ID == h.ID() {
					continue
				}
				if h.Network().Connectedness(p.ID) != network.Connected {
					_, err = h.Network().DialPeer(ctx, p.ID)
					if err != nil {
						continue
					}
				}
			}
		}
	}
}


// transfer models between nodes
func handleModelProtocol(host host.Host, modelRoot string) {
	host.SetStreamHandler(modelProtocolID, func(stream network.Stream) {
		defer stream.Close()

		remote := stream.Conn().RemotePeer()
		log.Printf("[protocol] incoming stream from %s\n", remote)

		reader := bufio.NewReader(stream)

		reqLine, err := reader.ReadString('\n')
		if err != nil {
			log.Printf("failed reading request: %v\n", err)
			return
		}

		reqLine = strings.TrimSpace(reqLine)
		// request format: "GET <modelID>\n"
		parts := strings.SplitN(reqLine, " ", 2)
		if len(parts) != 2 || strings.ToUpper(parts[0]) != "GET" {
			io.WriteString(stream, "ERR invalid request\n")
			return
		}

		modelID := parts[1]
		path := filepath.Join(modelRoot, modelID)
		file, err := os.Open(path)
		if err != nil {
			io.WriteString(stream, fmt.Sprintf("ERR open: %v\n", err))
			log.Printf("could not open model %s: %v\n", path, err)
			return
		}
		defer file.Close()

		io.WriteString(stream, "OK\n") // handshake
		// stream file bytes
		n, err := io.Copy(stream, file)
		if err != nil {
			log.Printf("error sending model: %v\n", err)
			return
		}

		log.Printf("sent %d bytes of model %s to %s\n", n, modelID, remote)
	})
}

// announcement of new models to pubsub
func announceModels(ctx context.Context, topic *pubsub.Topic, h host.Host, modelRoot string, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	pid := h.ID()

	for {
		files, err := os.ReadDir(modelRoot)
		if err != nil {
			log.Printf("announce: could not read model dir: %v\n", err)
			return
		}

		for _, fi := range files {
			if fi.IsDir() {
				continue
			}

			stat, _ := fi.Info()
			meta := ModelMeta{
				PeerID:   pid.String(),
				ModelID:  fi.Name(),
				Size:     stat.Size(),
				Filename: fi.Name(),
				Time:     time.Now().Unix(),
			}
			b, _ := json.Marshal(meta)
			if err := topic.Publish(ctx, b); err != nil {
				log.Printf("failed to publish model meta: %v\n", err)
			} else {
				log.Printf("[pubsub] announced model %s (%d bytes)\n", fi.Name(), stat.Size())
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// continue loop
		}
	}
}

// subscribe to topic of model announcements
func subscribeAnnouncements(ctx context.Context, topic *pubsub.Topic, h host.Host, modelRoot string) {
	sub, err := topic.Subscribe()
	if err != nil {
		log.Fatalf("subscribe: failed to subscribe: %v", err)
	}

	go func() {
		for {
			msg, err := sub.Next(ctx)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				log.Printf("error reading pubsub message: %v\n", err)
				continue
			}

			// ignore self published messages
			if msg.ReceivedFrom == h.ID() {
				continue
			}

			var m ModelMeta
			if err := json.Unmarshal(msg.Data, &m); err != nil {
				log.Printf("invalid meta from %s: %v\n", msg.ReceivedFrom, err)
				continue
			}
			log.Printf("[pubsub] discovered model announcement: peer=%s model=%s size=%d\n", m.PeerID, m.ModelID, m.Size)

			// try to fetch it (simple: fetch immediately once)
			go func(meta ModelMeta) {
				peerID, err := peer.Decode(meta.PeerID)
				if err != nil {
					log.Printf("invalid peer id: %v\n", err)
					return
				}
				// ensure we have addresses for the peer. If not, try to dial (mDNS likely supplied addr)
				ctx2, cancel := context.WithTimeout(ctx, 10*time.Second)
				defer cancel()

				if err := h.Connect(ctx2, peer.AddrInfo{ID: peerID}); err != nil {
					log.Printf("connect failed to %s: %v\n", meta.PeerID, err)
				}

				// open stream
				stream, err := h.NewStream(ctx2, peerID, modelProtocolID)
				if err != nil {
					log.Printf("open stream failed: %v\n", err)
					return
				}
				defer stream.Close()
				// request model
				_, err = stream.Write([]byte("GET " + meta.ModelID + "\n"))
				if err != nil {
					log.Printf("write request err: %v\n", err)
					return
				}

				br := bufio.NewReader(stream)
				line, err := br.ReadString('\n')
				if err != nil {
					log.Printf("failed read handshake: %v\n", err)
					return
				}

				if strings.HasPrefix(line, "OK") {
					outpath := filepath.Join(modelRoot, "remote_"+meta.ModelID)
					out, err := os.Create(outpath)
					if err != nil {
						log.Printf("create file err: %v\n", err)
						return
					}
					defer out.Close()
					n, err := io.Copy(out, br)
					if err != nil {
						log.Printf("copy err: %v\n", err)
						return
					}
					log.Printf("saved %d bytes to %s\n", n, outpath)
				} else {
					log.Printf("peer responded: %s", strings.TrimSpace(line))
				}
			}(m)
		}
	}()
}

func printAddrs(h host.Host) {
	fmt.Println("Host ID:", h.ID())
	fmt.Println("Listening on:")
	for _, a := range h.Addrs() {
		fmt.Printf("  %s/p2p/%s\n", a, h.ID())
	}
}

func main() {
	ctx := context.Background()
	var discoveryPeers addrList

	var (
		rendezvous = flag.String("rendezvous", "diabetes", "")
		listen = flag.String("listen", "/ip4/0.0.0.0/tcp/0", "multiaddr to listen on")
		localModelDir = flag.String("local", "../local-models", "directory containing local model files (one file per modelID)")
		remoteModelDir = flag.String("remote", "../remote-models", "directory containing remote model files (one file per modelID)")
		announceInt = flag.Duration("announce", 15*time.Second, "how often to announce available models on pubsub")
		nick = flag.String("nick", "", "optional human-readable nickname")	
	)
	flag.Var(&discoveryPeers, "peer", "Peer multiaddress for peer discovery")
	flag.Parse()

	// ensure model dirs exists
	if err := os.MkdirAll(*localModelDir, 0755); err != nil {
		log.Fatalf("could not create local model dir: %v", err)
	}

	if err := os.MkdirAll(*remoteModelDir, 0755); err != nil {
		log.Fatalf("could not create remote model dir: %v", err)
	}

	// create libp2p host
	addr, err := multiaddr.NewMultiaddr(*listen)
	if err != nil {
		log.Fatalf("invalid listen multiaddr: %v", err)
	}

	host, err := libp2p.New(
		libp2p.ListenAddrs(addr),
	)
	if err != nil {
		log.Fatalf("failed to create libp2p host: %v", err)
	}
	defer host.Close()

	printAddrs(host)
	if *nick != "" {
		fmt.Printf("nick: %s\n", *nick)
	}

	// setup pubsub
	ps, err := pubsub.NewGossipSub(ctx, host)
	if err != nil {
		log.Fatalf("pubsub init failed: %v", err)
	}

	topic, err := ps.Join(pubsubTopicName)
	if err != nil {
		log.Fatalf("failed to join pubsub topic: %v", err)
	}

	dht, err := NewKDHT(ctx, host, discoveryPeers)
	if err != nil {
		log.Fatal(err)
	}

	go Discover(ctx, host, dht, *rendezvous)


	// start protocol handler for model transfers
	handleModelProtocol(host, *localModelDir)

	// subscribe to announcements
	subscribeAnnouncements(ctx, topic, host, *remoteModelDir)

	// start announcer to periodically publish local model metadata
	go announceModels(ctx, topic, host, *localModelDir, *announceInt)

	// wait for a SIGINT or SIGTERM signal
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	<-ch
	fmt.Println("Received signal, shutting down...")
}

type addrList []multiaddr.Multiaddr

func (al *addrList) String() string {
	strs := make([]string, len(*al))
	for i, addr := range *al {
		strs[i] = addr.String()
	}
	return strings.Join(strs, ",")
}

func (al *addrList) Set(value string) error {
	addr, err := multiaddr.NewMultiaddr(value)
	if err != nil {
		return err
	}
	*al = append(*al, addr)
	return nil
}


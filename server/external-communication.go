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
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	peer "github.com/libp2p/go-libp2p/core/peer"
	peerstore "github.com/libp2p/go-libp2p/core/peerstore"
	"github.com/libp2p/go-libp2p/p2p/discovery/mdns"
	ma "github.com/multiformats/go-multiaddr"
)

func (n *mdnsNotifee) HandlePeerFound(pi peer.AddrInfo) {
	log.Printf("[mDNS] discovered: %s\n", pi.ID)
	// add to peerstore so we can dial directly
	n.h.Peerstore().AddAddrs(pi.ID, pi.Addrs, peerstore.TempAddrTTL)
}

func setupMDNS(ctx context.Context, h host.Host) error {
	// mdns.NewMdnsService(ctx, h, rendezvous, notifee)
	s := mdns.NewMdnsService(h, mdnsServiceTag, &mdnsNotifee{h: h})
	// the mdns pkg runs its own goroutine
	go func() {
		<-ctx.Done()
		s.Close()
	}()
	return nil
}

func handleModelProtocol(h host.Host, modelRoot string) {
	h.SetStreamHandler(modelProtocolID, func(s network.Stream) {
		defer s.Close()
		remote := s.Conn().RemotePeer()
		log.Printf("[protocol] incoming stream from %s\n", remote)
		r := bufio.NewReader(s)
		reqLine, err := r.ReadString('\n')
		if err != nil {
			log.Printf("failed reading request: %v\n", err)
			return
		}
		reqLine = strings.TrimSpace(reqLine)
		// request format: "GET <modelID>\n"
		parts := strings.SplitN(reqLine, " ", 2)
		if len(parts) != 2 || strings.ToUpper(parts[0]) != "GET" {
			io.WriteString(s, "ERR invalid request\n")
			return
		}
		modelID := parts[1]
		path := filepath.Join(modelRoot, modelID)
		f, err := os.Open(path)
		if err != nil {
			io.WriteString(s, fmt.Sprintf("ERR open: %v\n", err))
			log.Printf("could not open model %s: %v\n", path, err)
			return
		}
		defer f.Close()
		io.WriteString(s, "OK\n") // handshake
		// stream file bytes
		n, err := io.Copy(s, f)
		if err != nil {
			log.Printf("error sending model: %v\n", err)
			return
		}
		log.Printf("sent %d bytes of model %s to %s\n", n, modelID, remote)
	})
}

func announceModels(ctx context.Context, topic *pubsub.Topic, h host.Host, modelRoot string, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	pid := h.ID()

	for {
		// gather models from modelRoot
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

	var (
		listen      = flag.String("listen", "/ip4/0.0.0.0/tcp/0", "multiaddr to listen on")
		modelDir    = flag.String("models", "./models", "directory containing model files to serve (one file per modelID)")
		announceInt = flag.Duration("announce", 15*time.Second, "how often to announce available models on pubsub")
		nick        = flag.String("nick", "", "optional human-readable nickname")
	)
	flag.Parse()

	// ensure model dir exists
	if err := os.MkdirAll(*modelDir, 0755); err != nil {
		log.Fatalf("could not create model dir: %v", err)
	}

	// create libp2p host
	addr, err := ma.NewMultiaddr(*listen)
	if err != nil {
		log.Fatalf("invalid listen multiaddr: %v", err)
	}

	h, err := libp2p.New(
		libp2p.ListenAddrs(addr),
	)
	if err != nil {
		log.Fatalf("failed to create libp2p host: %v", err)
	}
	defer h.Close()

	printAddrs(h)
	if *nick != "" {
		fmt.Printf("nick: %s\n", *nick)
	}

	// setup mDNS discovery for LAN
	if err := setupMDNS(ctx, h); err != nil {
		log.Printf("warning: mdns setup failed: %v", err)
	}

	// setup pubsub
	ps, err := pubsub.NewGossipSub(ctx, h)
	if err != nil {
		log.Fatalf("pubsub init failed: %v", err)
	}

	topic, err := ps.Join(pubsubTopicName)
	if err != nil {
		log.Fatalf("failed to join pubsub topic: %v", err)
	}

	// start protocol handler for model transfers
	handleModelProtocol(h, *modelDir)

	// subscribe to announcements
	subscribeAnnouncements(ctx, topic, h, *modelDir)

	// start announcer to periodically publish local model metadata
	go announceModels(ctx, topic, h, *modelDir, *announceInt)

	// wait for a SIGINT or SIGTERM signal
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	<-ch
	fmt.Println("Received signal, shutting down...")
}

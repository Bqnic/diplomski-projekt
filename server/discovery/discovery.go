package discovery

import (
	"context"
	"fmt"
	"log"
	"time"

	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	peer "github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/discovery/routing"
	discUtil "github.com/libp2p/go-libp2p/p2p/discovery/util"
	"github.com/multiformats/go-multiaddr"
)

func NewKDHT(ctx context.Context, host host.Host, bootstrapPeerString string) (*routing.RoutingDiscovery, error) {
	var options []dht.Option
	var bootstrapPeer multiaddr.Multiaddr
	var err error

	if bootstrapPeerString == "" {
		options = append(options, dht.Mode(dht.ModeServer))
	} else {
		bootstrapPeer, err = multiaddr.NewMultiaddr(bootstrapPeerString)
		if err != nil {
			return nil, fmt.Errorf("invalid bootstrap peer multiaddr: %w", err)
		}
	}

	kdht, err := dht.New(ctx, host, options...)
	if err != nil {
		return nil, err
	}

	if err = kdht.Bootstrap(ctx); err != nil {
		return nil, err
	}
	
	if bootstrapPeer != nil {
		peerinfo, _ := peer.AddrInfoFromP2pAddr(bootstrapPeer)
		log.Printf("peer %s", peerinfo.String())

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

func Discover(ctx context.Context, h host.Host, rd *routing.RoutingDiscovery, rendezvous string) {
    discUtil.Advertise(ctx, rd, rendezvous)

    ticker := time.NewTicker(time.Second * 1)
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            peers, err := discUtil.FindPeers(ctx, rd, rendezvous)
            if err != nil {
                log.Printf("FindPeers err: %v", err)
                continue
            }
            for _, p := range peers {
                if p.ID == h.ID() {
                    continue
                }
                if h.Network().Connectedness(p.ID) != network.Connected {
                    log.Printf("Discovered peer via DHT: %s", p.ID)
                    h.Connect(ctx, p)
                }
            }
        }
    }
}
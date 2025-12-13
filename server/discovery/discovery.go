package discovery

import (
	"context"
	"fmt"
	"log"

	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p/core/host"
	peer "github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/discovery/routing"
	"github.com/multiformats/go-multiaddr"
)

func NewKDHT(ctx context.Context, host host.Host, bootstrapPeerString string) (*routing.RoutingDiscovery, error) {
	var bootstrapPeer multiaddr.Multiaddr
	var err error

	if bootstrapPeerString != "" {
		bootstrapPeer, err = multiaddr.NewMultiaddr(bootstrapPeerString)
		if err != nil {
			return nil, fmt.Errorf("invalid bootstrap peer multiaddr: %w", err)
		}
	}

	kdht, err := dht.New(ctx, host, dht.Mode(dht.ModeServer))
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
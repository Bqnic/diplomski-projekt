package discovery

import (
	"fmt"
	"log"

	"github.com/bqnic/diplomski-projekt/common"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	peer "github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/discovery/routing"
	"github.com/multiformats/go-multiaddr"
)

func NewKDHT(bootstrapPeerString string) (*routing.RoutingDiscovery, error) {
	var bootstrapPeer multiaddr.Multiaddr
	var err error

	if bootstrapPeerString != "" {
		bootstrapPeer, err = multiaddr.NewMultiaddr(bootstrapPeerString)
		if err != nil {
			return nil, fmt.Errorf("invalid bootstrap peer multiaddr: %w", err)
		}
	}

	kdht, err := dht.New(common.Ctx, common.Host, dht.Mode(dht.ModeServer))
	if err != nil {
		return nil, err
	}

	if err = kdht.Bootstrap(common.Ctx); err != nil {
		return nil, err
	}
	
	if bootstrapPeer != nil {
		peerinfo, _ := peer.AddrInfoFromP2pAddr(bootstrapPeer)
		log.Printf("peer %s", peerinfo.String())

		go func(peerinfo peer.AddrInfo) {
			if err := common.Host.Connect(common.Ctx, peerinfo); err != nil {
				log.Printf("Error connecting to %v: %v", peerinfo, err)
			} else {
				log.Printf("Connected to bootstrap node: %v", peerinfo)
			}
		}(*peerinfo)
	}

	return routing.NewRoutingDiscovery(kdht), nil
}
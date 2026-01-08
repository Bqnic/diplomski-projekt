package discovery

import (
	"fmt"

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
		common.Log.Infow("Bootstrap peer provided", "peer_short", common.ShortID(peerinfo.ID.String()), "addrs", peerinfo.Addrs)

		go func(peerinfo peer.AddrInfo) {
				if err := common.Host.Connect(common.Ctx, peerinfo); err != nil {
					common.Log.Errorw("Failed to connect to bootstrap peer", "peer_short", common.ShortID(peerinfo.ID.String()), "err", err)
				} else {
					common.Log.Infow("Connected to bootstrap peer", "peer_short", common.ShortID(peerinfo.ID.String()))
				}
		}(*peerinfo)
	}

	return routing.NewRoutingDiscovery(kdht), nil
}
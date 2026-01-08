package common

import (
	"github.com/libp2p/go-libp2p/core/host"
)

func PrintAddrs(h host.Host) {
	Log.Infof("Host ID: %s", h.ID())
}

// ShortID returns a shortened representation of long identifiers for easier logs.
func ShortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}
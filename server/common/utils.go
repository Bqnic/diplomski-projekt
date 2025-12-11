package common

import (
	"fmt"

	"github.com/libp2p/go-libp2p/core/host"
)

func PrintAddrs(h host.Host) {
	fmt.Println("Host ID:", h.ID())
	fmt.Println("Listening on:")
	for _, a := range h.Addrs() {
		fmt.Printf("  %s/p2p/%s\n", a, h.ID())
	}
}
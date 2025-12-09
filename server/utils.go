package main

import (
	"fmt"
	"strings"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/multiformats/go-multiaddr"
)

func printAddrs(h host.Host) {
	fmt.Println("Host ID:", h.ID())
	fmt.Println("Listening on:")
	for _, a := range h.Addrs() {
		fmt.Printf("  %s/p2p/%s\n", a, h.ID())
	}
}

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
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	libp2p "github.com/libp2p/go-libp2p"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/multiformats/go-multiaddr"
)

func main() {
	ctx := context.Background()
	var discoveryPeers addrList

	var (
		rendezvous = flag.String("rendezvous", "diabetes", "")
		listen = flag.String("listen", "", "multiaddr to listen on")
		localModelDir = flag.String("local", "../local-models", "directory containing local model files (one file per modelID)")
		remoteModelDir = flag.String("remote", "../remote-models", "directory containing remote model files (one file per modelID)")
		announceInt = flag.Duration("announce", 15*time.Second, "how often to announce available models on pubsub")
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


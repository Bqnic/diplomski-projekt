package main

import (
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/bqnic/diplomski-projekt/common"
	"github.com/bqnic/diplomski-projekt/communication"
	"github.com/bqnic/diplomski-projekt/discovery"
	localGrpc "github.com/bqnic/diplomski-projekt/grpc"
	localPubSub "github.com/bqnic/diplomski-projekt/pubsub"
	libp2p "github.com/libp2p/go-libp2p"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/multiformats/go-multiaddr"
)

func main() {
	listen := os.Getenv("FL_LISTEN")
	bootstrapPeer := os.Getenv("BOOTSTRAP_PEER")

	// ensure model dirs exists
	if err := os.MkdirAll(common.LocalModelDir, 0755); err != nil {
		log.Fatalf("could not create local model dir: %v", err)
	}

	if err := os.MkdirAll(common.RemoteModelDir, 0755); err != nil {
		log.Fatalf("could not create remote model dir: %v", err)
	}

	// create libp2p host
	addr, err := multiaddr.NewMultiaddr(listen)
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

	common.SetHost(host)

	common.PrintAddrs(host)

	// pubsub
	ps, err := pubsub.NewGossipSub(common.Ctx, host)
	if err != nil {
		log.Fatalf("pubsub init failed: %v", err)
	}

	topic, err := ps.Join(common.PubsubTopicName)
	if err != nil {
		log.Fatalf("failed to join pubsub topic: %v", err)
	}

	common.SetTopic(topic)

	// discovery
	_, err = discovery.NewKDHT(bootstrapPeer)
	if err != nil {
		log.Fatal(err)
	}

	// grpc
	lis, err := net.Listen("tcp", ":50051")
    if err != nil {
        panic(err)
    }
	
	grpcServer := localGrpc.SetupGrpcServer()
    go grpcServer.Serve(lis)

	// start protocol handler for model transfers
	communication.HandleModelProtocol()

	// subscribe to announcements
	localPubSub.SubscribeAnnouncements()

	// wait for a SIGINT or SIGTERM signal
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	<-ch
	fmt.Println("Received signal, shutting down...")
}


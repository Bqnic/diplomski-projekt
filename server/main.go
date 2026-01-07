package main

import (
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
		common.Log.Fatalw("Could not create local model directory", "dir", common.LocalModelDir, "err", err)
	}

	if err := os.MkdirAll(common.RemoteModelDir, 0755); err != nil {
		common.Log.Fatalw("Could not create remote model directory", "dir", common.RemoteModelDir, "err", err)
	}

	// create libp2p host
	addr, err := multiaddr.NewMultiaddr(listen)
	if err != nil {
		common.Log.Fatalw("Invalid listen multiaddr", "addr", listen, "err", err)
	}

	host, err := libp2p.New(
		libp2p.ListenAddrs(addr),
	)
	if err != nil {
		common.Log.Fatalw("Failed to create libp2p host", "err", err)
	}
	defer host.Close()

	common.SetHost(host)

	common.PrintAddrs(host)

	// pubsub
	ps, err := pubsub.NewGossipSub(common.Ctx, host)
	if err != nil {
		common.Log.Fatalw("Pubsub initialization failed", "err", err)
	}

	topic, err := ps.Join(common.PubsubTopicName)
	if err != nil {
		common.Log.Fatalw("Failed to join pubsub topic", "topic", common.PubsubTopicName, "err", err)
	}

	common.SetTopic(topic)

	// discovery
	_, err = discovery.NewKDHT(bootstrapPeer)
	if err != nil {
		common.Log.Fatalw("Discovery setup failed", "err", err)
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
	common.Log.Infow("Shutting down: signal received", "reason", "signal_received")
	if err := common.SyncLogger(); err != nil {
		// best-effort
	}
}


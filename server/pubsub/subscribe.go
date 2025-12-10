package pubsub

import (
	"context"
	"log"

	"github.com/bqnic/diplomski-projekt/communication"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/host"
)

func SubscribeAnnouncements(ctx context.Context, topic *pubsub.Topic, h host.Host, modelRoot string) {
	sub, err := topic.Subscribe()
	if err != nil {
		log.Fatalf("subscribe: failed to subscribe: %v", err)
	}

	go communication.GetRemoteModel(ctx, h, sub, modelRoot)
}
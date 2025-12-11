package pubsub

import (
	"log"

	"github.com/bqnic/diplomski-projekt/common"
	"github.com/bqnic/diplomski-projekt/communication"
)

func SubscribeAnnouncements() {
	sub, err := common.Topic.Subscribe()
	if err != nil {
		log.Fatalf("subscribe: failed to subscribe: %v", err)
	}

	log.Printf("subscribed")

	go communication.GetRemoteModel(sub)
}
package pubsub

import (
	"github.com/bqnic/diplomski-projekt/common"
	"github.com/bqnic/diplomski-projekt/communication"
)

func SubscribeAnnouncements() {
	sub, err := common.Topic.Subscribe()
	if err != nil {
		common.Log.Fatalw("Failed to subscribe to announcements", "err", err)
	}

	common.Log.Infow("Subscribed to announcements", "topic", common.PubsubTopicName)

	go communication.GetRemoteModel(sub)
}
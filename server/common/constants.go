package common

import (
	"context"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/host"
)

var PubsubTopicName = "fl-model-announcements"
var LocalModelDir string = "/app/shared/local-models"
var RemoteModelDir string = "/app/shared/remote-models"
var Ctx context.Context = context.Background()

var Host host.Host
var Topic *pubsub.Topic

func SetHost(host host.Host) {
	Host = host
}

func SetTopic(topic *pubsub.Topic) {
	Topic = topic
}
package common

import (
	"context"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/host"
)

var PubsubTopicName = "fl-model-announcements"
var Host host.Host
var Topic *pubsub.Topic
var Ctx context.Context
var LocalModelDir string
var RemoteModelDir string

func SetHost(host host.Host) {
	Host = host
}

func SetTopic(topic *pubsub.Topic) {
	Topic = topic
}

func SetContext(ctx context.Context) {
	Ctx = ctx
}

func SetModelDirs(localModelDir string, remoteModelDir string) {
	LocalModelDir = "/app" + localModelDir
	RemoteModelDir = "/app" + remoteModelDir
}
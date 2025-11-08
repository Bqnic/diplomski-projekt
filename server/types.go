package main

import "github.com/libp2p/go-libp2p/core/host"

const (
	// Rendezvous topic for mDNS and pubsub topic name for model announcements
	mdnsServiceTag   = "fl-mdns-demo"
	pubsubTopicName  = "fl-model-announcements"
	modelProtocolID  = "/fl/model/1.0.0" // custom stream protocol to request model bytes
)

// ModelMeta is published to the pubsub topic so peers can learn who has what
type ModelMeta struct {
	PeerID   string `json:"peer_id"`
	ModelID  string `json:"model_id"`
	Size     int64  `json:"size"`
	Filename string `json:"filename"`
	Time     int64  `json:"time_unix"`
}

// mdnsNotifee implements mdns.Notifee to be informed about newly discovered peers.
type mdnsNotifee struct {
	h host.Host
}
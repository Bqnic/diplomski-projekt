package pubsub

import "sync"

type Announcer struct {
	mu        sync.Mutex
	announced map[string]struct{}
}
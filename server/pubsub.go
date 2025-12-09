package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"time"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/host"
)

// announcement of new models to pubsub
func announceModels(ctx context.Context, topic *pubsub.Topic, h host.Host, modelRoot string, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	pid := h.ID()

	for {
		files, err := os.ReadDir(modelRoot)
		if err != nil {
			log.Printf("announce: could not read model dir: %v\n", err)
			return
		}

		for _, fi := range files {
			if fi.IsDir() {
				continue
			}

			stat, _ := fi.Info()
			meta := ModelMeta{
				PeerID:   pid.String(),
				ModelID:  fi.Name(),
				Size:     stat.Size(),
				Filename: fi.Name(),
				Time:     time.Now().Unix(),
			}
			b, _ := json.Marshal(meta)
			if err := topic.Publish(ctx, b); err != nil {
				log.Printf("failed to publish model meta: %v\n", err)
			} else {
				log.Printf("[pubsub] announced model %s (%d bytes)\n", fi.Name(), stat.Size())
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// continue loop
		}
	}
}

func subscribeAnnouncements(ctx context.Context, topic *pubsub.Topic, h host.Host, modelRoot string) {
	sub, err := topic.Subscribe()
	if err != nil {
		log.Fatalf("subscribe: failed to subscribe: %v", err)
	}

	go getRemoteModel(ctx, h, sub, modelRoot)
}
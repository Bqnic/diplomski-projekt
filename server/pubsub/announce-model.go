package pubsub

import (
	"context"
	"encoding/json"
	"log"
	"path/filepath"
	"time"

	"github.com/bqnic/diplomski-projekt/common"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/host"

	"github.com/radovskyb/watcher"
)

func AnnounceModel(ctx context.Context, topic *pubsub.Topic, h host.Host, modelRoot string) error {
	w := watcher.New()

	// Polling interval (100ms is fast but safe)
	w.SetMaxEvents(1)
	w.FilterOps(watcher.Create, watcher.Write, watcher.Rename, watcher.Move)

	// Recursive watch
	if err := w.AddRecursive(modelRoot); err != nil {
		return err
	}

	peerID := h.ID().String()
	ann := NewAnnouncer()

	// Debounce map: path → timer
	pending := make(map[string]*time.Timer)

	stabilize := func(path string) {
		if t, ok := pending[path]; ok {
			t.Stop()
		}

		pending[path] = time.AfterFunc(500*time.Millisecond, func() {
			// Hash and announce
			hash, size, err := hashFile(path)
			if err != nil {
				return
			}

			if ann.has(hash) {
				return
			}

			meta := common.ModelMeta{
				PeerID:   peerID,
				ModelID:  filepath.Base(path),
				Size:     size,
				Filename: filepath.Base(path),
				Time:     time.Now().Unix(),
				Hash:     hash,
			}

			b, _ := json.Marshal(meta)
			if err := topic.Publish(ctx, b); err != nil {
				log.Printf("failed to publish model meta: %v", err)
				return
			}

			ann.add(hash)
			log.Printf("[pubsub] announced model %s (%d bytes)", meta.Filename, meta.Size)
		})
	}

	// Start watcher loop
	go func() {
		defer log.Println("model watcher stopped")

		for {
			select {
			case ev := <-w.Event:
				path := ev.Path
				stabilize(path)

			case err := <-w.Error:
				log.Printf("watcher error: %v", err)

			case <-ctx.Done():
				w.Close()
				return
			}
		}
	}()

	// Start polling
	go func() {
		if err := w.Start(100 * time.Millisecond); err != nil {
			log.Printf("watcher failed: %v", err)
		}
	}()

	log.Printf("watching directory recursively: %s", modelRoot)
	return nil
}

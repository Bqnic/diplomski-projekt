package pubsub

import (
	"context"
	"encoding/json"
	"log"
	"path/filepath"
	"time"

	"github.com/bqnic/diplomski-projekt/common"
	"github.com/fsnotify/fsnotify"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/host"
)

func AnnounceModel(ctx context.Context, topic *pubsub.Topic, h host.Host, modelRoot string) error {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer w.Close()

	if err := w.Add(modelRoot); err != nil {
		return err
	}

	peerID := h.ID().String()
	ann := NewAnnouncer()

	pending := make(map[string]*time.Timer)

	stabilize := func(path string) {
		timer, exists := pending[path]
		if exists {
			timer.Stop()
		}
		timer = time.AfterFunc(500*time.Millisecond, func() {
			// Check hash and announce
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

		pending[path] = timer
	}

	go func() {
		defer log.Println("model watcher stopped")

		for {
			select {
			case ev := <-w.Events:
				if ev.Op&(fsnotify.Create|fsnotify.Write|fsnotify.Rename) != 0 {
					path := ev.Name
					stabilize(path)
				}

			case err := <-w.Errors:
				log.Printf("watcher error: %v\n", err)

			case <-ctx.Done():
				return
			}
		}
	}()

	return nil
}
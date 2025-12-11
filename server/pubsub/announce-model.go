package pubsub

import (
	"encoding/json"
	"log"

	"github.com/bqnic/diplomski-projekt/common"
)

func AnnounceModel(model common.ModelMeta) error {
	log.Printf("PUBSUB: got model ", model)

	b, _ := json.Marshal(model)
	if err := common.Topic.Publish(common.Ctx, b); err != nil {
		log.Printf("failed to publish model meta: %v", err)
		return err
	}

	log.Printf("[pubsub] announced model %s (%d bytes)", model.Size)

	return nil
}

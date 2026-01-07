package pubsub

import (
	"encoding/json"

	"github.com/bqnic/diplomski-projekt/common"
)

func AnnounceModel(model common.ModelMeta) error {
	common.Log.Debugw("Publishing model announcement", "model", model.ModelID, "size", model.Size)

	b, _ := json.Marshal(model)
	if err := common.Topic.Publish(common.Ctx, b); err != nil {
		common.Log.Errorw("Failed to publish model announcement", "model", model.ModelID, "err", err)
		return err
	}

	common.Log.Infow("Announced model to peers", "model", model.ModelID, "size", model.Size)

	return nil
}

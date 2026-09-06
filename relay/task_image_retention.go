package relay

import (
	"encoding/json"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

// imageTaskResponseWithRetention is the final serialization boundary. On expiry
// use the sanitized saved response, never freshly fetched or re-uploaded media.
func imageTaskResponseWithRetention(task *model.Task, body []byte) ([]byte, error) {
	now := common.GetTimestamp()
	view, err := task.ImageRetentionView(now)
	if err != nil {
		return nil, err
	}
	if view.ImageContentExpired(now) {
		body = view.Data
	}
	var payload map[string]json.RawMessage
	if len(body) > 0 {
		if err := common.Unmarshal(body, &payload); err != nil {
			return nil, err
		}
	}
	if payload == nil {
		payload = make(map[string]json.RawMessage)
	}
	availability := view.ImageAvailability(now)
	if availability.ImageStatus != "" {
		payload["image_status"], _ = common.Marshal(availability.ImageStatus)
		payload["image_expires_at"], _ = common.Marshal(availability.ImageExpiresAt)
		if availability.ImageHasURL {
			payload["image_has_url"] = json.RawMessage("true")
		}
		if availability.ImageMessage != "" {
			payload["image_message"], _ = common.Marshal(availability.ImageMessage)
		}
	}
	if view.ImageContentExpired(now) {
		payload["id"], _ = common.Marshal(view.TaskID)
		payload["state"], _ = common.Marshal(mapTaskStatusToSimple(view.Status))
	}
	return common.Marshal(payload)
}

package configurable

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// Accept both the documented /v1 base and the gateway's usual root base.
func YHURL(base, path string) string {
	base = strings.TrimRight(base, "/")
	if strings.HasSuffix(base, "/v1") && strings.HasPrefix(path, "/v1/") {
		path = strings.TrimPrefix(path, "/v1")
	}
	return base + "/" + strings.TrimLeft(path, "/")
}

// Restore raw metadata after mapping to retain numeric precision and extensions,
// while keeping the shared mapping's precedence for known generic fields.
func mergeYHVideoMetadata(c *gin.Context, req relaycommon.TaskSubmitReq, body []byte) ([]byte, error) {
	var original map[string]json.RawMessage
	if err := common.UnmarshalBodyReusable(c, &original); err != nil {
		return nil, err
	}
	raw, exists := original["metadata"]
	if !exists {
		return body, nil
	}
	if gjson.ParseBytes(raw).Type == gjson.String {
		var encoded string
		if err := common.Unmarshal(raw, &encoded); err != nil {
			return nil, err
		}
		if strings.TrimSpace(encoded) == "" {
			return body, nil
		}
		raw = []byte(encoded)
	}
	var metadata, mapped map[string]json.RawMessage
	if err := common.Unmarshal(raw, &metadata); err != nil {
		return nil, err
	}
	if err := common.Unmarshal(body, &mapped); err != nil {
		return nil, err
	}
	for key, value := range metadata {
		switch key {
		case "model", "omni_reference_task_type", "resolution", "ratio":
			// These fields already have the selected model or validated fallback.
			continue
		case "content":
			if isEmptyValue(req.Metadata["content"]) {
				continue
			}
		case "duration":
			if len(original["duration"]) > 0 || len(original["seconds"]) > 0 {
				continue
			}
		}
		mapped[key] = value
	}
	return common.Marshal(mapped)
}

func buildYHVideoRequest(c *gin.Context, body []byte, info *relaycommon.RelayInfo, fallbackPrompt string) ([]byte, error) {
	if info != nil && info.TaskRelayInfo != nil {
		for i, item := range gjson.GetBytes(body, "content").Array() {
			kind := item.Get("type").String()
			if kind != "image_url" && kind != "video_url" && kind != "audio_url" {
				continue
			}
			uri := item.Get(kind + ".url").String()
			if strings.HasPrefix(uri, "asset://") {
				if id := info.AssetAliases[strings.TrimPrefix(uri, "asset://")]; id != "" {
					var err error
					body, err = sjson.SetBytes(body, fmt.Sprintf("content.%d.%s.url", i, kind), "asset://"+id)
					if err != nil {
						return nil, err
					}
				}
			}
		}
	}
	var fields map[string]json.RawMessage
	if err := common.Unmarshal(body, &fields); err != nil || fields == nil {
		return nil, fmt.Errorf("video request must be an object")
	}
	modelName := fields["model"]
	delete(fields, "model")
	var content []json.RawMessage
	if err := common.Unmarshal(fields["content"], &content); err != nil {
		return nil, fmt.Errorf("content must be an array")
	}
	var texts []string
	for index, raw := range content {
		item := gjson.ParseBytes(raw)
		if item.Get("type").String() == "text" {
			if item.Get("text").Type != gjson.String {
				return nil, fmt.Errorf("text content must be a string")
			}
			if text := item.Get("text").String(); strings.TrimSpace(text) != "" {
				texts = append(texts, text)
			}
		}
		if item.Get("type").String() == "draft_task" {
			if info == nil || info.ChannelMeta == nil {
				return nil, fmt.Errorf("draft task requires channel ownership")
			}
			task, exists, err := model.GetByTaskIDOrUpstreamID(info.UserId, item.Get("draft_task.id").String())
			if err != nil {
				return nil, err
			}
			if !exists || task.ChannelId != info.ChannelId || task.Status != model.TaskStatusSuccess || !task.PrivateData.SeedanceDraft ||
				(task.PrivateData.Key == "" || task.PrivateData.Key != info.ApiKey) {
				return nil, fmt.Errorf("draft task is unavailable on this channel")
			}
			content[index], err = sjson.SetBytes(raw, "draft_task.id", task.GetUpstreamTaskID())
			if err != nil {
				return nil, err
			}
			for _, name := range []string{"duration", "ratio", "generate_audio"} {
				if _, exists := fields[name]; exists {
					return nil, fmt.Errorf("draft upgrade cannot specify %s", name)
				}
			}
		}
	}
	// Existing content text remains authoritative. When media-only content is
	// accompanied by a prompt, send that same text in both upstream locations.
	if len(texts) == 0 && strings.TrimSpace(fallbackPrompt) != "" {
		text, err := common.Marshal(map[string]string{"type": "text", "text": fallbackPrompt})
		if err != nil {
			return nil, err
		}
		content = append(content, text)
		texts = append(texts, fallbackPrompt)
	}
	// Store only the draft flag, never the complete request or media URLs.
	c.Set("yh_video_draft", gjson.GetBytes(body, "draft").Type == gjson.True)
	var err error
	fields["content"], err = common.Marshal(content)
	if err != nil {
		return nil, err
	}
	metadata, err := common.Marshal(fields)
	if err != nil {
		return nil, err
	}
	result := map[string]json.RawMessage{"model": modelName, "metadata": metadata}
	if len(texts) > 0 {
		result["prompt"], err = common.Marshal(strings.Join(texts, "\n"))
		if err != nil {
			return nil, err
		}
	}
	return common.Marshal(result)
}

func BuildYHVideoTaskResponse(task *model.Task, body []byte) ([]byte, error) {
	if task == nil {
		return nil, fmt.Errorf("task is required")
	}
	root := gjson.ParseBytes(body)
	if !root.Get("data.task_id").Exists() {
		return BuildVolcengineVideoTaskResponse(task, body)
	}
	status := root.Get("data.status").String()
	if (task.Status == model.TaskStatusSuccess && status != "SUCCESS") || (task.Status == model.TaskStatusFailure && status != "FAILURE") {
		return BuildVolcengineVideoTaskResponse(task, nil)
	}
	statuses := map[string]string{"NOT_START": "queued", "SUBMITTED": "queued", "QUEUED": "queued", "IN_PROGRESS": "running", "SUCCESS": "succeeded", "FAILURE": "failed"}
	public, ok := statuses[status]
	if !ok {
		return nil, fmt.Errorf("unrecognized YH task status")
	}
	result := map[string]json.RawMessage{}
	if data := root.Get("data.data"); data.Exists() && data.Type != gjson.Null {
		if !data.IsObject() {
			return nil, fmt.Errorf("invalid YH task result")
		}
		// Only official business fields belong in the client response. Preserve
		// entire content/usage objects and numeric precision, including extensions.
		for _, key := range []string{"content", "usage", "duration", "frames", "framespersecond", "resolution", "ratio", "seed", "generate_audio", "draft", "draft_task_id", "service_tier", "execution_expires_after", "safety_identifier", "tools", "output_format", "created_at", "updated_at", "error"} {
			if value := data.Get(key); value.Exists() {
				result[key] = json.RawMessage(value.Raw)
			}
		}
	}
	result["id"], _ = common.Marshal(task.GetNativeTaskID())
	result["model"], _ = common.Marshal(task.Properties.OriginModelName)
	result["status"], _ = common.Marshal(public)
	if _, ok := result["created_at"]; !ok {
		result["created_at"], _ = common.Marshal(task.SubmitTime)
	}
	if _, ok := result["updated_at"]; !ok {
		result["updated_at"], _ = common.Marshal(task.UpdatedAt)
	}
	if status != "SUCCESS" {
		delete(result, "content")
	}
	if status == "FAILURE" {
		if _, ok := result["error"]; !ok || string(result["error"]) == "null" {
			result["error"], _ = common.Marshal(map[string]string{"code": "TaskFailed", "message": firstJSONString(body, "data.fail_reason", "data.error.message")})
		}
	}
	return common.Marshal(result)
}

package configurable

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
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

// Decode raw fields with the same case-insensitive matching as TaskSubmitReq.
// RawMessage also distinguishes missing fields from explicit zero/null values.
type yhVideoRequestFields struct {
	Duration json.RawMessage `json:"duration"`
	Seconds  json.RawMessage `json:"seconds"`
	Metadata json.RawMessage `json:"metadata"`
}

func (r yhVideoRequestFields) hasTopLevelDuration() bool {
	return len(r.Duration) > 0 || len(r.Seconds) > 0
}

func parseYHVideoDuration(raw []byte) (int, error) {
	if gjson.ParseBytes(raw).Type == gjson.String {
		var text string
		if err := common.Unmarshal(raw, &text); err != nil {
			return 0, err
		}
		return strconv.Atoi(text)
	}
	var duration int
	err := common.Unmarshal(raw, &duration)
	return duration, err
}

func validateYHVideoTaskRequest(c *gin.Context, info *relaycommon.RelayInfo, action string) *dto.TaskError {
	return relaycommon.ValidateSeedanceTaskRequestWithNormalizer(c, info, action, func(req *relaycommon.TaskSubmitReq) error {
		var original yhVideoRequestFields
		if err := common.UnmarshalBodyReusable(c, &original); err != nil {
			return err
		}
		// Match outbound precedence, including explicitly empty/null top-level
		// fields. Never add a default duration to draft upgrade requests.
		if original.hasTopLevelDuration() || req.Metadata["duration"] == nil {
			return nil
		}
		raw, err := common.Marshal(req.Metadata["duration"])
		if err != nil {
			return fmt.Errorf("metadata.duration must be an integer")
		}
		duration, err := parseYHVideoDuration(raw)
		if err != nil {
			return fmt.Errorf("metadata.duration must be an integer")
		}
		req.Duration = duration
		return nil
	})
}

// Restore raw metadata after mapping to retain numeric precision and extensions,
// while keeping the shared mapping's precedence for known generic fields.
func mergeYHVideoMetadata(c *gin.Context, req relaycommon.TaskSubmitReq, body []byte) ([]byte, error) {
	var original yhVideoRequestFields
	if err := common.UnmarshalBodyReusable(c, &original); err != nil {
		return nil, err
	}
	raw := original.Metadata
	if gjson.ParseBytes(raw).Type == gjson.String {
		var encoded string
		if err := common.Unmarshal(raw, &encoded); err != nil {
			return nil, err
		}
		raw = []byte(strings.TrimSpace(encoded))
	}
	var metadata, mapped map[string]json.RawMessage
	if len(raw) > 0 {
		if err := common.Unmarshal(raw, &metadata); err != nil {
			return nil, err
		}
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
			if original.hasTopLevelDuration() {
				continue
			}
		}
		mapped[key] = value
	}
	// The shared DTO omits duration=0 on marshal. Restore only an explicitly
	// supplied integer zero; seconds still wins and null/absent stay omitted.
	if req.Duration == 0 && strings.TrimSpace(req.Seconds) == "" && len(original.Duration) > 0 && gjson.ParseBytes(original.Duration).Type != gjson.Null {
		if duration, err := parseYHVideoDuration(original.Duration); err == nil && duration == 0 {
			mapped["duration"] = json.RawMessage("0")
		}
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
	// Store only task attributes needed locally, never the request or media URLs.
	c.Set("yh_video_draft", gjson.GetBytes(body, "draft").Type == gjson.True)
	tier := gjson.GetBytes(body, "service_tier")
	c.Set("yh_video_service_tier", "")
	if tier.Type == gjson.String {
		c.Set("yh_video_service_tier", tier.String())
	}
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

// Reject malformed provider results before any foreground/background poll can
// persist a terminal state and settle its billing.
func validateYHVideoTaskResult(body []byte) error {
	if data := gjson.GetBytes(body, "data.data"); data.Exists() && data.Type != gjson.Null && !data.IsObject() {
		return fmt.Errorf("invalid YH task result")
	}
	return nil
}

// Listing must use persisted state: Data can still contain the create response
// after a running poll or a local timeout. Only reuse compatible result data.
func BuildYHVideoTaskListResponse(task *model.Task) ([]byte, error) {
	if task == nil {
		return nil, fmt.Errorf("task is required")
	}
	response, err := BuildYHVideoTaskResponse(task, task.Data)
	status := gjson.GetBytes(response, "status").String()
	matches := status == volcengineVideoTaskStatus("", task.Status)
	// Legacy flat responses can carry a more specific terminal failure state.
	if task.Status == model.TaskStatusFailure && (status == "cancelled" || status == "expired") {
		matches = true
	}
	if err != nil || !matches {
		// Also tolerate malformed payloads saved before result validation.
		return BuildYHVideoTaskResponse(task, nil)
	}
	return response, nil
}

func BuildYHVideoTaskResponse(task *model.Task, body []byte) ([]byte, error) {
	response, err := buildYHVideoTaskResponse(task, body)
	if err != nil {
		return nil, err
	}
	if task.PrivateData.YHServiceTier != "" && !gjson.GetBytes(response, "service_tier").Exists() {
		return sjson.SetBytes(response, "service_tier", task.PrivateData.YHServiceTier)
	}
	return response, nil
}

func buildYHVideoTaskResponse(task *model.Task, body []byte) ([]byte, error) {
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

package common

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/tidwall/gjson"
)

// IsSeedanceVideoProfile identifies the built-in Seedance protocol templates.
func IsSeedanceVideoProfile(id string) bool {
	switch id {
	case "doubao-seedance-2", "doubao-seedance-2-api-assets", "seedance2-modelsell",
		"seedance2-ark-task-assets", "seedance2-service-inference", "doubao-seedance-max-service-inference":
		return true
	}
	return false
}

// HasSeedanceInput follows the same full-content-over-shorthand precedence as
// the upstream mappings. A draft task or audio/video input need not have text.
func HasSeedanceInput(req TaskSubmitReq) bool {
	if content, ok := req.Metadata["content"]; ok && content != nil {
		data, err := common.Marshal(content)
		if err != nil {
			return false
		}
		items := gjson.ParseBytes(data)
		if items.IsArray() && len(items.Array()) > 0 {
			for _, item := range items.Array() {
				var path string
				switch item.Get("type").String() {
				case "text":
					path = "text"
				case "image_url", "video_url", "audio_url":
					path = item.Get("type").String() + ".url"
				case "draft_task":
					path = "draft_task.id"
				}
				if path != "" && strings.TrimSpace(item.Get(path).String()) != "" {
					return true
				}
			}
			return false
		}
	}
	return strings.TrimSpace(req.Prompt) != "" || len(req.Images) > 0 || strings.TrimSpace(req.Image) != "" ||
		len(SeedanceReferenceVideoURLs(req.Metadata["video_url"])) > 0 ||
		len(SeedanceReferenceVideoURLs(req.Metadata["audio_url"])) > 0
}

// SeedanceVideoMetadata exposes documented result fields across Ark, Service
// Inference/Max, and saved gateway envelopes. Never copy arbitrary upstream
// metadata: it can contain provider IDs, request inputs, or private fields.
func SeedanceVideoMetadata(data []byte) map[string]any {
	roots := []string{"", "task.", "task.metadata.", "data.", "data.data.task.", "data.data.task.metadata."}
	metadata := map[string]any{}
	first := func(paths ...string) (any, bool) {
		for _, root := range roots {
			for _, path := range paths {
				value := gjson.GetBytes(data, root+path)
				if value.Exists() && value.Type != gjson.Null {
					if value.Type == gjson.String && strings.TrimSpace(value.String()) == "" {
						continue
					}
					return value.Value(), true
				}
			}
		}
		return nil, false
	}
	if value, ok := first("last_frame_url", "content.last_frame_url"); ok {
		metadata["last_frame_url"] = value
	}
	usage := map[string]any{}
	for _, field := range []string{"completion_tokens", "total_tokens", "tool_usage"} {
		if value, ok := first("usage." + field); ok {
			usage[field] = value
		}
	}
	if len(usage) > 0 {
		metadata["usage"] = usage
	}
	for _, field := range []string{"output_format", "resolution", "ratio", "duration", "duration_seconds", "frames", "framespersecond",
		"seed", "watermark", "camera_fixed", "generate_audio", "draft", "draft_task_id", "service_tier", "execution_expires_after",
		"priority", "safety_identifier", "tools", "outputs", "prep", "revised_prompt"} {
		if value, ok := first(field); ok {
			metadata[field] = value
		}
	}
	return metadata
}

// RequestedDuration follows the same seconds-over-duration precedence as the
// video request mappings. It preserves Seedance's automatic-duration sentinel.
func (req TaskSubmitReq) RequestedDuration() int {
	if req.Seconds != "" {
		if seconds, err := strconv.Atoi(req.Seconds); err == nil {
			return seconds
		}
	}
	return req.Duration
}

// ValidateTaskDurationBoundsForRelay also identifies a Seedance channel when
// public aliases or Volcengine endpoint IDs do not contain the model family.
func ValidateTaskDurationBoundsForRelay(req TaskSubmitReq, info *RelayInfo) *dto.TaskError {
	if info == nil {
		return ValidateTaskDurationBounds(req)
	}
	models := []string{info.OriginModelName, info.CurrentModel(), info.GetUpstreamModelName()}
	if info.ChannelMeta != nil {
		if info.ChannelType == constant.ChannelTypeVolcEngine {
			models = append(models, "seedance")
		} else if info.ChannelType == constant.ChannelTypeConfigurable && info.ChannelSetting.Protocol != nil {
			models = append(models, info.ChannelSetting.Protocol.ProfileID)
		}
	}
	return ValidateTaskDurationBounds(req, models...)
}

func validateSeedanceOmniReferenceTask(req TaskSubmitReq) *dto.TaskError {
	if req.OmniReferenceTaskType == nil {
		return nil
	}
	invalid := func(message string) *dto.TaskError {
		return createTaskError(fmt.Errorf("omni_reference_task_type: %s", message), "invalid_request", http.StatusBadRequest, true)
	}
	taskType := *req.OmniReferenceTaskType
	switch taskType {
	case "auto", "reference", "edit", "extend":
	default:
		return invalid("must be auto, reference, edit or extend")
	}
	if req.Seconds != "" {
		seconds, err := strconv.Atoi(req.Seconds)
		if err != nil || (req.Duration != 0 && req.Duration != seconds) {
			return invalid("seconds and duration must specify the same integer duration")
		}
	}
	if taskType != "edit" && taskType != "extend" {
		return nil
	}
	ratio, _ := req.Metadata["ratio"].(string)
	if ratio == "" {
		ratio, _ = req.Metadata["aspect_ratio"].(string)
	}
	if ratio != "adaptive" {
		return invalid(taskType + " requires ratio=adaptive")
	}
	if taskType == "edit" && req.RequestedDuration() != -1 {
		return invalid("edit requires duration=-1")
	}
	// Inspect the exact content used for forwarding. If callers use the generic
	// shorthand, the Seedance mappings construct reference_video entries for it.
	if content, exists := req.Metadata["content"]; exists && content != nil {
		data, err := common.Marshal(content)
		var items []struct {
			Type     string `json:"type"`
			Role     string `json:"role"`
			VideoURL *struct {
				URL string `json:"url"`
			} `json:"video_url"`
		}
		if err != nil || common.Unmarshal(data, &items) != nil {
			return invalid("content must be an array of media items")
		}
		if len(items) > 0 {
			for _, item := range items {
				if item.Type == "video_url" && item.Role == "reference_video" && item.VideoURL != nil && item.VideoURL.URL != "" {
					return nil
				}
			}
			return invalid(taskType + " requires a reference_video in content")
		}
	}
	if len(SeedanceReferenceVideoURLs(req.Metadata["video_url"])) > 0 {
		return nil
	}
	return invalid(taskType + " requires a reference_video in content")
}

// SeedanceReferenceVideoURLs reads the generic video_url shorthand used when
// callers do not supply a content array.
func SeedanceReferenceVideoURLs(value any) []string {
	var urls []string
	switch videos := value.(type) {
	case string:
		if videos != "" {
			urls = append(urls, videos)
		}
	case []string:
		for _, video := range videos {
			if video != "" {
				urls = append(urls, video)
			}
		}
	case []any:
		for _, video := range videos {
			if url, ok := video.(string); ok && url != "" {
				urls = append(urls, url)
			}
		}
	}
	return urls
}

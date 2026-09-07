package common

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
)

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

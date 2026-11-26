package configurable

import (
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

type magiOptions struct {
	ImageURL      string   `json:"image_url,omitempty"`
	EndImageURL   string   `json:"end_image_url,omitempty"`
	ImageURLs     []string `json:"image_urls,omitempty"`
	VideoURLs     []string `json:"video_urls,omitempty"`
	AudioURLs     []string `json:"audio_urls,omitempty"`
	Resolution    string   `json:"resolution,omitempty"`
	AspectRatio   string   `json:"aspect_ratio,omitempty"`
	Duration      *int     `json:"duration,omitempty"`
	GenerateAudio *bool    `json:"generate_audio,omitempty"`
	Seed          *int64   `json:"seed,omitempty"`
	EnhancePrompt *bool    `json:"enhance_prompt,omitempty"`
}

func validateMagiVideo(c *gin.Context, info *relaycommon.RelayInfo, action string) *dto.TaskError {
	if c.ContentType() != "application/json" {
		return service.TaskErrorWrapperLocal(fmt.Errorf("MAGI-2 expects a JSON request"), "invalid_request", http.StatusUnsupportedMediaType)
	}
	var req relaycommon.TaskSubmitReq
	if err := common.UnmarshalBodyReusable(c, &req); err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
	}
	if mode := c.GetString(relaycommon.MagiNativeModeKey); mode != "" {
		if (req.Mode != "" && req.Mode != mode) || (req.Model != "" && req.Model != relaycommon.MagiModel) {
			return service.TaskErrorWrapperLocal(fmt.Errorf("model and mode must match the MAGI-2 endpoint"), "invalid_request", http.StatusBadRequest)
		}
		req.Model, req.Mode = relaycommon.MagiModel, mode
	}
	var body map[string]any
	if err := common.UnmarshalBodyReusable(c, &body); err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
	}
	metadata, ok := body["metadata"].(map[string]any)
	if !ok && body["metadata"] != nil {
		return service.TaskErrorWrapperLocal(fmt.Errorf("metadata must be a JSON object"), "invalid_request", http.StatusBadRequest)
	}
	req.Metadata = make(map[string]any)
	for _, key := range []string{"duration", "image_url", "end_image_url", "image_urls", "video_urls", "audio_urls", "aspect_ratio", "resolution", "generate_audio", "seed", "enhance_prompt"} {
		value := body[key]
		if value == nil {
			value = metadata[key]
		}
		if value != nil {
			req.Metadata[key] = value
		}
	}
	req.Seconds, _ = body["seconds"].(string)
	var opts magiOptions
	if err := req.UnmarshalMetadata(&opts); err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
	}
	modelSell := info.ChannelMeta != nil && info.ChannelSetting.Protocol != nil && info.ChannelSetting.Protocol.ProfileID == relaycommon.ModelSellMagiProfileID
	official := info.ChannelMeta != nil && info.ChannelSetting.Protocol != nil && info.ChannelSetting.Protocol.ProfileID == relaycommon.OfficialMagiProfileID
	if opts.Seed != nil && (*opts.Seed > 4294967295 || *opts.Seed < 0 && !(official && *opts.Seed == -1)) {
		return service.TaskErrorWrapperLocal(fmt.Errorf("invalid MAGI-2 seed"), "invalid_request", http.StatusBadRequest)
	}
	if official {
		for _, image := range req.ReferenceImages {
			opts.ImageURLs = append(opts.ImageURLs, image.URL)
		}
		if req.Video != nil {
			opts.VideoURLs = append(opts.VideoURLs, req.Video.URL)
		}
	}
	if err := opts.normalize(&req, modelSell); err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
	}
	req.Metadata["image_url"] = opts.ImageURL
	req.Metadata["end_image_url"] = opts.EndImageURL
	req.Metadata["image_urls"] = opts.ImageURLs
	req.Metadata["video_urls"] = opts.VideoURLs
	req.Metadata["resolution"] = opts.Resolution
	info.Action = action
	c.Set("task_request", req)
	return nil
}

func (opts *magiOptions) normalize(req *relaycommon.TaskSubmitReq, modelSell bool) error {
	duration := 5
	if opts.Duration != nil {
		duration = *opts.Duration
	}
	if req.Seconds != "" {
		seconds, err := strconv.Atoi(req.Seconds)
		if err != nil || (opts.Duration != nil && duration != seconds) {
			return fmt.Errorf("seconds must be an integer and agree with duration")
		}
		duration = seconds
	}
	if duration < 4 || duration > 30 {
		return fmt.Errorf("MAGI-2 duration must be between 4 and 30 seconds")
	}
	req.Duration = duration
	if opts.Resolution == "" {
		opts.Resolution = req.Size
	}
	if opts.Resolution != "" && opts.Resolution != "1080p" {
		return fmt.Errorf("MAGI-2 supports 1080p resolution")
	}
	if !slices.Contains([]string{"", "21:9", "16:9", "4:3", "1:1", "3:4", "9:16"}, opts.AspectRatio) {
		return fmt.Errorf("unsupported MAGI-2 aspect_ratio")
	}
	if opts.ImageURL == "" {
		opts.ImageURL = req.Image
	}
	if opts.ImageURL == "" {
		opts.ImageURL = req.InputReference
	}
	if modelSell && len(req.Images) > 0 && req.Mode != "reference-to-video" {
		if len(req.Images) > 2 || opts.ImageURL != "" || (len(req.Images) == 2 && opts.EndImageURL != "") {
			return fmt.Errorf("images accepts one start frame and one optional end frame; specify each frame once")
		}
		opts.ImageURL = req.Images[0]
		if len(req.Images) == 2 {
			opts.EndImageURL = req.Images[1]
		}
	} else if len(req.Images) > 0 && len(opts.ImageURLs) == 0 {
		if len(req.Images) == 1 && opts.ImageURL == "" && req.Mode != "reference-to-video" {
			opts.ImageURL = req.Images[0]
		} else {
			opts.ImageURLs = req.Images
		}
	}
	hasRefs := len(opts.ImageURLs)+len(opts.VideoURLs)+len(opts.AudioURLs) > 0
	hasFrames := opts.ImageURL != "" || opts.EndImageURL != ""
	if req.Mode == "" {
		switch {
		case hasRefs:
			req.Mode = "reference-to-video"
		case hasFrames:
			req.Mode = "image-to-video"
		default:
			req.Mode = "text-to-video"
		}
	}
	switch req.Mode {
	case "text-to-video":
		if strings.TrimSpace(req.Prompt) == "" || hasRefs || hasFrames {
			return fmt.Errorf("text-to-video requires a prompt and accepts no media")
		}
	case "image-to-video":
		if opts.ImageURL == "" || hasRefs {
			return fmt.Errorf("image-to-video requires image_url and accepts an optional end_image_url")
		}
	case "reference-to-video":
		if !hasRefs || hasFrames || len(opts.ImageURLs) > 10 || len(opts.VideoURLs) > 5 || len(opts.AudioURLs) > 5 {
			return fmt.Errorf("reference-to-video requires references: up to 10 images, 5 videos and 5 audio clips")
		}
	default:
		return fmt.Errorf("unsupported MAGI-2 video mode")
	}
	if req.Prompt != "" && (strings.TrimSpace(req.Prompt) == "" || utf8.RuneCountInString(req.Prompt) > 15000) {
		return fmt.Errorf("MAGI-2 prompt must contain non-whitespace text and at most 15000 characters")
	}
	for _, urls := range [][]string{opts.ImageURLs, opts.VideoURLs, opts.AudioURLs} {
		for _, raw := range urls {
			if !isMagiMediaURL(raw) {
				return fmt.Errorf("MAGI-2 references require HTTPS URLs")
			}
		}
	}
	for _, raw := range []string{opts.ImageURL, opts.EndImageURL} {
		if raw != "" && !isMagiMediaURL(raw) {
			return fmt.Errorf("MAGI-2 frames require HTTPS URLs")
		}
	}
	return nil
}

func isMagiMediaURL(raw string) bool {
	u, err := url.ParseRequestURI(raw)
	return err == nil && u.Scheme == "https" && u.Hostname() != ""
}

// MagiTaskResponse exposes only gateway-owned task identifiers.
func MagiTaskResponse(task *model.Task) ([]byte, error) {
	status := task.Status.ToVideoStatus()
	switch task.Status {
	case model.TaskStatusNotStart:
		status = "queued"
	case model.TaskStatusFailure:
		if raw := gjson.GetBytes(task.Data, "status").String(); raw == "canceled" || raw == "nsfw" {
			status = raw
		}
	}
	body := map[string]any{"request_id": task.TaskID, "status_url": "/requests/" + task.TaskID + "/status", "status": status}
	if task.Status == model.TaskStatusSuccess && task.PrivateData.ResultURL != "" {
		body["video"] = map[string]string{"url": task.PrivateData.ResultURL}
	}
	if task.Status == model.TaskStatusFailure && task.FailReason != "" {
		body["error"] = task.FailReason
	}
	return common.Marshal(body)
}

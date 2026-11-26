package configurable

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

func isSandSubmitPath(path string) bool {
	return path == relaycommon.SandGenerationsPath || path == relaycommon.SandArkTasksPath
}

func (a *TaskAdaptor) validateSandGeneration(c *gin.Context, info *relaycommon.RelayInfo) *dto.TaskError {
	invalid := func(err error) *dto.TaskError {
		return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
	}
	if c.ContentType() != "application/json" {
		return invalid(fmt.Errorf("Sand AI expects a JSON request"))
	}
	storage, err := common.GetBodyStorage(c)
	if err != nil {
		return invalid(err)
	}
	raw, err := storage.Bytes()
	if err != nil {
		return invalid(err)
	}
	var req relaycommon.TaskSubmitReq
	path := c.Request.URL.Path
	if isSandSubmitPath(path) {
		req, err = sandNativeTaskRequest(raw, path == relaycommon.SandArkTasksPath)
		if err != nil {
			return invalid(err)
		}
		c.Set("task_request", req)
	} else {
		if taskErr := validateMagiVideo(c, info, constant.TaskActionGenerate); taskErr != nil {
			return taskErr
		}
		req, err = relaycommon.GetTaskRequest(c)
		if err != nil {
			return invalid(err)
		}
		path = relaycommon.SandGenerationsPath
		// Native parts require text and cannot designate start or end frames.
		if req.Mode == "image-to-video" || strings.TrimSpace(req.Prompt) == "" {
			path = relaycommon.SandArkTasksPath
		}
	}
	a.selectedSubmitPath = path
	c.Set("sand_ark", path == relaycommon.SandArkTasksPath)
	info.Action = constant.TaskActionGenerate
	if info.OriginModelName == "" {
		info.OriginModelName, info.RequestModelName = req.Model, req.Model
	}
	// Snapshot the duration actually sent upstream, including generic defaults
	// and the Ark prompt suffix, for expressions using param("duration").
	billingBody, err := sjson.SetBytes(raw, "duration", req.Duration)
	if err != nil {
		return invalid(err)
	}
	info.BillingRequestInput = &billingexpr.RequestInput{Body: billingBody}
	return nil
}

func sandNativeTaskRequest(body []byte, ark bool) (relaycommon.TaskSubmitReq, error) {
	req := relaycommon.TaskSubmitReq{Model: gjson.GetBytes(body, "model").String()}
	field := "parts"
	if ark {
		field = "content"
	}
	items := gjson.GetBytes(body, field)
	if !items.IsArray() || len(items.Array()) == 0 || len(items.Array()) > 21 {
		return req, fmt.Errorf("%s must contain 1 to 21 items", field)
	}
	var frame, reference bool
	for _, item := range items.Array() {
		if item.Get("type").String() == "text" {
			req.Prompt = item.Get("text").String()
			continue
		}
		switch item.Get("role").String() {
		case "first_frame", "last_frame":
			frame = true
		default:
			reference = true
		}
	}
	if frame && reference {
		return req, fmt.Errorf("frame inputs cannot be combined with general references")
	}
	if !ark && strings.TrimSpace(req.Prompt) == "" {
		return req, fmt.Errorf("parts requires a nonblank text item")
	}
	d := gjson.GetBytes(body, "duration")
	var err error
	switch {
	case d.Type == gjson.Number:
		req.Duration, err = strconv.Atoi(d.Raw)
	case ark && !d.Exists():
		// Only options at the end of the text are interpreted by the Ark API.
		raw := strings.Fields(req.Prompt)
		for len(raw) >= 2 {
			flag, value := raw[len(raw)-2], raw[len(raw)-1]
			if flag != "--dur" && flag != "--rs" && flag != "--rt" {
				break
			}
			if flag == "--dur" {
				req.Duration, err = strconv.Atoi(value)
				break
			}
			raw = raw[:len(raw)-2]
		}
	}
	if err != nil || req.Duration < 1 || (ark && (req.Duration < 4 || req.Duration > 30)) {
		return req, fmt.Errorf("invalid Sand AI generation duration")
	}
	return req, nil
}

func buildSandGenerationBody(c *gin.Context, info *relaycommon.RelayInfo, path string) (io.Reader, error) {
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return nil, err
	}
	model := info.GetUpstreamModelName()
	if model == "" {
		model = req.Model
	}
	if isSandSubmitPath(c.Request.URL.Path) {
		storage, err := common.GetBodyStorage(c)
		if err != nil {
			return nil, err
		}
		body, err := storage.Bytes()
		if err != nil {
			return nil, err
		}
		body, err = sjson.SetBytes(body, "model", model)
		if err == nil && !gjson.GetBytes(body, "duration").Exists() {
			body, err = sjson.SetBytes(body, "duration", req.Duration)
		}
		return bytes.NewReader(body), err
	}
	var opts magiOptions
	if err := req.UnmarshalMetadata(&opts); err != nil {
		return nil, err
	}
	if opts.AspectRatio == "" {
		opts.AspectRatio = "16:9"
	}
	ark := path == relaycommon.SandArkTasksPath
	body := map[string]any{"model": model, "duration": req.Duration}
	items := make([]map[string]any, 0)
	if req.Prompt != "" {
		items = append(items, map[string]any{"type": "text", "text": req.Prompt})
	}
	add := func(kind, role, url string) {
		if url == "" {
			return
		}
		source := map[string]string{"url": url}
		if ark {
			items = append(items, map[string]any{"type": kind + "_url", "role": role, kind + "_url": source})
		} else {
			items = append(items, map[string]any{"type": role, kind: source})
		}
	}
	add("image", "first_frame", opts.ImageURL)
	add("image", "last_frame", opts.EndImageURL)
	for _, url := range opts.ImageURLs {
		add("image", "reference_image", url)
	}
	for _, url := range opts.VideoURLs {
		add("video", "reference_video", url)
	}
	for _, url := range opts.AudioURLs {
		add("audio", "reference_audio", url)
	}
	audio, enhance := "generateAudio", "enhancePrompt"
	if ark {
		body["content"], body["ratio"] = items, opts.AspectRatio
		audio, enhance = "generate_audio", "enhance_prompt"
	} else {
		body["parts"], body["aspectRatio"] = items, opts.AspectRatio
	}
	if opts.GenerateAudio != nil {
		body[audio] = *opts.GenerateAudio
	}
	if opts.EnhancePrompt != nil {
		body[enhance] = *opts.EnhancePrompt
	}
	if opts.Seed != nil {
		body["seed"] = *opts.Seed
	}
	if opts.Resolution != "" {
		body["resolution"] = opts.Resolution
	}
	data, err := common.Marshal(body)
	return bytes.NewReader(data), err
}

package doubao

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"

	"github.com/QuantumNous/new-api/constant"
	taskdto "github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
	"github.com/samber/lo"
)

// ============================
// Request / Response structures
// ============================

type ContentItem struct {
	Type      string     `json:"type,omitempty"`
	Text      string     `json:"text,omitempty"`
	ImageURL  *MediaURL  `json:"image_url,omitempty"`
	VideoURL  *MediaURL  `json:"video_url,omitempty"`
	AudioURL  *MediaURL  `json:"audio_url,omitempty"`
	Role      string     `json:"role,omitempty"`
	DraftTask *DraftTask `json:"draft_task,omitempty"`
}

type DraftTask struct {
	ID string `json:"id"`
}

type MediaURL struct {
	URL string `json:"url,omitempty"`
}

type requestPayload struct {
	Model                 string         `json:"model"`
	Content               []ContentItem  `json:"content,omitempty"`
	CallbackURL           string         `json:"callback_url,omitempty"`
	ReturnLastFrame       *dto.BoolValue `json:"return_last_frame,omitempty"`
	ServiceTier           string         `json:"service_tier,omitempty"`
	ExecutionExpiresAfter *dto.IntValue  `json:"execution_expires_after,omitempty"`
	GenerateAudio         *dto.BoolValue `json:"generate_audio,omitempty"`
	Draft                 *dto.BoolValue `json:"draft,omitempty"`
	Tools                 []struct {
		Type string `json:"type,omitempty"`
	} `json:"tools,omitempty"`
	SafetyIdentifier      string         `json:"safety_identifier,omitempty"`
	Priority              *dto.IntValue  `json:"priority,omitempty"`
	Resolution            string         `json:"resolution,omitempty"`
	Ratio                 string         `json:"ratio,omitempty"`
	Duration              *dto.IntValue  `json:"duration,omitempty"`
	Frames                *dto.IntValue  `json:"frames,omitempty"`
	Seed                  *dto.IntValue  `json:"seed,omitempty"`
	CameraFixed           *dto.BoolValue `json:"camera_fixed,omitempty"`
	Watermark             *dto.BoolValue `json:"watermark,omitempty"`
	OutputFormat          *string        `json:"output_format,omitempty"`
	OmniReferenceTaskType *string        `json:"omni_reference_task_type,omitempty"`
}

type responsePayload struct {
	ID string `json:"id"` // task_id
}

type responseTask struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Status  string `json:"status"`
	Content struct {
		VideoURL string `json:"video_url"`
	} `json:"content"`
	Seed            int    `json:"seed"`
	Resolution      string `json:"resolution"`
	Duration        int    `json:"duration"`
	Ratio           string `json:"ratio"`
	FramesPerSecond int    `json:"framespersecond"`
	ServiceTier     string `json:"service_tier"`
	Tools           []struct {
		Type string `json:"type"`
	} `json:"tools"`
	Usage struct {
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
		ToolUsage        struct {
			WebSearch int `json:"web_search"`
		} `json:"tool_usage"`
	} `json:"usage"`
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
	CreatedAt int64 `json:"created_at"`
	UpdatedAt int64 `json:"updated_at"`
}

// ============================
// Adaptor implementation
// ============================

type TaskAdaptor struct {
	taskcommon.BaseBilling
	ChannelType int
	apiKey      string
	baseURL     string
}

func (a *TaskAdaptor) Init(info *relaycommon.RelayInfo) {
	a.ChannelType = info.ChannelType
	a.baseURL = info.ChannelBaseUrl
	a.apiKey = info.ApiKey
}

// ValidateRequestAndSetAction parses body, validates fields and sets default action.
func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) (taskErr *taskdto.TaskError) {
	if isSeedanceNativeTaskRequest(c) {
		req, err := seedanceNativeTaskSubmitReq(c)
		if err != nil {
			return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
		}
		if taskErr := relaycommon.ValidateTaskDurationBoundsForRelay(req, info); taskErr != nil {
			return taskErr
		}
		info.Action = constant.TaskActionGenerate
		c.Set("task_request", req)
		return nil
	}
	// Accept only POST /v1/video/generations as "generate" action.
	return relaycommon.ValidateSeedanceTaskRequest(c, info, constant.TaskActionGenerate)
}

// BuildRequestURL constructs the upstream URL.
func (a *TaskAdaptor) BuildRequestURL(_ *relaycommon.RelayInfo) (string, error) {
	return fmt.Sprintf("%s/api/v3/contents/generations/tasks", a.baseURL), nil
}

// BuildRequestHeader sets required headers.
func (a *TaskAdaptor) BuildRequestHeader(_ *gin.Context, req *http.Request, _ *relaycommon.RelayInfo) error {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	return nil
}

// EstimateBilling 根据请求 metadata 中的输出分辨率与是否包含视频输入，返回相对基准价的计费 OtherRatio。
func (a *TaskAdaptor) EstimateBilling(c *gin.Context, info *relaycommon.RelayInfo) map[string]float64 {
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return nil
	}
	ratios := map[string]float64{}
	seconds := req.RequestedDuration()
	if seconds == -1 {
		seconds = relaycommon.MaxSeedanceTaskDurationSeconds
	}
	if seconds > 0 {
		ratios["seconds"] = float64(seconds)
	}
	hasVideo := hasVideoInMetadata(req.Metadata)
	resolution := req.Size
	if resolution == "" {
		resolution, _ = req.Metadata["resolution"].(string)
	}
	if ratio, ok := GetVideoInputRatio(info.OriginModelName, resolution, hasVideo); ok && ratio != 1 {
		ratios["video_input"] = ratio
	}
	if len(ratios) == 0 {
		return nil
	}
	return ratios
}

// hasVideoInMetadata 直接检查 metadata 的 content 数组是否包含 video_url 条目，
// 避免构建完整的上游 requestPayload。
func hasVideoInMetadata(metadata map[string]interface{}) bool {
	if metadata == nil {
		return false
	}
	contentRaw, ok := metadata["content"]
	if !ok || contentRaw == nil {
		return len(relaycommon.SeedanceReferenceVideoURLs(metadata["video_url"])) > 0
	}
	contentSlice, ok := contentRaw.([]interface{})
	if !ok {
		return false
	}
	if len(contentSlice) == 0 {
		return len(relaycommon.SeedanceReferenceVideoURLs(metadata["video_url"])) > 0
	}
	for _, item := range contentSlice {
		itemMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		if itemMap["type"] == "video_url" {
			return true
		}
		if _, has := itemMap["video_url"]; has {
			return true
		}
	}
	return false
}

// BuildRequestBody converts request into Doubao specific format.
func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	if isSeedanceNativeTaskRequest(c) {
		var body map[string]any
		if err := common.UnmarshalBodyReusable(c, &body); err != nil {
			return nil, err
		}
		if info.IsModelMapped {
			body["model"] = info.UpstreamModelName
		} else {
			info.UpstreamModelName, _ = body["model"].(string)
		}
		data, err := common.Marshal(body)
		if err != nil {
			return nil, err
		}
		return bytes.NewReader(data), nil
	}
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return nil, err
	}

	body, err := a.convertToRequestPayload(&req)
	if err != nil {
		return nil, errors.Wrap(err, "convert request payload failed")
	}
	if info.IsModelMapped {
		body.Model = info.UpstreamModelName
	} else {
		info.UpstreamModelName = body.Model
	}
	data, err := common.Marshal(body)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(data), nil
}

// DoRequest delegates to common helper.
func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	return channel.DoTaskApiRequest(a, c, info, requestBody)
}

// DoResponse handles upstream response, returns taskID etc.
func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (taskID string, taskData []byte, taskErr *taskdto.TaskError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		taskErr = service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
		return
	}
	_ = resp.Body.Close()

	// Parse Doubao response
	var dResp responsePayload
	if err := common.Unmarshal(responseBody, &dResp); err != nil {
		taskErr = service.TaskErrorWrapper(errors.Wrapf(err, "body: %s", responseBody), "unmarshal_response_body_failed", http.StatusInternalServerError)
		return
	}

	if dResp.ID == "" {
		taskErr = service.TaskErrorWrapper(fmt.Errorf("task_id is empty"), "invalid_response", http.StatusInternalServerError)
		return
	}

	ov := dto.NewOpenAIVideo()
	ov.ID = info.PublicTaskID
	ov.TaskID = info.PublicTaskID
	ov.CreatedAt = time.Now().Unix()
	ov.Model = info.OriginModelName

	c.JSON(http.StatusOK, ov)
	return dResp.ID, responseBody, nil
}

// FetchTask fetch task status
func (a *TaskAdaptor) FetchTask(baseUrl, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid task_id")
	}

	uri := fmt.Sprintf("%s/api/v3/contents/generations/tasks/%s", baseUrl, taskID)

	req, err := http.NewRequest(http.MethodGet, uri, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)

	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		return nil, fmt.Errorf("new proxy http client failed: %w", err)
	}
	return client.Do(req)
}

func (a *TaskAdaptor) GetModelList() []string {
	return ModelList
}

func (a *TaskAdaptor) GetChannelName() string {
	return ChannelName
}

func (a *TaskAdaptor) convertToRequestPayload(req *relaycommon.TaskSubmitReq) (*requestPayload, error) {
	r := requestPayload{
		Model:   req.Model,
		Content: []ContentItem{},
	}

	metadata := req.Metadata
	if err := taskcommon.UnmarshalMetadata(metadata, &r); err != nil {
		return nil, errors.Wrap(err, "unmarshal metadata failed")
	}
	if r.Ratio == "" {
		r.Ratio, _ = metadata["aspect_ratio"].(string)
	}
	if req.Size != "" {
		r.Resolution = req.Size
	}
	// Full content owns text, order and roles. Build shorthand only when it is
	// absent or empty, consistently with the configurable Seedance profiles.
	if len(r.Content) == 0 {
		if strings.TrimSpace(req.Prompt) != "" {
			r.Content = append(r.Content, ContentItem{Type: "text", Text: req.Prompt})
		}
		images := req.Images
		if len(images) == 0 && strings.TrimSpace(req.Image) != "" {
			images = []string{req.Image}
		}
		for _, url := range images {
			r.Content = append(r.Content, ContentItem{Type: "image_url", ImageURL: &MediaURL{URL: url}, Role: "reference_image"})
		}
		for _, url := range relaycommon.SeedanceReferenceVideoURLs(metadata["video_url"]) {
			r.Content = append(r.Content, ContentItem{
				Type: "video_url", VideoURL: &MediaURL{URL: url}, Role: "reference_video",
			})
		}
		for _, url := range relaycommon.SeedanceReferenceVideoURLs(metadata["audio_url"]) {
			r.Content = append(r.Content, ContentItem{
				Type: "audio_url", AudioURL: &MediaURL{URL: url}, Role: "reference_audio",
			})
		}
	}

	if sec := req.RequestedDuration(); sec != 0 {
		r.Duration = lo.ToPtr(dto.IntValue(sec))
	}
	if req.OmniReferenceTaskType != nil {
		r.OmniReferenceTaskType = req.OmniReferenceTaskType
	}

	return &r, nil
}

func isSeedanceNativeTaskRequest(c *gin.Context) bool {
	if c == nil || c.Request == nil || c.Request.URL == nil {
		return false
	}
	return c.Request.Method == http.MethodPost && c.Request.URL.Path == "/api/v3/contents/generations/tasks"
}

func seedanceNativeTaskSubmitReq(c *gin.Context) (relaycommon.TaskSubmitReq, error) {
	var metadata map[string]any
	if err := common.UnmarshalBodyReusable(c, &metadata); err != nil {
		return relaycommon.TaskSubmitReq{}, err
	}
	var native requestPayload
	if err := common.UnmarshalBodyReusable(c, &native); err != nil {
		return relaycommon.TaskSubmitReq{}, err
	}
	if strings.TrimSpace(native.Model) == "" {
		return relaycommon.TaskSubmitReq{}, fmt.Errorf("model field is required")
	}

	req := relaycommon.TaskSubmitReq{
		Model:                 native.Model,
		Size:                  native.Resolution,
		Metadata:              metadata,
		OmniReferenceTaskType: native.OmniReferenceTaskType,
	}
	if native.Duration != nil {
		req.Duration = int(*native.Duration)
		req.Seconds = strconv.Itoa(req.Duration)
	}
	if native.Ratio != "" {
		req.Metadata["ratio"] = native.Ratio
	}
	if native.Watermark != nil {
		req.Metadata["watermark"] = bool(*native.Watermark)
	}
	if native.GenerateAudio != nil {
		req.Metadata["generate_audio"] = bool(*native.GenerateAudio)
	}

	for _, item := range native.Content {
		switch item.Type {
		case "text":
			if req.Prompt == "" {
				req.Prompt = item.Text
			}
		case "image_url":
			if item.ImageURL != nil && item.ImageURL.URL != "" {
				req.Images = append(req.Images, item.ImageURL.URL)
			}
		}
	}
	delete(req.Metadata, "model")
	if !relaycommon.HasSeedanceInput(req) {
		return relaycommon.TaskSubmitReq{}, fmt.Errorf("prompt or media is required")
	}
	return req, nil
}

func (a *TaskAdaptor) ParseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
	resTask := responseTask{}
	if err := common.Unmarshal(respBody, &resTask); err != nil {
		return nil, errors.Wrap(err, "unmarshal task result failed")
	}

	taskResult := relaycommon.TaskInfo{
		Code: 0,
	}

	// Map Doubao status to internal status
	switch resTask.Status {
	case "pending", "queued":
		taskResult.Status = model.TaskStatusQueued
		taskResult.Progress = "10%"
	case "processing", "running":
		taskResult.Status = model.TaskStatusInProgress
		taskResult.Progress = "50%"
	case "succeeded":
		taskResult.Status = model.TaskStatusSuccess
		taskResult.Progress = "100%"
		taskResult.Url = resTask.Content.VideoURL
		// 解析 usage 信息用于按倍率计费
		taskResult.CompletionTokens = resTask.Usage.CompletionTokens
		taskResult.TotalTokens = resTask.Usage.TotalTokens
	case "failed", "cancelled", "canceled", "expired":
		taskResult.Status = model.TaskStatusFailure
		taskResult.Progress = "100%"
		taskResult.Reason = resTask.Error.Message
		if taskResult.Reason == "" {
			taskResult.Reason = "Task " + resTask.Status
		}
	default:
		// Unknown status, treat as processing
		taskResult.Status = model.TaskStatusInProgress
		taskResult.Progress = "30%"
	}

	return &taskResult, nil
}

func (a *TaskAdaptor) ConvertToOpenAIVideo(originTask *model.Task) ([]byte, error) {
	var dResp responseTask
	if err := common.Unmarshal(originTask.Data, &dResp); err != nil {
		return nil, errors.Wrap(err, "unmarshal doubao task data failed")
	}

	openAIVideo := dto.NewOpenAIVideo()
	openAIVideo.ID = originTask.TaskID
	openAIVideo.TaskID = originTask.TaskID
	openAIVideo.Status = originTask.Status.ToVideoStatus()
	openAIVideo.SetProgressStr(originTask.Progress)
	openAIVideo.SetMetadata("url", dResp.Content.VideoURL)
	openAIVideo.CreatedAt = originTask.CreatedAt
	openAIVideo.CompletedAt = originTask.UpdatedAt
	openAIVideo.Model = originTask.Properties.OriginModelName
	for key, value := range relaycommon.SeedanceVideoMetadata(originTask.Data) {
		openAIVideo.SetMetadata(key, value)
	}

	if originTask.Status == model.TaskStatusFailure {
		openAIVideo.Error = &dto.OpenAIVideoError{
			Message: dResp.Error.Message,
			Code:    dResp.Error.Code,
		}
		if openAIVideo.Error.Message == "" {
			openAIVideo.Error.Message = "Task " + dResp.Status
		}
		if openAIVideo.Error.Code == "" {
			openAIVideo.Error.Code = dResp.Status
		}
	}

	return common.Marshal(openAIVideo)
}

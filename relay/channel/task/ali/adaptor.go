package ali

import (
	"bytes"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	taskdto "github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	newapitypes "github.com/QuantumNous/new-api/types"
	"github.com/samber/lo"

	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
	"github.com/tidwall/sjson"
)

// ============================
// Request / Response structures
// ============================

// AliVideoRequest 阿里通义万相视频生成请求
type AliVideoRequest struct {
	Model      string              `json:"model"`
	Input      AliVideoInput       `json:"input"`
	Parameters *AliVideoParameters `json:"parameters,omitempty"`
}

// AliVideoMedia describes Wan2.7 image-to-video media inputs.
type AliVideoMedia struct {
	Type string `json:"type"`
	URL  string `json:"url"`
}

// AliVideoInput 视频输入参数
type AliVideoInput struct {
	Prompt         string          `json:"prompt,omitempty"`          // 文本提示词
	ImgURL         string          `json:"img_url,omitempty"`         // 首帧图像URL或Base64（图生视频）
	FirstFrameURL  string          `json:"first_frame_url,omitempty"` // 首帧图片URL（首尾帧生视频）
	LastFrameURL   string          `json:"last_frame_url,omitempty"`  // 尾帧图片URL（首尾帧生视频）
	AudioURL       string          `json:"audio_url,omitempty"`       // 音频URL（wan2.5支持）
	Media          []AliVideoMedia `json:"media,omitempty"`           // 媒体列表（wan2.7-i2v新协议）
	NegativePrompt string          `json:"negative_prompt,omitempty"` // 反向提示词
	Template       string          `json:"template,omitempty"`        // 视频特效模板
}

// AliVideoParameters 视频参数
type AliVideoParameters struct {
	Resolution   string `json:"resolution,omitempty"`    // 分辨率: 480P/720P/1080P（图生视频、首尾帧生视频）
	Size         string `json:"size,omitempty"`          // 尺寸: 如 "832*480"（文生视频）
	Duration     int    `json:"duration,omitempty"`      // 时长: 3-10秒
	PromptExtend bool   `json:"prompt_extend,omitempty"` // 是否开启prompt智能改写
	Watermark    bool   `json:"watermark,omitempty"`     // 是否添加水印
	Audio        *bool  `json:"audio,omitempty"`         // 是否添加音频（wan2.5）
	Seed         int    `json:"seed,omitempty"`          // 随机数种子
}

const wan3NativeRequestContextKey = "ali_wan3_native_request"

// Wan3VideoRequest mirrors the native DashScope Wan3 request. Optional scalar
// fields are pointers so explicit false/zero values survive the relay.
type Wan3VideoRequest struct {
	Model      string               `json:"model"`
	Input      Wan3VideoInput       `json:"input"`
	Parameters *Wan3VideoParameters `json:"parameters,omitempty"`
}

type Wan3VideoInput struct {
	Prompt *string         `json:"prompt,omitempty"`
	Media  []AliVideoMedia `json:"media,omitempty"`
}

type Wan3VideoParameters struct {
	Resolution   *string `json:"resolution,omitempty"`
	Ratio        *string `json:"ratio,omitempty"`
	Duration     *int    `json:"duration,omitempty"`
	Audio        *bool   `json:"audio,omitempty"`
	Seed         *int64  `json:"seed,omitempty"`
	PromptExtend *bool   `json:"prompt_extend,omitempty"`
	Watermark    *bool   `json:"watermark,omitempty"`
}

// AliVideoResponse 阿里通义万相响应
type AliVideoResponse struct {
	Output    AliVideoOutput `json:"output"`
	RequestID string         `json:"request_id"`
	Code      string         `json:"code,omitempty"`
	Message   string         `json:"message,omitempty"`
	Usage     *AliUsage      `json:"usage,omitempty"`
}

// AliVideoOutput 输出信息
type AliVideoOutput struct {
	TaskID        string `json:"task_id"`
	TaskStatus    string `json:"task_status"`
	SubmitTime    string `json:"submit_time,omitempty"`
	ScheduledTime string `json:"scheduled_time,omitempty"`
	EndTime       string `json:"end_time,omitempty"`
	OrigPrompt    string `json:"orig_prompt,omitempty"`
	ActualPrompt  string `json:"actual_prompt,omitempty"`
	VideoURL      string `json:"video_url,omitempty"`
	Code          string `json:"code,omitempty"`
	Message       string `json:"message,omitempty"`
}

// AliUsage 使用统计
type AliFloatValue float64

func (v *AliFloatValue) UnmarshalJSON(data []byte) error {
	var number float64
	if err := common.Unmarshal(data, &number); err == nil {
		*v = AliFloatValue(number)
		return nil
	}
	var text string
	if err := common.Unmarshal(data, &text); err != nil {
		return err
	}
	parsed, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return err
	}
	*v = AliFloatValue(parsed)
	return nil
}

func (v AliFloatValue) MarshalJSON() ([]byte, error) {
	return common.Marshal(float64(v))
}

type AliUsage struct {
	Duration            AliFloatValue `json:"duration,omitempty"`
	VideoCount          dto.IntValue  `json:"video_count,omitempty"`
	InputVideoDuration  AliFloatValue `json:"input_video_duration,omitempty"`
	OutputVideoDuration AliFloatValue `json:"output_video_duration,omitempty"`
	FPS                 dto.IntValue  `json:"fps,omitempty"`
	SR                  dto.IntValue  `json:"SR,omitempty"`
	Ratio               string        `json:"ratio,omitempty"`
}

type AliMetadata struct {
	// Input 相关
	AudioURL       string          `json:"audio_url,omitempty"`       // 音频URL
	ImgURL         string          `json:"img_url,omitempty"`         // 图片URL（图生视频）
	FirstFrameURL  string          `json:"first_frame_url,omitempty"` // 首帧图片URL（首尾帧生视频）
	LastFrameURL   string          `json:"last_frame_url,omitempty"`  // 尾帧图片URL（首尾帧生视频）
	Media          []AliVideoMedia `json:"media,omitempty"`           // 媒体列表（wan2.7-i2v新协议）
	NegativePrompt string          `json:"negative_prompt,omitempty"` // 反向提示词
	Template       string          `json:"template,omitempty"`        // 视频特效模板

	// Parameters 相关
	Resolution   *string `json:"resolution,omitempty"`    // 分辨率: 480P/720P/1080P
	Size         *string `json:"size,omitempty"`          // 尺寸: 如 "832*480"
	Duration     *int    `json:"duration,omitempty"`      // 时长
	PromptExtend *bool   `json:"prompt_extend,omitempty"` // 是否开启prompt智能改写
	Watermark    *bool   `json:"watermark,omitempty"`     // 是否添加水印
	Audio        *bool   `json:"audio,omitempty"`         // 是否添加音频
	Seed         *int    `json:"seed,omitempty"`          // 随机数种子
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

func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) (taskErr *taskdto.TaskError) {
	if isWan3NativeRequest(c) {
		var req Wan3VideoRequest
		if err := common.UnmarshalBodyReusable(c, &req); err != nil {
			return wan3TaskError("InvalidParameter", err.Error())
		}
		if err := validateWan3Request(&req); err != nil {
			return wan3TaskError("InvalidParameter", err.Error())
		}
		c.Set(wan3NativeRequestContextKey, req)
		taskReq := wan3TaskSubmitRequest(req)
		c.Set("task_request", taskReq)
		info.Action = constant.TaskActionGenerate
		if info.OriginModelName == "" {
			info.OriginModelName = req.Model
			info.RequestModelName = req.Model
		}
		return nil
	}

	var req relaycommon.TaskSubmitReq
	if err := common.UnmarshalBodyReusable(c, &req); err == nil && isWan3Model(req.Model) {
		wan3Req, convertErr := a.convertToWan3Request(info, req)
		if convertErr != nil {
			return wan3TaskError("InvalidParameter", convertErr.Error())
		}
		if validateErr := validateWan3Request(wan3Req); validateErr != nil {
			return wan3TaskError("InvalidParameter", validateErr.Error())
		}
		c.Set("task_request", req)
		info.Action = constant.TaskActionGenerate
		return nil
	}

	// ValidateMultipartDirect remains the legacy Wan2 validation path.
	return relaycommon.ValidateMultipartDirect(c, info)
}

func (a *TaskAdaptor) BuildRequestURL(info *relaycommon.RelayInfo) (string, error) {
	return fmt.Sprintf("%s/api/v1/services/aigc/video-generation/video-synthesis", a.baseURL), nil
}

// BuildRequestHeader sets required headers for Ali API
func (a *TaskAdaptor) BuildRequestHeader(c *gin.Context, req *http.Request, info *relaycommon.RelayInfo) error {
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-DashScope-Async", "enable") // 阿里异步任务必须设置
	return nil
}

func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	if native, ok := c.Get(wan3NativeRequestContextKey); ok {
		if _, valid := native.(Wan3VideoRequest); !valid {
			return nil, errors.New("invalid Wan3 native request context")
		}
		var nativeBody map[string]any
		if err := common.UnmarshalBodyReusable(c, &nativeBody); err != nil {
			return nil, errors.Wrap(err, "read_wan3_native_request_failed")
		}
		if info.IsModelMapped {
			nativeBody["model"] = info.UpstreamModelName
		}
		logger.LogJson(c, "ali Wan3 native request body", nativeBody)
		bodyBytes, err := common.Marshal(nativeBody)
		if err != nil {
			return nil, errors.Wrap(err, "marshal_wan3_request_failed")
		}
		return bytes.NewReader(bodyBytes), nil
	}

	taskReq, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return nil, errors.Wrap(err, "get_task_request_failed")
	}

	upstreamModel := taskReq.Model
	if info.IsModelMapped {
		upstreamModel = info.UpstreamModelName
	}
	if isWan3Model(upstreamModel) || isWan3Model(taskReq.Model) {
		wan3Req, err := a.convertToWan3Request(info, taskReq)
		if err != nil {
			return nil, errors.Wrap(err, "convert_to_wan3_request_failed")
		}
		logger.LogJson(c, "ali Wan3 request body", wan3Req)
		bodyBytes, err := common.Marshal(wan3Req)
		if err != nil {
			return nil, errors.Wrap(err, "marshal_wan3_request_failed")
		}
		return bytes.NewReader(bodyBytes), nil
	}

	aliReq, err := a.convertToAliRequest(info, taskReq)
	if err != nil {
		return nil, errors.Wrap(err, "convert_to_ali_request_failed")
	}
	logger.LogJson(c, "ali video request body", aliReq)

	bodyBytes, err := common.Marshal(aliReq)
	if err != nil {
		return nil, errors.Wrap(err, "marshal_ali_request_failed")
	}
	return bytes.NewReader(bodyBytes), nil
}

func isWan3Model(modelName string) bool {
	switch strings.TrimSpace(modelName) {
	case "wan3.0-video", "wan3.0-video-prime":
		return true
	default:
		return false
	}
}

func isWan3NativeRequest(c *gin.Context) bool {
	if c == nil || c.Request == nil || c.Request.Method != http.MethodPost {
		return false
	}
	if c.GetString("configurable_native_profile_id") != "dashscope-wan3-video" {
		return false
	}
	return strings.TrimRight(c.Request.URL.Path, "/") == "/api/v1/services/aigc/video-generation/video-synthesis"
}

func wan3TaskError(code, message string) *taskdto.TaskError {
	body, _ := common.Marshal(map[string]string{
		"code":    code,
		"message": message,
	})
	return &taskdto.TaskError{
		Code:       code,
		Message:    message,
		StatusCode: http.StatusBadRequest,
		LocalError: true,
		Error:      errors.New(message),
		RawBody:    body,
	}
}

func wan3TaskSubmitRequest(req Wan3VideoRequest) relaycommon.TaskSubmitReq {
	taskReq := relaycommon.TaskSubmitReq{
		Model:    req.Model,
		Metadata: map[string]interface{}{},
	}
	if req.Input.Prompt != nil {
		taskReq.Prompt = *req.Input.Prompt
	}
	for _, media := range req.Input.Media {
		taskReq.Images = append(taskReq.Images, media.URL)
	}
	if req.Parameters != nil {
		if req.Parameters.Resolution != nil {
			taskReq.Size = *req.Parameters.Resolution
		}
		if req.Parameters.Duration != nil {
			taskReq.Duration = *req.Parameters.Duration
			if taskReq.Duration == -1 {
				taskReq.Duration = 30
			}
		}
		parametersBytes, _ := common.Marshal(req.Parameters)
		var parameters map[string]interface{}
		if common.Unmarshal(parametersBytes, &parameters) == nil {
			taskReq.Metadata["parameters"] = parameters
		}
	}
	return taskReq
}

func (a *TaskAdaptor) convertToWan3Request(info *relaycommon.RelayInfo, req relaycommon.TaskSubmitReq) (*Wan3VideoRequest, error) {
	modelName := req.Model
	if info != nil && info.IsModelMapped {
		modelName = info.UpstreamModelName
	}
	wan3Req := &Wan3VideoRequest{
		Model: modelName,
		Input: Wan3VideoInput{},
	}
	if req.Prompt != "" {
		prompt := req.Prompt
		wan3Req.Input.Prompt = &prompt
	}

	var metadata struct {
		Input      Wan3VideoInput       `json:"input"`
		Parameters *Wan3VideoParameters `json:"parameters"`
	}
	if req.Metadata != nil {
		metadataBytes, err := common.Marshal(req.Metadata)
		if err != nil {
			return nil, err
		}
		if err := common.Unmarshal(metadataBytes, &metadata); err != nil {
			return nil, err
		}
	}
	if metadata.Input.Prompt != nil {
		wan3Req.Input.Prompt = metadata.Input.Prompt
	}
	if len(metadata.Input.Media) > 0 {
		wan3Req.Input.Media = metadata.Input.Media
	} else {
		if first := firstTaskImage(req); first != "" {
			wan3Req.Input.Media = append(wan3Req.Input.Media, AliVideoMedia{Type: "first_frame", URL: first})
		}
		if last := secondTaskImage(req); last != "" {
			wan3Req.Input.Media = append(wan3Req.Input.Media, AliVideoMedia{Type: "last_frame", URL: last})
		}
		for _, reference := range req.ReferenceImages {
			if strings.TrimSpace(reference.URL) != "" {
				wan3Req.Input.Media = append(wan3Req.Input.Media, AliVideoMedia{Type: "reference_image", URL: strings.TrimSpace(reference.URL)})
			}
		}
		if req.Video != nil && strings.TrimSpace(req.Video.URL) != "" {
			wan3Req.Input.Media = append(wan3Req.Input.Media, AliVideoMedia{Type: "reference_video", URL: strings.TrimSpace(req.Video.URL)})
		}
	}

	wan3Req.Parameters = metadata.Parameters
	if wan3Req.Parameters == nil {
		wan3Req.Parameters = &Wan3VideoParameters{}
	}
	if wan3Req.Parameters.Resolution == nil && strings.TrimSpace(req.Size) != "" {
		resolution := strings.ToUpper(strings.TrimSpace(req.Size))
		if !strings.HasSuffix(resolution, "P") {
			resolution += "P"
		}
		wan3Req.Parameters.Resolution = &resolution
	}
	if wan3Req.Parameters.Duration == nil {
		if req.Seconds != "" {
			duration, err := strconv.Atoi(req.Seconds)
			if err != nil {
				return nil, errors.Wrap(err, "convert seconds to int failed")
			}
			wan3Req.Parameters.Duration = &duration
		} else if req.Duration != 0 {
			duration := req.Duration
			wan3Req.Parameters.Duration = &duration
		}
	}
	return wan3Req, nil
}

func validateWan3Request(req *Wan3VideoRequest) error {
	if req == nil {
		return errors.New("request is required")
	}
	if !isWan3Model(req.Model) {
		return fmt.Errorf("unsupported model %q", req.Model)
	}
	prompt := ""
	if req.Input.Prompt != nil {
		prompt = strings.TrimSpace(*req.Input.Prompt)
	}
	if prompt == "" && len(req.Input.Media) == 0 {
		return errors.New("input.prompt and input.media cannot both be empty")
	}
	counts := map[string]int{}
	validTypes := map[string]bool{
		"first_frame": true, "last_frame": true, "reference_image": true,
		"reference_video": true, "reference_audio": true, "file": true, "link": true,
	}
	for i, media := range req.Input.Media {
		if !validTypes[media.Type] {
			return fmt.Errorf("input.media[%d].type is invalid", i)
		}
		if strings.TrimSpace(media.URL) == "" {
			return fmt.Errorf("input.media[%d].url is required", i)
		}
		counts[media.Type]++
	}
	limits := map[string]int{
		"first_frame": 1, "last_frame": 1, "reference_image": 10,
		"reference_video": 5, "reference_audio": 5, "file": 1, "link": 1,
	}
	for mediaType, limit := range limits {
		if counts[mediaType] > limit {
			return fmt.Errorf("input.media supports at most %d %s item(s)", limit, mediaType)
		}
	}
	frameMode := counts["first_frame"]+counts["last_frame"] > 0
	referenceMode := counts["reference_image"]+counts["reference_video"]+counts["reference_audio"] > 0
	fileMode := counts["file"] > 0
	linkMode := counts["link"] > 0
	if frameMode && (referenceMode || fileMode || linkMode) {
		return errors.New("first_frame/last_frame cannot be combined with reference, file, or link media")
	}
	if fileMode && linkMode {
		return errors.New("file and link media cannot be combined")
	}
	if req.Parameters == nil {
		return nil
	}
	if req.Parameters.Resolution != nil {
		resolution := strings.ToUpper(strings.TrimSpace(*req.Parameters.Resolution))
		if resolution != "480P" && resolution != "720P" && resolution != "1080P" {
			return errors.New("parameters.resolution must be 480P, 720P, or 1080P")
		}
	}
	if req.Parameters.Ratio != nil {
		validRatios := map[string]bool{"adaptive": true, "16:9": true, "4:3": true, "1:1": true, "3:4": true, "9:16": true}
		if !validRatios[*req.Parameters.Ratio] {
			return errors.New("parameters.ratio is invalid")
		}
	}
	if req.Parameters.Duration != nil {
		duration := *req.Parameters.Duration
		minimum := 2
		if counts["reference_video"] > 0 {
			minimum = 1
		}
		if duration != -1 && (duration < minimum || duration > 30) {
			return fmt.Errorf("parameters.duration must be -1 or between %d and 30", minimum)
		}
	}
	if req.Parameters.Seed != nil && (*req.Parameters.Seed < 0 || *req.Parameters.Seed > math.MaxInt32) {
		return fmt.Errorf("parameters.seed must be between 0 and %d", math.MaxInt32)
	}
	return nil
}

var (
	size480p = []string{
		"832*480",
		"480*832",
		"624*624",
	}
	size720p = []string{
		"1280*720",
		"720*1280",
		"960*960",
		"1088*832",
		"832*1088",
	}
	size1080p = []string{
		"1920*1080",
		"1080*1920",
		"1440*1440",
		"1632*1248",
		"1248*1632",
	}
)

func sizeToResolution(size string) (string, error) {
	if lo.Contains(size480p, size) {
		return "480P", nil
	} else if lo.Contains(size720p, size) {
		return "720P", nil
	} else if lo.Contains(size1080p, size) {
		return "1080P", nil
	}
	return "", fmt.Errorf("invalid size: %s", size)
}

func ProcessAliOtherRatios(aliReq *AliVideoRequest) (map[string]float64, error) {
	otherRatios := make(map[string]float64)
	aliRatios := map[string]map[string]float64{
		"wan2.6-i2v": {
			"720P":  1,
			"1080P": 1 / 0.6,
		},
		"wan2.5-t2v-preview": {
			"480P":  1,
			"720P":  2,
			"1080P": 1 / 0.3,
		},
		"wan2.2-t2v-plus": {
			"480P":  1,
			"1080P": 0.7 / 0.14,
		},
		"wan2.5-i2v-preview": {
			"480P":  1,
			"720P":  2,
			"1080P": 1 / 0.3,
		},
		"wan2.2-i2v-plus": {
			"480P":  1,
			"1080P": 0.7 / 0.14,
		},
		"wan2.2-kf2v-flash": {
			"480P":  1,
			"720P":  2,
			"1080P": 4.8,
		},
		"wan2.2-i2v-flash": {
			"480P": 1,
			"720P": 2,
		},
		"wan2.2-s2v": {
			"480P": 1,
			"720P": 0.9 / 0.5,
		},
	}
	var resolution string

	// size match
	if aliReq.Parameters.Size != "" {
		toResolution, err := sizeToResolution(aliReq.Parameters.Size)
		if err != nil {
			return nil, err
		}
		resolution = toResolution
	} else {
		resolution = strings.ToUpper(aliReq.Parameters.Resolution)
		if !strings.HasSuffix(resolution, "P") {
			resolution = resolution + "P"
		}
	}
	if otherRatio, ok := aliRatios[aliReq.Model]; ok {
		if ratio, ok := otherRatio[resolution]; ok {
			otherRatios[fmt.Sprintf("resolution-%s", resolution)] = ratio
		}
	}
	return otherRatios, nil
}

func processWan3OtherRatios(req *Wan3VideoRequest) map[string]float64 {
	seconds := 5.0
	resolution := "1080P"
	if req != nil && req.Parameters != nil {
		if req.Parameters.Duration != nil {
			seconds = float64(*req.Parameters.Duration)
			if seconds == -1 {
				seconds = 30
			}
		}
		if req.Parameters.Resolution != nil && strings.TrimSpace(*req.Parameters.Resolution) != "" {
			resolution = strings.ToUpper(strings.TrimSpace(*req.Parameters.Resolution))
		}
	}
	resolutionRatio := 4.0
	switch resolution {
	case "480P":
		resolutionRatio = 1
	case "720P":
		resolutionRatio = 2
	}
	return map[string]float64{
		"seconds":                                seconds,
		fmt.Sprintf("resolution-%s", resolution): resolutionRatio,
	}
}

func isWan27I2VModel(model string) bool {
	return strings.HasPrefix(model, "wan2.7-i2v")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func firstTaskImage(req relaycommon.TaskSubmitReq) string {
	if image := strings.TrimSpace(req.Image); image != "" {
		return image
	}
	for _, image := range req.Images {
		if trimmed := strings.TrimSpace(image); trimmed != "" {
			return trimmed
		}
	}
	if inputReference := strings.TrimSpace(req.InputReference); inputReference != "" {
		return inputReference
	}
	return ""
}

func secondTaskImage(req relaycommon.TaskSubmitReq) string {
	nonEmptyImages := 0
	for _, image := range req.Images {
		trimmed := strings.TrimSpace(image)
		if trimmed == "" {
			continue
		}
		nonEmptyImages++
		if nonEmptyImages == 2 {
			return trimmed
		}
	}
	return ""
}

func normalizeWan27I2VInput(aliReq *AliVideoRequest, req relaycommon.TaskSubmitReq) error {
	if !isWan27I2VModel(aliReq.Model) {
		return nil
	}

	if len(aliReq.Input.Media) == 0 {
		firstFrameURL := firstNonEmpty(aliReq.Input.FirstFrameURL, aliReq.Input.ImgURL, firstTaskImage(req))
		lastFrameURL := firstNonEmpty(aliReq.Input.LastFrameURL, secondTaskImage(req))
		audioURL := aliReq.Input.AudioURL

		if firstFrameURL != "" {
			aliReq.Input.Media = append(aliReq.Input.Media, AliVideoMedia{
				Type: "first_frame",
				URL:  firstFrameURL,
			})
		}
		if lastFrameURL != "" {
			aliReq.Input.Media = append(aliReq.Input.Media, AliVideoMedia{
				Type: "last_frame",
				URL:  lastFrameURL,
			})
		}
		if audioURL != "" {
			aliReq.Input.Media = append(aliReq.Input.Media, AliVideoMedia{
				Type: "driving_audio",
				URL:  audioURL,
			})
		}
	}

	if len(aliReq.Input.Media) == 0 {
		return fmt.Errorf("wan2.7-i2v requires image, images, input_reference, or input.media")
	}

	// Wan2.7 image-to-video uses the new input.media protocol. Avoid sending
	// legacy fields that belong to wan2.6 and earlier image-to-video APIs.
	aliReq.Input.ImgURL = ""
	aliReq.Input.FirstFrameURL = ""
	aliReq.Input.LastFrameURL = ""
	aliReq.Input.AudioURL = ""
	return nil
}

func (a *TaskAdaptor) convertToAliRequest(info *relaycommon.RelayInfo, req relaycommon.TaskSubmitReq) (*AliVideoRequest, error) {
	upstreamModel := req.Model
	if info.IsModelMapped {
		upstreamModel = info.UpstreamModelName
	}
	aliReq := &AliVideoRequest{
		Model: upstreamModel,
		Input: AliVideoInput{
			Prompt: req.Prompt,
			ImgURL: firstTaskImage(req),
		},
		Parameters: &AliVideoParameters{
			PromptExtend: true, // 默认开启智能改写
			Watermark:    false,
		},
	}

	// 处理分辨率映射
	if req.Size != "" {
		// text to video size must be contained *
		if strings.Contains(req.Model, "t2v") && !strings.Contains(req.Size, "*") {
			return nil, fmt.Errorf("invalid size: %s, example: %s", req.Size, "1920*1080")
		}
		if strings.Contains(req.Size, "*") {
			aliReq.Parameters.Size = req.Size
		} else {
			resolution := strings.ToUpper(req.Size)
			// 支持 480p, 720p, 1080p 或 480P, 720P, 1080P
			if !strings.HasSuffix(resolution, "P") {
				resolution = resolution + "P"
			}
			aliReq.Parameters.Resolution = resolution
		}
	} else {
		// 根据模型设置默认分辨率
		if strings.Contains(req.Model, "t2v") { // image to video
			if strings.HasPrefix(req.Model, "wan2.5") {
				aliReq.Parameters.Size = "1920*1080"
			} else if strings.HasPrefix(req.Model, "wan2.2") {
				aliReq.Parameters.Size = "1920*1080"
			} else {
				aliReq.Parameters.Size = "1280*720"
			}
		} else {
			if strings.HasPrefix(req.Model, "wan2.6") {
				aliReq.Parameters.Resolution = "1080P"
			} else if strings.HasPrefix(req.Model, "wan2.5") {
				aliReq.Parameters.Resolution = "1080P"
			} else if strings.HasPrefix(req.Model, "wan2.2-i2v-flash") {
				aliReq.Parameters.Resolution = "720P"
			} else if strings.HasPrefix(req.Model, "wan2.2-i2v-plus") {
				aliReq.Parameters.Resolution = "1080P"
			} else {
				aliReq.Parameters.Resolution = "720P"
			}
		}
	}

	// 处理时长
	if req.Duration > 0 {
		aliReq.Parameters.Duration = req.Duration
	} else if req.Seconds != "" {
		seconds, err := strconv.Atoi(req.Seconds)
		if err != nil {
			return nil, errors.Wrap(err, "convert seconds to int failed")
		} else {
			aliReq.Parameters.Duration = seconds
		}
	}
	if aliReq.Parameters.Duration <= 0 {
		aliReq.Parameters.Duration = 5 // 默认5秒
	}

	// 从 metadata 中提取额外参数
	if req.Metadata != nil {
		if metadataBytes, err := common.Marshal(req.Metadata); err == nil {
			err = common.Unmarshal(metadataBytes, aliReq)
			if err != nil {
				return nil, errors.Wrap(err, "unmarshal metadata failed")
			}
		} else {
			return nil, errors.Wrap(err, "marshal metadata failed")
		}
	}

	if aliReq.Model != upstreamModel {
		return nil, errors.New("can't change model with metadata")
	}

	if err := normalizeWan27I2VInput(aliReq, req); err != nil {
		return nil, err
	}

	return aliReq, nil
}

// EstimateBilling 根据用户请求参数计算 OtherRatios（时长、分辨率等）。
// 在 ValidateRequestAndSetAction 之后、价格计算之前调用。
func (a *TaskAdaptor) EstimateBilling(c *gin.Context, info *relaycommon.RelayInfo) map[string]float64 {
	taskReq, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return nil
	}

	if native, ok := c.Get(wan3NativeRequestContextKey); ok {
		if wan3Req, valid := native.(Wan3VideoRequest); valid {
			return processWan3OtherRatios(&wan3Req)
		}
	}
	upstreamModel := taskReq.Model
	if info != nil && info.IsModelMapped {
		upstreamModel = info.UpstreamModelName
	}
	if isWan3Model(taskReq.Model) || isWan3Model(upstreamModel) {
		wan3Req, convertErr := a.convertToWan3Request(info, taskReq)
		if convertErr != nil {
			return nil
		}
		return processWan3OtherRatios(wan3Req)
	}

	aliReq, err := a.convertToAliRequest(info, taskReq)
	if err != nil {
		return nil
	}

	// metadata can override Duration past standard request validation;
	// cap it because it is used as a billing multiplier.
	otherRatios := map[string]float64{
		"seconds": float64(min(aliReq.Parameters.Duration, relaycommon.MaxTaskDurationSeconds)),
	}
	ratios, err := ProcessAliOtherRatios(aliReq)
	if err != nil {
		return otherRatios
	}
	for k, v := range ratios {
		otherRatios[k] = v
	}
	return otherRatios
}

// DoRequest delegates to common helper
func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	return channel.DoTaskApiRequest(a, c, info, requestBody)
}

// DoResponse handles upstream response
func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (taskID string, taskData []byte, taskErr *taskdto.TaskError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		taskErr = service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
		return
	}
	_ = resp.Body.Close()

	// 解析阿里响应
	var aliResp AliVideoResponse
	if err := common.Unmarshal(responseBody, &aliResp); err != nil {
		taskErr = service.TaskErrorWrapper(errors.Wrapf(err, "body: %s", responseBody), "unmarshal_response_body_failed", http.StatusInternalServerError)
		return
	}

	// 检查错误
	if aliResp.Code != "" {
		taskErr = service.TaskErrorWrapper(fmt.Errorf("%s: %s", aliResp.Code, aliResp.Message), "ali_api_error", resp.StatusCode)
		if isWan3NativeRequest(c) {
			taskErr.RawBody = responseBody
		}
		return
	}

	if aliResp.Output.TaskID == "" {
		taskErr = service.TaskErrorWrapper(fmt.Errorf("task_id is empty"), "invalid_response", http.StatusInternalServerError)
		if isWan3NativeRequest(c) {
			taskErr.RawBody = responseBody
		}
		return
	}

	if isWan3NativeRequest(c) {
		nativeBody, patchErr := sjson.SetBytes(responseBody, "output.task_id", info.PublicTaskID)
		if patchErr != nil {
			taskErr = service.TaskErrorWrapper(patchErr, "build_native_response_failed", http.StatusInternalServerError)
			return
		}
		c.Data(http.StatusOK, "application/json", nativeBody)
		return aliResp.Output.TaskID, responseBody, nil
	}

	// 转换为 OpenAI 格式响应
	openAIResp := dto.NewOpenAIVideo()
	openAIResp.ID = info.PublicTaskID
	openAIResp.TaskID = info.PublicTaskID
	openAIResp.Model = c.GetString("model")
	if openAIResp.Model == "" && info != nil {
		openAIResp.Model = info.OriginModelName
	}
	openAIResp.Status = convertAliStatus(aliResp.Output.TaskStatus)
	openAIResp.CreatedAt = common.GetTimestamp()

	// 返回 OpenAI 格式
	c.JSON(http.StatusOK, openAIResp)

	return aliResp.Output.TaskID, responseBody, nil
}

// FetchTask 查询任务状态
func (a *TaskAdaptor) FetchTask(baseUrl, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid task_id")
	}

	uri := fmt.Sprintf("%s/api/v1/tasks/%s", baseUrl, taskID)

	req, err := http.NewRequest(http.MethodGet, uri, nil)
	if err != nil {
		return nil, err
	}

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

// ParseTaskResult 解析任务结果
func (a *TaskAdaptor) ParseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
	var aliResp AliVideoResponse
	if err := common.Unmarshal(respBody, &aliResp); err != nil {
		return nil, errors.Wrap(err, "unmarshal task result failed")
	}

	taskResult := relaycommon.TaskInfo{
		Code: 0,
	}

	// 状态映射
	switch aliResp.Output.TaskStatus {
	case "PENDING":
		taskResult.Status = model.TaskStatusQueued
	case "RUNNING":
		taskResult.Status = model.TaskStatusInProgress
	case "SUCCEEDED":
		taskResult.Status = model.TaskStatusSuccess
		// 阿里直接返回视频URL，不需要额外的代理端点
		taskResult.Url = aliResp.Output.VideoURL
	case "FAILED", "CANCELED", "UNKNOWN":
		taskResult.Status = model.TaskStatusFailure
		if aliResp.Message != "" {
			taskResult.Reason = aliResp.Message
		} else if aliResp.Output.Message != "" {
			taskResult.Reason = fmt.Sprintf("task failed, code: %s , message: %s", aliResp.Output.Code, aliResp.Output.Message)
		} else {
			taskResult.Reason = "task failed"
		}
	default:
		taskResult.Status = model.TaskStatusQueued
	}

	return &taskResult, nil
}

// ConvertToNativeFetchResponse preserves the complete DashScope payload while
// replacing the upstream task ID with this gateway's public task ID.
func (a *TaskAdaptor) ConvertToNativeFetchResponse(task *model.Task, upstream []byte) ([]byte, error) {
	if task == nil {
		return nil, errors.New("task is required")
	}
	if len(upstream) == 0 {
		status := "PENDING"
		switch task.Status {
		case model.TaskStatusInProgress:
			status = "RUNNING"
		case model.TaskStatusSuccess:
			status = "SUCCEEDED"
		case model.TaskStatusFailure:
			status = "FAILED"
		}
		return common.Marshal(map[string]any{
			"output": map[string]any{
				"task_id":     task.TaskID,
				"task_status": status,
			},
		})
	}
	return sjson.SetBytes(upstream, "output.task_id", task.TaskID)
}

// AdjustBillingOnComplete replaces the estimated Wan3 seconds with the actual
// input+output video duration reported by DashScope. Fixed per-call tasks are
// filtered by the shared settlement service before this hook is invoked.
func (a *TaskAdaptor) AdjustBillingOnComplete(task *model.Task, _ *relaycommon.TaskInfo) int {
	if task == nil || !isWan3Model(task.Properties.OriginModelName) {
		return 0
	}
	bc := task.PrivateData.BillingContext
	if bc == nil || bc.PerCallBilling || len(bc.OtherRatios) == 0 || task.Quota <= 0 {
		return 0
	}
	var aliResp AliVideoResponse
	if err := common.Unmarshal(task.Data, &aliResp); err != nil || aliResp.Usage == nil {
		return 0
	}
	actualSeconds := float64(aliResp.Usage.InputVideoDuration + aliResp.Usage.OutputVideoDuration)
	if actualSeconds <= 0 {
		actualSeconds = float64(aliResp.Usage.Duration)
	}
	if actualSeconds <= 0 || math.IsNaN(actualSeconds) || math.IsInf(actualSeconds, 0) {
		return 0
	}

	estimated := newapitypes.PriceData{OtherRatios: bc.OtherRatios}
	baseQuota := estimated.RemoveOtherRatiosFromFloat(float64(task.Quota))
	actualRatios := make(map[string]float64, len(bc.OtherRatios)+1)
	for key, value := range bc.OtherRatios {
		if key == "seconds" || strings.HasPrefix(key, "resolution-") {
			continue
		}
		actualRatios[key] = value
	}
	actualRatios["seconds"] = actualSeconds
	resolution := aliResp.Usage.SR
	if resolution == 0 {
		for key := range bc.OtherRatios {
			if strings.HasPrefix(key, "resolution-") {
				actualRatios[key] = bc.OtherRatios[key]
			}
		}
	} else {
		resolutionRatio := 4.0
		if resolution == 480 {
			resolutionRatio = 1
		} else if resolution == 720 {
			resolutionRatio = 2
		}
		actualRatios[fmt.Sprintf("resolution-%dP", resolution)] = resolutionRatio
	}
	actual := newapitypes.PriceData{OtherRatios: actualRatios}
	quota, _ := common.QuotaFromFloatChecked(actual.ApplyOtherRatiosToFloat(baseQuota))
	return quota
}

func (a *TaskAdaptor) ConvertToOpenAIVideo(task *model.Task) ([]byte, error) {
	var aliResp AliVideoResponse
	if err := common.Unmarshal(task.Data, &aliResp); err != nil {
		return nil, errors.Wrap(err, "unmarshal ali response failed")
	}

	openAIResp := dto.NewOpenAIVideo()
	openAIResp.ID = task.TaskID
	openAIResp.Status = convertAliStatus(aliResp.Output.TaskStatus)
	openAIResp.Model = task.Properties.OriginModelName
	openAIResp.SetProgressStr(task.Progress)
	openAIResp.CreatedAt = task.CreatedAt
	openAIResp.CompletedAt = task.UpdatedAt

	// 设置视频URL（核心字段）
	openAIResp.SetMetadata("url", aliResp.Output.VideoURL)
	openAIResp.SetMetadata("request_id", aliResp.RequestID)
	openAIResp.SetMetadata("submit_time", aliResp.Output.SubmitTime)
	openAIResp.SetMetadata("scheduled_time", aliResp.Output.ScheduledTime)
	openAIResp.SetMetadata("end_time", aliResp.Output.EndTime)
	openAIResp.SetMetadata("orig_prompt", aliResp.Output.OrigPrompt)
	openAIResp.SetMetadata("actual_prompt", aliResp.Output.ActualPrompt)
	if aliResp.Usage != nil {
		openAIResp.SetMetadata("usage", aliResp.Usage)
		seconds := float64(aliResp.Usage.OutputVideoDuration)
		if seconds <= 0 {
			seconds = float64(aliResp.Usage.Duration)
		}
		if seconds > 0 {
			openAIResp.Seconds = strconv.FormatFloat(seconds, 'f', -1, 64)
		}
		if aliResp.Usage.SR > 0 {
			openAIResp.Size = fmt.Sprintf("%dP", aliResp.Usage.SR)
		}
	}

	// 错误处理
	if aliResp.Code != "" {
		openAIResp.Error = &dto.OpenAIVideoError{
			Code:    aliResp.Code,
			Message: aliResp.Message,
		}
	} else if aliResp.Output.Code != "" {
		openAIResp.Error = &dto.OpenAIVideoError{
			Code:    aliResp.Output.Code,
			Message: aliResp.Output.Message,
		}
	}

	return common.Marshal(openAIResp)
}

func convertAliStatus(aliStatus string) string {
	switch aliStatus {
	case "PENDING":
		return dto.VideoStatusQueued
	case "RUNNING":
		return dto.VideoStatusInProgress
	case "SUCCEEDED":
		return dto.VideoStatusCompleted
	case "FAILED", "CANCELED", "UNKNOWN":
		return dto.VideoStatusFailed
	default:
		return dto.VideoStatusUnknown
	}
}

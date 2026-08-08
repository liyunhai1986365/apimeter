package configurable

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel"
	taskcommon "github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/pkg/errors"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

type TaskAdaptor struct {
	taskcommon.BaseBilling
	apiKey             string
	baseURL            string
	profile            *Profile
	selectedSubmitPath string
	selectedFetchResp  *ResponseConfig
}

func ExtractNativeModel(c *gin.Context, profile *Profile) (string, error) {
	if c == nil || profile == nil {
		return "", fmt.Errorf("invalid native profile context")
	}
	var body map[string]any
	if err := common.UnmarshalBodyReusable(c, &body); err != nil {
		return "", err
	}
	mapped, err := buildMappedMap(profile.videoNative().Submit.Request.Fields, body, nil)
	if err != nil {
		return "", err
	}
	modelName, _ := mapped["model"].(string)
	if strings.TrimSpace(modelName) == "" {
		return "", fmt.Errorf("model field is required")
	}
	return modelName, nil
}

func (a *TaskAdaptor) Init(info *relaycommon.RelayInfo) {
	a.apiKey = info.ApiKey
	a.baseURL = info.ChannelBaseUrl
	if info.ChannelMeta != nil {
		a.apiKey = info.ChannelMeta.ApiKey
		a.baseURL = info.ChannelMeta.ChannelBaseUrl
	}
	a.profile = resolveProfile(info)
}

func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) *dto.TaskError {
	if a.isNativeSubmitRequest(c) {
		req, err := a.parseNativeTaskRequest(c)
		if err != nil {
			return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
		}
		c.Set("task_request", req)
		info.Action = constant.TaskActionGenerate
		if info.OriginModelName == "" {
			info.OriginModelName = req.Model
			info.RequestModelName = req.Model
		}
		if info.ChannelMeta != nil && info.ChannelMeta.UpstreamModelName == "" {
			info.ChannelMeta.UpstreamModelName = req.Model
		}
		a.selectedSubmitPath = a.selectVideoSubmitPath(req, info)
		return nil
	}
	taskErr := relaycommon.ValidateBasicTaskRequest(c, info, constant.TaskActionGenerate)
	if taskErr != nil {
		return taskErr
	}
	if req, err := relaycommon.GetTaskRequest(c); err == nil {
		a.selectedSubmitPath = a.selectVideoSubmitPath(req, info)
	}
	return nil
}

func (a *TaskAdaptor) BuildRequestURL(info *relaycommon.RelayInfo) (string, error) {
	profile, err := a.requireProfile()
	if err != nil {
		return "", err
	}
	path := profile.videoSubmit().Path
	if strings.TrimSpace(a.selectedSubmitPath) != "" {
		path = a.selectedSubmitPath
	}
	return joinURL(a.baseURL, path), nil
}

func (a *TaskAdaptor) BuildRequestHeader(c *gin.Context, req *http.Request, info *relaycommon.RelayInfo) error {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if strings.TrimSpace(a.apiKey) != "" {
		req.Header.Set("Authorization", "Bearer "+a.apiKey)
	}
	if a.profile != nil {
		ApplyConfiguredHeaders(req, a.profile.videoSubmit().Headers, a.apiKey, "")
	}
	return nil
}

func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	profile, err := a.requireProfile()
	if err != nil {
		return nil, err
	}
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return nil, err
	}
	body, err := buildMappedBody(profile.videoSubmit().Body.Fields, req, info)
	if err != nil {
		return nil, err
	}
	data, err := common.Marshal(body)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(data), nil
}

func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	return channel.DoTaskApiRequest(a, c, info, requestBody)
}

func (a *TaskAdaptor) EstimateBilling(c *gin.Context, info *relaycommon.RelayInfo) map[string]float64 {
	if a.profile == nil || len(a.profile.videoBilling().Ratios) == 0 {
		return nil
	}
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return nil
	}
	reqBytes, err := common.Marshal(req)
	if err != nil {
		return nil
	}
	var source map[string]any
	if err := common.Unmarshal(reqBytes, &source); err != nil {
		return nil
	}
	ratios := make(map[string]float64)
	for _, ratio := range a.profile.videoBilling().Ratios {
		key := strings.TrimSpace(ratio.Key)
		if key == "" {
			continue
		}
		value := ratio.Value
		if ratio.From != "" {
			value = floatValue(valueFromSource(ratio.From, source, info))
		}
		if value == 0 && ratio.Default != 0 {
			value = ratio.Default
		}
		if ratio.OmitZero && value == 0 {
			continue
		}
		ratios[key] = value
	}
	return ratios
}

func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (string, []byte, *dto.TaskError) {
	profile, err := a.requireProfile()
	if err != nil {
		return "", nil, service.TaskErrorWrapper(err, "configurable_profile_error", http.StatusBadRequest)
	}
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", nil, service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
	}
	_ = resp.Body.Close()

	taskID := strings.TrimSpace(gjson.GetBytes(responseBody, profile.videoSubmit().Response.TaskIDPath).String())
	if taskID == "" {
		return "", nil, service.TaskErrorWrapper(fmt.Errorf("task_id is empty"), "invalid_response", http.StatusInternalServerError)
	}

	if a.isNativeSubmitRequest(c) {
		if strings.EqualFold(strings.TrimSpace(profile.videoNative().Submit.ResponseFormat), "openai_video") {
			ovBody, err := a.buildOpenAIVideoSubmitResponse(profile.videoSubmit().OpenAIResponse, responseBody, info)
			if err != nil {
				return "", nil, service.TaskErrorWrapper(err, "build_native_openai_video_response_failed", http.StatusInternalServerError)
			}
			c.Data(http.StatusOK, "application/json", ovBody)
		} else {
			nativeResponse, err := buildConfiguredResponse(profile.videoNative().Submit.Response, responseBody, info)
			if err != nil {
				return "", nil, service.TaskErrorWrapper(err, "build_native_response_failed", http.StatusInternalServerError)
			}
			c.Data(http.StatusOK, "application/json", nativeResponse)
		}
	} else {
		ovBody, err := a.buildOpenAIVideoSubmitResponse(profile.videoSubmit().OpenAIResponse, responseBody, info)
		if err != nil {
			return "", nil, service.TaskErrorWrapper(err, "build_openai_video_response_failed", http.StatusInternalServerError)
		}
		c.Data(http.StatusOK, "application/json", ovBody)
	}
	return taskID, responseBody, nil
}

func (a *TaskAdaptor) buildOpenAIVideoSubmitResponse(config ResponseConfig, responseBody []byte, info *relaycommon.RelayInfo) ([]byte, error) {
	profile, err := a.requireProfile()
	if err != nil {
		return nil, err
	}
	ov := dto.NewOpenAIVideo()
	ov.ID = publicTaskID(info)
	ov.TaskID = publicTaskID(info)
	ov.Model = info.OriginModelName
	ov.CreatedAt = time.Now().Unix()
	if status := mapStatus(profile.videoSubmit().Response.StatusMap, gjson.GetBytes(responseBody, profile.videoSubmit().Response.StatusPath).String()); status != "" {
		ov.Status = model.TaskStatus(status).ToVideoStatus()
	}
	ovBody, err := common.Marshal(ov)
	if err != nil {
		return nil, err
	}
	return applyOpenAIVideoResponseFields(config, ovBody, responseBody, info)
}

func (a *TaskAdaptor) FetchTask(baseUrl, key string, body map[string]any, proxy string) (*http.Response, error) {
	profile, err := a.requireProfile()
	if err != nil {
		return nil, err
	}
	taskID, _ := body["task_id"].(string)
	if taskID == "" {
		return nil, fmt.Errorf("invalid task_id")
	}
	fetch := profile.videoFetch()
	path := fetch.Path
	a.selectedFetchResp = nil
	for _, variant := range fetch.PathVariants {
		if pathVariantMatches(variant, body, nil) {
			path = variant.Path
			a.selectedFetchResp = variant.Response
			break
		}
	}
	url := joinURL(baseUrl, strings.ReplaceAll(path, "{task_id}", taskID))
	method := strings.ToUpper(strings.TrimSpace(fetch.Method))
	if method == "" {
		method = http.MethodGet
	}
	var requestBody io.Reader
	if method != http.MethodGet {
		payload := map[string]any{"task_id": taskID}
		payloadBytes, err := common.Marshal(payload)
		if err != nil {
			return nil, err
		}
		requestBody = bytes.NewReader(payloadBytes)
	}
	req, err := http.NewRequest(method, url, requestBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if requestBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if strings.TrimSpace(key) != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	ApplyConfiguredHeaders(req, profile.videoFetch().Headers, key, taskID)
	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		return nil, fmt.Errorf("new proxy http client failed: %w", err)
	}
	if client == nil {
		client = http.DefaultClient
	}
	return client.Do(req)
}

func (a *TaskAdaptor) ParseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
	profile, err := a.requireProfile()
	if err != nil {
		return nil, err
	}
	resp := profile.videoFetch().Response
	if a.selectedFetchResp != nil {
		resp = *a.selectedFetchResp
	}
	return ParseConfiguredTaskInfo(resp, respBody), nil
}

func ParseConfiguredTaskInfo(resp ResponseConfig, respBody []byte) *relaycommon.TaskInfo {
	status := mapStatus(resp.StatusMap, gjson.GetBytes(respBody, resp.StatusPath).String())
	info := &relaycommon.TaskInfo{
		TaskID:   gjson.GetBytes(respBody, resp.TaskIDPath).String(),
		Status:   status,
		Reason:   configuredTaskFailureReason(resp, respBody),
		Url:      gjson.GetBytes(respBody, resp.ResultURLPath).String(),
		Progress: progressString(gjson.GetBytes(respBody, resp.ProgressPath)),
	}
	totalTokensPath := strings.TrimSpace(resp.TotalTokensPath)
	if totalTokensPath == "" {
		totalTokensPath = "usage.total_tokens"
	}
	if totalTokens := int(gjson.GetBytes(respBody, totalTokensPath).Int()); totalTokens > 0 {
		info.TotalTokens = totalTokens
	}
	completionTokensPath := strings.TrimSpace(resp.CompletionTokensPath)
	if completionTokensPath == "" {
		completionTokensPath = "usage.completion_tokens"
	}
	if completionTokens := int(gjson.GetBytes(respBody, completionTokensPath).Int()); completionTokens > 0 {
		info.CompletionTokens = completionTokens
		if info.TotalTokens == 0 {
			info.TotalTokens = completionTokens
		}
	}
	if info.Progress == "" {
		switch model.TaskStatus(status) {
		case model.TaskStatusSuccess, model.TaskStatusFailure:
			info.Progress = "100%"
		case model.TaskStatusInProgress:
			info.Progress = "30%"
		case model.TaskStatusQueued, model.TaskStatusSubmitted:
			info.Progress = "10%"
		}
	}
	return info
}

func configuredTaskFailureReason(resp ResponseConfig, respBody []byte) string {
	if reason := firstJSONString(respBody, resp.ReasonPath); reason != "" {
		return reason
	}
	return firstJSONString(respBody, commonVideoFailureReasonPaths...)
}

func (a *TaskAdaptor) ConvertToOpenAIVideo(originTask *model.Task) ([]byte, error) {
	profile := profileForTask(originTask)
	video := dto.NewOpenAIVideo()
	video.ID = originTask.TaskID
	video.Model = originTask.Properties.OriginModelName
	video.Status = configurableVideoStatus(originTask.Status)
	video.SetProgressStr(originTask.Progress)
	video.CreatedAt = originTask.CreatedAt
	if originTask.Status == model.TaskStatusSuccess || originTask.Status == model.TaskStatusFailure {
		video.CompletedAt = originTask.UpdatedAt
	}
	resultURL := firstJSONString(originTask.Data, commonVideoResultURLPaths...)
	if resultURL == "" {
		resultURL = originTask.GetResultURL()
	}
	if resultURL != "" {
		video.SetMetadata("url", resultURL)
	}
	if originTask.Status == model.TaskStatusFailure {
		reason := firstJSONString(originTask.Data, commonVideoFailureReasonPaths...)
		if reason == "" {
			reason = originTask.FailReason
		}
		video.Error = &dto.OpenAIVideoError{Message: reason, Code: "failed"}
	}
	body, err := common.Marshal(video)
	if err != nil {
		return nil, err
	}
	if profile == nil {
		return body, nil
	}
	return applyOpenAIVideoResponseFields(profile.videoFetch().OpenAIResponse, body, originTask.Data, &relaycommon.RelayInfo{
		OriginModelName: originTask.Properties.OriginModelName,
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: originTask.Properties.UpstreamModelName,
		},
		TaskRelayInfo: &relaycommon.TaskRelayInfo{
			PublicTaskID: originTask.TaskID,
		},
	})
}

func configurableVideoStatus(status model.TaskStatus) string {
	if status == model.TaskStatusNotStart {
		return string(model.TaskStatusNotStart)
	}
	return status.ToVideoStatus()
}

var commonVideoResultURLPaths = []string{
	"video.url",
	"video_url",
	"url",
	"output.video_url",
	"output.url",
	"output.video.url",
	"output.results.0.url",
	"output.videos.0.url",
	"data.video_url",
	"data.url",
	"data.video.url",
	"data.result.url",
	"data.results.0.url",
	"data.videos.0.url",
	"data.task_result.videos.0.url",
	"result.video_url",
	"result.url",
	"result.video.url",
	"result.videos.0.url",
}

var commonVideoFailureReasonPaths = []string{
	"task.metadata.error.message",
	"task.error.message",
	"task.error",
	"error.message",
	"error",
	"output.message",
	"output.error.message",
	"data.message",
	"data.error.message",
	"result.message",
	"message",
}

func firstJSONString(data []byte, paths ...string) string {
	for _, path := range paths {
		result := gjson.GetBytes(data, path)
		if !result.Exists() || result.Type != gjson.String {
			continue
		}
		if value := strings.TrimSpace(result.String()); value != "" {
			return value
		}
	}
	return ""
}

func profileForTask(task *model.Task) *Profile {
	if task == nil || task.ChannelId <= 0 {
		return nil
	}
	channelModel, err := model.CacheGetChannel(task.ChannelId)
	if err != nil || channelModel == nil || channelModel.Type != constant.ChannelTypeConfigurable {
		return nil
	}
	setting := channelModel.GetSetting()
	if setting.Protocol == nil || strings.TrimSpace(setting.Protocol.ProfileID) == "" {
		return nil
	}
	profile, ok := GetProfile(setting.Protocol.ProfileID)
	if !ok {
		return nil
	}
	return profile
}

func (a *TaskAdaptor) ConvertToNativeFetchResponse(originTask *model.Task, upstream []byte) ([]byte, error) {
	profile, err := a.requireProfile()
	if err != nil {
		return nil, err
	}
	if strings.EqualFold(strings.TrimSpace(profile.videoNative().Fetch.ResponseFormat), "task_response") {
		if len(upstream) > 0 {
			originTask.Data = upstream
		}
		return common.Marshal(dto.TaskResponse[any]{
			Code: "success",
			Data: configurableTaskModel2Dto(originTask),
		})
	}
	return buildConfiguredResponse(profile.videoNative().Fetch.Response, upstream, &relaycommon.RelayInfo{
		TaskRelayInfo: &relaycommon.TaskRelayInfo{
			PublicTaskID: originTask.TaskID,
		},
	})
}

func configurableTaskModel2Dto(task *model.Task) *dto.TaskDto {
	return &dto.TaskDto{
		ID:         task.ID,
		CreatedAt:  task.CreatedAt,
		UpdatedAt:  task.UpdatedAt,
		TaskID:     task.TaskID,
		Platform:   string(task.Platform),
		UserId:     task.UserId,
		Group:      task.Group,
		ChannelId:  task.ChannelId,
		Quota:      task.Quota,
		Action:     task.Action,
		Status:     string(task.Status),
		FailReason: task.FailReason,
		ResultURL:  task.GetResultURL(),
		SubmitTime: task.SubmitTime,
		StartTime:  task.StartTime,
		FinishTime: task.FinishTime,
		Progress:   task.Progress,
		Properties: task.Properties,
		Username:   task.Username,
		Data:       task.Data,
	}
}

func (a *TaskAdaptor) GetModelList() []string {
	return nil
}

func (a *TaskAdaptor) GetChannelName() string {
	if a.profile != nil && a.profile.Name != "" {
		return a.profile.Name
	}
	return "Configurable"
}

func (a *TaskAdaptor) requireProfile() (*Profile, error) {
	if a.profile == nil {
		return nil, fmt.Errorf("configurable protocol profile not found")
	}
	return a.profile, nil
}

func (a *TaskAdaptor) selectVideoSubmitPath(req relaycommon.TaskSubmitReq, info *relaycommon.RelayInfo) string {
	if a.profile == nil {
		return ""
	}
	source, err := taskRequestSource(req)
	if err != nil {
		return ""
	}
	return selectEndpointPath(a.profile.videoSubmit(), source, info)
}

func taskRequestSource(req relaycommon.TaskSubmitReq) (map[string]any, error) {
	data, err := common.Marshal(req)
	if err != nil {
		return nil, err
	}
	var source map[string]any
	if err := common.Unmarshal(data, &source); err != nil {
		return nil, err
	}
	return source, nil
}

func resolveProfile(info *relaycommon.RelayInfo) *Profile {
	profileID := "generic-video-json"
	if info != nil && info.ChannelMeta != nil && info.ChannelMeta.ChannelSetting.Protocol != nil {
		if configured := strings.TrimSpace(info.ChannelMeta.ChannelSetting.Protocol.ProfileID); configured != "" {
			profileID = configured
		}
	}
	profile, ok := GetProfile(profileID)
	if !ok {
		return nil
	}
	return profile
}

func (a *TaskAdaptor) isNativeSubmitRequest(c *gin.Context) bool {
	return a.profile != nil &&
		c != nil &&
		c.Request != nil &&
		nativeEndpointMatches(a.profile.videoNative().Submit, c.Request.Method, c.Request.URL.Path)
}

func nativeEndpointMatches(endpoint NativeEndpointConfig, method, path string) bool {
	return strings.EqualFold(strings.TrimSpace(endpoint.Method), strings.TrimSpace(method)) &&
		nativePathMatches(endpoint.Path, path)
}

func (a *TaskAdaptor) parseNativeTaskRequest(c *gin.Context) (relaycommon.TaskSubmitReq, error) {
	if a.profile == nil {
		return relaycommon.TaskSubmitReq{}, fmt.Errorf("configurable protocol profile not found")
	}
	var body map[string]any
	if err := common.UnmarshalBodyReusable(c, &body); err != nil {
		return relaycommon.TaskSubmitReq{}, err
	}
	mapped, err := buildMappedMap(a.profile.videoNative().Submit.Request.Fields, body, nil)
	if err != nil {
		return relaycommon.TaskSubmitReq{}, err
	}
	data, err := common.Marshal(mapped)
	if err != nil {
		return relaycommon.TaskSubmitReq{}, err
	}
	var req relaycommon.TaskSubmitReq
	if err := common.Unmarshal(data, &req); err != nil {
		return relaycommon.TaskSubmitReq{}, err
	}
	if strings.TrimSpace(req.Prompt) == "" {
		return relaycommon.TaskSubmitReq{}, fmt.Errorf("prompt is required")
	}
	if req.Metadata == nil {
		req.Metadata = map[string]interface{}{}
	}
	return req, nil
}

func buildMappedBody(fields []FieldMapping, req relaycommon.TaskSubmitReq, info *relaycommon.RelayInfo) (map[string]any, error) {
	var source map[string]any
	reqBytes, err := common.Marshal(req)
	if err != nil {
		return nil, err
	}
	if err := common.Unmarshal(reqBytes, &source); err != nil {
		return nil, err
	}
	return buildMappedMap(fields, source, info)
}

func buildMappedMap(fields []FieldMapping, source map[string]any, info *relaycommon.RelayInfo) (map[string]any, error) {
	result := "{}"
	for _, field := range fields {
		if !fieldMatches(field, source, info) {
			continue
		}
		value := field.Value
		if field.From != "" {
			value = valueFromSource(field.From, source, info)
		}
		value = firstFallbackValue(value, field, func(path string) any {
			return valueFromSource(path, source, info)
		})
		var err error
		value, err = applyFieldTransform(field.Transform, value, field)
		if err != nil {
			return nil, errors.Wrapf(err, "transform field %s", field.To)
		}
		value = applyFieldValueMap(value, field)
		if field.OmitEmpty && isEmptyValue(value) {
			continue
		}
		if field.Append {
			result, err = appendJSONValue(result, field.To, value)
		} else {
			result, err = sjson.Set(result, field.To, value)
		}
		if err != nil {
			return nil, errors.Wrapf(err, "set field %s", field.To)
		}
	}
	var body map[string]any
	if err := common.Unmarshal([]byte(result), &body); err != nil {
		return nil, err
	}
	return body, nil
}

func BuildMappedMap(fields []FieldMapping, source map[string]any, info *relaycommon.RelayInfo) (map[string]any, error) {
	return buildMappedMap(fields, source, info)
}

func appendJSONValue(jsonText string, path string, value any) (string, error) {
	if isEmptyValue(value) {
		return jsonText, nil
	}
	existing := gjson.Get(jsonText, path)
	var values []any
	if existing.Exists() {
		if existing.IsArray() {
			values = existing.Value().([]any)
		} else {
			values = append(values, existing.Value())
		}
	}
	switch v := value.(type) {
	case []map[string]string:
		for _, item := range v {
			values = append(values, item)
		}
	case []map[string]any:
		for _, item := range v {
			values = append(values, item)
		}
	case []any:
		values = append(values, v...)
	default:
		values = append(values, value)
	}
	return sjson.Set(jsonText, path, values)
}

func valueFromSource(path string, request map[string]any, info *relaycommon.RelayInfo) any {
	switch path {
	case "upstream_model":
		if info != nil && info.ChannelMeta != nil && info.ChannelMeta.UpstreamModelName != "" {
			return info.ChannelMeta.UpstreamModelName
		}
		if info != nil {
			return info.OriginModelName
		}
		return ""
	case "origin_model":
		if info != nil {
			return info.OriginModelName
		}
		return ""
	}
	if strings.HasPrefix(path, "request.") {
		path = strings.TrimPrefix(path, "request.")
	}
	if strings.HasPrefix(path, "body.") {
		path = strings.TrimPrefix(path, "body.")
	}
	data, _ := common.Marshal(request)
	value := gjson.GetBytes(data, path)
	if !value.Exists() {
		return nil
	}
	return value.Value()
}

func fieldMatches(field FieldMapping, source map[string]any, info *relaycommon.RelayInfo) bool {
	for _, condition := range field.Conditions {
		if !pathConditionMatches(condition, source, info) {
			return false
		}
	}
	if strings.TrimSpace(field.WhenModelContains) == "" {
		return true
	}
	modelName := strings.ToLower(resolveSourceModel(source, info))
	return strings.Contains(modelName, strings.ToLower(strings.TrimSpace(field.WhenModelContains)))
}

func resolveSourceModel(source map[string]any, info *relaycommon.RelayInfo) string {
	if info != nil && info.ChannelMeta != nil && info.ChannelMeta.UpstreamModelName != "" {
		return info.ChannelMeta.UpstreamModelName
	}
	if info != nil && info.OriginModelName != "" {
		return info.OriginModelName
	}
	if source != nil {
		if modelName, ok := source["model"].(string); ok {
			return modelName
		}
	}
	return ""
}

func applyFieldTransform(transform string, value any, field FieldMapping) (any, error) {
	switch strings.TrimSpace(transform) {
	case "":
		return value, nil
	case "to_int":
		return toIntValue(value)
	case "media_objects":
		return mediaObjects(value, field), nil
	case "seedance_text_content":
		return seedanceTextContent(value), nil
	case "seedance_image_content":
		return seedanceMediaContent(value, "image_url", "reference_image", field), nil
	case "seedance_video_content":
		return seedanceMediaContent(value, "video_url", "reference_video", field), nil
	case "seedance_audio_content":
		return seedanceMediaContent(value, "audio_url", "reference_audio", field), nil
	case "image_mode":
		return imageMode(value), nil
	case "image_aspect_ratio":
		return imageAspectRatio(value), nil
	case "image_resolution":
		return imageResolution(value), nil
	case "wavespeed_image_aspect_ratio":
		return wavespeedImageAspectRatio(value), nil
	case "wavespeed_image_resolution":
		return wavespeedImageResolution(value), nil
	case "image_resolution_upper":
		return imageResolutionUpper(value), nil
	case "moxing_aspect_ratio":
		return moxingAspectRatio(value), nil
	case "image_quality":
		return imageQuality(value), nil
	default:
		return nil, fmt.Errorf("unsupported transform %q", transform)
	}
}

func applyFieldValueMap(value any, field FieldMapping) any {
	if len(field.ValueMap) == 0 || isEmptyValue(value) {
		return value
	}
	if mapped, ok := field.ValueMap[fmt.Sprint(value)]; ok {
		return mapped
	}
	if text, ok := value.(string); ok {
		if mapped, ok := field.ValueMap[strings.ToLower(strings.TrimSpace(text))]; ok {
			return mapped
		}
	}
	return value
}

func seedanceTextContent(value any) []map[string]any {
	text, ok := value.(string)
	if !ok || strings.TrimSpace(text) == "" {
		return nil
	}
	return []map[string]any{{
		"type": "text",
		"text": text,
	}}
}

func seedanceMediaContent(value any, contentType, role string, field FieldMapping) []map[string]any {
	urls := stringValues(value)
	if field.FirstOnly && len(urls) > 1 {
		urls = urls[:1]
	}
	content := make([]map[string]any, 0, len(urls))
	for _, url := range urls {
		if strings.TrimSpace(url) == "" {
			continue
		}
		content = append(content, map[string]any{
			"type": contentType,
			contentType: map[string]string{
				"url": url,
			},
			"role": role,
		})
	}
	return content
}

func toIntValue(value any) (any, error) {
	if value == nil {
		return nil, nil
	}
	switch v := value.(type) {
	case int:
		return v, nil
	case int64:
		return int(v), nil
	case float64:
		return int(v), nil
	case string:
		if strings.TrimSpace(v) == "" {
			return nil, nil
		}
		intVal, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil {
			return nil, err
		}
		return intVal, nil
	default:
		return value, nil
	}
}

func floatValue(value any) float64 {
	switch v := value.(type) {
	case int:
		return float64(v)
	case int64:
		return float64(v)
	case float64:
		return v
	case string:
		f, _ := strconv.ParseFloat(strings.TrimSpace(v), 64)
		return f
	default:
		return 0
	}
}

func mediaObjects(value any, field FieldMapping) []map[string]string {
	mediaType := strings.TrimSpace(field.MediaType)
	if mediaType == "" {
		return nil
	}
	urls := stringValues(value)
	if field.FirstOnly && len(urls) > 1 {
		urls = urls[:1]
	}
	media := make([]map[string]string, 0, len(urls))
	for _, url := range urls {
		if strings.TrimSpace(url) != "" {
			media = append(media, map[string]string{"type": mediaType, "url": url})
		}
	}
	return media
}

func stringValues(value any) []string {
	switch v := value.(type) {
	case string:
		if strings.TrimSpace(v) == "" {
			return nil
		}
		return []string{v}
	case []string:
		return v
	case []any:
		values := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				values = append(values, s)
			}
		}
		return values
	default:
		return nil
	}
}

func isEmptyValue(value any) bool {
	if value == nil {
		return true
	}
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v) == ""
	case []any:
		return len(v) == 0
	case []string:
		return len(v) == 0
	case map[string]any:
		return len(v) == 0
	}
	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.Chan, reflect.Map, reflect.Pointer, reflect.Slice:
		return rv.IsNil() || rv.Len() == 0
	}
	return false
}

func mapStatus(statusMap map[string]string, upstream string) string {
	if upstream == "" {
		return ""
	}
	if mapped, ok := statusMap[strings.ToLower(upstream)]; ok {
		return mapped
	}
	switch strings.ToUpper(upstream) {
	case string(model.TaskStatusSuccess), string(model.TaskStatusFailure), string(model.TaskStatusInProgress), string(model.TaskStatusQueued), string(model.TaskStatusSubmitted):
		return strings.ToUpper(upstream)
	}
	return string(model.TaskStatusUnknown)
}

func buildConfiguredResponse(config ResponseConfig, upstream []byte, info *relaycommon.RelayInfo) ([]byte, error) {
	var source map[string]any
	if len(upstream) > 0 {
		if err := common.Unmarshal(upstream, &source); err != nil {
			return nil, err
		}
	}
	if config.Passthrough {
		for _, field := range config.Fields {
			value := field.Value
			if field.From != "" {
				value = responseValueFromSource(field.From, source, info)
			}
			value = firstFallbackValue(value, field, func(path string) any {
				return responseValueFromSource(path, source, info)
			})
			var err error
			value, err = applyFieldTransform(field.Transform, value, field)
			if err != nil {
				return nil, err
			}
			value = applyFieldValueMap(value, field)
			if field.OmitEmpty && isEmptyValue(value) {
				continue
			}
			upstream, err = sjson.SetBytes(upstream, field.To, value)
			if err != nil {
				return nil, err
			}
		}
		return upstream, nil
	}
	body, err := buildMappedMap(config.Fields, source, info)
	if err != nil {
		return nil, err
	}
	return common.Marshal(body)
}

func BuildConfiguredResponse(config ResponseConfig, upstream []byte, info *relaycommon.RelayInfo) ([]byte, error) {
	return buildConfiguredResponse(config, upstream, info)
}

func firstFallbackValue(value any, field FieldMapping, sourceValue func(string) any) any {
	if !isEmptyValue(value) {
		return value
	}
	fallbacks := make([]string, 0, 1+len(field.FallbackFroms))
	if strings.TrimSpace(field.FallbackFrom) != "" {
		fallbacks = append(fallbacks, field.FallbackFrom)
	}
	fallbacks = append(fallbacks, field.FallbackFroms...)
	for _, fallback := range fallbacks {
		fallback = strings.TrimSpace(fallback)
		if fallback == "" {
			continue
		}
		value = sourceValue(fallback)
		if !isEmptyValue(value) {
			return value
		}
	}
	return value
}

func applyOpenAIVideoResponseFields(config ResponseConfig, body []byte, upstream []byte, info *relaycommon.RelayInfo) ([]byte, error) {
	if len(config.Fields) == 0 {
		return body, nil
	}
	var source map[string]any
	if len(upstream) > 0 {
		if err := common.Unmarshal(upstream, &source); err != nil {
			return nil, err
		}
	}
	for _, field := range config.Fields {
		value := field.Value
		if field.From != "" {
			value = responseValueFromSource(field.From, source, info)
		}
		value = firstFallbackValue(value, field, func(path string) any {
			return responseValueFromSource(path, source, info)
		})
		var err error
		value, err = applyFieldTransform(field.Transform, value, field)
		if err != nil {
			return nil, err
		}
		value = applyFieldValueMap(value, field)
		if field.OmitEmpty && isEmptyValue(value) {
			continue
		}
		body, err = sjson.SetBytes(body, field.To, value)
		if err != nil {
			return nil, err
		}
	}
	return body, nil
}

func responseValueFromSource(path string, source map[string]any, info *relaycommon.RelayInfo) any {
	switch strings.TrimSpace(path) {
	case "public_task_id":
		return publicTaskID(info)
	case "upstream_model":
		return resolveSourceModel(source, info)
	}
	if strings.HasPrefix(path, "upstream.") {
		path = strings.TrimPrefix(path, "upstream.")
	}
	return valueFromSource(path, source, info)
}

func progressString(result gjson.Result) string {
	if !result.Exists() {
		return ""
	}
	if result.Type == gjson.Number {
		return fmt.Sprintf("%d%%", int(result.Float()))
	}
	progress := strings.TrimSpace(result.String())
	if progress == "" {
		return ""
	}
	if strings.HasSuffix(progress, "%") {
		return progress
	}
	return progress + "%"
}

func publicTaskID(info *relaycommon.RelayInfo) string {
	if info != nil && info.TaskRelayInfo != nil && info.TaskRelayInfo.PublicTaskID != "" {
		return info.TaskRelayInfo.PublicTaskID
	}
	return ""
}

func joinURL(baseURL, path string) string {
	return strings.TrimRight(baseURL, "/") + "/" + strings.TrimLeft(path, "/")
}

func ApplyConfiguredHeaders(req *http.Request, headers []HeaderConfig, apiKey string, taskID string) {
	for _, header := range headers {
		name := strings.TrimSpace(header.Name)
		if name == "" {
			continue
		}
		value := strings.ReplaceAll(header.Value, "{api_key}", apiKey)
		value = strings.ReplaceAll(value, "{task_id}", taskID)
		value = applyHeaderTransform(header.Transform, value, apiKey)
		req.Header.Set(name, value)
	}
}

func applyHeaderTransform(transform string, value string, apiKey string) string {
	switch strings.ToLower(strings.TrimSpace(transform)) {
	case "":
		return value
	case "kling_jwt":
		token, err := buildKlingJWT(apiKey)
		if err != nil {
			return strings.ReplaceAll(value, "{kling_jwt}", apiKey)
		}
		return strings.ReplaceAll(value, "{kling_jwt}", token)
	default:
		return value
	}
}

func buildKlingJWT(apiKey string) (string, error) {
	parts := strings.Split(apiKey, "|")
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid kling api_key, required format access_key|secret_key")
	}
	accessKey := strings.TrimSpace(parts[0])
	secretKey := strings.TrimSpace(parts[1])
	if accessKey == "" || secretKey == "" {
		return "", fmt.Errorf("invalid kling api_key, required format access_key|secret_key")
	}
	now := time.Now().Unix()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"iss": accessKey,
		"exp": now + 1800,
		"nbf": now - 5,
	})
	token.Header["typ"] = "JWT"
	return token.SignedString([]byte(secretKey))
}

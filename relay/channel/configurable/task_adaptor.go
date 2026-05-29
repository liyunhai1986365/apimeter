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
	"github.com/pkg/errors"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

type TaskAdaptor struct {
	taskcommon.BaseBilling
	apiKey  string
	baseURL string
	profile *Profile
}

func ExtractNativeModel(c *gin.Context, profile *Profile) (string, error) {
	if c == nil || profile == nil {
		return "", fmt.Errorf("invalid native profile context")
	}
	var body map[string]any
	if err := common.UnmarshalBodyReusable(c, &body); err != nil {
		return "", err
	}
	mapped, err := buildMappedMap(profile.Native.Submit.Request.Fields, body, nil)
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
		return nil
	}
	return relaycommon.ValidateBasicTaskRequest(c, info, constant.TaskActionGenerate)
}

func (a *TaskAdaptor) BuildRequestURL(info *relaycommon.RelayInfo) (string, error) {
	profile, err := a.requireProfile()
	if err != nil {
		return "", err
	}
	return joinURL(a.baseURL, profile.Submit.Path), nil
}

func (a *TaskAdaptor) BuildRequestHeader(c *gin.Context, req *http.Request, info *relaycommon.RelayInfo) error {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if strings.TrimSpace(a.apiKey) != "" {
		req.Header.Set("Authorization", "Bearer "+a.apiKey)
	}
	if a.profile != nil {
		applyConfiguredHeaders(req, a.profile.Submit.Headers, a.apiKey, "")
	}
	return nil
}

func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	profile, err := a.requireProfile()
	if err != nil {
		return nil, err
	}
	if a.isNativeSubmitRequest(c) && profile.Native.Submit.Passthrough {
		bodyStorage, err := common.GetBodyStorage(c)
		if err != nil {
			return nil, err
		}
		if _, err := bodyStorage.Seek(0, io.SeekStart); err != nil {
			return nil, err
		}
		return bodyStorage, nil
	}
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return nil, err
	}
	body, err := buildMappedBody(profile.Submit.Body.Fields, req, info)
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
	if a.profile == nil || len(a.profile.Billing.Ratios) == 0 {
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
	for _, ratio := range a.profile.Billing.Ratios {
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

	taskID := strings.TrimSpace(gjson.GetBytes(responseBody, profile.Submit.Response.TaskIDPath).String())
	if taskID == "" {
		return "", nil, service.TaskErrorWrapper(fmt.Errorf("task_id is empty"), "invalid_response", http.StatusInternalServerError)
	}

	if a.isNativeSubmitRequest(c) {
		nativeResponse, err := buildConfiguredResponse(profile.Native.Submit.Response, responseBody, info)
		if err != nil {
			return "", nil, service.TaskErrorWrapper(err, "build_native_response_failed", http.StatusInternalServerError)
		}
		c.Data(http.StatusOK, "application/json", nativeResponse)
	} else {
		ov := dto.NewOpenAIVideo()
		ov.ID = publicTaskID(info)
		ov.TaskID = publicTaskID(info)
		ov.Model = info.OriginModelName
		ov.CreatedAt = time.Now().Unix()
		if status := mapStatus(profile.Submit.Response.StatusMap, gjson.GetBytes(responseBody, profile.Submit.Response.StatusPath).String()); status != "" {
			ov.Status = model.TaskStatus(status).ToVideoStatus()
		}
		ovBody, err := common.Marshal(ov)
		if err != nil {
			return "", nil, service.TaskErrorWrapper(err, "marshal_openai_video_failed", http.StatusInternalServerError)
		}
		ovBody, err = applyOpenAIVideoResponseFields(profile.Submit.OpenAIResponse, ovBody, responseBody, info)
		if err != nil {
			return "", nil, service.TaskErrorWrapper(err, "build_openai_video_response_failed", http.StatusInternalServerError)
		}
		c.Data(http.StatusOK, "application/json", ovBody)
	}
	return taskID, responseBody, nil
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
	url := joinURL(baseUrl, strings.ReplaceAll(profile.Fetch.Path, "{task_id}", taskID))
	method := strings.ToUpper(strings.TrimSpace(profile.Fetch.Method))
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
	applyConfiguredHeaders(req, profile.Fetch.Headers, key, taskID)
	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		return nil, fmt.Errorf("new proxy http client failed: %w", err)
	}
	return client.Do(req)
}

func (a *TaskAdaptor) ParseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
	profile, err := a.requireProfile()
	if err != nil {
		return nil, err
	}
	resp := profile.Fetch.Response
	status := mapStatus(resp.StatusMap, gjson.GetBytes(respBody, resp.StatusPath).String())
	info := &relaycommon.TaskInfo{
		TaskID:   gjson.GetBytes(respBody, resp.TaskIDPath).String(),
		Status:   status,
		Reason:   gjson.GetBytes(respBody, resp.ReasonPath).String(),
		Url:      gjson.GetBytes(respBody, resp.ResultURLPath).String(),
		Progress: progressString(gjson.GetBytes(respBody, resp.ProgressPath)),
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
	return info, nil
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
	return applyOpenAIVideoResponseFields(profile.Fetch.OpenAIResponse, body, originTask.Data, &relaycommon.RelayInfo{
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
	"error.message",
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
	return buildConfiguredResponse(profile.Native.Fetch.Response, upstream, &relaycommon.RelayInfo{
		TaskRelayInfo: &relaycommon.TaskRelayInfo{
			PublicTaskID: originTask.TaskID,
		},
	})
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
		nativeEndpointMatches(a.profile.Native.Submit, c.Request.Method, c.Request.URL.Path)
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
	mapped, err := buildMappedMap(a.profile.Native.Submit.Request.Fields, body, nil)
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
		if isEmptyValue(value) && field.FallbackFrom != "" {
			value = valueFromSource(field.FallbackFrom, source, info)
		}
		var err error
		value, err = applyFieldTransform(field.Transform, value, field)
		if err != nil {
			return nil, errors.Wrapf(err, "transform field %s", field.To)
		}
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
	default:
		return nil, fmt.Errorf("unsupported transform %q", transform)
	}
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
			var err error
			value, err = applyFieldTransform(field.Transform, value, field)
			if err != nil {
				return nil, err
			}
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
		var err error
		value, err = applyFieldTransform(field.Transform, value, field)
		if err != nil {
			return nil, err
		}
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

func applyConfiguredHeaders(req *http.Request, headers []HeaderConfig, apiKey string, taskID string) {
	for _, header := range headers {
		name := strings.TrimSpace(header.Name)
		if name == "" {
			continue
		}
		value := strings.ReplaceAll(header.Value, "{api_key}", apiKey)
		value = strings.ReplaceAll(value, "{task_id}", taskID)
		req.Header.Set(name, value)
	}
}

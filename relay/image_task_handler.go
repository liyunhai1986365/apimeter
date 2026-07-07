package relay

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/configurable"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

type imageResponseCapture struct {
	gin.ResponseWriter
	status int
	header http.Header
	body   bytes.Buffer
}

func newImageResponseCapture(writer gin.ResponseWriter) *imageResponseCapture {
	return &imageResponseCapture{
		ResponseWriter: writer,
		status:         http.StatusOK,
		header:         make(http.Header),
	}
}

func (w *imageResponseCapture) Header() http.Header {
	if w.header == nil {
		w.header = make(http.Header)
	}
	return w.header
}

func (w *imageResponseCapture) WriteHeader(code int) {
	w.status = code
}

func (w *imageResponseCapture) Write(data []byte) (int, error) {
	return w.body.Write(data)
}

func (w *imageResponseCapture) WriteString(s string) (int, error) {
	return w.body.WriteString(s)
}

func (w *imageResponseCapture) Status() int {
	return w.status
}

func (w *imageResponseCapture) Size() int {
	return w.body.Len()
}

func (w *imageResponseCapture) Written() bool {
	return w.body.Len() > 0
}

func (w *imageResponseCapture) WriteHeaderNow() {
	if w.status == 0 {
		w.status = http.StatusOK
	}
}

func (w *imageResponseCapture) HasBody() bool {
	return w.body.Len() > 0
}

func (w *imageResponseCapture) BodyBytes() []byte {
	return w.body.Bytes()
}

func (w *imageResponseCapture) ReplaceBody(status int, body []byte) {
	w.status = status
	w.body.Reset()
	_, _ = w.body.Write(body)
}

func (w *imageResponseCapture) Flush() {
}

func (w *imageResponseCapture) WriteCaptured() error {
	for k, v := range w.Header() {
		if len(v) == 0 {
			continue
		}
		w.ResponseWriter.Header()[k] = append([]string(nil), v...)
	}
	if w.status != 0 {
		w.ResponseWriter.Header().Set("Content-Length", fmt.Sprintf("%d", w.body.Len()))
		w.ResponseWriter.WriteHeader(w.status)
	}
	if w.body.Len() > 0 {
		if _, err := w.ResponseWriter.Write(w.body.Bytes()); err != nil {
			return err
		}
	}
	return nil
}

type imageAsyncSubmitResponse struct {
	ID string `json:"id"`
}

type imageAsyncTaskResponse struct {
	ID         string                 `json:"id"`
	State      string                 `json:"state"`
	Progress   int                    `json:"progress"`
	CreateTime int64                  `json:"create_time"`
	UpdateTime int64                  `json:"update_time"`
	Action     string                 `json:"action"`
	Data       dto.ImageTaskData      `json:"data"`
	Error      map[string]interface{} `json:"error,omitempty"`
}

const defaultImageAsyncWaitTimeoutSeconds = 600

func parseImageAsyncSubmitResponse(body []byte) (string, bool) {
	var payload imageAsyncSubmitResponse
	if err := common.Unmarshal(body, &payload); err == nil {
		if taskID := strings.TrimSpace(payload.ID); taskID != "" {
			return taskID, true
		}
	}
	return "", false
}

func newImageSubmitUncertainError(err error, errorCode types.ErrorCode, statusCode int) *types.NewAPIError {
	return types.NewOpenAIError(err, errorCode, statusCode, types.ErrOptionWithSkipRetry())
}

func markImageSubmitUncertainError(err *types.NewAPIError) *types.NewAPIError {
	if err == nil {
		return nil
	}
	return newImageSubmitUncertainError(err, err.GetErrorCode(), err.StatusCode)
}

func recordImageAsyncSubmitResponse(info *relaycommon.RelayInfo, responseBody []byte) *types.NewAPIError {
	upstreamTaskID, ok := parseImageAsyncSubmitResponse(responseBody)
	if !ok {
		return nil
	}
	if !info.IsAsyncImageRequest {
		return nil
	}
	return insertImageAsyncTask(info, upstreamTaskID, responseBody)
}

func insertImageAsyncTask(info *relaycommon.RelayInfo, upstreamTaskID string, responseBody []byte) *types.NewAPIError {
	taskID := upstreamTaskID
	task := model.InitTask(constant.TaskPlatform(strconv.Itoa(info.ChannelType)), info)
	task.TaskID = taskID
	task.PrivateData.UpstreamTaskID = upstreamTaskID
	task.PrivateData.AsyncImage = true
	task.PrivateData.BillingSource = info.BillingSource
	task.PrivateData.SubscriptionId = info.SubscriptionId
	task.PrivateData.TokenId = info.TokenId
	task.TokenId = info.TokenId
	if info.TokenId > 0 {
		if token, err := model.GetTokenById(info.TokenId); err == nil {
			task.TokenName = token.Name
		}
	}
	task.PrivateData.BillingContext = &model.TaskBillingContext{
		ModelPrice:      info.PriceData.ModelPrice,
		GroupRatio:      info.PriceData.GroupRatioInfo.GroupRatio,
		ModelRatio:      info.PriceData.ModelRatio,
		OtherRatios:     info.PriceData.OtherRatios,
		OriginModelName: info.OriginModelName,
		PerCallBilling:  info.PriceData.UsePrice,
	}
	task.Quota = info.PriceData.QuotaToPreConsume
	task.Data = responseBody
	task.Action = constant.TaskActionGenerate
	task.Status = model.TaskStatusSubmitted
	task.Progress = "0%"

	if err := task.Insert(); err != nil {
		return newImageSubmitUncertainError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	return nil
}

func waitImageAsyncSubmitResponse(c *gin.Context, info *relaycommon.RelayInfo, responseBody []byte) ([]byte, *types.NewAPIError) {
	upstreamTaskID, ok := parseImageAsyncSubmitResponse(responseBody)
	if !ok || info.IsAsyncImageRequest {
		return nil, nil
	}
	if info.ChannelMeta == nil {
		return nil, newImageSubmitUncertainError(fmt.Errorf("missing channel metadata"), types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}

	baseURL := info.ChannelBaseUrl
	if strings.TrimSpace(baseURL) == "" {
		baseURL = info.ChannelMeta.ChannelBaseUrl
	}
	if baseURL == "" && info.ChannelType >= 0 && info.ChannelType < len(constant.ChannelBaseURLs) {
		baseURL = constant.GetChannelBaseURL(info.ChannelType)
	}
	key := info.ApiKey
	if strings.TrimSpace(key) == "" {
		key = info.ChannelMeta.ApiKey
	}
	proxy := info.ChannelSetting.Proxy

	body, err := waitImageTaskSucceeded(c, baseURL, key, upstreamTaskID, proxy, info.ChannelSetting)
	if err != nil {
		return nil, newImageSubmitUncertainError(err, types.ErrorCodeBadResponseBody, http.StatusBadGateway)
	}
	output, err := convertImageAsyncResultToOpenAIResponse(c, info, body)
	if err != nil {
		return nil, newImageSubmitUncertainError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	return output, nil
}

func waitImageTaskSucceeded(c *gin.Context, baseURL, key, upstreamTaskID, proxy string, settings ...dto.ChannelSettings) ([]byte, error) {
	waitSeconds := 2 * time.Second
	maxStep := imageAsyncWaitMaxStep(c)
	if maxStep <= 0 {
		return nil, fmt.Errorf("image async task wait disabled")
	}
	for step := 0; step < maxStep; step++ {
		if err := imageAsyncWaitContextErr(c); err != nil {
			return nil, err
		}
		body, statusCode, err := fetchImageTaskResultOnce(baseURL, key, upstreamTaskID, proxy, settings...)
		if err != nil {
			return nil, err
		}
		if statusCode < http.StatusOK || statusCode >= http.StatusMultipleChoices {
			return nil, fmt.Errorf("fetch image task status code: %d, body: %s", statusCode, string(body))
		}

		upstream, normalizedBody, err := normalizeImageTaskResponseForRelay(upstreamTaskID, body, configurableImageProfileFromSettingsSlice(settings...))
		if err != nil {
			return nil, err
		}
		status, _, failReason, _ := normalizeImageTaskResultForRelay(upstream)
		switch status {
		case model.TaskStatusSuccess:
			return normalizedBody, nil
		case model.TaskStatusFailure:
			if failReason == "" {
				failReason = "image task failed"
			}
			return nil, errors.New(failReason)
		}

		if step < maxStep-1 {
			if err := sleepImageAsyncWait(c, waitSeconds); err != nil {
				return nil, err
			}
		}
	}
	return nil, fmt.Errorf("image async task wait timeout")
}

func imageAsyncWaitMaxStep(c *gin.Context) int {
	if c != nil {
		if settings, ok := c.Get(string(constant.ContextKeyChannelSetting)); ok {
			if channelSettings, ok := settings.(dto.ChannelSettings); ok &&
				channelSettings.Protocol != nil &&
				channelSettings.Protocol.ImageAsyncWaitTimeoutSeconds != nil {
				return imageAsyncWaitSteps(*channelSettings.Protocol.ImageAsyncWaitTimeoutSeconds)
			}
		}
	}
	return imageAsyncWaitSteps(common.GetEnvOrDefault("IMAGE_ASYNC_WAIT_TIMEOUT_SECONDS", defaultImageAsyncWaitTimeoutSeconds))
}

func imageAsyncWaitSteps(timeoutSeconds int) int {
	if timeoutSeconds <= 0 {
		return 0
	}
	return (timeoutSeconds + 1) / 2
}

func imageAsyncWaitContextErr(c *gin.Context) error {
	if c == nil || c.Request == nil || c.Request.Context() == nil {
		return nil
	}
	if err := c.Request.Context().Err(); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		return err
	}
	return nil
}

func sleepImageAsyncWait(c *gin.Context, d time.Duration) error {
	if c == nil || c.Request == nil || c.Request.Context() == nil {
		time.Sleep(d)
		return nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-c.Request.Context().Done():
		return c.Request.Context().Err()
	case <-timer.C:
		return nil
	}
}

func normalizeImageTaskResponseForRelay(upstreamTaskID string, body []byte, profile *configurable.Profile) (dto.ImageTaskResponse, []byte, error) {
	if profile != nil {
		if upstream, ok, err := configurable.ImageTaskResponseFromProfile(upstreamTaskID, profile, body); ok || err != nil {
			if err != nil {
				return dto.ImageTaskResponse{}, nil, err
			}
			normalized, err := common.Marshal(upstream)
			return upstream, normalized, err
		}
	}

	var upstream dto.ImageTaskResponse
	if err := common.Unmarshal(body, &upstream); err != nil {
		return dto.ImageTaskResponse{}, nil, err
	}
	return upstream, body, nil
}

func convertImageAsyncResultToOpenAIResponse(c *gin.Context, info *relaycommon.RelayInfo, body []byte) ([]byte, error) {
	var taskResp dto.ImageTaskResponse
	if err := common.Unmarshal(body, &taskResp); err != nil {
		return nil, err
	}
	imageResp := dto.ImageResponse{
		Created: common.GetTimestamp(),
	}
	if taskResp.CreateTime != 0 {
		imageResp.Created = taskResp.CreateTime
	} else if info != nil && !info.StartTime.IsZero() {
		imageResp.Created = info.StartTime.Unix()
	}
	for _, image := range taskResp.Data.Images {
		imageData, err := imageTaskImageToOpenAIData(imageStorageContext(c, info), image, buildImageResponsePolicy(info))
		if err != nil {
			return nil, err
		}
		if imageData.Url != "" || imageData.B64Json != "" {
			imageResp.Data = append(imageResp.Data, imageData)
		}
	}
	if len(imageResp.Data) == 0 {
		return nil, fmt.Errorf("image task succeeded without images")
	}
	return common.Marshal(imageResp)
}

type imageResponsePolicy struct {
	format string
	store  string
}

func buildImageResponsePolicy(info *relaycommon.RelayInfo) imageResponsePolicy {
	policy := imageResponsePolicy{
		format: "url",
		store:  dto.TokenImageStoreDefault,
	}
	if info != nil {
		settings := info.TokenImageSettings.Normalized()
		policy.store = settings.Store
		if settings.Format == dto.TokenImageFormatURL || settings.Format == dto.TokenImageFormatB64JSON {
			policy.format = settings.Format
			return policy
		}
		if request, ok := info.Request.(*dto.ImageRequest); ok {
			switch strings.ToLower(strings.TrimSpace(request.ResponseFormat)) {
			case "url":
				policy.format = "url"
				return policy
			case "b64_json", "base64":
				policy.format = "b64_json"
				return policy
			}
		}
		if info.ChannelMeta != nil {
			if responseFormat := info.ChannelSetting.OpenAIImageResponseFormatOverride(); responseFormat != "" {
				policy.format = responseFormat
				return policy
			}
		}
	}
	return policy
}

func imageTaskImageToOpenAIData(c *gin.Context, image dto.ImageTaskImage, policy imageResponsePolicy) (dto.ImageData, error) {
	imageURL := strings.TrimSpace(image.URL)
	imageBase64 := strings.TrimSpace(image.B64Json)
	switch policy.store {
	case dto.TokenImageStoreForceStoreURLAndBase64:
		data, err := imageTaskImageToStoredURLAndBase64(c, imageURL, imageBase64)
		if err != nil {
			return dto.ImageData{}, err
		}
		if policy.format == "b64_json" {
			return dto.ImageData{B64Json: data.B64Json}, nil
		}
		return dto.ImageData{Url: data.Url}, nil
	}
	if policy.format == "b64_json" {
		if imageBase64 != "" {
			_, cleanBase64, err := service.DecodeBase64FileData(imageBase64)
			if err != nil {
				return dto.ImageData{}, err
			}
			return dto.ImageData{B64Json: cleanBase64}, nil
		}
		if imageURL == "" {
			return dto.ImageData{}, nil
		}
		if !isHTTPImageReference(imageURL) {
			_, cleanBase64, err := service.DecodeBase64FileData(imageURL)
			if err != nil {
				return dto.ImageData{}, err
			}
			return dto.ImageData{B64Json: cleanBase64}, nil
		}
		_, base64Data, err := service.GetImageFromUrl(imageURL)
		if err != nil {
			return dto.ImageData{}, err
		}
		return dto.ImageData{B64Json: base64Data}, nil
	}
	if imageURL != "" {
		if !isHTTPImageReference(imageURL) {
			url, err := service.SaveTemporaryImageBase64(c, imageURL)
			if err != nil {
				return dto.ImageData{}, err
			}
			return dto.ImageData{Url: url}, nil
		}
		return dto.ImageData{Url: imageURL}, nil
	}
	if imageBase64 == "" {
		return dto.ImageData{}, nil
	}
	url, err := service.SaveTemporaryImageBase64(c, imageBase64)
	if err != nil {
		return dto.ImageData{}, err
	}
	return dto.ImageData{Url: url}, nil
}

func imageTaskImageToStoredURLAndBase64(c *gin.Context, imageURL, imageBase64 string) (dto.ImageData, error) {
	if imageBase64 != "" {
		_, cleanBase64, err := service.DecodeBase64FileData(imageBase64)
		if err != nil {
			return dto.ImageData{}, err
		}
		url, err := service.SaveTemporaryImageBase64(c, imageBase64)
		if err != nil {
			return dto.ImageData{}, err
		}
		return dto.ImageData{Url: url, B64Json: cleanBase64}, nil
	}
	if imageURL == "" {
		return dto.ImageData{}, nil
	}
	if !isHTTPImageReference(imageURL) {
		_, cleanBase64, err := service.DecodeBase64FileData(imageURL)
		if err != nil {
			return dto.ImageData{}, err
		}
		url, err := service.SaveTemporaryImageBase64(c, imageURL)
		if err != nil {
			return dto.ImageData{}, err
		}
		return dto.ImageData{Url: url, B64Json: cleanBase64}, nil
	}
	_, base64Data, err := service.GetImageFromUrl(imageURL)
	if err != nil {
		return dto.ImageData{}, err
	}
	url, err := service.SaveTemporaryImageBase64(c, base64Data)
	if err != nil {
		return dto.ImageData{}, err
	}
	return dto.ImageData{Url: url, B64Json: base64Data}, nil
}

func isHTTPImageReference(ref string) bool {
	ref = strings.ToLower(strings.TrimSpace(ref))
	return strings.HasPrefix(ref, "http://") || strings.HasPrefix(ref, "https://")
}

func normalizeOpenAIImageResponseByFormat(c *gin.Context, info *relaycommon.RelayInfo, body []byte) ([]byte, bool, error) {
	policy := buildImageResponsePolicy(info)
	storageCtx := imageStorageContext(c, info)
	var payload map[string]any
	if err := common.Unmarshal(body, &payload); err != nil {
		return nil, false, err
	}
	dataValue, ok := payload["data"].([]any)
	if !ok {
		return body, false, nil
	}

	changed := false
	for i, item := range dataValue {
		image, ok := item.(map[string]any)
		if !ok {
			continue
		}
		normalized, err := imageTaskImageToOpenAIData(storageCtx, dto.ImageTaskImage{
			URL:     stringFromImageMap(image, "url"),
			B64Json: stringFromImageMap(image, "b64_json", "base64", "image_base64", "imageBase64"),
		}, policy)
		if err != nil {
			return nil, false, err
		}
		if policy.store == dto.TokenImageStoreForceStoreURLAndBase64 {
			if normalized.Url == "" && normalized.B64Json == "" {
				continue
			}
			if policy.format == "b64_json" {
				if stringFromImageMap(image, "b64_json") != normalized.B64Json {
					image["b64_json"] = normalized.B64Json
					changed = true
				}
				if stringFromImageMap(image, "url") != "" {
					image["url"] = ""
					changed = true
				}
			} else {
				if stringFromImageMap(image, "url") != normalized.Url {
					image["url"] = normalized.Url
					changed = true
				}
				if stringFromImageMap(image, "b64_json") != "" {
					image["b64_json"] = ""
					changed = true
				}
			}
			if _, ok := image["b64_json"]; !ok && policy.format != "b64_json" {
				image["b64_json"] = ""
				changed = true
			}
			if _, ok := image["base64"]; ok {
				image["base64"] = ""
				changed = true
			}
			if _, ok := image["image_base64"]; ok {
				image["image_base64"] = ""
				changed = true
			}
			if _, ok := image["imageBase64"]; ok {
				image["imageBase64"] = ""
				changed = true
			}
			dataValue[i] = image
			continue
		}
		if policy.format == "b64_json" {
			if normalized.B64Json == "" {
				continue
			}
			if stringFromImageMap(image, "b64_json") != normalized.B64Json {
				image["b64_json"] = normalized.B64Json
				changed = true
			}
			if stringFromImageMap(image, "url") != "" {
				image["url"] = ""
				changed = true
			}
			if stringFromImageMap(image, "base64") != "" {
				image["base64"] = ""
				changed = true
			}
			if stringFromImageMap(image, "image_base64") != "" {
				image["image_base64"] = ""
				changed = true
			}
			if stringFromImageMap(image, "imageBase64") != "" {
				image["imageBase64"] = ""
				changed = true
			}
			dataValue[i] = image
			continue
		}
		if normalized.Url == "" {
			continue
		}
		if stringFromImageMap(image, "url") != normalized.Url {
			image["url"] = normalized.Url
			changed = true
		}
		if stringFromImageMap(image, "b64_json") != "" {
			image["b64_json"] = ""
			changed = true
		}
		if _, ok := image["b64_json"]; !ok {
			image["b64_json"] = ""
			changed = true
		}
		if _, ok := image["base64"]; ok {
			image["base64"] = ""
			changed = true
		}
		if _, ok := image["image_base64"]; ok {
			image["image_base64"] = ""
			changed = true
		}
		if _, ok := image["imageBase64"]; ok {
			image["imageBase64"] = ""
			changed = true
		}
		dataValue[i] = image
	}
	if !changed {
		return body, false, nil
	}
	payload["data"] = dataValue
	normalized, err := common.Marshal(payload)
	return normalized, true, err
}

func imageStorageContext(c *gin.Context, info *relaycommon.RelayInfo) *gin.Context {
	if c == nil || info == nil || !info.UserSetting.ImageStorage.IsReady() {
		return c
	}
	if _, ok := common.GetContextKeyType[dto.UserSetting](c, constant.ContextKeyUserSetting); ok {
		return c
	}
	common.SetContextKey(c, constant.ContextKeyUserSetting, info.UserSetting)
	return c
}

func stringFromImageMap(image map[string]any, keys ...string) string {
	for _, key := range keys {
		value, ok := image[key]
		if !ok || value == nil {
			continue
		}
		valueString := strings.TrimSpace(fmt.Sprint(value))
		if valueString != "" {
			return valueString
		}
	}
	return ""
}

func fetchImageTaskResultOnce(baseURL, key, upstreamTaskID, proxy string, settings ...dto.ChannelSettings) ([]byte, int, error) {
	profile := configurableImageProfileFromSettingsSlice(settings...)
	var req *http.Request
	var err error
	if profile != nil {
		req, err = buildConfigurableImageTaskFetchRequest(baseURL, key, upstreamTaskID, profile)
	} else {
		req, err = buildImageTaskFetchRequest(baseURL, key, upstreamTaskID)
	}
	if err != nil {
		return nil, 0, err
	}
	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		return nil, 0, err
	}
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, err
	}
	return body, resp.StatusCode, nil
}

func normalizeImageTaskResultForRelay(resp dto.ImageTaskResponse) (model.TaskStatus, string, string, string) {
	state := strings.ToLower(strings.TrimSpace(resp.State))
	switch state {
	case "running", "processing":
		return model.TaskStatusInProgress, imageTaskProgress(resp.Progress, "0%"), "", firstRelayImageURL(resp.Data.Images)
	case "queued", "submitted", "pending":
		return model.TaskStatusQueued, imageTaskProgress(resp.Progress, "0%"), "", firstRelayImageURL(resp.Data.Images)
	case "succeeded", "success", "done":
		return model.TaskStatusSuccess, imageTaskProgress(resp.Progress, "100%"), "", firstRelayImageURL(resp.Data.Images)
	case "failed", "failure", "error", "cancelled", "canceled":
		return model.TaskStatusFailure, imageTaskProgress(resp.Progress, "100%"), imageTaskFailReasonForRelay(resp.Error), firstRelayImageURL(resp.Data.Images)
	default:
		if len(resp.Data.Images) > 0 {
			return model.TaskStatusSuccess, imageTaskProgress(resp.Progress, "100%"), "", firstRelayImageURL(resp.Data.Images)
		}
		return model.TaskStatusInProgress, imageTaskProgress(resp.Progress, "0%"), "", ""
	}
}

func imageTaskProgress(progress int, fallback string) string {
	if progress <= 0 {
		return fallback
	}
	return fmt.Sprintf("%d%%", progress)
}

func firstRelayImageURL(images []dto.ImageTaskImage) string {
	if len(images) == 0 {
		return ""
	}
	return strings.TrimSpace(images[0].URL)
}

func imageTaskFailReasonForRelay(errData map[string]any) string {
	if len(errData) == 0 {
		return ""
	}
	if msg, ok := errData["message"].(string); ok && strings.TrimSpace(msg) != "" {
		return msg
	}
	if msg, ok := errData["msg"].(string); ok && strings.TrimSpace(msg) != "" {
		return msg
	}
	return ""
}

func ImageTaskFetch(c *gin.Context) *dto.TaskError {
	taskID := c.Param("id")
	if taskID == "" {
		taskID = c.Param("task_id")
	}
	if taskID == "" {
		return service.TaskErrorWrapperLocal(errors.New("task id is required"), "invalid_request", http.StatusBadRequest)
	}

	userID := c.GetInt("id")
	task, exist, err := model.GetByTaskId(userID, taskID)
	if err != nil {
		return service.TaskErrorWrapper(err, "get_task_failed", http.StatusInternalServerError)
	}
	if !exist {
		return service.TaskErrorWrapperLocal(errors.New("task_not_exist"), "task_not_exist", http.StatusBadRequest)
	}

	body, statusCode, err := fetchImageTaskFromUpstream(task)
	if err != nil {
		return service.TaskErrorWrapper(err, "fetch_task_failed", http.StatusInternalServerError)
	}
	if statusCode < http.StatusOK || statusCode >= http.StatusMultipleChoices {
		return service.TaskErrorWrapper(fmt.Errorf("%s", string(body)), "fetch_task_failed", statusCode)
	}

	output, err := convertImageTaskResponse(task, body)
	if err != nil {
		return service.TaskErrorWrapper(err, "convert_task_failed", http.StatusInternalServerError)
	}

	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(http.StatusOK)
	_, _ = c.Writer.Write(output)
	return nil
}

func fetchImageTaskFromUpstream(task *model.Task) ([]byte, int, error) {
	channelModel, err := model.GetChannelById(task.ChannelId, true)
	if err != nil {
		return nil, 0, err
	}
	baseURL := channelModel.GetBaseURL()
	if baseURL == "" && channelModel.Type >= 0 && channelModel.Type < len(constant.ChannelBaseURLs) {
		baseURL = constant.GetChannelBaseURL(channelModel.Type)
	}
	if strings.TrimSpace(baseURL) == "" {
		return nil, 0, fmt.Errorf("channel base url is empty")
	}

	profile := configurableImageProfileFromChannel(channelModel)
	var req *http.Request
	if profile != nil {
		req, err = buildConfigurableImageTaskFetchRequest(baseURL, channelModel.Key, task.GetUpstreamTaskID(), profile)
	} else {
		req, err = buildImageTaskFetchRequest(baseURL, channelModel.Key, task.GetUpstreamTaskID())
	}
	if err != nil {
		return nil, 0, err
	}

	client, err := service.GetHttpClientWithProxy(channelModel.GetSetting().Proxy)
	if err != nil {
		return nil, 0, err
	}
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, err
	}
	return body, resp.StatusCode, nil
}

func FetchConfigurableImageTaskForPolling(ch *model.Channel, task *model.Task, upstreamTaskID, key, proxy string) ([]byte, int, bool, error) {
	if ch == nil || ch.Type != constant.ChannelTypeConfigurable {
		return nil, 0, false, nil
	}
	profile := configurableImageProfileFromChannel(ch)
	if profile == nil {
		return nil, 0, false, nil
	}
	baseURL := ch.GetBaseURL()
	if strings.TrimSpace(baseURL) == "" && ch.Type >= 0 && ch.Type < len(constant.ChannelBaseURLs) {
		baseURL = constant.GetChannelBaseURL(ch.Type)
	}
	if strings.TrimSpace(baseURL) == "" {
		return nil, 0, true, fmt.Errorf("channel base url is empty")
	}
	if strings.TrimSpace(upstreamTaskID) == "" && task != nil {
		upstreamTaskID = task.GetUpstreamTaskID()
	}
	req, err := buildConfigurableImageTaskFetchRequest(baseURL, key, upstreamTaskID, profile)
	if err != nil {
		return nil, 0, true, err
	}
	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		return nil, 0, true, err
	}
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, true, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, true, err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return body, resp.StatusCode, true, nil
	}
	normalized, ok, err := configurable.ImageTaskResponseBytesFromProfile(upstreamTaskID, profile, body)
	if err != nil {
		return nil, resp.StatusCode, true, err
	}
	if ok {
		normalized, err = imageTaskResponseWithURLImages(nil, normalized)
		if err != nil {
			return nil, resp.StatusCode, true, err
		}
		return normalized, resp.StatusCode, true, nil
	}
	return body, resp.StatusCode, true, nil
}

func buildImageTaskFetchRequest(baseURL, key, upstreamTaskID string) (*http.Request, error) {
	taskPath := "/v1/tasks/" + upstreamTaskID
	requestURL := relaycommon.GetFullRequestURL(baseURL, taskPath, constant.ChannelTypeOpenAI)
	req, err := http.NewRequest(http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if strings.TrimSpace(key) != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	return req, nil
}

func buildConfigurableImageTaskFetchRequest(baseURL, key, upstreamTaskID string, profile *configurable.Profile) (*http.Request, error) {
	if profile == nil || !strings.EqualFold(strings.TrimSpace(profile.MediaType), "image") {
		return nil, fmt.Errorf("configurable image profile not found")
	}
	requestURL := strings.TrimRight(baseURL, "/") + "/" + strings.TrimLeft(strings.ReplaceAll(profile.Fetch.Path, "{task_id}", upstreamTaskID), "/")
	method := strings.ToUpper(strings.TrimSpace(profile.Fetch.Method))
	if method == "" {
		method = http.MethodGet
	}
	req, err := http.NewRequest(method, requestURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if strings.TrimSpace(key) != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	configurable.ApplyConfiguredHeaders(req, profile.Fetch.Headers, key, upstreamTaskID)
	return req, nil
}

func configurableImageProfileFromSettingsSlice(settings ...dto.ChannelSettings) *configurable.Profile {
	if len(settings) == 0 {
		return nil
	}
	return configurableImageProfileFromSettings(settings[0])
}

func configurableImageProfileFromSettings(settings dto.ChannelSettings) *configurable.Profile {
	if settings.Protocol == nil || strings.TrimSpace(settings.Protocol.ProfileID) == "" {
		return nil
	}
	profile, ok := configurable.GetProfile(settings.Protocol.ProfileID)
	if !ok || !strings.EqualFold(strings.TrimSpace(profile.MediaType), "image") {
		return nil
	}
	return profile
}

func configurableImageProfileFromChannel(channelModel *model.Channel) *configurable.Profile {
	if channelModel == nil || channelModel.Type != constant.ChannelTypeConfigurable {
		return nil
	}
	return configurableImageProfileFromSettings(channelModel.GetSetting())
}

func convertImageTaskResponse(task *model.Task, body []byte) ([]byte, error) {
	if profile := configurableImageProfileForTask(task); profile != nil {
		if normalized, ok, err := configurable.ImageTaskResponseBytesFromProfile(task.GetUpstreamTaskID(), profile, body); ok || err != nil {
			if err != nil {
				return normalized, err
			}
			return imageTaskResponseWithURLImages(nil, normalized)
		}
	}

	var upstream imageAsyncTaskResponse
	if err := common.Unmarshal(body, &upstream); err != nil {
		return nil, err
	}

	response := dto.ImageTaskResponse{
		ID:         task.TaskID,
		State:      upstream.State,
		Progress:   upstream.Progress,
		CreateTime: firstNonZeroInt64(upstream.CreateTime, task.CreatedAt),
		UpdateTime: firstNonZeroInt64(upstream.UpdateTime, task.UpdatedAt),
		Action:     common.GetStringIfEmpty(upstream.Action, task.Action),
		Data:       upstream.Data,
	}
	if response.State == "" {
		response.State = mapTaskStatusToSimple(task.Status)
	}
	normalized, err := common.Marshal(response)
	if err != nil {
		return nil, err
	}
	return imageTaskResponseWithURLImages(nil, normalized)
}

func imageTaskResponseWithURLImages(c *gin.Context, body []byte) ([]byte, error) {
	var response dto.ImageTaskResponse
	if err := common.Unmarshal(body, &response); err != nil {
		return nil, err
	}
	changed := false
	for i, image := range response.Data.Images {
		if strings.TrimSpace(image.URL) != "" || strings.TrimSpace(image.B64Json) == "" {
			continue
		}
		imageURL, err := service.SaveTemporaryImageBase64(c, image.B64Json)
		if err != nil {
			return nil, err
		}
		response.Data.Images[i].URL = imageURL
		response.Data.Images[i].B64Json = ""
		changed = true
	}
	if !changed {
		return body, nil
	}
	return common.Marshal(response)
}

func configurableImageProfileForTask(task *model.Task) *configurable.Profile {
	if task == nil || task.ChannelId <= 0 {
		return nil
	}
	channelModel, err := model.GetChannelById(task.ChannelId, true)
	if err != nil {
		return nil
	}
	return configurableImageProfileFromChannel(channelModel)
}

func firstNonZeroInt64(value int64, fallback int64) int64 {
	if value != 0 {
		return value
	}
	return fallback
}

package relay

import (
	"bytes"
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
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

type imageResponseCapture struct {
	gin.ResponseWriter
	status int
	body   bytes.Buffer
}

func newImageResponseCapture(writer gin.ResponseWriter) *imageResponseCapture {
	return &imageResponseCapture{
		ResponseWriter: writer,
		status:         http.StatusOK,
	}
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

func (w *imageResponseCapture) WriteCaptured() {
	if w.status != 0 {
		w.ResponseWriter.Header().Set("Content-Length", fmt.Sprintf("%d", w.body.Len()))
		w.ResponseWriter.WriteHeader(w.status)
	}
	if w.body.Len() > 0 {
		_, _ = w.ResponseWriter.Write(w.body.Bytes())
	}
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

func parseImageAsyncSubmitResponse(body []byte) (string, bool) {
	var payload imageAsyncSubmitResponse
	if err := common.Unmarshal(body, &payload); err != nil {
		return "", false
	}
	if strings.TrimSpace(payload.ID) == "" {
		return "", false
	}
	return payload.ID, true
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
		return types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	return nil
}

func waitImageAsyncSubmitResponse(c *gin.Context, info *relaycommon.RelayInfo, responseBody []byte) ([]byte, *types.NewAPIError) {
	upstreamTaskID, ok := parseImageAsyncSubmitResponse(responseBody)
	if !ok || info.IsAsyncImageRequest {
		return nil, nil
	}
	if info.ChannelMeta == nil {
		return nil, types.NewOpenAIError(fmt.Errorf("missing channel metadata"), types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}

	baseURL := info.ChannelBaseUrl
	if baseURL == "" && info.ChannelType >= 0 && info.ChannelType < len(constant.ChannelBaseURLs) {
		baseURL = constant.ChannelBaseURLs[info.ChannelType]
	}
	key := info.ApiKey
	proxy := info.ChannelSetting.Proxy

	body, err := waitImageTaskSucceeded(c, baseURL, key, upstreamTaskID, proxy)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusBadGateway)
	}
	output, err := convertImageAsyncResultToOpenAIResponse(info, body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	return output, nil
}

func waitImageTaskSucceeded(c *gin.Context, baseURL, key, upstreamTaskID, proxy string) ([]byte, error) {
	waitSeconds := 2 * time.Second
	maxStep := 60
	for step := 0; step < maxStep; step++ {
		body, statusCode, err := fetchImageTaskResultOnce(baseURL, key, upstreamTaskID, proxy)
		if err != nil {
			return nil, err
		}
		if statusCode < http.StatusOK || statusCode >= http.StatusMultipleChoices {
			return nil, fmt.Errorf("fetch image task status code: %d, body: %s", statusCode, string(body))
		}

		var upstream dto.ImageTaskResponse
		if err := common.Unmarshal(body, &upstream); err != nil {
			return nil, err
		}
		status, _, failReason, _ := normalizeImageTaskResultForRelay(upstream)
		switch status {
		case model.TaskStatusSuccess:
			return body, nil
		case model.TaskStatusFailure:
			if failReason == "" {
				failReason = "image task failed"
			}
			return nil, errors.New(failReason)
		}

		timer := time.NewTimer(waitSeconds)
		select {
		case <-c.Request.Context().Done():
			timer.Stop()
			return nil, c.Request.Context().Err()
		case <-timer.C:
		}
	}
	return nil, fmt.Errorf("image async task wait timeout")
}

func convertImageAsyncResultToOpenAIResponse(info *relaycommon.RelayInfo, body []byte) ([]byte, error) {
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
		imageResp.Data = append(imageResp.Data, dto.ImageData{
			Url: strings.TrimSpace(image.URL),
		})
	}
	if len(imageResp.Data) == 0 {
		return nil, fmt.Errorf("image task succeeded without images")
	}
	return common.Marshal(imageResp)
}

func fetchImageTaskResultOnce(baseURL, key, upstreamTaskID, proxy string) ([]byte, int, error) {
	req, err := buildImageTaskFetchRequest(baseURL, key, upstreamTaskID)
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
		baseURL = constant.ChannelBaseURLs[channelModel.Type]
	}
	if strings.TrimSpace(baseURL) == "" {
		return nil, 0, fmt.Errorf("channel base url is empty")
	}

	req, err := buildImageTaskFetchRequest(baseURL, channelModel.Key, task.GetUpstreamTaskID())
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

func buildImageTaskFetchRequest(baseURL, key, upstreamTaskID string) (*http.Request, error) {
	requestURL := relaycommon.GetFullRequestURL(baseURL, "/v1/tasks/"+upstreamTaskID, constant.ChannelTypeOpenAI)
	req, err := http.NewRequest(http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if relaycommon.IsDuomiImageAsyncUpstream(baseURL) {
		req.Header.Set("Authorization", key)
	} else {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	return req, nil
}

func convertImageTaskResponse(task *model.Task, body []byte) ([]byte, error) {
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
	return common.Marshal(response)
}

func firstNonZeroInt64(value int64, fallback int64) int64 {
	if value != 0 {
		return value
	}
	return fallback
}

package suno

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
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
)

type TaskAdaptor struct {
	taskcommon.BaseBilling
	ChannelType int
}

// ParseTaskResult is not used for Suno tasks.
// Suno polling uses a dedicated batch-fetch path (service.UpdateSunoTasks) that
// receives dto.TaskResponse[[]dto.SunoDataResponse] from the upstream /fetch API.
// This differs from the per-task polling used by video adaptors.
func (a *TaskAdaptor) ParseTaskResult([]byte) (*relaycommon.TaskInfo, error) {
	return nil, fmt.Errorf("suno uses batch polling via UpdateSunoTasks, ParseTaskResult is not applicable")
}

func (a *TaskAdaptor) Init(info *relaycommon.RelayInfo) {
	a.ChannelType = info.ChannelType
}

func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) (taskErr *dto.TaskError) {
	action := strings.ToUpper(c.Param("action"))

	var sunoRequest *dto.SunoSubmitReq
	err := common.UnmarshalBodyReusable(c, &sunoRequest)
	if err != nil {
		taskErr = service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
		return
	}
	err = actionValidate(c, sunoRequest, action, isDuomiUpstream(info.ChannelBaseUrl))
	if err != nil {
		taskErr = service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
		return
	}

	//if sunoRequest.ContinueClipId != "" {
	//	if sunoRequest.TaskID == "" {
	//		taskErr = service.TaskErrorWrapperLocal(fmt.Errorf("task id is empty"), "invalid_request", http.StatusBadRequest)
	//		return
	//	}
	//	info.OriginTaskID = sunoRequest.TaskID
	//}

	info.Action = action
	c.Set("task_request", sunoRequest)
	return nil
}

func (a *TaskAdaptor) BuildRequestURL(info *relaycommon.RelayInfo) (string, error) {
	baseURL := info.ChannelBaseUrl
	if isDuomiUpstream(baseURL) {
		return buildDuomiSubmitURL(baseURL, info.Action), nil
	}
	fullRequestURL := fmt.Sprintf("%s%s", strings.TrimRight(baseURL, "/"), "/suno/submit/"+info.Action)
	return fullRequestURL, nil
}

func (a *TaskAdaptor) BuildRequestHeader(c *gin.Context, req *http.Request, info *relaycommon.RelayInfo) error {
	contentType := c.Request.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/json"
	}
	req.Header.Set("Accept", c.Request.Header.Get("Accept"))
	if isDuomiUpstream(info.ChannelBaseUrl) {
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", info.ApiKey)
		return nil
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Authorization", "Bearer "+info.ApiKey)
	return nil
}

func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	sunoRequest, ok := c.Get("task_request")
	if !ok {
		return nil, fmt.Errorf("task_request not found in context")
	}
	if isDuomiUpstream(info.ChannelBaseUrl) {
		req, ok := sunoRequest.(*dto.SunoSubmitReq)
		if !ok {
			return nil, fmt.Errorf("invalid task_request type")
		}
		data, err := common.Marshal(buildDuomiSubmitRequest(req))
		if err != nil {
			return nil, err
		}
		return bytes.NewReader(data), nil
	}
	data, err := common.Marshal(sunoRequest)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(data), nil
}

func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	return channel.DoTaskApiRequest(a, c, info, requestBody)
}

func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (taskID string, taskData []byte, taskErr *dto.TaskError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		taskErr = service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
		return
	}
	if isDuomiUpstream(info.ChannelBaseUrl) {
		var duomiResponse dto.DuomiSunoSubmitResponse
		err = common.Unmarshal(responseBody, &duomiResponse)
		if err != nil {
			taskErr = service.TaskErrorWrapper(err, "unmarshal_response_body_failed", http.StatusInternalServerError)
			return
		}
		if duomiResponse.Code < 200 || duomiResponse.Code >= 300 {
			taskErr = service.TaskErrorWrapper(fmt.Errorf("%s", duomiResponse.Msg), fmt.Sprintf("%d", duomiResponse.Code), http.StatusInternalServerError)
			return
		}
		taskIDs := extractDuomiSubmitTaskIDs(duomiResponse.Data)
		if len(taskIDs) == 0 {
			taskErr = service.TaskErrorWrapper(fmt.Errorf("duomi response missing task_id"), "empty_task_id", http.StatusInternalServerError)
			return
		}
		publicResponse := dto.TaskResponse[string]{
			Code:    dto.TaskSuccessCode,
			Message: duomiResponse.Msg,
			Data:    info.PublicTaskID,
		}
		c.JSON(http.StatusOK, publicResponse)
		return strings.Join(taskIDs, ","), nil, nil
	}
	var sunoResponse dto.TaskResponse[string]
	err = common.Unmarshal(responseBody, &sunoResponse)
	if err != nil {
		taskErr = service.TaskErrorWrapper(err, "unmarshal_response_body_failed", http.StatusInternalServerError)
		return
	}
	if !sunoResponse.IsSuccess() {
		taskErr = service.TaskErrorWrapper(fmt.Errorf("%s", sunoResponse.Message), sunoResponse.Code, http.StatusInternalServerError)
		return
	}

	// 使用公开 task_xxxx ID 替换上游 ID 返回给客户端
	publicResponse := dto.TaskResponse[string]{
		Code:    sunoResponse.Code,
		Message: sunoResponse.Message,
		Data:    info.PublicTaskID,
	}
	c.JSON(http.StatusOK, publicResponse)

	return sunoResponse.Data, nil, nil
}

func (a *TaskAdaptor) GetModelList() []string {
	return ModelList
}

func (a *TaskAdaptor) GetChannelName() string {
	return ChannelName
}

func (a *TaskAdaptor) FetchTask(baseUrl, key string, body map[string]any, proxy string) (*http.Response, error) {
	if isDuomiUpstream(baseUrl) {
		return fetchDuomiTasks(baseUrl, key, body, proxy)
	}
	requestUrl := fmt.Sprintf("%s/suno/fetch", baseUrl)
	byteBody, err := common.Marshal(body)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", requestUrl, bytes.NewBuffer(byteBody))
	if err != nil {
		common.SysLog(fmt.Sprintf("Get Task error: %v", err))
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)
	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		return nil, fmt.Errorf("new proxy http client failed: %w", err)
	}
	return client.Do(req)
}

func actionValidate(c *gin.Context, sunoRequest *dto.SunoSubmitReq, action string, duomi bool) (err error) {
	switch action {
	case constant.SunoActionMusic:
		if sunoRequest.Mv == "" {
			if duomi {
				sunoRequest.Mv = "chirp-v3-5"
				return nil
			}
			sunoRequest.Mv = "chirp-v3-0"
		}
	case constant.SunoActionLyrics:
		if sunoRequest.Prompt == "" {
			err = fmt.Errorf("prompt_empty")
			return
		}
	default:
		err = fmt.Errorf("invalid_action")
	}
	return
}

func isDuomiUpstream(baseURL string) bool {
	normalized := strings.ToLower(strings.TrimRight(strings.TrimSpace(baseURL), "/"))
	return strings.Contains(normalized, "api.wike.cc") ||
		strings.Contains(normalized, "duomiapi.com") ||
		strings.HasSuffix(normalized, "/api/suno/generate") ||
		strings.HasSuffix(normalized, "/api/suno/submit/lyrics")
}

func buildDuomiSubmitURL(baseURL, action string) string {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if strings.HasSuffix(base, "/api/suno/generate") || strings.HasSuffix(base, "/api/suno/submit/lyrics") {
		return base
	}
	if strings.EqualFold(action, constant.SunoActionLyrics) {
		return joinDuomiAPIPath(base, "/suno/submit/lyrics")
	}
	return joinDuomiAPIPath(base, "/suno/generate")
}

func buildDuomiFeedURL(baseURL, taskID string) string {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	feedURL := joinDuomiAPIPath(base, "/suno/feed")
	u, err := url.Parse(feedURL)
	if err != nil {
		return feedURL
	}
	q := u.Query()
	q.Set("task_id", taskID)
	u.RawQuery = q.Encode()
	return u.String()
}

func joinDuomiAPIPath(baseURL, apiPath string) string {
	if strings.HasSuffix(baseURL, "/api") {
		return baseURL + apiPath
	}
	if strings.HasSuffix(baseURL, "/api/suno") {
		return baseURL + strings.TrimPrefix(apiPath, "/suno")
	}
	return baseURL + "/api" + apiPath
}

func buildDuomiSubmitRequest(req *dto.SunoSubmitReq) map[string]any {
	payload := map[string]any{
		"custom_mode":       boolToDuomiInt(req.CustomMode != nil && bool(*req.CustomMode)),
		"make_instrumental": boolToDuomiInt(bool(req.MakeInstrumental)),
	}
	if req.CallbackURL != "" {
		payload["callback_url"] = req.CallbackURL
	}
	if req.Prompt != "" {
		payload["prompt"] = req.Prompt
	}
	if req.Mv != "" {
		payload["mv"] = req.Mv
	}
	if req.Title != "" {
		payload["title"] = req.Title
	}
	if req.Tags != "" {
		payload["tags"] = req.Tags
	}
	if req.ContinueAt != 0 {
		payload["continue_at"] = req.ContinueAt
	}
	if req.ContinueClipId != "" {
		payload["continue_clip_id"] = req.ContinueClipId
	}
	if req.NegativeTags != "" {
		payload["negative_tags"] = req.NegativeTags
	}
	if req.CoverClipID != "" {
		payload["cover_clip_id"] = req.CoverClipID
	}
	if req.GptDescriptionPrompt != "" {
		payload["gpt_description_prompt"] = req.GptDescriptionPrompt
	}
	if req.Metadata != nil {
		payload["metadata"] = req.Metadata
	}
	return payload
}

func boolToDuomiInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func fetchDuomiTasks(baseUrl, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskIDs := extractDuomiTaskIDs(body["ids"])
	items := make([]dto.SunoDataResponse, 0, len(taskIDs))
	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		return nil, fmt.Errorf("new proxy http client failed: %w", err)
	}
	for _, taskID := range taskIDs {
		req, err := http.NewRequest(http.MethodGet, buildDuomiFeedURL(baseUrl, taskID), nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", key)
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		responseBody, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if readErr != nil {
			return nil, readErr
		}
		if resp.StatusCode != http.StatusOK {
			return &http.Response{
				StatusCode: resp.StatusCode,
				Body:       io.NopCloser(bytes.NewReader(responseBody)),
			}, nil
		}
		parsedItems, err := parseDuomiFetchResponse(taskID, responseBody)
		if err != nil {
			return nil, err
		}
		items = append(items, parsedItems...)
	}
	responseBody, err := common.Marshal(dto.TaskResponse[[]dto.SunoDataResponse]{
		Code: dto.TaskSuccessCode,
		Data: items,
	})
	if err != nil {
		return nil, err
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewReader(responseBody)),
	}, nil
}

func extractDuomiTaskIDs(v any) []string {
	switch ids := v.(type) {
	case []string:
		return cleanDuomiTaskIDs(ids)
	case []any:
		result := make([]string, 0, len(ids))
		for _, id := range ids {
			if s, ok := id.(string); ok {
				result = append(result, cleanDuomiTaskIDs([]string{s})...)
			}
		}
		return result
	default:
		return nil
	}
}

func extractDuomiSubmitTaskIDs(data []byte) []string {
	if len(data) == 0 || string(bytes.TrimSpace(data)) == "null" {
		return nil
	}
	var stringIDs []string
	if err := common.Unmarshal(data, &stringIDs); err == nil {
		return cleanDuomiTaskIDs(stringIDs)
	}
	var stringID string
	if err := common.Unmarshal(data, &stringID); err == nil {
		return cleanDuomiTaskIDs([]string{stringID})
	}
	var objectIDs []duomiTaskIDObject
	if err := common.Unmarshal(data, &objectIDs); err == nil {
		return cleanDuomiTaskIDObjects(objectIDs)
	}
	var objectID duomiTaskIDObject
	if err := common.Unmarshal(data, &objectID); err == nil {
		return cleanDuomiTaskIDObjects([]duomiTaskIDObject{objectID})
	}
	return nil
}

type duomiTaskIDObject struct {
	TaskID string `json:"task_id"`
	ID     string `json:"id"`
}

func cleanDuomiTaskIDObjects(objects []duomiTaskIDObject) []string {
	result := make([]string, 0, len(objects))
	for _, object := range objects {
		if object.TaskID != "" {
			result = append(result, object.TaskID)
			continue
		}
		if object.ID != "" {
			result = append(result, object.ID)
		}
	}
	return cleanDuomiTaskIDs(result)
}

func cleanDuomiTaskIDs(ids []string) []string {
	result := make([]string, 0, len(ids))
	for _, id := range ids {
		for _, part := range strings.Split(id, ",") {
			part = strings.TrimSpace(part)
			if part != "" {
				result = append(result, part)
			}
		}
	}
	return result
}

func parseDuomiFetchResponse(taskID string, responseBody []byte) ([]dto.SunoDataResponse, error) {
	var response dto.DuomiSunoFeedResponse
	if err := common.Unmarshal(responseBody, &response); err != nil {
		return nil, err
	}
	if response.Code < 200 || response.Code >= 300 {
		return nil, fmt.Errorf("duomi fetch failed: %s", response.Msg)
	}
	data := response.Data
	if data.TaskID == "" {
		data.TaskID = taskID
	}
	if data.TaskID == "" {
		return nil, fmt.Errorf("duomi response missing task_id")
	}
	status := mapDuomiStatus(firstNonEmpty(string(data.State), string(data.Status)))
	item := dto.SunoDataResponse{
		TaskID:     data.TaskID,
		Action:     constant.SunoActionMusic,
		Status:     string(status),
		FailReason: duomiFailReason(data),
	}
	if status == model.TaskStatusSuccess || data.AudioURL != "" {
		item.Data = buildDuomiSunoSongData(data)
	} else {
		raw, err := common.Marshal(data)
		if err != nil {
			return nil, err
		}
		item.Data = raw
	}
	if status == model.TaskStatusSuccess || status == model.TaskStatusFailure {
		item.FinishTime = time.Now().Unix()
	}
	return []dto.SunoDataResponse{item}, nil
}

func mapDuomiStatus(state string) model.TaskStatus {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "0", "1", "pending", "queued", "queueing":
		return model.TaskStatusQueued
	case "2", "running", "processing":
		return model.TaskStatusInProgress
	case "3", "succeeded", "success":
		return model.TaskStatusSuccess
	case "-1", "4", "error", "failed", "failure":
		return model.TaskStatusFailure
	default:
		return model.TaskStatusUnknown
	}
}

func duomiFailReason(data dto.DuomiSunoFeedData) string {
	if mapDuomiStatus(firstNonEmpty(string(data.State), string(data.Status))) != model.TaskStatusFailure {
		return ""
	}
	if data.Msg != "" {
		return data.Msg
	}
	return "duomi task failed"
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func buildDuomiSunoSongData(data dto.DuomiSunoFeedData) []byte {
	songID := data.ClipID
	if songID == "" {
		songID = data.TaskID
	}
	song := dto.SunoSong{
		ID:                songID,
		VideoURL:          data.VideoURL,
		AudioURL:          data.AudioURL,
		ImageURL:          data.ImageURL,
		ImageLargeURL:     data.ImageLargeURL,
		MajorModelVersion: data.Mv,
		ModelName:         data.Mv,
		Status:            string(model.TaskStatusSuccess),
		Title:             data.Title,
		Text:              data.Prompt,
		Metadata: dto.SunoMetadata{
			Tags:                 data.Tags,
			Prompt:               data.Prompt,
			GPTDescriptionPrompt: data.GptDescriptionPrompt,
			Duration:             data.Duration,
		},
	}
	raw, _ := common.Marshal([]dto.SunoSong{song})
	return raw
}

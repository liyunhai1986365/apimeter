package middleware

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

type requestLogResponseWriter struct {
	gin.ResponseWriter
	body      []byte
	limit     int
	enabled   bool
	truncated bool
}

func (w *requestLogResponseWriter) Write(data []byte) (int, error) {
	w.capture(data)
	return w.ResponseWriter.Write(data)
}

func (w *requestLogResponseWriter) WriteString(s string) (int, error) {
	w.capture([]byte(s))
	return w.ResponseWriter.WriteString(s)
}

func (w *requestLogResponseWriter) Flush() {
	w.ResponseWriter.Flush()
}

func (w *requestLogResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("response writer does not support hijacking")
	}
	return hijacker.Hijack()
}

func (w *requestLogResponseWriter) CloseNotify() <-chan bool {
	return w.ResponseWriter.CloseNotify()
}

func (w *requestLogResponseWriter) Pusher() http.Pusher {
	return w.ResponseWriter.Pusher()
}

func (w *requestLogResponseWriter) capturedBody() string {
	if !w.enabled {
		return ""
	}
	if w.truncated {
		return string(w.body) + "...[CAPTURED_TRUNCATED]"
	}
	return string(w.body)
}

func (w *requestLogResponseWriter) capture(data []byte) {
	if !w.enabled || w.limit <= 0 || len(data) == 0 {
		return
	}
	remain := w.limit - len(w.body)
	if remain <= 0 {
		w.truncated = true
		return
	}
	if len(data) > remain {
		w.body = append(w.body, data[:remain]...)
		w.truncated = true
		return
	}
	w.body = append(w.body, data...)
}

func RequestLog() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !shouldCaptureRequestLog(c) || !model.CanAcceptRequestLog() {
			c.Next()
			return
		}

		writer := &requestLogResponseWriter{
			ResponseWriter: c.Writer,
			limit:          common.RequestLogMaxResponseBytes,
			enabled:        common.RequestLogCaptureResponseBodyEnabled,
		}
		if writer.enabled && writer.limit > 0 {
			writer.body = make([]byte, 0, min(writer.limit, 32*1024))
		}
		c.Writer = writer
		startTime := time.Now()

		c.Next()

		record := buildRequestLogRecord(c, writer, startTime)
		if shouldSkipRequestLogRecord(c, record) {
			return
		}
		model.EnqueueRequestLog(record)
	}
}

func shouldCaptureRequestLog(c *gin.Context) bool {
	if c == nil || c.Request == nil || c.Request.URL == nil {
		return false
	}
	return isRelayRequestLogEndpoint(c.Request.Method, c.Request.URL.Path)
}

func isRelayRequestLogEndpoint(method string, path string) bool {
	switch {
	case method == http.MethodGet && (path == "/v1/models" || strings.HasPrefix(path, "/v1/models/")):
		return true
	case method == http.MethodGet && (path == "/v1beta/models" || path == "/v1beta/openai/models"):
		return true
	case method == http.MethodPost && path == "/pg/chat/completions":
		return true
	case strings.HasPrefix(path, "/v1/"):
		return isRelayV1RequestLogEndpoint(method, strings.TrimPrefix(path, "/v1"))
	case strings.HasPrefix(path, "/v1beta/"):
		return method == http.MethodPost && strings.HasPrefix(path, "/v1beta/models/")
	case strings.HasPrefix(path, "/mj/"):
		return isRelayMidjourneyRequestLogEndpoint(method, strings.TrimPrefix(path, "/mj"))
	case isRelayModeMidjourneyPath(path):
		return isRelayMidjourneyRequestLogEndpoint(method, path[strings.Index(path, "/mj/")+len("/mj"):])
	case strings.HasPrefix(path, "/suno/"):
		return isRelaySunoRequestLogEndpoint(method, strings.TrimPrefix(path, "/suno"))
	case strings.HasPrefix(path, "/kling/v2/"):
		return isRelayKlingRequestLogEndpoint(method, strings.TrimPrefix(path, "/kling/v2"))
	case path == "/jimeng" || path == "/jimeng/":
		return method == http.MethodPost
	default:
		return false
	}
}

func isRelayV1RequestLogEndpoint(method string, suffix string) bool {
	if method == http.MethodGet {
		return suffix == "/realtime" ||
			strings.HasPrefix(suffix, "/tasks/") ||
			strings.HasPrefix(suffix, "/video/generations/") ||
			strings.HasPrefix(suffix, "/videos/") && strings.HasSuffix(suffix, "/content")
	}
	if method != http.MethodPost {
		return false
	}
	switch suffix {
	case "/messages",
		"/completions",
		"/chat/completions",
		"/responses",
		"/responses/compact",
		"/edits",
		"/images/generations",
		"/images/edits",
		"/embeddings",
		"/audio/transcriptions",
		"/audio/translations",
		"/audio/speech",
		"/rerank",
		"/moderations",
		"/video/generations",
		"/videos":
		return true
	default:
		return strings.HasPrefix(suffix, "/engines/") && strings.HasSuffix(suffix, "/embeddings") ||
			strings.HasPrefix(suffix, "/models/") ||
			strings.HasPrefix(suffix, "/videos/") && strings.HasSuffix(suffix, "/remix")
	}
}

func isRelayModeMidjourneyPath(path string) bool {
	if !strings.HasPrefix(path, "/") {
		return false
	}
	idx := strings.Index(path[1:], "/")
	if idx < 0 {
		return false
	}
	return strings.HasPrefix(path[idx+1:], "/mj/")
}

func isRelayMidjourneyRequestLogEndpoint(method string, suffix string) bool {
	if method == http.MethodGet {
		return strings.HasPrefix(suffix, "/task/") ||
			strings.HasPrefix(suffix, "/image/")
	}
	if method != http.MethodPost {
		return false
	}
	switch suffix {
	case "/submit/action",
		"/submit/shorten",
		"/submit/modal",
		"/submit/imagine",
		"/submit/change",
		"/submit/simple-change",
		"/submit/describe",
		"/submit/blend",
		"/submit/edits",
		"/submit/video",
		"/task/list-by-condition",
		"/insight-face/swap",
		"/submit/upload-discord-images":
		return true
	default:
		return false
	}
}

func isRelaySunoRequestLogEndpoint(method string, suffix string) bool {
	if method == http.MethodPost {
		return strings.HasPrefix(suffix, "/submit/") || suffix == "/fetch"
	}
	return method == http.MethodGet && strings.HasPrefix(suffix, "/fetch/")
}

func isRelayKlingRequestLogEndpoint(method string, suffix string) bool {
	if method == http.MethodPost {
		return suffix == "/videos/text2video" || suffix == "/videos/image2video"
	}
	return method == http.MethodGet &&
		(strings.HasPrefix(suffix, "/videos/text2video/") || strings.HasPrefix(suffix, "/videos/image2video/"))
}

func shouldSkipRequestLogRecord(c *gin.Context, record model.RequestLogRecord) bool {
	if record.Status != "success" {
		return false
	}
	if !isTaskFetchRequestLogPath(c.Request.Method, c.Request.URL.Path) {
		return false
	}
	return !taskFetchResponseHasTerminalStatus(record.ResponseBody)
}

func isTaskFetchRequestLogPath(method string, path string) bool {
	if method == http.MethodGet {
		return strings.HasPrefix(path, "/v1/tasks/") ||
			strings.HasPrefix(path, "/v1/video/generations/") ||
			strings.HasPrefix(path, "/v1/videos/") ||
			strings.HasPrefix(path, "/kling/v2/videos/text2video/") ||
			strings.HasPrefix(path, "/kling/v2/videos/image2video/") ||
			strings.HasPrefix(path, "/suno/fetch/") ||
			strings.HasPrefix(path, "/mj/task/") ||
			strings.Contains(path, "/mj/task/")
	}
	if method == http.MethodPost {
		return path == "/suno/fetch" ||
			path == "/mj/task/list-by-condition" ||
			strings.HasSuffix(path, "/mj/task/list-by-condition")
	}
	return false
}

func taskFetchResponseHasTerminalStatus(responseBody string) bool {
	if strings.TrimSpace(responseBody) == "" {
		return true
	}
	var payload any
	if err := common.Unmarshal([]byte(responseBody), &payload); err != nil {
		return true
	}
	statuses := collectTaskResponseStatuses(payload)
	if len(statuses) == 0 {
		return true
	}
	for _, status := range statuses {
		if !isTerminalTaskStatus(status) {
			return false
		}
	}
	return true
}

func collectTaskResponseStatuses(value any) []string {
	statuses := make([]string, 0, 2)
	collectTaskResponseStatusesInto(value, &statuses)
	return statuses
}

func collectTaskResponseStatusesInto(value any, statuses *[]string) {
	switch typed := value.(type) {
	case map[string]any:
		for _, key := range []string{"status", "task_status"} {
			if raw, ok := typed[key]; ok {
				if status, ok := raw.(string); ok && strings.TrimSpace(status) != "" {
					*statuses = append(*statuses, status)
				}
			}
		}
		if data, ok := typed["data"]; ok {
			collectTaskResponseStatusesInto(data, statuses)
		}
		if output, ok := typed["output"]; ok {
			collectTaskResponseStatusesInto(output, statuses)
		}
	case []any:
		for _, item := range typed {
			collectTaskResponseStatusesInto(item, statuses)
		}
	}
}

func isTerminalTaskStatus(status string) bool {
	normalized := strings.ToLower(strings.TrimSpace(status))
	normalized = strings.ReplaceAll(normalized, "-", "_")
	normalized = strings.ReplaceAll(normalized, " ", "_")
	switch normalized {
	case "success", "succeeded", "completed", "complete", "done",
		"failure", "failed", "fail", "error", "cancelled", "canceled", "expired":
		return true
	default:
		return false
	}
}

func buildRequestLogRecord(c *gin.Context, writer *requestLogResponseWriter, startTime time.Time) model.RequestLogRecord {
	requestBody, requestTruncated := readRequestLogRequestBody(c)
	requestHeaders := marshalRequestLogMap(flattenHeaders(c.Request.Header))
	statusCode := writer.Status()
	status := "success"
	if statusCode >= http.StatusBadRequest || len(c.Errors) > 0 {
		status = "error"
	}

	extra := map[string]any{
		"duration_ms": time.Since(startTime).Milliseconds(),
	}
	if useChannels := c.GetStringSlice("use_channel"); len(useChannels) > 0 {
		extra["use_channel"] = useChannels
	}
	if relayFormat := string(getRelayFormat(c)); relayFormat != "" {
		extra["relay_format"] = relayFormat
	}
	if writer.truncated {
		extra["response_capture_truncated"] = true
	}
	if requestTruncated {
		extra["request_capture_truncated"] = true
	}
	responseBody, responseEncoding := writer.capturedBody(), strings.ToLower(c.Writer.Header().Get("Content-Encoding"))
	responseBody, responseDecoded := decodeCapturedResponseBody(responseBody, responseEncoding)
	if responseDecoded {
		extra["response_capture_decoded"] = responseEncoding
	}

	errorMessage := ""
	if len(c.Errors) > 0 {
		errorMessage = c.Errors.String()
	}

	return model.RequestLogRecord{
		CreatedAt:         startTime.Unix(),
		UserId:            common.GetContextKeyInt(c, constant.ContextKeyUserId),
		Username:          common.GetContextKeyString(c, constant.ContextKeyUserName),
		UserGroup:         common.GetContextKeyString(c, constant.ContextKeyUserGroup),
		UserEmail:         common.GetContextKeyString(c, constant.ContextKeyUserEmail),
		TokenId:           common.GetContextKeyInt(c, constant.ContextKeyTokenId),
		TokenName:         c.GetString("token_name"),
		ChannelId:         common.GetContextKeyInt(c, constant.ContextKeyChannelId),
		ChannelName:       common.GetContextKeyString(c, constant.ContextKeyChannelName),
		ChannelType:       common.GetContextKeyInt(c, constant.ContextKeyChannelType),
		ChannelBaseUrl:    common.GetContextKeyString(c, constant.ContextKeyChannelBaseUrl),
		IsMultiKey:        common.GetContextKeyBool(c, constant.ContextKeyChannelIsMultiKey),
		MultiKeyIndex:     common.GetContextKeyInt(c, constant.ContextKeyChannelMultiKeyIndex),
		RequestIP:         c.ClientIP(),
		RequestMethod:     c.Request.Method,
		RequestPath:       c.Request.URL.Path,
		RequestURL:        c.Request.URL.String(),
		RequestHeaders:    requestHeaders,
		RequestId:         c.GetString(common.RequestIdKey),
		UpstreamRequestId: c.GetString(common.UpstreamRequestIdKey),
		RelayMode:         c.GetInt("relay_mode"),
		RelayFormat:       string(getRelayFormat(c)),
		ModelName:         common.GetContextKeyString(c, constant.ContextKeyOriginalModel),
		UpstreamModelName: upstreamModelName(c),
		IsStream:          common.GetContextKeyBool(c, constant.ContextKeyIsStream),
		Status:            status,
		StatusCode:        statusCode,
		ErrorMessage:      errorMessage,
		RequestBody:       requestBody,
		ResponseBody:      responseBody,
		Extra:             marshalRequestLogMap(extra),
	}
}

func decodeCapturedResponseBody(body string, encoding string) (string, bool) {
	if body == "" {
		return body, false
	}
	if strings.TrimSpace(strings.ToLower(encoding)) != "gzip" {
		return body, false
	}
	reader, err := gzip.NewReader(bytes.NewReader([]byte(body)))
	if err != nil {
		return body, false
	}
	defer reader.Close()

	limit := common.RequestLogMaxResponseBytes
	if limit <= 0 {
		limit = 512 * 1024
	}
	buf := bytes.NewBuffer(make([]byte, 0, min(limit, 32*1024)))
	n, err := io.CopyN(buf, reader, int64(limit)+1)
	if err != nil && !errors.Is(err, io.EOF) {
		return body, false
	}
	data := buf.Bytes()
	if n > int64(limit) && len(data) > limit {
		data = data[:limit]
		return string(data) + "...[CAPTURED_TRUNCATED]", true
	}
	return string(data), true
}

func upstreamModelName(c *gin.Context) string {
	if value, exists := c.Get("upstream_model_name"); exists {
		if modelName, ok := value.(string); ok && modelName != "" {
			return modelName
		}
	}
	return common.GetContextKeyString(c, constant.ContextKeyOriginalModel)
}

func readRequestLogRequestBody(c *gin.Context) (string, bool) {
	cachedStorage, exists := c.Get(common.KeyBodyStorage)
	if !exists || cachedStorage == nil {
		return "", false
	}
	storage, ok := cachedStorage.(common.BodyStorage)
	if !ok {
		return "", false
	}
	if _, err := storage.Seek(0, io.SeekStart); err != nil {
		return "", false
	}
	limit := common.RequestLogMaxRequestBytes
	if limit <= 0 {
		limit = 256 * 1024
	}
	buf := bytes.NewBuffer(make([]byte, 0, min(limit, 32*1024)))
	n, err := io.CopyN(buf, storage, int64(limit)+1)
	truncated := n > int64(limit)
	if err != nil {
		if !errors.Is(err, io.EOF) {
			return "", false
		}
	}
	data := buf.Bytes()
	if truncated && len(data) > limit {
		data = data[:limit]
	}
	if _, err = storage.Seek(0, io.SeekStart); err == nil {
		c.Request.Body = io.NopCloser(storage)
	}
	if truncated {
		return string(data) + "...[CAPTURED_TRUNCATED]", true
	}
	return string(data), false
}

func flattenHeaders(headers http.Header) map[string]any {
	result := make(map[string]any, len(headers))
	for key, values := range headers {
		if len(values) == 1 {
			result[key] = values[0]
		} else if len(values) > 1 {
			result[key] = strings.Join(values, ",")
		}
	}
	return result
}

func marshalRequestLogMap(data map[string]any) string {
	if len(data) == 0 {
		return ""
	}
	b, err := common.Marshal(data)
	if err != nil {
		return ""
	}
	return string(b)
}

func getRelayFormat(c *gin.Context) types.RelayFormat {
	switch {
	case strings.HasPrefix(c.Request.URL.Path, "/v1/messages"):
		return types.RelayFormatClaude
	case strings.HasPrefix(c.Request.URL.Path, "/v1beta/models"):
		return types.RelayFormatGemini
	case strings.HasPrefix(c.Request.URL.Path, "/v1/responses"):
		return types.RelayFormatOpenAIResponses
	default:
		return types.RelayFormatOpenAI
	}
}

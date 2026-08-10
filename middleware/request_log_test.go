package middleware

import (
	"bytes"
	"compress/gzip"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type captureRequestLogStore struct{}

func (captureRequestLogStore) WriteBatch(records []model.RequestLogRecord) error {
	return nil
}

func TestBuildRequestLogRecordCapturesRelayContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	originalRequestMax := common.RequestLogMaxRequestBytes
	originalCaptureResponse := common.RequestLogCaptureResponseBodyEnabled
	t.Cleanup(func() {
		common.RequestLogMaxRequestBytes = originalRequestMax
		common.RequestLogCaptureResponseBodyEnabled = originalCaptureResponse
	})
	common.RequestLogMaxRequestBytes = 256
	common.RequestLogCaptureResponseBodyEnabled = true

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-test","api_key":"sk-secret"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Forwarded-For", "203.0.113.10")
	resp := httptest.NewRecorder()

	c, _ := gin.CreateTestContext(resp)
	c.Request = req
	storage, err := common.CreateBodyStorage([]byte(`{"model":"gpt-test","api_key":"sk-secret"}`))
	require.NoError(t, err)
	defer storage.Close()
	c.Set(common.KeyBodyStorage, storage)
	writer := &requestLogResponseWriter{ResponseWriter: c.Writer, limit: 128, enabled: true}
	c.Writer = writer

	common.SetContextKey(c, constant.ContextKeyUserId, 7)
	common.SetContextKey(c, constant.ContextKeyUserName, "alice")
	common.SetContextKey(c, constant.ContextKeyUserGroup, "vip")
	common.SetContextKey(c, constant.ContextKeyUserEmail, "alice@example.com")
	common.SetContextKey(c, constant.ContextKeyTokenId, 9)
	c.Set("token_name", "prod-token")
	common.SetContextKey(c, constant.ContextKeyChannelId, 11)
	common.SetContextKey(c, constant.ContextKeyChannelName, "openai-main")
	common.SetContextKey(c, constant.ContextKeyChannelType, 1)
	common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, "https://upstream.example.com")
	common.SetContextKey(c, constant.ContextKeyOriginalModel, "gpt-test")
	common.SetContextKey(c, constant.ContextKeyIsStream, true)
	c.Set(common.RequestIdKey, "req_abc")
	c.JSON(http.StatusOK, gin.H{"output": "hello"})

	record := buildRequestLogRecord(c, writer, time.Now())
	require.Equal(t, 7, record.UserId)
	require.Equal(t, "alice", record.Username)
	require.Equal(t, "203.0.113.10", record.RequestIP)
	require.Equal(t, 11, record.ChannelId)
	require.Equal(t, "openai-main", record.ChannelName)
	require.Equal(t, "gpt-test", record.ModelName)
	require.True(t, record.IsStream)
	require.Contains(t, record.RequestBody, "sk-secret")
	require.Contains(t, record.ResponseBody, "hello")
}

func TestBuildRequestLogRecordReadsRequestBodyOnlyUpToConfiguredLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	originalRequestMax := common.RequestLogMaxRequestBytes
	originalCaptureResponse := common.RequestLogCaptureResponseBodyEnabled
	t.Cleanup(func() {
		common.RequestLogMaxRequestBytes = originalRequestMax
		common.RequestLogCaptureResponseBodyEnabled = originalCaptureResponse
	})
	common.RequestLogMaxRequestBytes = 16
	common.RequestLogCaptureResponseBodyEnabled = true

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(strings.Repeat("x", 1024)))
	resp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(resp)
	c.Request = req
	storage, err := common.CreateBodyStorage([]byte(strings.Repeat("x", 1024)))
	require.NoError(t, err)
	defer storage.Close()
	c.Set(common.KeyBodyStorage, storage)
	writer := &requestLogResponseWriter{ResponseWriter: c.Writer, limit: 128, enabled: true}
	c.Writer = writer

	record := buildRequestLogRecord(c, writer, time.Now())

	require.LessOrEqual(t, len(record.RequestBody), common.RequestLogMaxRequestBytes+len("...[CAPTURED_TRUNCATED]"))
	require.Contains(t, record.Extra, "request_capture_truncated")
}

func TestBuildRequestLogRecordDoesNotReadUncachedRequestBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(strings.Repeat("x", 1024)))
	resp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(resp)
	c.Request = req
	writer := &requestLogResponseWriter{ResponseWriter: c.Writer, limit: 128, enabled: true}
	c.Writer = writer

	record := buildRequestLogRecord(c, writer, time.Now())

	require.Empty(t, record.RequestBody)
}

func TestRequestLogResponseCaptureCanBeDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	originalCaptureResponse := common.RequestLogCaptureResponseBodyEnabled
	t.Cleanup(func() {
		common.RequestLogCaptureResponseBodyEnabled = originalCaptureResponse
	})
	common.RequestLogCaptureResponseBodyEnabled = false

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-test"}`))
	resp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(resp)
	c.Request = req
	writer := &requestLogResponseWriter{ResponseWriter: c.Writer, limit: 128, enabled: false}
	c.Writer = writer
	c.JSON(http.StatusOK, gin.H{"output": strings.Repeat("hello", 100)})

	record := buildRequestLogRecord(c, writer, time.Now())

	require.Empty(t, record.ResponseBody)
	require.NotContains(t, record.Extra, "response_capture_truncated")
}

func TestBuildRequestLogRecordDecodesGzipResponseCapture(t *testing.T) {
	gin.SetMode(gin.TestMode)
	originalResponseMax := common.RequestLogMaxResponseBytes
	originalCaptureResponse := common.RequestLogCaptureResponseBodyEnabled
	t.Cleanup(func() {
		common.RequestLogMaxResponseBytes = originalResponseMax
		common.RequestLogCaptureResponseBodyEnabled = originalCaptureResponse
	})
	common.RequestLogMaxResponseBytes = 1024
	common.RequestLogCaptureResponseBodyEnabled = true

	var compressed bytes.Buffer
	gzipWriter := gzip.NewWriter(&compressed)
	_, err := gzipWriter.Write([]byte(`{"output":"hello"}`))
	require.NoError(t, err)
	require.NoError(t, gzipWriter.Close())

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-test"}`))
	resp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(resp)
	c.Request = req
	writer := &requestLogResponseWriter{ResponseWriter: c.Writer, limit: 1024, enabled: true}
	c.Writer = writer
	c.Header("Content-Encoding", "gzip")
	_, err = c.Writer.Write(compressed.Bytes())
	require.NoError(t, err)

	record := buildRequestLogRecord(c, writer, time.Now())

	require.Equal(t, `{"output":"hello"}`, record.ResponseBody)
	require.Contains(t, record.Extra, "response_capture_decoded")
}

func TestBuildRequestLogRecordPreservesLatinTextResponseStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)
	originalLogDB := model.LOG_DB
	originalEnabled := common.RequestLogEnabled
	t.Cleanup(func() {
		model.LOG_DB = originalLogDB
		common.RequestLogEnabled = originalEnabled
		model.SetRequestLogStoreForTest(nil)
	})
	common.RequestLogEnabled = true
	model.SetRequestLogStoreForTest(captureRequestLogStore{})
	model.LOG_DB = nil

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-test"}`))
	resp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(resp)
	c.Request = req
	writer := &requestLogResponseWriter{ResponseWriter: c.Writer, limit: 256, enabled: true}
	c.Writer = writer
	c.String(http.StatusOK, `{"output":"ä½ å¥½ï¼Œè¿æ¯ä¹±ç "}`)

	record := buildRequestLogRecord(c, writer, time.Now())
	record, err := model.RecordRequestLog(record)
	require.NoError(t, err)

	require.Equal(t, "success", record.Status)
	require.Empty(t, record.ErrorCode)
	require.Empty(t, record.ErrorMessage)
}

func TestRequestLogEndpointFilterOnlyAllowsRelayRequests(t *testing.T) {
	allowed := []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/v1/chat/completions"},
		{http.MethodPost, "/v1/responses"},
		{http.MethodGet, "/v1/models"},
		{http.MethodGet, "/v1/models/gpt-test"},
		{http.MethodPost, "/v1beta/models/gemini-pro:generateContent"},
		{http.MethodPost, "/mj/submit/imagine"},
		{http.MethodPost, "/relax/mj/submit/imagine"},
		{http.MethodPost, "/suno/submit/music"},
		{http.MethodPost, "/kling/v2/videos/text2video"},
		{http.MethodPost, "/jimeng/"},
	}
	for _, tc := range allowed {
		require.True(t, isRelayRequestLogEndpoint(tc.method, tc.path), "%s %s should be captured", tc.method, tc.path)
	}

	blocked := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/robots.txt"},
		{http.MethodGet, "/static/js/index.aa946bcdb0.js"},
		{http.MethodGet, "/wp-admin/install.php"},
		{http.MethodGet, "/"},
		{http.MethodGet, "/dashboard"},
		{http.MethodGet, "/api/user/self"},
		{http.MethodGet, "/not-mj/mj-ish"},
		{http.MethodPost, "/v1/not-a-relay"},
		{http.MethodGet, "/v1/chat/completions"},
	}
	for _, tc := range blocked {
		require.False(t, isRelayRequestLogEndpoint(tc.method, tc.path), "%s %s should not be captured", tc.method, tc.path)
	}
}

func TestTaskFetchRequestLogSkipsOnlyInProgressSuccessResponses(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cases := []struct {
		name       string
		method     string
		path       string
		statusCode int
		body       string
		wantSkip   bool
	}{
		{
			name:       "single queued task",
			method:     http.MethodGet,
			path:       "/v1/videos/task_123",
			statusCode: http.StatusOK,
			body:       `{"id":"task_123","status":"queued"}`,
			wantSkip:   true,
		},
		{
			name:       "task response data in progress",
			method:     http.MethodPost,
			path:       "/suno/fetch",
			statusCode: http.StatusOK,
			body:       `{"code":"success","data":[{"task_id":"a","status":"IN_PROGRESS"}]}`,
			wantSkip:   true,
		},
		{
			name:       "success task",
			method:     http.MethodGet,
			path:       "/v1/videos/task_123",
			statusCode: http.StatusOK,
			body:       `{"id":"task_123","status":"completed"}`,
			wantSkip:   false,
		},
		{
			name:       "failure task",
			method:     http.MethodGet,
			path:       "/kling/v2/videos/text2video/task_123",
			statusCode: http.StatusOK,
			body:       `{"task_id":"task_123","task_status":"failed"}`,
			wantSkip:   false,
		},
		{
			name:       "http error always logs",
			method:     http.MethodGet,
			path:       "/v1/videos/task_123",
			statusCode: http.StatusBadRequest,
			body:       `{"error":"task_not_exist"}`,
			wantSkip:   false,
		},
		{
			name:       "non task endpoint always logs",
			method:     http.MethodPost,
			path:       "/v1/responses",
			statusCode: http.StatusOK,
			body:       `{"status":"queued"}`,
			wantSkip:   false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			resp := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(resp)
			c.Request = req
			writer := &requestLogResponseWriter{ResponseWriter: c.Writer, limit: 1024, enabled: true}
			c.Writer = writer
			c.String(tc.statusCode, tc.body)

			record := buildRequestLogRecord(c, writer, time.Now())

			require.Equal(t, tc.wantSkip, shouldSkipRequestLogRecord(c, record))
		})
	}
}

func TestRequestLogMiddlewareSkipsCaptureWhenQueueIsFull(t *testing.T) {
	gin.SetMode(gin.TestMode)
	originalEnabled := common.RequestLogEnabled
	originalQueueSize := common.RequestLogQueueSize
	t.Cleanup(func() {
		common.RequestLogEnabled = originalEnabled
		common.RequestLogQueueSize = originalQueueSize
		model.SetRequestLogStoreForTest(nil)
	})

	common.RequestLogEnabled = true
	common.RequestLogQueueSize = 1
	model.SetRequestLogStoreForTest(captureRequestLogStore{})
	model.ResetRequestLogQueueForTest(common.RequestLogQueueSize)
	require.True(t, model.EnqueueRequestLog(model.RequestLogRecord{RequestId: "already-full"}))

	router := gin.New()
	router.Use(RequestLog())
	router.POST("/v1/chat/completions", func(c *gin.Context) {
		_, wrapped := c.Writer.(*requestLogResponseWriter)
		require.False(t, wrapped)
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(strings.Repeat("x", 1024)))
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	require.Equal(t, http.StatusOK, resp.Code)
	require.False(t, model.EnqueueRequestLog(model.RequestLogRecord{RequestId: "drop-me"}))
}

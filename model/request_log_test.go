package model

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

type captureRequestLogStore struct {
	records []RequestLogRecord
}

func (s *captureRequestLogStore) WriteBatch(records []RequestLogRecord) error {
	s.records = append(s.records, records...)
	return nil
}

func TestInitRequestLogStoreDisabledWithoutOpenObserveConfig(t *testing.T) {
	originalEnabled := common.RequestLogEnabled
	originalQueue := requestLogQueue
	originalStore := requestLogStore
	t.Cleanup(func() {
		common.RequestLogEnabled = originalEnabled
		requestLogQueue = originalQueue
		requestLogStore = originalStore
	})

	common.RequestLogEnabled = true
	requestLogQueue = make(chan RequestLogRecord, 1)
	t.Setenv("REQUEST_LOG_STORAGE", "")
	t.Setenv("REQUEST_LOG_OPENOBSERVE_ENDPOINT", "")

	require.Error(t, InitRequestLogStore())
	require.False(t, common.RequestLogEnabled)
	require.Nil(t, requestLogQueue)
	require.Nil(t, requestLogStore)
}

func TestInitRequestLogStoreIgnoresLegacySQLDSNWithoutOpenObserve(t *testing.T) {
	originalEnabled := common.RequestLogEnabled
	originalQueue := requestLogQueue
	originalStore := requestLogStore
	t.Cleanup(func() {
		common.RequestLogEnabled = originalEnabled
		requestLogQueue = originalQueue
		requestLogStore = originalStore
	})

	common.RequestLogEnabled = true
	requestLogQueue = make(chan RequestLogRecord, 1)
	t.Setenv("REQUEST_LOG_LEGACY_SQL_DSN", "file:"+t.TempDir()+"/request-log.db?cache=shared")
	t.Setenv("REQUEST_LOG_OPENOBSERVE_ENDPOINT", "")
	t.Setenv("REQUEST_LOG_OPENOBSERVE_USER", "")
	t.Setenv("REQUEST_LOG_OPENOBSERVE_PASSWORD", "")
	t.Setenv("REQUEST_LOG_OPENOBSERVE_TOKEN", "")

	require.Error(t, InitRequestLogStore())
	require.False(t, common.RequestLogEnabled)
	require.Nil(t, requestLogQueue)
	require.Nil(t, requestLogStore)
}

func TestInitRequestLogStoreEnablesOpenObserveWithoutSQLDSN(t *testing.T) {
	originalEnabled := common.RequestLogEnabled
	originalQueue := requestLogQueue
	originalQueueSize := common.RequestLogQueueSize
	originalStore := requestLogStore
	t.Cleanup(func() {
		common.RequestLogEnabled = originalEnabled
		requestLogQueue = originalQueue
		common.RequestLogQueueSize = originalQueueSize
		requestLogStore = originalStore
	})

	common.RequestLogEnabled = true
	common.RequestLogQueueSize = 2
	t.Setenv("REQUEST_LOG_OPENOBSERVE_ENDPOINT", "http://127.0.0.1:5080")
	t.Setenv("REQUEST_LOG_OPENOBSERVE_ORG", "default")
	t.Setenv("REQUEST_LOG_OPENOBSERVE_STREAM", "request_log_records")
	t.Setenv("REQUEST_LOG_OPENOBSERVE_USER", "root@example.com")
	t.Setenv("REQUEST_LOG_OPENOBSERVE_PASSWORD", "secret")

	require.NoError(t, InitRequestLogStore())
	require.True(t, common.RequestLogEnabled)
	require.NotNil(t, requestLogQueue)
	require.Equal(t, 2, cap(requestLogQueue))
}

func TestOpenObserveRequestLogStoreWritesBatch(t *testing.T) {
	originalRequestMax := common.RequestLogMaxRequestBytes
	originalResponseMax := common.RequestLogMaxResponseBytes
	originalRedact := common.RequestLogRedactEnabled
	t.Cleanup(func() {
		common.RequestLogMaxRequestBytes = originalRequestMax
		common.RequestLogMaxResponseBytes = originalResponseMax
		common.RequestLogRedactEnabled = originalRedact
	})
	common.RequestLogMaxRequestBytes = 1024
	common.RequestLogMaxResponseBytes = 1024
	common.RequestLogRedactEnabled = true

	var gotPath string
	var gotAuth string
	var gotContentType string
	var records []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotContentType = r.Header.Get("Content-Type")
		require.NoError(t, common.DecodeJson(r.Body, &records))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"code":200,"status":"ok","records":1}`))
	}))
	defer server.Close()

	store := newOpenObserveRequestLogStore(openObserveRequestLogConfig{
		Endpoint: server.URL + "/",
		Org:      "default",
		Stream:   "request_log_records",
		User:     "root@example.com",
		Password: "secret",
		Timeout:  time.Second,
	})

	record := RequestLogRecord{
		CreatedAt:    1710000000,
		UserId:       12,
		RequestId:    "req_123",
		RequestPath:  "/v1/chat/completions",
		Status:       "success",
		StatusCode:   200,
		RequestBody:  `{"api_key":"sk-secret"}`,
		ResponseBody: `{"ok":true}`,
	}
	prepareRequestLogRecord(&record)
	err := store.WriteBatch([]RequestLogRecord{record})

	require.NoError(t, err)
	require.Equal(t, "/api/default/request_log_records/_json", gotPath)
	require.Equal(t, "Basic cm9vdEBleGFtcGxlLmNvbTpzZWNyZXQ=", gotAuth)
	require.Contains(t, gotContentType, "application/json")
	require.Len(t, records, 1)
	require.Equal(t, float64(1710000000000000), records[0]["_timestamp"])
	require.Equal(t, "request_log", records[0]["log_type"])
	require.Equal(t, "req_123", records[0]["request_id"])
	require.Equal(t, "success", records[0]["status"])
	require.NotContains(t, records[0]["request_body"], "sk-secret")
	require.Contains(t, records[0]["request_body"], requestLogRedactedVal)
}

func TestRecordRequestLogSanitizesAndTruncatesContent(t *testing.T) {
	originalEnabled := common.RequestLogEnabled
	originalRequestMax := common.RequestLogMaxRequestBytes
	originalResponseMax := common.RequestLogMaxResponseBytes
	originalRedact := common.RequestLogRedactEnabled
	originalStore := requestLogStore
	t.Cleanup(func() {
		common.RequestLogEnabled = originalEnabled
		common.RequestLogMaxRequestBytes = originalRequestMax
		common.RequestLogMaxResponseBytes = originalResponseMax
		common.RequestLogRedactEnabled = originalRedact
		requestLogStore = originalStore
	})

	common.RequestLogEnabled = true
	common.RequestLogMaxRequestBytes = 32
	common.RequestLogMaxResponseBytes = 24
	common.RequestLogRedactEnabled = true
	store := &captureRequestLogStore{}
	requestLogStore = store

	createdAt := time.Now().Unix()
	record, err := RecordRequestLog(RequestLogRecord{
		CreatedAt:      createdAt,
		UserId:         12,
		Username:       "alice",
		UserGroup:      "vip",
		UserEmail:      "alice@example.com",
		TokenId:        34,
		TokenName:      "prod-token",
		ChannelId:      56,
		ChannelName:    "openai-main",
		ChannelType:    1,
		ChannelBaseUrl: "https://upstream.example.com/v1",
		RequestIP:      "203.0.113.10",
		RequestId:      "req_123",
		ModelName:      "gpt-test",
		RequestBody:    `{"api_key":"sk-secret","messages":[{"content":"` + strings.Repeat("hello", 20) + `"}]}`,
		ResponseBody:   `{"output":"` + strings.Repeat("world", 20) + `"}`,
	})

	require.NoError(t, err)
	require.Equal(t, createdAt, record.CreatedAt)
	require.Equal(t, 12, record.UserId)
	require.Equal(t, "req_123", record.RequestId)
	require.True(t, record.RequestTruncated)
	require.True(t, record.ResponseTruncated)
	require.NotEmpty(t, record.RequestHash)
	require.NotEmpty(t, record.ResponseHash)
	require.NotContains(t, record.RequestBody, "sk-secret")
	require.Contains(t, record.RequestBody, "[REDACTED]")
	require.LessOrEqual(t, len(record.RequestBody), common.RequestLogMaxRequestBytes+len(truncatedSuffix))
	require.LessOrEqual(t, len(record.ResponseBody), common.RequestLogMaxResponseBytes+len(truncatedSuffix))
	require.Len(t, store.records, 1)
	require.Equal(t, record.RequestBody, store.records[0].RequestBody)
	require.Equal(t, record.ResponseBody, store.records[0].ResponseBody)
}

func TestRequestLogRedactionPreservesTokenUsageFields(t *testing.T) {
	content := `{"max_tokens":128,"usage":{"prompt_tokens":12,"completion_tokens":34},"access_token":"secret-value"}`

	redacted := redactContent(content)

	require.Contains(t, redacted, `"max_tokens":128`)
	require.Contains(t, redacted, `"prompt_tokens":12`)
	require.Contains(t, redacted, `"completion_tokens":34`)
	require.NotContains(t, redacted, "secret-value")
	require.Contains(t, redacted, requestLogRedactedVal)
}

func TestRecordRequestLogPreservesStatusWhenResponseBodyContainsLatinText(t *testing.T) {
	originalLogDB := LOG_DB
	originalEnabled := common.RequestLogEnabled
	originalRedact := common.RequestLogRedactEnabled
	originalStore := requestLogStore
	t.Cleanup(func() {
		LOG_DB = originalLogDB
		common.RequestLogEnabled = originalEnabled
		common.RequestLogRedactEnabled = originalRedact
		requestLogStore = originalStore
	})

	common.RequestLogEnabled = true
	common.RequestLogRedactEnabled = false
	store := &captureRequestLogStore{}
	requestLogStore = store

	origin := Log{
		CreatedAt: common.GetTimestamp(),
		Type:      LogTypeConsume,
		UserId:    12,
		Username:  "alice",
		Content:   "normal consume",
		RequestId: "req_latin_text",
	}
	require.NoError(t, LOG_DB.Create(&origin).Error)

	record, err := RecordRequestLog(RequestLogRecord{
		UserId:       12,
		RequestId:    "req_latin_text",
		Status:       "success",
		StatusCode:   200,
		ResponseBody: `{"output":"ä½ å¥½ï¼Œè¿æ¯ä¹±ç "}`,
	})

	require.NoError(t, err)
	require.Equal(t, "success", record.Status)
	require.Empty(t, record.ErrorCode)
	require.Empty(t, record.ErrorMessage)

	var saved Log
	require.NoError(t, LOG_DB.First(&saved, origin.Id).Error)
	require.Equal(t, LogTypeConsume, saved.Type)
	require.Equal(t, "normal consume", saved.Content)
}

func TestRecordRequestLogNormalizesInvalidUTF8Content(t *testing.T) {
	originalEnabled := common.RequestLogEnabled
	originalRedact := common.RequestLogRedactEnabled
	originalStore := requestLogStore
	t.Cleanup(func() {
		common.RequestLogEnabled = originalEnabled
		common.RequestLogRedactEnabled = originalRedact
		requestLogStore = originalStore
	})

	common.RequestLogEnabled = true
	common.RequestLogRedactEnabled = false
	requestLogStore = &captureRequestLogStore{}

	record, err := RecordRequestLog(RequestLogRecord{
		RequestId:    "req_binary",
		RequestBody:  string([]byte{0xff, 0xfe, 0xfd}),
		ResponseBody: string([]byte{0x1f, 0x8b, 0x08, 0xff}),
	})

	require.NoError(t, err)
	require.Contains(t, record.RequestBody, binaryContentPrefix)
	require.Contains(t, record.ResponseBody, binaryContentPrefix)
	require.True(t, utf8.ValidString(record.RequestBody))
	require.True(t, utf8.ValidString(record.ResponseBody))
}

func TestTruncateContentPreservesValidUTF8(t *testing.T) {
	content, truncated := truncateContent("你好世界", 5)

	require.True(t, truncated)
	require.True(t, utf8.ValidString(content))
	require.Contains(t, content, truncatedSuffix)
}

func TestEnqueueRequestLogNeverBlocksWhenQueueIsFull(t *testing.T) {
	originalEnabled := common.RequestLogEnabled
	originalQueue := requestLogQueue
	originalStore := requestLogStore
	t.Cleanup(func() {
		common.RequestLogEnabled = originalEnabled
		requestLogQueue = originalQueue
		requestLogStore = originalStore
	})

	common.RequestLogEnabled = true
	requestLogQueue = make(chan RequestLogRecord, 1)
	requestLogQueue <- RequestLogRecord{RequestId: "already-full"}

	ok := EnqueueRequestLog(RequestLogRecord{RequestId: "drop-me"})
	require.False(t, ok)
}

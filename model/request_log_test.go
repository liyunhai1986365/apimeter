package model

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func TestInitRequestLogDBDisabledWithoutDSN(t *testing.T) {
	originalDB := REQUEST_LOG_DB
	originalEnabled := common.RequestLogEnabled
	t.Cleanup(func() {
		REQUEST_LOG_DB = originalDB
		common.RequestLogEnabled = originalEnabled
	})

	REQUEST_LOG_DB = nil
	common.RequestLogEnabled = true
	t.Setenv("REQUEST_LOG_SQL_DSN", "")

	require.NoError(t, InitRequestLogDB())
	require.Nil(t, REQUEST_LOG_DB)
	require.False(t, common.RequestLogEnabled)
}

func TestRecordRequestLogSanitizesAndTruncatesContent(t *testing.T) {
	originalDB := REQUEST_LOG_DB
	originalEnabled := common.RequestLogEnabled
	originalRequestMax := common.RequestLogMaxRequestBytes
	originalResponseMax := common.RequestLogMaxResponseBytes
	originalRedact := common.RequestLogRedactEnabled
	t.Cleanup(func() {
		REQUEST_LOG_DB = originalDB
		common.RequestLogEnabled = originalEnabled
		common.RequestLogMaxRequestBytes = originalRequestMax
		common.RequestLogMaxResponseBytes = originalResponseMax
		common.RequestLogRedactEnabled = originalRedact
	})

	dbPath := t.TempDir() + "/request-log.db"
	t.Setenv("REQUEST_LOG_SQL_DSN", "file:"+dbPath+"?cache=shared")
	common.RequestLogEnabled = true
	common.RequestLogMaxRequestBytes = 32
	common.RequestLogMaxResponseBytes = 24
	common.RequestLogRedactEnabled = true

	require.NoError(t, InitRequestLogDB())
	require.NotNil(t, REQUEST_LOG_DB)

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
	require.NotZero(t, record.Id)
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

	var saved RequestLogRecord
	require.NoError(t, REQUEST_LOG_DB.First(&saved, "request_id = ?", "req_123").Error)
	require.Equal(t, record.RequestBody, saved.RequestBody)
	require.Equal(t, record.ResponseBody, saved.ResponseBody)
}

func TestRecordRequestLogMarksOriginalLogAsErrorWhenResponseHasMojibake(t *testing.T) {
	originalDB := REQUEST_LOG_DB
	originalLogDB := LOG_DB
	originalEnabled := common.RequestLogEnabled
	originalRedact := common.RequestLogRedactEnabled
	t.Cleanup(func() {
		REQUEST_LOG_DB = originalDB
		LOG_DB = originalLogDB
		common.RequestLogEnabled = originalEnabled
		common.RequestLogRedactEnabled = originalRedact
	})

	dbPath := t.TempDir() + "/request-log.db"
	t.Setenv("REQUEST_LOG_SQL_DSN", "file:"+dbPath+"?cache=shared")
	common.RequestLogEnabled = true
	common.RequestLogRedactEnabled = false

	require.NoError(t, InitRequestLogDB())
	LOG_DB = REQUEST_LOG_DB
	require.NoError(t, LOG_DB.AutoMigrate(&Log{}))

	origin := Log{
		CreatedAt: common.GetTimestamp(),
		Type:      LogTypeConsume,
		UserId:    12,
		Username:  "alice",
		Content:   "normal consume",
		RequestId: "req_mojibake",
	}
	require.NoError(t, LOG_DB.Create(&origin).Error)

	record, err := RecordRequestLog(RequestLogRecord{
		UserId:       12,
		RequestId:    "req_mojibake",
		Status:       "success",
		StatusCode:   200,
		ResponseBody: `{"output":"ä½ å¥½ï¼Œè¿æ¯ä¹±ç "}`,
	})

	require.NoError(t, err)
	require.Equal(t, "error", record.Status)
	require.Equal(t, "mojibake_detected", record.ErrorCode)
	require.Contains(t, record.ErrorMessage, "乱码")

	var saved Log
	require.NoError(t, LOG_DB.First(&saved, origin.Id).Error)
	require.Equal(t, LogTypeError, saved.Type)
	require.Equal(t, "返回结果存在乱码", saved.Content)
}

func TestRecordRequestLogNormalizesInvalidUTF8Content(t *testing.T) {
	originalDB := REQUEST_LOG_DB
	originalEnabled := common.RequestLogEnabled
	originalRedact := common.RequestLogRedactEnabled
	t.Cleanup(func() {
		REQUEST_LOG_DB = originalDB
		common.RequestLogEnabled = originalEnabled
		common.RequestLogRedactEnabled = originalRedact
	})

	dbPath := t.TempDir() + "/request-log.db"
	t.Setenv("REQUEST_LOG_SQL_DSN", "file:"+dbPath+"?cache=shared")
	common.RequestLogEnabled = true
	common.RequestLogRedactEnabled = false

	require.NoError(t, InitRequestLogDB())

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
	originalDB := REQUEST_LOG_DB
	originalEnabled := common.RequestLogEnabled
	originalQueue := requestLogQueue
	t.Cleanup(func() {
		REQUEST_LOG_DB = originalDB
		common.RequestLogEnabled = originalEnabled
		requestLogQueue = originalQueue
	})

	REQUEST_LOG_DB = nil
	common.RequestLogEnabled = true
	requestLogQueue = make(chan RequestLogRecord, 1)
	requestLogQueue <- RequestLogRecord{RequestId: "already-full"}

	ok := EnqueueRequestLog(RequestLogRecord{RequestId: "drop-me"})
	require.False(t, ok)
}

package model

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func TestRecordErrorRequestLogTrimsAndStoresRequestEvidence(t *testing.T) {
	truncateTables(t)

	record := &ErrorRequestLog{
		LogId:          12,
		RequestId:      " req-error ",
		UserId:         10,
		TokenId:        20,
		ModelName:      " gpt-4o ",
		RequestMethod:  " POST ",
		RequestPath:    " /v1/chat/completions ",
		RequestBody:    strings.Repeat("x", 32),
		RequestHash:    " hash-1 ",
		RequestHeaders: `{"Authorization":"[REDACTED]"}`,
		ErrorCode:      " invalid_request ",
		StatusCode:     400,
	}

	require.NoError(t, RecordErrorRequestLog(record))
	require.NotZero(t, record.Id)

	var saved ErrorRequestLog
	require.NoError(t, LOG_DB.First(&saved, record.Id).Error)
	require.Equal(t, "req-error", saved.RequestId)
	require.Equal(t, "gpt-4o", saved.ModelName)
	require.Equal(t, "POST", saved.RequestMethod)
	require.Equal(t, "/v1/chat/completions", saved.RequestPath)
	require.Equal(t, "hash-1", saved.RequestHash)
	require.Equal(t, 12, saved.LogId)
	require.Equal(t, "invalid_request", saved.ErrorCode)
	require.Contains(t, saved.RequestHeaders, "[REDACTED]")

	byLogID, err := GetErrorRequestLogByLogID(12)
	require.NoError(t, err)
	require.Equal(t, saved.Id, byLogID.Id)
}

func TestEnqueueErrorRequestLogNeverBlocksWhenQueueIsFull(t *testing.T) {
	originalQueue := errorRequestLogQueue
	t.Cleanup(func() {
		errorRequestLogQueue = originalQueue
	})

	errorRequestLogQueue = make(chan *ErrorRequestLog, 1)
	errorRequestLogQueue <- &ErrorRequestLog{RequestId: "already-full"}

	ok := EnqueueErrorRequestLog(&ErrorRequestLog{RequestId: "drop-me"})
	require.False(t, ok)
}

func TestFlushErrorRequestLogPersistsAndAttachesReference(t *testing.T) {
	truncateTables(t)

	log := &Log{
		Type:      LogTypeError,
		UserId:    10,
		ChannelId: 20,
		ModelName: "gpt-4o",
		TokenName: "prod-token",
		Content:   "upstream failed",
		TokenId:   30,
		Other: common.MapToJsonStr(map[string]interface{}{
			"request_hash": "hash-before",
		}),
	}
	require.NoError(t, LOG_DB.Create(log).Error)
	require.NotZero(t, log.Id)

	record := &ErrorRequestLog{
		LogId:         log.Id,
		RequestId:     "req-async",
		RequestHash:   "hash-after",
		RequestBody:   `{"model":"gpt-4o"}`,
		ErrorCode:     "bad_response",
		StatusCode:    502,
		RequestPath:   "/v1/chat/completions",
		RequestMethod: "POST",
	}

	require.NoError(t, flushErrorRequestLog(record))
	require.NotZero(t, record.Id)

	var saved ErrorRequestLog
	require.NoError(t, LOG_DB.First(&saved, record.Id).Error)
	require.Equal(t, log.Id, saved.LogId)
	require.Equal(t, "req-async", saved.RequestId)

	var savedLog Log
	require.NoError(t, LOG_DB.First(&savedLog, log.Id).Error)
	other, err := common.StrToMap(savedLog.Other)
	require.NoError(t, err)
	require.Equal(t, float64(record.Id), other["error_request_log_id"])
	require.Equal(t, "hash-after", other["request_hash"])
}

package model

import (
	"strings"
	"testing"

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

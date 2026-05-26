package model

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetUserLogsDeduplicatesErrorLogsByRequestID(t *testing.T) {
	truncateTables(t)

	require.NoError(t, LOG_DB.Create(&[]Log{
		{Id: 1, UserId: 1001, Type: LogTypeError, CreatedAt: 100, RequestId: "req_same", Content: "first error"},
		{Id: 2, UserId: 1001, Type: LogTypeError, CreatedAt: 101, RequestId: "req_same", Content: "last error"},
		{Id: 3, UserId: 1001, Type: LogTypeError, CreatedAt: 102, RequestId: "req_other", Content: "other error"},
		{Id: 4, UserId: 1001, Type: LogTypeError, CreatedAt: 103, RequestId: "", Content: "empty request id"},
		{Id: 5, UserId: 1001, Type: LogTypeError, CreatedAt: 104, RequestId: "", Content: "empty request id second"},
		{Id: 6, UserId: 1001, Type: LogTypeConsume, CreatedAt: 105, RequestId: "req_same", Content: "consume"},
	}).Error)

	logs, total, err := GetUserLogs(1001, LogTypeUnknown, 0, 0, "", "", 0, 20, "", "", "")

	require.NoError(t, err)
	require.Equal(t, int64(5), total)
	contents := logContents(logs)
	require.NotContains(t, contents, "first error")
	require.Contains(t, contents, "last error")
	require.Contains(t, contents, "other error")
	require.Contains(t, contents, "empty request id")
	require.Contains(t, contents, "empty request id second")
	require.Contains(t, contents, "consume")
}

func TestGetAllLogsDeduplicatesErrorLogsByRequestIDWithinFilters(t *testing.T) {
	truncateTables(t)

	require.NoError(t, LOG_DB.Create(&[]Log{
		{Id: 11, UserId: 1001, Type: LogTypeError, CreatedAt: 100, RequestId: "req_filtered", ModelName: "gpt-a", Content: "first filtered error"},
		{Id: 12, UserId: 1001, Type: LogTypeError, CreatedAt: 101, RequestId: "req_filtered", ModelName: "gpt-a", Content: "last filtered error"},
		{Id: 13, UserId: 1001, Type: LogTypeError, CreatedAt: 102, RequestId: "req_filtered", ModelName: "gpt-b", Content: "newer outside model filter"},
	}).Error)

	logs, total, err := GetAllLogs(LogTypeError, 0, 0, "gpt-a", "", "", 0, 20, 0, "", "", "")

	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, logs, 1)
	require.Equal(t, "last filtered error", logs[0].Content)
}

func logContents(logs []*Log) []string {
	contents := make([]string, 0, len(logs))
	for _, log := range logs {
		contents = append(contents, log.Content)
	}
	return contents
}

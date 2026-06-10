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

func TestGetUserLogsFiltersByWorkspaceName(t *testing.T) {
	truncateTables(t)

	alpha := Workspace{Id: 101, UserId: 1001, Name: "Project Alpha", Status: WorkspaceStatusEnabled}
	beta := Workspace{Id: 102, UserId: 1001, Name: "Project Beta", Status: WorkspaceStatusEnabled}
	require.NoError(t, DB.Create(&[]Workspace{alpha, beta}).Error)
	require.NoError(t, DB.Create(&[]Token{
		{Id: 201, UserId: 1001, WorkspaceId: alpha.Id, Name: "alpha-key", Key: "alpha-log-key"},
		{Id: 202, UserId: 1001, WorkspaceId: beta.Id, Name: "beta-key", Key: "beta-log-key"},
	}).Error)
	require.NoError(t, LOG_DB.Create(&[]Log{
		{Id: 31, UserId: 1001, Type: LogTypeConsume, CreatedAt: 100, TokenId: 201, TokenName: "alpha-key", Content: "alpha usage"},
		{Id: 32, UserId: 1001, Type: LogTypeConsume, CreatedAt: 101, TokenId: 202, TokenName: "beta-key", Content: "beta usage"},
	}).Error)

	logs, total, err := GetUserLogs(1001, LogTypeUnknown, 0, 0, "", "", 0, 20, "", "", "", "Project Alpha")

	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, logs, 1)
	require.Equal(t, "alpha usage", logs[0].Content)
}

func TestSumUsedQuotaFiltersByTokenAndWorkspaceName(t *testing.T) {
	truncateTables(t)

	alpha := Workspace{Id: 111, UserId: 1001, Name: "Workspace Alpha", Status: WorkspaceStatusEnabled}
	beta := Workspace{Id: 112, UserId: 1001, Name: "Workspace Beta", Status: WorkspaceStatusEnabled}
	require.NoError(t, DB.Create(&[]Workspace{alpha, beta}).Error)
	require.NoError(t, DB.Create(&[]Token{
		{Id: 211, UserId: 1001, WorkspaceId: alpha.Id, Name: "shared-key", Key: "alpha-shared-key"},
		{Id: 212, UserId: 1001, WorkspaceId: beta.Id, Name: "shared-key", Key: "beta-shared-key"},
	}).Error)
	require.NoError(t, LOG_DB.Create(&[]Log{
		{Id: 41, UserId: 1001, Type: LogTypeConsume, CreatedAt: 100, TokenId: 211, TokenName: "shared-key", Quota: 100, PromptTokens: 3, CompletionTokens: 7},
		{Id: 42, UserId: 1001, Type: LogTypeConsume, CreatedAt: 101, TokenId: 212, TokenName: "shared-key", Quota: 300, PromptTokens: 11, CompletionTokens: 13},
	}).Error)

	stat, err := SumUsedQuota(LogTypeConsume, 0, 0, "", "", "shared-key", 0, "", "Workspace Alpha")

	require.NoError(t, err)
	require.Equal(t, 100, stat.Quota)
}

func TestGetQuotaDatesFromLogsFiltersByWorkspaceAndToken(t *testing.T) {
	truncateTables(t)

	alpha := Workspace{Id: 121, UserId: 1001, Name: "Analytics Alpha", Status: WorkspaceStatusEnabled}
	beta := Workspace{Id: 122, UserId: 1001, Name: "Analytics Beta", Status: WorkspaceStatusEnabled}
	require.NoError(t, DB.Create(&[]Workspace{alpha, beta}).Error)
	require.NoError(t, DB.Create(&[]Token{
		{Id: 221, UserId: 1001, WorkspaceId: alpha.Id, Name: "analytics-key", Key: "alpha-analytics-key"},
		{Id: 222, UserId: 1001, WorkspaceId: beta.Id, Name: "analytics-key", Key: "beta-analytics-key"},
	}).Error)
	require.NoError(t, LOG_DB.Create(&[]Log{
		{Id: 51, UserId: 1001, Username: "alice", Type: LogTypeConsume, CreatedAt: 3600, TokenId: 221, TokenName: "analytics-key", ModelName: "gpt-alpha", Quota: 150, PromptTokens: 10, CompletionTokens: 20},
		{Id: 52, UserId: 1001, Username: "alice", Type: LogTypeConsume, CreatedAt: 7200, TokenId: 222, TokenName: "analytics-key", ModelName: "gpt-beta", Quota: 450, PromptTokens: 30, CompletionTokens: 40},
	}).Error)

	data, err := GetQuotaDatesFromLogs(0, 10000, "", "analytics-key", "Analytics Alpha", 0)

	require.NoError(t, err)
	require.Len(t, data, 1)
	require.Equal(t, "gpt-alpha", data[0].ModelName)
	require.Equal(t, 150, data[0].Quota)
	require.Equal(t, 30, data[0].TokenUsed)
}

func TestTaskLogsFilterByTokenAndWorkspaceName(t *testing.T) {
	truncateTables(t)

	alpha := Workspace{Id: 131, UserId: 1001, Name: "Task Alpha", Status: WorkspaceStatusEnabled}
	beta := Workspace{Id: 132, UserId: 1001, Name: "Task Beta", Status: WorkspaceStatusEnabled}
	require.NoError(t, DB.Create(&[]Workspace{alpha, beta}).Error)
	require.NoError(t, DB.Create(&[]Token{
		{Id: 231, UserId: 1001, WorkspaceId: alpha.Id, Name: "task-key", Key: "alpha-task-key"},
		{Id: 232, UserId: 1001, WorkspaceId: beta.Id, Name: "task-key", Key: "beta-task-key"},
	}).Error)
	require.NoError(t, DB.Create(&[]Task{
		{TaskID: "task-alpha", UserId: 1001, TokenId: 231, TokenName: "task-key", Status: TaskStatusSuccess, SubmitTime: 100},
		{TaskID: "task-beta", UserId: 1001, TokenId: 232, TokenName: "task-key", Status: TaskStatusSuccess, SubmitTime: 101},
	}).Error)

	tasks := TaskGetAllUserTask(1001, 0, 20, SyncTaskQueryParams{
		TokenName:     "task-key",
		WorkspaceName: "Task Alpha",
	})
	total := TaskCountAllUserTask(1001, SyncTaskQueryParams{
		TokenName:     "task-key",
		WorkspaceName: "Task Alpha",
	})

	require.Equal(t, int64(1), total)
	require.Len(t, tasks, 1)
	require.Equal(t, "task-alpha", tasks[0].TaskID)
}

func TestBackfillTaskTokenFieldsFromPrivateData(t *testing.T) {
	truncateTables(t)

	workspace := Workspace{Id: 141, UserId: 1001, Name: "Backfill Workspace", Status: WorkspaceStatusEnabled}
	require.NoError(t, DB.Create(&workspace).Error)
	require.NoError(t, DB.Create(&Token{
		Id:          241,
		UserId:      1001,
		WorkspaceId: workspace.Id,
		Name:        "backfill-key",
		Key:         "backfill-token-key",
	}).Error)
	require.NoError(t, DB.Create(&Task{
		TaskID:      "task-backfill",
		UserId:      1001,
		PrivateData: TaskPrivateData{TokenId: 241},
		Status:      TaskStatusSuccess,
		SubmitTime:  100,
	}).Error)

	require.NoError(t, BackfillTaskTokenFields())

	var task Task
	require.NoError(t, DB.Where("task_id = ?", "task-backfill").First(&task).Error)
	require.Equal(t, 241, task.TokenId)
	require.Equal(t, "backfill-key", task.TokenName)
}

func logContents(logs []*Log) []string {
	contents := make([]string, 0, len(logs))
	for _, log := range logs {
		contents = append(contents, log.Content)
	}
	return contents
}

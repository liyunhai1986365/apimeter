package model

import (
	"fmt"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupAgentAnalyticsTestDB(t *testing.T) {
	t.Helper()
	originalDB := DB
	originalLogDB := LOG_DB
	originalGroupCol := commonGroupCol
	originalLogGroupCol := logGroupCol

	mainDB, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:agent-analytics-main-%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	require.NoError(t, err)
	logDB, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:agent-analytics-log-%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	require.NoError(t, err)
	DB = mainDB
	LOG_DB = logDB
	commonGroupCol = "`group`"
	logGroupCol = "`group`"
	require.NoError(t, DB.AutoMigrate(&AgentUser{}, &AgentLedger{}))
	require.NoError(t, LOG_DB.AutoMigrate(&Log{}))
	t.Cleanup(func() {
		DB = originalDB
		LOG_DB = originalLogDB
		commonGroupCol = originalGroupCol
		logGroupCol = originalLogGroupCol
	})
}

func TestGetAgentAnalyticsScopesAndAggregatesLogs(t *testing.T) {
	setupAgentAnalyticsTestDB(t)
	require.NoError(t, DB.Create(&[]AgentUser{
		{AgentId: 1, UserId: 101, Status: AgentUserStatusEnabled, CreatedAt: 1500},
		{AgentId: 1, UserId: 102, Status: AgentUserStatusDisabled, CreatedAt: 500},
		{AgentId: 2, UserId: 201, Status: AgentUserStatusEnabled, CreatedAt: 1500},
	}).Error)
	require.NoError(t, DB.Create(&[]AgentLedger{
		{AgentId: 1, UserId: 101, Type: AgentLedgerTypeConsumeProfit, ProfitQuota: 120, CreatedAt: 1800},
		{AgentId: 2, UserId: 201, Type: AgentLedgerTypeConsumeProfit, ProfitQuota: 999, CreatedAt: 1800},
	}).Error)
	require.NoError(t, LOG_DB.Create(&[]Log{
		{UserId: 101, Username: "alice", CreatedAt: 1200, Type: LogTypeConsume, ModelName: "gpt-5", PromptTokens: 10, CompletionTokens: 5, Quota: 100},
		{UserId: 101, Username: "alice", CreatedAt: 1800, Type: LogTypeError, ModelName: "gpt-5"},
		{UserId: 102, Username: "bob", CreatedAt: 2400, Type: LogTypeConsume, ModelName: "claude", PromptTokens: 20, CompletionTokens: 7, Quota: 300},
		{UserId: 201, Username: "other-agent", CreatedAt: 2400, Type: LogTypeConsume, ModelName: "gpt-5", PromptTokens: 999, Quota: 9999},
	}).Error)

	analytics, err := GetAgentAnalytics(1, 1000, 3000, 1000)
	require.NoError(t, err)
	require.Equal(t, int64(2), analytics.Summary.TotalUsers)
	require.Equal(t, int64(1), analytics.Summary.ActiveUsers)
	require.Equal(t, int64(1), analytics.Summary.NewUsers)
	require.Equal(t, int64(2), analytics.Summary.UsageUsers)
	require.Equal(t, int64(3), analytics.Summary.TotalRequests)
	require.Equal(t, int64(2), analytics.Summary.SuccessfulRequests)
	require.Equal(t, int64(1), analytics.Summary.FailedRequests)
	require.Equal(t, int64(42), analytics.Summary.TotalTokens)
	require.Equal(t, int64(400), analytics.Summary.TotalQuota)
	require.Equal(t, int64(120), analytics.Summary.ProfitQuota)
	require.InDelta(t, 66.666, analytics.Summary.SuccessRate, 0.01)
	require.Len(t, analytics.Trend, 3)
	require.Equal(t, int64(1), analytics.Trend[0].Requests)
	require.Equal(t, int64(1), analytics.Trend[0].Errors)
	require.Equal(t, "claude", analytics.TopModels[0].ModelName)
	require.Equal(t, "alice", analytics.TopUsers[0].Username)
	require.Len(t, analytics.RecentLogs, 3)
	for _, item := range analytics.RecentLogs {
		require.NotEqual(t, 201, item.UserId)
	}
}

func TestGetAgentAnalyticsLogsScopesFiltersAndPaginates(t *testing.T) {
	setupAgentAnalyticsTestDB(t)
	require.NoError(t, DB.Create(&[]AgentUser{
		{AgentId: 1, UserId: 101, Status: AgentUserStatusEnabled},
		{AgentId: 2, UserId: 201, Status: AgentUserStatusEnabled},
	}).Error)
	require.NoError(t, LOG_DB.Create(&[]Log{
		{UserId: 101, Username: "alice", CreatedAt: 1000, Type: LogTypeConsume, ModelName: "gpt-5", RequestId: "req-1"},
		{UserId: 101, Username: "alice", CreatedAt: 1100, Type: LogTypeError, ModelName: "gpt-5", RequestId: "req-2"},
		{UserId: 101, Username: "alice", CreatedAt: 1200, Type: LogTypeConsume, ModelName: "claude", RequestId: "req-3"},
		{UserId: 201, Username: "other-agent", CreatedAt: 1300, Type: LogTypeConsume, ModelName: "gpt-5", RequestId: "req-4"},
	}).Error)

	logs, total, err := GetAgentAnalyticsLogs(1, AgentAnalyticsLogFilters{
		StartTimestamp: 900,
		EndTimestamp:   1400,
		Type:           LogTypeConsume,
		ModelName:      "gpt",
	}, 0, 20)
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, logs, 1)
	require.Equal(t, "req-1", logs[0].RequestId)

	logs, total, err = GetAgentAnalyticsLogs(1, AgentAnalyticsLogFilters{
		StartTimestamp: 900,
		EndTimestamp:   1400,
	}, 1, 1)
	require.NoError(t, err)
	require.Equal(t, int64(3), total)
	require.Len(t, logs, 1)
	require.Equal(t, "req-2", logs[0].RequestId)
}

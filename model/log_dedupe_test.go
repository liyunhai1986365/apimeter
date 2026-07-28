package model

import (
	"strconv"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
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

	logs, total, err := GetUserLogs(1001, LogTypeUnknown, 0, 0, "", "", 0, 20, "", "", "", "", nil)

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

	logs, total, err := GetUserLogs(1001, LogTypeUnknown, 0, 0, "", "", 0, 20, "", "", "", "Project Alpha", nil)

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

	stat, err := SumUsedQuota(LogTypeConsume, 0, 0, "", "", "shared-key", 0, "", "Workspace Alpha", nil)

	require.NoError(t, err)
	require.Equal(t, 100, stat.Quota)
}

func TestSumUsedQuotaAggregatesChannelCostFields(t *testing.T) {
	truncateTables(t)

	require.NoError(t, LOG_DB.Create(&[]Log{
		{
			Id:        141,
			UserId:    1001,
			Type:      LogTypeConsume,
			CreatedAt: 100,
			Quota:     1500,
			Other:     `{"group_ratio":1.5,"channel_ratio":0.8,"cost_base_quota":1000,"cost_quota":800,"profit_quota":700}`,
		},
		{
			Id:        142,
			UserId:    1001,
			Type:      LogTypeConsume,
			CreatedAt: 101,
			Quota:     600,
			Other:     `{"group_ratio":1.2,"channel_ratio":0.5}`,
		},
		{
			Id:        143,
			UserId:    1001,
			Type:      LogTypeError,
			CreatedAt: 102,
			Quota:     900,
			Other:     `{"group_ratio":1.5,"channel_ratio":0.8,"cost_quota":480}`,
		},
	}).Error)

	stat, err := SumUsedQuota(LogTypeConsume, 0, 0, "", "", "", 0, "", "", nil)

	require.NoError(t, err)
	require.Equal(t, 2100, stat.Quota)
	require.Equal(t, 1050, stat.CostQuota)
	require.Equal(t, 1050, stat.ProfitQuota)
}

func TestSumUsedQuotaSubtractsRefundLogs(t *testing.T) {
	truncateTables(t)

	now := time.Now().Unix()
	require.NoError(t, LOG_DB.Create(&[]Log{
		{
			Id:               144,
			UserId:           1001,
			Type:             LogTypeConsume,
			CreatedAt:        100,
			Quota:            1500,
			PromptTokens:     10,
			CompletionTokens: 20,
			Other:            `{"cost_base_quota":1000,"cost_quota":800,"profit_quota":700}`,
		},
		{
			Id:        145,
			UserId:    1001,
			Type:      LogTypeRefund,
			CreatedAt: 101,
			Quota:     500,
			Other:     `{"cost_base_quota":400,"cost_quota":300,"profit_quota":200}`,
		},
		{
			Id:               146,
			UserId:           1001,
			Type:             LogTypeConsume,
			CreatedAt:        now,
			Quota:            200,
			PromptTokens:     3,
			CompletionTokens: 4,
			Other:            `{"cost_quota":100,"profit_quota":100}`,
		},
	}).Error)

	stat, err := SumUsedQuota(LogTypeUnknown, 0, 0, "", "", "", 0, "", "", nil)

	require.NoError(t, err)
	require.Equal(t, 1200, stat.Quota)
	require.Equal(t, 600, stat.CostQuota)
	require.Equal(t, 600, stat.ProfitQuota)
	require.Equal(t, 1, stat.Rpm)
	require.Equal(t, 7, stat.Tpm)

	refundStat, err := SumUsedQuota(LogTypeRefund, 0, 0, "", "", "", 0, "", "", nil)
	require.NoError(t, err)
	require.Equal(t, -500, refundStat.Quota)
	require.Equal(t, -300, refundStat.CostQuota)
	require.Equal(t, -200, refundStat.ProfitQuota)
	require.Equal(t, 0, refundStat.Rpm)
	require.Equal(t, 0, refundStat.Tpm)
}

func TestSumUsedQuotaByUserMatchesDashboardNetLedger(t *testing.T) {
	truncateTables(t)

	require.NoError(t, LOG_DB.Create(&[]Log{
		{Id: 147, UserId: 1001, Username: "old-name", Type: LogTypeConsume, CreatedAt: 100, Quota: 1000},
		{Id: 148, UserId: 1001, Username: "new-name", Type: LogTypeRefund, CreatedAt: 101, Quota: 700},
		{Id: 149, UserId: 2002, Username: "new-name", Type: LogTypeConsume, CreatedAt: 102, Quota: 9000},
	}).Error)

	stat, err := SumUsedQuotaByUser(1001, LogTypeUnknown, 0, 0, "", "", 0, "", "", nil)

	require.NoError(t, err)
	require.Equal(t, 300, stat.Quota)
}

func TestSumModelProfitStatsAggregatesByModelAndDate(t *testing.T) {
	truncateTables(t)

	require.NoError(t, LOG_DB.Create(&[]Log{
		{
			Id:        151,
			UserId:    1001,
			Type:      LogTypeConsume,
			CreatedAt: 100,
			ModelName: "gpt-4o",
			Quota:     1500,
			Other:     `{"cost_base_quota":1000,"cost_quota":800,"profit_quota":700}`,
		},
		{
			Id:        152,
			UserId:    1002,
			Type:      LogTypeConsume,
			CreatedAt: 120,
			ModelName: "gpt-4o",
			Quota:     300,
			Other:     `{"group_ratio":1.5,"channel_ratio":0.2}`,
		},
		{
			Id:        153,
			UserId:    1001,
			Type:      LogTypeConsume,
			CreatedAt: 130,
			ModelName: "claude-sonnet-4",
			Quota:     400,
			Other:     `{"cost_quota":500,"profit_quota":-100}`,
		},
		{
			Id:        154,
			UserId:    1001,
			Type:      LogTypeConsume,
			CreatedAt: 90,
			ModelName: "outside",
			Quota:     900,
			Other:     `{"cost_quota":100,"profit_quota":800}`,
		},
	}).Error)

	summary, err := SumModelProfitStats(LogTypeConsume, 100, 130, "", "", "", 0, "")

	require.NoError(t, err)
	require.Equal(t, 2200, summary.Quota)
	require.Equal(t, 1340, summary.CostQuota)
	require.Equal(t, 860, summary.ProfitQuota)
	require.Len(t, summary.Items, 2)

	require.Equal(t, "gpt-4o", summary.Items[0].ModelName)
	require.Equal(t, 1800, summary.Items[0].Quota)
	require.Equal(t, 840, summary.Items[0].CostQuota)
	require.Equal(t, 960, summary.Items[0].ProfitQuota)
	require.Equal(t, 2, summary.Items[0].RequestCount)

	require.Equal(t, "claude-sonnet-4", summary.Items[1].ModelName)
	require.Equal(t, 400, summary.Items[1].Quota)
	require.Equal(t, 500, summary.Items[1].CostQuota)
	require.Equal(t, -100, summary.Items[1].ProfitQuota)
	require.Equal(t, 1, summary.Items[1].RequestCount)
}

func TestFormatUserLogsRemovesChannelCostFields(t *testing.T) {
	logs := []*Log{
		{
			Id:    10,
			Other: `{"channel_ratio":0.8,"cost_base_quota":1000,"cost_quota":800,"profit_quota":700,"profit_rate":0.4667,"billing_source":"wallet","base_quota":900}`,
		},
	}

	formatUserLogs(logs, 0)

	require.NotContains(t, logs[0].Other, "channel_ratio")
	require.NotContains(t, logs[0].Other, "cost_base_quota")
	require.NotContains(t, logs[0].Other, "cost_quota")
	require.NotContains(t, logs[0].Other, "profit_quota")
	require.NotContains(t, logs[0].Other, "profit_rate")
	require.Contains(t, logs[0].Other, "billing_source")
	require.Contains(t, logs[0].Other, "base_quota")
}

func TestStripChannelCostFieldsFromLogsPreservesAdminFields(t *testing.T) {
	logs := []*Log{
		{
			Id:    10,
			Other: `{"admin_info":{"node":"n1"},"stream_status":"completed","channel_ratio":0.8,"cost_base_quota":1000,"cost_quota":800,"profit_quota":700,"profit_rate":0.4667,"billing_source":"wallet"}`,
		},
	}

	StripChannelCostFieldsFromLogs(logs)

	require.NotContains(t, logs[0].Other, "channel_ratio")
	require.NotContains(t, logs[0].Other, "cost_base_quota")
	require.NotContains(t, logs[0].Other, "cost_quota")
	require.NotContains(t, logs[0].Other, "profit_quota")
	require.NotContains(t, logs[0].Other, "profit_rate")
	require.Contains(t, logs[0].Other, "admin_info")
	require.Contains(t, logs[0].Other, "stream_status")
	require.Contains(t, logs[0].Other, "billing_source")
}

func TestGetUserWorkspaceUsageStatsAggregatesWorkspaceKeys(t *testing.T) {
	truncateTables(t)

	now := time.Date(2026, 6, 10, 15, 0, 0, 0, time.UTC)
	alpha := Workspace{Id: 301, UserId: 1001, Name: "Usage Alpha", Status: WorkspaceStatusEnabled}
	beta := Workspace{Id: 302, UserId: 1001, Name: "Usage Beta", Status: WorkspaceStatusEnabled}
	require.NoError(t, DB.Create(&[]Workspace{alpha, beta}).Error)
	require.NoError(t, DB.Create(&[]Token{
		{Id: 401, UserId: 1001, WorkspaceId: alpha.Id, Name: "alpha-main", Key: "alpha-main-key", UsedQuota: 1000},
		{Id: 402, UserId: 1001, WorkspaceId: alpha.Id, Name: "alpha-side", Key: "alpha-side-key", UsedQuota: 3000},
		{Id: 403, UserId: 1001, WorkspaceId: beta.Id, Name: "beta-main", Key: "beta-main-key", UsedQuota: 9000},
		{Id: 404, UserId: 1001, WorkspaceId: alpha.Id, Name: "alpha-subscription", Key: "alpha-sub-key", UsedQuota: 7000, BillingSource: "subscription"},
	}).Error)
	require.NoError(t, LOG_DB.Create(&[]Log{
		{Id: 61, UserId: 1001, Type: LogTypeConsume, CreatedAt: now.Add(-time.Hour).Unix(), TokenId: 401, TokenName: "alpha-main", Quota: 100},
		{Id: 62, UserId: 1001, Type: LogTypeConsume, CreatedAt: time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC).Unix(), TokenId: 402, TokenName: "alpha-side", Quota: 200},
		{Id: 63, UserId: 1001, Type: LogTypeConsume, CreatedAt: time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC).Unix(), TokenId: 401, TokenName: "alpha-main", Quota: 300},
		{Id: 64, UserId: 1001, Type: LogTypeConsume, CreatedAt: time.Date(2026, 5, 31, 12, 0, 0, 0, time.UTC).Unix(), TokenId: 401, TokenName: "alpha-main", Quota: 400},
		{Id: 65, UserId: 1001, Type: LogTypeConsume, CreatedAt: now.Add(-time.Hour).Unix(), TokenId: 403, TokenName: "beta-main", Quota: 900},
		{Id: 66, UserId: 1001, Type: LogTypeConsume, CreatedAt: now.Add(-time.Hour).Unix(), TokenId: 404, TokenName: "alpha-subscription", Quota: 800},
		{Id: 67, UserId: 1001, Type: LogTypeError, CreatedAt: now.Add(-time.Hour).Unix(), TokenId: 401, TokenName: "alpha-main", Quota: 500},
	}).Error)

	stats, err := GetUserWorkspaceUsageStats(1001, alpha.Id, now)

	require.NoError(t, err)
	require.Equal(t, alpha.Id, stats.WorkspaceId)
	require.Equal(t, 100, stats.TodayQuota)
	require.Equal(t, 300, stats.WeekQuota)
	require.Equal(t, 600, stats.MonthQuota)
	require.Equal(t, 4000, stats.TotalUsedQuota)
}

func TestAttachTokenTodayUsedQuotaAggregatesTodayConsumeLogs(t *testing.T) {
	truncateTables(t)

	now := time.Date(2026, 6, 10, 15, 0, 0, 0, time.UTC)
	tokens := []*Token{
		{Id: 431, UserId: 1001, Name: "today-main", Key: "today-main-key"},
		{Id: 432, UserId: 1001, Name: "today-side", Key: "today-side-key"},
	}
	require.NoError(t, DB.Create(tokens).Error)
	require.NoError(t, LOG_DB.Create(&[]Log{
		{Id: 81, UserId: 1001, Type: LogTypeConsume, CreatedAt: now.Add(-time.Hour).Unix(), TokenId: 431, TokenName: "today-main", Quota: 100},
		{Id: 82, UserId: 1001, Type: LogTypeConsume, CreatedAt: now.Add(-2 * time.Hour).Unix(), TokenId: 431, TokenName: "today-main", Quota: 250},
		{Id: 83, UserId: 1001, Type: LogTypeConsume, CreatedAt: now.Add(-time.Hour).Unix(), TokenId: 432, TokenName: "today-side", Quota: 900},
		{Id: 84, UserId: 1001, Type: LogTypeConsume, CreatedAt: time.Date(2026, 6, 9, 23, 59, 0, 0, time.UTC).Unix(), TokenId: 431, TokenName: "today-main", Quota: 500},
		{Id: 85, UserId: 1001, Type: LogTypeError, CreatedAt: now.Add(-time.Hour).Unix(), TokenId: 431, TokenName: "today-main", Quota: 700},
	}).Error)

	require.NoError(t, AttachTokenTodayUsedQuota(tokens, now))

	require.Equal(t, 350, tokens[0].TodayUsedQuota)
	require.Equal(t, 900, tokens[1].TodayUsedQuota)
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

	data, err := GetQuotaDatesFromLogs(0, 10000, "", "analytics-key", "Analytics Alpha", 0, nil)

	require.NoError(t, err)
	require.Len(t, data, 1)
	require.Equal(t, "gpt-alpha", data[0].ModelName)
	require.Equal(t, 150, data[0].Quota)
	require.Equal(t, 30, data[0].TokenUsed)
}

func TestGetQuotaDatesFromLogsKeepsSupplierGroupsSeparate(t *testing.T) {
	truncateTables(t)

	require.NoError(t, LOG_DB.Create(&[]Log{
		{Id: 91, UserId: 1001, Username: "alice", Type: LogTypeConsume, CreatedAt: 3610, ModelName: "gpt-test", Group: "default", Quota: 300},
		{Id: 92, UserId: 1001, Username: "alice", Type: LogTypeConsume, CreatedAt: 3620, ModelName: "gpt-test", Group: "vip", Quota: 500},
		{Id: 93, UserId: 1001, Username: "alice", Type: LogTypeRefund, CreatedAt: 3630, ModelName: "gpt-test", Group: "vip", Quota: 200},
	}).Error)

	data, err := GetQuotaDatesFromLogs(0, 10000, "alice", "", "", 0, nil)

	require.NoError(t, err)
	require.Len(t, data, 2)
	byGroup := make(map[string]*QuotaData, len(data))
	for _, item := range data {
		byGroup[item.UseGroup] = item
	}
	require.Equal(t, 300, byGroup["default"].Quota)
	require.Equal(t, 300, byGroup["vip"].Quota)
	require.Equal(t, 1, byGroup["vip"].Count)
}

func TestGetQuotaDatesFromLogsIncludesAndSeparatesCacheTokens(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.Create(&[]Channel{
		{Id: 301, Type: constant.ChannelTypeAnthropic},
		{Id: 302, Type: constant.ChannelTypeOpenRouter},
	}).Error)

	require.NoError(t, LOG_DB.Create(&[]Log{
		{
			Id:               53,
			UserId:           1001,
			Username:         "alice",
			Type:             LogTypeConsume,
			CreatedAt:        3600,
			ModelName:        "claude-test",
			ChannelId:        301,
			PromptTokens:     100,
			CompletionTokens: 20,
			Other:            `{"cache_tokens":30,"cache_write_tokens":10}`,
		},
		{
			Id:               54,
			UserId:           1001,
			Username:         "alice",
			Type:             LogTypeConsume,
			CreatedAt:        3600,
			ModelName:        "gpt-test",
			ChannelId:        302,
			PromptTokens:     100,
			CompletionTokens: 20,
			Other:            `{"cache_tokens":30}`,
		},
	}).Error)
	require.NoError(t, BackfillLogTokenMetrics())

	data, err := GetQuotaDatesFromLogs(0, 10000, "alice", "", "", 0, nil)

	require.NoError(t, err)
	require.Len(t, data, 2)
	byModel := make(map[string]*QuotaData, len(data))
	for _, item := range data {
		byModel[item.ModelName] = item
	}
	require.Equal(t, 160, byModel["claude-test"].TokenUsed)
	require.Equal(t, 30, byModel["claude-test"].CacheReadTokens)
	require.Equal(t, 10, byModel["claude-test"].CacheWriteTokens)
	require.Equal(t, 40, byModel["claude-test"].CacheTokenUsed)
	require.Equal(t, 120, byModel["gpt-test"].TokenUsed)
	require.Equal(t, 30, byModel["gpt-test"].CacheTokenUsed)
}

func TestBackfillLogTokenMetricsAfterSkipsCompletedWatermark(t *testing.T) {
	truncateTables(t)
	require.NoError(t, LOG_DB.Create(&[]Log{
		{Id: 53, Type: LogTypeConsume, PromptTokens: 100, Other: `{"cache_tokens":30}`},
		{Id: 54, Type: LogTypeConsume, PromptTokens: 200, Other: `{"cache_tokens":40}`},
	}).Error)

	watermark, err := BackfillLogTokenMetricsAfter(53)

	require.NoError(t, err)
	require.Equal(t, 54, watermark)
	var logs []Log
	require.NoError(t, LOG_DB.Order("id ASC").Find(&logs).Error)
	require.Zero(t, logs[0].InputTokens)
	require.Zero(t, logs[0].CacheReadTokens)
	require.Equal(t, 200, logs[1].InputTokens)
	require.Equal(t, 40, logs[1].CacheReadTokens)
}

func TestMigrateLogTokenMetricsPersistsAdvancedWatermark(t *testing.T) {
	truncateTables(t)
	common.OptionMapRWMutex.Lock()
	originalOptionMap := common.OptionMap
	common.OptionMap = map[string]string{LogTokenMetricsBackfillWatermarkOption: "61"}
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		common.OptionMap = originalOptionMap
		common.OptionMapRWMutex.Unlock()
	})
	require.NoError(t, DB.Create(&Option{Key: LogTokenMetricsBackfillWatermarkOption, Value: "61"}).Error)
	require.NoError(t, LOG_DB.Create(&[]Log{
		{Id: 61, Type: LogTypeConsume, PromptTokens: 100, Other: `{"cache_tokens":30}`},
		{Id: 62, Type: LogTypeConsume, PromptTokens: 200, Other: `{"cache_tokens":40}`},
	}).Error)

	require.NoError(t, migrateLogTokenMetrics())

	var option Option
	require.NoError(t, DB.First(&option, "key = ?", LogTokenMetricsBackfillWatermarkOption).Error)
	require.Equal(t, "62", option.Value)
	common.OptionMapRWMutex.RLock()
	require.Equal(t, "62", common.OptionMap[LogTokenMetricsBackfillWatermarkOption])
	common.OptionMapRWMutex.RUnlock()
}

func TestGetQuotaDatesFromLogsSubtractsTaskRefunds(t *testing.T) {
	truncateTables(t)

	require.NoError(t, LOG_DB.Create(&[]Log{
		{
			Id:        61,
			UserId:    1001,
			Username:  "alice",
			Type:      LogTypeConsume,
			CreatedAt: 3610,
			TokenId:   221,
			TokenName: "seedance-key",
			ModelName: "seedance-2.0",
			Quota:     1000,
		},
		{
			Id:               62,
			UserId:           1001,
			Username:         "alice",
			Type:             LogTypeRefund,
			CreatedAt:        3650,
			TokenId:          221,
			TokenName:        "seedance-key",
			ModelName:        "seedance-2.0",
			Quota:            700,
			PromptTokens:     10,
			CompletionTokens: 20,
			Other:            `{"pre_consumed_quota":1000,"actual_quota":300}`,
		},
	}).Error)

	data, err := GetQuotaDatesFromLogs(0, 10000, "alice", "", "", 0, nil)

	require.NoError(t, err)
	require.Len(t, data, 1)
	require.Equal(t, "seedance-2.0", data[0].ModelName)
	require.Equal(t, int64(3600), data[0].CreatedAt)
	require.Equal(t, 1, data[0].Count)
	require.Equal(t, 300, data[0].Quota)
	require.Equal(t, 30, data[0].TokenUsed)
}

func TestGetQuotaDatesFromLogsDoesNotCountTaskSupplementTwice(t *testing.T) {
	truncateTables(t)

	require.NoError(t, LOG_DB.Create(&[]Log{
		{Id: 63, UserId: 1001, Username: "alice", Type: LogTypeConsume, CreatedAt: 3610, ModelName: "seedance-2.0", Quota: 1000, Other: `{"is_task":true}`},
		{Id: 64, UserId: 1001, Username: "alice", Type: LogTypeConsume, CreatedAt: 3650, ModelName: "seedance-2.0", Quota: 200, PromptTokens: 10, CompletionTokens: 20, Content: "tiered_expr", Other: `{"pre_consumed_quota":1000,"actual_quota":1200}`},
		{Id: 65, UserId: 1001, Username: "alice", Type: LogTypeConsume, CreatedAt: 3660, ModelName: "gpt-image", Quota: 500, PromptTokens: 30, CompletionTokens: 40, Content: "image usage", Other: `{"pre_consumed_quota":100,"actual_quota":500}`},
	}).Error)

	data, err := GetQuotaDatesFromLogs(0, 10000, "alice", "", "", 0, nil)

	require.NoError(t, err)
	require.Len(t, data, 2)
	byModel := make(map[string]*QuotaData)
	for _, item := range data {
		byModel[item.ModelName] = item
	}
	require.Equal(t, 1, byModel["seedance-2.0"].Count)
	require.Equal(t, 1200, byModel["seedance-2.0"].Quota)
	require.Equal(t, 30, byModel["seedance-2.0"].TokenUsed)
	require.Equal(t, 1, byModel["gpt-image"].Count)
	require.Equal(t, 500, byModel["gpt-image"].Quota)
	require.Equal(t, 70, byModel["gpt-image"].TokenUsed)
}

func TestGetUsageDimensionTrendsFromLogsAggregatesTokensAndEnrichesWorkspaces(t *testing.T) {
	truncateTables(t)

	alpha := Workspace{Id: 123, UserId: 1001, Name: "Dimension Alpha", Status: WorkspaceStatusEnabled}
	beta := Workspace{Id: 124, UserId: 1001, Name: "Dimension Beta", Status: WorkspaceStatusEnabled}
	require.NoError(t, DB.Create(&[]Workspace{alpha, beta}).Error)
	require.NoError(t, DB.Create(&[]Token{
		{Id: 223, UserId: 1001, WorkspaceId: alpha.Id, Name: "alpha-dimension-key", Key: "alpha-dimension-key-value"},
		{Id: 224, UserId: 1001, WorkspaceId: beta.Id, Name: "beta-dimension-key", Key: "beta-dimension-key-value"},
	}).Error)
	require.NoError(t, LOG_DB.Create(&[]Log{
		{Id: 71, UserId: 1001, Username: "alice", Type: LogTypeConsume, CreatedAt: 3600, TokenId: 223, TokenName: "alpha-dimension-key", Quota: 150, PromptTokens: 10, CompletionTokens: 20},
		{Id: 72, UserId: 1001, Username: "alice", Type: LogTypeConsume, CreatedAt: 3600, TokenId: 223, TokenName: "alpha-dimension-key", Quota: 250, PromptTokens: 30, CompletionTokens: 40},
		{Id: 73, UserId: 1001, Username: "alice", Type: LogTypeConsume, CreatedAt: 7200, TokenId: 224, TokenName: "beta-dimension-key", Quota: 450, PromptTokens: 50, CompletionTokens: 60},
		{Id: 74, UserId: 1001, Username: "alice", Type: LogTypeError, CreatedAt: 3600, TokenId: 223, TokenName: "alpha-dimension-key", Quota: 900},
	}).Error)

	data, err := GetUsageDimensionTrendsFromLogs(0, 10000, "", "", "", 0, nil)

	require.NoError(t, err)
	require.Len(t, data, 2)

	byTokenAndTime := make(map[string]*UsageDimensionTrendData)
	for _, item := range data {
		byTokenAndTime[item.TokenName+"-"+strconv.FormatInt(item.CreatedAt, 10)] = item
	}
	alphaItem := byTokenAndTime["alpha-dimension-key-3600"]
	require.NotNil(t, alphaItem)
	require.Equal(t, alpha.Id, alphaItem.WorkspaceId)
	require.Equal(t, alpha.Name, alphaItem.WorkspaceName)
	require.Equal(t, 2, alphaItem.Count)
	require.Equal(t, 400, alphaItem.Quota)
	require.Equal(t, 100, alphaItem.TokenUsed)

	betaItem := byTokenAndTime["beta-dimension-key-7200"]
	require.NotNil(t, betaItem)
	require.Equal(t, beta.Id, betaItem.WorkspaceId)
	require.Equal(t, beta.Name, betaItem.WorkspaceName)
	require.Equal(t, 1, betaItem.Count)
	require.Equal(t, 450, betaItem.Quota)
	require.Equal(t, 110, betaItem.TokenUsed)
}

func TestGetUsageDimensionTrendsFromLogsSubtractsRefundQuota(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.Create(&Token{Id: 225, UserId: 1001, Name: "seedance-key", Key: "seedance-key-value"}).Error)
	require.NoError(t, LOG_DB.Create(&[]Log{
		{Id: 75, UserId: 1001, Username: "alice", Type: LogTypeConsume, CreatedAt: 3610, TokenId: 225, TokenName: "seedance-key", Quota: 1000},
		{Id: 76, UserId: 1001, Username: "alice", Type: LogTypeRefund, CreatedAt: 3650, TokenId: 225, TokenName: "seedance-key", Quota: 700, PromptTokens: 10, CompletionTokens: 20},
	}).Error)

	data, err := GetUsageDimensionTrendsFromLogs(0, 10000, "alice", "", "", 0, nil)

	require.NoError(t, err)
	require.Len(t, data, 1)
	require.Equal(t, int64(3600), data[0].CreatedAt)
	require.Equal(t, 1, data[0].Count)
	require.Equal(t, 300, data[0].Quota)
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

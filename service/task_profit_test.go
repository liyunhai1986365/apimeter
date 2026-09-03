package service

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func seedDeferredProfitTask(t *testing.T) *model.Task {
	t.Helper()
	seedUser(t, 1, 10000)
	seedToken(t, 1, 1, "task-profit-key", 10000)
	seedChannel(t, 1)
	require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", 1).Update("channel_ratio", 0.8).Error)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/videos", nil)
	c.Set("username", "test_user")
	c.Set("token_name", "test_token")
	info := &relaycommon.RelayInfo{
		UserId: 1, TokenId: 1, OriginModelName: "test-model", UsingGroup: "default",
		ChannelMeta:   &relaycommon.ChannelMeta{ChannelId: 1, ChannelRatio: 0.8},
		TaskRelayInfo: &relaycommon.TaskRelayInfo{Action: "generate"},
		PriceData:     types.PriceData{Quota: 1500, GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 1.5}},
	}
	LogTaskConsumption(c, info)
	seedChargedAccounting(t, 1, 1, 1, 1500, 1)
	task := makeTask(1, 1, 1500, 1, BillingSourceWallet, 0)
	task.PrivateData.BillingContext.GroupRatio = 1.5
	task.PrivateData.BillingContext.DeferredCost = DeferTaskCost(info)
	require.NoError(t, model.DB.Create(task).Error)
	return task
}

func TestDeferredTaskProfitUsesActualChargeAndSettlementChannelCost(t *testing.T) {
	for _, tc := range []struct {
		name   string
		actual int
		failed bool
	}{
		{name: "partial refund", actual: 900},
		{name: "supplement", actual: 2100},
		{name: "exact preconsume", actual: 1500},
		{name: "zero actual", actual: 0},
		{name: "failure refund", actual: 0, failed: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			truncate(t)
			task := seedDeferredProfitTask(t)
			pending, err := model.SumUsedQuota(model.LogTypeUnknown, 0, 0, "", "", "", 0, "", "", nil)
			require.NoError(t, err)
			require.Equal(t, 1500, pending.Quota)
			require.Zero(t, pending.CostQuota)
			require.Zero(t, pending.ProfitQuota)
			summary, err := model.SumModelProfitStats(model.LogTypeUnknown, 0, 0, "", "", "", 0, "")
			require.NoError(t, err)
			require.Empty(t, summary.Items)

			// Pricing changes between submission and settlement must use the final
			// channel cost, while the customer's original group ratio stays frozen.
			require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", 1).Update("channel_ratio", 0.6).Error)
			if tc.failed {
				require.True(t, RefundTaskQuota(context.Background(), task, "upstream failure"))
			} else {
				RecalculateTaskQuota(context.Background(), task, tc.actual, "actual usage")
			}
			actualBase := tc.actual * 2 / 3
			actualCost := actualBase * 6 / 10
			actualProfit := tc.actual - actualCost
			log := getLastLog(t)
			require.NotNil(t, log)
			other, err := common.StrToMap(log.Other)
			require.NoError(t, err)
			require.Equal(t, model.TaskCostSettled, other["task_cost_state"])
			require.Equal(t, float64(actualCost), other["cost_quota"])
			require.Equal(t, float64(actualProfit), other["profit_quota"])
			require.Equal(t, tc.actual, task.Quota)
			require.Equal(t, 10000+1500-tc.actual, getUserQuota(t, 1))
			require.Equal(t, 10000+1500-tc.actual, getTokenRemainQuota(t, 1))
			require.Equal(t, tc.actual, getTokenUsedQuota(t, 1))
			used, requests := getUserUsageAccounting(t, 1)
			require.Equal(t, tc.actual, used)
			require.Equal(t, 1, requests)
			require.Equal(t, int64(tc.actual), getChannelUsedQuota(t, 1))
			stat, err := model.SumUsedQuota(model.LogTypeUnknown, 0, 0, "", "", "", 0, "", "", nil)
			require.NoError(t, err)
			require.Equal(t, tc.actual, stat.Quota)
			require.Equal(t, actualCost, stat.CostQuota)
			require.Equal(t, actualProfit, stat.ProfitQuota)
			require.Equal(t, 1, stat.Rpm)
			summary, err = model.SumModelProfitStats(model.LogTypeUnknown, 0, 0, "", "", "", 0, "")
			require.NoError(t, err)
			require.Equal(t, tc.actual, summary.Quota)
			require.Equal(t, actualCost, summary.CostQuota)
			require.Equal(t, actualProfit, summary.ProfitQuota)
			require.Len(t, summary.Items, 1)
			require.Equal(t, 1, summary.Items[0].RequestCount)

			// Reload as another polling pass would, then ensure no duplicate money
			// or profit is booked and later channel edits cannot rewrite history.
			var reloaded model.Task
			require.NoError(t, model.DB.First(&reloaded, task.ID).Error)
			require.True(t, reloaded.PrivateData.BillingContext.CostSettled)
			require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", 1).Update("channel_ratio", 0.9).Error)
			if tc.failed {
				require.True(t, RefundTaskQuota(context.Background(), &reloaded, "duplicate failure"))
			} else {
				RecalculateTaskQuota(context.Background(), &reloaded, tc.actual, "duplicate completion")
			}
			require.Equal(t, int64(2), countLogs(t))
			rechecked, err := model.SumModelProfitStats(model.LogTypeUnknown, 0, 0, "", "", "", 0, "")
			require.NoError(t, err)
			require.Equal(t, summary, rechecked)
		})
	}
}

func TestDeferredTaskProfitCompletesWithoutUsageAdjustment(t *testing.T) {
	truncate(t)
	task := seedDeferredProfitTask(t)
	SettleTaskBillingOnComplete(context.Background(), &mockAdaptor{}, task, &relaycommon.TaskInfo{Status: model.TaskStatusSuccess})
	require.True(t, task.PrivateData.BillingContext.CostSettled)
	require.Equal(t, 1500, task.Quota)
	require.Equal(t, 10000, getUserQuota(t, 1))
	require.Equal(t, int64(2), countLogs(t))
	require.Zero(t, getLastLog(t).Quota)
}

func TestDeferredTaskProfitDoesNotInventMissingChannelCost(t *testing.T) {
	truncate(t)
	task := seedDeferredProfitTask(t)
	require.NoError(t, model.DB.Delete(&model.Channel{}, 1).Error)
	RecalculateTaskQuota(context.Background(), task, 900, "actual usage")
	require.Equal(t, 10600, getUserQuota(t, 1))
	other, err := common.StrToMap(getLastLog(t).Other)
	require.NoError(t, err)
	require.Equal(t, model.TaskCostUnavailable, other["task_cost_state"])
	summary, err := model.SumModelProfitStats(model.LogTypeUnknown, 0, 0, "", "", "", 0, "")
	require.NoError(t, err)
	require.Zero(t, summary.ProfitQuota)
}

package model

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestModelProfitRecognizesTaskSettlementInRefundPeriod(t *testing.T) {
	truncateTables(t)
	require.NoError(t, LOG_DB.Create(&[]Log{
		{Type: LogTypeConsume, CreatedAt: 100, ModelName: "video", Quota: 1500, Other: `{"task_cost_state":"pending","cost_quota":1000,"profit_quota":500}`},
		{Type: LogTypeRefund, CreatedAt: 200, ModelName: "video", Quota: 1000, Other: `{"task_cost_state":"settled","pre_consumed_quota":1500,"actual_quota":500,"cost_base_quota":500,"cost_quota":200,"profit_quota":300}`},
		{Type: LogTypeConsume, CreatedAt: 150, ModelName: "ordinary", Quota: 1000, Other: `{"cost_quota":600,"profit_quota":400}`},
		{Type: LogTypeRefund, CreatedAt: 160, ModelName: "ordinary", Quota: 200, Other: `{"cost_quota":120,"profit_quota":80}`},
	}).Error)

	pending, err := SumModelProfitStats(LogTypeUnknown, 100, 100, "", "", "", 0, "")
	require.NoError(t, err)
	require.Empty(t, pending.Items)
	settled, err := SumModelProfitStats(LogTypeUnknown, 200, 200, "", "", "", 0, "")
	require.NoError(t, err)
	require.Equal(t, 500, settled.Quota)
	require.Equal(t, 200, settled.CostQuota)
	require.Equal(t, 300, settled.ProfitQuota)
	require.Equal(t, 1, settled.Items[0].RequestCount)

	all, err := SumModelProfitStats(LogTypeUnknown, 100, 200, "", "", "", 0, "")
	require.NoError(t, err)
	require.Equal(t, 1300, all.Quota)
	require.Equal(t, 680, all.CostQuota)
	require.Equal(t, 620, all.ProfitQuota)
	require.Len(t, all.Items, 2)
	refunds, err := SumModelProfitStats(LogTypeRefund, 200, 200, "video", "", "", 0, "")
	require.NoError(t, err)
	require.Equal(t, settled, refunds)
	consume, err := SumModelProfitStats(LogTypeConsume, 100, 200, "video", "", "", 0, "")
	require.NoError(t, err)
	require.Empty(t, consume.Items)

	// Balance statistics retain the signed refund delta while cost statistics
	// recognize the full realized task profit during this same period.
	stat, err := SumUsedQuota(LogTypeUnknown, 200, 200, "video", "", "", 0, "", "", nil)
	require.NoError(t, err)
	require.Equal(t, -1000, stat.Quota)
	require.Equal(t, 200, stat.CostQuota)
	require.Equal(t, 300, stat.ProfitQuota)
}

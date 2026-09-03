package controller

import (
	"bytes"
	"encoding/csv"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestWriteBillingBreakdownCSVExportsSupplierDiscountRatioAndAmountsAsUSD(t *testing.T) {
	originalQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 500000
	t.Cleanup(func() {
		common.QuotaPerUnit = originalQuotaPerUnit
	})

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)

	writeBillingBreakdownCSV(ctx, []model.BillingBreakdownRow{
		{
			PeriodValue:      "2026-05",
			ModelName:        "gpt-test",
			Group:            "vip",
			GroupRatio:       0.53,
			RequestCount:     1,
			InputTokens:      1000,
			OutputTokens:     2000,
			CacheReadTokens:  300,
			CacheWriteTokens: 400,
			OriginalAmount:   500000,
			DiscountAmount:   250000,
			SettlementAmount: 1000,
		},
	}, "billing.csv")

	body := bytes.TrimPrefix(recorder.Body.Bytes(), []byte("\xEF\xBB\xBF"))
	records, err := csv.NewReader(bytes.NewReader(body)).ReadAll()
	require.NoError(t, err)
	require.Len(t, records, 2)
	require.Equal(t, []string{
		"账期",
		"模型",
		"供应商",
		"请求数",
		"输入 Tokens",
		"输出 Tokens",
		"缓存读取 Tokens",
		"缓存写入 Tokens",
		"原价(USD)",
		"折扣",
		"结算金额(USD)",
	}, records[0])
	require.Equal(t, "1.000000", records[1][8])
	require.Equal(t, "-47%", records[1][9])
	require.Equal(t, "0.002000", records[1][10])
}

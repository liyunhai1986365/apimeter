package service

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

func TestGoogleSearchPricesAndSettlement(t *testing.T) {
	for _, tc := range []struct {
		model   string
		queries int
		dollars string
	}{
		{"gemini-3-flash-preview", 3, "0.042"},
		{"gemini-4-argon-pro", 3, "0.042"},
		{"gemini-2.5-pro", 3, "0.035"},
		{"gemini-2.0-flash", 3, "0.035"},
		{"gemini-2.5-pro", 0, "0"},
	} {
		t.Run(tc.model+tc.dollars, func(t *testing.T) {
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			info := &relaycommon.RelayInfo{}
			info.SetBillableToolCount(dto.BuildInToolGoogleSearch, tc.queries)
			if tc.queries > 0 {
				info.SetBillableToolCount(relaycommon.GoogleSearchGroundedPromptTool, 1)
			}
			// The removed legacy marker must not add a second charge.
			ctx.Set("gemini_google_search_call", true)
			summary := textQuotaSummary{ModelName: tc.model, GroupRatio: 0.8}
			quota := calculateTextToolCallSurcharge(ctx, info, &summary)
			want := decimal.RequireFromString(tc.dollars).Mul(decimal.NewFromFloat(common.QuotaPerUnit)).Mul(decimal.NewFromFloat(.8))
			require.True(t, want.Equal(quota), "want %s, got %s", want, quota)
		})
	}
	key := "google_search:gemini-3-test-override*"
	operation_setting.SetToolPriceForTest(key, 2)
	t.Cleanup(func() { operation_setting.DeleteToolPriceForTest(key) })
	require.Equal(t, 2.0, operation_setting.GetToolPriceForModel(dto.BuildInToolGoogleSearch, "gemini-3-test-override"))
}

func TestGoogleSearchLegacyPriceKeepsPerRequestUnit(t *testing.T) {
	for _, price := range []float64{0, 10} {
		for _, name := range []string{"gemini-2.5-review-legacy", "gemini-3-review-legacy"} {
			t.Run(name+decimal.NewFromFloat(price).String(), func(t *testing.T) {
				key := "google_search:" + name + "*"
				operation_setting.SetToolPriceForTest(key, price)
				t.Cleanup(func() { operation_setting.DeleteToolPriceForTest(key) })
				ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
				info := &relaycommon.RelayInfo{}
				info.SetBillableToolCount(dto.BuildInToolGoogleSearch, 3)
				info.SetBillableToolCount(relaycommon.GoogleSearchGroundedPromptTool, 1)
				summary := textQuotaSummary{ModelName: name, GroupRatio: 0.8}
				got := calculateTextToolCallSurcharge(ctx, info, &summary)
				want := decimal.NewFromFloat(price / 1000).Mul(decimal.NewFromFloat(common.QuotaPerUnit)).Mul(decimal.NewFromFloat(0.8))
				require.True(t, want.Equal(got), "want %s got %s", want, got)
			})
		}
	}
}

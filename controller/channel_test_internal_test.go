package controller

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSettleTestQuotaUsesTieredBilling(t *testing.T) {
	info := &relaycommon.RelayInfo{
		TieredBillingSnapshot: &billingexpr.BillingSnapshot{
			BillingMode:   "tiered_expr",
			ExprString:    `param("stream") == true ? tier("stream", p * 3) : tier("base", p * 2)`,
			ExprHash:      billingexpr.ExprHashString(`param("stream") == true ? tier("stream", p * 3) : tier("base", p * 2)`),
			GroupRatio:    1,
			EstimatedTier: "stream",
			QuotaPerUnit:  common.QuotaPerUnit,
			ExprVersion:   1,
		},
		BillingRequestInput: &billingexpr.RequestInput{
			Body: []byte(`{"stream":true}`),
		},
	}

	quota, result := settleTestQuota(info, types.PriceData{
		ModelRatio:      1,
		CompletionRatio: 2,
	}, &dto.Usage{
		PromptTokens: 1000,
	})

	require.Equal(t, 1500, quota)
	require.NotNil(t, result)
	require.Equal(t, "stream", result.MatchedTier)
}

func TestBuildTestLogOtherInjectsTieredInfo(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())

	info := &relaycommon.RelayInfo{
		TieredBillingSnapshot: &billingexpr.BillingSnapshot{
			BillingMode: "tiered_expr",
			ExprString:  `tier("base", p * 2)`,
		},
		ChannelMeta: &relaycommon.ChannelMeta{},
	}
	priceData := types.PriceData{
		GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 1},
	}
	usage := &dto.Usage{
		PromptTokensDetails: dto.InputTokenDetails{
			CachedTokens: 12,
		},
	}

	other := buildTestLogOther(ctx, info, priceData, usage, &billingexpr.TieredResult{
		MatchedTier: "base",
	})

	require.Equal(t, "tiered_expr", other["billing_mode"])
	require.Equal(t, "base", other["matched_tier"])
	require.NotEmpty(t, other["expr_b64"])
}

func TestAutoDisableRulesSkipChannelsWithoutAutoBan(t *testing.T) {
	setupChannelControllerBatchTestDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Log{}, &model.ChannelOperationRecord{}))

	originSetting := *operation_setting.GetMonitorSetting()
	t.Cleanup(func() {
		*operation_setting.GetMonitorSetting() = originSetting
	})
	setting := operation_setting.GetMonitorSetting()
	setting.ChannelAutoOperationEnabled = true
	setting.ChannelAutoDisableRules = []operation_setting.ChannelAutoDisableRule{
		{
			ID:                  "errors",
			Name:                "Errors",
			Enabled:             true,
			WindowMinutes:       10,
			ErrorCountThreshold: 1,
			ProtectLast:         false,
		},
	}

	autoBanDisabled := 0
	channel := &model.Channel{
		Id:      11,
		Name:    "auto-ban-off",
		Type:    1,
		Status:  common.ChannelStatusEnabled,
		Models:  "gpt-test",
		Group:   "default",
		AutoBan: &autoBanDisabled,
	}
	require.NoError(t, model.DB.Create(channel).Error)
	require.NoError(t, model.DB.Create(&model.Log{
		CreatedAt: time.Now().Unix() - 10,
		Type:      model.LogTypeError,
		ModelName: "gpt-test",
		ChannelId: channel.Id,
		Content:   "upstream failed",
	}).Error)

	disabled, err := disableChannelByAutoOperationIfNeeded(channel)
	require.NoError(t, err)
	require.False(t, disabled)

	var updated model.Channel
	require.NoError(t, model.DB.First(&updated, channel.Id).Error)
	require.Equal(t, common.ChannelStatusEnabled, updated.Status)

	var operationCount int64
	require.NoError(t, model.DB.Model(&model.ChannelOperationRecord{}).Count(&operationCount).Error)
	require.Zero(t, operationCount)
}

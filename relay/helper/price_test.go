package helper

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestModelPriceHelperTieredUsesPreloadedRequestInput(t *testing.T) {
	gin.SetMode(gin.TestMode)

	saved := map[string]string{}
	require.NoError(t, config.GlobalConfig.SaveToDB(func(key, value string) error {
		saved[key] = value
		return nil
	}))
	t.Cleanup(func() {
		require.NoError(t, config.GlobalConfig.LoadFromDB(saved))
	})

	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"billing_setting.billing_mode": `{"tiered-test-model":"tiered_expr"}`,
		"billing_setting.billing_expr": `{"tiered-test-model":"param(\"stream\") == true ? tier(\"stream\", p * 3) : tier(\"base\", p * 2)"}`,
	}))

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodPost, "/api/channel/test/1", nil)
	req.Body = nil
	req.ContentLength = 0
	req.Header.Set("Content-Type", "application/json")
	ctx.Request = req
	ctx.Set("group", "default")

	info := &relaycommon.RelayInfo{
		OriginModelName: "tiered-test-model",
		UserGroup:       "default",
		UsingGroup:      "default",
		RequestHeaders:  map[string]string{"Content-Type": "application/json"},
		BillingRequestInput: &billingexpr.RequestInput{
			Headers: map[string]string{"Content-Type": "application/json"},
			Body:    []byte(`{"stream":true}`),
		},
	}

	priceData, err := ModelPriceHelper(ctx, info, 1000, &types.TokenCountMeta{})
	require.NoError(t, err)
	require.Equal(t, 1500, priceData.QuotaToPreConsume)
	require.NotNil(t, info.TieredBillingSnapshot)
	require.Equal(t, "stream", info.TieredBillingSnapshot.EstimatedTier)
	require.Equal(t, billing_setting.BillingModeTieredExpr, info.TieredBillingSnapshot.BillingMode)
	require.Equal(t, common.QuotaPerUnit, info.TieredBillingSnapshot.QuotaPerUnit)
}

func TestModelPriceHelperTieredCanPriceSeedanceRequestShape(t *testing.T) {
	gin.SetMode(gin.TestMode)

	saved := map[string]string{}
	require.NoError(t, config.GlobalConfig.SaveToDB(func(key, value string) error {
		saved[key] = value
		return nil
	}))
	t.Cleanup(func() {
		require.NoError(t, config.GlobalConfig.LoadFromDB(saved))
	})

	expr := `param("resolution") == "1080p" && param("content.#(type==\"video_url\").video_url.url") != nil ? tier("1080p_video", c * 4.3055555556) : tier("base", c * 6.3888888889)`
	exprJSON, err := common.Marshal(expr)
	require.NoError(t, err)
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"billing_setting.billing_mode": `{"doubao-seedance-2.0":"tiered_expr"}`,
		"billing_setting.billing_expr": `{"doubao-seedance-2.0":` + string(exprJSON) + `}`,
	}))

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodPost, "/api/v3/contents/generations/tasks", nil)
	req.Body = nil
	req.ContentLength = 0
	req.Header.Set("Content-Type", "application/json")
	ctx.Request = req
	ctx.Set("group", "default")

	info := &relaycommon.RelayInfo{
		OriginModelName: "doubao-seedance-2.0",
		UserGroup:       "default",
		UsingGroup:      "default",
		RequestHeaders:  map[string]string{"Content-Type": "application/json"},
		BillingRequestInput: &billingexpr.RequestInput{
			Headers: map[string]string{"Content-Type": "application/json"},
			Body:    []byte(`{"resolution":"1080p","content":[{"type":"video_url","video_url":{"url":"https://example.com/input.mp4"}}]}`),
		},
	}

	priceData, err := ModelPriceHelper(ctx, info, 0, &types.TokenCountMeta{MaxTokens: 1000})
	require.NoError(t, err)
	require.Equal(t, int(0.5*common.QuotaPerUnit), priceData.QuotaToPreConsume)
	require.NotNil(t, info.TieredBillingSnapshot)
	require.Equal(t, "1080p_video", info.TieredBillingSnapshot.EstimatedTier)
	require.Equal(t, 2153, info.TieredBillingSnapshot.EstimatedQuotaAfterGroup)
}

func TestModelPriceHelperTieredKeepsEstimatedPreConsumeForNonSeedanceTask(t *testing.T) {
	gin.SetMode(gin.TestMode)

	saved := map[string]string{}
	require.NoError(t, config.GlobalConfig.SaveToDB(func(key, value string) error {
		saved[key] = value
		return nil
	}))
	t.Cleanup(func() {
		require.NoError(t, config.GlobalConfig.LoadFromDB(saved))
	})

	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"billing_setting.billing_mode": `{"other-video-model":"tiered_expr"}`,
		"billing_setting.billing_expr": `{"other-video-model":"tier(\"base\", c * 6.3888888889)"}`,
	}))

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodPost, "/api/v3/contents/generations/tasks", nil)
	req.Body = nil
	req.ContentLength = 0
	req.Header.Set("Content-Type", "application/json")
	ctx.Request = req
	ctx.Set("group", "default")

	info := &relaycommon.RelayInfo{
		OriginModelName: "other-video-model",
		UserGroup:       "default",
		UsingGroup:      "default",
		RequestHeaders:  map[string]string{"Content-Type": "application/json"},
		BillingRequestInput: &billingexpr.RequestInput{
			Headers: map[string]string{"Content-Type": "application/json"},
			Body:    []byte(`{"resolution":"720p"}`),
		},
	}

	priceData, err := ModelPriceHelper(ctx, info, 0, &types.TokenCountMeta{MaxTokens: 1000})
	require.NoError(t, err)
	require.Equal(t, 3194, priceData.QuotaToPreConsume)
	require.NotNil(t, info.TieredBillingSnapshot)
	require.Equal(t, 3194, info.TieredBillingSnapshot.EstimatedQuotaAfterGroup)
}

func TestHandleGroupRatioUsesAgentConfiguredRatio(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"vip":1,"svip":1}`))
	})
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"vip":1.25}`))

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	info := &relaycommon.RelayInfo{
		UserGroup:  "default",
		UsingGroup: "vip",
		AgentContext: &types.AgentContext{
			AgentID: 3,
			Domain:  "agent.example.com",
			GroupRatios: map[string]float64{
				"vip": 1.5,
			},
		},
	}

	groupRatioInfo := HandleGroupRatio(ctx, info)
	require.Equal(t, 1.5, groupRatioInfo.GroupRatio)
	require.Equal(t, 1.25, groupRatioInfo.BaseGroupRatio)
	require.True(t, groupRatioInfo.HasAgentRatio)
	require.NotNil(t, info.AgentBillingSnapshot)
	require.Equal(t, "vip", info.AgentBillingSnapshot.Group)
	require.Equal(t, 1.25, info.AgentBillingSnapshot.BaseGroupRatio)
	require.Equal(t, 1.5, info.AgentBillingSnapshot.ChargedGroupRatio)
}

func TestHandleGroupRatioUsesMappedAgentProxyGroup(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"vip":1,"svip":1}`))
	})
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"vip":1.25}`))

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	info := &relaycommon.RelayInfo{
		UserGroup:  "agent-vip",
		UsingGroup: "agent-vip",
		TokenGroup: "agent-vip",
		AgentContext: &types.AgentContext{
			AgentID: 3,
			Domain:  "agent.example.com",
			Groups: map[string]types.AgentGroup{
				"agent-vip": {
					GroupName:       "agent-vip",
					SystemGroupName: "vip",
					EffectiveRatio:  1.5,
					Visible:         true,
					Available:       true,
				},
			},
			GroupRatios: map[string]float64{
				"agent-vip": 1.5,
			},
		},
	}

	groupRatioInfo := HandleGroupRatio(ctx, info)
	require.Equal(t, 1.5, groupRatioInfo.GroupRatio)
	require.Equal(t, 1.25, groupRatioInfo.BaseGroupRatio)
	require.True(t, groupRatioInfo.HasAgentRatio)
	require.Equal(t, "vip", info.UsingGroup)
	require.NotNil(t, info.AgentBillingSnapshot)
	require.Equal(t, "agent-vip", info.AgentBillingSnapshot.Group)
	require.Equal(t, 1.25, info.AgentBillingSnapshot.BaseGroupRatio)
	require.Equal(t, 1.5, info.AgentBillingSnapshot.ChargedGroupRatio)
}

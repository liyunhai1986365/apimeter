package helper

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
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

func TestModelPriceHelperTieredUsesSeedancePerSecondBaseForOpenAIVideoGenerations(t *testing.T) {
	gin.SetMode(gin.TestMode)

	saved := map[string]string{}
	require.NoError(t, config.GlobalConfig.SaveToDB(func(key, value string) error {
		saved[key] = value
		return nil
	}))
	t.Cleanup(func() {
		require.NoError(t, config.GlobalConfig.LoadFromDB(saved))
	})

	expr := `tier("expensive", c * 7.7)`
	exprJSON, err := common.Marshal(expr)
	require.NoError(t, err)
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"billing_setting.billing_mode": `{"dreamina-seedance-2-0-ep":"tiered_expr"}`,
		"billing_setting.billing_expr": `{"dreamina-seedance-2-0-ep":` + string(exprJSON) + `}`,
	}))

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodPost, "/v1/video/generations", nil)
	req.Body = nil
	req.ContentLength = 0
	req.Header.Set("Content-Type", "application/json")
	ctx.Request = req
	ctx.Set("group", "default")

	info := &relaycommon.RelayInfo{
		OriginModelName: "dreamina-seedance-2-0-ep",
		UserGroup:       "default",
		UsingGroup:      "default",
		RequestHeaders:  map[string]string{"Content-Type": "application/json"},
		BillingRequestInput: &billingexpr.RequestInput{
			Headers: map[string]string{"Content-Type": "application/json"},
			Body:    []byte(`{"model":"dreamina-seedance-2-0-ep","duration":10}`),
		},
	}

	priceData, err := ModelPriceHelper(ctx, info, 0, &types.TokenCountMeta{MaxTokens: 250000})
	require.NoError(t, err)
	require.Equal(t, int(0.5*common.QuotaPerUnit), priceData.QuotaToPreConsume)
	require.NotNil(t, info.TieredBillingSnapshot)
	require.Equal(t, "expensive", info.TieredBillingSnapshot.EstimatedTier)
	require.Equal(t, 962500, info.TieredBillingSnapshot.EstimatedQuotaAfterGroup)
}

func TestModelPriceHelperTieredUsesSeedancePerSecondBaseForV1Video(t *testing.T) {
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
		"billing_setting.billing_mode": `{"dreamina-seedance-2-0-fast-ep":"tiered_expr"}`,
		"billing_setting.billing_expr": `{"dreamina-seedance-2-0-fast-ep":"tier(\"base\", c * 5.6)"}`,
	}))

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodPost, "/v1/video", nil)
	req.Body = nil
	req.ContentLength = 0
	req.Header.Set("Content-Type", "application/json")
	ctx.Request = req
	ctx.Set("group", "default")

	info := &relaycommon.RelayInfo{
		OriginModelName: "dreamina-seedance-2-0-fast-ep",
		UserGroup:       "default",
		UsingGroup:      "default",
		RequestHeaders:  map[string]string{"Content-Type": "application/json"},
		BillingRequestInput: &billingexpr.RequestInput{
			Headers: map[string]string{"Content-Type": "application/json"},
			Body:    []byte(`{"model":"dreamina-seedance-2-0-fast-ep","duration":8}`),
		},
	}

	priceData, err := ModelPriceHelper(ctx, info, 0, &types.TokenCountMeta{MaxTokens: 250000})
	require.NoError(t, err)
	require.Equal(t, int(0.5*common.QuotaPerUnit), priceData.QuotaToPreConsume)
	require.NotNil(t, info.TieredBillingSnapshot)
	require.Equal(t, 700000, info.TieredBillingSnapshot.EstimatedQuotaAfterGroup)
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

func TestModelPriceHelperBillingRatioMatrix(t *testing.T) {
	gin.SetMode(gin.TestMode)
	originalModelRatio := ratio_setting.ModelRatio2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"vip":1,"svip":1}`))
		require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(`{}`))
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(originalModelRatio))
	})
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"vip":1.25}`))
	require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(`{"member":{"vip":1.4}}`))
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"matrix-test-model":1}`))

	tests := []struct {
		name              string
		info              *relaycommon.RelayInfo
		wantGroupRatio    float64
		wantBaseRatio     float64
		wantPreConsume    int
		wantAgentSnapshot bool
		wantAgentBase     float64
		wantAgentCharged  float64
	}{
		{
			name: "system user uses system group ratio",
			info: &relaycommon.RelayInfo{
				UserGroup:       "default",
				UsingGroup:      "vip",
				TokenGroup:      "vip",
				OriginModelName: "matrix-test-model",
			},
			wantGroupRatio: 1.25,
			wantBaseRatio:  1.25,
			wantPreConsume: 625,
		},
		{
			name: "system user group override uses overridden ratio",
			info: &relaycommon.RelayInfo{
				UserGroup:       "member",
				UsingGroup:      "vip",
				TokenGroup:      "vip",
				OriginModelName: "matrix-test-model",
			},
			wantGroupRatio: 1.4,
			wantBaseRatio:  1.4,
			wantPreConsume: 700,
		},
		{
			name: "agent user uses agent configured ratio",
			info: &relaycommon.RelayInfo{
				UserGroup:       "default",
				UsingGroup:      "vip",
				TokenGroup:      "vip",
				OriginModelName: "matrix-test-model",
				AgentContext:    testAgentContext(1.1, 1.5, nil),
			},
			wantGroupRatio:    1.5,
			wantBaseRatio:     1.25,
			wantPreConsume:    750,
			wantAgentSnapshot: true,
			wantAgentBase:     1.1,
			wantAgentCharged:  1.5,
		},
		{
			name: "agent user group override uses agent override ratio",
			info: &relaycommon.RelayInfo{
				UserGroup:       "member",
				UsingGroup:      "vip",
				TokenGroup:      "vip",
				OriginModelName: "matrix-test-model",
				AgentContext: testAgentContext(1.1, 1.5, map[string]float64{
					"vip": 1.7,
				}),
			},
			wantGroupRatio:    1.7,
			wantBaseRatio:     1.4,
			wantPreConsume:    850,
			wantAgentSnapshot: true,
			wantAgentBase:     1.1,
			wantAgentCharged:  1.7,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

			priceData, err := ModelPriceHelper(ctx, tt.info, 100, &types.TokenCountMeta{})
			require.NoError(t, err)
			require.Equal(t, tt.wantGroupRatio, priceData.GroupRatioInfo.GroupRatio)
			require.Equal(t, tt.wantBaseRatio, priceData.GroupRatioInfo.BaseGroupRatio)
			require.Equal(t, tt.wantPreConsume, priceData.QuotaToPreConsume)
			if tt.wantAgentSnapshot {
				require.True(t, priceData.GroupRatioInfo.HasAgentRatio)
				require.NotNil(t, tt.info.AgentBillingSnapshot)
				require.Equal(t, "vip", tt.info.AgentBillingSnapshot.Group)
				require.Equal(t, tt.wantAgentBase, tt.info.AgentBillingSnapshot.BaseGroupRatio)
				require.Equal(t, tt.wantAgentCharged, tt.info.AgentBillingSnapshot.ChargedGroupRatio)
			} else {
				require.False(t, priceData.GroupRatioInfo.HasAgentRatio)
				require.Nil(t, tt.info.AgentBillingSnapshot)
			}
		})
	}
}

func testAgentContext(agentRatio float64, configuredRatio float64, memberOverrides map[string]float64) *types.AgentContext {
	ctx := &types.AgentContext{
		AgentID: 3,
		Domain:  "agent.example.com",
		Groups: map[string]types.AgentGroup{
			"vip": {
				GroupName:       "vip",
				SystemGroupName: "vip",
				AgentRatio:      agentRatio,
				EffectiveRatio:  configuredRatio,
				Visible:         true,
				Available:       true,
			},
		},
		GroupRatios: map[string]float64{
			"vip": configuredRatio,
		},
	}
	if memberOverrides != nil {
		ctx.UserGroups = map[string]types.AgentUserGroup{
			"member": {
				GroupName:   "member",
				GroupRatios: memberOverrides,
			},
		}
	}
	return ctx
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

func TestHandleGroupRatioKeepsSystemUserBillingWithoutAgent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"vip":1,"svip":1}`))
		require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(`{}`))
	})
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"vip":1.25}`))
	require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(`{"member":{"vip":1.4}}`))

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	info := &relaycommon.RelayInfo{
		UserGroup:  "member",
		UsingGroup: "vip",
		TokenGroup: "vip",
	}

	groupRatioInfo := HandleGroupRatio(ctx, info)

	require.Equal(t, 1.4, groupRatioInfo.GroupRatio)
	require.Equal(t, 1.4, groupRatioInfo.BaseGroupRatio)
	require.Equal(t, 1.4, groupRatioInfo.GroupSpecialRatio)
	require.True(t, groupRatioInfo.HasSpecialRatio)
	require.False(t, groupRatioInfo.HasAgentRatio)
	require.Nil(t, info.AgentBillingSnapshot)
	require.Equal(t, "vip", info.UsingGroup)
}

func TestHandleGroupRatioUsesAgentSystemGroupRule(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"vip":1,"svip":1}`))
	})
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"vip":1.25}`))

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	info := &relaycommon.RelayInfo{
		UserGroup:  "vip",
		UsingGroup: "vip",
		TokenGroup: "vip",
		AgentContext: &types.AgentContext{
			AgentID: 3,
			Domain:  "agent.example.com",
			Groups: map[string]types.AgentGroup{
				"vip": {
					GroupName:       "vip",
					SystemGroupName: "vip",
					AgentRatio:      1.1,
					EffectiveRatio:  1.5,
					Visible:         true,
					Available:       true,
				},
			},
			GroupRatios: map[string]float64{
				"vip": 1.5,
			},
		},
	}

	groupRatioInfo := HandleGroupRatio(ctx, info)
	require.Equal(t, 1.5, groupRatioInfo.GroupRatio)
	require.Equal(t, 1.25, groupRatioInfo.BaseGroupRatio)
	require.True(t, groupRatioInfo.HasAgentRatio)
	require.Equal(t, "vip", info.UsingGroup)
	require.NotNil(t, info.AgentBillingSnapshot)
	require.Equal(t, "vip", info.AgentBillingSnapshot.Group)
	require.Equal(t, 1.1, info.AgentBillingSnapshot.BaseGroupRatio)
	require.Equal(t, 1.5, info.AgentBillingSnapshot.ChargedGroupRatio)
}

func TestHandleGroupRatioUsesAgentUserGroupOverride(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"vip":1,"svip":1}`))
	})
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"vip":1.25}`))

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	info := &relaycommon.RelayInfo{
		UserGroup:  "member",
		UsingGroup: "vip",
		TokenGroup: "vip",
		AgentContext: &types.AgentContext{
			AgentID: 3,
			Domain:  "agent.example.com",
			Groups: map[string]types.AgentGroup{
				"vip": {
					GroupName:       "vip",
					SystemGroupName: "vip",
					AgentRatio:      1.1,
					EffectiveRatio:  1.5,
					Visible:         true,
					Available:       true,
				},
			},
			UserGroups: map[string]types.AgentUserGroup{
				"member": {
					GroupName: "member",
					GroupRatios: map[string]float64{
						"vip": 1.7,
					},
				},
			},
			GroupRatios: map[string]float64{
				"vip": 1.5,
			},
		},
	}

	groupRatioInfo := HandleGroupRatio(ctx, info)

	require.Equal(t, 1.7, groupRatioInfo.GroupRatio)
	require.Equal(t, 1.25, groupRatioInfo.BaseGroupRatio)
	require.True(t, groupRatioInfo.HasAgentRatio)
	require.NotNil(t, info.AgentBillingSnapshot)
	require.Equal(t, "vip", info.AgentBillingSnapshot.Group)
	require.Equal(t, 1.1, info.AgentBillingSnapshot.BaseGroupRatio)
	require.Equal(t, 1.7, info.AgentBillingSnapshot.ChargedGroupRatio)
}

func TestHandleGroupRatioFloorsAgentChargeAtMatchedAgentRatio(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"vip":1,"svip":1}`))
	})
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"vip":1.25}`))
	require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(`{"member":{"vip":1.2}}`))
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(`{}`))
	})

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	info := &relaycommon.RelayInfo{
		UserGroup:  "member",
		UsingGroup: "vip",
		TokenGroup: "vip",
		AgentContext: &types.AgentContext{
			AgentID: 3,
			Domain:  "agent.example.com",
			Groups: map[string]types.AgentGroup{
				"vip": {
					GroupName:       "vip",
					SystemGroupName: "vip",
					AgentRatio:      1.0,
					EffectiveRatio:  1.1,
					Visible:         true,
					Available:       true,
				},
			},
			UserGroups: map[string]types.AgentUserGroup{
				"member": {
					GroupName: "member",
					GroupRatios: map[string]float64{
						"vip": 1.1,
					},
				},
			},
			GroupRatios: map[string]float64{
				"vip": 1.1,
			},
		},
	}

	groupRatioInfo := HandleGroupRatio(ctx, info)

	require.Equal(t, 1.1, groupRatioInfo.GroupRatio)
	require.Equal(t, 1.2, groupRatioInfo.BaseGroupRatio)
	require.True(t, groupRatioInfo.HasAgentRatio)
	require.NotNil(t, info.AgentBillingSnapshot)
	require.Equal(t, "vip", info.AgentBillingSnapshot.Group)
	require.Equal(t, 1.0, info.AgentBillingSnapshot.BaseGroupRatio)
	require.Equal(t, 1.1, info.AgentBillingSnapshot.ChargedGroupRatio)
}

func TestHandleGroupRatioUsesSelectedAgentGroupFromAutoRouting(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"vip":1,"svip":1}`))
	})
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":0.2}`))

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	common.SetContextKey(ctx, constant.ContextKeyAgentSelectedGroup, "default")
	info := &relaycommon.RelayInfo{
		UserGroup:  "member",
		UsingGroup: "default",
		TokenGroup: "auto",
		AgentContext: &types.AgentContext{
			AgentID: 3,
			Domain:  "agent.example.com",
			Groups: map[string]types.AgentGroup{
				"default": {
					GroupName:       "default",
					SystemGroupName: "default",
					AgentRatio:      0.24,
					EffectiveRatio:  0.24,
					Visible:         true,
					Available:       true,
				},
			},
			UserGroups: map[string]types.AgentUserGroup{
				"member": {
					GroupName: "member",
					GroupRatios: map[string]float64{
						"default": 0.31,
					},
				},
			},
			GroupRatios: map[string]float64{
				"default": 0.24,
			},
		},
	}

	groupRatioInfo := HandleGroupRatio(ctx, info)

	require.Equal(t, 0.31, groupRatioInfo.GroupRatio)
	require.Equal(t, 0.2, groupRatioInfo.BaseGroupRatio)
	require.True(t, groupRatioInfo.HasAgentRatio)
	require.NotNil(t, info.AgentBillingSnapshot)
	require.Equal(t, "default", info.AgentBillingSnapshot.Group)
	require.Equal(t, 0.24, info.AgentBillingSnapshot.BaseGroupRatio)
	require.Equal(t, 0.31, info.AgentBillingSnapshot.ChargedGroupRatio)
}

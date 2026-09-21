package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSmartRetryAssetResourceNeverReplays(t *testing.T) {
	for _, strategy := range model.RoutingStrategies() {
		t.Run(strategy, func(t *testing.T) {
			preserveSmartRetryTestConfiguration(t)
			db := openConfigurableResourceTestDB(t)
			originalRetries := common.RetryTimes
			originalAuto := setting.AutoGroups2JsonString()
			t.Cleanup(func() {
				common.RetryTimes = originalRetries
				require.NoError(t, setting.UpdateAutoGroupsByJsonString(originalAuto))
			})
			common.RetryTimes = 0
			require.NoError(t, db.AutoMigrate(&model.RoutingStrategySnapshot{}))
			require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"default","primary":"primary","backup":"backup"}`))
			require.NoError(t, setting.UpdateAutoGroupsByJsonString(`["primary","backup"]`))
			require.NoError(t, model.UpsertRoutingStrategySnapshot(&model.RoutingStrategySnapshot{Strategy: strategy, UserGroup: "default", Groups: `["primary","backup"]`, Scores: `{}`, Config: `{}`}))
			var calls [2]atomic.Int32
			for i, group := range []string{"primary", "backup"} {
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls[i].Add(1)
					w.Header().Set("Content-Type", "application/json")
					if i == 0 {
						w.WriteHeader(http.StatusServiceUnavailable)
						_, _ = w.Write([]byte(`{"error":{"message":"overloaded","type":"server_error"}}`))
						return
					}
					_, _ = w.Write([]byte(`{"code":0,"data":{"Id":"asset-backup"}}`))
				}))
				t.Cleanup(upstream.Close)
				priority := int64(i * 100)
				channel := &model.Channel{Id: 400 + i, Type: constant.ChannelTypeConfigurable, Key: "test-key", Status: common.ChannelStatusEnabled, Name: group, Group: group, Models: "placeholder", BaseURL: &upstream.URL, Priority: &priority}
				channel.SetSetting(dto.ChannelSettings{Protocol: &dto.ChannelProtocolSettings{ProfileID: "doubao-seedance-2"}})
				require.NoError(t, db.Create(channel).Error)
				require.NoError(t, db.Create(&model.Ability{Group: group, Model: "placeholder", ChannelId: channel.Id, Enabled: true, Priority: &priority}).Error)
			}
			router := gin.New()
			router.POST("/material/assets", func(c *gin.Context) {
				common.SetContextKey(c, constant.ContextKeyUserId, 1)
				common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
				common.SetContextKey(c, constant.ContextKeyUsingGroup, "auto")
				common.SetContextKey(c, constant.ContextKeyTokenGroupPolicy, `{"type":"routing_strategy","strategy":"`+strategy+`"}`)
				common.SetContextKey(c, constant.ContextKeyTokenCrossGroupRetry, false)
			}, middleware.ConfigurableResource("doubao-seedance-2", "material_assets"), RelayConfigurableResource)
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/material/assets", strings.NewReader(`{"url":"https://cdn.example.com/test.jpg","asset_type":"Image"}`))
			request.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(recorder, request)
			require.Equal(t, http.StatusServiceUnavailable, recorder.Code, recorder.Body.String())
			require.JSONEq(t, `{"error":{"message":"overloaded","type":"server_error"}}`, recorder.Body.String())
			require.Equal(t, int32(1), calls[0].Load(), "ranking must take precedence over the backup channel's higher priority")
			require.Zero(t, calls[1].Load(), "asset requests must not replay against another supplier account")
		})
	}
}

func TestSmartRetryTokenCreationAlwaysEnablesGroupFailover(t *testing.T) {
	for _, strategy := range model.RoutingStrategies() {
		t.Run(strategy, func(t *testing.T) {
			preserveSmartRetryTestConfiguration(t)
			db := setupTokenControllerTestDB(t)
			setTokenTestGroups(t)
			ctx, recorder := newAuthenticatedContext(t, http.MethodPost, "/api/token/", map[string]any{"name": "smart-token", "expired_time": -1, "unlimited_quota": true, "group": "auto", "group_policy": `{"type":"routing_strategy","strategy":"` + strategy + `"}`, "cross_group_retry": false}, 1)
			ctx.Set("group", "default")
			AddToken(ctx)
			response := decodeAPIResponse(t, recorder)
			require.True(t, response.Success, response.Message)
			var detail tokenResponseItem
			require.NoError(t, common.Unmarshal(response.Data, &detail))
			var token model.Token
			require.NoError(t, db.First(&token, detail.ID).Error)
			require.True(t, token.CrossGroupRetry)
		})
	}
}

// These integration tests reuse older fixtures which restore hard-coded defaults.
// Restore their actual entry configuration after those fixtures have cleaned up.
func preserveSmartRetryTestConfiguration(t *testing.T) {
	t.Helper()
	usable, ratios, auto := setting.UserUsableGroups2JSONString(), ratio_setting.GroupRatio2JSONString(), setting.AutoGroups2JsonString()
	db, logDB := model.DB, model.LOG_DB
	sqlite, mysql, postgres, redis, memory := common.UsingSQLite, common.UsingMySQL, common.UsingPostgreSQL, common.RedisEnabled, common.MemoryCacheEnabled
	t.Cleanup(func() {
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(usable))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(ratios))
		require.NoError(t, setting.UpdateAutoGroupsByJsonString(auto))
		model.DB, model.LOG_DB = db, logDB
		common.UsingSQLite, common.UsingMySQL, common.UsingPostgreSQL, common.RedisEnabled, common.MemoryCacheEnabled = sqlite, mysql, postgres, redis, memory
		model.InitColForTest()
	})
}

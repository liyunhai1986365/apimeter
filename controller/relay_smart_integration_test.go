package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSmartRetryDistributeAndRelayReachBackup(t *testing.T) {
	for _, strategy := range model.RoutingStrategies() {
		t.Run(strategy, func(t *testing.T) {
			preserveSmartRetryTestConfiguration(t)
			db := openConfigurableResourceTestDB(t)
			originalRetries, originalConsumeLog := common.RetryTimes, common.LogConsumeEnabled
			originalModelRatios, originalAuto := ratio_setting.ModelRatio2JSONString(), setting.AutoGroups2JsonString()
			t.Cleanup(func() {
				common.RetryTimes = originalRetries
				common.LogConsumeEnabled = originalConsumeLog
				require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(originalModelRatios))
				require.NoError(t, setting.UpdateAutoGroupsByJsonString(originalAuto))
			})
			common.RetryTimes = 5
			common.LogConsumeEnabled = false
			require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"gpt-smart-retry-test":0}`))
			require.NoError(t, db.AutoMigrate(&model.RoutingStrategySnapshot{}))
			require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"default","primary":"primary","backup":"backup"}`))
			require.NoError(t, setting.UpdateAutoGroupsByJsonString(`["primary","backup"]`))
			require.NoError(t, model.UpsertRoutingStrategySnapshot(&model.RoutingStrategySnapshot{Strategy: strategy, UserGroup: "default", Groups: `["primary","backup"]`, Scores: `{}`, Config: `{}`}))
			require.NoError(t, db.Create(&model.User{Id: 999, Username: "smart-retry-test", Quota: 1000000, Group: "default", Status: common.UserStatusEnabled}).Error)
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
					_, _ = w.Write([]byte(`{"id":"chatcmpl-test","object":"chat.completion","model":"gpt-smart-retry-test","choices":[{"index":0,"message":{"role":"assistant","content":"backup ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
				}))
				t.Cleanup(upstream.Close)
				enabled := i != 0
				priority := int64(10)
				weight := uint(100)
				channel := &model.Channel{Id: 700 + i, Type: constant.ChannelTypeOpenAI, Key: "test-key", Status: common.ChannelStatusEnabled, Name: group, Group: group, Models: "gpt-smart-retry-test", BaseURL: &upstream.URL, Priority: &priority, Weight: &weight, RetryEnabled: &enabled}
				require.NoError(t, db.Create(channel).Error)
				require.NoError(t, db.Create(&model.Ability{Group: group, Model: "gpt-smart-retry-test", ChannelId: channel.Id, Enabled: true, Priority: &priority, Weight: weight}).Error)
			}
			service.InitHttpClient()
			router := gin.New()
			router.POST("/v1/chat/completions", func(c *gin.Context) {
				common.SetContextKey(c, constant.ContextKeyUserId, 999)
				common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
				common.SetContextKey(c, constant.ContextKeyUsingGroup, "auto")
				common.SetContextKey(c, constant.ContextKeyTokenGroup, "auto")
				common.SetContextKey(c, constant.ContextKeyTokenGroupPolicy, `{"type":"routing_strategy","strategy":"`+strategy+`"}`)
				common.SetContextKey(c, constant.ContextKeyTokenCrossGroupRetry, false)
			}, middleware.Distribute(), func(c *gin.Context) { Relay(c, types.RelayFormatOpenAI) })
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-smart-retry-test","messages":[{"role":"user","content":"hello"}]}`))
			request.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(recorder, request)
			require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
			require.Contains(t, recorder.Body.String(), "backup ok")
			require.Equal(t, int32(1), calls[0].Load())
			require.Equal(t, int32(1), calls[1].Load())
		})
	}
}

package controller

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSmartRetryFirstAttemptKeepsGroupPlan(t *testing.T) {
	for _, strategy := range model.RoutingStrategies() {
		t.Run(strategy, func(t *testing.T) {
			for _, tc := range []struct {
				name         string
				retry        int
				channelRetry bool
			}{
				{"first_channel_retry_disabled", 5, false},
				{"zero_group_retries", 0, true},
			} {
				t.Run(tc.name, func(t *testing.T) {
					db := openRelayRetryEventTestDB(t)
					origRetry, origCache := common.RetryTimes, common.MemoryCacheEnabled
					origGroups, origUsable := setting.AutoGroups2JsonString(), setting.UserUsableGroups2JSONString()
					t.Cleanup(func() {
						require.NoError(t, setting.UpdateAutoGroupsByJsonString(origGroups))
						require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(origUsable))
					})
					origRules := operation_setting.AutomaticRetryPolicyRules
					common.RetryTimes = tc.retry
					common.MemoryCacheEnabled = true
					operation_setting.AutomaticRetryPolicyRules = nil
					t.Cleanup(func() {
						common.RetryTimes = origRetry
						common.MemoryCacheEnabled = origCache
						operation_setting.AutomaticRetryPolicyRules = origRules
					})
					require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}, &model.RoutingStrategySnapshot{}))
					require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"default","audit-a":"A","audit-b":"B"}`))
					require.NoError(t, setting.UpdateAutoGroupsByJsonString(`["audit-a","audit-b"]`))
					priority := int64(10)
					weight := uint(100)
					for i, g := range []string{"audit-a", "audit-b"} {
						ch := model.Channel{Id: 9001 + i, Type: 1, Key: "audit-placeholder", Status: common.ChannelStatusEnabled, Name: g, Group: g, Models: "gpt-audit", Priority: &priority, Weight: &weight}
						require.NoError(t, db.Create(&ch).Error)
						require.NoError(t, db.Create(&model.Ability{Group: g, Model: "gpt-audit", ChannelId: ch.Id, Enabled: true, Priority: &priority, Weight: weight}).Error)
					}
					require.NoError(t, model.UpsertRoutingStrategySnapshot(&model.RoutingStrategySnapshot{Strategy: strategy, UserGroup: "default", Groups: `["audit-a","audit-b"]`, Scores: `{}`, Config: `{}`}))
					model.InitChannelCache()
					c, _ := gin.CreateTestContext(httptest.NewRecorder())
					c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
					common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
					common.SetContextKey(c, constant.ContextKeyTokenGroup, "auto")
					common.SetContextKey(c, constant.ContextKeyUsingGroup, "auto")
					common.SetContextKey(c, constant.ContextKeyTokenGroupPolicy, `{"type":"routing_strategy","strategy":"`+strategy+`"}`)
					common.SetContextKey(c, constant.ContextKeyTokenCrossGroupRetry, true)
					// Execute the distributor selection, then the fresh controller parameter exactly as the two call sites do.
					first, group, err := service.CacheGetRandomSatisfiedChannel(&service.RetryParam{Ctx: c, TokenGroup: "auto", ModelName: "gpt-audit", Retry: common.GetPointer(0)})
					require.NoError(t, err)
					require.Equal(t, "audit-a", group)
					require.Nil(t, middleware.SetupContextForSelectedChannel(c, first, "gpt-audit"))
					common.SetContextKey(c, constant.ContextKeyChannelRetryEnabled, tc.channelRetry)
					info := relaycommon.GenRelayInfoOpenAI(c, nil)
					require.Nil(t, info.ChannelMeta)
					p := &service.RetryParam{Ctx: c, TokenGroup: info.TokenGroup, ModelName: "gpt-audit", Retry: common.GetPointer(0)}
					selected, apiErr := getChannel(c, info, p)
					require.Nil(t, apiErr)
					require.Equal(t, first.Id, selected.Id)
					if !selectedChannelAllowsRetry(c) {
						require.True(t, p.AdvanceToNextTokenGroup())
					}
					upstreamErr := types.NewOpenAIError(errors.New("upstream overloaded"), types.ErrorCodeBadResponseStatusCode, http.StatusServiceUnavailable)
					decision := shouldRetryWithTokenGroupPlan(c, upstreamErr, p.RemainingSystemRetries(), p.HasNextTokenGroup())
					require.True(t, decision, "the healthy backup must remain reachable after the first failure")
					info.InitChannelMeta(c)
					p.IncreaseRetry()
					backup, backupErr := getChannel(c, info, p)
					require.Nil(t, backupErr)
					require.Equal(t, 9002, backup.Id)
				})
			}
		})
	}
}

func TestSmartRetrySafetyPrecedesPolicy(t *testing.T) {
	origRules := operation_setting.AutomaticRetryPolicyRules
	operation_setting.AutomaticRetryPolicyRules = nil
	t.Cleanup(func() { operation_setting.AutomaticRetryPolicyRules = origRules })
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	common.SetContextKey(c, constant.ContextKeyChannelSetting, dto.ChannelSettings{})
	_, err := c.Writer.Write([]byte("data: partial output\n\n"))
	require.NoError(t, err)
	apiErr := types.NewOpenAIError(errors.New("response.failed after delta"), types.ErrorCodeBadResponse, http.StatusInternalServerError)
	require.True(t, c.Writer.Written())
	require.False(t, shouldRetry(c, apiErr, 3))
	operation_setting.AutomaticRetryPolicyRules = []operation_setting.RetryPolicyRule{{Name: "retry 500", Action: operation_setting.RetryPolicyActionRetry, StatusCodes: "500"}}
	c2, _ := gin.CreateTestContext(httptest.NewRecorder())
	c2.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	common.SetContextKey(c2, constant.ContextKeyChannelSetting, dto.ChannelSettings{})
	c2.Set("specific_channel_id", 9001)
	skipErr := types.NewOpenAIError(errors.New("non-replayable"), types.ErrorCodeBadResponse, http.StatusInternalServerError, types.ErrOptionWithSkipRetry())
	require.False(t, shouldRetry(c2, skipErr, 3))
}

func TestSmartRetryTaskDoesNotReplayAmbiguousSubmission(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", nil)
	for _, taskErr := range []*dto.TaskError{
		{Code: "do_request_failed", StatusCode: 500},
		{Code: "copy_response_body_failed", StatusCode: 500},
		{StatusCode: 504}, {StatusCode: 524}, {StatusCode: 408},
		{LocalError: true, StatusCode: 500},
	} {
		require.False(t, shouldRetryTaskRelay(c, 1, taskErr, 3, true))
	}
	require.True(t, shouldRetryTaskRelay(c, 1, &dto.TaskError{StatusCode: 503}, 3, true))
}

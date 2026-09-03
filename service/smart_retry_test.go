package service

import (
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func smartRetryTestContext(t *testing.T, strategy string) *gin.Context {
	t.Helper()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
	common.SetContextKey(c, constant.ContextKeyTokenGroupPolicy, `{"type":"routing_strategy","strategy":"`+strategy+`"}`)
	common.SetContextKey(c, constant.ContextKeyTokenCrossGroupRetry, false)
	return c
}

func TestSmartRetryAllStrategiesCapGroupsAndKeepUntriedPeers(t *testing.T) {
	for _, strategy := range model.RoutingStrategies() {
		for _, memory := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s/cache=%v", strategy, memory), func(t *testing.T) {
				db := openChannelSelectTestDB(t)
				originalRetry := common.RetryTimes
				groups, usable := setting.AutoGroups2JsonString(), setting.UserUsableGroups2JSONString()
				t.Cleanup(func() {
					common.RetryTimes = originalRetry
					require.NoError(t, setting.UpdateAutoGroupsByJsonString(groups))
					require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(usable))
				})
				common.RetryTimes = 5
				require.NoError(t, db.AutoMigrate(&model.RoutingStrategySnapshot{}))
				require.NoError(t, setting.UpdateAutoGroupsByJsonString(`["vip","backup"]`))
				require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"default","vip":"vip","backup":"backup"}`))
				for i := 0; i < 5; i++ {
					createChannelSelectFixture(t, db, 700+i, "vip", 100-int64(i/2))
					require.NoError(t, db.Model(&model.Channel{}).Where("id = ?", 700+i).Update("key", fmt.Sprintf("test-key-%d", i)).Error)
				}
				createChannelSelectFixture(t, db, 800, "backup", 100)
				require.NoError(t, model.UpsertRoutingStrategySnapshot(&model.RoutingStrategySnapshot{Strategy: strategy, UserGroup: "default", Groups: `["vip","backup"]`, Scores: `{}`, Config: `{}`}))
				model.InitChannelCache()
				common.MemoryCacheEnabled = memory
				c := smartRetryTestContext(t, strategy)
				p := &RetryParam{Ctx: c, TokenGroup: "auto", ModelName: "gpt-test", Retry: common.GetPointer(0)}
				seen := map[int]bool{}
				for i := 0; i < 5; i++ {
					require.True(t, p.BeginAttempt())
					channel, group, err := CacheGetRandomSatisfiedChannel(p)
					require.NoError(t, err)
					require.NotNil(t, channel)
					require.False(t, seen[channel.Id])
					seen[channel.Id] = true
					if i < 2 {
						require.Contains(t, []int{700, 701}, channel.Id)
					}
					if i < 4 {
						require.Equal(t, "vip", group)
					} else {
						require.Equal(t, 800, channel.Id)
						require.Equal(t, "backup", group)
					}
					RecordSmartRetryGroupAttempt(c, group)
					common.SetContextKey(c, constant.ContextKeyChannelKey, channel.Key)
					MarkSmartRetryChannelFailure(c, channel, p.ModelName, false)
					p.IncreaseRetry()
				}
				require.False(t, seen[704], "the fifth request in one group exceeds the three-retry cap")
			})
		}
	}
}

func TestSmartRetryBudgetSurvivesModelFallbackAndExpires(t *testing.T) {
	c := smartRetryTestContext(t, model.RoutingStrategySmartAuto)
	for i := 0; i < SmartMaxAttempts; i++ {
		p := &RetryParam{Ctx: c, ModelName: fmt.Sprintf("model-%d", i)}
		require.True(t, p.BeginAttempt())
		require.Equal(t, i, p.GetAttempt())
	}
	require.False(t, (&RetryParam{Ctx: c}).BeginAttempt())
	c = smartRetryTestContext(t, model.RoutingStrategySpeedFirst)
	p := &RetryParam{Ctx: c}
	require.True(t, p.BeginAttempt())
	requestSmartRetryState(c).started = time.Now().Add(-SmartRetryWindow - time.Second)
	require.False(t, p.BeginAttempt())
	require.False(t, WaitBeforeSmartRetry(c))
}

func TestSmartRetryExcludesFailedCredentialAcrossChannelAliases(t *testing.T) {
	c := smartRetryTestContext(t, model.RoutingStrategySuccessFirst)
	base := "https://upstream.invalid"
	first := &model.Channel{Id: 1, Type: 1, BaseURL: &base, Key: "same-key"}
	alias := &model.Channel{Id: 2, Type: 1, BaseURL: &base, Key: "same-key"}
	common.SetContextKey(c, constant.ContextKeyChannelKey, first.Key)
	MarkSmartRetryChannelFailure(c, first, "model", false)
	require.False(t, SmartRetryChannelFilter(c, "model")(alias))
	alias.ModelMapping = common.GetPointer(`{"model":"another-upstream-model"}`)
	require.True(t, SmartRetryChannelFilter(c, "model")(alias))
	alias.ModelMapping = nil
	alias.Key = "other-key"
	require.True(t, SmartRetryChannelFilter(c, "model")(alias))
	require.True(t, SmartRetryChannelFilter(c, "fallback-model")(first))
}

func TestSmartRetryMultiKeyUsesUntriedCredential(t *testing.T) {
	for _, mode := range []constant.MultiKeyMode{constant.MultiKeyModeRandom, constant.MultiKeyModePolling} {
		t.Run(string(mode), func(t *testing.T) {
			db := openChannelSelectTestDB(t)
			channel := &model.Channel{Id: 600, Type: 1, Group: "default", Models: "model", Key: "failed-key\nfresh-key", Status: common.ChannelStatusEnabled, ChannelInfo: model.ChannelInfo{IsMultiKey: true, MultiKeyMode: mode}}
			require.NoError(t, db.Create(channel).Error)
			require.NoError(t, db.Create(&model.Ability{Group: "default", Model: "model", ChannelId: 600, Enabled: true}).Error)
			model.InitChannelCache()
			c := smartRetryTestContext(t, model.RoutingStrategyPriceFirst)
			common.SetContextKey(c, constant.ContextKeyChannelKey, "failed-key")
			MarkSmartRetryChannelFailure(c, channel, "model", false)
			require.True(t, SmartRetryChannelFilter(c, "model")(channel))
			key, index, err := channel.GetNextEnabledKeyMatching(func(key string, _ int) bool { return ChannelRetryKeyAllowed(c, channel, "model", key) })
			require.Nil(t, err)
			require.Equal(t, "fresh-key", key)
			require.Equal(t, 1, index)
		})
	}
}

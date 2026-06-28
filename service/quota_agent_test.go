package service

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	agentservice "github.com/QuantumNous/new-api/service/agent"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func resetQuotaAgentTables(t *testing.T) {
	t.Helper()

	gin.SetMode(gin.TestMode)
	common.RedisEnabled = false
	common.BatchUpdateEnabled = false

	t.Cleanup(func() {
		model.DB.Exec("DELETE FROM agent_ledgers")
		model.LOG_DB.Exec("DELETE FROM logs")
		model.DB.Exec("DELETE FROM channels")
		model.DB.Exec("DELETE FROM tokens")
		model.DB.Exec("DELETE FROM users")
	})
	model.DB.Exec("DELETE FROM agent_ledgers")
	model.LOG_DB.Exec("DELETE FROM logs")
	model.DB.Exec("DELETE FROM channels")
	model.DB.Exec("DELETE FROM tokens")
	model.DB.Exec("DELETE FROM users")
}

func newRealtimeAgentRelayInfo() *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{
		UserId:          100,
		TokenId:         200,
		TokenKey:        "sk-agent-test",
		UserGroup:       "member",
		UsingGroup:      "vip",
		TokenGroup:      "vip",
		OriginModelName: "realtime-agent-test",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelId: 300,
		},
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
}

func newRealtimeAgentUsage() *dto.RealtimeUsage {
	return &dto.RealtimeUsage{
		TotalTokens: 100,
		InputTokens: 100,
		InputTokenDetails: dto.InputTokenDetails{
			TextTokens: 100,
		},
	}
}

func setupRealtimeBillingRatioSettings(t *testing.T) {
	t.Helper()
	originalModelRatio := ratio_setting.ModelRatio2JSONString()
	originalCompletionRatio := ratio_setting.CompletionRatio2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"vip":1,"svip":1}`))
		require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(`{}`))
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(originalModelRatio))
		require.NoError(t, ratio_setting.UpdateCompletionRatioByJSONString(originalCompletionRatio))
	})
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"vip":1.25}`))
	require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(`{"member":{"vip":1.2}}`))
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"realtime-agent-test":1}`))
	require.NoError(t, ratio_setting.UpdateCompletionRatioByJSONString(`{"realtime-agent-test":1}`))
}

func seedRealtimeBillingUserAndToken(t *testing.T, tokenKey string) {
	t.Helper()

	require.NoError(t, model.DB.Create(&model.User{
		Id:       100,
		Username: "agent-user",
		Quota:    10000,
		Status:   common.UserStatusEnabled,
		Group:    "default",
	}).Error)
	require.NoError(t, model.DB.Create(&model.Token{
		Id:          200,
		UserId:      100,
		Key:         common.NormalizeTokenKey(tokenKey),
		Status:      common.TokenStatusEnabled,
		RemainQuota: 10000,
		Name:        "agent-token",
	}).Error)
	require.NoError(t, model.DB.Create(&model.Channel{
		Id:     300,
		Name:   "agent-channel",
		Key:    "test-key",
		Status: common.ChannelStatusEnabled,
	}).Error)
}

func TestPreWssConsumeQuotaUsesAgentConfiguredRatioAndSnapshot(t *testing.T) {
	resetQuotaAgentTables(t)
	setupRealtimeBillingRatioSettings(t)
	seedRealtimeBillingUserAndToken(t, "sk-agent-test")

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/realtime", nil)
	info := newRealtimeAgentRelayInfo()
	usage := newRealtimeAgentUsage()

	require.NoError(t, PreWssConsumeQuota(ctx, info, usage))

	var user model.User
	require.NoError(t, model.DB.First(&user, 100).Error)
	require.Equal(t, 9890, user.Quota)
	var token model.Token
	require.NoError(t, model.DB.First(&token, 200).Error)
	require.Equal(t, 9890, token.RemainQuota)
	require.Equal(t, 110, token.UsedQuota)
	require.Equal(t, 1.1, info.PriceData.GroupRatioInfo.GroupRatio)
	require.Equal(t, 1.2, info.PriceData.GroupRatioInfo.BaseGroupRatio)
	require.True(t, info.PriceData.GroupRatioInfo.HasAgentRatio)
	require.NotNil(t, info.AgentBillingSnapshot)
	require.Equal(t, "vip", info.AgentBillingSnapshot.Group)
	require.Equal(t, 1.0, info.AgentBillingSnapshot.BaseGroupRatio)
	require.Equal(t, 1.1, info.AgentBillingSnapshot.ChargedGroupRatio)
	require.Equal(t, 100, agentservice.BaseQuotaFromCharged(info.AgentBillingSnapshot, 110))
}

func TestPreWssConsumeQuotaBillingRatioMatrix(t *testing.T) {
	resetQuotaAgentTables(t)
	setupRealtimeBillingRatioSettings(t)

	tests := []struct {
		name              string
		info              *relaycommon.RelayInfo
		wantGroupRatio    float64
		wantBaseRatio     float64
		wantQuota         int
		wantAgentSnapshot bool
		wantAgentBase     float64
		wantAgentCharged  float64
	}{
		{
			name: "system user uses system group ratio",
			info: &relaycommon.RelayInfo{
				UserId:          100,
				TokenId:         200,
				TokenKey:        "sk-system-test",
				UserGroup:       "default",
				UsingGroup:      "vip",
				TokenGroup:      "vip",
				OriginModelName: "realtime-agent-test",
			},
			wantGroupRatio: 1.25,
			wantBaseRatio:  1.25,
			wantQuota:      125,
		},
		{
			name: "system user group override uses overridden ratio",
			info: &relaycommon.RelayInfo{
				UserId:          100,
				TokenId:         200,
				TokenKey:        "sk-system-test",
				UserGroup:       "member",
				UsingGroup:      "vip",
				TokenGroup:      "vip",
				OriginModelName: "realtime-agent-test",
			},
			wantGroupRatio: 1.2,
			wantBaseRatio:  1.2,
			wantQuota:      120,
		},
		{
			name:              "agent user uses agent configured ratio",
			info:              newRealtimeAgentRelayInfo(),
			wantGroupRatio:    1.1,
			wantBaseRatio:     1.2,
			wantQuota:         110,
			wantAgentSnapshot: true,
			wantAgentBase:     1.0,
			wantAgentCharged:  1.1,
		},
		{
			name: "agent user group override uses agent override ratio",
			info: func() *relaycommon.RelayInfo {
				info := newRealtimeAgentRelayInfo()
				info.AgentContext.UserGroups["member"].GroupRatios["vip"] = 1.3
				return info
			}(),
			wantGroupRatio:    1.3,
			wantBaseRatio:     1.2,
			wantQuota:         130,
			wantAgentSnapshot: true,
			wantAgentBase:     1.0,
			wantAgentCharged:  1.3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetQuotaAgentTables(t)
			seedRealtimeBillingUserAndToken(t, tt.info.TokenKey)

			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/realtime", nil)
			usage := newRealtimeAgentUsage()

			require.NoError(t, PreWssConsumeQuota(ctx, tt.info, usage))

			var user model.User
			require.NoError(t, model.DB.First(&user, 100).Error)
			require.Equal(t, 10000-tt.wantQuota, user.Quota)
			var token model.Token
			require.NoError(t, model.DB.First(&token, 200).Error)
			require.Equal(t, 10000-tt.wantQuota, token.RemainQuota)
			require.Equal(t, tt.wantQuota, token.UsedQuota)
			require.Equal(t, tt.wantGroupRatio, tt.info.PriceData.GroupRatioInfo.GroupRatio)
			require.Equal(t, tt.wantBaseRatio, tt.info.PriceData.GroupRatioInfo.BaseGroupRatio)
			if tt.wantAgentSnapshot {
				require.True(t, tt.info.PriceData.GroupRatioInfo.HasAgentRatio)
				require.NotNil(t, tt.info.AgentBillingSnapshot)
				require.Equal(t, "vip", tt.info.AgentBillingSnapshot.Group)
				require.Equal(t, tt.wantAgentBase, tt.info.AgentBillingSnapshot.BaseGroupRatio)
				require.Equal(t, tt.wantAgentCharged, tt.info.AgentBillingSnapshot.ChargedGroupRatio)
			} else {
				require.False(t, tt.info.PriceData.GroupRatioInfo.HasAgentRatio)
				require.Nil(t, tt.info.AgentBillingSnapshot)
			}
		})
	}
}

func TestAgentSettleConsumeWritesProfitFromAgentSnapshot(t *testing.T) {
	resetQuotaAgentTables(t)

	snapshot := &types.AgentBillingSnapshot{
		AgentID:           3,
		Domain:            "agent.example.com",
		Group:             "vip",
		BaseGroupRatio:    1.0,
		ChargedGroupRatio: 1.1,
	}
	baseQuota := agentservice.BaseQuotaFromCharged(snapshot, 110)
	require.Equal(t, 100, baseQuota)

	require.NoError(t, agentservice.SettleConsume(snapshot, 100, 300, baseQuota, 110))

	var ledger model.AgentLedger
	require.NoError(t, model.DB.First(&ledger).Error)
	require.Equal(t, 3, ledger.AgentId)
	require.Equal(t, 100, ledger.UserId)
	require.Equal(t, 300, ledger.LogId)
	require.Equal(t, model.AgentLedgerTypeConsumeProfit, ledger.Type)
	require.Equal(t, 100, ledger.BaseQuota)
	require.Equal(t, 110, ledger.ChargedQuota)
	require.Equal(t, 10, ledger.ProfitQuota)
	require.Equal(t, 10, ledger.BalanceAfter)
}

func TestPostWssConsumeQuotaRecordsAgentProfitLedger(t *testing.T) {
	resetQuotaAgentTables(t)
	setupRealtimeBillingRatioSettings(t)

	tests := []struct {
		name           string
		info           *relaycommon.RelayInfo
		wantQuota      int
		wantLogGroup   string
		wantGroupRatio float64
		wantLedger     bool
		wantBaseQuota  int
		wantProfit     int
	}{
		{
			name: "system user records consume log without agent ledger",
			info: &relaycommon.RelayInfo{
				UserId:          100,
				TokenId:         200,
				TokenKey:        "sk-system-test",
				UserGroup:       "default",
				UsingGroup:      "vip",
				TokenGroup:      "vip",
				OriginModelName: "realtime-agent-test",
				ChannelMeta: &relaycommon.ChannelMeta{
					ChannelId: 300,
				},
			},
			wantQuota:      125,
			wantLogGroup:   "vip",
			wantGroupRatio: 1.25,
		},
		{
			name: "system user group override records overridden consume log without agent ledger",
			info: &relaycommon.RelayInfo{
				UserId:          100,
				TokenId:         200,
				TokenKey:        "sk-system-test",
				UserGroup:       "member",
				UsingGroup:      "vip",
				TokenGroup:      "vip",
				OriginModelName: "realtime-agent-test",
				ChannelMeta: &relaycommon.ChannelMeta{
					ChannelId: 300,
				},
			},
			wantQuota:      120,
			wantLogGroup:   "vip",
			wantGroupRatio: 1.2,
		},
		{
			name:           "agent user records consume log and profit ledger",
			info:           newRealtimeAgentRelayInfo(),
			wantQuota:      110,
			wantLogGroup:   "vip",
			wantGroupRatio: 1.1,
			wantLedger:     true,
			wantBaseQuota:  100,
			wantProfit:     10,
		},
		{
			name: "agent user group override records consume log and higher profit ledger",
			info: func() *relaycommon.RelayInfo {
				info := newRealtimeAgentRelayInfo()
				info.AgentContext.UserGroups["member"].GroupRatios["vip"] = 1.3
				return info
			}(),
			wantQuota:      130,
			wantLogGroup:   "vip",
			wantGroupRatio: 1.3,
			wantLedger:     true,
			wantBaseQuota:  100,
			wantProfit:     30,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetQuotaAgentTables(t)
			seedRealtimeBillingUserAndToken(t, tt.info.TokenKey)

			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/realtime", nil)
			ctx.Set("token_name", "test-token")
			tt.info.IsStream = true
			tt.info.StartTime = time.Now()
			tt.info.FirstResponseTime = time.Now()
			usage := newRealtimeAgentUsage()

			require.NoError(t, PreWssConsumeQuota(ctx, tt.info, usage))
			tt.info.FinalPreConsumedQuota = tt.wantQuota
			PostWssConsumeQuota(ctx, tt.info, "realtime-agent-test", usage, "")

			var log model.Log
			require.NoError(t, model.LOG_DB.First(&log).Error)
			require.Equal(t, 100, log.UserId)
			require.Equal(t, tt.wantQuota, log.Quota)
			require.Equal(t, tt.wantLogGroup, log.Group)
			other, err := common.StrToMap(log.Other)
			require.NoError(t, err)
			require.Equal(t, tt.wantGroupRatio, other["group_ratio"])

			var ledgerCount int64
			require.NoError(t, model.DB.Model(&model.AgentLedger{}).Count(&ledgerCount).Error)
			if !tt.wantLedger {
				require.Equal(t, int64(0), ledgerCount)
				return
			}
			require.Equal(t, int64(1), ledgerCount)
			var ledger model.AgentLedger
			require.NoError(t, model.DB.First(&ledger).Error)
			require.Equal(t, 3, ledger.AgentId)
			require.Equal(t, 100, ledger.UserId)
			require.Equal(t, log.Id, ledger.LogId)
			require.Equal(t, tt.wantBaseQuota, ledger.BaseQuota)
			require.Equal(t, tt.wantQuota, ledger.ChargedQuota)
			require.Equal(t, tt.wantProfit, ledger.ProfitQuota)
			require.Equal(t, tt.wantProfit, ledger.BalanceAfter)
		})
	}
}

package service

import (
	"errors"
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	hosttypes "github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestClaudeCacheOnlyUsageRemainsBillable(t *testing.T) {
	for _, tc := range []struct {
		name      string
		usage     dto.ClaudeUsage
		wantQuota int
	}{
		{name: "one-hour write", usage: dto.ClaudeUsage{CacheCreationInputTokens: 20000, CacheCreation: &dto.ClaudeCacheCreationUsage{Ephemeral1hInputTokens: 20000}}, wantQuota: 100000},
		{name: "five-minute write", usage: dto.ClaudeUsage{CacheCreationInputTokens: 100, CacheCreation: &dto.ClaudeCacheCreationUsage{Ephemeral5mInputTokens: 100}}, wantQuota: 313},
		{name: "unsplit write", usage: dto.ClaudeUsage{CacheCreationInputTokens: 100}, wantQuota: 313},
		{name: "split without aggregate", usage: dto.ClaudeUsage{CacheCreation: &dto.ClaudeCacheCreationUsage{Ephemeral1hInputTokens: 100}}, wantQuota: 500},
		{name: "read", usage: dto.ClaudeUsage{CacheReadInputTokens: 100}, wantQuota: 25},
		{name: "empty", usage: dto.ClaudeUsage{}, wantQuota: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			info := &relaycommon.RelayInfo{
				RelayFormat: types.RelayFormatClaude, OriginModelName: "claude-sonnet-4-5",
				PriceData: hosttypes.PriceData{
					ModelRatio: 2.5, CompletionRatio: 5, CacheRatio: .1,
					CacheCreationRatio: 1.25, CacheCreation5mRatio: 1.25, CacheCreation1hRatio: 2,
					GroupRatioInfo: hosttypes.GroupRatioInfo{GroupRatio: 1},
				},
			}
			usage := &dto.Usage{UsageSemantic: dto.BillingUsageSemanticAnthropic, BillingUsage: dto.NewClaudeMessagesBillingUsage(&tc.usage)}
			summary := calculateTextQuotaSummary(ctx, info, effectiveBillingUsage(usage))
			require.Zero(t, summary.PromptTokens)
			require.Zero(t, summary.CompletionTokens)
			require.Equal(t, tc.wantQuota, summary.Quota)
			require.Equal(t, tc.wantQuota > 0, summary.hasBillableUsage())
		})
	}
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	info := &relaycommon.RelayInfo{RelayFormat: types.RelayFormatOpenAI, PriceData: hosttypes.PriceData{ModelRatio: 1, CacheRatio: 1, GroupRatioInfo: hosttypes.GroupRatioInfo{GroupRatio: 1}}}
	summary := calculateTextQuotaSummary(ctx, info, &dto.Usage{UsageSemantic: dto.BillingUsageSemanticOpenAI, PromptTokensDetails: dto.InputTokenDetails{CachedTokens: 100}})
	require.Zero(t, summary.Quota, "OpenAI cache details are a subset of the missing total, not standalone Claude usage")
}

func TestClaudeCacheOnlySettlementBalancesLogsAndAgentProfit(t *testing.T) {
	for _, interrupted := range []bool{false, true} {
		for _, canonicalOnly := range []bool{false, true} {
			t.Run(fmt.Sprintf("interrupted=%v/canonicalOnly=%v", interrupted, canonicalOnly), func(t *testing.T) {
				truncate(t)
				seedUser(t, 9261, 100000)
				seedToken(t, 9262, 9261, "claude-cache-only", 100000)
				seedChannel(t, 9263)
				require.NoError(t, model.DB.Create(&model.Agent{Id: 9264, Name: "cache-agent", Slug: "cache-agent", Status: model.AgentStatusEnabled}).Error)
				previousLogs := common.LogConsumeEnabled
				common.LogConsumeEnabled = false
				t.Cleanup(func() { common.LogConsumeEnabled = previousLogs })
				ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
				ctx.Request = httptest.NewRequest("POST", "/v1/messages", nil)
				info := &relaycommon.RelayInfo{
					UserId: 9261, TokenId: 9262, TokenKey: "claude-cache-only", ForcePreConsume: true,
					OriginModelName: "claude-sonnet-4-5", RelayFormat: types.RelayFormatClaude, FinalRequestRelayFormat: types.RelayFormatClaude,
					StartTime: time.Now(), ChannelMeta: &relaycommon.ChannelMeta{ChannelId: 9263}, IsStream: true,
					PriceData:            hosttypes.PriceData{ModelRatio: 1, CompletionRatio: 5, CacheRatio: .1, GroupRatioInfo: hosttypes.GroupRatioInfo{GroupRatio: 1.2}},
					AgentBillingSnapshot: &hosttypes.AgentBillingSnapshot{AgentID: 9264, BaseGroupRatio: 1, ChargedGroupRatio: 1.2},
				}
				require.Nil(t, PreConsumeBilling(ctx, 1000, info))
				usage := &dto.Usage{UsageSemantic: dto.BillingUsageSemanticAnthropic, PromptTokensDetails: dto.InputTokenDetails{CachedTokens: 1000}}
				if canonicalOnly {
					usage = &dto.Usage{BillingUsage: dto.NewClaudeMessagesBillingUsage(&dto.ClaudeUsage{CacheReadInputTokens: 1000})}
				}
				if interrupted {
					apiErr := relaycommon.WithStreamUsage(types.NewOpenAIError(errors.New("stream interrupted"), types.ErrorCodeBadResponse, 502), usage)
					var marker *relaycommon.StreamUsageError
					require.ErrorAs(t, apiErr, &marker)
					SettleInterruptedStream(ctx, info, apiErr)
					SettleInterruptedStream(ctx, info, apiErr)
					info.Billing.Refund(ctx)
				} else {
					PostTextConsumeQuota(ctx, info, usage, nil)
				}
				require.False(t, info.Billing.NeedsRefund())
				require.Zero(t, usage.PromptTokens)
				require.Zero(t, usage.CompletionTokens)
				require.Equal(t, 100000-120, getUserQuota(t, 9261))
				require.Equal(t, 100000-120, getTokenRemainQuota(t, 9262))
				used, requests := getUserUsageAccounting(t, 9261)
				require.Equal(t, 120, used)
				require.Equal(t, 1, requests)
				require.EqualValues(t, 120, getChannelUsedQuota(t, 9263))
				var logs []model.Log
				require.NoError(t, model.DB.Where("user_id = ? AND type = ?", 9261, model.LogTypeConsume).Find(&logs).Error)
				require.Len(t, logs, 1)
				require.Equal(t, 120, logs[0].Quota)
				require.Zero(t, logs[0].PromptTokens)
				require.Zero(t, logs[0].CompletionTokens)
				require.Equal(t, 1000, logs[0].CacheReadTokens)
				require.Equal(t, 1000, logs[0].InputTokens)
				require.NotContains(t, logs[0].Content, "无法扣费")
				var ledgers []model.AgentLedger
				require.NoError(t, model.DB.Where("agent_id = ?", 9264).Find(&ledgers).Error)
				require.Len(t, ledgers, 1)
				require.Equal(t, logs[0].Id, ledgers[0].LogId)
				require.Equal(t, 100, ledgers[0].BaseQuota)
				require.Equal(t, 120, ledgers[0].ChargedQuota)
				require.Equal(t, 20, ledgers[0].ProfitQuota)
			})
		}
	}
}

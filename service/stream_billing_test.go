package service

import (
	"errors"
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	perfmetrics "github.com/QuantumNous/new-api/pkg/perf_metrics"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	hosttypes "github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestInterruptedStreamSettlesWalletAndTokenExactlyOnce(t *testing.T) {
	for _, protocol := range []types.RelayFormat{types.RelayFormatOpenAI, types.RelayFormatOpenAIResponses, types.RelayFormatClaude} {
		for _, tiered := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/tiered=%v", protocol, tiered), func(t *testing.T) {
				truncate(t)
				seedUser(t, 9161, 100000)
				seedToken(t, 9162, 9161, "stream-billing-test", 100000)
				seedChannel(t, 9163)
				ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
				ctx.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
				info := &relaycommon.RelayInfo{
					UserId: 9161, TokenId: 9162, TokenKey: "stream-billing-test", ForcePreConsume: true,
					OriginModelName: "gemini-3-flash-preview", RelayFormat: protocol, FinalRequestRelayFormat: types.RelayFormatGemini,
					StartTime: time.Now(), ChannelMeta: &relaycommon.ChannelMeta{ChannelId: 9163},
					PriceData: hosttypes.PriceData{ModelRatio: 1, CompletionRatio: 2, GroupRatioInfo: hosttypes.GroupRatioInfo{GroupRatio: 1}},
				}
				if protocol == types.RelayFormatOpenAIResponses {
					info.RelayMode = relayconstant.RelayModeResponses
				}
				want := 140
				if tiered {
					info.TieredBillingSnapshot = makeSnapshot(`p * 4 + c * 10`, 1, 100, 20)
					want = 300
				}
				require.Nil(t, PreConsumeBilling(ctx, 1000, info))
				usage := &dto.Usage{PromptTokens: 100, CompletionTokens: 20, TotalTokens: 120}
				apiErr := relaycommon.WithStreamUsage(types.NewOpenAIError(errors.New("stream interrupted"), types.ErrorCodeBadResponse, 502), usage)
				// Conversion wrappers must not hide the billing marker.
				wrapped := fmt.Errorf("conversion failed: %w", apiErr)
				SettleInterruptedStream(ctx, info, wrapped)
				SettleInterruptedStream(ctx, info, wrapped)
				require.False(t, info.Billing.(*BillingSession).NeedsRefund())
				info.Billing.Refund(ctx) // mirrors the controller error defer
				var user model.User
				require.NoError(t, model.DB.First(&user, 9161).Error)
				require.Equal(t, 100000-want, user.Quota)
				require.Equal(t, want, user.UsedQuota)
				require.Equal(t, 1, user.RequestCount)
				var token model.Token
				require.NoError(t, model.DB.First(&token, 9162).Error)
				require.Equal(t, 100000-want, token.RemainQuota)
				var logs []model.Log
				require.NoError(t, model.DB.Where("user_id = ? AND type = ?", 9161, model.LogTypeConsume).Find(&logs).Error)
				require.Len(t, logs, 1)
				require.Equal(t, want, logs[0].Quota)
				require.Contains(t, logs[0].Content, "流式响应中断")
			})
		}
	}
}

func TestInterruptedSearchOnlyStreamSettlesExactlyOnce(t *testing.T) {
	previousCountToken := constant.CountToken
	constant.CountToken = false
	t.Cleanup(func() { constant.CountToken = previousCountToken })
	for _, protocol := range []types.RelayFormat{types.RelayFormatOpenAI, types.RelayFormatOpenAIResponses} {
		for _, tc := range []struct {
			model string
			quota int
		}{
			{"gemini-3-flash-preview", 7000}, // $14 per 1,000 queries
			{"gemini-2.5-pro", 17500},        // $35 per 1,000 grounded prompts
		} {
			for _, tiered := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/tiered=%v", protocol, tc.model, tiered), func(t *testing.T) {
					truncate(t)
					seedUser(t, 9191, 100000)
					seedToken(t, 9192, 9191, "stream-search-only", 100000)
					seedChannel(t, 9193)
					ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
					ctx.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
					info := &relaycommon.RelayInfo{
						UserId: 9191, TokenId: 9192, TokenKey: "stream-search-only", ForcePreConsume: true,
						OriginModelName: tc.model, RelayFormat: protocol, FinalRequestRelayFormat: types.RelayFormatGemini,
						StartTime: time.Now(), ChannelMeta: &relaycommon.ChannelMeta{ChannelId: 9193},
						PriceData: hosttypes.PriceData{ModelRatio: 1, CompletionRatio: 2, GroupRatioInfo: hosttypes.GroupRatioInfo{GroupRatio: 1}},
					}
					if protocol == types.RelayFormatOpenAIResponses {
						info.RelayMode = relayconstant.RelayModeResponses
					}
					if tiered {
						info.TieredBillingSnapshot = makeSnapshot(`p * 4 + c * 10`, 1, 0, 0)
					}
					estimate, err := EstimateRequestToken(ctx, &types.TokenCountMeta{CombineText: "Find today's weather"}, info)
					require.NoError(t, err)
					require.Zero(t, estimate)
					info.SetEstimatePromptTokens(estimate)
					require.Nil(t, PreConsumeBilling(ctx, 1000, info))
					info.SetBillableToolCount(dto.BuildInToolGoogleSearch, 1)
					info.SetBillableToolCount(relaycommon.GoogleSearchGroundedPromptTool, 1)
					// Only a streaming marker can authorize interrupted settlement.
					SettleInterruptedStream(ctx, info, types.NewOpenAIError(errors.New("upstream rejected"), types.ErrorCodeBadResponse, 502))
					require.True(t, info.Billing.NeedsRefund())
					apiErr := relaycommon.WithStreamUsage(types.NewOpenAIError(errors.New("stream interrupted"), types.ErrorCodeBadResponse, 502), &dto.Usage{})
					SettleInterruptedStream(ctx, info, apiErr)
					SettleInterruptedStream(ctx, info, apiErr)
					require.False(t, info.Billing.NeedsRefund())
					require.False(t, info.AttemptSampleRecorded)
					info.Billing.Refund(ctx)
					require.Equal(t, 100000-tc.quota, getUserQuota(t, 9191))
					require.Equal(t, 100000-tc.quota, getTokenRemainQuota(t, 9192))
					used, requests := getUserUsageAccounting(t, 9191)
					require.Equal(t, tc.quota, used)
					require.Equal(t, 1, requests)
					require.EqualValues(t, tc.quota, getChannelUsedQuota(t, 9193))
					var logs []model.Log
					require.NoError(t, model.DB.Where("user_id = ? AND type = ?", 9191, model.LogTypeConsume).Find(&logs).Error)
					require.Len(t, logs, 1)
					require.Equal(t, tc.quota, logs[0].Quota)
					require.Zero(t, logs[0].PromptTokens)
					require.Zero(t, logs[0].CompletionTokens)
					var other map[string]interface{}
					require.NoError(t, common.UnmarshalJsonStr(logs[0].Other, &other))
					require.Len(t, other["tool_surcharges"], 1)
				})
			}
		}
	}
}

func TestInterruptedStreamWithoutUsageKeepsRefund(t *testing.T) {
	const freeTool = "stream-refund-free-tool"
	operation_setting.SetToolPriceForTest(freeTool, 0)
	t.Cleanup(func() { operation_setting.DeleteToolPriceForTest(freeTool) })
	for _, scenario := range []string{"text", "audio", "nil usage", "fixed price", "free tool", "unused tool"} {
		t.Run(scenario, func(t *testing.T) {
			truncate(t)
			seedUser(t, 9171, 100000)
			seedToken(t, 9172, 9171, "stream-refund-test", 100000)
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Request = httptest.NewRequest("POST", "/v1/responses", nil)
			info := &relaycommon.RelayInfo{
				UserId: 9171, TokenId: 9172, TokenKey: "stream-refund-test", ForcePreConsume: true,
				OriginModelName: "gemini-3-flash-preview", RelayMode: relayconstant.RelayModeResponses,
				ChannelMeta: &relaycommon.ChannelMeta{}, StartTime: time.Now(),
				PriceData: hosttypes.PriceData{ModelRatio: 1, GroupRatioInfo: hosttypes.GroupRatioInfo{GroupRatio: 1}},
			}
			usage := &dto.Usage{}
			switch scenario {
			case "audio":
				info.OriginModelName = "gpt-4o-audio-preview"
			case "nil usage":
				usage = nil
			case "fixed price":
				info.PriceData.UsePrice, info.PriceData.ModelPrice = true, 1
			case "free tool":
				info.SetBillableToolCount(freeTool, 1)
			case "unused tool":
				info.SetBillableToolCount(dto.BuildInToolGoogleSearch, 0)
			}
			require.Nil(t, PreConsumeBilling(ctx, 1000, info))
			upstreamErr := types.NewOpenAIError(errors.New("upstream rejected"), types.ErrorCodeBadResponse, 502)
			SettleInterruptedStream(ctx, info, upstreamErr)
			zeroUsageErr := relaycommon.WithStreamUsage(types.NewOpenAIError(errors.New("stream interrupted"), types.ErrorCodeBadResponse, 502), usage)
			SettleInterruptedStream(ctx, info, zeroUsageErr)
			SettleInterruptedStream(ctx, info, zeroUsageErr)
			require.True(t, info.Billing.NeedsRefund())
			info.Billing.Refund(ctx)
			require.Eventually(t, func() bool {
				return getUserQuota(t, 9171) == 100000 && getTokenRemainQuota(t, 9172) == 100000
			}, time.Second, time.Millisecond)
			var count int64
			require.NoError(t, model.DB.Model(&model.Log{}).Where("user_id = ? AND type = ?", 9171, model.LogTypeConsume).Count(&count).Error)
			require.Zero(t, count)
		})
	}
}

func TestStreamSettlementPreservesPerformanceOutcome(t *testing.T) {
	require.NoError(t, model.DB.AutoMigrate(&model.PerfMetric{}))
	for _, audio := range []bool{false, true} {
		for _, success := range []bool{false, true} {
			t.Run(fmt.Sprintf("audio=%v/success=%v", audio, success), func(t *testing.T) {
				truncate(t)
				seedUser(t, 9181, 100000)
				seedToken(t, 9182, 9181, "stream-metrics", 100000)
				seedChannel(t, 9183)
				ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
				ctx.Request = httptest.NewRequest("POST", "/v1/responses", nil)
				modelName := fmt.Sprintf("stream-metrics-%d", time.Now().UnixNano())
				if audio {
					modelName = "gpt-4o-audio-" + modelName
				}
				info := &relaycommon.RelayInfo{
					UserId: 9181, TokenId: 9182, TokenKey: "stream-metrics", ForcePreConsume: true,
					OriginModelName: modelName, UsingGroup: "default", StartTime: time.Now(),
					RelayMode: relayconstant.RelayModeResponses, RelayFormat: types.RelayFormatOpenAIResponses,
					ChannelMeta: &relaycommon.ChannelMeta{ChannelId: 9183},
					PriceData:   hosttypes.PriceData{ModelRatio: 1, CompletionRatio: 2, GroupRatioInfo: hosttypes.GroupRatioInfo{GroupRatio: 1}},
				}
				info.BeginAttempt()
				require.Nil(t, PreConsumeBilling(ctx, 1000, info))
				usage := &dto.Usage{PromptTokens: 100, CompletionTokens: 20, TotalTokens: 120}
				usage.PromptTokensDetails.TextTokens = 100
				usage.CompletionTokenDetails.TextTokens = 20
				if success {
					if audio {
						PostAudioConsumeQuota(ctx, info, usage, "")
					} else {
						PostTextConsumeQuota(ctx, info, usage, nil)
					}
					perfmetrics.RecordRelaySample(info, true, 0) // controller success branch
				} else {
					apiErr := relaycommon.WithStreamUsage(types.NewOpenAIError(errors.New("incomplete function arguments"), types.ErrorCodeBadResponse, 502), usage)
					SettleInterruptedStream(ctx, info, apiErr)
					SettleInterruptedStream(ctx, info, apiErr)
					require.False(t, info.AttemptSampleRecorded, "settlement must leave the attempt outcome to the controller")
					// Same snapshot operation as controller.recordRelayAttemptFailure.
					snapshot := *info
					perfmetrics.RecordRelaySample(&snapshot, false, 0)
				}
				require.False(t, info.Billing.NeedsRefund())
				result, err := perfmetrics.QuerySummaryAll(1, nil)
				require.NoError(t, err)
				found := false
				for _, row := range result.Models {
					if row.ModelName == modelName {
						found = true
						require.EqualValues(t, 1, row.RequestCount)
						wantRate := float64(0)
						if success {
							wantRate = 100
						}
						require.Equal(t, wantRate, row.SuccessRate)
					}
				}
				require.True(t, found)
			})
		}
	}
}

package claude

import (
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestClaudeStreamFinalUsagePreservesCountedZeros(t *testing.T) {
	for _, protocol := range []types.RelayFormat{types.RelayFormatClaude, types.RelayFormatOpenAI} {
		for _, tc := range []struct {
			name, startUsage, deltaUsage, stopReason string
			text                                     bool
			wantInput, wantOutput, wantRead, want1h  int
			wantEstimate                             bool
		}{
			{name: "cache-only refusal", startUsage: `{"input_tokens":0,"output_tokens":0,"cache_creation_input_tokens":20000,"cache_creation":{"ephemeral_1h_input_tokens":20000}}`, deltaUsage: `{"output_tokens":0}`, stopReason: "refusal", want1h: 20000},
			{name: "explicit final zero replaces provisional output", startUsage: `{"input_tokens":0,"output_tokens":1,"cache_read_input_tokens":50000}`, deltaUsage: `{"output_tokens":0}`, stopReason: "refusal", wantRead: 50000},
			{name: "complete zero refusal", startUsage: `{"input_tokens":0,"output_tokens":0}`, deltaUsage: `{"output_tokens":0}`, stopReason: "refusal"},
			{name: "broken cache-only input", startUsage: `{"input_tokens":0,"output_tokens":1,"cache_read_input_tokens":50000}`, text: true, wantOutput: 2, wantRead: 50000, wantEstimate: true},
			{name: "broken cache-only without output", startUsage: `{"input_tokens":0,"output_tokens":0,"cache_read_input_tokens":50000}`, wantRead: 50000},
			{name: "zeros after content are estimated", startUsage: `{"input_tokens":0,"output_tokens":0}`, deltaUsage: `{"output_tokens":0}`, stopReason: "end_turn", text: true, wantInput: 1200, wantOutput: 2, wantEstimate: true},
			{name: "zero delta input keeps initial input", startUsage: `{"input_tokens":1000,"output_tokens":1}`, deltaUsage: `{"input_tokens":0,"output_tokens":50}`, stopReason: "end_turn", text: true, wantInput: 1000, wantOutput: 50},
			{name: "omitted delta output keeps previous output", startUsage: `{"input_tokens":1000,"output_tokens":5}`, deltaUsage: `{}`, stopReason: "end_turn", wantInput: 1000, wantOutput: 5},
		} {
			t.Run(string(protocol)+"/"+tc.name, func(t *testing.T) {
				ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
				ctx.Request = httptest.NewRequest("POST", "/v1/messages", nil)
				info := &relaycommon.RelayInfo{RelayFormat: protocol, ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "claude-sonnet-4-5"}}
				info.SetEstimatePromptTokens(1200)
				state := &ClaudeResponseInfo{Usage: &dto.Usage{}}
				start := fmt.Sprintf(`{"type":"message_start","message":{"id":"msg_zero","model":"claude-sonnet-4-5","usage":%s}}`, tc.startUsage)
				require.Nil(t, HandleStreamResponseData(ctx, info, state, start))
				if tc.text {
					require.Nil(t, HandleStreamResponseData(ctx, info, state, `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hello"}}`))
				}
				if tc.deltaUsage != "" {
					delta := fmt.Sprintf(`{"type":"message_delta","delta":{"stop_reason":%q},"usage":%s}`, tc.stopReason, tc.deltaUsage)
					require.Nil(t, HandleStreamResponseData(ctx, info, state, delta))
				}
				HandleStreamFinalResponse(ctx, info, state)
				require.Equal(t, tc.wantInput, state.Usage.PromptTokens)
				require.Equal(t, tc.wantOutput, state.Usage.CompletionTokens)
				require.Equal(t, tc.wantRead, state.Usage.PromptTokensDetails.CachedTokens)
				require.Equal(t, tc.want1h, state.Usage.ClaudeCacheCreation1hTokens)
				require.Equal(t, tc.wantEstimate, common.GetContextKeyBool(ctx, constant.ContextKeyLocalCountTokens))
				if tc.wantInput+tc.wantOutput+tc.wantRead+tc.want1h == 0 {
					require.Nil(t, state.Usage.BillingUsage)
					return
				}
				require.NotNil(t, state.Usage.BillingUsage)
				canonical := state.Usage.BillingUsage.ClaudeUsage
				require.NotNil(t, canonical)
				require.Equal(t, tc.wantInput, canonical.InputTokens)
				require.Equal(t, tc.wantOutput, canonical.OutputTokens)
				require.Equal(t, tc.wantRead, canonical.CacheReadInputTokens)
				require.Equal(t, tc.want1h, canonical.GetCacheCreation1hTokens())
			})
		}
	}
}

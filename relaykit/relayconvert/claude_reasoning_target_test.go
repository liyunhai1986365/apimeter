package relayconvert

import (
	"encoding/json"
	"testing"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/convmeta"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestClaudeReasoningRespectsSourceDefaultAndOpenAITarget(t *testing.T) {
	for _, tc := range []struct {
		name, source, target, effort, wantEffort, wantDiagnostic string
		thinking                                                 *dto.Thinking
	}{
		{name: "opus 5.5 omitted", source: "claude-opus-5-5", target: "gpt-5.1", wantEffort: "medium"},
		{name: "opus 5.5 adaptive", source: "claude-opus-5-5", target: "gpt-5.1", thinking: &dto.Thinking{Type: "adaptive"}, wantEffort: "medium"},
		{name: "haiku 5.5 omitted", source: "claude-haiku-5-5", target: "o3", wantEffort: "medium"},
		{name: "sonnet 5.5 omitted", source: "claude-sonnet-5-5", target: "o3", wantEffort: "high"},
		{name: "explicit effort wins default", source: "claude-opus-5-5", target: "gpt-5.1", effort: "high", wantEffort: "high"},
		{name: "manual budget", source: "claude-sonnet-4-5", target: "o3", thinking: &dto.Thinking{Type: "enabled", BudgetTokens: kitutil.GetPointer(2048)}, wantEffort: "medium"},
		{name: "disabled", source: "claude-sonnet-5-5", target: "gpt-5.1", thinking: &dto.Thinking{Type: "between_tools"}, wantEffort: "none"},
		{name: "o3 cannot disable", source: "claude-sonnet-5-5", target: "o3", thinking: &dto.Thinking{Type: "disabled"}, wantEffort: "low", wantDiagnostic: "openai_reasoning_effort_adjusted"},
		{name: "o3 minimal", source: "claude-sonnet-5-5", target: "openai/o3-2025-04-16", effort: "minimal", wantEffort: "low", wantDiagnostic: "openai_reasoning_effort_adjusted"},
		{name: "o3 max", source: "claude-opus-5-5", target: "o3", effort: "max", wantEffort: "high", wantDiagnostic: "openai_reasoning_effort_adjusted"},
		{name: "gpt5 cannot disable", source: "claude-opus-5-5", target: "gpt-5", thinking: &dto.Thinking{Type: "disabled"}, wantEffort: "minimal", wantDiagnostic: "openai_reasoning_effort_adjusted"},
		{name: "gpt5.1 minimal", source: "claude-opus-5-5", target: "gpt-5.1", effort: "minimal", wantEffort: "low", wantDiagnostic: "openai_reasoning_effort_adjusted"},
		{name: "gpt5.2 max", source: "claude-opus-5-5", target: "gpt-5.2", effort: "max", wantEffort: "xhigh", wantDiagnostic: "openai_reasoning_effort_adjusted"},
		{name: "nonreasoning default omitted", source: "claude-opus-5-5", target: "gpt-4o"},
		{name: "nonreasoning explicit omitted", source: "claude-opus-5-5", target: "gpt-4.1", effort: "high", wantDiagnostic: "openai_reasoning_unsupported"},
		{name: "nonreasoning disabled", source: "claude-opus-5-5", target: "gpt-4o", thinking: &dto.Thinking{Type: "disabled"}},
		{name: "unknown source no default", source: "private-source", target: "gpt-5.1"},
		{name: "unknown source adaptive", source: "private-source", target: "gpt-5.1", thinking: &dto.Thinking{Type: "adaptive"}, wantEffort: "high"},
		{name: "unknown target explicit effort", source: "claude-opus-5-5", target: "private-target", effort: "xhigh", wantEffort: "xhigh"},
		{name: "future target retains effort", source: "claude-opus-5-5", target: "gpt-5.6-sol", effort: "xhigh", wantEffort: "xhigh"},
	} {
		for _, format := range []types.RelayFormat{types.RelayFormatOpenAI, types.RelayFormatOpenAIResponses} {
			for _, policy := range []types.ConversionLossPolicy{types.ConversionLossPolicyAllow, types.ConversionLossPolicyStrict} {
				t.Run(tc.name+"/"+string(format)+"/"+string(policy), func(t *testing.T) {
					info := &convmeta.Values{OriginModelName: tc.source, UpstreamModelName: tc.target, ChannelMetaAttached: true, ReasoningEffort: "stale", Options: &convmeta.Options{ToolLossPolicy: policy}}
					req := &dto.ClaudeRequest{Model: tc.target, Thinking: tc.thinking, Messages: []dto.ClaudeMessage{{Role: "user", Content: "hello"}}}
					if tc.effort != "" {
						req.OutputConfig = json.RawMessage(`{"effort":"` + tc.effort + `"}`)
					}
					before, err := kitutil.Marshal(req)
					require.NoError(t, err)
					result, err := ConvertRequest(nil, info, format, req)
					if tc.wantDiagnostic != "" && policy == types.ConversionLossPolicyStrict {
						var loss *types.ConversionLossError
						require.ErrorAs(t, err, &loss)
						require.Equal(t, tc.wantDiagnostic, loss.Diagnostics[0].Code)
						return
					}
					require.NoError(t, err)
					if tc.wantDiagnostic == "" {
						require.Empty(t, result.Diagnostics)
					} else {
						require.Len(t, result.Diagnostics, 1)
						require.Equal(t, tc.wantDiagnostic, result.Diagnostics[0].Code)
					}
					body, err := kitutil.Marshal(result.Value)
					require.NoError(t, err)
					require.Equal(t, tc.target, gjson.GetBytes(body, "model").String())
					path := "reasoning_effort"
					if format == types.RelayFormatOpenAIResponses {
						path = "reasoning.effort"
					}
					require.Equal(t, tc.wantEffort, gjson.GetBytes(body, path).String())
					if tc.wantEffort == "" {
						require.False(t, gjson.GetBytes(body, "reasoning_effort").Exists())
						require.False(t, gjson.GetBytes(body, "reasoning").Exists())
					}
					require.Equal(t, tc.wantEffort, info.ReasoningEffort)
					after, err := kitutil.Marshal(req)
					require.NoError(t, err)
					require.Equal(t, string(before), string(after), "source must remain reusable for retries")
				})
			}
		}
	}
}

func TestClaudeReasoningOpenRouterKeepsBudgetAndVisibility(t *testing.T) {
	for _, tc := range []struct {
		name, source, effort, want string
		thinking                   *dto.Thinking
	}{
		{name: "default medium", source: "claude-opus-5-5", want: `{"enabled":true,"effort":"medium"}`},
		{name: "explicit high", source: "claude-opus-5-5", effort: "high", want: `{"enabled":true,"effort":"high"}`},
		{name: "adaptive hidden", source: "claude-opus-5-5", thinking: &dto.Thinking{Type: "adaptive", Display: "omitted"}, want: `{"enabled":true,"effort":"medium","exclude":true}`},
		{name: "manual budget hidden", source: "claude-sonnet-4-5", thinking: &dto.Thinking{Type: "enabled", BudgetTokens: kitutil.GetPointer(2048), Display: "omitted"}, want: `{"enabled":true,"max_tokens":2048,"exclude":true}`},
		{name: "manual budget visible", source: "claude-sonnet-4-5", thinking: &dto.Thinking{Type: "enabled", BudgetTokens: kitutil.GetPointer(8192), Display: "summarized"}, want: `{"enabled":true,"max_tokens":8192,"exclude":false}`},
		{name: "disabled", source: "claude-sonnet-5-5", thinking: &dto.Thinking{Type: "disabled"}, want: `{"enabled":false}`},
		{name: "zero budget", source: "claude-sonnet-4-5", thinking: &dto.Thinking{Type: "enabled", BudgetTokens: kitutil.GetPointer(0)}, want: `{"enabled":false}`},
		{name: "unknown omitted", source: "private-model"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			info := &convmeta.Values{OriginModelName: tc.source, Options: &convmeta.Options{OpenRouterDialect: true}}
			req := dto.ClaudeRequest{Model: "anthropic/" + tc.source, Thinking: tc.thinking, Messages: []dto.ClaudeMessage{{Role: "user", Content: "hello"}}}
			if tc.effort != "" {
				req.OutputConfig = json.RawMessage(`{"effort":"` + tc.effort + `"}`)
			}
			out, err := ClaudeMessagesRequestToOpenAIChat(req, info)
			require.NoError(t, err)
			if tc.want == "" {
				require.Empty(t, out.Reasoning)
			} else {
				require.JSONEq(t, tc.want, string(out.Reasoning))
			}
			require.Empty(t, out.Verbosity, "Claude effort controls reasoning, not answer verbosity")
		})
	}
}

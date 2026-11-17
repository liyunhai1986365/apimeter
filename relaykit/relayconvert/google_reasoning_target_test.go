package relayconvert

import (
	"testing"

	"github.com/QuantumNous/new-api/relaykit/dto"
	kitutil "github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/stretchr/testify/require"
)

func TestGeminiReasoningRespectsOpenAITarget(t *testing.T) {
	for _, tc := range []struct {
		name, source, target string
		config               *dto.GeminiThinkingConfig
		wantEffort           string
		wantLoss             bool
	}{
		{name: "dynamic default", source: "gemini-2.5-flash", target: "gpt-4o"},
		{name: "disabled default", source: "gemini-2.5-flash-lite", target: "gpt-4o-mini"},
		{name: "level default", source: "gemini-3-flash-preview", target: "gpt-4.1"},
		{name: "minimal default", source: "gemini-3.1-flash-lite", target: "gpt-4o-2024-08-06"},
		{name: "older GPT", source: "gemini-2.5-flash", target: "gpt-3.5-turbo"},
		{name: "provider prefix", source: "gemini-2.5-flash", target: "openai/gpt-4o"},
		{name: "explicit budget", source: "gemini-2.5-flash", target: "gpt-4o", config: &dto.GeminiThinkingConfig{ThinkingBudget: kitutil.GetPointer(8192)}, wantLoss: true},
		{name: "explicit level", source: "gemini-3-flash-preview", target: "gpt-4o", config: &dto.GeminiThinkingConfig{ThinkingLevel: "high"}, wantLoss: true},
		{name: "explicit disable", source: "gemini-2.5-flash", target: "gpt-4o", config: &dto.GeminiThinkingConfig{ThinkingBudget: kitutil.GetPointer(0)}},
		{name: "summary visibility only", source: "gemini-2.5-flash", target: "gpt-4o", config: &dto.GeminiThinkingConfig{IncludeThoughts: kitutil.GetPointer(false)}},
		{name: "effort suffix", source: "gemini-3-flash-preview-high", target: "gpt-4o", wantLoss: true},
		{name: "supported default", source: "gemini-2.5-flash", target: "o3", wantEffort: "high"},
		{name: "supported budget", source: "gemini-2.5-flash", target: "gpt-5.1", config: &dto.GeminiThinkingConfig{ThinkingBudget: kitutil.GetPointer(8192)}, wantEffort: "medium"},
		{name: "supported explicit zero", source: "gemini-2.5-flash", target: "gpt-5.1", config: &dto.GeminiThinkingConfig{ThinkingBudget: kitutil.GetPointer(0)}, wantEffort: "none"},
		{name: "compatible Gemini upstream", source: "gemini-2.5-flash", target: "gemini-2.5-flash", wantEffort: "high"},
		{name: "unknown upstream alias", source: "gemini-2.5-flash", target: "private-model", config: &dto.GeminiThinkingConfig{ThinkingBudget: kitutil.GetPointer(8192)}, wantEffort: "medium"},
	} {
		for _, format := range []types.RelayFormat{types.RelayFormatOpenAI, types.RelayFormatOpenAIResponses} {
			for _, policy := range []types.ConversionLossPolicy{types.ConversionLossPolicyAllow, types.ConversionLossPolicySafe, types.ConversionLossPolicyStrict} {
				t.Run(tc.name+"/"+string(format)+"/"+string(policy), func(t *testing.T) {
					info := googleInfo(tc.source)
					info.UpstreamModelName = tc.target
					info.Options.ToolLossPolicy = policy
					info.ReasoningEffort = "stale"
					req := &dto.GeminiChatRequest{
						GenerationConfig: dto.GeminiChatGenerationConfig{ThinkingConfig: tc.config},
						Contents:         []dto.GeminiChatContent{{Role: "user", Parts: []dto.GeminiPart{{Text: "hello"}}}},
					}
					before, err := kitutil.Marshal(req)
					require.NoError(t, err)
					result, err := ConvertRequest(nil, info, format, req)
					if tc.wantLoss && policy == types.ConversionLossPolicyStrict {
						var loss *types.ConversionLossError
						require.ErrorAs(t, err, &loss)
						require.Equal(t, "openai_reasoning_unsupported", loss.Diagnostics[0].Code)
						return
					}
					require.NoError(t, err)
					if tc.wantLoss {
						require.Len(t, result.Diagnostics, 1)
						require.Equal(t, "openai_reasoning_unsupported", result.Diagnostics[0].Code)
					} else {
						require.Empty(t, result.Diagnostics)
					}
					encoded, err := kitutil.Marshal(result.Value)
					require.NoError(t, err)
					var fields map[string]any
					require.NoError(t, kitutil.Unmarshal(encoded, &fields))
					require.Equal(t, tc.target, fields["model"])
					if tc.wantEffort == "" {
						require.NotContains(t, fields, "reasoning_effort")
						require.NotContains(t, fields, "reasoning")
					} else if format == types.RelayFormatOpenAI {
						require.Equal(t, tc.wantEffort, fields["reasoning_effort"])
					} else {
						require.Equal(t, tc.wantEffort, fields["reasoning"].(map[string]any)["effort"])
					}
					require.Equal(t, tc.wantEffort, info.ReasoningEffort)
					after, err := kitutil.Marshal(req)
					require.NoError(t, err)
					require.Equal(t, string(before), string(after), "conversion must leave the source request reusable for retries")
				})
			}
		}
	}
}

func TestGeminiReasoningStillReachesClaudeThroughChat(t *testing.T) {
	info := googleInfo("gemini-2.5-flash")
	info.UpstreamModelName = "claude-opus-4-6"
	req := &dto.GeminiChatRequest{
		GenerationConfig: dto.GeminiChatGenerationConfig{MaxOutputTokens: kitutil.GetPointer(uint(8192))},
		Contents:         []dto.GeminiChatContent{{Role: "user", Parts: []dto.GeminiPart{{Text: "hello"}}}},
	}
	result, err := ConvertRequest(nil, info, types.RelayFormatClaude, req)
	require.NoError(t, err)
	out := result.Value.(*dto.ClaudeRequest)
	require.NotNil(t, out.Thinking)
	require.Equal(t, "adaptive", out.Thinking.Type)
	require.JSONEq(t, `{"effort":"high"}`, string(out.OutputConfig))
}

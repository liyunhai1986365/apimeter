package openai

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOpenRouterReasoningControlsAndUsageLog(t *testing.T) {
	type testCase struct {
		name, model, effort, nested, wantModel, wantEffort, wantReasoning string
		thinking, wantTopEffort                                           string
		preserveOrigin, preserveMapped                                    bool
	}
	tests := []testCase{
		{name: "GPT62 none", model: "gpt-6.2-sol", effort: "none", wantEffort: "none", wantReasoning: `{"enabled":false}`},
		{name: "GPT5 none survives later compatibility block", model: "gpt-5.2", effort: "none", wantEffort: "none", wantReasoning: `{"enabled":false}`},
		{name: "o series none survives later compatibility block", model: "o3", effort: "none", wantEffort: "none", wantReasoning: `{"enabled":false}`},
		{name: "provider qualified GPT62 none", model: "openai/gpt-6.2-sol", effort: "none", wantEffort: "none", wantReasoning: `{"enabled":false}`},
		{name: "other provider none", model: "anthropic/claude-sonnet-4.5", effort: "none", wantEffort: "none", wantReasoning: `{"enabled":false}`},
		{name: "absent effort stays absent", model: "anthropic/claude-sonnet-4.5"},
		{name: "ordinary effort", model: "gpt-5.2", effort: "medium", wantEffort: "medium", wantReasoning: `{"enabled":true,"effort":"medium"}`},
		{name: "thinking alias enables default", model: "anthropic/claude-sonnet-4.5-thinking", wantModel: "anthropic/claude-sonnet-4.5", wantEffort: "high", wantReasoning: `{"enabled":true}`},
		{name: "thinking alias overrides top-level none", model: "gpt-5.2-thinking", effort: "none", wantModel: "gpt-5.2", wantEffort: "high", wantReasoning: `{"enabled":true}`},
		{name: "thinking alias retains explicit effort", model: "anthropic/claude-sonnet-4.5-thinking", effort: "low", wantModel: "anthropic/claude-sonnet-4.5", wantEffort: "low", wantReasoning: `{"enabled":true,"effort":"low"}`},
		{name: "preserved origin does not enable thinking", model: "gpt-6.2-sol-thinking", effort: "none", preserveOrigin: true, wantEffort: "none", wantReasoning: `{"enabled":false}`},
		{name: "preserved mapped model does not enable thinking", model: "gpt-6.2-sol-thinking", effort: "none", preserveMapped: true, wantEffort: "none", wantReasoning: `{"enabled":false}`},
		{
			name: "nested effort wins without changing unknown values", model: "gpt-5.2", effort: "none", wantEffort: "low",
			nested:        `{"effort":"low","exclude":false,"opaque":{"integer":9007199254740993,"decimal":0.1000000000000000000000000001},"nullable":null}`,
			wantReasoning: `{"effort":"low","exclude":false,"opaque":{"integer":9007199254740993,"decimal":0.1000000000000000000000000001},"nullable":null}`,
		},
		{
			name: "thinking alias retains nested priority", model: "anthropic/claude-sonnet-4.5-thinking", effort: "none", wantModel: "anthropic/claude-sonnet-4.5", wantEffort: "medium",
			nested: `{"effort":"medium","exclude":true}`, wantReasoning: `{"effort":"medium","exclude":true}`,
		},
		{
			name: "GPT62 none suffix wins over nested strength", model: "gpt-6.2-sol-none", wantModel: "gpt-6.2-sol", wantEffort: "none",
			nested:        `{"effort":"high","max_tokens":8192,"exclude":false,"opaque":{"integer":9007199254740993,"decimal":0.1000000000000000000000000001}}`,
			wantReasoning: `{"enabled":false,"exclude":false,"opaque":{"integer":9007199254740993,"decimal":0.1000000000000000000000000001}}`,
		},
		{
			name: "nested disabled wins over nested effort", model: "gpt-6.2-sol", effort: "high", wantEffort: "none",
			nested: `{"enabled":false,"effort":"high","exclude":false}`, wantReasoning: `{"enabled":false,"effort":"high","exclude":false}`,
		},
		{
			name: "nested none wins over nested enable", model: "gpt-6.2-sol", effort: "high", wantEffort: "none",
			nested: `{"enabled":true,"effort":"none"}`, wantReasoning: `{"enabled":true,"effort":"none"}`,
		},
		{
			name: "zero budget retains false and exact metadata", model: "gpt-6.2-sol", effort: "high", wantEffort: "none",
			nested:        `{"enabled":true,"effort":"high","max_tokens":0,"exclude":false,"opaque":{"integer":9007199254740993,"decimal":0.1000000000000000000000000001},"nullable":null}`,
			wantReasoning: `{"enabled":true,"effort":"high","max_tokens":0,"exclude":false,"opaque":{"integer":9007199254740993,"decimal":0.1000000000000000000000000001},"nullable":null}`,
		},
		{
			name: "exact budget determines logged strength", model: "gpt-6.2-sol", effort: "high", wantEffort: "low",
			nested: `{"enabled":true,"effort":"high","max_tokens":1024}`, wantReasoning: `{"enabled":true,"effort":"high","max_tokens":1024}`,
		},
		{
			name: "large budget determines logged strength", model: "gpt-6.2-sol", effort: "low", wantEffort: "high",
			nested: `{"effort":"low","max_tokens":8193}`, wantReasoning: `{"effort":"low","max_tokens":8193}`,
		},
		{
			name: "budget uses existing accounting precedence over disabled", model: "gpt-6.2-sol", effort: "none", wantEffort: "medium",
			nested: `{"enabled":false,"max_tokens":8192}`, wantReasoning: `{"enabled":false,"max_tokens":8192}`,
		},
		{
			name: "visibility alone clears discarded effort", model: "gpt-6.2-sol", effort: "none",
			nested: `{"exclude":false,"opaque":123}`, wantReasoning: `{"exclude":false,"opaque":123}`,
		},
		{name: "empty nested clears discarded effort", model: "gpt-6.2-sol", effort: "high", nested: `{}`, wantReasoning: `{}`},
		{name: "null nested clears discarded effort", model: "gpt-6.2-sol", effort: "none", nested: `null`, wantReasoning: `null`},
		{
			name: "invalid nested type still passes through without stale log", model: "gpt-6.2-sol", effort: "none",
			nested: `{"enabled":"invalid"}`, wantReasoning: `{"enabled":"invalid"}`,
		},
		{
			name: "unknown nested effort still passes through without stale log", model: "gpt-6.2-sol", effort: "none",
			nested: `{"effort":"custom-effort"}`, wantReasoning: `{"effort":"custom-effort"}`,
		},
		{
			name: "legacy thinking replaces earlier nested disable", model: "anthropic/claude-sonnet-4.5", effort: "none", nested: `{"enabled":false}`,
			thinking: `{"type":"enabled","budget_tokens":1024}`, wantEffort: "low", wantReasoning: `{"enabled":true,"max_tokens":1024}`,
		},
		{
			name: "legacy thinking replaces earlier nested effort", model: "anthropic/claude-sonnet-4.5", effort: "low", nested: `{"effort":"low","exclude":false}`,
			thinking: `{"type":"enabled","budget_tokens":16384}`, wantEffort: "high", wantReasoning: `{"enabled":true,"max_tokens":16384}`,
		},
		{
			name: "late legacy suffix retains wire and nested priority", model: "gpt-5.2-high", wantModel: "gpt-5.2", wantTopEffort: "high", wantEffort: "none",
			nested: `{"enabled":false}`, wantReasoning: `{"enabled":false}`,
		},
		{name: "late legacy suffix without nested logs final top", model: "o3-high", wantModel: "o3", wantTopEffort: "high", wantEffort: "high"},
	}
	// The first three models reproduce the original nine regressions; bare
	// GPT-6.2 also passes through its compatibility helper before this branch.
	for _, model := range []string{"gpt-5.2", "o3", "openai/gpt-6.2-sol", "gpt-6.2-sol"} {
		tests = append(tests,
			testCase{name: model + " nested disable replaces top", model: model, effort: "high", nested: `{"enabled":false,"exclude":false}`, wantReasoning: `{"enabled":false,"exclude":false}`, wantEffort: "none"},
			testCase{name: model + " nested enable replaces top", model: model, effort: "none", nested: `{"enabled":true}`, wantReasoning: `{"enabled":true}`, wantEffort: "high"},
			testCase{name: model + " nested budget replaces top", model: model, effort: "none", nested: `{"max_tokens":8192,"exclude":true}`, wantReasoning: `{"max_tokens":8192,"exclude":true}`, wantEffort: "medium"},
		)
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			const origin = "client-alias"
			settings := model_setting.GetGlobalSettings()
			previousBlacklist := settings.ThinkingModelBlacklist
			if tc.preserveOrigin {
				settings.ThinkingModelBlacklist = []string{origin}
			} else if tc.preserveMapped {
				settings.ThinkingModelBlacklist = []string{tc.model}
			}
			t.Cleanup(func() { settings.ThinkingModelBlacklist = previousBlacklist })

			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			request := &dto.GeneralOpenAIRequest{
				Model: origin, ReasoningEffort: tc.effort, Reasoning: json.RawMessage(tc.nested),
				Messages:            []dto.Message{{Role: "user", Content: "hello"}},
				MaxCompletionTokens: common.GetPointer(uint(0)), ParallelToolCalls: common.GetPointer(false),
			}
			if tc.thinking != "" {
				request.THINKING = json.RawMessage(tc.thinking)
			}
			info := relaycommon.GenRelayInfoOpenAI(c, request)
			info.OriginModelName = origin
			info.ChannelMeta = &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeOpenRouter, UpstreamModelName: origin}
			mapping, err := common.Marshal(map[string]string{origin: tc.model})
			require.NoError(t, err)
			c.Set("model_mapping", string(mapping))
			require.NoError(t, helper.ModelMappedHelper(c, info, request))
			converted, err := (&Adaptor{}).ConvertOpenAIRequest(c, info, request)
			require.NoError(t, err)
			body, err := common.Marshal(converted)
			require.NoError(t, err)
			var outbound dto.GeneralOpenAIRequest
			require.NoError(t, common.Unmarshal(body, &outbound))
			wantModel := tc.wantModel
			if wantModel == "" {
				wantModel = tc.model
			}
			require.Equal(t, wantModel, outbound.Model)
			require.Equal(t, tc.wantTopEffort, outbound.ReasoningEffort)
			require.Equal(t, common.GetPointer(uint(0)), outbound.MaxCompletionTokens)
			require.Equal(t, common.GetPointer(false), outbound.ParallelToolCalls)
			require.Empty(t, outbound.THINKING)
			if tc.wantReasoning == "" {
				require.Empty(t, outbound.Reasoning)
			} else {
				var want, actual map[string]json.RawMessage
				require.NoError(t, common.Unmarshal([]byte(tc.wantReasoning), &want))
				require.NoError(t, common.Unmarshal(outbound.Reasoning, &actual))
				require.Equal(t, want, actual, "nested values must retain their exact JSON representation")
			}
			require.Equal(t, tc.wantEffort, info.ReasoningEffort)
			other := service.GenerateTextOtherInfo(c, info, 1, 1, 1, 0, 1, 0, -1)
			if tc.wantEffort == "" {
				require.NotContains(t, other, "reasoning_effort")
			} else {
				require.Equal(t, tc.wantEffort, other["reasoning_effort"])
			}
		})
	}
}

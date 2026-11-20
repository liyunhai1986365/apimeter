package openai

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGPT62MappedChatCompatibility(t *testing.T) {
	for _, tc := range []struct {
		name, origin, target, wantModel, effort, wantEffort string
		channelType                                         int
		adapt, preserveSuffix                               bool
	}{
		{name: "Astra default", target: "gpt-6.2-astra", adapt: true},
		{name: "Sol none still filters sampling", target: "gpt-6.2-sol", effort: "none", wantEffort: "none", adapt: true},
		{name: "Azure", target: "gpt-6.2-sol", channelType: constant.ChannelTypeAzure, adapt: true},
		{name: "dated snapshot", target: "gpt-6.2-astra-2026-11-18", adapt: true},
		{name: "effort suffix", target: "gpt-6.2-sol-high", wantModel: "gpt-6.2-sol", wantEffort: "high", adapt: true},
		{name: "snapshot effort suffix", target: "gpt-6.2-astra-2026-11-18-xhigh", wantModel: "gpt-6.2-astra-2026-11-18", wantEffort: "xhigh", adapt: true},
		{name: "mapped origin suffix", origin: "gpt-6.2-sol-high", target: "gpt-6.2-sol", wantEffort: "high", adapt: true},
		{name: "origin suffix takes precedence", origin: "client-alias-high", target: "gpt-6.2-sol-low", wantModel: "gpt-6.2-sol", effort: "medium", wantEffort: "high", adapt: true},
		{name: "preserved origin suffix", origin: "gpt-6.2-sol-high", target: "gpt-6.2-sol", preserveSuffix: true, effort: "low", wantEffort: "low", adapt: true},
		{name: "preserved suffix", target: "gpt-6.2-sol-high", preserveSuffix: true},
		{name: "unknown variant", target: "gpt-6.2-sol-custom"},
		{name: "invalid snapshot", target: "gpt-6.2-astra-2026-99-01"},
		{name: "future family", target: "gpt-6.3-sol"},
		{name: "private deployment", target: "my-gpt-6.2-astra"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			origin := tc.origin
			if origin == "" {
				origin = "client-alias"
			}
			settings := model_setting.GetGlobalSettings()
			previousBlacklist := settings.ThinkingModelBlacklist
			if tc.preserveSuffix {
				settings.ThinkingModelBlacklist = []string{tc.target}
				if tc.origin != "" {
					settings.ThinkingModelBlacklist = []string{tc.origin}
				}
			}
			t.Cleanup(func() { settings.ThinkingModelBlacklist = previousBlacklist })
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			mapping, err := common.Marshal(map[string]string{origin: tc.target})
			require.NoError(t, err)
			c.Set("model_mapping", string(mapping))
			channelType := tc.channelType
			if channelType == 0 {
				channelType = constant.ChannelTypeOpenAI
			}
			info := &relaycommon.RelayInfo{OriginModelName: origin, ChannelMeta: &relaycommon.ChannelMeta{
				ChannelType: channelType, UpstreamModelName: origin,
			}}
			var request dto.GeneralOpenAIRequest
			require.NoError(t, common.UnmarshalJsonStr(`{"model":"client-alias","messages":[{"role":"system","content":"Be concise"},{"role":"user","content":"hello"}],"max_tokens":16,"temperature":0.7,"top_p":0.9,"logprobs":true,"top_logprobs":3}`, &request))
			request.ReasoningEffort = tc.effort
			request.Model = origin
			require.NoError(t, helper.ModelMappedHelper(c, info, &request))
			converted, err := (&Adaptor{}).ConvertOpenAIRequest(c, info, &request)
			require.NoError(t, err)
			body, err := common.Marshal(converted)
			require.NoError(t, err)
			var fields map[string]any
			require.NoError(t, common.Unmarshal(body, &fields))
			wantModel := tc.wantModel
			if wantModel == "" {
				wantModel = tc.target
			}
			require.Equal(t, wantModel, fields["model"])
			require.Equal(t, wantModel, info.UpstreamModelName)
			require.Equal(t, tc.wantEffort, request.ReasoningEffort)
			if tc.adapt {
				require.Equal(t, float64(16), fields["max_completion_tokens"])
				for _, field := range []string{"max_tokens", "temperature", "top_p", "logprobs", "top_logprobs"} {
					require.NotContains(t, fields, field)
				}
				require.Equal(t, "developer", request.Messages[0].Role)
				require.Equal(t, "developer", request.GetSystemRoleName())
				require.Equal(t, tc.wantEffort, info.ReasoningEffort)
			} else {
				require.NotContains(t, fields, "max_completion_tokens")
				for _, field := range []string{"max_tokens", "temperature", "top_p", "logprobs", "top_logprobs"} {
					require.Contains(t, fields, field)
				}
				require.Equal(t, "system", request.Messages[0].Role)
			}
		})
	}
}

func TestGPT62CompletionLimitPreservesExplicitZero(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		want       *uint
	}{
		{name: "absent", body: `{}`},
		{name: "explicit max_tokens zero", body: `{"max_tokens":0}`, want: common.GetPointer(uint(0))},
		{name: "explicit completion zero wins", body: `{"max_tokens":16,"max_completion_tokens":0}`, want: common.GetPointer(uint(0))},
		{name: "completion limit wins", body: `{"max_tokens":16,"max_completion_tokens":32}`, want: common.GetPointer(uint(32))},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var request dto.GeneralOpenAIRequest
			require.NoError(t, common.UnmarshalJsonStr(tc.body, &request))
			info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gpt-6.2-sol"}}
			converted, err := (&Adaptor{}).ConvertOpenAIRequest(nil, info, &request)
			require.NoError(t, err)
			body, err := common.Marshal(converted)
			require.NoError(t, err)
			var fields map[string]any
			require.NoError(t, common.Unmarshal(body, &fields))
			require.NotContains(t, fields, "max_tokens")
			if tc.want == nil {
				require.NotContains(t, fields, "max_completion_tokens")
			} else {
				require.Equal(t, float64(*tc.want), fields["max_completion_tokens"])
			}
		})
	}
}

func TestGPT62OpenRouterReasoningNormalization(t *testing.T) {
	for _, tc := range []struct {
		name, model, origin, effort, wantEffort string
		wantReasoning                           map[string]any
	}{
		{name: "suffix", model: "gpt-6.2-sol-high", wantEffort: "high", wantReasoning: map[string]any{"enabled": true, "effort": "high"}},
		{name: "explicit", model: "gpt-6.2-sol", effort: "medium", wantEffort: "medium", wantReasoning: map[string]any{"enabled": true, "effort": "medium"}},
		{name: "mapped suffix", model: "gpt-6.2-sol", origin: "gpt-6.2-sol-high", wantEffort: "high", wantReasoning: map[string]any{"enabled": true, "effort": "high"}},
		{name: "thinking alias", model: "gpt-6.2-sol-thinking", effort: "low", wantEffort: "low", wantReasoning: map[string]any{"enabled": true, "effort": "low"}},
		{name: "none disables OpenRouter reasoning", model: "gpt-6.2-sol-none", wantEffort: "none", wantReasoning: map[string]any{"enabled": false}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			info := &relaycommon.RelayInfo{OriginModelName: tc.origin, ChannelMeta: &relaycommon.ChannelMeta{
				ChannelType: constant.ChannelTypeOpenRouter, UpstreamModelName: tc.model,
			}}
			request := &dto.GeneralOpenAIRequest{
				Model: tc.model, ReasoningEffort: tc.effort,
				MaxTokens: common.GetPointer(uint(16)), Temperature: common.GetPointer(0.7),
			}
			converted, err := (&Adaptor{}).ConvertOpenAIRequest(nil, info, request)
			require.NoError(t, err)
			body, err := common.Marshal(converted)
			require.NoError(t, err)
			var fields map[string]any
			require.NoError(t, common.Unmarshal(body, &fields))
			require.Equal(t, "gpt-6.2-sol", fields["model"])
			require.Equal(t, float64(16), fields["max_completion_tokens"])
			require.NotContains(t, fields, "temperature")
			require.NotContains(t, fields, "reasoning_effort")
			require.Equal(t, tc.wantEffort, info.ReasoningEffort)
			require.Equal(t, tc.wantReasoning, fields["reasoning"])
		})
	}
}

func TestGPT62OpenRouterNestedReasoningSuffix(t *testing.T) {
	for _, tc := range []struct {
		name, origin, target, effort, nested, wantModel, wantEffort, wantReasoning string
		preserveSuffix                                                             bool
	}{
		{name: "origin overrides nested effort", origin: "client-alias-high", target: "gpt-6.2-sol", nested: `{"enabled":true,"effort":"low","exclude":true,"summary":"auto"}`, wantModel: "gpt-6.2-sol", wantEffort: "high", wantReasoning: `{"enabled":true,"effort":"high","exclude":true,"summary":"auto"}`},
		{name: "mapped suffix overrides exact budget", target: "gpt-6.2-sol-low", nested: `{"enabled":true,"max_tokens":8192,"exclude":false}`, wantModel: "gpt-6.2-sol", wantEffort: "low", wantReasoning: `{"enabled":true,"effort":"low","exclude":false}`},
		{name: "origin overrides mapped and explicit effort", origin: "client-alias-high", target: "gpt-6.2-sol-low", effort: "medium", nested: `{"effort":"minimal"}`, wantModel: "gpt-6.2-sol", wantEffort: "high", wantReasoning: `{"enabled":true,"effort":"high"}`},
		{name: "none disables existing budget", target: "gpt-6.2-sol-none", nested: `{"enabled":true,"effort":"low","max_tokens":8192,"exclude":false}`, wantModel: "gpt-6.2-sol", wantEffort: "none", wantReasoning: `{"enabled":false,"exclude":false}`},
		{name: "suffix overrides explicit zero budget", target: "gpt-6.2-sol-high", nested: `{"enabled":false,"max_tokens":0}`, wantModel: "gpt-6.2-sol", wantEffort: "high", wantReasoning: `{"enabled":true,"effort":"high"}`},
		{name: "thinking alias retains origin precedence", origin: "client-alias-high", target: "gpt-6.2-sol-thinking", nested: `{"max_tokens":1024,"exclude":true}`, wantModel: "gpt-6.2-sol", wantEffort: "high", wantReasoning: `{"enabled":true,"effort":"high","exclude":true}`},
		{name: "blacklisted origin retains nested controls", origin: "client-alias-high", target: "gpt-6.2-sol", effort: "low", nested: `{"enabled":true,"effort":"low","exclude":false}`, preserveSuffix: true, wantModel: "gpt-6.2-sol", wantEffort: "low", wantReasoning: `{"enabled":true,"effort":"low","exclude":false}`},
		{name: "no suffix retains explicit zero budget", target: "gpt-6.2-sol", effort: "none", nested: `{"enabled":false,"max_tokens":0,"exclude":false}`, wantModel: "gpt-6.2-sol", wantEffort: "none", wantReasoning: `{"enabled":false,"max_tokens":0,"exclude":false}`},
		{name: "unknown family retains nested controls", target: "gpt-6.3-sol-high", nested: `{"enabled":true,"effort":"low"}`, wantModel: "gpt-6.3-sol-high", wantEffort: "low", wantReasoning: `{"enabled":true,"effort":"low"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			origin := tc.origin
			if origin == "" {
				origin = "client-alias"
			}
			settings := model_setting.GetGlobalSettings()
			previousBlacklist := settings.ThinkingModelBlacklist
			if tc.preserveSuffix {
				settings.ThinkingModelBlacklist = []string{origin}
			}
			t.Cleanup(func() { settings.ThinkingModelBlacklist = previousBlacklist })
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			mapping, err := common.Marshal(map[string]string{origin: tc.target})
			require.NoError(t, err)
			c.Set("model_mapping", string(mapping))
			info := &relaycommon.RelayInfo{OriginModelName: origin, ChannelMeta: &relaycommon.ChannelMeta{
				ChannelType: constant.ChannelTypeOpenRouter, UpstreamModelName: origin,
			}}
			request := &dto.GeneralOpenAIRequest{Model: origin, ReasoningEffort: tc.effort, Reasoning: []byte(tc.nested)}
			require.NoError(t, helper.ModelMappedHelper(c, info, request))
			converted, err := (&Adaptor{}).ConvertOpenAIRequest(c, info, request)
			require.NoError(t, err)
			body, err := common.Marshal(converted)
			require.NoError(t, err)
			var outbound dto.GeneralOpenAIRequest
			require.NoError(t, common.Unmarshal(body, &outbound))
			require.Equal(t, tc.wantModel, outbound.Model)
			require.Empty(t, outbound.ReasoningEffort)
			require.JSONEq(t, tc.wantReasoning, string(outbound.Reasoning))
			require.Equal(t, tc.wantEffort, info.ReasoningEffort)
		})
	}
}

func TestGPT62OpenRouterNestedEffortSurvivesMappingAndLogging(t *testing.T) {
	for _, model := range []string{"gpt-6.2-sol", "gpt-6.2-astra-2026-11-18"} {
		t.Run(model, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			var request dto.GeneralOpenAIRequest
			require.NoError(t, common.UnmarshalJsonStr(`{"model":"client-alias","messages":[{"role":"user","content":"hello"}],"reasoning":{"effort":"high","exclude":false}}`, &request))
			info := relaycommon.GenRelayInfoOpenAI(c, &request)
			require.Equal(t, "high", info.ReasoningEffort)
			info.OriginModelName = "client-alias"
			info.ChannelMeta = &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeOpenRouter, UpstreamModelName: "client-alias"}
			mapping, err := common.Marshal(map[string]string{"client-alias": model})
			require.NoError(t, err)
			c.Set("model_mapping", string(mapping))
			require.NoError(t, helper.ModelMappedHelper(c, info, &request))
			converted, err := (&Adaptor{}).ConvertOpenAIRequest(c, info, &request)
			require.NoError(t, err)
			body, err := common.Marshal(converted)
			require.NoError(t, err)
			var outbound dto.GeneralOpenAIRequest
			require.NoError(t, common.Unmarshal(body, &outbound))
			require.Equal(t, model, outbound.Model)
			require.JSONEq(t, `{"effort":"high","exclude":false}`, string(outbound.Reasoning))
			require.Empty(t, outbound.ReasoningEffort)
			other := service.GenerateTextOtherInfo(c, info, 1, 1, 1, 0, 1, 0, -1)
			require.Equal(t, "high", info.ReasoningEffort)
			require.Equal(t, "high", other["reasoning_effort"])
		})
	}
}

func TestGPT62OpenRouterClaudeReasoningSuffix(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	c.Set("model_mapping", `{"claude-sonnet-5-5-high":"gpt-6.2-sol"}`)
	info := &relaycommon.RelayInfo{OriginModelName: "claude-sonnet-5-5-high", ChannelMeta: &relaycommon.ChannelMeta{
		ChannelType: constant.ChannelTypeOpenRouter, UpstreamModelName: "claude-sonnet-5-5-high",
	}}
	request := &dto.ClaudeRequest{
		Model: info.OriginModelName, MaxTokens: common.GetPointer(uint(128)),
		Thinking: &dto.Thinking{Type: "enabled", BudgetTokens: common.GetPointer(2048), Display: "omitted"},
		Messages: []dto.ClaudeMessage{{Role: "user", Content: "hello"}},
	}
	require.NoError(t, helper.ModelMappedHelper(c, info, request))
	converted, err := (&Adaptor{}).ConvertClaudeRequest(c, info, request)
	require.NoError(t, err)
	body, err := common.Marshal(converted)
	require.NoError(t, err)
	var outbound dto.GeneralOpenAIRequest
	require.NoError(t, common.Unmarshal(body, &outbound))
	require.Equal(t, "gpt-6.2-sol", outbound.Model)
	require.JSONEq(t, `{"enabled":true,"effort":"high","exclude":true}`, string(outbound.Reasoning))
	require.Empty(t, outbound.ReasoningEffort)
	require.Equal(t, "high", info.ReasoningEffort)
}

func TestGPT62OpenRouterInvalidNestedReasoningIsClientError(t *testing.T) {
	for _, nested := range []string{`"invalid"`, `{"max_tokens":"invalid"}`, `{"effort":"invalid"}`, `{"exclude":"invalid"}`} {
		t.Run(nested, func(t *testing.T) {
			info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{
				ChannelType: constant.ChannelTypeOpenRouter, UpstreamModelName: "gpt-6.2-sol-high",
			}}
			request := &dto.GeneralOpenAIRequest{Model: info.UpstreamModelName, Reasoning: []byte(nested)}
			_, err := (&Adaptor{}).ConvertOpenAIRequest(nil, info, request)
			var apiErr *types.NewAPIError
			require.ErrorAs(t, err, &apiErr)
			require.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
			require.True(t, types.IsSkipRetryError(apiErr))
		})
	}
}

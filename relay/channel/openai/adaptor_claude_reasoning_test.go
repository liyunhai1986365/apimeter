package openai_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relay/channel/openai"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestClaudeMappedOpenAIReasoningControls(t *testing.T) {
	for _, tc := range []struct {
		name, source, target, wantEffort, wantReasoning string
		thinking                                        *dto.Thinking
		router                                          bool
	}{
		{name: "default medium", source: "claude-opus-5-5", target: "o3", wantEffort: "medium"},
		{name: "default omitted for nonreasoning target", source: "claude-opus-5-5", target: "gpt-4o"},
		{name: "o3 cannot disable", source: "claude-sonnet-5-5", target: "o3", thinking: &dto.Thinking{Type: "disabled"}, wantEffort: "low"},
		{name: "router default", source: "claude-opus-5-5", target: "anthropic/claude-opus-5-5", router: true, wantReasoning: `{"enabled":true,"effort":"medium"}`},
		{name: "router budget and hidden summary", source: "claude-sonnet-4-5", target: "anthropic/claude-sonnet-4-5", router: true, thinking: &dto.Thinking{Type: "enabled", BudgetTokens: common.GetPointer(2048), Display: "omitted"}, wantReasoning: `{"enabled":true,"max_tokens":2048,"exclude":true}`},
		{name: "router disabled", source: "claude-sonnet-5-5", target: "anthropic/claude-sonnet-5-5", router: true, thinking: &dto.Thinking{Type: "disabled"}, wantReasoning: `{"enabled":false}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
			channel := constant.ChannelTypeOpenAI
			if tc.router {
				channel = constant.ChannelTypeOpenRouter
			}
			info := &relaycommon.RelayInfo{
				OriginModelName: tc.source,
				ChannelMeta:     &relaycommon.ChannelMeta{ChannelType: channel, UpstreamModelName: tc.target},
			}
			req := &dto.ClaudeRequest{Model: tc.target, MaxTokens: common.GetPointer[uint](8192), Thinking: tc.thinking, Messages: []dto.ClaudeMessage{{Role: "user", Content: "hello"}}}
			adaptor := &openai.Adaptor{}
			adaptor.Init(info)
			result, err := adaptor.ConvertClaudeRequest(ctx, info, req)
			require.NoError(t, err)
			body, err := common.Marshal(result)
			require.NoError(t, err)
			require.Equal(t, tc.target, gjson.GetBytes(body, "model").String())
			require.Equal(t, tc.wantEffort, gjson.GetBytes(body, "reasoning_effort").String())
			if tc.wantReasoning == "" {
				require.False(t, gjson.GetBytes(body, "reasoning").Exists())
			} else {
				require.JSONEq(t, tc.wantReasoning, gjson.GetBytes(body, "reasoning").Raw)
			}
			require.False(t, gjson.GetBytes(body, "verbosity").Exists())
		})
	}
}

func TestClaudeInvalidEffortOnKnownOpenAITargetIsClientError(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	info := &relaycommon.RelayInfo{OriginModelName: "private-source", ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeOpenAI, UpstreamModelName: "o3"}}
	req := &dto.ClaudeRequest{Model: "o3", OutputConfig: json.RawMessage(`{"effort":"typo"}`), Messages: []dto.ClaudeMessage{{Role: "user", Content: "hello"}}}
	adaptor := &openai.Adaptor{}
	adaptor.Init(info)
	_, err := adaptor.ConvertClaudeRequest(ctx, info, req)
	require.ErrorContains(t, err, "unsupported reasoning effort")
	var apiErr *types.NewAPIError
	require.ErrorAs(t, err, &apiErr)
	require.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
	require.True(t, types.IsSkipRetryError(apiErr))
}

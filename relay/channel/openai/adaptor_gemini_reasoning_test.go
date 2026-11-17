package openai_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relay/channel/openai"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGeminiMappedOpenAIReasoningControls(t *testing.T) {
	for _, tc := range []struct {
		name, source, target, policy string
		budget                       *int
		wantEffort                   string
		wantReject                   bool
	}{
		{name: "default GPT-4o", source: "gemini-2.5-flash", target: "gpt-4o"},
		{name: "default Flash-Lite", source: "gemini-2.5-flash-lite", target: "gpt-4o"},
		{name: "strict default", source: "gemini-3-flash-preview", target: "gpt-4.1", policy: "strict"},
		{name: "supported target", source: "gemini-2.5-flash", target: "o3", wantEffort: "high"},
		{name: "safe explicit thinking", source: "gemini-2.5-flash", target: "gpt-4o", policy: "safe", budget: common.GetPointer(8192)},
		{name: "strict explicit thinking", source: "gemini-2.5-flash", target: "gpt-4o", policy: "strict", budget: common.GetPointer(8192), wantReject: true},
		{name: "strict explicit disable", source: "gemini-2.5-flash", target: "gpt-4o", policy: "strict", budget: common.GetPointer(0)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1beta/models/"+tc.source+":generateContent", nil)
			mapping, err := common.Marshal(map[string]string{tc.source: tc.target})
			require.NoError(t, err)
			c.Set("model_mapping", string(mapping))
			info := &relaycommon.RelayInfo{
				OriginModelName: tc.source,
				ChannelMeta: &relaycommon.ChannelMeta{
					ChannelType: constant.ChannelTypeOpenAI, UpstreamModelName: tc.source,
					ChannelOtherSettings: dto.ChannelOtherSettings{ToolLossPolicy: tc.policy},
				},
			}
			req := &dto.GeminiChatRequest{Contents: []dto.GeminiChatContent{{Role: "user", Parts: []dto.GeminiPart{{Text: "hello"}}}}}
			if tc.budget != nil {
				req.GenerationConfig.ThinkingConfig = &dto.GeminiThinkingConfig{ThinkingBudget: tc.budget}
			}
			require.NoError(t, helper.ModelMappedHelper(c, info, req))
			adaptor := &openai.Adaptor{}
			adaptor.Init(info)
			converted, err := adaptor.ConvertGeminiRequest(c, info, req)
			if tc.wantReject {
				var apiErr *types.NewAPIError
				require.ErrorAs(t, err, &apiErr)
				require.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
				require.True(t, types.IsSkipRetryError(apiErr))
				return
			}
			require.NoError(t, err)
			body, err := common.Marshal(converted)
			require.NoError(t, err)
			var fields map[string]any
			require.NoError(t, common.Unmarshal(body, &fields))
			require.Equal(t, tc.target, fields["model"])
			if tc.wantEffort == "" {
				require.NotContains(t, fields, "reasoning_effort")
			} else {
				require.Equal(t, tc.wantEffort, fields["reasoning_effort"])
			}
			require.Equal(t, tc.wantEffort, info.ReasoningEffort)
		})
	}
}

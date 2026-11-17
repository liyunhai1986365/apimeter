package claude

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestNativeClaudeFieldsSurviveCopyAndConversion(t *testing.T) {
	const body = `{"model":"claude-opus-5-5","max_tokens":64,"output_config":{"effort":"medium"},"thinking":{"type":"adaptive","display":"summarized","block_binding":{"prefix_mismatch_behavior":"drop_block"},"future_field":{"id":9007199254740993}},"messages":[{"role":"user","content":"summary"},{"role":"system","content":[],"output_config":{"effort":"high"}},{"role":"assistant","content":"done"}]}`
	var original dto.ClaudeRequest
	require.NoError(t, common.UnmarshalJsonStr(body, &original))
	outbound, err := common.DeepCopy(&original)
	require.NoError(t, err)
	out, err := (&Adaptor{}).ConvertClaudeRequest(nil, &relaycommon.RelayInfo{}, outbound)
	require.NoError(t, err)
	encoded, err := common.Marshal(out)
	require.NoError(t, err)
	require.JSONEq(t, body, string(encoded))
	require.Equal(t, "9007199254740993", gjson.GetBytes(encoded, "thinking.future_field.id").Raw)
	require.False(t, gjson.GetBytes(encoded, "messages.0.output_config").Exists())
	require.False(t, gjson.GetBytes(encoded, "messages.2.output_config").Exists())
}

func TestSetupRequestHeaderForwardsEveryAnthropicBetaValue(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	c.Request.Header.Add("anthropic-beta", "fast-mode-2026-02-01")
	c.Request.Header.Add("anthropic-beta", "thinking-binding-controls-2026-08-01")
	info := &relaycommon.RelayInfo{
		OriginModelName: "claude-opus-5-5",
		ChannelMeta:     &relaycommon.ChannelMeta{ApiKey: "test-key"},
	}
	header := http.Header{}
	require.NoError(t, (&Adaptor{}).SetupRequestHeader(c, &header, info))
	require.Equal(t, []string{"fast-mode-2026-02-01,thinking-binding-controls-2026-08-01"}, header.Values("anthropic-beta"))
}

func TestLegacyClaudeConverterPreservesAdjacentAssistantTools(t *testing.T) {
	var request dto.GeneralOpenAIRequest
	require.NoError(t, common.UnmarshalJsonStr(`{"model":"claude-sonnet-4-5","max_tokens":1024,"messages":[{"role":"user","content":"look up a and b"},{"role":"assistant","content":"","tool_calls":[{"id":"a","type":"function","function":{"name":"lookup","arguments":"{}"}}]},{"role":"assistant","content":"","tool_calls":[{"id":"b","type":"function","function":{"name":"lookup","arguments":"{}"}}]},{"role":"tool","tool_call_id":"a","content":"result a"},{"role":"tool","tool_call_id":"b","content":"result b"}]}`, &request))
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	converted, err := RequestOpenAI2ClaudeMessage(c, request)
	require.NoError(t, err)
	require.Len(t, converted.Messages, 3)
	encoded, err := common.Marshal(converted)
	require.NoError(t, err)
	var ids []string
	for _, block := range gjson.GetBytes(encoded, "messages.1.content").Array() {
		if block.Get("type").String() == "tool_use" {
			ids = append(ids, block.Get("id").String())
		}
	}
	require.Equal(t, []string{"a", "b"}, ids)
	require.Equal(t, "a", gjson.GetBytes(encoded, "messages.2.content.0.tool_use_id").String())
	require.Equal(t, "b", gjson.GetBytes(encoded, "messages.2.content.1.tool_use_id").String())
}

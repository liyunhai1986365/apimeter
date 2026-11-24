package relayconvert

import (
	"encoding/json"
	"testing"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/stretchr/testify/require"
)

func TestClaudeStructuredOutputSurvivesChatAndGeminiConversion(t *testing.T) {
	const model = "claude-sonnet-4-6"
	const schema = `{"type":"object","properties":{"n":{"type":"integer","const":9007199254740993}},"required":["n"],"additionalProperties":false}`
	for _, source := range []string{"chat", "gemini"} {
		t.Run(source, func(t *testing.T) {
			info := googleInfo(model)
			info.Options.ToolLossPolicy = types.ConversionLossPolicyStrict
			var req any
			if source == "chat" {
				var chat dto.GeneralOpenAIRequest
				require.NoError(t, kitutil.UnmarshalJsonStr(`{"model":"`+model+`","max_tokens":128,"reasoning_effort":"high","tools":[{"type":"function","function":{"name":"lookup","parameters":{"type":"object","properties":{"q":{"type":"string"}}}}}],"response_format":{"type":"json_schema","json_schema":{"name":"answer","strict":true,"schema":`+schema+`}}}`, &chat))
				req = &chat
			} else {
				info.OriginModelName = "gemini-2.0-flash"
				var gemini dto.GeminiChatRequest
				require.NoError(t, kitutil.UnmarshalJsonStr(`{"contents":[{"role":"user","parts":[{"text":"answer"}]}],"generationConfig":{"maxOutputTokens":128,"responseJsonSchema":`+schema+`,"responseMimeType":"application/json"}}`, &gemini))
				req = &gemini
			}
			out, err := ConvertRequest(nil, info, types.RelayFormatClaude, req)
			require.NoError(t, err)
			require.Empty(t, out.Diagnostics)
			dest := out.Value.(*dto.ClaudeRequest)
			require.Equal(t, uint(128), *dest.MaxTokens)
			var config struct {
				Effort string                     `json:"effort"`
				Format map[string]json.RawMessage `json:"format"`
			}
			require.NoError(t, kitutil.Unmarshal(dest.OutputConfig, &config))
			require.Len(t, config.Format, 2)
			require.Equal(t, `"json_schema"`, string(config.Format["type"]))
			require.Equal(t, schema, string(config.Format["schema"]))
			if source == "chat" {
				require.Equal(t, "high", config.Effort)
				toolWire, err := kitutil.Marshal(dest.Tools)
				require.NoError(t, err)
				require.Contains(t, string(toolWire), `"q":{"type":"string"}`)
			}
		})
	}
}

func TestClaudeUnrepresentableChatResponseFormatFollowsPolicy(t *testing.T) {
	for _, policy := range []types.ConversionLossPolicy{types.ConversionLossPolicyAllow, types.ConversionLossPolicySafe, types.ConversionLossPolicyStrict} {
		for _, format := range []*dto.ResponseFormat{nil, {Type: "text"}, {Type: "json_object"}, {Type: "audio"}, {Type: "json_schema", JsonSchema: []byte(`{"schema":true}`)}} {
			info := googleInfo("claude-sonnet-4-6")
			info.Options.ToolLossPolicy = policy
			req := &dto.GeneralOpenAIRequest{Model: info.UpstreamModelName, MaxTokens: kitutil.GetPointer(uint(64)), ResponseFormat: format}
			out, err := ConvertRequest(nil, info, types.RelayFormatClaude, req)
			loss := format != nil && format.Type != "text"
			if loss && policy != types.ConversionLossPolicyAllow {
				var rejected *types.ConversionLossError
				require.ErrorAs(t, err, &rejected)
				require.Equal(t, "response_format", rejected.Diagnostics[0].Path)
				continue
			}
			require.NoError(t, err)
			require.Empty(t, out.Value.(*dto.ClaudeRequest).OutputConfig)
			if loss {
				require.Len(t, out.Diagnostics, 1)
				require.Equal(t, "unsupported_response_format", out.Diagnostics[0].Code)
			} else {
				require.Empty(t, out.Diagnostics)
			}
		}
	}
}

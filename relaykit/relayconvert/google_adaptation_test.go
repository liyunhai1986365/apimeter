package relayconvert

import (
	"context"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/convmeta"
	kitutil "github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/stretchr/testify/require"
)

func googleInfo(model string) *convmeta.Values {
	return &convmeta.Values{UpstreamModelName: model, OriginModelName: model, ChannelMetaAttached: true,
		Options: &convmeta.Options{Gemini: convmeta.GeminiOptions{ThinkingAdapterEnabled: true, ThinkingAdapterBudgetTokensPercentage: .6, SupportsJSONSchema: func(string) bool { return true }}},
	}
}

func TestGoogleThinkingAndExplicitZeroControls(t *testing.T) {
	for _, tc := range []struct {
		model, extra, effort string
		budget               *int
		level                string
		include              *bool
	}{
		{model: "gemini-2.5-flash", extra: `{"google":{"thinking_config":{"include_thoughts":false,"thinking_budget":0}}}`, budget: kitutil.GetPointer(0), include: kitutil.GetPointer(false)},
		{model: "gemini-3-flash-preview", extra: `{"google":{"thinking_config":{"thinking_level":"LOW","include_thoughts":false}}}`, level: "low", include: kitutil.GetPointer(false)},
		{model: "gemini-2.5-flash", effort: "HIGH", budget: kitutil.GetPointer(24576)},
		{model: "gemini-3-flash-preview-high", effort: "low", level: "high"},
		{model: "gemini-4-argon-pro", effort: "high", level: "high"},
	} {
		t.Run(tc.model+tc.effort, func(t *testing.T) {
			request := &dto.GeneralOpenAIRequest{Model: tc.model, ExtraBody: []byte(tc.extra), ReasoningEffort: tc.effort, TopP: kitutil.GetPointer(0.0), Seed: kitutil.GetPointer(0.0)}
			result, err := ConvertRequest(context.Background(), googleInfo(tc.model), types.RelayFormatGemini, request)
			require.NoError(t, err)
			config := result.Value.(*dto.GeminiChatRequest).GenerationConfig
			require.NotNil(t, config.TopP)
			require.Zero(t, *config.TopP)
			require.NotNil(t, config.Seed)
			require.Zero(t, *config.Seed)
			require.NotNil(t, config.ThinkingConfig)
			require.Equal(t, tc.budget, config.ThinkingConfig.ThinkingBudget)
			require.Equal(t, tc.level, config.ThinkingConfig.ThinkingLevel)
			if tc.include != nil {
				require.Equal(t, tc.include, config.ThinkingConfig.IncludeThoughts)
			}
		})
	}
	info := googleInfo("gemini-3-flash-preview")
	request := &dto.GeminiChatRequest{GenerationConfig: dto.GeminiChatGenerationConfig{ThinkingConfig: &dto.GeminiThinkingConfig{ThinkingLevel: "FUTURE_LEVEL", IncludeThoughts: kitutil.GetPointer(false)}}}
	require.NoError(t, ApplyGeminiThinkingConfig(request, info))
	require.Equal(t, "FUTURE_LEVEL", request.GenerationConfig.ThinkingConfig.ThinkingLevel)
	_, err := ConvertRequest(nil, info, types.RelayFormatGemini, &dto.GeneralOpenAIRequest{Model: info.UpstreamModelName, ReasoningEffort: "typo"})
	require.Error(t, err)
}

func TestGoogleSchemasPreserveConstraintsAndPropertyOrder(t *testing.T) {
	for _, responses := range []bool{false, true} {
		for _, beta := range []bool{false, true} {
			info := googleInfo("gemini-2.5-flash")
			info.Options.Gemini.SupportsJSONSchema = func(string) bool { return beta }
			var request any
			schema := `{"type":"object","properties":{"z":{"const":0},"a":{"oneOf":[{"type":"string"},{"type":"number"}]}}}`
			if responses {
				var req dto.OpenAIResponsesRequest
				require.NoError(t, kitutil.UnmarshalJsonStr(`{"model":"gemini-2.5-flash","tools":[{"type":"function","name":"f","parameters":`+schema+`}],"text":{"format":{"type":"json_schema","name":"out","schema":`+schema+`}}}`, &req))
				request = &req
			} else {
				var req dto.GeneralOpenAIRequest
				require.NoError(t, kitutil.UnmarshalJsonStr(`{"model":"gemini-2.5-flash","tools":[{"type":"function","function":{"name":"f","parameters":`+schema+`}}],"response_format":{"type":"json_schema","json_schema":{"name":"out","schema":`+schema+`}}}`, &req))
				request = &req
			}
			result, err := ConvertRequest(nil, info, types.RelayFormatGemini, request)
			require.NoError(t, err)
			out := result.Value.(*dto.GeminiChatRequest)
			encoded, err := kitutil.Marshal(out)
			require.NoError(t, err)
			if beta {
				require.Contains(t, string(encoded), `"parametersJsonSchema":`+schema)
				require.Contains(t, string(encoded), `"responseJsonSchema":`+schema)
			} else {
				require.NotContains(t, string(encoded), `parametersJsonSchema`)
				require.NotEmpty(t, result.Diagnostics)
			}
		}
	}
}

func TestGoogleResponsesCustomToolsRoundTrip(t *testing.T) {
	info := googleInfo("gemini-3-flash-preview")
	var request dto.OpenAIResponsesRequest
	require.NoError(t, kitutil.UnmarshalJsonStr(`{"model":"gemini-3-flash-preview","tools":[{"type":"custom","name":"exec"}],"tool_choice":{"type":"custom","name":"exec"},"input":[{"type":"custom_tool_call","call_id":"call_1","name":"exec","input":"ls -al"},{"type":"custom_tool_call_output","call_id":"call_1","output":"ok"}]}`, &request))
	result, err := ConvertRequest(nil, info, types.RelayFormatGemini, &request)
	require.NoError(t, err)
	gemini := result.Value.(*dto.GeminiChatRequest)
	require.Equal(t, []string{"exec"}, gemini.ToolConfig.FunctionCallingConfig.AllowedFunctionNames)
	chat := ResponseGeminiChat2OpenAI("resp_1", 1, &dto.GeminiChatResponse{Candidates: []dto.GeminiChatCandidate{{Content: dto.GeminiChatContent{Parts: []dto.GeminiPart{{FunctionCall: &dto.FunctionCall{ID: "call_2", FunctionName: "exec", Arguments: map[string]any{"input": "echo hi"}}}}}}}})
	converted, err := ConvertResponse(nil, info, types.RelayFormatOpenAIResponses, chat)
	require.NoError(t, err)
	out := converted.Value.(*dto.OpenAIResponsesResponse)
	require.Len(t, out.Output, 1)
	require.Equal(t, "custom_tool_call", out.Output[0].Type)
	require.Equal(t, "echo hi", *out.Output[0].Input)
	state, err := NewResponseStreamState(types.RelayFormatOpenAI, types.RelayFormatOpenAIResponses, ResponseStreamOptions{ID: "resp_1", Model: info.UpstreamModelName})
	require.NoError(t, err)
	for _, args := range []string{`{"input":"echo `, `hi"}`} {
		_, err = ConvertStreamResponseChunk(nil, info, state, &dto.ChatCompletionsStreamResponse{Choices: []dto.ChatCompletionsStreamResponseChoice{{Delta: dto.ChatCompletionsStreamResponseChoiceDelta{ToolCalls: []dto.ToolCallResponse{{ID: "call_2", Type: "function", Index: kitutil.GetPointer(0), Function: dto.FunctionResponse{Name: "exec", Arguments: args}}}}}}})
		require.NoError(t, err)
	}
	events, err := FinalizeStreamResponse(nil, info, state)
	require.NoError(t, err)
	var final *dto.OpenAIResponsesResponse
	for _, event := range events {
		e := event.Value.(ChatToResponsesStreamEvent)
		if e.Type == "response.completed" {
			final = e.Payload.Response
		}
	}
	require.NotNil(t, final)
	require.Equal(t, "custom_tool_call", final.Output[0].Type)
	require.Equal(t, "echo hi", *final.Output[0].Input)
}

func TestGoogleResponsesToolOutputMedia(t *testing.T) {
	for _, model := range []string{"gemini-2.5-flash", "gemini-3-flash-preview"} {
		var request dto.OpenAIResponsesRequest
		require.NoError(t, kitutil.UnmarshalJsonStr(`{"model":"`+model+`","input":[{"type":"function_call","call_id":"c1","name":"see","arguments":"{}"},{"type":"function_call_output","call_id":"c1","output":[{"type":"input_text","text":"look"},{"type":"input_image","image_url":"data:image/png;base64,aGVsbG8="}]}]}`, &request))
		result, err := ConvertRequest(nil, googleInfo(model), types.RelayFormatGemini, &request)
		require.NoError(t, err)
		parts := result.Value.(*dto.GeminiChatRequest).Contents[1].Parts
		encoded, err := kitutil.Marshal(parts[0].FunctionResponse.Response)
		require.NoError(t, err)
		require.NotContains(t, string(encoded), "aGVsbG8=")
		if strings.HasPrefix(model, "gemini-3") {
			require.Len(t, parts, 1)
			require.Contains(t, string(parts[0].FunctionResponse.Parts), "aGVsbG8=")
		} else {
			require.Len(t, parts, 2)
			require.Equal(t, "aGVsbG8=", parts[1].InlineData.Data)
		}
	}
}

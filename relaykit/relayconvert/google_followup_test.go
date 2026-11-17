package relayconvert

import (
	"github.com/QuantumNous/new-api/relaykit/dto"
	kitutil "github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

func TestGoogleNamespacesRoundTrip(t *testing.T) {
	for _, kind := range []string{"function", "custom"} {
		t.Run(kind, func(t *testing.T) {
			info := googleInfo("gemini-3-flash-preview")
			var req dto.OpenAIResponsesRequest
			require.NoError(t, kitutil.UnmarshalJsonStr(`{"model":"gemini-3-flash-preview","tools":[{"type":"namespace","name":"shell","tools":[{"type":"`+kind+`","name":"run","parameters":{"type":"object","properties":{"z":{"const":0},"a":{"const":1}}}}]}],"tool_choice":{"type":"`+kind+`","namespace":"shell","name":"run"},"input":[{"type":"`+kind+`_call","name":"run","namespace":"shell","call_id":"c1","arguments":"{}","input":"ls"}]}`, &req))
			if kind == "custom" {
				req.Input = []byte(`[{"type":"custom_tool_call","name":"run","namespace":"shell","call_id":"c1","input":"ls"}]`)
			}
			result, err := ConvertRequest(nil, info, types.RelayFormatGemini, &req)
			require.NoError(t, err)
			out := result.Value.(*dto.GeminiChatRequest)
			require.Equal(t, []string{"shell__run"}, out.ToolConfig.FunctionCallingConfig.AllowedFunctionNames)
			require.Equal(t, "shell__run", out.Contents[0].Parts[0].FunctionCall.FunctionName)
			if kind == "function" {
				require.True(t, strings.Index(string(out.Tools), `"z"`) < strings.Index(string(out.Tools), `"a"`))
			}
			source := &dto.GeminiChatResponse{Candidates: []dto.GeminiChatCandidate{{Content: dto.GeminiChatContent{Parts: []dto.GeminiPart{{FunctionCall: &dto.FunctionCall{ID: "c2", FunctionName: "shell__run", Arguments: map[string]any{"input": "pwd"}}}}}}}}
			resultResp, err := ConvertResponse(nil, info, types.RelayFormatOpenAIResponses, source)
			require.NoError(t, err)
			item := resultResp.Value.(*dto.OpenAIResponsesResponse).Output[0]
			require.Equal(t, "shell", item.Namespace)
			require.Equal(t, "run", item.Name)
			state, err := NewResponseStreamState(types.RelayFormatGemini, types.RelayFormatOpenAIResponses, ResponseStreamOptions{ID: "r", Model: info.UpstreamModelName})
			require.NoError(t, err)
			_, err = ConvertStreamResponseChunk(nil, info, state, source)
			require.NoError(t, err)
			events, err := FinalizeStreamResponse(nil, info, state)
			require.NoError(t, err)
			final := events[len(events)-1].Value.(ChatToResponsesStreamEvent).Payload.Response
			require.Equal(t, item.Namespace, final.Output[0].Namespace)
			require.Equal(t, item.Name, final.Output[0].Name)
			require.Equal(t, item.Type, final.Output[0].Type)
		})
	}
}
func TestGoogleToolConflictsAndLossPolicies(t *testing.T) {
	var req dto.OpenAIResponsesRequest
	require.NoError(t, kitutil.UnmarshalJsonStr(`{"model":"gemini-3-flash-preview","tools":[{"type":"custom","name":"run"},{"type":"function","name":"run"},{"type":"function","name":"run"},{"type":"namespace","name":"functions","tools":[{"type":"function","name":"other"}]}]}`, &req))
	info := googleInfo(req.Model)
	result, err := ConvertRequest(nil, info, types.RelayFormatGemini, &req)
	require.NoError(t, err)
	var groups []struct {
		Functions []map[string]any `json:"functionDeclarations"`
	}
	require.NoError(t, kitutil.Unmarshal(result.Value.(*dto.GeminiChatRequest).Tools, &groups))
	require.Len(t, groups[0].Functions, 2)
	require.False(t, info.ResponsesToolState().IsCustomTool("run"))
	require.NotEmpty(t, result.Diagnostics)
	info.Options.ToolLossPolicy = types.ConversionLossPolicySafe
	_, err = ConvertRequest(nil, info, types.RelayFormatGemini, &req)
	require.Error(t, err)
	req.Tools = []byte(`[{"type":"custom","name":"execute"}]`)
	_, err = ConvertRequest(nil, info, types.RelayFormatGemini, &req)
	require.NoError(t, err)
	info.Options.ToolLossPolicy = types.ConversionLossPolicyStrict
	_, err = ConvertRequest(nil, info, types.RelayFormatGemini, &req)
	require.Error(t, err)
	info.Options.ToolLossPolicy = types.ConversionLossPolicySafe
	req.Tools = []byte(`[{"type":"web_search","filters":{"allowed_domains":["example.com"]}}]`)
	_, err = ConvertRequest(nil, info, types.RelayFormatGemini, &req)
	require.Error(t, err)
}
func TestGoogleReverseHistoryMediaAndZero(t *testing.T) {
	var req dto.GeminiChatRequest
	require.NoError(t, kitutil.UnmarshalJsonStr(`{"generationConfig":{"topP":0,"topK":0,"maxOutputTokens":0,"thinkingConfig":{"thinkingBudget":0}},"contents":[{"role":"model","parts":[{"text":"hidden","thought":true},{"text":"visible"},{"functionCall":{"id":"kept","name":"a","args":{}}},{"functionCall":{"name":"b","args":{}}}]},{"role":"user","parts":[{"functionResponse":{"name":"b","response":{"output":"b"}}},{"functionResponse":{"id":"kept","name":"a","response":{"output":"a"}}},{"inlineData":{"mimeType":"text/plain","data":"aGk="}},{"inlineData":{"mimeType":"audio/wav","data":"AA=="}}]}],"tools":[{"functionDeclarations":[{"name":"a","parameters":{"type":"OBJECT","properties":{"x":{"type":"STRING","nullable":true}}}}]}]}`, &req))
	info := googleInfo("gemini-2.5-flash")
	info.UpstreamModelName = "gpt-4o"
	result, err := ConvertRequest(nil, info, types.RelayFormatOpenAI, &req)
	require.NoError(t, err)
	out := result.Value.(*dto.GeneralOpenAIRequest)
	require.Empty(t, out.ReasoningEffort, "gpt-4o needs no reasoning parameter to keep thinking disabled")
	require.Equal(t, 0.0, *out.TopP)
	require.Zero(t, *out.TopK)
	require.Zero(t, *out.MaxTokens)
	calls := out.Messages[0].ParseToolCalls()
	require.Equal(t, "kept", calls[0].ID)
	require.Equal(t, calls[1].ID, out.Messages[1].ToolCallId)
	require.Equal(t, "kept", out.Messages[2].ToolCallId)
	require.Equal(t, "hidden", out.Messages[0].GetReasoningContent())
	require.Equal(t, "visible", out.Messages[0].StringContent())
	parts := out.Messages[3].ParseContent()
	require.Equal(t, "hi", parts[0].Text)
	require.Equal(t, dto.ContentTypeInputAudio, parts[1].Type)
	encoded, err := kitutil.Marshal(out.Tools)
	require.NoError(t, err)
	require.Contains(t, string(encoded), `"type":["string","null"]`)
}
func TestGoogleSearchOutputLateAndDeduplicated(t *testing.T) {
	info := googleInfo("gemini-3-flash-preview")
	source := &dto.GeminiChatResponse{Candidates: []dto.GeminiChatCandidate{{GroundingMetadata: &dto.GeminiGroundingMetadata{WebSearchQueries: []string{" cat ", "cat", "dog", ""}}}}}
	result, err := ConvertResponse(nil, info, types.RelayFormatOpenAIResponses, source)
	require.NoError(t, err)
	item := result.Value.(*dto.OpenAIResponsesResponse).Output[0]
	require.Equal(t, "web_search_call", item.Type)
	require.JSONEq(t, `{"type":"search","queries":["cat","dog"]}`, string(item.Action))
	state, err := NewResponseStreamState(types.RelayFormatGemini, types.RelayFormatOpenAIResponses, ResponseStreamOptions{ID: "r", Model: info.UpstreamModelName})
	require.NoError(t, err)
	for range 2 {
		_, err = ConvertStreamResponseChunk(nil, info, state, source)
		require.NoError(t, err)
	}
	events, err := FinalizeStreamResponse(nil, info, state)
	require.NoError(t, err)
	var done int
	for _, e := range events {
		event := e.Value.(ChatToResponsesStreamEvent)
		if event.Type == "response.web_search_call.completed" {
			done++
		}
	}
	require.Equal(t, 1, done)
	final := events[len(events)-1].Value.(ChatToResponsesStreamEvent).Payload.Response
	require.Len(t, final.Output, 1)
	require.Equal(t, item.Action, final.Output[0].Action)
	again, err := FinalizeStreamResponse(nil, info, state)
	require.NoError(t, err)
	require.Empty(t, again)
}

package relayconvert

import (
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/convmeta"
	kitutil "github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/stretchr/testify/require"
)

func TestParallelToolCallsJSONAndConversionRoundTrip(t *testing.T) {
	for _, value := range []string{"", "true", "false"} {
		t.Run("value="+value, func(t *testing.T) {
			body := `{"model":"gemini-3-flash-preview","max_tokens":100,"messages":[{"role":"user","content":"hello"}]`
			if value != "" {
				body += `,"parallel_tool_calls":` + value
			}
			body += `}`
			var req dto.GeneralOpenAIRequest
			require.NoError(t, kitutil.UnmarshalJsonStr(body, &req))
			check := func(v any) {
				t.Helper()
				encoded, err := kitutil.Marshal(v)
				require.NoError(t, err)
				var fields map[string]any
				require.NoError(t, kitutil.Unmarshal(encoded, &fields))
				if value == "" {
					require.NotContains(t, fields, "parallel_tool_calls")
				} else {
					require.Equal(t, value == "true", fields["parallel_tool_calls"])
				}
			}
			check(&req)
			info := googleInfo(req.Model)
			responses, err := ConvertRequest(nil, info, types.RelayFormatOpenAIResponses, &req)
			require.NoError(t, err)
			check(responses.Value)
			chat, err := ConvertRequest(nil, info, types.RelayFormatOpenAI, responses.Value)
			require.NoError(t, err)
			check(chat.Value)
			claude, err := ConvertRequest(nil, info, types.RelayFormatClaude, &req)
			require.NoError(t, err)
			encoded, err := kitutil.Marshal(claude.Value)
			require.NoError(t, err)
			if value == "false" {
				require.Contains(t, string(encoded), `"disable_parallel_tool_use":true`)
			}
			info.Options.ToolLossPolicy = types.ConversionLossPolicySafe
			_, err = ConvertRequest(nil, info, types.RelayFormatGemini, &req)
			require.NoError(t, err, "without tools there is no parallel function calling to restrict")
		})
	}
}

func TestGoogleThinkingAliasesPreserveThoughtOutput(t *testing.T) {
	for _, model := range []string{"gemini-2.5-flash-thinking", "gemini-2.5-flash-thinking-1024", "gemini-3-flash-preview-high"} {
		for _, explicitFalse := range []bool{false, true} {
			t.Run(model+"/explicitFalse="+strconv.FormatBool(explicitFalse), func(t *testing.T) {
				req := &dto.GeneralOpenAIRequest{Model: model}
				if explicitFalse {
					req.ExtraBody = []byte(`{"google":{"thinking_config":{"include_thoughts":false}}}`)
				}
				out, err := ConvertRequest(nil, googleInfo(model), types.RelayFormatGemini, req)
				require.NoError(t, err)
				cfg := out.Value.(*dto.GeminiChatRequest).GenerationConfig.ThinkingConfig
				require.NotNil(t, cfg)
				require.Equal(t, kitutil.GetPointer(!explicitFalse), cfg.IncludeThoughts)
				native := &dto.GeminiChatRequest{}
				if explicitFalse {
					native.GenerationConfig.ThinkingConfig = &dto.GeminiThinkingConfig{IncludeThoughts: kitutil.GetPointer(false)}
				}
				require.NoError(t, ApplyGeminiThinkingConfig(native, googleInfo(model)))
				require.Equal(t, kitutil.GetPointer(!explicitFalse), native.GenerationConfig.ThinkingConfig.IncludeThoughts)
			})
		}
	}
	plain := &dto.GeminiChatRequest{}
	require.NoError(t, ApplyGeminiThinkingConfig(plain, googleInfo("gemini-3-flash-preview")))
	require.Nil(t, plain.GenerationConfig.ThinkingConfig)
}

func TestGoogleResponsesAutoChoiceWithoutFunctions(t *testing.T) {
	for _, policy := range []types.ConversionLossPolicy{types.ConversionLossPolicySafe, types.ConversionLossPolicyStrict} {
		for _, tools := range []string{`[]`, `[{"type":"web_search"}]`} {
			info := googleInfo("gemini-3-flash-preview")
			info.Options.ToolLossPolicy = policy
			req := &dto.OpenAIResponsesRequest{Model: info.UpstreamModelName, Tools: []byte(tools), ToolChoice: []byte(`"auto"`)}
			out, err := ConvertRequest(nil, info, types.RelayFormatGemini, req)
			require.NoError(t, err, "%s %s", policy, tools)
			require.Empty(t, out.Diagnostics)
			if tools != `[]` {
				require.Contains(t, string(out.Value.(*dto.GeminiChatRequest).Tools), "googleSearch")
			}
			req.ToolChoice = []byte(`"required"`)
			_, err = ConvertRequest(nil, info, types.RelayFormatGemini, req)
			require.Error(t, err)
		}
	}
}

func TestGoogleResponsesNoneChoiceDisablesAllTools(t *testing.T) {
	for _, policy := range []types.ConversionLossPolicy{types.ConversionLossPolicyAllow, types.ConversionLossPolicySafe, types.ConversionLossPolicyStrict} {
		for _, tools := range []string{
			``, `[]`, `[{"type":"web_search"}]`,
			`[{"type":"function","name":"lookup","parameters":{"type":"object","properties":{}}}]`,
			`[{"type":"web_search"},{"type":"function","name":"lookup","parameters":{"type":"object","properties":{}}}]`,
		} {
			t.Run(string(policy)+"/"+tools, func(t *testing.T) {
				info := googleInfo("gemini-3-flash-preview")
				info.Options.ToolLossPolicy = policy
				req := &dto.OpenAIResponsesRequest{Model: info.UpstreamModelName, Tools: []byte(tools), ToolChoice: []byte(` "none" `)}
				out, err := ConvertRequest(nil, info, types.RelayFormatGemini, req)
				require.NoError(t, err)
				require.Empty(t, out.Diagnostics)
				converted := out.Value.(*dto.GeminiChatRequest)
				for _, tool := range converted.GetTools() {
					require.Nil(t, tool.GoogleSearch)
					require.NotNil(t, tool.FunctionDeclarations)
				}
				if len(converted.GetTools()) > 0 {
					require.NotNil(t, converted.ToolConfig)
					body, err := kitutil.Marshal(converted.ToolConfig)
					require.NoError(t, err)
					require.JSONEq(t, `{"functionCallingConfig":{"mode":"NONE"}}`, string(body))
				} else {
					require.Nil(t, converted.ToolConfig)
				}
				// Conversion does not mutate the caller's request.
				require.Equal(t, tools, string(req.Tools))
			})
		}
	}
}

func TestGoogleResponsesNoneIgnoresDisabledExecutionConstraints(t *testing.T) {
	for _, policy := range []types.ConversionLossPolicy{types.ConversionLossPolicySafe, types.ConversionLossPolicyStrict} {
		for _, tools := range []string{
			`[]`,
			`[{"type":"web_search","filters":{"allowed_domains":["example.com"]},"external_web_access":false,"max_uses":1}]`,
			`[{"type":"file_search","vector_store_ids":["vs_example"]}]`,
			`[{"type":"function","name":"lookup","strict":true,"parameters":{"type":"object","properties":{"count":{"type":"integer","enum":[1,2]}}}}]`,
			`[{"type":"custom","name":"grammar","format":{"type":"grammar","syntax":"lark","definition":"start: /[a-z]+/"}}]`,
			`[{"type":"namespace","name":"project","tools":[{"type":"custom","name":"lookup"}]}]`,
		} {
			t.Run(string(policy)+"/"+tools, func(t *testing.T) {
				info := googleInfo("gemini-2.0-flash")
				info.Options.ToolLossPolicy = policy
				info.Options.Gemini.SupportsJSONSchema = func(string) bool { return false }
				req := &dto.OpenAIResponsesRequest{Model: info.UpstreamModelName, Tools: []byte(tools), ToolChoice: []byte(`"none"`), ParallelToolCalls: []byte(`false`)}
				out, err := ConvertRequest(nil, info, types.RelayFormatGemini, req)
				require.NoError(t, err)
				require.Empty(t, out.Diagnostics)
				converted := out.Value.(*dto.GeminiChatRequest)
				for _, tool := range converted.GetTools() {
					require.Nil(t, tool.GoogleSearch)
					require.NotNil(t, tool.FunctionDeclarations)
					require.Equal(t, dto.FunctionCallingConfigMode("NONE"), converted.ToolConfig.FunctionCallingConfig.Mode)
				}
			})
		}
	}
}

func TestGoogleResponsesNonePreservesToolHistory(t *testing.T) {
	info := googleInfo("gemini-3-flash-preview")
	info.Options.ToolLossPolicy = types.ConversionLossPolicyStrict
	req := &dto.OpenAIResponsesRequest{
		Model: info.UpstreamModelName, ToolChoice: []byte(`"none"`), ParallelToolCalls: []byte(`false`),
		Tools: []byte(`[{"type":"namespace","name":"project","tools":[{"type":"custom","name":"lookup","format":{"type":"grammar"}}]}]`),
		Input: []byte(`[{"type":"custom_tool_call","namespace":"project","name":"lookup","call_id":"call_1","input":"hello"},{"type":"custom_tool_call_output","call_id":"call_1","output":"world"}]`),
	}
	out, err := ConvertRequest(nil, info, types.RelayFormatGemini, req)
	require.NoError(t, err)
	require.Empty(t, out.Diagnostics)
	converted := out.Value.(*dto.GeminiChatRequest)
	require.Len(t, converted.Contents, 2)
	call := converted.Contents[0].Parts[0].FunctionCall
	require.Equal(t, "project__lookup", call.FunctionName)
	require.Equal(t, map[string]any{convmeta.CustomToolInputArgument: "hello"}, call.Arguments)
	result := converted.Contents[1].Parts[0].FunctionResponse
	require.Equal(t, call.FunctionName, result.Name)
	state := convmeta.ResponsesToolStateOf(info)
	require.True(t, state.IsCustomTool(call.FunctionName))
	namespace, name := state.ResponsesToolName(call.FunctionName)
	require.Equal(t, "project", namespace)
	require.Equal(t, "lookup", name)
}

func TestGoogleResponsesActiveToolConstraintsStillReject(t *testing.T) {
	for _, policy := range []types.ConversionLossPolicy{types.ConversionLossPolicySafe, types.ConversionLossPolicyStrict} {
		for _, choice := range []string{`"auto"`, `"required"`} {
			for _, constrainedSearch := range []bool{false, true} {
				info := googleInfo("gemini-3-flash-preview")
				info.Options.ToolLossPolicy = policy
				req := &dto.OpenAIResponsesRequest{Model: info.UpstreamModelName, ToolChoice: []byte(choice), Tools: []byte(`[{"type":"function","name":"lookup"}]`), ParallelToolCalls: []byte(`false`)}
				if constrainedSearch {
					req.ParallelToolCalls = nil
					req.Tools = []byte(`[{"type":"function","name":"lookup"},{"type":"web_search","filters":{"allowed_domains":["example.com"]}}]`)
				}
				_, err := ConvertRequest(nil, info, types.RelayFormatGemini, req)
				require.Error(t, err, "%s %s search=%v", policy, choice, constrainedSearch)
			}
		}
	}
}

func TestGoogleResponsesNoneStillValidatesRequest(t *testing.T) {
	for _, tc := range []struct{ name, tools, parallel, text, input string }{
		{name: "parallel type", parallel: `"false"`},
		{name: "tools type", tools: `{}`},
		{name: "missing function name", tools: `[{"type":"function"}]`},
		{name: "invalid namespace member", tools: `[{"type":"namespace","name":"project","tools":[{"type":"web_search"}]}]`},
		{name: "ambiguous tool history", tools: `[{"type":"function","name":"lookup"},{"type":"custom","name":"lookup"}]`},
		{name: "response schema loss", text: `{"format":{"type":"json_schema","schema":{"type":"object","properties":{"count":{"type":"integer","enum":[1,2]}}}}}`},
		{name: "invalid input", input: `[{"type":"function_call","call_id":"call_1"}]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			info := googleInfo("gemini-2.0-flash")
			info.Options.ToolLossPolicy = types.ConversionLossPolicyStrict
			info.Options.Gemini.SupportsJSONSchema = func(string) bool { return false }
			req := &dto.OpenAIResponsesRequest{Model: info.UpstreamModelName, ToolChoice: []byte(`"none"`), Tools: []byte(tc.tools), ParallelToolCalls: []byte(tc.parallel), Text: []byte(tc.text), Input: []byte(tc.input)}
			_, err := ConvertRequest(nil, info, types.RelayFormatGemini, req)
			require.Error(t, err)
		})
	}
}

func TestGoogleChatNoneDisablesToolsWithoutExecutionDiagnostics(t *testing.T) {
	function := `{"type":"function","function":{"name":"lookup","strict":true,"parameters":{"type":"object","properties":{"count":{"type":"integer","enum":[1,2]}}}}}`
	hosted := `{"type":"function","function":{"name":"googleSearch"}},{"type":"function","function":{"name":"codeExecution"}},{"type":"function","function":{"name":"urlContext"}}`
	for _, tc := range []struct {
		name, tools string
		functions   int
	}{
		{name: "omitted"},
		{name: "empty", tools: `[]`},
		{name: "hosted", tools: `[` + hosted + `]`},
		{name: "function", tools: `[` + function + `]`, functions: 1},
		{name: "mixed", tools: `[` + function + `,` + hosted + `]`, functions: 1},
	} {
		for _, policy := range []types.ConversionLossPolicy{types.ConversionLossPolicyAllow, types.ConversionLossPolicySafe, types.ConversionLossPolicyStrict} {
			t.Run(tc.name+"/"+string(policy), func(t *testing.T) {
				info := googleInfo("gemini-2.0-flash")
				info.Options.ToolLossPolicy = policy
				info.Options.Gemini.SupportsJSONSchema = func(string) bool { return false }
				body := `{"model":"gemini-2.0-flash","messages":[{"role":"user","content":"Only answer with text"}],"tool_choice":"none","parallel_tool_calls":false`
				if tc.tools != "" {
					body += `,"tools":` + tc.tools
				}
				body += `}`
				var req dto.GeneralOpenAIRequest
				require.NoError(t, kitutil.UnmarshalJsonStr(body, &req))
				before, err := kitutil.Marshal(req)
				require.NoError(t, err)
				out, err := ConvertRequest(nil, info, types.RelayFormatGemini, &req)
				require.NoError(t, err)
				require.Empty(t, out.Diagnostics)
				converted := out.Value.(*dto.GeminiChatRequest)
				count := 0
				for _, tool := range converted.GetTools() {
					require.Nil(t, tool.GoogleSearch)
					require.Nil(t, tool.CodeExecution)
					require.Nil(t, tool.URLContext)
					declarations, ok := tool.FunctionDeclarations.([]any)
					require.True(t, ok)
					count += len(declarations)
				}
				require.Equal(t, tc.functions, count)
				if count == 0 {
					require.Nil(t, converted.ToolConfig)
				} else {
					require.NotNil(t, converted.ToolConfig)
					require.NotNil(t, converted.ToolConfig.FunctionCallingConfig)
					require.Equal(t, dto.FunctionCallingConfigMode("NONE"), converted.ToolConfig.FunctionCallingConfig.Mode)
				}
				after, err := kitutil.Marshal(req)
				require.NoError(t, err)
				require.JSONEq(t, string(before), string(after))
			})
		}
	}
}

func TestGoogleChatNonePreservesToolHistory(t *testing.T) {
	var req dto.GeneralOpenAIRequest
	require.NoError(t, kitutil.UnmarshalJsonStr(`{
		"model":"gemini-3-flash-preview","tool_choice":"none","parallel_tool_calls":false,
		"tools":[{"type":"function","function":{"name":"lookup","strict":true}}],
		"messages":[
			{"role":"assistant","tool_calls":[{"id":"call_1","type":"function","function":{"name":"lookup","arguments":"{\"count\":1}"}}]},
			{"role":"tool","tool_call_id":"call_1","content":"{\"result\":\"ok\"}"}
		]
	}`, &req))
	info := googleInfo(req.Model)
	info.Options.ToolLossPolicy = types.ConversionLossPolicyStrict
	out, err := ConvertRequest(nil, info, types.RelayFormatGemini, &req)
	require.NoError(t, err)
	require.Empty(t, out.Diagnostics)
	converted := out.Value.(*dto.GeminiChatRequest)
	require.Len(t, converted.Contents, 2)
	call := converted.Contents[0].Parts[0].FunctionCall
	require.NotNil(t, call)
	require.Equal(t, "call_1", call.ID)
	require.Equal(t, "lookup", call.FunctionName)
	require.Equal(t, map[string]any{"count": float64(1)}, call.Arguments)
	result := converted.Contents[1].Parts[0].FunctionResponse
	require.NotNil(t, result)
	require.JSONEq(t, `"call_1"`, string(result.ID))
	require.Equal(t, call.FunctionName, result.Name)
	require.Equal(t, map[string]any{"result": "ok"}, result.Response)
}

func TestGoogleChatToolChoiceKeepsRelevantValidation(t *testing.T) {
	for _, tc := range []struct {
		name, choice, fields, code, message string
		policy                              types.ConversionLossPolicy
	}{
		{name: "auto parallel", choice: "auto", fields: `"parallel_tool_calls":false,"tools":[{"type":"function","function":{"name":"lookup"}}]`, code: "unsupported_parallel_tool_control", policy: types.ConversionLossPolicySafe},
		{name: "required parallel", choice: "required", fields: `"parallel_tool_calls":false,"tools":[{"type":"function","function":{"name":"lookup"}}]`, code: "unsupported_parallel_tool_control", policy: types.ConversionLossPolicyStrict},
		{name: "active strict", choice: "auto", fields: `"tools":[{"type":"function","function":{"name":"lookup","strict":true}}]`, code: "unsupported_function_strict", policy: types.ConversionLossPolicyStrict},
		{name: "active schema", choice: "auto", fields: `"tools":[{"type":"function","function":{"name":"lookup","parameters":{"type":"object","properties":{"count":{"type":"integer","enum":[1,2]}}}}}]`, code: "gemini_schema_keyword_dropped", policy: types.ConversionLossPolicyStrict},
		{name: "disabled response schema", choice: "none", fields: `"parallel_tool_calls":false,"response_format":{"type":"json_schema","json_schema":{"name":"output","schema":{"type":"object","properties":{"count":{"type":"integer","enum":[1,2]}}}}}`, code: "gemini_schema_keyword_dropped", policy: types.ConversionLossPolicyStrict},
		{name: "disabled invalid thinking", choice: "none", fields: `"extra_body":{"google":{"thinking_config":{"thinking_budget":"invalid"}}}`, message: "thinking_budget", policy: types.ConversionLossPolicyStrict},
		{name: "disabled invalid history", choice: "none", fields: `"messages":[{"role":"assistant","tool_calls":[{"id":"call_1","type":"function","function":{"name":"lookup","arguments":"invalid"}}]}]`, message: "invalid arguments", policy: types.ConversionLossPolicyStrict},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var req dto.GeneralOpenAIRequest
			require.NoError(t, kitutil.UnmarshalJsonStr(`{"model":"gemini-2.0-flash","tool_choice":"`+tc.choice+`",`+tc.fields+`}`, &req))
			info := googleInfo(req.Model)
			info.Options.ToolLossPolicy = tc.policy
			info.Options.Gemini.SupportsJSONSchema = func(string) bool { return false }
			_, err := ConvertRequest(nil, info, types.RelayFormatGemini, &req)
			if tc.code == "" {
				require.ErrorContains(t, err, tc.message)
			} else {
				var loss *types.ConversionLossError
				require.ErrorAs(t, err, &loss)
				require.Len(t, loss.Diagnostics, 1)
				require.Equal(t, tc.code, loss.Diagnostics[0].Code)
			}
		})
	}
}

func TestGoogleToolControlDiagnosticsMatchActiveConstraints(t *testing.T) {
	for _, responses := range []bool{false, true} {
		protocol := "chat"
		input := `"messages":[{"role":"user","content":"Hello"}]`
		function := `{"type":"function","function":{"name":"lookup"}}`
		nonStrict := `{"type":"function","function":{"name":"lookup","strict":false}}`
		strict := `{"type":"function","function":{"name":"lookup","strict":true}}`
		hosted := `{"type":"function","function":{"name":"googleSearch"}}`
		if responses {
			protocol = "responses"
			input = `"input":"Hello"`
			function = `{"type":"function","name":"lookup"}`
			nonStrict = `{"type":"function","name":"lookup","strict":false}`
			strict = `{"type":"function","name":"lookup","strict":true}`
			hosted = `{"type":"web_search"}`
		}
		for _, tc := range []struct {
			name, fields, diagnostic string
			semanticLoss             bool
		}{
			{name: "no tools", fields: `"parallel_tool_calls":false`},
			{name: "empty tools", fields: `"tools":[],"parallel_tool_calls":false`},
			{name: "hosted only", fields: `"tools":[` + hosted + `],"parallel_tool_calls":false`},
			{name: "strict omitted", fields: `"tools":[` + function + `]`},
			{name: "strict false", fields: `"tools":[` + nonStrict + `]`},
			{name: "strict true", fields: `"tools":[` + strict + `]`, diagnostic: "unsupported_function_strict"},
			{name: "active parallel constraint", fields: `"tools":[` + function + `],"parallel_tool_calls":false`, diagnostic: "unsupported_parallel_tool_control", semanticLoss: true},
			{name: "mixed parallel constraint", fields: `"tools":[` + function + `,` + hosted + `],"parallel_tool_calls":false`, diagnostic: "unsupported_parallel_tool_control", semanticLoss: true},
		} {
			for _, policy := range []types.ConversionLossPolicy{types.ConversionLossPolicyAllow, types.ConversionLossPolicySafe, types.ConversionLossPolicyStrict} {
				t.Run(protocol+"/"+tc.name+"/"+string(policy), func(t *testing.T) {
					var req any = new(dto.GeneralOpenAIRequest)
					if responses {
						req = new(dto.OpenAIResponsesRequest)
					}
					require.NoError(t, kitutil.UnmarshalJsonStr(`{"model":"gemini-2.0-flash",`+input+`,`+tc.fields+`}`, req))
					before, err := kitutil.Marshal(req)
					require.NoError(t, err)
					info := googleInfo("gemini-2.0-flash")
					info.Options.ToolLossPolicy = policy
					out, err := ConvertRequest(nil, info, types.RelayFormatGemini, req)
					reject := tc.diagnostic != "" && (policy == types.ConversionLossPolicyStrict || policy == types.ConversionLossPolicySafe && tc.semanticLoss)
					if reject {
						var loss *types.ConversionLossError
						require.ErrorAs(t, err, &loss)
						require.Len(t, loss.Diagnostics, 1)
						require.Equal(t, tc.diagnostic, loss.Diagnostics[0].Code)
					} else {
						require.NoError(t, err)
						if tc.diagnostic == "" {
							require.Empty(t, out.Diagnostics)
						} else {
							require.Len(t, out.Diagnostics, 1)
							require.Equal(t, tc.diagnostic, out.Diagnostics[0].Code)
						}
						converted := out.Value.(*dto.GeminiChatRequest)
						require.NotEmpty(t, converted.Contents)
						if tc.name == "hosted only" {
							require.Len(t, converted.GetTools(), 1)
							require.NotNil(t, converted.GetTools()[0].GoogleSearch)
						}
					}
					after, err := kitutil.Marshal(req)
					require.NoError(t, err)
					require.JSONEq(t, string(before), string(after))
				})
			}
		}
	}
}

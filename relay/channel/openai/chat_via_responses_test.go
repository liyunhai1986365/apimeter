package openai

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newResponsesChatTestContext(t *testing.T, body string, isStream bool) (*gin.Context, *httptest.ResponseRecorder, *http.Response, *relaycommon.RelayInfo) {
	t.Helper()

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	c.Set(common.RequestIdKey, "responses-test")

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
	}
	info := &relaycommon.RelayInfo{
		ChannelMeta:        &relaycommon.ChannelMeta{UpstreamModelName: "gpt-test"},
		IsStream:           isStream,
		RelayFormat:        types.RelayFormatOpenAI,
		ShouldIncludeUsage: true,
		DisablePing:        true,
	}
	return c, recorder, resp, info
}

func TestOaiResponsesToChatStreamHandlerConvertsSSEOrderAndUsage(t *testing.T) {
	oldMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(oldMode) })

	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })

	body := strings.Join([]string{
		`data: {"type":"response.created","response":{"id":"resp_1","model":"gpt-test","created_at":1710000000}}`,
		`data: {"type":"response.output_text.delta","delta":"hello"}`,
		`data: {"type":"response.output_item.added","output_index":1,"item":{"type":"function_call","id":"fc_1","call_id":"call_1","name":"lookup"}}`,
		`data: {"type":"response.function_call_arguments.delta","output_index":1,"delta":"{\"q\":\"x\"}"}`,
		`data: {"type":"response.completed","response":{"status":"completed","usage":{"input_tokens":2,"output_tokens":3,"total_tokens":5}}}`,
		`data: [DONE]`,
		``,
	}, "\n")

	c, recorder, resp, info := newResponsesChatTestContext(t, body, true)

	usage, err := OaiResponsesToChatStreamHandler(c, info, resp)
	require.Nil(t, err)
	require.NotNil(t, usage)
	require.Equal(t, 2, usage.PromptTokens)
	require.Equal(t, 3, usage.CompletionTokens)
	require.Equal(t, 5, usage.TotalTokens)

	got := recorder.Body.String()
	require.Equal(t, "text/event-stream", recorder.Header().Get("Content-Type"))
	require.Contains(t, got, `"role":"assistant"`)
	require.Contains(t, got, `"content":"hello"`)
	require.Contains(t, got, `"name":"lookup"`)
	require.Contains(t, got, `"arguments":"{\"q\":\"x\"}"`)
	require.Contains(t, got, `"finish_reason":"tool_calls"`)
	require.Contains(t, got, `"usage":{"prompt_tokens":2,"completion_tokens":3,"total_tokens":5`)
	require.Contains(t, got, `data: [DONE]`)
	requireOrderedSubstrings(t, got,
		`"role":"assistant"`,
		`"content":"hello"`,
		`"name":"lookup"`,
		`"arguments":"{\"q\":\"x\"}"`,
		`"finish_reason":"tool_calls"`,
		`"usage":{"prompt_tokens":2,"completion_tokens":3,"total_tokens":5`,
		`data: [DONE]`,
	)
}

func TestResponsesToChatRejectsUpstreamStreamError(t *testing.T) {
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })

	for _, event := range []struct{ name, data string }{
		{"error", `{"type":"error","code":"bad_response","message":"upstream refused request","param":"model","sequence_number":0}`},
		{"response.failed", `{"type":"response.failed","response":{"error":{"code":"bad_response","message":"upstream refused request","param":"model"}}}`},
	} {
		for _, target := range []string{"buffered", "chat", "claude"} {
			t.Run(event.name+"/"+target, func(t *testing.T) {
				body := ": PING\n\nevent: " + event.name + "\ndata: " + event.data + "\n\ndata: [DONE]\n\n"
				c, recorder, resp, info := newResponsesChatTestContext(t, body, target != "buffered")
				if target == "claude" {
					info.RelayFormat = types.RelayFormatClaude
				}
				handler := OaiResponsesToChatStreamHandler
				if target == "buffered" {
					handler = OaiResponsesToChatBufferedStreamHandler
				}
				usage, failure := handler(c, info, resp)
				require.Nil(t, usage)
				require.NotNil(t, failure)
				require.Equal(t, http.StatusBadGateway, failure.StatusCode)
				converted := failure.ToOpenAIError()
				require.Equal(t, "bad_response", converted.Code)
				require.Equal(t, "upstream refused request", converted.Message)
				require.Equal(t, "model", converted.Param)
				require.Empty(t, recorder.Body.String())
			})
		}
	}
}

func TestResponsesToChatSettlesReportedUsageOnStreamFailure(t *testing.T) {
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })
	const snapshot = `{"input_tokens":100,"output_tokens":20,"total_tokens":120,"input_tokens_details":{"cached_tokens":30}}`
	for _, tc := range []struct{ name, final string }{
		{"failure snapshot", `{"type":"response.failed","response":{"usage":` + snapshot + `,"error":{"message":"upstream stopped"}}}`},
		{"earlier snapshot", `{"type":"error","message":"upstream stopped"}`},
	} {
		for _, target := range []string{"buffered", "stream"} {
			t.Run(tc.name+"/"+target, func(t *testing.T) {
				body := "data: " + `{"type":"response.in_progress","response":{"usage":` + snapshot + `}}` + "\n\ndata: " +
					`{"type":"response.output_text.delta","delta":"partial reply"}` + "\n\ndata: " + tc.final + "\n\n"
				c, recorder, resp, info := newResponsesChatTestContext(t, body, target == "stream")
				handler := OaiResponsesToChatBufferedStreamHandler
				if info.IsStream {
					handler = OaiResponsesToChatStreamHandler
				}
				usage, failure := handler(c, info, resp)
				require.NotNil(t, failure)
				require.NotNil(t, usage)
				require.Equal(t, 120, usage.TotalTokens)
				require.Equal(t, 30, usage.PromptTokensDetails.CachedTokens)
				if info.IsStream {
					require.Contains(t, recorder.Body.String(), "partial reply")
					require.NotContains(t, recorder.Body.String(), "[DONE]")
				} else {
					require.Empty(t, recorder.Body.String())
				}
				var billable *relaycommon.StreamUsageError
				require.ErrorAs(t, failure, &billable)
				calls := 0
				settle := func(snapshot *dto.Usage) {
					calls++
					require.Same(t, usage, snapshot)
				}
				billable.SettleOnce(settle)
				billable.SettleOnce(settle)
				require.Equal(t, 1, calls)
			})
		}
	}
}

func TestResponsesJSONFailureUsesExplicitUsage(t *testing.T) {
	for _, stream := range []bool{false, true} {
		for _, status := range []string{"failed", "cancelled"} {
			c, recorder, resp, info := newResponsesChatTestContext(t, `{"status":"`+status+`","usage":{"input_tokens":100,"output_tokens":20,"total_tokens":120}}`, stream)
			resp.Header.Set("Content-Type", "application/json")
			if stream {
				info.RelayFormat = types.RelayFormatClaude
			}
			usage, failure := OaiResponsesToChatHandler(c, info, resp)
			require.NotNil(t, failure)
			require.Equal(t, http.StatusBadGateway, failure.StatusCode)
			require.Empty(t, recorder.Body.String())
			require.NotNil(t, usage)
			require.Equal(t, 120, usage.TotalTokens)
			var marker *relaycommon.StreamUsageError
			require.ErrorAs(t, failure, &marker)
			calls := 0
			settle := func(actual *dto.Usage) { calls++; require.Same(t, usage, actual) }
			marker.SettleOnce(settle)
			marker.SettleOnce(settle)
			require.Equal(t, 1, calls)
		}
	}
}

func TestResponsesToChatRequiresUpstreamTerminal(t *testing.T) {
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })
	for _, mode := range []string{"buffered", "stream"} {
		for _, tc := range []struct{ name, terminal, finish string }{
			{"eof", "", ""}, {"bare done", "data: [DONE]\n\n", ""},
			{"completed", "data: " + `{"type":"response.completed","response":{"status":"completed"}}` + "\n\n", "stop"},
			{"done alias", "data: " + `{"type":"response.done","response":{"status":"completed"}}` + "\n\n", "stop"},
			{"incomplete", "data: " + `{"type":"response.incomplete"}` + "\n\n", "length"},
		} {
			t.Run(mode+"/"+tc.name, func(t *testing.T) {
				body := "data: " + `{"type":"response.in_progress","response":{"usage":{"input_tokens":100,"output_tokens":20,"total_tokens":120}}}` + "\n\ndata: " + `{"type":"response.output_text.delta","delta":"partial reply"}` + "\n\n" + tc.terminal
				c, recorder, resp, info := newResponsesChatTestContext(t, body, mode == "stream")
				handler := OaiResponsesToChatBufferedStreamHandler
				if info.IsStream {
					handler = OaiResponsesToChatStreamHandler
				}
				usage, failure := handler(c, info, resp)
				require.NotNil(t, usage)
				require.Equal(t, 120, usage.TotalTokens)
				got := recorder.Body.String()
				if tc.finish != "" {
					require.Nil(t, failure)
					require.Contains(t, got, `"finish_reason":"`+tc.finish+`"`)
					return
				}
				require.NotNil(t, failure)
				require.Equal(t, http.StatusBadGateway, failure.StatusCode)
				var marker *relaycommon.StreamUsageError
				require.ErrorAs(t, failure, &marker)
				require.NotContains(t, got, `"finish_reason":"stop"`)
				require.NotContains(t, got, "[DONE]")
				if mode == "buffered" {
					require.Empty(t, got)
				}
			})
		}
	}
}

func TestOaiResponsesToChatStreamHandlerConvertsClaudeSSETerminalsAndUsage(t *testing.T) {
	oldMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(oldMode) })

	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })

	body := strings.Join([]string{
		`data: {"type":"response.created","response":{"id":"resp_1","model":"gpt-test","created_at":1710000000}}`,
		`data: {"type":"response.output_text.delta","delta":"hello"}`,
		`data: {"type":"response.completed","response":{"status":"completed","usage":{"input_tokens":2,"output_tokens":3,"total_tokens":5}}}`,
		`data: [DONE]`,
		``,
	}, "\n")

	c, recorder, resp, info := newResponsesChatTestContext(t, body, true)
	info.RelayFormat = types.RelayFormatClaude

	usage, err := OaiResponsesToChatStreamHandler(c, info, resp)
	require.Nil(t, err)
	require.NotNil(t, usage)
	assert.Equal(t, 2, usage.PromptTokens)
	assert.Equal(t, 3, usage.CompletionTokens)
	assert.Equal(t, 5, usage.TotalTokens)

	got := recorder.Body.String()
	assert.Equal(t, "text/event-stream", recorder.Header().Get("Content-Type"))
	assert.Equal(t, 1, strings.Count(got, "event: message_start\n"))
	assert.Equal(t, 1, strings.Count(got, "event: content_block_stop\n"))
	assert.Equal(t, 1, strings.Count(got, "event: message_delta\n"))
	assert.Equal(t, 1, strings.Count(got, "event: message_stop\n"))

	messageDeltaFrame := ""
	for _, frame := range strings.Split(got, "\n\n") {
		if strings.HasPrefix(frame, "event: message_delta\n") {
			messageDeltaFrame = frame
			break
		}
	}
	require.NotEmpty(t, messageDeltaFrame)
	assert.Contains(t, messageDeltaFrame, `"type":"message_delta"`)
	assert.Contains(t, messageDeltaFrame, `"stop_reason":"end_turn"`)
	assert.Contains(t, messageDeltaFrame, `"input_tokens":2`)
	assert.Contains(t, messageDeltaFrame, `"output_tokens":3`)
	requireOrderedSubstrings(t, got,
		"event: message_start\n",
		"event: content_block_stop\n",
		"event: message_delta\n",
		"event: message_stop\n",
	)
}

func TestOaiResponsesToChatBufferedStreamHandlerReturnsJSONFromSSE(t *testing.T) {
	oldMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(oldMode) })

	body := strings.Join([]string{
		`data: {"type":"response.output_text.delta","delta":"buffered text"}`,
		`data: {"type":"response.output_item.added","output_index":0,"item":{"type":"function_call","id":"fc_1","call_id":"call_1","name":"lookup"}}`,
		`data: {"type":"response.function_call_arguments.delta","output_index":0,"delta":"{\"q\":\"x\"}"}`,
		`data: {"type":"response.done","response":{"model":"gpt-test","status":"completed","usage":{"input_tokens":1,"output_tokens":2,"total_tokens":3}}}`,
		`data: [DONE]`,
		``,
	}, "\n")

	c, recorder, resp, info := newResponsesChatTestContext(t, body, false)

	usage, err := OaiResponsesToChatBufferedStreamHandler(c, info, resp)
	require.Nil(t, err)
	require.NotNil(t, usage)
	require.Equal(t, 3, usage.TotalTokens)

	got := recorder.Body.String()
	require.NotContains(t, got, `data:`)
	require.Contains(t, got, `"object":"chat.completion"`)
	require.Contains(t, got, `"content":"buffered text"`)
	require.Contains(t, got, `"name":"lookup"`)
	require.Contains(t, got, `"arguments":"{\"q\":\"x\"}"`)
	require.Contains(t, got, `"finish_reason":"tool_calls"`)
}

func TestOaiChatToResponsesStreamHandlerConvertsSSEOrderAndUsage(t *testing.T) {
	oldMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(oldMode) })

	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })

	body := strings.Join([]string{
		`data: {"id":"chatcmpl_1","object":"chat.completion.chunk","created":1710000000,"model":"gpt-test","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}`,
		`data: {"id":"chatcmpl_1","object":"chat.completion.chunk","created":1710000000,"model":"gpt-test","choices":[{"index":0,"delta":{"content":"hello"},"finish_reason":null}]}`,
		`data: {"id":"chatcmpl_1","object":"chat.completion.chunk","created":1710000000,"model":"gpt-test","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"lookup"}}]},"finish_reason":null}]}`,
		`data: {"id":"chatcmpl_1","object":"chat.completion.chunk","created":1710000000,"model":"gpt-test","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"q\":\"x\"}"}}]},"finish_reason":null}]}`,
		`data: {"id":"chatcmpl_1","object":"chat.completion.chunk","created":1710000000,"model":"gpt-test","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`,
		`data: {"id":"chatcmpl_1","object":"chat.completion.chunk","created":1710000000,"model":"gpt-test","choices":[],"usage":{"prompt_tokens":2,"completion_tokens":3,"total_tokens":5}}`,
		`data: [DONE]`,
		``,
	}, "\n")

	c, recorder, resp, info := newResponsesChatTestContext(t, body, true)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	usage, err := OaiChatToResponsesStreamHandler(c, info, resp)
	require.Nil(t, err)
	require.NotNil(t, usage)
	require.Equal(t, 2, usage.PromptTokens)
	require.Equal(t, 3, usage.CompletionTokens)
	require.Equal(t, 5, usage.TotalTokens)

	got := recorder.Body.String()
	require.Equal(t, "text/event-stream", recorder.Header().Get("Content-Type"))
	require.Contains(t, got, `event: response.created`)
	require.Contains(t, got, `event: response.output_text.delta`)
	require.Contains(t, got, `"delta":"hello"`)
	require.Contains(t, got, `event: response.function_call_arguments.delta`)
	require.Contains(t, got, `"delta":"{\"q\":\"x\"}"`)
	require.Contains(t, got, `event: response.completed`)
	require.Contains(t, got, `"input_tokens":2`)
	require.Contains(t, got, `"output_tokens":3`)
	requireOrderedSubstrings(t, got,
		`event: response.created`,
		`event: response.output_item.added`,
		`event: response.output_text.delta`,
		`event: response.output_item.added`,
		`event: response.function_call_arguments.delta`,
		`event: response.output_text.done`,
		`event: response.function_call_arguments.done`,
		`event: response.completed`,
	)
}

func requireOrderedSubstrings(t *testing.T, s string, parts ...string) {
	t.Helper()

	offset := 0
	for _, part := range parts {
		idx := strings.Index(s[offset:], part)
		require.NotEqualf(t, -1, idx, "missing %q after byte offset %d", part, offset)
		offset += idx + len(part)
	}
}

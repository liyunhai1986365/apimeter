package openai

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

const responsesJSONStreamFixture = `{
	"id":"resp_json", "object":"response", "model":"gpt-test", "created_at":1710000000,
	"status":"completed",
	"output":[
		{"type":"reasoning","content":[{"type":"reasoning_text","text":"Plan."}]},
		{"type":"message","role":"assistant","content":[{"type":"output_text","text":"hello "}]},
		{"type":"message","role":"assistant","content":[{"type":"output_text","text":"world"}]},
		{"type":"function_call","id":"fc_1","call_id":"call_1","name":"lookup","arguments":"{\"big\":9007199254740993,\"small\":1e-30,\"zero\":0,\"flag\":false}"}
	],
	"usage":{"input_tokens":100,"output_tokens":20,"total_tokens":120,
		"input_tokens_details":{"cached_tokens":80},"output_tokens_details":{"reasoning_tokens":5}}
}`

func TestOaiResponsesJSONToClaudeStream(t *testing.T) {
	for _, status := range []string{"completed", "incomplete"} {
		for _, ping := range []bool{false, true} {
			name := status + "/before_ping"
			if ping {
				name = status + "/after_ping"
			}
			t.Run(name, func(t *testing.T) {
				body := strings.Replace(responsesJSONStreamFixture, `"status":"completed"`, `"status":"`+status+`"`, 1)
				c, recorder, resp, info := newResponsesChatTestContext(t, body, true)
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
				info.RelayFormat = types.RelayFormatClaude
				resp.Header.Set("Content-Type", "application/json")
				resp.Header.Set("Content-Length", "999")
				interval := time.Hour
				if ping {
					interval = time.Millisecond
				}
				stop := helper.StartStreamKeepAlive(c, interval)
				defer stop()
				if ping {
					require.Eventually(t, c.Writer.Written, time.Second, time.Millisecond)
				}

				usage, apiErr := OaiResponsesToChatHandler(c, info, resp)
				require.Nil(t, apiErr)
				require.NotNil(t, usage)
				require.Equal(t, 100, usage.PromptTokens)
				require.Equal(t, 20, usage.CompletionTokens)
				require.Equal(t, 120, usage.TotalTokens)
				require.Equal(t, 80, usage.PromptTokensDetails.CachedTokens)
				require.Equal(t, 5, usage.CompletionTokenDetails.ReasoningTokens)
				require.NotNil(t, usage.BillingUsage)
				require.Equal(t, dto.BillingUsageSourceOAIResponses, usage.BillingUsage.Source)
				require.Same(t, usage, info.EnsureClaudeConvertInfo().Usage)
				require.Equal(t, "text/event-stream", recorder.Result().Header.Get("Content-Type"))
				require.Empty(t, recorder.Result().Header.Get("Content-Length"))
				require.Equal(t, ping, strings.HasPrefix(recorder.Body.String(), ": PING\n\n"))

				events := parseClaudeJSONFallbackEvents(t, recorder.Body.String())
				require.Equal(t, "message_start", events[0].Type)
				require.Equal(t, "message_stop", events[len(events)-1].Type)
				counts := make(map[string]int)
				openBlocks := make(map[int]bool)
				var text, thinking, arguments strings.Builder
				var finalUsage *dto.ClaudeUsage
				var stopReason string
				for _, event := range events {
					counts[event.Type]++
					switch event.Type {
					case "content_block_start":
						require.NotNil(t, event.Index)
						require.False(t, openBlocks[*event.Index])
						openBlocks[*event.Index] = true
						if event.ContentBlock.Type == "tool_use" {
							require.Equal(t, "call_1", event.ContentBlock.Id)
							require.Equal(t, "lookup", event.ContentBlock.Name)
						}
					case "content_block_delta":
						require.NotNil(t, event.Index)
						require.True(t, openBlocks[*event.Index])
						switch event.Delta.Type {
						case "text_delta":
							text.WriteString(*event.Delta.Text)
						case "thinking_delta":
							thinking.WriteString(*event.Delta.Thinking)
						case "input_json_delta":
							arguments.WriteString(*event.Delta.PartialJson)
						}
					case "content_block_stop":
						require.NotNil(t, event.Index)
						require.True(t, openBlocks[*event.Index])
						delete(openBlocks, *event.Index)
					case "message_delta":
						require.Empty(t, openBlocks)
						finalUsage = event.Usage
						stopReason = *event.Delta.StopReason
					}
				}
				require.Empty(t, openBlocks)
				require.Equal(t, 1, counts["message_start"])
				require.Equal(t, 1, counts["message_delta"])
				require.Equal(t, 1, counts["message_stop"])
				require.Equal(t, 3, counts["content_block_start"])
				require.Equal(t, "hello world", text.String())
				require.Equal(t, "Plan.", thinking.String())
				require.Equal(t, `{"big":9007199254740993,"small":1e-30,"zero":0,"flag":false}`, arguments.String())
				require.Equal(t, "9007199254740993", gjson.Get(arguments.String(), "big").Raw)
				require.NotNil(t, finalUsage)
				require.Equal(t, 100, finalUsage.InputTokens)
				require.Equal(t, 20, finalUsage.OutputTokens)
				require.Equal(t, 80, finalUsage.CacheReadInputTokens)
				if status == "incomplete" {
					require.Equal(t, "max_tokens", stopReason)
				} else {
					require.Equal(t, "tool_use", stopReason)
				}
			})
		}
	}
}

func TestOaiResponsesJSONToClaudeStreamRejectsFailedResponse(t *testing.T) {
	for _, tc := range []struct{ status, detail, want string }{
		{"failed", `{"code":"server_error","message":"upstream failed"}`, "upstream failed"},
		{"failed", "null", "upstream response failed"},
		{"cancelled", "null", "upstream response cancelled"},
	} {
		t.Run(tc.want, func(t *testing.T) {
			body := `{"model":"o3","status":"` + tc.status + `","error":` + tc.detail + `,"output":[]}`
			c, recorder, resp, info := newResponsesChatTestContext(t, body, true)
			info.RelayFormat = types.RelayFormatClaude
			usage, apiErr := OaiResponsesToChatHandler(c, info, resp)
			require.ErrorContains(t, apiErr, tc.want)
			require.Equal(t, http.StatusBadGateway, apiErr.StatusCode)
			require.Nil(t, usage)
			require.Empty(t, recorder.Body.String(), "a failed response must not emit successful Claude events")
		})
	}
}

func TestOaiResponsesJSONToClaudeNonStreamAndUsageParity(t *testing.T) {
	for _, upstreamUsage := range []bool{true, false} {
		body := responsesJSONStreamFixture
		if !upstreamUsage {
			var response dto.OpenAIResponsesResponse
			require.NoError(t, common.UnmarshalJsonStr(body, &response))
			response.Usage = nil
			encoded, err := common.Marshal(response)
			require.NoError(t, err)
			body = string(encoded)
		}

		c, recorder, resp, info := newResponsesChatTestContext(t, body, false)
		info.RelayFormat = types.RelayFormatClaude
		resp.Header.Set("Content-Type", "application/json")
		jsonUsage, apiErr := OaiResponsesToChatHandler(c, info, resp)
		require.Nil(t, apiErr)
		require.Equal(t, "application/json", recorder.Result().Header.Get("Content-Type"))
		require.NotContains(t, recorder.Body.String(), "event:")
		require.Equal(t, "message", gjson.GetBytes(recorder.Body.Bytes(), "type").String())
		require.Equal(t, "9007199254740993", gjson.GetBytes(recorder.Body.Bytes(), "content.#(type==tool_use).input.big").Raw)

		streamContext, streamRecorder, streamResp, streamInfo := newResponsesChatTestContext(t, body, true)
		streamInfo.RelayFormat = types.RelayFormatClaude
		streamResp.Header.Set("Content-Type", "application/json")
		streamUsage, streamErr := OaiResponsesToChatHandler(streamContext, streamInfo, streamResp)
		require.Nil(t, streamErr)
		require.Equal(t, jsonUsage, streamUsage, "changing delivery to SSE must not change settlement usage")
		events := parseClaudeJSONFallbackEvents(t, streamRecorder.Body.String())
		for _, event := range events {
			if event.Type == "message_delta" {
				require.Equal(t, jsonUsage.CompletionTokens, event.Usage.OutputTokens)
			}
		}
	}
}

func parseClaudeJSONFallbackEvents(t *testing.T, body string) []dto.ClaudeResponse {
	t.Helper()
	var events []dto.ClaudeResponse
	for _, frame := range strings.Split(body, "\n\n") {
		if frame == "" || strings.HasPrefix(frame, ":") {
			continue
		}
		parts := strings.SplitN(frame, "\ndata: ", 2)
		require.Len(t, parts, 2, "expected a Claude SSE frame, got %q", frame)
		var event dto.ClaudeResponse
		require.NoError(t, common.UnmarshalJsonStr(parts[1], &event))
		require.Equal(t, "event: "+event.Type, parts[0])
		events = append(events, event)
	}
	require.NotEmpty(t, events)
	return events
}

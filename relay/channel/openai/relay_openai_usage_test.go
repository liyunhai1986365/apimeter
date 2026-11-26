package openai

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestOaiStreamHandlerCompletesReportedUsage(t *testing.T) {
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })
	for _, tc := range []struct {
		name, counters string
		p, c, total    int
		audio          bool
	}{
		{"missing output", `"prompt_tokens":10,"total_tokens":110`, 10, 100, 110, false},
		{"missing input", `"completion_tokens":100,"total_tokens":110`, 10, 100, 110, false},
		{"missing total", `"prompt_tokens":10,"completion_tokens":100`, 10, 100, 110, false},
		{"explicit zero", `"prompt_tokens":0,"completion_tokens":0,"total_tokens":0`, 0, 0, 0, false},
		{"reported counts win", `"prompt_tokens":10,"completion_tokens":0,"total_tokens":110`, 10, 0, 110, false},
		{"null output", `"prompt_tokens":11,"total_tokens":1011,"completion_tokens":null`, 11, 1000, 1011, false},
		{"audio penultimate", `"prompt_tokens":10,"total_tokens":110`, 10, 100, 110, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			frame := `{"choices":[],"usage":{` + tc.counters + `,"completion_tokens_details":{"audio_tokens":0,"extension":9007199254740993}}}`
			body := "data: " + frame + "\n\n"
			if tc.audio {
				body += "data: {\"choices\":[],\"usage\":null}\n\n"
			}
			ctx, rec, resp, info := newResponsesChatTestContext(t, body+"data: [DONE]\n\n", true)
			if tc.audio {
				info.UpstreamModelName = "gpt-audio"
			}
			info.SetEstimatePromptTokens(17)
			usage, err := OaiStreamHandler(ctx, info, resp)
			require.Nil(t, err)
			require.Equal(t, []int{tc.p, tc.c, tc.total}, []int{usage.PromptTokens, usage.CompletionTokens, usage.TotalTokens})
			var count int
			for _, line := range strings.Split(rec.Body.String(), "\n") {
				raw := gjson.Get(strings.TrimPrefix(line, "data: "), "usage")
				if !raw.IsObject() {
					continue
				}
				count++
				for key, want := range map[string]int{"prompt_tokens": tc.p, "completion_tokens": tc.c, "total_tokens": tc.total} {
					require.Equal(t, gjson.Number, raw.Get(key).Type)
					require.EqualValues(t, want, raw.Get(key).Int())
				}
				require.Equal(t, "9007199254740993", raw.Get("completion_tokens_details.extension").Raw)
			}
			require.Equal(t, 1, count)
		})
	}
	t.Run("zero audio trailer keeps earlier usage for billing", func(t *testing.T) {
		body := "data: " + `{"choices":[],"usage":{"prompt_tokens":10,"total_tokens":110,"completion_tokens_details":{"audio_tokens":20}}}` + "\n\n" +
			"data: " + `{"choices":[],"usage":{"prompt_tokens":0,"completion_tokens":0,"total_tokens":0}}` + "\n\ndata: [DONE]\n\n"
		ctx, _, resp, info := newResponsesChatTestContext(t, body, true)
		info.UpstreamModelName = "gpt-audio"
		usage, err := OaiStreamHandler(ctx, info, resp)
		require.Nil(t, err)
		require.Equal(t, 100, usage.CompletionTokens)
		require.Equal(t, 20, usage.CompletionTokenDetails.AudioTokens)
	})
}

func TestOaiStreamUsageIndependentOfFrameLayout(t *testing.T) {
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })
	const (
		input    = `{"prompt_tokens":10,"prompt_tokens_details":{"cached_tokens":3,"extension":9007199254740993}}`
		output   = `{"completion_tokens":100,"prompt_tokens_details":{"audio_tokens":2}}`
		complete = `{"prompt_tokens":10,"completion_tokens":100,"total_tokens":110}`
		zero     = `{"prompt_tokens":0,"completion_tokens":0,"total_tokens":0}`
	)
	for _, tc := range []struct {
		name    string
		reports []string
		want    [3]int
		details bool
	}{
		{"usage before stop", []string{complete, "null", "null"}, [3]int{10, 100, 110}, false},
		{"input then output", []string{input, output}, [3]int{10, 100, 110}, true},
		{"repeated reversed snapshots", []string{output, input, input, output}, [3]int{10, 100, 110}, true},
		{"later explicit zero", []string{complete, `{"completion_tokens":0,"total_tokens":10}`}, [3]int{10, 0, 10}, false},
		{"null does not erase input", []string{input, `{"prompt_tokens":null,"completion_tokens":100,"prompt_tokens_details":{"audio_tokens":2}}`}, [3]int{10, 100, 110}, true},
		{"empty trailer", []string{complete, zero}, [3]int{10, 100, 110}, false},
		{"explicitly free", []string{zero}, [3]int{0, 0, 0}, false},
		{"updated snapshot without total", []string{complete, `{"completion_tokens":120}`}, [3]int{10, 120, 130}, false},
		{"new prompt and total replace old output", []string{`{"prompt_tokens":10,"completion_tokens":4,"total_tokens":14}`, `{"prompt_tokens":10,"total_tokens":110}`}, [3]int{10, 100, 110}, false},
		{"new output and total replace old prompt", []string{`{"prompt_tokens":2,"completion_tokens":100,"total_tokens":102}`, `{"completion_tokens":100,"total_tokens":110}`}, [3]int{10, 100, 110}, false},
		{"new output replaces subtotal inferred from old total", []string{`{"prompt_tokens":10,"total_tokens":14}`, output}, [3]int{10, 100, 110}, false},
		{"new prompt replaces inferred prompt", []string{`{"completion_tokens":100,"total_tokens":102}`, input}, [3]int{10, 100, 110}, false},
		{"new pair without total supersedes total", []string{complete, `{"prompt_tokens":11,"completion_tokens":101}`}, [3]int{11, 101, 112}, false},
		{"updated total alone keeps known prompt", []string{complete, `{"total_tokens":130}`}, [3]int{10, 120, 130}, false},
		{"new output after prompt was corrected by total", []string{`{"prompt_tokens":2,"completion_tokens":4,"total_tokens":6}`, `{"completion_tokens":4,"total_tokens":14}`, output}, [3]int{10, 100, 110}, false},
		{"superseding pair can imply zero", []string{complete, `{"prompt_tokens":10,"total_tokens":10}`}, [3]int{10, 0, 10}, false},
		{"no usage estimates prompt", []string{"null"}, [3]int{17, 0, 17}, false},
	} {
		for _, include := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s/include=%t", tc.name, include), func(t *testing.T) {
				var sse strings.Builder
				for i, report := range tc.reports {
					choices := `[]`
					if i == len(tc.reports)-1 {
						choices = `[{"delta":{"tool_calls":[{"index":0,"function":{"name":"lookup","arguments":"{}"}}]},"finish_reason":"tool_calls"}]`
					}
					fmt.Fprintf(&sse, "data: {\"choices\":%s,\"usage\":%s}\n\n", choices, report)
				}
				sse.WriteString("data: [DONE]\n\n")
				ctx, rec, resp, info := newResponsesChatTestContext(t, sse.String(), true)
				info.ShouldIncludeUsage = include
				info.SetEstimatePromptTokens(17)
				u, err := OaiStreamHandler(ctx, info, resp)
				require.Nil(t, err)
				require.Equal(t, tc.want, [3]int{u.PromptTokens, u.CompletionTokens, u.TotalTokens})
				require.Equal(t, 1, strings.Count(rec.Body.String(), `"lookup"`))
				require.Equal(t, 1, strings.Count(rec.Body.String(), "[DONE]"))
				var reports []gjson.Result
				for _, line := range strings.Split(rec.Body.String(), "\n") {
					if raw := gjson.Get(strings.TrimPrefix(line, "data: "), "usage"); raw.IsObject() {
						reports = append(reports, raw)
					}
				}
				if !include {
					require.Empty(t, reports)
					return
				}
				require.Len(t, reports, 1)
				for i, key := range []string{"prompt_tokens", "completion_tokens", "total_tokens"} {
					require.Equal(t, gjson.Number, reports[0].Get(key).Type)
					require.EqualValues(t, tc.want[i], reports[0].Get(key).Int())
				}
				if tc.details {
					require.Equal(t, 3, u.PromptTokensDetails.CachedTokens)
					require.Equal(t, 2, u.PromptTokensDetails.AudioTokens)
					require.Equal(t, "9007199254740993", reports[0].Get("prompt_tokens_details.extension").Raw)
				}
			})
		}
	}
}

// A complete old snapshot must not affect any counter determined by a new pair.
func FuzzChatUsageSupersedingPair(f *testing.F) {
	f.Add(uint16(10), uint16(4), uint16(10), uint16(100))
	f.Add(uint16(2), uint16(100), uint16(10), uint16(100))
	f.Add(uint16(90), uint16(30), uint16(0), uint16(0))
	f.Fuzz(func(t *testing.T, oldP, oldC, newP, newC uint16) {
		p0, c0, t0 := int(oldP), int(oldC), int(oldP)+int(oldC)
		p, c, total := int(newP), int(newC), int(newP)+int(newC)
		previous := chatTokenCounts{&p0, &c0, &t0}
		want := chatTokenCounts{&p, &c, &total}
		for _, pair := range []chatTokenCounts{{&p, &c, nil}, {&p, nil, &total}, {nil, &c, &total}} {
			require.Equal(t, want, pair.merge(previous))
		}
	})
}

func TestOaiStreamUsageWithConvertedTerminalChoice(t *testing.T) {
	old := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = old })
	body := strings.Join([]string{
		`data: {"choices":[{"delta":{"content":"first text"}}],"usage":{"prompt_tokens":10}}`,
		`data: {"choices":[{"delta":{},"finish_reason":"stop"}]}`,
		`data: {"choices":[],"usage":{"completion_tokens":100}}`,
		`data: [DONE]`,
		"",
	}, "\n\n")
	for _, format := range []types.RelayFormat{types.RelayFormatClaude, types.RelayFormatGemini} {
		t.Run(string(format), func(t *testing.T) {
			ctx, rec, resp, info := newResponsesChatTestContext(t, body, true)
			info.RelayFormat = format
			u, err := OaiStreamHandler(ctx, info, resp)
			require.Nil(t, err)
			require.Equal(t, [3]int{10, 100, 110}, [3]int{u.PromptTokens, u.CompletionTokens, u.TotalTokens})
			wire := rec.Body.String()
			require.Equal(t, 1, strings.Count(wire, "first text"))
			if format == types.RelayFormatClaude {
				require.Equal(t, 1, strings.Count(wire, `"type":"message_start"`))
				require.Equal(t, 1, strings.Count(wire, `"type":"message_stop"`))
				require.Contains(t, wire, `"input_tokens":10`)
				require.Contains(t, wire, `"output_tokens":100`)
			} else {
				require.Contains(t, wire, `"finishReason":"STOP"`)
				require.Contains(t, wire, `"totalTokenCount":110`)
			}
		})
	}
}

func TestOpenaiHandlerPreservesUpstreamChatUsage(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	body := `{
		"id":"resp_test",
		"object":"chat.completion",
		"created":1,
		"model":"gpt-test",
		"choices":[],
		"usage":{
			"prompt_tokens":100,
			"completion_tokens":20,
			"total_tokens":120,
			"prompt_tokens_details":{"cached_tokens":80,"audio_tokens":0},
			"completion_tokens_details":{"reasoning_tokens":0,"audio_tokens":0,"accepted_prediction_tokens":3,"rejected_prediction_tokens":4},
			"upstream_extension":"preserved"
		}
	}`
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}
	info := &relaycommon.RelayInfo{
		RelayFormat: types.RelayFormatOpenAI,
		ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeOpenAI},
	}

	usage, err := OpenaiHandler(c, info, resp)

	require.Nil(t, err)
	require.Equal(t, 100, usage.PromptTokens)
	require.JSONEq(t, body, recorder.Body.String())
}

func TestOpenaiHandlerModelFallbackPreservesUpstreamChatUsage(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	body := `{
		"id":"resp_test",
		"object":"chat.completion",
		"created":1,
		"model":"fallback-model",
		"choices":[],
		"usage":{
			"prompt_tokens":100,
			"completion_tokens":20,
			"total_tokens":120,
			"completion_tokens_details":{"accepted_prediction_tokens":3,"rejected_prediction_tokens":4},
			"upstream_extension":"preserved"
		}
	}`
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}
	info := &relaycommon.RelayInfo{
		OriginModelName:        "requested-model",
		ModelFallbackAttempted: true,
		RelayFormat:            types.RelayFormatOpenAI,
		ChannelMeta:            &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeOpenAI},
	}

	_, err := OpenaiHandler(c, info, resp)
	require.Nil(t, err)

	var payload map[string]any
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	require.Equal(t, "requested-model", payload["model"])
	usage := payload["usage"].(map[string]any)
	require.Equal(t, "preserved", usage["upstream_extension"])
	require.NotContains(t, usage, "claude_cache_creation_5_m_tokens")
	require.NotContains(t, usage, "input_tokens")
	completionDetails := usage["completion_tokens_details"].(map[string]any)
	require.EqualValues(t, 3, completionDetails["accepted_prediction_tokens"])
	require.EqualValues(t, 4, completionDetails["rejected_prediction_tokens"])
}

func TestOpenaiHandlerEstimatedUsageUsesChatCompletionSchema(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	body := `{
		"id":"resp_test",
		"object":"chat.completion",
		"created":1,
		"model":"gpt-test",
		"choices":[{"index":0,"message":{"role":"assistant","content":"hello"},"finish_reason":"stop"}],
		"usage":{"completion_tokens":2}
	}`
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}
	info := &relaycommon.RelayInfo{
		RelayFormat: types.RelayFormatOpenAI,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:       constant.ChannelTypeOpenAI,
			UpstreamModelName: "gpt-test",
		},
	}

	info.SetEstimatePromptTokens(17)
	actual, err := OpenaiHandler(c, info, resp)
	require.Nil(t, err)
	require.Equal(t, 17, actual.PromptTokens)
	require.Equal(t, 19, actual.TotalTokens)

	var payload map[string]any
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	usage := payload["usage"].(map[string]any)
	require.Len(t, usage, 3)
	require.Contains(t, usage, "prompt_tokens")
	require.Contains(t, usage, "completion_tokens")
	require.Contains(t, usage, "total_tokens")
	require.NotContains(t, usage, "claude_cache_creation_5_m_tokens")
	require.NotContains(t, usage, "input_tokens")
	require.NotContains(t, usage, "prompt_tokens_details")
}

func TestOpenaiHandlerFillsMissingCountersWithoutDroppingDetails(t *testing.T) {
	for _, tc := range []struct {
		name, counters string
		p, c, total    int
	}{
		{"explicit zero input", `"prompt_tokens":0,"completion_tokens":100,"total_tokens":100`, 0, 100, 100},
		{"missing input, reported total", `"completion_tokens":100,"total_tokens":113`, 13, 100, 113},
		{"missing input and total", `"completion_tokens":100`, 17, 100, 117},
		{"missing output", `"prompt_tokens":0,"total_tokens":100`, 0, 100, 100},
		{"positive input, missing output", `"prompt_tokens":10,"total_tokens":110`, 10, 100, 110},
		{"positive input, missing total", `"prompt_tokens":10,"completion_tokens":100`, 10, 100, 110},
		{"positive input, null output", `"prompt_tokens":10,"completion_tokens":null,"total_tokens":110`, 10, 100, 110},
		{"explicit zero output", `"prompt_tokens":10,"completion_tokens":0`, 10, 0, 10},
		{"positive input only, estimate reply", `"prompt_tokens":10`, 10, 1, 11},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			audio := 0
			if tc.c >= 20 {
				audio = 20
			}
			body := fmt.Sprintf(`{"choices":[{"message":{"role":"assistant","content":"hello"}}],"usage":{%s,"completion_tokens_details":{"audio_tokens":%d,"custom_count":9007199254740993}}}`, tc.counters, audio)
			info := &relaycommon.RelayInfo{RelayFormat: types.RelayFormatOpenAI, ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeOpenAI, UpstreamModelName: "gpt-4o"}}
			info.SetEstimatePromptTokens(17)
			usage, err := OpenaiHandler(c, info, &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))})
			require.Nil(t, err)
			require.Equal(t, tc.p, usage.PromptTokens)
			require.Equal(t, tc.c, usage.CompletionTokens)
			require.Equal(t, tc.total, usage.TotalTokens)
			require.Equal(t, audio, usage.CompletionTokenDetails.AudioTokens)
			wire := gjson.GetBytes(rec.Body.Bytes(), "usage")
			for _, key := range []string{"prompt_tokens", "completion_tokens", "total_tokens"} {
				require.Equal(t, gjson.Number, wire.Get(key).Type, key)
			}
			require.Equal(t, int64(tc.p), wire.Get("prompt_tokens").Int())
			require.Equal(t, int64(tc.c), wire.Get("completion_tokens").Int())
			require.Equal(t, int64(tc.total), wire.Get("total_tokens").Int())
			require.Equal(t, int64(audio), wire.Get("completion_tokens_details.audio_tokens").Int())
			require.Equal(t, "9007199254740993", wire.Get("completion_tokens_details.custom_count").Raw)
		})
	}
}

func TestOpenaiHandlerEstimatesUnreportedUsageButRespectsReportedZeros(t *testing.T) {
	for _, suffix := range []string{"", `,"usage":null`, `,"usage":{}`, `,"usage":{"prompt_tokens":0,"completion_tokens":0,"total_tokens":0}`} {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
		info := &relaycommon.RelayInfo{RelayFormat: types.RelayFormatOpenAI, ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeOpenAI, UpstreamModelName: "gpt-4o"}}
		info.SetEstimatePromptTokens(17)
		body := `{"model":"gpt-4o","choices":[{"message":{"role":"assistant","content":"hello"}}]` + suffix + "}"
		usage, err := OpenaiHandler(c, info, &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))})
		require.Nil(t, err)
		if strings.Contains(suffix, "prompt_tokens") {
			require.Zero(t, usage.PromptTokens)
			require.Zero(t, usage.CompletionTokens)
			require.JSONEq(t, body, rec.Body.String())
		} else {
			require.Equal(t, 17, usage.PromptTokens)
			require.Positive(t, usage.CompletionTokens)
			require.Equal(t, 17+usage.CompletionTokens, usage.TotalTokens)
			require.EqualValues(t, usage.TotalTokens, gjson.GetBytes(rec.Body.Bytes(), "usage.total_tokens").Int())
		}
	}
}

func TestOaiResponsesToChatHandlerSeparatesPublicAndBillingUsage(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	body := `{
		"id":"resp_test",
		"object":"response",
		"created_at":1,
		"model":"gpt-test",
		"output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"hello"}]}],
		"usage":{
			"input_tokens":100,
			"output_tokens":20,
			"total_tokens":120,
			"input_tokens_details":{"cached_tokens":80},
			"output_tokens_details":{"reasoning_tokens":5}
		}
	}`
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}
	info := &relaycommon.RelayInfo{
		RelayFormat: types.RelayFormatOpenAI,
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gpt-test"},
	}

	usage, err := OaiResponsesToChatHandler(c, info, resp)
	require.Nil(t, err)
	require.Equal(t, 100, usage.InputTokens)
	require.Equal(t, 20, usage.OutputTokens)

	var payload map[string]any
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	publicUsage := payload["usage"].(map[string]any)
	require.EqualValues(t, 100, publicUsage["prompt_tokens"])
	require.EqualValues(t, 20, publicUsage["completion_tokens"])
	require.NotContains(t, publicUsage, "input_tokens")
	require.NotContains(t, publicUsage, "output_tokens")
	require.NotContains(t, publicUsage, "input_tokens_details")
	require.NotContains(t, publicUsage, "output_tokens_details")
	require.NotContains(t, publicUsage, "claude_cache_creation_5_m_tokens")
	require.EqualValues(t, 80, publicUsage["prompt_tokens_details"].(map[string]any)["cached_tokens"])
	require.EqualValues(t, 5, publicUsage["completion_tokens_details"].(map[string]any)["reasoning_tokens"])
}

func TestOpenaiHandlerWithUsageMapsOutputTokenDetails(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	body := `{
		"created": 1710000000,
		"data": [{"url":"https://example.com/image.png"}],
		"usage": {
			"input_tokens": 12,
			"output_tokens": 1764,
			"total_tokens": 1776,
			"input_tokens_details": {
				"text_tokens": 10,
				"image_tokens": 2
			},
			"output_tokens_details": {
				"image_tokens": 1764,
				"text_tokens": 0
			}
		}
	}`
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}

	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeOpenAI},
	}

	usage, err := OpenaiHandlerWithUsage(c, info, resp)

	require.Nil(t, err)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, 12, usage.PromptTokens)
	require.Equal(t, 1764, usage.CompletionTokens)
	require.Equal(t, 2, usage.PromptTokensDetails.ImageTokens)
	require.Equal(t, 10, usage.PromptTokensDetails.TextTokens)
	require.Equal(t, 1764, usage.CompletionTokenDetails.ImageTokens)
	require.Equal(t, 0, usage.CompletionTokenDetails.TextTokens)
}

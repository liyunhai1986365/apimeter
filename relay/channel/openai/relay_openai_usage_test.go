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
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

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
		"usage":{"prompt_tokens":0,"completion_tokens":2,"total_tokens":2}
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

	_, err := OpenaiHandler(c, info, resp)
	require.Nil(t, err)

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
		"data": [],
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

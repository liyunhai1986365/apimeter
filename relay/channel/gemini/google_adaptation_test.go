package gemini

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func googleTestContext(t *testing.T) (*gin.Context, *httptest.ResponseRecorder, *relaycommon.RelayInfo) {
	t.Helper()
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 300
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	info := &relaycommon.RelayInfo{OriginModelName: "gemini-3-flash-preview", RelayFormat: types.RelayFormatOpenAI, ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gemini-3-flash-preview"}}
	info.SetEstimatePromptTokens(99)
	return ctx, recorder, info
}

func TestGoogleStreamUsageAndSearchAcrossPartialFrames(t *testing.T) {
	ctx, _, info := googleTestContext(t)
	body := `data: {"candidates":[{"content":{"parts":[{"text":"Hello"}]},"groundingMetadata":{"webSearchQueries":[" cat ","cat",""]}}],"usageMetadata":{"promptTokenCount":100,"cachedContentTokenCount":40,"thoughtsTokenCount":7,"promptTokensDetails":[{"modality":"AUDIO","tokenCount":20}]}}

data: {"candidates":[{"groundingMetadata":{"webSearchQueries":["cat","dog"],"imageSearchQueries":["dog","bird"]}}],"usageMetadata":{"candidatesTokenCount":30,"candidatesTokensDetails":[{"modality":"IMAGE","tokenCount":10}]}}

data: {"candidates":[{"finishReason":"STOP","groundingMetadata":{"webSearchQueries":["bird"]}}],"usageMetadata":{"candidatesTokenCount":40,"candidatesTokensDetails":[{"modality":"IMAGE","tokenCount":20}]}}

data: [DONE]

`
	usage, apiErr := GeminiChatStreamHandler(ctx, info, &http.Response{Body: io.NopCloser(strings.NewReader(body))})
	require.Nil(t, apiErr)
	require.Equal(t, 100, usage.PromptTokens)
	require.Equal(t, 47, usage.CompletionTokens)
	require.Equal(t, 40, usage.PromptTokensDetails.CachedTokens)
	require.Equal(t, 20, usage.PromptTokensDetails.AudioTokens)
	require.Equal(t, 20, usage.CompletionTokenDetails.ImageTokens)
	require.Equal(t, 147, usage.TotalTokens)
	require.Equal(t, 40, usage.BillingUsage.GeminiUsageMetadata.CandidatesTokenCount)
	require.Equal(t, 3, info.ResponsesUsageInfo.BuiltInTools[dto.BuildInToolGoogleSearch].CallCount)
	require.Equal(t, 1, info.ResponsesUsageInfo.BuiltInTools[relaycommon.GoogleSearchGroundedPromptTool].CallCount)
}

func TestGoogleEmbeddingsPreferUpstreamUsage(t *testing.T) {
	for _, native := range []bool{false, true} {
		for _, batch := range []bool{false, true} {
			for _, present := range []bool{false, true} {
				ctx, recorder, info := googleTestContext(t)
				info.IsGeminiBatchEmbedding = batch
				body := `{"embedding":{"values":[0.5]},"embeddings":[{"values":[0.5]}]`
				if present {
					body += `,"usageMetadata":{"promptTokenCount":12,"promptTokenDetails":[{"modality":"IMAGE","tokenCount":5},{"modality":"TEXT","tokenCount":7}]}`
				}
				body += "}"
				response := &http.Response{Body: io.NopCloser(strings.NewReader(body))}
				var usage *dto.Usage
				var apiErr *types.NewAPIError
				if native {
					usage, apiErr = NativeGeminiEmbeddingHandler(ctx, response, info)
				} else {
					usage, apiErr = GeminiEmbeddingHandler(ctx, info, response)
				}
				require.Nil(t, apiErr)
				if present {
					require.Equal(t, 12, usage.PromptTokens)
					require.Equal(t, 5, usage.PromptTokensDetails.ImageTokens)
					require.False(t, usage.BillingUsage.Estimated)
				} else {
					require.Equal(t, 99, usage.PromptTokens)
					require.True(t, usage.BillingUsage.Estimated)
				}
				if native {
					require.JSONEq(t, body, recorder.Body.String())
				}
			}
		}
	}
}

func TestGoogleHostStreamReconstructsFunctionAndRejectsTruncation(t *testing.T) {
	first := `data: {"candidates":[{"content":{"parts":[{"functionCall":{"id":"call_1","name":"run","partialArgs":[{"jsonPath":"$.input","stringValue":"hello"}],"willContinue":true}}]}}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":2}}

`
	last := `data: {"candidates":[{"content":{"parts":[{"functionCall":{"partialArgs":[{"jsonPath":"$.input","stringValue":" world"}],"willContinue":false}}]},"finishReason":"STOP"}]}

`
	for _, responses := range []bool{false, true} {
		for _, complete := range []bool{false, true} {
			ctx, recorder, info := googleTestContext(t)
			body := first
			if complete {
				body += last
			}
			body += "data: [DONE]\n\n"
			response := &http.Response{Body: io.NopCloser(strings.NewReader(body))}
			var apiErr *types.NewAPIError
			if responses {
				info.RelayFormat = types.RelayFormatOpenAIResponses
				_, apiErr = GeminiResponsesStreamHandler(ctx, info, response)
			} else {
				_, apiErr = GeminiChatStreamHandler(ctx, info, response)
			}
			if complete {
				require.Nil(t, apiErr)
				require.Contains(t, recorder.Body.String(), "hello world")
				require.Contains(t, recorder.Body.String(), "call_1")
			} else {
				require.NotNil(t, apiErr)
				require.NotContains(t, recorder.Body.String(), "response.completed")
				require.NotContains(t, recorder.Body.String(), `"finish_reason":"tool_calls"`)
			}
		}
	}
}

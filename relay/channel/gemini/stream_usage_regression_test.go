package gemini

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type disconnectedGoogleWriter struct {
	gin.ResponseWriter
	cancel context.CancelFunc
}

func (w disconnectedGoogleWriter) Write([]byte) (int, error) { w.cancel(); return 0, io.ErrClosedPipe }
func (w disconnectedGoogleWriter) WriteString(string) (int, error) {
	w.cancel()
	return 0, io.ErrClosedPipe
}

func TestGoogleFailedStreamsCarryUsageForSettlement(t *testing.T) {
	for _, protocol := range []string{"chat", "responses", "claude"} {
		for _, failure := range []string{"partial", "disconnect"} {
			t.Run(protocol+"/"+failure, func(t *testing.T) {
				ctx, _, info := googleTestContext(t)
				info.IsStream = true
				if protocol == "claude" {
					info.RelayFormat = types.RelayFormatClaude
				}
				payload := `{"candidates":[{"content":{"parts":[{"text":"hello"}]}}],"usageMetadata":{"promptTokenCount":100,"candidatesTokenCount":20}}`
				if failure == "partial" {
					payload = `{"candidates":[{"content":{"parts":[{"functionCall":{"id":"a","name":"run","partialArgs":[{"jsonPath":"$.text","stringValue":"x"}],"willContinue":true}}]}}],"usageMetadata":{"promptTokenCount":100,"candidatesTokenCount":20}}`
				} else {
					requestContext, cancel := context.WithCancel(ctx.Request.Context())
					defer cancel()
					ctx.Request = ctx.Request.WithContext(requestContext)
					ctx.Writer = disconnectedGoogleWriter{ctx.Writer, cancel}
				}
				resp := &http.Response{Body: io.NopCloser(strings.NewReader("data: " + payload + "\n\ndata: [DONE]\n\n"))}
				var usage *dto.Usage
				var apiErr *types.NewAPIError
				if protocol == "responses" {
					usage, apiErr = GeminiResponsesStreamHandler(ctx, info, resp)
				} else {
					usage, apiErr = GeminiChatStreamHandler(ctx, info, resp)
				}
				if failure == "disconnect" && protocol != "chat" {
					// Legacy Responses/Claude writers swallow write errors, but still
					// return usage for the ordinary successful settlement path.
					require.Nil(t, apiErr)
					require.Equal(t, 100, usage.PromptTokens)
					require.Equal(t, 20, usage.CompletionTokens)
					return
				}
				require.NotNil(t, apiErr)
				require.True(t, types.IsSkipRetryError(apiErr))
				var marker *relaycommon.StreamUsageError
				require.True(t, errors.As(apiErr, &marker))
				calls := 0
				for i := 0; i < 2; i++ {
					marker.SettleOnce(func(got *dto.Usage) {
						calls++
						require.Same(t, usage, got)
						require.Equal(t, 100, got.PromptTokens)
						require.Equal(t, 20, got.CompletionTokens)
					})
				}
				require.Equal(t, 1, calls)
			})
		}
	}
}

func TestGoogleContinuationStreamUsesLengthTermination(t *testing.T) {
	for _, protocol := range []string{"chat", "responses"} {
		t.Run(protocol, func(t *testing.T) {
			ctx, recorder, info := googleTestContext(t)
			info.IsStream = true
			payload := `data: {"candidates":[{"content":{"parts":[{"text":"partial answer"}]},"finishReason":"CONTINUATION"}],"usageMetadata":{"promptTokenCount":100,"candidatesTokenCount":20}}` + "\n\ndata: [DONE]\n\n"
			resp := &http.Response{Body: io.NopCloser(strings.NewReader(payload))}
			var usage *dto.Usage
			var apiErr *types.NewAPIError
			if protocol == "responses" {
				usage, apiErr = GeminiResponsesStreamHandler(ctx, info, resp)
			} else {
				usage, apiErr = GeminiChatStreamHandler(ctx, info, resp)
			}
			require.Nil(t, apiErr)
			require.Equal(t, 100, usage.PromptTokens)
			require.Equal(t, 20, usage.CompletionTokens)
			require.NotContains(t, recorder.Body.String(), "content_filter")
			if protocol == "responses" {
				require.Contains(t, recorder.Body.String(), `"reason":"max_output_tokens"`)
				require.Contains(t, recorder.Body.String(), "response.incomplete")
			} else {
				require.Contains(t, recorder.Body.String(), `"finish_reason":"length"`)
			}
		})
	}
}

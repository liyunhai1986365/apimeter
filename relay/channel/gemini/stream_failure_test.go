package gemini

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

const partialGeminiTextStream = "data: " + `{"candidates":[{"content":{"parts":[{"text":"partial answer"}]}}],"usageMetadata":{"promptTokenCount":100,"candidatesTokenCount":20}}` + "\n\n"

type interruptedGeminiReader struct{}

func (interruptedGeminiReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

func TestGoogleSearchOnlyInterruptedStreamPreservesSettlement(t *testing.T) {
	for _, format := range []types.RelayFormat{types.RelayFormatOpenAI, types.RelayFormatOpenAIResponses} {
		t.Run(string(format), func(t *testing.T) {
			c, recorder, info := googleTestContext(t)
			info.IsStream, info.DisablePing, info.RelayFormat = true, true, format
			// CountToken=false allows a real request to have no local estimate.
			info.SetEstimatePromptTokens(0)
			frame := `data: {"candidates":[{"groundingMetadata":{"webSearchQueries":["latest weather"]}}]}` + "\n\n"
			response := &http.Response{Body: io.NopCloser(io.MultiReader(strings.NewReader(frame), interruptedGeminiReader{}))}
			var usage *dto.Usage
			var apiErr *types.NewAPIError
			if format == types.RelayFormatOpenAIResponses {
				usage, apiErr = GeminiResponsesStreamHandler(c, info, response)
			} else {
				usage, apiErr = GeminiChatStreamHandler(c, info, response)
			}
			require.NotNil(t, apiErr)
			require.True(t, types.IsSkipRetryError(apiErr))
			require.NotNil(t, usage)
			require.Zero(t, usage.TotalTokens)
			require.Equal(t, 1, info.ResponsesUsageInfo.BuiltInTools[dto.BuildInToolGoogleSearch].CallCount)
			require.Equal(t, 1, info.ResponsesUsageInfo.BuiltInTools[relaycommon.GoogleSearchGroundedPromptTool].CallCount)
			var marker *relaycommon.StreamUsageError
			require.ErrorAs(t, apiErr, &marker)
			marker.SettleOnce(func(got *dto.Usage) { require.Same(t, usage, got) })
			require.NotContains(t, recorder.Body.String(), "response.completed")
			if format == types.RelayFormatOpenAIResponses {
				require.Equal(t, 1, strings.Count(recorder.Body.String(), "event: response.failed\n"))
			}
		})
	}
}

func TestGoogleAbnormalStreamsPreserveUsageWithoutNormalCompletion(t *testing.T) {
	for _, protocol := range []string{"chat", "responses"} {
		for _, failure := range []string{"read", "parse"} {
			t.Run(protocol+"/"+failure, func(t *testing.T) {
				ctx, recorder, info := googleTestContext(t)
				info.IsStream, info.DisablePing = true, true
				var tail io.Reader = interruptedGeminiReader{}
				if failure == "parse" {
					tail = strings.NewReader("data: {invalid json}\n\n")
				}
				resp := &http.Response{Body: io.NopCloser(io.MultiReader(strings.NewReader(partialGeminiTextStream), tail))}
				var usage *dto.Usage
				var apiErr *types.NewAPIError
				if protocol == "responses" {
					info.RelayFormat = types.RelayFormatOpenAIResponses
					usage, apiErr = GeminiResponsesStreamHandler(ctx, info, resp)
				} else {
					usage, apiErr = GeminiChatStreamHandler(ctx, info, resp)
				}
				require.NotNil(t, apiErr)
				require.Equal(t, http.StatusBadGateway, apiErr.StatusCode)
				require.True(t, types.IsSkipRetryError(apiErr))
				require.Equal(t, 100, usage.PromptTokens)
				require.Equal(t, 20, usage.CompletionTokens)
				var marker *relaycommon.StreamUsageError
				require.True(t, errors.As(apiErr, &marker))
				settlements := 0
				for i := 0; i < 2; i++ {
					marker.SettleOnce(func(got *dto.Usage) {
						settlements++
						require.Same(t, usage, got)
					})
				}
				require.Equal(t, 1, settlements)
				if failure == "read" {
					require.ErrorIs(t, info.StreamStatus.EndError, io.ErrUnexpectedEOF)
				} else {
					require.True(t, info.StreamStatus.HasErrors())
				}
				body := recorder.Body.String()
				require.Contains(t, body, "partial answer")
				require.NotContains(t, body, `"finish_reason":"stop"`)
				require.NotContains(t, body, "[DONE]")
				require.NotContains(t, body, "response.completed")
				if protocol == "responses" {
					require.True(t, common.GetContextKeyBool(ctx, constant.ContextKeyRelayFailed))
					require.True(t, common.GetContextKeyBool(ctx, constant.ContextKeyRelayStreamErrorSent))
					require.Equal(t, 1, strings.Count(body, "event: response.failed\n"))
					failed := readGeminiFailedResponse(t, body)
					require.Equal(t, `"failed"`, string(failed.Status))
					require.NotNil(t, failed.Error)
					require.Equal(t, 100, failed.Usage.InputTokens)
					require.Equal(t, 20, failed.Usage.OutputTokens)
					require.Len(t, failed.Output, 1)
					require.Equal(t, "incomplete", failed.Output[0].Status)
					require.Equal(t, "partial answer", failed.Output[0].Content[0].Text)
				}
			})
		}
	}
}

func TestGoogleResponsesTailFailurePreservesCompletedItems(t *testing.T) {
	c, recorder, info := googleTestContext(t)
	info.IsStream, info.DisablePing, info.RelayFormat = true, true, types.RelayFormatOpenAIResponses
	frame := `data: {"candidates":[{"content":{"parts":[{"text":"finished answer"},{"functionCall":{"id":"call_done","name":"run","args":{"ok":true}}}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":100,"candidatesTokenCount":20}}` + "\n\n"
	usage, apiErr := GeminiResponsesStreamHandler(c, info, &http.Response{Body: io.NopCloser(io.MultiReader(strings.NewReader(frame), interruptedGeminiReader{}))})
	require.NotNil(t, apiErr)
	require.True(t, types.IsSkipRetryError(apiErr))
	require.Equal(t, 100, usage.PromptTokens)
	require.Equal(t, 20, usage.CompletionTokens)
	var marker *relaycommon.StreamUsageError
	require.True(t, errors.As(apiErr, &marker))
	marker.SettleOnce(func(actual *dto.Usage) { require.Same(t, usage, actual) })
	require.True(t, common.GetContextKeyBool(c, constant.ContextKeyRelayFailed))
	require.True(t, common.GetContextKeyBool(c, constant.ContextKeyRelayStreamErrorSent))

	body := recorder.Body.String()
	done := make(map[string]string)
	for _, line := range strings.Split(body, "\n") {
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var event dto.ResponsesStreamResponse
		require.NoError(t, common.UnmarshalJsonStr(strings.TrimPrefix(line, "data: "), &event))
		if event.Type == "response.output_item.done" {
			require.NotNil(t, event.Item)
			require.NotContains(t, done, event.Item.ID, "item must not emit done twice")
			require.Equal(t, "completed", event.Item.Status)
			done[event.Item.ID] = event.Item.Status
		}
	}
	require.Len(t, done, 2)
	failed := readGeminiFailedResponse(t, body)
	require.Equal(t, `"failed"`, string(failed.Status))
	require.Len(t, failed.Output, 2)
	for _, item := range failed.Output {
		require.Equal(t, "completed", item.Status)
		require.Equal(t, done[item.ID], item.Status)
	}
	require.Equal(t, 1, strings.Count(body, "event: response.failed\n"))
	require.NotContains(t, body, "event: response.completed\n")
}

func TestGoogleResponsesMultiCandidateFailurePreservesClosedMessage(t *testing.T) {
	c, recorder, info := googleTestContext(t)
	info.IsStream, info.DisablePing, info.RelayFormat = true, true, types.RelayFormatOpenAIResponses
	first := `data: {"candidates":[{"index":0,"content":{"parts":[{"text":"first completed"}]},"finishReason":"STOP"}]}` + "\n\n"
	second := `data: {"candidates":[{"index":1,"content":{"parts":[{"text":"second unfinished"}]}}],"usageMetadata":{"promptTokenCount":100,"candidatesTokenCount":20,"totalTokenCount":120}}` + "\n\n"
	body := io.MultiReader(strings.NewReader(first+second), interruptedGeminiReader{})
	usage, apiErr := GeminiResponsesStreamHandler(c, info, &http.Response{Body: io.NopCloser(body)})
	require.NotNil(t, apiErr)
	require.True(t, types.IsSkipRetryError(apiErr))
	require.ErrorIs(t, info.StreamStatus.EndError, io.ErrUnexpectedEOF)
	require.NotNil(t, usage)
	require.Equal(t, 120, usage.TotalTokens)
	var marker *relaycommon.StreamUsageError
	require.ErrorAs(t, apiErr, &marker)
	marker.SettleOnce(func(got *dto.Usage) { require.Same(t, usage, got) })

	stream := recorder.Body.String()
	done := make(map[string]dto.ResponsesOutput)
	indexes := make(map[string]int)
	for _, line := range strings.Split(stream, "\n") {
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var event dto.ResponsesStreamResponse
		require.NoError(t, common.UnmarshalJsonStr(strings.TrimPrefix(line, "data: "), &event))
		switch event.Type {
		case "response.output_item.added":
			require.NotContains(t, indexes, event.Item.ID)
			require.Equal(t, len(indexes), *event.OutputIndex)
			indexes[event.Item.ID] = *event.OutputIndex
		case "response.output_text.delta":
			require.Contains(t, indexes, event.ItemID)
			require.NotContains(t, done, event.ItemID, "a closed message cannot receive another candidate's text")
			require.Equal(t, indexes[event.ItemID], *event.OutputIndex)
		case "response.output_item.done":
			require.NotContains(t, done, event.Item.ID)
			done[event.Item.ID] = *event.Item
		}
	}
	failed := readGeminiFailedResponse(t, stream)
	require.Equal(t, `"failed"`, string(failed.Status))
	require.Equal(t, 120, failed.Usage.TotalTokens)
	require.Len(t, done, 2)
	require.Len(t, failed.Output, 2)
	for index, item := range failed.Output {
		require.Equal(t, done[item.ID], item, "closed message content and status must remain immutable")
		require.Equal(t, index, indexes[item.ID])
	}
	require.Equal(t, "first completed", failed.Output[0].Content[0].Text)
	require.Equal(t, "completed", failed.Output[0].Status)
	require.Equal(t, "second unfinished", failed.Output[1].Content[0].Text)
	require.Equal(t, "incomplete", failed.Output[1].Status)
	require.Equal(t, 1, strings.Count(stream, "event: response.failed\n"))
	require.NotContains(t, stream, "event: response.completed\n")
}

func readGeminiFailedResponse(t *testing.T, body string) *dto.OpenAIResponsesResponse {
	t.Helper()
	for _, line := range strings.Split(body, "\n") {
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var event dto.ResponsesStreamResponse
		require.NoError(t, common.UnmarshalJsonStr(strings.TrimPrefix(line, "data: "), &event))
		if event.Type == "response.failed" {
			require.NotNil(t, event.Response)
			return event.Response
		}
	}
	t.Fatal("missing response.failed payload")
	return nil
}

type failedTerminalGoogleWriter struct{ gin.ResponseWriter }

func (w failedTerminalGoogleWriter) Write(data []byte) (int, error) {
	if strings.Contains(string(data), "event: response.failed") {
		return 0, io.ErrClosedPipe
	}
	return w.ResponseWriter.Write(data)
}

func TestGoogleResponsesFailureMarksOnlyDeliveredTerminal(t *testing.T) {
	ctx, recorder, info := googleTestContext(t)
	info.IsStream, info.DisablePing = true, true
	info.RelayFormat = types.RelayFormatOpenAIResponses
	ctx.Writer = failedTerminalGoogleWriter{ctx.Writer}
	body := io.MultiReader(strings.NewReader(partialGeminiTextStream), interruptedGeminiReader{})
	usage, apiErr := GeminiResponsesStreamHandler(ctx, info, &http.Response{Body: io.NopCloser(body)})
	require.NotNil(t, apiErr)
	require.NotNil(t, usage)
	require.True(t, common.GetContextKeyBool(ctx, constant.ContextKeyRelayFailed))
	require.False(t, common.GetContextKeyBool(ctx, constant.ContextKeyRelayStreamErrorSent))
	require.NotContains(t, recorder.Body.String(), "event: response.failed")
	require.NotContains(t, recorder.Body.String(), "response.completed")
	var marker *relaycommon.StreamUsageError
	require.True(t, errors.As(apiErr, &marker))
}

func TestGoogleNormalStreamFinalizationRemainsCompatible(t *testing.T) {
	usage := "data: " + `{"usageMetadata":{"promptTokenCount":100,"candidatesTokenCount":20}}` + "\n\n"
	stop := "data: " + `{"candidates":[{"finishReason":"STOP"}]}` + "\n\n"
	for _, protocol := range []string{"chat", "responses"} {
		for _, test := range []struct{ name, body string }{
			{"text_eof", partialGeminiTextStream},
			{"usage_eof", usage},
			{"terminal_usage_eof", partialGeminiTextStream + stop + usage},
			{"done", partialGeminiTextStream + "data: [DONE]\n\n"},
		} {
			t.Run(protocol+"/"+test.name, func(t *testing.T) {
				ctx, recorder, info := googleTestContext(t)
				info.IsStream, info.DisablePing = true, true
				resp := &http.Response{Body: io.NopCloser(strings.NewReader(test.body))}
				var apiErr *types.NewAPIError
				if protocol == "responses" {
					info.RelayFormat = types.RelayFormatOpenAIResponses
					_, apiErr = GeminiResponsesStreamHandler(ctx, info, resp)
				} else {
					_, apiErr = GeminiChatStreamHandler(ctx, info, resp)
				}
				require.Nil(t, apiErr)
				require.Nil(t, info.StreamStatus.EndError)
				require.False(t, info.StreamStatus.HasErrors())
				require.False(t, common.GetContextKeyBool(ctx, constant.ContextKeyRelayFailed))
				require.False(t, common.GetContextKeyBool(ctx, constant.ContextKeyRelayStreamErrorSent))
				if protocol == "responses" {
					require.Equal(t, 1, strings.Count(recorder.Body.String(), "event: response.completed\n"))
				} else {
					require.Equal(t, 1, strings.Count(recorder.Body.String(), `"finish_reason":"stop"`))
				}
			})
		}
	}
}

type geminiErrorReader struct{ err error }

func (r geminiErrorReader) Read([]byte) (int, error) { return 0, r.err }

func TestGoogleResponsesFailureMasksNetworkEndpoints(t *testing.T) {
	c, recorder, info := googleTestContext(t)
	info.IsStream, info.DisablePing, info.RelayFormat = true, true, types.RelayFormatOpenAIResponses
	readErr := &net.OpError{Op: "read", Net: "tcp", Source: &net.TCPAddr{IP: net.ParseIP("10.10.10.7"), Port: 49100}, Addr: &net.TCPAddr{IP: net.ParseIP("192.0.2.99"), Port: 443}, Err: syscall.ECONNRESET}
	body := io.MultiReader(strings.NewReader(partialGeminiTextStream), geminiErrorReader{readErr})
	_, apiErr := GeminiResponsesStreamHandler(c, info, &http.Response{Body: io.NopCloser(body)})
	require.NotNil(t, apiErr)
	failed := readGeminiFailedResponse(t, recorder.Body.String())
	require.True(t, common.GetContextKeyBool(c, constant.ContextKeyRelayStreamErrorSent))
	require.Equal(t, apiErr.ToOpenAIError().Message, failed.GetOpenAIError().Message)
	require.NotContains(t, failed.GetOpenAIError().Message, "10.10.10.7")
	require.NotContains(t, failed.GetOpenAIError().Message, "192.0.2.99")
	// Internal diagnostics retain the original error and both network endpoints.
	require.Contains(t, apiErr.Error(), "10.10.10.7")
	require.Contains(t, apiErr.Error(), "192.0.2.99")
	require.ErrorIs(t, info.StreamStatus.EndError, readErr)
}

func TestGoogleResponsesFailurePreservesObservedWebSearch(t *testing.T) {
	c, recorder, info := googleTestContext(t)
	info.IsStream, info.DisablePing, info.RelayFormat = true, true, types.RelayFormatOpenAIResponses
	frame := `data: {"candidates":[{"content":{"parts":[{"text":"answer from search"}]},"groundingMetadata":{"webSearchQueries":["latest weather"]}}],"usageMetadata":{"promptTokenCount":100,"candidatesTokenCount":20}}` + "\n\n"
	usage, apiErr := GeminiResponsesStreamHandler(c, info, &http.Response{Body: io.NopCloser(io.MultiReader(strings.NewReader(frame), interruptedGeminiReader{}))})
	require.NotNil(t, apiErr)
	require.Equal(t, 100, usage.PromptTokens)
	require.Equal(t, 1, info.ResponsesUsageInfo.BuiltInTools[dto.BuildInToolGoogleSearch].CallCount)
	failed := readGeminiFailedResponse(t, recorder.Body.String())
	require.Equal(t, `"failed"`, string(failed.Status))
	require.Len(t, failed.Output, 2)
	require.Equal(t, "message", failed.Output[0].Type)
	require.Equal(t, "incomplete", failed.Output[0].Status)
	require.Equal(t, "web_search_call", failed.Output[1].Type)
	require.Equal(t, "completed", failed.Output[1].Status)
	require.Equal(t, 1, strings.Count(recorder.Body.String(), "event: response.failed\n"))
	require.NotContains(t, recorder.Body.String(), "event: response.completed\n")
}

func TestGoogleResponsesConversionFailurePreservesBufferedSearch(t *testing.T) {
	c, recorder, info := googleTestContext(t)
	info.IsStream, info.DisablePing, info.RelayFormat = true, true, types.RelayFormatOpenAIResponses
	brokenCall := `data: {"candidates":[{"content":{"parts":[{"functionCall":{"name":"run","partialArgs":[{"jsonPath":"invalid","stringValue":"x"}],"willContinue":true}}]}}]}` + "\n\n"
	bufferedText := `data: {"candidates":[{"content":{"parts":[{"text":"must not be sent"}]}}]}` + "\n\n"
	search := `data: {"candidates":[{"groundingMetadata":{"webSearchQueries":["latest weather"]}}],"usageMetadata":{"promptTokenCount":1000,"candidatesTokenCount":500,"totalTokenCount":1500}}` + "\n\n"
	body := &geminiNoopCloseReader{reader: strings.NewReader(partialGeminiTextStream + brokenCall + strings.Repeat(bufferedText, 30) + search)}
	usage, apiErr := GeminiResponsesStreamHandler(c, info, &http.Response{Body: body})
	require.NotNil(t, apiErr)
	require.Contains(t, apiErr.Error(), "unsupported Gemini partial argument path")
	require.Equal(t, 1500, usage.TotalTokens)
	require.Equal(t, 1, info.ResponsesUsageInfo.BuiltInTools[dto.BuildInToolGoogleSearch].CallCount)
	require.Zero(t, body.readsAfterClose.Load())
	failed := readGeminiFailedResponse(t, recorder.Body.String())
	require.Equal(t, `"failed"`, string(failed.Status))
	require.Len(t, failed.Output, 2)
	require.Equal(t, "partial answer", failed.Output[0].Content[0].Text)
	require.Equal(t, "incomplete", failed.Output[0].Status)
	require.Equal(t, "web_search_call", failed.Output[1].Type)
	require.Equal(t, "completed", failed.Output[1].Status)
	require.NotContains(t, recorder.Body.String(), "must not be sent")
	require.NotContains(t, recorder.Body.String(), "event: response.completed\n")
	require.Equal(t, 1, strings.Count(recorder.Body.String(), "event: response.failed\n"))
	require.True(t, common.GetContextKeyBool(c, constant.ContextKeyRelayStreamErrorSent))
}

// The next read either observes EOF or stays active until Close. This lets the
// downstream writer fail only after all test frames were read upstream.
type geminiBufferedUsageReader struct {
	reader    *strings.Reader
	active    bool
	readDone  chan struct{}
	closed    chan struct{}
	readOnce  sync.Once
	closeOnce sync.Once
}

func (r *geminiBufferedUsageReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	if err == io.EOF {
		r.readOnce.Do(func() { close(r.readDone) })
		if r.active {
			<-r.closed
			return n, io.ErrClosedPipe
		}
	}
	return n, err
}

func (r *geminiBufferedUsageReader) Close() error {
	r.closeOnce.Do(func() { close(r.closed) })
	return nil
}

type geminiFailWriteAfterRead struct {
	gin.ResponseWriter
	readDone <-chan struct{}
	writes   int
	cancel   context.CancelFunc
}

func (w *geminiFailWriteAfterRead) Write([]byte) (int, error) {
	<-w.readDone
	w.writes++
	if w.cancel != nil {
		w.cancel()
	}
	return 0, io.ErrClosedPipe
}

func (w *geminiFailWriteAfterRead) WriteString(s string) (int, error) { return w.Write([]byte(s)) }

func TestGoogleResponsesWriteErrorRetainsBufferedUsage(t *testing.T) {
	for _, active := range []bool{false, true} {
		t.Run(map[bool]string{false: "upstream_eof", true: "upstream_still_active"}[active], func(t *testing.T) {
			c, recorder, info := googleTestContext(t)
			info.IsStream, info.DisablePing, info.RelayFormat = true, true, types.RelayFormatOpenAIResponses
			stream := `data: {"candidates":[{"content":{"parts":[{"text":"hello"}]}}]}` + "\n\n" +
				`data: {"usageMetadata":{"promptTokenCount":1000,"candidatesTokenCount":500,"totalTokenCount":1500}}` + "\n\n"
			body := &geminiBufferedUsageReader{reader: strings.NewReader(stream), active: active, readDone: make(chan struct{}), closed: make(chan struct{})}
			writer := &geminiFailWriteAfterRead{ResponseWriter: c.Writer, readDone: body.readDone}
			c.Writer = writer
			// A broken drain implementation must not wait for an active upstream
			// to finish. The watchdog bounds a failure without leaking a worker.
			var timedOut atomic.Bool
			watchdog := time.AfterFunc(2*time.Second, func() {
				timedOut.Store(true)
				_ = body.Close()
			})
			defer watchdog.Stop()
			usage, apiErr := GeminiResponsesStreamHandler(c, info, &http.Response{Body: body})
			require.False(t, timedOut.Load(), "handler waited for the active upstream")
			require.NotNil(t, apiErr)
			require.Equal(t, 2, info.ReceivedResponseCount)
			require.Equal(t, 1000, usage.PromptTokens)
			require.Equal(t, 500, usage.CompletionTokens)
			require.Equal(t, 1500, usage.TotalTokens)
			require.NotContains(t, recorder.Body.String(), "response.completed")
			require.False(t, common.GetContextKeyBool(c, constant.ContextKeyRelayStreamErrorSent))
			require.Equal(t, 1, writer.writes, "failed downstream connection must not be written again")
			var marker *relaycommon.StreamUsageError
			require.True(t, errors.As(apiErr, &marker))
			settlements := 0
			for i := 0; i < 2; i++ {
				marker.SettleOnce(func(actual *dto.Usage) {
					settlements++
					require.Same(t, usage, actual)
				})
			}
			require.Equal(t, 1, settlements)
			select {
			case <-body.closed:
			default:
				t.Fatal("upstream body was not closed")
			}
		})
	}
}

type geminiNoopCloseReader struct {
	reader          *strings.Reader
	closed          atomic.Bool
	readsAfterClose atomic.Int64
}

func (r *geminiNoopCloseReader) Read(p []byte) (int, error) {
	if r.closed.Load() {
		r.readsAfterClose.Add(1)
	}
	return r.reader.Read(p)
}

func (r *geminiNoopCloseReader) Close() error {
	r.closed.Store(true)
	return nil
}

func TestGoogleResponsesWriteErrorDrainsScannerBufferWithoutReadingAgain(t *testing.T) {
	for _, cancelRequest := range []bool{false, true} {
		t.Run(map[bool]string{false: "write_error", true: "write_error_and_cancel"}[cancelRequest], func(t *testing.T) {
			c, _, info := googleTestContext(t)
			info.IsStream, info.DisablePing, info.RelayFormat = true, true, types.RelayFormatOpenAIResponses
			// More frames than the callback queue can hold puts the authoritative usage
			// in bufio.Scanner's buffer when the first downstream write fails.
			frame := `data: {"candidates":[{"content":{"parts":[{"text":"hello"}]}}]}` + "\n\n"
			stream := strings.Repeat(frame, 30) + "data: {invalid json}\n\n" +
				`data: {"usageMetadata":{"promptTokenCount":1000,"candidatesTokenCount":500,"totalTokenCount":1500}}` + "\n\n"
			body := &geminiNoopCloseReader{reader: strings.NewReader(stream)}
			ready := make(chan struct{})
			close(ready)
			writer := &geminiFailWriteAfterRead{ResponseWriter: c.Writer, readDone: ready}
			if cancelRequest {
				ctx, cancel := context.WithCancel(c.Request.Context())
				defer cancel()
				c.Request = c.Request.WithContext(ctx)
				writer.cancel = cancel
			}
			c.Writer = writer
			usage, apiErr := GeminiResponsesStreamHandler(c, info, &http.Response{Body: body})
			require.NotNil(t, apiErr)
			require.Contains(t, apiErr.Error(), io.ErrClosedPipe.Error())
			require.Equal(t, 32, info.ReceivedResponseCount)
			require.Equal(t, 1000, usage.PromptTokens)
			require.Equal(t, 500, usage.CompletionTokens)
			require.Equal(t, 1500, usage.TotalTokens)
			require.True(t, body.closed.Load())
			require.Zero(t, body.readsAfterClose.Load(), "Close may be a no-op; draining must not start another read")
			require.Equal(t, 1, writer.writes)
		})
	}
}

type geminiWaitUntilClosedWriter struct {
	gin.ResponseWriter
	closed <-chan struct{}
	once   sync.Once
}

func (w *geminiWaitUntilClosedWriter) Write(p []byte) (int, error) {
	w.once.Do(func() { <-w.closed })
	return w.ResponseWriter.Write(p)
}

func (w *geminiWaitUntilClosedWriter) WriteString(s string) (int, error) { return w.Write([]byte(s)) }

func TestGoogleTimeoutDuringLastWriteDoesNotComplete(t *testing.T) {
	for _, protocol := range []string{"chat", "responses"} {
		t.Run(protocol, func(t *testing.T) {
			c, recorder, info := googleTestContext(t)
			info.IsStream, info.DisablePing = true, true
			constant.StreamingTimeout = 1
			body := &geminiBufferedUsageReader{reader: strings.NewReader(partialGeminiTextStream), readDone: make(chan struct{}), closed: make(chan struct{})}
			c.Writer = &geminiWaitUntilClosedWriter{ResponseWriter: c.Writer, closed: body.closed}
			var usage *dto.Usage
			var apiErr *types.NewAPIError
			if protocol == "responses" {
				info.RelayFormat = types.RelayFormatOpenAIResponses
				usage, apiErr = GeminiResponsesStreamHandler(c, info, &http.Response{Body: body})
			} else {
				usage, apiErr = GeminiChatStreamHandler(c, info, &http.Response{Body: body})
			}
			require.NotNil(t, apiErr)
			require.True(t, types.IsSkipRetryError(apiErr))
			require.Equal(t, 100, usage.PromptTokens)
			require.Equal(t, 20, usage.CompletionTokens)
			// The first EOF remains recorded, but the later callback timeout
			// still prevents successful finalization and carries billable usage.
			require.Equal(t, relaycommon.StreamEndReasonEOF, info.StreamStatus.EndReason)
			require.True(t, info.StreamStatus.HasErrors())
			var marker *relaycommon.StreamUsageError
			require.True(t, errors.As(apiErr, &marker))
			require.NotContains(t, recorder.Body.String(), "event: response.completed\n")
			require.NotContains(t, recorder.Body.String(), `"finish_reason":"stop"`)
			require.NotContains(t, recorder.Body.String(), "[DONE]")
			if protocol == "responses" {
				require.Equal(t, 1, strings.Count(recorder.Body.String(), "event: response.failed\n"))
			}
		})
	}
}

func TestGoogleResponsesLateGroundingPreservesFinishAndSnapshot(t *testing.T) {
	for _, finish := range []struct{ reason, status, incomplete string }{
		{"STOP", "completed", ""},
		{"MAX_TOKENS", "incomplete", "max_output_tokens"},
		{"SAFETY", "incomplete", "content_filter"},
	} {
		for _, failedTail := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/failed_tail=%t", finish.reason, failedTail), func(t *testing.T) {
				c, recorder, info := googleTestContext(t)
				info.IsStream, info.DisablePing, info.RelayFormat = true, true, types.RelayFormatOpenAIResponses
				first := "data: " + fmt.Sprintf(`{"candidates":[{"index":0,"content":{"parts":[{"text":"晴天"}]},"finishReason":%q}]}`, finish.reason) + "\n\n"
				grounding := `data: {"candidates":[{"index":0,"groundingMetadata":{"webSearchQueries":["weather"],"groundingChunks":[{"web":{"uri":"https://weather.example","title":"Weather"}}],"groundingSupports":[{"segment":{"partIndex":0,"startIndex":0,"endIndex":6,"text":"晴天"},"groundingChunkIndices":[0]}]}}],"usageMetadata":{"promptTokenCount":100,"candidatesTokenCount":20,"totalTokenCount":120}}` + "\n\n"
				var tail io.Reader = strings.NewReader("data: [DONE]\n\n")
				if failedTail {
					tail = interruptedGeminiReader{}
				}
				usage, apiErr := GeminiResponsesStreamHandler(c, info, &http.Response{Body: io.NopCloser(io.MultiReader(strings.NewReader(first+grounding), tail))})
				if failedTail {
					require.NotNil(t, apiErr)
					var marker *relaycommon.StreamUsageError
					require.ErrorAs(t, apiErr, &marker)
					marker.SettleOnce(func(got *dto.Usage) { require.Same(t, usage, got) })
				} else {
					require.Nil(t, apiErr)
				}
				require.Equal(t, 120, usage.TotalTokens)
				require.Equal(t, 1, info.ResponsesUsageInfo.BuiltInTools[dto.BuildInToolGoogleSearch].CallCount)
				var final *dto.OpenAIResponsesResponse
				var done *dto.ResponsesOutput
				var messageID string
				annotationSeen, textSeen := false, false
				terminalCount := 0
				for _, line := range strings.Split(recorder.Body.String(), "\n") {
					if !strings.HasPrefix(line, "data: ") {
						continue
					}
					var event dto.ResponsesStreamResponse
					require.NoError(t, common.UnmarshalJsonStr(strings.TrimPrefix(line, "data: "), &event))
					switch event.Type {
					case "response.output_text.delta":
						require.Nil(t, done)
						require.Equal(t, "晴天", event.Delta)
						messageID = event.ItemID
						textSeen = true
					case "response.output_text.annotation.added":
						require.True(t, textSeen, "the text must already have streamed")
						require.Nil(t, done, "citation must arrive before message.done")
						require.Equal(t, messageID, event.ItemID)
						annotationSeen = true
					case "response.output_item.done":
						if event.Item.Type == "message" {
							require.True(t, annotationSeen)
							require.Nil(t, done, "only the original message closes")
							done = event.Item
						}
					case "response.completed", "response.incomplete", "response.failed":
						terminalCount++
						want := "response." + finish.status
						if failedTail {
							want = "response.failed"
						}
						require.Equal(t, want, event.Type)
						final = event.Response
					}
				}
				require.Equal(t, 1, terminalCount)
				require.NotNil(t, done)
				require.NotNil(t, final)
				require.Len(t, final.Output, 2) // original message and hosted search
				require.Equal(t, *done, final.Output[0], "closed snapshot must not change")
				require.Equal(t, finish.status, done.Status)
				require.Equal(t, "晴天", done.Content[0].Text)
				require.Len(t, done.Content[0].Annotations, 1)
				citation := done.Content[0].Annotations[0].(map[string]any)
				require.Equal(t, float64(0), citation["start_index"])
				require.Equal(t, float64(2), citation["end_index"])
				require.Equal(t, "https://weather.example", citation["url"])
				require.Equal(t, 120, final.Usage.TotalTokens)
				if failedTail || finish.incomplete == "" {
					require.Nil(t, final.IncompleteDetails)
				} else {
					require.NotNil(t, final.IncompleteDetails)
					require.Equal(t, finish.incomplete, final.IncompleteDetails.Reason)
				}
			})
		}
	}
}

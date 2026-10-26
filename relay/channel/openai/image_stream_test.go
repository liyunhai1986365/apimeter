package openai

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/iotest"
	"time"

	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func newOpenAIImageStreamTest(t *testing.T, mode int) (*gin.Context, *httptest.ResponseRecorder, *relaycommon.RelayInfo) {
	t.Helper()
	previousTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = previousTimeout })
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeOpenAI},
		RelayMode:   mode, IsStream: true, DisablePing: true,
	}
	return c, recorder, info
}

func imageEvent(eventType, fields string) string {
	return "event: " + eventType + "\ndata: {\"type\":\"" + eventType + "\"," + fields + "}\n\n"
}

func TestOpenAIImageStreamForwardsGenerationAndEditEvents(t *testing.T) {
	for _, tt := range []struct {
		prefix string
		mode   int
	}{
		{"image_generation", relayconstant.RelayModeImagesGenerations},
		{"image_edit", relayconstant.RelayModeImagesEdits},
	} {
		t.Run(tt.prefix, func(t *testing.T) {
			c, recorder, info := newOpenAIImageStreamTest(t, tt.mode)
			// Detection must also work for custom model aliases via Content-Type.
			info.IsStream = false
			partial := imageEvent(tt.prefix+".partial_image", `"partial_image_index":0,"b64_json":"preview"`)
			completed := imageEvent(tt.prefix+".completed", `"b64_json":"final","size":"1024x1024","usage":{"input_tokens":12,"output_tokens":100,"total_tokens":112,"input_tokens_details":{"text_tokens":10,"image_tokens":2},"output_tokens_details":{"image_tokens":100}}`)
			resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream; charset=utf-8"}}, Body: io.NopCloser(strings.NewReader(partial + completed))}
			usage, apiErr := (&Adaptor{}).DoResponse(c, resp, info)
			require.Nil(t, apiErr)
			require.True(t, info.IsStream)
			require.Equal(t, "text/event-stream", recorder.Header().Get("Content-Type"))
			require.Equal(t, partial+completed, recorder.Body.String(), "preserve event names and original image payloads")
			got := usage.(*dto.Usage)
			require.Equal(t, 12, got.PromptTokens)
			require.Equal(t, 100, got.CompletionTokens)
			require.Equal(t, 112, got.TotalTokens)
			require.Equal(t, 2, got.PromptTokensDetails.ImageTokens)
			require.Equal(t, 10, got.PromptTokensDetails.TextTokens)
			require.Equal(t, 100, got.CompletionTokenDetails.ImageTokens)
		})
	}
}

func TestOpenAIImageStreamFailuresDoNotReturnUsage(t *testing.T) {
	partial := imageEvent("image_generation.partial_image", `"b64_json":"preview","partial_image_index":0`)
	for _, tt := range []struct {
		name    string
		body    string
		message string
	}{
		{"flat upstream error", imageEvent("error", `"message":"generation failed","code":"generation_failed"`), "generation failed"},
		{"nested upstream error", partial + imageEvent("error", `"error":{"message":"generation failed","type":"server_error"}`), "generation failed"},
		{"untyped upstream error", `event: error` + "\ndata: {\"error\":{\"message\":\"generation failed\"}}\n\n", "generation failed"},
		{"failed event", imageEvent("image_generation.failed", `"message":"generation failed"`), "generation failed"},
		{"error in completed event", imageEvent("image_generation.completed", `"b64_json":"image","error":{"message":"generation failed"}`), "generation failed"},
		{"partial then EOF", partial, "before a completed image"},
		{"partial then DONE", partial + "data: [DONE]\n\n", "before a completed image"},
		{"empty stream", "", "before a completed image"},
		{"malformed event", "event: image_generation.completed\ndata: not-json\n\n", "invalid image stream event"},
		{"missing image", imageEvent("image_generation.completed", `"usage":{"total_tokens":10}`), "without an image"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c, recorder, info := newOpenAIImageStreamTest(t, relayconstant.RelayModeImagesGenerations)
			resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(tt.body))}
			usage, apiErr := OpenaiHandlerWithUsage(c, info, resp)
			require.NotNil(t, apiErr)
			require.Contains(t, apiErr.Error(), tt.message)
			require.Nil(t, usage)
			require.Contains(t, recorder.Body.String(), tt.message)
		})
	}
}

func TestOpenAIImageStreamReadFailureDoesNotReturnUsage(t *testing.T) {
	c, recorder, info := newOpenAIImageStreamTest(t, relayconstant.RelayModeImagesGenerations)
	partial := imageEvent("image_generation.partial_image", `"b64_json":"preview"`)
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(io.MultiReader(strings.NewReader(partial), iotest.ErrReader(errors.New("upstream disconnected"))))}
	usage, apiErr := OpenaiHandlerWithUsage(c, info, resp)
	require.Nil(t, usage)
	require.NotNil(t, apiErr)
	require.Contains(t, apiErr.Error(), "upstream disconnected")
	require.Contains(t, recorder.Body.String(), "event: error")
}

type imageFlushNotifier struct {
	gin.ResponseWriter
	flushed chan struct{}
}

func (w *imageFlushNotifier) Flush() {
	w.ResponseWriter.Flush()
	select {
	case w.flushed <- struct{}{}:
	default:
	}
}

func TestOpenAIImageStreamFlushesBeforeCompletion(t *testing.T) {
	c, recorder, info := newOpenAIImageStreamTest(t, relayconstant.RelayModeImagesGenerations)
	notifier := &imageFlushNotifier{ResponseWriter: c.Writer, flushed: make(chan struct{}, 1)}
	c.Writer = notifier
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: reader}
	done := make(chan error, 1)
	go func() {
		_, apiErr := OpenaiHandlerWithUsage(c, info, resp)
		if apiErr != nil {
			done <- apiErr
		} else {
			done <- nil
		}
	}()
	partial := imageEvent("image_generation.partial_image", `"b64_json":"preview"`)
	_, err := io.WriteString(writer, partial)
	require.NoError(t, err)
	select {
	case <-notifier.flushed:
	case <-time.After(3 * time.Second):
		t.Fatal("partial image was buffered until completion")
	}
	completed := imageEvent("image_generation.completed", `"b64_json":"final","usage":{"input_tokens":5,"output_tokens":10}`)
	_, err = io.WriteString(writer, completed)
	require.NoError(t, err)
	// A completed image must flush immediately while later images/usage can
	// still follow. The stream ends only on EOF or [DONE].
	select {
	case <-notifier.flushed:
	case <-time.After(3 * time.Second):
		t.Fatal("completed image was buffered until EOF")
	}
	usageEvent := "data: {\"usage\":{\"input_tokens\":7,\"output_tokens\":20}}\n\n"
	_, err = io.WriteString(writer, usageEvent+"data: [DONE]\n\n")
	require.NoError(t, err)
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(3 * time.Second):
		t.Fatal("handler kept waiting after [DONE]")
	}
	require.Equal(t, partial+completed+usageEvent+"data: [DONE]\n\n", recorder.Body.String())
}

func TestOpenAIImageStreamLargeImageWithEmptyError(t *testing.T) {
	c, recorder, info := newOpenAIImageStreamTest(t, relayconstant.RelayModeImagesGenerations)
	completed := imageEvent("image_generation.completed", `"b64_json":"`+strings.Repeat("a", 256<<10)+`","error":"","usage":{"input_tokens":5,"output_tokens":10}`)
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(completed))}
	usage, apiErr := OpenaiHandlerWithUsage(c, info, resp)
	require.Nil(t, apiErr)
	require.Equal(t, 15, usage.TotalTokens)
	require.Equal(t, completed, recorder.Body.String())
}

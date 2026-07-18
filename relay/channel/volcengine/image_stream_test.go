package volcengine

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestHandleImageStreamResponseForwardsSeedreamEventsAndUsage(t *testing.T) {
	originalTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	defer func() { constant.StreamingTimeout = originalTimeout }()
	c, recorder := newSeedreamStreamContext()
	info := &relaycommon.RelayInfo{
		RelayMode:    relayconstant.RelayModeImagesGenerations,
		IsStream:     true,
		StreamStatus: relaycommon.NewStreamStatus(),
	}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader("data: {\"type\":\"image_generation.partial_succeeded\",\"image_index\":0,\"url\":\"https://example.com/a.png\"}\n\ndata: {\"type\":\"image_generation.completed\",\"usage\":{\"output_tokens\":123,\"total_tokens\":123}}\n\ndata: [DONE]\n")),
	}

	usage, apiErr := handleImageStreamResponse(c, resp, info)
	require.Nil(t, apiErr)
	require.Equal(t, 123, usage.OutputTokens)
	require.Equal(t, 123, usage.CompletionTokens)
	require.Contains(t, recorder.Body.String(), "image_generation.partial_succeeded")
	require.Contains(t, recorder.Body.String(), "image_generation.completed")
}

func newSeedreamStreamContext() (*gin.Context, *httptest.ResponseRecorder) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	return c, recorder
}

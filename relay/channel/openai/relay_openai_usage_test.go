package openai

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

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

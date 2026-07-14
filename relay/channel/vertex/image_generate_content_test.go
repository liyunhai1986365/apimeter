package vertex

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestDoResponseUsesGeminiGenerateContentImageHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	info := &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeImagesGenerations,
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "custom-provider-photo-v9",
		},
	}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body: io.NopCloser(strings.NewReader(`{
			"candidates":[{"content":{"parts":[{"inlineData":{"mimeType":"image/png","data":"dmVydGV4"}}]}}],
			"usageMetadata":{"promptTokenCount":7,"candidatesTokenCount":9,"totalTokenCount":16}
		}`)),
	}

	usage, err := (&Adaptor{RequestMode: RequestModeGemini}).DoResponse(c, resp, info)
	require.Nil(t, err)
	require.Equal(t, 16, usage.(*dto.Usage).TotalTokens)

	var got dto.ImageResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &got))
	require.Equal(t, "dmVydGV4", got.Data[0].B64Json)
}

func TestConvertImageRequestUsesGeminiGenerateContentConversion(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	info := &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeImagesGenerations,
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "custom-provider-photo-v9",
		},
	}
	n := uint(2)

	converted, err := (&Adaptor{RequestMode: RequestModeGemini}).ConvertImageRequest(c, info, dto.ImageRequest{
		Model:  "custom-provider-photo-v9",
		Prompt: "draw a skyline",
		N:      &n,
	})
	require.NoError(t, err)
	got, ok := converted.(*dto.GeminiChatRequest)
	require.True(t, ok)
	require.Equal(t, "draw a skyline", got.Contents[0].Parts[0].Text)
	require.Equal(t, []string{"TEXT", "IMAGE"}, got.GenerationConfig.ResponseModalities)
	require.NotNil(t, got.GenerationConfig.CandidateCount)
	require.Equal(t, 2, *got.GenerationConfig.CandidateCount)
}

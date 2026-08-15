package gemini

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
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestConvertOpenAIImageRequiresPromptAsRequestError(t *testing.T) {
	_, err := ConvertOpenAIImageToGenerateContent(nil, &relaycommon.RelayInfo{}, dto.ImageRequest{})
	require.Error(t, err)
	require.True(t, types.IsRequestError(err))
}

func TestConvertImageRequestToGeminiGenerateContent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	info := &relaycommon.RelayInfo{
		RelayMode:       relayconstant.RelayModeImagesGenerations,
		OriginModelName: "gemini-3.1-flash-image-preview",
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gemini-3.1-flash-image-preview",
		},
	}
	n := uint(2)
	request := dto.ImageRequest{
		Model:  "gemini-3.1-flash-image-preview",
		Prompt: "draw a cat",
		N:      &n,
		Size:   "2048x2048",
	}

	converted, err := (&Adaptor{}).ConvertImageRequest(c, info, request)
	require.NoError(t, err)
	got, ok := converted.(*dto.GeminiChatRequest)
	require.True(t, ok)
	require.Equal(t, "draw a cat", got.Contents[0].Parts[0].Text)
	require.Equal(t, []string{"TEXT", "IMAGE"}, got.GenerationConfig.ResponseModalities)
	require.NotNil(t, got.GenerationConfig.CandidateCount)
	require.Equal(t, 2, *got.GenerationConfig.CandidateCount)
	require.JSONEq(t, `{"aspectRatio":"1:1","imageSize":"2K"}`, string(got.GenerationConfig.ImageConfig))
}

func TestConvertImageRequestToGeminiGenerateContentWithReference(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	info := &relaycommon.RelayInfo{
		RelayMode:       relayconstant.RelayModeImagesGenerations,
		OriginModelName: "gemini-future-image-model",
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gemini-future-image-model",
		},
	}
	references, err := common.Marshal([]string{"data:image/png;base64,aW5wdXQ="})
	require.NoError(t, err)

	converted, err := (&Adaptor{}).ConvertImageRequest(c, info, dto.ImageRequest{
		Model:  "gemini-future-image-model",
		Prompt: "edit this",
		Images: references,
	})
	require.NoError(t, err)
	got, ok := converted.(*dto.GeminiChatRequest)
	require.True(t, ok)
	require.Len(t, got.Contents, 1)
	require.Len(t, got.Contents[0].Parts, 2)
	require.Equal(t, "edit this", got.Contents[0].Parts[0].Text)
	require.NotNil(t, got.Contents[0].Parts[1].InlineData)
	require.Equal(t, "image/png", got.Contents[0].Parts[1].InlineData.MimeType)
	require.Equal(t, "aW5wdXQ=", got.Contents[0].Parts[1].InlineData.Data)
}

func TestConvertImageRequestConvertsArbitraryConfiguredImageModel(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	info := &relaycommon.RelayInfo{
		RelayMode:       relayconstant.RelayModeImagesGenerations,
		OriginModelName: "custom-provider-photo-v9",
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "custom-provider-photo-v9",
		},
	}

	converted, err := (&Adaptor{}).ConvertImageRequest(c, info, dto.ImageRequest{
		Model:  "custom-provider-photo-v9",
		Prompt: "draw",
	})
	require.NoError(t, err)
	require.IsType(t, &dto.GeminiChatRequest{}, converted)
}

func TestGeminiGenerateContentImageHandlerConvertsInlineData(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	info := &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeImagesGenerations,
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "custom-provider-photo-v9",
		},
	}
	resp := geminiGenerateContentResponse(`{
		"candidates":[
			{"content":{"parts":[
				{"text":"generated image"},
				{"inlineData":{"mimeType":"image/png","data":"aW1hZ2Ux"}}
			]}},
			{"content":{"parts":[
				{"inlineData":{"mimeType":"image/jpeg","data":"aW1hZ2Uy"}}
			]}}
		],
		"usageMetadata":{
			"promptTokenCount":12,
			"candidatesTokenCount":34,
			"totalTokenCount":46,
			"cachedContentTokenCount":2
		}
	}`)

	usage, err := GeminiGenerateContentImageHandler(c, info, resp)
	require.Nil(t, err)
	require.Equal(t, 12, usage.PromptTokens)
	require.Equal(t, 34, usage.CompletionTokens)
	require.Equal(t, 46, usage.TotalTokens)
	require.Equal(t, 2, usage.PromptTokensDetails.CachedTokens)

	var got dto.ImageResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &got))
	require.Len(t, got.Data, 2)
	require.Equal(t, "aW1hZ2Ux", got.Data[0].B64Json)
	require.Equal(t, "aW1hZ2Uy", got.Data[1].B64Json)
}

func TestGeminiGenerateContentImageHandlerRejectsTextOnlyResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	info := &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeImagesGenerations,
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gemini-future-image-model",
		},
	}
	resp := geminiGenerateContentResponse(`{
		"candidates":[{"content":{"parts":[{"text":"I cannot generate that image"}]}}],
		"usageMetadata":{"promptTokenCount":4,"candidatesTokenCount":6,"totalTokenCount":10}
	}`)

	usage, err := GeminiGenerateContentImageHandler(c, info, resp)
	require.Nil(t, usage)
	require.NotNil(t, err)
	require.Equal(t, "bad_response_body", string(err.GetErrorCode()))
	require.Empty(t, recorder.Body.String())
}

func TestGeminiAdaptorDoResponseUsesGenerateContentImageHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	info := &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeImagesGenerations,
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "custom-provider-photo-v9",
		},
	}
	resp := geminiGenerateContentResponse(`{
		"candidates":[{"content":{"parts":[{"inlineData":{"mimeType":"image/png","data":"aW1hZ2U="}}]}}],
		"usageMetadata":{"promptTokenCount":3,"candidatesTokenCount":5,"totalTokenCount":8}
	}`)

	usage, err := (&Adaptor{}).DoResponse(c, resp, info)
	require.Nil(t, err)
	require.Equal(t, 8, usage.(*dto.Usage).TotalTokens)

	var got dto.ImageResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &got))
	require.Equal(t, "aW1hZ2U=", got.Data[0].B64Json)
}

func geminiGenerateContentResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

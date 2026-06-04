package openai

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func rawJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	data, err := common.Marshal(value)
	require.NoError(t, err)
	return data
}

func TestConvertImageRequestUsesDuomiGeminiTextToImagePayload(t *testing.T) {
	adaptor := &Adaptor{}
	info := &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeImagesGenerations,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelBaseUrl: "https://duomiapi.com",
		},
		RequestURLPath: "/v1/images/generations",
	}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)

	payload, err := adaptor.ConvertImageRequest(c, info, dto.ImageRequest{
		Model:  "gemini-3-pro-image-preview",
		Prompt: "中文海报",
		Size:   "1K",
	})
	require.NoError(t, err)

	got, ok := payload.(map[string]any)
	require.True(t, ok)
	require.Equal(t, "gemini-3-pro-image-preview", got["model"])
	require.Equal(t, "中文海报", got["prompt"])
	require.Equal(t, "1K", got["image_size"])
	_, exists := got["image_urls"]
	require.False(t, exists)
}

func TestConvertImageRequestUsesDuomiGeminiImageEditPayload(t *testing.T) {
	adaptor := &Adaptor{}
	info := &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeImagesGenerations,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelBaseUrl: "https://duomiapi.com",
		},
		RequestURLPath: "/v1/images/generations",
	}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)

	payload, err := adaptor.ConvertImageRequest(c, info, dto.ImageRequest{
		Model:  "gemini-2.5-flash-image",
		Prompt: "改成红色字",
		Image:  []byte(`"https://cdn.example.com/ref.jpg"`),
	})
	require.NoError(t, err)

	got, ok := payload.(map[string]any)
	require.True(t, ok)
	require.Equal(t, "gemini-2.5-flash-image", got["model"])
	require.Equal(t, "改成红色字", got["prompt"])
	require.Equal(t, []string{"https://cdn.example.com/ref.jpg"}, got["image_urls"])
}

func TestDuomiGeminiImageRequestUsesNanoBananaEndpointAndRawAuthorization(t *testing.T) {
	info := &relaycommon.RelayInfo{
		RelayMode:       relayconstant.RelayModeImagesGenerations,
		RequestURLPath:  "/v1/images/generations",
		OriginModelName: "gemini-3-pro-image-preview",
		Request: &dto.ImageRequest{
			Model:  "gemini-3-pro-image-preview",
			Prompt: "中文海报",
		},
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:    constant.ChannelTypeOpenAI,
			ApiType:        constant.APITypeOpenAI,
			ChannelBaseUrl: "https://duomiapi.com",
			ApiKey:         "duomi-key",
		},
	}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	c.Request.Header.Set("Content-Type", "application/json")

	adaptor := &Adaptor{}
	gotURL, err := adaptor.GetRequestURL(info)
	require.NoError(t, err)
	require.Equal(t, "https://duomiapi.com/api/gemini/nano-banana", gotURL)

	header := http.Header{}
	err = adaptor.SetupRequestHeader(c, &header, info)
	require.NoError(t, err)
	require.Equal(t, "duomi-key", header.Get("Authorization"))
}

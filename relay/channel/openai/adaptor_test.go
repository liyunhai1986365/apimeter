package openai

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOpenAIImageRequestKeepsStandardEndpointAndBearerAuthorization(t *testing.T) {
	info := &relaycommon.RelayInfo{
		RelayMode:       relayconstant.RelayModeImagesGenerations,
		RequestURLPath:  "/v1/images/generations",
		OriginModelName: "gemini-3-pro-image-preview",
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
	require.Equal(t, "https://duomiapi.com/v1/images/generations", gotURL)

	header := http.Header{}
	err = adaptor.SetupRequestHeader(c, &header, info)
	require.NoError(t, err)
	require.Equal(t, "Bearer duomi-key", header.Get("Authorization"))
}

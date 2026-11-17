package service

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGoogleMediaResolutionEstimateUsesRequestMetadata(t *testing.T) {
	oldCount, oldMedia := constant.CountToken, constant.GetMediaToken
	constant.CountToken = true
	constant.GetMediaToken = false
	t.Cleanup(func() { constant.CountToken = oldCount; constant.GetMediaToken = oldMedia })
	for _, tc := range []struct {
		model, level string
		want         int
	}{{"gemini-3-flash-preview", "MEDIA_RESOLUTION_LOW", 280}, {"gemini-3-pro-preview", "MEDIA_RESOLUTION_MEDIUM", 560}, {"gemini-4-argon", "MEDIA_RESOLUTION_HIGH", 1120}, {"gemini-4-argon", "MEDIA_RESOLUTION_ULTRA_HIGH", 2240}, {"gemini-2.5-flash", "", 1290}} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/v1beta/models/model:generateContent", nil)
		common.SetContextKey(c, constant.ContextKeyOriginalModel, tc.model)
		var req dto.GeminiChatRequest
		require.NoError(t, common.UnmarshalJsonStr(`{"generationConfig":{"mediaResolution":"`+tc.level+`"},"contents":[{"parts":[{"inlineData":{"mimeType":"image/png","data":"AA=="}}]}]}`, &req))
		got, err := EstimateRequestToken(c, req.GetTokenCountMeta(), &relaycommon.RelayInfo{RelayFormat: types.RelayFormatGemini})
		require.NoError(t, err)
		require.Equal(t, tc.want, got)
	}
}
func TestGoogleLossPolicyReturnsClientError(t *testing.T) {
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gemini-3-flash-preview", ChannelOtherSettings: dto.ChannelOtherSettings{ToolLossPolicy: "safe"}}}
	req := &dto.OpenAIResponsesRequest{Model: "gemini-3-flash-preview", Tools: []byte(`[{"type":"file_search"}]`)}
	_, err := ConvertRequest(nil, info, types.RelayFormatGemini, req)
	var apiErr *types.NewAPIError
	require.ErrorAs(t, err, &apiErr)
	require.Equal(t, 400, apiErr.StatusCode)
}

func TestGoogleTextEstimateKeepsExistingModelEstimator(t *testing.T) {
	for _, model := range []string{"gemini-2.5-flash", "gemini-3-pro-preview", "gemini-4-argon"} {
		const input = "请解释这段代码中的缓存逻辑。 Explain the cache behavior."
		require.Equal(t, EstimateTokenByModel(model, input), CountTextToken(input, model), model)
	}
}

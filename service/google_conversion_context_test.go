package service

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGoogleConversionKeepsFileCleanupContext(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	t.Cleanup(func() { CleanupFileSources(c) })

	var request dto.GeneralOpenAIRequest
	require.NoError(t, common.UnmarshalJsonStr(`{"model":"gemini-2.5-flash","messages":[{"role":"user","content":[{"type":"image_url","image_url":{"url":"data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aKcoAAAAASUVORK5CYII="}}]}]}`, &request))

	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: request.Model}}
	result, err := ConvertRequest(c, info, types.RelayFormatGemini, &request)
	require.NoError(t, err)
	out := result.Value.(*dto.GeminiChatRequest)
	require.Len(t, out.Contents, 1)
	require.Len(t, out.Contents[0].Parts, 1)
	require.NotNil(t, out.Contents[0].Parts[0].InlineData)
	sources, ok := c.Get(string(constant.ContextKeyFileSourcesToCleanup))
	require.True(t, ok, "conversion must retain the host request context for file cleanup")
	require.NotEmpty(t, sources)
}

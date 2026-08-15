package zhipu_4v

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestZhipuImageHandlerPreservesURLByDefault(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body: io.NopCloser(strings.NewReader(
			`{"created":123,"data":[{"url":"http://127.0.0.1:1/must-not-download.png"}]}`,
		)),
		Header: make(http.Header),
	}
	info := &relaycommon.RelayInfo{StartTime: time.Now()}

	usage, apiErr := zhipu4vImageHandler(c, resp, info)
	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	require.Equal(t, "http://127.0.0.1:1/must-not-download.png", gjson.GetBytes(recorder.Body.Bytes(), "data.0.url").String())
	require.False(t, gjson.GetBytes(recorder.Body.Bytes(), "data.0.b64_json").Exists())
}

func TestZhipuImageResponseFormatUsesTokenThenRequestThenChannel(t *testing.T) {
	request := &dto.ImageRequest{ResponseFormat: dto.TokenImageFormatURL}
	info := &relaycommon.RelayInfo{
		Request:            request,
		TokenImageSettings: dto.TokenImageSettings{Format: dto.TokenImageFormatB64JSON},
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelSetting: dto.ChannelSettings{OpenAIImageResponseFormat: dto.TokenImageFormatURL},
		},
	}
	require.True(t, zhipuWantsBase64(info, false))

	info.TokenImageSettings = dto.TokenImageSettings{}
	require.False(t, zhipuWantsBase64(info, true))

	request.ResponseFormat = ""
	info.ChannelSetting.OpenAIImageResponseFormat = dto.TokenImageFormatB64JSON
	require.True(t, zhipuWantsBase64(info, false))
}

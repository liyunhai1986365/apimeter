package replicate

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestReplicateTokenURLOverridesClientBase64WithoutDownload(t *testing.T) {
	request := &dto.ImageRequest{ResponseFormat: dto.TokenImageFormatB64JSON}
	info := &relaycommon.RelayInfo{
		Request:            request,
		TokenImageSettings: dto.TokenImageSettings{Format: dto.TokenImageFormatURL},
	}
	require.False(t, replicateWantsBase64(info, request))

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	response := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(`{"status":"succeeded","output":"http://127.0.0.1:1/unreachable.png"}`)),
		Header:     make(http.Header),
	}
	usage, apiErr := (&Adaptor{}).DoResponse(c, response, info)
	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	require.Equal(t, "http://127.0.0.1:1/unreachable.png", gjson.GetBytes(recorder.Body.Bytes(), "data.0.url").String())
}

func TestReplicateTokenBase64OverridesClientURL(t *testing.T) {
	request := &dto.ImageRequest{ResponseFormat: dto.TokenImageFormatURL}
	info := &relaycommon.RelayInfo{
		TokenImageSettings: dto.TokenImageSettings{Format: dto.TokenImageFormatB64JSON},
	}

	require.True(t, replicateWantsBase64(info, request))
}

func TestReplicateInvalidExtraIsRequestErrorBeforeEditUpload(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", nil)
	info := &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeImagesEdits,
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "black-forest-labs/flux-1.1-pro",
		},
	}
	request := dto.ImageRequest{
		Prompt:      "edit this image",
		ExtraFields: []byte(`"not-an-object"`),
	}

	_, err := (&Adaptor{}).ConvertImageRequest(c, info, request)
	require.Error(t, err)
	require.True(t, types.IsRequestError(err))
}

package service

import (
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestEstimateRequestTokenGeminiRemoteImages(t *testing.T) {
	previousCount, previousMedia, previousNotStream := constant.CountToken, constant.GetMediaToken, constant.GetMediaTokenNotStream
	previousClient, previousProtectedClient := httpClient, ssrfProtectedHTTPClient
	previousWorker, previousMaxSize := system_setting.WorkerUrl, constant.MaxFileDownloadMB
	fetch := system_setting.GetFetchSetting()
	previousFetch := *fetch
	t.Cleanup(func() {
		constant.CountToken, constant.GetMediaToken, constant.GetMediaTokenNotStream = previousCount, previousMedia, previousNotStream
		httpClient, ssrfProtectedHTTPClient = previousClient, previousProtectedClient
		system_setting.WorkerUrl, constant.MaxFileDownloadMB = previousWorker, previousMaxSize
		*fetch = previousFetch
	})
	constant.CountToken = true
	constant.MaxFileDownloadMB = 1
	system_setting.WorkerUrl = ""
	fetch.EnableSSRFProtection = false
	png, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=")
	require.NoError(t, err)

	for _, tt := range []struct {
		name, model                string
		media, stream, nonStream   bool
		status                     int
		downloadErr                error
		undecodable                bool
		wantImageTokens, wantFetch int
	}{
		{name: "upstream-only image URL", media: true, stream: true, status: http.StatusForbidden, wantImageTokens: 1290, wantFetch: 1},
		{name: "network failure", media: true, stream: true, downloadErr: errors.New("network unreachable"), wantImageTokens: 1290, wantFetch: 1},
		{name: "unrecognized image format", media: true, stream: true, status: http.StatusOK, undecodable: true, wantImageTokens: 1290, wantFetch: 1},
		{name: "available dimensions", media: true, stream: true, status: http.StatusOK, wantImageTokens: 258, wantFetch: 1},
		{name: "media counting disabled", stream: true, wantImageTokens: 1290},
		{name: "nonstream counting disabled", media: true, wantImageTokens: 1290},
		{name: "nonstream download failure", media: true, nonStream: true, status: http.StatusForbidden, wantImageTokens: 1290, wantFetch: 1},
		{name: "nonstream available dimensions", media: true, nonStream: true, status: http.StatusOK, wantImageTokens: 258, wantFetch: 1},
		{name: "Gemini 3 resolution estimate", model: "gemini-3-flash-preview", media: true, stream: true, wantImageTokens: 1120},
	} {
		t.Run(tt.name, func(t *testing.T) {
			constant.GetMediaToken, constant.GetMediaTokenNotStream = tt.media, tt.nonStream
			fetches := 0
			httpClient = &http.Client{Transport: tokenImageTransport(func(r *http.Request) (*http.Response, error) {
				fetches++
				require.Equal(t, "https://images.example.test/input.png", r.URL.String())
				if tt.downloadErr != nil {
					return nil, tt.downloadErr
				}
				body := string(png)
				if tt.undecodable {
					body = "image data without a supported local decoder"
				}
				return &http.Response{StatusCode: tt.status, Header: http.Header{"Content-Type": {"image/png"}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
			})}
			ssrfProtectedHTTPClient = httpClient
			var request dto.GeneralOpenAIRequest
			require.NoError(t, common.UnmarshalJsonStr(`{"messages":[{"role":"user","content":[{"type":"image_url","image_url":{"url":"https://images.example.test/input.png"}}]}]}`, &request))
			request.Model = tt.model
			if request.Model == "" {
				request.Model = "gemini-2.5-flash"
			}
			request.Stream = &tt.stream
			before, err := common.Marshal(request)
			require.NoError(t, err)
			meta := request.GetTokenCountMeta()
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			t.Cleanup(func() { CleanupFileSources(c) })
			common.SetContextKey(c, constant.ContextKeyOriginalModel, request.Model)
			tokens, err := EstimateRequestToken(c, meta, &relaycommon.RelayInfo{IsStream: tt.stream, RelayFormat: types.RelayFormatOpenAI})
			require.NoError(t, err)
			require.Equal(t, tt.wantFetch, fetches)
			// The single user message adds six framing tokens to its text and image.
			require.Equal(t, CountTextToken(meta.CombineText, request.Model)+6+tt.wantImageTokens, tokens)
			require.Equal(t, tokens, common.GetContextKeyInt(c, constant.ContextKeyPromptTokens))
			after, err := common.Marshal(request)
			require.NoError(t, err)
			require.Equal(t, string(before), string(after), "estimation must preserve the image URL sent upstream")
		})
	}
}

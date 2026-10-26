package service

import (
	"encoding/base64"
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

type tokenImageTransport func(*http.Request) (*http.Response, error)

func (f tokenImageTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestEstimateRequestTokenImageDownloads(t *testing.T) {
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
	constant.CountToken, constant.GetMediaToken, constant.GetMediaTokenNotStream = true, true, true
	constant.MaxFileDownloadMB = 1
	system_setting.WorkerUrl = ""
	fetch.EnableSSRFProtection = false
	png, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=")
	require.NoError(t, err)
	for _, tt := range []struct {
		name, model     string
		status, fetches int
		wantError       bool
	}{
		{"Claude fixed image estimate", "claude-opus-test", http.StatusForbidden, 0, false},
		{"Opus alias fixed image estimate", "opus5-5", http.StatusForbidden, 0, false},
		{"GPT download error preserved", "gpt-4o", http.StatusForbidden, 1, true},
		{"GPT image dimensions still loaded", "gpt-4o", http.StatusOK, 1, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			fetches := 0
			httpClient = &http.Client{Transport: tokenImageTransport(func(r *http.Request) (*http.Response, error) {
				fetches++
				require.Equal(t, "https://images.example.test/tool.png", r.URL.String())
				return &http.Response{StatusCode: tt.status, Header: http.Header{"Content-Type": {"image/png"}}, Body: io.NopCloser(strings.NewReader(string(png))), Request: r}, nil
			})}
			ssrfProtectedHTTPClient = httpClient
			var request dto.ClaudeRequest
			require.NoError(t, common.Unmarshal([]byte(`{"messages":[{"role":"assistant","content":[{"type":"tool_use","id":"call","name":"view","input":{}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"call","content":[{"type":"image","source":{"type":"url","url":"https://images.example.test/tool.png"}}]}]}]}`), &request))
			request.Model = tt.model
			meta := request.GetTokenCountMeta()
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
			t.Cleanup(func() { CleanupFileSources(c) })
			common.SetContextKey(c, constant.ContextKeyOriginalModel, tt.model)
			tokens, err := EstimateRequestToken(c, meta, &relaycommon.RelayInfo{IsStream: true, RelayFormat: types.RelayFormatClaude})
			require.Equal(t, tt.fetches, fetches)
			if tt.wantError {
				require.ErrorContains(t, err, "status code: 403")
				return
			}
			require.NoError(t, err)
			if tt.model == "gpt-4o" {
				require.NotNil(t, meta.Files[0].Source.GetCache())
				require.Greater(t, tokens, CountTextToken(meta.CombineText, tt.model))
			} else {
				require.Equal(t, CountTextToken(meta.CombineText, tt.model)+520, tokens)
			}
		})
	}
}

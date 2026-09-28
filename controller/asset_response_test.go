package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestAssetResponsesHideInternalFields(t *testing.T) {
	for _, mode := range []string{"create", "detail", "list", "upstream-error", "legacy-error"} {
		t.Run(mode, func(t *testing.T) {
			payload := `{"Result":{"Id":"owned","Status":"Expired","ExpireTime":1234567890,"channel_id":72,"channel_name":"private-supplier","asset_credentials":{"api_key":"secret"}},"admin_info":{"trace":"private"},"ResponseMetadata":{"RequestId":"public-request"}}`
			if mode == "list" {
				payload = `{"Result":{"Items":[{"Id":"owned","Status":"Expired","ExpireTime":1234567890,"ChannelId":72,"ChannelName":"private-supplier"}],"TotalCount":1},"channel_type":999,"ResponseMetadata":{"RequestId":"public-request"}}`
			}
			status := http.StatusOK
			if mode == "upstream-error" || mode == "legacy-error" {
				status = http.StatusNotFound
				payload = `{"error":{"code":"AssetExpired","message":"asset expired","channelId":72,"channelName":"private-supplier","api_key":"secret"},"request_id":"public-request"}`
			}
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("X-Request-Id", "public-request")
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(status)
				_, _ = w.Write([]byte(payload))
			}))
			defer upstream.Close()
			r := assetSecurityRouter(t, upstream.URL, "youniyouju", "")
			if mode != "legacy-error" && mode != "create" {
				seedAssetOwnershipForTest(t, 20, 1, "asset", "owned")
			}
			method, path, body := "GET", "/api/assets/owned", ""
			if mode == "list" {
				path = "/api/assets"
			} else if mode == "create" {
				seedAssetOwnershipForTest(t, 20, 1, "group", "own-group")
				method, path, body = "POST", "/api/assets", `{"GroupId":"own-group","URL":"https://example.com/image.png"}`
			}
			got := seedanceCall(r, method, path, body, 1)
			require.Equal(t, status, got.Code, got.Body.String())
			for _, hidden := range []string{"channel", "Channel", "private-supplier", "secret", "admin_info", "asset_credentials"} {
				require.NotContains(t, got.Body.String(), hidden)
			}
			require.Contains(t, got.Body.String(), "public-request")
			require.Equal(t, "public-request", got.Header().Get("X-Request-Id"))
			if status == http.StatusOK {
				require.Contains(t, got.Body.String(), `"Id":"owned"`)
				require.Contains(t, got.Body.String(), `"Status":"Expired"`)
				require.Contains(t, got.Body.String(), `"ExpireTime":1234567890`)
			} else {
				require.Equal(t, "AssetExpired", gjson.Get(got.Body.String(), "error.code").String())
			}
		})
	}
}

func TestAssetLookupNetworkErrorHidesSupplierAddress(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	upstream.Close()
	r := assetSecurityRouter(t, upstream.URL, "youniyouju", "")
	got := seedanceCall(r, "GET", "/api/assets/legacy", "", 1)
	require.Equal(t, http.StatusBadGateway, got.Code, got.Body.String())
	require.NotContains(t, got.Body.String(), upstream.URL)
	require.NotContains(t, got.Body.String(), "dial tcp")
	require.Equal(t, "asset_lookup_failed", gjson.Get(got.Body.String(), "error.code").String())
}

func TestPublicAssetResponsePreservesLargeNumbers(t *testing.T) {
	body, err := publicAssetResponse([]byte(`{"Result":{"Id":"asset-1","version":9007199254740993,"channel_id":1}}`))
	require.NoError(t, err)
	require.JSONEq(t, `{"Result":{"Id":"asset-1","version":9007199254740993}}`, string(body))
	var result map[string]any
	require.NoError(t, common.Unmarshal(body, &result))
	require.NotContains(t, result["Result"], "channel_id")
}

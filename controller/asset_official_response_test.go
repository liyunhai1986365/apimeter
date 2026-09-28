package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestOfficialAssetResponseContracts(t *testing.T) {
	for _, tc := range []struct {
		name, action, body, result, code, message string
		status, wantStatus                        int
	}{
		{
			name: "official-fields-and-precision", action: "GetAsset", status: 200,
			body:   `{"ResponseMetadata":{"RequestId":"upstream-request","Region":"cn-shanghai"},"Result":{"Id":"asset-1","Status":"Expired","ExpireTime":0,"extension":{"n":9007199254740993,"enabled":false},"channel_id":42}}`,
			result: `{"Id":"asset-1","Status":"Expired","ExpireTime":0,"extension":{"n":9007199254740993,"enabled":false}}`,
		},
		{
			name: "third-party-create", action: "CreateAsset", status: 201,
			body:   `{"code":0,"message":"ok","data":{"asset_id":"supplier-id","name":"reference","asset_type":"Image","status":"processing","group_id":"group-1","url":"https://example.com/a.png"}}`,
			result: `{"Id":"supplier-id","Name":"reference","AssetType":"Image","Status":"Processing","GroupId":"group-1","URL":"https://example.com/a.png"}`,
		},
		{
			name: "third-party-list", action: "ListAssets", status: 200,
			body:   `{"data":{"items":[{"id":"asset-1","status":"active","channel_name":"private"}],"total":1,"page":1,"page_size":20,"next_token":"asset-v1-cursor"}}`,
			result: `{"Items":[{"Id":"asset-1","Status":"Active"}],"TotalCount":1,"PageNumber":1,"PageSize":20,"NextToken":"asset-v1-cursor"}`,
		},
		{
			name: "array-list-and-outer-pagination", action: "ListAssetGroups", status: 200,
			body:   `{"data":[{"group_id":"group-1","name":"reference"}],"total":1,"page_size":20}`,
			result: `{"Items":[{"Id":"group-1","Name":"reference"}],"TotalCount":1,"PageSize":20}`,
		},
		{name: "empty-list", action: "ListAssets", status: 200, body: `{"Result":{"Items":null,"TotalCount":0}}`, result: `{"Items":[],"TotalCount":0}`},
		{name: "bare-list", action: "ListAssets", status: 200, body: `[{"id":"asset-1"}]`, result: `{"Items":[{"Id":"asset-1"}]}`},
		{
			name: "asset-audit-failure-is-a-successful-query", action: "GetAsset", status: 200,
			body:   `{"Result":{"Id":"asset-1","Status":"Failed","Error":{"Code":"InputImageSensitiveContentDetected","Message":"audit rejected"}}}`,
			result: `{"Id":"asset-1","Status":"Failed","Error":{"Code":"InputImageSensitiveContentDetected","Message":"audit rejected"}}`,
		},
		{
			name: "liveness-session", action: "CreateVisualValidateSession", status: 200,
			body:   `{"BytedToken":"session","H5Link":"https://example.com/auth","CallbackURL":"https://client.example/callback"}`,
			result: `{"BytedToken":"session","H5Link":"https://example.com/auth","CallbackURL":"https://client.example/callback"}`,
		},
		{name: "liveness-result", action: "GetVisualValidateResult", status: 200, body: `{"group_id":"human-group"}`, result: `{"GroupId":"human-group"}`},
		{name: "rest-delete", action: "DeleteAsset", status: 204, wantStatus: 200, result: `{}`},
		{name: "supplier-delete", action: "DeleteAssetGroup", status: 200, body: `{"Result":{"Id":"deleted"}}`, result: `{}`},
		{name: "official-error", action: "GetAsset", status: 404, body: `{"ResponseMetadata":{"RequestId":"upstream-request","Error":{"Code":"AssetNotFound","Message":"missing","CodeN":123}}}`, code: "AssetNotFound", message: "missing"},
		{name: "rest-error", action: "CreateAsset", status: 429, body: `{"error":{"code":"RateLimit","message":"try later","channel_id":42}}`, code: "RateLimit", message: "try later"},
		{name: "business-error-with-http-200", action: "CreateAsset", status: 200, body: `{"code":1001,"message":"invalid URL","data":null}`, code: "1001", message: "invalid URL"},
		{name: "auth-error", action: "GetAsset", status: 401, body: `{"success":false,"message":"invalid token"}`, code: "asset_request_failed", message: "invalid token"},
		{name: "gateway-string-error", action: "GetAsset", status: 400, body: `{"error":"invalid resource ID"}`, code: "asset_request_failed", message: "invalid resource ID"},
		{name: "html-error", action: "GetAsset", status: 502, body: `<html>private proxy diagnostics</html>`, code: "asset_request_failed", message: "Bad Gateway"},
		{name: "malformed-success", action: "GetAsset", status: 200, wantStatus: 502, body: `<html>proxy failure</html>`, code: "asset_response_failed"},
		{name: "null-success", action: "GetAsset", status: 200, wantStatus: 502, body: `null`, code: "asset_response_failed"},
		{name: "invalid-result-keeps-request-id", action: "CreateAsset", status: 200, wantStatus: 502, body: `{"ResponseMetadata":{"RequestId":"upstream-request"},"Result":null}`, code: "asset_response_failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := gin.New()
			r.POST("/", ArkAssetResponse, func(c *gin.Context) {
				c.Set(common.RequestIdKey, "gateway-request")
				c.Header("X-Request-Id", "upstream-header")
				c.Header("Retry-After", "5")
				c.Header("Content-Length", fmt.Sprint(len(tc.body)))
				c.Data(tc.status, "text/plain", []byte(tc.body))
			})
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest("POST", "/?Action="+tc.action+"&Version=2024-01-01", nil))
			wantStatus := tc.wantStatus
			if wantStatus == 0 {
				wantStatus = tc.status
			}
			require.Equal(t, wantStatus, w.Code, w.Body.String())
			require.Equal(t, "application/json; charset=utf-8", w.Header().Get("Content-Type"))
			require.Empty(t, w.Header().Get("Content-Length"))
			require.Equal(t, "5", w.Header().Get("Retry-After"))
			require.Equal(t, "upstream-header", w.Header().Get("X-Request-Id"))
			body := gjson.ParseBytes(w.Body.Bytes())
			require.Equal(t, tc.action, body.Get("ResponseMetadata.Action").String())
			require.Equal(t, "2024-01-01", body.Get("ResponseMetadata.Version").String())
			require.Equal(t, "ark", body.Get("ResponseMetadata.Service").String())
			requestID := "upstream-header"
			if strings.Contains(tc.body, "upstream-request") {
				requestID = "upstream-request"
			}
			require.Equal(t, requestID, body.Get("ResponseMetadata.RequestId").String())
			require.NotContains(t, w.Body.String(), "private")
			require.NotContains(t, w.Body.String(), "channel_")
			if tc.result != "" {
				require.Len(t, body.Map(), 2)
				require.JSONEq(t, tc.result, body.Get("Result").Raw)
				require.False(t, body.Get("ResponseMetadata.Error").Exists())
			} else {
				require.Len(t, body.Map(), 1)
				require.Equal(t, tc.code, body.Get("ResponseMetadata.Error.Code").String())
				if tc.message != "" {
					require.Equal(t, tc.message, body.Get("ResponseMetadata.Error.Message").String())
				}
			}
		})
	}
}

func TestOfficialAssetResponseLifecycle(t *testing.T) {
	for _, backend := range []string{"tgxmaas", "youniyouju", "volcengine-assets"} {
		t.Run(backend, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("X-Request-Id", "supplier-request")
				switch {
				case r.URL.Query().Get("Action") == "ListAssets" || strings.HasSuffix(r.URL.Path, "/list"):
					_, _ = w.Write([]byte(`{"Result":{"Items":[{"Id":"own-1","Status":"Expired"},{"Id":"foreign"},{"Id":"own-2","Status":"Active"}],"TotalCount":3}}`))
				case r.Method == "DELETE" || r.URL.Query().Get("Action") == "DeleteAsset":
					w.WriteHeader(http.StatusNoContent)
				default:
					_, _ = w.Write([]byte(`{"Result":{"Id":"own-1","Status":"Active","channel_id":20}}`))
				}
			}))
			defer upstream.Close()
			r := assetSecurityRouter(t, upstream.URL, backend, "")
			seedAssetOwnershipForTest(t, 20, 1, "asset", "own-1")
			seedAssetOwnershipForTest(t, 20, 1, "asset", "own-2")
			seedAssetOwnershipForTest(t, 20, 2, "asset", "foreign")
			list := seedanceCall(r, "POST", "/?Action=ListAssets&Version=2024-01-01", `{"MaxResults":1}`, 1)
			require.Equal(t, 200, list.Code, list.Body.String())
			require.Equal(t, int64(2), gjson.GetBytes(list.Body.Bytes(), "Result.TotalCount").Int())
			require.Equal(t, "own-1", gjson.GetBytes(list.Body.Bytes(), "Result.Items.0.Id").String())
			require.Equal(t, "Expired", gjson.GetBytes(list.Body.Bytes(), "Result.Items.0.Status").String())
			require.Equal(t, "supplier-request", gjson.GetBytes(list.Body.Bytes(), "ResponseMetadata.RequestId").String())
			cursor := gjson.GetBytes(list.Body.Bytes(), "Result.NextToken").String()
			require.True(t, strings.HasPrefix(cursor, "asset-v1-"))
			next := seedanceCall(r, "POST", "/?Action=ListAssets&Version=2024-01-01", fmt.Sprintf(`{"MaxResults":1,"NextToken":%q}`, cursor), 1)
			require.Equal(t, 200, next.Code, next.Body.String())
			require.Equal(t, "own-2", gjson.GetBytes(next.Body.Bytes(), "Result.Items.0.Id").String())
			require.Empty(t, gjson.GetBytes(next.Body.Bytes(), "Result.NextToken").String())
			require.NotContains(t, next.Body.String(), "foreign")
			denied := seedanceCall(r, "POST", "/?Action=GetAsset&Version=2024-01-01", `{"Id":"foreign"}`, 1)
			require.Equal(t, 404, denied.Code)
			require.Equal(t, "asset_not_found", gjson.GetBytes(denied.Body.Bytes(), "ResponseMetadata.Error.Code").String())
			deleted := seedanceCall(r, "POST", "/?Action=DeleteAsset&Version=2024-01-01", `{"Id":"own-1"}`, 1)
			require.Equal(t, 200, deleted.Code, deleted.Body.String())
			require.Equal(t, `{}`, gjson.GetBytes(deleted.Body.Bytes(), "Result").Raw)
			gone := seedanceCall(r, "POST", "/?Action=GetAsset&Version=2024-01-01", `{"Id":"own-1"}`, 1)
			require.Equal(t, 404, gone.Code)
		})
	}
}

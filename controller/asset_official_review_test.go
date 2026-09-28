package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestOfficialAssetReviewFailedDeleteKeepsOwnership(t *testing.T) {
	for _, body := range []string{
		`{"Error":{"Code":"DeleteDenied","Message":"asset in use"}}`,
		`{"Code":"DeleteDenied","Message":"asset in use"}`,
		`{"Success":false,"Message":"asset in use"}`,
		`{"Error":{"channel_id":42}}`,
	} {
		t.Run(body, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, "DeleteAsset", r.URL.Query().Get("Action"))
				_, _ = w.Write([]byte(body))
			}))
			defer upstream.Close()
			r := assetSecurityRouter(t, upstream.URL, "youniyouju", "")
			seedAssetOwnershipForTest(t, 20, 1, "asset", "owned")
			w := seedanceCall(r, "POST", "/?Action=DeleteAsset&Version=2024-01-01", `{"Id":"owned"}`, 1)
			require.Equal(t, 200, w.Code, w.Body.String())
			bindings, err := model.FindAssetBindings(1, "asset", "owned")
			require.NoError(t, err)
			require.Len(t, bindings, 1, "failed upstream deletion must keep local ownership")
			require.True(t, gjson.GetBytes(w.Body.Bytes(), "ResponseMetadata.Error").IsObject(), w.Body.String())
			require.False(t, gjson.GetBytes(w.Body.Bytes(), "Result").Exists())
			require.NotContains(t, w.Body.String(), "channel_id")
		})
	}
}

func TestOfficialAssetReviewBareListKeepsPagination(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{"Id":"own-1"},{"Id":"foreign"},{"Id":"own-2"}]`))
	}))
	defer upstream.Close()
	r := assetSecurityRouter(t, upstream.URL, "youniyouju", "")
	seedAssetOwnershipForTest(t, 20, 1, "asset", "own-1")
	seedAssetOwnershipForTest(t, 20, 1, "asset", "own-2")
	seedAssetOwnershipForTest(t, 20, 2, "asset", "foreign")
	w := seedanceCall(r, "POST", "/?Action=ListAssets&Version=2024-01-01", `{"MaxResults":1}`, 1)
	require.Equal(t, 200, w.Code, w.Body.String())
	cursor := gjson.GetBytes(w.Body.Bytes(), "Result.NextToken").String()
	require.NotEmpty(t, cursor, "the customer must be able to reach their second asset")
	require.Equal(t, int64(2), gjson.GetBytes(w.Body.Bytes(), "Result.TotalCount").Int())
	next := seedanceCall(r, "POST", "/?Action=ListAssets&Version=2024-01-01", fmt.Sprintf(`{"MaxResults":1,"NextToken":%q}`, cursor), 1)
	require.Equal(t, 200, next.Code, next.Body.String())
	require.Equal(t, "own-2", gjson.GetBytes(next.Body.Bytes(), "Result.Items.0.Id").String())
	require.Empty(t, gjson.GetBytes(next.Body.Bytes(), "Result.NextToken").String())
	require.NotContains(t, next.Body.String(), "foreign")
}

func TestOfficialAssetReviewMalformedAndExtendedResults(t *testing.T) {
	for _, tc := range []struct {
		name, action, body, result string
		status                     int
	}{
		{"null-create", "CreateAsset", `{"Result":null}`, "", 502},
		{"missing-create-id", "CreateAsset", `{"code":0,"data":{}}`, "", 502},
		{"invalid-id-type", "GetAsset", `{"Result":{"Id":123}}`, "", 502},
		{"empty-official-error", "GetAsset", `{"ResponseMetadata":{"Error":{ }},"Result":{"Id":"owned"}}`, `{"Id":"owned"}`, 200},
		{"item-data-extension", "ListAssets", `{"Result":{"Items":[{"Id":"owned","data":{"label":"extension"}}]}}`, `{"Items":[{"Id":"owned","data":{"label":"extension"}}]}`, 200},
		{"bare-data-extension", "GetAsset", `{"id":"owned","data":{"label":"extension"}}`, `{"Id":"owned","data":{"label":"extension"}}`, 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := gin.New()
			r.POST("/", ArkAssetResponse, func(c *gin.Context) { c.Data(200, "application/json", []byte(tc.body)) })
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest("POST", "/?Action="+tc.action+"&Version=2024-01-01", nil))
			require.Equal(t, tc.status, w.Code, w.Body.String())
			if tc.result == "" {
				require.Equal(t, "asset_response_failed", gjson.GetBytes(w.Body.Bytes(), "ResponseMetadata.Error.Code").String())
			} else {
				require.JSONEq(t, tc.result, gjson.GetBytes(w.Body.Bytes(), "Result").Raw)
			}
		})
	}
}

func TestOfficialAssetReviewPanicUsesOfficialError(t *testing.T) {
	r := gin.New()
	r.Use(gin.CustomRecovery(func(c *gin.Context, _ any) { c.String(500, "outer recovery") }))
	r.POST("/", ArkAssetResponse, func(c *gin.Context) {
		c.Data(200, "application/json", []byte(`{"Result":{"Id":"partial"}}`))
		panic("private connection details")
	})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("POST", "/?Action=GetAsset&Version=2024-01-01", strings.NewReader(`{"Id":"owned"}`)))
	require.Equal(t, 500, w.Code)
	require.Equal(t, "asset_internal_error", gjson.GetBytes(w.Body.Bytes(), "ResponseMetadata.Error.Code").String())
	require.NotContains(t, w.Body.String(), "private")
	require.NotContains(t, w.Body.String(), "partial")
}

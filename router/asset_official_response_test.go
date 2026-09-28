package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/i18n"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestOfficialAssetAuthenticationResponseInFullRouter(t *testing.T) {
	gin.SetMode(gin.TestMode)
	require.NoError(t, i18n.Init())
	r := gin.New()
	SetApiRouter(r)
	SetDashboardRouter(r)
	SetRelayRouter(r)
	SetVideoRouter(r)
	for _, path := range []string{
		"/?Action=CreateAsset&Version=2024-01-01",
		"/api/volcengine_asset?Action=CreateAsset&Version=2024-01-01",
		"/api/assets",
	} {
		t.Run(path, func(t *testing.T) {
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, path, nil))
			require.Equal(t, http.StatusUnauthorized, w.Code, w.Body.String())
			body := gjson.ParseBytes(w.Body.Bytes())
			if path[1] == '?' {
				require.Equal(t, "CreateAsset", body.Get("ResponseMetadata.Action").String())
				require.NotEmpty(t, body.Get("ResponseMetadata.Error.Code").String())
				require.NotEmpty(t, body.Get("ResponseMetadata.Error.Message").String())
				require.NotEmpty(t, body.Get("ResponseMetadata.RequestId").String())
				require.False(t, body.Get("error").Exists())
			} else {
				require.True(t, body.Get("error").Exists())
				require.False(t, body.Get("ResponseMetadata").Exists())
			}
		})
	}
}

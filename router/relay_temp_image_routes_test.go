package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRelayTempImageRouteSupportsHead(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tempDir := t.TempDir()
	t.Setenv("API_TEMP_IMAGE_STORAGE", "local")
	t.Setenv("API_TEMP_IMAGE_DIR", tempDir)
	t.Setenv("API_TEMP_IMAGE_PUBLIC_BASE_URL", "")
	imageURL, err := service.SaveTemporaryImageBytes(nil, []byte("fake-jpg"), "image/jpeg", "source.jpg")
	require.NoError(t, err)

	router := gin.New()
	SetApiRouter(router)

	headRecorder := httptest.NewRecorder()
	headReq := httptest.NewRequest(http.MethodHead, imageURL, nil)
	router.ServeHTTP(headRecorder, headReq)
	require.Equal(t, http.StatusOK, headRecorder.Code)
	require.Empty(t, headRecorder.Body.String())

	getRecorder := httptest.NewRecorder()
	getReq := httptest.NewRequest(http.MethodGet, imageURL, nil)
	router.ServeHTTP(getRecorder, getReq)
	require.Equal(t, http.StatusOK, getRecorder.Code)
	require.Equal(t, "fake-jpg", getRecorder.Body.String())
}

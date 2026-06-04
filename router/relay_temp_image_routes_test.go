package router

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRelayTempImageRouteSupportsHead(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tempDir := t.TempDir()
	t.Setenv("API_TEMP_IMAGE_STORAGE", "local")
	t.Setenv("API_TEMP_IMAGE_DIR", tempDir)
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "source.jpg"), []byte("fake-jpg"), 0644))

	router := gin.New()
	SetApiRouter(router)

	headRecorder := httptest.NewRecorder()
	headReq := httptest.NewRequest(http.MethodHead, "/api/relay-temp-images/source.jpg", nil)
	router.ServeHTTP(headRecorder, headReq)
	require.Equal(t, http.StatusOK, headRecorder.Code)
	require.Empty(t, headRecorder.Body.String())

	getRecorder := httptest.NewRecorder()
	getReq := httptest.NewRequest(http.MethodGet, "/api/relay-temp-images/source.jpg", nil)
	router.ServeHTTP(getRecorder, getReq)
	require.Equal(t, http.StatusOK, getRecorder.Code)
	require.Equal(t, "fake-jpg", getRecorder.Body.String())
}

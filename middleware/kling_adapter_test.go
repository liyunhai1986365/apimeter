package middleware

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestKlingRequestConvertKeepsOfficialImageFieldForConfigurableChannels(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(KlingRequestConvert())
	router.POST("/kling/v1/videos/image2video", func(c *gin.Context) {
		body, err := io.ReadAll(c.Request.Body)
		require.NoError(t, err)
		require.Equal(t, "/v1/video/generations", c.Request.URL.Path)
		require.Equal(t, "kling-v3", gjson.GetBytes(body, "model").String())
		require.Equal(t, "让人物挥手", gjson.GetBytes(body, "prompt").String())
		require.Equal(t, "https://example.com/start.png", gjson.GetBytes(body, "image").String())
		require.Equal(t, "https://example.com/start.png", gjson.GetBytes(body, "metadata.image").String())
		c.Status(http.StatusNoContent)
	})

	body := []byte(`{"model_name":"kling-v3","prompt":"让人物挥手","image":"https://example.com/start.png","duration":"5"}`)
	req := httptest.NewRequest(http.MethodPost, "/kling/v1/videos/image2video", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusNoContent, recorder.Code)
}

package middleware

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestBodyStorageCleanupRemovesMultipartTemporaryFiles(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("image", "large.png")
	require.NoError(t, err)
	_, err = part.Write(bytes.Repeat([]byte("x"), 4096))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	var temporaryPath string
	engine := gin.New()
	engine.Use(BodyStorageCleanup())
	engine.POST("/upload", func(c *gin.Context) {
		require.NoError(t, c.Request.ParseMultipartForm(1))
		originalForm := c.Request.MultipartForm
		reusedForm, parseErr := common.ParseMultipartFormReusable(c)
		require.NoError(t, parseErr)
		require.Same(t, originalForm, reusedForm)
		file, openErr := c.Request.MultipartForm.File["image"][0].Open()
		require.NoError(t, openErr)
		if diskFile, ok := file.(*os.File); ok {
			temporaryPath = diskFile.Name()
		}
		require.NoError(t, file.Close())
		require.NotEmpty(t, temporaryPath)
		require.FileExists(t, temporaryPath)
		c.Status(http.StatusNoContent)
	})

	request := httptest.NewRequest(http.MethodPost, "/upload", bytes.NewReader(body.Bytes()))
	request.Header.Set("Content-Type", writer.FormDataContentType())
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusNoContent, recorder.Code)
	require.NoFileExists(t, temporaryPath)
}

package service

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSaveTemporaryImageUploadsToOSSAndReturnsSignedURL(t *testing.T) {
	var uploadedPath string
	var uploadedBody string
	var uploadedContentType string
	var uploadedAuthorization string
	ossServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPut, r.Method)
		uploadedPath = r.URL.Path
		uploadedContentType = r.Header.Get("Content-Type")
		uploadedAuthorization = r.Header.Get("Authorization")
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		uploadedBody = string(body)
		w.WriteHeader(http.StatusOK)
	}))
	defer ossServer.Close()

	t.Setenv("API_TEMP_IMAGE_STORAGE", "oss")
	t.Setenv("API_TEMP_IMAGE_OSS_ENDPOINT", ossServer.URL)
	t.Setenv("API_TEMP_IMAGE_OSS_BUCKET", "bucket")
	t.Setenv("API_TEMP_IMAGE_OSS_ACCESS_KEY_ID", "access-key")
	t.Setenv("API_TEMP_IMAGE_OSS_ACCESS_KEY_SECRET", "access-secret")
	t.Setenv("API_TEMP_IMAGE_OSS_PREFIX", "test-prefix")
	t.Setenv("API_TEMP_IMAGE_OSS_SIGNED_URL_EXPIRE_SECONDS", "3600")

	imageURL, err := SaveTemporaryImage(nil, multipartImageFileHeader(t, "source.jpg", "image/jpeg", "fake-jpg"))
	require.NoError(t, err)

	require.True(t, strings.HasPrefix(uploadedPath, "/bucket/test-prefix/"), uploadedPath)
	require.True(t, strings.HasSuffix(uploadedPath, ".jpg"), uploadedPath)
	require.Equal(t, "fake-jpg", uploadedBody)
	require.Equal(t, "image/jpeg", uploadedContentType)
	require.True(t, strings.HasPrefix(uploadedAuthorization, "OSS access-key:"), uploadedAuthorization)
	require.Contains(t, imageURL, "/bucket/test-prefix/")
	require.Contains(t, imageURL, "Expires=")
	require.Contains(t, imageURL, "OSSAccessKeyId=access-key")
	require.Contains(t, imageURL, "Signature=")
}

func TestSaveTemporaryImageUsesOSSPublicBaseURL(t *testing.T) {
	ossServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ossServer.Close()

	t.Setenv("API_TEMP_IMAGE_STORAGE", "oss")
	t.Setenv("API_TEMP_IMAGE_OSS_ENDPOINT", ossServer.URL)
	t.Setenv("API_TEMP_IMAGE_OSS_BUCKET", "bucket")
	t.Setenv("API_TEMP_IMAGE_OSS_ACCESS_KEY_ID", "access-key")
	t.Setenv("API_TEMP_IMAGE_OSS_ACCESS_KEY_SECRET", "access-secret")
	t.Setenv("API_TEMP_IMAGE_OSS_PREFIX", "test-prefix")
	t.Setenv("API_TEMP_IMAGE_OSS_PUBLIC_BASE_URL", "https://cdn.example.com/temp")

	imageURL, err := SaveTemporaryImage(nil, multipartImageFileHeader(t, "source.png", "image/png", "fake-png"))
	require.NoError(t, err)

	require.True(t, strings.HasPrefix(imageURL, "https://cdn.example.com/temp/test-prefix/"), imageURL)
	require.True(t, strings.HasSuffix(imageURL, ".png"), imageURL)
	require.NotContains(t, imageURL, "Signature=")
}

func multipartImageFileHeader(t *testing.T, filename, contentType, content string) *multipart.FileHeader {
	t.Helper()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("image", filename)
	require.NoError(t, err)
	_, err = part.Write([]byte(content))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	req := httptest.NewRequest(http.MethodPost, "/upload", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	require.NoError(t, req.ParseMultipartForm(1024))
	fileHeader := req.MultipartForm.File["image"][0]
	fileHeader.Header.Set("Content-Type", contentType)
	return fileHeader
}

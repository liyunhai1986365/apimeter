package service

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/gin-gonic/gin"
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

func TestSaveTemporaryImageUploadsToR2AndReturnsPublicURL(t *testing.T) {
	var uploadedPath string
	var uploadedBody string
	var uploadedAuthorization string
	var uploadedContentSHA string
	r2Server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPut, r.Method)
		uploadedPath = r.URL.Path
		uploadedAuthorization = r.Header.Get("Authorization")
		uploadedContentSHA = r.Header.Get("X-Amz-Content-Sha256")
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		uploadedBody = string(body)
		w.WriteHeader(http.StatusOK)
	}))
	defer r2Server.Close()

	t.Setenv("API_TEMP_IMAGE_STORAGE", "r2")
	t.Setenv("API_TEMP_IMAGE_R2_ENDPOINT", r2Server.URL)
	t.Setenv("API_TEMP_IMAGE_R2_BUCKET", "r2-bucket")
	t.Setenv("API_TEMP_IMAGE_R2_ACCESS_KEY_ID", "r2-access-key")
	t.Setenv("API_TEMP_IMAGE_R2_ACCESS_KEY_SECRET", "r2-access-secret")
	t.Setenv("API_TEMP_IMAGE_R2_PREFIX", "r2-prefix")
	t.Setenv("API_TEMP_IMAGE_R2_PUBLIC_BASE_URL", "https://pub.example.com/assets")

	imageURL, err := SaveTemporaryImage(nil, multipartImageFileHeader(t, "source.png", "image/png", "fake-png"))
	require.NoError(t, err)

	require.Equal(t, "fake-png", uploadedBody)
	require.True(t, strings.HasPrefix(uploadedPath, "/r2-bucket/r2-prefix/"), uploadedPath)
	require.True(t, strings.HasSuffix(uploadedPath, ".png"), uploadedPath)
	require.Contains(t, uploadedAuthorization, "AWS4-HMAC-SHA256")
	require.Contains(t, uploadedAuthorization, "Credential=r2-access-key/")
	require.NotEmpty(t, uploadedContentSHA)
	require.True(t, strings.HasPrefix(imageURL, "https://pub.example.com/assets/r2-prefix/"), imageURL)
	require.True(t, strings.HasSuffix(imageURL, ".png"), imageURL)
}

func TestSaveTemporaryImageUploadsMultipartToR2WithUnsignedPayload(t *testing.T) {
	var uploadedContentSHA string
	var uploadedContentLength int64
	r2Server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPut, r.Method)
		uploadedContentSHA = r.Header.Get("X-Amz-Content-Sha256")
		uploadedContentLength = r.ContentLength
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.Equal(t, "fake-png", string(body))
		w.WriteHeader(http.StatusOK)
	}))
	defer r2Server.Close()

	t.Setenv("API_TEMP_IMAGE_STORAGE", "r2")
	t.Setenv("API_TEMP_IMAGE_R2_ENDPOINT", r2Server.URL)
	t.Setenv("API_TEMP_IMAGE_R2_BUCKET", "r2-bucket")
	t.Setenv("API_TEMP_IMAGE_R2_ACCESS_KEY_ID", "r2-access-key")
	t.Setenv("API_TEMP_IMAGE_R2_ACCESS_KEY_SECRET", "r2-access-secret")
	t.Setenv("API_TEMP_IMAGE_R2_PREFIX", "r2-prefix")
	t.Setenv("API_TEMP_IMAGE_R2_PUBLIC_BASE_URL", "https://pub.example.com/assets")

	_, err := SaveTemporaryImage(nil, multipartImageFileHeader(t, "source.png", "image/png", "fake-png"))
	require.NoError(t, err)

	require.Equal(t, "UNSIGNED-PAYLOAD", uploadedContentSHA)
	require.Equal(t, int64(len("fake-png")), uploadedContentLength)
}

func TestSaveTemporaryImageAutoKeepsOSSWhenBothOSSAndR2Configured(t *testing.T) {
	var ossUploaded bool
	ossServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ossUploaded = true
		w.WriteHeader(http.StatusOK)
	}))
	defer ossServer.Close()
	r2Server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("auto storage should keep OSS when OSS is configured")
	}))
	defer r2Server.Close()

	t.Setenv("API_TEMP_IMAGE_STORAGE", "auto")
	t.Setenv("API_TEMP_IMAGE_OSS_ENDPOINT", ossServer.URL)
	t.Setenv("API_TEMP_IMAGE_OSS_BUCKET", "oss-bucket")
	t.Setenv("API_TEMP_IMAGE_OSS_ACCESS_KEY_ID", "oss-access-key")
	t.Setenv("API_TEMP_IMAGE_OSS_ACCESS_KEY_SECRET", "oss-access-secret")
	t.Setenv("API_TEMP_IMAGE_R2_ENDPOINT", r2Server.URL)
	t.Setenv("API_TEMP_IMAGE_R2_BUCKET", "r2-bucket")
	t.Setenv("API_TEMP_IMAGE_R2_ACCESS_KEY_ID", "r2-access-key")
	t.Setenv("API_TEMP_IMAGE_R2_ACCESS_KEY_SECRET", "r2-access-secret")

	_, err := SaveTemporaryImage(nil, multipartImageFileHeader(t, "source.png", "image/png", "fake-png"))
	require.NoError(t, err)
	require.True(t, ossUploaded)
}

func TestSaveTemporaryImageBase64UsesUserOSSWhenConfigured(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var uploadedPath string
	var uploadedBody string
	var uploadedAuthorization string
	userOSSServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPut, r.Method)
		uploadedPath = r.URL.Path
		uploadedAuthorization = r.Header.Get("Authorization")
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		uploadedBody = string(body)
		w.WriteHeader(http.StatusOK)
	}))
	defer userOSSServer.Close()

	t.Setenv("API_TEMP_IMAGE_STORAGE", "local")
	t.Setenv("API_TEMP_IMAGE_DIR", t.TempDir())
	t.Setenv("API_TEMP_IMAGE_PUBLIC_BASE_URL", "https://site.example.com")

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(c, constant.ContextKeyUserSetting, dto.UserSetting{
		ImageStorage: dto.UserImageStorageSetting{
			Enabled:         true,
			Type:            dto.UserImageStorageTypeAliyunOSS,
			Endpoint:        userOSSServer.URL,
			Bucket:          "user-bucket",
			AccessKeyID:     "user-access-key",
			AccessKeySecret: "user-access-secret",
			ObjectPrefix:    "user-prefix",
			PublicBaseURL:   "https://user-cdn.example.com",
		},
	})

	imageURL, err := SaveTemporaryImageBase64(c, "data:image/png;base64,ZmFrZS1wbmc=")
	require.NoError(t, err)

	require.Equal(t, "fake-png", uploadedBody)
	require.True(t, strings.HasPrefix(uploadedPath, "/user-bucket/user-prefix/"), uploadedPath)
	require.True(t, strings.HasPrefix(uploadedAuthorization, "OSS user-access-key:"), uploadedAuthorization)
	require.True(t, strings.HasPrefix(imageURL, "https://user-cdn.example.com/user-prefix/"), imageURL)
	require.NotContains(t, imageURL, "site.example.com")
}

func TestSaveTemporaryImageBase64UsesUserOSSAccelerateEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)

	normalOSSServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("normal OSS endpoint should not receive upload when accelerate endpoint is configured")
	}))
	defer normalOSSServer.Close()

	var uploadedPath string
	var uploadedBody string
	accelerateOSSServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPut, r.Method)
		uploadedPath = r.URL.Path
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		uploadedBody = string(body)
		w.WriteHeader(http.StatusOK)
	}))
	defer accelerateOSSServer.Close()

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(c, constant.ContextKeyUserSetting, dto.UserSetting{
		ImageStorage: dto.UserImageStorageSetting{
			Enabled:            true,
			Type:               dto.UserImageStorageTypeAliyunOSS,
			Endpoint:           normalOSSServer.URL,
			AccelerateEndpoint: accelerateOSSServer.URL,
			Bucket:             "user-bucket",
			AccessKeyID:        "user-access-key",
			AccessKeySecret:    "user-access-secret",
			ObjectPrefix:       "user-prefix",
			PublicBaseURL:      "https://user-cdn.example.com",
		},
	})

	imageURL, err := SaveTemporaryImageBase64(c, "data:image/png;base64,ZmFrZS1wbmc=")
	require.NoError(t, err)

	require.Equal(t, "fake-png", uploadedBody)
	require.True(t, strings.HasPrefix(uploadedPath, "/user-bucket/user-prefix/"), uploadedPath)
	require.True(t, strings.HasPrefix(imageURL, "https://user-cdn.example.com/user-prefix/"), imageURL)
}

func TestTempImageOSSObjectURLAcceptsBucketAccelerateEndpoint(t *testing.T) {
	requestURL, canonicalResource, err := tempImageOSSObjectURL(tempImageStorageConfig{
		OSSEndpoint:           "https://oss-cn-hangzhou.aliyuncs.com",
		OSSAccelerateEndpoint: "https://user-bucket.oss-accelerate.aliyuncs.com",
		OSSBucket:             "user-bucket",
		OSSAccessKeyID:        "user-access-key",
		OSSAccessKeySecret:    "user-access-secret",
	}, "user-prefix/test.txt")
	require.NoError(t, err)

	require.Equal(t, "https://user-bucket.oss-accelerate.aliyuncs.com/user-prefix/test.txt", requestURL)
	require.Equal(t, "/user-bucket/user-prefix/test.txt", canonicalResource)
}

func TestSaveTemporaryImageBase64UsesUserR2WhenConfigured(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var uploadedPath string
	var uploadedBody string
	var uploadedAuthorization string
	userR2Server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPut, r.Method)
		uploadedPath = r.URL.Path
		uploadedAuthorization = r.Header.Get("Authorization")
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		uploadedBody = string(body)
		w.WriteHeader(http.StatusOK)
	}))
	defer userR2Server.Close()

	t.Setenv("API_TEMP_IMAGE_STORAGE", "local")
	t.Setenv("API_TEMP_IMAGE_DIR", t.TempDir())
	t.Setenv("API_TEMP_IMAGE_PUBLIC_BASE_URL", "https://site.example.com")

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(c, constant.ContextKeyUserSetting, dto.UserSetting{
		ImageStorage: dto.UserImageStorageSetting{
			Enabled:         true,
			Type:            dto.UserImageStorageTypeCloudflareR2,
			Endpoint:        userR2Server.URL,
			Bucket:          "user-r2-bucket",
			AccessKeyID:     "user-r2-access-key",
			AccessKeySecret: "user-r2-access-secret",
			ObjectPrefix:    "user-r2-prefix",
			PublicBaseURL:   "https://user-r2-public.example.com",
		},
	})

	imageURL, err := SaveTemporaryImageBase64(c, "data:image/png;base64,ZmFrZS1wbmc=")
	require.NoError(t, err)

	require.Equal(t, "fake-png", uploadedBody)
	require.True(t, strings.HasPrefix(uploadedPath, "/user-r2-bucket/user-r2-prefix/"), uploadedPath)
	require.Contains(t, uploadedAuthorization, "AWS4-HMAC-SHA256")
	require.True(t, strings.HasPrefix(imageURL, "https://user-r2-public.example.com/user-r2-prefix/"), imageURL)
	require.NotContains(t, imageURL, "site.example.com")
}

func TestSaveTemporaryImageBase64FallsBackToSiteStorageWhenUserOSSDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Setenv("API_TEMP_IMAGE_STORAGE", "local")
	t.Setenv("API_TEMP_IMAGE_DIR", t.TempDir())
	t.Setenv("API_TEMP_IMAGE_PUBLIC_BASE_URL", "https://site.example.com")

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(c, constant.ContextKeyUserSetting, dto.UserSetting{
		ImageStorage: dto.UserImageStorageSetting{
			Enabled:         false,
			Type:            dto.UserImageStorageTypeAliyunOSS,
			Endpoint:        "https://user-oss.example.com",
			Bucket:          "user-bucket",
			AccessKeyID:     "user-access-key",
			AccessKeySecret: "user-access-secret",
		},
	})

	imageURL, err := SaveTemporaryImageBase64(c, "data:image/png;base64,ZmFrZS1wbmc=")
	require.NoError(t, err)

	require.True(t, strings.HasPrefix(imageURL, "https://site.example.com/api/relay-temp-images/"), imageURL)
}

func TestTestUserImageStorageUploadsTestTextFile(t *testing.T) {
	var uploadedPath string
	var uploadedBody string
	var uploadedContentType string
	ossServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPut, r.Method)
		uploadedPath = r.URL.Path
		uploadedContentType = r.Header.Get("Content-Type")
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		uploadedBody = string(body)
		w.WriteHeader(http.StatusOK)
	}))
	defer ossServer.Close()

	publicURL, err := TestUserImageStorage(dto.UserImageStorageSetting{
		Enabled:         true,
		Type:            dto.UserImageStorageTypeAliyunOSS,
		Endpoint:        ossServer.URL,
		Bucket:          "test-bucket",
		AccessKeyID:     "access-key",
		AccessKeySecret: "access-secret",
		ObjectPrefix:    "user-prefix",
		PublicBaseURL:   "https://cdn.example.com",
	})
	require.NoError(t, err)

	require.Equal(t, "test", uploadedBody)
	require.Equal(t, "text/plain; charset=utf-8", uploadedContentType)
	require.True(t, strings.HasPrefix(uploadedPath, "/test-bucket/user-prefix/"), uploadedPath)
	require.True(t, strings.HasSuffix(uploadedPath, "/test.txt"), uploadedPath)
	require.True(t, strings.HasPrefix(publicURL, "https://cdn.example.com/user-prefix/"), publicURL)
	require.True(t, strings.HasSuffix(publicURL, "/test.txt"), publicURL)
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

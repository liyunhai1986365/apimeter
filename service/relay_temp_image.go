package service

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	relayTempImageRoutePrefix          = "/api/relay-temp-images/"
	defaultTempImageOSSObjectPrefix    = "relay-temp-images"
	defaultTempImageOSSSignedURLExpire = 24 * 60 * 60
)

type tempImageStorageConfig struct {
	Storage               string
	LocalDir              string
	LocalPublicBaseURL    string
	Provider              string
	OSSEndpoint           string
	OSSAccelerateEndpoint string
	OSSBucket             string
	OSSAccessKeyID        string
	OSSAccessKeySecret    string
	OSSObjectPrefix       string
	OSSPublicBaseURL      string
	OSSSignedURLExpire    int
	R2Region              string
}

func SaveRelayTempImage(c *gin.Context, fileHeader *multipart.FileHeader) (string, error) {
	return SaveTemporaryImage(c, fileHeader)
}

func SaveTemporaryImage(c *gin.Context, fileHeader *multipart.FileHeader) (string, error) {
	cfg, err := tempImageStorageConfigFromEnv(c)
	if err != nil {
		return "", err
	}
	return saveTemporaryImageWithConfig(c, fileHeader, cfg)
}

func SaveTemporaryImageBase64(c *gin.Context, base64Data string) (string, error) {
	mimeType, cleanBase64, err := DecodeBase64FileData(base64Data)
	if err != nil {
		return "", err
	}
	data, err := base64.StdEncoding.DecodeString(cleanBase64)
	if err != nil {
		return "", fmt.Errorf("decode temp image base64 failed: %w", err)
	}
	return SaveTemporaryImageBytes(c, data, mimeType, tempImageFilenameForMime(mimeType))
}

func SaveTemporaryImageCleanBase64(c *gin.Context, cleanBase64, mimeType string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(strings.TrimSpace(cleanBase64))
	if err != nil {
		return "", fmt.Errorf("decode temp image base64 failed: %w", err)
	}
	if strings.TrimSpace(mimeType) == "" {
		mimeType = "image/png"
	}
	return SaveTemporaryImageBytes(c, data, mimeType, tempImageFilenameForMime(mimeType))
}

func SaveTemporaryImageBytes(c *gin.Context, data []byte, contentType, filename string) (string, error) {
	if len(data) == 0 {
		return "", fmt.Errorf("missing image data")
	}
	cfg, err := tempImageStorageConfigFromEnv(c)
	if err != nil {
		return "", err
	}
	return saveTemporaryImageBytesWithConfig(c, data, contentType, filename, cfg)
}

func TestUserImageStorage(setting dto.UserImageStorageSetting) (string, error) {
	cfg, err := tempImageStorageConfigFromUserImageStorage(setting)
	if err != nil {
		return "", err
	}
	return saveTemporaryFileBytesToCloudStorage([]byte("test"), "text/plain; charset=utf-8", "test.txt", cfg)
}

func saveTemporaryImageWithConfig(c *gin.Context, fileHeader *multipart.FileHeader, cfg tempImageStorageConfig) (string, error) {
	if fileHeader == nil {
		return "", fmt.Errorf("missing image file")
	}
	switch strings.ToLower(strings.TrimSpace(cfg.Storage)) {
	case "oss", "r2":
		return saveTemporaryImageToCloudStorage(fileHeader, cfg)
	case "local", "":
		return saveTemporaryImageToLocal(c, fileHeader, cfg)
	default:
		return "", fmt.Errorf("unsupported temp image storage: %s", cfg.Storage)
	}
}

func saveTemporaryImageBytesWithConfig(c *gin.Context, data []byte, contentType, filename string, cfg tempImageStorageConfig) (string, error) {
	switch strings.ToLower(strings.TrimSpace(cfg.Storage)) {
	case "oss", "r2":
		return saveTemporaryImageBytesToCloudStorage(data, contentType, filename, cfg)
	case "local", "":
		return saveTemporaryImageBytesToLocal(c, data, filename, cfg)
	default:
		return "", fmt.Errorf("unsupported temp image storage: %s", cfg.Storage)
	}
}

func saveTemporaryImageToCloudStorage(fileHeader *multipart.FileHeader, cfg tempImageStorageConfig) (string, error) {
	switch strings.ToLower(strings.TrimSpace(cfg.Storage)) {
	case "oss":
		return saveTemporaryImageToOSS(fileHeader, cfg)
	case "r2":
		file, err := fileHeader.Open()
		if err != nil {
			return "", fmt.Errorf("open image file failed: %w", err)
		}
		defer file.Close()
		return saveTemporaryImageReaderToR2(file, fileHeader.Size, relayTempImageContentType(fileHeader), fileHeader.Filename, cfg)
	default:
		return "", fmt.Errorf("unsupported cloud temp image storage: %s", cfg.Storage)
	}
}

func saveTemporaryImageBytesToCloudStorage(data []byte, contentType, filename string, cfg tempImageStorageConfig) (string, error) {
	switch strings.ToLower(strings.TrimSpace(cfg.Storage)) {
	case "oss":
		return saveTemporaryImageBytesToOSS(data, contentType, filename, cfg)
	case "r2":
		return saveTemporaryImageBytesToR2(data, contentType, filename, cfg)
	default:
		return "", fmt.Errorf("unsupported cloud temp image storage: %s", cfg.Storage)
	}
}

func saveTemporaryImageToLocal(c *gin.Context, fileHeader *multipart.FileHeader, cfg tempImageStorageConfig) (string, error) {
	file, err := fileHeader.Open()
	if err != nil {
		return "", fmt.Errorf("open image file failed: %w", err)
	}
	defer file.Close()

	dir := cfg.LocalDir
	if dir == "" {
		dir = relayTempImageDir()
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("create temp image dir failed: %w", err)
	}

	fileID := uuid.NewString() + relayTempImageExt(fileHeader.Filename)
	path := filepath.Join(dir, fileID)
	out, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0644)
	if err != nil {
		return "", fmt.Errorf("create temp image file failed: %w", err)
	}
	if _, err := io.Copy(out, file); err != nil {
		_ = out.Close()
		_ = os.Remove(path)
		return "", fmt.Errorf("write temp image file failed: %w", err)
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(path)
		return "", fmt.Errorf("close temp image file failed: %w", err)
	}

	publicBaseURL := strings.TrimRight(strings.TrimSpace(cfg.LocalPublicBaseURL), "/")
	if publicBaseURL == "" {
		publicBaseURL = relayTempImagePublicBaseURL(c)
	}
	return publicBaseURL + relayTempImageRoutePrefix + fileID, nil
}

func saveTemporaryImageBytesToLocal(c *gin.Context, data []byte, filename string, cfg tempImageStorageConfig) (string, error) {
	dir := cfg.LocalDir
	if dir == "" {
		dir = relayTempImageDir()
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("create temp image dir failed: %w", err)
	}

	fileID := uuid.NewString() + relayTempImageExt(filename)
	path := filepath.Join(dir, fileID)
	if err := os.WriteFile(path, data, 0644); err != nil {
		return "", fmt.Errorf("write temp image file failed: %w", err)
	}

	publicBaseURL := strings.TrimRight(strings.TrimSpace(cfg.LocalPublicBaseURL), "/")
	if publicBaseURL == "" {
		publicBaseURL = relayTempImagePublicBaseURL(c)
	}
	return publicBaseURL + relayTempImageRoutePrefix + fileID, nil
}

func saveTemporaryImageToOSS(fileHeader *multipart.FileHeader, cfg tempImageStorageConfig) (string, error) {
	if err := validateOSSConfig(cfg); err != nil {
		return "", err
	}
	file, err := fileHeader.Open()
	if err != nil {
		return "", fmt.Errorf("open image file failed: %w", err)
	}
	defer file.Close()

	objectName := tempImageOSSObjectName(cfg.OSSObjectPrefix, relayTempImageExt(fileHeader.Filename))
	contentType := relayTempImageContentType(fileHeader)
	requestURL, canonicalResource, err := tempImageOSSObjectURL(cfg, objectName)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequest(http.MethodPut, requestURL, file)
	if err != nil {
		return "", fmt.Errorf("create oss upload request failed: %w", err)
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Date", time.Now().UTC().Format(http.TimeFormat))
	if fileHeader.Size > 0 {
		req.ContentLength = fileHeader.Size
	}
	stringToSign := strings.Join([]string{
		http.MethodPut,
		"",
		contentType,
		req.Header.Get("Date"),
		canonicalResource,
	}, "\n")
	req.Header.Set("Authorization", "OSS "+cfg.OSSAccessKeyID+":"+ossSignature(cfg.OSSAccessKeySecret, stringToSign))

	resp, err := serviceHTTPClient().Do(req)
	if err != nil {
		return "", fmt.Errorf("upload temp image to oss failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("upload temp image to oss status %d: %s", resp.StatusCode, string(body))
	}
	return tempImageOSSPublicURL(cfg, objectName, canonicalResource)
}

func saveTemporaryImageBytesToOSS(data []byte, contentType, filename string, cfg tempImageStorageConfig) (string, error) {
	if err := validateOSSConfig(cfg); err != nil {
		return "", err
	}
	if contentType = strings.TrimSpace(contentType); contentType == "" {
		contentType = tempImageContentTypeForFilename(filename)
	}
	objectName := tempImageOSSObjectName(cfg.OSSObjectPrefix, relayTempImageExt(filename))
	requestURL, canonicalResource, err := tempImageOSSObjectURL(cfg, objectName)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequest(http.MethodPut, requestURL, bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("create oss upload request failed: %w", err)
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Date", time.Now().UTC().Format(http.TimeFormat))
	req.ContentLength = int64(len(data))
	stringToSign := strings.Join([]string{
		http.MethodPut,
		"",
		contentType,
		req.Header.Get("Date"),
		canonicalResource,
	}, "\n")
	req.Header.Set("Authorization", "OSS "+cfg.OSSAccessKeyID+":"+ossSignature(cfg.OSSAccessKeySecret, stringToSign))

	resp, err := serviceHTTPClient().Do(req)
	if err != nil {
		return "", fmt.Errorf("upload temp image to oss failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("upload temp image to oss status %d: %s", resp.StatusCode, string(body))
	}
	return tempImageOSSPublicURL(cfg, objectName, canonicalResource)
}

func saveTemporaryFileBytesToOSS(data []byte, contentType, filename string, cfg tempImageStorageConfig) (string, error) {
	if err := validateOSSConfig(cfg); err != nil {
		return "", err
	}
	if contentType = strings.TrimSpace(contentType); contentType == "" {
		contentType = "application/octet-stream"
	}
	objectName := tempImageOSSObjectName(cfg.OSSObjectPrefix, "/"+filepath.Base(filename))
	requestURL, canonicalResource, err := tempImageOSSObjectURL(cfg, objectName)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequest(http.MethodPut, requestURL, bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("create oss upload request failed: %w", err)
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Date", time.Now().UTC().Format(http.TimeFormat))
	req.ContentLength = int64(len(data))
	stringToSign := strings.Join([]string{
		http.MethodPut,
		"",
		contentType,
		req.Header.Get("Date"),
		canonicalResource,
	}, "\n")
	req.Header.Set("Authorization", "OSS "+cfg.OSSAccessKeyID+":"+ossSignature(cfg.OSSAccessKeySecret, stringToSign))

	resp, err := serviceHTTPClient().Do(req)
	if err != nil {
		return "", fmt.Errorf("upload temp image to oss failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("upload temp image to oss status %d: %s", resp.StatusCode, string(body))
	}
	return tempImageOSSPublicURL(cfg, objectName, canonicalResource)
}

func saveTemporaryFileBytesToCloudStorage(data []byte, contentType, filename string, cfg tempImageStorageConfig) (string, error) {
	switch strings.ToLower(strings.TrimSpace(cfg.Storage)) {
	case "oss":
		return saveTemporaryFileBytesToOSS(data, contentType, filename, cfg)
	case "r2":
		return saveTemporaryFileBytesToR2(data, contentType, filename, cfg)
	default:
		return "", fmt.Errorf("unsupported cloud temp image storage: %s", cfg.Storage)
	}
}

func saveTemporaryImageBytesToR2(data []byte, contentType, filename string, cfg tempImageStorageConfig) (string, error) {
	if contentType = strings.TrimSpace(contentType); contentType == "" {
		contentType = tempImageContentTypeForFilename(filename)
	}
	objectName := tempImageOSSObjectName(cfg.OSSObjectPrefix, relayTempImageExt(filename))
	return putBytesToR2(data, contentType, objectName, cfg)
}

func saveTemporaryFileBytesToR2(data []byte, contentType, filename string, cfg tempImageStorageConfig) (string, error) {
	if contentType = strings.TrimSpace(contentType); contentType == "" {
		contentType = "application/octet-stream"
	}
	objectName := tempImageOSSObjectName(cfg.OSSObjectPrefix, "/"+filepath.Base(filename))
	return putBytesToR2(data, contentType, objectName, cfg)
}

func saveTemporaryImageReaderToR2(reader io.Reader, size int64, contentType, filename string, cfg tempImageStorageConfig) (string, error) {
	if contentType = strings.TrimSpace(contentType); contentType == "" {
		contentType = tempImageContentTypeForFilename(filename)
	}
	objectName := tempImageOSSObjectName(cfg.OSSObjectPrefix, relayTempImageExt(filename))
	return putReaderToR2(reader, size, contentType, objectName, cfg, "UNSIGNED-PAYLOAD")
}

func putBytesToR2(data []byte, contentType, objectName string, cfg tempImageStorageConfig) (string, error) {
	return putReaderToR2(bytes.NewReader(data), int64(len(data)), contentType, objectName, cfg, sha256Hex(data))
}

func putReaderToR2(reader io.Reader, size int64, contentType, objectName string, cfg tempImageStorageConfig, payloadHash string) (string, error) {
	if err := validateR2Config(cfg); err != nil {
		return "", err
	}
	if reader == nil {
		return "", fmt.Errorf("missing r2 upload body")
	}
	if payloadHash == "" {
		payloadHash = "UNSIGNED-PAYLOAD"
	}
	requestURL, err := tempImageR2ObjectURL(cfg, objectName)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequest(http.MethodPut, requestURL, reader)
	if err != nil {
		return "", fmt.Errorf("create r2 upload request failed: %w", err)
	}
	req.Header.Set("Content-Type", contentType)
	if size >= 0 {
		req.ContentLength = size
	}
	signR2Request(req, cfg, payloadHash)

	resp, err := serviceHTTPClient().Do(req)
	if err != nil {
		return "", fmt.Errorf("upload temp image to r2 failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("upload temp image to r2 status %d: %s", resp.StatusCode, string(body))
	}
	return tempImageR2PublicURL(cfg, objectName)
}

func tempImageStorageConfigFromEnv(c *gin.Context) (tempImageStorageConfig, error) {
	if cfg, ok, err := tempImageStorageConfigFromUserSetting(c); ok || err != nil {
		return cfg, err
	}
	cfg := tempImageStorageConfig{
		Storage:            strings.ToLower(strings.TrimSpace(os.Getenv("API_TEMP_IMAGE_STORAGE"))),
		LocalDir:           strings.TrimSpace(os.Getenv("API_TEMP_IMAGE_DIR")),
		LocalPublicBaseURL: strings.TrimRight(strings.TrimSpace(os.Getenv("API_TEMP_IMAGE_PUBLIC_BASE_URL")), "/"),
		OSSEndpoint:        strings.TrimRight(strings.TrimSpace(os.Getenv("API_TEMP_IMAGE_OSS_ENDPOINT")), "/"),
		OSSBucket:          strings.TrimSpace(os.Getenv("API_TEMP_IMAGE_OSS_BUCKET")),
		OSSAccessKeyID:     strings.TrimSpace(os.Getenv("API_TEMP_IMAGE_OSS_ACCESS_KEY_ID")),
		OSSAccessKeySecret: strings.TrimSpace(os.Getenv("API_TEMP_IMAGE_OSS_ACCESS_KEY_SECRET")),
		OSSObjectPrefix:    strings.Trim(strings.TrimSpace(os.Getenv("API_TEMP_IMAGE_OSS_PREFIX")), "/"),
		OSSPublicBaseURL:   strings.TrimRight(strings.TrimSpace(os.Getenv("API_TEMP_IMAGE_OSS_PUBLIC_BASE_URL")), "/"),
		OSSSignedURLExpire: common.GetEnvOrDefault("API_TEMP_IMAGE_OSS_SIGNED_URL_EXPIRE_SECONDS", defaultTempImageOSSSignedURLExpire),
		R2Region:           strings.TrimSpace(os.Getenv("API_TEMP_IMAGE_R2_REGION")),
	}
	ossConfigured := cfg.hasOSSConfig()
	r2Configured := hasR2EnvConfig()
	useR2Config := cfg.Storage == "r2" || ((cfg.Storage == "" || cfg.Storage == "auto") && r2Configured)
	if useR2Config {
		cfg.OSSEndpoint = strings.TrimRight(strings.TrimSpace(os.Getenv("API_TEMP_IMAGE_R2_ENDPOINT")), "/")
		cfg.OSSBucket = strings.TrimSpace(os.Getenv("API_TEMP_IMAGE_R2_BUCKET"))
		cfg.OSSAccessKeyID = strings.TrimSpace(os.Getenv("API_TEMP_IMAGE_R2_ACCESS_KEY_ID"))
		cfg.OSSAccessKeySecret = strings.TrimSpace(os.Getenv("API_TEMP_IMAGE_R2_ACCESS_KEY_SECRET"))
		cfg.OSSObjectPrefix = strings.Trim(strings.TrimSpace(os.Getenv("API_TEMP_IMAGE_R2_PREFIX")), "/")
		cfg.OSSPublicBaseURL = strings.TrimRight(strings.TrimSpace(os.Getenv("API_TEMP_IMAGE_R2_PUBLIC_BASE_URL")), "/")
		cfg.OSSSignedURLExpire = common.GetEnvOrDefault("API_TEMP_IMAGE_R2_SIGNED_URL_EXPIRE_SECONDS", defaultTempImageOSSSignedURLExpire)
	}
	if cfg.R2Region == "" {
		cfg.R2Region = "auto"
	}
	if cfg.OSSObjectPrefix == "" {
		cfg.OSSObjectPrefix = defaultTempImageOSSObjectPrefix
	}
	if cfg.Storage == "" || cfg.Storage == "auto" {
		if r2Configured {
			cfg.Storage = "r2"
		} else if ossConfigured {
			cfg.Storage = "oss"
		} else {
			cfg.Storage = "local"
		}
	}
	if cfg.LocalPublicBaseURL == "" {
		cfg.LocalPublicBaseURL = relayTempImagePublicBaseURL(c)
	}
	if cfg.Storage == "oss" {
		if err := validateOSSConfig(cfg); err != nil {
			return cfg, err
		}
	}
	if cfg.Storage == "r2" {
		if err := validateR2Config(cfg); err != nil {
			return cfg, err
		}
	}
	return cfg, nil
}

func tempImageStorageConfigFromUserSetting(c *gin.Context) (tempImageStorageConfig, bool, error) {
	if c == nil {
		return tempImageStorageConfig{}, false, nil
	}
	setting, ok := common.GetContextKeyType[dto.UserSetting](c, constant.ContextKeyUserSetting)
	if !ok {
		return tempImageStorageConfig{}, false, nil
	}
	imageStorage := setting.ImageStorage
	if !imageStorage.IsReady() {
		return tempImageStorageConfig{}, false, nil
	}
	return tempImageStorageConfigFromUserImageStorageWithOK(imageStorage)
}

func tempImageStorageConfigFromUserImageStorageWithOK(imageStorage dto.UserImageStorageSetting) (tempImageStorageConfig, bool, error) {
	cfg, err := tempImageStorageConfigFromUserImageStorage(imageStorage)
	return cfg, true, err
}

func tempImageStorageConfigFromUserImageStorage(imageStorage dto.UserImageStorageSetting) (tempImageStorageConfig, error) {
	if !imageStorage.IsReady() {
		return tempImageStorageConfig{}, fmt.Errorf("user image storage config is incomplete")
	}
	storage := "oss"
	if imageStorage.Type == dto.UserImageStorageTypeCloudflareR2 {
		storage = "r2"
	}
	cfg := tempImageStorageConfig{
		Storage:               storage,
		OSSEndpoint:           strings.TrimRight(strings.TrimSpace(imageStorage.Endpoint), "/"),
		OSSAccelerateEndpoint: strings.TrimRight(strings.TrimSpace(imageStorage.AccelerateEndpoint), "/"),
		OSSBucket:             strings.TrimSpace(imageStorage.Bucket),
		OSSAccessKeyID:        strings.TrimSpace(imageStorage.AccessKeyID),
		OSSAccessKeySecret:    strings.TrimSpace(imageStorage.AccessKeySecret),
		OSSObjectPrefix:       strings.Trim(strings.TrimSpace(imageStorage.ObjectPrefix), "/"),
		OSSPublicBaseURL:      strings.TrimRight(strings.TrimSpace(imageStorage.PublicBaseURL), "/"),
		OSSSignedURLExpire:    common.GetEnvOrDefault("API_TEMP_IMAGE_OSS_SIGNED_URL_EXPIRE_SECONDS", defaultTempImageOSSSignedURLExpire),
		R2Region:              "auto",
	}
	if storage == "r2" {
		cfg.OSSSignedURLExpire = common.GetEnvOrDefault("API_TEMP_IMAGE_R2_SIGNED_URL_EXPIRE_SECONDS", defaultTempImageOSSSignedURLExpire)
	}
	if cfg.OSSObjectPrefix == "" {
		cfg.OSSObjectPrefix = defaultTempImageOSSObjectPrefix
	}
	if storage == "r2" {
		if err := validateR2Config(cfg); err != nil {
			return cfg, err
		}
		return cfg, nil
	}
	if err := validateOSSConfig(cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func hasR2EnvConfig() bool {
	return strings.TrimSpace(os.Getenv("API_TEMP_IMAGE_R2_ENDPOINT")) != "" ||
		strings.TrimSpace(os.Getenv("API_TEMP_IMAGE_R2_BUCKET")) != "" ||
		strings.TrimSpace(os.Getenv("API_TEMP_IMAGE_R2_ACCESS_KEY_ID")) != "" ||
		strings.TrimSpace(os.Getenv("API_TEMP_IMAGE_R2_ACCESS_KEY_SECRET")) != ""
}

func (cfg tempImageStorageConfig) hasOSSConfig() bool {
	return cfg.OSSEndpoint != "" &&
		cfg.OSSBucket != "" &&
		cfg.OSSAccessKeyID != "" &&
		cfg.OSSAccessKeySecret != ""
}

func validateOSSConfig(cfg tempImageStorageConfig) error {
	missing := make([]string, 0)
	if cfg.OSSEndpoint == "" {
		missing = append(missing, "API_TEMP_IMAGE_OSS_ENDPOINT")
	}
	if cfg.OSSBucket == "" {
		missing = append(missing, "API_TEMP_IMAGE_OSS_BUCKET")
	}
	if cfg.OSSAccessKeyID == "" {
		missing = append(missing, "API_TEMP_IMAGE_OSS_ACCESS_KEY_ID")
	}
	if cfg.OSSAccessKeySecret == "" {
		missing = append(missing, "API_TEMP_IMAGE_OSS_ACCESS_KEY_SECRET")
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing oss temp image config: %s", strings.Join(missing, ", "))
	}
	return nil
}

func validateR2Config(cfg tempImageStorageConfig) error {
	missing := make([]string, 0)
	if cfg.OSSEndpoint == "" {
		missing = append(missing, "API_TEMP_IMAGE_R2_ENDPOINT")
	}
	if cfg.OSSBucket == "" {
		missing = append(missing, "API_TEMP_IMAGE_R2_BUCKET")
	}
	if cfg.OSSAccessKeyID == "" {
		missing = append(missing, "API_TEMP_IMAGE_R2_ACCESS_KEY_ID")
	}
	if cfg.OSSAccessKeySecret == "" {
		missing = append(missing, "API_TEMP_IMAGE_R2_ACCESS_KEY_SECRET")
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing r2 temp image config: %s", strings.Join(missing, ", "))
	}
	return nil
}

func tempImageOSSObjectName(prefix, suffix string) string {
	prefix = strings.Trim(prefix, "/")
	name := time.Now().UTC().Format("20060102") + "/" + uuid.NewString() + suffix
	if prefix == "" {
		return name
	}
	return prefix + "/" + name
}

func tempImageOSSObjectURL(cfg tempImageStorageConfig, objectName string) (string, string, error) {
	uploadEndpoint := cfg.OSSEndpoint
	if strings.TrimSpace(cfg.OSSAccelerateEndpoint) != "" {
		uploadEndpoint = cfg.OSSAccelerateEndpoint
	}
	endpoint, err := parseOSSEndpoint(uploadEndpoint)
	if err != nil {
		return "", "", err
	}
	escapedObjectName := escapeOSSObjectName(objectName)
	canonicalResource := "/" + cfg.OSSBucket + "/" + objectName
	if shouldUsePathStyleOSSURL(endpoint) {
		u := *endpoint
		u.Path = strings.TrimRight(u.Path, "/") + "/" + cfg.OSSBucket + "/" + escapedObjectName
		return u.String(), canonicalResource, nil
	}
	u := *endpoint
	if !ossEndpointHasBucketHost(endpoint, cfg.OSSBucket) {
		u.Host = cfg.OSSBucket + "." + u.Host
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/" + escapedObjectName
	return u.String(), canonicalResource, nil
}

func tempImageOSSPublicURL(cfg tempImageStorageConfig, objectName, canonicalResource string) (string, error) {
	escapedObjectName := escapeOSSObjectName(objectName)
	if cfg.OSSPublicBaseURL != "" {
		return strings.TrimRight(cfg.OSSPublicBaseURL, "/") + "/" + escapedObjectName, nil
	}
	expires := time.Now().Add(time.Duration(cfg.OSSSignedURLExpire) * time.Second).Unix()
	requestURL, _, err := tempImageOSSObjectURL(cfg, objectName)
	if err != nil {
		return "", err
	}
	stringToSign := strings.Join([]string{
		http.MethodGet,
		"",
		"",
		strconv.FormatInt(expires, 10),
		canonicalResource,
	}, "\n")
	parsed, err := url.Parse(requestURL)
	if err != nil {
		return "", fmt.Errorf("parse oss signed url failed: %w", err)
	}
	query := parsed.Query()
	query.Set("Expires", strconv.FormatInt(expires, 10))
	query.Set("OSSAccessKeyId", cfg.OSSAccessKeyID)
	query.Set("Signature", ossSignature(cfg.OSSAccessKeySecret, stringToSign))
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func tempImageR2ObjectURL(cfg tempImageStorageConfig, objectName string) (string, error) {
	endpoint, err := parseOSSEndpoint(cfg.OSSEndpoint)
	if err != nil {
		return "", err
	}
	escapedObjectName := escapeOSSObjectName(objectName)
	u := *endpoint
	u.Path = strings.TrimRight(u.Path, "/") + "/" + cfg.OSSBucket + "/" + escapedObjectName
	return u.String(), nil
}

func tempImageR2PublicURL(cfg tempImageStorageConfig, objectName string) (string, error) {
	escapedObjectName := escapeOSSObjectName(objectName)
	if cfg.OSSPublicBaseURL != "" {
		return strings.TrimRight(cfg.OSSPublicBaseURL, "/") + "/" + escapedObjectName, nil
	}
	expires := cfg.OSSSignedURLExpire
	if expires <= 0 {
		expires = defaultTempImageOSSSignedURLExpire
	}
	requestURL, err := tempImageR2ObjectURL(cfg, objectName)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequest(http.MethodGet, requestURL, nil)
	if err != nil {
		return "", fmt.Errorf("create r2 signed url request failed: %w", err)
	}
	return presignR2Request(req, cfg, time.Duration(expires)*time.Second), nil
}

func parseOSSEndpoint(endpoint string) (*url.URL, error) {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return nil, fmt.Errorf("oss endpoint is required")
	}
	if !strings.HasPrefix(endpoint, "http://") && !strings.HasPrefix(endpoint, "https://") {
		endpoint = "https://" + endpoint
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Host == "" {
		return nil, fmt.Errorf("invalid oss endpoint: %s", endpoint)
	}
	return parsed, nil
}

func shouldUsePathStyleOSSURL(endpoint *url.URL) bool {
	host := endpoint.Hostname()
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil
}

func ossEndpointHasBucketHost(endpoint *url.URL, bucket string) bool {
	bucket = strings.TrimSpace(bucket)
	if bucket == "" || endpoint == nil {
		return false
	}
	return strings.HasPrefix(strings.ToLower(endpoint.Hostname()), strings.ToLower(bucket)+".")
}

func escapeOSSObjectName(objectName string) string {
	parts := strings.Split(objectName, "/")
	for i, part := range parts {
		parts[i] = url.PathEscape(part)
	}
	return strings.Join(parts, "/")
}

func ossSignature(secret, stringToSign string) string {
	mac := hmac.New(sha1.New, []byte(secret))
	_, _ = mac.Write([]byte(stringToSign))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func signR2Request(req *http.Request, cfg tempImageStorageConfig, payloadHash string) {
	now := time.Now().UTC()
	if payloadHash == "" {
		payloadHash = "UNSIGNED-PAYLOAD"
	}
	req.Header.Set("Host", req.URL.Host)
	req.Header.Set("X-Amz-Content-Sha256", payloadHash)
	req.Header.Set("X-Amz-Date", now.Format("20060102T150405Z"))
	req.Header.Set("Authorization", r2Authorization(req.Method, req.URL, req.Header, cfg, payloadHash, now))
}

func presignR2Request(req *http.Request, cfg tempImageStorageConfig, expires time.Duration) string {
	now := time.Now().UTC()
	credentialScope := r2CredentialScope(cfg, now)
	query := req.URL.Query()
	query.Set("X-Amz-Algorithm", "AWS4-HMAC-SHA256")
	query.Set("X-Amz-Credential", cfg.OSSAccessKeyID+"/"+credentialScope)
	query.Set("X-Amz-Date", now.Format("20060102T150405Z"))
	query.Set("X-Amz-Expires", strconv.FormatInt(int64(expires/time.Second), 10))
	query.Set("X-Amz-SignedHeaders", "host")
	req.URL.RawQuery = query.Encode()

	canonicalRequest := strings.Join([]string{
		req.Method,
		r2CanonicalURI(req.URL),
		r2CanonicalQuery(req.URL.Query()),
		"host:" + strings.ToLower(req.URL.Host) + "\n",
		"host",
		"UNSIGNED-PAYLOAD",
	}, "\n")
	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256",
		now.Format("20060102T150405Z"),
		credentialScope,
		sha256Hex([]byte(canonicalRequest)),
	}, "\n")
	signature := hex.EncodeToString(hmacSHA256Bytes(r2SigningKey(cfg, now), stringToSign))
	query.Set("X-Amz-Signature", signature)
	req.URL.RawQuery = query.Encode()
	return req.URL.String()
}

func r2Authorization(method string, u *url.URL, header http.Header, cfg tempImageStorageConfig, payloadHash string, now time.Time) string {
	credentialScope := r2CredentialScope(cfg, now)
	signedHeaders := "content-type;host;x-amz-content-sha256;x-amz-date"
	canonicalHeaders := strings.Join([]string{
		"content-type:" + strings.TrimSpace(header.Get("Content-Type")),
		"host:" + strings.ToLower(u.Host),
		"x-amz-content-sha256:" + payloadHash,
		"x-amz-date:" + header.Get("X-Amz-Date"),
		"",
	}, "\n")
	canonicalRequest := strings.Join([]string{
		method,
		r2CanonicalURI(u),
		r2CanonicalQuery(u.Query()),
		canonicalHeaders,
		signedHeaders,
		payloadHash,
	}, "\n")
	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256",
		header.Get("X-Amz-Date"),
		credentialScope,
		sha256Hex([]byte(canonicalRequest)),
	}, "\n")
	signature := hex.EncodeToString(hmacSHA256Bytes(r2SigningKey(cfg, now), stringToSign))
	return fmt.Sprintf(
		"AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		cfg.OSSAccessKeyID,
		credentialScope,
		signedHeaders,
		signature,
	)
}

func r2CredentialScope(cfg tempImageStorageConfig, t time.Time) string {
	region := strings.TrimSpace(cfg.R2Region)
	if region == "" {
		region = "auto"
	}
	return t.Format("20060102") + "/" + region + "/s3/aws4_request"
}

func r2SigningKey(cfg tempImageStorageConfig, t time.Time) []byte {
	region := strings.TrimSpace(cfg.R2Region)
	if region == "" {
		region = "auto"
	}
	kDate := hmacSHA256Bytes([]byte("AWS4"+cfg.OSSAccessKeySecret), t.Format("20060102"))
	kRegion := hmacSHA256Bytes(kDate, region)
	kService := hmacSHA256Bytes(kRegion, "s3")
	return hmacSHA256Bytes(kService, "aws4_request")
}

func r2CanonicalURI(u *url.URL) string {
	if u.EscapedPath() == "" {
		return "/"
	}
	return u.EscapedPath()
}

func r2CanonicalQuery(values url.Values) string {
	return values.Encode()
}

func hmacSHA256Bytes(key []byte, data string) []byte {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(data))
	return mac.Sum(nil)
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func relayTempImageContentType(fileHeader *multipart.FileHeader) string {
	if fileHeader != nil {
		if contentType := strings.TrimSpace(fileHeader.Header.Get("Content-Type")); contentType != "" {
			return contentType
		}
		return tempImageContentTypeForFilename(fileHeader.Filename)
	}
	return tempImageContentTypeForFilename("")
}

func tempImageContentTypeForFilename(filename string) string {
	switch relayTempImageExt(filename) {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".webp":
		return "image/webp"
	default:
		return "application/octet-stream"
	}
}

func tempImageFilenameForMime(mimeType string) string {
	switch strings.ToLower(strings.TrimSpace(mimeType)) {
	case "image/jpeg", "image/jpg":
		return "image.jpg"
	case "image/webp":
		return "image.webp"
	default:
		return "image.png"
	}
}

func relayTempImageExt(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".jpg", ".jpeg", ".png", ".webp":
		return ext
	default:
		return ".png"
	}
}

func ServeRelayTempImage(c *gin.Context) {
	fileID := filepath.Base(strings.TrimSpace(c.Param("id")))
	if fileID == "." || fileID == "/" || fileID == "" {
		c.Status(http.StatusNotFound)
		return
	}
	path := filepath.Join(relayTempImageDir(), fileID)
	c.File(path)
}

func relayTempImageDir() string {
	if dir := strings.TrimSpace(os.Getenv("API_TEMP_IMAGE_DIR")); dir != "" {
		return dir
	}
	return filepath.Join(common.GetDiskCacheDir(), "relay-temp-images")
}

func relayTempImagePublicBaseURL(c *gin.Context) string {
	if baseURL := strings.TrimRight(strings.TrimSpace(os.Getenv("API_TEMP_IMAGE_PUBLIC_BASE_URL")), "/"); baseURL != "" {
		return baseURL
	}
	if c == nil || c.Request == nil {
		return ""
	}
	scheme := strings.TrimSpace(c.GetHeader("X-Forwarded-Proto"))
	if scheme == "" {
		scheme = "https"
		if c.Request.TLS == nil {
			scheme = "http"
		}
	}
	host := strings.TrimSpace(c.GetHeader("X-Forwarded-Host"))
	if host == "" {
		host = c.Request.Host
	}
	if host == "" {
		return ""
	}
	return scheme + "://" + host
}

func serviceHTTPClient() *http.Client {
	client := GetHttpClient()
	if client != nil {
		return client
	}
	return http.DefaultClient
}

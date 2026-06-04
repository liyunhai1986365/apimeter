package service

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
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
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	relayTempImageRoutePrefix          = "/api/relay-temp-images/"
	defaultTempImageOSSObjectPrefix    = "relay-temp-images"
	defaultTempImageOSSSignedURLExpire = 24 * 60 * 60
)

type tempImageStorageConfig struct {
	Storage            string
	LocalDir           string
	LocalPublicBaseURL string
	OSSEndpoint        string
	OSSBucket          string
	OSSAccessKeyID     string
	OSSAccessKeySecret string
	OSSObjectPrefix    string
	OSSPublicBaseURL   string
	OSSSignedURLExpire int
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

func saveTemporaryImageWithConfig(c *gin.Context, fileHeader *multipart.FileHeader, cfg tempImageStorageConfig) (string, error) {
	if fileHeader == nil {
		return "", fmt.Errorf("missing image file")
	}
	switch strings.ToLower(strings.TrimSpace(cfg.Storage)) {
	case "oss":
		return saveTemporaryImageToOSS(fileHeader, cfg)
	case "local", "":
		return saveTemporaryImageToLocal(c, fileHeader, cfg)
	default:
		return "", fmt.Errorf("unsupported temp image storage: %s", cfg.Storage)
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

func tempImageStorageConfigFromEnv(c *gin.Context) (tempImageStorageConfig, error) {
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
	}
	if cfg.OSSObjectPrefix == "" {
		cfg.OSSObjectPrefix = defaultTempImageOSSObjectPrefix
	}
	if cfg.Storage == "" || cfg.Storage == "auto" {
		if cfg.hasOSSConfig() {
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
	return cfg, nil
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

func tempImageOSSObjectName(prefix, ext string) string {
	prefix = strings.Trim(prefix, "/")
	name := time.Now().UTC().Format("20060102") + "/" + uuid.NewString() + ext
	if prefix == "" {
		return name
	}
	return prefix + "/" + name
}

func tempImageOSSObjectURL(cfg tempImageStorageConfig, objectName string) (string, string, error) {
	endpoint, err := parseOSSEndpoint(cfg.OSSEndpoint)
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
	u.Host = cfg.OSSBucket + "." + u.Host
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

func relayTempImageContentType(fileHeader *multipart.FileHeader) string {
	if fileHeader != nil {
		if contentType := strings.TrimSpace(fileHeader.Header.Get("Content-Type")); contentType != "" {
			return contentType
		}
	}
	switch relayTempImageExt(fileHeader.Filename) {
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

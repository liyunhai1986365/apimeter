package model

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
)

const (
	truncatedSuffix       = "...[TRUNCATED]"
	requestLogRedactedVal = "[REDACTED]"
	binaryContentPrefix   = "[BASE64_BINARY_CONTENT]"
)

var (
	requestLogQueue      chan RequestLogRecord
	requestLogWorkerOnce sync.Once
	requestLogStore      RequestLogStore
)

type RequestLogStore interface {
	WriteBatch(records []RequestLogRecord) error
}

type openObserveRequestLogConfig struct {
	Endpoint string
	Org      string
	Stream   string
	User     string
	Password string
	Token    string
	Timeout  time.Duration
}

type openObserveRequestLogStore struct {
	cfg    openObserveRequestLogConfig
	client *http.Client
}

type RequestLogRecord struct {
	LogId             int    `json:"log_id"`
	CreatedAt         int64  `json:"created_at"`
	ExpiresAt         int64  `json:"expires_at"`
	UserId            int    `json:"user_id"`
	Username          string `json:"username"`
	UserGroup         string `json:"user_group"`
	UserEmail         string `json:"user_email"`
	TokenId           int    `json:"token_id"`
	TokenName         string `json:"token_name"`
	ChannelId         int    `json:"channel_id"`
	ChannelName       string `json:"channel_name"`
	ChannelType       int    `json:"channel_type"`
	ChannelBaseUrl    string `json:"channel_base_url"`
	IsMultiKey        bool   `json:"is_multi_key"`
	MultiKeyIndex     int    `json:"multi_key_index"`
	RequestIP         string `json:"request_ip"`
	RequestMethod     string `json:"request_method"`
	RequestPath       string `json:"request_path"`
	RequestURL        string `json:"request_url"`
	RequestHeaders    string `json:"request_headers"`
	RequestId         string `json:"request_id"`
	UpstreamRequestId string `json:"upstream_request_id"`
	RelayMode         int    `json:"relay_mode"`
	RelayFormat       string `json:"relay_format"`
	ModelName         string `json:"model_name"`
	UpstreamModelName string `json:"upstream_model_name"`
	IsStream          bool   `json:"is_stream"`
	Status            string `json:"status"`
	StatusCode        int    `json:"status_code"`
	ErrorCode         string `json:"error_code"`
	ErrorMessage      string `json:"error_message"`
	RequestBody       string `json:"request_body"`
	ResponseBody      string `json:"response_body"`
	RequestSize       int    `json:"request_size"`
	ResponseSize      int    `json:"response_size"`
	RequestTruncated  bool   `json:"request_truncated"`
	ResponseTruncated bool   `json:"response_truncated"`
	RequestHash       string `json:"request_hash"`
	ResponseHash      string `json:"response_hash"`
	Extra             string `json:"extra"`
}

func InitRequestLogStore() error {
	requestLogStore = nil
	requestLogQueue = nil

	if !common.RequestLogEnabled {
		return nil
	}

	storage := strings.ToLower(strings.TrimSpace(common.GetEnvOrDefaultString("REQUEST_LOG_STORAGE", "openobserve")))
	if storage != "openobserve" {
		common.RequestLogEnabled = false
		return fmt.Errorf("request log storage only supports openobserve")
	}

	store, err := initOpenObserveRequestLogStore()
	if err != nil {
		common.RequestLogEnabled = false
		return err
	}
	requestLogStore = store
	initRequestLogQueue()
	common.RequestLogEnabled = true
	common.SysLog("request log OpenObserve storage initialized")
	return nil
}

func initRequestLogQueue() {
	if common.RequestLogQueueSize <= 0 {
		common.RequestLogQueueSize = 1000
	}
	requestLogQueue = make(chan RequestLogRecord, common.RequestLogQueueSize)
}

func initOpenObserveRequestLogStore() (RequestLogStore, error) {
	cfg := openObserveRequestLogConfig{
		Endpoint: strings.TrimSpace(os.Getenv("REQUEST_LOG_OPENOBSERVE_ENDPOINT")),
		Org:      strings.TrimSpace(common.GetEnvOrDefaultString("REQUEST_LOG_OPENOBSERVE_ORG", "default")),
		Stream:   strings.TrimSpace(common.GetEnvOrDefaultString("REQUEST_LOG_OPENOBSERVE_STREAM", "request_log_records")),
		User:     strings.TrimSpace(os.Getenv("REQUEST_LOG_OPENOBSERVE_USER")),
		Password: strings.TrimSpace(os.Getenv("REQUEST_LOG_OPENOBSERVE_PASSWORD")),
		Token:    strings.TrimSpace(os.Getenv("REQUEST_LOG_OPENOBSERVE_TOKEN")),
		Timeout:  time.Second * time.Duration(common.GetEnvOrDefault("REQUEST_LOG_OPENOBSERVE_TIMEOUT_SECONDS", 10)),
	}
	if cfg.Endpoint == "" {
		return nil, fmt.Errorf("REQUEST_LOG_OPENOBSERVE_ENDPOINT is required when REQUEST_LOG_STORAGE=openobserve")
	}
	if cfg.Org == "" {
		return nil, fmt.Errorf("REQUEST_LOG_OPENOBSERVE_ORG is required when REQUEST_LOG_STORAGE=openobserve")
	}
	if cfg.Stream == "" {
		return nil, fmt.Errorf("REQUEST_LOG_OPENOBSERVE_STREAM is required when REQUEST_LOG_STORAGE=openobserve")
	}
	if cfg.Token == "" && (cfg.User == "" || cfg.Password == "") {
		return nil, fmt.Errorf("REQUEST_LOG_OPENOBSERVE_TOKEN or REQUEST_LOG_OPENOBSERVE_USER/PASSWORD is required when REQUEST_LOG_STORAGE=openobserve")
	}
	return newOpenObserveRequestLogStore(cfg), nil
}

func newOpenObserveRequestLogStore(cfg openObserveRequestLogConfig) *openObserveRequestLogStore {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 10 * time.Second
	}
	cfg.Endpoint = strings.TrimRight(strings.TrimSpace(cfg.Endpoint), "/")
	return &openObserveRequestLogStore{
		cfg: cfg,
		client: &http.Client{
			Timeout: cfg.Timeout,
		},
	}
}

func (s *openObserveRequestLogStore) WriteBatch(records []RequestLogRecord) error {
	if s == nil {
		return fmt.Errorf("request log OpenObserve storage is not initialized")
	}
	payload := make([]map[string]any, 0, len(records))
	for i := range records {
		payload = append(payload, requestLogRecordToOpenObserveDocument(records[i]))
	}
	body, err := common.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, s.ingestURL(), bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if s.cfg.Token != "" {
		req.Header.Set("Authorization", "Bearer "+s.cfg.Token)
	} else {
		req.SetBasicAuth(s.cfg.User, s.cfg.Password)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("OpenObserve request log ingest failed with status %d", resp.StatusCode)
	}
	return nil
}

func (s *openObserveRequestLogStore) ingestURL() string {
	return fmt.Sprintf("%s/api/%s/%s/_json", s.cfg.Endpoint, s.cfg.Org, s.cfg.Stream)
}

func requestLogRecordToOpenObserveDocument(record RequestLogRecord) map[string]any {
	return map[string]any{
		"_timestamp":          record.CreatedAt * 1000000,
		"log_type":            "request_log",
		"log_id":              record.LogId,
		"created_at":          record.CreatedAt,
		"expires_at":          record.ExpiresAt,
		"user_id":             record.UserId,
		"username":            record.Username,
		"user_group":          record.UserGroup,
		"user_email":          record.UserEmail,
		"token_id":            record.TokenId,
		"token_name":          record.TokenName,
		"channel_id":          record.ChannelId,
		"channel_name":        record.ChannelName,
		"channel_type":        record.ChannelType,
		"channel_base_url":    record.ChannelBaseUrl,
		"is_multi_key":        record.IsMultiKey,
		"multi_key_index":     record.MultiKeyIndex,
		"request_ip":          record.RequestIP,
		"request_method":      record.RequestMethod,
		"request_path":        record.RequestPath,
		"request_url":         record.RequestURL,
		"request_headers":     record.RequestHeaders,
		"request_id":          record.RequestId,
		"upstream_request_id": record.UpstreamRequestId,
		"relay_mode":          record.RelayMode,
		"relay_format":        record.RelayFormat,
		"model_name":          record.ModelName,
		"upstream_model_name": record.UpstreamModelName,
		"is_stream":           record.IsStream,
		"status":              record.Status,
		"status_code":         record.StatusCode,
		"error_code":          record.ErrorCode,
		"error_message":       record.ErrorMessage,
		"request_body":        record.RequestBody,
		"response_body":       record.ResponseBody,
		"request_size":        record.RequestSize,
		"response_size":       record.ResponseSize,
		"request_truncated":   record.RequestTruncated,
		"response_truncated":  record.ResponseTruncated,
		"request_hash":        record.RequestHash,
		"response_hash":       record.ResponseHash,
		"extra":               record.Extra,
	}
}

func StartRequestLogWorker() {
	if !common.RequestLogEnabled || requestLogStore == nil || requestLogQueue == nil {
		return
	}
	requestLogWorkerOnce.Do(func() {
		go runRequestLogWorker()
	})
}

func EnqueueRequestLog(record RequestLogRecord) bool {
	if !CanAcceptRequestLog() {
		return false
	}
	select {
	case requestLogQueue <- record:
		return true
	default:
		return false
	}
}

func SetRequestLogStoreForTest(store RequestLogStore) {
	requestLogStore = store
}

func ResetRequestLogQueueForTest(size int) {
	if size <= 0 {
		requestLogQueue = nil
		return
	}
	requestLogQueue = make(chan RequestLogRecord, size)
}

func CanAcceptRequestLog() bool {
	if !common.RequestLogEnabled || requestLogQueue == nil {
		return false
	}
	return len(requestLogQueue) < cap(requestLogQueue)
}

func RecordRequestLog(record RequestLogRecord) (RequestLogRecord, error) {
	if requestLogStore == nil {
		return record, fmt.Errorf("request log storage is not initialized")
	}
	prepareRequestLogRecord(&record)
	batch := []RequestLogRecord{record}
	if err := requestLogStore.WriteBatch(batch); err != nil {
		return record, err
	}
	return batch[0], nil
}

func runRequestLogWorker() {
	flushInterval := time.Second * time.Duration(common.RequestLogFlushIntervalSeconds)
	if flushInterval <= 0 {
		flushInterval = time.Second
	}
	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()

	batchSize := common.RequestLogBatchSize
	if batchSize <= 0 {
		batchSize = 100
	}
	batch := make([]RequestLogRecord, 0, batchSize)
	flush := func() {
		if len(batch) == 0 || requestLogStore == nil {
			return
		}
		for i := range batch {
			prepareRequestLogRecord(&batch[i])
		}
		if err := requestLogStore.WriteBatch(batch); err != nil {
			common.SysError("failed to batch record request logs: " + err.Error())
			for i := range batch {
				if singleErr := requestLogStore.WriteBatch([]RequestLogRecord{batch[i]}); singleErr != nil {
					common.SysError("failed to record request log: " + singleErr.Error())
				}
			}
		}
		batch = batch[:0]
	}

	for {
		select {
		case record := <-requestLogQueue:
			batch = append(batch, record)
			if len(batch) >= batchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		}
	}
}

func prepareRequestLogRecord(record *RequestLogRecord) {
	now := common.GetTimestamp()
	if record.CreatedAt == 0 {
		record.CreatedAt = now
	}
	if common.RequestLogRetentionDays > 0 && record.ExpiresAt == 0 {
		record.ExpiresAt = record.CreatedAt + int64(common.RequestLogRetentionDays*24*60*60)
	}

	record.RequestHash = hashContent(record.RequestBody)
	record.ResponseHash = hashContent(record.ResponseBody)
	record.RequestBody = normalizeRequestLogText(record.RequestBody)
	record.ResponseBody = normalizeRequestLogText(record.ResponseBody)
	record.RequestHeaders = normalizeRequestLogText(record.RequestHeaders)
	record.ErrorMessage = normalizeRequestLogText(record.ErrorMessage)
	record.Extra = normalizeRequestLogText(record.Extra)

	if common.RequestLogRedactEnabled {
		record.RequestHeaders = redactContent(record.RequestHeaders)
		record.RequestBody = redactContent(record.RequestBody)
		record.ResponseBody = redactContent(record.ResponseBody)
		record.ErrorMessage = redactContent(record.ErrorMessage)
		record.Extra = redactContent(record.Extra)
	}

	record.RequestBody, record.RequestTruncated = truncateContent(record.RequestBody, common.RequestLogMaxRequestBytes)
	record.ResponseBody, record.ResponseTruncated = truncateContent(record.ResponseBody, common.RequestLogMaxResponseBytes)
	record.RequestSize = len(record.RequestBody)
	record.ResponseSize = len(record.ResponseBody)
	record.RequestHeaders, _ = truncateContent(record.RequestHeaders, common.RequestLogMaxRequestBytes)
	record.ErrorMessage, _ = truncateContent(record.ErrorMessage, common.RequestLogMaxResponseBytes)
	record.Extra, _ = truncateContent(record.Extra, common.RequestLogMaxResponseBytes)
}

func hashContent(content string) string {
	if content == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(content))
	return fmt.Sprintf("%x", sum)
}

func truncateContent(content string, maxBytes int) (string, bool) {
	content = normalizeRequestLogText(content)
	if maxBytes <= 0 || len(content) <= maxBytes {
		return content, false
	}
	truncated := content[:maxBytes]
	for !utf8.ValidString(truncated) && len(truncated) > 0 {
		truncated = truncated[:len(truncated)-1]
	}
	return truncated + truncatedSuffix, true
}

func normalizeRequestLogText(content string) string {
	if content == "" || utf8.ValidString(content) {
		return content
	}
	encoded := base64.StdEncoding.EncodeToString([]byte(content))
	return binaryContentPrefix + "\n" + encoded
}

func redactContent(content string) string {
	if strings.TrimSpace(content) == "" {
		return content
	}
	var data any
	if err := common.Unmarshal([]byte(content), &data); err == nil {
		redactJSONValue(data, "")
		if b, err := common.Marshal(data); err == nil {
			return string(b)
		}
	}
	return redactPlainText(content)
}

func redactJSONValue(value any, key string) {
	switch typed := value.(type) {
	case map[string]any:
		for k, v := range typed {
			if isSensitiveLogKey(k) {
				typed[k] = requestLogRedactedVal
				continue
			}
			redactJSONValue(v, k)
		}
	case []any:
		for i := range typed {
			redactJSONValue(typed[i], key)
		}
	}
}

func redactPlainText(content string) string {
	redacted := content
	for _, key := range []string{
		"authorization", "api_key", "apikey", "access_token", "refresh_token",
		"password", "token", "secret", "key", "cookie", "set-cookie", "x-api-key",
	} {
		lower := strings.ToLower(redacted)
		idx := strings.Index(lower, key)
		if idx < 0 {
			continue
		}
		valueStart := idx + len(key)
		for valueStart < len(redacted) {
			ch := redacted[valueStart]
			if ch != ' ' && ch != '\t' && ch != ':' && ch != '=' && ch != '"' && ch != '\'' {
				break
			}
			valueStart++
		}
		valueEnd := valueStart
		for valueEnd < len(redacted) {
			ch := redacted[valueEnd]
			if ch == ',' || ch == '&' || ch == '\n' || ch == '\r' || ch == '"' || ch == '\'' || ch == '}' {
				break
			}
			valueEnd++
		}
		if valueEnd > valueStart {
			redacted = redacted[:valueStart] + requestLogRedactedVal + redacted[valueEnd:]
		}
	}
	return redacted
}

func isSensitiveLogKey(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	switch normalized {
	case "authorization", "proxy-authorization", "api_key", "apikey", "access_token", "refresh_token",
		"auth_token", "bearer_token", "id_token", "session_token", "password", "token", "secret", "key",
		"private_key", "client_secret", "cookie", "set-cookie", "x-api-key":
		return true
	default:
		return strings.Contains(normalized, "secret") ||
			strings.Contains(normalized, "password") ||
			strings.Contains(normalized, "api_key") ||
			strings.Contains(normalized, "apikey")
	}
}

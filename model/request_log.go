package model

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

const (
	truncatedSuffix       = "...[TRUNCATED]"
	requestLogRedactedVal = "[REDACTED]"
	mojibakeErrorCode     = "mojibake_detected"
	mojibakeErrorMessage  = "返回结果存在乱码"
	binaryContentPrefix   = "[BASE64_BINARY_CONTENT]"
)

var (
	requestLogQueue      chan RequestLogRecord
	requestLogWorkerOnce sync.Once
)

type RequestLogRecord struct {
	Id                int64  `json:"id" gorm:"primaryKey"`
	LogId             int    `json:"log_id" gorm:"index"`
	CreatedAt         int64  `json:"created_at" gorm:"bigint;index:idx_request_logs_created_at;index:idx_request_logs_user_created,priority:2"`
	ExpiresAt         int64  `json:"expires_at" gorm:"bigint;index"`
	UserId            int    `json:"user_id" gorm:"index:idx_request_logs_user_created,priority:1"`
	Username          string `json:"username" gorm:"type:varchar(128);index;default:''"`
	UserGroup         string `json:"user_group" gorm:"type:varchar(64);index;default:''"`
	UserEmail         string `json:"user_email" gorm:"type:varchar(255);index;default:''"`
	TokenId           int    `json:"token_id" gorm:"index"`
	TokenName         string `json:"token_name" gorm:"type:varchar(128);index;default:''"`
	ChannelId         int    `json:"channel_id" gorm:"index"`
	ChannelName       string `json:"channel_name" gorm:"type:varchar(128);index;default:''"`
	ChannelType       int    `json:"channel_type" gorm:"index"`
	ChannelBaseUrl    string `json:"channel_base_url" gorm:"type:varchar(512);default:''"`
	IsMultiKey        bool   `json:"is_multi_key" gorm:"default:false"`
	MultiKeyIndex     int    `json:"multi_key_index" gorm:"default:0"`
	RequestIP         string `json:"request_ip" gorm:"type:varchar(64);index;default:''"`
	RequestMethod     string `json:"request_method" gorm:"type:varchar(16);default:''"`
	RequestPath       string `json:"request_path" gorm:"type:varchar(512);index;default:''"`
	RequestURL        string `json:"request_url"`
	RequestHeaders    string `json:"request_headers"`
	RequestId         string `json:"request_id" gorm:"type:varchar(64);index"`
	UpstreamRequestId string `json:"upstream_request_id" gorm:"type:varchar(128);index"`
	RelayMode         int    `json:"relay_mode" gorm:"index"`
	RelayFormat       string `json:"relay_format" gorm:"type:varchar(64);index;default:''"`
	ModelName         string `json:"model_name" gorm:"type:varchar(128);index;default:''"`
	UpstreamModelName string `json:"upstream_model_name" gorm:"type:varchar(128);index;default:''"`
	IsStream          bool   `json:"is_stream" gorm:"index;default:false"`
	Status            string `json:"status" gorm:"type:varchar(32);index;default:''"`
	StatusCode        int    `json:"status_code" gorm:"index"`
	ErrorCode         string `json:"error_code" gorm:"type:varchar(128);index;default:''"`
	ErrorMessage      string `json:"error_message"`
	RequestBody       string `json:"request_body"`
	ResponseBody      string `json:"response_body"`
	RequestSize       int    `json:"request_size" gorm:"default:0"`
	ResponseSize      int    `json:"response_size" gorm:"default:0"`
	RequestTruncated  bool   `json:"request_truncated" gorm:"default:false"`
	ResponseTruncated bool   `json:"response_truncated" gorm:"default:false"`
	RequestHash       string `json:"request_hash" gorm:"type:varchar(64);index;default:''"`
	ResponseHash      string `json:"response_hash" gorm:"type:varchar(64);index;default:''"`
	Extra             string `json:"extra"`
}

func InitRequestLogDB() error {
	dsn := os.Getenv("REQUEST_LOG_SQL_DSN")
	if strings.TrimSpace(dsn) == "" || !common.RequestLogEnabled {
		REQUEST_LOG_DB = nil
		common.RequestLogEnabled = false
		requestLogQueue = nil
		return nil
	}

	db, err := openRequestLogDB(dsn)
	if err != nil {
		return err
	}
	if common.DebugEnabled {
		db = db.Debug()
	}
	REQUEST_LOG_DB = db

	sqlDB, err := REQUEST_LOG_DB.DB()
	if err != nil {
		return err
	}
	sqlDB.SetMaxIdleConns(common.GetEnvOrDefault("REQUEST_LOG_SQL_MAX_IDLE_CONNS", 10))
	sqlDB.SetMaxOpenConns(common.GetEnvOrDefault("REQUEST_LOG_SQL_MAX_OPEN_CONNS", 100))
	sqlDB.SetConnMaxLifetime(time.Second * time.Duration(common.GetEnvOrDefault("REQUEST_LOG_SQL_MAX_LIFETIME", 60)))

	if err := REQUEST_LOG_DB.AutoMigrate(&RequestLogRecord{}); err != nil {
		return err
	}
	if err := ensureRequestLogLargeTextColumns(REQUEST_LOG_DB); err != nil {
		return err
	}

	if common.RequestLogQueueSize <= 0 {
		common.RequestLogQueueSize = 1000
	}
	requestLogQueue = make(chan RequestLogRecord, common.RequestLogQueueSize)
	common.RequestLogEnabled = true
	common.SysLog("request log database initialized")
	return nil
}

func openRequestLogDB(dsn string) (*gorm.DB, error) {
	switch {
	case strings.HasPrefix(dsn, "postgres://"), strings.HasPrefix(dsn, "postgresql://"):
		common.SysLog("using PostgreSQL as request log database")
		return gorm.Open(postgres.New(postgres.Config{
			DSN:                  dsn,
			PreferSimpleProtocol: true,
		}), &gorm.Config{PrepareStmt: true})
	case strings.HasPrefix(dsn, "file:"), strings.HasSuffix(dsn, ".db"), strings.HasPrefix(dsn, "local"):
		common.SysLog("using SQLite as request log database")
		if strings.HasPrefix(dsn, "local") {
			dsn = common.SQLitePath
		}
		return gorm.Open(sqlite.Open(dsn), &gorm.Config{PrepareStmt: true})
	default:
		common.SysLog("using MySQL as request log database")
		if !strings.Contains(dsn, "parseTime") {
			if strings.Contains(dsn, "?") {
				dsn += "&parseTime=true"
			} else {
				dsn += "?parseTime=true"
			}
		}
		return gorm.Open(mysql.Open(dsn), &gorm.Config{PrepareStmt: true})
	}
}

func ensureRequestLogLargeTextColumns(db *gorm.DB) error {
	if db == nil || db.Dialector.Name() != "mysql" {
		return nil
	}
	for _, columnName := range []string{
		"request_url",
		"request_headers",
		"error_message",
		"request_body",
		"response_body",
		"extra",
	} {
		if err := ensureMySQLRequestLogMediumTextColumn(db, columnName); err != nil {
			return err
		}
	}
	return nil
}

func ensureMySQLRequestLogMediumTextColumn(db *gorm.DB, columnName string) error {
	var columnType string
	err := db.Raw(`SELECT COLUMN_TYPE FROM information_schema.columns
		WHERE table_schema = DATABASE() AND table_name = ? AND column_name = ?`,
		"request_log_records", columnName).Scan(&columnType).Error
	if err != nil {
		return fmt.Errorf("failed to query request log column %s: %w", columnName, err)
	}
	normalized := strings.ToLower(columnType)
	if strings.Contains(normalized, "mediumtext") || strings.Contains(normalized, "longtext") {
		return nil
	}
	if err := db.Exec(fmt.Sprintf("ALTER TABLE `request_log_records` MODIFY COLUMN `%s` MEDIUMTEXT", columnName)).Error; err != nil {
		return fmt.Errorf("failed to migrate request_log_records.%s to MEDIUMTEXT: %w", columnName, err)
	}
	common.SysLog(fmt.Sprintf("migrated request_log_records.%s to MEDIUMTEXT", columnName))
	return nil
}

func StartRequestLogWorker() {
	if !common.RequestLogEnabled || REQUEST_LOG_DB == nil || requestLogQueue == nil {
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

func CanAcceptRequestLog() bool {
	if !common.RequestLogEnabled || requestLogQueue == nil {
		return false
	}
	return len(requestLogQueue) < cap(requestLogQueue)
}

func RecordRequestLog(record RequestLogRecord) (RequestLogRecord, error) {
	if REQUEST_LOG_DB == nil {
		return record, fmt.Errorf("request log database is not initialized")
	}
	prepareRequestLogRecord(&record)
	return record, REQUEST_LOG_DB.Create(&record).Error
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
		if len(batch) == 0 || REQUEST_LOG_DB == nil {
			return
		}
		for i := range batch {
			prepareRequestLogRecord(&batch[i])
		}
		if err := REQUEST_LOG_DB.Create(&batch).Error; err != nil {
			common.SysError("failed to batch record request logs: " + err.Error())
			for i := range batch {
				if singleErr := REQUEST_LOG_DB.Create(&batch[i]).Error; singleErr != nil {
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
	markMojibakeIfNeeded(record)
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

	if record.ErrorCode == mojibakeErrorCode {
		markOriginalLogAsMojibake(record)
	}
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
	case "authorization", "api_key", "apikey", "access_token", "refresh_token",
		"password", "token", "secret", "key", "cookie", "set-cookie", "x-api-key":
		return true
	default:
		return strings.Contains(normalized, "secret") ||
			strings.Contains(normalized, "password") ||
			strings.Contains(normalized, "token") ||
			strings.Contains(normalized, "api_key")
	}
}

func markMojibakeIfNeeded(record *RequestLogRecord) {
	if record == nil || record.ResponseBody == "" {
		return
	}
	if !containsMojibake(record.ResponseBody) {
		return
	}
	record.Status = "error"
	record.ErrorCode = mojibakeErrorCode
	record.ErrorMessage = mojibakeErrorMessage
	if record.StatusCode < 400 {
		record.StatusCode = 500
	}
}

func containsMojibake(content string) bool {
	if content == "" {
		return false
	}
	if !utf8.ValidString(content) {
		return true
	}

	runes := []rune(content)
	if len(runes) == 0 {
		return false
	}

	replacementCount := 0
	suspiciousCount := 0
	for i, r := range runes {
		if r == utf8.RuneError {
			replacementCount++
			continue
		}
		if isMojibakeRune(r) {
			suspiciousCount++
		}
		if i+2 < len(runes) && isCommonMojibakeTriplet(runes[i], runes[i+1], runes[i+2]) {
			suspiciousCount += 3
		}
	}

	if replacementCount > 0 {
		return true
	}
	if suspiciousCount >= 3 {
		return true
	}
	return len(runes) >= 20 && suspiciousCount*100/len(runes) >= 10
}

func isMojibakeRune(r rune) bool {
	switch r {
	case 'Ã', 'Â', 'ð', 'Ð', 'Ñ', 'ä', 'å', 'æ', 'ç', 'è', 'é', 'ï':
		return true
	default:
		return false
	}
}

func isCommonMojibakeTriplet(a rune, b rune, c rune) bool {
	if !isMojibakeRune(a) {
		return false
	}
	return unicode.Is(unicode.Latin, b) && unicode.Is(unicode.Latin, c)
}

func markOriginalLogAsMojibake(record *RequestLogRecord) {
	if LOG_DB == nil {
		return
	}

	update := map[string]any{
		"type":    LogTypeError,
		"content": mojibakeErrorMessage,
	}
	if record.LogId > 0 {
		if err := LOG_DB.Model(&Log{}).Where("id = ?", record.LogId).Updates(update).Error; err != nil {
			common.SysError("failed to mark original log as mojibake by log id: " + err.Error())
		}
		return
	}
	if record.RequestId != "" {
		if err := LOG_DB.Model(&Log{}).Where("request_id = ?", record.RequestId).Updates(update).Error; err != nil {
			common.SysError("failed to mark original log as mojibake by request id: " + err.Error())
		}
	}
}

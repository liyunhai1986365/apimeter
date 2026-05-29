package model

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
)

const (
	ModelBillingPeriodDay   = "day"
	ModelBillingPeriodMonth = "month"
)

type ModelBillingRecord struct {
	Id                 int     `json:"id"`
	UserId             int     `json:"user_id" gorm:"index:idx_model_billing_user_period,priority:1;index"`
	TokenId            int     `json:"token_id" gorm:"index"`
	LogId              int     `json:"log_id" gorm:"uniqueIndex"`
	RequestId          string  `json:"request_id" gorm:"type:varchar(64);index;default:''"`
	ModelName          string  `json:"model_name" gorm:"type:varchar(191);index:idx_model_billing_model_period,priority:1;index"`
	BillingSource      string  `json:"billing_source" gorm:"type:varchar(32);index;default:'wallet'"`
	Group              string  `json:"group" gorm:"type:varchar(64);index;default:''"`
	GroupRatio         float64 `json:"group_ratio" gorm:"default:1"`
	BillingMode        string  `json:"billing_mode" gorm:"type:varchar(32);index;default:''"`
	CreatedAt          int64   `json:"created_at" gorm:"bigint;index"`
	BillDate           string  `json:"bill_date" gorm:"type:varchar(10);index:idx_model_billing_user_period,priority:2;index:idx_model_billing_model_period,priority:2"`
	BillMonth          string  `json:"bill_month" gorm:"type:varchar(7);index"`
	InputTokens        int64   `json:"input_tokens" gorm:"type:bigint;default:0"`
	OutputTokens       int64   `json:"output_tokens" gorm:"type:bigint;default:0"`
	CacheWriteTokens   int64   `json:"cache_write_tokens" gorm:"type:bigint;default:0"`
	CacheReadTokens    int64   `json:"cache_read_tokens" gorm:"type:bigint;default:0"`
	CacheWrite5mTokens int64   `json:"cache_write_5m_tokens" gorm:"type:bigint;default:0"`
	CacheWrite1hTokens int64   `json:"cache_write_1h_tokens" gorm:"type:bigint;default:0"`
	OriginalQuota      int64   `json:"original_quota" gorm:"type:bigint;default:0"`
	DiscountQuota      int64   `json:"discount_quota" gorm:"type:bigint;default:0"`
	PayableQuota       int64   `json:"payable_quota" gorm:"type:bigint;default:0"`
}

type ModelBillingSummaryQuery struct {
	UserId         int
	Period         string
	StartTimestamp int64
	EndTimestamp   int64
	ModelName      string
	Group          string
	BillingSource  string
}

type ModelBillingSummaryRow struct {
	Period           string `json:"period"`
	ModelName        string `json:"model_name"`
	RequestCount     int    `json:"request_count"`
	InputTokens      int64  `json:"input_tokens"`
	OutputTokens     int64  `json:"output_tokens"`
	CacheWriteTokens int64  `json:"cache_write_tokens"`
	CacheReadTokens  int64  `json:"cache_read_tokens"`
	OriginalQuota    int64  `json:"original_quota"`
	DiscountQuota    int64  `json:"discount_quota"`
	PayableQuota     int64  `json:"payable_quota"`
}

type ModelBillingBackfillOptions struct {
	StartTimestamp int64
	EndTimestamp   int64
	BatchSize      int
}

type ModelBillingBackfillResult struct {
	Scanned int64 `json:"scanned"`
	Created int64 `json:"created"`
	Skipped int64 `json:"skipped"`
	Failed  int64 `json:"failed"`
}

func normalizeModelBillingPeriod(period string) string {
	switch strings.TrimSpace(period) {
	case ModelBillingPeriodMonth:
		return ModelBillingPeriodMonth
	default:
		return ModelBillingPeriodDay
	}
}

func fillModelBillingPeriod(record *ModelBillingRecord) {
	if record == nil {
		return
	}
	if record.CreatedAt <= 0 {
		record.CreatedAt = time.Now().Unix()
	}
	t := time.Unix(record.CreatedAt, 0).UTC()
	if record.BillDate == "" {
		record.BillDate = t.Format("2006-01-02")
	}
	if record.BillMonth == "" {
		record.BillMonth = t.Format("2006-01")
	}
	if strings.TrimSpace(record.BillingSource) == "" {
		record.BillingSource = "wallet"
	}
	if record.GroupRatio == 0 {
		record.GroupRatio = 1
	}
}

func RecordModelBilling(record *ModelBillingRecord) error {
	if record == nil {
		return errors.New("model billing record is nil")
	}
	if strings.TrimSpace(record.ModelName) == "" {
		return errors.New("model name is empty")
	}
	fillModelBillingPeriod(record)
	return LOG_DB.Create(record).Error
}

func GetModelBillingSummary(query ModelBillingSummaryQuery) ([]ModelBillingSummaryRow, error) {
	period := normalizeModelBillingPeriod(query.Period)
	periodCol := "bill_date"
	if period == ModelBillingPeriodMonth {
		periodCol = "bill_month"
	}

	tx := LOG_DB.Model(&ModelBillingRecord{})
	if query.UserId > 0 {
		tx = tx.Where("user_id = ?", query.UserId)
	}
	if query.StartTimestamp > 0 {
		tx = tx.Where("created_at >= ?", query.StartTimestamp)
	}
	if query.EndTimestamp > 0 {
		tx = tx.Where("created_at <= ?", query.EndTimestamp)
	}
	if strings.TrimSpace(query.ModelName) != "" {
		tx = tx.Where("model_name = ?", strings.TrimSpace(query.ModelName))
	}
	if strings.TrimSpace(query.Group) != "" {
		tx = tx.Where(logGroupCol+" = ?", strings.TrimSpace(query.Group))
	}
	if strings.TrimSpace(query.BillingSource) != "" {
		tx = tx.Where("billing_source = ?", strings.TrimSpace(query.BillingSource))
	}

	var rows []ModelBillingSummaryRow
	err := tx.Select(periodCol + " AS period, model_name, COUNT(*) AS request_count, " +
		"COALESCE(SUM(input_tokens), 0) AS input_tokens, " +
		"COALESCE(SUM(output_tokens), 0) AS output_tokens, " +
		"COALESCE(SUM(cache_write_tokens), 0) AS cache_write_tokens, " +
		"COALESCE(SUM(cache_read_tokens), 0) AS cache_read_tokens, " +
		"COALESCE(SUM(original_quota), 0) AS original_quota, " +
		"COALESCE(SUM(discount_quota), 0) AS discount_quota, " +
		"COALESCE(SUM(payable_quota), 0) AS payable_quota").
		Group(periodCol + ", model_name").
		Order(periodCol + " asc, payable_quota desc, model_name asc").
		Scan(&rows).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return []ModelBillingSummaryRow{}, nil
	}
	return rows, err
}

func BackfillModelBillingFromLogs(options ModelBillingBackfillOptions) (ModelBillingBackfillResult, error) {
	if options.BatchSize <= 0 {
		options.BatchSize = 1000
	}
	if err := LOG_DB.AutoMigrate(&ModelBillingRecord{}); err != nil {
		return ModelBillingBackfillResult{}, err
	}

	var result ModelBillingBackfillResult
	lastID := 0
	for {
		var logs []Log
		tx := LOG_DB.Model(&Log{}).
			Where("id > ? AND type = ?", lastID, LogTypeConsume).
			Order("id asc").
			Limit(options.BatchSize)
		if options.StartTimestamp > 0 {
			tx = tx.Where("created_at >= ?", options.StartTimestamp)
		}
		if options.EndTimestamp > 0 {
			tx = tx.Where("created_at <= ?", options.EndTimestamp)
		}
		if err := tx.Find(&logs).Error; err != nil {
			return result, err
		}
		if len(logs) == 0 {
			break
		}

		for i := range logs {
			log := logs[i]
			lastID = log.Id
			result.Scanned++
			created, err := backfillModelBillingFromLog(&log)
			if err != nil {
				result.Failed++
				continue
			}
			if created {
				result.Created++
			} else {
				result.Skipped++
			}
		}
		if len(logs) < options.BatchSize {
			break
		}
	}
	return result, nil
}

func backfillModelBillingFromLog(log *Log) (bool, error) {
	if log == nil || log.Id <= 0 || strings.TrimSpace(log.ModelName) == "" {
		return false, nil
	}
	var count int64
	if err := LOG_DB.Model(&ModelBillingRecord{}).Where("log_id = ?", log.Id).Count(&count).Error; err != nil {
		return false, err
	}
	if count > 0 {
		return false, nil
	}

	other, _ := common.StrToMap(log.Other)
	groupRatio := modelBillingFloat(other, "group_ratio", 1)
	if groupRatio == 0 {
		groupRatio = 1
	}
	payableQuota := int64(log.Quota)
	originalQuota := payableQuota
	if groupRatio > 0 {
		originalQuota = int64(math.Round(float64(payableQuota) / groupRatio))
	}
	cacheReadTokens := int64(modelBillingInt(other, "cache_tokens", 0))
	cacheWriteTokens := int64(modelBillingInt(other, "cache_write_tokens", 0))
	cacheWrite5mTokens := int64(modelBillingInt(other, "cache_creation_tokens_5m", 0))
	cacheWrite1hTokens := int64(modelBillingInt(other, "cache_creation_tokens_1h", 0))
	if cacheWriteTokens == 0 {
		cacheWriteTokens = int64(modelBillingInt(other, "cache_creation_tokens", 0))
		if cacheWrite5mTokens+cacheWrite1hTokens > cacheWriteTokens {
			cacheWriteTokens = cacheWrite5mTokens + cacheWrite1hTokens
		}
	}

	inputTokens := int64(log.PromptTokens)
	if modelBillingBool(other, "claude", false) {
		inputTokens = int64(log.PromptTokens)
	} else if totalInput := modelBillingInt(other, "input_tokens_total", 0); totalInput > 0 {
		inputTokens = int64(totalInput) - cacheReadTokens - cacheWriteTokens
	} else {
		inputTokens = int64(log.PromptTokens) - cacheReadTokens - cacheWriteTokens
	}
	if textInput := modelBillingInt(other, "text_input", 0); textInput > 0 {
		inputTokens = int64(textInput)
	}
	if inputTokens < 0 {
		inputTokens = 0
	}

	outputTokens := int64(log.CompletionTokens)
	if textOutput := modelBillingInt(other, "text_output", 0); textOutput > 0 {
		outputTokens = int64(textOutput)
	}

	record := &ModelBillingRecord{
		UserId:             log.UserId,
		TokenId:            log.TokenId,
		LogId:              log.Id,
		RequestId:          log.RequestId,
		ModelName:          log.ModelName,
		BillingSource:      modelBillingString(other, "billing_source", "wallet"),
		Group:              log.Group,
		GroupRatio:         groupRatio,
		BillingMode:        modelBillingString(other, "billing_mode", "ratio"),
		CreatedAt:          log.CreatedAt,
		InputTokens:        inputTokens,
		OutputTokens:       outputTokens,
		CacheWriteTokens:   cacheWriteTokens,
		CacheReadTokens:    cacheReadTokens,
		CacheWrite5mTokens: cacheWrite5mTokens,
		CacheWrite1hTokens: cacheWrite1hTokens,
		OriginalQuota:      originalQuota,
		DiscountQuota:      originalQuota - payableQuota,
		PayableQuota:       payableQuota,
	}
	if err := RecordModelBilling(record); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") || strings.Contains(strings.ToLower(err.Error()), "duplicate") {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func modelBillingString(values map[string]interface{}, key string, fallback string) string {
	if values == nil {
		return fallback
	}
	if value, ok := values[key].(string); ok && strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return fallback
}

func modelBillingBool(values map[string]interface{}, key string, fallback bool) bool {
	if values == nil {
		return fallback
	}
	if value, ok := values[key].(bool); ok {
		return value
	}
	return fallback
}

func modelBillingFloat(values map[string]interface{}, key string, fallback float64) float64 {
	if values == nil {
		return fallback
	}
	switch value := values[key].(type) {
	case float64:
		return value
	case float32:
		return float64(value)
	case int:
		return float64(value)
	case int64:
		return float64(value)
	case string:
		var parsed float64
		if _, err := fmt.Sscanf(value, "%f", &parsed); err == nil {
			return parsed
		}
	}
	return fallback
}

func modelBillingInt(values map[string]interface{}, key string, fallback int) int {
	if values == nil {
		return fallback
	}
	switch value := values[key].(type) {
	case float64:
		return int(value)
	case float32:
		return int(value)
	case int:
		return value
	case int64:
		return int(value)
	case string:
		var parsed int
		if _, err := fmt.Sscanf(value, "%d", &parsed); err == nil {
			return parsed
		}
	}
	return fallback
}

package model

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

const (
	BillingSourceWallet       = "wallet"
	BillingSourceSubscription = "subscription"

	AccountLedgerEntryTypeTopup      = "topup"
	AccountLedgerEntryTypeConsume    = "consume"
	AccountLedgerEntryTypeRefund     = "refund"
	AccountLedgerEntryTypeAdjustment = "adjustment"

	AccountLedgerSourceUsage      = "billing_usage"
	AccountLedgerSourceLog        = "log"
	AccountLedgerSourceAdjustment = "adjustment"

	BillingStatementPeriodDay   = "day"
	BillingStatementPeriodMonth = "month"

	BillingStatementStatusOpen      = "open"
	BillingStatementStatusConfirmed = "confirmed"
	BillingStatementStatusException = "exception"

	BillingStatementSummaryDimensionModel  = "model"
	BillingStatementSummaryDimensionGroup  = "group"
	BillingStatementSummaryDimensionKey    = "key"
	BillingStatementSummaryDimensionSource = "source"
)

type BillingUsageItem struct {
	Id               int     `json:"id"`
	UserId           int     `json:"user_id" gorm:"index:idx_billing_usage_user_time,priority:1;index"`
	TokenId          int     `json:"token_id" gorm:"index"`
	TokenName        string  `json:"token_name" gorm:"type:varchar(191);index;default:''"`
	WorkspaceId      int     `json:"workspace_id" gorm:"index;default:0"`
	LogId            int     `json:"log_id" gorm:"uniqueIndex"`
	RequestId        string  `json:"request_id" gorm:"type:varchar(64);index;default:''"`
	ModelName        string  `json:"model_name" gorm:"type:varchar(191);index;default:''"`
	Group            string  `json:"group" gorm:"type:varchar(64);index;default:''"`
	BillingSource    string  `json:"billing_source" gorm:"type:varchar(32);index;default:'wallet'"`
	BillingMode      string  `json:"billing_mode" gorm:"type:varchar(32);index;default:''"`
	GroupRatio       float64 `json:"group_ratio" gorm:"default:1"`
	PricingVersion   string  `json:"pricing_version" gorm:"type:varchar(64);default:''"`
	BilledAt         int64   `json:"billed_at" gorm:"bigint;index:idx_billing_usage_user_time,priority:2"`
	BillingDate      string  `json:"billing_date" gorm:"type:varchar(10);index"`
	BillingMonth     string  `json:"billing_month" gorm:"type:varchar(7);index"`
	InputTokens      int64   `json:"input_tokens" gorm:"type:bigint;default:0"`
	OutputTokens     int64   `json:"output_tokens" gorm:"type:bigint;default:0"`
	CacheReadTokens  int64   `json:"cache_read_tokens" gorm:"type:bigint;default:0"`
	CacheWriteTokens int64   `json:"cache_write_tokens" gorm:"type:bigint;default:0"`
	BillableUnits    int64   `json:"billable_units" gorm:"type:bigint;default:0"`
	OriginalAmount   int64   `json:"original_amount" gorm:"type:bigint;default:0"`
	DiscountAmount   int64   `json:"discount_amount" gorm:"type:bigint;default:0"`
	SettlementAmount int64   `json:"settlement_amount" gorm:"type:bigint;default:0"`
	PriceSnapshot    string  `json:"price_snapshot" gorm:"type:text"`
	ReconciledStatus string  `json:"reconciled_status" gorm:"type:varchar(32);default:'pending'"`
}

type AccountLedgerEntry struct {
	Id            int    `json:"id"`
	UserId        int    `json:"user_id" gorm:"index:idx_account_ledger_user_time,priority:1;index"`
	AccountType   string `json:"account_type" gorm:"type:varchar(32);index;default:'wallet'"`
	EntryType     string `json:"entry_type" gorm:"type:varchar(32);index"`
	Amount        int64  `json:"amount" gorm:"type:bigint;default:0"`
	BalanceBefore int64  `json:"balance_before" gorm:"type:bigint;default:0"`
	BalanceAfter  int64  `json:"balance_after" gorm:"type:bigint;default:0"`
	SourceType    string `json:"source_type" gorm:"type:varchar(64);index;default:''"`
	SourceId      string `json:"source_id" gorm:"type:varchar(64);index;default:''"`
	LogId         int    `json:"log_id" gorm:"index"`
	RequestId     string `json:"request_id" gorm:"type:varchar(64);index;default:''"`
	ModelName     string `json:"model_name" gorm:"type:varchar(191);default:''"`
	TokenId       int    `json:"token_id" gorm:"index;default:0"`
	TokenName     string `json:"token_name" gorm:"type:varchar(191);default:''"`
	OccurredAt    int64  `json:"occurred_at" gorm:"bigint;index:idx_account_ledger_user_time,priority:2"`
	LedgerDate    string `json:"ledger_date" gorm:"type:varchar(10);index"`
	LedgerMonth   string `json:"ledger_month" gorm:"type:varchar(7);index"`
	Content       string `json:"content" gorm:"type:text"`
}

type BillingStatement struct {
	Id               int    `json:"id"`
	StatementNo      string `json:"statement_no" gorm:"type:varchar(64);uniqueIndex"`
	UserId           int    `json:"user_id" gorm:"index:idx_billing_statement_user_period,priority:1;index"`
	Period           string `json:"period" gorm:"type:varchar(16);index"`
	PeriodValue      string `json:"period_value" gorm:"type:varchar(16);index:idx_billing_statement_user_period,priority:2"`
	PeriodStart      int64  `json:"period_start" gorm:"bigint;index"`
	PeriodEnd        int64  `json:"period_end" gorm:"bigint;index"`
	OpeningBalance   int64  `json:"opening_balance" gorm:"type:bigint;default:0"`
	ClosingBalance   int64  `json:"closing_balance" gorm:"type:bigint;default:0"`
	TopupAmount      int64  `json:"topup_amount" gorm:"type:bigint;default:0"`
	ConsumeAmount    int64  `json:"consume_amount" gorm:"type:bigint;default:0"`
	RefundAmount     int64  `json:"refund_amount" gorm:"type:bigint;default:0"`
	AdjustmentAmount int64  `json:"adjustment_amount" gorm:"type:bigint;default:0"`
	RequestCount     int64  `json:"request_count" gorm:"type:bigint;default:0"`
	InputTokens      int64  `json:"input_tokens" gorm:"type:bigint;default:0"`
	OutputTokens     int64  `json:"output_tokens" gorm:"type:bigint;default:0"`
	CacheReadTokens  int64  `json:"cache_read_tokens" gorm:"type:bigint;default:0"`
	CacheWriteTokens int64  `json:"cache_write_tokens" gorm:"type:bigint;default:0"`
	OriginalAmount   int64  `json:"original_amount" gorm:"type:bigint;default:0"`
	DiscountAmount   int64  `json:"discount_amount" gorm:"type:bigint;default:0"`
	SettlementAmount int64  `json:"settlement_amount" gorm:"type:bigint;default:0"`
	DifferenceAmount int64  `json:"difference_amount" gorm:"type:bigint;default:0"`
	Status           string `json:"status" gorm:"type:varchar(32);index;default:'open'"`
	GeneratedAt      int64  `json:"generated_at" gorm:"bigint;index"`
	FinalizedAt      int64  `json:"finalized_at" gorm:"bigint;default:0"`
	ExceptionCount   int    `json:"exception_count" gorm:"default:0"`
}

type BillingStatementSummary struct {
	Id               int    `json:"id"`
	StatementNo      string `json:"statement_no" gorm:"type:varchar(64);index"`
	UserId           int    `json:"user_id" gorm:"index"`
	Period           string `json:"period" gorm:"type:varchar(16);index"`
	PeriodValue      string `json:"period_value" gorm:"type:varchar(16);index"`
	Dimension        string `json:"dimension" gorm:"type:varchar(32);index"`
	DimensionValue   string `json:"dimension_value" gorm:"type:varchar(191);index"`
	RequestCount     int64  `json:"request_count" gorm:"type:bigint;default:0"`
	InputTokens      int64  `json:"input_tokens" gorm:"type:bigint;default:0"`
	OutputTokens     int64  `json:"output_tokens" gorm:"type:bigint;default:0"`
	CacheReadTokens  int64  `json:"cache_read_tokens" gorm:"type:bigint;default:0"`
	CacheWriteTokens int64  `json:"cache_write_tokens" gorm:"type:bigint;default:0"`
	OriginalAmount   int64  `json:"original_amount" gorm:"type:bigint;default:0"`
	DiscountAmount   int64  `json:"discount_amount" gorm:"type:bigint;default:0"`
	SettlementAmount int64  `json:"settlement_amount" gorm:"type:bigint;default:0"`
}

type BillingV2BackfillOptions struct {
	StartTimestamp int64
	EndTimestamp   int64
	BatchSize      int
}

type BillingV2BackfillResult struct {
	Scanned       int64 `json:"scanned"`
	UsageCreated  int64 `json:"usage_created"`
	LedgerCreated int64 `json:"ledger_created"`
	Skipped       int64 `json:"skipped"`
	Failed        int64 `json:"failed"`
}

type BillingUsageItemQuery struct {
	UserId       int
	StartTime    int64
	EndTime      int64
	BillingMonth string
	ModelName    string
	Group        string
	TokenName    string
	RequestId    string
	Limit        int
	Offset       int
}

type AccountLedgerQuery struct {
	UserId      int
	StartTime   int64
	EndTime     int64
	LedgerMonth string
	EntryType   string
	Limit       int
	Offset      int
}

type BillingStatementQuery struct {
	UserId     int
	StartMonth string
	EndMonth   string
	Period     string
	Limit      int
	Offset     int
}

type DailyBillingReconciliation struct {
	Date                  string `json:"date"`
	UsageSettlementAmount int64  `json:"usage_settlement_amount"`
	AccountConsumeAmount  int64  `json:"account_consume_amount"`
	DifferenceAmount      int64  `json:"difference_amount"`
	RequestCount          int64  `json:"request_count"`
	Status                string `json:"status"`
}

type BillingUsageTotals struct {
	RequestCount     int64 `json:"request_count"`
	InputTokens      int64 `json:"input_tokens"`
	OutputTokens     int64 `json:"output_tokens"`
	CacheReadTokens  int64 `json:"cache_read_tokens"`
	CacheWriteTokens int64 `json:"cache_write_tokens"`
	OriginalAmount   int64 `json:"original_amount"`
	DiscountAmount   int64 `json:"discount_amount"`
	SettlementAmount int64 `json:"settlement_amount"`
}

func fillBillingUsagePeriod(item *BillingUsageItem) {
	if item.BilledAt <= 0 {
		item.BilledAt = time.Now().Unix()
	}
	t := time.Unix(item.BilledAt, 0).UTC()
	if item.BillingDate == "" {
		item.BillingDate = t.Format("2006-01-02")
	}
	if item.BillingMonth == "" {
		item.BillingMonth = t.Format("2006-01")
	}
	if strings.TrimSpace(item.BillingSource) == "" {
		item.BillingSource = BillingSourceWallet
	}
	if item.GroupRatio == 0 {
		item.GroupRatio = 1
	}
	if item.BillableUnits == 0 {
		item.BillableUnits = item.InputTokens + item.OutputTokens + item.CacheReadTokens + item.CacheWriteTokens
	}
}

func RecordBillingUsageItem(item *BillingUsageItem) error {
	if item == nil {
		return errors.New("billing usage item is nil")
	}
	if item.UserId <= 0 {
		return errors.New("user id is required")
	}
	fillBillingUsagePeriod(item)
	return LOG_DB.Create(item).Error
}

func fillAccountLedgerPeriod(entry *AccountLedgerEntry) {
	if entry.OccurredAt <= 0 {
		entry.OccurredAt = time.Now().Unix()
	}
	t := time.Unix(entry.OccurredAt, 0).UTC()
	if entry.LedgerDate == "" {
		entry.LedgerDate = t.Format("2006-01-02")
	}
	if entry.LedgerMonth == "" {
		entry.LedgerMonth = t.Format("2006-01")
	}
	if strings.TrimSpace(entry.AccountType) == "" {
		entry.AccountType = BillingSourceWallet
	}
}

func RecordAccountLedgerEntry(entry *AccountLedgerEntry) error {
	if entry == nil {
		return errors.New("account ledger entry is nil")
	}
	if entry.UserId <= 0 {
		return errors.New("user id is required")
	}
	fillAccountLedgerPeriod(entry)
	return LOG_DB.Create(entry).Error
}

func BackfillBillingV2FromLogs(options BillingV2BackfillOptions) (BillingV2BackfillResult, error) {
	if options.BatchSize <= 0 {
		options.BatchSize = 1000
	}
	if err := LOG_DB.AutoMigrate(&BillingUsageItem{}, &AccountLedgerEntry{}, &BillingStatement{}, &BillingStatementSummary{}); err != nil {
		return BillingV2BackfillResult{}, err
	}

	var result BillingV2BackfillResult
	lastID := 0
	balances := map[string]int64{}
	for {
		var logs []Log
		tx := LOG_DB.Model(&Log{}).
			Where("id > ?", lastID).
			Where("type IN ?", []int{LogTypeTopup, LogTypeConsume, LogTypeRefund, LogTypeManage}).
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
			if _, err := backfillBillingV2Log(&log, balances, &result); err != nil {
				result.Failed++
			}
		}
		if len(logs) < options.BatchSize {
			break
		}
	}
	return result, nil
}

func backfillBillingV2Log(log *Log, balances map[string]int64, result *BillingV2BackfillResult) (bool, error) {
	var ledgerCount int64
	if err := LOG_DB.Model(&AccountLedgerEntry{}).Where("log_id = ?", log.Id).Count(&ledgerCount).Error; err != nil {
		return false, err
	}
	if ledgerCount > 0 {
		result.Skipped++
		return false, nil
	}

	lineType, amount, ok := billingLogAmount(*log)
	if !ok {
		result.Skipped++
		return false, nil
	}
	accountType := BillingSourceWallet
	other, _ := common.StrToMap(log.Other)
	if log.Type == LogTypeConsume {
		accountType = billingValueString(other, "billing_source", BillingSourceWallet)
	}
	key := fmt.Sprintf("%d:%s", log.UserId, accountType)
	before := balances[key]
	after := before + amount
	balances[key] = after
	ledger := &AccountLedgerEntry{
		UserId:        log.UserId,
		AccountType:   accountType,
		EntryType:     lineType,
		Amount:        amount,
		BalanceBefore: before,
		BalanceAfter:  after,
		SourceType:    AccountLedgerSourceLog,
		SourceId:      strconv.Itoa(log.Id),
		LogId:         log.Id,
		RequestId:     log.RequestId,
		ModelName:     log.ModelName,
		TokenId:       log.TokenId,
		TokenName:     log.TokenName,
		OccurredAt:    log.CreatedAt,
		Content:       log.Content,
	}
	if log.Type == LogTypeConsume {
		ledger.SourceType = AccountLedgerSourceUsage
	}
	if err := RecordAccountLedgerEntry(ledger); err != nil {
		return false, err
	}
	result.LedgerCreated++

	if log.Type != LogTypeConsume {
		return true, nil
	}
	var usageCount int64
	if err := LOG_DB.Model(&BillingUsageItem{}).Where("log_id = ?", log.Id).Count(&usageCount).Error; err != nil {
		return true, err
	}
	if usageCount > 0 {
		return true, nil
	}
	usage := billingUsageItemFromLog(log, other)
	if err := RecordBillingUsageItem(&usage); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") || strings.Contains(strings.ToLower(err.Error()), "duplicate") {
			return true, nil
		}
		return true, err
	}
	result.UsageCreated++
	return true, nil
}

func RecordBillingV2ConsumeLog(logId int) (bool, error) {
	if logId <= 0 {
		return false, nil
	}
	var log Log
	if err := LOG_DB.Where("id = ? AND type = ?", logId, LogTypeConsume).First(&log).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}
	var usageCount int64
	if err := LOG_DB.Model(&BillingUsageItem{}).Where("log_id = ?", log.Id).Count(&usageCount).Error; err != nil {
		return false, err
	}
	var ledgerCount int64
	if err := LOG_DB.Model(&AccountLedgerEntry{}).Where("log_id = ?", log.Id).Count(&ledgerCount).Error; err != nil {
		return false, err
	}
	if usageCount > 0 && ledgerCount > 0 {
		return false, nil
	}
	other, _ := common.StrToMap(log.Other)
	usage := billingUsageItemFromLog(&log, other)
	accountType := usage.BillingSource
	if strings.TrimSpace(accountType) == "" {
		accountType = BillingSourceWallet
	}
	created := false
	err := LOG_DB.Transaction(func(tx *gorm.DB) error {
		if usageCount == 0 {
			fillBillingUsagePeriod(&usage)
			if err := tx.Create(&usage).Error; err != nil {
				if strings.Contains(strings.ToLower(err.Error()), "unique") || strings.Contains(strings.ToLower(err.Error()), "duplicate") {
					return nil
				}
				return err
			}
			created = true
		}
		if ledgerCount > 0 {
			return nil
		}
		before, err := latestAccountLedgerBalanceTx(tx, log.UserId, accountType)
		if err != nil {
			return err
		}
		amount := -int64(log.Quota)
		ledger := &AccountLedgerEntry{
			UserId:        log.UserId,
			AccountType:   accountType,
			EntryType:     AccountLedgerEntryTypeConsume,
			Amount:        amount,
			BalanceBefore: before,
			BalanceAfter:  before + amount,
			SourceType:    AccountLedgerSourceUsage,
			SourceId:      strconv.Itoa(log.Id),
			LogId:         log.Id,
			RequestId:     log.RequestId,
			ModelName:     log.ModelName,
			TokenId:       log.TokenId,
			TokenName:     log.TokenName,
			OccurredAt:    log.CreatedAt,
			Content:       log.Content,
		}
		fillAccountLedgerPeriod(ledger)
		if err := tx.Create(ledger).Error; err != nil {
			return err
		}
		created = true
		return nil
	})
	return created, err
}

func latestAccountLedgerBalanceTx(tx *gorm.DB, userId int, accountType string) (int64, error) {
	var latest AccountLedgerEntry
	err := tx.Model(&AccountLedgerEntry{}).
		Where("user_id = ? AND account_type = ?", userId, accountType).
		Order("occurred_at desc, id desc").
		First(&latest).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return latest.BalanceAfter, nil
}

func billingUsageItemFromLog(log *Log, other map[string]interface{}) BillingUsageItem {
	groupRatio := billingValueFloat(other, "group_ratio", 1)
	if groupRatio == 0 {
		groupRatio = 1
	}
	settlement := int64(log.Quota)
	original := settlement
	if groupRatio > 0 {
		original = int64(math.Round(float64(settlement) / groupRatio))
	}
	cacheRead := int64(billingValueInt(other, "cache_tokens", 0))
	cacheWrite := int64(billingValueInt(other, "cache_write_tokens", 0))
	input := int64(log.PromptTokens) - cacheRead - cacheWrite
	if inputTotal := billingValueInt(other, "input_tokens_total", 0); inputTotal > 0 {
		input = int64(inputTotal) - cacheRead - cacheWrite
	}
	if textInput := billingValueInt(other, "text_input", 0); textInput > 0 {
		input = int64(textInput)
	}
	if input < 0 {
		input = 0
	}
	output := int64(log.CompletionTokens)
	if textOutput := billingValueInt(other, "text_output", 0); textOutput > 0 {
		output = int64(textOutput)
	}
	return BillingUsageItem{
		UserId:           log.UserId,
		TokenId:          log.TokenId,
		TokenName:        log.TokenName,
		LogId:            log.Id,
		RequestId:        log.RequestId,
		ModelName:        log.ModelName,
		Group:            log.Group,
		BillingSource:    billingValueString(other, "billing_source", BillingSourceWallet),
		BillingMode:      billingValueString(other, "billing_mode", "ratio"),
		GroupRatio:       groupRatio,
		BilledAt:         log.CreatedAt,
		InputTokens:      input,
		OutputTokens:     output,
		CacheReadTokens:  cacheRead,
		CacheWriteTokens: cacheWrite,
		OriginalAmount:   original,
		DiscountAmount:   original - settlement,
		SettlementAmount: settlement,
		PriceSnapshot:    log.Other,
	}
}

func GetBillingUsageItems(query BillingUsageItemQuery) ([]BillingUsageItem, error) {
	limit := query.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	tx := LOG_DB.Model(&BillingUsageItem{})
	if query.UserId > 0 {
		tx = tx.Where("user_id = ?", query.UserId)
	}
	if query.StartTime > 0 {
		tx = tx.Where("billed_at >= ?", query.StartTime)
	}
	if query.EndTime > 0 {
		tx = tx.Where("billed_at <= ?", query.EndTime)
	}
	if strings.TrimSpace(query.BillingMonth) != "" {
		tx = tx.Where("billing_month = ?", strings.TrimSpace(query.BillingMonth))
	}
	if strings.TrimSpace(query.ModelName) != "" {
		tx = tx.Where("model_name = ?", strings.TrimSpace(query.ModelName))
	}
	if strings.TrimSpace(query.Group) != "" {
		tx = tx.Where(logGroupCol+" = ?", strings.TrimSpace(query.Group))
	}
	if strings.TrimSpace(query.TokenName) != "" {
		tx = tx.Where("token_name = ?", strings.TrimSpace(query.TokenName))
	}
	if strings.TrimSpace(query.RequestId) != "" {
		tx = tx.Where("request_id = ?", strings.TrimSpace(query.RequestId))
	}
	var rows []BillingUsageItem
	err := tx.Order("billed_at desc, id desc").Limit(limit).Offset(query.Offset).Find(&rows).Error
	return rows, err
}

func GetBillingUsageTotals(query BillingUsageItemQuery) (BillingUsageTotals, error) {
	tx := LOG_DB.Model(&BillingUsageItem{})
	if query.UserId > 0 {
		tx = tx.Where("user_id = ?", query.UserId)
	}
	if query.StartTime > 0 {
		tx = tx.Where("billed_at >= ?", query.StartTime)
	}
	if query.EndTime > 0 {
		tx = tx.Where("billed_at <= ?", query.EndTime)
	}
	if strings.TrimSpace(query.BillingMonth) != "" {
		tx = tx.Where("billing_month = ?", strings.TrimSpace(query.BillingMonth))
	}
	if strings.TrimSpace(query.ModelName) != "" {
		tx = tx.Where("model_name = ?", strings.TrimSpace(query.ModelName))
	}
	if strings.TrimSpace(query.Group) != "" {
		tx = tx.Where(logGroupCol+" = ?", strings.TrimSpace(query.Group))
	}
	if strings.TrimSpace(query.TokenName) != "" {
		tx = tx.Where("token_name = ?", strings.TrimSpace(query.TokenName))
	}
	if strings.TrimSpace(query.RequestId) != "" {
		tx = tx.Where("request_id = ?", strings.TrimSpace(query.RequestId))
	}
	var totals BillingUsageTotals
	err := tx.Select("COUNT(*) AS request_count, COALESCE(SUM(input_tokens),0) AS input_tokens, COALESCE(SUM(output_tokens),0) AS output_tokens, COALESCE(SUM(cache_read_tokens),0) AS cache_read_tokens, COALESCE(SUM(cache_write_tokens),0) AS cache_write_tokens, COALESCE(SUM(original_amount),0) AS original_amount, COALESCE(SUM(discount_amount),0) AS discount_amount, COALESCE(SUM(settlement_amount),0) AS settlement_amount").
		Scan(&totals).Error
	return totals, err
}

func GetAccountLedgerEntries(query AccountLedgerQuery) ([]AccountLedgerEntry, error) {
	limit := query.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	tx := LOG_DB.Model(&AccountLedgerEntry{})
	if query.UserId > 0 {
		tx = tx.Where("user_id = ?", query.UserId)
	}
	if query.StartTime > 0 {
		tx = tx.Where("occurred_at >= ?", query.StartTime)
	}
	if query.EndTime > 0 {
		tx = tx.Where("occurred_at <= ?", query.EndTime)
	}
	if strings.TrimSpace(query.LedgerMonth) != "" {
		tx = tx.Where("ledger_month = ?", strings.TrimSpace(query.LedgerMonth))
	}
	if strings.TrimSpace(query.EntryType) != "" {
		tx = tx.Where("entry_type = ?", strings.TrimSpace(query.EntryType))
	}
	var rows []AccountLedgerEntry
	err := tx.Order("occurred_at asc, id asc").Limit(limit).Offset(query.Offset).Find(&rows).Error
	return rows, err
}

func GetDailyBillingReconciliations(query BillingStatementQuery) ([]DailyBillingReconciliation, error) {
	usage := LOG_DB.Model(&BillingUsageItem{})
	ledger := LOG_DB.Model(&AccountLedgerEntry{})
	if query.UserId > 0 {
		usage = usage.Where("user_id = ?", query.UserId)
		ledger = ledger.Where("user_id = ?", query.UserId)
	}
	if strings.TrimSpace(query.StartMonth) != "" {
		usage = usage.Where("billing_month >= ?", query.StartMonth)
		ledger = ledger.Where("ledger_month >= ?", query.StartMonth)
	}
	if strings.TrimSpace(query.EndMonth) != "" {
		usage = usage.Where("billing_month <= ?", query.EndMonth)
		ledger = ledger.Where("ledger_month <= ?", query.EndMonth)
	}
	type usageRow struct {
		Date   string
		Amount int64
		Count  int64
	}
	type ledgerRow struct {
		Date   string
		Amount int64
	}
	var usageRows []usageRow
	var ledgerRows []ledgerRow
	if err := usage.Select("billing_date AS date, COALESCE(SUM(settlement_amount), 0) AS amount, COUNT(*) AS count").
		Group("billing_date").Scan(&usageRows).Error; err != nil {
		return nil, err
	}
	if err := ledger.Where("entry_type = ?", AccountLedgerEntryTypeConsume).
		Select("ledger_date AS date, COALESCE(SUM(ABS(amount)), 0) AS amount").
		Group("ledger_date").Scan(&ledgerRows).Error; err != nil {
		return nil, err
	}
	byDate := map[string]*DailyBillingReconciliation{}
	for _, row := range usageRows {
		byDate[row.Date] = &DailyBillingReconciliation{
			Date:                  row.Date,
			UsageSettlementAmount: row.Amount,
			RequestCount:          row.Count,
		}
	}
	for _, row := range ledgerRows {
		item := byDate[row.Date]
		if item == nil {
			item = &DailyBillingReconciliation{Date: row.Date}
			byDate[row.Date] = item
		}
		item.AccountConsumeAmount = row.Amount
	}
	rows := make([]DailyBillingReconciliation, 0, len(byDate))
	for _, row := range byDate {
		row.DifferenceAmount = row.AccountConsumeAmount - row.UsageSettlementAmount
		if row.DifferenceAmount == 0 {
			row.Status = BillingStatementStatusConfirmed
		} else {
			row.Status = BillingStatementStatusException
		}
		rows = append(rows, *row)
	}
	return rows, nil
}

func GenerateMonthlyBillingStatement(userId int, month string) (BillingStatement, []BillingStatementSummary, error) {
	monthTime, err := time.ParseInLocation("2006-01", strings.TrimSpace(month), time.UTC)
	if err != nil {
		return BillingStatement{}, nil, err
	}
	start := monthTime.Unix()
	end := monthTime.AddDate(0, 1, 0).Add(-time.Second).Unix()
	statementNo := fmt.Sprintf("BILL-%s-%d", strings.ReplaceAll(month, "-", ""), userId)

	var usageTotals struct {
		RequestCount     int64
		InputTokens      int64
		OutputTokens     int64
		CacheReadTokens  int64
		CacheWriteTokens int64
		OriginalAmount   int64
		DiscountAmount   int64
		SettlementAmount int64
	}
	if err := LOG_DB.Model(&BillingUsageItem{}).
		Where("user_id = ? AND billing_month = ?", userId, month).
		Select("COUNT(*) AS request_count, COALESCE(SUM(input_tokens),0) AS input_tokens, COALESCE(SUM(output_tokens),0) AS output_tokens, COALESCE(SUM(cache_read_tokens),0) AS cache_read_tokens, COALESCE(SUM(cache_write_tokens),0) AS cache_write_tokens, COALESCE(SUM(original_amount),0) AS original_amount, COALESCE(SUM(discount_amount),0) AS discount_amount, COALESCE(SUM(settlement_amount),0) AS settlement_amount").
		Scan(&usageTotals).Error; err != nil {
		return BillingStatement{}, nil, err
	}
	var ledgerTotals []struct {
		EntryType string
		Amount    int64
	}
	if err := LOG_DB.Model(&AccountLedgerEntry{}).
		Where("user_id = ? AND ledger_month = ?", userId, month).
		Select("entry_type, COALESCE(SUM(amount), 0) AS amount").
		Group("entry_type").
		Scan(&ledgerTotals).Error; err != nil {
		return BillingStatement{}, nil, err
	}
	statement := BillingStatement{
		StatementNo:      statementNo,
		UserId:           userId,
		Period:           BillingStatementPeriodMonth,
		PeriodValue:      month,
		PeriodStart:      start,
		PeriodEnd:        end,
		RequestCount:     usageTotals.RequestCount,
		InputTokens:      usageTotals.InputTokens,
		OutputTokens:     usageTotals.OutputTokens,
		CacheReadTokens:  usageTotals.CacheReadTokens,
		CacheWriteTokens: usageTotals.CacheWriteTokens,
		OriginalAmount:   usageTotals.OriginalAmount,
		DiscountAmount:   usageTotals.DiscountAmount,
		SettlementAmount: usageTotals.SettlementAmount,
		GeneratedAt:      time.Now().Unix(),
	}
	for _, row := range ledgerTotals {
		switch row.EntryType {
		case AccountLedgerEntryTypeTopup:
			statement.TopupAmount = row.Amount
		case AccountLedgerEntryTypeConsume:
			statement.ConsumeAmount = -row.Amount
		case AccountLedgerEntryTypeRefund:
			statement.RefundAmount = row.Amount
		case AccountLedgerEntryTypeAdjustment:
			statement.AdjustmentAmount = row.Amount
		}
	}
	statement.DifferenceAmount = statement.ConsumeAmount - statement.SettlementAmount
	if statement.DifferenceAmount == 0 {
		statement.Status = BillingStatementStatusConfirmed
		statement.FinalizedAt = statement.GeneratedAt
	} else {
		statement.Status = BillingStatementStatusException
		statement.ExceptionCount = 1
	}
	summaries, err := buildBillingStatementSummaries(statement)
	if err != nil {
		return BillingStatement{}, nil, err
	}
	err = LOG_DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("statement_no = ?", statementNo).Delete(&BillingStatementSummary{}).Error; err != nil {
			return err
		}
		var existing BillingStatement
		err := tx.Where("statement_no = ?", statementNo).First(&existing).Error
		if err != nil && err != gorm.ErrRecordNotFound {
			return err
		}
		if existing.Id > 0 {
			statement.Id = existing.Id
			if err := tx.Save(&statement).Error; err != nil {
				return err
			}
		} else if err := tx.Create(&statement).Error; err != nil {
			return err
		}
		if len(summaries) > 0 {
			if err := tx.Create(&summaries).Error; err != nil {
				return err
			}
		}
		return nil
	})
	return statement, summaries, err
}

func buildBillingStatementSummaries(statement BillingStatement) ([]BillingStatementSummary, error) {
	var items []BillingUsageItem
	if err := LOG_DB.Model(&BillingUsageItem{}).
		Where("user_id = ? AND billing_month = ?", statement.UserId, statement.PeriodValue).
		Find(&items).Error; err != nil {
		return nil, err
	}
	summaries := make([]BillingStatementSummary, 0, len(items)*4)
	collectors := map[string]map[string]*BillingStatementSummary{
		BillingStatementSummaryDimensionModel:  {},
		BillingStatementSummaryDimensionGroup:  {},
		BillingStatementSummaryDimensionKey:    {},
		BillingStatementSummaryDimensionSource: {},
	}
	for _, item := range items {
		values := map[string]string{
			BillingStatementSummaryDimensionModel:  item.ModelName,
			BillingStatementSummaryDimensionGroup:  item.Group,
			BillingStatementSummaryDimensionKey:    item.TokenName,
			BillingStatementSummaryDimensionSource: item.BillingSource,
		}
		for dimension, value := range values {
			if value == "" {
				value = "-"
			}
			row := collectors[dimension][value]
			if row == nil {
				row = &BillingStatementSummary{
					StatementNo:    statement.StatementNo,
					UserId:         statement.UserId,
					Period:         statement.Period,
					PeriodValue:    statement.PeriodValue,
					Dimension:      dimension,
					DimensionValue: value,
				}
				collectors[dimension][value] = row
			}
			row.RequestCount++
			row.InputTokens += item.InputTokens
			row.OutputTokens += item.OutputTokens
			row.CacheReadTokens += item.CacheReadTokens
			row.CacheWriteTokens += item.CacheWriteTokens
			row.OriginalAmount += item.OriginalAmount
			row.DiscountAmount += item.DiscountAmount
			row.SettlementAmount += item.SettlementAmount
		}
	}
	dimensionOrder := []string{
		BillingStatementSummaryDimensionModel,
		BillingStatementSummaryDimensionGroup,
		BillingStatementSummaryDimensionKey,
		BillingStatementSummaryDimensionSource,
	}
	for _, dimension := range dimensionOrder {
		for _, row := range collectors[dimension] {
			summaries = append(summaries, *row)
		}
	}
	return summaries, nil
}

func GetBillingStatements(query BillingStatementQuery) ([]BillingStatement, error) {
	limit := query.Limit
	if limit <= 0 || limit > 100 {
		limit = 24
	}
	tx := LOG_DB.Model(&BillingStatement{})
	if query.UserId > 0 {
		tx = tx.Where("user_id = ?", query.UserId)
	}
	if strings.TrimSpace(query.Period) != "" {
		tx = tx.Where("period = ?", query.Period)
	}
	if strings.TrimSpace(query.StartMonth) != "" {
		tx = tx.Where("period_value >= ?", query.StartMonth)
	}
	if strings.TrimSpace(query.EndMonth) != "" {
		tx = tx.Where("period_value <= ?", query.EndMonth)
	}
	var rows []BillingStatement
	err := tx.Order("period_value desc, id desc").Limit(limit).Offset(query.Offset).Find(&rows).Error
	return rows, err
}

func GetBillingStatementSummaries(statementNo string) ([]BillingStatementSummary, error) {
	var rows []BillingStatementSummary
	err := LOG_DB.Model(&BillingStatementSummary{}).
		Where("statement_no = ?", strings.TrimSpace(statementNo)).
		Order("dimension asc, settlement_amount desc").
		Find(&rows).Error
	return rows, err
}

package model

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBillingV2BackfillCreatesUsageAndAccountLedger(t *testing.T) {
	truncateTables(t)
	require.NoError(t, LOG_DB.AutoMigrate(
		&BillingUsageItem{},
		&AccountLedgerEntry{},
		&BillingStatement{},
		&BillingStatementSummary{},
	))
	day := time.Date(2026, 6, 12, 0, 0, 0, 0, time.UTC)
	require.NoError(t, LOG_DB.Create(&[]Log{
		{
			Id:        7001,
			UserId:    1001,
			CreatedAt: day.Add(time.Hour).Unix(),
			Type:      LogTypeTopup,
			Quota:     50000,
			Content:   "top up",
		},
		{
			Id:               7002,
			UserId:           1001,
			CreatedAt:        day.Add(2 * time.Hour).Unix(),
			Type:             LogTypeConsume,
			ModelName:        "gpt-4.1",
			TokenId:          2001,
			TokenName:        "prod-key",
			Group:            "vip",
			RequestId:        "req-billing-v2",
			Quota:            8000,
			PromptTokens:     1200,
			CompletionTokens: 300,
			Other: common.MapToJsonStr(map[string]interface{}{
				"group_ratio":    0.8,
				"billing_source": "wallet",
				"billing_mode":   "tiered_expr",
				"cache_tokens":   100,
			}),
		},
		{
			Id:        7003,
			UserId:    1001,
			CreatedAt: day.Add(3 * time.Hour).Unix(),
			Type:      LogTypeRefund,
			Quota:     1000,
			Content:   "refund",
		},
	}).Error)

	result, err := BackfillBillingV2FromLogs(BillingV2BackfillOptions{
		StartTimestamp: day.Unix(),
		EndTimestamp:   day.AddDate(0, 0, 1).Add(-time.Second).Unix(),
		BatchSize:      100,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(3), result.Scanned)
	assert.Equal(t, int64(1), result.UsageCreated)
	assert.Equal(t, int64(3), result.LedgerCreated)

	usageItems, err := GetBillingUsageItems(BillingUsageItemQuery{
		UserId:    1001,
		StartTime: day.Unix(),
		EndTime:   day.AddDate(0, 0, 1).Add(-time.Second).Unix(),
		Limit:     20,
	})
	require.NoError(t, err)
	require.Len(t, usageItems, 1)
	assert.Equal(t, "req-billing-v2", usageItems[0].RequestId)
	assert.Equal(t, "gpt-4.1", usageItems[0].ModelName)
	assert.Equal(t, "vip", usageItems[0].Group)
	assert.Equal(t, int64(1100), usageItems[0].InputTokens)
	assert.Equal(t, int64(300), usageItems[0].OutputTokens)
	assert.Equal(t, int64(10000), usageItems[0].OriginalAmount)
	assert.Equal(t, int64(2000), usageItems[0].DiscountAmount)
	assert.Equal(t, int64(8000), usageItems[0].SettlementAmount)

	ledger, err := GetAccountLedgerEntries(AccountLedgerQuery{
		UserId:    1001,
		StartTime: day.Unix(),
		EndTime:   day.AddDate(0, 0, 1).Add(-time.Second).Unix(),
		Limit:     20,
	})
	require.NoError(t, err)
	require.Len(t, ledger, 3)
	assert.Equal(t, AccountLedgerEntryTypeTopup, ledger[0].EntryType)
	assert.Equal(t, int64(50000), ledger[0].Amount)
	assert.Equal(t, int64(50000), ledger[0].BalanceAfter)
	assert.Equal(t, AccountLedgerEntryTypeConsume, ledger[1].EntryType)
	assert.Equal(t, int64(-8000), ledger[1].Amount)
	assert.Equal(t, int64(42000), ledger[1].BalanceAfter)
	assert.Equal(t, AccountLedgerEntryTypeRefund, ledger[2].EntryType)
	assert.Equal(t, int64(1000), ledger[2].Amount)
	assert.Equal(t, int64(43000), ledger[2].BalanceAfter)
}

func TestBillingV2SkipsUserOwnedProviderConsumeLogs(t *testing.T) {
	truncateTables(t)
	require.NoError(t, LOG_DB.AutoMigrate(
		&BillingUsageItem{},
		&AccountLedgerEntry{},
		&BillingStatement{},
		&BillingStatementSummary{},
	))

	day := time.Date(2026, 6, 12, 0, 0, 0, 0, time.UTC)
	log := &Log{
		Id:               7051,
		UserId:           1001,
		CreatedAt:        day.Add(time.Hour).Unix(),
		Type:             LogTypeConsume,
		ModelName:        "gpt-5",
		TokenId:          2001,
		TokenName:        "byok-key",
		Group:            BuildUserOwnedProviderGroup(1001, 9001),
		RequestId:        "req-user-owned-provider",
		Quota:            8000,
		PromptTokens:     1200,
		CompletionTokens: 300,
		Other: common.MapToJsonStr(map[string]interface{}{
			"billing_source": "user_owned_provider",
			"billing_mode":   "ratio",
		}),
	}
	require.NoError(t, LOG_DB.Create(log).Error)

	created, err := RecordBillingUsageConsumeLog(log.Id)
	require.NoError(t, err)
	assert.False(t, created)

	var usageCount int64
	require.NoError(t, LOG_DB.Model(&BillingUsageItem{}).Count(&usageCount).Error)
	assert.Equal(t, int64(0), usageCount)

	var ledgerCount int64
	require.NoError(t, LOG_DB.Model(&AccountLedgerEntry{}).Count(&ledgerCount).Error)
	assert.Equal(t, int64(0), ledgerCount)

	result, err := BackfillBillingV2FromLogs(BillingV2BackfillOptions{
		StartTimestamp: day.Unix(),
		EndTimestamp:   day.AddDate(0, 0, 1).Add(-time.Second).Unix(),
		BatchSize:      100,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(1), result.Scanned)
	assert.Equal(t, int64(0), result.UsageCreated)
	assert.Equal(t, int64(0), result.LedgerCreated)
	assert.Equal(t, int64(1), result.Skipped)
}

func TestBillingV2UsageItemInfersPerRequestBillingModeFromFixedPrice(t *testing.T) {
	usage := billingUsageItemFromLog(&Log{
		Id:        7101,
		UserId:    1001,
		CreatedAt: time.Date(2026, 6, 12, 0, 0, 0, 0, time.UTC).Unix(),
		Type:      LogTypeConsume,
		ModelName: "image-model",
		Quota:     500,
	}, map[string]interface{}{
		"model_price": 0.01,
	})

	assert.Equal(t, "per-request", usage.BillingMode)
}

func TestBillingV2BuildsMonthlyStatementAndDailyReconciliation(t *testing.T) {
	truncateTables(t)
	require.NoError(t, LOG_DB.AutoMigrate(
		&BillingUsageItem{},
		&AccountLedgerEntry{},
		&BillingStatement{},
		&BillingStatementSummary{},
	))

	day := time.Date(2026, 6, 12, 0, 0, 0, 0, time.UTC)
	require.NoError(t, RecordBillingUsageItem(&BillingUsageItem{
		UserId:           1001,
		TokenId:          2001,
		TokenName:        "prod-key",
		LogId:            7101,
		RequestId:        "req-1",
		ModelName:        "gpt-4.1",
		Group:            "vip",
		BillingSource:    BillingSourceWallet,
		GroupRatio:       0.8,
		BilledAt:         day.Add(time.Hour).Unix(),
		BillingDate:      "2026-06-12",
		BillingMonth:     "2026-06",
		InputTokens:      1000,
		OutputTokens:     200,
		OriginalAmount:   10000,
		DiscountAmount:   2000,
		SettlementAmount: 8000,
	}))
	require.NoError(t, RecordBillingUsageItem(&BillingUsageItem{
		UserId:           1001,
		TokenId:          2002,
		TokenName:        "side-key",
		LogId:            7102,
		RequestId:        "req-2",
		ModelName:        "claude-sonnet-4",
		Group:            "default",
		BillingSource:    BillingSourceWallet,
		GroupRatio:       1,
		BilledAt:         day.Add(2 * time.Hour).Unix(),
		BillingDate:      "2026-06-12",
		BillingMonth:     "2026-06",
		InputTokens:      500,
		OutputTokens:     100,
		OriginalAmount:   5000,
		SettlementAmount: 5000,
	}))
	require.NoError(t, RecordBillingUsageItem(&BillingUsageItem{
		UserId:           1001,
		TokenId:          2003,
		TokenName:        "byok-key",
		LogId:            7103,
		RequestId:        "req-user-owned-provider",
		ModelName:        "gpt-5",
		Group:            BuildUserOwnedProviderGroup(1001, 9001),
		BillingSource:    BillingSourceUserProvider,
		GroupRatio:       1,
		BilledAt:         day.Add(3 * time.Hour).Unix(),
		BillingDate:      "2026-06-12",
		BillingMonth:     "2026-06",
		InputTokens:      700,
		OutputTokens:     100,
		OriginalAmount:   9000,
		SettlementAmount: 9000,
	}))
	require.NoError(t, RecordAccountLedgerEntry(&AccountLedgerEntry{
		UserId:        1001,
		AccountType:   BillingSourceWallet,
		EntryType:     AccountLedgerEntryTypeConsume,
		Amount:        -8000,
		BalanceBefore: 20000,
		BalanceAfter:  12000,
		SourceType:    AccountLedgerSourceUsage,
		SourceId:      "7101",
		OccurredAt:    day.Add(time.Hour).Unix(),
		LedgerDate:    "2026-06-12",
		LedgerMonth:   "2026-06",
	}))
	require.NoError(t, RecordAccountLedgerEntry(&AccountLedgerEntry{
		UserId:        1001,
		AccountType:   BillingSourceUserProvider,
		EntryType:     AccountLedgerEntryTypeConsume,
		Amount:        -9000,
		BalanceBefore: 0,
		BalanceAfter:  -9000,
		SourceType:    AccountLedgerSourceUsage,
		SourceId:      "7103",
		OccurredAt:    day.Add(3 * time.Hour).Unix(),
		LedgerDate:    "2026-06-12",
		LedgerMonth:   "2026-06",
	}))
	require.NoError(t, RecordAccountLedgerEntry(&AccountLedgerEntry{
		UserId:        1001,
		AccountType:   BillingSourceWallet,
		EntryType:     AccountLedgerEntryTypeConsume,
		Amount:        -5000,
		BalanceBefore: 12000,
		BalanceAfter:  7000,
		SourceType:    AccountLedgerSourceUsage,
		SourceId:      "7102",
		OccurredAt:    day.Add(2 * time.Hour).Unix(),
		LedgerDate:    "2026-06-12",
		LedgerMonth:   "2026-06",
	}))

	reconciliations, err := GetDailyBillingReconciliations(BillingStatementQuery{
		UserId:     1001,
		StartMonth: "2026-06",
		EndMonth:   "2026-06",
	})
	require.NoError(t, err)
	require.Len(t, reconciliations, 1)
	assert.Equal(t, "2026-06-12", reconciliations[0].Date)
	assert.Equal(t, int64(13000), reconciliations[0].UsageSettlementAmount)
	assert.Equal(t, int64(13000), reconciliations[0].AccountConsumeAmount)
	assert.Equal(t, BillingStatementStatusConfirmed, reconciliations[0].Status)

	statement, summaries, err := GenerateMonthlyBillingStatement(1001, "2026-06")
	require.NoError(t, err)
	assert.Equal(t, BillingStatementPeriodMonth, statement.Period)
	assert.Equal(t, int64(1500), statement.InputTokens)
	assert.Equal(t, int64(300), statement.OutputTokens)
	assert.Equal(t, int64(15000), statement.OriginalAmount)
	assert.Equal(t, int64(2000), statement.DiscountAmount)
	assert.Equal(t, int64(13000), statement.SettlementAmount)
	assert.Equal(t, BillingStatementStatusConfirmed, statement.Status)
	require.Len(t, summaries, 2)
	var gptVipSummary BillingStatementSummary
	for _, summary := range summaries {
		assert.Equal(t, BillingStatementSummaryDimensionMonthModelGroup, summary.Dimension)
		if summary.ModelName == "gpt-4.1" && summary.Group == "vip" {
			gptVipSummary = summary
		}
	}
	assert.Equal(t, int64(1000), gptVipSummary.InputTokens)
	assert.Equal(t, int64(200), gptVipSummary.OutputTokens)
	assert.Equal(t, int64(8000), gptVipSummary.SettlementAmount)
}

func TestBillingV2BreakdownRowsAggregateByPeriodModelGroup(t *testing.T) {
	truncateTables(t)
	require.NoError(t, LOG_DB.AutoMigrate(
		&BillingUsageItem{},
		&AccountLedgerEntry{},
		&BillingStatement{},
		&BillingStatementSummary{},
	))

	day := time.Date(2026, 6, 12, 0, 0, 0, 0, time.UTC)
	require.NoError(t, RecordBillingUsageItem(&BillingUsageItem{
		UserId:           1001,
		TokenId:          2001,
		TokenName:        "prod-key",
		LogId:            7301,
		RequestId:        "req-breakdown-1",
		ModelName:        "gpt-4.1",
		Group:            "vip",
		BillingSource:    BillingSourceWallet,
		BillingMode:      "tiered_expr",
		GroupRatio:       0.8,
		BilledAt:         day.Add(time.Hour).Unix(),
		InputTokens:      1000,
		OutputTokens:     200,
		CacheReadTokens:  100,
		CacheWriteTokens: 50,
		OriginalAmount:   10000,
		DiscountAmount:   2000,
		SettlementAmount: 8000,
	}))
	require.NoError(t, RecordBillingUsageItem(&BillingUsageItem{
		UserId:           1001,
		TokenId:          2001,
		TokenName:        "prod-key",
		LogId:            7302,
		RequestId:        "req-breakdown-2",
		ModelName:        "gpt-4.1",
		Group:            "vip",
		BillingSource:    BillingSourceWallet,
		BillingMode:      "tiered_expr",
		GroupRatio:       0.8,
		BilledAt:         day.Add(2 * time.Hour).Unix(),
		InputTokens:      500,
		OutputTokens:     100,
		CacheReadTokens:  20,
		CacheWriteTokens: 10,
		OriginalAmount:   5000,
		DiscountAmount:   1000,
		SettlementAmount: 4000,
	}))
	require.NoError(t, RecordBillingUsageItem(&BillingUsageItem{
		UserId:           1001,
		TokenId:          2002,
		TokenName:        "default-key",
		LogId:            7303,
		RequestId:        "req-breakdown-3",
		ModelName:        "gpt-4.1",
		Group:            "default",
		BillingSource:    BillingSourceWallet,
		BillingMode:      "ratio",
		GroupRatio:       1,
		BilledAt:         day.AddDate(0, 0, 1).Add(time.Hour).Unix(),
		InputTokens:      200,
		OutputTokens:     80,
		OriginalAmount:   1000,
		SettlementAmount: 1000,
	}))
	require.NoError(t, RecordBillingUsageItem(&BillingUsageItem{
		UserId:           1001,
		TokenId:          2003,
		TokenName:        "vip-ratio-key",
		LogId:            7304,
		RequestId:        "req-breakdown-4",
		ModelName:        "gpt-4.1",
		Group:            "vip",
		BillingSource:    BillingSourceWallet,
		BillingMode:      "ratio",
		GroupRatio:       0.8,
		BilledAt:         day.Add(3 * time.Hour).Unix(),
		InputTokens:      300,
		OutputTokens:     60,
		OriginalAmount:   3000,
		DiscountAmount:   600,
		SettlementAmount: 2400,
	}))
	require.NoError(t, RecordBillingUsageItem(&BillingUsageItem{
		UserId:           1001,
		TokenId:          2004,
		TokenName:        "vip-fixed-key",
		LogId:            7305,
		RequestId:        "req-breakdown-5",
		ModelName:        "gpt-4.1",
		Group:            "vip",
		BillingSource:    BillingSourceWallet,
		BillingMode:      "per-request",
		GroupRatio:       1,
		BilledAt:         day.Add(4 * time.Hour).Unix(),
		OriginalAmount:   500,
		SettlementAmount: 500,
	}))

	monthlyRows, err := GetBillingBreakdownRows(BillingBreakdownQuery{
		UserId: 1001,
		Period: BillingStatementPeriodMonth,
		Month:  "2026-06",
	})
	require.NoError(t, err)
	require.Len(t, monthlyRows, 2)
	var vipRow BillingBreakdownRow
	for _, row := range monthlyRows {
		if row.ModelName == "gpt-4.1" && row.Group == "vip" {
			vipRow = row
		}
	}
	assert.Equal(t, "2026-06", vipRow.PeriodValue)
	assert.Equal(t, BillingSourceWallet, vipRow.BillingSource)
	assert.Equal(t, int64(4), vipRow.RequestCount)
	assert.Equal(t, int64(1800), vipRow.InputTokens)
	assert.Equal(t, int64(360), vipRow.OutputTokens)
	assert.Equal(t, int64(120), vipRow.CacheReadTokens)
	assert.Equal(t, int64(60), vipRow.CacheWriteTokens)
	assert.Equal(t, int64(18500), vipRow.OriginalAmount)
	assert.Equal(t, int64(3600), vipRow.DiscountAmount)
	assert.Equal(t, int64(14900), vipRow.SettlementAmount)

	exportRows, err := GetBillingBreakdownRows(BillingBreakdownQuery{
		UserId:       1001,
		Period:       BillingStatementPeriodMonth,
		Month:        "2026-06",
		Limit:        1,
		NoPagination: true,
	})
	require.NoError(t, err)
	require.Len(t, exportRows, 2)

	dailyRows, err := GetBillingBreakdownRows(BillingBreakdownQuery{
		UserId:    1001,
		Period:    BillingStatementPeriodDay,
		StartDate: "2026-06-12",
		EndDate:   "2026-06-12",
	})
	require.NoError(t, err)
	require.Len(t, dailyRows, 1)
	assert.Equal(t, "2026-06-12", dailyRows[0].PeriodValue)

	statement, summaries, err := GenerateMonthlyBillingStatement(1001, "2026-06")
	require.NoError(t, err)
	assert.Equal(t, int64(15900), statement.SettlementAmount)
	var detail BillingStatementSummary
	for _, summary := range summaries {
		if summary.Dimension == BillingStatementSummaryDimensionMonthModelGroup &&
			summary.ModelName == "gpt-4.1" &&
			summary.Group == "vip" {
			detail = summary
		}
	}
	assert.Equal(t, "2026-06", detail.PeriodValue)
	assert.Equal(t, int64(14900), detail.SettlementAmount)
	assert.Equal(t, int64(3600), detail.DiscountAmount)
}

func TestGenerateRecentMonthlyBillingStatementsBackfillsPreviousFullMonth(t *testing.T) {
	truncateTables(t)
	require.NoError(t, LOG_DB.AutoMigrate(
		&BillingUsageItem{},
		&AccountLedgerEntry{},
		&BillingStatement{},
		&BillingStatementSummary{},
	))

	now := time.Date(2026, 6, 13, 10, 0, 0, 0, time.UTC)
	may := time.Date(2026, 5, 12, 0, 0, 0, 0, time.UTC)
	june := time.Date(2026, 6, 2, 0, 0, 0, 0, time.UTC)
	require.NoError(t, LOG_DB.Create(&[]Log{
		{
			Id:        7401,
			UserId:    1001,
			CreatedAt: may.Add(time.Hour).Unix(),
			Type:      LogTypeTopup,
			Quota:     20000,
			Content:   "may topup",
		},
		{
			Id:               7402,
			UserId:           1001,
			CreatedAt:        may.Add(2 * time.Hour).Unix(),
			Type:             LogTypeConsume,
			ModelName:        "gpt-4.1",
			TokenId:          2001,
			TokenName:        "may-key",
			Group:            "vip",
			RequestId:        "req-may-billing",
			Quota:            8000,
			PromptTokens:     1000,
			CompletionTokens: 300,
			Other: common.MapToJsonStr(map[string]interface{}{
				"group_ratio":    0.8,
				"billing_source": "wallet",
				"billing_mode":   "tiered_expr",
			}),
		},
		{
			Id:               7403,
			UserId:           1002,
			CreatedAt:        may.Add(3 * time.Hour).Unix(),
			Type:             LogTypeConsume,
			ModelName:        "claude-sonnet-4",
			Group:            "default",
			RequestId:        "req-may-user-2",
			Quota:            5000,
			PromptTokens:     700,
			CompletionTokens: 200,
		},
		{
			Id:               7404,
			UserId:           1001,
			CreatedAt:        june.Add(time.Hour).Unix(),
			Type:             LogTypeConsume,
			ModelName:        "gpt-4.1",
			Group:            "vip",
			RequestId:        "req-june-billing",
			Quota:            3000,
			PromptTokens:     300,
			CompletionTokens: 80,
		},
	}).Error)

	result, err := GenerateRecentMonthlyBillingStatements(now, 100)
	require.NoError(t, err)
	assert.Equal(t, "2026-05", result.Month)
	assert.Equal(t, int64(3), result.Backfill.Scanned)
	assert.Equal(t, int64(2), result.Backfill.UsageCreated)
	assert.Equal(t, 2, result.StatementCount)
	assert.Empty(t, result.FailedUsers)

	statements, err := GetBillingStatements(BillingStatementQuery{
		StartMonth: "2026-05",
		EndMonth:   "2026-06",
		Period:     BillingStatementPeriodMonth,
		Limit:      10,
	})
	require.NoError(t, err)
	require.Len(t, statements, 2)
	for _, statement := range statements {
		assert.Equal(t, "2026-05", statement.PeriodValue)
	}

	juneRows, err := GetBillingBreakdownRows(BillingBreakdownQuery{
		Period: BillingStatementPeriodMonth,
		Month:  "2026-06",
	})
	require.NoError(t, err)
	assert.Empty(t, juneRows)
}

func TestRecordBillingUsageConsumeLogDoesNotExtendLedger(t *testing.T) {
	truncateTables(t)
	require.NoError(t, LOG_DB.AutoMigrate(
		&BillingUsageItem{},
		&AccountLedgerEntry{},
		&BillingStatement{},
		&BillingStatementSummary{},
	))
	require.NoError(t, DB.Create(&User{
		Id:       1001,
		Username: "billing-usage-only",
		Quota:    50000,
	}).Error)

	day := time.Date(2026, 6, 12, 0, 0, 0, 0, time.UTC)
	require.NoError(t, RecordAccountLedgerEntry(&AccountLedgerEntry{
		UserId:        1001,
		AccountType:   BillingSourceWallet,
		EntryType:     AccountLedgerEntryTypeTopup,
		Amount:        20000,
		BalanceBefore: 0,
		BalanceAfter:  20000,
		OccurredAt:    day.Unix(),
	}))
	require.NoError(t, LOG_DB.Create(&Log{
		Id:               7201,
		UserId:           1001,
		CreatedAt:        day.Add(time.Hour).Unix(),
		Type:             LogTypeConsume,
		ModelName:        "gpt-4.1",
		TokenId:          2001,
		TokenName:        "prod-key",
		Group:            "vip",
		RequestId:        "req-live-billing-v2",
		Quota:            6000,
		PromptTokens:     800,
		CompletionTokens: 200,
		Other: common.MapToJsonStr(map[string]interface{}{
			"group_ratio":    0.75,
			"billing_source": "wallet",
			"billing_mode":   "ratio",
		}),
	}).Error)

	created, err := RecordBillingUsageConsumeLog(7201)
	require.NoError(t, err)
	assert.True(t, created)

	created, err = RecordBillingUsageConsumeLog(7201)
	require.NoError(t, err)
	assert.False(t, created)

	usageItems, err := GetBillingUsageItems(BillingUsageItemQuery{UserId: 1001, Limit: 10})
	require.NoError(t, err)
	require.Len(t, usageItems, 1)
	assert.Equal(t, int64(6000), usageItems[0].SettlementAmount)
	assert.Equal(t, int64(8000), usageItems[0].OriginalAmount)

	ledger, err := GetAccountLedgerEntries(AccountLedgerQuery{UserId: 1001, Limit: 10})
	require.NoError(t, err)
	require.Len(t, ledger, 1)
	assert.Equal(t, AccountLedgerEntryTypeTopup, ledger[0].EntryType)
	assert.Equal(t, int64(20000), ledger[0].BalanceAfter)

	var userQuota int
	require.NoError(t, DB.Model(&User{}).Where("id = ?", 1001).Pluck("quota", &userQuota).Error)
	assert.Equal(t, 50000, userQuota)
}

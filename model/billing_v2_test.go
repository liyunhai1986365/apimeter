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
	require.Len(t, summaries, 7)
	var gptSummary BillingStatementSummary
	for _, summary := range summaries {
		if summary.Dimension == BillingStatementSummaryDimensionModel && summary.DimensionValue == "gpt-4.1" {
			gptSummary = summary
		}
	}
	assert.Equal(t, int64(8000), gptSummary.SettlementAmount)
}

func TestRecordBillingV2ConsumeLogExtendsLedgerBalance(t *testing.T) {
	truncateTables(t)
	require.NoError(t, LOG_DB.AutoMigrate(
		&BillingUsageItem{},
		&AccountLedgerEntry{},
		&BillingStatement{},
		&BillingStatementSummary{},
	))

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

	created, err := RecordBillingV2ConsumeLog(7201)
	require.NoError(t, err)
	assert.True(t, created)

	created, err = RecordBillingV2ConsumeLog(7201)
	require.NoError(t, err)
	assert.False(t, created)

	usageItems, err := GetBillingUsageItems(BillingUsageItemQuery{UserId: 1001, Limit: 10})
	require.NoError(t, err)
	require.Len(t, usageItems, 1)
	assert.Equal(t, int64(6000), usageItems[0].SettlementAmount)
	assert.Equal(t, int64(8000), usageItems[0].OriginalAmount)

	ledger, err := GetAccountLedgerEntries(AccountLedgerQuery{UserId: 1001, Limit: 10})
	require.NoError(t, err)
	require.Len(t, ledger, 2)
	assert.Equal(t, AccountLedgerEntryTypeConsume, ledger[1].EntryType)
	assert.Equal(t, int64(-6000), ledger[1].Amount)
	assert.Equal(t, int64(20000), ledger[1].BalanceBefore)
	assert.Equal(t, int64(14000), ledger[1].BalanceAfter)
}

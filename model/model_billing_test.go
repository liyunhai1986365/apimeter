package model

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModelBillingSummaryAggregatesByDayAndMonth(t *testing.T) {
	truncateTables(t)
	require.NoError(t, LOG_DB.AutoMigrate(&ModelBillingRecord{}))

	may29 := time.Date(2026, 5, 29, 10, 30, 0, 0, time.UTC).Unix()
	may30 := time.Date(2026, 5, 30, 9, 0, 0, 0, time.UTC).Unix()
	june1 := time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC).Unix()

	require.NoError(t, RecordModelBilling(&ModelBillingRecord{
		UserId:           1001,
		TokenId:          2001,
		LogId:            3001,
		RequestId:        "req-1",
		ModelName:        "gpt-4.1",
		BillingSource:    "wallet",
		Group:            "vip",
		GroupRatio:       0.8,
		BillingMode:      "ratio",
		CreatedAt:        may29,
		InputTokens:      1000,
		OutputTokens:     200,
		CacheWriteTokens: 300,
		CacheReadTokens:  400,
		OriginalQuota:    10000,
		DiscountQuota:    2000,
		PayableQuota:     8000,
	}))
	require.NoError(t, RecordModelBilling(&ModelBillingRecord{
		UserId:           1001,
		TokenId:          2002,
		LogId:            3002,
		RequestId:        "req-2",
		ModelName:        "gpt-4.1",
		BillingSource:    "subscription",
		Group:            "vip",
		GroupRatio:       0.8,
		BillingMode:      "tiered_expr",
		CreatedAt:        may29 + 3600,
		InputTokens:      500,
		OutputTokens:     100,
		CacheWriteTokens: 50,
		CacheReadTokens:  75,
		OriginalQuota:    5000,
		DiscountQuota:    1000,
		PayableQuota:     4000,
	}))
	require.NoError(t, RecordModelBilling(&ModelBillingRecord{
		UserId:        1001,
		LogId:         3003,
		ModelName:     "claude-sonnet-4",
		BillingSource: "wallet",
		Group:         "default",
		GroupRatio:    1,
		BillingMode:   "ratio",
		CreatedAt:     may30,
		InputTokens:   800,
		OutputTokens:  160,
		OriginalQuota: 6000,
		PayableQuota:  6000,
	}))
	require.NoError(t, RecordModelBilling(&ModelBillingRecord{
		UserId:        1002,
		LogId:         3004,
		ModelName:     "gpt-4.1",
		BillingSource: "wallet",
		Group:         "default",
		GroupRatio:    1,
		BillingMode:   "ratio",
		CreatedAt:     may29,
		InputTokens:   999,
		OutputTokens:  999,
		OriginalQuota: 9999,
		PayableQuota:  9999,
	}))
	require.NoError(t, RecordModelBilling(&ModelBillingRecord{
		UserId:        1001,
		LogId:         3005,
		ModelName:     "gpt-4.1",
		BillingSource: "wallet",
		Group:         "vip",
		GroupRatio:    0.8,
		BillingMode:   "ratio",
		CreatedAt:     june1,
		InputTokens:   100,
		OutputTokens:  20,
		OriginalQuota: 1000,
		DiscountQuota: 200,
		PayableQuota:  800,
	}))

	dayRows, err := GetModelBillingSummary(ModelBillingSummaryQuery{
		UserId:         1001,
		Period:         ModelBillingPeriodDay,
		StartTimestamp: time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC).Unix(),
		EndTimestamp:   time.Date(2026, 5, 31, 23, 59, 59, 0, time.UTC).Unix(),
	})
	require.NoError(t, err)
	require.Len(t, dayRows, 2)

	assert.Equal(t, "2026-05-29", dayRows[0].Period)
	assert.Equal(t, "gpt-4.1", dayRows[0].ModelName)
	assert.Equal(t, 2, dayRows[0].RequestCount)
	assert.Equal(t, int64(1500), dayRows[0].InputTokens)
	assert.Equal(t, int64(300), dayRows[0].OutputTokens)
	assert.Equal(t, int64(350), dayRows[0].CacheWriteTokens)
	assert.Equal(t, int64(475), dayRows[0].CacheReadTokens)
	assert.Equal(t, int64(15000), dayRows[0].OriginalQuota)
	assert.Equal(t, int64(3000), dayRows[0].DiscountQuota)
	assert.Equal(t, int64(12000), dayRows[0].PayableQuota)

	assert.Equal(t, "2026-05-30", dayRows[1].Period)
	assert.Equal(t, "claude-sonnet-4", dayRows[1].ModelName)
	assert.Equal(t, int64(6000), dayRows[1].PayableQuota)

	monthRows, err := GetModelBillingSummary(ModelBillingSummaryQuery{
		UserId:         1001,
		Period:         ModelBillingPeriodMonth,
		StartTimestamp: time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC).Unix(),
		EndTimestamp:   time.Date(2026, 6, 30, 23, 59, 59, 0, time.UTC).Unix(),
		ModelName:      "gpt-4.1",
	})
	require.NoError(t, err)
	require.Len(t, monthRows, 2)
	assert.Equal(t, "2026-05", monthRows[0].Period)
	assert.Equal(t, int64(12000), monthRows[0].PayableQuota)
	assert.Equal(t, "2026-06", monthRows[1].Period)
	assert.Equal(t, int64(800), monthRows[1].PayableQuota)
}

func TestMigrateDBCreatesModelBillingTableWhenLogDBUsesMainDB(t *testing.T) {
	truncateTables(t)
	require.NoError(t, LOG_DB.Migrator().DropTable(&ModelBillingRecord{}))
	require.False(t, LOG_DB.Migrator().HasTable(&ModelBillingRecord{}))

	require.NoError(t, migrateDB())

	require.True(t, LOG_DB.Migrator().HasTable(&ModelBillingRecord{}))
	rows, err := GetModelBillingSummary(ModelBillingSummaryQuery{
		UserId: 1001,
		Period: ModelBillingPeriodDay,
	})
	require.NoError(t, err)
	require.Empty(t, rows)
}

func TestBackfillModelBillingFromConsumeLogs(t *testing.T) {
	truncateTables(t)
	require.NoError(t, LOG_DB.AutoMigrate(&ModelBillingRecord{}))

	createdAt := time.Date(2026, 5, 29, 10, 30, 0, 0, time.UTC).Unix()
	require.NoError(t, LOG_DB.Create(&Log{
		Id:               4101,
		UserId:           1001,
		CreatedAt:        createdAt,
		Type:             LogTypeConsume,
		ModelName:        "gpt-4.1",
		Quota:            8000,
		PromptTokens:     1000,
		CompletionTokens: 200,
		TokenId:          2001,
		Group:            "vip",
		RequestId:        "req-backfill-1",
		Other: common.MapToJsonStr(map[string]interface{}{
			"group_ratio":              0.8,
			"billing_source":           "subscription",
			"billing_mode":             "tiered_expr",
			"cache_tokens":             300,
			"cache_write_tokens":       100,
			"cache_creation_tokens_5m": 40,
			"cache_creation_tokens_1h": 60,
		}),
	}).Error)
	require.NoError(t, LOG_DB.Create(&Log{
		Id:               4102,
		UserId:           1001,
		CreatedAt:        createdAt,
		Type:             LogTypeConsume,
		ModelName:        "gpt-4.1",
		Quota:            100,
		PromptTokens:     10,
		CompletionTokens: 20,
	}).Error)
	require.NoError(t, RecordModelBilling(&ModelBillingRecord{
		UserId:       1001,
		LogId:        4102,
		ModelName:    "gpt-4.1",
		CreatedAt:    createdAt,
		PayableQuota: 100,
	}))

	result, err := BackfillModelBillingFromLogs(ModelBillingBackfillOptions{
		BatchSize: 10,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(2), result.Scanned)
	assert.Equal(t, int64(1), result.Created)
	assert.Equal(t, int64(1), result.Skipped)

	var record ModelBillingRecord
	require.NoError(t, LOG_DB.Where("log_id = ?", 4101).First(&record).Error)
	assert.Equal(t, "2026-05-29", record.BillDate)
	assert.Equal(t, "2026-05", record.BillMonth)
	assert.Equal(t, int64(600), record.InputTokens)
	assert.Equal(t, int64(200), record.OutputTokens)
	assert.Equal(t, int64(100), record.CacheWriteTokens)
	assert.Equal(t, int64(300), record.CacheReadTokens)
	assert.Equal(t, int64(10000), record.OriginalQuota)
	assert.Equal(t, int64(2000), record.DiscountQuota)
	assert.Equal(t, int64(8000), record.PayableQuota)
	assert.Equal(t, "subscription", record.BillingSource)
	assert.Equal(t, "tiered_expr", record.BillingMode)

	result, err = BackfillModelBillingFromLogs(ModelBillingBackfillOptions{
		BatchSize: 10,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(0), result.Created)
	assert.Equal(t, int64(2), result.Skipped)
}

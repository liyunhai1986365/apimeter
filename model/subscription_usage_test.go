package model

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetSubscriptionKeyUsageStatsAggregatesTokenLogsByDay(t *testing.T) {
	truncateTables(t)

	now := time.Date(2026, 5, 24, 10, 0, 0, 0, time.UTC)
	dayStart := func(daysAgo int) int64 {
		return time.Date(2026, 5, 24-daysAgo, 0, 0, 0, 0, time.UTC).Unix()
	}
	require.NoError(t, LOG_DB.Create(&Log{
		UserId:           7001,
		Type:             LogTypeConsume,
		TokenId:          8101,
		CreatedAt:        dayStart(0) + 3600,
		Quota:            120,
		PromptTokens:     10,
		CompletionTokens: 20,
	}).Error)
	require.NoError(t, LOG_DB.Create(&Log{
		UserId:           7001,
		Type:             LogTypeConsume,
		TokenId:          8101,
		CreatedAt:        dayStart(0) + 7200,
		Quota:            80,
		PromptTokens:     5,
		CompletionTokens: 15,
	}).Error)
	require.NoError(t, LOG_DB.Create(&Log{
		UserId:           7001,
		Type:             LogTypeConsume,
		TokenId:          8101,
		CreatedAt:        dayStart(1) + 3600,
		Quota:            50,
		PromptTokens:     7,
		CompletionTokens: 8,
	}).Error)
	require.NoError(t, LOG_DB.Create(&Log{
		UserId:    7001,
		Type:      LogTypeError,
		TokenId:   8101,
		CreatedAt: dayStart(0) + 9000,
		Quota:     999,
	}).Error)
	require.NoError(t, LOG_DB.Create(&Log{
		UserId:    7001,
		Type:      LogTypeConsume,
		TokenId:   9999,
		CreatedAt: dayStart(0) + 3600,
		Quota:     999,
	}).Error)

	stats, err := GetSubscriptionKeyUsageStats(7001, 8101, 7, now)
	require.NoError(t, err)

	assert.Equal(t, 3, stats.TotalRequests)
	assert.Equal(t, 2, stats.TodayRequests)
	assert.Equal(t, 250, stats.TotalQuota)
	assert.Equal(t, 65, stats.TotalTokens)
	require.Len(t, stats.Points, 7)
	assert.Equal(t, "2026-05-18", stats.Points[0].Date)
	assert.Equal(t, "2026-05-24", stats.Points[6].Date)
	assert.Equal(t, 2, stats.Points[6].Requests)
	assert.Equal(t, 200, stats.Points[6].Quota)
	assert.Equal(t, 50, stats.Points[5].Quota)
	assert.Equal(t, 0, stats.Points[4].Requests)
}

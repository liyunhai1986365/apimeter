package service

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupModelMonitorTestDB(t *testing.T) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Model{}, &model.Channel{}, &model.Ability{}, &model.Log{}, &model.ModelChannelTestResult{}, &model.ChannelOperationRecord{}, &model.User{}))

	originDB := model.DB
	originLogDB := model.LOG_DB
	model.DB = db
	model.LOG_DB = db
	modelMonitorCache.Lock()
	modelMonitorCache.items = map[string]modelMonitorCacheEntry{}
	modelMonitorCache.Unlock()
	t.Cleanup(func() {
		model.DB = originDB
		model.LOG_DB = originLogDB
		modelMonitorCache.Lock()
		modelMonitorCache.items = map[string]modelMonitorCacheEntry{}
		modelMonitorCache.Unlock()
	})
}

func TestGetLatestModelChannelTestResultsKeepsNewestPerChannel(t *testing.T) {
	setupModelMonitorTestDB(t)
	now := time.Now().Unix()

	results := []model.ModelChannelTestResult{
		{ModelID: 1, ModelName: "gpt-test", ChannelID: 11, ChannelName: "openai-a", Success: false, Message: "old failure", CreatedAt: now - 30},
		{ModelID: 1, ModelName: "gpt-test", ChannelID: 11, ChannelName: "openai-a", Success: true, Message: "new success", CreatedAt: now - 10},
		{ModelID: 1, ModelName: "gpt-test", ChannelID: 12, ChannelName: "openai-b", Success: false, Message: "latest b", CreatedAt: now - 20},
		{ModelID: 2, ModelName: "other-model", ChannelID: 11, ChannelName: "openai-a", Success: false, Message: "other model", CreatedAt: now},
	}
	require.NoError(t, model.InsertModelChannelTestResults(results))

	latest, err := model.GetLatestModelChannelTestResults(1)
	require.NoError(t, err)
	require.Len(t, latest, 2)
	require.EqualValues(t, 11, latest[0].ChannelID)
	require.True(t, latest[0].Success)
	require.Equal(t, "new success", latest[0].Message)
	require.EqualValues(t, 12, latest[1].ChannelID)
	require.Equal(t, "latest b", latest[1].Message)
}

func TestNormalizeModelMonitorWindowDefaultsToTenMinutes(t *testing.T) {
	now := int64(1_700_000_000)
	filter := NormalizeModelMonitorWindow(ModelMonitorQuery{}, now)

	require.EqualValues(t, 600, filter.WindowSeconds)
	require.Equal(t, now-600, filter.StartTimestamp)
	require.Equal(t, now, filter.EndTimestamp)
	require.Equal(t, "10m", filter.Window)
}

func TestNormalizeModelMonitorWindowCapsOversizedRange(t *testing.T) {
	now := int64(1_700_000_000)
	filter := NormalizeModelMonitorWindow(ModelMonitorQuery{
		StartTimestamp: now - 9*24*3600,
		EndTimestamp:   now,
	}, now)

	require.EqualValues(t, MaxModelMonitorWindowSeconds, filter.WindowSeconds)
	require.Equal(t, now-MaxModelMonitorWindowSeconds, filter.StartTimestamp)
	require.Equal(t, now, filter.EndTimestamp)
}

func TestGetModelMonitorAggregatesLogsByChannelWithinWindow(t *testing.T) {
	setupModelMonitorTestDB(t)
	now := time.Now().Unix()

	require.NoError(t, model.DB.Create(&model.Model{Id: 1, ModelName: "gpt-test", Status: 1}).Error)
	require.NoError(t, model.DB.Create(&model.Channel{Id: 11, Name: "openai-a", Type: 1, Status: 1, Models: "gpt-test", Group: "default"}).Error)
	require.NoError(t, model.DB.Create(&model.Channel{Id: 12, Name: "openai-b", Type: 1, Status: 1, Models: "gpt-test", Group: "default"}).Error)
	require.NoError(t, model.DB.Create(&model.Ability{Group: "default", Model: "gpt-test", ChannelId: 11, Enabled: true}).Error)
	require.NoError(t, model.DB.Create(&model.Ability{Group: "default", Model: "gpt-test", ChannelId: 12, Enabled: true}).Error)

	logs := []model.Log{
		{CreatedAt: now - 60, Type: model.LogTypeConsume, ModelName: "gpt-test", ChannelId: 11, Quota: 120, PromptTokens: 10, CompletionTokens: 20, UseTime: 1, Group: "default"},
		{CreatedAt: now - 50, Type: model.LogTypeConsume, ModelName: "gpt-test", ChannelId: 11, Quota: 80, PromptTokens: 5, CompletionTokens: 15, UseTime: 3, Group: "default"},
		{CreatedAt: now - 40, Type: model.LogTypeError, ModelName: "gpt-test", ChannelId: 11, Content: "upstream timeout", UseTime: 2, Group: "default", Other: `{"code":"timeout"}`},
		{CreatedAt: now - 30, Type: model.LogTypeError, ModelName: "gpt-test", ChannelId: 12, Content: "rate limited", Group: "default", Other: `{"code":"rate_limit"}`},
		{CreatedAt: now - 3600, Type: model.LogTypeError, ModelName: "gpt-test", ChannelId: 11, Content: "old error", Group: "default"},
		{CreatedAt: now - 20, Type: model.LogTypeConsume, ModelName: "other-model", ChannelId: 11, Quota: 999, Group: "default"},
	}
	require.NoError(t, model.LOG_DB.Create(&logs).Error)

	result, err := GetModelMonitor(1, ModelMonitorQuery{EndTimestamp: now}, now)
	require.NoError(t, err)

	require.Equal(t, "gpt-test", result.ModelName)
	require.EqualValues(t, 4, result.Summary.TotalRequests)
	require.EqualValues(t, 2, result.Summary.SuccessRequests)
	require.EqualValues(t, 2, result.Summary.ErrorRequests)
	require.InDelta(t, 50, result.Summary.SuccessRate, 0.01)
	require.Len(t, result.Channels, 2)

	first := result.Channels[0]
	require.EqualValues(t, 11, first.ChannelID)
	require.EqualValues(t, 3, first.TotalRequests)
	require.EqualValues(t, 2, first.SuccessRequests)
	require.EqualValues(t, 1, first.ErrorRequests)
	require.EqualValues(t, 200, first.Quota)
	require.InDelta(t, 66.67, first.SuccessRate, 0.01)
	require.Equal(t, "upstream timeout", first.LatestError)
	require.Equal(t, "timeout", first.LatestErrorCode)

	second := result.Channels[1]
	require.EqualValues(t, 12, second.ChannelID)
	require.EqualValues(t, 1, second.TotalRequests)
	require.EqualValues(t, 0, second.SuccessRequests)
	require.EqualValues(t, 1, second.ErrorRequests)
	require.Equal(t, "rate limited", second.LatestError)
}

func TestGetModelMonitorComputesStreamingLatencyMetrics(t *testing.T) {
	setupModelMonitorTestDB(t)
	now := time.Now().Unix()

	require.NoError(t, model.DB.Create(&model.Model{Id: 1, ModelName: "gpt-test", Status: 1}).Error)
	require.NoError(t, model.DB.Create(&model.Channel{Id: 11, Name: "openai-a", Type: 1, Status: 1, Models: "gpt-test", Group: "default"}).Error)
	require.NoError(t, model.DB.Create(&model.Ability{Group: "default", Model: "gpt-test", ChannelId: 11, Enabled: true}).Error)

	logs := []model.Log{
		{CreatedAt: now - 60, Type: model.LogTypeConsume, ModelName: "gpt-test", ChannelId: 11, CompletionTokens: 10, UseTime: 2, Group: "default", Other: `{"frt":500}`},
		{CreatedAt: now - 50, Type: model.LogTypeConsume, ModelName: "gpt-test", ChannelId: 11, CompletionTokens: 30, UseTime: 4, Group: "default", Other: `{"frt":1000}`},
		{CreatedAt: now - 40, Type: model.LogTypeConsume, ModelName: "gpt-test", ChannelId: 11, CompletionTokens: 0, UseTime: 9, Group: "default", Other: `{}`},
	}
	require.NoError(t, model.LOG_DB.Create(&logs).Error)

	result, err := GetModelMonitor(1, ModelMonitorQuery{EndTimestamp: now}, now)
	require.NoError(t, err)
	require.Len(t, result.Channels, 1)

	channel := result.Channels[0]
	require.InDelta(t, 5.0, channel.AvgUseTime, 0.01)
	require.Equal(t, 9, channel.P90UseTime)
	require.InDelta(t, 0.75, channel.AvgTtft, 0.01)
	require.InDelta(t, 0.1125, channel.TPOT, 0.0001)
	require.InDelta(t, 8.89, channel.TokensPerSecond, 0.01)

	require.InDelta(t, 5.0, result.Summary.AvgUseTime, 0.01)
	require.Equal(t, 9, result.Summary.P90UseTime)
	require.InDelta(t, 0.75, result.Summary.AvgTtft, 0.01)
	require.InDelta(t, 0.1125, result.Summary.TPOT, 0.0001)
	require.InDelta(t, 8.89, result.Summary.TokensPerSecond, 0.01)
}

func TestGetModelMonitorCountsOnlyFinalRetryLogByRequestID(t *testing.T) {
	setupModelMonitorTestDB(t)
	now := time.Now().Unix()

	require.NoError(t, model.DB.Create(&model.Model{Id: 1, ModelName: "gpt-test", Status: 1}).Error)
	require.NoError(t, model.DB.Create(&model.Channel{Id: 11, Name: "openai-a", Type: 1, Status: 1, Models: "gpt-test", Group: "default"}).Error)
	require.NoError(t, model.DB.Create(&model.Channel{Id: 12, Name: "openai-b", Type: 1, Status: 1, Models: "gpt-test", Group: "default"}).Error)
	require.NoError(t, model.DB.Create(&model.Ability{Group: "default", Model: "gpt-test", ChannelId: 11, Enabled: true}).Error)
	require.NoError(t, model.DB.Create(&model.Ability{Group: "default", Model: "gpt-test", ChannelId: 12, Enabled: true}).Error)

	logs := []model.Log{
		{CreatedAt: now - 90, Type: model.LogTypeError, ModelName: "gpt-test", ChannelId: 11, Content: "retry failed first", Group: "default", RequestId: "req-success"},
		{CreatedAt: now - 80, Type: model.LogTypeConsume, ModelName: "gpt-test", ChannelId: 12, Quota: 120, PromptTokens: 10, CompletionTokens: 20, UseTime: 3, Group: "default", RequestId: "req-success"},
		{CreatedAt: now - 70, Type: model.LogTypeError, ModelName: "gpt-test", ChannelId: 11, Content: "retry failed first", Group: "default", RequestId: "req-fail"},
		{CreatedAt: now - 60, Type: model.LogTypeError, ModelName: "gpt-test", ChannelId: 12, Content: "retry failed last", Group: "default", RequestId: "req-fail"},
		{CreatedAt: now - 50, Type: model.LogTypeError, ModelName: "gpt-test", ChannelId: 11, Content: "single failure", Group: "default"},
	}
	require.NoError(t, model.LOG_DB.Create(&logs).Error)

	result, err := GetModelMonitor(1, ModelMonitorQuery{EndTimestamp: now}, now)
	require.NoError(t, err)

	require.EqualValues(t, 3, result.Summary.TotalRequests)
	require.EqualValues(t, 1, result.Summary.SuccessRequests)
	require.EqualValues(t, 2, result.Summary.ErrorRequests)
	require.InDelta(t, 33.33, result.Summary.SuccessRate, 0.01)

	channelsByID := map[int]ModelMonitorChannel{}
	for _, channel := range result.Channels {
		channelsByID[channel.ChannelID] = channel
	}

	first := channelsByID[11]
	require.EqualValues(t, 11, first.ChannelID)
	require.EqualValues(t, 1, first.TotalRequests)
	require.EqualValues(t, 0, first.SuccessRequests)
	require.EqualValues(t, 1, first.ErrorRequests)
	require.Equal(t, "single failure", first.LatestError)

	second := channelsByID[12]
	require.EqualValues(t, 12, second.ChannelID)
	require.EqualValues(t, 2, second.TotalRequests)
	require.EqualValues(t, 1, second.SuccessRequests)
	require.EqualValues(t, 1, second.ErrorRequests)
	require.Equal(t, "retry failed last", second.LatestError)
}

func TestClearModelMonitorCacheRefreshesChannelPriority(t *testing.T) {
	setupModelMonitorTestDB(t)
	now := time.Now().Unix()

	initialPriority := int64(1)
	updatedPriority := int64(9)
	require.NoError(t, model.DB.Create(&model.Model{Id: 1, ModelName: "gpt-test", Status: 1}).Error)
	require.NoError(t, model.DB.Create(&model.Channel{Id: 11, Name: "openai-a", Type: 1, Status: 1, Models: "gpt-test", Group: "default", Priority: &initialPriority}).Error)
	require.NoError(t, model.DB.Create(&model.Ability{Group: "default", Model: "gpt-test", ChannelId: 11, Enabled: true, Priority: &initialPriority}).Error)

	first, err := GetModelMonitor(1, ModelMonitorQuery{EndTimestamp: now}, now)
	require.NoError(t, err)
	require.Len(t, first.Channels, 1)
	require.EqualValues(t, initialPriority, first.Channels[0].Priority)

	require.NoError(t, model.DB.Model(&model.Ability{}).Where("channel_id = ?", 11).Update("priority", updatedPriority).Error)

	cached, err := GetModelMonitor(1, ModelMonitorQuery{EndTimestamp: now}, now)
	require.NoError(t, err)
	require.Len(t, cached.Channels, 1)
	require.EqualValues(t, initialPriority, cached.Channels[0].Priority)

	ClearModelMonitorCache()

	refreshed, err := GetModelMonitor(1, ModelMonitorQuery{EndTimestamp: now}, now)
	require.NoError(t, err)
	require.Len(t, refreshed.Channels, 1)
	require.EqualValues(t, updatedPriority, refreshed.Channels[0].Priority)
}

func TestListModelMonitorModelsSortsByErrorRateDescendingBeforePaging(t *testing.T) {
	setupModelMonitorTestDB(t)
	now := time.Now().Unix()

	models := []model.Model{
		{Id: 1, ModelName: "stable-model", Status: 1},
		{Id: 2, ModelName: "broken-model", Status: 1},
		{Id: 3, ModelName: "mixed-model", Status: 1},
	}
	require.NoError(t, model.DB.Create(&models).Error)
	for _, m := range models {
		require.NoError(t, model.DB.Create(&model.Channel{
			Id:     100 + m.Id,
			Name:   m.ModelName + "-channel",
			Type:   1,
			Status: 1,
			Models: m.ModelName,
			Group:  "default",
		}).Error)
		require.NoError(t, model.DB.Create(&model.Ability{
			Group:     "default",
			Model:     m.ModelName,
			ChannelId: 100 + m.Id,
			Enabled:   true,
		}).Error)
	}

	logs := []model.Log{
		{CreatedAt: now - 60, Type: model.LogTypeConsume, ModelName: "stable-model", ChannelId: 101, Group: "default"},
		{CreatedAt: now - 50, Type: model.LogTypeConsume, ModelName: "stable-model", ChannelId: 101, Group: "default"},
		{CreatedAt: now - 60, Type: model.LogTypeError, ModelName: "broken-model", ChannelId: 102, Group: "default"},
		{CreatedAt: now - 50, Type: model.LogTypeError, ModelName: "broken-model", ChannelId: 102, Group: "default"},
		{CreatedAt: now - 60, Type: model.LogTypeConsume, ModelName: "mixed-model", ChannelId: 103, Group: "default"},
		{CreatedAt: now - 50, Type: model.LogTypeError, ModelName: "mixed-model", ChannelId: 103, Group: "default"},
	}
	require.NoError(t, model.LOG_DB.Create(&logs).Error)

	result, err := ListModelMonitorModels(0, 2, 1, "", ModelMonitorQuery{EndTimestamp: now}, now)
	require.NoError(t, err)
	require.Len(t, result.Items, 2)
	require.Equal(t, "broken-model", result.Items[0].ModelName)
	require.InDelta(t, 100, result.Items[0].Summary.ErrorRate, 0.01)
	require.Equal(t, "mixed-model", result.Items[1].ModelName)
	require.InDelta(t, 50, result.Items[1].Summary.ErrorRate, 0.01)
	require.EqualValues(t, 3, result.Total)
}

func TestListModelMonitorModelsFiltersLatestTestsToCurrentChannels(t *testing.T) {
	setupModelMonitorTestDB(t)
	now := time.Now().Unix()

	require.NoError(t, model.DB.Create(&model.Model{Id: 1, ModelName: "gpt-test", Status: 1}).Error)
	require.NoError(t, model.DB.Create(&model.Channel{Id: 11, Name: "openai-a", Type: 1, Status: common.ChannelStatusEnabled, Models: "gpt-test", Group: "default"}).Error)
	require.NoError(t, model.DB.Create(&model.Channel{Id: 12, Name: "openai-b", Type: 1, Status: common.ChannelStatusEnabled, Models: "other-model", Group: "default"}).Error)
	require.NoError(t, model.DB.Create(&model.Ability{Group: "default", Model: "gpt-test", ChannelId: 11, Enabled: true}).Error)
	require.NoError(t, model.InsertModelChannelTestResults([]model.ModelChannelTestResult{
		{ModelID: 1, ModelName: "gpt-test", ChannelID: 11, ChannelName: "openai-a", Success: false, CreatedAt: now - 10},
		{ModelID: 1, ModelName: "gpt-test", ChannelID: 12, ChannelName: "openai-b", Success: false, CreatedAt: now - 20},
	}))

	result, err := ListModelMonitorModels(0, 10, 1, "", ModelMonitorQuery{EndTimestamp: now}, now)
	require.NoError(t, err)
	require.Len(t, result.Items, 1)
	require.Len(t, result.Items[0].Channels, 1)
	require.EqualValues(t, 11, result.Items[0].Channels[0].ChannelID)
	require.Len(t, result.Items[0].LatestTests, 1)
	require.EqualValues(t, 11, result.Items[0].LatestTests[0].ChannelID)
}

func TestListModelMonitorModelsReturnsEmptyLatestTestsArrayWhenNoCurrentChannels(t *testing.T) {
	setupModelMonitorTestDB(t)
	now := time.Now().Unix()

	require.NoError(t, model.DB.Create(&model.Model{Id: 1, ModelName: "gpt-test", Status: 1}).Error)
	require.NoError(t, model.InsertModelChannelTestResults([]model.ModelChannelTestResult{
		{ModelID: 1, ModelName: "gpt-test", ChannelID: 12, ChannelName: "removed-channel", Success: false, CreatedAt: now - 20},
	}))

	result, err := ListModelMonitorModels(0, 10, 1, "", ModelMonitorQuery{EndTimestamp: now}, now)
	require.NoError(t, err)
	require.Len(t, result.Items, 1)
	require.Empty(t, result.Items[0].Channels)
	require.NotNil(t, result.Items[0].LatestTests)
	require.Empty(t, result.Items[0].LatestTests)
}

func TestGetChannelModelErrorStatsAppliesThresholdAndWindow(t *testing.T) {
	setupModelMonitorTestDB(t)
	now := time.Now().Unix()

	logs := []model.Log{
		{CreatedAt: now - 60, Type: model.LogTypeError, ModelName: "gpt-test", ChannelId: 11, Content: "first failure"},
		{CreatedAt: now - 50, Type: model.LogTypeError, ModelName: "gpt-test", ChannelId: 11, Content: "second failure"},
		{CreatedAt: now - 40, Type: model.LogTypeConsume, ModelName: "gpt-test", ChannelId: 11, Content: "success is ignored"},
		{CreatedAt: now - 30, Type: model.LogTypeError, ModelName: "other-model", ChannelId: 11, Content: "other model failure"},
		{CreatedAt: now - 20, Type: model.LogTypeError, ModelName: "gpt-test", ChannelId: 12, Content: "other channel failure"},
		{CreatedAt: now - 3600, Type: model.LogTypeError, ModelName: "gpt-test", ChannelId: 11, Content: "old failure"},
	}
	require.NoError(t, model.LOG_DB.Create(&logs).Error)

	stats, err := GetChannelModelErrorStats(11, 2, 10*60, now)
	require.NoError(t, err)
	require.Len(t, stats, 1)
	require.Equal(t, "gpt-test", stats[0].ModelName)
	require.EqualValues(t, 2, stats[0].ErrorCount)
	require.EqualValues(t, 3, stats[0].TotalRequests)
	require.InDelta(t, 66.67, stats[0].ErrorRate, 0.01)
	require.Equal(t, "second failure", stats[0].LatestError)
	require.Equal(t, now-50, stats[0].LatestErrorAt)
}

func TestGetChannelModelErrorStatsCountsOnlyFinalRetryLogByRequestID(t *testing.T) {
	setupModelMonitorTestDB(t)
	now := time.Now().Unix()

	logs := []model.Log{
		{CreatedAt: now - 90, Type: model.LogTypeError, ModelName: "gpt-test", ChannelId: 11, Content: "retry failed first", Group: "default", RequestId: "req-success"},
		{CreatedAt: now - 80, Type: model.LogTypeConsume, ModelName: "gpt-test", ChannelId: 12, Content: "retry success", Group: "default", RequestId: "req-success"},
		{CreatedAt: now - 70, Type: model.LogTypeError, ModelName: "gpt-test", ChannelId: 11, Content: "retry failed first", Group: "default", RequestId: "req-fail"},
		{CreatedAt: now - 60, Type: model.LogTypeError, ModelName: "gpt-test", ChannelId: 11, Content: "retry failed last", Group: "default", RequestId: "req-fail"},
		{CreatedAt: now - 50, Type: model.LogTypeError, ModelName: "gpt-test", ChannelId: 11, Content: "single failure", Group: "default"},
	}
	require.NoError(t, model.LOG_DB.Create(&logs).Error)

	stats, err := GetChannelModelErrorStats(ChannelModelErrorStatsQuery{
		ChannelID:        11,
		Threshold:        2,
		WindowSeconds:    10 * 60,
		Now:              now,
		MinRequests:      2,
		ErrorRatePercent: 50,
	})
	require.NoError(t, err)
	require.Len(t, stats, 1)
	require.Equal(t, "gpt-test", stats[0].ModelName)
	require.EqualValues(t, 2, stats[0].TotalRequests)
	require.EqualValues(t, 2, stats[0].ErrorCount)
	require.InDelta(t, 100, stats[0].ErrorRate, 0.01)
	require.Equal(t, "single failure", stats[0].LatestError)
	require.Equal(t, now-50, stats[0].LatestErrorAt)
}

func TestGetChannelModelErrorStatsRequiresMinimumRequestsAndErrorRate(t *testing.T) {
	setupModelMonitorTestDB(t)
	now := time.Now().Unix()

	logs := []model.Log{
		{CreatedAt: now - 60, Type: model.LogTypeError, ModelName: "low-volume", ChannelId: 11, Content: "failure"},
		{CreatedAt: now - 50, Type: model.LogTypeError, ModelName: "low-rate", ChannelId: 11, Content: "first failure"},
		{CreatedAt: now - 45, Type: model.LogTypeError, ModelName: "low-rate", ChannelId: 11, Content: "second failure"},
		{CreatedAt: now - 44, Type: model.LogTypeConsume, ModelName: "low-rate", ChannelId: 11},
		{CreatedAt: now - 43, Type: model.LogTypeConsume, ModelName: "low-rate", ChannelId: 11},
		{CreatedAt: now - 42, Type: model.LogTypeConsume, ModelName: "low-rate", ChannelId: 11},
		{CreatedAt: now - 41, Type: model.LogTypeConsume, ModelName: "low-rate", ChannelId: 11},
		{CreatedAt: now - 40, Type: model.LogTypeError, ModelName: "bad-model", ChannelId: 11, Content: "first bad failure"},
		{CreatedAt: now - 35, Type: model.LogTypeError, ModelName: "bad-model", ChannelId: 11, Content: "second bad failure"},
		{CreatedAt: now - 30, Type: model.LogTypeConsume, ModelName: "bad-model", ChannelId: 11},
	}
	require.NoError(t, model.LOG_DB.Create(&logs).Error)

	stats, err := GetChannelModelErrorStats(ChannelModelErrorStatsQuery{
		ChannelID:        11,
		Threshold:        2,
		WindowSeconds:    10 * 60,
		Now:              now,
		MinRequests:      3,
		ErrorRatePercent: 50,
	})
	require.NoError(t, err)
	require.Len(t, stats, 1)
	require.Equal(t, "bad-model", stats[0].ModelName)
	require.EqualValues(t, 3, stats[0].TotalRequests)
	require.InDelta(t, 66.67, stats[0].ErrorRate, 0.01)
}

func TestCanDisableChannelForModelProtectsLastAvailableChannel(t *testing.T) {
	setupModelMonitorTestDB(t)

	require.NoError(t, model.DB.Create(&model.Channel{Id: 11, Name: "primary", Type: 1, Status: common.ChannelStatusEnabled, Models: "gpt-test", Group: "default"}).Error)
	require.NoError(t, model.DB.Create(&model.Ability{Group: "default", Model: "gpt-test", ChannelId: 11, Enabled: true}).Error)

	ok, err := CanDisableChannelForModel(11, "gpt-test", "default")
	require.NoError(t, err)
	require.False(t, ok)

	require.NoError(t, model.DB.Create(&model.Channel{Id: 12, Name: "secondary", Type: 1, Status: common.ChannelStatusEnabled, Models: "gpt-test", Group: "default"}).Error)
	require.NoError(t, model.DB.Create(&model.Ability{Group: "default", Model: "gpt-test", ChannelId: 12, Enabled: true}).Error)

	ok, err = CanDisableChannelForModel(11, "gpt-test", "default")
	require.NoError(t, err)
	require.True(t, ok)
}

func TestCanDisableWholeChannelProtectsEveryModel(t *testing.T) {
	setupModelMonitorTestDB(t)

	require.NoError(t, model.DB.Create(&model.Channel{Id: 11, Name: "primary", Type: 1, Status: common.ChannelStatusEnabled, Models: "gpt-test,solo-model", Group: "default"}).Error)
	require.NoError(t, model.DB.Create(&model.Ability{Group: "default", Model: "gpt-test", ChannelId: 11, Enabled: true}).Error)
	require.NoError(t, model.DB.Create(&model.Ability{Group: "default", Model: "solo-model", ChannelId: 11, Enabled: true}).Error)
	require.NoError(t, model.DB.Create(&model.Channel{Id: 12, Name: "secondary", Type: 1, Status: common.ChannelStatusEnabled, Models: "gpt-test", Group: "default"}).Error)
	require.NoError(t, model.DB.Create(&model.Ability{Group: "default", Model: "gpt-test", ChannelId: 12, Enabled: true}).Error)

	ok, missing, err := CanDisableWholeChannelPreservingModels(11)
	require.NoError(t, err)
	require.False(t, ok)
	require.Equal(t, []string{"default/solo-model"}, missing)

	require.NoError(t, model.DB.Create(&model.Channel{Id: 13, Name: "solo-secondary", Type: 1, Status: common.ChannelStatusEnabled, Models: "solo-model", Group: "default"}).Error)
	require.NoError(t, model.DB.Create(&model.Ability{Group: "default", Model: "solo-model", ChannelId: 13, Enabled: true}).Error)

	ok, missing, err = CanDisableWholeChannelPreservingModels(11)
	require.NoError(t, err)
	require.True(t, ok)
	require.Empty(t, missing)
}

func TestFindAutoEnableCandidateChannelsMatchesModelAndGroup(t *testing.T) {
	setupModelMonitorTestDB(t)

	lowPriority := int64(1)
	highPriority := int64(10)
	require.NoError(t, model.DB.Create(&model.Channel{Id: 11, Name: "failed", Type: 1, Status: common.ChannelStatusEnabled, Models: "gpt-test", Group: "default"}).Error)
	require.NoError(t, model.DB.Create(&model.Channel{Id: 12, Name: "candidate-low", Type: 1, Status: common.ChannelStatusAutoDisabled, Models: "gpt-test", Group: "default", Priority: &lowPriority}).Error)
	require.NoError(t, model.DB.Create(&model.Channel{Id: 13, Name: "wrong-model", Type: 1, Status: common.ChannelStatusAutoDisabled, Models: "other-model", Group: "default", Priority: &highPriority}).Error)
	require.NoError(t, model.DB.Create(&model.Channel{Id: 14, Name: "manual", Type: 1, Status: common.ChannelStatusManuallyDisabled, Models: "gpt-test", Group: "default", Priority: &highPriority}).Error)
	require.NoError(t, model.DB.Create(&model.Channel{Id: 15, Name: "wrong-group", Type: 1, Status: common.ChannelStatusAutoDisabled, Models: "gpt-test", Group: "vip", Priority: &highPriority}).Error)
	require.NoError(t, model.DB.Create(&model.Channel{Id: 16, Name: "candidate-high", Type: 1, Status: common.ChannelStatusAutoDisabled, Models: "gpt-test", Group: "default", Priority: &highPriority}).Error)

	candidates, err := FindAutoEnableCandidateChannels(AutoEnableCandidateQuery{
		FailedChannelID: 11,
		ModelName:       "gpt-test",
		Group:           "default",
	})
	require.NoError(t, err)
	require.Len(t, candidates, 2)
	require.EqualValues(t, 16, candidates[0].Id)
	require.EqualValues(t, 12, candidates[1].Id)
}

func TestChannelOperationRecordsAreStoredSeparatelyFromLogs(t *testing.T) {
	setupModelMonitorTestDB(t)

	require.NoError(t, model.RecordChannelOperation(model.ChannelOperationRecord{
		ChannelID:   11,
		ChannelName: "openai-a",
		Action:      model.ChannelOperationActionDisable,
		Source:      model.ChannelOperationSourceAuto,
		Reason:      "model errors reached threshold",
		Status:      3,
		ModelName:   "gpt-test",
	}))
	require.NoError(t, model.RecordChannelOperation(model.ChannelOperationRecord{
		ChannelID:   12,
		ChannelName: "openai-b",
		Action:      model.ChannelOperationActionEnable,
		Source:      model.ChannelOperationSourceAuto,
		Status:      1,
	}))

	items, total, err := model.GetChannelOperationRecords(model.ChannelOperationRecordQuery{
		ChannelID: 11,
		Action:    model.ChannelOperationActionDisable,
		Page:      1,
		PageSize:  20,
	})
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Len(t, items, 1)
	require.Equal(t, "openai-a", items[0].ChannelName)
	require.Equal(t, model.ChannelOperationActionDisable, items[0].Action)

	var logCount int64
	require.NoError(t, model.LOG_DB.Model(&model.Log{}).Count(&logCount).Error)
	require.Zero(t, logCount)
}

func TestChannelStatusChangeUsesDedicatedOperationRecords(t *testing.T) {
	setupModelMonitorTestDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.ChannelOperationRecord{}))
	require.NoError(t, model.DB.Create(&model.Channel{
		Id:     11,
		Name:   "openai-a",
		Type:   1,
		Status: 1,
		Models: "gpt-test",
		Group:  "default",
	}).Error)
	require.NoError(t, model.DB.Create(&model.Ability{
		Group:     "default",
		Model:     "gpt-test",
		ChannelId: 11,
		Enabled:   true,
	}).Error)

	DisableWholeChannel(11, "openai-a", "model errors reached threshold")

	items, total, err := model.GetChannelOperationRecords(model.ChannelOperationRecordQuery{
		ChannelID: 11,
		Page:      1,
		PageSize:  20,
	})
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Len(t, items, 1)
	require.Equal(t, model.ChannelOperationActionDisable, items[0].Action)
	require.Equal(t, model.ChannelOperationSourceAuto, items[0].Source)

	var logCount int64
	require.NoError(t, model.LOG_DB.Model(&model.Log{}).Count(&logCount).Error)
	require.Zero(t, logCount)
}

func setupChannelOperationRecordTestDB(t *testing.T) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.ChannelOperationRecord{}, &model.Log{}))

	originDB := model.DB
	originLogDB := model.LOG_DB
	model.DB = db
	model.LOG_DB = db
	t.Cleanup(func() {
		model.DB = originDB
		model.LOG_DB = originLogDB
	})
}

func TestGetChannelOperationRecordsFiltersByTimeActionAndChannel(t *testing.T) {
	setupChannelOperationRecordTestDB(t)
	now := time.Now().Unix()

	records := []model.ChannelOperationRecord{
		{ChannelID: 11, ChannelName: "openai-a", Action: model.ChannelOperationActionDisable, Source: model.ChannelOperationSourceAuto, Reason: "recent", Status: 3, CreatedAt: now - 60},
		{ChannelID: 11, ChannelName: "openai-a", Action: model.ChannelOperationActionEnable, Source: model.ChannelOperationSourceAuto, Reason: "enabled", Status: 1, CreatedAt: now - 50},
		{ChannelID: 12, ChannelName: "openai-b", Action: model.ChannelOperationActionDisable, Source: model.ChannelOperationSourceAuto, Reason: "other channel", Status: 3, CreatedAt: now - 40},
		{ChannelID: 11, ChannelName: "openai-a", Action: model.ChannelOperationActionDisable, Source: model.ChannelOperationSourceAuto, Reason: "old", Status: 3, CreatedAt: now - 3600},
	}
	require.NoError(t, model.DB.Create(&records).Error)

	items, total, err := model.GetChannelOperationRecords(model.ChannelOperationRecordQuery{
		ChannelID:      11,
		Action:         model.ChannelOperationActionDisable,
		StartTimestamp: now - 120,
		EndTimestamp:   now,
		Page:           1,
		PageSize:       20,
	})
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Len(t, items, 1)
	require.Equal(t, "recent", items[0].Reason)
}

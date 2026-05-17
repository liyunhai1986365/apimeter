package service

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupModelMonitorTestDB(t *testing.T) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Model{}, &model.Channel{}, &model.Ability{}, &model.Log{}, &model.ModelChannelTestResult{}))

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

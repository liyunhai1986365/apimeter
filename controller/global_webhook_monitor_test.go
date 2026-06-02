package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupGlobalWebhookControllerTestDB(t *testing.T) {
	t.Helper()

	originDB := model.DB
	originLogDB := model.LOG_DB
	originUsingSQLite := common.UsingSQLite
	originUsingMySQL := common.UsingMySQL
	originUsingPostgreSQL := common.UsingPostgreSQL
	originRedisEnabled := common.RedisEnabled

	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Log{}))

	model.DB = db
	model.LOG_DB = db
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.RedisEnabled = false
	model.InitColForTest()

	t.Cleanup(func() {
		model.DB = originDB
		model.LOG_DB = originLogDB
		common.UsingSQLite = originUsingSQLite
		common.UsingMySQL = originUsingMySQL
		common.UsingPostgreSQL = originUsingPostgreSQL
		common.RedisEnabled = originRedisEnabled
		model.InitColForTest()
	})
}

func TestShouldSendGlobalWebhookNotificationSuppressesSameFingerprintWithinWindow(t *testing.T) {
	globalWebhookNotifyState.Lock()
	globalWebhookNotifyState.lastFingerprint = ""
	globalWebhookNotifyState.lastNotifiedAt = 0
	globalWebhookNotifyState.Unlock()

	setting := &operation_setting.WebhookSetting{SuppressMinutes: 30}
	payload := service.NewGlobalWebhookPayload("New API 监控告警", []service.GlobalWebhookEvent{
		{Type: "model_error", ChannelID: 1, ChannelName: "openai-a", ModelName: "gpt-test", ErrorRequests: 3},
	}, 1700000000)

	require.True(t, shouldSendGlobalWebhookNotification(setting, payload, 1700000000))
	require.False(t, shouldSendGlobalWebhookNotification(setting, payload, 1700000100))

	changedPayload := service.NewGlobalWebhookPayload("New API 监控告警", []service.GlobalWebhookEvent{
		{Type: "model_error", ChannelID: 1, ChannelName: "openai-a", ModelName: "gpt-test", ErrorRequests: 4},
	}, 1700000200)
	require.True(t, shouldSendGlobalWebhookNotification(setting, changedPayload, 1700000200))
	require.True(t, shouldSendGlobalWebhookNotification(setting, payload, 1700002000))
}

func TestBuildGlobalWebhookTestPayloadUsesLatestErrorLog(t *testing.T) {
	setupGlobalWebhookControllerTestDB(t)

	require.NoError(t, model.DB.Create(&model.Channel{Id: 11, Name: "openai-a", Type: 1}).Error)
	require.NoError(t, model.LOG_DB.Create(&[]model.Log{
		{Id: 1, CreatedAt: 100, Type: model.LogTypeError, ChannelId: 11, ModelName: "gpt-old", Content: "old error", RequestId: "req-old"},
		{Id: 2, CreatedAt: 200, Type: model.LogTypeError, ChannelId: 11, ModelName: "gpt-test", Content: "latest timeout", RequestId: "req-latest", Other: `{"code":"timeout"}`},
		{Id: 3, CreatedAt: 300, Type: model.LogTypeConsume, ChannelId: 11, ModelName: "gpt-test", Content: "consume"},
	}).Error)

	payload, err := buildGlobalWebhookTestPayloadFromLatestError(1700000000)

	require.NoError(t, err)
	require.Equal(t, "New API Webhook 测试告警", payload.Title)
	require.Len(t, payload.Events, 1)
	event := payload.Events[0]
	require.Equal(t, "webhook_test_latest_error", event.Type)
	require.Equal(t, "openai-a", event.ChannelName)
	require.Equal(t, "gpt-test", event.ModelName)
	require.Equal(t, "latest timeout", event.Message)
	require.Equal(t, "timeout", event.ErrorCode)
	require.Equal(t, int64(200), event.CreatedAt)
	require.Equal(t, "req-latest", event.Extra["request_id"])
}

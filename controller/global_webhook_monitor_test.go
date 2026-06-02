package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/require"
)

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

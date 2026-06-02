package service

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/stretchr/testify/require"
)

func TestNewGlobalWebhookPayloadSummarizesEvents(t *testing.T) {
	threshold := 5.0
	balance := 3.2

	payload := NewGlobalWebhookPayload("New API 监控告警", []GlobalWebhookEvent{
		{Type: "channel_balance_low", ChannelID: 1, ChannelName: "openai-a", Balance: &balance, Threshold: &threshold},
		{Type: "channel_balance_error", ChannelID: 2, ChannelName: "openai-b", Message: "尚未实现"},
		{Type: "model_error", ChannelID: 3, ChannelName: "openai-c", ModelName: "gpt-test", ErrorRequests: 4},
		{Type: "channel_test_failed", ChannelID: 4, ChannelName: "openai-d", ModelName: "gpt-test", Message: "timeout"},
	}, 1700000000)

	require.Equal(t, "global_monitor", payload.Type)
	require.EqualValues(t, 1700000000, payload.Timestamp)
	require.Equal(t, 1, payload.Summary.LowBalanceChannels)
	require.Equal(t, 1, payload.Summary.BalanceCheckErrors)
	require.Equal(t, 1, payload.Summary.ModelErrorItems)
	require.Equal(t, 1, payload.Summary.FailedChannelTests)
	require.Len(t, payload.Events, 4)
}

func TestSendGlobalWebhookPayloadPostsStructuredJSONWithSignature(t *testing.T) {
	fetchSetting := system_setting.GetFetchSetting()
	originalSSRF := fetchSetting.EnableSSRFProtection
	fetchSetting.EnableSSRFProtection = false
	t.Cleanup(func() {
		fetchSetting.EnableSSRFProtection = originalSSRF
	})

	secret := "signing-secret"
	var receivedSignature string
	var receivedPayload GlobalWebhookPayload
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "application/json", r.Header.Get("Content-Type"))
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		receivedSignature = r.Header.Get("X-Webhook-Signature")
		require.NoError(t, common.Unmarshal(body, &receivedPayload))
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	payload := NewGlobalWebhookPayload("New API 监控告警", []GlobalWebhookEvent{{Type: "model_error", ChannelID: 1}}, 1700000000)
	require.NoError(t, SendGlobalWebhookPayload(server.URL, secret, payload))

	payloadBytes, err := common.Marshal(payload)
	require.NoError(t, err)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payloadBytes)
	require.Equal(t, hex.EncodeToString(mac.Sum(nil)), receivedSignature)
	require.Equal(t, "global_monitor", receivedPayload.Type)
	require.Equal(t, 1, receivedPayload.Summary.ModelErrorItems)
}

package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"unicode/utf8"

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

func TestIsWeComWebhookURLRecognizesEnterpriseWechatRobot(t *testing.T) {
	require.True(t, isWeComWebhookURL("https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=693a91f6-7xxx"))
	require.False(t, isWeComWebhookURL("https://qyapi.weixin.qq.com/cgi-bin/webhook/send"))
	require.False(t, isWeComWebhookURL("https://example.com/cgi-bin/webhook/send?key=693a91f6"))
}

func TestMarshalGlobalWebhookRequestBodyUsesWeComMarkdownFormat(t *testing.T) {
	threshold := 5.0
	balance := 3.2
	payload := NewGlobalWebhookPayload("New API 监控告警", []GlobalWebhookEvent{
		{Type: "channel_balance_low", ChannelID: 1, ChannelName: "openai-a", Balance: &balance, Threshold: &threshold},
		{Type: "model_error", ChannelID: 2, ChannelName: "openai-b", ModelName: "gpt-test", ErrorRequests: 3, TotalRequests: 5, ErrorRate: 60, Message: "upstream timeout"},
	}, 1700000000)

	body, err := marshalGlobalWebhookRequestBody("https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=693a91f6", payload)
	require.NoError(t, err)

	var got struct {
		MsgType  string `json:"msgtype"`
		Markdown struct {
			Content string `json:"content"`
		} `json:"markdown"`
	}
	require.NoError(t, common.Unmarshal(body, &got))
	require.Equal(t, "markdown", got.MsgType)
	require.Contains(t, got.Markdown.Content, "## New API 监控告警")
	require.Contains(t, got.Markdown.Content, "渠道余额低")
	require.Contains(t, got.Markdown.Content, "模型错误")
	require.LessOrEqual(t, len(got.Markdown.Content), weComMarkdownMaxBytes)
}

func TestMarshalWeComWebhookPayloadIncludesLatestErrorTestEvent(t *testing.T) {
	payload := NewGlobalWebhookPayload("New API Webhook 测试告警", []GlobalWebhookEvent{
		{
			Type:        "webhook_test_latest_error",
			ChannelID:   12,
			ChannelName: "openai-a",
			ModelName:   "gpt-test",
			ErrorCode:   "timeout",
			Message:     "upstream timeout",
			CreatedAt:   1700000000,
			Extra: map[string]interface{}{
				"request_id": "req-123",
			},
		},
	}, 1700000100)

	body, err := marshalGlobalWebhookRequestBody("https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=693a91f6", payload)
	require.NoError(t, err)

	var got struct {
		MsgType  string `json:"msgtype"`
		Markdown struct {
			Content string `json:"content"`
		} `json:"markdown"`
	}
	require.NoError(t, common.Unmarshal(body, &got))
	require.Equal(t, "markdown", got.MsgType)
	require.Contains(t, got.Markdown.Content, "测试发送")
	require.Contains(t, got.Markdown.Content, "openai-a (#12)")
	require.Contains(t, got.Markdown.Content, "gpt-test")
	require.Contains(t, got.Markdown.Content, "timeout")
	require.Contains(t, got.Markdown.Content, "req-123")
}

func TestBuildWeComMarkdownContentTruncatesToDocumentLimit(t *testing.T) {
	events := make([]GlobalWebhookEvent, 0, 40)
	for i := 0; i < 40; i++ {
		events = append(events, GlobalWebhookEvent{
			Type:          "model_error",
			ChannelID:     i + 1,
			ChannelName:   "channel",
			ModelName:     "gpt-test",
			ErrorRequests: 10,
			TotalRequests: 20,
			ErrorRate:     50,
			Message:       strings.Repeat("错误", 300),
		})
	}
	payload := NewGlobalWebhookPayload("New API 监控告警", events, 1700000000)

	content := buildWeComMarkdownContent(payload)

	require.LessOrEqual(t, len(content), weComMarkdownMaxBytes)
	require.True(t, utf8.ValidString(content))
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

func TestSendGlobalWebhookPayloadPostsWeComMarkdownPayload(t *testing.T) {
	fetchSetting := system_setting.GetFetchSetting()
	originalSSRF := fetchSetting.EnableSSRFProtection
	fetchSetting.EnableSSRFProtection = false

	originalClient := httpClient
	t.Cleanup(func() {
		fetchSetting.EnableSSRFProtection = originalSSRF
		httpClient = originalClient
	})

	var receivedPayload struct {
		MsgType  string `json:"msgtype"`
		Markdown struct {
			Content string `json:"content"`
		} `json:"markdown"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "qyapi.weixin.qq.com", r.Host)
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.NoError(t, common.Unmarshal(body, &receivedPayload))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"errcode":0,"errmsg":"ok"}`))
	}))
	defer server.Close()

	serverURL, err := url.Parse(server.URL)
	require.NoError(t, err)
	httpClient = &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network string, addr string) (net.Conn, error) {
				return net.Dial(network, serverURL.Host)
			},
		},
	}

	payload := NewGlobalWebhookPayload("New API 监控告警", []GlobalWebhookEvent{{Type: "channel_test_failed", ChannelID: 1, ChannelName: "openai-a", ModelName: "gpt-test", Message: "timeout"}}, 1700000000)
	err = SendGlobalWebhookPayload("http://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=693a91f6", "", payload)
	require.NoError(t, err)
	require.Equal(t, "markdown", receivedPayload.MsgType)
	require.Contains(t, receivedPayload.Markdown.Content, "渠道测试失败")
}

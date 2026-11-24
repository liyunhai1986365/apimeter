package service

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/stretchr/testify/require"
)

const testWeComWebhookURL = "https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=test-key"

type webhookTestTransport func(*http.Request) (*http.Response, error)

func (transport webhookTestTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return transport(req)
}

// Intercept every outbound request, including Worker requests, without opening
// sockets or contacting a real notification recipient.
func mockWebhookDelivery(t *testing.T, worker bool, status int, response string, inspect func(string, map[string]string, []byte)) {
	t.Helper()
	fetchSetting := system_setting.GetFetchSetting()
	originalSSRF := fetchSetting.EnableSSRFProtection
	originalClient, originalProtectedClient := httpClient, ssrfProtectedHTTPClient
	originalWorkerURL, originalWorkerKey := system_setting.WorkerUrl, system_setting.WorkerValidKey
	t.Cleanup(func() {
		fetchSetting.EnableSSRFProtection = originalSSRF
		httpClient, ssrfProtectedHTTPClient = originalClient, originalProtectedClient
		system_setting.WorkerUrl, system_setting.WorkerValidKey = originalWorkerURL, originalWorkerKey
	})
	fetchSetting.EnableSSRFProtection = false
	system_setting.WorkerUrl = ""
	if worker {
		system_setting.WorkerUrl = "https://worker.example.com"
		system_setting.WorkerValidKey = "worker-test-key"
	}
	mockClient := func(viaWorker bool) *http.Client {
		return &http.Client{Transport: webhookTestTransport(func(req *http.Request) (*http.Response, error) {
			require.Equal(t, http.MethodPost, req.Method)
			require.Equal(t, "application/json", req.Header.Get("Content-Type"))
			body, err := io.ReadAll(req.Body)
			require.NoError(t, err)
			targetURL := req.URL.String()
			headers := map[string]string{"X-Webhook-Signature": req.Header.Get("X-Webhook-Signature")}
			if viaWorker {
				require.Equal(t, "https://worker.example.com/", targetURL)
				var envelope WorkerRequest
				require.NoError(t, common.Unmarshal(body, &envelope))
				require.Equal(t, "worker-test-key", envelope.Key)
				require.Equal(t, http.MethodPost, envelope.Method)
				require.Equal(t, "application/json", envelope.Headers["Content-Type"])
				targetURL, headers, body = envelope.URL, envelope.Headers, envelope.Body
			}
			if inspect != nil {
				inspect(targetURL, headers, body)
			}
			return &http.Response{
				StatusCode: status,
				Body:       io.NopCloser(strings.NewReader(response)),
				Header:     make(http.Header),
			}, nil
		})}
	}
	httpClient = mockClient(worker)
	ssrfProtectedHTTPClient = mockClient(false)
}

func TestSendWebhookNotifyUsesWeComMarkdown(t *testing.T) {
	for _, mode := range []string{"direct", "worker"} {
		t.Run(mode, func(t *testing.T) {
			delivered := false
			mockWebhookDelivery(t, mode == "worker", http.StatusOK, `{"errcode":0,"errmsg":"ok"}`, func(targetURL string, _ map[string]string, body []byte) {
				delivered = true
				require.Equal(t, testWeComWebhookURL, targetURL)
				var payload weComMarkdownPayload
				require.NoError(t, common.Unmarshal(body, &payload))
				require.Equal(t, "markdown", payload.MsgType)
				require.Equal(t, "## 您的额度即将用尽\n\n当前余额 5 美元，消耗 50%。\n[立即充值](https://example.com/wallet?a=1&b=2)", payload.Markdown.Content)
			})
			err := SendWebhookNotify("  "+testWeComWebhookURL+"  ", "", dto.NewNotify(
				dto.NotifyTypeQuotaExceed, "您的额度即将用尽",
				"当前余额 {{value}}，消耗 50%。<br/><a href='{{value}}'>立即充值</a>",
				[]interface{}{"5 美元", "https://example.com/wallet?a=1&amp;b=2"},
			), "")
			require.NoError(t, err)
			require.True(t, delivered)
		})
	}
}

func TestWeComWebhookReceiptSizeLimit(t *testing.T) {
	const ok = `{"errcode":0}`
	atLimit := ok + strings.Repeat(" ", weComReceiptMaxBytes-len(ok))
	for _, tc := range []struct {
		name  string
		extra int
	}{
		{"at_limit", 0},
		{"one_over", 1},
		{"stops_reading", 32},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := strings.NewReader(atLimit + strings.Repeat(" ", tc.extra))
			err := validateWeComWebhookResponse(&http.Response{Body: io.NopCloser(body), ContentLength: -1})
			if tc.extra == 0 {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, "wecom webhook response exceeds")
			require.Equal(t, tc.extra-1, body.Len())
		})
	}
}

func TestWebhookDeliveryRejectsWeComFailures(t *testing.T) {
	for _, sender := range []string{"user", "global"} {
		for _, mode := range []string{"direct", "worker"} {
			for _, test := range []struct {
				name     string
				status   int
				response string
				wantErr  string
			}{
				{"business_error", http.StatusOK, `{"errcode":93000,"errmsg":"invalid webhook url"}`, "errcode=93000 errmsg=invalid webhook url"},
				{"missing_code", http.StatusOK, `{"errmsg":"ok"}`, "missing errcode"},
				{"null_code", http.StatusOK, `{"errcode":null}`, "missing errcode"},
				{"invalid_json", http.StatusOK, `<html>gateway error</html>`, "failed to decode wecom webhook response"},
				{"http_error", http.StatusTooManyRequests, `{"errcode":0}`, "status code: 429"},
			} {
				t.Run(sender+"/"+mode+"/"+test.name, func(t *testing.T) {
					mockWebhookDelivery(t, mode == "worker", test.status, test.response, nil)
					var err error
					if sender == "user" {
						err = SendWebhookNotify(testWeComWebhookURL, "", dto.NewNotify(dto.NotifyTypeChannelUpdate, "渠道已停用", "timeout", nil), "")
					} else {
						err = SendGlobalWebhookPayload(testWeComWebhookURL, "", NewGlobalWebhookPayload("监控告警", nil, 1700000000), "")
					}
					require.ErrorContains(t, err, test.wantErr)
				})
			}
		}
	}
}

func TestSendWebhookNotifyPreservesGenericPayloadAndSignature(t *testing.T) {
	for _, mode := range []string{"direct", "worker"} {
		t.Run(mode, func(t *testing.T) {
			secret := "test-signing-secret"
			delivered := false
			mockWebhookDelivery(t, mode == "worker", http.StatusNoContent, "", func(targetURL string, headers map[string]string, body []byte) {
				delivered = true
				require.Equal(t, "https://example.com/webhook", targetURL)
				var payload WebhookPayload
				require.NoError(t, common.Unmarshal(body, &payload))
				require.Equal(t, dto.NotifyTypeQuotaExceed, payload.Type)
				require.Equal(t, "余额提醒", payload.Title)
				require.Equal(t, "余额：$5，进度：50%<br/>请充值", payload.Content)
				require.Equal(t, []interface{}{"$5"}, payload.Values)
				require.Positive(t, payload.Timestamp)
				mac := hmac.New(sha256.New, []byte(secret))
				_, err := mac.Write(body)
				require.NoError(t, err)
				require.Equal(t, hex.EncodeToString(mac.Sum(nil)), headers["X-Webhook-Signature"])
			})
			err := SendWebhookNotify("https://example.com/webhook", secret, dto.NewNotify(dto.NotifyTypeQuotaExceed, "余额提醒", "余额：{{value}}，进度：50%<br/>请充值", []interface{}{"$5"}), "")
			require.NoError(t, err)
			require.True(t, delivered)
		})
	}
}

func TestWeComUserNotificationLimitsUTF8Content(t *testing.T) {
	body, err := marshalUserWebhookRequestBody(testWeComWebhookURL, WebhookPayload{
		Title:   "渠道\n告警",
		Content: strings.Repeat("首 Token 超时，需要检查上游。", 1000),
	}, "")
	require.NoError(t, err)
	var payload weComMarkdownPayload
	require.NoError(t, common.Unmarshal(body, &payload))
	require.LessOrEqual(t, len(payload.Markdown.Content), weComMarkdownMaxBytes)
	require.True(t, utf8.ValidString(payload.Markdown.Content))
	require.True(t, strings.HasPrefix(payload.Markdown.Content, "## 渠道 告警\n\n"))
	require.Contains(t, payload.Markdown.Content, "内容已截断")
}

func TestWeComNotificationMarkdownPreservesReadableDetails(t *testing.T) {
	for _, test := range []struct {
		name    string
		content string
		want    string
	}{
		{"plain_error", "上游错误 <nil>，耗时 < 30 秒\n重试 50%", "上游错误 <nil>，耗时 < 30 秒\n重试 50%"},
		{"html_template", "<p>检查 <strong>A &amp; B</strong></p><ul><li>超时</li><li>失败</li></ul>", "检查 A & B\n\n\n- 超时\n\n- 失败"},
		{"ignore_scripts", "<style>body{color:red}</style>超时<script>alert('x')</script>", "超时"},
		{"unsupported_link", "<a href='javascript:alert(1)'>详情</a>", "详情"},
	} {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.want, weComNotificationMarkdown(test.content))
		})
	}
}

func TestWebhookExplicitFormatOverridesURL(t *testing.T) {
	for _, sender := range []string{"user", "global"} {
		for _, mode := range []string{"direct", "worker"} {
			for _, format := range []string{dto.WebhookFormatGeneric, dto.WebhookFormatWeCom} {
				t.Run(sender+"/"+mode+"/"+format, func(t *testing.T) {
					webhookURL := testWeComWebhookURL
					response := "accepted"
					if format == dto.WebhookFormatWeCom {
						webhookURL = "https://notifications.example.com/robot"
						response = `{"errcode":0,"errmsg":"ok"}`
					}
					delivered := false
					mockWebhookDelivery(t, mode == "worker", http.StatusOK, response, func(targetURL string, headers map[string]string, body []byte) {
						delivered = true
						require.Equal(t, webhookURL, targetURL)
						var payload map[string]interface{}
						require.NoError(t, common.Unmarshal(body, &payload))
						if format == dto.WebhookFormatWeCom {
							require.Equal(t, "markdown", payload["msgtype"])
							require.NotContains(t, payload, "type")
							require.Empty(t, headers["X-Webhook-Signature"])
							require.Empty(t, headers["Authorization"])
						} else {
							require.Contains(t, payload, "type")
							require.NotContains(t, payload, "msgtype")
							require.NotEmpty(t, headers["X-Webhook-Signature"])
						}
					})
					var err error
					if sender == "user" {
						err = SendWebhookNotify(webhookURL, "existing-secret", dto.NewNotify(dto.NotifyTypeChannelUpdate, "渠道通知", "timeout", nil), format)
					} else {
						err = SendGlobalWebhookPayload(webhookURL, "existing-secret", NewGlobalWebhookPayload("监控告警", nil, 1700000000), format)
					}
					require.NoError(t, err)
					require.True(t, delivered)
				})
			}
		}
	}
}

func TestWebhookRejectsUnknownFormatBeforeSending(t *testing.T) {
	mockWebhookDelivery(t, false, http.StatusOK, "", func(_ string, _ map[string]string, _ []byte) {
		t.Fatal("invalid format must not send a notification")
	})
	require.ErrorContains(t, SendWebhookNotify(testWeComWebhookURL, "", dto.Notify{}, "invalid"), "invalid webhook format")
	require.ErrorContains(t, SendGlobalWebhookPayload(testWeComWebhookURL, "", GlobalWebhookPayload{}, "invalid"), "invalid webhook format")
}

func TestChannelOperationNotificationUsesConfiguredWebhookFormat(t *testing.T) {
	setting := operation_setting.GetWebhookSetting()
	original := *setting
	t.Cleanup(func() { *setting = original })
	setting.Enabled = true
	setting.URL = "https://notifications.example.com/robot"
	setting.Format = dto.WebhookFormatWeCom
	delivered := false
	mockWebhookDelivery(t, false, http.StatusOK, `{"errcode":0}`, func(_ string, _ map[string]string, body []byte) {
		delivered = true
		var payload weComMarkdownPayload
		require.NoError(t, common.Unmarshal(body, &payload))
		require.Equal(t, "markdown", payload.MsgType)
		require.Contains(t, payload.Markdown.Content, "自动禁用")
	})
	require.NoError(t, SendGlobalWebhookChannelOperationRecord(model.ChannelOperationRecord{
		Action:      model.ChannelOperationActionDisable,
		Source:      model.ChannelOperationSourceAuto,
		ChannelID:   1,
		ChannelName: "test-channel",
		Reason:      "timeout",
	}))
	require.True(t, delivered)
}

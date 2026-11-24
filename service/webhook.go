package service

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
)

// WebhookPayload webhook 通知的负载数据
type WebhookPayload struct {
	Type      string        `json:"type"`
	Title     string        `json:"title"`
	Content   string        `json:"content"`
	Values    []interface{} `json:"values,omitempty"`
	Timestamp int64         `json:"timestamp"`
}

// generateSignature 生成 webhook 签名
func generateSignature(secret string, payload []byte) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write(payload)
	return hex.EncodeToString(h.Sum(nil))
}

// SendWebhookNotify 发送 webhook 通知
func SendWebhookNotify(webhookURL string, secret string, data dto.Notify, format string) error {
	webhookURL = strings.TrimSpace(webhookURL)
	format, err := resolveWebhookFormat(webhookURL, format)
	if err != nil {
		return err
	}
	// 处理占位符
	content := data.Content
	for _, value := range data.Values {
		content = strings.Replace(content, dto.ContentValueParam, fmt.Sprint(value), 1)
	}

	// 构建 webhook 负载
	payload := WebhookPayload{
		Type:      data.Type,
		Title:     data.Title,
		Content:   content,
		Values:    data.Values,
		Timestamp: time.Now().Unix(),
	}

	// 序列化负载
	payloadBytes, err := marshalUserWebhookRequestBody(webhookURL, payload, format)
	if err != nil {
		return fmt.Errorf("failed to marshal webhook payload: %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, webhookURL, bytes.NewReader(payloadBytes))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if secret != "" && format == dto.WebhookFormatGeneric {
		req.Header.Set("X-Webhook-Signature", generateSignature(secret, payloadBytes))
	}
	return sendPersonalNotification(req, format)
}

func marshalUserWebhookRequestBody(webhookURL string, payload WebhookPayload, format string) ([]byte, error) {
	format, err := resolveWebhookFormat(webhookURL, format)
	if err != nil {
		return nil, err
	}
	if format == dto.WebhookFormatGeneric {
		return common.Marshal(payload)
	}
	content := "## " + truncateSingleLine(payload.Title, 200) + "\n\n" + weComNotificationMarkdown(payload.Content)
	return marshalWeComMarkdown(content)
}

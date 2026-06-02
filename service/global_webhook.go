package service

import (
	"bytes"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/system_setting"
)

type GlobalWebhookPayload struct {
	Type      string                 `json:"type"`
	Title     string                 `json:"title"`
	Timestamp int64                  `json:"timestamp"`
	Summary   GlobalWebhookSummary   `json:"summary"`
	Events    []GlobalWebhookEvent   `json:"events"`
	Meta      map[string]interface{} `json:"meta,omitempty"`
}

type GlobalWebhookSummary struct {
	LowBalanceChannels int `json:"low_balance_channels"`
	BalanceCheckErrors int `json:"balance_check_errors"`
	ModelErrorItems    int `json:"model_error_items"`
	FailedChannelTests int `json:"failed_channel_tests"`
}

type GlobalWebhookEvent struct {
	Type          string                 `json:"type"`
	ChannelID     int                    `json:"channel_id,omitempty"`
	ChannelName   string                 `json:"channel_name,omitempty"`
	ChannelType   int                    `json:"channel_type,omitempty"`
	ModelName     string                 `json:"model_name,omitempty"`
	Balance       *float64               `json:"balance,omitempty"`
	Threshold     *float64               `json:"threshold,omitempty"`
	ErrorCode     string                 `json:"error_code,omitempty"`
	Message       string                 `json:"message,omitempty"`
	TotalRequests int64                  `json:"total_requests,omitempty"`
	ErrorRequests int64                  `json:"error_requests,omitempty"`
	ErrorRate     float64                `json:"error_rate,omitempty"`
	LatestErrorAt int64                  `json:"latest_error_at,omitempty"`
	ResponseTime  float64                `json:"response_time,omitempty"`
	CreatedAt     int64                  `json:"created_at,omitempty"`
	Extra         map[string]interface{} `json:"extra,omitempty"`
}

func NewGlobalWebhookPayload(title string, events []GlobalWebhookEvent, now int64) GlobalWebhookPayload {
	if now <= 0 {
		now = time.Now().Unix()
	}
	payload := GlobalWebhookPayload{
		Type:      "global_monitor",
		Title:     title,
		Timestamp: now,
		Events:    events,
	}
	for _, event := range events {
		switch event.Type {
		case "channel_balance_low":
			payload.Summary.LowBalanceChannels++
		case "channel_balance_error":
			payload.Summary.BalanceCheckErrors++
		case "model_error":
			payload.Summary.ModelErrorItems++
		case "channel_test_failed":
			payload.Summary.FailedChannelTests++
		}
	}
	return payload
}

func SendGlobalWebhookPayload(webhookURL string, secret string, payload GlobalWebhookPayload) error {
	webhookURL = strings.TrimSpace(webhookURL)
	if webhookURL == "" {
		return fmt.Errorf("webhook url is empty")
	}
	payloadBytes, err := common.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal global webhook payload: %v", err)
	}

	if system_setting.EnableWorker() {
		workerReq := &WorkerRequest{
			URL:    webhookURL,
			Key:    system_setting.WorkerValidKey,
			Method: http.MethodPost,
			Headers: map[string]string{
				"Content-Type": "application/json",
			},
			Body: payloadBytes,
		}
		if secret != "" {
			signature := generateSignature(secret, payloadBytes)
			workerReq.Headers["X-Webhook-Signature"] = signature
			workerReq.Headers["Authorization"] = "Bearer " + secret
		}
		resp, err := DoWorkerRequest(workerReq)
		if err != nil {
			return fmt.Errorf("failed to send global webhook request through worker: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return fmt.Errorf("global webhook request failed with status code: %d", resp.StatusCode)
		}
		return nil
	}

	fetchSetting := system_setting.GetFetchSetting()
	if err := common.ValidateURLWithFetchSetting(webhookURL, fetchSetting.EnableSSRFProtection, fetchSetting.AllowPrivateIp, fetchSetting.DomainFilterMode, fetchSetting.IpFilterMode, fetchSetting.DomainList, fetchSetting.IpList, fetchSetting.AllowedPorts, fetchSetting.ApplyIPFilterForDomain); err != nil {
		return fmt.Errorf("request reject: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, webhookURL, bytes.NewBuffer(payloadBytes))
	if err != nil {
		return fmt.Errorf("failed to create global webhook request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if secret != "" {
		req.Header.Set("X-Webhook-Signature", generateSignature(secret, payloadBytes))
	}
	client := GetHttpClient()
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send global webhook request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("global webhook request failed with status code: %d", resp.StatusCode)
	}
	return nil
}

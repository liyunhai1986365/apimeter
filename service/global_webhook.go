package service

import (
	"bytes"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"
)

const weComMarkdownMaxBytes = 4096

type GlobalWebhookPayload struct {
	Type      string                 `json:"type"`
	Title     string                 `json:"title"`
	Timestamp int64                  `json:"timestamp"`
	Summary   GlobalWebhookSummary   `json:"summary"`
	Events    []GlobalWebhookEvent   `json:"events"`
	Meta      map[string]interface{} `json:"meta,omitempty"`
}

type GlobalWebhookSummary struct {
	ModelErrorItems      int `json:"model_error_items"`
	FailedChannelTests   int `json:"failed_channel_tests"`
	AutoDisabledChannels int `json:"auto_disabled_channels"`
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
		case "model_error":
			payload.Summary.ModelErrorItems++
		case "channel_test_failed":
			payload.Summary.FailedChannelTests++
		case "channel_auto_disabled":
			payload.Summary.AutoDisabledChannels++
		}
	}
	return payload
}

func SendGlobalWebhookChannelOperationRecord(record model.ChannelOperationRecord) error {
	if record.Action != model.ChannelOperationActionDisable || record.Source != model.ChannelOperationSourceAuto {
		return nil
	}
	setting := operation_setting.GetWebhookSetting()
	if !setting.Enabled || strings.TrimSpace(setting.URL) == "" {
		return nil
	}
	now := common.GetTimestamp()
	if record.CreatedAt > 0 {
		now = record.CreatedAt
	}
	createdAt := now
	event := GlobalWebhookEvent{
		Type:          "channel_auto_disabled",
		ChannelID:     record.ChannelID,
		ChannelName:   record.ChannelName,
		ModelName:     record.ModelName,
		Message:       strings.TrimSpace(record.Reason),
		TotalRequests: record.TotalRequests,
		ErrorRequests: record.ErrorRequests,
		ErrorRate:     record.ErrorRate,
		CreatedAt:     createdAt,
		Extra: map[string]interface{}{
			"operation_record_id": record.Id,
			"status":              record.Status,
			"target_channel_id":   record.TargetChannelID,
			"operation_group_id":  record.OperationGroupID,
			"window_seconds":      record.WindowSeconds,
			"success":             record.Success,
			"extra":               record.Extra,
		},
	}
	payload := NewGlobalWebhookPayload("New API 自动禁用告警", []GlobalWebhookEvent{event}, now)
	payload.Meta = map[string]interface{}{
		"node_name": common.NodeName,
		"version":   common.Version,
	}
	return SendGlobalWebhookPayload(setting.URL, setting.Secret, payload)
}

func SendGlobalWebhookPayload(webhookURL string, secret string, payload GlobalWebhookPayload) error {
	webhookURL = strings.TrimSpace(webhookURL)
	if webhookURL == "" {
		return fmt.Errorf("webhook url is empty")
	}
	payloadBytes, err := marshalGlobalWebhookRequestBody(webhookURL, payload)
	if err != nil {
		return fmt.Errorf("failed to marshal global webhook payload: %v", err)
	}
	isWeComWebhook := isWeComWebhookURL(webhookURL)

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
		if isWeComWebhook {
			return validateWeComWebhookResponse(resp)
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
	if isWeComWebhook {
		return validateWeComWebhookResponse(resp)
	}
	return nil
}

func marshalGlobalWebhookRequestBody(webhookURL string, payload GlobalWebhookPayload) ([]byte, error) {
	if isWeComWebhookURL(webhookURL) {
		return marshalWeComWebhookPayload(payload)
	}
	return common.Marshal(payload)
}

func isWeComWebhookURL(webhookURL string) bool {
	parsed, err := url.Parse(strings.TrimSpace(webhookURL))
	if err != nil {
		return false
	}
	return strings.EqualFold(parsed.Hostname(), "qyapi.weixin.qq.com") &&
		parsed.Path == "/cgi-bin/webhook/send" &&
		strings.TrimSpace(parsed.Query().Get("key")) != ""
}

type weComMarkdownPayload struct {
	MsgType  string `json:"msgtype"`
	Markdown struct {
		Content string `json:"content"`
	} `json:"markdown"`
}

func marshalWeComWebhookPayload(payload GlobalWebhookPayload) ([]byte, error) {
	weComPayload := weComMarkdownPayload{MsgType: "markdown"}
	weComPayload.Markdown.Content = buildWeComMarkdownContent(payload)
	return common.Marshal(weComPayload)
}

func buildWeComMarkdownContent(payload GlobalWebhookPayload) string {
	lines := []string{
		fmt.Sprintf("## %s", nonEmpty(payload.Title, "New API 监控告警")),
		fmt.Sprintf(">时间：<font color=\"comment\">%s</font>", time.Unix(payload.Timestamp, 0).Format("2006-01-02 15:04:05")),
		fmt.Sprintf(">自动禁用：<font color=\"warning\">%d</font>，模型错误：<font color=\"warning\">%d</font>", payload.Summary.AutoDisabledChannels, payload.Summary.ModelErrorItems),
		fmt.Sprintf(">渠道测试失败：<font color=\"warning\">%d</font>", payload.Summary.FailedChannelTests),
	}
	if len(payload.Events) == 0 {
		lines = append(lines, "", "无告警事件。")
		return truncateUTF8ByBytes(strings.Join(lines, "\n"), weComMarkdownMaxBytes)
	}

	lines = append(lines, "", "**事件明细**")
	for i, event := range payload.Events {
		if i >= 20 {
			lines = append(lines, fmt.Sprintf("- 其余 %d 条事件已省略", len(payload.Events)-i))
			break
		}
		lines = append(lines, "- "+formatWeComEventLine(event))
	}
	return truncateUTF8ByBytes(strings.Join(lines, "\n"), weComMarkdownMaxBytes)
}

func formatWeComEventLine(event GlobalWebhookEvent) string {
	channel := strings.TrimSpace(event.ChannelName)
	if channel == "" && event.ChannelID > 0 {
		channel = fmt.Sprintf("#%d", event.ChannelID)
	} else if channel != "" && event.ChannelID > 0 {
		channel = fmt.Sprintf("%s (#%d)", channel, event.ChannelID)
	}
	if channel == "" {
		channel = "unknown"
	}

	switch event.Type {
	case "model_error":
		return fmt.Sprintf("模型错误：<font color=\"warning\">%s</font> / `%s`，错误 %d/%d，错误率 %.2f%%，最近错误：%s", channel, nonEmpty(event.ModelName, "-"), event.ErrorRequests, event.TotalRequests, event.ErrorRate, truncateSingleLine(event.Message, 160))
	case "channel_test_failed":
		return fmt.Sprintf("渠道测试失败：<font color=\"warning\">%s</font> / `%s`，%s", channel, nonEmpty(event.ModelName, "-"), truncateSingleLine(event.Message, 160))
	case "channel_auto_disabled":
		detail := fmt.Sprintf("自动禁用：<font color=\"warning\">%s</font>", channel)
		if strings.TrimSpace(event.ModelName) != "" {
			detail += fmt.Sprintf(" / `%s`", strings.TrimSpace(event.ModelName))
		}
		if event.TotalRequests > 0 || event.ErrorRequests > 0 {
			detail += fmt.Sprintf("，错误 %d/%d，错误率 %.2f%%", event.ErrorRequests, event.TotalRequests, event.ErrorRate)
		}
		if strings.TrimSpace(event.Message) != "" {
			detail += fmt.Sprintf("，原因：%s", truncateSingleLine(event.Message, 160))
		}
		return detail
	case "webhook_test_latest_error":
		requestID := ""
		if event.Extra != nil {
			requestID = strings.TrimSpace(fmt.Sprint(event.Extra["request_id"]))
		}
		detail := fmt.Sprintf("测试发送：<font color=\"warning\">%s</font> / `%s`，错误码 `%s`，最近错误：%s", channel, nonEmpty(event.ModelName, "-"), nonEmpty(event.ErrorCode, "-"), truncateSingleLine(event.Message, 160))
		if requestID != "" {
			detail += fmt.Sprintf("，请求 ID `%s`", truncateSingleLine(requestID, 80))
		}
		return detail
	default:
		return fmt.Sprintf("%s：%s", event.Type, truncateSingleLine(event.Message, 180))
	}
}

func truncateSingleLine(value string, maxRunes int) string {
	value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	if maxRunes <= 0 {
		return value
	}
	runes := []rune(value)
	if len(runes) <= maxRunes {
		return value
	}
	return string(runes[:maxRunes]) + "..."
}

func truncateUTF8ByBytes(value string, maxBytes int) string {
	if maxBytes <= 0 || len(value) <= maxBytes {
		return value
	}
	suffix := "\n...（内容已截断）"
	limit := maxBytes - len(suffix)
	if limit <= 0 {
		return ""
	}
	truncated := value[:limit]
	for !utf8.ValidString(truncated) && len(truncated) > 0 {
		truncated = truncated[:len(truncated)-1]
	}
	return truncated + suffix
}

func nonEmpty(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}

func validateWeComWebhookResponse(resp *http.Response) error {
	var result struct {
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
	}
	if err := common.DecodeJson(resp.Body, &result); err != nil {
		return fmt.Errorf("failed to decode wecom webhook response: %v", err)
	}
	if result.ErrCode != 0 {
		return fmt.Errorf("wecom webhook request failed: errcode=%d errmsg=%s", result.ErrCode, result.ErrMsg)
	}
	return nil
}

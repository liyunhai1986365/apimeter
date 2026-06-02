package controller

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
)

const (
	globalWebhookMonitorDefaultIntervalMinutes = 30
	globalWebhookMonitorMinIntervalMinutes     = 1
	globalWebhookMonitorMaxEvents              = 60
)

var (
	globalWebhookMonitorTaskOnce    sync.Once
	globalWebhookMonitorTaskRunning atomic.Bool
	globalWebhookNotifyState        = struct {
		sync.Mutex
		lastFingerprint string
		lastNotifiedAt  int64
	}{}
)

func StartGlobalWebhookMonitorTask() {
	globalWebhookMonitorTaskOnce.Do(func() {
		if !common.IsMasterNode {
			return
		}
		go func() {
			for {
				setting := operation_setting.GetWebhookSetting()
				if !setting.Enabled {
					time.Sleep(time.Minute)
					continue
				}
				interval := setting.IntervalMinutes
				if interval < globalWebhookMonitorMinIntervalMinutes {
					interval = globalWebhookMonitorDefaultIntervalMinutes
				}
				time.Sleep(time.Duration(interval) * time.Minute)
				runGlobalWebhookMonitorTaskOnce()
			}
		}()
	})
}

func runGlobalWebhookMonitorTaskOnce() {
	if !globalWebhookMonitorTaskRunning.CompareAndSwap(false, true) {
		return
	}
	defer globalWebhookMonitorTaskRunning.Store(false)

	setting := operation_setting.GetWebhookSetting()
	if !setting.Enabled || strings.TrimSpace(setting.URL) == "" {
		return
	}

	events := collectGlobalWebhookMonitorEvents(setting)
	if len(events) == 0 && !setting.NotifyOnEmptyResult {
		return
	}

	now := common.GetTimestamp()
	payload := service.NewGlobalWebhookPayload("New API 监控告警", events, now)
	payload.Meta = map[string]interface{}{
		"node_name": common.NodeName,
		"version":   common.Version,
	}
	if !shouldSendGlobalWebhookNotification(setting, payload, now) {
		common.SysLog("global webhook monitor notification skipped by suppress window")
		return
	}
	if err := service.SendGlobalWebhookPayload(setting.URL, setting.Secret, payload); err != nil {
		common.SysLog(fmt.Sprintf("failed to send global webhook monitor payload: %v", err))
		return
	}
	common.SysLog(fmt.Sprintf("global webhook monitor notification sent: events=%d", len(events)))
}

func collectGlobalWebhookMonitorEvents(setting *operation_setting.WebhookSetting) []service.GlobalWebhookEvent {
	events := make([]service.GlobalWebhookEvent, 0)
	if setting.BalanceCheckEnabled {
		events = append(events, collectGlobalWebhookBalanceEvents(setting)...)
	}
	if setting.ModelErrorCheckEnabled {
		events = append(events, collectGlobalWebhookModelErrorEvents(setting)...)
	}
	if setting.ChannelTestCheckEnabled {
		events = append(events, collectGlobalWebhookChannelTestEvents(setting)...)
	}
	if len(events) > globalWebhookMonitorMaxEvents {
		return events[:globalWebhookMonitorMaxEvents]
	}
	return events
}

func collectGlobalWebhookBalanceEvents(setting *operation_setting.WebhookSetting) []service.GlobalWebhookEvent {
	channels, err := model.GetAllChannels(0, 0, true, false)
	if err != nil {
		common.SysLog(fmt.Sprintf("global webhook balance query channels failed: %v", err))
		return []service.GlobalWebhookEvent{{
			Type:    "channel_balance_error",
			Message: err.Error(),
		}}
	}
	events := make([]service.GlobalWebhookEvent, 0)
	threshold := setting.BalanceThreshold
	for _, channel := range channels {
		if channel == nil || channel.Status != common.ChannelStatusEnabled || channel.ChannelInfo.IsMultiKey {
			continue
		}
		balance, err := updateChannelBalance(channel)
		if err != nil {
			events = append(events, service.GlobalWebhookEvent{
				Type:        "channel_balance_error",
				ChannelID:   channel.Id,
				ChannelName: channel.Name,
				ChannelType: channel.Type,
				Message:     err.Error(),
			})
		} else if balance <= threshold {
			balanceValue := balance
			thresholdValue := threshold
			events = append(events, service.GlobalWebhookEvent{
				Type:        "channel_balance_low",
				ChannelID:   channel.Id,
				ChannelName: channel.Name,
				ChannelType: channel.Type,
				Balance:     &balanceValue,
				Threshold:   &thresholdValue,
			})
		}
		if common.RequestInterval > 0 {
			time.Sleep(common.RequestInterval)
		}
		if len(events) >= globalWebhookMonitorMaxEvents {
			return events
		}
	}
	return events
}

func collectGlobalWebhookModelErrorEvents(setting *operation_setting.WebhookSetting) []service.GlobalWebhookEvent {
	if setting.ModelErrorWindowMinutes <= 0 || setting.ModelErrorThreshold <= 0 {
		return []service.GlobalWebhookEvent{}
	}
	channels, err := model.GetAllChannels(0, 0, true, false)
	if err != nil {
		common.SysLog(fmt.Sprintf("global webhook model error query channels failed: %v", err))
		return []service.GlobalWebhookEvent{{
			Type:    "model_error",
			Message: err.Error(),
		}}
	}
	events := make([]service.GlobalWebhookEvent, 0)
	now := common.GetTimestamp()
	for _, channel := range channels {
		if channel == nil || channel.Status != common.ChannelStatusEnabled {
			continue
		}
		stats, err := service.GetChannelModelErrorStats(service.ChannelModelErrorStatsQuery{
			ChannelID:        channel.Id,
			Threshold:        setting.ModelErrorThreshold,
			WindowSeconds:    int64(setting.ModelErrorWindowMinutes) * 60,
			Now:              now,
			MinRequests:      setting.ModelErrorMinRequests,
			ErrorRatePercent: setting.ModelErrorRate,
		})
		if err != nil {
			common.SysLog(fmt.Sprintf("global webhook model error stats failed: channel_id=%d err=%v", channel.Id, err))
			continue
		}
		for _, stat := range stats {
			events = append(events, service.GlobalWebhookEvent{
				Type:          "model_error",
				ChannelID:     channel.Id,
				ChannelName:   channel.Name,
				ChannelType:   channel.Type,
				ModelName:     stat.ModelName,
				Message:       stat.LatestError,
				TotalRequests: stat.TotalRequests,
				ErrorRequests: stat.ErrorCount,
				ErrorRate:     stat.ErrorRate,
				LatestErrorAt: stat.LatestErrorAt,
			})
			if len(events) >= globalWebhookMonitorMaxEvents {
				return events
			}
		}
	}
	return events
}

func collectGlobalWebhookChannelTestEvents(setting *operation_setting.WebhookSetting) []service.GlobalWebhookEvent {
	windowMinutes := setting.ChannelTestWindowMinutes
	if windowMinutes <= 0 {
		windowMinutes = 60
	}
	since := common.GetTimestamp() - int64(windowMinutes)*60
	var results []model.ModelChannelTestResult
	err := model.DB.Model(&model.ModelChannelTestResult{}).
		Where("success = ? AND created_at >= ?", false, since).
		Order("created_at DESC, id DESC").
		Limit(globalWebhookMonitorMaxEvents).
		Find(&results).Error
	if err != nil {
		common.SysLog(fmt.Sprintf("global webhook channel test query failed: %v", err))
		return []service.GlobalWebhookEvent{{
			Type:    "channel_test_failed",
			Message: err.Error(),
		}}
	}
	events := make([]service.GlobalWebhookEvent, 0, len(results))
	for _, result := range results {
		events = append(events, service.GlobalWebhookEvent{
			Type:         "channel_test_failed",
			ChannelID:    result.ChannelID,
			ChannelName:  result.ChannelName,
			ChannelType:  result.ChannelType,
			ModelName:    result.ModelName,
			ErrorCode:    result.ErrorCode,
			Message:      result.Message,
			ResponseTime: result.ResponseTime,
			CreatedAt:    result.CreatedAt,
		})
	}
	return events
}

func shouldSendGlobalWebhookNotification(setting *operation_setting.WebhookSetting, payload service.GlobalWebhookPayload, now int64) bool {
	fingerprint := buildGlobalWebhookFingerprint(payload)
	suppressSeconds := int64(setting.SuppressMinutes) * 60
	if suppressSeconds < 0 {
		suppressSeconds = 0
	}

	globalWebhookNotifyState.Lock()
	defer globalWebhookNotifyState.Unlock()
	if suppressSeconds > 0 &&
		globalWebhookNotifyState.lastNotifiedAt > 0 &&
		now-globalWebhookNotifyState.lastNotifiedAt < suppressSeconds &&
		globalWebhookNotifyState.lastFingerprint == fingerprint {
		return false
	}
	globalWebhookNotifyState.lastNotifiedAt = now
	globalWebhookNotifyState.lastFingerprint = fingerprint
	return true
}

func buildGlobalWebhookFingerprint(payload service.GlobalWebhookPayload) string {
	builder := strings.Builder{}
	builder.WriteString(fmt.Sprintf("%d:%d:%d:%d",
		payload.Summary.LowBalanceChannels,
		payload.Summary.BalanceCheckErrors,
		payload.Summary.ModelErrorItems,
		payload.Summary.FailedChannelTests,
	))
	for _, event := range payload.Events {
		builder.WriteString(fmt.Sprintf("|%s:%d:%s:%s:%s:%d:%d",
			event.Type,
			event.ChannelID,
			event.ChannelName,
			event.ModelName,
			event.ErrorCode,
			event.ErrorRequests,
			event.CreatedAt,
		))
	}
	return builder.String()
}

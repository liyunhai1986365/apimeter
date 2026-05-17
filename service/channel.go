package service

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"
)

func formatNotifyType(channelId int, status int) string {
	return fmt.Sprintf("%s_%d_%d", dto.NotifyTypeChannelUpdate, channelId, status)
}

func formatChannelStatusChangeContent(channelName string, channelId int, action string, reason string) string {
	content := fmt.Sprintf("渠道状态变更：通道「%s」（#%d）已被%s", channelName, channelId, action)
	if strings.TrimSpace(reason) != "" {
		content += fmt.Sprintf("，原因：%s", reason)
	}
	return content
}

type ChannelStatusChangeRecordMeta struct {
	TargetChannelID  int
	OperationGroupID string
	WindowSeconds    int64
	TotalRequests    int64
	ErrorRequests    int64
	ErrorRate        float64
	Success          *bool
	Extra            string
}

func recordChannelStatusChangeLog(channelName string, channelId int, action string, reason string, modelName string) string {
	success := true
	return recordChannelStatusChangeLogWithMeta(channelName, channelId, action, reason, modelName, ChannelStatusChangeRecordMeta{Success: &success})
}

func recordChannelStatusChangeLogWithMeta(channelName string, channelId int, action string, reason string, modelName string, meta ChannelStatusChangeRecordMeta) string {
	content := formatChannelStatusChangeContent(channelName, channelId, action, reason)
	recordAction := model.ChannelOperationActionDisable
	status := common.ChannelStatusAutoDisabled
	if action == "启用" {
		recordAction = model.ChannelOperationActionEnable
		status = common.ChannelStatusEnabled
	}
	if err := model.RecordChannelOperation(model.ChannelOperationRecord{
		ChannelID:        channelId,
		ChannelName:      channelName,
		Action:           recordAction,
		Source:           model.ChannelOperationSourceAuto,
		Status:           status,
		Reason:           reason,
		ModelName:        modelName,
		TargetChannelID:  meta.TargetChannelID,
		OperationGroupID: meta.OperationGroupID,
		WindowSeconds:    meta.WindowSeconds,
		TotalRequests:    meta.TotalRequests,
		ErrorRequests:    meta.ErrorRequests,
		ErrorRate:        meta.ErrorRate,
		Success:          meta.Success == nil || *meta.Success,
		Extra:            meta.Extra,
	}); err != nil {
		common.SysLog(fmt.Sprintf("failed to record channel operation: channel_id=%d, action=%s, error=%v", channelId, recordAction, err))
	}
	return content
}

// disable & notify
func DisableChannel(channelError types.ChannelError, reason string) {
	common.SysLog(fmt.Sprintf("通道「%s」（#%d）发生错误，准备禁用，原因：%s", channelError.ChannelName, channelError.ChannelId, reason))

	// 检查是否启用自动禁用功能
	if !channelError.AutoBan {
		common.SysLog(fmt.Sprintf("通道「%s」（#%d）未启用自动禁用功能，跳过禁用操作", channelError.ChannelName, channelError.ChannelId))
		return
	}

	success := model.UpdateChannelStatus(channelError.ChannelId, channelError.UsingKey, common.ChannelStatusAutoDisabled, reason)
	if success {
		subject := fmt.Sprintf("通道「%s」（#%d）已被禁用", channelError.ChannelName, channelError.ChannelId)
		content := recordChannelStatusChangeLog(channelError.ChannelName, channelError.ChannelId, "禁用", reason, "")
		NotifyRootUser(formatNotifyType(channelError.ChannelId, common.ChannelStatusAutoDisabled), subject, content)
	}
}

func EnableChannel(channelId int, usingKey string, channelName string) {
	EnableChannelForModel(channelId, usingKey, channelName, "", ChannelStatusChangeRecordMeta{})
}

func EnableChannelForModel(channelId int, usingKey string, channelName string, modelName string, meta ChannelStatusChangeRecordMeta) {
	success := model.UpdateChannelStatus(channelId, usingKey, common.ChannelStatusEnabled, "")
	if success {
		subject := fmt.Sprintf("通道「%s」（#%d）已被启用", channelName, channelId)
		content := recordChannelStatusChangeLogWithMeta(channelName, channelId, "启用", "", modelName, meta)
		NotifyRootUser(formatNotifyType(channelId, common.ChannelStatusEnabled), subject, content)
	}
}

func DisableWholeChannel(channelId int, channelName string, reason string) {
	DisableWholeChannelForModel(channelId, channelName, reason, "")
}

func DisableWholeChannelForModel(channelId int, channelName string, reason string, modelName string) {
	DisableWholeChannelForModelWithMeta(channelId, channelName, reason, modelName, ChannelStatusChangeRecordMeta{})
}

func DisableWholeChannelForModelWithMeta(channelId int, channelName string, reason string, modelName string, meta ChannelStatusChangeRecordMeta) {
	common.SysLog(fmt.Sprintf("通道「%s」（#%d）触发自动运营禁用，原因：%s", channelName, channelId, reason))
	success := model.UpdateWholeChannelStatus(channelId, common.ChannelStatusAutoDisabled, reason)
	if success {
		subject := fmt.Sprintf("通道「%s」（#%d）已被禁用", channelName, channelId)
		content := recordChannelStatusChangeLogWithMeta(channelName, channelId, "禁用", reason, modelName, meta)
		NotifyRootUser(formatNotifyType(channelId, common.ChannelStatusAutoDisabled), subject, content)
	}
}

func ShouldDisableChannel(err *types.NewAPIError) bool {
	if !common.AutomaticDisableChannelEnabled {
		return false
	}
	if err == nil {
		return false
	}
	if types.IsChannelError(err) {
		return true
	}
	if types.IsSkipRetryError(err) {
		return false
	}
	if operation_setting.ShouldDisableByStatusCode(err.StatusCode) {
		return true
	}

	lowerMessage := strings.ToLower(err.Error())
	search, _ := AcSearch(lowerMessage, operation_setting.AutomaticDisableKeywords, true)
	return search
}

func ShouldEnableChannel(newAPIError *types.NewAPIError, status int) bool {
	if !common.AutomaticEnableChannelEnabled {
		return false
	}
	if newAPIError != nil {
		return false
	}
	if status != common.ChannelStatusAutoDisabled {
		return false
	}
	return true
}

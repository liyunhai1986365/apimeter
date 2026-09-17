package controller

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay"
	"github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/tidwall/gjson"
)

func UpdateVideoTaskAll(ctx context.Context, platform constant.TaskPlatform, taskChannelM map[int][]string, taskM map[string]*model.Task) error {
	for channelId, taskIds := range taskChannelM {
		if err := updateVideoTaskAll(ctx, platform, channelId, taskIds, taskM); err != nil {
			logger.LogError(ctx, fmt.Sprintf("Channel #%d failed to update video async tasks: %s", channelId, err.Error()))
		}
	}
	return nil
}

func updateVideoTaskAll(ctx context.Context, platform constant.TaskPlatform, channelId int, taskIds []string, taskM map[string]*model.Task) error {
	logger.LogInfo(ctx, fmt.Sprintf("Channel #%d pending video tasks: %d", channelId, len(taskIds)))
	if len(taskIds) == 0 {
		return nil
	}
	cacheGetChannel, err := model.CacheGetChannel(channelId)
	if err != nil {
		errUpdate := model.TaskBulkUpdate(taskIds, map[string]any{
			"fail_reason": fmt.Sprintf("Failed to get channel info, channel ID: %d", channelId),
			"status":      "FAILURE",
			"progress":    "100%",
		})
		if errUpdate != nil {
			common.SysLog(fmt.Sprintf("UpdateVideoTask error: %v", errUpdate))
		}
		return fmt.Errorf("CacheGetChannel failed: %w", err)
	}
	adaptor := relay.GetTaskAdaptor(platform)
	if adaptor == nil {
		return fmt.Errorf("video adaptor not found")
	}
	info := &relaycommon.RelayInfo{}
	info.ChannelMeta = &relaycommon.ChannelMeta{
		ChannelBaseUrl: cacheGetChannel.GetBaseURL(),
	}
	info.ApiKey = cacheGetChannel.Key
	adaptor.Init(info)
	for _, taskId := range taskIds {
		if err := updateVideoSingleTask(ctx, adaptor, cacheGetChannel, taskId, taskM); err != nil {
			logger.LogError(ctx, fmt.Sprintf("Failed to update video task %s: %s", taskId, err.Error()))
		}
	}
	return nil
}

func updateVideoSingleTask(ctx context.Context, adaptor channel.TaskAdaptor, channel *model.Channel, taskId string, taskM map[string]*model.Task) error {
	baseURL := constant.GetChannelBaseURL(channel.Type)
	if channel.GetBaseURL() != "" {
		baseURL = channel.GetBaseURL()
	}
	proxy := channel.GetSetting().Proxy

	task := taskM[taskId]
	if task == nil {
		logger.LogError(ctx, fmt.Sprintf("Task %s not found in taskM", taskId))
		return fmt.Errorf("task %s not found", taskId)
	}
	key := channel.Key

	privateData := task.PrivateData
	if privateData.Key != "" {
		key = privateData.Key
	}
	resp, err := adaptor.FetchTask(baseURL, key, map[string]any{
		"task_id": taskId,
		"action":  task.Action,
	}, proxy)
	if err != nil {
		return fmt.Errorf("fetchTask failed for task %s: %w", taskId, err)
	}
	//if resp.StatusCode != http.StatusOK {
	//return fmt.Errorf("get Video Task status code: %d", resp.StatusCode)
	//}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("readAll failed for task %s: %w", taskId, err)
	}

	if isSeedanceTaskChannel(channel) {
		if resp.StatusCode < 200 || resp.StatusCode >= 300 || !gjson.ValidBytes(responseBody) {
			return fmt.Errorf("Seedance upstream query unavailable (HTTP %d)", resp.StatusCode)
		}
		if statusErr := relaycommon.ValidateSeedanceTaskStatusForAdaptor(adaptor, responseBody); statusErr != nil {
			return statusErr
		}
		result, parseErr := adaptor.ParseTaskResult(responseBody)
		if parseErr != nil || result == nil {
			return fmt.Errorf("invalid Seedance task response")
		}
		if identityErr := relaycommon.ValidateSeedanceTaskIdentityForAdaptor(adaptor, responseBody, result, task.GetUpstreamTaskID(), task.PrivateData.OfficialTaskID); identityErr != nil {
			return identityErr
		}
		task.Data = responseBody
		return updateSeedanceVideoTask(ctx, adaptor, task, result)
	}

	logger.LogDebug(ctx, "UpdateVideoSingleTask response: %s", responseBody)

	taskResult := &relaycommon.TaskInfo{}
	// try parse as New API response format
	var responseItems dto.TaskResponse[model.Task]
	if err = common.Unmarshal(responseBody, &responseItems); err == nil && responseItems.IsSuccess() {
		logger.LogDebug(ctx, "UpdateVideoSingleTask parsed as new api response format: %+v", responseItems)
		t := responseItems.Data
		taskResult.TaskID = t.TaskID
		taskResult.Status = string(t.Status)
		taskResult.Url = t.FailReason
		taskResult.Progress = t.Progress
		taskResult.Reason = t.FailReason
		task.Data = t.Data
	} else if taskResult, err = adaptor.ParseTaskResult(responseBody); err != nil {
		return fmt.Errorf("parseTaskResult failed for task %s: %w", taskId, err)
	} else {
		task.Data = redactVideoResponseBody(responseBody)
	}

	logger.LogDebug(ctx, "UpdateVideoSingleTask taskResult: %+v", taskResult)

	now := time.Now().Unix()
	if taskResult.Status == "" {
		//return fmt.Errorf("task %s status is empty", taskId)
		taskResult = relaycommon.FailTaskInfo("upstream returned empty status")
	}

	// 记录原本的状态，防止重复退款
	shouldRefund := false
	quota := task.Quota
	preStatus := task.Status

	task.Status = model.TaskStatus(taskResult.Status)
	switch taskResult.Status {
	case model.TaskStatusSubmitted:
		task.Progress = "10%"
	case model.TaskStatusQueued:
		task.Progress = "20%"
	case model.TaskStatusInProgress:
		task.Progress = "30%"
		if task.StartTime == 0 {
			task.StartTime = now
		}
	case model.TaskStatusSuccess:
		task.Progress = "100%"
		if task.FinishTime == 0 {
			task.FinishTime = now
		}
		if !(len(taskResult.Url) > 5 && taskResult.Url[:5] == "data:") {
			task.FailReason = taskResult.Url
		}

		// 如果配置了表达式计费，优先按冻结的表达式和请求上下文重新计费。
		if service.RecalculateTaskQuotaByTieredExpr(ctx, task, taskResult) {
			break
		}
		// 如果返回了 total_tokens 并且配置了模型倍率(非固定价格),则按提交时冻结的计费上下文重新计费
		if taskResult.TotalTokens > 0 {
			service.RecalculateTaskQuotaByTokens(ctx, task, taskResult.TotalTokens)
		} else {
			service.RecalculateTaskQuota(ctx, task, task.Quota, "任务完成，按预扣额度结算")
		}
	case model.TaskStatusFailure:
		logger.LogJson(ctx, fmt.Sprintf("Task %s failed", taskId), task)
		task.Status = model.TaskStatusFailure
		task.Progress = "100%"
		if task.FinishTime == 0 {
			task.FinishTime = now
		}
		task.FailReason = taskResult.Reason
		logger.LogInfo(ctx, fmt.Sprintf("Task %s failed: %s", task.TaskID, task.FailReason))
		taskResult.Progress = "100%"
		if quota != 0 {
			if preStatus != model.TaskStatusFailure {
				shouldRefund = true
			} else {
				logger.LogWarn(ctx, fmt.Sprintf("Task %s already in failure status, skip refund", task.TaskID))
			}
		}
	default:
		return fmt.Errorf("unknown task status %s for task %s", taskResult.Status, taskId)
	}
	if taskResult.Progress != "" {
		task.Progress = taskResult.Progress
	}
	if err := task.Update(); err != nil {
		common.SysLog("UpdateVideoTask task error: " + err.Error())
		shouldRefund = false
	}

	if shouldRefund {
		// 任务失败且之前状态不是失败才退还额度，防止重复退还。
		// 统一退款入口同时处理钱包/订阅、令牌额度和代理收益冲减。
		service.RefundTaskQuota(ctx, task, task.FailReason)
	}

	return nil
}

func redactVideoResponseBody(body []byte) []byte {
	var m map[string]any
	if err := common.Unmarshal(body, &m); err != nil {
		return body
	}
	resp, _ := m["response"].(map[string]any)
	if resp != nil {
		delete(resp, "bytesBase64Encoded")
		if v, ok := resp["video"].(string); ok {
			resp["video"] = truncateBase64(v)
		}
		if vs, ok := resp["videos"].([]any); ok {
			for i := range vs {
				if vm, ok := vs[i].(map[string]any); ok {
					delete(vm, "bytesBase64Encoded")
				}
			}
		}
	}
	b, err := common.Marshal(m)
	if err != nil {
		return body
	}
	return b
}

func truncateBase64(s string) string {
	const maxKeep = 256
	if len(s) <= maxKeep {
		return s
	}
	return s[:maxKeep] + "..."
}

// Seedance foreground queries and background polling must claim the same
// transition before billing. Stale workers cannot overwrite a completed task.
func isSeedanceTaskChannel(ch *model.Channel) bool {
	if ch.Type == constant.ChannelTypeVolcEngine || ch.Type == constant.ChannelTypeDoubaoVideo {
		return true
	}
	settings := ch.GetSetting()
	return ch.Type == constant.ChannelTypeConfigurable && settings.Protocol != nil && relaycommon.IsSeedanceVideoProfile(settings.Protocol.ProfileID)
}

func updateSeedanceVideoTask(ctx context.Context, adaptor channel.TaskAdaptor, task *model.Task, result *relaycommon.TaskInfo) error {
	previous := task.Status
	if previous == model.TaskStatusSuccess || previous == model.TaskStatusFailure {
		return nil
	}
	switch result.Status {
	case model.TaskStatusSubmitted, model.TaskStatusQueued, model.TaskStatusInProgress, model.TaskStatusSuccess, model.TaskStatusFailure:
	default:
		return fmt.Errorf("unknown Seedance task status %q", result.Status)
	}
	task.Status = model.TaskStatus(result.Status)
	task.Progress = result.Progress
	if result.Url != "" {
		task.PrivateData.ResultURL = result.Url
	}
	now := time.Now().Unix()
	if task.Status == model.TaskStatusInProgress && task.StartTime == 0 {
		task.StartTime = now
	}
	terminal := task.Status == model.TaskStatusSuccess || task.Status == model.TaskStatusFailure
	if terminal {
		task.Progress = "100%"
		if task.FinishTime == 0 {
			task.FinishTime = now
		}
	}
	if task.Status == model.TaskStatusFailure {
		task.FailReason = result.Reason
	}
	won, err := task.UpdateWithStatus(previous)
	if err != nil || !won {
		return err
	}
	if task.Status == model.TaskStatusSuccess {
		service.SettleTaskBillingOnComplete(ctx, adaptor, task, result)
	}
	if task.Status == model.TaskStatusFailure && task.Quota != 0 {
		service.RefundTaskQuota(ctx, task, task.FailReason)
	}
	return nil
}

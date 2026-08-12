package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	agentservice "github.com/QuantumNous/new-api/service/agent"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

// LogTaskConsumption 记录任务消费日志和统计信息（仅记录，不涉及实际扣费）。
// 实际扣费已由 BillingSession（PreConsumeBilling + SettleBilling）完成。
func LogTaskConsumption(c *gin.Context, info *relaycommon.RelayInfo) {
	tokenName := c.GetString("token_name")
	logContent := fmt.Sprintf("操作 %s", info.Action)
	// 支持任务仅按次计费
	if info.PriceData.UsePrice || common.StringsContains(constant.TaskPricePatches, info.OriginModelName) {
		logContent = fmt.Sprintf("%s，按次计费", logContent)
	} else {
		if len(info.PriceData.OtherRatios) > 0 {
			var contents []string
			for key, ra := range info.PriceData.OtherRatios {
				if 1.0 != ra {
					contents = append(contents, fmt.Sprintf("%s: %.2f", key, ra))
				}
			}
			if len(contents) > 0 {
				logContent = fmt.Sprintf("%s, 计算参数：%s", logContent, strings.Join(contents, ", "))
			}
		}
	}
	other := make(map[string]interface{})
	agentservice.UpdateBillingSnapshotQuota(info.AgentBillingSnapshot, info.PriceData.Quota)
	other["is_task"] = true
	other["request_path"] = c.Request.URL.Path
	other["model_price"] = info.PriceData.ModelPrice
	if info.PriceData.ModelRatio > 0 {
		other["model_ratio"] = info.PriceData.ModelRatio
	}
	other["group_ratio"] = info.PriceData.GroupRatioInfo.GroupRatio
	InjectTieredBillingSnapshotInfo(other, info.TieredBillingSnapshot, nil)
	if info.PriceData.GroupRatioInfo.HasSpecialRatio {
		other["user_group_ratio"] = info.PriceData.GroupRatioInfo.GroupSpecialRatio
	}
	if info.IsModelMapped {
		other["is_model_mapped"] = true
		other["upstream_model_name"] = info.UpstreamModelName
	}
	appendChannelCostInfo(other, info, info.PriceData.Quota)
	logID := model.RecordConsumeLog(c, info.UserId, model.RecordConsumeLogParams{
		Force:     info.AgentBillingSnapshot != nil,
		ChannelId: info.ChannelId,
		ModelName: info.OriginModelName,
		TokenName: tokenName,
		Quota:     info.PriceData.Quota,
		Content:   logContent,
		TokenId:   info.TokenId,
		Group:     info.UsingGroup,
		Other:     other,
	})
	if info.AgentBillingSnapshot != nil {
		if err := agentservice.SettleConsume(info.AgentBillingSnapshot, info.UserId, logID, info.PriceData.Quota); err != nil {
			logger.LogError(c, "error settling agent ledger: "+err.Error())
		}
	}
	model.UpdateUserUsedQuotaAndRequestCount(info.UserId, info.PriceData.Quota)
	model.UpdateChannelUsedQuota(info.ChannelId, info.PriceData.Quota)
}

// ---------------------------------------------------------------------------
// 异步任务计费辅助函数
// ---------------------------------------------------------------------------

// resolveTokenKey 通过 TokenId 运行时获取令牌 Key（用于 Redis 缓存操作）。
// 如果令牌已被删除或查询失败，返回空字符串。
func resolveTokenKey(ctx context.Context, tokenId int, taskID string) string {
	token, err := model.GetTokenById(tokenId)
	if err != nil {
		logger.LogWarn(ctx, fmt.Sprintf("获取令牌 key 失败 (tokenId=%d, task=%s): %s", tokenId, taskID, err.Error()))
		return ""
	}
	return token.Key
}

// taskIsSubscription 判断任务是否通过订阅计费。
func taskIsSubscription(task *model.Task) bool {
	return task.PrivateData.BillingSource == BillingSourceSubscription && task.PrivateData.SubscriptionId > 0
}

// taskAdjustFunding 调整任务的资金来源（钱包或订阅），delta > 0 表示扣费，delta < 0 表示退还。
func taskAdjustFunding(task *model.Task, delta int) error {
	if taskIsSubscription(task) {
		return model.PostConsumeUserSubscriptionDelta(task.PrivateData.SubscriptionId, int64(delta))
	}
	if delta > 0 {
		return model.DecreaseUserQuota(task.UserId, delta, false)
	}
	return model.IncreaseUserQuota(task.UserId, -delta, false)
}

// taskAdjustTokenQuota 调整任务的令牌额度，delta > 0 表示扣费，delta < 0 表示退还。
// 需要通过 resolveTokenKey 运行时获取 key（不从 PrivateData 中读取）。
func taskAdjustTokenQuota(ctx context.Context, task *model.Task, delta int) {
	if task.PrivateData.TokenId <= 0 || delta == 0 {
		return
	}
	tokenKey := resolveTokenKey(ctx, task.PrivateData.TokenId, task.TaskID)
	if tokenKey == "" {
		return
	}
	var err error
	if delta > 0 {
		err = model.DecreaseTokenQuota(task.PrivateData.TokenId, tokenKey, delta)
	} else {
		err = model.IncreaseTokenQuota(task.PrivateData.TokenId, tokenKey, -delta)
	}
	if err != nil {
		logger.LogWarn(ctx, fmt.Sprintf("调整令牌额度失败 (delta=%d, task=%s): %s", delta, task.TaskID, err.Error()))
	}
}

// taskBillingOther 从 task 的 BillingContext 构建日志 Other 字段。
func taskBillingOther(task *model.Task) map[string]interface{} {
	other := make(map[string]interface{})
	if bc := task.PrivateData.BillingContext; bc != nil {
		other["model_price"] = bc.ModelPrice
		if bc.ModelRatio > 0 {
			other["model_ratio"] = bc.ModelRatio
		}
		other["group_ratio"] = bc.GroupRatio
		if len(bc.OtherRatios) > 0 {
			for k, v := range bc.OtherRatios {
				other[k] = v
			}
		}
		if snap := bc.TieredBillingSnapshot; snap != nil {
			InjectTieredBillingSnapshotInfo(other, snap, nil)
			other["estimated_prompt_tokens"] = snap.EstimatedPromptTokens
			other["estimated_completion_tokens"] = snap.EstimatedCompletionTokens
			other["estimated_quota_before_group"] = snap.EstimatedQuotaBeforeGroup
			other["estimated_quota_after_group"] = snap.EstimatedQuotaAfterGroup
		}
	}
	props := task.Properties
	if props.UpstreamModelName != "" && props.UpstreamModelName != props.OriginModelName {
		other["is_model_mapped"] = true
		other["upstream_model_name"] = props.UpstreamModelName
	}
	return other
}

// taskModelName 从 BillingContext 或 Properties 中获取模型名称。
func taskModelName(task *model.Task) string {
	if bc := task.PrivateData.BillingContext; bc != nil && bc.OriginModelName != "" {
		return bc.OriginModelName
	}
	return task.Properties.OriginModelName
}

// RefundTaskQuota 统一的任务失败退款逻辑。
// 当异步任务失败时，将预扣的 quota 退还给用户（支持钱包和订阅），并退还令牌额度。
func RefundTaskQuota(ctx context.Context, task *model.Task, reason string) {
	quota := task.Quota
	if quota == 0 {
		return
	}

	// 1. 退还资金来源（钱包或订阅）
	if err := taskAdjustFunding(task, -quota); err != nil {
		logger.LogWarn(ctx, fmt.Sprintf("退还资金来源失败 task %s: %s", task.TaskID, err.Error()))
		return
	}

	// 2. 退还令牌额度
	taskAdjustTokenQuota(ctx, task, -quota)

	// 3. 记录日志
	other := taskBillingOther(task)
	other["task_id"] = task.TaskID
	other["reason"] = reason
	other["refund_quota"] = quota
	other["pre_consumed_quota"] = quota
	other["actual_quota"] = 0
	other["actual_total_tokens"] = 0
	other["actual_completion_tokens"] = 0
	bc := task.PrivateData.BillingContext
	settlesAgentRefund := !task.PrivateData.AsyncImage && bc != nil && bc.AgentBillingSnapshot != nil
	logID := model.RecordTaskBillingLog(model.RecordTaskBillingLogParams{
		Force:     settlesAgentRefund,
		UserId:    task.UserId,
		LogType:   model.LogTypeRefund,
		Content:   "",
		ChannelId: task.ChannelId,
		ModelName: taskModelName(task),
		Quota:     quota,
		TokenId:   task.PrivateData.TokenId,
		Group:     task.Group,
		Other:     other,
	})
	// Native async image submissions only reserve user quota at submit time; their
	// agent profit is settled once actual usage arrives, so a failed submission
	// has no earlier agent ledger entry to reverse.
	if settlesAgentRefund {
		if err := agentservice.SettleConsumeAdjustment(bc.AgentBillingSnapshot, task.UserId, logID, quota, 0); err != nil {
			logger.LogError(ctx, fmt.Sprintf("任务失败代理收益退款失败 task %s: %s", task.TaskID, err.Error()))
		}
	}
}

func taskTieredActualQuota(ctx context.Context, task *model.Task, taskResult *relaycommon.TaskInfo) (int, *billingexpr.TieredResult, bool) {
	if task == nil || taskResult == nil || task.PrivateData.BillingContext == nil {
		return 0, nil, false
	}
	bc := task.PrivateData.BillingContext
	snap := bc.TieredBillingSnapshot
	if snap == nil || snap.BillingMode != "tiered_expr" {
		return 0, nil, false
	}
	totalTokens := taskResult.TotalTokens
	if totalTokens <= 0 {
		totalTokens = taskResult.CompletionTokens
	}
	if totalTokens <= 0 {
		return 0, nil, false
	}
	requestInput := billingexpr.RequestInput{}
	if bc.BillingRequestInput != nil {
		requestInput = *bc.BillingRequestInput
	}
	result, err := billingexpr.ComputeTieredQuotaWithRequest(snap, billingexpr.TokenParams{
		C: float64(totalTokens),
	}, requestInput)
	if err != nil {
		logger.LogWarn(ctx, fmt.Sprintf("任务 %s 表达式计费结算失败: %s", task.TaskID, err.Error()))
		return 0, nil, false
	}
	return result.ActualQuotaAfterGroup, &result, true
}

func RecalculateTaskQuotaByTieredExpr(ctx context.Context, task *model.Task, taskResult *relaycommon.TaskInfo) bool {
	actualQuota, tieredResult, ok := taskTieredActualQuota(ctx, task, taskResult)
	if !ok {
		return false
	}
	reason := "tiered_expr"
	if tieredResult != nil && tieredResult.MatchedTier != "" {
		reason = fmt.Sprintf("tiered_expr：tier=%s", tieredResult.MatchedTier)
	}
	extraOther := map[string]interface{}{}
	InjectTieredBillingSnapshotInfo(extraOther, task.PrivateData.BillingContext.TieredBillingSnapshot, tieredResult)
	totalTokens := taskResult.TotalTokens
	if totalTokens <= 0 {
		totalTokens = taskResult.CompletionTokens
	}
	promptTokens := totalTokens - taskResult.CompletionTokens
	if promptTokens < 0 {
		promptTokens = 0
	}
	extraOther["actual_total_tokens"] = totalTokens
	extraOther["actual_prompt_tokens"] = promptTokens
	extraOther["actual_completion_tokens"] = taskResult.CompletionTokens
	if tieredResult != nil && tieredResult.Clamp != nil {
		attachQuotaSaturationToOther(extraOther, tieredResult.Clamp)
	}
	recalculateTaskQuota(ctx, task, actualQuota, reason, extraOther)
	return true
}

// RecalculateTaskQuota 通用的异步差额结算。
// actualQuota 是任务完成后的实际应扣额度，与预扣额度 (task.Quota) 做差额结算。
// reason 用于日志记录（例如 "token重算" 或 "adaptor调整"）。
func RecalculateTaskQuota(ctx context.Context, task *model.Task, actualQuota int, reason string) {
	recalculateTaskQuota(ctx, task, actualQuota, reason, nil)
}

// SettleImageTaskUsage recalculates a completed image task from the frozen
// submission-time pricing snapshot and adjusts the pre-consumed quota.
func SettleImageTaskUsage(ctx context.Context, task *model.Task, usage *dto.Usage, responseBody []byte) {
	if task == nil || usage == nil || !imageTaskUsageHasTokens(usage) {
		return
	}
	usage = normalizeImageTaskUsage(usage)
	bc := task.PrivateData.BillingContext
	if bc == nil {
		return
	}
	if task.PrivateData.BillingSource == BillingSourceUserOwnedProvider {
		settleImageTaskActualQuota(ctx, task, 0, normalizeImageTaskUsage(usage), nil)
		return
	}
	if bc.PerCallBilling {
		settleImageTaskActualQuota(ctx, task, task.Quota, normalizeImageTaskUsage(usage), nil)
		return
	}
	relayInfo := imageTaskSettlementRelayInfo(task, responseBody)
	ginContext := &gin.Context{}
	summary := calculateTextQuotaSummary(ginContext, relayInfo, usage)
	tieredUsedVars := map[string]bool(nil)
	if relayInfo.TieredBillingSnapshot != nil {
		tieredUsedVars = billingexpr.UsedVars(relayInfo.TieredBillingSnapshot.ExprString)
	}
	var tieredResult *billingexpr.TieredResult
	if ok, quota, result := TryTieredSettle(relayInfo, BuildTieredTokenParams(usage, usage.UsageSemantic == "anthropic", tieredUsedVars)); ok {
		summary.Quota = composeTieredTextQuota(relayInfo, summary, quota, result)
		tieredResult = result
	}
	settleImageTaskActualQuota(ctx, task, summary.Quota, usage, tieredResult)
}

func normalizeImageTaskUsage(usage *dto.Usage) *dto.Usage {
	if usage == nil {
		return nil
	}
	normalized := *usage
	if normalized.PromptTokens == 0 {
		normalized.PromptTokens = normalized.InputTokens
	}
	if normalized.CompletionTokens == 0 {
		normalized.CompletionTokens = normalized.OutputTokens
	}
	if normalized.TotalTokens == 0 {
		normalized.TotalTokens = normalized.PromptTokens + normalized.CompletionTokens
	}
	if normalized.InputTokensDetails != nil {
		normalized.PromptTokensDetails = *normalized.InputTokensDetails
	}
	if normalized.OutputTokensDetails != nil {
		normalized.CompletionTokenDetails = *normalized.OutputTokensDetails
	}
	return &normalized
}

func imageTaskUsageHasTokens(usage *dto.Usage) bool {
	return usage != nil && (usage.PromptTokens != 0 || usage.CompletionTokens != 0 ||
		usage.TotalTokens != 0 || usage.InputTokens != 0 || usage.OutputTokens != 0)
}

func imageTaskUsageOther(usage *dto.Usage) map[string]interface{} {
	promptTokens := usage.PromptTokens
	completionTokens := usage.CompletionTokens
	if promptTokens == 0 {
		promptTokens = usage.InputTokens
	}
	if completionTokens == 0 {
		completionTokens = usage.OutputTokens
	}
	totalTokens := usage.TotalTokens
	if totalTokens == 0 {
		totalTokens = promptTokens + completionTokens
	}
	return map[string]interface{}{
		"actual_prompt_tokens":     promptTokens,
		"actual_completion_tokens": completionTokens,
		"actual_total_tokens":      totalTokens,
	}
}

func imageTaskSettlementRelayInfo(task *model.Task, responseBody []byte) *relaycommon.RelayInfo {
	bc := task.PrivateData.BillingContext
	requestInput := &billingexpr.RequestInput{}
	if bc.BillingRequestInput != nil {
		*requestInput = *bc.BillingRequestInput
		requestInput.Headers = cloneTaskBillingHeaders(bc.BillingRequestInput.Headers)
		requestInput.Body = append([]byte(nil), bc.BillingRequestInput.Body...)
	}
	requestInput.ResponseBody = append([]byte(nil), responseBody...)
	return &relaycommon.RelayInfo{
		UserId:                task.UserId,
		UsingGroup:            task.Group,
		OriginModelName:       taskModelName(task),
		StartTime:             time.Unix(task.SubmitTime, 0),
		BillingRequestInput:   requestInput,
		TieredBillingSnapshot: bc.TieredBillingSnapshot,
		PriceData: types.PriceData{
			ModelPrice: bc.ModelPrice, ModelRatio: bc.ModelRatio,
			CompletionRatio: bc.CompletionRatio, CacheRatio: bc.CacheRatio,
			CacheCreationRatio: bc.CacheCreationRatio, CacheCreation5mRatio: bc.CacheCreation5mRatio,
			CacheCreation1hRatio: bc.CacheCreation1hRatio, ImageRatio: bc.ImageRatio,
			AudioRatio: bc.AudioRatio, AudioCompletionRatio: bc.AudioCompletionRatio,
			OtherRatios: bc.OtherRatios, UsePrice: bc.PerCallBilling,
			GroupRatioInfo: types.GroupRatioInfo{GroupRatio: bc.GroupRatio},
		},
	}
}

func cloneTaskBillingHeaders(source map[string]string) map[string]string {
	if len(source) == 0 {
		return nil
	}
	clone := make(map[string]string, len(source))
	for key, value := range source {
		clone[key] = value
	}
	return clone
}

func settleImageTaskActualQuota(ctx context.Context, task *model.Task, actualQuota int, usage *dto.Usage, tieredResult *billingexpr.TieredResult) {
	if actualQuota < 0 {
		return
	}
	preConsumedQuota := task.Quota
	quotaDelta := actualQuota - preConsumedQuota
	if quotaDelta != 0 {
		if err := taskAdjustFunding(task, quotaDelta); err != nil {
			logger.LogError(ctx, fmt.Sprintf("图片任务差额结算资金调整失败 task %s: %s", task.TaskID, err.Error()))
			return
		}
		taskAdjustTokenQuota(ctx, task, quotaDelta)
	}
	task.Quota = actualQuota
	if err := model.DB.Model(task).Update("quota", actualQuota).Error; err != nil {
		logger.LogWarn(ctx, fmt.Sprintf("更新图片任务实际扣费失败 task %s: %s", task.TaskID, err.Error()))
	}
	model.UpdateUserUsedQuotaAndRequestCount(task.UserId, actualQuota)
	model.UpdateChannelUsedQuota(task.ChannelId, actualQuota)
	other := taskBillingOther(task)
	for key, value := range imageTaskUsageOther(usage) {
		other[key] = value
	}
	other["task_id"] = task.TaskID
	other["pre_consumed_quota"] = preConsumedQuota
	other["actual_quota"] = actualQuota
	if tieredResult != nil && task.PrivateData.BillingContext.TieredBillingSnapshot != nil {
		InjectTieredBillingSnapshotInfo(other, task.PrivateData.BillingContext.TieredBillingSnapshot, tieredResult)
	}
	relayInfo := imageTaskSettlementRelayInfo(task, nil)
	appendChannelCostInfo(other, relayInfo, actualQuota)
	snapshot := task.PrivateData.BillingContext.AgentBillingSnapshot
	logID := model.RecordTaskBillingLog(model.RecordTaskBillingLogParams{
		Force:  snapshot != nil,
		UserId: task.UserId, LogType: model.LogTypeConsume, Content: "image usage",
		ChannelId: task.ChannelId, ModelName: taskModelName(task), Quota: actualQuota,
		PromptTokens:     intFromMap(other, "actual_prompt_tokens"),
		CompletionTokens: intFromMap(other, "actual_completion_tokens"),
		TokenId:          task.PrivateData.TokenId, Group: task.Group, Other: other,
	})
	if snapshot != nil {
		if err := agentservice.SettleConsume(snapshot, task.UserId, logID, actualQuota); err != nil {
			logger.LogError(ctx, fmt.Sprintf("图片任务代理计费结算失败 task %s: %s", task.TaskID, err.Error()))
		}
	}
}

func recalculateTaskQuota(ctx context.Context, task *model.Task, actualQuota int, reason string, extraOther map[string]interface{}) {
	if actualQuota <= 0 {
		return
	}
	preConsumedQuota := task.Quota
	quotaDelta := actualQuota - preConsumedQuota

	if quotaDelta == 0 {
		logger.LogInfo(ctx, fmt.Sprintf("任务 %s 预扣费准确（%s，%s）",
			task.TaskID, logger.LogQuota(actualQuota), reason))
		return
	}

	logger.LogInfo(ctx, fmt.Sprintf("任务 %s 差额结算：delta=%s（实际：%s，预扣：%s，%s）",
		task.TaskID,
		logger.LogQuota(quotaDelta),
		logger.LogQuota(actualQuota),
		logger.LogQuota(preConsumedQuota),
		reason,
	))

	// 调整资金来源
	if err := taskAdjustFunding(task, quotaDelta); err != nil {
		logger.LogError(ctx, fmt.Sprintf("差额结算资金调整失败 task %s: %s", task.TaskID, err.Error()))
		return
	}

	// 调整令牌额度
	taskAdjustTokenQuota(ctx, task, quotaDelta)

	task.Quota = actualQuota
	if err := model.DB.Model(task).Update("quota", actualQuota).Error; err != nil {
		logger.LogWarn(ctx, fmt.Sprintf("更新任务实际扣费失败 task %s: %s", task.TaskID, err.Error()))
	}

	var logType int
	var logQuota int
	if quotaDelta > 0 {
		logType = model.LogTypeConsume
		logQuota = quotaDelta
		model.UpdateUserUsedQuotaAndRequestCount(task.UserId, quotaDelta)
		model.UpdateChannelUsedQuota(task.ChannelId, quotaDelta)
	} else {
		logType = model.LogTypeRefund
		logQuota = -quotaDelta
	}
	other := taskBillingOther(task)
	other["task_id"] = task.TaskID
	other["pre_consumed_quota"] = preConsumedQuota
	other["actual_quota"] = actualQuota
	for key, value := range extraOther {
		other[key] = value
	}
	bc := task.PrivateData.BillingContext
	settlesAgentAdjustment := bc != nil && bc.AgentBillingSnapshot != nil
	logID := model.RecordTaskBillingLog(model.RecordTaskBillingLogParams{
		Force:            settlesAgentAdjustment,
		UserId:           task.UserId,
		LogType:          logType,
		Content:          reason,
		ChannelId:        task.ChannelId,
		ModelName:        taskModelName(task),
		Quota:            logQuota,
		PromptTokens:     intFromMap(other, "actual_prompt_tokens"),
		CompletionTokens: intFromMap(other, "actual_completion_tokens"),
		TokenId:          task.PrivateData.TokenId,
		Group:            task.Group,
		Other:            other,
	})
	if settlesAgentAdjustment {
		if err := agentservice.SettleConsumeAdjustment(bc.AgentBillingSnapshot, task.UserId, logID, preConsumedQuota, actualQuota); err != nil {
			logger.LogError(ctx, fmt.Sprintf("任务差额代理收益结算失败 task %s: %s", task.TaskID, err.Error()))
		}
	}
}

func intFromMap(values map[string]interface{}, key string) int {
	if values == nil {
		return 0
	}
	switch value := values[key].(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	case float32:
		return int(value)
	default:
		return 0
	}
}

// RecalculateTaskQuotaByTokens 根据实际 token 消耗重新计费（异步差额结算）。
// 当任务成功且返回了 totalTokens 时，根据模型倍率和分组倍率重新计算实际扣费额度，
// 与预扣费的差额进行补扣或退还。支持钱包和订阅计费来源。
func RecalculateTaskQuotaByTokens(ctx context.Context, task *model.Task, totalTokens int) {
	if totalTokens <= 0 {
		return
	}

	modelName := taskModelName(task)

	// 获取模型价格和倍率
	modelRatio, hasRatioSetting := taskBillingModelRatio(task, modelName)
	// 只有配置了倍率(非固定价格)时才按 token 重新计费
	if !hasRatioSetting || modelRatio <= 0 {
		return
	}

	finalGroupRatio := taskBillingGroupRatio(task)
	if finalGroupRatio < 0 {
		return
	}

	// 计算 OtherRatios 乘积（视频折扣、时长等）
	otherMultiplier := 1.0
	if bc := task.PrivateData.BillingContext; bc != nil {
		for _, r := range bc.OtherRatios {
			if r != 1.0 && r > 0 {
				otherMultiplier *= r
			}
		}
	}

	// 计算实际应扣费额度: totalTokens * modelRatio * groupRatio * otherMultiplier
	actualQuota, clamp := common.QuotaFromFloatChecked(float64(totalTokens) * modelRatio * finalGroupRatio * otherMultiplier)

	reason := fmt.Sprintf("token重算：tokens=%d, modelRatio=%.2f, groupRatio=%.2f, otherMultiplier=%.4f", totalTokens, modelRatio, finalGroupRatio, otherMultiplier)
	extraOther := map[string]interface{}{}
	if clamp != nil {
		attachQuotaSaturationToOther(extraOther, clamp)
	}
	recalculateTaskQuota(ctx, task, actualQuota, reason, extraOther)
}

func taskBillingGroupRatio(task *model.Task) float64 {
	if task == nil {
		return -1
	}
	if bc := task.PrivateData.BillingContext; bc != nil && bc.GroupRatio >= 0 {
		return bc.GroupRatio
	}
	group := task.Group
	if group == "" {
		user, err := model.GetUserById(task.UserId, false)
		if err == nil {
			group = user.Group
		}
	}
	if group == "" {
		return -1
	}
	groupRatio := ratio_setting.GetGroupRatio(group)
	if userGroupRatio, hasUserGroupRatio := ratio_setting.GetGroupGroupRatio(group, group); hasUserGroupRatio {
		return userGroupRatio
	}
	return groupRatio
}

func taskBillingModelRatio(task *model.Task, modelName string) (float64, bool) {
	if task != nil {
		if bc := task.PrivateData.BillingContext; bc != nil && bc.ModelRatio > 0 {
			return bc.ModelRatio, true
		}
	}
	modelRatio, ok, _ := ratio_setting.GetModelRatio(modelName)
	return modelRatio, ok
}

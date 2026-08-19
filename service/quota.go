package service

import (
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	perfmetrics "github.com/QuantumNous/new-api/pkg/perf_metrics"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	agentservice "github.com/QuantumNous/new-api/service/agent"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/bytedance/gopkg/util/gopool"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

type TokenDetails struct {
	TextTokens  int
	AudioTokens int
}

type QuotaInfo struct {
	InputDetails  TokenDetails
	OutputDetails TokenDetails
	ModelName     string
	UsePrice      bool
	ModelPrice    float64
	ModelRatio    float64
	GroupRatio    float64
}

type quotaNotifyPlan struct {
	threshold      int
	notifyType     string
	remainingQuota int
}

func defaultQuotaNotifyThresholds() []quotaNotifyPlan {
	return []quotaNotifyPlan{
		{threshold: int(10 * common.QuotaPerUnit), notifyType: dto.NotifyTypeQuotaExceed + "_usd_10"},
		{threshold: int(5 * common.QuotaPerUnit), notifyType: dto.NotifyTypeQuotaExceed + "_usd_5"},
		{threshold: 0, notifyType: dto.NotifyTypeQuotaExceed + "_insufficient"},
	}
}

func quotaNotifyPlans(userQuota int, consumeQuota int, customThreshold float64) []quotaNotifyPlan {
	remainingQuota := userQuota - consumeQuota
	if customThreshold > 0 {
		threshold := int(customThreshold)
		if userQuota > 0 && remainingQuota <= 0 {
			return []quotaNotifyPlan{{
				threshold:      0,
				notifyType:     dto.NotifyTypeQuotaExceed + "_insufficient",
				remainingQuota: remainingQuota,
			}}
		}
		if userQuota < threshold || remainingQuota >= threshold {
			return nil
		}
		return []quotaNotifyPlan{{
			threshold:      threshold,
			notifyType:     dto.NotifyTypeQuotaExceed,
			remainingQuota: remainingQuota,
		}}
	}

	plans := make([]quotaNotifyPlan, 0, 3)
	var selected *quotaNotifyPlan
	for _, plan := range defaultQuotaNotifyThresholds() {
		crossedThreshold := userQuota >= plan.threshold && remainingQuota < plan.threshold
		if plan.threshold == 0 {
			crossedThreshold = userQuota > 0 && remainingQuota <= 0
		}
		if crossedThreshold {
			plan.remainingQuota = remainingQuota
			selected = &plan
		}
	}
	if selected != nil {
		plans = append(plans, *selected)
	}
	return plans
}

func filterQuotaNotifyPlansByState(plans []quotaNotifyPlan, state map[string]int) []quotaNotifyPlan {
	if len(plans) == 0 || len(state) == 0 {
		return plans
	}
	filtered := make([]quotaNotifyPlan, 0, len(plans))
	for _, plan := range plans {
		if sentThreshold, ok := state[plan.notifyType]; ok && sentThreshold == plan.threshold {
			continue
		}
		filtered = append(filtered, plan)
	}
	return filtered
}

func markQuotaNotifyStateSent(state map[string]int, plan quotaNotifyPlan) map[string]int {
	if state == nil {
		state = make(map[string]int)
	}
	state[plan.notifyType] = plan.threshold
	return state
}

func normalizeQuotaNotifyState(state map[string]int, remainingQuota int) bool {
	if len(state) == 0 {
		return false
	}
	changed := false
	for notifyType, threshold := range state {
		recovered := remainingQuota >= threshold
		if threshold == 0 {
			recovered = remainingQuota > 0
		}
		if recovered {
			delete(state, notifyType)
			changed = true
		}
	}
	return changed
}

func persistUserQuotaNotifyState(userId int, state map[string]int) error {
	user, err := model.GetUserById(userId, true)
	if err != nil {
		return err
	}
	setting := user.GetSetting()
	setting.QuotaNotifyState = state
	user.SetSetting(setting)
	if err := model.DB.Model(&model.User{}).Where("id = ?", userId).Update("setting", user.Setting).Error; err != nil {
		return err
	}
	return model.InvalidateUserCache(userId)
}

func reserveUserQuotaNotifyState(userId int, plan quotaNotifyPlan, remainingQuota int) (bool, error) {
	for i := 0; i < 3; i++ {
		user, err := model.GetUserById(userId, true)
		if err != nil {
			return false, err
		}
		setting := user.GetSetting()
		if setting.QuotaNotifyState == nil {
			setting.QuotaNotifyState = map[string]int{}
		}
		normalizeQuotaNotifyState(setting.QuotaNotifyState, remainingQuota)
		if sentThreshold, ok := setting.QuotaNotifyState[plan.notifyType]; ok && sentThreshold == plan.threshold {
			return false, nil
		}
		setting.QuotaNotifyState = markQuotaNotifyStateSent(setting.QuotaNotifyState, plan)

		oldSetting := user.Setting
		user.SetSetting(setting)
		res := model.DB.Model(&model.User{}).
			Where("id = ? AND setting = ?", userId, oldSetting).
			Update("setting", user.Setting)
		if res.Error != nil {
			return false, res.Error
		}
		if res.RowsAffected == 1 {
			return true, model.InvalidateUserCache(userId)
		}
	}
	return false, fmt.Errorf("failed to reserve quota notify state for user %d after retries", userId)
}

func hasCustomModelRatio(modelName string, currentRatio float64) bool {
	defaultRatio, exists := ratio_setting.GetDefaultModelRatioMap()[modelName]
	if !exists {
		return true
	}
	return currentRatio != defaultRatio
}

func calculateAudioQuota(info QuotaInfo) (int, *common.QuotaClamp) {
	if info.UsePrice {
		modelPrice := decimal.NewFromFloat(info.ModelPrice)
		quotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
		groupRatio := decimal.NewFromFloat(info.GroupRatio)

		quota := modelPrice.Mul(quotaPerUnit).Mul(groupRatio)
		return common.QuotaFromDecimalChecked(quota)
	}

	completionRatio := decimal.NewFromFloat(ratio_setting.GetCompletionRatio(info.ModelName))
	audioRatio := decimal.NewFromFloat(ratio_setting.GetAudioRatio(info.ModelName))
	audioCompletionRatio := decimal.NewFromFloat(ratio_setting.GetAudioCompletionRatio(info.ModelName))

	groupRatio := decimal.NewFromFloat(info.GroupRatio)
	modelRatio := decimal.NewFromFloat(info.ModelRatio)
	ratio := groupRatio.Mul(modelRatio)

	inputTextTokens := decimal.NewFromInt(int64(info.InputDetails.TextTokens))
	outputTextTokens := decimal.NewFromInt(int64(info.OutputDetails.TextTokens))
	inputAudioTokens := decimal.NewFromInt(int64(info.InputDetails.AudioTokens))
	outputAudioTokens := decimal.NewFromInt(int64(info.OutputDetails.AudioTokens))

	quota := decimal.Zero
	quota = quota.Add(inputTextTokens)
	quota = quota.Add(outputTextTokens.Mul(completionRatio))
	quota = quota.Add(inputAudioTokens.Mul(audioRatio))
	quota = quota.Add(outputAudioTokens.Mul(audioRatio).Mul(audioCompletionRatio))

	quota = quota.Mul(ratio)

	// If ratio is not zero and quota is less than or equal to zero, set quota to 1
	if !ratio.IsZero() && quota.LessThanOrEqual(decimal.Zero) {
		quota = decimal.NewFromInt(1)
	}

	return common.QuotaFromDecimalChecked(quota)
}

func PreWssConsumeQuota(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, usage *dto.RealtimeUsage) error {
	if relayInfo.UsePrice {
		return nil
	}
	userQuota, err := model.GetUserQuota(relayInfo.UserId, false)
	if err != nil {
		return err
	}

	token, err := model.GetTokenByKey(common.NormalizeTokenKey(relayInfo.TokenKey), false)
	if err != nil {
		return err
	}

	modelName := relayInfo.OriginModelName
	textInputTokens := usage.InputTokenDetails.TextTokens
	textOutTokens := usage.OutputTokenDetails.TextTokens
	audioInputTokens := usage.InputTokenDetails.AudioTokens
	audioOutTokens := usage.OutputTokenDetails.AudioTokens
	modelRatio, _, _ := ratio_setting.GetModelRatio(modelName)

	autoGroup, exists := common.GetContextKey(ctx, constant.ContextKeyAutoGroup)
	if exists {
		logger.LogDebug(ctx, "final group: %s", autoGroup)
		relayInfo.UsingGroup = autoGroup.(string)
	}

	groupRatio := agentservice.ResolveGroupRatio(ctx, relayInfo.AgentContext, relayInfo.UserGroup, relayInfo.TokenGroup, relayInfo.UsingGroup)
	if agentGroup, ok := agentservice.ResolveGroup(relayInfo.AgentContext, relayInfo.UsingGroup); ok {
		relayInfo.UsingGroup = agentGroup.SystemGroupName
	}
	if autoGroup, exists := common.GetContextKey(ctx, constant.ContextKeyAutoGroup); exists {
		if group, ok := autoGroup.(string); ok && group != "" {
			relayInfo.UsingGroup = group
		}
	}
	relayInfo.AgentBillingSnapshot = groupRatio.Snapshot
	relayInfo.PriceData.GroupRatioInfo = types.GroupRatioInfo{
		GroupRatio:        groupRatio.GroupRatio,
		GroupSpecialRatio: groupRatio.GroupSpecialRatio,
		HasSpecialRatio:   groupRatio.HasSpecialRatio,
		BaseGroupRatio:    groupRatio.BaseGroupRatio,
		AgentGroupRatio:   groupRatio.AgentGroupRatio,
		HasAgentRatio:     groupRatio.HasAgentRatio,
	}
	relayInfo.PriceData.ModelRatio = modelRatio
	relayInfo.PriceData.UsePrice = relayInfo.UsePrice

	quotaInfo := QuotaInfo{
		InputDetails: TokenDetails{
			TextTokens:  textInputTokens,
			AudioTokens: audioInputTokens,
		},
		OutputDetails: TokenDetails{
			TextTokens:  textOutTokens,
			AudioTokens: audioOutTokens,
		},
		ModelName:  modelName,
		UsePrice:   relayInfo.UsePrice,
		ModelRatio: modelRatio,
		GroupRatio: groupRatio.GroupRatio,
	}

	quota, clamp := calculateAudioQuota(quotaInfo)
	noteQuotaClamp(relayInfo, clamp)

	if userQuota < quota {
		return fmt.Errorf("user quota is not enough, user quota: %s, need quota: %s", logger.FormatQuota(userQuota), logger.FormatQuota(quota))
	}

	if !token.UnlimitedQuota && token.RemainQuota < quota {
		return fmt.Errorf("token quota is not enough, token remain quota: %s, need quota: %s", logger.FormatQuota(token.RemainQuota), logger.FormatQuota(quota))
	}

	err = PostConsumeQuota(relayInfo, quota, 0, false)
	if err != nil {
		return err
	}
	logger.LogInfo(ctx, "realtime streaming consume quota success, quota: "+fmt.Sprintf("%d", quota))
	return nil
}

func PostWssConsumeQuota(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, modelName string,
	usage *dto.RealtimeUsage, extraContent string) {

	var tieredResult *billingexpr.TieredResult
	tieredOk, tieredQuota, tieredRes := TryTieredSettle(relayInfo, billingexpr.TokenParams{
		P:   float64(usage.InputTokens),
		C:   float64(usage.OutputTokens),
		Len: float64(usage.InputTokens),
	})
	if tieredOk {
		tieredResult = tieredRes
	}

	useTimeSeconds := time.Now().Unix() - relayInfo.StartTime.Unix()
	textInputTokens := usage.InputTokenDetails.TextTokens
	textOutTokens := usage.OutputTokenDetails.TextTokens

	audioInputTokens := usage.InputTokenDetails.AudioTokens
	audioOutTokens := usage.OutputTokenDetails.AudioTokens

	tokenName := ctx.GetString("token_name")
	completionRatio := decimal.NewFromFloat(ratio_setting.GetCompletionRatio(modelName))
	audioRatio := decimal.NewFromFloat(ratio_setting.GetAudioRatio(relayInfo.OriginModelName))
	audioCompletionRatio := decimal.NewFromFloat(ratio_setting.GetAudioCompletionRatio(modelName))

	modelRatio := relayInfo.PriceData.ModelRatio
	groupRatio := relayInfo.PriceData.GroupRatioInfo.GroupRatio
	modelPrice := relayInfo.PriceData.ModelPrice
	usePrice := relayInfo.PriceData.UsePrice

	quotaInfo := QuotaInfo{
		InputDetails: TokenDetails{
			TextTokens:  textInputTokens,
			AudioTokens: audioInputTokens,
		},
		OutputDetails: TokenDetails{
			TextTokens:  textOutTokens,
			AudioTokens: audioOutTokens,
		},
		ModelName:  modelName,
		UsePrice:   usePrice,
		ModelRatio: modelRatio,
		GroupRatio: groupRatio,
	}

	quota, clamp := calculateAudioQuota(quotaInfo)
	noteQuotaClamp(relayInfo, clamp)
	if tieredOk {
		quota = tieredQuota
		if tieredResult != nil {
			noteQuotaClamp(relayInfo, tieredResult.Clamp)
		}
	}

	totalTokens := usage.TotalTokens
	var logContent string
	if !usePrice {
		logContent = fmt.Sprintf("模型倍率 %.2f，补全倍率 %.2f，音频倍率 %.2f，音频补全倍率 %.2f，分组倍率 %.2f",
			modelRatio, completionRatio.InexactFloat64(), audioRatio.InexactFloat64(), audioCompletionRatio.InexactFloat64(), groupRatio)
	} else {
		logContent = fmt.Sprintf("模型价格 %.2f，分组倍率 %.2f", modelPrice, groupRatio)
	}

	// record all the consume log even if quota is 0
	if totalTokens == 0 {
		// in this case, must be some error happened
		// we cannot just return, because we may have to return the pre-consumed quota
		quota = 0
		logContent += "（可能是上游超时）"
		logger.LogError(ctx, fmt.Sprintf("total tokens is 0, cannot consume quota, userId %d, channelId %d, "+
			"tokenId %d, model %s， pre-consumed quota %d", relayInfo.UserId, relayInfo.ChannelId, relayInfo.TokenId, modelName, relayInfo.FinalPreConsumedQuota))
	} else {
		if relayInfo.AgentBillingSnapshot != nil {
			baseQuota := agentservice.BaseQuotaFromCharged(relayInfo.AgentBillingSnapshot, quota)
			relayInfo.AgentBillingSnapshot.BaseEstimatedQuota = baseQuota
			relayInfo.AgentBillingSnapshot.ChargedEstimatedQuota = quota
		}
		model.UpdateUserUsedQuotaAndRequestCount(relayInfo.UserId, quota)
		model.UpdateChannelUsedQuota(relayInfo.ChannelId, quota)
	}

	if err := SettleBilling(ctx, relayInfo, quota); err != nil {
		logger.LogError(ctx, "error settling billing: "+err.Error())
	}

	logModel := modelName
	if extraContent != "" {
		logContent += ", " + extraContent
	}
	other := GenerateWssOtherInfo(ctx, relayInfo, usage, modelRatio, groupRatio,
		completionRatio.InexactFloat64(), audioRatio.InexactFloat64(), audioCompletionRatio.InexactFloat64(), modelPrice, relayInfo.PriceData.GroupRatioInfo.GroupSpecialRatio)
	if tieredResult != nil {
		InjectTieredBillingInfo(other, relayInfo, tieredResult)
	}
	attachQuotaSaturation(ctx, relayInfo, other)
	appendChannelCostInfo(other, relayInfo, quota)
	logID := model.RecordConsumeLog(ctx, relayInfo.UserId, model.RecordConsumeLogParams{
		Force:            relayInfo.AgentBillingSnapshot != nil && totalTokens > 0,
		ChannelId:        relayInfo.ChannelId,
		PromptTokens:     usage.InputTokens,
		CompletionTokens: usage.OutputTokens,
		ModelName:        logModel,
		TokenName:        tokenName,
		Quota:            quota,
		Content:          logContent,
		TokenId:          relayInfo.TokenId,
		UseTimeSeconds:   int(useTimeSeconds),
		IsStream:         relayInfo.IsStream,
		Group:            relayInfo.UsingGroup,
		Other:            other,
	})
	if relayInfo.AgentBillingSnapshot != nil && totalTokens > 0 {
		if err := agentservice.SettleConsume(relayInfo.AgentBillingSnapshot, relayInfo.UserId, logID, quota); err != nil {
			logger.LogError(ctx, "error settling agent ledger: "+err.Error())
		}
	}
}

func CalcOpenRouterCacheCreateTokens(usage dto.Usage, priceData types.PriceData) int {
	if priceData.CacheCreationRatio == 1 {
		return 0
	}
	quotaPrice := priceData.ModelRatio / common.QuotaPerUnit
	promptCacheCreatePrice := quotaPrice * priceData.CacheCreationRatio
	promptCacheReadPrice := quotaPrice * priceData.CacheRatio
	completionPrice := quotaPrice * priceData.CompletionRatio

	cost, _ := usage.Cost.(float64)
	totalPromptTokens := float64(usage.PromptTokens)
	completionTokens := float64(usage.CompletionTokens)
	promptCacheReadTokens := float64(usage.PromptTokensDetails.CachedTokens)

	return int(math.Round((cost -
		totalPromptTokens*quotaPrice +
		promptCacheReadTokens*(quotaPrice-promptCacheReadPrice) -
		completionTokens*completionPrice) /
		(promptCacheCreatePrice - quotaPrice)))
}

func PostAudioConsumeQuota(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, usage *dto.Usage, extraContent string) {

	var tieredUsedVars map[string]bool
	if snap := relayInfo.TieredBillingSnapshot; snap != nil {
		tieredUsedVars = billingexpr.UsedVars(snap.ExprString)
	}
	var tieredResult *billingexpr.TieredResult
	tieredOk, tieredQuota, tieredRes := TryTieredSettle(relayInfo, BuildTieredTokenParams(usage, false, tieredUsedVars))
	if tieredOk {
		tieredResult = tieredRes
	}

	useTimeSeconds := time.Now().Unix() - relayInfo.StartTime.Unix()
	textInputTokens := usage.PromptTokensDetails.TextTokens
	textOutTokens := usage.CompletionTokenDetails.TextTokens

	audioInputTokens := usage.PromptTokensDetails.AudioTokens
	audioOutTokens := usage.CompletionTokenDetails.AudioTokens

	tokenName := ctx.GetString("token_name")
	completionRatio := decimal.NewFromFloat(ratio_setting.GetCompletionRatio(relayInfo.OriginModelName))
	audioRatio := decimal.NewFromFloat(ratio_setting.GetAudioRatio(relayInfo.OriginModelName))
	audioCompletionRatio := decimal.NewFromFloat(ratio_setting.GetAudioCompletionRatio(relayInfo.OriginModelName))

	modelRatio := relayInfo.PriceData.ModelRatio
	groupRatio := relayInfo.PriceData.GroupRatioInfo.GroupRatio
	modelPrice := relayInfo.PriceData.ModelPrice
	usePrice := relayInfo.PriceData.UsePrice

	quotaInfo := QuotaInfo{
		InputDetails: TokenDetails{
			TextTokens:  textInputTokens,
			AudioTokens: audioInputTokens,
		},
		OutputDetails: TokenDetails{
			TextTokens:  textOutTokens,
			AudioTokens: audioOutTokens,
		},
		ModelName:  relayInfo.OriginModelName,
		UsePrice:   usePrice,
		ModelRatio: modelRatio,
		GroupRatio: groupRatio,
	}

	quota, clamp := calculateAudioQuota(quotaInfo)
	noteQuotaClamp(relayInfo, clamp)
	if tieredOk {
		quota = tieredQuota
		if tieredResult != nil {
			noteQuotaClamp(relayInfo, tieredResult.Clamp)
		}
	}

	totalTokens := usage.TotalTokens
	var logContent string
	if !usePrice {
		logContent = fmt.Sprintf("模型倍率 %.2f，补全倍率 %.2f，音频倍率 %.2f，音频补全倍率 %.2f，分组倍率 %.2f",
			modelRatio, completionRatio.InexactFloat64(), audioRatio.InexactFloat64(), audioCompletionRatio.InexactFloat64(), groupRatio)
	} else {
		logContent = fmt.Sprintf("模型价格 %.2f，分组倍率 %.2f", modelPrice, groupRatio)
	}

	// record all the consume log even if quota is 0
	if totalTokens == 0 {
		// in this case, must be some error happened
		// we cannot just return, because we may have to return the pre-consumed quota
		quota = 0
		logContent += "（可能是上游超时）"
		logger.LogError(ctx, fmt.Sprintf("total tokens is 0, cannot consume quota, userId %d, channelId %d, "+
			"tokenId %d, model %s， pre-consumed quota %d", relayInfo.UserId, relayInfo.ChannelId, relayInfo.TokenId, relayInfo.OriginModelName, relayInfo.FinalPreConsumedQuota))
	} else {
		if relayInfo.AgentBillingSnapshot != nil {
			baseQuota := agentservice.BaseQuotaFromCharged(relayInfo.AgentBillingSnapshot, quota)
			relayInfo.AgentBillingSnapshot.BaseEstimatedQuota = baseQuota
			relayInfo.AgentBillingSnapshot.ChargedEstimatedQuota = quota
		}
		model.UpdateUserUsedQuotaAndRequestCount(relayInfo.UserId, quota)
		model.UpdateChannelUsedQuota(relayInfo.ChannelId, quota)
	}

	if err := SettleBilling(ctx, relayInfo, quota); err != nil {
		logger.LogError(ctx, "error settling billing: "+err.Error())
	}

	logModel := relayInfo.OriginModelName
	if extraContent != "" {
		logContent += ", " + extraContent
	}
	other := GenerateAudioOtherInfo(ctx, relayInfo, usage, modelRatio, groupRatio,
		completionRatio.InexactFloat64(), audioRatio.InexactFloat64(), audioCompletionRatio.InexactFloat64(), modelPrice, relayInfo.PriceData.GroupRatioInfo.GroupSpecialRatio)
	if tieredResult != nil {
		InjectTieredBillingInfo(other, relayInfo, tieredResult)
	}
	attachQuotaSaturation(ctx, relayInfo, other)
	appendChannelCostInfo(other, relayInfo, quota)
	logID := model.RecordConsumeLog(ctx, relayInfo.UserId, model.RecordConsumeLogParams{
		Force:            relayInfo.AgentBillingSnapshot != nil && totalTokens > 0,
		ChannelId:        relayInfo.ChannelId,
		PromptTokens:     usage.PromptTokens,
		CompletionTokens: usage.CompletionTokens,
		ModelName:        logModel,
		TokenName:        tokenName,
		Quota:            quota,
		Content:          logContent,
		TokenId:          relayInfo.TokenId,
		UseTimeSeconds:   int(useTimeSeconds),
		IsStream:         relayInfo.IsStream,
		Group:            relayInfo.UsingGroup,
		Other:            other,
	})
	if relayInfo.AgentBillingSnapshot != nil && totalTokens > 0 {
		if err := agentservice.SettleConsume(relayInfo.AgentBillingSnapshot, relayInfo.UserId, logID, quota); err != nil {
			logger.LogError(ctx, "error settling agent ledger: "+err.Error())
		}
	}
	gopool.Go(func() {
		perfmetrics.RecordRelaySample(relayInfo, true, int64(usage.CompletionTokens))
	})
}

func PreConsumeTokenQuota(relayInfo *relaycommon.RelayInfo, quota int) error {
	if quota < 0 {
		return errors.New("quota 不能为负数！")
	}
	if relayInfo.IsPlayground {
		return nil
	}
	reserved, err := model.TryReserveTokenQuota(relayInfo.TokenId, relayInfo.TokenKey, quota, relayInfo.TokenUnlimited)
	if err != nil {
		return err
	}
	if !reserved {
		remainQuota := 0
		if token, tokenErr := model.GetTokenByKey(relayInfo.TokenKey, false); tokenErr == nil && token != nil {
			remainQuota = token.RemainQuota
		}
		return fmt.Errorf("token quota is not enough, token remain quota: %s, need quota: %s", logger.FormatQuota(remainQuota), logger.FormatQuota(quota))
	}
	return nil
}

type postConsumeQuotaResult struct {
	FundingApplied bool
	TokenApplied   bool
}

func PostConsumeQuota(relayInfo *relaycommon.RelayInfo, quota int, preConsumedQuota int, sendEmail bool) error {
	_, err := postConsumeQuotaWithResult(relayInfo, quota, preConsumedQuota, sendEmail)
	return err
}

func postConsumeQuotaWithResult(relayInfo *relaycommon.RelayInfo, quota int, preConsumedQuota int, sendEmail bool) (result postConsumeQuotaResult, err error) {

	// 1) Consume from wallet quota OR subscription item
	if relayInfo != nil && relayInfo.BillingSource == BillingSourceSubscription {
		if relayInfo.SubscriptionId == 0 {
			return result, errors.New("subscription id is missing")
		}
		delta := int64(quota)
		if delta != 0 {
			if err := model.PostConsumeUserSubscriptionDelta(relayInfo.SubscriptionId, delta); err != nil {
				return result, err
			}
			relayInfo.SubscriptionPostDelta += delta
		}
	} else {
		// Wallet
		if quota > 0 {
			err = model.DecreaseUserQuota(relayInfo.UserId, quota, false)
		} else {
			err = model.IncreaseUserQuota(relayInfo.UserId, -quota, false)
		}
		if err != nil {
			return result, err
		}
	}
	result.FundingApplied = true

	if !relayInfo.IsPlayground {
		if quota > 0 {
			err = model.DecreaseTokenQuota(relayInfo.TokenId, relayInfo.TokenKey, quota)
		} else {
			err = model.IncreaseTokenQuota(relayInfo.TokenId, relayInfo.TokenKey, -quota)
		}
		if err != nil {
			return result, err
		}
		result.TokenApplied = true
	}

	if sendEmail {
		if (quota + preConsumedQuota) != 0 {
			checkAndSendQuotaNotify(relayInfo, quota, preConsumedQuota)
		}
	}

	return result, nil
}

func checkAndSendQuotaNotify(relayInfo *relaycommon.RelayInfo, quota int, preConsumedQuota int) {
	gopool.Go(func() {
		userSetting := relayInfo.UserSetting
		consumeQuota := quota + preConsumedQuota
		remainingQuota := relayInfo.UserQuota - consumeQuota
		if userSetting.QuotaNotifyState == nil {
			userSetting.QuotaNotifyState = map[string]int{}
		}
		stateChanged := normalizeQuotaNotifyState(userSetting.QuotaNotifyState, remainingQuota)
		plans := filterQuotaNotifyPlansByState(
			quotaNotifyPlans(relayInfo.UserQuota, consumeQuota, userSetting.QuotaWarningThreshold),
			userSetting.QuotaNotifyState,
		)
		if stateChanged && len(plans) == 0 {
			if err := persistUserQuotaNotifyState(relayInfo.UserId, userSetting.QuotaNotifyState); err != nil {
				common.SysError(fmt.Sprintf("failed to persist quota notify state for user %d: %s", relayInfo.UserId, err.Error()))
			}
		}
		for _, plan := range plans {
			reserved, err := reserveUserQuotaNotifyState(relayInfo.UserId, plan, remainingQuota)
			if err != nil {
				common.SysError(fmt.Sprintf("failed to reserve quota notify state for user %d: %s", relayInfo.UserId, err.Error()))
				continue
			}
			if !reserved {
				continue
			}
			prompt := "您的额度即将用尽"
			topUpLink := PaymentReturnURL("/wallet")

			// 根据通知方式生成不同的内容格式
			var content string
			var values []interface{}

			notifyType := userSetting.NotifyType
			if notifyType == "" {
				notifyType = dto.NotifyTypeEmail
			}

			if notifyType == dto.NotifyTypeBark {
				// Bark推送使用简短文本，不支持HTML
				content = "{{value}}，剩余额度：{{value}}，请及时充值"
				values = []interface{}{prompt, logger.FormatQuota(plan.remainingQuota)}
			} else if notifyType == dto.NotifyTypeGotify {
				content = "{{value}}，当前剩余额度为 {{value}}，请及时充值。"
				values = []interface{}{prompt, logger.FormatQuota(plan.remainingQuota)}
			} else {
				// 默认内容格式，适用于Email和Webhook（支持HTML）
				content = "{{value}}，当前剩余额度为 {{value}}，为了不影响您的使用，请及时充值。<br/>充值链接：<a href='{{value}}'>{{value}}</a>"
				values = []interface{}{prompt, logger.FormatQuota(plan.remainingQuota), topUpLink, topUpLink}
			}

			err = NotifyUser(relayInfo.UserId, relayInfo.UserEmail, relayInfo.UserSetting, dto.NewNotify(plan.notifyType, prompt, content, values))
			if err != nil {
				common.SysError(fmt.Sprintf("failed to send quota notify to user %d: %s", relayInfo.UserId, err.Error()))
				continue
			}
		}
	})
}

func checkAndSendSubscriptionQuotaNotify(relayInfo *relaycommon.RelayInfo) {
	gopool.Go(func() {
		if relayInfo == nil {
			return
		}
		if relayInfo.SubscriptionId == 0 || relayInfo.SubscriptionAmountTotal <= 0 {
			return
		}

		usedAfter := relayInfo.SubscriptionAmountUsedAfterPreConsume + relayInfo.SubscriptionPostDelta
		remaining := relayInfo.SubscriptionAmountTotal - usedAfter
		userSetting := relayInfo.UserSetting
		if userSetting.QuotaNotifyState == nil {
			userSetting.QuotaNotifyState = map[string]int{}
		}
		stateChanged := normalizeQuotaNotifyState(userSetting.QuotaNotifyState, int(remaining))
		plans := filterQuotaNotifyPlansByState(
			quotaNotifyPlans(int(relayInfo.SubscriptionAmountTotal), int(usedAfter), userSetting.QuotaWarningThreshold),
			userSetting.QuotaNotifyState,
		)
		if stateChanged && len(plans) == 0 {
			if err := persistUserQuotaNotifyState(relayInfo.UserId, userSetting.QuotaNotifyState); err != nil {
				common.SysError(fmt.Sprintf("failed to persist subscription quota notify state for user %d: %s", relayInfo.UserId, err.Error()))
			}
		}

		prompt := "您的订阅额度即将用尽"
		topUpLink := PaymentReturnURL("/wallet")

		for _, plan := range plans {
			reserved, err := reserveUserQuotaNotifyState(relayInfo.UserId, plan, int(remaining))
			if err != nil {
				common.SysError(fmt.Sprintf("failed to reserve subscription quota notify state for user %d: %s", relayInfo.UserId, err.Error()))
				continue
			}
			if !reserved {
				continue
			}
			var content string
			var values []interface{}
			notifyType := relayInfo.UserSetting.NotifyType
			if notifyType == "" {
				notifyType = dto.NotifyTypeEmail
			}

			if notifyType == dto.NotifyTypeBark {
				content = "{{value}}，剩余额度：{{value}}，请及时充值"
				values = []interface{}{prompt, logger.FormatQuota(plan.remainingQuota)}
			} else if notifyType == dto.NotifyTypeGotify {
				content = "{{value}}，当前剩余额度为 {{value}}，请及时充值。"
				values = []interface{}{prompt, logger.FormatQuota(plan.remainingQuota)}
			} else {
				content = "{{value}}，当前剩余额度为 {{value}}，为了不影响您的使用，请及时充值。<br/>充值链接：<a href='{{value}}'>{{value}}</a>"
				values = []interface{}{prompt, logger.FormatQuota(plan.remainingQuota), topUpLink, topUpLink}
			}

			if err := NotifyUser(relayInfo.UserId, relayInfo.UserEmail, relayInfo.UserSetting, dto.NewNotify(plan.notifyType, prompt, content, values)); err != nil {
				common.SysError(fmt.Sprintf("failed to send subscription quota notify to user %d: %s", relayInfo.UserId, err.Error()))
				continue
			}
		}
	})
}

func checkAndSendInsufficientQuotaNotify(relayInfo *relaycommon.RelayInfo, remainingQuota int) {
	if relayInfo == nil {
		return
	}
	gopool.Go(func() {
		userSetting := relayInfo.UserSetting
		if userSetting.QuotaNotifyState == nil {
			userSetting.QuotaNotifyState = map[string]int{}
		}
		plan := quotaNotifyPlan{
			threshold:      0,
			notifyType:     dto.NotifyTypeQuotaExceed + "_insufficient",
			remainingQuota: remainingQuota,
		}
		if len(filterQuotaNotifyPlansByState([]quotaNotifyPlan{plan}, userSetting.QuotaNotifyState)) == 0 {
			return
		}
		reserved, err := reserveUserQuotaNotifyState(relayInfo.UserId, plan, remainingQuota)
		if err != nil {
			common.SysError(fmt.Sprintf("failed to reserve insufficient quota notify state for user %d: %s", relayInfo.UserId, err.Error()))
			return
		}
		if !reserved {
			return
		}

		prompt := "您的额度不足"
		topUpLink := PaymentReturnURL("/console/topup")
		var content string
		var values []interface{}
		notifyType := userSetting.NotifyType
		if notifyType == "" {
			notifyType = dto.NotifyTypeEmail
		}
		if notifyType == dto.NotifyTypeBark {
			content = "{{value}}，剩余额度：{{value}}，请及时充值"
			values = []interface{}{prompt, logger.FormatQuota(remainingQuota)}
		} else if notifyType == dto.NotifyTypeGotify {
			content = "{{value}}，当前剩余额度为 {{value}}，请及时充值。"
			values = []interface{}{prompt, logger.FormatQuota(remainingQuota)}
		} else {
			content = "{{value}}，当前剩余额度为 {{value}}，为了不影响您的使用，请及时充值。<br/>充值链接：<a href='{{value}}'>{{value}}</a>"
			values = []interface{}{prompt, logger.FormatQuota(remainingQuota), topUpLink, topUpLink}
		}

		if err := NotifyUser(relayInfo.UserId, relayInfo.UserEmail, relayInfo.UserSetting, dto.NewNotify(plan.notifyType, prompt, content, values)); err != nil {
			common.SysError(fmt.Sprintf("failed to send insufficient quota notify to user %d: %s", relayInfo.UserId, err.Error()))
			return
		}
	})
}

package controller

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	perfmetrics "github.com/QuantumNous/new-api/pkg/perf_metrics"
	"github.com/QuantumNous/new-api/relay"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relay/conversion"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/bytedance/gopkg/util/gopool"
	"github.com/samber/lo"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

func relayHandler(c *gin.Context, info *relaycommon.RelayInfo) *types.NewAPIError {
	var err *types.NewAPIError
	switch info.RelayMode {
	case relayconstant.RelayModeImagesGenerations, relayconstant.RelayModeImagesEdits:
		err = relay.ImageHelper(c, info)
	case relayconstant.RelayModeAudioSpeech:
		fallthrough
	case relayconstant.RelayModeAudioTranslation:
		fallthrough
	case relayconstant.RelayModeAudioTranscription:
		err = relay.AudioHelper(c, info)
	case relayconstant.RelayModeRerank:
		err = relay.RerankHelper(c, info)
	case relayconstant.RelayModeEmbeddings:
		err = relay.EmbeddingHelper(c, info)
	case relayconstant.RelayModeResponses, relayconstant.RelayModeResponsesCompact:
		err = relay.ResponsesHelper(c, info)
	default:
		err = relay.TextHelper(c, info)
	}
	return err
}

func geminiRelayHandler(c *gin.Context, info *relaycommon.RelayInfo) *types.NewAPIError {
	var err *types.NewAPIError
	if strings.Contains(c.Request.URL.Path, "embed") {
		err = relay.GeminiEmbeddingHandler(c, info)
	} else {
		err = relay.GeminiHelper(c, info)
	}
	return err
}

func relayDispatchFormat(entryFormat types.RelayFormat, info *relaycommon.RelayInfo) types.RelayFormat {
	if info != nil && info.RelayFormat != "" {
		return info.RelayFormat
	}
	return entryFormat
}

func Relay(c *gin.Context, relayFormat types.RelayFormat) {

	requestId := c.GetString(common.RequestIdKey)
	//group := common.GetContextKeyString(c, constant.ContextKeyUsingGroup)
	//originalModel := common.GetContextKeyString(c, constant.ContextKeyOriginalModel)

	var (
		newAPIError *types.NewAPIError
		ws          *websocket.Conn
	)

	if relayFormat == types.RelayFormatOpenAIRealtime {
		var err error
		ws, err = upgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			helper.WssError(c, ws, types.NewError(err, types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry()).ToOpenAIError())
			return
		}
		defer ws.Close()
	}

	defer func() {
		if newAPIError != nil {
			service.MarkRetryRouteFinal(c, false, "failed")
			recordRelayErrorLog(c, nil, newAPIError)
			logger.LogError(c, fmt.Sprintf("relay error: %s", newAPIError.Error()))
			newAPIError.SetMessage(common.MessageWithRequestId(newAPIError.Error(), requestId))
			switch relayFormat {
			case types.RelayFormatOpenAIRealtime:
				helper.WssError(c, ws, newAPIError.ToOpenAIError())
			case types.RelayFormatClaude:
				c.JSON(newAPIError.StatusCode, gin.H{
					"type":  "error",
					"error": newAPIError.ToClaudeError(),
				})
			default:
				c.JSON(newAPIError.StatusCode, gin.H{
					"error": newAPIError.ToOpenAIError(),
				})
			}
		} else {
			service.MarkRetryRouteFinal(c, true, "success")
		}
	}()

	request, err := helper.GetAndValidateRequest(c, relayFormat)
	if err != nil {
		// Map "request body too large" to 413 so clients can handle it correctly
		if common.IsRequestBodyTooLargeError(err) || errors.Is(err, common.ErrRequestBodyTooLarge) {
			newAPIError = types.NewErrorWithStatusCode(err, types.ErrorCodeReadRequestBodyFailed, http.StatusRequestEntityTooLarge, types.ErrOptionWithSkipRetry())
		} else {
			newAPIError = types.NewError(err, types.ErrorCodeInvalidRequest)
		}
		return
	}
	relayMode := relayconstant.Path2RelayMode(c.Request.URL.Path)
	if contextRelayMode := c.GetInt("relay_mode"); contextRelayMode != 0 {
		relayMode = contextRelayMode
	}
	request, conversionPlan, err := conversion.ApplyRequest(c, relayFormat, relayMode, request)
	if err != nil {
		newAPIError = types.NewError(err, types.ErrorCodeInvalidRequest)
		return
	}
	if conversionPlan != nil {
		conversionPlan.Store(c)
	}

	relayInfo, err := relaycommon.GenRelayInfo(c, relayFormat, request, ws)
	if err != nil {
		newAPIError = types.NewError(err, types.ErrorCodeGenRelayInfoFailed)
		return
	}
	if relayInfo.IsAsyncImageRequest && relayInfo.RelayMode == relayconstant.RelayModeImagesGenerations {
		relayInfo.ForcePreConsume = true
	}

	needSensitiveCheck := setting.ShouldCheckPromptSensitive()
	needCountToken := constant.CountToken
	// Avoid building huge CombineText (strings.Join) when token counting and sensitive check are both disabled.
	var meta *types.TokenCountMeta
	if needSensitiveCheck || needCountToken {
		meta = request.GetTokenCountMeta()
	} else {
		meta = fastTokenCountMetaForPricing(request)
	}

	if needSensitiveCheck && meta != nil {
		contains, words := service.CheckSensitiveText(meta.CombineText)
		if contains {
			logger.LogWarn(c, fmt.Sprintf("user sensitive words detected: %s", strings.Join(words, ", ")))
			newAPIError = types.NewError(err, types.ErrorCodeSensitiveWordsDetected)
			return
		}
	}

	tokens, err := service.EstimateRequestToken(c, meta, relayInfo)
	if err != nil {
		newAPIError = types.NewError(err, types.ErrorCodeCountTokenFailed)
		return
	}

	relayInfo.SetEstimatePromptTokens(tokens)

	priceData, err := helper.ModelPriceHelper(c, relayInfo, tokens, meta)
	if err != nil {
		newAPIError = types.NewError(err, types.ErrorCodeModelPriceError, types.ErrOptionWithStatusCode(http.StatusBadRequest))
		return
	}

	// common.SetContextKey(c, constant.ContextKeyTokenCountMeta, meta)

	if priceData.FreeModel {
		logger.LogInfo(c, fmt.Sprintf("模型 %s 免费，跳过预扣费", relayInfo.OriginModelName))
	} else {
		newAPIError = service.PreConsumeBilling(c, priceData.QuotaToPreConsume, relayInfo)
		if newAPIError != nil {
			return
		}
	}

	defer func() {
		// Only return quota if downstream failed and quota was actually pre-consumed
		if newAPIError != nil {
			newAPIError = service.NormalizeViolationFeeError(newAPIError)
			if relayInfo.Billing != nil {
				relayInfo.Billing.Refund(c)
			}
			service.ChargeViolationFeeIfNeeded(c, relayInfo, newAPIError)
		}
	}()

	relayInfo.RetryIndex = 0
	relayInfo.LastError = nil

	modelAttempts := buildModelAttempts(relayInfo)
	for attemptIndex, attemptModel := range modelAttempts {
		relayInfo.CurrentModelName = attemptModel
		if attemptIndex > 0 {
			relayInfo.ModelFallbackAttempted = true
			relayInfo.FallbackModelName = attemptModel
			c.Set("model_fallback_primary", relayInfo.OriginModelName)
			c.Set("model_fallback_model", attemptModel)
			c.Set("model_fallback_mode", relayInfo.ModelFallbackMode)
			resetModelAttemptContext(c)
			logger.LogInfo(c, fmt.Sprintf("模型 %s 故障后尝试备选模型 %s", relayInfo.OriginModelName, attemptModel))
		}
		retryParam := &service.RetryParam{
			Ctx:        c,
			TokenGroup: relayInfo.TokenGroup,
			ModelName:  attemptModel,
			Retry:      common.GetPointer(0),
		}
		shouldTryNextModel := false
		for ; retryParam.GetRetry() <= common.RetryTimes; retryParam.IncreaseRetry() {
			relayInfo.RetryIndex = retryParam.GetRetry()
			c.Set("relay_retry_index", retryParam.GetRetry())
			channel, channelErr := getChannel(c, relayInfo, retryParam)
			if channelErr != nil {
				logger.LogError(c, channelErr.Error())
				newAPIError = relayErrorAfterChannelSelectionFailure(relayInfo, channelErr)
				break
			}

			addUsedChannel(c, channel.Id)
			bodyStorage, bodyErr := common.GetBodyStorage(c)
			if bodyErr != nil {
				// Ensure consistent 413 for oversized bodies even when error occurs later (e.g., retry path)
				if common.IsRequestBodyTooLargeError(bodyErr) || errors.Is(bodyErr, common.ErrRequestBodyTooLarge) {
					newAPIError = types.NewErrorWithStatusCode(bodyErr, types.ErrorCodeReadRequestBodyFailed, http.StatusRequestEntityTooLarge, types.ErrOptionWithSkipRetry())
				} else {
					newAPIError = types.NewErrorWithStatusCode(bodyErr, types.ErrorCodeReadRequestBodyFailed, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
				}
				break
			}
			c.Request.Body = io.NopCloser(bodyStorage)

			switch relayDispatchFormat(relayFormat, relayInfo) {
			case types.RelayFormatOpenAIRealtime:
				newAPIError = relay.WssHelper(c, relayInfo)
			case types.RelayFormatClaude:
				newAPIError = relay.ClaudeHelper(c, relayInfo)
			case types.RelayFormatGemini:
				newAPIError = geminiRelayHandler(c, relayInfo)
			default:
				newAPIError = relayHandler(c, relayInfo)
			}

			if newAPIError == nil {
				relayInfo.LastError = nil
				return
			}

			newAPIError = service.NormalizeViolationFeeError(newAPIError)
			relayInfo.LastError = newAPIError

			processChannelError(c, *types.NewChannelError(channel.Id, channel.Type, channel.Name, channel.ChannelInfo.IsMultiKey, common.GetContextKeyString(c, constant.ContextKeyChannelKey), channel.GetAutoBan()), newAPIError)

			remainingRetries := common.RetryTimes - retryParam.GetRetry()
			if !shouldRetry(c, newAPIError, remainingRetries) {
				if attemptIndex == 0 && len(modelAttempts) > 1 && canTryFallbackModel(c, newAPIError) {
					shouldTryNextModel = true
				}
				break
			}
		}
		if !shouldTryNextModel {
			break
		}
	}

	useChannel := c.GetStringSlice("use_channel")
	if len(useChannel) > 1 {
		retryLogStr := fmt.Sprintf("重试：%s", strings.Trim(strings.Join(strings.Fields(fmt.Sprint(useChannel)), "->"), "[]"))
		logger.LogInfo(c, retryLogStr)
	}
	if shouldRecordPerfFailure(newAPIError) {
		gopool.Go(func() {
			perfmetrics.RecordRelaySample(relayInfo, false, 0)
		})
	}
}

func relayErrorAfterChannelSelectionFailure(info *relaycommon.RelayInfo, channelErr *types.NewAPIError) *types.NewAPIError {
	if info != nil && info.LastError != nil {
		return info.LastError
	}
	return channelErr
}

func shouldRecordPerfFailure(err *types.NewAPIError) bool {
	if err == nil {
		return false
	}
	if err.StatusCode >= http.StatusBadRequest &&
		err.StatusCode < http.StatusInternalServerError &&
		err.StatusCode != http.StatusTooManyRequests {
		return false
	}
	return true
}

var upgrader = websocket.Upgrader{
	Subprotocols: []string{"realtime"}, // WS 握手支持的协议，如果有使用 Sec-WebSocket-Protocol，则必须在此声明对应的 Protocol TODO add other protocol
	CheckOrigin: func(r *http.Request) bool {
		return true // 允许跨域
	},
}

func addUsedChannel(c *gin.Context, channelId int) {
	useChannel := c.GetStringSlice("use_channel")
	useChannel = append(useChannel, fmt.Sprintf("%d", channelId))
	c.Set("use_channel", useChannel)
}

func buildModelAttempts(info *relaycommon.RelayInfo) []string {
	if info == nil {
		return nil
	}
	primaryModel := info.OriginModelName
	attempts := []string{primaryModel}
	userFallbackSetting := model_setting.UserModelFallbackSetting{
		Mode: model_setting.ModelFallbackModeInherit,
	}
	if info.UserSetting.ModelFallback != nil {
		parsed, ok := parseUserModelFallbackSetting(info.UserSetting.ModelFallback)
		if ok {
			userFallbackSetting = parsed
		}
	}
	rule, mode, ok := model_setting.ResolveModelFallbackRule(primaryModel, userFallbackSetting)
	info.ModelFallbackMode = mode
	if ok {
		attempts = append(attempts, rule.FallbackModel)
	}
	return attempts
}

func parseUserModelFallbackSetting(raw any) (model_setting.UserModelFallbackSetting, bool) {
	switch value := raw.(type) {
	case model_setting.UserModelFallbackSetting:
		return value, true
	case map[string]any:
		bytes, err := common.Marshal(value)
		if err != nil {
			return model_setting.UserModelFallbackSetting{}, false
		}
		var setting model_setting.UserModelFallbackSetting
		if err := common.Unmarshal(bytes, &setting); err != nil {
			return model_setting.UserModelFallbackSetting{}, false
		}
		return setting, true
	default:
		bytes, err := common.Marshal(value)
		if err != nil {
			return model_setting.UserModelFallbackSetting{}, false
		}
		var setting model_setting.UserModelFallbackSetting
		if err := common.Unmarshal(bytes, &setting); err != nil {
			return model_setting.UserModelFallbackSetting{}, false
		}
		return setting, true
	}
}

func canTryFallbackModel(c *gin.Context, err *types.NewAPIError) bool {
	if err == nil {
		return false
	}
	if c.Writer != nil && c.Writer.Written() {
		return false
	}
	if _, ok := c.Get("specific_channel_id"); ok {
		return false
	}
	if service.ShouldSkipRetryAfterChannelAffinityFailure(c) || types.IsSkipRetryError(err) {
		return false
	}
	code := err.StatusCode
	if code >= 200 && code < 300 {
		return false
	}
	if code < 100 || code > 599 {
		return true
	}
	if operation_setting.IsAlwaysSkipRetryCode(err.GetErrorCode()) {
		return false
	}
	return model_setting.ShouldFallbackByStatusCode(code)
}

func resetModelAttemptContext(c *gin.Context) {
	for _, key := range []constant.ContextKey{
		constant.ContextKeyAutoGroup,
		constant.ContextKeyAutoGroupIndex,
		constant.ContextKeyAutoGroupRetryIndex,
	} {
		delete(c.Keys, string(key))
	}
}

func fastTokenCountMetaForPricing(request dto.Request) *types.TokenCountMeta {
	if request == nil {
		return &types.TokenCountMeta{}
	}
	meta := &types.TokenCountMeta{
		TokenType: types.TokenTypeTokenizer,
	}
	switch r := request.(type) {
	case *dto.GeneralOpenAIRequest:
		maxCompletionTokens := lo.FromPtrOr(r.MaxCompletionTokens, uint(0))
		maxTokens := lo.FromPtrOr(r.MaxTokens, uint(0))
		if maxCompletionTokens > maxTokens {
			meta.MaxTokens = int(maxCompletionTokens)
		} else {
			meta.MaxTokens = int(maxTokens)
		}
	case *dto.OpenAIResponsesRequest:
		meta.MaxTokens = int(lo.FromPtrOr(r.MaxOutputTokens, uint(0)))
	case *dto.ClaudeRequest:
		meta.MaxTokens = int(lo.FromPtr(r.MaxTokens))
	case *dto.ImageRequest:
		// Pricing for image requests depends on ImagePriceRatio; safe to compute even when CountToken is disabled.
		return r.GetTokenCountMeta()
	default:
		// Best-effort: leave CombineText empty to avoid large allocations.
	}
	return meta
}

func getChannel(c *gin.Context, info *relaycommon.RelayInfo, retryParam *service.RetryParam) (*model.Channel, *types.NewAPIError) {
	if info.ChannelMeta == nil {
		autoBan := c.GetBool("auto_ban")
		autoBanInt := 1
		if !autoBan {
			autoBanInt = 0
		}
		return &model.Channel{
			Id:      c.GetInt("channel_id"),
			Type:    c.GetInt("channel_type"),
			Name:    c.GetString("channel_name"),
			AutoBan: &autoBanInt,
		}, nil
	}
	channel, selectGroup, err := service.CacheGetRandomSatisfiedChannel(retryParam)

	info.PriceData.GroupRatioInfo = helper.HandleGroupRatio(c, info)

	if err != nil {
		return nil, types.NewError(fmt.Errorf("获取分组 %s 下模型 %s 的可用渠道失败（retry）: %s", selectGroup, retryParam.ModelName, err.Error()), types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry())
	}
	if channel == nil {
		return nil, types.NewError(fmt.Errorf("分组 %s 下模型 %s 的可用渠道不存在（retry）", selectGroup, retryParam.ModelName), types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry())
	}

	newAPIError := middleware.SetupContextForSelectedChannel(c, channel, retryParam.ModelName)
	if newAPIError != nil {
		return nil, newAPIError
	}
	service.UpdateCurrentRetryRouteTarget(c, channel, selectGroup)
	return channel, nil
}

func shouldRetry(c *gin.Context, openaiErr *types.NewAPIError, retryTimes int) bool {
	if openaiErr == nil {
		return false
	}
	if !selectedChannelAllowsRetry(c) {
		return false
	}
	retryIndex, _ := c.Get("relay_retry_index")
	if currentRetry, ok := retryIndex.(int); ok && service.RetryPolicyRecoveryExceeded(c, currentRetry) {
		return false
	}
	if decision := operation_setting.ShouldRetryByPolicy(buildRetryPolicyInput(c, openaiErr)); decision.Matched {
		return shouldRetryByPolicyDecision(c, decision, retryIndex, openaiErr)
	}
	if types.IsSkipRetryError(openaiErr) {
		return false
	}
	if types.IsChannelError(openaiErr) {
		return true
	}
	if retryTimes <= 0 {
		return false
	}
	if _, ok := c.Get("specific_channel_id"); ok {
		return false
	}
	code := openaiErr.StatusCode
	if code >= 200 && code < 300 {
		return false
	}
	if code < 100 || code > 599 {
		return true
	}
	if operation_setting.IsAlwaysSkipRetryCode(openaiErr.GetErrorCode()) {
		return false
	}
	if service.ShouldSkipRetryAfterChannelAffinityFailure(c) {
		return false
	}
	return operation_setting.ShouldRetryByStatusCode(code)
}

func shouldRetryByPolicyDecision(c *gin.Context, decision operation_setting.RetryPolicyDecision, retryIndex any, openaiErr *types.NewAPIError) bool {
	if !decision.ShouldRetry {
		service.RecordRetryRouteDecision(c, decision, openaiErr)
		service.ClearRetryPolicyRecovery(c)
		return false
	}
	if currentRetry, ok := retryIndex.(int); ok && decision.MaxRetries > 0 && currentRetry >= decision.MaxRetries {
		service.SetRetryPolicyRecovery(c, decision)
		return false
	}
	service.SetRetryPolicyRecovery(c, decision)
	if service.ShouldSkipRetryAfterChannelAffinityFailure(c) && !service.RetryPolicyRecoveryAllowsAffinityRetry(c) {
		return false
	}
	service.RecordRetryRouteDecision(c, decision, openaiErr)
	return true
}

func buildRetryPolicyInput(c *gin.Context, err *types.NewAPIError) operation_setting.RetryPolicyInput {
	input := operation_setting.RetryPolicyInput{}
	if err != nil {
		input.ErrorType = err.GetErrorType()
		input.ErrorCode = err.GetErrorCode()
		input.StatusCode = err.StatusCode
		input.ErrorMessage = err.Error()
	}
	if c == nil {
		return input
	}
	input.ModelName = c.GetString("original_model")
	if input.ModelName == "" {
		input.ModelName = common.GetContextKeyString(c, constant.ContextKeyOriginalModel)
	}
	input.ChannelID = common.GetContextKeyInt(c, constant.ContextKeyChannelId)
	input.ChannelType = common.GetContextKeyInt(c, constant.ContextKeyChannelType)
	input.Group = common.GetContextKeyString(c, constant.ContextKeyUserGroup)
	if input.Group == "" {
		input.Group = c.GetString("group")
	}
	if c.Request != nil && c.Request.URL != nil {
		input.RequestPath = c.Request.URL.Path
	}
	input.IsStream = common.GetContextKeyBool(c, constant.ContextKeyIsStream)
	input.TokenID = common.GetContextKeyInt(c, constant.ContextKeyTokenId)
	input.WorkspaceID = retryPolicyInputWorkspaceID(input.TokenID)
	if channelSetting, ok := common.GetContextKeyType[dto.ChannelSettings](c, constant.ContextKeyChannelSetting); ok {
		input.ChannelRules = retryPolicyRulesToOperationRules(channelSetting.RetryPolicyRules)
	}
	return input
}

func retryPolicyInputWorkspaceID(tokenID int) int {
	if tokenID <= 0 || model.DB == nil {
		return 0
	}
	var token model.Token
	if err := model.DB.Select("id", "workspace_id").Where("id = ?", tokenID).First(&token).Error; err != nil {
		return 0
	}
	return token.WorkspaceId
}

func retryPolicyRulesToOperationRules(rules []dto.RetryPolicyRule) []operation_setting.RetryPolicyRule {
	if len(rules) == 0 {
		return nil
	}
	result := make([]operation_setting.RetryPolicyRule, 0, len(rules))
	for _, rule := range rules {
		result = append(result, operation_setting.RetryPolicyRule{
			Enabled:         rule.Enabled,
			Priority:        rule.Priority,
			Name:            rule.Name,
			Action:          rule.Action,
			Targets:         retryPolicyTargetsToOperationTargets(rule.Targets),
			Strategy:        retryPolicyStrategyToOperationStrategy(rule.Strategy),
			Models:          rule.Models,
			Groups:          rule.Groups,
			RequestPaths:    rule.RequestPaths,
			Stream:          rule.Stream,
			TokenIDs:        rule.TokenIDs,
			WorkspaceIDs:    rule.WorkspaceIDs,
			ChannelIDs:      rule.ChannelIDs,
			ChannelTypes:    rule.ChannelTypes,
			ErrorTypes:      rule.ErrorTypes,
			ErrorCodes:      rule.ErrorCodes,
			StatusCodes:     rule.StatusCodes,
			MessageContains: rule.MessageContains,
			RetryGroups:     rule.RetryGroups,
			MaxRetries:      rule.MaxRetries,
			Conditions: operation_setting.RetryPolicyConditions{
				Models:          rule.Conditions.Models,
				Groups:          rule.Conditions.Groups,
				RequestPaths:    rule.Conditions.RequestPaths,
				Stream:          rule.Conditions.Stream,
				TokenIDs:        rule.Conditions.TokenIDs,
				WorkspaceIDs:    rule.Conditions.WorkspaceIDs,
				ChannelIDs:      rule.Conditions.ChannelIDs,
				ChannelTypes:    rule.Conditions.ChannelTypes,
				ErrorTypes:      rule.Conditions.ErrorTypes,
				ErrorCodes:      rule.Conditions.ErrorCodes,
				StatusCodes:     rule.Conditions.StatusCodes,
				MessageContains: rule.Conditions.MessageContains,
			},
		})
	}
	return result
}

func retryPolicyTargetsToOperationTargets(targets dto.RetryPolicyTargets) operation_setting.RetryPolicyTargets {
	return operation_setting.RetryPolicyTargets{
		Groups:      targets.Groups,
		ChannelIDs:  targets.ChannelIDs,
		ChannelTags: targets.ChannelTags,
		Model:       targets.Model,
	}
}

func retryPolicyStrategyToOperationStrategy(strategy dto.RetryPolicyStrategy) operation_setting.RetryPolicyStrategy {
	return operation_setting.RetryPolicyStrategy{
		MaxRetries:           strategy.MaxRetries,
		ExcludeFailedChannel: strategy.ExcludeFailedChannel,
		PreferHealthy:        strategy.PreferHealthy,
		ProtectLast:          strategy.ProtectLast,
		RecordRequestLog:     strategy.RecordRequestLog,
		SampleRate:           strategy.SampleRate,
	}
}

func processChannelError(c *gin.Context, channelError types.ChannelError, err *types.NewAPIError) {
	logger.LogError(c, fmt.Sprintf("channel error (channel #%d, status code: %d): %s", channelError.ChannelId, err.StatusCode, err.Error()))
	// 不要使用context获取渠道信息，异步处理时可能会出现渠道信息不一致的情况
	// do not use context to get channel info, there may be inconsistent channel info when processing asynchronously
	if service.ShouldDisableChannel(err) && channelError.AutoBan {
		gopool.Go(func() {
			service.DisableChannel(channelError, err.ErrorWithStatusCode())
		})
	}

	recordRelayErrorLog(c, &channelError, err)

}

const ginKeyRelayErrorLogID = "relay_error_log_id"

func recordRelayErrorLog(c *gin.Context, channelError *types.ChannelError, err *types.NewAPIError) int {
	if c == nil || err == nil {
		return 0
	}
	if channelError == nil {
		if existing, ok := c.Get(ginKeyRelayErrorLogID); ok {
			if id, ok := existing.(int); ok && id > 0 {
				return id
			}
		}
	}
	if existing, ok := c.Get(relayErrorLogDedupKey(c, channelError, err)); ok {
		if id, ok := existing.(int); ok && id > 0 {
			return id
		}
	}
	if !constant.ErrorLogEnabled || !types.IsRecordErrorLog(err) {
		return 0
	}

	userId := common.GetContextKeyInt(c, constant.ContextKeyUserId)
	tokenName := c.GetString("token_name")
	modelName := common.GetContextKeyString(c, constant.ContextKeyOriginalModel)
	if modelName == "" {
		modelName = c.GetString("original_model")
	}
	tokenId := common.GetContextKeyInt(c, constant.ContextKeyTokenId)
	userGroup := common.GetContextKeyString(c, constant.ContextKeyUsingGroup)
	if userGroup == "" {
		userGroup = c.GetString("group")
	}
	channelId := common.GetContextKeyInt(c, constant.ContextKeyChannelId)
	channelName := common.GetContextKeyString(c, constant.ContextKeyChannelName)
	channelType := common.GetContextKeyInt(c, constant.ContextKeyChannelType)
	if channelError != nil && channelId == 0 {
		channelId = channelError.ChannelId
	}

	requestRecord := buildRelayErrorRequestLog(c, err)
	other := buildRelayErrorLogOther(c, err, channelId, channelName, channelType, requestRecord)
	startTime := common.GetContextKeyTime(c, constant.ContextKeyRequestStartTime)
	if startTime.IsZero() {
		startTime = time.Now()
	}
	useTimeSeconds := int(time.Since(startTime).Seconds())
	logID := model.CreateErrorLog(c, userId, channelId, modelName, tokenName, err.MaskSensitiveErrorWithStatusCode(), tokenId, useTimeSeconds, common.GetContextKeyBool(c, constant.ContextKeyIsStream), userGroup, other)
	if logID > 0 {
		if requestRecord != nil {
			requestRecord.LogId = logID
			if !model.EnqueueErrorRequestLog(requestRecord) {
				logger.LogError(c, "error request log queue is full; dropped request evidence")
			}
		}
		c.Set(ginKeyRelayErrorLogID, logID)
		c.Set(relayErrorLogDedupKey(c, channelError, err), logID)
		service.AttachRetryRouteLog(c, logID)
	}
	return logID
}

func relayErrorLogDedupKey(c *gin.Context, channelError *types.ChannelError, err *types.NewAPIError) string {
	channelId := 0
	if channelError != nil {
		channelId = channelError.ChannelId
	} else {
		channelId = common.GetContextKeyInt(c, constant.ContextKeyChannelId)
	}
	return fmt.Sprintf("relay_error_log_id:%d:%d:%s:%s", channelId, err.StatusCode, err.GetErrorCode(), err.Error())
}

func buildRelayErrorLogOther(c *gin.Context, err *types.NewAPIError, channelId int, channelName string, channelType int, requestRecord *model.ErrorRequestLog) map[string]interface{} {
	other := make(map[string]interface{})
	if c.Request != nil && c.Request.URL != nil {
		other["request_path"] = c.Request.URL.Path
		other["request_method"] = c.Request.Method
	}
	other["error_type"] = err.GetErrorType()
	other["error_code"] = err.GetErrorCode()
	other["status_code"] = err.StatusCode
	other["channel_id"] = channelId
	other["channel_name"] = channelName
	other["channel_type"] = channelType
	if eventID, ok := service.GetCurrentRetryRouteEventID(c); ok {
		other["retry_route_event_ids"] = []int{eventID}
	}
	other["request_log_lookup"] = map[string]interface{}{
		"request_id": c.GetString(common.RequestIdKey),
	}
	if requestRecord != nil && requestRecord.RequestHash != "" {
		other["request_hash"] = requestRecord.RequestHash
	}
	other["admin_info"] = buildRelayErrorAdminInfo(c)
	return other
}

func buildRelayErrorAdminInfo(c *gin.Context) map[string]interface{} {
	adminInfo := make(map[string]interface{})
	adminInfo["use_channel"] = c.GetStringSlice("use_channel")
	if c.GetString("model_fallback_model") != "" {
		adminInfo["model_fallback"] = map[string]interface{}{
			"primary_model":  c.GetString("model_fallback_primary"),
			"fallback_model": c.GetString("model_fallback_model"),
			"mode":           c.GetString("model_fallback_mode"),
		}
	}
	isMultiKey := common.GetContextKeyBool(c, constant.ContextKeyChannelIsMultiKey)
	if isMultiKey {
		adminInfo["is_multi_key"] = true
		adminInfo["multi_key_index"] = common.GetContextKeyInt(c, constant.ContextKeyChannelMultiKeyIndex)
	}
	service.AppendChannelAffinityAdminInfo(c, adminInfo)
	service.AppendRetryPolicyRecoveryAdminInfo(c, adminInfo)
	return adminInfo
}

func buildRelayErrorRequestLog(c *gin.Context, err *types.NewAPIError) *model.ErrorRequestLog {
	record := &model.ErrorRequestLog{
		RequestId:         c.GetString(common.RequestIdKey),
		UpstreamRequestId: c.GetString(common.UpstreamRequestIdKey),
		UserId:            common.GetContextKeyInt(c, constant.ContextKeyUserId),
		Username:          common.GetContextKeyString(c, constant.ContextKeyUserName),
		TokenId:           common.GetContextKeyInt(c, constant.ContextKeyTokenId),
		TokenName:         c.GetString("token_name"),
		ModelName:         common.GetContextKeyString(c, constant.ContextKeyOriginalModel),
		Group:             common.GetContextKeyString(c, constant.ContextKeyUsingGroup),
		IsStream:          common.GetContextKeyBool(c, constant.ContextKeyIsStream),
		ErrorType:         string(err.GetErrorType()),
		ErrorCode:         string(err.GetErrorCode()),
		StatusCode:        err.StatusCode,
	}
	if record.ModelName == "" {
		record.ModelName = c.GetString("original_model")
	}
	if record.Group == "" {
		record.Group = c.GetString("group")
	}
	if c.Request != nil {
		record.RequestMethod = c.Request.Method
		if c.Request.URL != nil {
			record.RequestPath = c.Request.URL.Path
			record.RequestURL = c.Request.URL.RequestURI()
		}
		record.ContentLength = c.Request.ContentLength
	}
	body := readRelayErrorRequestBody(c)
	headers := relayErrorRequestHeaders(c)
	requestBody, requestHash, requestTruncated, requestHeaders := model.PrepareRequestSnapshotForErrorLog(body, headers)
	record.RequestBody = requestBody
	record.RequestHash = requestHash
	record.RequestTruncated = requestTruncated
	record.RequestHeaders = requestHeaders
	if record.RequestId == "" && record.RequestHash == "" && record.RequestPath == "" {
		return nil
	}
	return record
}

func readRelayErrorRequestBody(c *gin.Context) string {
	cachedStorage, exists := c.Get(common.KeyBodyStorage)
	if !exists || cachedStorage == nil {
		return ""
	}
	storage, ok := cachedStorage.(common.BodyStorage)
	if !ok {
		return ""
	}
	if _, err := storage.Seek(0, io.SeekStart); err != nil {
		return ""
	}
	limit := common.RequestLogMaxRequestBytes
	if limit <= 0 {
		limit = 256 * 1024
	}
	buf := bytes.NewBuffer(make([]byte, 0, min(limit, 32*1024)))
	n, err := io.CopyN(buf, storage, int64(limit)+1)
	truncated := n > int64(limit)
	if err != nil && !errors.Is(err, io.EOF) {
		return ""
	}
	data := buf.Bytes()
	if truncated && len(data) > limit {
		data = data[:limit]
	}
	if _, err = storage.Seek(0, io.SeekStart); err == nil && c.Request != nil {
		c.Request.Body = io.NopCloser(storage)
	}
	if truncated {
		return string(data) + "...[CAPTURED_TRUNCATED]"
	}
	return string(data)
}

func relayErrorRequestHeaders(c *gin.Context) string {
	if c == nil || c.Request == nil || len(c.Request.Header) == 0 {
		return ""
	}
	headers := make(map[string][]string, len(c.Request.Header))
	for key, values := range c.Request.Header {
		headers[key] = append([]string(nil), values...)
	}
	data, err := common.Marshal(headers)
	if err != nil {
		return ""
	}
	return string(data)
}

func RelayMidjourney(c *gin.Context) {
	relayInfo, err := relaycommon.GenRelayInfo(c, types.RelayFormatMjProxy, nil, nil)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"description": fmt.Sprintf("failed to generate relay info: %s", err.Error()),
			"type":        "upstream_error",
			"code":        4,
		})
		return
	}

	var mjErr *dto.MidjourneyResponse
	switch relayInfo.RelayMode {
	case relayconstant.RelayModeMidjourneyNotify:
		mjErr = relay.RelayMidjourneyNotify(c)
	case relayconstant.RelayModeMidjourneyTaskFetch, relayconstant.RelayModeMidjourneyTaskFetchByCondition:
		mjErr = relay.RelayMidjourneyTask(c, relayInfo.RelayMode)
	case relayconstant.RelayModeMidjourneyTaskImageSeed:
		mjErr = relay.RelayMidjourneyTaskImageSeed(c)
	case relayconstant.RelayModeSwapFace:
		mjErr = relay.RelaySwapFace(c, relayInfo)
	default:
		mjErr = relay.RelayMidjourneySubmit(c, relayInfo)
	}
	//err = relayMidjourneySubmit(c, relayMode)
	log.Println(mjErr)
	if mjErr != nil {
		statusCode := http.StatusBadRequest
		if mjErr.Code == 30 {
			mjErr.Result = "当前分组负载已饱和，请稍后再试，或升级账户以提升服务质量。"
			statusCode = http.StatusTooManyRequests
		}
		c.JSON(statusCode, gin.H{
			"description": fmt.Sprintf("%s %s", mjErr.Description, mjErr.Result),
			"type":        "upstream_error",
			"code":        mjErr.Code,
		})
		channelId := c.GetInt("channel_id")
		logger.LogError(c, fmt.Sprintf("relay error (channel #%d, status code %d): %s", channelId, statusCode, fmt.Sprintf("%s %s", mjErr.Description, mjErr.Result)))
	}
}

func RelayNotImplemented(c *gin.Context) {
	err := types.OpenAIError{
		Message: "API not implemented",
		Type:    "new_api_error",
		Param:   "",
		Code:    "api_not_implemented",
	}
	c.JSON(http.StatusNotImplemented, gin.H{
		"error": err,
	})
}

func RelayNotFound(c *gin.Context) {
	err := types.OpenAIError{
		Message: fmt.Sprintf("Invalid URL (%s %s)", c.Request.Method, c.Request.URL.Path),
		Type:    "invalid_request_error",
		Param:   "",
		Code:    "",
	}
	c.JSON(http.StatusNotFound, gin.H{
		"error": err,
	})
}

func RelayTaskFetch(c *gin.Context) {
	relayInfo, err := relaycommon.GenRelayInfo(c, types.RelayFormatTask, nil, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, &dto.TaskError{
			Code:       "gen_relay_info_failed",
			Message:    err.Error(),
			StatusCode: http.StatusInternalServerError,
		})
		return
	}
	if relayInfo.RelayMode == relayconstant.RelayModeImageTaskFetchByID {
		if taskErr := relay.ImageTaskFetch(c); taskErr != nil {
			respondTaskError(c, taskErr)
		}
		return
	}
	if taskErr := relay.RelayTaskFetch(c, relayInfo.RelayMode); taskErr != nil {
		respondTaskError(c, taskErr)
	}
}

func RelayTask(c *gin.Context) {
	relayInfo, err := relaycommon.GenRelayInfo(c, types.RelayFormatTask, nil, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, &dto.TaskError{
			Code:       "gen_relay_info_failed",
			Message:    err.Error(),
			StatusCode: http.StatusInternalServerError,
		})
		return
	}

	if taskErr := relay.ResolveOriginTask(c, relayInfo); taskErr != nil {
		respondTaskError(c, taskErr)
		return
	}

	var result *relay.TaskSubmitResult
	var taskErr *dto.TaskError
	defer func() {
		if taskErr != nil && relayInfo.Billing != nil {
			relayInfo.Billing.Refund(c)
		}
	}()

	retryParam := &service.RetryParam{
		Ctx:        c,
		TokenGroup: relayInfo.TokenGroup,
		ModelName:  relayInfo.OriginModelName,
		Retry:      common.GetPointer(0),
	}

	for ; retryParam.GetRetry() <= common.RetryTimes; retryParam.IncreaseRetry() {
		var channel *model.Channel

		if lockedCh, ok := relayInfo.LockedChannel.(*model.Channel); ok && lockedCh != nil {
			channel = lockedCh
			if retryParam.GetRetry() > 0 {
				if setupErr := middleware.SetupContextForSelectedChannel(c, channel, relayInfo.OriginModelName); setupErr != nil {
					taskErr = service.TaskErrorWrapperLocal(setupErr.Err, "setup_locked_channel_failed", http.StatusInternalServerError)
					break
				}
			}
		} else {
			var channelErr *types.NewAPIError
			channel, channelErr = getChannel(c, relayInfo, retryParam)
			if channelErr != nil {
				logger.LogError(c, channelErr.Error())
				taskErr = service.TaskErrorWrapperLocal(channelErr.Err, "get_channel_failed", http.StatusInternalServerError)
				break
			}
		}

		addUsedChannel(c, channel.Id)
		bodyStorage, bodyErr := common.GetBodyStorage(c)
		if bodyErr != nil {
			if common.IsRequestBodyTooLargeError(bodyErr) || errors.Is(bodyErr, common.ErrRequestBodyTooLarge) {
				taskErr = service.TaskErrorWrapperLocal(bodyErr, "read_request_body_failed", http.StatusRequestEntityTooLarge)
			} else {
				taskErr = service.TaskErrorWrapperLocal(bodyErr, "read_request_body_failed", http.StatusBadRequest)
			}
			break
		}
		c.Request.Body = io.NopCloser(bodyStorage)

		result, taskErr = relay.RelayTaskSubmit(c, relayInfo)
		if taskErr == nil {
			break
		}

		if !taskErr.LocalError {
			processChannelError(c,
				*types.NewChannelError(channel.Id, channel.Type, channel.Name, channel.ChannelInfo.IsMultiKey,
					common.GetContextKeyString(c, constant.ContextKeyChannelKey), channel.GetAutoBan()),
				types.NewOpenAIError(taskErr.Error, types.ErrorCodeBadResponseStatusCode, taskErr.StatusCode))
		}

		if !shouldRetryTaskRelay(c, channel.Id, taskErr, common.RetryTimes-retryParam.GetRetry()) {
			break
		}
	}

	useChannel := c.GetStringSlice("use_channel")
	if len(useChannel) > 1 {
		retryLogStr := fmt.Sprintf("重试：%s", strings.Trim(strings.Join(strings.Fields(fmt.Sprint(useChannel)), "->"), "[]"))
		logger.LogInfo(c, retryLogStr)
	}

	// ── 成功：结算 + 日志 + 插入任务 ──
	if taskErr == nil {
		if settleErr := service.SettleBilling(c, relayInfo, result.Quota); settleErr != nil {
			common.SysError("settle task billing error: " + settleErr.Error())
		}
		service.LogTaskConsumption(c, relayInfo)

		task := model.InitTask(result.Platform, relayInfo)
		task.PrivateData.UpstreamTaskID = result.UpstreamTaskID
		task.PrivateData.BillingSource = relayInfo.BillingSource
		task.PrivateData.SubscriptionId = relayInfo.SubscriptionId
		task.PrivateData.TokenId = relayInfo.TokenId
		task.TokenId = relayInfo.TokenId
		task.TokenName = c.GetString("token_name")
		task.PrivateData.BillingContext = &model.TaskBillingContext{
			ModelPrice:            relayInfo.PriceData.ModelPrice,
			GroupRatio:            relayInfo.PriceData.GroupRatioInfo.GroupRatio,
			ModelRatio:            relayInfo.PriceData.ModelRatio,
			CompletionRatio:       relayInfo.PriceData.CompletionRatio,
			CacheRatio:            relayInfo.PriceData.CacheRatio,
			CacheCreationRatio:    relayInfo.PriceData.CacheCreationRatio,
			CacheCreation5mRatio:  relayInfo.PriceData.CacheCreation5mRatio,
			CacheCreation1hRatio:  relayInfo.PriceData.CacheCreation1hRatio,
			ImageRatio:            relayInfo.PriceData.ImageRatio,
			AudioRatio:            relayInfo.PriceData.AudioRatio,
			AudioCompletionRatio:  relayInfo.PriceData.AudioCompletionRatio,
			OtherRatios:           relayInfo.PriceData.OtherRatios,
			OriginModelName:       relayInfo.OriginModelName,
			PerCallBilling:        common.StringsContains(constant.TaskPricePatches, relayInfo.OriginModelName) || relayInfo.PriceData.UsePrice,
			TieredBillingSnapshot: relayInfo.TieredBillingSnapshot,
			BillingRequestInput:   relayInfo.BillingRequestInput,
			AgentBillingSnapshot:  relayInfo.AgentBillingSnapshot,
		}
		task.Quota = result.Quota
		task.Data = result.TaskData
		task.Action = relayInfo.Action
		if insertErr := task.Insert(); insertErr != nil {
			common.SysError("insert task error: " + insertErr.Error())
		}
	}

	if taskErr != nil {
		respondTaskError(c, taskErr)
	}
}

// respondTaskError 统一输出 Task 错误响应（含 429 限流提示改写）
func respondTaskError(c *gin.Context, taskErr *dto.TaskError) {
	if taskErr.StatusCode == http.StatusTooManyRequests {
		taskErr.Message = "当前分组上游负载已饱和，请稍后再试"
	}
	c.JSON(taskErr.StatusCode, taskErr)
}

func shouldRetryTaskRelay(c *gin.Context, channelId int, taskErr *dto.TaskError, retryTimes int) bool {
	if taskErr == nil {
		return false
	}
	if !selectedChannelAllowsRetry(c) {
		return false
	}
	if service.ShouldSkipRetryAfterChannelAffinityFailure(c) {
		return false
	}
	if retryTimes <= 0 {
		return false
	}
	if _, ok := c.Get("specific_channel_id"); ok {
		return false
	}
	if taskErr.StatusCode == http.StatusTooManyRequests {
		return true
	}
	if taskErr.StatusCode == 307 {
		return true
	}
	if taskErr.StatusCode/100 == 5 {
		// 超时不重试
		if operation_setting.IsAlwaysSkipRetryStatusCode(taskErr.StatusCode) {
			return false
		}
		return true
	}
	if taskErr.StatusCode == http.StatusBadRequest {
		return false
	}
	if taskErr.StatusCode == 408 {
		// azure处理超时不重试
		return false
	}
	if taskErr.LocalError {
		return false
	}
	if taskErr.StatusCode/100 == 2 {
		return false
	}
	return true
}

func selectedChannelAllowsRetry(c *gin.Context) bool {
	if c == nil {
		return true
	}
	value, exists := common.GetContextKey(c, constant.ContextKeyChannelRetryEnabled)
	if !exists {
		return true
	}
	enabled, ok := value.(bool)
	if !ok {
		return true
	}
	return enabled
}

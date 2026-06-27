package service

import (
	"errors"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/conversion"
	agentservice "github.com/QuantumNous/new-api/service/agent"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

const contextKeyConfigurableNativeProfileIDs = "configurable_native_profile_ids"

type RetryParam struct {
	Ctx          *gin.Context
	TokenGroup   string
	ModelName    string
	Retry        *int
	resetNextTry bool
}

func (p *RetryParam) GetRetry() int {
	if p.Retry == nil {
		return 0
	}
	return *p.Retry
}

func (p *RetryParam) SetRetry(retry int) {
	p.Retry = &retry
}

func (p *RetryParam) IncreaseRetry() {
	if p.resetNextTry {
		p.resetNextTry = false
		return
	}
	if p.Retry == nil {
		p.Retry = new(int)
	}
	*p.Retry++
}

func (p *RetryParam) ResetRetryNextTry() {
	p.resetNextTry = true
}

// CacheGetRandomSatisfiedChannel tries to get a random channel that satisfies the requirements.
// 尝试获取一个满足要求的随机渠道。
//
// For "auto" tokenGroup with cross-group Retry enabled:
// 对于启用了跨分组重试的 "auto" tokenGroup：
//
//   - Each group will exhaust all its priorities before moving to the next group.
//     每个分组会用完所有优先级后才会切换到下一个分组。
//
//   - Uses ContextKeyAutoGroupIndex to track current group index.
//     使用 ContextKeyAutoGroupIndex 跟踪当前分组索引。
//
//   - Uses ContextKeyAutoGroupRetryIndex to track the global Retry count when current group started.
//     使用 ContextKeyAutoGroupRetryIndex 跟踪当前分组开始时的全局重试次数。
//
//   - priorityRetry = Retry - startRetryIndex, represents the priority level within current group.
//     priorityRetry = Retry - startRetryIndex，表示当前分组内的优先级级别。
//
//   - When GetRandomSatisfiedChannel returns nil (priorities exhausted), moves to next group.
//     当 GetRandomSatisfiedChannel 返回 nil（优先级用完）时，切换到下一个分组。
//
// Example flow (2 groups, each with 2 priorities, RetryTimes=3):
// 示例流程（2个分组，每个有2个优先级，RetryTimes=3）：
//
//	Retry=0: GroupA, priority0 (startRetryIndex=0, priorityRetry=0)
//	         分组A, 优先级0
//
//	Retry=1: GroupA, priority1 (startRetryIndex=0, priorityRetry=1)
//	         分组A, 优先级1
//
//	Retry=2: GroupA exhausted → GroupB, priority0 (startRetryIndex=2, priorityRetry=0)
//	         分组A用完 → 分组B, 优先级0
//
//	Retry=3: GroupB, priority1 (startRetryIndex=2, priorityRetry=1)
//	         分组B, 优先级1
func CacheGetRandomSatisfiedChannel(param *RetryParam) (*model.Channel, string, error) {
	var channel *model.Channel
	var err error
	selectGroup := param.TokenGroup
	channelGroup := param.TokenGroup
	userGroup := common.GetContextKeyString(param.Ctx, constant.ContextKeyUserGroup)
	agentCtx, _ := common.GetContextKeyType[*types.AgentContext](param.Ctx, constant.ContextKeyAgentContext)
	systemChannelGroups := []string{channelGroup}
	if agentCtx != nil {
		systemChannelGroups = agentservice.ResolveSystemGroups(agentCtx, param.TokenGroup)
		if len(systemChannelGroups) > 0 {
			channelGroup = systemChannelGroups[0]
		}
		if groups := agentservice.ResolveSystemGroups(agentCtx, userGroup); len(groups) > 0 {
			userGroup = groups[0]
		}
	}
	filter := BuildProtocolChannelFilter(param)

	if recoveryGroup, ok := RetryPolicyRecoveryGroupForAttempt(param.Ctx, param.GetRetry()); ok {
		if err := validateRetryPolicyRecoveryGroup(param.Ctx, recoveryGroup); err != nil {
			return nil, recoveryGroup, err
		}
		if model.IsUserOwnedProviderGroup(recoveryGroup) {
			channel, err = model.GetUserOwnedProviderChannelForGroup(common.GetContextKeyInt(param.Ctx, constant.ContextKeyUserId), recoveryGroup, param.ModelName)
			if err == nil {
				common.SetContextKey(param.Ctx, constant.ContextKeyTokenBillingSource, BillingSourceUserOwnedProvider)
			}
		} else {
			channel, err = model.GetRandomSatisfiedChannelWithFilter(recoveryGroup, param.ModelName, 0, filter)
		}
		if errors.Is(err, model.ErrNoChannelMatchedFilter) {
			return nil, recoveryGroup, unsupportedProtocolError(param)
		}
		if err != nil {
			return nil, recoveryGroup, err
		}
		if channel == nil {
			return nil, recoveryGroup, nil
		}
		setSelectedGroupContext(param.Ctx, recoveryGroup, recoveryGroup)
		return channel, recoveryGroup, nil
	}

	policyGroups := ResolveTokenGroupChain(param.Ctx, param.TokenGroup)
	hasRoutingStrategyPolicy := IsRoutingStrategyTokenPolicy(param.Ctx)
	if len(policyGroups) > 0 || hasRoutingStrategyPolicy {
		startGroupIndex := 0
		agentPolicyGroupNames := map[string]string{}
		if agentCtx != nil {
			_, agentPolicyGroupNames = agentAutoGroups(agentCtx, userGroup)
		}
		crossGroupRetry := common.GetContextKeyBool(param.Ctx, constant.ContextKeyTokenCrossGroupRetry)
		advanceGroupAfterFailure := hasRoutingStrategyPolicy && param.GetRetry() > 0

		if lastGroupIndex, exists := common.GetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex); exists {
			if idx, ok := lastGroupIndex.(int); ok {
				startGroupIndex = idx
			}
		}
		if advanceGroupAfterFailure && startGroupIndex+1 < len(policyGroups) {
			startGroupIndex++
			common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex, startGroupIndex)
			param.SetRetry(0)
		}

		for i := startGroupIndex; i < len(policyGroups); i++ {
			policyGroup := policyGroups[i]
			priorityRetry := param.GetRetry()
			if i > startGroupIndex {
				priorityRetry = 0
			}
			logger.LogDebug(param.Ctx, "Policy selecting group: %s, priorityRetry: %d", policyGroup, priorityRetry)

			if model.IsUserOwnedProviderGroup(policyGroup) {
				channel, err = model.GetUserOwnedProviderChannelForGroup(common.GetContextKeyInt(param.Ctx, constant.ContextKeyUserId), policyGroup, param.ModelName)
				if err == nil {
					common.SetContextKey(param.Ctx, constant.ContextKeyTokenBillingSource, BillingSourceUserOwnedProvider)
				} else if shouldStopOnProtocolMismatch(param) {
					return nil, policyGroup, err
				}
			} else {
				channel, err = model.GetRandomSatisfiedChannelWithFilter(policyGroup, param.ModelName, priorityRetry, filter)
				if err != nil && !errors.Is(err, model.ErrNoChannelMatchedFilter) {
					return nil, policyGroup, err
				}
				if errors.Is(err, model.ErrNoChannelMatchedFilter) && shouldStopOnProtocolMismatch(param) {
					return nil, policyGroup, unsupportedProtocolError(param)
				}
			}
			if channel == nil {
				logger.LogDebug(param.Ctx, "No available channel in policy group %s for model %s at priorityRetry %d, trying next group", policyGroup, param.ModelName, priorityRetry)
				common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex, i+1)
				common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupRetryIndex, 0)
				param.SetRetry(0)
				continue
			}
			setSelectedGroupContext(param.Ctx, policyGroup, agentPolicyGroupNames[policyGroup])
			selectGroup = policyGroup
			logger.LogDebug(param.Ctx, "Policy selected group: %s", policyGroup)

			if crossGroupRetry && priorityRetry >= common.RetryTimes && i+1 < len(policyGroups) {
				logger.LogDebug(param.Ctx, "Current policy group %s retries exhausted (priorityRetry=%d >= RetryTimes=%d), preparing switch to next group for next retry", policyGroup, priorityRetry, common.RetryTimes)
				common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex, i+1)
				param.SetRetry(0)
				param.ResetRetryNextTry()
			} else {
				common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex, i)
			}
			break
		}
	} else if param.TokenGroup == "auto" {
		autoGroups := GetUserAutoGroup(userGroup)
		agentAutoGroupNames := map[string]string{}
		if agentCtx, ok := common.GetContextKeyType[*types.AgentContext](param.Ctx, constant.ContextKeyAgentContext); ok && agentCtx != nil {
			autoGroups, agentAutoGroupNames = agentAutoGroups(agentCtx, userGroup)
		}
		if len(autoGroups) == 0 {
			return nil, selectGroup, errors.New("auto groups is not enabled")
		}

		// startGroupIndex: the group index to start searching from
		// startGroupIndex: 开始搜索的分组索引
		startGroupIndex := 0
		crossGroupRetry := common.GetContextKeyBool(param.Ctx, constant.ContextKeyTokenCrossGroupRetry)

		if lastGroupIndex, exists := common.GetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex); exists {
			if idx, ok := lastGroupIndex.(int); ok {
				startGroupIndex = idx
			}
		}

		for i := startGroupIndex; i < len(autoGroups); i++ {
			autoGroup := autoGroups[i]
			// Calculate priorityRetry for current group
			// 计算当前分组的 priorityRetry
			priorityRetry := param.GetRetry()
			// If moved to a new group, reset priorityRetry and update startRetryIndex
			// 如果切换到新分组，重置 priorityRetry 并更新 startRetryIndex
			if i > startGroupIndex {
				priorityRetry = 0
			}
			logger.LogDebug(param.Ctx, "Auto selecting group: %s, priorityRetry: %d", autoGroup, priorityRetry)

			channel, err = model.GetRandomSatisfiedChannelWithFilter(autoGroup, param.ModelName, priorityRetry, filter)
			if err != nil && !errors.Is(err, model.ErrNoChannelMatchedFilter) {
				return nil, autoGroup, err
			}
			if errors.Is(err, model.ErrNoChannelMatchedFilter) && shouldStopOnProtocolMismatch(param) {
				return nil, autoGroup, unsupportedProtocolError(param)
			}
			if channel == nil {
				// Current group has no available channel for this model, try next group
				// 当前分组没有该模型的可用渠道，尝试下一个分组
				logger.LogDebug(param.Ctx, "No available channel in group %s for model %s at priorityRetry %d, trying next group", autoGroup, param.ModelName, priorityRetry)
				// 重置状态以尝试下一个分组
				common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex, i+1)
				common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupRetryIndex, 0)
				// Reset retry counter so outer loop can continue for next group
				// 重置重试计数器，以便外层循环可以为下一个分组继续
				param.SetRetry(0)
				continue
			}
			setSelectedGroupContext(param.Ctx, autoGroup, agentAutoGroupNames[autoGroup])
			selectGroup = autoGroup
			logger.LogDebug(param.Ctx, "Auto selected group: %s", autoGroup)

			// Prepare state for next retry
			// 为下一次重试准备状态
			if crossGroupRetry && priorityRetry >= common.RetryTimes && i+1 < len(autoGroups) {
				// Current group has exhausted all retries, prepare to switch to next group
				// This request still uses current group, but next retry will use next group
				// 当前分组已用完所有重试次数，准备切换到下一个分组
				// 本次请求仍使用当前分组，但下次重试将使用下一个分组
				logger.LogDebug(param.Ctx, "Current group %s retries exhausted (priorityRetry=%d >= RetryTimes=%d), preparing switch to next group for next retry", autoGroup, priorityRetry, common.RetryTimes)
				common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex, i+1)
				// Reset retry counter so outer loop can continue for next group
				// 重置重试计数器，以便外层循环可以为下一个分组继续
				param.SetRetry(0)
				param.ResetRetryNextTry()
			} else {
				// Stay in current group, save current state
				// 保持在当前分组，保存当前状态
				common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex, i)
			}
			break
		}
	} else {
		for _, group := range systemChannelGroups {
			channel, err = model.GetRandomSatisfiedChannelWithFilter(group, param.ModelName, param.GetRetry(), filter)
			if err != nil && !errors.Is(err, model.ErrNoChannelMatchedFilter) {
				return nil, param.TokenGroup, err
			}
			if errors.Is(err, model.ErrNoChannelMatchedFilter) {
				if shouldStopOnProtocolMismatch(param) {
					return nil, param.TokenGroup, unsupportedProtocolError(param)
				}
				continue
			}
			if channel != nil {
				setSelectedGroupContext(param.Ctx, group, "")
				selectGroup = group
				break
			}
		}
	}
	return channel, selectGroup, nil
}

func agentAutoGroups(agentCtx *types.AgentContext, userGroup string) ([]string, map[string]string) {
	visibleGroups := agentservice.VisibleGroupsForUser(agentCtx, userGroup)
	agentGroupNames := make([]string, 0, len(visibleGroups))
	for groupName := range visibleGroups {
		agentGroupNames = append(agentGroupNames, groupName)
	}
	sort.Strings(agentGroupNames)
	autoGroups := make([]string, 0, len(agentGroupNames))
	agentGroupBySystemGroup := make(map[string]string, len(visibleGroups))
	seenSystemGroups := make(map[string]struct{}, len(visibleGroups))
	for _, groupName := range agentGroupNames {
		group := visibleGroups[groupName]
		systemGroupName := strings.TrimSpace(group.SystemGroupName)
		if systemGroupName == "" {
			continue
		}
		if _, exists := agentGroupBySystemGroup[systemGroupName]; !exists {
			agentGroupBySystemGroup[systemGroupName] = group.GroupName
		}
		if _, exists := seenSystemGroups[systemGroupName]; exists {
			continue
		}
		seenSystemGroups[systemGroupName] = struct{}{}
		autoGroups = append(autoGroups, systemGroupName)
	}
	sort.Strings(autoGroups)
	return autoGroups, agentGroupBySystemGroup
}

func setSelectedGroupContext(ctx *gin.Context, systemGroup string, agentGroup string) {
	common.SetContextKey(ctx, constant.ContextKeyAutoGroup, systemGroup)
	if strings.TrimSpace(agentGroup) != "" {
		common.SetContextKey(ctx, constant.ContextKeyAgentSelectedGroup, agentGroup)
		return
	}
	if _, exists := common.GetContextKey(ctx, constant.ContextKeyAgentSelectedGroup); exists {
		ctx.Set(string(constant.ContextKeyAgentSelectedGroup), "")
	}
}

func shouldStopOnProtocolMismatch(param *RetryParam) bool {
	if param == nil || param.Ctx == nil {
		return true
	}
	if param.TokenGroup != "auto" && len(ResolveTokenGroupChain(param.Ctx, param.TokenGroup)) == 0 {
		return true
	}
	return !common.GetContextKeyBool(param.Ctx, constant.ContextKeyTokenCrossGroupRetry)
}

func unsupportedImageChatProtocolError(modelName string) error {
	req := conversion.RequirementFromMode(conversion.RequestModeOpenAIChat, modelName)
	return req.UnsupportedError(modelName)
}

func unsupportedProtocolError(param *RetryParam) error {
	req := conversion.RequirementFromHTTPRequest(param.Ctx, param.ModelName)
	if req.Empty() {
		return unsupportedImageChatProtocolError(param.ModelName)
	}
	return req.UnsupportedError(param.ModelName)
}

func BuildProtocolChannelFilter(param *RetryParam) model.ChannelFilter {
	if param == nil || param.Ctx == nil || param.Ctx.Request == nil {
		return nil
	}
	if profileID := param.Ctx.GetString("configurable_native_profile_id"); profileID != "" {
		profileIDs := configurableNativeProfileIDsFromContext(param.Ctx, profileID)
		return func(channel *model.Channel) bool {
			if channel == nil {
				return false
			}
			if channel.Type == constant.ChannelTypeConfigurable {
				settings := channel.GetSetting()
				if settings.Protocol == nil {
					return false
				}
				for _, id := range profileIDs {
					if settings.Protocol.ProfileID == id {
						return true
					}
				}
				return false
			}
			if nativeSeedanceProfileSupportsChannel(profileIDs, param.ModelName, channel.Type) {
				return true
			}
			return false
		}
	}
	req := conversion.RequirementFromHTTPRequest(param.Ctx, param.ModelName)
	if req.Empty() {
		return nil
	}
	channelReq := conversion.CanonicalRequirement(req)
	return func(channel *model.Channel) bool {
		if channel == nil {
			return false
		}
		return channelReq.Supports(channel.GetSetting())
	}
}

func configurableNativeProfileIDsFromContext(c *gin.Context, fallbackProfileID string) []string {
	ids := []string{}
	if c != nil {
		if raw, exists := common.GetContextKey(c, contextKeyConfigurableNativeProfileIDs); exists {
			switch values := raw.(type) {
			case []string:
				ids = append(ids, values...)
			case []any:
				for _, value := range values {
					if id, ok := value.(string); ok {
						ids = append(ids, id)
					}
				}
			}
		}
	}
	if len(ids) == 0 {
		ids = append(ids, fallbackProfileID)
	}
	seen := map[string]bool{}
	deduped := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		deduped = append(deduped, id)
		seen[id] = true
	}
	return deduped
}

func nativeSeedanceProfileSupportsChannel(profileIDs []string, modelName string, channelType int) bool {
	if channelType != constant.ChannelTypeVolcEngine && channelType != constant.ChannelTypeDoubaoVideo {
		return false
	}
	if !strings.HasPrefix(modelName, "doubao-seedance-2-0-") {
		return false
	}
	for _, profileID := range profileIDs {
		if profileID == "doubao-seedance-2" || profileID == "doubao-seedance-2-api-assets" {
			return true
		}
	}
	return false
}

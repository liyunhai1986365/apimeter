package service

import (
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
)

const ginKeyRetryPolicyRecovery = "retry_policy_recovery"

type RetryPolicyRecoveryContext struct {
	Source           string
	RuleName         string
	Action           string
	StartAttempt     int
	RetryGroups      []string
	MaxRetries       int
	TargetChannelIDs []int
	TargetTags       []string
	TargetModel      string
	ExcludeChannelID int
}

func SetRetryPolicyRecovery(c *gin.Context, decision operation_setting.RetryPolicyDecision) {
	if c == nil || !decision.Matched || !decision.ShouldRetry {
		return
	}
	groups := cleanRetryPolicyRecoveryGroups(decision.RetryGroups)
	targetChannelIDs := cleanRetryPolicyRecoveryInts(decision.TargetChannelIDs)
	targetTags := cleanRetryPolicyRecoveryStrings(decision.TargetTags)
	if len(groups) == 0 && len(targetChannelIDs) == 0 && len(targetTags) == 0 && decision.MaxRetries <= 0 && strings.TrimSpace(decision.TargetModel) == "" {
		return
	}
	startAttempt := 0
	if rawAttempt, exists := c.Get("relay_retry_index"); exists {
		if attempt, valid := rawAttempt.(int); valid && attempt > 0 {
			startAttempt = attempt
		}
	}
	if existing, ok := GetRetryPolicyRecovery(c); ok &&
		existing.Source == strings.TrimSpace(decision.Source) &&
		existing.RuleName == strings.TrimSpace(decision.RuleName) &&
		existing.Action == strings.TrimSpace(decision.Action) {
		startAttempt = existing.StartAttempt
	}
	c.Set(ginKeyRetryPolicyRecovery, RetryPolicyRecoveryContext{
		Source:           strings.TrimSpace(decision.Source),
		RuleName:         strings.TrimSpace(decision.RuleName),
		Action:           strings.TrimSpace(decision.Action),
		StartAttempt:     startAttempt,
		RetryGroups:      groups,
		MaxRetries:       decision.MaxRetries,
		TargetChannelIDs: targetChannelIDs,
		TargetTags:       targetTags,
		TargetModel:      strings.TrimSpace(decision.TargetModel),
		ExcludeChannelID: decision.ExcludeChannelID,
	})
}

func ClearRetryPolicyRecovery(c *gin.Context) {
	if c == nil || c.Keys == nil {
		return
	}
	delete(c.Keys, ginKeyRetryPolicyRecovery)
}

func GetRetryPolicyRecovery(c *gin.Context) (RetryPolicyRecoveryContext, bool) {
	if c == nil {
		return RetryPolicyRecoveryContext{}, false
	}
	raw, ok := c.Get(ginKeyRetryPolicyRecovery)
	if !ok {
		return RetryPolicyRecoveryContext{}, false
	}
	recovery, ok := raw.(RetryPolicyRecoveryContext)
	if !ok {
		return RetryPolicyRecoveryContext{}, false
	}
	return recovery, len(recovery.RetryGroups) > 0 || recovery.MaxRetries > 0 || len(recovery.TargetChannelIDs) > 0 || len(recovery.TargetTags) > 0 || strings.TrimSpace(recovery.TargetModel) != "" || recovery.ExcludeChannelID > 0
}

func AppendRetryPolicyRecoveryAdminInfo(c *gin.Context, adminInfo map[string]interface{}) {
	if c == nil || adminInfo == nil {
		return
	}
	recovery, ok := GetRetryPolicyRecovery(c)
	if !ok {
		return
	}
	adminInfo["retry_policy_recovery"] = map[string]interface{}{
		"source":             recovery.Source,
		"rule_name":          recovery.RuleName,
		"action":             recovery.Action,
		"start_attempt":      recovery.StartAttempt,
		"retry_groups":       recovery.RetryGroups,
		"max_retries":        recovery.MaxRetries,
		"target_channel_ids": recovery.TargetChannelIDs,
		"target_tags":        recovery.TargetTags,
		"target_model":       recovery.TargetModel,
		"exclude_channel_id": recovery.ExcludeChannelID,
	}
}

func RetryPolicyRecoveryAllowsAffinityRetry(c *gin.Context) bool {
	recovery, ok := GetRetryPolicyRecovery(c)
	return ok && len(recovery.RetryGroups) > 0
}

func RetryPolicyRecoveryExceeded(c *gin.Context, retryIndex int) bool {
	recovery, ok := GetRetryPolicyRecovery(c)
	if !ok || recovery.MaxRetries <= 0 {
		return false
	}
	return retryIndex-recovery.StartAttempt >= recovery.MaxRetries
}

func RetryPolicyRecoveryGroupForAttempt(c *gin.Context, retryIndex int) (string, bool) {
	recovery, ok := GetRetryPolicyRecovery(c)
	recoveryAttempt := retryIndex - recovery.StartAttempt
	if !ok || len(recovery.RetryGroups) == 0 || recoveryAttempt <= 0 {
		return "", false
	}
	index := recoveryAttempt - 1
	if index >= len(recovery.RetryGroups) {
		index = len(recovery.RetryGroups) - 1
	}
	group := strings.TrimSpace(recovery.RetryGroups[index])
	if group == "" {
		return "", false
	}
	return group, true
}

func RetryPolicyRecoveryGroupsForAttempt(c *gin.Context, retryIndex int, modelName string) []string {
	recovery, ok := GetRetryPolicyRecovery(c)
	if !ok || retryIndex-recovery.StartAttempt <= 0 {
		return nil
	}
	if len(recovery.RetryGroups) > 0 {
		if group, ok := RetryPolicyRecoveryGroupForAttempt(c, retryIndex); ok {
			return []string{group}
		}
		return nil
	}
	return deriveRetryPolicyRecoveryGroups(recovery, modelName)
}

func validateRetryPolicyRecoveryGroup(c *gin.Context, group string) error {
	userGroup := common.GetContextKeyString(c, constant.ContextKeyUserGroup)
	userID := common.GetContextKeyInt(c, constant.ContextKeyUserId)
	return ValidateExplicitTokenGroupForUser(group, userGroup, userID)
}

func cleanRetryPolicyRecoveryGroups(groups []string) []string {
	if len(groups) == 0 {
		return nil
	}
	clean := make([]string, 0, len(groups))
	seen := make(map[string]bool, len(groups))
	for _, group := range groups {
		group = strings.TrimSpace(group)
		if group == "" || seen[group] {
			continue
		}
		seen[group] = true
		clean = append(clean, group)
	}
	return clean
}

func RetryPolicyRecoveryModelForAttempt(c *gin.Context, fallback string) string {
	recovery, ok := GetRetryPolicyRecovery(c)
	if !ok || strings.TrimSpace(recovery.TargetModel) == "" {
		return fallback
	}
	return strings.TrimSpace(recovery.TargetModel)
}

func RetryPolicyRecoveryFilter(c *gin.Context) model.ChannelFilter {
	recovery, ok := GetRetryPolicyRecovery(c)
	if !ok {
		return nil
	}
	targetIDs := intSet(recovery.TargetChannelIDs)
	targetTags := stringSet(recovery.TargetTags)
	hasTargetIDs := len(targetIDs) > 0
	hasTargetTags := len(targetTags) > 0
	excludeChannelID := recovery.ExcludeChannelID
	if !hasTargetIDs && !hasTargetTags && excludeChannelID <= 0 {
		return nil
	}
	return func(channel *model.Channel) bool {
		if channel == nil {
			return false
		}
		if excludeChannelID > 0 && channel.Id == excludeChannelID {
			return false
		}
		if hasTargetIDs && !targetIDs[channel.Id] {
			return false
		}
		if hasTargetTags && !targetTags[strings.ToLower(strings.TrimSpace(channel.GetTag()))] {
			return false
		}
		return true
	}
}

func deriveRetryPolicyRecoveryGroups(recovery RetryPolicyRecoveryContext, modelName string) []string {
	if len(recovery.TargetChannelIDs) == 0 && len(recovery.TargetTags) == 0 {
		return nil
	}
	query := model.DB.Model(&model.Channel{}).Where("status = ?", common.ChannelStatusEnabled)
	if len(recovery.TargetChannelIDs) > 0 && len(recovery.TargetTags) > 0 {
		query = query.Where("id IN ? OR tag IN ?", recovery.TargetChannelIDs, recovery.TargetTags)
	} else if len(recovery.TargetChannelIDs) > 0 {
		query = query.Where("id IN ?", recovery.TargetChannelIDs)
	} else {
		query = query.Where("tag IN ?", recovery.TargetTags)
	}

	var channels []model.Channel
	if err := query.Find(&channels).Error; err != nil {
		common.SysLog("failed to derive retry policy recovery groups: " + err.Error())
		return nil
	}

	seen := map[string]bool{}
	groups := make([]string, 0)
	for i := range channels {
		channel := &channels[i]
		if strings.TrimSpace(modelName) != "" && !channelSupportsModel(channel, modelName) {
			continue
		}
		if recovery.ExcludeChannelID > 0 && channel.Id == recovery.ExcludeChannelID {
			continue
		}
		for _, group := range channel.GetGroups() {
			group = strings.TrimSpace(group)
			if group == "" || seen[group] {
				continue
			}
			seen[group] = true
			groups = append(groups, group)
		}
	}
	sort.Strings(groups)
	return groups
}

func cleanRetryPolicyRecoveryInts(values []int) []int {
	if len(values) == 0 {
		return nil
	}
	clean := make([]int, 0, len(values))
	seen := map[int]bool{}
	for _, value := range values {
		if value <= 0 || seen[value] {
			continue
		}
		seen[value] = true
		clean = append(clean, value)
	}
	return clean
}

func cleanRetryPolicyRecoveryStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	clean := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		key := strings.ToLower(value)
		if value == "" || seen[key] {
			continue
		}
		seen[key] = true
		clean = append(clean, value)
	}
	return clean
}

func intSet(values []int) map[int]bool {
	result := map[int]bool{}
	for _, value := range values {
		result[value] = true
	}
	return result
}

func stringSet(values []string) map[string]bool {
	result := map[string]bool{}
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value != "" {
			result[value] = true
		}
	}
	return result
}

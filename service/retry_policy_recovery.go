package service

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
)

const ginKeyRetryPolicyRecovery = "retry_policy_recovery"

type RetryPolicyRecoveryContext struct {
	Source      string
	RuleName    string
	RetryGroups []string
	MaxRetries  int
}

func SetRetryPolicyRecovery(c *gin.Context, decision operation_setting.RetryPolicyDecision) {
	if c == nil || !decision.Matched || !decision.ShouldRetry {
		return
	}
	groups := cleanRetryPolicyRecoveryGroups(decision.RetryGroups)
	if len(groups) == 0 && decision.MaxRetries <= 0 {
		return
	}
	c.Set(ginKeyRetryPolicyRecovery, RetryPolicyRecoveryContext{
		Source:      strings.TrimSpace(decision.Source),
		RuleName:    strings.TrimSpace(decision.RuleName),
		RetryGroups: groups,
		MaxRetries:  decision.MaxRetries,
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
	return recovery, len(recovery.RetryGroups) > 0 || recovery.MaxRetries > 0
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
		"source":       recovery.Source,
		"rule_name":    recovery.RuleName,
		"retry_groups": recovery.RetryGroups,
		"max_retries":  recovery.MaxRetries,
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
	return retryIndex >= recovery.MaxRetries
}

func RetryPolicyRecoveryGroupForAttempt(c *gin.Context, retryIndex int) (string, bool) {
	recovery, ok := GetRetryPolicyRecovery(c)
	if !ok || len(recovery.RetryGroups) == 0 || retryIndex <= 0 {
		return "", false
	}
	index := retryIndex - 1
	if index >= len(recovery.RetryGroups) {
		index = len(recovery.RetryGroups) - 1
	}
	group := strings.TrimSpace(recovery.RetryGroups[index])
	if group == "" {
		return "", false
	}
	return group, true
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

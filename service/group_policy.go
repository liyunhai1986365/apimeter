package service

import (
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	agentservice "github.com/QuantumNous/new-api/service/agent"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

const (
	TokenGroupPolicyTypeOrdered = "ordered"
	AutoGroupName               = "auto"
)

type TokenGroupPolicy struct {
	Type   string   `json:"type"`
	Groups []string `json:"groups"`
}

func NormalizeTokenGroupPolicy(raw string, userGroup string) (string, string, error) {
	return NormalizeTokenGroupPolicyForUser(raw, userGroup, 0)
}

func NormalizeTokenGroupPolicyForUser(raw string, userGroup string, userID int) (string, string, error) {
	policy, ok, err := parseTokenGroupPolicy(raw)
	if err != nil {
		return "", "", err
	}
	if !ok {
		return strings.TrimSpace(raw), "", nil
	}
	if policy.Type != TokenGroupPolicyTypeOrdered {
		return "", "", errors.New("unsupported group policy type")
	}

	groups, err := normalizePolicyGroups(policy.Groups, userGroup, userID)
	if err != nil {
		return "", "", err
	}
	if len(groups) == 1 && groups[0] == AutoGroupName {
		return AutoGroupName, "", nil
	}

	normalizedPolicy := TokenGroupPolicy{
		Type:   TokenGroupPolicyTypeOrdered,
		Groups: groups,
	}
	data, err := common.Marshal(normalizedPolicy)
	if err != nil {
		return "", "", err
	}
	return groups[0], string(data), nil
}

func ResolveTokenGroupChain(ctx *gin.Context, tokenGroup string) []string {
	raw := common.GetContextKeyString(ctx, constant.ContextKeyTokenGroupPolicy)
	policy, ok, err := parseTokenGroupPolicy(raw)
	if err != nil || !ok || policy.Type != TokenGroupPolicyTypeOrdered {
		return nil
	}

	userGroup := common.GetContextKeyString(ctx, constant.ContextKeyUserGroup)
	if group, ok := agentservice.ResolveGroupFromRequest(ctx, userGroup); ok {
		userGroup = group.SystemGroupName
	}
	chain := make([]string, 0, len(policy.Groups))
	seen := make(map[string]bool)
	for _, item := range policy.Groups {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		var expanded []string
		if item == AutoGroupName {
			expanded = GetUserAutoGroup(userGroup)
			if agentCtx, ok := common.GetContextKeyType[*types.AgentContext](ctx, constant.ContextKeyAgentContext); ok && agentCtx != nil {
				expanded = agentAutoGroups(agentCtx, userGroup)
			}
		} else {
			expanded = []string{item}
		}
		for _, group := range expanded {
			group = strings.TrimSpace(group)
			if group == "" || seen[group] {
				continue
			}
			seen[group] = true
			chain = append(chain, group)
		}
	}
	if len(chain) == 0 && strings.TrimSpace(tokenGroup) != "" && tokenGroup != AutoGroupName {
		return []string{strings.TrimSpace(tokenGroup)}
	}
	return chain
}

func parseTokenGroupPolicy(raw string) (TokenGroupPolicy, bool, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return TokenGroupPolicy{}, false, nil
	}
	var policy TokenGroupPolicy
	if err := common.UnmarshalJsonStr(raw, &policy); err != nil {
		return TokenGroupPolicy{}, false, err
	}
	return policy, true, nil
}

func normalizePolicyGroups(groups []string, userGroup string, userID int) ([]string, error) {
	cleanGroups := make([]string, 0, len(groups))
	seen := make(map[string]bool)
	userUsableGroups := GetUserUsableGroups(userGroup)
	for _, group := range groups {
		group = strings.TrimSpace(group)
		if group == "" {
			continue
		}
		if group == AutoGroupName {
			return []string{AutoGroupName}, nil
		}
		if seen[group] {
			continue
		}
		if model.IsUserOwnedProviderGroup(group) {
			if userID <= 0 {
				return nil, errors.New("无权访问 " + group + " 分组")
			}
			ok, err := model.UserOwnsProviderGroup(userID, group)
			if err != nil {
				return nil, err
			}
			if !ok {
				return nil, errors.New("无权访问 " + group + " 分组")
			}
			seen[group] = true
			cleanGroups = append(cleanGroups, group)
			continue
		}
		if _, ok := userUsableGroups[group]; !ok {
			return nil, errors.New("无权访问 " + group + " 分组")
		}
		if !ratio_setting.ContainsGroupRatio(group) {
			return nil, errors.New("分组 " + group + " 已被弃用")
		}
		seen[group] = true
		cleanGroups = append(cleanGroups, group)
	}
	if len(cleanGroups) == 0 {
		cleanGroups = append(cleanGroups, AutoGroupName)
	}
	return cleanGroups, nil
}

func ValidateExplicitTokenGroup(group string, userGroup string) error {
	return ValidateExplicitTokenGroupForUser(group, userGroup, 0)
}

func ValidateExplicitTokenGroupForUser(group string, userGroup string, userID int) error {
	group = strings.TrimSpace(group)
	if group == "" || group == AutoGroupName {
		return nil
	}
	if model.IsUserOwnedProviderGroup(group) {
		if userID <= 0 {
			return errors.New("无权访问 " + group + " 分组")
		}
		ok, err := model.UserOwnsProviderGroup(userID, group)
		if err != nil {
			return err
		}
		if !ok {
			return errors.New("无权访问 " + group + " 分组")
		}
		return nil
	}
	if _, ok := GetUserUsableGroups(userGroup)[group]; !ok {
		return errors.New("无权访问 " + group + " 分组")
	}
	if !ratio_setting.ContainsGroupRatio(group) {
		return errors.New("分组 " + group + " 已被弃用")
	}
	return nil
}

func DefaultTokenGroupPolicy() string {
	data, err := common.Marshal(TokenGroupPolicy{
		Type:   TokenGroupPolicyTypeOrdered,
		Groups: []string{AutoGroupName},
	})
	if err != nil {
		return ""
	}
	return string(data)
}

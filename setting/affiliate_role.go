package setting

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
)

const DefaultAffiliateRoleConfigsJSON = "[]"

var affiliateRoleIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

type AffiliateRoleConfig struct {
	Id                 string   `json:"id"`
	Name               string   `json:"name"`
	TopUpRewardRatio   *float64 `json:"topup_reward_ratio,omitempty"`
	TopUpRewardLimit   *int     `json:"topup_reward_limit,omitempty"`
	InviterRewardQuota *int     `json:"inviter_reward_quota,omitempty"`
	InviteeRewardQuota *int     `json:"invitee_reward_quota,omitempty"`
}

type AffiliateRewardPolicy struct {
	RoleId             string  `json:"role_id"`
	RoleName           string  `json:"role_name"`
	UsesDefaultRole    bool    `json:"uses_default_role"`
	TopUpRewardRatio   float64 `json:"topup_reward_ratio"`
	TopUpRewardLimit   int     `json:"topup_reward_limit"`
	InviterRewardQuota int     `json:"inviter_reward_quota"`
	InviteeRewardQuota int     `json:"invitee_reward_quota"`
}

var affiliateRoleState = struct {
	sync.RWMutex
	roles []AffiliateRoleConfig
}{roles: []AffiliateRoleConfig{}}

func ParseAffiliateRoleConfigs(jsonStr string) ([]AffiliateRoleConfig, error) {
	if strings.TrimSpace(jsonStr) == "" {
		jsonStr = DefaultAffiliateRoleConfigsJSON
	}

	var roles []AffiliateRoleConfig
	if err := common.UnmarshalJsonStr(jsonStr, &roles); err != nil {
		return nil, fmt.Errorf("invalid affiliate role configuration: %w", err)
	}
	if roles == nil {
		roles = []AffiliateRoleConfig{}
	}

	seen := make(map[string]struct{}, len(roles))
	for i := range roles {
		roles[i].Id = strings.TrimSpace(roles[i].Id)
		roles[i].Name = strings.TrimSpace(roles[i].Name)
		if !affiliateRoleIDPattern.MatchString(roles[i].Id) {
			return nil, fmt.Errorf("affiliate role %d has an invalid id", i+1)
		}
		if roles[i].Name == "" {
			return nil, fmt.Errorf("affiliate role %q requires a name", roles[i].Id)
		}
		if utf8.RuneCountInString(roles[i].Name) > 128 {
			return nil, fmt.Errorf("affiliate role %q name cannot exceed 128 characters", roles[i].Id)
		}
		if _, exists := seen[roles[i].Id]; exists {
			return nil, fmt.Errorf("duplicate affiliate role id %q", roles[i].Id)
		}
		seen[roles[i].Id] = struct{}{}
		if roles[i].TopUpRewardRatio != nil && (*roles[i].TopUpRewardRatio < 0 || *roles[i].TopUpRewardRatio > 100) {
			return nil, fmt.Errorf("affiliate role %q top-up reward ratio must be between 0 and 100", roles[i].Id)
		}
		if roles[i].TopUpRewardLimit != nil && *roles[i].TopUpRewardLimit < 0 {
			return nil, fmt.Errorf("affiliate role %q top-up reward limit cannot be negative", roles[i].Id)
		}
		if roles[i].InviterRewardQuota != nil && *roles[i].InviterRewardQuota < 0 {
			return nil, fmt.Errorf("affiliate role %q inviter reward cannot be negative", roles[i].Id)
		}
		if roles[i].InviteeRewardQuota != nil && *roles[i].InviteeRewardQuota < 0 {
			return nil, fmt.Errorf("affiliate role %q invitee reward cannot be negative", roles[i].Id)
		}
	}

	return roles, nil
}

func ValidateAffiliateRoleConfigsJSON(jsonStr string) error {
	_, err := ParseAffiliateRoleConfigs(jsonStr)
	return err
}

func UpdateAffiliateRoleConfigsByJSONString(jsonStr string) error {
	roles, err := ParseAffiliateRoleConfigs(jsonStr)
	if err != nil {
		return err
	}
	affiliateRoleState.Lock()
	affiliateRoleState.roles = roles
	affiliateRoleState.Unlock()
	return nil
}

func AffiliateRoleConfigs2JsonString() string {
	roles := GetAffiliateRoleConfigs()
	data, err := common.Marshal(roles)
	if err != nil {
		return DefaultAffiliateRoleConfigsJSON
	}
	return string(data)
}

func GetAffiliateRoleConfigs() []AffiliateRoleConfig {
	affiliateRoleState.RLock()
	defer affiliateRoleState.RUnlock()
	roles := make([]AffiliateRoleConfig, len(affiliateRoleState.roles))
	copy(roles, affiliateRoleState.roles)
	return roles
}

func AffiliateRoleExists(roleId string) bool {
	roleId = strings.TrimSpace(roleId)
	if roleId == "" {
		return true
	}
	_, ok := findAffiliateRole(roleId)
	return ok
}

func ResolveAffiliateRewardPolicy(roleId string) AffiliateRewardPolicy {
	policy := AffiliateRewardPolicy{
		UsesDefaultRole:    true,
		TopUpRewardRatio:   common.AffiliateTopUpRewardRatio,
		TopUpRewardLimit:   common.AffiliateTopUpRewardLimit,
		InviterRewardQuota: common.QuotaForInviter,
		InviteeRewardQuota: common.QuotaForInvitee,
	}
	role, ok := findAffiliateRole(strings.TrimSpace(roleId))
	if !ok {
		return policy
	}

	policy.RoleId = role.Id
	policy.RoleName = role.Name
	policy.UsesDefaultRole = false
	if role.TopUpRewardRatio != nil {
		policy.TopUpRewardRatio = *role.TopUpRewardRatio / 100
	}
	if role.TopUpRewardLimit != nil {
		policy.TopUpRewardLimit = *role.TopUpRewardLimit
	}
	if role.InviterRewardQuota != nil {
		policy.InviterRewardQuota = *role.InviterRewardQuota
	}
	if role.InviteeRewardQuota != nil {
		policy.InviteeRewardQuota = *role.InviteeRewardQuota
	}
	return policy
}

func AffiliateRoleConfigsHaveRewards(jsonStr string) (bool, error) {
	roles, err := ParseAffiliateRoleConfigs(jsonStr)
	if err != nil {
		return false, err
	}
	for _, role := range roles {
		if role.TopUpRewardRatio != nil && *role.TopUpRewardRatio > 0 {
			return true, nil
		}
		if role.InviterRewardQuota != nil && *role.InviterRewardQuota > 0 {
			return true, nil
		}
		if role.InviteeRewardQuota != nil && *role.InviteeRewardQuota > 0 {
			return true, nil
		}
	}
	return false, nil
}

func findAffiliateRole(roleId string) (AffiliateRoleConfig, bool) {
	if roleId == "" {
		return AffiliateRoleConfig{}, false
	}
	affiliateRoleState.RLock()
	defer affiliateRoleState.RUnlock()
	for _, role := range affiliateRoleState.roles {
		if role.Id == roleId {
			return role, true
		}
	}
	return AffiliateRoleConfig{}, false
}

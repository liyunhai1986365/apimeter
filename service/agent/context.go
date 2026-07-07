package agent

import (
	"net"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type Context = types.AgentContext
type BillingSnapshot = types.AgentBillingSnapshot

type GroupRatioResult struct {
	GroupRatio        float64
	GroupSpecialRatio float64
	HasSpecialRatio   bool
	BaseGroupRatio    float64
	AgentGroupRatio   float64
	HasAgentRatio     bool
	DisplayGroup      string
	UserGroup         string
	Snapshot          *types.AgentBillingSnapshot
}

func NormalizeHost(host string) string {
	host = strings.TrimSpace(strings.ToLower(host))
	host = strings.TrimSuffix(host, ".")
	if host == "" {
		return ""
	}
	if splitHost, _, err := net.SplitHostPort(host); err == nil {
		host = splitHost
	} else if idx := strings.LastIndex(host, ":"); idx > -1 {
		host = host[:idx]
	}
	host = strings.TrimSuffix(host, ".")
	return host
}

func ResolveByRequest(c *gin.Context) (*Context, error) {
	if c == nil || c.Request == nil {
		return nil, nil
	}
	return ResolveByHost(c.Request.Host)
}

func ResolveByHost(host string) (*Context, error) {
	if model.DB == nil {
		return nil, nil
	}
	domain := NormalizeHost(host)
	if domain == "" {
		return nil, nil
	}
	agentWithDomain, err := model.GetActiveAgentByDomain(domain)
	if err != nil || agentWithDomain == nil {
		return nil, err
	}
	groupRatios, err := EffectiveGroupRatioMap(agentWithDomain.Id)
	if err != nil {
		return nil, err
	}
	groups, err := EffectiveGroupMap(agentWithDomain.Id)
	if err != nil {
		return nil, err
	}
	userGroups, err := UserGroupConfigMap(agentWithDomain.Id)
	if err != nil {
		return nil, err
	}
	return &Context{
		AgentID:       agentWithDomain.Id,
		Domain:        agentWithDomain.Domain,
		OwnerUserID:   agentWithDomain.OwnerUserId,
		DefaultMarkup: agentWithDomain.DefaultMarkup,
		Branding:      agentWithDomain.Branding,
		Groups:        groups,
		UserGroups:    userGroups,
		GroupRatios:   groupRatios,
	}, nil
}

func RequireUserInAgent(ctx *Context, userID int) error {
	if ctx == nil || ctx.AgentID == 0 {
		return nil
	}
	ok, err := model.IsUserInAgent(ctx.AgentID, userID)
	if err != nil {
		return err
	}
	if !ok {
		return model.ErrAgentUserNotFound
	}
	return nil
}

func BindUser(ctx *Context, userID int, source string) error {
	if ctx == nil || ctx.AgentID == 0 {
		return nil
	}
	return model.BindUserToAgent(ctx.AgentID, userID, source)
}

func GetUserGroup(ctx *Context, userID int, fallbackGroup string) (string, error) {
	if ctx == nil || ctx.AgentID == 0 || userID == 0 {
		return fallbackGroup, nil
	}
	var agentUser model.AgentUser
	err := model.DB.Where("agent_id = ? AND user_id = ? AND status = ?", ctx.AgentID, userID, model.AgentUserStatusEnabled).First(&agentUser).Error
	if err != nil {
		return fallbackGroup, err
	}
	group := strings.TrimSpace(agentUser.Group)
	if group == "" {
		return fallbackGroup, nil
	}
	return group, nil
}

func VisibleGroupsForUser(ctx *Context, userGroup string) map[string]types.AgentGroup {
	result := make(map[string]types.AgentGroup)
	if ctx == nil {
		return result
	}
	for _, group := range ctx.Groups {
		if !group.Visible || !group.Available {
			continue
		}
		result[group.GroupName] = group
	}
	if current, ok := ResolveUserGroup(ctx, userGroup); ok {
		for _, groupName := range current.VisibleGroups {
			group, ok := ResolveGroup(ctx, groupName)
			if !ok {
				continue
			}
			if ratio, ok := current.GroupRatios[group.SystemGroupName]; ok {
				if ratio < group.AgentRatio {
					ratio = group.AgentRatio
				}
				group.EffectiveRatio = ratio
			}
			result[group.GroupName] = group
		}
	}
	return result
}

func UserCanSeeGroup(ctx *Context, userGroup string, groupName string) bool {
	if strings.TrimSpace(groupName) == "" {
		return false
	}
	if current, ok := ResolveGroup(ctx, userGroup); ok && current.GroupName == groupName {
		return true
	}
	_, ok := VisibleGroupsForUser(ctx, userGroup)[groupName]
	return ok
}

func ResolveUserGroup(ctx *Context, groupName string) (types.AgentUserGroup, bool) {
	if ctx == nil || strings.TrimSpace(groupName) == "" {
		return types.AgentUserGroup{}, false
	}
	group, ok := ctx.UserGroups[groupName]
	return group, ok
}

func ResolveGroup(ctx *Context, groupName string) (types.AgentGroup, bool) {
	if ctx == nil || strings.TrimSpace(groupName) == "" {
		return types.AgentGroup{}, false
	}
	group, ok := ctx.Groups[groupName]
	if ok && group.Available {
		return group, true
	}
	return types.AgentGroup{}, false
}

func ResolveGroupFromRequest(c *gin.Context, groupName string) (types.AgentGroup, bool) {
	agentCtx, ok := common.GetContextKeyType[*types.AgentContext](c, constant.ContextKeyAgentContext)
	if !ok || agentCtx == nil {
		return types.AgentGroup{}, false
	}
	return ResolveGroup(agentCtx, groupName)
}

func ResolveGroupRatio(ctx *gin.Context, agentCtx *Context, userGroup string, tokenGroup string, usingGroup string) GroupRatioResult {
	result := GroupRatioResult{
		GroupRatio:        1,
		GroupSpecialRatio: -1,
		DisplayGroup:      usingGroup,
		UserGroup:         userGroup,
	}

	if ctx != nil {
		if autoGroup, exists := common.GetContextKey(ctx, constant.ContextKeyAutoGroup); exists {
			if group, ok := autoGroup.(string); ok && strings.TrimSpace(group) != "" {
				usingGroup = group
			}
		}
	}

	result.DisplayGroup = usingGroup
	matchedAgentGroup := types.AgentGroup{}
	hasMatchedAgentGroup := false
	if group, ok := ResolveGroup(agentCtx, tokenGroup); ok {
		result.DisplayGroup = group.GroupName
		matchedAgentGroup = group
		hasMatchedAgentGroup = true
	} else if group, ok := ResolveGroup(agentCtx, usingGroup); ok {
		result.DisplayGroup = group.GroupName
		usingGroup = group.SystemGroupName
		matchedAgentGroup = group
		hasMatchedAgentGroup = true
	}

	if group, ok := ResolveGroup(agentCtx, result.UserGroup); ok {
		result.UserGroup = group.SystemGroupName
	}

	if userGroupRatio, ok := ratio_setting.GetGroupGroupRatio(result.UserGroup, usingGroup); ok {
		result.GroupSpecialRatio = userGroupRatio
		result.GroupRatio = userGroupRatio
		result.HasSpecialRatio = true
	} else {
		result.GroupRatio = ratio_setting.GetGroupRatio(usingGroup)
	}
	result.BaseGroupRatio = result.GroupRatio

	if agentCtx != nil {
		if selectedAgentGroup := getContextKeyString(ctx, constant.ContextKeyAgentSelectedGroup); selectedAgentGroup != "" {
			if group, ok := ResolveGroup(agentCtx, selectedAgentGroup); ok && group.GroupName == usingGroup {
				result.DisplayGroup = group.GroupName
				matchedAgentGroup = group
				hasMatchedAgentGroup = true
			}
		}
		if agentRatio, ok := GroupRatioForUserGroup(agentCtx, result.UserGroup, result.DisplayGroup); ok {
			agentBaseRatio := result.BaseGroupRatio
			if hasMatchedAgentGroup && matchedAgentGroup.AgentRatio > 0 {
				agentBaseRatio = matchedAgentGroup.AgentRatio
			}
			if agentRatio < agentBaseRatio {
				agentRatio = agentBaseRatio
			}
			result.GroupRatio = agentRatio
			result.AgentGroupRatio = agentRatio
			result.HasAgentRatio = true
			if result.HasSpecialRatio {
				result.GroupSpecialRatio = agentRatio
			}
			result.Snapshot = &types.AgentBillingSnapshot{
				AgentID:           agentCtx.AgentID,
				Domain:            agentCtx.Domain,
				Group:             usingGroup,
				BaseGroupRatio:    agentBaseRatio,
				ChargedGroupRatio: agentRatio,
			}
		}
	}

	return result
}

func getContextKeyString(ctx *gin.Context, key constant.ContextKey) string {
	if ctx == nil {
		return ""
	}
	return common.GetContextKeyString(ctx, key)
}

func GroupRatioForUserGroup(agentCtx *Context, userGroup string, displayGroup string) (float64, bool) {
	if agentCtx == nil {
		return 0, false
	}
	systemGroup := displayGroup
	if group, ok := ResolveGroup(agentCtx, displayGroup); ok {
		systemGroup = group.SystemGroupName
	}
	if group, ok := ResolveUserGroup(agentCtx, userGroup); ok && group.GroupRatios != nil {
		if ratio, ok := group.GroupRatios[systemGroup]; ok {
			return ratio, true
		}
	}
	ratio, ok := agentCtx.GroupRatios[systemGroup]
	return ratio, ok
}

func ApplySnapshot(snapshot *BillingSnapshot, baseQuota int) int {
	return baseQuota
}

func BaseQuotaFromCharged(snapshot *BillingSnapshot, chargedQuota int) int {
	if snapshot == nil || snapshot.AgentID == 0 || snapshot.BaseGroupRatio <= 0 || snapshot.ChargedGroupRatio <= 0 {
		return chargedQuota
	}
	return common.QuotaRound(float64(chargedQuota) * snapshot.BaseGroupRatio / snapshot.ChargedGroupRatio)
}

func SettleConsume(snapshot *BillingSnapshot, userID int, logID int, baseQuota int, chargedQuota int) error {
	if snapshot == nil || snapshot.AgentID == 0 {
		return nil
	}
	if baseQuota <= 0 && chargedQuota > 0 {
		baseQuota = BaseQuotaFromCharged(snapshot, chargedQuota)
	}
	profitQuota := chargedQuota - baseQuota
	if profitQuota <= 0 {
		return nil
	}
	return model.DB.Transaction(func(tx *gorm.DB) error {
		var totalProfit int
		if err := tx.Model(&model.AgentLedger{}).
			Select("COALESCE(SUM(profit_quota), 0)").
			Where("agent_id = ?", snapshot.AgentID).
			Scan(&totalProfit).Error; err != nil {
			return err
		}
		ledger := &model.AgentLedger{
			AgentId:      snapshot.AgentID,
			UserId:       userID,
			LogId:        logID,
			Type:         model.AgentLedgerTypeConsumeProfit,
			BaseQuota:    baseQuota,
			ChargedQuota: chargedQuota,
			ProfitQuota:  profitQuota,
			BalanceAfter: totalProfit + profitQuota,
		}
		return tx.Create(ledger).Error
	})
}

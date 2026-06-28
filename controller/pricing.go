package controller

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	perfmetrics "github.com/QuantumNous/new-api/pkg/perf_metrics"
	"github.com/QuantumNous/new-api/service"
	agentservice "github.com/QuantumNous/new-api/service/agent"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

func filterPricingByUsableGroups(pricing []model.Pricing, usableGroup map[string]string) []model.Pricing {
	if len(pricing) == 0 {
		return pricing
	}
	if len(usableGroup) == 0 {
		return []model.Pricing{}
	}

	filtered := make([]model.Pricing, 0, len(pricing))
	for _, item := range pricing {
		if common.StringsContains(item.EnableGroup, "all") {
			filtered = append(filtered, item)
			continue
		}
		for _, group := range item.EnableGroup {
			if _, ok := usableGroup[group]; ok {
				filtered = append(filtered, item)
				break
			}
		}
	}
	return filtered
}

func GetPricing(c *gin.Context) {
	pricing := model.GetPricing()
	userId, exists := c.Get("id")
	usableGroup := map[string]string{}
	groupRatio := map[string]float64{}
	for s, f := range ratio_setting.GetGroupRatioCopy() {
		groupRatio[s] = f
	}
	var group string
	if exists {
		user, err := model.GetUserCache(userId.(int))
		if err == nil {
			group = user.Group
			groupForSystemRatio := group
			for g := range groupRatio {
				ratio, ok := ratio_setting.GetGroupGroupRatio(groupForSystemRatio, g)
				if ok {
					groupRatio[g] = ratio
				}
			}
		}
	}
	groupRatio = applyAgentGroupRatios(c, groupRatio)

	usableGroup = service.GetUserUsableGroups(group)
	if agentCtx, ok := common.GetContextKeyType[*types.AgentContext](c, constant.ContextKeyAgentContext); ok && agentCtx != nil {
		if userID, hasUserID := c.Get("id"); hasUserID {
			if agentUserGroup, err := agentservice.GetUserGroup(agentCtx, userID.(int), group); err == nil {
				group = agentUserGroup
			}
		}
		agentUsableGroup := map[string]string{}
		agentGroupRatio := map[string]float64{}
		agentSystemGroups := make(map[string]struct{})
		visibleGroups := agentservice.VisibleGroupsForUser(agentCtx, group)
		for _, agentGroup := range visibleGroups {
			desc := strings.TrimSpace(agentGroup.Description)
			if desc == "" {
				desc = setting.GetUsableGroupDescription(agentGroup.SystemGroupName)
			}
			agentUsableGroup[agentGroup.GroupName] = desc
			agentGroupRatio[agentGroup.GroupName] = agentGroup.EffectiveRatio
			agentSystemGroups[agentGroup.SystemGroupName] = struct{}{}
		}
		systemUsableGroup := make(map[string]string, len(agentSystemGroups))
		for systemGroup := range agentSystemGroups {
			systemUsableGroup[systemGroup] = setting.GetUsableGroupDescription(systemGroup)
		}
		pricing = filterPricingByUsableGroups(pricing, systemUsableGroup)
		for i := range pricing {
			if common.StringsContains(pricing[i].EnableGroup, "all") {
				keys := make([]string, 0, len(agentUsableGroup))
				for groupName := range agentUsableGroup {
					keys = append(keys, groupName)
				}
				pricing[i].EnableGroup = keys
				continue
			}
			enableGroups := make([]string, 0, len(pricing[i].EnableGroup))
			for _, systemGroup := range pricing[i].EnableGroup {
				if _, ok := agentSystemGroups[systemGroup]; !ok {
					continue
				}
				for _, agentGroup := range visibleGroups {
					if agentGroup.SystemGroupName == systemGroup && agentGroup.Available {
						enableGroups = append(enableGroups, agentGroup.GroupName)
					}
				}
			}
			pricing[i].EnableGroup = enableGroups
		}
		c.JSON(200, gin.H{
			"success":            true,
			"data":               pricing,
			"vendors":            model.GetVendors(),
			"group_ratio":        agentGroupRatio,
			"group_perf":         getPricingGroupPerformance(agentUsableGroup),
			"usable_group":       agentUsableGroup,
			"group_display":      setting.GetGroupDisplayConfig(),
			"supported_endpoint": model.GetSupportedEndpointMap(),
			"auto_groups":        []string{},
			"pricing_version":    "a42d372ccf0b5dd13ecf71203521f9d2",
		})
		return
	}
	pricing = filterPricingByUsableGroups(pricing, usableGroup)
	// check groupRatio contains usableGroup
	for group := range ratio_setting.GetGroupRatioCopy() {
		if _, ok := usableGroup[group]; !ok {
			delete(groupRatio, group)
		}
	}

	c.JSON(200, gin.H{
		"success":            true,
		"data":               pricing,
		"vendors":            model.GetVendors(),
		"group_ratio":        groupRatio,
		"group_perf":         getPricingGroupPerformance(usableGroup),
		"usable_group":       usableGroup,
		"group_display":      setting.GetGroupDisplayConfig(),
		"supported_endpoint": model.GetSupportedEndpointMap(),
		"auto_groups":        service.GetUserAutoGroup(group),
		"pricing_version":    "a42d372ccf0b5dd13ecf71203521f9d2",
	})
}

func getPricingGroupPerformance[T any](usableGroup map[string]T) map[string]perfmetrics.GroupSummary {
	groups := make([]string, 0, len(usableGroup))
	for group := range usableGroup {
		if group != "" && group != "auto" {
			groups = append(groups, group)
		}
	}
	result, err := perfmetrics.QueryGroupSummary(24, groups)
	if err != nil {
		return map[string]perfmetrics.GroupSummary{}
	}
	perfByGroup := make(map[string]perfmetrics.GroupSummary, len(result.Groups))
	for _, item := range result.Groups {
		perfByGroup[item.Group] = item
	}
	return perfByGroup
}

func ResetModelRatio(c *gin.Context) {
	defaultStr := ratio_setting.DefaultModelRatio2JSONString()
	err := model.UpdateOption("ModelRatio", defaultStr)
	if err != nil {
		c.JSON(200, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	err = ratio_setting.UpdateModelRatioByJSONString(defaultStr)
	if err != nil {
		c.JSON(200, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	c.JSON(200, gin.H{
		"success": true,
		"message": "重置模型倍率成功",
	})
}

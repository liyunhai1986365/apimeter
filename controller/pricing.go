package controller

import (
	"net/http"
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

type pricingGroupRatioResolver func(modelName, group string) float64

func buildPricingGroupModelRatios(pricing []model.Pricing, groupRatios map[string]float64, resolve pricingGroupRatioResolver) map[string]map[string]float64 {
	overrides := make(map[string]map[string]float64)
	if resolve == nil {
		return overrides
	}
	for _, item := range pricing {
		groups := item.EnableGroup
		if common.StringsContains(groups, "all") {
			groups = make([]string, 0, len(groupRatios))
			for group := range groupRatios {
				groups = append(groups, group)
			}
		}
		for _, group := range groups {
			fallback, ok := groupRatios[group]
			if !ok {
				continue
			}
			effective := resolve(item.ModelName, group)
			if effective == fallback {
				continue
			}
			if overrides[group] == nil {
				overrides[group] = make(map[string]float64)
			}
			overrides[group][item.ModelName] = effective
		}
	}
	return overrides
}

func GetPricing(c *gin.Context) {
	getPricingForUserGroup(c, "", false)
}

func GetPricingQuotation(c *gin.Context) {
	userGroup := strings.TrimSpace(c.Query("user_group"))
	if userGroup == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "user_group is required",
		})
		return
	}
	if !isConfiguredUserGroup(userGroup) {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "invalid user_group",
		})
		return
	}

	getPricingForUserGroup(c, userGroup, true)
}

func getPricingForUserGroup(c *gin.Context, requestedUserGroup string, hasRequestedUserGroup bool) {
	pricing := model.GetPricing()
	userId, exists := c.Get("id")
	usableGroup := map[string]string{}
	groupRatio := map[string]float64{}
	for s, f := range ratio_setting.GetGroupRatioCopy() {
		groupRatio[s] = f
	}
	var group string
	if hasRequestedUserGroup {
		group = requestedUserGroup
	} else if exists {
		user, err := model.GetUserCache(userId.(int))
		if err == nil {
			group = user.Group
		}
	}
	for g := range groupRatio {
		ratio, ok := ratio_setting.GetGroupGroupRatio(group, g)
		if ok {
			groupRatio[g] = ratio
		}
	}
	groupRatio = applyAgentGroupRatios(c, groupRatio)

	usableGroup = service.GetUserUsableGroups(group)
	if agentCtx, ok := common.GetContextKeyType[*types.AgentContext](c, constant.ContextKeyAgentContext); ok && agentCtx != nil {
		if userID, hasUserID := c.Get("id"); hasUserID && !hasRequestedUserGroup {
			if agentUserGroup, err := agentservice.GetUserGroup(agentCtx, userID.(int), group); err == nil {
				group = agentUserGroup
			}
		}
		agentUsableGroup := map[string]string{}
		agentGroupRatio := map[string]float64{}
		agentSystemGroups := make(map[string]struct{})
		visibleGroups := agentservice.VisibleGroupsForUser(agentCtx, group)
		groupDisplay := setting.GetGroupDisplayConfig()
		displayBySystemGroup := make(map[string]setting.GroupDisplayGroup, len(groupDisplay.Groups))
		for _, displayGroup := range groupDisplay.Groups {
			displayBySystemGroup[displayGroup.Group] = displayGroup
		}
		for _, agentGroup := range visibleGroups {
			desc := strings.TrimSpace(agentGroup.Description)
			if desc == "" {
				desc = setting.GetUsableGroupDescription(agentGroup.SystemGroupName)
			}
			agentUsableGroup[agentGroup.GroupName] = desc
			agentGroupRatio[agentGroup.GroupName] = agentGroup.EffectiveRatio
			agentSystemGroups[agentGroup.SystemGroupName] = struct{}{}
			if displayGroup, ok := displayBySystemGroup[agentGroup.SystemGroupName]; ok && agentGroup.GroupName != agentGroup.SystemGroupName {
				displayGroup.Group = agentGroup.GroupName
				groupDisplay.Groups = append(groupDisplay.Groups, displayGroup)
			}
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
		visibleGroupByName := make(map[string]types.AgentGroup, len(visibleGroups))
		for _, visibleGroup := range visibleGroups {
			visibleGroupByName[visibleGroup.GroupName] = visibleGroup
		}
		groupModelRatio := buildPricingGroupModelRatios(pricing, agentGroupRatio, func(modelName, displayGroup string) float64 {
			visibleGroup, ok := visibleGroupByName[displayGroup]
			if !ok {
				return agentGroupRatio[displayGroup]
			}
			return agentservice.ResolveGroupRatio(c, agentCtx, group, displayGroup, visibleGroup.SystemGroupName, modelName).GroupRatio
		})
		c.JSON(200, gin.H{
			"success":            true,
			"data":               pricing,
			"vendors":            model.GetVendors(),
			"user_group":         group,
			"group_ratio":        agentGroupRatio,
			"group_model_ratio":  groupModelRatio,
			"group_perf":         getPricingGroupPerformance(agentUsableGroup),
			"usable_group":       agentUsableGroup,
			"group_display":      groupDisplay,
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
	groupModelRatio := buildPricingGroupModelRatios(pricing, groupRatio, func(modelName, usingGroup string) float64 {
		return service.GetUserGroupRatioForModel(group, usingGroup, modelName).Ratio
	})

	c.JSON(200, gin.H{
		"success":            true,
		"data":               pricing,
		"vendors":            model.GetVendors(),
		"user_group":         group,
		"group_ratio":        groupRatio,
		"group_model_ratio":  groupModelRatio,
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

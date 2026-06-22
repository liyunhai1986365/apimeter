package controller

import (
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	perfmetrics "github.com/QuantumNous/new-api/pkg/perf_metrics"
	"github.com/QuantumNous/new-api/service"
	agentservice "github.com/QuantumNous/new-api/service/agent"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
)

func GetPerfMetricsSummary(c *gin.Context) {
	hours := 24
	if rawHours := c.Query("hours"); rawHours != "" {
		if parsed, err := strconv.Atoi(rawHours); err == nil {
			hours = parsed
		}
	}

	activeGroups := append(lo.Keys(ratio_setting.GetGroupRatioCopy()), "auto")
	result, err := perfmetrics.QuerySummaryAll(hours, activeGroups)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result,
	})
}

func GetPerfMetricsGroups(c *gin.Context) {
	hours := 24
	if rawHours := c.Query("hours"); rawHours != "" {
		if parsed, err := strconv.Atoi(rawHours); err == nil {
			hours = parsed
		}
	}

	result, err := perfmetrics.QueryGroupSummary(hours, getVisiblePerfMetricGroups(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result,
	})
}

func GetPerfMetrics(c *gin.Context) {
	modelName := c.Query("model")
	if modelName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "model is required",
		})
		return
	}

	hours := 24
	if rawHours := c.Query("hours"); rawHours != "" {
		if parsed, err := strconv.Atoi(rawHours); err == nil {
			hours = parsed
		}
	}

	result, err := perfmetrics.Query(perfmetrics.QueryParams{
		Model: modelName,
		Group: c.Query("group"),
		Hours: hours,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	result.Groups = filterActiveGroups(result.Groups)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result,
	})
}

func getVisiblePerfMetricGroups(c *gin.Context) []string {
	groupSet := map[string]struct{}{}
	userGroup := ""
	userID, hasUserID := c.Get("id")
	if hasUserID {
		if user, err := model.GetUserCache(userID.(int)); err == nil {
			userGroup = user.Group
		}
	}

	if agentCtx, ok := common.GetContextKeyType[*types.AgentContext](c, constant.ContextKeyAgentContext); ok && agentCtx != nil {
		agentUserGroup := userGroup
		if hasUserID {
			if resolved, err := agentservice.GetUserGroup(agentCtx, userID.(int), userGroup); err == nil {
				agentUserGroup = resolved
			}
		}
		for _, group := range agentservice.VisibleGroupsForUser(agentCtx, agentUserGroup) {
			if group.Available {
				groupSet[group.GroupName] = struct{}{}
			}
		}
	} else if hasUserID {
		for group := range service.GetUserUsableGroups(userGroup) {
			groupSet[group] = struct{}{}
		}
	} else {
		for group := range ratio_setting.GetGroupRatioCopy() {
			groupSet[group] = struct{}{}
		}
	}

	if hasUserID {
		if channels, err := model.ListUserOwnedProviderChannels(userID.(int)); err == nil {
			for _, channel := range channels {
				if channel.Group != "" {
					groupSet[channel.Group] = struct{}{}
				}
			}
		}
	}

	groups := make([]string, 0, len(groupSet))
	for group := range groupSet {
		groups = append(groups, group)
	}
	return groups
}

func filterActiveGroups(groups []perfmetrics.GroupResult) []perfmetrics.GroupResult {
	activeRatios := ratio_setting.GetGroupRatioCopy()
	return lo.Filter(groups, func(g perfmetrics.GroupResult, _ int) bool {
		_, ok := activeRatios[g.Group]
		return ok || g.Group == "auto"
	})
}

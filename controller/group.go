package controller

import (
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	agentservice "github.com/QuantumNous/new-api/service/agent"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

func GetGroups(c *gin.Context) {
	groupNames, configured := setting.GetUserGroupNamesFromDisplayConfig()
	if !configured {
		groupNames = make([]string, 0)
		for groupName := range ratio_setting.GetGroupRatioCopy() {
			groupNames = append(groupNames, groupName)
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    groupNames,
	})
}

func GetUserGroups(c *gin.Context) {
	usableGroups := make(map[string]map[string]interface{})
	userGroup := ""
	userId := c.GetInt("id")
	userGroup, _ = model.GetUserGroup(userId, false)
	if agentCtx, ok := common.GetContextKeyType[*types.AgentContext](c, constant.ContextKeyAgentContext); ok && agentCtx != nil {
		if agentUserGroup, err := agentservice.GetUserGroup(agentCtx, userId, userGroup); err == nil {
			userGroup = agentUserGroup
		}
		for _, group := range agentservice.VisibleGroupsForUser(agentCtx, userGroup) {
			usableGroups[group.GroupName] = map[string]interface{}{
				"ratio": group.EffectiveRatio,
				"desc":  setting.GetUsableGroupDescription(group.SystemGroupName),
			}
		}
		if channels, err := model.ListUserOwnedProviderChannels(userId); err == nil {
			for _, channel := range channels {
				usableGroups[channel.Group] = map[string]interface{}{
					"ratio": "自有",
					"desc":  channel.Name,
					"scope": model.ChannelScopeUserOwned,
				}
			}
		}
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "",
			"data":    usableGroups,
		})
		return
	}
	userUsableGroups := service.GetUserUsableGroups(userGroup)
	for groupName, _ := range ratio_setting.GetGroupRatioCopy() {
		// UserUsableGroups contains the groups that the user can use
		if desc, ok := userUsableGroups[groupName]; ok {
			ratio := service.GetUserGroupRatio(userGroup, groupName)
			usableGroups[groupName] = map[string]interface{}{
				"ratio": applyAgentGroupRatio(c, groupName, ratio),
				"desc":  desc,
			}
		}
	}
	if _, ok := userUsableGroups["auto"]; ok {
		usableGroups["auto"] = map[string]interface{}{
			"ratio": "自动",
			"desc":  setting.GetUsableGroupDescription("auto"),
		}
	}
	if channels, err := model.ListUserOwnedProviderChannels(userId); err == nil {
		for _, channel := range channels {
			usableGroups[channel.Group] = map[string]interface{}{
				"ratio": "自有",
				"desc":  channel.Name,
				"scope": model.ChannelScopeUserOwned,
			}
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    usableGroups,
	})
}

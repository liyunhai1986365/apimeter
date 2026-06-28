package controller

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	agentservice "github.com/QuantumNous/new-api/service/agent"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

func applyAgentGroupRatios(c *gin.Context, groupRatios map[string]float64) map[string]float64 {
	result := make(map[string]float64, len(groupRatios))
	for groupName, ratio := range groupRatios {
		result[groupName] = ratio
	}
	agentCtx, ok := common.GetContextKeyType[*types.AgentContext](c, constant.ContextKeyAgentContext)
	if !ok || agentCtx == nil {
		return result
	}
	for groupName, ratio := range result {
		agentRatio, exists := agentCtx.GroupRatios[groupName]
		if !exists {
			continue
		}
		if agentRatio < ratio {
			agentRatio = ratio
		}
		result[groupName] = agentRatio
	}
	return result
}

func applyAgentGroupRatio(c *gin.Context, groupName string, ratio float64) float64 {
	agentCtx, ok := common.GetContextKeyType[*types.AgentContext](c, constant.ContextKeyAgentContext)
	if !ok || agentCtx == nil {
		return ratio
	}
	agentRatio, exists := agentCtx.GroupRatios[groupName]
	if !exists {
		return ratio
	}
	if agentRatio < ratio {
		return ratio
	}
	return agentRatio
}

func applyAgentUserGroupRatio(agentCtx *types.AgentContext, userGroup string, group types.AgentGroup) float64 {
	ratio := group.EffectiveRatio
	agentRatio, exists := agentservice.GroupRatioForUserGroup(agentCtx, userGroup, group.GroupName)
	if !exists {
		return ratio
	}
	baseRatio := group.AgentRatio
	if baseRatio <= 0 {
		baseRatio = group.SystemRatio
	}
	if agentRatio < baseRatio {
		return baseRatio
	}
	return agentRatio
}

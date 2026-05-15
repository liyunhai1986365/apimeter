package controller

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
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

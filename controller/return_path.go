package controller

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

func paymentReturnPath(suffix string) string {
	base := strings.TrimRight(system_setting.ServerAddress, "/")
	return base + common.ThemeAwarePath(suffix)
}

func paymentReturnPathForRequest(c *gin.Context, suffix string) string {
	if agentCtx, ok := common.GetContextKeyType[*types.AgentContext](c, constant.ContextKeyAgentContext); ok && agentCtx != nil {
		base := strings.TrimRight(buildAgentServerAddress(c, agentCtx.Domain), "/")
		return base + common.ThemeAwarePath(suffix)
	}
	return paymentReturnPath(suffix)
}

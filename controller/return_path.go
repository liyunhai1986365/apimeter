package controller

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

func paymentReturnPath(suffix string) string {
	base := strings.TrimRight(system_setting.ServerAddress, "/")
	return base + common.ThemeAwarePath(suffix)
}

func paymentReturnPathForRequest(c *gin.Context, suffix string) string {
	base := strings.TrimRight(siteServerAddressForRequest(c), "/")
	return base + common.ThemeAwarePath(suffix)
}

func siteServerAddressForRequest(c *gin.Context) string {
	if siteURL := emailSiteURLForRequest(c); siteURL != "" {
		return siteURL
	}
	return system_setting.ServerAddress
}

func emailSiteURLForRequest(c *gin.Context) string {
	if agentCtx, ok := common.GetContextKeyType[*types.AgentContext](c, constant.ContextKeyAgentContext); ok && agentCtx != nil {
		return buildAgentServerAddress(c, agentCtx.Domain)
	}
	return ""
}

func emailSiteURLForUserAndRequest(c *gin.Context, userId int) (string, error) {
	if agentCtx, ok := common.GetContextKeyType[*types.AgentContext](c, constant.ContextKeyAgentContext); ok && agentCtx != nil {
		belongsToAgent, err := model.IsUserInAgent(agentCtx.AgentID, userId)
		if err != nil {
			return "", err
		}
		if belongsToAgent {
			return buildAgentServerAddress(c, agentCtx.Domain), nil
		}
	}
	return service.EmailSiteURLForUser(userId)
}

func agentEmailSiteURLForRequest(c *gin.Context, agentId int) (string, error) {
	if agentCtx, ok := common.GetContextKeyType[*types.AgentContext](c, constant.ContextKeyAgentContext); ok && agentCtx != nil && agentCtx.AgentID == agentId {
		return buildAgentServerAddress(c, agentCtx.Domain), nil
	}
	domain, err := model.GetPrimaryActiveAgentDomain(agentId)
	if err != nil || domain == nil {
		return "", err
	}
	return service.AgentEmailSiteURL(domain.Domain), nil
}

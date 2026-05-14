package middleware

import (
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	agentservice "github.com/QuantumNous/new-api/service/agent"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

func AgentResolver() gin.HandlerFunc {
	return func(c *gin.Context) {
		agentCtx, err := agentservice.ResolveByRequest(c)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": "failed to resolve agent domain",
			})
			c.Abort()
			return
		}
		if agentCtx != nil {
			common.SetContextKey(c, constant.ContextKeyAgentContext, agentCtx)
		}
		c.Next()
	}
}

func RequireTokenAgentAccess(c *gin.Context, userID int) bool {
	agentCtx, ok := common.GetContextKeyType[*types.AgentContext](c, constant.ContextKeyAgentContext)
	if !ok || agentCtx == nil {
		return true
	}
	if err := agentservice.RequireUserInAgent(agentCtx, userID); err != nil {
		abortWithOpenAiMessage(c, http.StatusForbidden, "该 API Key 无权访问当前代理域名", types.ErrorCodeAccessDenied)
		return false
	}
	return true
}

func AgentUserAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		agentCtx, ok := common.GetContextKeyType[*types.AgentContext](c, constant.ContextKeyAgentContext)
		if !ok || agentCtx == nil {
			c.Next()
			return
		}
		if err := agentservice.RequireUserInAgent(agentCtx, c.GetInt("id")); err != nil {
			c.JSON(http.StatusForbidden, gin.H{
				"success": false,
				"message": "当前用户无权访问该代理站点",
			})
			c.Abort()
			return
		}
		c.Next()
	}
}

func AgentOwnerAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		agentCtx, ok := common.GetContextKeyType[*types.AgentContext](c, constant.ContextKeyAgentContext)
		userID := c.GetInt("id")
		role := c.GetInt("role")
		if !ok || agentCtx == nil {
			hasAgentConsole, err := model.UserHasAgentConsole(userID)
			if err == nil && hasAgentConsole {
				c.Next()
				return
			}
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{
					"success": false,
					"message": "failed to check agent permission",
				})
				c.Abort()
				return
			}
		}
		if (!ok || agentCtx == nil) && role >= common.RoleAdminUser {
			c.Next()
			return
		}
		if !ok || agentCtx == nil {
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"message": "当前域名未配置代理站点",
			})
			c.Abort()
			return
		}
		if agentCtx.OwnerUserID != userID && role < common.RoleAdminUser {
			c.JSON(http.StatusForbidden, gin.H{
				"success": false,
				"message": "当前用户无权管理该代理站点",
			})
			c.Abort()
			return
		}
		c.Next()
	}
}

package middleware

import (
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

func WorkspaceAccountScope() gin.HandlerFunc {
	return func(c *gin.Context) {
		scope, err := service.ResolveWorkspaceAccessScope(c.GetInt("id"))
		if err != nil {
			common.ApiError(c, err)
			c.Abort()
			return
		}
		common.SetContextKey(c, constant.ContextKeyWorkspaceActorUserId, scope.ActorUserId)
		common.SetContextKey(c, constant.ContextKeyWorkspaceOwnerUserId, scope.OwnerUserId)
		common.SetContextKey(c, constant.ContextKeyWorkspaceSubaccount, scope.IsSubaccount)
		common.SetContextKey(c, constant.ContextKeyWorkspaceIds, scope.WorkspaceIds)
		c.Set("workspace_access_scope", scope)
		c.Next()
	}
}

func RequireWorkspaceMainAccount() gin.HandlerFunc {
	return func(c *gin.Context) {
		scope, err := service.ResolveWorkspaceAccessScope(c.GetInt("id"))
		if err != nil {
			common.ApiError(c, err)
			c.Abort()
			return
		}
		if scope.IsSubaccount {
			c.JSON(http.StatusForbidden, gin.H{"success": false, "message": "workspace accounts cannot access this feature"})
			c.Abort()
			return
		}
		c.Set("workspace_access_scope", scope)
		c.Next()
	}
}

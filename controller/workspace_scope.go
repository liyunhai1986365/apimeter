package controller

import (
	"errors"

	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

// ErrWorkspaceScopeMissing is returned when a handler needs the workspace scope but the
// route did not run middleware.WorkspaceAccountScope(). Failing closed matters: a
// silent fallback would hand a workspace account the owner's unrestricted scope.
var ErrWorkspaceScopeMissing = errors.New("workspace access scope is unavailable for this request")

func workspaceAccessScope(c *gin.Context) (*service.WorkspaceAccessScope, error) {
	if value, ok := c.Get("workspace_access_scope"); ok {
		if scope, valid := value.(*service.WorkspaceAccessScope); valid && scope != nil {
			return scope, nil
		}
	}
	return nil, ErrWorkspaceScopeMissing
}

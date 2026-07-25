package controller

import (
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

type workspaceRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type workspaceQuotaResetRequest struct {
	Enabled bool   `json:"enabled"`
	Period  string `json:"period"`
	Amount  int    `json:"amount"`
}

func ListWorkspaces(c *gin.Context) {
	scope, err := workspaceAccessScope(c)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	workspaces, err := model.ListUserWorkspaces(scope.OwnerUserId, scope.WorkspaceFilter())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if scope.IsSubaccount {
		// Membership is administrative metadata; child accounts only need their own
		// accessible workspace list and must not discover sibling accounts.
		for i := range workspaces {
			workspaces[i].AccessUsers = []model.WorkspaceAccessUser{}
		}
	}
	common.ApiSuccess(c, workspaces)
}

func CreateWorkspace(c *gin.Context) {
	scope, err := workspaceAccessScope(c)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if scope.IsSubaccount {
		common.ApiErrorMsg(c, "workspace accounts cannot create workspaces")
		return
	}
	var req workspaceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	workspace, err := model.CreateUserWorkspace(scope.OwnerUserId, req.Name, req.Description)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, workspace)
}

func UpdateWorkspace(c *gin.Context) {
	scope, scopeErr := workspaceAccessScope(c)
	if scopeErr != nil {
		common.ApiError(c, scopeErr)
		return
	}
	if scope.IsSubaccount {
		common.ApiErrorMsg(c, "workspace accounts cannot update workspaces")
		return
	}
	workspaceId, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	var req workspaceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	if !scope.CanAccessWorkspace(workspaceId) {
		common.ApiErrorMsg(c, "workspace not found or unavailable")
		return
	}
	workspace, err := model.UpdateUserWorkspace(scope.OwnerUserId, workspaceId, req.Name, req.Description)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, workspace)
}

func DeleteWorkspace(c *gin.Context) {
	scope, err := workspaceAccessScope(c)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if scope.IsSubaccount {
		common.ApiErrorMsg(c, "workspace accounts cannot delete workspaces")
		return
	}
	workspaceId, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.DeleteUserWorkspace(scope.OwnerUserId, workspaceId); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

func GetWorkspaceUsageStats(c *gin.Context) {
	scope, scopeErr := workspaceAccessScope(c)
	if scopeErr != nil {
		common.ApiError(c, scopeErr)
		return
	}
	workspaceId, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if !scope.CanAccessWorkspace(workspaceId) {
		common.ApiErrorMsg(c, "workspace not found or unavailable")
		return
	}
	stats, err := model.GetUserWorkspaceUsageStats(scope.OwnerUserId, workspaceId, time.Time{})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, stats)
}

func GetWorkspaceQuotaResetConfig(c *gin.Context) {
	scope, scopeErr := workspaceAccessScope(c)
	if scopeErr != nil {
		common.ApiError(c, scopeErr)
		return
	}
	workspaceId, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if !scope.CanAccessWorkspace(workspaceId) {
		common.ApiErrorMsg(c, "workspace not found or unavailable")
		return
	}
	config, err := model.GetUserWorkspaceQuotaResetConfig(scope.OwnerUserId, workspaceId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, config)
}

func UpdateWorkspaceQuotaResetConfig(c *gin.Context) {
	scope, err := workspaceAccessScope(c)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if scope.IsSubaccount {
		common.ApiErrorMsg(c, "workspace accounts cannot change workspace quota rules")
		return
	}
	workspaceId, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	var req workspaceQuotaResetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	config, err := model.UpdateUserWorkspaceQuotaResetConfig(
		scope.OwnerUserId,
		workspaceId,
		model.WorkspaceQuotaResetConfigRequest{
			Enabled: req.Enabled,
			Period:  req.Period,
			Amount:  req.Amount,
		},
		time.Time{},
	)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, config)
}

func ResetWorkspaceQuotaNow(c *gin.Context) {
	scope, err := workspaceAccessScope(c)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if scope.IsSubaccount {
		common.ApiErrorMsg(c, "workspace accounts cannot reset workspace quotas")
		return
	}
	workspaceId, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	config, err := model.ResetUserWorkspaceQuotaNow(scope.OwnerUserId, workspaceId, time.Time{})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, config)
}

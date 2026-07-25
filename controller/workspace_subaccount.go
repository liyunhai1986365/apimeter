package controller

import (
	"fmt"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

type workspaceSubaccountRequest struct {
	Username     string `json:"username"`
	DisplayName  string `json:"display_name"`
	Email        string `json:"email"`
	Password     string `json:"password"`
	WorkspaceIds []int  `json:"workspace_ids"`
}

type workspaceSubaccountStatusRequest struct {
	Status int `json:"status"`
}

type workspaceSubaccountPasswordRequest struct {
	Password string `json:"password"`
}

func ListWorkspaceSubaccounts(c *gin.Context) {
	items, err := model.ListWorkspaceSubaccounts(c.GetInt("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, items)
}

func GetWorkspaceSubaccount(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	user, err := model.GetWorkspaceSubaccount(c.GetInt("id"), id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	workspaceIds, err := model.ListAccessibleWorkspaceIds(c.GetInt("id"), id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if workspaceIds == nil {
		// A nil filter means "unrestricted" downstream; this account simply has none.
		workspaceIds = []int{}
	}
	workspaces, err := model.ListUserWorkspaces(c.GetInt("id"), workspaceIds)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"user": user, "workspaces": workspaces})
}

func CreateWorkspaceSubaccount(c *gin.Context) {
	var req workspaceSubaccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	user := &model.User{
		Username: req.Username, DisplayName: req.DisplayName,
		Email: req.Email, Password: req.Password,
	}
	if err := model.CreateWorkspaceSubaccount(c.GetInt("id"), user, req.WorkspaceIds); err != nil {
		common.ApiError(c, err)
		return
	}
	model.RecordLog(c.GetInt("id"), model.LogTypeManage, fmt.Sprintf("created workspace account %s (%d)", user.Username, user.Id))
	user.Password = ""
	common.ApiSuccess(c, user)
}

func UpdateWorkspaceSubaccount(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	var req workspaceSubaccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	user, err := model.UpdateWorkspaceSubaccount(c.GetInt("id"), id, req.DisplayName, req.Email)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	model.RecordLog(c.GetInt("id"), model.LogTypeManage, fmt.Sprintf("updated workspace account %s (%d)", user.Username, user.Id))
	common.ApiSuccess(c, user)
}

func UpdateWorkspaceSubaccountStatus(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	var req workspaceSubaccountStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.SetWorkspaceSubaccountStatus(c.GetInt("id"), id, req.Status); err != nil {
		common.ApiError(c, err)
		return
	}
	model.RecordLog(c.GetInt("id"), model.LogTypeManage, fmt.Sprintf("changed workspace account %d status to %d", id, req.Status))
	common.ApiSuccess(c, nil)
}

func ResetWorkspaceSubaccountPassword(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	var req workspaceSubaccountPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.ResetWorkspaceSubaccountPassword(c.GetInt("id"), id, req.Password); err != nil {
		common.ApiError(c, err)
		return
	}
	model.RecordLog(c.GetInt("id"), model.LogTypeManage, fmt.Sprintf("reset workspace account %d password", id))
	common.ApiSuccess(c, nil)
}

func DeleteWorkspaceSubaccount(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.DeleteWorkspaceSubaccount(c.GetInt("id"), id); err != nil {
		common.ApiError(c, err)
		return
	}
	model.RecordLog(c.GetInt("id"), model.LogTypeManage, fmt.Sprintf("deleted workspace account %d", id))
	common.ApiSuccess(c, nil)
}

type workspaceAccessRequest struct {
	// AccessUserIds is the full member list after the call, not a delta.
	AccessUserIds []int `json:"access_user_ids"`
}

// SetWorkspaceAccess replaces the workspace's member list. An empty list revokes all access.
func SetWorkspaceAccess(c *gin.Context) {
	workspaceId, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	var req workspaceAccessRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	workspace, err := model.SetWorkspaceAccess(c.GetInt("id"), workspaceId, req.AccessUserIds)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	model.RecordLog(c.GetInt("id"), model.LogTypeManage,
		fmt.Sprintf("set workspace %d members to %v", workspaceId, req.AccessUserIds))
	common.ApiSuccess(c, workspace)
}

// RevokeWorkspaceAccess clears every member of the workspace.
func RevokeWorkspaceAccess(c *gin.Context) {
	workspaceId, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	workspace, err := model.SetWorkspaceAccess(c.GetInt("id"), workspaceId, nil)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	model.RecordLog(c.GetInt("id"), model.LogTypeManage, fmt.Sprintf("revoked workspace %d access", workspaceId))
	common.ApiSuccess(c, workspace)
}

// SetWorkspaceSubaccountWorkspaces replaces the full workspace set one child account can reach.
func SetWorkspaceSubaccountWorkspaces(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	var req workspaceSubaccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.SetSubaccountWorkspaces(c.GetInt("id"), id, req.WorkspaceIds); err != nil {
		common.ApiError(c, err)
		return
	}
	model.RecordLog(c.GetInt("id"), model.LogTypeManage,
		fmt.Sprintf("set workspace account %d workspaces to %v", id, req.WorkspaceIds))
	common.ApiSuccess(c, nil)
}

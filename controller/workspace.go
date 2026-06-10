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

func ListWorkspaces(c *gin.Context) {
	workspaces, err := model.ListUserWorkspaces(c.GetInt("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, workspaces)
}

func CreateWorkspace(c *gin.Context) {
	var req workspaceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	workspace, err := model.CreateUserWorkspace(c.GetInt("id"), req.Name, req.Description)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, workspace)
}

func UpdateWorkspace(c *gin.Context) {
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
	workspace, err := model.UpdateUserWorkspace(c.GetInt("id"), workspaceId, req.Name, req.Description)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, workspace)
}

func DeleteWorkspace(c *gin.Context) {
	workspaceId, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.DeleteUserWorkspace(c.GetInt("id"), workspaceId); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

func GetWorkspaceUsageStats(c *gin.Context) {
	workspaceId, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	stats, err := model.GetUserWorkspaceUsageStats(c.GetInt("id"), workspaceId, time.Time{})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, stats)
}

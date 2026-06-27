package controller

import (
	"context"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

type routingStrategyOverrideRequest struct {
	Strategy       string   `json:"strategy"`
	UserGroup      string   `json:"user_group"`
	Groups         []string `json:"groups"`
	ManualOverride bool     `json:"manual_override"`
}

func ListRoutingStrategySnapshots(c *gin.Context) {
	snapshots, err := model.ListRoutingStrategySnapshots()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"snapshots":  snapshots,
			"strategies": model.RoutingStrategies(),
		},
	})
}

func RefreshRoutingStrategySnapshots(c *gin.Context) {
	result, err := service.RefreshRoutingStrategySnapshots(context.Background())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    result,
	})
}

func SaveRoutingStrategySnapshotOverride(c *gin.Context) {
	var req routingStrategyOverrideRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "无效的参数"})
		return
	}
	req.Strategy = strings.TrimSpace(req.Strategy)
	req.UserGroup = strings.TrimSpace(req.UserGroup)
	if !model.ValidRoutingStrategy(req.Strategy) || req.UserGroup == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "无效的策略或用户分组"})
		return
	}
	groups := make([]string, 0, len(req.Groups))
	seen := map[string]bool{}
	for _, group := range req.Groups {
		group = strings.TrimSpace(group)
		if group == "" || seen[group] {
			continue
		}
		seen[group] = true
		groups = append(groups, group)
	}
	groupsJSON, err := common.Marshal(groups)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	snapshot := &model.RoutingStrategySnapshot{
		Strategy:       req.Strategy,
		UserGroup:      req.UserGroup,
		Groups:         string(groupsJSON),
		Scores:         "{}",
		Config:         "{}",
		ManualOverride: req.ManualOverride,
	}
	if err := model.UpsertRoutingStrategySnapshot(snapshot); err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    snapshot,
	})
}

package controller

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/console_setting"
	"github.com/gin-gonic/gin"
)

type agentAnnouncementRequest struct {
	Title     string `json:"title"`
	Content   string `json:"content"`
	Type      string `json:"type"`
	Extra     string `json:"extra"`
	PublishAt *int64 `json:"publish_at"`
	Enabled   *bool  `json:"enabled"`
}

func normalizeAgentAnnouncementRequest(req *agentAnnouncementRequest) {
	req.Title = strings.TrimSpace(req.Title)
	req.Content = strings.TrimSpace(req.Content)
	req.Type = strings.TrimSpace(req.Type)
	req.Extra = strings.TrimSpace(req.Extra)
	if req.Type == "" {
		req.Type = "general"
	}
}

func validateAgentAnnouncementRequest(req *agentAnnouncementRequest) error {
	normalizeAgentAnnouncementRequest(req)
	if req.PublishAt != nil && *req.PublishAt <= 0 {
		return fmt.Errorf("invalid announcement publish time")
	}
	return console_setting.ValidateAnnouncementFields(req.Title, req.Content, req.Type, req.Extra)
}

func agentAnnouncementID(c *gin.Context) (int, bool) {
	id, err := strconv.Atoi(c.Param("announcement_id"))
	if err != nil || id <= 0 {
		common.ApiErrorMsg(c, "invalid announcement id")
		return 0, false
	}
	return id, true
}

func AgentListAnnouncements(c *gin.Context) {
	agentID, ok := currentAgentID(c)
	if !ok {
		return
	}
	pageInfo := common.GetPageQuery(c)
	announcements, total, err := model.ListAgentAnnouncements(agentID, pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(announcements)
	common.ApiSuccess(c, pageInfo)
}

func AgentCreateAnnouncement(c *gin.Context) {
	agentID, ok := currentAgentID(c)
	if !ok {
		return
	}
	var req agentAnnouncementRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiError(c, err)
		return
	}
	if err := validateAgentAnnouncementRequest(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	publishAt := time.Now().Unix()
	if req.PublishAt != nil {
		publishAt = *req.PublishAt
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	operatorID := c.GetInt("id")
	announcement := &model.AgentAnnouncement{
		AgentId:   agentID,
		Title:     req.Title,
		Content:   req.Content,
		Type:      req.Type,
		Extra:     req.Extra,
		PublishAt: publishAt,
		Enabled:   enabled,
		CreatedBy: operatorID,
		UpdatedBy: operatorID,
	}
	if err := model.CreateAgentAnnouncement(announcement); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, announcement)
}

func AgentUpdateAnnouncement(c *gin.Context) {
	agentID, ok := currentAgentID(c)
	if !ok {
		return
	}
	announcementID, ok := agentAnnouncementID(c)
	if !ok {
		return
	}
	announcement, err := model.GetAgentAnnouncement(agentID, announcementID)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	var req agentAnnouncementRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiError(c, err)
		return
	}
	if err := validateAgentAnnouncementRequest(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	announcement.Title = req.Title
	announcement.Content = req.Content
	announcement.Type = req.Type
	announcement.Extra = req.Extra
	if req.PublishAt != nil {
		announcement.PublishAt = *req.PublishAt
	}
	if req.Enabled != nil {
		announcement.Enabled = *req.Enabled
	}
	announcement.UpdatedBy = c.GetInt("id")
	if err := model.UpdateAgentAnnouncement(announcement); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, announcement)
}

func AgentDeleteAnnouncement(c *gin.Context) {
	agentID, ok := currentAgentID(c)
	if !ok {
		return
	}
	announcementID, ok := agentAnnouncementID(c)
	if !ok {
		return
	}
	if err := model.DeleteAgentAnnouncement(agentID, announcementID); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

func AgentSendAnnouncementEmail(c *gin.Context) {
	agentID, ok := currentAgentID(c)
	if !ok {
		return
	}
	announcementID, ok := agentAnnouncementID(c)
	if !ok {
		return
	}
	announcement, err := model.GetAgentAnnouncement(agentID, announcementID)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	agent, err := model.GetAgentById(agentID)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	siteURL, err := agentEmailSiteURLForRequest(c, agentID)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	summary, err := service.BroadcastAgentAnnouncementEmail(service.BroadcastAgentAnnouncementEmailRequest{
		AgentID:   agentID,
		AgentName: agent.Name,
		Title:     announcement.Title,
		Content:   announcement.Content,
		Type:      announcement.Type,
		SiteURL:   siteURL,
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	sentAt := time.Now().Unix()
	if err := model.RecordAgentAnnouncementEmailResult(agentID, announcementID, sentAt, summary.Total, summary.Sent, summary.Failed); err != nil {
		common.SysLog(fmt.Sprintf("failed to record agent %d announcement %d email result: %s", agentID, announcementID, err.Error()))
	}
	common.SysLog(fmt.Sprintf("agent %d announcement %d email broadcast by user %d: total=%d sent=%d failed=%d", agentID, announcementID, c.GetInt("id"), summary.Total, summary.Sent, summary.Failed))
	common.ApiSuccess(c, summary)
}

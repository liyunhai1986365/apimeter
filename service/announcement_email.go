package service

import (
	"fmt"
	"html"
	"regexp"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/console_setting"
)

type AnnouncementEmailSender func(subject string, receiver string, content string) error

var AnnouncementEmailSenderFunc AnnouncementEmailSender = common.SendEmail

type BroadcastAnnouncementEmailRequest struct {
	Title        string
	Content      string
	Type         string
	Audience     string
	TargetGroups []string
	Send         AnnouncementEmailSender
}

type BroadcastAgentAnnouncementEmailRequest struct {
	AgentID   int
	AgentName string
	Title     string
	Content   string
	Type      string
	Send      AnnouncementEmailSender
}

type BroadcastAnnouncementEmailSummary struct {
	Total  int      `json:"total"`
	Sent   int      `json:"sent"`
	Failed int      `json:"failed"`
	Errors []string `json:"errors,omitempty"`
}

func announcementEmailContent(content string, announcementType string) string {
	announcementType = strings.TrimSpace(announcementType)
	if announcementType == "" {
		announcementType = "general"
	}
	return fmt.Sprintf("<!-- announcement-type:%s -->\n%s", announcementType, renderAnnouncementMarkdown(content))
}

func agentAnnouncementEmailContent(content string, announcementType string, agentID int, agentName string) string {
	return fmt.Sprintf(
		"<!-- announcement-source:agent:%d -->\n<p><strong>%s</strong></p>\n%s",
		agentID,
		html.EscapeString(strings.TrimSpace(agentName)),
		announcementEmailContent(content, announcementType),
	)
}

var (
	markdownLinkPattern = regexp.MustCompile(`\[([^\]]+)\]\((https?://[^)\s]+)\)`)
	markdownBoldPattern = regexp.MustCompile(`\*\*([^*]+)\*\*`)
	markdownCodePattern = regexp.MustCompile("`([^`]+)`")
)

func renderInlineMarkdown(line string) string {
	escaped := html.EscapeString(line)
	escaped = markdownLinkPattern.ReplaceAllString(escaped, `<a href="$2">$1</a>`)
	escaped = markdownBoldPattern.ReplaceAllString(escaped, `<strong>$1</strong>`)
	escaped = markdownCodePattern.ReplaceAllString(escaped, `<code>$1</code>`)
	return escaped
}

func flushList(builder *strings.Builder, listItems []string) []string {
	if len(listItems) == 0 {
		return listItems
	}
	builder.WriteString("<ul>")
	for _, item := range listItems {
		builder.WriteString("<li>")
		builder.WriteString(renderInlineMarkdown(item))
		builder.WriteString("</li>")
	}
	builder.WriteString("</ul>")
	return nil
}

func renderAnnouncementMarkdown(content string) string {
	lines := strings.Split(strings.TrimSpace(content), "\n")
	var builder strings.Builder
	var paragraph []string
	var listItems []string

	flushParagraph := func() {
		if len(paragraph) == 0 {
			return
		}
		builder.WriteString("<p>")
		for index, line := range paragraph {
			if index > 0 {
				builder.WriteString("<br>")
			}
			builder.WriteString(renderInlineMarkdown(line))
		}
		builder.WriteString("</p>")
		paragraph = nil
	}

	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" {
			flushParagraph()
			listItems = flushList(&builder, listItems)
			continue
		}
		if strings.HasPrefix(line, "### ") {
			flushParagraph()
			listItems = flushList(&builder, listItems)
			builder.WriteString("<h3>")
			builder.WriteString(renderInlineMarkdown(strings.TrimSpace(strings.TrimPrefix(line, "### "))))
			builder.WriteString("</h3>")
			continue
		}
		if strings.HasPrefix(line, "## ") {
			flushParagraph()
			listItems = flushList(&builder, listItems)
			builder.WriteString("<h2>")
			builder.WriteString(renderInlineMarkdown(strings.TrimSpace(strings.TrimPrefix(line, "## "))))
			builder.WriteString("</h2>")
			continue
		}
		if strings.HasPrefix(line, "# ") {
			flushParagraph()
			listItems = flushList(&builder, listItems)
			builder.WriteString("<h1>")
			builder.WriteString(renderInlineMarkdown(strings.TrimSpace(strings.TrimPrefix(line, "# "))))
			builder.WriteString("</h1>")
			continue
		}
		if strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ") {
			flushParagraph()
			listItems = append(listItems, strings.TrimSpace(line[2:]))
			continue
		}
		listItems = flushList(&builder, listItems)
		paragraph = append(paragraph, line)
	}

	flushParagraph()
	flushList(&builder, listItems)

	if builder.Len() == 0 {
		return html.EscapeString(strings.TrimSpace(content))
	}
	return builder.String()
}

func BroadcastAnnouncementEmail(req BroadcastAnnouncementEmailRequest) (BroadcastAnnouncementEmailSummary, error) {
	summary := BroadcastAnnouncementEmailSummary{}
	sender := req.Send
	if sender == nil {
		sender = AnnouncementEmailSenderFunc
	}

	req.Title = strings.TrimSpace(req.Title)
	req.Content = strings.TrimSpace(req.Content)
	if req.Title == "" || req.Content == "" {
		return summary, fmt.Errorf("announcement email title and content are required")
	}
	req.Type = strings.TrimSpace(req.Type)
	if req.Type != "" && !console_setting.IsValidAnnouncementType(req.Type) {
		return summary, fmt.Errorf("invalid announcement type")
	}
	req.Audience = strings.TrimSpace(req.Audience)
	if req.Audience == "" {
		req.Audience = console_setting.AnnouncementAudienceAll
	}
	if req.Audience != console_setting.AnnouncementAudienceAll && req.Audience != console_setting.AnnouncementAudienceMainSite {
		return summary, fmt.Errorf("invalid announcement audience")
	}
	targetGroups, err := console_setting.NormalizeAnnouncementTargetGroups(req.TargetGroups)
	if err != nil {
		return summary, err
	}

	var users []model.User
	userQuery := model.DB.
		Select("id", "email").
		Where("status = ?", common.UserStatusEnabled).
		Where("email <> ?", "")
	if len(targetGroups) > 0 {
		userQuery = userQuery.Where(map[string]interface{}{"group": targetGroups})
	}
	if err := userQuery.
		Order("id asc").
		Find(&users).Error; err != nil {
		return summary, err
	}

	blockedAgentEmails := make(map[string]struct{})
	if req.Audience == console_setting.AnnouncementAudienceMainSite {
		var agentUsers []model.User
		if err := model.DB.Model(&model.User{}).
			Select("users.id", "users.email").
			Joins("JOIN agent_users ON agent_users.user_id = users.id").
			Where("users.status = ?", common.UserStatusEnabled).
			Where("users.email <> ?", "").
			Find(&agentUsers).Error; err != nil {
			return summary, err
		}
		for _, user := range agentUsers {
			email := strings.ToLower(strings.TrimSpace(user.Email))
			if email != "" {
				blockedAgentEmails[email] = struct{}{}
			}
		}
	}

	seen := make(map[string]struct{}, len(users))
	receivers := make([]string, 0, len(users))
	for _, user := range users {
		email := strings.TrimSpace(user.Email)
		if email == "" {
			continue
		}
		key := strings.ToLower(email)
		if _, blocked := blockedAgentEmails[key]; blocked {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		receivers = append(receivers, email)
	}
	sort.Strings(receivers)

	summary.Total = len(receivers)
	content := announcementEmailContent(req.Content, req.Type)
	for _, receiver := range receivers {
		if err := sender(req.Title, receiver, content); err != nil {
			summary.Failed++
			summary.Errors = append(summary.Errors, fmt.Sprintf("%s: %s", receiver, err.Error()))
			common.SysLog(fmt.Sprintf("failed to send announcement email to %s: %s", common.MaskEmail(receiver), err.Error()))
			continue
		}
		summary.Sent++
	}
	return summary, nil
}

func BroadcastAgentAnnouncementEmail(req BroadcastAgentAnnouncementEmailRequest) (BroadcastAnnouncementEmailSummary, error) {
	summary := BroadcastAnnouncementEmailSummary{}
	if req.AgentID <= 0 {
		return summary, fmt.Errorf("invalid agent id")
	}
	req.Title = strings.TrimSpace(req.Title)
	req.Content = strings.TrimSpace(req.Content)
	req.Type = strings.TrimSpace(req.Type)
	req.AgentName = strings.TrimSpace(req.AgentName)
	if err := console_setting.ValidateAnnouncementFields(req.Title, req.Content, req.Type, ""); err != nil {
		return summary, err
	}
	if req.AgentName == "" {
		return summary, fmt.Errorf("agent name is required")
	}

	var users []model.User
	if err := model.DB.Model(&model.User{}).
		Select("users.id", "users.email").
		Joins("JOIN agent_users ON agent_users.user_id = users.id").
		Where("agent_users.agent_id = ? AND agent_users.status = ?", req.AgentID, model.AgentUserStatusEnabled).
		Where("users.status = ?", common.UserStatusEnabled).
		Where("users.email <> ?", "").
		Order("users.id asc").
		Find(&users).Error; err != nil {
		return summary, err
	}

	seen := make(map[string]struct{}, len(users))
	receivers := make([]string, 0, len(users))
	for _, user := range users {
		receiver := strings.TrimSpace(user.Email)
		if receiver == "" {
			continue
		}
		key := strings.ToLower(receiver)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		receivers = append(receivers, receiver)
	}
	sort.Strings(receivers)

	sender := req.Send
	if sender == nil {
		sender = AnnouncementEmailSenderFunc
	}
	summary.Total = len(receivers)
	content := agentAnnouncementEmailContent(req.Content, req.Type, req.AgentID, req.AgentName)
	for _, receiver := range receivers {
		if err := sender(req.Title, receiver, content); err != nil {
			summary.Failed++
			summary.Errors = append(summary.Errors, fmt.Sprintf("%s: %s", receiver, err.Error()))
			common.SysLog(fmt.Sprintf("failed to send agent %d announcement email to %s: %s", req.AgentID, common.MaskEmail(receiver), err.Error()))
			continue
		}
		summary.Sent++
	}
	return summary, nil
}

package service

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	agentservice "github.com/QuantumNous/new-api/service/agent"
	"github.com/QuantumNous/new-api/setting/system_setting"
)

func AgentEmailSiteURL(domain string) string {
	domain = agentservice.NormalizeHost(domain)
	if domain == "" {
		return ""
	}
	return "https://" + domain
}

func EmailSiteURLsForUsers(userIds []int) (map[int]string, error) {
	domains, err := model.ListPrimaryActiveAgentDomainsForUsers(userIds)
	if err != nil {
		return nil, err
	}
	result := make(map[int]string, len(domains))
	for userId, domain := range domains {
		if siteURL := AgentEmailSiteURL(domain); siteURL != "" {
			result[userId] = siteURL
		}
	}
	return result, nil
}

func EmailSiteURLForUser(userId int) (string, error) {
	if userId <= 0 {
		return "", nil
	}
	siteURLs, err := EmailSiteURLsForUsers([]int{userId})
	if err != nil {
		return "", err
	}
	return siteURLs[userId], nil
}

func RewriteEmailContentForSite(content string, siteURL string) string {
	if strings.TrimSpace(siteURL) == "" {
		return content
	}
	return common.RewriteEmailSiteLinks(content, system_setting.ServerAddress, siteURL)
}

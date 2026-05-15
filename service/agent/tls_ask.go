package agent

import (
	"os"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

const (
	TLSAskSecretOptionKey = "AgentTLSAskSecret"
	TLSAskSecretEnv       = "AGENT_TLS_ASK_SECRET"
)

func GetTLSAskSecret() string {
	common.OptionMapRWMutex.RLock()
	secret := ""
	if common.OptionMap != nil {
		secret = common.OptionMap[TLSAskSecretOptionKey]
	}
	common.OptionMapRWMutex.RUnlock()
	if strings.TrimSpace(secret) == "" {
		secret = os.Getenv(TLSAskSecretEnv)
	}
	return strings.TrimSpace(secret)
}

func AuthorizeTLSAskSecret(headerSecret string, querySecret string) bool {
	expectedSecret := GetTLSAskSecret()
	if expectedSecret == "" {
		return true
	}
	return strings.TrimSpace(headerSecret) == expectedSecret || strings.TrimSpace(querySecret) == expectedSecret
}

func CanIssueTLSCertificateForDomain(rawDomain string) (bool, error) {
	if model.DB == nil {
		return false, nil
	}
	domain := NormalizeHost(rawDomain)
	if domain == "" {
		return false, ErrInvalidAgentDomain
	}

	var count int64
	err := model.DB.Model(&model.AgentDomain{}).
		Joins("JOIN agents ON agents.id = agent_domains.agent_id").
		Where(
			"agent_domains.domain = ? AND agent_domains.status = ? AND agent_domains.verified_at > ? AND agents.status = ?",
			domain,
			model.AgentDomainStatusActive,
			0,
			model.AgentStatusEnabled,
		).
		Count(&count).Error
	if err != nil {
		return false, err
	}
	if count > 0 {
		return true, nil
	}
	return canIssueTLSCertificateForCNAMETarget(domain)
}

func canIssueTLSCertificateForCNAMETarget(domain string) (bool, error) {
	baseDomain := GetCNAMEBaseDomain()
	if baseDomain == "" || domain == baseDomain || !strings.HasSuffix(domain, "."+baseDomain) {
		return false, nil
	}
	verifyToken := strings.TrimSuffix(domain, "."+baseDomain)
	if verifyToken == "" || strings.Contains(verifyToken, ".") {
		return false, nil
	}

	var count int64
	err := model.DB.Model(&model.AgentDomain{}).
		Joins("JOIN agents ON agents.id = agent_domains.agent_id").
		Where(
			"agent_domains.verify_token = ? AND agent_domains.status = ? AND agent_domains.verified_at > ? AND agents.status = ?",
			verifyToken,
			model.AgentDomainStatusActive,
			0,
			model.AgentStatusEnabled,
		).
		Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

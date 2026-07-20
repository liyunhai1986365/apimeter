package agent

import (
	"errors"
	"os"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

const (
	CNAMEBaseDomainOptionKey = "AgentCNAMEBaseDomain"
	CNAMEBaseDomainEnv       = "AGENT_CNAME_BASE_DOMAIN"
)

var (
	ErrAgentCNAMEBaseDomainNotConfigured = errors.New("agent cname base domain is not configured")
)

func GetCNAMEBaseDomain() string {
	common.OptionMapRWMutex.RLock()
	baseDomain := common.OptionMap[CNAMEBaseDomainOptionKey]
	common.OptionMapRWMutex.RUnlock()
	if strings.TrimSpace(baseDomain) == "" {
		baseDomain = os.Getenv(CNAMEBaseDomainEnv)
	}
	return NormalizeHost(baseDomain)
}

func CNAMEVerifyTarget(verifyToken string, baseDomain string) (string, error) {
	baseDomain = NormalizeHost(baseDomain)
	if baseDomain == "" {
		return "", ErrAgentCNAMEBaseDomainNotConfigured
	}
	token := strings.Trim(strings.ToLower(strings.TrimSpace(verifyToken)), ".")
	if token == "" {
		return "", ErrInvalidAgentDomain
	}
	return token + "." + baseDomain, nil
}

func FillDomainCNAMETarget(domain *model.AgentDomain) {
	if domain == nil {
		return
	}
	target, err := CNAMEVerifyTarget(domain.VerifyToken, GetCNAMEBaseDomain())
	if err != nil {
		domain.CNAMETarget = ""
		return
	}
	domain.CNAMETarget = target
}

func FillDomainCNAMETargets(domains []*model.AgentDomain) {
	for _, domain := range domains {
		FillDomainCNAMETarget(domain)
	}
}

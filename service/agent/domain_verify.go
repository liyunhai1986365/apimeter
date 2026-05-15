package agent

import (
	"context"
	"errors"
	"net"
	"os"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"gorm.io/gorm"
)

const (
	CNAMEBaseDomainOptionKey = "AgentCNAMEBaseDomain"
	CNAMEBaseDomainEnv       = "AGENT_CNAME_BASE_DOMAIN"
)

var (
	ErrAgentCNAMEBaseDomainNotConfigured = errors.New("agent cname base domain is not configured")
	ErrAgentDomainCNAMEUnverified        = errors.New("agent domain cname is not verified")
)

type CNAMEChainResolver func(ctx context.Context, host string) ([]string, error)

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

func LookupCNAMEChain(ctx context.Context, host string) ([]string, error) {
	host = NormalizeHost(host)
	if host == "" {
		return nil, ErrInvalidAgentDomain
	}
	cname, err := net.DefaultResolver.LookupCNAME(ctx, host)
	if err != nil {
		return nil, err
	}
	return []string{cname}, nil
}

func VerifyDomainCNAME(ctx context.Context, agentID int, domainID int) (*model.AgentDomain, error) {
	return VerifyDomainCNAMEWithResolver(ctx, agentID, domainID, GetCNAMEBaseDomain(), LookupCNAMEChain)
}

func VerifyDomainCNAMEWithResolver(ctx context.Context, agentID int, domainID int, baseDomain string, resolver CNAMEChainResolver) (*model.AgentDomain, error) {
	if resolver == nil {
		resolver = LookupCNAMEChain
	}

	var domain model.AgentDomain
	if err := model.DB.Where("id = ? AND agent_id = ?", domainID, agentID).First(&domain).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, model.ErrAgentNotFound
		}
		return nil, err
	}

	expectedTarget, err := CNAMEVerifyTarget(domain.VerifyToken, baseDomain)
	if err != nil {
		return nil, err
	}

	cnames, err := resolver(ctx, domain.Domain)
	if err != nil {
		return nil, err
	}

	for _, cname := range cnames {
		if NormalizeHost(cname) == expectedTarget {
			if err := model.DB.Model(&model.AgentDomain{}).
				Where("id = ? AND agent_id = ?", domainID, agentID).
				Updates(map[string]interface{}{
					"status":      model.AgentDomainStatusActive,
					"verified_at": common.GetTimestamp(),
				}).Error; err != nil {
				return nil, err
			}
			if err := model.DB.Where("id = ? AND agent_id = ?", domainID, agentID).First(&domain).Error; err != nil {
				return nil, err
			}
			FillDomainCNAMETarget(&domain)
			return &domain, nil
		}
	}

	return nil, ErrAgentDomainCNAMEUnverified
}

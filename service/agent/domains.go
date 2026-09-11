package agent

import (
	"errors"
	"net"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"gorm.io/gorm"
)

const MaxAgentDomainsPerBatch = 50

var ErrInvalidAgentDomainBatch = errors.New("provide between 1 and 50 agent domains")

// CreateDomains adds aliases atomically. Existing domains are never replaced;
// every alias resolves to the same agent and therefore the same user membership.
func CreateDomains(agentID int, rawDomains []string) ([]*model.AgentDomain, error) {
	if len(rawDomains) == 0 || len(rawDomains) > MaxAgentDomainsPerBatch {
		return nil, ErrInvalidAgentDomainBatch
	}
	if _, err := model.GetAgentById(agentID); err != nil {
		return nil, err
	}
	domains := make([]*model.AgentDomain, 0, len(rawDomains))
	names := make([]string, 0, len(rawDomains))
	seen := make(map[string]bool, len(rawDomains))
	for _, raw := range rawDomains {
		name, err := normalizeNewAgentDomain(raw)
		if err != nil {
			return nil, err
		}
		if seen[name] {
			continue
		}
		seen[name] = true
		token, err := common.GenerateRandomCharsKey(32)
		if err != nil {
			return nil, err
		}
		names = append(names, name)
		domains = append(domains, &model.AgentDomain{
			AgentId: agentID, Domain: name, Status: model.AgentDomainStatusActive,
			VerifyToken: strings.ToLower(token), ForceHttps: true,
		})
	}
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&model.AgentDomain{}).Where("domain IN ?", names).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return ErrAgentDomainAlreadyExists
		}
		return tx.Create(&domains).Error
	})
	if err != nil {
		if isAgentDomainDuplicateError(err) {
			return nil, ErrAgentDomainAlreadyExists
		}
		return nil, err
	}
	for _, domain := range domains {
		InvalidateDomainResolution(domain.Domain)
		FillDomainCNAMETarget(domain)
	}
	return domains, nil
}

func normalizeNewAgentDomain(raw string) (string, error) {
	host := strings.TrimSpace(raw)
	if strings.Contains(host, ":") {
		name, port, err := net.SplitHostPort(host)
		portNumber, portErr := strconv.Atoi(port)
		if err != nil || portErr != nil || portNumber < 1 || portNumber > 65535 {
			return "", ErrInvalidAgentDomain
		}
		host = name
	}
	host = NormalizeHost(host)
	if len(host) > 253 || !strings.Contains(host, ".") || net.ParseIP(host) != nil {
		return "", ErrInvalidAgentDomain
	}
	for _, label := range strings.Split(host, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return "", ErrInvalidAgentDomain
		}
		for _, char := range label {
			if !(char >= 'a' && char <= 'z' || char >= '0' && char <= '9' || char == '-') {
				return "", ErrInvalidAgentDomain
			}
		}
	}
	return host, nil
}

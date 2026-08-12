package agent

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
)

func TestCanIssueTLSCertificateForDomainRequiresActiveDomainAndEnabledAgent(t *testing.T) {
	setupAgentTestDB(t)

	require.NoError(t, model.DB.Create(&model.Agent{
		Id:            1,
		OwnerUserId:   10,
		Name:          "Agent One",
		Slug:          "agent-one",
		Status:        model.AgentStatusEnabled,
		PriceMode:     model.AgentPriceModeMultiplier,
		DefaultMarkup: 1,
	}).Error)
	require.NoError(t, model.DB.Create(&model.AgentDomain{
		AgentId: 1,
		Domain:  "api.customer.com",
		Status:  model.AgentDomainStatusActive,
	}).Error)

	allowed, err := CanIssueTLSCertificateForDomain("API.Customer.com:443")

	require.NoError(t, err)
	require.True(t, allowed)
}

func TestCanIssueTLSCertificateForDomainAllowsActiveCNAMETarget(t *testing.T) {
	setupAgentTestDB(t)
	common.OptionMapRWMutex.Lock()
	if common.OptionMap == nil {
		common.OptionMap = make(map[string]string)
	}
	oldBaseDomain := common.OptionMap[CNAMEBaseDomainOptionKey]
	common.OptionMap[CNAMEBaseDomainOptionKey] = "agent-cname.apimeter.ai"
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		common.OptionMap[CNAMEBaseDomainOptionKey] = oldBaseDomain
		common.OptionMapRWMutex.Unlock()
	})

	require.NoError(t, model.DB.Create(&model.Agent{
		Id:            1,
		OwnerUserId:   10,
		Name:          "Agent One",
		Slug:          "agent-one",
		Status:        model.AgentStatusEnabled,
		PriceMode:     model.AgentPriceModeMultiplier,
		DefaultMarkup: 1,
	}).Error)
	require.NoError(t, model.DB.Create(&model.AgentDomain{
		AgentId:     1,
		Domain:      "lovtokens.com",
		Status:      model.AgentDomainStatusActive,
		VerifyToken: "verify-token",
	}).Error)

	allowed, err := CanIssueTLSCertificateForDomain("verify-token.agent-cname.apimeter.ai")

	require.NoError(t, err)
	require.True(t, allowed)
}

func TestCanIssueTLSCertificateForDomainRejectsInactiveOrDisabledAgentDomains(t *testing.T) {
	setupAgentTestDB(t)

	require.NoError(t, model.DB.Create(&model.Agent{
		Id:            1,
		OwnerUserId:   10,
		Name:          "Agent One",
		Slug:          "agent-one",
		Status:        model.AgentStatusEnabled,
		PriceMode:     model.AgentPriceModeMultiplier,
		DefaultMarkup: 1,
	}).Error)
	require.NoError(t, model.DB.Create(&model.Agent{
		Id:            2,
		OwnerUserId:   20,
		Name:          "Agent Two",
		Slug:          "agent-two",
		Status:        model.AgentStatusDisabled,
		PriceMode:     model.AgentPriceModeMultiplier,
		DefaultMarkup: 1,
	}).Error)
	require.NoError(t, model.DB.Create([]*model.AgentDomain{
		{
			AgentId: 1,
			Domain:  "pending.customer.com",
			Status:  model.AgentDomainStatusPending,
		},
		{
			AgentId: 2,
			Domain:  "disabled-agent.customer.com",
			Status:  model.AgentDomainStatusActive,
		},
	}).Error)

	for _, domain := range []string{
		"pending.customer.com",
		"disabled-agent.customer.com",
		"missing.customer.com",
	} {
		allowed, err := CanIssueTLSCertificateForDomain(domain)
		require.NoError(t, err)
		require.False(t, allowed)
	}
}

func TestAuthorizeTLSAskSecret(t *testing.T) {
	common.OptionMapRWMutex.Lock()
	if common.OptionMap == nil {
		common.OptionMap = make(map[string]string)
	}
	oldSecret := common.OptionMap[TLSAskSecretOptionKey]
	common.OptionMap[TLSAskSecretOptionKey] = "super-secret"
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		common.OptionMap[TLSAskSecretOptionKey] = oldSecret
		common.OptionMapRWMutex.Unlock()
	})

	require.True(t, AuthorizeTLSAskSecret("", "super-secret"))
	require.True(t, AuthorizeTLSAskSecret("super-secret", ""))
	require.False(t, AuthorizeTLSAskSecret("", "wrong"))
	require.False(t, AuthorizeTLSAskSecret("", ""))
}

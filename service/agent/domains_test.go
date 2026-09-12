package agent

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
)

func TestCreateDomainsPreservesExistingDomainsAndResolvesEveryAlias(t *testing.T) {
	setupAgentTestDB(t)
	agent := &model.Agent{Name: "Multi domain", Slug: "multi", Status: model.AgentStatusEnabled, OwnerUserId: 10}
	require.NoError(t, model.DB.Create(agent).Error)
	old, err := CreateDomain(agent.Id, "original.example.com")
	require.NoError(t, err)
	ctx, err := ResolveByHost("api2.example.com")
	require.NoError(t, err)
	require.Nil(t, ctx) // A miss before creation must not hide the new alias.
	domains, err := CreateDomains(agent.Id, []string{" API1.Example.com:443 ", "api2.example.com.", "api1.example.com"})
	require.NoError(t, err)
	require.Len(t, domains, 2)
	require.NotEqual(t, domains[0].VerifyToken, domains[1].VerifyToken)
	for _, domain := range append(domains, old) {
		ctx, err := ResolveByHost(domain.Domain)
		require.NoError(t, err)
		require.NotNil(t, ctx)
		require.Equal(t, agent.Id, ctx.AgentID)
		require.Equal(t, domain.Domain, ctx.Domain)
		allowed, err := CanIssueTLSCertificateForDomain(domain.Domain)
		require.NoError(t, err)
		require.True(t, allowed)
	}
	require.NoError(t, UpdateDomainStatus(agent.Id, domains[0].Id, model.AgentDomainStatusDisabled))
	ctx, err = ResolveByHost(domains[0].Domain)
	require.NoError(t, err)
	require.Nil(t, ctx)
	allowed, err := CanIssueTLSCertificateForDomain(domains[0].Domain)
	require.NoError(t, err)
	require.False(t, allowed)
	ctx, err = ResolveByHost(domains[1].Domain)
	require.NoError(t, err)
	require.NotNil(t, ctx)
	require.Equal(t, agent.Id, ctx.AgentID)
}

func TestCreateDomainsRejectsWholeBatchOnConflictOrInvalidDomain(t *testing.T) {
	setupAgentTestDB(t)
	agent := &model.Agent{Name: "Agent", Slug: "agent", Status: model.AgentStatusEnabled}
	other := &model.Agent{Name: "Other", Slug: "other", Status: model.AgentStatusEnabled}
	require.NoError(t, model.DB.Create(agent).Error)
	require.NoError(t, model.DB.Create(other).Error)
	_, err := CreateDomain(other.Id, "taken.example.com")
	require.NoError(t, err)
	for _, tc := range []struct {
		name  string
		input []string
		want  error
	}{
		{"conflict", []string{"new.example.com", "taken.example.com"}, ErrAgentDomainAlreadyExists},
		{"invalid", []string{"new.example.com", "bad.example.com/path"}, ErrInvalidAgentDomain},
		{"empty", nil, ErrInvalidAgentDomainBatch},
		{"too many", make([]string, MaxAgentDomainsPerBatch+1), ErrInvalidAgentDomainBatch},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := CreateDomains(agent.Id, tc.input)
			require.ErrorIs(t, err, tc.want)
			var count int64
			require.NoError(t, model.DB.Model(&model.AgentDomain{}).Where("agent_id = ?", agent.Id).Count(&count).Error)
			require.Zero(t, count)
		})
	}
	_, err = CreateDomains(other.Id+1, []string{"orphan.example.com"})
	require.ErrorIs(t, err, model.ErrAgentNotFound)
}

func TestNormalizeNewAgentDomain(t *testing.T) {
	for _, input := range []string{"https://api.example.com", "example.com,other.com", "*.example.com", "api..example.com", "-api.example.com", "api-.example.com", "api.example.com/path", "api.example.com?x=1", "127.0.0.1", "localhost", "api.example.com:wrong", "api.example.com:65536", strings.Repeat("a", 64) + ".example.com"} {
		t.Run(input, func(t *testing.T) {
			_, err := normalizeNewAgentDomain(input)
			require.ErrorIs(t, err, ErrInvalidAgentDomain)
		})
	}
}

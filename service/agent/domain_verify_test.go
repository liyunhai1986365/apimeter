package agent

import (
	"context"
	"errors"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
)

func TestCNAMEVerifyTargetUsesTokenAndBaseDomain(t *testing.T) {
	target, err := CNAMEVerifyTarget(" token-123 ", " agent-cname.example.com. ")

	require.NoError(t, err)
	require.Equal(t, "token-123.agent-cname.example.com", target)
}

func TestVerifyDomainCNAMEActivatesDomainWhenCNAMEMatchesTarget(t *testing.T) {
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
	domain := &model.AgentDomain{
		AgentId:     1,
		Domain:      "api.customer.com",
		Status:      model.AgentDomainStatusPending,
		VerifyToken: "verify-token",
	}
	require.NoError(t, model.DB.Create(domain).Error)

	verified, err := VerifyDomainCNAMEWithResolver(
		context.Background(),
		1,
		domain.Id,
		"agent-cname.example.com",
		func(ctx context.Context, host string) ([]string, error) {
			require.Equal(t, "api.customer.com", host)
			return []string{"verify-token.agent-cname.example.com."}, nil
		},
	)

	require.NoError(t, err)
	require.Equal(t, model.AgentDomainStatusActive, verified.Status)
	require.Greater(t, verified.VerifiedAt, int64(0))
}

func TestVerifyDomainCNAMERejectsMismatchedCNAME(t *testing.T) {
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
	domain := &model.AgentDomain{
		AgentId:     1,
		Domain:      "api.customer.com",
		Status:      model.AgentDomainStatusPending,
		VerifyToken: "verify-token",
	}
	require.NoError(t, model.DB.Create(domain).Error)

	verified, err := VerifyDomainCNAMEWithResolver(
		context.Background(),
		1,
		domain.Id,
		"agent-cname.example.com",
		func(ctx context.Context, host string) ([]string, error) {
			return []string{"other-token.agent-cname.example.com"}, nil
		},
	)

	require.ErrorIs(t, err, ErrAgentDomainCNAMEUnverified)
	require.Nil(t, verified)

	var reloaded model.AgentDomain
	require.NoError(t, model.DB.First(&reloaded, "id = ?", domain.Id).Error)
	require.Equal(t, model.AgentDomainStatusPending, reloaded.Status)
	require.Equal(t, int64(0), reloaded.VerifiedAt)
}

func TestVerifyDomainCNAMERequiresConfiguredBaseDomain(t *testing.T) {
	_, err := CNAMEVerifyTarget("verify-token", "")

	require.ErrorIs(t, err, ErrAgentCNAMEBaseDomainNotConfigured)
}

func TestVerifyDomainCNAMEReturnsLookupErrors(t *testing.T) {
	setupAgentTestDB(t)

	domain := &model.AgentDomain{
		AgentId:     1,
		Domain:      "api.customer.com",
		Status:      model.AgentDomainStatusPending,
		VerifyToken: "verify-token",
	}
	require.NoError(t, model.DB.Create(domain).Error)

	lookupErr := errors.New("dns timeout")
	verified, err := VerifyDomainCNAMEWithResolver(
		context.Background(),
		1,
		domain.Id,
		"agent-cname.example.com",
		func(ctx context.Context, host string) ([]string, error) {
			return nil, lookupErr
		},
	)

	require.ErrorIs(t, err, lookupErr)
	require.Nil(t, verified)
}

func TestTryAutoVerifyDomainCNAMEReturnsNilWhenUnverified(t *testing.T) {
	setupAgentTestDB(t)

	domain := &model.AgentDomain{
		AgentId:     1,
		Domain:      "api.customer.com",
		Status:      model.AgentDomainStatusPending,
		VerifyToken: "verify-token",
	}
	require.NoError(t, model.DB.Create(domain).Error)

	verified, err := TryAutoVerifyDomainCNAMEWithResolver(
		context.Background(),
		1,
		domain.Id,
		"agent-cname.example.com",
		func(ctx context.Context, host string) ([]string, error) {
			return []string{"other-token.agent-cname.example.com"}, nil
		},
	)

	require.NoError(t, err)
	require.Nil(t, verified)
}

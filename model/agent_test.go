package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func insertAgentForOwnershipTest(t *testing.T, ownerUserId int, status int) {
	t.Helper()
	agent := &Agent{
		OwnerUserId: ownerUserId,
		Name:        "Agent Ownership",
		Slug:        "agent-ownership-test",
		Status:      status,
		PriceMode:   AgentPriceModeMultiplier,
	}
	require.NoError(t, DB.Create(agent).Error)
}

func TestUserOwnsAgent(t *testing.T) {
	t.Run("returns false when user has no agent", func(t *testing.T) {
		truncateTables(t)

		ownsAgent, err := UserOwnsAgent(1001)

		require.NoError(t, err)
		assert.False(t, ownsAgent)
	})

	t.Run("returns true for non-disabled owned agent", func(t *testing.T) {
		truncateTables(t)

		insertAgentForOwnershipTest(t, 1002, AgentStatusEnabled)

		ownsAgent, err := UserOwnsAgent(1002)

		require.NoError(t, err)
		assert.True(t, ownsAgent)
	})

	t.Run("returns false for disabled owned agent", func(t *testing.T) {
		truncateTables(t)

		insertAgentForOwnershipTest(t, 1003, AgentStatusDisabled)

		ownsAgent, err := UserOwnsAgent(1003)

		require.NoError(t, err)
		assert.False(t, ownsAgent)
	})
}

func TestListAgentDomainsByStatus(t *testing.T) {
	truncateTables(t)

	agentA := &Agent{
		OwnerUserId:   2001,
		Name:          "Alpha Agent",
		Slug:          "alpha-agent",
		Status:        AgentStatusEnabled,
		PriceMode:     AgentPriceModeMultiplier,
		DefaultMarkup: 1,
	}
	agentB := &Agent{
		OwnerUserId:   2002,
		Name:          "Beta Agent",
		Slug:          "beta-agent",
		Status:        AgentStatusEnabled,
		PriceMode:     AgentPriceModeMultiplier,
		DefaultMarkup: 1,
	}
	require.NoError(t, DB.Create(agentA).Error)
	require.NoError(t, DB.Create(agentB).Error)
	require.NoError(t, DB.Create(&AgentDomain{
		AgentId:     agentA.Id,
		Domain:      "pending.example.com",
		Status:      AgentDomainStatusPending,
		VerifyToken: "token-pending",
	}).Error)
	require.NoError(t, DB.Create(&AgentDomain{
		AgentId:     agentB.Id,
		Domain:      "active.example.com",
		Status:      AgentDomainStatusActive,
		VerifyToken: "token-active",
	}).Error)

	domains, total, err := ListAgentDomainsByStatus(AgentDomainStatusPending, 0, 10)

	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, domains, 1)
	assert.Equal(t, "pending.example.com", domains[0].Domain)
	assert.Equal(t, agentA.Id, domains[0].AgentId)
	assert.Equal(t, "Alpha Agent", domains[0].AgentName)
	assert.Equal(t, "alpha-agent", domains[0].AgentSlug)
	assert.Equal(t, 2001, domains[0].OwnerUserId)
}

func TestUserHasAgentConsole(t *testing.T) {
	t.Run("returns true for agent owner", func(t *testing.T) {
		truncateTables(t)

		insertAgentForOwnershipTest(t, 3001, AgentStatusEnabled)

		hasConsole, err := UserHasAgentConsole(3001)

		require.NoError(t, err)
		assert.True(t, hasConsole)
	})

	t.Run("returns true for enabled bound user", func(t *testing.T) {
		truncateTables(t)

		agent := &Agent{
			OwnerUserId:   3002,
			Name:          "Bound Agent",
			Slug:          "bound-agent",
			Status:        AgentStatusEnabled,
			PriceMode:     AgentPriceModeMultiplier,
			DefaultMarkup: 1,
		}
		require.NoError(t, DB.Create(agent).Error)
		require.NoError(t, BindUserToAgent(agent.Id, 3003, AgentUserSourceAdminBind))

		hasConsole, err := UserHasAgentConsole(3003)

		require.NoError(t, err)
		assert.True(t, hasConsole)
	})

	t.Run("returns false for user without opened agent access", func(t *testing.T) {
		truncateTables(t)

		hasConsole, err := UserHasAgentConsole(3004)

		require.NoError(t, err)
		assert.False(t, hasConsole)
	})
}

func TestGetAgentByConsoleUserId(t *testing.T) {
	t.Run("returns agent for bound user", func(t *testing.T) {
		truncateTables(t)

		agent := &Agent{
			OwnerUserId:   4001,
			Name:          "Console Agent",
			Slug:          "console-agent",
			Status:        AgentStatusEnabled,
			PriceMode:     AgentPriceModeMultiplier,
			DefaultMarkup: 1,
		}
		require.NoError(t, DB.Create(agent).Error)
		require.NoError(t, BindUserToAgent(agent.Id, 4002, AgentUserSourceAdminBind))

		got, err := GetAgentByConsoleUserId(4002)

		require.NoError(t, err)
		assert.Equal(t, agent.Id, got.Id)
	})

	t.Run("does not return disabled agent", func(t *testing.T) {
		truncateTables(t)

		agent := &Agent{
			OwnerUserId:   4003,
			Name:          "Disabled Agent",
			Slug:          "disabled-agent",
			Status:        AgentStatusDisabled,
			PriceMode:     AgentPriceModeMultiplier,
			DefaultMarkup: 1,
		}
		require.NoError(t, DB.Create(agent).Error)
		require.NoError(t, BindUserToAgent(agent.Id, 4004, AgentUserSourceAdminBind))

		_, err := GetAgentByConsoleUserId(4004)

		require.ErrorIs(t, err, ErrAgentNotFound)
	})
}

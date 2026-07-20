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

func TestActivateLegacyPendingAgentDomains(t *testing.T) {
	truncateTables(t)

	require.NoError(t, DB.Create(&AgentDomain{
		AgentId: 1,
		Domain:  "pending.example.com",
		Status:  AgentDomainStatusPending,
	}).Error)
	require.NoError(t, DB.Create(&AgentDomain{
		AgentId: 2,
		Domain:  "disabled.example.com",
		Status:  AgentDomainStatusDisabled,
	}).Error)

	require.NoError(t, activateLegacyPendingAgentDomains())

	var domains []AgentDomain
	require.NoError(t, DB.Order("id ASC").Find(&domains).Error)
	require.Equal(t, AgentDomainStatusActive, domains[0].Status)
	require.Equal(t, AgentDomainStatusDisabled, domains[1].Status)
}

func TestUserHasAgentConsole(t *testing.T) {
	t.Run("returns true for agent owner", func(t *testing.T) {
		truncateTables(t)

		insertAgentForOwnershipTest(t, 3001, AgentStatusEnabled)

		hasConsole, err := UserHasAgentConsole(3001)

		require.NoError(t, err)
		assert.True(t, hasConsole)
	})

	t.Run("returns false for enabled bound user", func(t *testing.T) {
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
		assert.False(t, hasConsole)
	})

	t.Run("returns false for user without opened agent access", func(t *testing.T) {
		truncateTables(t)

		hasConsole, err := UserHasAgentConsole(3004)

		require.NoError(t, err)
		assert.False(t, hasConsole)
	})
}

func TestGetAgentByConsoleUserId(t *testing.T) {
	t.Run("does not return agent for bound user", func(t *testing.T) {
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

		require.ErrorIs(t, err, ErrAgentNotFound)
		assert.Nil(t, got)
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

func TestListAgentUsersIncludesUserProfile(t *testing.T) {
	truncateTables(t)

	agent := &Agent{
		OwnerUserId:   5001,
		Name:          "Profile Agent",
		Slug:          "profile-agent",
		Status:        AgentStatusEnabled,
		PriceMode:     AgentPriceModeMultiplier,
		DefaultMarkup: 1,
	}
	otherAgent := &Agent{
		OwnerUserId:   5002,
		Name:          "Other Agent",
		Slug:          "other-agent",
		Status:        AgentStatusEnabled,
		PriceMode:     AgentPriceModeMultiplier,
		DefaultMarkup: 1,
	}
	require.NoError(t, DB.Create(agent).Error)
	require.NoError(t, DB.Create(otherAgent).Error)

	user := &User{
		Username:     "agent-user",
		Password:     "hashed-password",
		DisplayName:  "Agent User",
		Email:        "agent-user@example.com",
		Role:         1,
		Status:       1,
		Group:        "default",
		Quota:        123456,
		UsedQuota:    6543,
		RequestCount: 42,
		AffCode:      "agent-user-aff",
		CreatedAt:    1710000000,
		LastLoginAt:  1710003600,
	}
	otherUser := &User{
		Username: "other-agent-user",
		Password: "hashed-password",
		Role:     1,
		Status:   1,
		Group:    "default",
		AffCode:  "other-agent-user-aff",
	}
	require.NoError(t, DB.Create(user).Error)
	require.NoError(t, DB.Create(otherUser).Error)
	require.NoError(t, BindUserToAgent(agent.Id, user.Id, AgentUserSourceDomain))
	require.NoError(t, BindUserToAgent(otherAgent.Id, otherUser.Id, AgentUserSourceDomain))

	users, total, err := ListAgentUsers(agent.Id, "agent-user", 0, 10)

	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, users, 1)
	assert.Equal(t, agent.Id, users[0].AgentId)
	assert.Equal(t, user.Id, users[0].UserId)
	assert.Equal(t, "agent-user", users[0].Username)
	assert.Equal(t, "Agent User", users[0].DisplayName)
	assert.Equal(t, "agent-user@example.com", users[0].Email)
	assert.Equal(t, "default", users[0].Group)
	assert.Equal(t, 123456, users[0].Quota)
	assert.Equal(t, 6543, users[0].UsedQuota)
	assert.Equal(t, 42, users[0].RequestCount)
	assert.Equal(t, 1, users[0].UserStatus)
	assert.Equal(t, 1, users[0].Role)
	assert.Equal(t, int64(1710000000), users[0].UserCreatedAt)
	assert.Equal(t, int64(1710003600), users[0].LastLoginAt)
}

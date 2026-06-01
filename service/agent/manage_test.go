package agent

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
)

func TestUpdateBrandingUpdatesOnlyTargetAgent(t *testing.T) {
	setupAgentTestDB(t)

	require.NoError(t, model.DB.Create(&model.Agent{
		Id:            1,
		OwnerUserId:   10,
		Name:          "Agent One",
		Slug:          "agent-one",
		Status:        model.AgentStatusEnabled,
		PriceMode:     model.AgentPriceModeMultiplier,
		DefaultMarkup: 1,
		Branding:      `{"site_name":"Old","logo":"/old.png"}`,
	}).Error)
	require.NoError(t, model.DB.Create(&model.Agent{
		Id:            2,
		OwnerUserId:   20,
		Name:          "Agent Two",
		Slug:          "agent-two",
		Status:        model.AgentStatusEnabled,
		PriceMode:     model.AgentPriceModeMultiplier,
		DefaultMarkup: 1,
		Branding:      `{"site_name":"Keep","logo":"/keep.png"}`,
	}).Error)

	updated, err := UpdateBranding(1, `{"site_name":"New","logo":"https://agent.example/logo.png"}`)

	require.NoError(t, err)
	require.Equal(t, `{"site_name":"New","logo":"https://agent.example/logo.png"}`, updated.Branding)

	var untouched model.Agent
	require.NoError(t, model.DB.First(&untouched, "id = ?", 2).Error)
	require.Equal(t, `{"site_name":"Keep","logo":"/keep.png"}`, untouched.Branding)
}

func TestUpdateUserGlobalStatusOnlyAffectsBoundAgentUser(t *testing.T) {
	setupAgentTestDB(t)

	agent := createManageTestAgent(t, 10, "agent-a")
	otherAgent := createManageTestAgent(t, 20, "agent-b")
	user := createManageTestUser(t, "agent-user", "default", common.UserStatusEnabled)
	otherUser := createManageTestUser(t, "other-user", "default", common.UserStatusEnabled)
	require.NoError(t, model.BindUserToAgent(agent.Id, user.Id, model.AgentUserSourceDomain))
	require.NoError(t, model.BindUserToAgent(otherAgent.Id, otherUser.Id, model.AgentUserSourceDomain))

	require.NoError(t, UpdateUserGlobalStatus(agent.Id, user.Id, common.UserStatusDisabled))
	require.ErrorIs(t, UpdateUserGlobalStatus(agent.Id, otherUser.Id, common.UserStatusDisabled), model.ErrAgentUserNotFound)

	updated, err := model.GetUserById(user.Id, false)
	require.NoError(t, err)
	require.Equal(t, common.UserStatusDisabled, updated.Status)

	untouched, err := model.GetUserById(otherUser.Id, false)
	require.NoError(t, err)
	require.Equal(t, common.UserStatusEnabled, untouched.Status)
}

func TestUpdateUserGroupRequiresAgentConfiguredGroup(t *testing.T) {
	setupAgentTestDB(t)

	agent := createManageTestAgent(t, 10, "agent-a")
	user := createManageTestUser(t, "agent-user", "default", common.UserStatusEnabled)
	require.NoError(t, model.BindUserToAgent(agent.Id, user.Id, model.AgentUserSourceDomain))
	require.NoError(t, model.DB.Create(&model.AgentGroupRatio{
		AgentId:   agent.Id,
		GroupName: "vip",
		Ratio:     1,
	}).Error)

	require.ErrorIs(t, UpdateUserGroup(agent.Id, user.Id, "svip"), ErrAgentUserGroupNotAllowed)
	require.NoError(t, UpdateUserGroup(agent.Id, user.Id, "vip"))

	updated, err := model.GetUserById(user.Id, false)
	require.NoError(t, err)
	require.Equal(t, "default", updated.Group)

	var agentUser model.AgentUser
	require.NoError(t, model.DB.Where("agent_id = ? AND user_id = ?", agent.Id, user.Id).First(&agentUser).Error)
	require.Equal(t, "vip", agentUser.Group)
}

func createManageTestAgent(t *testing.T, ownerUserID int, slug string) *model.Agent {
	t.Helper()
	agent := &model.Agent{
		OwnerUserId:   ownerUserID,
		Name:          slug,
		Slug:          slug,
		Status:        model.AgentStatusEnabled,
		PriceMode:     model.AgentPriceModeMultiplier,
		DefaultMarkup: 1,
	}
	require.NoError(t, model.DB.Create(agent).Error)
	return agent
}

func createManageTestUser(t *testing.T, username string, group string, status int) *model.User {
	t.Helper()
	user := &model.User{
		Username:    username,
		Password:    "hashed-password",
		DisplayName: username,
		Email:       username + "@example.com",
		Role:        common.RoleCommonUser,
		Status:      status,
		Group:       group,
		AffCode:     username + "-aff",
	}
	require.NoError(t, model.DB.Create(user).Error)
	return user
}

package agent

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
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

func TestUpdateUserGroupRequiresAgentConfiguredUserGroup(t *testing.T) {
	setupAgentTestDB(t)

	agent := createManageTestAgent(t, 10, "agent-a")
	user := createManageTestUser(t, "agent-user", "default", common.UserStatusEnabled)
	require.NoError(t, model.BindUserToAgent(agent.Id, user.Id, model.AgentUserSourceDomain))
	_, err := UpsertUserGroupConfig(agent.Id, "member", []string{"vip"})
	require.NoError(t, err)

	require.ErrorIs(t, UpdateUserGroup(agent.Id, user.Id, "vip"), ErrAgentUserGroupNotAllowed)
	require.NoError(t, UpdateUserGroup(agent.Id, user.Id, "member"))

	updated, err := model.GetUserById(user.Id, false)
	require.NoError(t, err)
	require.Equal(t, "default", updated.Group)

	var agentUser model.AgentUser
	require.NoError(t, model.DB.Where("agent_id = ? AND user_id = ?", agent.Id, user.Id).First(&agentUser).Error)
	require.Equal(t, "member", agentUser.Group)
}

func TestVisibleGroupsForUserAppendsUserGroupConfiguredGroups(t *testing.T) {
	setupAgentTestDB(t)
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"vip":1,"svip":1}`))
		require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(`{"vip":{"edit_this":0.9}}`))
	})
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"vip":1.25,"svip":1.5}`))

	agent := createManageTestAgent(t, 10, "agent-a")
	_, err := UpsertGroupRatio(agent.Id, "default", "default", "", 1, true)
	require.NoError(t, err)
	_, err = UpsertGroupRatio(agent.Id, "agent-vip", "vip", "", 1.5, true)
	require.NoError(t, err)
	_, err = UpsertGroupRatio(agent.Id, "internal-svip", "svip", "", 1.8, false)
	require.NoError(t, err)
	_, err = UpsertUserGroupConfig(agent.Id, "member", []string{"internal-svip"})
	require.NoError(t, err)

	groups, err := EffectiveGroupMap(agent.Id)
	require.NoError(t, err)
	userGroups, err := UserGroupConfigMap(agent.Id)
	require.NoError(t, err)
	ctx := &Context{AgentID: agent.Id, Groups: groups, UserGroups: userGroups}

	visibleGroups := VisibleGroupsForUser(ctx, "member")
	require.Contains(t, visibleGroups, "default")
	require.Contains(t, visibleGroups, "agent-vip")
	require.Contains(t, visibleGroups, "internal-svip")
	require.False(t, visibleGroups["internal-svip"].Visible)
}

func TestUserGroupConfigStoresGroupRatioOverrides(t *testing.T) {
	setupAgentTestDB(t)
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"vip":1,"svip":1}`))
	})
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"vip":1.25}`))

	agent := createManageTestAgent(t, 10, "agent-a")
	_, err := UpsertGroupRatio(agent.Id, "agent-vip", "vip", "", 1.5, true)
	require.NoError(t, err)
	_, err = UpsertUserGroupConfig(agent.Id, "member", []string{"agent-vip"}, map[string]float64{
		"agent-vip": 1.3,
		" ":         2,
		"negative":  -1,
	})
	require.NoError(t, err)

	views, err := ListUserGroupConfigs(agent.Id)
	require.NoError(t, err)
	require.Len(t, views, 1)
	require.Equal(t, map[string]float64{"agent-vip": 1.3}, views[0].GroupRatios)

	userGroups, err := UserGroupConfigMap(agent.Id)
	require.NoError(t, err)
	require.Equal(t, map[string]float64{"agent-vip": 1.3}, userGroups["member"].GroupRatios)
}

func TestUserGroupConfigRatioOverrideIsFlooredByAgentConfiguredRatio(t *testing.T) {
	setupAgentTestDB(t)
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"vip":1,"svip":1}`))
		require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(`{"vip":{"edit_this":0.9}}`))
	})
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"vip":0.2}`))
	require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(`{"agent-owner":{"vip":0.24}}`))

	owner := createManageTestUser(t, "agent-owner", "agent-owner", common.UserStatusEnabled)
	agent := createManageTestAgent(t, owner.Id, "agent-a")
	_, err := UpsertGroupRatio(agent.Id, "agent-vip", "vip", "", 0.24, true)
	require.NoError(t, err)

	_, err = UpsertUserGroupConfig(agent.Id, "member", []string{"agent-vip"}, map[string]float64{
		"agent-vip": 0.22,
	})
	require.NoError(t, err)

	userGroups, err := UserGroupConfigMap(agent.Id)
	require.NoError(t, err)
	require.Equal(t, map[string]float64{"agent-vip": 0.24}, userGroups["member"].GroupRatios)

	_, err = UpsertUserGroupConfig(agent.Id, "member", []string{"agent-vip"}, map[string]float64{
		"agent-vip": 0.18,
	})
	require.NoError(t, err)

	userGroups, err = UserGroupConfigMap(agent.Id)
	require.NoError(t, err)
	require.Equal(t, map[string]float64{"agent-vip": 0.24}, userGroups["member"].GroupRatios)
}

func TestUserGroupConfigRatioAllowsPriceBetweenAgentConfiguredAndEffectiveRatio(t *testing.T) {
	setupAgentTestDB(t)
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"vip":1,"svip":1}`))
		require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(`{"vip":{"edit_this":0.9}}`))
	})
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"svip":1}`))
	require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(`{"agent-owner":{"svip":1.2}}`))

	owner := createManageTestUser(t, "agent-owner", "agent-owner", common.UserStatusEnabled)
	agent := createManageTestAgent(t, owner.Id, "agent-a")
	_, err := UpsertGroupRatio(agent.Id, "svip", "svip", "", 1.2, true)
	require.NoError(t, err)

	_, err = UpsertUserGroupConfig(agent.Id, "member", []string{"svip"}, map[string]float64{
		"svip": 1.1,
	})
	require.NoError(t, err)

	userGroups, err := UserGroupConfigMap(agent.Id)
	require.NoError(t, err)
	require.Equal(t, map[string]float64{"svip": 1.2}, userGroups["member"].GroupRatios)
}

func TestAgentGroupRatiosUseOwnerSystemDiscountAsInitialRatio(t *testing.T) {
	setupAgentTestDB(t)
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"vip":1,"svip":1}`))
		require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(`{"vip":{"edit_this":0.9}}`))
	})
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"vip":1.25,"svip":1.5}`))
	require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(`{"default":{"vip":1.4}}`))

	owner := createManageTestUser(t, "agent-owner", "default", common.UserStatusEnabled)
	agent := createManageTestAgent(t, owner.Id, "agent-a")

	_, err := UpsertGroupRatio(agent.Id, "agent-vip", "vip", "", 1.3, true)
	require.ErrorIs(t, err, ErrAgentGroupRatioBelowSystem)

	_, err = UpsertGroupRatio(agent.Id, "agent-vip", "vip", "", 1.45, true)
	require.NoError(t, err)

	ratios, err := ListGroupRatios(agent.Id)
	require.NoError(t, err)

	var found GroupRatioView
	for _, ratio := range ratios {
		if ratio.GroupName == "agent-vip" {
			found = ratio
			break
		}
	}

	require.Equal(t, "agent-vip", found.GroupName)
	require.Equal(t, "vip", found.SystemGroupName)
	require.Equal(t, 1.4, found.AgentRatio)
	require.Equal(t, 1.4, found.SystemRatio)
	require.Equal(t, 1.45, found.ConfiguredRatio)
	require.Equal(t, 1.45, found.EffectiveRatio)

	groups, err := EffectiveGroupMap(agent.Id)
	require.NoError(t, err)
	require.Equal(t, 1.4, groups["agent-vip"].AgentRatio)
	require.Equal(t, 1.4, groups["agent-vip"].SystemRatio)
	require.Equal(t, 1.45, groups["agent-vip"].ConfiguredRatio)
	require.Equal(t, 1.45, groups["agent-vip"].EffectiveRatio)
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

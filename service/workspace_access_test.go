package service_test

import (
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupWorkspaceAccessTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	originalDB := model.DB
	dsn := fmt.Sprintf("file:workspace-access-%s?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Workspace{}, &model.WorkspaceMember{}, &model.Token{}))
	model.DB = db
	t.Cleanup(func() { model.DB = originalDB })
	return db
}

// grantWorkspaceMember writes the access edge directly so the seed does not depend on the
// controller-level validation path.
func grantWorkspaceMember(t *testing.T, db *gorm.DB, workspaceId int, userId int) {
	t.Helper()
	require.NoError(t, db.Create(&model.WorkspaceMember{
		WorkspaceId: workspaceId, UserId: userId, CreatedTime: common.GetTimestamp(),
	}).Error)
}

func revokeWorkspaceMembers(t *testing.T, db *gorm.DB, userId int) {
	t.Helper()
	require.NoError(t, db.Where("user_id = ?", userId).Delete(&model.WorkspaceMember{}).Error)
}

func seedWorkspaceAccessAccounts(t *testing.T, db *gorm.DB) (model.User, model.User, model.Workspace, model.Workspace) {
	t.Helper()
	owner := model.User{Username: "owner", Password: "hashed", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", AffCode: "ownr"}
	require.NoError(t, db.Create(&owner).Error)
	child := model.User{Username: "workspace-user", Password: "hashed", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, ParentUserId: owner.Id, Group: "default", AffCode: "wusr"}
	require.NoError(t, db.Create(&child).Error)
	assigned := model.Workspace{UserId: owner.Id, Name: "Assigned", Status: model.WorkspaceStatusEnabled}
	unassigned := model.Workspace{UserId: owner.Id, Name: "Private", Status: model.WorkspaceStatusEnabled}
	require.NoError(t, db.Create(&assigned).Error)
	require.NoError(t, db.Create(&unassigned).Error)
	grantWorkspaceMember(t, db, assigned.Id, child.Id)
	return owner, child, assigned, unassigned
}

func TestResolveWorkspaceAccessScopeSeparatesActorOwnerAndWorkspaces(t *testing.T) {
	db := setupWorkspaceAccessTestDB(t)
	owner, child, assigned, _ := seedWorkspaceAccessAccounts(t, db)

	scope, err := service.ResolveWorkspaceAccessScope(child.Id)
	require.NoError(t, err)
	require.True(t, scope.IsSubaccount)
	require.Equal(t, child.Id, scope.ActorUserId)
	require.Equal(t, owner.Id, scope.OwnerUserId)
	require.Equal(t, []int{assigned.Id}, scope.WorkspaceIds)
	require.Equal(t, []int{assigned.Id}, scope.WorkspaceFilter())
}

// A main account must stay unrestricted: WorkspaceFilter has to return nil, not an empty
// slice, otherwise every owner-facing query would silently return nothing.
func TestResolveWorkspaceAccessScopeLeavesMainAccountUnrestricted(t *testing.T) {
	db := setupWorkspaceAccessTestDB(t)
	owner, _, _, _ := seedWorkspaceAccessAccounts(t, db)

	scope, err := service.ResolveWorkspaceAccessScope(owner.Id)
	require.NoError(t, err)
	require.False(t, scope.IsSubaccount)
	require.Equal(t, owner.Id, scope.OwnerUserId)
	require.Nil(t, scope.WorkspaceFilter())
}

// One workspace may now be shared by several child accounts, and one child account may
// reach several workspaces. Both directions must resolve.
func TestWorkspaceMembershipIsManyToMany(t *testing.T) {
	db := setupWorkspaceAccessTestDB(t)
	owner, child, assigned, other := seedWorkspaceAccessAccounts(t, db)
	second := model.User{Username: "workspace-user-2", Password: "hashed", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, ParentUserId: owner.Id, Group: "default", AffCode: "wus2"}
	require.NoError(t, db.Create(&second).Error)
	grantWorkspaceMember(t, db, assigned.Id, second.Id)
	grantWorkspaceMember(t, db, other.Id, child.Id)

	firstScope, err := service.ResolveWorkspaceAccessScope(child.Id)
	require.NoError(t, err)
	require.ElementsMatch(t, []int{assigned.Id, other.Id}, firstScope.WorkspaceIds)

	secondScope, err := service.ResolveWorkspaceAccessScope(second.Id)
	require.NoError(t, err)
	require.Equal(t, []int{assigned.Id}, secondScope.WorkspaceIds)
}

func TestCreateWorkspaceSubaccountForcesOwnerBillingDefaults(t *testing.T) {
	db := setupWorkspaceAccessTestDB(t)
	owner, _, assigned, _ := seedWorkspaceAccessAccounts(t, db)

	child := model.User{
		Username:     "new-workspace-user",
		Password:     "temporary-password",
		DisplayName:  "New Workspace User",
		Quota:        12345,
		UsedQuota:    678,
		RequestCount: 9,
		Role:         common.RoleRootUser,
	}
	require.NoError(t, model.CreateWorkspaceSubaccount(owner.Id, &child, []int{assigned.Id}))

	var persisted model.User
	require.NoError(t, db.First(&persisted, child.Id).Error)
	require.Equal(t, owner.Id, persisted.ParentUserId)
	require.Equal(t, common.RoleCommonUser, persisted.Role)
	require.Equal(t, common.UserStatusEnabled, persisted.Status)
	require.True(t, persisted.MustChangePassword)
	require.Zero(t, persisted.Quota)
	require.Zero(t, persisted.UsedQuota)
	require.Zero(t, persisted.RequestCount)
	require.Equal(t, owner.Group, persisted.Group)

	// The new account joins the workspace without evicting the account seeded above.
	var members []model.WorkspaceMember
	require.NoError(t, db.Where("workspace_id = ?", assigned.Id).Find(&members).Error)
	memberIds := make([]int, 0, len(members))
	for _, member := range members {
		memberIds = append(memberIds, member.UserId)
	}
	require.Contains(t, memberIds, child.Id)
	require.Len(t, memberIds, 2)
}

func TestWorkspaceTokenScopeNeverFallsBackWhenEmpty(t *testing.T) {
	db := setupWorkspaceAccessTestDB(t)
	owner, child, _, unassigned := seedWorkspaceAccessAccounts(t, db)
	token := model.Token{UserId: owner.Id, WorkspaceId: unassigned.Id, Name: "private", Key: "private-key"}
	require.NoError(t, db.Create(&token).Error)
	revokeWorkspaceMembers(t, db, child.Id)

	scope, err := service.ResolveWorkspaceAccessScope(child.Id)
	require.NoError(t, err)
	require.Empty(t, scope.WorkspaceIds)
	// Non-nil empty filter: restricted to nothing, never widened to the owner's tenant.
	require.NotNil(t, scope.WorkspaceFilter())
	require.Empty(t, scope.WorkspaceFilter())
	tokens, err := model.GetAllUserTokens(owner.Id, 0, 20, 0, scope.WorkspaceFilter())
	require.NoError(t, err)
	require.Empty(t, tokens)
}

func TestEmptyWorkspaceScopeCannotReadOwnerLogsTasksOrUsage(t *testing.T) {
	db := setupWorkspaceAccessTestDB(t)
	owner, child, _, unassigned := seedWorkspaceAccessAccounts(t, db)
	originalLogDB := model.LOG_DB
	model.LOG_DB = db
	t.Cleanup(func() { model.LOG_DB = originalLogDB })
	require.NoError(t, db.AutoMigrate(&model.Log{}, &model.Task{}, &model.QuotaData{}))

	token := model.Token{UserId: owner.Id, WorkspaceId: unassigned.Id, Name: "private", Key: "private-scope-key"}
	require.NoError(t, db.Create(&token).Error)
	require.NoError(t, db.Create(&model.Log{UserId: owner.Id, TokenId: token.Id, Type: model.LogTypeConsume, CreatedAt: 100, Quota: 10}).Error)
	require.NoError(t, db.Create(&model.Task{UserId: owner.Id, TokenId: token.Id, TaskID: "private-task", Status: model.TaskStatusSuccess}).Error)
	require.NoError(t, db.Create(&model.QuotaData{UserID: owner.Id, TokenID: token.Id, CreatedAt: 100, Count: 1, Quota: 10}).Error)
	revokeWorkspaceMembers(t, db, child.Id)

	scope, err := service.ResolveWorkspaceAccessScope(child.Id)
	require.NoError(t, err)
	allowedWorkspaceIds := scope.WorkspaceFilter()
	require.NotNil(t, allowedWorkspaceIds)
	require.Empty(t, allowedWorkspaceIds)

	logs, total, err := model.GetUserLogs(owner.Id, model.LogTypeUnknown, 0, 0, "", "", 0, 20, "", "", "", "", allowedWorkspaceIds)
	require.NoError(t, err)
	require.Empty(t, logs)
	require.Zero(t, total)

	taskQuery := model.SyncTaskQueryParams{AllowedWorkspaceIds: allowedWorkspaceIds}
	require.Empty(t, model.TaskGetAllUserTask(owner.Id, 0, 20, taskQuery))
	require.Zero(t, model.TaskCountAllUserTask(owner.Id, taskQuery))

	usage, err := model.GetSelfTokenQuotaData(0, 0, owner.Id, allowedWorkspaceIds)
	require.NoError(t, err)
	require.Empty(t, usage)
}

// The same queries must still return the owner's data for an accessible workspace,
// otherwise the empty-scope assertions above would pass for the wrong reason.
func TestWorkspaceScopeReadsOwnerDataInsideAccessibleWorkspace(t *testing.T) {
	db := setupWorkspaceAccessTestDB(t)
	owner, child, assigned, unassigned := seedWorkspaceAccessAccounts(t, db)
	originalLogDB := model.LOG_DB
	model.LOG_DB = db
	t.Cleanup(func() { model.LOG_DB = originalLogDB })
	require.NoError(t, db.AutoMigrate(&model.Log{}, &model.Task{}, &model.QuotaData{}))

	visible := model.Token{UserId: owner.Id, WorkspaceId: assigned.Id, Name: "visible", Key: "visible-key"}
	hidden := model.Token{UserId: owner.Id, WorkspaceId: unassigned.Id, Name: "hidden", Key: "hidden-key"}
	require.NoError(t, db.Create(&visible).Error)
	require.NoError(t, db.Create(&hidden).Error)
	require.NoError(t, db.Create(&model.Log{UserId: owner.Id, TokenId: visible.Id, TokenName: "visible", Type: model.LogTypeConsume, CreatedAt: 100, Quota: 10}).Error)
	require.NoError(t, db.Create(&model.Log{UserId: owner.Id, TokenId: hidden.Id, TokenName: "hidden", Type: model.LogTypeConsume, CreatedAt: 101, Quota: 20}).Error)

	scope, err := service.ResolveWorkspaceAccessScope(child.Id)
	require.NoError(t, err)

	tokens, err := model.GetAllUserTokens(owner.Id, 0, 20, 0, scope.WorkspaceFilter())
	require.NoError(t, err)
	require.Len(t, tokens, 1)
	require.Equal(t, visible.Id, tokens[0].Id)

	logs, _, err := model.GetUserLogs(owner.Id, model.LogTypeUnknown, 0, 0, "", "", 0, 20, "", "", "", "", scope.WorkspaceFilter())
	require.NoError(t, err)
	require.Len(t, logs, 1)
	require.Equal(t, visible.Id, logs[0].TokenId)
}

func TestDisableWorkspaceSubaccountKeepsOwnerTokensEnabled(t *testing.T) {
	db := setupWorkspaceAccessTestDB(t)
	owner, child, assigned, unassigned := seedWorkspaceAccessAccounts(t, db)
	accessible := model.Token{UserId: owner.Id, WorkspaceId: assigned.Id, Name: "accessible", Key: "accessible-key", Status: common.TokenStatusEnabled}
	private := model.Token{UserId: owner.Id, WorkspaceId: unassigned.Id, Name: "private", Key: "private-key", Status: common.TokenStatusEnabled}
	subscription := model.Token{UserId: owner.Id, WorkspaceId: assigned.Id, Name: "subscription", Key: "subscription-key", Status: common.TokenStatusEnabled, BillingSource: "subscription"}
	require.NoError(t, db.Create(&accessible).Error)
	require.NoError(t, db.Create(&private).Error)
	require.NoError(t, db.Create(&subscription).Error)

	require.NoError(t, model.SetWorkspaceSubaccountStatus(owner.Id, child.Id, common.UserStatusDisabled))
	require.NoError(t, db.First(&accessible, accessible.Id).Error)
	require.NoError(t, db.First(&private, private.Id).Error)
	require.NoError(t, db.First(&subscription, subscription.Id).Error)
	require.Equal(t, common.TokenStatusEnabled, accessible.Status)
	require.Equal(t, common.TokenStatusEnabled, private.Status)
	require.Equal(t, common.TokenStatusEnabled, subscription.Status)
}

func TestDeleteWorkspaceSubaccountRevokesAccessWithoutChangingOwnerTokens(t *testing.T) {
	db := setupWorkspaceAccessTestDB(t)
	owner, child, assigned, _ := seedWorkspaceAccessAccounts(t, db)
	token := model.Token{UserId: owner.Id, WorkspaceId: assigned.Id, Name: "owner-token", Key: "owner-token-key", Status: common.TokenStatusEnabled}
	require.NoError(t, db.Create(&token).Error)

	require.NoError(t, model.DeleteWorkspaceSubaccount(owner.Id, child.Id))
	require.NoError(t, db.First(&token, token.Id).Error)
	require.Equal(t, common.TokenStatusEnabled, token.Status)
	require.NoError(t, db.First(&assigned, assigned.Id).Error)
	var memberCount int64
	require.NoError(t, db.Model(&model.WorkspaceMember{}).Where("user_id = ?", child.Id).Count(&memberCount).Error)
	require.Zero(t, memberCount)
}

func TestSetWorkspaceAccessRejectsDefaultAndForeignChild(t *testing.T) {
	db := setupWorkspaceAccessTestDB(t)
	owner, child, assigned, _ := seedWorkspaceAccessAccounts(t, db)
	defaultWorkspace := model.Workspace{UserId: owner.Id, Name: model.DefaultWorkspaceName, IsDefault: true, Status: model.WorkspaceStatusEnabled}
	require.NoError(t, db.Create(&defaultWorkspace).Error)
	foreignOwner := model.User{Username: "foreign-owner", Password: "hashed", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, AffCode: "forn"}
	require.NoError(t, db.Create(&foreignOwner).Error)
	foreignChild := model.User{Username: "foreign-child", Password: "hashed", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, ParentUserId: foreignOwner.Id, AffCode: "forc"}
	require.NoError(t, db.Create(&foreignChild).Error)

	_, err := model.SetWorkspaceAccess(owner.Id, defaultWorkspace.Id, []int{child.Id})
	require.ErrorIs(t, err, model.ErrDefaultWorkspaceAccess)
	_, err = model.SetWorkspaceAccess(owner.Id, assigned.Id, []int{foreignChild.Id})
	require.ErrorIs(t, err, model.ErrInvalidWorkspaceAccessAccount)

	// A rejected call must not touch the existing member list.
	var memberCount int64
	require.NoError(t, db.Model(&model.WorkspaceMember{}).Where("workspace_id = ?", assigned.Id).Count(&memberCount).Error)
	require.Equal(t, int64(1), memberCount)
}

// SetWorkspaceAccess is a replace, not a merge: the request body is the full member list.
func TestSetWorkspaceAccessReplacesMemberList(t *testing.T) {
	db := setupWorkspaceAccessTestDB(t)
	owner, child, assigned, _ := seedWorkspaceAccessAccounts(t, db)
	second := model.User{Username: "second-child", Password: "hashed", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, ParentUserId: owner.Id, Group: "default", AffCode: "secd"}
	require.NoError(t, db.Create(&second).Error)

	workspace, err := model.SetWorkspaceAccess(owner.Id, assigned.Id, []int{child.Id, second.Id})
	require.NoError(t, err)
	require.Len(t, workspace.AccessUsers, 2)

	workspace, err = model.SetWorkspaceAccess(owner.Id, assigned.Id, []int{second.Id})
	require.NoError(t, err)
	require.Len(t, workspace.AccessUsers, 1)
	require.Equal(t, second.Id, workspace.AccessUsers[0].Id)

	workspace, err = model.SetWorkspaceAccess(owner.Id, assigned.Id, nil)
	require.NoError(t, err)
	require.Empty(t, workspace.AccessUsers)
}

func TestSetWorkspaceAccessKeepsExistingDisabledMemberButRejectsNewOne(t *testing.T) {
	db := setupWorkspaceAccessTestDB(t)
	owner, child, assigned, _ := seedWorkspaceAccessAccounts(t, db)
	disabled := model.User{
		Username: "disabled-child", Password: "hashed", Role: common.RoleCommonUser,
		Status: common.UserStatusDisabled, ParentUserId: owner.Id, Group: "default", AffCode: "dsbd",
	}
	require.NoError(t, db.Create(&disabled).Error)
	require.NoError(t, db.Model(&child).Update("status", common.UserStatusDisabled).Error)

	workspace, err := model.SetWorkspaceAccess(owner.Id, assigned.Id, []int{child.Id})
	require.NoError(t, err)
	require.Len(t, workspace.AccessUsers, 1)
	require.Equal(t, child.Id, workspace.AccessUsers[0].Id)

	_, err = model.SetWorkspaceAccess(owner.Id, assigned.Id, []int{child.Id, disabled.Id})
	require.ErrorIs(t, err, model.ErrInvalidWorkspaceAccessAccount)

	var memberIds []int
	require.NoError(t, db.Model(&model.WorkspaceMember{}).
		Where("workspace_id = ?", assigned.Id).Pluck("user_id", &memberIds).Error)
	require.Equal(t, []int{child.Id}, memberIds)
}

// SetSubaccountWorkspaces is the mirror image: it replaces one account's whole workspace set.
func TestSetSubaccountWorkspacesReplacesWholeSet(t *testing.T) {
	db := setupWorkspaceAccessTestDB(t)
	owner, child, assigned, other := seedWorkspaceAccessAccounts(t, db)

	require.NoError(t, model.SetSubaccountWorkspaces(owner.Id, child.Id, []int{other.Id}))
	scope, err := service.ResolveWorkspaceAccessScope(child.Id)
	require.NoError(t, err)
	require.Equal(t, []int{other.Id}, scope.WorkspaceIds)

	require.NoError(t, model.SetSubaccountWorkspaces(owner.Id, child.Id, []int{assigned.Id, other.Id}))
	scope, err = service.ResolveWorkspaceAccessScope(child.Id)
	require.NoError(t, err)
	require.ElementsMatch(t, []int{assigned.Id, other.Id}, scope.WorkspaceIds)

	require.NoError(t, model.SetSubaccountWorkspaces(owner.Id, child.Id, nil))
	scope, err = service.ResolveWorkspaceAccessScope(child.Id)
	require.NoError(t, err)
	require.Empty(t, scope.WorkspaceIds)
}

func TestSetSubaccountWorkspacesKeepsExistingDisabledAccessButRejectsNewAccess(t *testing.T) {
	db := setupWorkspaceAccessTestDB(t)
	owner, child, assigned, other := seedWorkspaceAccessAccounts(t, db)
	require.NoError(t, db.Model(&child).Update("status", common.UserStatusDisabled).Error)

	require.NoError(t, model.SetSubaccountWorkspaces(owner.Id, child.Id, []int{assigned.Id}))
	err := model.SetSubaccountWorkspaces(owner.Id, child.Id, []int{assigned.Id, other.Id})
	require.ErrorIs(t, err, model.ErrInvalidWorkspaceAccessAccount)

	var workspaceIds []int
	require.NoError(t, db.Model(&model.WorkspaceMember{}).
		Where("user_id = ?", child.Id).Pluck("workspace_id", &workspaceIds).Error)
	require.Equal(t, []int{assigned.Id}, workspaceIds)

	require.NoError(t, model.SetSubaccountWorkspaces(owner.Id, child.Id, nil))
	require.NoError(t, db.Model(&model.WorkspaceMember{}).
		Where("user_id = ?", child.Id).Pluck("workspace_id", &workspaceIds).Error)
	require.Empty(t, workspaceIds)
}

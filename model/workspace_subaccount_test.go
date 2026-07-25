package model

import (
	"errors"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func seedWorkspaceSubaccountLifecycle(t *testing.T) (User, User, User, Workspace, Token) {
	t.Helper()

	owner := User{
		Id: 9101, Username: "workspace-owner", Password: "owner-password",
		DisplayName: "Workspace Owner", Role: common.RoleCommonUser,
		Status: common.UserStatusEnabled, Group: "default", AffCode: "wo91",
	}
	otherOwner := User{
		Id: 9102, Username: "other-owner", Password: "other-password",
		DisplayName: "Other Owner", Role: common.RoleCommonUser,
		Status: common.UserStatusEnabled, Group: "default", AffCode: "oo91",
	}
	child := User{
		Id: 9103, Username: "workspace-child", Password: "old-password",
		DisplayName: "Workspace Child", Role: common.RoleCommonUser,
		Status: common.UserStatusEnabled, Group: "default", AffCode: "wc91",
		ParentUserId: owner.Id,
	}
	require.NoError(t, DB.Create(&[]User{owner, otherOwner, child}).Error)

	workspace := Workspace{
		Id: 9201, UserId: owner.Id, Name: "Shared Workspace",
		Status: WorkspaceStatusEnabled,
	}
	require.NoError(t, DB.Create(&workspace).Error)
	require.NoError(t, DB.Create(&WorkspaceMember{
		WorkspaceId: workspace.Id,
		UserId:      child.Id,
	}).Error)

	token := Token{
		Id: 9301, UserId: owner.Id, WorkspaceId: workspace.Id,
		Name: "shared-key", Key: "shared-key-value",
		Status: common.TokenStatusEnabled,
	}
	require.NoError(t, DB.Create(&token).Error)

	return owner, otherOwner, child, workspace, token
}

func TestWorkspaceSubaccountStatusAndPasswordManagement(t *testing.T) {
	truncateTables(t)
	owner, otherOwner, child, workspace, _ := seedWorkspaceSubaccountLifecycle(t)

	require.ErrorIs(t,
		SetWorkspaceSubaccountStatus(otherOwner.Id, child.Id, common.UserStatusDisabled),
		ErrWorkspaceSubaccountNotFound,
	)
	require.NoError(t,
		SetWorkspaceSubaccountStatus(owner.Id, child.Id, common.UserStatusDisabled),
	)

	var updated User
	require.NoError(t, DB.First(&updated, child.Id).Error)
	require.Equal(t, common.UserStatusDisabled, updated.Status)

	var membershipCount int64
	require.NoError(t, DB.Model(&WorkspaceMember{}).
		Where("workspace_id = ? AND user_id = ?", workspace.Id, child.Id).
		Count(&membershipCount).Error)
	require.Equal(t, int64(1), membershipCount)

	require.ErrorIs(t,
		ResetWorkspaceSubaccountPassword(otherOwner.Id, child.Id, "new-password"),
		ErrWorkspaceSubaccountNotFound,
	)
	require.NoError(t,
		ResetWorkspaceSubaccountPassword(owner.Id, child.Id, "new-password"),
	)

	require.NoError(t, DB.First(&updated, child.Id).Error)
	require.True(t, updated.MustChangePassword)
	require.True(t, common.ValidatePasswordAndHash("new-password", updated.Password))
	require.False(t, common.ValidatePasswordAndHash("old-password", updated.Password))
}

func TestDeleteWorkspaceSubaccountPreservesOwnerWorkspaceAndToken(t *testing.T) {
	truncateTables(t)
	owner, otherOwner, child, workspace, token := seedWorkspaceSubaccountLifecycle(t)

	require.ErrorIs(t,
		DeleteWorkspaceSubaccount(otherOwner.Id, child.Id),
		ErrWorkspaceSubaccountNotFound,
	)
	require.NoError(t, DeleteWorkspaceSubaccount(owner.Id, child.Id))

	var deleted User
	require.True(t, errors.Is(DB.First(&deleted, child.Id).Error, gorm.ErrRecordNotFound))
	require.NoError(t, DB.Unscoped().First(&deleted, child.Id).Error)
	require.True(t, deleted.DeletedAt.Valid)

	var membershipCount int64
	require.NoError(t, DB.Model(&WorkspaceMember{}).
		Where("user_id = ?", child.Id).
		Count(&membershipCount).Error)
	require.Zero(t, membershipCount)

	var preservedWorkspace Workspace
	require.NoError(t, DB.First(&preservedWorkspace, workspace.Id).Error)
	require.Equal(t, owner.Id, preservedWorkspace.UserId)

	var preservedToken Token
	require.NoError(t, DB.First(&preservedToken, token.Id).Error)
	require.Equal(t, owner.Id, preservedToken.UserId)
	require.Equal(t, workspace.Id, preservedToken.WorkspaceId)
}

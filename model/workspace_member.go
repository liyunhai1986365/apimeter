package model

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// WorkspaceMember grants a workspace account (User.ParentUserId > 0) access to a
// workspace owned by its parent. It is an access-control edge only: the workspace,
// its tokens, the quota and every log row still belong to Workspace.UserId.
type WorkspaceMember struct {
	WorkspaceId int   `json:"workspace_id" gorm:"primaryKey;autoIncrement:false"`
	UserId      int   `json:"user_id" gorm:"primaryKey;autoIncrement:false;index"`
	CreatedTime int64 `json:"created_time" gorm:"bigint"`
}

// WorkspaceAccessUser is the display projection of a member, returned with workspace lists.
type WorkspaceAccessUser struct {
	Id          int    `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
}

const workspaceScopeCacheTTL = 10 * time.Minute

func getWorkspaceScopeCacheKey(userId int) string {
	return fmt.Sprintf("workspace_scope:%d", userId)
}

// InvalidateWorkspaceScopeCache drops the cached workspace id list of a workspace
// account. Call it after any membership, workspace or account status change.
func InvalidateWorkspaceScopeCache(userId int) error {
	if !common.RedisEnabled || userId <= 0 {
		return nil
	}
	return common.RedisDelKey(getWorkspaceScopeCacheKey(userId))
}

func invalidateWorkspaceScopeCacheForWorkspace(workspaceId int) {
	if !common.RedisEnabled || workspaceId <= 0 {
		return
	}
	var userIds []int
	if err := DB.Model(&WorkspaceMember{}).
		Where("workspace_id = ?", workspaceId).
		Pluck("user_id", &userIds).Error; err != nil {
		return
	}
	for _, userId := range userIds {
		_ = InvalidateWorkspaceScopeCache(userId)
	}
}

func cacheGetAccessibleWorkspaceIds(userId int) ([]int, bool) {
	if !common.RedisEnabled {
		return nil, false
	}
	raw, err := common.RedisGet(getWorkspaceScopeCacheKey(userId))
	if err != nil {
		return nil, false
	}
	if raw == "" {
		return []int{}, true
	}
	parts := strings.Split(raw, ",")
	ids := make([]int, 0, len(parts))
	for _, part := range parts {
		id, convErr := strconv.Atoi(part)
		if convErr != nil {
			return nil, false
		}
		ids = append(ids, id)
	}
	return ids, true
}

func cacheSetAccessibleWorkspaceIds(userId int, ids []int) {
	if !common.RedisEnabled {
		return
	}
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		parts = append(parts, strconv.Itoa(id))
	}
	_ = common.RedisSet(getWorkspaceScopeCacheKey(userId), strings.Join(parts, ","), workspaceScopeCacheTTL)
}

// ListAccessibleWorkspaceIds returns the ids of workspaces owned by ownerUserId that
// accessUserId is a member of. Soft-deleted workspaces are excluded by the GORM scope.
func ListAccessibleWorkspaceIds(ownerUserId int, accessUserId int) ([]int, error) {
	if ownerUserId <= 0 || accessUserId <= 0 {
		return []int{}, nil
	}
	if ids, ok := cacheGetAccessibleWorkspaceIds(accessUserId); ok {
		return ids, nil
	}
	ids, err := listAccessibleWorkspaceIdsFromDB(DB, ownerUserId, accessUserId)
	if err != nil {
		return nil, err
	}
	cacheSetAccessibleWorkspaceIds(accessUserId, ids)
	return ids, nil
}

func listAccessibleWorkspaceIdsFromDB(tx *gorm.DB, ownerUserId int, accessUserId int) ([]int, error) {
	ids := make([]int, 0)
	err := tx.Model(&Workspace{}).
		Joins("JOIN workspace_members ON workspace_members.workspace_id = workspaces.id").
		Where("workspaces.user_id = ? AND workspace_members.user_id = ?", ownerUserId, accessUserId).
		Order("workspaces.id asc").
		Pluck("workspaces.id", &ids).Error
	return ids, err
}

// ListWorkspaceAccessUsers maps workspace id -> members, for the owner's workspace list.
func ListWorkspaceAccessUsers(workspaceIds []int) (map[int][]WorkspaceAccessUser, error) {
	result := map[int][]WorkspaceAccessUser{}
	if len(workspaceIds) == 0 {
		return result, nil
	}
	var members []WorkspaceMember
	if err := DB.Where("workspace_id IN ?", workspaceIds).
		Order("workspace_id asc, user_id asc").Find(&members).Error; err != nil {
		return nil, err
	}
	if len(members) == 0 {
		return result, nil
	}
	userIdSet := map[int]struct{}{}
	userIds := make([]int, 0, len(members))
	for _, member := range members {
		if _, ok := userIdSet[member.UserId]; ok {
			continue
		}
		userIdSet[member.UserId] = struct{}{}
		userIds = append(userIds, member.UserId)
	}
	var users []User
	if err := DB.Select("id", "username", "display_name").
		Where("id IN ?", userIds).Find(&users).Error; err != nil {
		return nil, err
	}
	usersById := make(map[int]WorkspaceAccessUser, len(users))
	for _, user := range users {
		usersById[user.Id] = WorkspaceAccessUser{
			Id: user.Id, Username: user.Username, DisplayName: user.DisplayName,
		}
	}
	for _, member := range members {
		user, ok := usersById[member.UserId]
		if !ok {
			continue
		}
		result[member.WorkspaceId] = append(result[member.WorkspaceId], user)
	}
	return result, nil
}

// replaceWorkspaceMembersTx sets the exact member list of a single workspace.
func replaceWorkspaceMembersTx(tx *gorm.DB, workspaceId int, userIds []int) error {
	var previous []int
	if err := tx.Model(&WorkspaceMember{}).
		Where("workspace_id = ?", workspaceId).Pluck("user_id", &previous).Error; err != nil {
		return err
	}
	if err := tx.Where("workspace_id = ?", workspaceId).Delete(&WorkspaceMember{}).Error; err != nil {
		return err
	}
	if err := addWorkspaceMembersTx(tx, workspaceId, userIds); err != nil {
		return err
	}
	for _, userId := range append(previous, userIds...) {
		_ = InvalidateWorkspaceScopeCache(userId)
	}
	return nil
}

func addWorkspaceMembersTx(tx *gorm.DB, workspaceId int, userIds []int) error {
	if len(userIds) == 0 {
		return nil
	}
	now := common.GetTimestamp()
	seen := map[int]struct{}{}
	members := make([]WorkspaceMember, 0, len(userIds))
	for _, userId := range userIds {
		if userId <= 0 {
			continue
		}
		if _, ok := seen[userId]; ok {
			continue
		}
		seen[userId] = struct{}{}
		members = append(members, WorkspaceMember{
			WorkspaceId: workspaceId, UserId: userId, CreatedTime: now,
		})
	}
	if len(members) == 0 {
		return nil
	}
	if err := tx.Create(&members).Error; err != nil {
		return err
	}
	for _, member := range members {
		_ = InvalidateWorkspaceScopeCache(member.UserId)
	}
	return nil
}

// addMemberToWorkspacesTx grants one account access to several workspaces at once.
func addMemberToWorkspacesTx(tx *gorm.DB, userId int, workspaceIds []int) error {
	if userId <= 0 || len(workspaceIds) == 0 {
		return nil
	}
	now := common.GetTimestamp()
	seen := map[int]struct{}{}
	members := make([]WorkspaceMember, 0, len(workspaceIds))
	for _, workspaceId := range workspaceIds {
		if workspaceId <= 0 {
			continue
		}
		if _, ok := seen[workspaceId]; ok {
			continue
		}
		seen[workspaceId] = struct{}{}
		members = append(members, WorkspaceMember{
			WorkspaceId: workspaceId, UserId: userId, CreatedTime: now,
		})
	}
	if len(members) == 0 {
		return nil
	}
	if err := tx.Create(&members).Error; err != nil {
		return err
	}
	return InvalidateWorkspaceScopeCache(userId)
}

func deleteWorkspaceMembersByUserTx(tx *gorm.DB, userId int) error {
	if userId <= 0 {
		return nil
	}
	if err := tx.Where("user_id = ?", userId).Delete(&WorkspaceMember{}).Error; err != nil {
		return err
	}
	return InvalidateWorkspaceScopeCache(userId)
}

func deleteWorkspaceMembersByWorkspaceTx(tx *gorm.DB, workspaceId int) error {
	if workspaceId <= 0 {
		return nil
	}
	var userIds []int
	if err := tx.Model(&WorkspaceMember{}).
		Where("workspace_id = ?", workspaceId).Pluck("user_id", &userIds).Error; err != nil {
		return err
	}
	if err := tx.Where("workspace_id = ?", workspaceId).Delete(&WorkspaceMember{}).Error; err != nil {
		return err
	}
	for _, userId := range userIds {
		_ = InvalidateWorkspaceScopeCache(userId)
	}
	return nil
}

// migrateWorkspaceMembers backfills workspace_members from the single-holder columns
// used by the earlier drafts (manager_user_id, then access_user_id). Both are read-only
// here; the legacy columns are left in place and are no longer consulted at runtime.
func migrateWorkspaceMembers() error {
	if DB == nil || !DB.Migrator().HasTable(&Workspace{}) || !DB.Migrator().HasTable(&WorkspaceMember{}) {
		return nil
	}
	legacyColumns := make([]string, 0, 2)
	for _, candidate := range []string{"access_user_id", "manager_user_id"} {
		if DB.Migrator().HasColumn(&Workspace{}, candidate) {
			legacyColumns = append(legacyColumns, candidate)
		}
	}
	if len(legacyColumns) == 0 {
		return nil
	}
	type legacyAssignment struct {
		Id           int
		AccessUserId int
	}
	now := common.GetTimestamp()
	backfilled := 0
	for _, legacyColumn := range legacyColumns {
		var assignments []legacyAssignment
		if err := DB.Table("workspaces").
			Select(fmt.Sprintf("id, %s as access_user_id", legacyColumn)).
			Where(fmt.Sprintf("%s > ?", legacyColumn), 0).
			Scan(&assignments).Error; err != nil {
			return fmt.Errorf("failed to read legacy workspace assignments: %w", err)
		}
		for _, assignment := range assignments {
			var count int64
			if err := DB.Model(&WorkspaceMember{}).
				Where("workspace_id = ? AND user_id = ?", assignment.Id, assignment.AccessUserId).
				Count(&count).Error; err != nil {
				return fmt.Errorf("failed to check workspace member: %w", err)
			}
			if count > 0 {
				continue
			}
			member := WorkspaceMember{
				WorkspaceId: assignment.Id, UserId: assignment.AccessUserId, CreatedTime: now,
			}
			if err := DB.Create(&member).Error; err != nil {
				return fmt.Errorf("failed to backfill workspace member: %w", err)
			}
			backfilled++
		}
	}
	if backfilled > 0 {
		common.SysLog(fmt.Sprintf("backfilled %d workspace member(s) from legacy workspace columns", backfilled))
	}
	return nil
}

var ErrWorkspaceMemberNotAllowed = errors.New("workspace access can only be granted to an enabled child account")

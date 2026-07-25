package model

import (
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"gorm.io/gorm"
)

var (
	ErrNotWorkspaceAccountOwner      = errors.New("only a main account can manage workspace accounts")
	ErrWorkspaceSubaccountNotFound   = errors.New("workspace account not found")
	ErrInvalidWorkspaceAccessAccount = errors.New("workspace access can only be granted to an enabled child account")
	ErrDefaultWorkspaceAccess        = errors.New("the default workspace cannot be accessed by a child account")
)

type WorkspaceSubaccountSummary struct {
	Id                 int    `json:"id"`
	Username           string `json:"username"`
	DisplayName        string `json:"display_name"`
	Email              string `json:"email"`
	Status             int    `json:"status"`
	MustChangePassword bool   `json:"must_change_password"`
	CreatedAt          int64  `json:"created_at"`
	LastLoginAt        int64  `json:"last_login_at"`
	WorkspaceCount     int64  `json:"workspace_count"`
	TokenCount         int64  `json:"token_count"`
	LastUsedAt         int64  `json:"last_used_at"`
}

func ensureWorkspaceAccountOwner(tx *gorm.DB, ownerUserId int) (*User, error) {
	if ownerUserId <= 0 {
		return nil, ErrNotWorkspaceAccountOwner
	}
	var owner User
	if err := tx.First(&owner, "id = ?", ownerUserId).Error; err != nil {
		return nil, err
	}
	if owner.ParentUserId != 0 || owner.Status != common.UserStatusEnabled {
		return nil, ErrNotWorkspaceAccountOwner
	}
	return &owner, nil
}

func GetWorkspaceSubaccount(ownerUserId int, subaccountId int) (*User, error) {
	var user User
	err := DB.Omit("password", "access_token").
		Where("id = ? AND parent_user_id = ?", subaccountId, ownerUserId).
		First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrWorkspaceSubaccountNotFound
	}
	return &user, err
}

// ListWorkspaceSubaccounts returns every child account of ownerUserId with its
// aggregate counters. Counters are resolved in two GROUP BY queries rather than
// per-account lookups.
func ListWorkspaceSubaccounts(ownerUserId int) ([]WorkspaceSubaccountSummary, error) {
	if _, err := ensureWorkspaceAccountOwner(DB, ownerUserId); err != nil {
		return nil, err
	}
	var users []User
	if err := DB.Omit("password", "access_token").
		Where("parent_user_id = ?", ownerUserId).
		Order("id desc").Find(&users).Error; err != nil {
		return nil, err
	}
	if len(users) == 0 {
		return []WorkspaceSubaccountSummary{}, nil
	}

	var workspaceRows []struct {
		UserId         int
		WorkspaceCount int64
	}
	if err := DB.Model(&Workspace{}).
		Select("workspace_members.user_id as user_id, count(workspaces.id) as workspace_count").
		Joins("JOIN workspace_members ON workspace_members.workspace_id = workspaces.id").
		Where("workspaces.user_id = ?", ownerUserId).
		Group("workspace_members.user_id").
		Scan(&workspaceRows).Error; err != nil {
		return nil, err
	}
	workspaceCounts := make(map[int]int64, len(workspaceRows))
	for _, row := range workspaceRows {
		workspaceCounts[row.UserId] = row.WorkspaceCount
	}

	var tokenRows []struct {
		UserId     int
		TokenCount int64
		LastUsedAt int64
	}
	if err := DB.Model(&Token{}).
		Select("workspace_members.user_id as user_id, count(tokens.id) as token_count, COALESCE(MAX(tokens.accessed_time), 0) as last_used_at").
		Joins("JOIN workspace_members ON workspace_members.workspace_id = tokens.workspace_id").
		Joins("JOIN workspaces ON workspaces.id = workspace_members.workspace_id AND workspaces.deleted_at IS NULL").
		Where("tokens.user_id = ? AND workspaces.user_id = ? AND (tokens.billing_source IS NULL OR tokens.billing_source <> ?)",
			ownerUserId, ownerUserId, "subscription").
		Group("workspace_members.user_id").
		Scan(&tokenRows).Error; err != nil {
		return nil, err
	}
	tokenCounts := make(map[int]int64, len(tokenRows))
	lastUsedAt := make(map[int]int64, len(tokenRows))
	for _, row := range tokenRows {
		tokenCounts[row.UserId] = row.TokenCount
		lastUsedAt[row.UserId] = row.LastUsedAt
	}

	result := make([]WorkspaceSubaccountSummary, 0, len(users))
	for _, user := range users {
		result = append(result, WorkspaceSubaccountSummary{
			Id: user.Id, Username: user.Username, DisplayName: user.DisplayName,
			Email: user.Email, Status: user.Status, MustChangePassword: user.MustChangePassword,
			CreatedAt: user.CreatedAt, LastLoginAt: user.LastLoginAt,
			WorkspaceCount: workspaceCounts[user.Id],
			TokenCount:     tokenCounts[user.Id],
			LastUsedAt:     lastUsedAt[user.Id],
		})
	}
	return result, nil
}

func CreateWorkspaceSubaccount(ownerUserId int, user *User, workspaceIds []int) error {
	if user == nil {
		return errors.New("workspace account is required")
	}
	user.Username = strings.TrimSpace(user.Username)
	user.DisplayName = strings.TrimSpace(user.DisplayName)
	user.Email = NormalizeEmail(user.Email)
	if user.Username == "" || len([]rune(user.Username)) > UserNameMaxLength {
		return errors.New("invalid username")
	}
	if len(user.Password) < 8 || len(user.Password) > 20 {
		return errors.New("password must be between 8 and 20 characters")
	}
	if user.DisplayName == "" {
		user.DisplayName = user.Username
	}
	if len([]rune(user.DisplayName)) > UserNameMaxLength {
		return errors.New("display name is too long")
	}

	return DB.Transaction(func(tx *gorm.DB) error {
		owner, err := ensureWorkspaceAccountOwner(tx, ownerUserId)
		if err != nil {
			return err
		}
		var count int64
		// Unscoped: usernames are globally unique and a soft-deleted account keeps its name.
		if err := tx.Unscoped().Model(&User{}).Where("username = ?", user.Username).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return errors.New("用户名 \"" + user.Username + "\" 已被占用（用户名在全站唯一，且已删除账号的用户名不会释放），请换一个，例如加上团队前缀")
		}
		return withNormalizedEmailLock(tx, user.Email, func(tx *gorm.DB) error {
			if err := user.prepareForInsert(tx); err != nil {
				return err
			}
			user.Role = common.RoleCommonUser
			user.Status = common.UserStatusEnabled
			user.ParentUserId = ownerUserId
			user.MustChangePassword = true
			user.Quota = 0
			user.UsedQuota = 0
			user.RequestCount = 0
			user.Group = owner.Group
			user.AffCode = common.GetRandomString(4)
			if user.Setting == "" {
				user.SetSetting(dto.UserSetting{})
			}
			if err := tx.Create(user).Error; err != nil {
				return err
			}
			if len(workspaceIds) == 0 {
				return nil
			}
			if err := assertAssignableWorkspacesTx(tx, ownerUserId, workspaceIds); err != nil {
				return err
			}
			return addMemberToWorkspacesTx(tx, user.Id, workspaceIds)
		})
	})
}

func UpdateWorkspaceSubaccount(ownerUserId int, subaccountId int, displayName string, email string) (*User, error) {
	displayName = strings.TrimSpace(displayName)
	email = NormalizeEmail(email)
	if displayName == "" || len([]rune(displayName)) > UserNameMaxLength {
		return nil, errors.New("invalid display name")
	}
	var updated User
	err := DB.Transaction(func(tx *gorm.DB) error {
		if _, err := ensureWorkspaceAccountOwner(tx, ownerUserId); err != nil {
			return err
		}
		if err := tx.Where("id = ? AND parent_user_id = ?", subaccountId, ownerUserId).First(&updated).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrWorkspaceSubaccountNotFound
			}
			return err
		}
		return withNormalizedEmailLock(tx, email, func(tx *gorm.DB) error {
			if err := ensureEmailAvailableWithTx(tx, email, subaccountId); err != nil {
				return err
			}
			updated.DisplayName = displayName
			updated.Email = email
			return tx.Model(&updated).Select("display_name", "email").Updates(&updated).Error
		})
	})
	if err != nil {
		return nil, err
	}
	_ = invalidateUserCache(subaccountId)
	updated.Password = ""
	updated.AccessToken = nil
	return &updated, nil
}

func SetWorkspaceSubaccountStatus(ownerUserId int, subaccountId int, status int) error {
	if status != common.UserStatusEnabled && status != common.UserStatusDisabled {
		return errors.New("invalid user status")
	}
	err := DB.Transaction(func(tx *gorm.DB) error {
		if _, err := ensureWorkspaceAccountOwner(tx, ownerUserId); err != nil {
			return err
		}
		var child User
		if err := tx.Where("id = ? AND parent_user_id = ?", subaccountId, ownerUserId).First(&child).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrWorkspaceSubaccountNotFound
			}
			return err
		}
		return tx.Model(&child).Update("status", status).Error
	})
	if err != nil {
		return err
	}
	_ = invalidateUserCache(subaccountId)
	_ = InvalidateWorkspaceScopeCache(subaccountId)
	return nil
}

func ResetWorkspaceSubaccountPassword(ownerUserId int, subaccountId int, password string) error {
	if len(password) < 8 || len(password) > 20 {
		return errors.New("password must be between 8 and 20 characters")
	}
	hashedPassword, err := common.Password2Hash(password)
	if err != nil {
		return err
	}
	result := DB.Model(&User{}).
		Where("id = ? AND parent_user_id = ?", subaccountId, ownerUserId).
		Updates(map[string]interface{}{"password": hashedPassword, "must_change_password": true})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrWorkspaceSubaccountNotFound
	}
	return invalidateUserCache(subaccountId)
}

func DeleteWorkspaceSubaccount(ownerUserId int, subaccountId int) error {
	err := DB.Transaction(func(tx *gorm.DB) error {
		if _, err := ensureWorkspaceAccountOwner(tx, ownerUserId); err != nil {
			return err
		}
		var child User
		if err := tx.Where("id = ? AND parent_user_id = ?", subaccountId, ownerUserId).First(&child).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrWorkspaceSubaccountNotFound
			}
			return err
		}
		if err := deleteWorkspaceMembersByUserTx(tx, subaccountId); err != nil {
			return err
		}
		return tx.Delete(&child).Error
	})
	if err != nil {
		return err
	}
	_ = invalidateUserCache(subaccountId)
	return nil
}

// assertAssignableWorkspacesTx verifies every id is a non-default workspace of the owner.
func assertAssignableWorkspacesTx(tx *gorm.DB, ownerUserId int, workspaceIds []int) error {
	if len(workspaceIds) == 0 {
		return nil
	}
	var workspaces []Workspace
	if err := tx.Where("id IN ? AND user_id = ?", workspaceIds, ownerUserId).Find(&workspaces).Error; err != nil {
		return err
	}
	if len(workspaces) != len(workspaceIds) {
		return errors.New("one or more workspaces are unavailable")
	}
	for _, workspace := range workspaces {
		if workspace.IsDefault {
			return ErrDefaultWorkspaceAccess
		}
	}
	return nil
}

// SetWorkspaceAccess replaces the member list of a workspace. An empty accessUserIds
// revokes all access. New members must be enabled; an existing disabled member may be
// retained or removed while the owner edits the rest of the member list.
func SetWorkspaceAccess(ownerUserId int, workspaceId int, accessUserIds []int) (*Workspace, error) {
	var workspace Workspace
	err := DB.Transaction(func(tx *gorm.DB) error {
		if _, err := ensureWorkspaceAccountOwner(tx, ownerUserId); err != nil {
			return err
		}
		if err := tx.Where("id = ? AND user_id = ?", workspaceId, ownerUserId).First(&workspace).Error; err != nil {
			return err
		}
		if workspace.IsDefault && len(accessUserIds) > 0 {
			return ErrDefaultWorkspaceAccess
		}
		var currentUserIds []int
		if err := tx.Model(&WorkspaceMember{}).
			Where("workspace_id = ?", workspaceId).
			Pluck("user_id", &currentUserIds).Error; err != nil {
			return err
		}
		currentMembers := make(map[int]struct{}, len(currentUserIds))
		for _, userId := range currentUserIds {
			currentMembers[userId] = struct{}{}
		}
		for _, accessUserId := range accessUserIds {
			var child User
			if err := tx.Where("id = ? AND parent_user_id = ?",
				accessUserId, ownerUserId).First(&child).Error; err != nil {
				return ErrInvalidWorkspaceAccessAccount
			}
			if child.Status != common.UserStatusEnabled {
				if _, alreadyAssigned := currentMembers[accessUserId]; !alreadyAssigned {
					return ErrInvalidWorkspaceAccessAccount
				}
			}
		}
		if err := replaceWorkspaceMembersTx(tx, workspaceId, accessUserIds); err != nil {
			return err
		}
		workspace.UpdatedTime = common.GetTimestamp()
		return tx.Model(&workspace).Select("updated_time").Updates(&workspace).Error
	})
	if err != nil {
		return nil, err
	}
	accessUsers, err := ListWorkspaceAccessUsers([]int{workspaceId})
	if err != nil {
		return nil, err
	}
	workspace.AccessUsers = accessUsers[workspaceId]
	if workspace.AccessUsers == nil {
		workspace.AccessUsers = []WorkspaceAccessUser{}
	}
	return &workspace, nil
}

// SetSubaccountWorkspaces replaces the full workspace set a single child account can reach.
func SetSubaccountWorkspaces(ownerUserId int, subaccountId int, workspaceIds []int) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		if _, err := ensureWorkspaceAccountOwner(tx, ownerUserId); err != nil {
			return err
		}
		var child User
		if err := tx.Where("id = ? AND parent_user_id = ?", subaccountId, ownerUserId).First(&child).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrWorkspaceSubaccountNotFound
			}
			return err
		}
		var currentWorkspaceIds []int
		if err := tx.Model(&WorkspaceMember{}).
			Where("user_id = ?", subaccountId).
			Pluck("workspace_id", &currentWorkspaceIds).Error; err != nil {
			return err
		}
		if child.Status != common.UserStatusEnabled {
			currentWorkspaces := make(map[int]struct{}, len(currentWorkspaceIds))
			for _, workspaceId := range currentWorkspaceIds {
				currentWorkspaces[workspaceId] = struct{}{}
			}
			for _, workspaceId := range workspaceIds {
				if _, alreadyAssigned := currentWorkspaces[workspaceId]; !alreadyAssigned {
					return ErrInvalidWorkspaceAccessAccount
				}
			}
		}
		if err := assertAssignableWorkspacesTx(tx, ownerUserId, workspaceIds); err != nil {
			return err
		}
		if err := deleteWorkspaceMembersByUserTx(tx, subaccountId); err != nil {
			return err
		}
		return addMemberToWorkspacesTx(tx, subaccountId, workspaceIds)
	})
}

package model

import (
	"fmt"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// legacyWorkspaceAssignment reproduces the single-holder shape used by the earlier drafts:
// first `manager_user_id`, then `access_user_id`. Both are backfilled into workspace_members.
type legacyWorkspaceAssignment struct {
	Id            int `gorm:"primaryKey"`
	UserId        int `gorm:"column:user_id;default:0"`
	ManagerUserId int `gorm:"column:manager_user_id;default:0"`
}

func (legacyWorkspaceAssignment) TableName() string {
	return "workspaces"
}

type legacyWorkspaceAccessAssignment struct {
	Id           int `gorm:"primaryKey"`
	UserId       int `gorm:"column:user_id;default:0"`
	AccessUserId int `gorm:"column:access_user_id;default:0"`
}

type legacyWorkspaceBothAssignments struct {
	Id            int `gorm:"primaryKey"`
	UserId        int `gorm:"column:user_id;default:0"`
	ManagerUserId int `gorm:"column:manager_user_id;default:0"`
	AccessUserId  int `gorm:"column:access_user_id;default:0"`
}

func (legacyWorkspaceBothAssignments) TableName() string {
	return "workspaces"
}

func (legacyWorkspaceAccessAssignment) TableName() string {
	return "workspaces"
}

func setupLegacyWorkspaceDB(t *testing.T, legacyModel any) *gorm.DB {
	t.Helper()
	originalDB := DB
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	require.NoError(t, err)
	DB = db
	t.Cleanup(func() { DB = originalDB })
	require.NoError(t, db.AutoMigrate(legacyModel))
	require.NoError(t, db.AutoMigrate(&WorkspaceMember{}))
	return db
}

func TestMigrateWorkspaceMembersBackfillsLegacyManagerColumn(t *testing.T) {
	db := setupLegacyWorkspaceDB(t, &legacyWorkspaceAssignment{})
	assigned := legacyWorkspaceAssignment{UserId: 7, ManagerUserId: 42}
	unassigned := legacyWorkspaceAssignment{UserId: 7}
	require.NoError(t, db.Create(&assigned).Error)
	require.NoError(t, db.Create(&unassigned).Error)

	require.NoError(t, migrateWorkspaceMembers())

	var members []WorkspaceMember
	require.NoError(t, db.Find(&members).Error)
	require.Len(t, members, 1)
	require.Equal(t, assigned.Id, members[0].WorkspaceId)
	require.Equal(t, 42, members[0].UserId)
}

func TestMigrateWorkspaceMembersBackfillsLegacyAccessColumn(t *testing.T) {
	db := setupLegacyWorkspaceDB(t, &legacyWorkspaceAccessAssignment{})
	assigned := legacyWorkspaceAccessAssignment{UserId: 7, AccessUserId: 43}
	require.NoError(t, db.Create(&assigned).Error)

	require.NoError(t, migrateWorkspaceMembers())

	var members []WorkspaceMember
	require.NoError(t, db.Find(&members).Error)
	require.Len(t, members, 1)
	require.Equal(t, assigned.Id, members[0].WorkspaceId)
	require.Equal(t, 43, members[0].UserId)
}

// The migration runs on every boot, so a second pass must not duplicate the edge — the
// composite primary key would reject the insert and fail startup.
func TestMigrateWorkspaceMembersIsIdempotent(t *testing.T) {
	db := setupLegacyWorkspaceDB(t, &legacyWorkspaceAccessAssignment{})
	require.NoError(t, db.Create(&legacyWorkspaceAccessAssignment{UserId: 7, AccessUserId: 43}).Error)

	require.NoError(t, migrateWorkspaceMembers())
	require.NoError(t, migrateWorkspaceMembers())

	var count int64
	require.NoError(t, db.Model(&WorkspaceMember{}).Count(&count).Error)
	require.Equal(t, int64(1), count)
}

func TestMigrateWorkspaceMembersBackfillsBothLegacyColumns(t *testing.T) {
	db := setupLegacyWorkspaceDB(t, &legacyWorkspaceBothAssignments{})
	managerAssigned := legacyWorkspaceBothAssignments{UserId: 7, ManagerUserId: 42}
	accessAssigned := legacyWorkspaceBothAssignments{UserId: 7, AccessUserId: 43}
	require.NoError(t, db.Create(&managerAssigned).Error)
	require.NoError(t, db.Create(&accessAssigned).Error)

	require.NoError(t, migrateWorkspaceMembers())

	var members []WorkspaceMember
	require.NoError(t, db.Order("workspace_id asc").Find(&members).Error)
	require.Len(t, members, 2)
	require.Equal(t, 42, members[0].UserId)
	require.Equal(t, 43, members[1].UserId)
}

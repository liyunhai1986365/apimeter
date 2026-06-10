package model

import (
	"errors"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

const (
	WorkspaceStatusEnabled  = 1
	WorkspaceStatusDisabled = 2

	DefaultWorkspaceName = "Default"
)

type Workspace struct {
	Id          int            `json:"id"`
	UserId      int            `json:"user_id" gorm:"index"`
	Name        string         `json:"name" gorm:"type:varchar(64);index"`
	Description string         `json:"description" gorm:"type:varchar(255);default:''"`
	IsDefault   bool           `json:"is_default" gorm:"index;default:false"`
	Status      int            `json:"status" gorm:"default:1"`
	CreatedTime int64          `json:"created_time" gorm:"bigint"`
	UpdatedTime int64          `json:"updated_time" gorm:"bigint"`
	DeletedAt   gorm.DeletedAt `gorm:"index"`
}

type WorkspaceWithTokenCount struct {
	Workspace
	TokenCount int64 `json:"token_count"`
}

type WorkspaceUsageStats struct {
	WorkspaceId    int `json:"workspace_id"`
	TodayQuota     int `json:"today_quota"`
	WeekQuota      int `json:"week_quota"`
	MonthQuota     int `json:"month_quota"`
	TotalUsedQuota int `json:"total_used_quota"`
}

func EnsureDefaultWorkspace(userId int) (*Workspace, error) {
	if userId <= 0 {
		return nil, errors.New("userId 无效")
	}
	var workspace Workspace
	err := DB.Where("user_id = ? AND is_default = ?", userId, true).First(&workspace).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		now := common.GetTimestamp()
		workspace = Workspace{
			UserId:      userId,
			Name:        DefaultWorkspaceName,
			Description: "",
			IsDefault:   true,
			Status:      WorkspaceStatusEnabled,
			CreatedTime: now,
			UpdatedTime: now,
		}
		if err := DB.Create(&workspace).Error; err != nil {
			return nil, err
		}
	}
	if err := BackfillUserTokensWorkspace(userId, workspace.Id); err != nil {
		return nil, err
	}
	return &workspace, nil
}

func BackfillUserTokensWorkspace(userId int, workspaceId int) error {
	if userId <= 0 || workspaceId <= 0 {
		return errors.New("userId 或 workspaceId 无效")
	}
	return DB.Model(&Token{}).
		Where("user_id = ? AND workspace_id = ?", userId, 0).
		Update("workspace_id", workspaceId).Error
}

func GetUserWorkspaceByID(userId int, workspaceId int) (*Workspace, error) {
	if userId <= 0 || workspaceId <= 0 {
		return nil, errors.New("userId 或 workspaceId 无效")
	}
	if _, err := EnsureDefaultWorkspace(userId); err != nil {
		return nil, err
	}
	var workspace Workspace
	err := DB.Where("id = ? AND user_id = ?", workspaceId, userId).First(&workspace).Error
	return &workspace, err
}

func ResolveUserWorkspace(userId int, workspaceId int) (*Workspace, error) {
	if workspaceId > 0 {
		return GetUserWorkspaceByID(userId, workspaceId)
	}
	return EnsureDefaultWorkspace(userId)
}

func ListUserWorkspaces(userId int) ([]WorkspaceWithTokenCount, error) {
	if userId <= 0 {
		return nil, errors.New("userId 无效")
	}
	if _, err := EnsureDefaultWorkspace(userId); err != nil {
		return nil, err
	}
	var workspaces []Workspace
	if err := DB.Where("user_id = ?", userId).Order("is_default desc, id asc").Find(&workspaces).Error; err != nil {
		return nil, err
	}
	result := make([]WorkspaceWithTokenCount, 0, len(workspaces))
	for _, workspace := range workspaces {
		var count int64
		if err := DB.Model(&Token{}).
			Where("user_id = ? AND workspace_id = ? AND (billing_source IS NULL OR billing_source <> ?)", userId, workspace.Id, "subscription").
			Count(&count).Error; err != nil {
			return nil, err
		}
		result = append(result, WorkspaceWithTokenCount{
			Workspace:  workspace,
			TokenCount: count,
		})
	}
	return result, nil
}

func CreateUserWorkspace(userId int, name string, description string) (*Workspace, error) {
	if userId <= 0 {
		return nil, errors.New("userId 无效")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("工作区名称不能为空")
	}
	if len([]rune(name)) > 64 {
		return nil, errors.New("工作区名称不能超过 64 个字符")
	}
	description = strings.TrimSpace(description)
	if len([]rune(description)) > 255 {
		return nil, errors.New("工作区描述不能超过 255 个字符")
	}
	if _, err := EnsureDefaultWorkspace(userId); err != nil {
		return nil, err
	}
	now := common.GetTimestamp()
	workspace := &Workspace{
		UserId:      userId,
		Name:        name,
		Description: description,
		IsDefault:   false,
		Status:      WorkspaceStatusEnabled,
		CreatedTime: now,
		UpdatedTime: now,
	}
	return workspace, DB.Create(workspace).Error
}

func UpdateUserWorkspace(userId int, workspaceId int, name string, description string) (*Workspace, error) {
	workspace, err := GetUserWorkspaceByID(userId, workspaceId)
	if err != nil {
		return nil, err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("工作区名称不能为空")
	}
	if len([]rune(name)) > 64 {
		return nil, errors.New("工作区名称不能超过 64 个字符")
	}
	description = strings.TrimSpace(description)
	if len([]rune(description)) > 255 {
		return nil, errors.New("工作区描述不能超过 255 个字符")
	}
	workspace.Name = name
	workspace.Description = description
	workspace.UpdatedTime = common.GetTimestamp()
	return workspace, DB.Model(workspace).Select("name", "description", "updated_time").Updates(workspace).Error
}

func DeleteUserWorkspace(userId int, workspaceId int) error {
	workspace, err := GetUserWorkspaceByID(userId, workspaceId)
	if err != nil {
		return err
	}
	if workspace.IsDefault {
		return errors.New("默认工作区不能删除")
	}
	var count int64
	if err := DB.Model(&Token{}).Where("user_id = ? AND workspace_id = ?", userId, workspaceId).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return errors.New("请先删除或移动该工作区下的令牌")
	}
	return DB.Delete(workspace).Error
}

func GetUserWorkspaceUsageStats(userId int, workspaceId int, now time.Time) (WorkspaceUsageStats, error) {
	stats := WorkspaceUsageStats{WorkspaceId: workspaceId}
	if userId <= 0 || workspaceId <= 0 {
		return stats, errors.New("userId 或 workspaceId 无效")
	}
	workspace, err := GetUserWorkspaceByID(userId, workspaceId)
	if err != nil {
		return stats, err
	}
	stats.WorkspaceId = workspace.Id
	if now.IsZero() {
		now = time.Now()
	}

	var tokenIDs []int
	tokenQuery := func() *gorm.DB {
		return DB.Model(&Token{}).
			Where("user_id = ? AND workspace_id = ? AND (billing_source IS NULL OR billing_source <> ?)", userId, workspace.Id, "subscription")
	}
	if err := tokenQuery().Pluck("id", &tokenIDs).Error; err != nil {
		return stats, err
	}
	if len(tokenIDs) == 0 {
		return stats, nil
	}

	var total struct {
		TotalUsedQuota int `gorm:"column:total_used_quota"`
	}
	if err := tokenQuery().Select("COALESCE(SUM(used_quota), 0) AS total_used_quota").Scan(&total).Error; err != nil {
		return stats, err
	}
	stats.TotalUsedQuota = total.TotalUsedQuota

	startOfToday := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	weekday := int(startOfToday.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	startOfWeek := startOfToday.AddDate(0, 0, 1-weekday)
	startOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())

	var period struct {
		TodayQuota int `gorm:"column:today_quota"`
		WeekQuota  int `gorm:"column:week_quota"`
		MonthQuota int `gorm:"column:month_quota"`
	}
	if err := LOG_DB.Model(&Log{}).
		Select(
			`COALESCE(SUM(CASE WHEN created_at >= ? THEN quota ELSE 0 END), 0) AS today_quota,
COALESCE(SUM(CASE WHEN created_at >= ? THEN quota ELSE 0 END), 0) AS week_quota,
COALESCE(SUM(CASE WHEN created_at >= ? THEN quota ELSE 0 END), 0) AS month_quota`,
			startOfToday.Unix(),
			startOfWeek.Unix(),
			startOfMonth.Unix(),
		).
		Where("user_id = ? AND token_id IN ? AND type = ? AND created_at <= ?", userId, tokenIDs, LogTypeConsume, now.Unix()).
		Scan(&period).Error; err != nil {
		return stats, err
	}
	stats.TodayQuota = period.TodayQuota
	stats.WeekQuota = period.WeekQuota
	stats.MonthQuota = period.MonthQuota

	return stats, nil
}

func AttachWorkspaceNames(tokens []*Token) error {
	if len(tokens) == 0 {
		return nil
	}
	ids := make([]int, 0, len(tokens))
	for _, token := range tokens {
		if token != nil && token.WorkspaceId > 0 {
			ids = append(ids, token.WorkspaceId)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	var workspaces []Workspace
	if err := DB.Select("id", "name").Where("id IN ?", ids).Find(&workspaces).Error; err != nil {
		return err
	}
	names := make(map[int]string, len(workspaces))
	for _, workspace := range workspaces {
		names[workspace.Id] = workspace.Name
	}
	for _, token := range tokens {
		if token != nil {
			token.WorkspaceName = names[token.WorkspaceId]
		}
	}
	return nil
}

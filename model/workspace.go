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

	WorkspaceQuotaResetPeriodDaily   = "daily"
	WorkspaceQuotaResetPeriodWeekly  = "weekly"
	WorkspaceQuotaResetPeriodMonthly = "monthly"
)

type Workspace struct {
	Id                      int            `json:"id"`
	UserId                  int            `json:"user_id" gorm:"index"`
	Name                    string         `json:"name" gorm:"type:varchar(64);index"`
	Description             string         `json:"description" gorm:"type:varchar(255);default:''"`
	IsDefault               bool           `json:"is_default" gorm:"index;default:false"`
	Status                  int            `json:"status" gorm:"default:1"`
	QuotaResetEnabled       bool           `json:"quota_reset_enabled" gorm:"default:false"`
	QuotaResetPeriod        string         `json:"quota_reset_period" gorm:"type:varchar(16);default:''"`
	QuotaResetAmount        int            `json:"quota_reset_amount" gorm:"default:0"`
	QuotaResetLastAppliedAt int64          `json:"quota_reset_last_applied_at" gorm:"bigint;default:0"`
	QuotaResetNextAt        int64          `json:"quota_reset_next_at" gorm:"bigint;default:0;index"`
	CreatedTime             int64          `json:"created_time" gorm:"bigint"`
	UpdatedTime             int64          `json:"updated_time" gorm:"bigint"`
	DeletedAt               gorm.DeletedAt `gorm:"index"`
}

type WorkspaceWithTokenCount struct {
	Workspace
	TokenCount int64 `json:"token_count"`
}

type WorkspaceFilterOption struct {
	Id        int    `json:"id"`
	Name      string `json:"name"`
	IsDefault bool   `json:"is_default"`
}

type WorkspaceUsageStats struct {
	WorkspaceId    int `json:"workspace_id"`
	TodayQuota     int `json:"today_quota"`
	WeekQuota      int `json:"week_quota"`
	MonthQuota     int `json:"month_quota"`
	TotalUsedQuota int `json:"total_used_quota"`
}

type WorkspaceQuotaResetConfig struct {
	WorkspaceId   int    `json:"workspace_id"`
	Enabled       bool   `json:"enabled"`
	Period        string `json:"period"`
	Amount        int    `json:"amount"`
	LastAppliedAt int64  `json:"last_applied_at"`
	NextAt        int64  `json:"next_at"`
}

type WorkspaceQuotaResetConfigRequest struct {
	Enabled bool   `json:"enabled"`
	Period  string `json:"period"`
	Amount  int    `json:"amount"`
}

func (workspace Workspace) QuotaResetConfig() WorkspaceQuotaResetConfig {
	return WorkspaceQuotaResetConfig{
		WorkspaceId:   workspace.Id,
		Enabled:       workspace.QuotaResetEnabled,
		Period:        workspace.QuotaResetPeriod,
		Amount:        workspace.QuotaResetAmount,
		LastAppliedAt: workspace.QuotaResetLastAppliedAt,
		NextAt:        workspace.QuotaResetNextAt,
	}
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

func ListUserWorkspaceFilterOptions(userId int) ([]WorkspaceFilterOption, error) {
	if userId <= 0 {
		return nil, errors.New("userId 无效")
	}
	if _, err := EnsureDefaultWorkspace(userId); err != nil {
		return nil, err
	}
	var options []WorkspaceFilterOption
	err := DB.Model(&Workspace{}).
		Select("id", "name", "is_default").
		Where("user_id = ?", userId).
		Order("is_default desc, id asc").
		Scan(&options).Error
	return options, err
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

func GetUserWorkspaceQuotaResetConfig(userId int, workspaceId int) (WorkspaceQuotaResetConfig, error) {
	workspace, err := GetUserWorkspaceByID(userId, workspaceId)
	if err != nil {
		return WorkspaceQuotaResetConfig{}, err
	}
	return workspace.QuotaResetConfig(), nil
}

func UpdateUserWorkspaceQuotaResetConfig(userId int, workspaceId int, req WorkspaceQuotaResetConfigRequest, now time.Time) (WorkspaceQuotaResetConfig, error) {
	workspace, err := GetUserWorkspaceByID(userId, workspaceId)
	if err != nil {
		return WorkspaceQuotaResetConfig{}, err
	}
	if now.IsZero() {
		now = time.Now()
	}

	period := strings.TrimSpace(req.Period)
	if period == "" {
		period = WorkspaceQuotaResetPeriodDaily
	}
	if !isValidWorkspaceQuotaResetPeriod(period) {
		return WorkspaceQuotaResetConfig{}, errors.New("工作区配额周期无效")
	}
	if req.Amount < 0 {
		return WorkspaceQuotaResetConfig{}, errors.New("工作区配额不能为负数")
	}
	if req.Enabled && req.Amount <= 0 {
		return WorkspaceQuotaResetConfig{}, errors.New("启用工作区配额时额度必须大于 0")
	}

	shouldApplyNow := req.Enabled && (!workspace.QuotaResetEnabled || workspace.QuotaResetLastAppliedAt == 0)
	nowUnix := now.Unix()
	workspace.QuotaResetEnabled = req.Enabled
	workspace.QuotaResetPeriod = period
	workspace.QuotaResetAmount = req.Amount
	workspace.UpdatedTime = common.GetTimestamp()
	if req.Enabled {
		workspace.QuotaResetNextAt = NextWorkspaceQuotaResetAt(period, now).Unix()
		if shouldApplyNow {
			workspace.QuotaResetLastAppliedAt = nowUnix
		}
	} else {
		workspace.QuotaResetNextAt = 0
	}

	selectColumns := []string{
		"quota_reset_enabled",
		"quota_reset_period",
		"quota_reset_amount",
		"quota_reset_next_at",
		"updated_time",
	}
	if shouldApplyNow {
		selectColumns = append(selectColumns, "quota_reset_last_applied_at")
	}

	err = DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(workspace).Select(selectColumns).Updates(workspace).Error; err != nil {
			return err
		}
		if shouldApplyNow {
			return resetWorkspaceTokenQuotasTx(tx, workspace, nowUnix)
		}
		return nil
	})
	if err != nil {
		return WorkspaceQuotaResetConfig{}, err
	}
	if shouldApplyNow {
		if err := InvalidateUserTokensCache(userId); err != nil {
			common.SysLog("failed to invalidate workspace quota token cache: " + err.Error())
		}
	}
	return workspace.QuotaResetConfig(), nil
}

func ResetUserWorkspaceQuotaNow(userId int, workspaceId int, now time.Time) (WorkspaceQuotaResetConfig, error) {
	workspace, err := GetUserWorkspaceByID(userId, workspaceId)
	if err != nil {
		return WorkspaceQuotaResetConfig{}, err
	}
	if !workspace.QuotaResetEnabled || workspace.QuotaResetAmount <= 0 {
		return WorkspaceQuotaResetConfig{}, errors.New("请先启用工作区配额规则")
	}
	if now.IsZero() {
		now = time.Now()
	}
	nowUnix := now.Unix()

	workspace.QuotaResetLastAppliedAt = nowUnix
	workspace.UpdatedTime = common.GetTimestamp()
	err = DB.Transaction(func(tx *gorm.DB) error {
		if err := resetWorkspaceTokenQuotasTx(tx, workspace, nowUnix); err != nil {
			return err
		}
		return tx.Model(workspace).
			Select("quota_reset_last_applied_at", "updated_time").
			Updates(workspace).Error
	})
	if err != nil {
		return WorkspaceQuotaResetConfig{}, err
	}
	if err := InvalidateUserTokensCache(userId); err != nil {
		common.SysLog("failed to invalidate workspace quota token cache: " + err.Error())
	}
	return workspace.QuotaResetConfig(), nil
}

func ResetUserTokenWorkspaceQuota(userId int, tokenId int, now time.Time) (*Token, error) {
	if userId <= 0 || tokenId <= 0 {
		return nil, errors.New("令牌参数无效")
	}
	token, err := GetTokenByIds(tokenId, userId)
	if err != nil {
		return nil, err
	}
	if token.BillingSource == "subscription" || token.UserSubscriptionId > 0 {
		return nil, errors.New("订阅密钥不支持工作区配额重置")
	}
	if token.WorkspaceId <= 0 {
		return nil, errors.New("令牌未关联工作区")
	}
	workspace, err := GetUserWorkspaceByID(userId, token.WorkspaceId)
	if err != nil {
		return nil, err
	}
	if !workspace.QuotaResetEnabled || workspace.QuotaResetAmount <= 0 {
		return nil, errors.New("请先启用工作区配额规则")
	}
	if now.IsZero() {
		now = time.Now()
	}
	nowUnix := now.Unix()

	if err := DB.Transaction(func(tx *gorm.DB) error {
		return resetWorkspaceTokenQuotaTx(tx, workspace, token.Id, nowUnix)
	}); err != nil {
		return nil, err
	}
	if err := InvalidateUserTokensCache(userId); err != nil {
		common.SysLog("failed to invalidate workspace quota token cache: " + err.Error())
	}
	token.RemainQuota = workspace.QuotaResetAmount
	token.UnlimitedQuota = false
	token.AccessedTime = nowUnix
	if token.Status == common.TokenStatusExhausted {
		token.Status = common.TokenStatusEnabled
	}
	token.WorkspaceName = workspace.Name
	return token, nil
}

func isValidWorkspaceQuotaResetPeriod(period string) bool {
	switch period {
	case WorkspaceQuotaResetPeriodDaily, WorkspaceQuotaResetPeriodWeekly, WorkspaceQuotaResetPeriodMonthly:
		return true
	default:
		return false
	}
}

func NextWorkspaceQuotaResetAt(period string, now time.Time) time.Time {
	if now.IsZero() {
		now = time.Now()
	}
	startOfToday := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	switch period {
	case WorkspaceQuotaResetPeriodWeekly:
		weekday := int(startOfToday.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		return startOfToday.AddDate(0, 0, 8-weekday)
	case WorkspaceQuotaResetPeriodMonthly:
		return time.Date(now.Year(), now.Month()+1, 1, 0, 0, 0, 0, now.Location())
	default:
		return startOfToday.AddDate(0, 0, 1)
	}
}

func ResetDueWorkspaceQuotaConfigs(limit int, now time.Time) (int, error) {
	if limit <= 0 {
		limit = 200
	}
	if now.IsZero() {
		now = time.Now()
	}
	nowUnix := now.Unix()

	var workspaces []Workspace
	if err := DB.Where("quota_reset_enabled = ? AND quota_reset_next_at > 0 AND quota_reset_next_at <= ?", true, nowUnix).
		Order("quota_reset_next_at asc").
		Limit(limit).
		Find(&workspaces).Error; err != nil {
		return 0, err
	}
	if len(workspaces) == 0 {
		return 0, nil
	}

	resetCount := 0
	for _, workspace := range workspaces {
		workspaceCopy := workspace
		err := DB.Transaction(func(tx *gorm.DB) error {
			var locked Workspace
			if err := tx.Set("gorm:query_option", "FOR UPDATE").
				Where("id = ? AND quota_reset_enabled = ? AND quota_reset_next_at > 0 AND quota_reset_next_at <= ?", workspaceCopy.Id, true, nowUnix).
				First(&locked).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return nil
				}
				return err
			}
			if !isValidWorkspaceQuotaResetPeriod(locked.QuotaResetPeriod) {
				locked.QuotaResetPeriod = WorkspaceQuotaResetPeriodDaily
			}
			if locked.QuotaResetAmount <= 0 {
				return tx.Model(&locked).Select("quota_reset_enabled", "quota_reset_next_at", "updated_time").Updates(map[string]interface{}{
					"quota_reset_enabled": false,
					"quota_reset_next_at": 0,
					"updated_time":        common.GetTimestamp(),
				}).Error
			}
			if err := resetWorkspaceTokenQuotasTx(tx, &locked, nowUnix); err != nil {
				return err
			}
			locked.QuotaResetLastAppliedAt = nowUnix
			locked.QuotaResetNextAt = NextWorkspaceQuotaResetAt(locked.QuotaResetPeriod, now).Unix()
			locked.UpdatedTime = common.GetTimestamp()
			if err := tx.Model(&locked).Select("quota_reset_last_applied_at", "quota_reset_next_at", "updated_time").Updates(&locked).Error; err != nil {
				return err
			}
			resetCount++
			return nil
		})
		if err != nil {
			return resetCount, err
		}
		if err := InvalidateUserTokensCache(workspaceCopy.UserId); err != nil {
			common.SysLog("failed to invalidate workspace quota token cache: " + err.Error())
		}
	}
	return resetCount, nil
}

func resetWorkspaceTokenQuotasTx(tx *gorm.DB, workspace *Workspace, nowUnix int64) error {
	if tx == nil || workspace == nil || workspace.Id <= 0 || workspace.UserId <= 0 {
		return errors.New("工作区配额重置参数无效")
	}
	if err := tx.Model(&Token{}).
		Where("user_id = ? AND workspace_id = ? AND (billing_source IS NULL OR billing_source <> ?) AND user_subscription_id = ?", workspace.UserId, workspace.Id, "subscription", 0).
		Updates(map[string]interface{}{
			"remain_quota":    workspace.QuotaResetAmount,
			"unlimited_quota": false,
			"accessed_time":   nowUnix,
		}).Error; err != nil {
		return err
	}
	return tx.Model(&Token{}).
		Where("user_id = ? AND workspace_id = ? AND (billing_source IS NULL OR billing_source <> ?) AND user_subscription_id = ? AND status = ?", workspace.UserId, workspace.Id, "subscription", 0, common.TokenStatusExhausted).
		Update("status", common.TokenStatusEnabled).Error
}

func resetWorkspaceTokenQuotaTx(tx *gorm.DB, workspace *Workspace, tokenId int, nowUnix int64) error {
	if tx == nil || workspace == nil || workspace.Id <= 0 || workspace.UserId <= 0 || tokenId <= 0 {
		return errors.New("工作区令牌配额重置参数无效")
	}
	if err := tx.Model(&Token{}).
		Where("id = ? AND user_id = ? AND workspace_id = ? AND (billing_source IS NULL OR billing_source <> ?) AND user_subscription_id = ?", tokenId, workspace.UserId, workspace.Id, "subscription", 0).
		Updates(map[string]interface{}{
			"remain_quota":    workspace.QuotaResetAmount,
			"unlimited_quota": false,
			"accessed_time":   nowUnix,
		}).Error; err != nil {
		return err
	}
	return tx.Model(&Token{}).
		Where("id = ? AND user_id = ? AND workspace_id = ? AND (billing_source IS NULL OR billing_source <> ?) AND user_subscription_id = ? AND status = ?", tokenId, workspace.UserId, workspace.Id, "subscription", 0, common.TokenStatusExhausted).
		Update("status", common.TokenStatusEnabled).Error
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

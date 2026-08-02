package model

import (
	"errors"
	"strconv"
	"strings"

	"gorm.io/gorm"
)

const (
	AgentStatusDisabled = 0
	AgentStatusEnabled  = 1
	AgentStatusPending  = 2

	AgentDomainStatusPending  = 0
	AgentDomainStatusActive   = 1
	AgentDomainStatusDisabled = 2

	AgentUserStatusDisabled = 0
	AgentUserStatusEnabled  = 1

	AgentPriceModeMultiplier    = "multiplier"
	AgentPriceModeModelOverride = "model_override"

	AgentUserSourceDomain    = "domain"
	AgentUserSourceInvite    = "invite"
	AgentUserSourceAdminBind = "admin_bind"

	AgentLedgerTypeConsumeProfit = "consume_profit"
	AgentLedgerTypeWithdraw      = "withdraw"
	AgentLedgerTypeAdjustment    = "adjustment"
	AgentLedgerTypeUserTopup     = "user_topup"

	AgentWithdrawalStatusPending   = "pending"
	AgentWithdrawalStatusApproved  = "approved"
	AgentWithdrawalStatusPaid      = "paid"
	AgentWithdrawalStatusRejected  = "rejected"
	AgentWithdrawalStatusCancelled = "cancelled"
)

var ErrAgentUserNotFound = errors.New("agent user not found")
var ErrAgentNotFound = errors.New("agent not found")
var ErrAgentUserAlreadyBound = errors.New("user already belongs to another agent")

type Agent struct {
	Id                 int     `json:"id"`
	OwnerUserId        int     `json:"owner_user_id" gorm:"index;column:owner_user_id"`
	Name               string  `json:"name" gorm:"type:varchar(64);index"`
	Slug               string  `json:"slug" gorm:"type:varchar(64);uniqueIndex"`
	Status             int     `json:"status" gorm:"type:int;index"`
	PriceMode          string  `json:"price_mode" gorm:"type:varchar(32);default:'multiplier'"`
	DefaultMarkup      float64 `json:"default_markup" gorm:"precision:10;scale:6;default:1"`
	MinWithdrawAmount  int     `json:"min_withdraw_amount" gorm:"type:int;default:0"`
	WithdrawFeeRate    float64 `json:"withdraw_fee_rate" gorm:"precision:10;scale:6;default:0"`
	SettlementCurrency string  `json:"settlement_currency" gorm:"type:varchar(16);default:'USD'"`
	Branding           string  `json:"branding" gorm:"type:text"`
	CreatedAt          int64   `json:"created_at" gorm:"autoCreateTime;column:created_at"`
	UpdatedAt          int64   `json:"updated_at" gorm:"autoUpdateTime;column:updated_at"`
}

type AgentDomain struct {
	Id          int    `json:"id"`
	AgentId     int    `json:"agent_id" gorm:"index;column:agent_id"`
	Domain      string `json:"domain" gorm:"type:varchar(255);uniqueIndex"`
	Status      int    `json:"status" gorm:"type:int;index"`
	VerifyToken string `json:"verify_token" gorm:"type:varchar(128)"`
	CNAMETarget string `json:"cname_target" gorm:"-"`
	ForceHttps  bool   `json:"force_https" gorm:"column:force_https"`
	CreatedAt   int64  `json:"created_at" gorm:"autoCreateTime;column:created_at"`
	UpdatedAt   int64  `json:"updated_at" gorm:"autoUpdateTime;column:updated_at"`
}

type AgentUser struct {
	Id        int    `json:"id"`
	AgentId   int    `json:"agent_id" gorm:"index:idx_agent_users_agent_user,priority:1;column:agent_id"`
	UserId    int    `json:"user_id" gorm:"uniqueIndex;index:idx_agent_users_agent_user,priority:2;column:user_id"`
	Source    string `json:"source" gorm:"type:varchar(32);default:'domain'"`
	Status    int    `json:"status" gorm:"type:int;index"`
	Group     string `json:"group" gorm:"type:varchar(64);default:'';column:group"`
	CreatedAt int64  `json:"created_at" gorm:"autoCreateTime;column:created_at"`
}

type AgentUserWithProfile struct {
	Id                 int    `json:"id" gorm:"column:id"`
	AgentId            int    `json:"agent_id" gorm:"column:agent_id"`
	UserId             int    `json:"user_id" gorm:"column:user_id"`
	Source             string `json:"source" gorm:"column:source"`
	AgentUserStatus    int    `json:"agent_user_status" gorm:"column:agent_user_status"`
	AgentUserGroup     string `json:"agent_user_group" gorm:"column:agent_user_group"`
	AgentUserCreatedAt int64  `json:"agent_user_created_at" gorm:"column:agent_user_created_at"`

	Username        string `json:"username" gorm:"column:username"`
	DisplayName     string `json:"display_name" gorm:"column:display_name"`
	Email           string `json:"email" gorm:"column:email"`
	Role            int    `json:"role" gorm:"column:role"`
	UserStatus      int    `json:"status" gorm:"column:user_status"`
	Group           string `json:"group" gorm:"column:user_group"`
	SystemGroup     string `json:"system_group" gorm:"column:system_group"`
	Quota           int    `json:"quota" gorm:"column:quota"`
	UsedQuota       int    `json:"used_quota" gorm:"column:used_quota"`
	RequestCount    int    `json:"request_count" gorm:"column:request_count"`
	AffCount        int    `json:"aff_count" gorm:"column:aff_count"`
	AffQuota        int    `json:"aff_quota" gorm:"column:aff_quota"`
	AffHistoryQuota int    `json:"aff_history_quota" gorm:"column:aff_history_quota"`
	InviterId       int    `json:"inviter_id" gorm:"column:inviter_id"`
	Remark          string `json:"remark,omitempty" gorm:"column:remark"`
	UserCreatedAt   int64  `json:"created_at" gorm:"column:user_created_at"`
	LastLoginAt     int64  `json:"last_login_at" gorm:"column:last_login_at"`
}

type AgentPricingRule struct {
	Id           int     `json:"id"`
	AgentId      int     `json:"agent_id" gorm:"index:idx_agent_pricing_agent_model,priority:1;column:agent_id"`
	ModelPattern string  `json:"model_pattern" gorm:"type:varchar(128);index:idx_agent_pricing_agent_model,priority:2"`
	Markup       float64 `json:"markup" gorm:"precision:10;scale:6;default:1"`
	Enabled      bool    `json:"enabled" gorm:"index"`
	CreatedAt    int64   `json:"created_at" gorm:"autoCreateTime;column:created_at"`
	UpdatedAt    int64   `json:"updated_at" gorm:"autoUpdateTime;column:updated_at"`
}

type AgentGroupRatio struct {
	Id              int     `json:"id"`
	AgentId         int     `json:"agent_id" gorm:"index:idx_agent_group_ratio_agent_group,priority:1;column:agent_id"`
	GroupName       string  `json:"group_name" gorm:"type:varchar(64);index:idx_agent_group_ratio_agent_group,priority:2"`
	SystemGroupName string  `json:"system_group_name" gorm:"type:varchar(64);index"`
	Description     string  `json:"description" gorm:"type:varchar(255);default:''"`
	Ratio           float64 `json:"ratio" gorm:"precision:10;scale:6;default:1"`
	Visible         bool    `json:"visible"`
	CreatedAt       int64   `json:"created_at" gorm:"autoCreateTime;column:created_at"`
	UpdatedAt       int64   `json:"updated_at" gorm:"autoUpdateTime;column:updated_at"`
}

type AgentUserGroupConfig struct {
	Id            int    `json:"id"`
	AgentId       int    `json:"agent_id" gorm:"uniqueIndex:idx_agent_user_group_agent_group,priority:1;column:agent_id"`
	GroupName     string `json:"group_name" gorm:"type:varchar(64);uniqueIndex:idx_agent_user_group_agent_group,priority:2"`
	VisibleGroups string `json:"visible_groups" gorm:"type:text;column:visible_groups"`
	GroupRatios   string `json:"group_ratios" gorm:"type:text;column:group_ratios"`
	CreatedAt     int64  `json:"created_at" gorm:"autoCreateTime;column:created_at"`
	UpdatedAt     int64  `json:"updated_at" gorm:"autoUpdateTime;column:updated_at"`
}

type AgentLedger struct {
	Id             int    `json:"id"`
	AgentId        int    `json:"agent_id" gorm:"index;column:agent_id"`
	UserId         int    `json:"user_id" gorm:"index;column:user_id"`
	OperatorUserId int    `json:"operator_user_id" gorm:"index;column:operator_user_id"`
	LogId          int    `json:"log_id" gorm:"index;column:log_id"`
	Type           string `json:"type" gorm:"type:varchar(32);index"`
	BaseQuota      int    `json:"base_quota" gorm:"type:int;default:0;column:base_quota"`
	ChargedQuota   int    `json:"charged_quota" gorm:"type:int;default:0;column:charged_quota"`
	ProfitQuota    int    `json:"profit_quota" gorm:"type:int;default:0;column:profit_quota"`
	BalanceAfter   int    `json:"balance_after" gorm:"type:int;default:0;column:balance_after"`
	Remark         string `json:"remark" gorm:"type:text"`
	CreatedAt      int64  `json:"created_at" gorm:"autoCreateTime;column:created_at"`
}

type AgentWithdrawal struct {
	Id          int     `json:"id"`
	AgentId     int     `json:"agent_id" gorm:"index;column:agent_id"`
	AmountQuota int     `json:"amount_quota" gorm:"type:int;default:0;column:amount_quota"`
	AmountMoney float64 `json:"amount_money" gorm:"precision:18;scale:6;default:0;column:amount_money"`
	Fee         float64 `json:"fee" gorm:"precision:18;scale:6;default:0"`
	Status      string  `json:"status" gorm:"type:varchar(32);default:'pending';index"`
	AccountInfo string  `json:"account_info" gorm:"type:text;column:account_info"`
	AdminRemark string  `json:"admin_remark" gorm:"type:varchar(255);column:admin_remark"`
	CreatedAt   int64   `json:"created_at" gorm:"autoCreateTime;column:created_at"`
	ProcessedAt int64   `json:"processed_at" gorm:"bigint;default:0;column:processed_at"`
}

type AgentWithDomain struct {
	Agent
	Domain string `json:"domain"`
}

type AgentDomainWithAgent struct {
	AgentDomain
	AgentName   string `json:"agent_name"`
	AgentSlug   string `json:"agent_slug"`
	OwnerUserId int    `json:"owner_user_id"`
}

func GetActiveAgentByDomain(domain string) (*AgentWithDomain, error) {
	var result AgentWithDomain
	query := DB.Table("agents").
		Select("agents.*, agent_domains.domain").
		Joins("JOIN agent_domains ON agent_domains.agent_id = agents.id").
		Where("agent_domains.domain = ? AND agent_domains.status = ? AND agents.status = ?", domain, AgentDomainStatusActive, AgentStatusEnabled).
		Limit(1).
		Find(&result)
	if query.Error != nil {
		return nil, query.Error
	}
	if query.RowsAffected == 0 {
		return nil, nil
	}
	return &result, nil
}

func IsUserInAgent(agentId int, userId int) (bool, error) {
	var count int64
	err := DB.Model(&AgentUser{}).
		Where("agent_id = ? AND user_id = ? AND status = ?", agentId, userId, AgentUserStatusEnabled).
		Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func UserOwnsAgent(userId int) (bool, error) {
	var count int64
	err := DB.Model(&Agent{}).
		Where("owner_user_id = ? AND status <> ?", userId, AgentStatusDisabled).
		Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func UserHasAgentConsole(userId int) (bool, error) {
	_, err := GetAgentByConsoleUserId(userId)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, ErrAgentNotFound) {
		return false, nil
	}
	return false, err
}

func GetAgentByOwnerUserId(userId int) (*Agent, error) {
	var agent Agent
	err := DB.Where("owner_user_id = ? AND status <> ?", userId, AgentStatusDisabled).
		Order("id desc").
		First(&agent).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrAgentNotFound
	}
	return &agent, err
}

func GetAgentByConsoleUserId(userId int) (*Agent, error) {
	return GetAgentByOwnerUserId(userId)
}

func BindUserToAgent(agentId int, userId int, source string) error {
	if agentId == 0 || userId == 0 {
		return ErrAgentUserNotFound
	}
	if source == "" {
		source = AgentUserSourceDomain
	}
	var existing AgentUser
	err := DB.Where("user_id = ?", userId).First(&existing).Error
	if err == nil {
		if existing.AgentId != agentId {
			return ErrAgentUserAlreadyBound
		}
		return DB.Model(&existing).Updates(map[string]interface{}{
			"source": source,
			"status": AgentUserStatusEnabled,
		}).Error
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	agentUser := &AgentUser{
		AgentId: agentId,
		UserId:  userId,
		Source:  source,
		Status:  AgentUserStatusEnabled,
	}
	return DB.Create(agentUser).Error
}

func GetAgentById(id int) (*Agent, error) {
	var agent Agent
	err := DB.First(&agent, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrAgentNotFound
	}
	return &agent, err
}

func ListAgents(startIdx int, num int) ([]*Agent, int64, error) {
	var agents []*Agent
	var total int64
	tx := DB.Model(&Agent{})
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := tx.Order("id desc").Limit(num).Offset(startIdx).Find(&agents).Error; err != nil {
		return nil, 0, err
	}
	return agents, total, nil
}

func ListAgentDomains(agentId int, startIdx int, num int) ([]*AgentDomain, int64, error) {
	var domains []*AgentDomain
	var total int64
	tx := DB.Model(&AgentDomain{}).Where("agent_id = ?", agentId)
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := tx.Order("id desc").Limit(num).Offset(startIdx).Find(&domains).Error; err != nil {
		return nil, 0, err
	}
	return domains, total, nil
}

func ListAgentDomainsByStatus(status int, startIdx int, num int) ([]*AgentDomainWithAgent, int64, error) {
	var domains []*AgentDomainWithAgent
	var total int64
	tx := DB.Model(&AgentDomain{}).
		Joins("JOIN agents ON agents.id = agent_domains.agent_id")
	if status >= 0 {
		tx = tx.Where("agent_domains.status = ?", status)
	}
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := tx.
		Select("agent_domains.*, agents.name AS agent_name, agents.slug AS agent_slug, agents.owner_user_id AS owner_user_id").
		Order("agent_domains.id desc").
		Limit(num).
		Offset(startIdx).
		Find(&domains).Error; err != nil {
		return nil, 0, err
	}
	return domains, total, nil
}

func ListAgentGroupRatios(agentId int) ([]*AgentGroupRatio, error) {
	ratios := make([]*AgentGroupRatio, 0)
	if err := DB.Where("agent_id = ?", agentId).Order("group_name asc").Find(&ratios).Error; err != nil {
		return nil, err
	}
	return ratios, nil
}

func ListAgentUserGroupConfigs(agentId int) ([]*AgentUserGroupConfig, error) {
	groups := make([]*AgentUserGroupConfig, 0)
	if err := DB.Where("agent_id = ?", agentId).Order("group_name asc").Find(&groups).Error; err != nil {
		return nil, err
	}
	return groups, nil
}

func GetAgentUserGroupConfigMap(agentId int) (map[string]*AgentUserGroupConfig, error) {
	groups, err := ListAgentUserGroupConfigs(agentId)
	if err != nil {
		return nil, err
	}
	result := make(map[string]*AgentUserGroupConfig, len(groups))
	for _, group := range groups {
		result[group.GroupName] = group
	}
	return result, nil
}

func ListAgentUsers(agentId int, keyword string, startIdx int, num int) ([]*AgentUserWithProfile, int64, error) {
	var users []*AgentUserWithProfile
	var total int64
	groupCol := commonGroupCol
	if groupCol == "" {
		groupCol = "`group`"
	}
	tx := DB.Model(&AgentUser{}).
		Joins("JOIN users ON users.id = agent_users.user_id").
		Where("agent_users.agent_id = ?", agentId)
	if keyword != "" {
		keyword = strings.TrimSpace(keyword)
		like := "%" + keyword + "%"
		if userId, err := strconv.Atoi(keyword); err == nil {
			tx = tx.Where("users.id = ? OR users.username LIKE ? OR users.email LIKE ? OR users.display_name LIKE ?", userId, like, like, like)
		} else {
			tx = tx.Where("users.username LIKE ? OR users.email LIKE ? OR users.display_name LIKE ?", like, like, like)
		}
	}
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	selectFields := []string{
		"agent_users.id",
		"agent_users.agent_id",
		"agent_users.user_id",
		"agent_users.source",
		"agent_users.status AS agent_user_status",
		"agent_users." + groupCol + " AS agent_user_group",
		"agent_users.created_at AS agent_user_created_at",
		"users.username",
		"users.display_name",
		"users.email",
		"users.role",
		"users.status AS user_status",
		"COALESCE(NULLIF(agent_users." + groupCol + ", ''), users." + groupCol + ") AS user_group",
		"users." + groupCol + " AS system_group",
		"users.quota",
		"users.used_quota",
		"users.request_count",
		"users.aff_count",
		"users.aff_quota",
		"users.aff_history AS aff_history_quota",
		"users.inviter_id",
		"users.remark",
		"users.created_at AS user_created_at",
		"users.last_login_at",
	}
	if err := tx.Select(strings.Join(selectFields, ", ")).
		Order("agent_users.id desc").
		Limit(num).
		Offset(startIdx).
		Find(&users).Error; err != nil {
		return nil, 0, err
	}
	return users, total, nil
}

func ListAgentLedger(agentId int, startIdx int, num int) ([]*AgentLedger, int64, error) {
	var ledgers []*AgentLedger
	var total int64
	tx := DB.Model(&AgentLedger{}).Where("agent_id = ?", agentId)
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := tx.Order("id desc").Limit(num).Offset(startIdx).Find(&ledgers).Error; err != nil {
		return nil, 0, err
	}
	return ledgers, total, nil
}

func ListAgentWithdrawals(agentId int, startIdx int, num int) ([]*AgentWithdrawal, int64, error) {
	var withdrawals []*AgentWithdrawal
	var total int64
	tx := DB.Model(&AgentWithdrawal{})
	if agentId > 0 {
		tx = tx.Where("agent_id = ?", agentId)
	}
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := tx.Order("id desc").Limit(num).Offset(startIdx).Find(&withdrawals).Error; err != nil {
		return nil, 0, err
	}
	return withdrawals, total, nil
}

package agent

import (
	"errors"
	"math"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrInvalidAgentDomain         = errors.New("invalid agent domain")
	ErrAgentDomainAlreadyExists   = errors.New("agent domain already exists")
	ErrInvalidAgentGroupRatio     = errors.New("agent group ratio must be not less than 0")
	ErrAgentGroupRatioNotFound    = errors.New("agent group ratio not found")
	ErrAgentGroupRatioBelowSystem = errors.New("agent group ratio cannot be lower than system group ratio")
	ErrAgentSystemGroupNotFound   = errors.New("agent system group not found")
	ErrInsufficientAgentBalance   = errors.New("insufficient agent balance")
	ErrInvalidAgentBalanceAmount  = errors.New("agent balance amount must be greater than 0")
	ErrInvalidWithdrawalStatus    = errors.New("invalid withdrawal status")
	ErrInvalidAgentUserStatus     = errors.New("invalid agent user status")
	ErrAgentUserGroupNotAllowed   = errors.New("agent user group is not allowed")
)

type Balance struct {
	ProfitQuota              int     `json:"profit_quota"`
	PendingWithdrawalQuota   int     `json:"pending_withdrawal_quota"`
	ApprovedWithdrawalQuota  int     `json:"approved_withdrawal_quota"`
	AvailableQuota           int     `json:"available_quota"`
	Currency                 string  `json:"currency"`
	ProfitAmount             float64 `json:"profit_amount"`
	PendingWithdrawalAmount  float64 `json:"pending_withdrawal_amount"`
	ApprovedWithdrawalAmount float64 `json:"approved_withdrawal_amount"`
	AvailableAmount          float64 `json:"available_amount"`
}

type GroupRatioView struct {
	GroupName       string  `json:"group_name"`
	SystemGroupName string  `json:"system_group_name"`
	Description     string  `json:"description"`
	AgentRatio      float64 `json:"agent_ratio"`
	SystemRatio     float64 `json:"system_ratio"`
	ConfiguredRatio float64 `json:"configured_ratio"`
	EffectiveRatio  float64 `json:"effective_ratio"`
	Configured      bool    `json:"configured"`
	Visible         bool    `json:"visible"`
	Available       bool    `json:"available"`
}

type UserGroupConfigView struct {
	GroupName     string             `json:"group_name"`
	VisibleGroups []string           `json:"visible_groups"`
	GroupRatios   map[string]float64 `json:"group_ratios"`
}

type LedgerView struct {
	Id                 int     `json:"id"`
	AgentId            int     `json:"agent_id"`
	UserId             int     `json:"user_id"`
	LogId              int     `json:"log_id"`
	OperatorUserId     int     `json:"operator_user_id"`
	Type               string  `json:"type"`
	BaseQuota          int     `json:"base_quota"`
	ChargedQuota       int     `json:"charged_quota"`
	ProfitQuota        int     `json:"profit_quota"`
	BalanceAfter       int     `json:"balance_after"`
	Currency           string  `json:"currency"`
	BaseAmount         float64 `json:"base_amount"`
	ChargedAmount      float64 `json:"charged_amount"`
	ProfitAmount       float64 `json:"profit_amount"`
	BalanceAfterAmount float64 `json:"balance_after_amount"`
	Remark             string  `json:"remark"`
	CreatedAt          int64   `json:"created_at"`
}

type WithdrawalView struct {
	Id               int     `json:"id"`
	AgentId          int     `json:"agent_id"`
	AmountQuota      int     `json:"amount_quota"`
	AmountMoney      float64 `json:"amount_money"`
	SettlementAmount float64 `json:"settlement_amount"`
	Currency         string  `json:"currency"`
	Fee              float64 `json:"fee"`
	Status           string  `json:"status"`
	AccountInfo      string  `json:"account_info"`
	AdminRemark      string  `json:"admin_remark"`
	CreatedAt        int64   `json:"created_at"`
	ProcessedAt      int64   `json:"processed_at"`
}

func GetBalance(agentID int) (*Balance, error) {
	balance, err := getBalanceWithTx(model.DB, agentID)
	if err != nil {
		return nil, err
	}
	currency, err := getSettlementCurrencyWithTx(model.DB, agentID)
	if err != nil {
		return nil, err
	}
	fillBalanceAmounts(balance, currency)
	return balance, nil
}

func GetBalances(agents []*model.Agent) (map[int]*Balance, error) {
	balances := make(map[int]*Balance, len(agents))
	if len(agents) == 0 {
		return balances, nil
	}

	agentIDs := make([]int, 0, len(agents))
	for _, item := range agents {
		if item == nil {
			continue
		}
		agentIDs = append(agentIDs, item.Id)
		balances[item.Id] = &Balance{}
	}
	if len(agentIDs) == 0 {
		return balances, nil
	}

	type ledgerTotal struct {
		AgentId     int `gorm:"column:agent_id"`
		ProfitQuota int `gorm:"column:profit_quota"`
	}
	var ledgerTotals []ledgerTotal
	if err := model.DB.Model(&model.AgentLedger{}).
		Select("agent_id, COALESCE(SUM(profit_quota), 0) AS profit_quota").
		Where("agent_id IN ?", agentIDs).
		Group("agent_id").
		Scan(&ledgerTotals).Error; err != nil {
		return nil, err
	}
	for _, total := range ledgerTotals {
		if balance := balances[total.AgentId]; balance != nil {
			balance.ProfitQuota = total.ProfitQuota
		}
	}

	type withdrawalTotal struct {
		AgentId     int    `gorm:"column:agent_id"`
		Status      string `gorm:"column:status"`
		AmountQuota int    `gorm:"column:amount_quota"`
	}
	var withdrawalTotals []withdrawalTotal
	if err := model.DB.Model(&model.AgentWithdrawal{}).
		Select("agent_id, status, COALESCE(SUM(amount_quota), 0) AS amount_quota").
		Where("agent_id IN ? AND status IN ?", agentIDs, []string{
			model.AgentWithdrawalStatusPending,
			model.AgentWithdrawalStatusApproved,
		}).
		Group("agent_id, status").
		Scan(&withdrawalTotals).Error; err != nil {
		return nil, err
	}
	for _, total := range withdrawalTotals {
		balance := balances[total.AgentId]
		if balance == nil {
			continue
		}
		switch total.Status {
		case model.AgentWithdrawalStatusPending:
			balance.PendingWithdrawalQuota = total.AmountQuota
		case model.AgentWithdrawalStatusApproved:
			balance.ApprovedWithdrawalQuota = total.AmountQuota
		}
	}

	for _, item := range agents {
		if item == nil {
			continue
		}
		balance := balances[item.Id]
		balance.AvailableQuota = balance.ProfitQuota - balance.PendingWithdrawalQuota - balance.ApprovedWithdrawalQuota
		fillBalanceAmounts(balance, item.SettlementCurrency)
	}
	return balances, nil
}

func BuildLedgerViews(agentID int, ledgers []*model.AgentLedger) ([]*LedgerView, error) {
	currency, err := getSettlementCurrencyWithTx(model.DB, agentID)
	if err != nil {
		return nil, err
	}
	views := make([]*LedgerView, 0, len(ledgers))
	for _, ledger := range ledgers {
		views = append(views, buildLedgerView(ledger, currency))
	}
	return views, nil
}

func BuildWithdrawalViews(withdrawals []*model.AgentWithdrawal) ([]*WithdrawalView, error) {
	currencies := make(map[int]string)
	views := make([]*WithdrawalView, 0, len(withdrawals))
	for _, withdrawal := range withdrawals {
		currency, ok := currencies[withdrawal.AgentId]
		if !ok {
			var err error
			currency, err = getSettlementCurrencyWithTx(model.DB, withdrawal.AgentId)
			if err != nil {
				return nil, err
			}
			currencies[withdrawal.AgentId] = currency
		}
		views = append(views, buildWithdrawalView(withdrawal, currency))
	}
	return views, nil
}

func CreateDomain(agentID int, rawDomain string) (*model.AgentDomain, error) {
	domain := NormalizeHost(rawDomain)
	if domain == "" || !strings.Contains(domain, ".") {
		return nil, ErrInvalidAgentDomain
	}
	var existing model.AgentDomain
	err := model.DB.Where("domain = ?", domain).First(&existing).Error
	if err == nil {
		return nil, ErrAgentDomainAlreadyExists
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	token, err := common.GenerateRandomCharsKey(32)
	if err != nil {
		return nil, err
	}
	token = strings.ToLower(token)
	agentDomain := &model.AgentDomain{
		AgentId:     agentID,
		Domain:      domain,
		Status:      model.AgentDomainStatusPending,
		VerifyToken: token,
		ForceHttps:  true,
	}
	if err := model.DB.Create(agentDomain).Error; err != nil {
		if isAgentDomainDuplicateError(err) {
			return nil, ErrAgentDomainAlreadyExists
		}
		return nil, err
	}
	FillDomainCNAMETarget(agentDomain)
	return agentDomain, nil
}

func isAgentDomainDuplicateError(err error) bool {
	if err == nil {
		return false
	}
	message := err.Error()
	return strings.Contains(message, "idx_agent_domains_domain") ||
		strings.Contains(message, "Duplicate entry") ||
		strings.Contains(message, "UNIQUE constraint failed: agent_domains.domain") ||
		strings.Contains(message, "duplicate key value violates unique constraint")
}

func ActivateDomain(agentID int, domainID int) error {
	return model.DB.Model(&model.AgentDomain{}).
		Where("id = ? AND agent_id = ?", domainID, agentID).
		Updates(map[string]interface{}{
			"status":      model.AgentDomainStatusActive,
			"verified_at": common.GetTimestamp(),
		}).Error
}

func UpdateDomainStatus(agentID int, domainID int, status int) error {
	return model.DB.Model(&model.AgentDomain{}).
		Where("id = ? AND agent_id = ?", domainID, agentID).
		Update("status", status).Error
}

func UpdateBranding(agentID int, branding string) (*model.Agent, error) {
	branding = strings.TrimSpace(branding)
	if err := model.DB.Model(&model.Agent{}).
		Where("id = ?", agentID).
		Update("branding", branding).Error; err != nil {
		return nil, err
	}
	return model.GetAgentById(agentID)
}

func EffectiveGroupRatioMap(agentID int, userGroups ...string) (map[string]float64, error) {
	groupMap, err := EffectiveGroupMap(agentID, userGroups...)
	if err != nil {
		return nil, err
	}
	result := make(map[string]float64, len(groupMap))
	for groupName, group := range groupMap {
		if !group.Available {
			continue
		}
		result[groupName] = group.EffectiveRatio
	}
	return result, nil
}

func EffectiveGroupMap(agentID int, userGroups ...string) (map[string]types.AgentGroup, error) {
	configuredGroups, err := model.ListAgentGroupRatios(agentID)
	if err != nil {
		return nil, err
	}
	userGroup, err := resolveAgentOwnerUserGroup(agentID, userGroups...)
	if err != nil {
		return nil, err
	}
	systemRatios := agentTokenUsableSystemRatios(userGroup)
	agentRatios := effectiveSystemGroupRatioMap(userGroup)
	result := make(map[string]types.AgentGroup, len(systemRatios))
	for systemGroupName, systemRatio := range systemRatios {
		agentRatio := agentRatios[systemGroupName]
		if agentRatio <= 0 {
			agentRatio = systemRatio
		}
		result[systemGroupName] = types.AgentGroup{
			GroupName:       systemGroupName,
			SystemGroupName: systemGroupName,
			AgentRatio:      agentRatio,
			SystemRatio:     systemRatio,
			ConfiguredRatio: 0,
			EffectiveRatio:  agentRatio,
			Visible:         true,
			Available:       true,
		}
	}
	for _, configured := range configuredGroups {
		group := buildAgentGroup(configured, systemRatios, agentRatios)
		if !group.Available {
			continue
		}
		result[group.SystemGroupName] = group
	}
	return result, nil
}

func ListGroupRatios(agentID int, userGroups ...string) ([]GroupRatioView, error) {
	userGroup, err := resolveAgentOwnerUserGroup(agentID, userGroups...)
	if err != nil {
		return nil, err
	}
	systemRatios := agentTokenUsableSystemRatios(userGroup)
	agentRatios := effectiveSystemGroupRatioMap(userGroup)
	configuredGroups, err := model.ListAgentGroupRatios(agentID)
	if err != nil {
		return nil, err
	}
	views := make([]GroupRatioView, 0, len(configuredGroups)+len(systemRatios))
	configuredNames := make(map[string]struct{}, len(configuredGroups))
	for _, configured := range configuredGroups {
		group := buildAgentGroup(configured, systemRatios, agentRatios)
		if !group.Available {
			continue
		}
		configuredNames[group.SystemGroupName] = struct{}{}
		views = append(views, GroupRatioView{
			GroupName:       group.SystemGroupName,
			SystemGroupName: group.SystemGroupName,
			Description:     group.Description,
			AgentRatio:      group.AgentRatio,
			SystemRatio:     group.SystemRatio,
			ConfiguredRatio: group.ConfiguredRatio,
			EffectiveRatio:  group.EffectiveRatio,
			Configured:      true,
			Visible:         group.Visible,
			Available:       group.Available,
		})
	}
	for systemGroupName, systemRatio := range systemRatios {
		if _, ok := configuredNames[systemGroupName]; ok {
			continue
		}
		agentRatio := agentRatios[systemGroupName]
		if agentRatio <= 0 {
			agentRatio = systemRatio
		}
		views = append(views, GroupRatioView{
			GroupName:       systemGroupName,
			SystemGroupName: systemGroupName,
			Description:     "",
			AgentRatio:      agentRatio,
			SystemRatio:     systemRatio,
			ConfiguredRatio: 0,
			EffectiveRatio:  agentRatio,
			Configured:      false,
			Visible:         true,
			Available:       true,
		})
	}
	sort.Slice(views, func(i, j int) bool {
		if views[i].Configured != views[j].Configured {
			return views[i].Configured
		}
		return views[i].GroupName < views[j].GroupName
	})
	return views, nil
}

func UserGroupConfigMap(agentID int) (map[string]types.AgentUserGroup, error) {
	configs, err := model.ListAgentUserGroupConfigs(agentID)
	if err != nil {
		return nil, err
	}
	result := make(map[string]types.AgentUserGroup, len(configs))
	for _, config := range configs {
		groupName := strings.TrimSpace(config.GroupName)
		if groupName == "" {
			continue
		}
		result[groupName] = types.AgentUserGroup{
			GroupName:     groupName,
			VisibleGroups: parseVisibleGroups(config.VisibleGroups),
			GroupRatios:   parseGroupRatios(config.GroupRatios),
		}
	}
	return result, nil
}

func ListUserGroupConfigs(agentID int) ([]UserGroupConfigView, error) {
	configs, err := model.ListAgentUserGroupConfigs(agentID)
	if err != nil {
		return nil, err
	}
	views := make([]UserGroupConfigView, 0, len(configs))
	for _, config := range configs {
		groupName := strings.TrimSpace(config.GroupName)
		if groupName == "" {
			continue
		}
		views = append(views, UserGroupConfigView{
			GroupName:     groupName,
			VisibleGroups: parseVisibleGroups(config.VisibleGroups),
			GroupRatios:   parseGroupRatios(config.GroupRatios),
		})
	}
	return views, nil
}

func UpsertUserGroupConfig(agentID int, groupName string, visibleGroups []string, groupRatios ...map[string]float64) (*model.AgentUserGroupConfig, error) {
	groupName = strings.TrimSpace(groupName)
	visibleGroups = normalizeVisibleGroupsForGroup(groupName, visibleGroups)
	normalizedGroupRatios, err := normalizeGroupRatioOverridesForAgent(agentID, firstGroupRatioMap(groupRatios...))
	if err != nil {
		return nil, err
	}
	if groupName == "" {
		return nil, ErrAgentUserGroupNotAllowed
	}
	groupRatiosJSON, err := marshalGroupRatios(normalizedGroupRatios)
	if err != nil {
		return nil, err
	}
	var config model.AgentUserGroupConfig
	err = model.DB.Where("agent_id = ? AND group_name = ?", agentID, groupName).First(&config).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		config = model.AgentUserGroupConfig{
			AgentId:       agentID,
			GroupName:     groupName,
			VisibleGroups: strings.Join(visibleGroups, ","),
			GroupRatios:   groupRatiosJSON,
		}
		err = model.DB.Create(&config).Error
	} else {
		err = model.DB.Model(&config).Updates(map[string]interface{}{
			"visible_groups": strings.Join(visibleGroups, ","),
			"group_ratios":   groupRatiosJSON,
		}).Error
	}
	if err != nil {
		return nil, err
	}
	return &config, nil
}

func UpsertGroupRatio(agentID int, groupName string, systemGroupName string, description string, ratio float64, visible bool, userGroups ...string) (*model.AgentGroupRatio, error) {
	groupName = strings.TrimSpace(groupName)
	systemGroupName = strings.TrimSpace(systemGroupName)
	description = strings.TrimSpace(description)
	if systemGroupName == "" {
		systemGroupName = groupName
	}
	if systemGroupName == "" {
		return nil, ErrAgentGroupRatioNotFound
	}
	groupName = systemGroupName
	userGroup, err := resolveAgentOwnerUserGroup(agentID, userGroups...)
	if err != nil {
		return nil, err
	}
	if !agentCanConfigureSystemGroup(userGroup, systemGroupName) {
		return nil, ErrAgentSystemGroupNotFound
	}
	agentRatio := effectiveSystemGroupRatio(userGroup, systemGroupName)
	if ratio < 0 {
		return nil, ErrInvalidAgentGroupRatio
	}
	if ratio < agentRatio {
		return nil, ErrAgentGroupRatioBelowSystem
	}
	var groupRatio model.AgentGroupRatio
	err = model.DB.Where("agent_id = ? AND system_group_name = ?", agentID, systemGroupName).First(&groupRatio).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		groupRatio = model.AgentGroupRatio{
			AgentId:         agentID,
			GroupName:       groupName,
			SystemGroupName: systemGroupName,
			Description:     description,
			Ratio:           ratio,
			Visible:         visible,
		}
		err = model.DB.Create(&groupRatio).Error
	} else {
		err = model.DB.Model(&groupRatio).Updates(map[string]interface{}{
			"system_group_name": systemGroupName,
			"description":       description,
			"ratio":             ratio,
			"visible":           visible,
		}).Error
	}
	if err != nil {
		return nil, err
	}
	return &groupRatio, nil
}

func resolveAgentOwnerUserGroup(agentID int, userGroups ...string) (string, error) {
	if userGroup := firstNonEmpty(userGroups...); userGroup != "" {
		return userGroup, nil
	}
	agent, err := model.GetAgentById(agentID)
	if err != nil {
		if errors.Is(err, model.ErrAgentNotFound) {
			return "", nil
		}
		return "", err
	}
	if agent.OwnerUserId <= 0 {
		return "", nil
	}
	user, err := model.GetUserById(agent.OwnerUserId, false)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(user.Group), nil
}

func effectiveSystemGroupRatioMap(userGroup string) map[string]float64 {
	systemRatios := ratio_setting.GetGroupRatioCopy()
	userGroup = strings.TrimSpace(userGroup)
	if userGroup == "" {
		return systemRatios
	}
	for groupName := range systemRatios {
		systemRatios[groupName] = effectiveSystemGroupRatio(userGroup, groupName)
	}
	return systemRatios
}

func effectiveSystemGroupRatio(userGroup string, systemGroupName string) float64 {
	systemRatio := ratio_setting.GetGroupRatio(systemGroupName)
	userGroup = strings.TrimSpace(userGroup)
	if userGroup == "" {
		return systemRatio
	}
	if userGroupRatio, ok := ratio_setting.GetGroupGroupRatio(userGroup, systemGroupName); ok {
		return userGroupRatio
	}
	return systemRatio
}

func agentTokenUsableSystemRatios(userGroup string) map[string]float64 {
	systemRatios := ratio_setting.GetGroupRatioCopy()
	userUsableGroups := agentOwnerTokenUsableGroups(userGroup)
	result := make(map[string]float64, len(userUsableGroups))
	for groupName := range userUsableGroups {
		if ratio, ok := systemRatios[groupName]; ok {
			result[groupName] = ratio
		}
	}
	return result
}

func agentOwnerTokenUsableGroups(userGroup string) map[string]string {
	groupsCopy := setting.GetUserUsableGroupsCopy()
	userGroup = strings.TrimSpace(userGroup)
	if userGroup != "" {
		if specialSettings, ok := ratio_setting.GetGroupRatioSetting().GroupSpecialUsableGroup.Get(userGroup); ok {
			for specialGroup, desc := range specialSettings {
				if strings.HasPrefix(specialGroup, "-:") {
					delete(groupsCopy, strings.TrimPrefix(specialGroup, "-:"))
					continue
				}
				if strings.HasPrefix(specialGroup, "+:") {
					groupsCopy[strings.TrimPrefix(specialGroup, "+:")] = desc
					continue
				}
				groupsCopy[specialGroup] = desc
			}
		}
		if _, ok := groupsCopy[userGroup]; !ok {
			groupsCopy[userGroup] = "用户分组"
		}
	}
	if userGroups, configured := setting.GetUserGroupNamesFromDisplayConfig(); configured {
		for _, group := range userGroups {
			delete(groupsCopy, group)
		}
	}
	return groupsCopy
}

func agentCanConfigureSystemGroup(userGroup string, systemGroupName string) bool {
	systemGroupName = strings.TrimSpace(systemGroupName)
	if systemGroupName == "" {
		return false
	}
	_, ok := agentTokenUsableSystemRatios(userGroup)[systemGroupName]
	return ok
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func buildAgentGroup(configured *model.AgentGroupRatio, systemRatios map[string]float64, agentRatios map[string]float64) types.AgentGroup {
	systemGroupName := strings.TrimSpace(configured.SystemGroupName)
	if systemGroupName == "" {
		systemGroupName = strings.TrimSpace(configured.GroupName)
	}
	systemRatio, available := systemRatios[systemGroupName]
	agentRatio := agentRatios[systemGroupName]
	if agentRatio <= 0 {
		agentRatio = systemRatio
	}
	effectiveRatio := configured.Ratio
	if !available {
		effectiveRatio = 0
	} else if effectiveRatio < agentRatio {
		effectiveRatio = agentRatio
	}
	return types.AgentGroup{
		GroupName:       systemGroupName,
		SystemGroupName: systemGroupName,
		Description:     strings.TrimSpace(configured.Description),
		AgentRatio:      agentRatio,
		SystemRatio:     systemRatio,
		ConfiguredRatio: configured.Ratio,
		EffectiveRatio:  effectiveRatio,
		Visible:         configured.Visible,
		Available:       available,
	}
}

func normalizeVisibleGroups(groups []string) []string {
	seen := make(map[string]struct{}, len(groups))
	result := make([]string, 0, len(groups))
	for _, group := range groups {
		group = strings.TrimSpace(group)
		if group == "" {
			continue
		}
		if _, ok := seen[group]; ok {
			continue
		}
		seen[group] = struct{}{}
		result = append(result, group)
	}
	sort.Strings(result)
	return result
}

func normalizeVisibleGroupsForGroup(groupName string, groups []string) []string {
	groupName = strings.TrimSpace(groupName)
	normalized := normalizeVisibleGroups(groups)
	if groupName == "" {
		return normalized
	}
	result := normalized[:0]
	for _, group := range normalized {
		if group == groupName {
			continue
		}
		result = append(result, group)
	}
	return result
}

func parseVisibleGroups(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	return normalizeVisibleGroups(strings.Split(raw, ","))
}

func firstGroupRatioMap(values ...map[string]float64) map[string]float64 {
	if len(values) == 0 {
		return nil
	}
	return values[0]
}

func normalizeGroupRatioOverrides(groupRatios map[string]float64) map[string]float64 {
	if len(groupRatios) == 0 {
		return nil
	}
	result := make(map[string]float64, len(groupRatios))
	for groupName, ratio := range groupRatios {
		groupName = strings.TrimSpace(groupName)
		if groupName == "" || ratio < 0 {
			continue
		}
		result[groupName] = ratio
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func normalizeGroupRatioOverridesForAgent(agentID int, groupRatios map[string]float64) (map[string]float64, error) {
	groupRatios = normalizeGroupRatioOverrides(groupRatios)
	if len(groupRatios) == 0 {
		return nil, nil
	}
	groups, err := EffectiveGroupMap(agentID)
	if err != nil {
		return nil, err
	}
	result := make(map[string]float64, len(groupRatios))
	for groupName, ratio := range groupRatios {
		group, ok := groups[groupName]
		if !ok || !group.Available {
			continue
		}
		minRatio := group.AgentRatio
		if minRatio < 0 {
			minRatio = 0
		}
		if ratio < minRatio {
			ratio = minRatio
		}
		result[groupName] = ratio
	}
	if len(result) == 0 {
		return nil, nil
	}
	return result, nil
}

func parseGroupRatios(raw string) map[string]float64 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var groupRatios map[string]float64
	if err := common.UnmarshalJsonStr(raw, &groupRatios); err != nil {
		return nil
	}
	return normalizeGroupRatioOverrides(groupRatios)
}

func marshalGroupRatios(groupRatios map[string]float64) (string, error) {
	groupRatios = normalizeGroupRatioOverrides(groupRatios)
	if len(groupRatios) == 0 {
		return "", nil
	}
	data, err := common.Marshal(groupRatios)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func UpdateUserGlobalStatus(agentID int, userID int, status int) error {
	if status != common.UserStatusEnabled && status != common.UserStatusDisabled {
		return ErrInvalidAgentUserStatus
	}
	if ok, err := model.IsUserInAgent(agentID, userID); err != nil {
		return err
	} else if !ok {
		return model.ErrAgentUserNotFound
	}
	user, err := model.GetUserById(userID, false)
	if err != nil {
		return err
	}
	user.Status = status
	if err := user.Update(false); err != nil {
		return err
	}
	if err := model.InvalidateUserCache(userID); err != nil {
		return err
	}
	if err := model.InvalidateUserTokensCache(userID); err != nil {
		return err
	}
	return nil
}

func UpdateUserGroup(agentID int, userID int, groupName string) error {
	groupName = strings.TrimSpace(groupName)
	if groupName == "" {
		return ErrAgentUserGroupNotAllowed
	}
	if ok, err := model.IsUserInAgent(agentID, userID); err != nil {
		return err
	} else if !ok {
		return model.ErrAgentUserNotFound
	}
	userGroups, err := model.GetAgentUserGroupConfigMap(agentID)
	if err != nil {
		return err
	}
	_, ok := userGroups[groupName]
	if !ok {
		return ErrAgentUserGroupNotAllowed
	}
	if err := model.DB.Model(&model.AgentUser{}).
		Where("agent_id = ? AND user_id = ?", agentID, userID).
		Update("group", groupName).Error; err != nil {
		return err
	}
	return nil
}

func SubmitWithdrawal(agentID int, amountQuota int, amountMoney float64, accountInfo string) (*model.AgentWithdrawal, error) {
	if amountQuota <= 0 {
		return nil, ErrInsufficientAgentBalance
	}
	var created *model.AgentWithdrawal
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		if err := lockAgentBalance(tx, agentID); err != nil {
			return err
		}
		balance, err := getBalanceWithTx(tx, agentID)
		if err != nil {
			return err
		}
		if balance.AvailableQuota < amountQuota {
			return ErrInsufficientAgentBalance
		}
		withdrawal := &model.AgentWithdrawal{
			AgentId:     agentID,
			AmountQuota: amountQuota,
			AmountMoney: amountMoney,
			Status:      model.AgentWithdrawalStatusPending,
			AccountInfo: strings.TrimSpace(accountInfo),
		}
		if err := tx.Create(withdrawal).Error; err != nil {
			return err
		}
		created = withdrawal
		return nil
	})
	return created, err
}

func AddBalanceAmount(agentID int, operatorUserID int, amountMoney float64, remark string) (*Balance, error) {
	if amountMoney <= 0 || math.IsNaN(amountMoney) || math.IsInf(amountMoney, 0) {
		return nil, ErrInvalidAgentBalanceAmount
	}
	currency, err := getSettlementCurrencyWithTx(model.DB, agentID)
	if err != nil {
		return nil, err
	}
	amountQuota := settlementAmountToQuota(amountMoney, currency)
	if amountQuota <= 0 {
		return nil, ErrInvalidAgentBalanceAmount
	}
	err = model.DB.Transaction(func(tx *gorm.DB) error {
		if err := lockAgentBalance(tx, agentID); err != nil {
			return err
		}
		balance, err := getBalanceWithTx(tx, agentID)
		if err != nil {
			return err
		}
		return tx.Create(&model.AgentLedger{
			AgentId:        agentID,
			OperatorUserId: operatorUserID,
			Type:           model.AgentLedgerTypeAdjustment,
			ProfitQuota:    amountQuota,
			BalanceAfter:   balance.ProfitQuota + amountQuota,
			Remark:         strings.TrimSpace(remark),
		}).Error
	})
	if err != nil {
		return nil, err
	}
	return GetBalance(agentID)
}

func FundUserBalanceAmount(agentID int, operatorUserID int, userID int, amountMoney float64) (*Balance, error) {
	if amountMoney <= 0 || math.IsNaN(amountMoney) || math.IsInf(amountMoney, 0) {
		return nil, ErrInvalidAgentBalanceAmount
	}
	currency, err := getSettlementCurrencyWithTx(model.DB, agentID)
	if err != nil {
		return nil, err
	}
	amountQuota := settlementAmountToQuota(amountMoney, currency)
	return FundUserBalanceQuota(agentID, operatorUserID, userID, amountQuota)
}

func FundUserBalanceQuota(agentID int, operatorUserID int, userID int, amountQuota int) (*Balance, error) {
	if amountQuota <= 0 {
		return nil, ErrInvalidAgentBalanceAmount
	}
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		if err := lockAgentBalance(tx, agentID); err != nil {
			return err
		}
		var membership model.AgentUser
		if err := tx.Where("agent_id = ? AND user_id = ? AND status = ?", agentID, userID, model.AgentUserStatusEnabled).
			First(&membership).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return model.ErrAgentUserNotFound
			}
			return err
		}
		balance, err := getBalanceWithTx(tx, agentID)
		if err != nil {
			return err
		}
		if balance.AvailableQuota < amountQuota {
			return ErrInsufficientAgentBalance
		}
		result := tx.Model(&model.User{}).Where("id = ?", userID).
			Update("quota", gorm.Expr("quota + ?", amountQuota))
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return gorm.ErrRecordNotFound
		}
		return tx.Create(&model.AgentLedger{
			AgentId:        agentID,
			UserId:         userID,
			OperatorUserId: operatorUserID,
			Type:           model.AgentLedgerTypeUserTopup,
			ProfitQuota:    -amountQuota,
			BalanceAfter:   balance.ProfitQuota - amountQuota,
		}).Error
	})
	if err != nil {
		return nil, err
	}
	if err := model.InvalidateUserCache(userID); err != nil {
		common.SysLog("failed to invalidate agent-funded user quota cache: " + err.Error())
	}
	return GetBalance(agentID)
}

func SubmitWithdrawalAmount(agentID int, amountMoney float64, accountInfo string) (*model.AgentWithdrawal, error) {
	if amountMoney <= 0 {
		return nil, ErrInsufficientAgentBalance
	}
	currency, err := getSettlementCurrencyWithTx(model.DB, agentID)
	if err != nil {
		return nil, err
	}
	amountQuota := settlementAmountToQuota(amountMoney, currency)
	return SubmitWithdrawal(agentID, amountQuota, amountMoney, accountInfo)
}

func CompleteWithdrawal(withdrawalID int, status string, adminRemark string) error {
	status = strings.TrimSpace(status)
	if status != model.AgentWithdrawalStatusApproved &&
		status != model.AgentWithdrawalStatusRejected &&
		status != model.AgentWithdrawalStatusPaid {
		return ErrInvalidWithdrawalStatus
	}
	return model.DB.Transaction(func(tx *gorm.DB) error {
		var withdrawal model.AgentWithdrawal
		if err := tx.First(&withdrawal, "id = ?", withdrawalID).Error; err != nil {
			return err
		}
		if err := lockAgentBalance(tx, withdrawal.AgentId); err != nil {
			return err
		}
		if err := tx.First(&withdrawal, "id = ?", withdrawalID).Error; err != nil {
			return err
		}
		switch status {
		case model.AgentWithdrawalStatusApproved:
			if withdrawal.Status != model.AgentWithdrawalStatusPending {
				return ErrInvalidWithdrawalStatus
			}
			return tx.Model(&withdrawal).Updates(map[string]interface{}{
				"status":       model.AgentWithdrawalStatusApproved,
				"admin_remark": adminRemark,
				"processed_at": common.GetTimestamp(),
			}).Error
		case model.AgentWithdrawalStatusRejected:
			if withdrawal.Status != model.AgentWithdrawalStatusPending && withdrawal.Status != model.AgentWithdrawalStatusApproved {
				return ErrInvalidWithdrawalStatus
			}
			return tx.Model(&withdrawal).Updates(map[string]interface{}{
				"status":       model.AgentWithdrawalStatusRejected,
				"admin_remark": adminRemark,
				"processed_at": common.GetTimestamp(),
			}).Error
		case model.AgentWithdrawalStatusPaid:
			if withdrawal.Status != model.AgentWithdrawalStatusApproved {
				return ErrInvalidWithdrawalStatus
			}
			balance, err := getBalanceWithTx(tx, withdrawal.AgentId)
			if err != nil {
				return err
			}
			balanceAfter := balance.ProfitQuota - withdrawal.AmountQuota
			ledger := &model.AgentLedger{
				AgentId:      withdrawal.AgentId,
				Type:         model.AgentLedgerTypeWithdraw,
				ProfitQuota:  -withdrawal.AmountQuota,
				BalanceAfter: balanceAfter,
			}
			if err := tx.Create(ledger).Error; err != nil {
				return err
			}
			return tx.Model(&withdrawal).Updates(map[string]interface{}{
				"status":       model.AgentWithdrawalStatusPaid,
				"admin_remark": adminRemark,
				"processed_at": common.GetTimestamp(),
			}).Error
		}
		return ErrInvalidWithdrawalStatus
	})
}

func lockAgentBalance(tx *gorm.DB, agentID int) error {
	query := tx.Select("id").Where("id = ?", agentID)
	if !common.UsingSQLite {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var agent model.Agent
	return query.First(&agent).Error
}

func getBalanceWithTx(tx *gorm.DB, agentID int) (*Balance, error) {
	var profitQuota int
	if err := tx.Model(&model.AgentLedger{}).
		Select("COALESCE(SUM(profit_quota), 0)").
		Where("agent_id = ?", agentID).
		Scan(&profitQuota).Error; err != nil {
		return nil, err
	}
	var pendingQuota int
	if err := tx.Model(&model.AgentWithdrawal{}).
		Select("COALESCE(SUM(amount_quota), 0)").
		Where("agent_id = ? AND status = ?", agentID, model.AgentWithdrawalStatusPending).
		Scan(&pendingQuota).Error; err != nil {
		return nil, err
	}
	var approvedQuota int
	if err := tx.Model(&model.AgentWithdrawal{}).
		Select("COALESCE(SUM(amount_quota), 0)").
		Where("agent_id = ? AND status = ?", agentID, model.AgentWithdrawalStatusApproved).
		Scan(&approvedQuota).Error; err != nil {
		return nil, err
	}
	return &Balance{
		ProfitQuota:             profitQuota,
		PendingWithdrawalQuota:  pendingQuota,
		ApprovedWithdrawalQuota: approvedQuota,
		AvailableQuota:          profitQuota - pendingQuota - approvedQuota,
	}, nil
}

func getSettlementCurrencyWithTx(tx *gorm.DB, agentID int) (string, error) {
	var agent model.Agent
	err := tx.Select("settlement_currency").Where("id = ?", agentID).First(&agent).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "USD", nil
		}
		return "", err
	}
	return normalizeSettlementCurrency(agent.SettlementCurrency), nil
}

func normalizeSettlementCurrency(currency string) string {
	switch strings.ToUpper(strings.TrimSpace(currency)) {
	case "CNY", "RMB":
		return "RMB"
	default:
		return "USD"
	}
}

func settlementCurrencyRate(currency string) float64 {
	if normalizeSettlementCurrency(currency) == "RMB" && operation_setting.USDExchangeRate > 0 {
		return operation_setting.USDExchangeRate
	}
	return 1
}

func quotaToSettlementAmount(quota int, currency string) float64 {
	return float64(quota) / common.QuotaPerUnit * settlementCurrencyRate(currency)
}

func settlementAmountToQuota(amount float64, currency string) int {
	return int(math.Ceil(amount / settlementCurrencyRate(currency) * common.QuotaPerUnit))
}

func fillBalanceAmounts(balance *Balance, currency string) {
	balance.Currency = normalizeSettlementCurrency(currency)
	balance.ProfitAmount = quotaToSettlementAmount(balance.ProfitQuota, currency)
	balance.PendingWithdrawalAmount = quotaToSettlementAmount(balance.PendingWithdrawalQuota, currency)
	balance.ApprovedWithdrawalAmount = quotaToSettlementAmount(balance.ApprovedWithdrawalQuota, currency)
	balance.AvailableAmount = quotaToSettlementAmount(balance.AvailableQuota, currency)
}

func buildLedgerView(ledger *model.AgentLedger, currency string) *LedgerView {
	normalizedCurrency := normalizeSettlementCurrency(currency)
	return &LedgerView{
		Id:                 ledger.Id,
		AgentId:            ledger.AgentId,
		UserId:             ledger.UserId,
		LogId:              ledger.LogId,
		OperatorUserId:     ledger.OperatorUserId,
		Type:               ledger.Type,
		BaseQuota:          ledger.BaseQuota,
		ChargedQuota:       ledger.ChargedQuota,
		ProfitQuota:        ledger.ProfitQuota,
		BalanceAfter:       ledger.BalanceAfter,
		Currency:           normalizedCurrency,
		BaseAmount:         quotaToSettlementAmount(ledger.BaseQuota, normalizedCurrency),
		ChargedAmount:      quotaToSettlementAmount(ledger.ChargedQuota, normalizedCurrency),
		ProfitAmount:       quotaToSettlementAmount(ledger.ProfitQuota, normalizedCurrency),
		BalanceAfterAmount: quotaToSettlementAmount(ledger.BalanceAfter, normalizedCurrency),
		Remark:             ledger.Remark,
		CreatedAt:          ledger.CreatedAt,
	}
}

func buildWithdrawalView(withdrawal *model.AgentWithdrawal, currency string) *WithdrawalView {
	normalizedCurrency := normalizeSettlementCurrency(currency)
	settlementAmount := withdrawal.AmountMoney
	if settlementAmount <= 0 {
		settlementAmount = quotaToSettlementAmount(withdrawal.AmountQuota, normalizedCurrency)
	}
	return &WithdrawalView{
		Id:               withdrawal.Id,
		AgentId:          withdrawal.AgentId,
		AmountQuota:      withdrawal.AmountQuota,
		AmountMoney:      settlementAmount,
		SettlementAmount: settlementAmount,
		Currency:         normalizedCurrency,
		Fee:              withdrawal.Fee,
		Status:           withdrawal.Status,
		AccountInfo:      withdrawal.AccountInfo,
		AdminRemark:      withdrawal.AdminRemark,
		CreatedAt:        withdrawal.CreatedAt,
		ProcessedAt:      withdrawal.ProcessedAt,
	}
}

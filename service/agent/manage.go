package agent

import (
	"errors"
	"math"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"gorm.io/gorm"
)

var (
	ErrInvalidAgentDomain         = errors.New("invalid agent domain")
	ErrAgentDomainAlreadyExists   = errors.New("agent domain already exists")
	ErrInvalidAgentGroupRatio     = errors.New("agent group ratio must be not less than 0")
	ErrAgentGroupRatioNotFound    = errors.New("agent group ratio not found")
	ErrAgentGroupRatioBelowSystem = errors.New("agent group ratio cannot be lower than system group ratio")
	ErrAgentSystemGroupNotFound   = errors.New("agent system group not found")
	ErrInsufficientAgentBalance   = errors.New("insufficient agent balance")
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
	SystemRatio     float64 `json:"system_ratio"`
	ConfiguredRatio float64 `json:"configured_ratio"`
	EffectiveRatio  float64 `json:"effective_ratio"`
	Configured      bool    `json:"configured"`
	Visible         bool    `json:"visible"`
	Available       bool    `json:"available"`
}

type UserGroupConfigView struct {
	GroupName     string   `json:"group_name"`
	VisibleGroups []string `json:"visible_groups"`
}

type LedgerView struct {
	Id                 int     `json:"id"`
	AgentId            int     `json:"agent_id"`
	UserId             int     `json:"user_id"`
	LogId              int     `json:"log_id"`
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

func EffectiveGroupRatioMap(agentID int) (map[string]float64, error) {
	groupMap, err := EffectiveGroupMap(agentID)
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

func EffectiveGroupMap(agentID int) (map[string]types.AgentGroup, error) {
	configuredGroups, err := model.ListAgentGroupRatios(agentID)
	if err != nil {
		return nil, err
	}
	systemRatios := ratio_setting.GetGroupRatioCopy()
	result := make(map[string]types.AgentGroup, len(configuredGroups))
	for _, configured := range configuredGroups {
		group := buildAgentGroup(configured, systemRatios)
		if !group.Available {
			continue
		}
		result[group.GroupName] = group
	}
	return result, nil
}

func ListGroupRatios(agentID int) ([]GroupRatioView, error) {
	systemRatios := ratio_setting.GetGroupRatioCopy()
	configuredGroups, err := model.ListAgentGroupRatios(agentID)
	if err != nil {
		return nil, err
	}
	views := make([]GroupRatioView, 0, len(configuredGroups)+len(systemRatios))
	configuredNames := make(map[string]struct{}, len(configuredGroups))
	for _, configured := range configuredGroups {
		group := buildAgentGroup(configured, systemRatios)
		configuredNames[group.GroupName] = struct{}{}
		views = append(views, GroupRatioView{
			GroupName:       group.GroupName,
			SystemGroupName: group.SystemGroupName,
			Description:     group.Description,
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
		views = append(views, GroupRatioView{
			GroupName:       systemGroupName,
			SystemGroupName: systemGroupName,
			Description:     "",
			SystemRatio:     systemRatio,
			ConfiguredRatio: 0,
			EffectiveRatio:  systemRatio,
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
		})
	}
	return views, nil
}

func UpsertUserGroupConfig(agentID int, groupName string, visibleGroups []string) (*model.AgentUserGroupConfig, error) {
	groupName = strings.TrimSpace(groupName)
	visibleGroups = normalizeVisibleGroupsForGroup(groupName, visibleGroups)
	if groupName == "" {
		return nil, ErrAgentUserGroupNotAllowed
	}
	var config model.AgentUserGroupConfig
	err := model.DB.Where("agent_id = ? AND group_name = ?", agentID, groupName).First(&config).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		config = model.AgentUserGroupConfig{
			AgentId:       agentID,
			GroupName:     groupName,
			VisibleGroups: strings.Join(visibleGroups, ","),
		}
		err = model.DB.Create(&config).Error
	} else {
		err = model.DB.Model(&config).Updates(map[string]interface{}{
			"visible_groups": strings.Join(visibleGroups, ","),
		}).Error
	}
	if err != nil {
		return nil, err
	}
	return &config, nil
}

func UpsertGroupRatio(agentID int, groupName string, systemGroupName string, description string, ratio float64, visible bool) (*model.AgentGroupRatio, error) {
	groupName = strings.TrimSpace(groupName)
	systemGroupName = strings.TrimSpace(systemGroupName)
	description = strings.TrimSpace(description)
	if groupName == "" {
		return nil, ErrAgentGroupRatioNotFound
	}
	if systemGroupName == "" {
		systemGroupName = groupName
	}
	if !ratio_setting.ContainsGroupRatio(systemGroupName) {
		return nil, ErrAgentSystemGroupNotFound
	}
	systemRatio := ratio_setting.GetGroupRatio(systemGroupName)
	if ratio < 0 {
		return nil, ErrInvalidAgentGroupRatio
	}
	if ratio < systemRatio {
		return nil, ErrAgentGroupRatioBelowSystem
	}
	var groupRatio model.AgentGroupRatio
	err := model.DB.Where("agent_id = ? AND group_name = ?", agentID, groupName).First(&groupRatio).Error
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

func buildAgentGroup(configured *model.AgentGroupRatio, systemRatios map[string]float64) types.AgentGroup {
	systemGroupName := strings.TrimSpace(configured.SystemGroupName)
	if systemGroupName == "" {
		systemGroupName = strings.TrimSpace(configured.GroupName)
	}
	systemRatio, available := systemRatios[systemGroupName]
	effectiveRatio := configured.Ratio
	if !available {
		effectiveRatio = 0
	} else if effectiveRatio < systemRatio {
		effectiveRatio = systemRatio
	}
	return types.AgentGroup{
		GroupName:       configured.GroupName,
		SystemGroupName: systemGroupName,
		Description:     strings.TrimSpace(configured.Description),
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

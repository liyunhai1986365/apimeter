package agent

import (
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"gorm.io/gorm"
)

var (
	ErrInvalidAgentDomain       = errors.New("invalid agent domain")
	ErrInvalidAgentMarkup       = errors.New("agent markup must be greater than 0")
	ErrInsufficientAgentBalance = errors.New("insufficient agent balance")
	ErrInvalidWithdrawalStatus  = errors.New("invalid withdrawal status")
)

type Balance struct {
	ProfitQuota             int `json:"profit_quota"`
	PendingWithdrawalQuota  int `json:"pending_withdrawal_quota"`
	ApprovedWithdrawalQuota int `json:"approved_withdrawal_quota"`
	AvailableQuota          int `json:"available_quota"`
}

func GetBalance(agentID int) (*Balance, error) {
	var profitQuota int
	if err := model.DB.Model(&model.AgentLedger{}).
		Select("COALESCE(SUM(profit_quota), 0)").
		Where("agent_id = ?", agentID).
		Scan(&profitQuota).Error; err != nil {
		return nil, err
	}

	var pendingQuota int
	if err := model.DB.Model(&model.AgentWithdrawal{}).
		Select("COALESCE(SUM(amount_quota), 0)").
		Where("agent_id = ? AND status = ?", agentID, model.AgentWithdrawalStatusPending).
		Scan(&pendingQuota).Error; err != nil {
		return nil, err
	}

	var approvedQuota int
	if err := model.DB.Model(&model.AgentWithdrawal{}).
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

func CreateDomain(agentID int, rawDomain string) (*model.AgentDomain, error) {
	domain := NormalizeHost(rawDomain)
	if domain == "" || !strings.Contains(domain, ".") {
		return nil, ErrInvalidAgentDomain
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
		return nil, err
	}
	FillDomainCNAMETarget(agentDomain)
	return agentDomain, nil
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

func UpsertPricingRule(agentID int, modelPattern string, markup float64, enabled bool) (*model.AgentPricingRule, error) {
	modelPattern = strings.TrimSpace(modelPattern)
	if modelPattern == "" {
		modelPattern = "*"
	}
	if markup <= 0 {
		return nil, ErrInvalidAgentMarkup
	}
	var rule model.AgentPricingRule
	err := model.DB.Where("agent_id = ? AND model_pattern = ?", agentID, modelPattern).First(&rule).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		rule = model.AgentPricingRule{
			AgentId:      agentID,
			ModelPattern: modelPattern,
			Markup:       markup,
			Enabled:      enabled,
		}
		err = model.DB.Create(&rule).Error
	} else {
		err = model.DB.Model(&rule).Updates(map[string]interface{}{
			"markup":  markup,
			"enabled": enabled,
		}).Error
	}
	if err != nil {
		return nil, err
	}
	return &rule, nil
}

func ListPricingRules(agentID int, startIdx int, num int) ([]*model.AgentPricingRule, int64, error) {
	return model.ListAgentPricingRules(agentID, startIdx, num)
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

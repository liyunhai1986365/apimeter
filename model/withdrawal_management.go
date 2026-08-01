package model

import (
	"errors"
	"strings"
)

const (
	WithdrawalSourceUser  = "user"
	WithdrawalSourceAgent = "agent"
)

var ErrInvalidWithdrawalSource = errors.New("invalid withdrawal source")

type WithdrawalManagementItem struct {
	Id            int     `json:"id"`
	Source        string  `json:"source"`
	ApplicantId   int     `json:"applicant_id"`
	ApplicantName string  `json:"applicant_name"`
	OwnerUserId   int     `json:"owner_user_id"`
	AmountQuota   int     `json:"amount_quota"`
	AmountMoney   float64 `json:"amount_money"`
	Currency      string  `json:"currency"`
	Fee           float64 `json:"fee"`
	Status        string  `json:"status"`
	AccountInfo   string  `json:"account_info"`
	AdminRemark   string  `json:"admin_remark"`
	CreatedAt     int64   `json:"created_at"`
	ProcessedAt   int64   `json:"processed_at"`
}

func ListWithdrawalManagement(source string, status string, startIdx int, pageSize int) ([]WithdrawalManagementItem, int64, error) {
	source = strings.TrimSpace(source)
	status = strings.TrimSpace(status)
	if source != "" && source != WithdrawalSourceUser && source != WithdrawalSourceAgent {
		return nil, 0, ErrInvalidWithdrawalSource
	}

	queries := make([]string, 0, 2)
	args := make([]any, 0, 4)
	var total int64

	if source == "" || source == WithdrawalSourceUser {
		query := `SELECT affiliate_withdrawals.id AS id,
			'user' AS source,
			affiliate_withdrawals.user_id AS applicant_id,
			COALESCE(NULLIF(users.display_name, ''), users.username, '') AS applicant_name,
			affiliate_withdrawals.user_id AS owner_user_id,
			affiliate_withdrawals.amount_quota AS amount_quota,
			0 AS amount_money,
			'' AS currency,
			0 AS fee,
			affiliate_withdrawals.status AS status,
			affiliate_withdrawals.account_info AS account_info,
			affiliate_withdrawals.admin_remark AS admin_remark,
			affiliate_withdrawals.created_at AS created_at,
			affiliate_withdrawals.processed_at AS processed_at
		FROM affiliate_withdrawals
		LEFT JOIN users ON users.id = affiliate_withdrawals.user_id`
		countQuery := DB.Model(&AffiliateWithdrawal{})
		if status != "" {
			query += " WHERE affiliate_withdrawals.status = ?"
			args = append(args, status)
			countQuery = countQuery.Where("status = ?", status)
		}
		var count int64
		if err := countQuery.Count(&count).Error; err != nil {
			return nil, 0, err
		}
		total += count
		queries = append(queries, query)
	}

	if source == "" || source == WithdrawalSourceAgent {
		query := `SELECT agent_withdrawals.id AS id,
			'agent' AS source,
			agent_withdrawals.agent_id AS applicant_id,
			COALESCE(NULLIF(agents.name, ''), agents.slug, '') AS applicant_name,
			COALESCE(agents.owner_user_id, 0) AS owner_user_id,
			agent_withdrawals.amount_quota AS amount_quota,
			agent_withdrawals.amount_money AS amount_money,
			COALESCE(NULLIF(agents.settlement_currency, ''), 'USD') AS currency,
			agent_withdrawals.fee AS fee,
			agent_withdrawals.status AS status,
			agent_withdrawals.account_info AS account_info,
			agent_withdrawals.admin_remark AS admin_remark,
			agent_withdrawals.created_at AS created_at,
			agent_withdrawals.processed_at AS processed_at
		FROM agent_withdrawals
		LEFT JOIN agents ON agents.id = agent_withdrawals.agent_id`
		countQuery := DB.Model(&AgentWithdrawal{})
		if status != "" {
			query += " WHERE agent_withdrawals.status = ?"
			args = append(args, status)
			countQuery = countQuery.Where("status = ?", status)
		}
		var count int64
		if err := countQuery.Count(&count).Error; err != nil {
			return nil, 0, err
		}
		total += count
		queries = append(queries, query)
	}

	items := make([]WithdrawalManagementItem, 0)
	query := "SELECT * FROM (" + strings.Join(queries, " UNION ALL ") + ") AS withdrawal_items " +
		"ORDER BY created_at DESC, id DESC, source ASC LIMIT ? OFFSET ?"
	args = append(args, pageSize, startIdx)
	if err := DB.Raw(query, args...).Scan(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

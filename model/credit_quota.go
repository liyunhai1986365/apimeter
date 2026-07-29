package model

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

const (
	CreditQuotaOperationGrant = "grant"
	CreditQuotaOperationRepay = "repay"
)

var (
	ErrCreditQuotaInvalidAmount       = errors.New("信控金额必须大于 0")
	ErrCreditQuotaInvalidOperation    = errors.New("不支持的信控操作")
	ErrCreditQuotaRepaymentExceedsDue = errors.New("还款金额不能超过待还信控额度")
	ErrCreditQuotaNumericOverflow     = errors.New("信控金额超出服务器整数范围")
	ErrCreditQuotaRemarkTooLong       = errors.New("信控备注不能超过 255 个字符")
)

// CreditQuotaRecord is the transactional audit trail for credit grants and
// repayments. Granting credit increases both the wallet balance and the
// outstanding credit amount. Repayment only reduces outstanding credit.
type CreditQuotaRecord struct {
	Id               int    `json:"id"`
	UserId           int    `json:"user_id" gorm:"index:idx_credit_quota_user_time,priority:1;index"`
	OperatorUserId   int    `json:"operator_user_id" gorm:"index"`
	OperatorUsername string `json:"operator_username" gorm:"type:varchar(64)"`
	Operation        string `json:"operation" gorm:"type:varchar(16);index"`
	Amount           int    `json:"amount" gorm:"type:bigint"`
	CreditBefore     int    `json:"credit_before" gorm:"type:bigint"`
	CreditAfter      int    `json:"credit_after" gorm:"type:bigint"`
	BalanceBefore    int    `json:"balance_before" gorm:"type:bigint"`
	BalanceAfter     int    `json:"balance_after" gorm:"type:bigint"`
	Remark           string `json:"remark" gorm:"type:varchar(255)"`
	CreatedAt        int64  `json:"created_at" gorm:"autoCreateTime;column:created_at;index:idx_credit_quota_user_time,priority:2"`
}

// ManageUserCreditQuota applies one credit-control operation atomically.
//
//   - grant: quota += amount, credit_quota += amount
//   - repay: credit_quota -= amount, quota stays unchanged
func ManageUserCreditQuota(
	userId int,
	operatorUserId int,
	operatorUsername string,
	operation string,
	amount int,
	remark string,
) (*User, *CreditQuotaRecord, error) {
	if amount <= 0 {
		return nil, nil, ErrCreditQuotaInvalidAmount
	}
	if operation != CreditQuotaOperationGrant && operation != CreditQuotaOperationRepay {
		return nil, nil, ErrCreditQuotaInvalidOperation
	}
	remark = strings.TrimSpace(remark)
	if utf8.RuneCountInString(remark) > 255 {
		return nil, nil, ErrCreditQuotaRemarkTooLong
	}

	var user User
	var record CreditQuotaRecord
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := lockForUpdate(tx).Where("id = ?", userId).First(&user).Error; err != nil {
			return err
		}

		record = CreditQuotaRecord{
			UserId:           userId,
			OperatorUserId:   operatorUserId,
			OperatorUsername: strings.TrimSpace(operatorUsername),
			Operation:        operation,
			Amount:           amount,
			CreditBefore:     user.CreditQuota,
			BalanceBefore:    user.Quota,
			Remark:           remark,
		}

		switch operation {
		case CreditQuotaOperationGrant:
			if user.CreditQuota > int(^uint(0)>>1)-amount || user.Quota > int(^uint(0)>>1)-amount {
				return ErrCreditQuotaNumericOverflow
			}
			user.CreditQuota += amount
			user.Quota += amount
		case CreditQuotaOperationRepay:
			if amount > user.CreditQuota {
				return ErrCreditQuotaRepaymentExceedsDue
			}
			user.CreditQuota -= amount
		}

		record.CreditAfter = user.CreditQuota
		record.BalanceAfter = user.Quota
		if err := tx.Model(&User{}).Where("id = ?", user.Id).Updates(map[string]interface{}{
			"quota":        user.Quota,
			"credit_quota": user.CreditQuota,
		}).Error; err != nil {
			return fmt.Errorf("更新用户信控额度失败: %w", err)
		}
		if err := tx.Create(&record).Error; err != nil {
			return fmt.Errorf("记录信控操作失败: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	if err := InvalidateUserCache(userId); err != nil {
		common.SysLog(fmt.Sprintf("failed to invalidate user cache after credit quota operation for user %d: %s", userId, err.Error()))
	}
	return &user, &record, nil
}

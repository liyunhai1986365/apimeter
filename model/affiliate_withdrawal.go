package model

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	AffiliateWithdrawalStatusPending   = "pending"
	AffiliateWithdrawalStatusApproved  = "approved"
	AffiliateWithdrawalStatusPaid      = "paid"
	AffiliateWithdrawalStatusRejected  = "rejected"
	AffiliateWithdrawalStatusCancelled = "cancelled"
)

var (
	ErrAffiliateWithdrawalAmount  = errors.New("invalid affiliate withdrawal amount")
	ErrAffiliateWithdrawalBalance = errors.New("insufficient affiliate reward balance")
	ErrAffiliateWithdrawalAccount = errors.New("withdrawal account is required")
	ErrAffiliateWithdrawalStatus  = errors.New("invalid affiliate withdrawal status")
)

type AffiliateWithdrawal struct {
	Id          int    `json:"id"`
	UserId      int    `json:"user_id" gorm:"index;column:user_id"`
	AmountQuota int    `json:"amount_quota" gorm:"type:int;default:0;column:amount_quota"`
	Status      string `json:"status" gorm:"type:varchar(32);default:'pending';index"`
	AccountInfo string `json:"account_info" gorm:"type:text;column:account_info"`
	AdminRemark string `json:"admin_remark" gorm:"type:varchar(255);column:admin_remark"`
	CreatedAt   int64  `json:"created_at" gorm:"autoCreateTime;column:created_at"`
	ProcessedAt int64  `json:"processed_at" gorm:"bigint;default:0;column:processed_at"`
}

func creditAffiliateRewardAccount(tx *gorm.DB, userId int, rewardQuota int64) error {
	if userId <= 0 || rewardQuota <= 0 {
		return nil
	}
	return tx.Model(&User{}).Where("id = ?", userId).Updates(map[string]interface{}{
		"aff_quota":   gorm.Expr("aff_quota + ?", rewardQuota),
		"aff_history": gorm.Expr("aff_history + ?", rewardQuota),
	}).Error
}

func SubmitAffiliateWithdrawal(userId int, amountQuota int, accountInfo string) (*AffiliateWithdrawal, error) {
	accountInfo = strings.TrimSpace(accountInfo)
	if amountQuota < int(common.QuotaPerUnit) {
		return nil, ErrAffiliateWithdrawalAmount
	}
	if accountInfo == "" || utf8.RuneCountInString(accountInfo) > 2000 {
		return nil, ErrAffiliateWithdrawalAccount
	}

	var created *AffiliateWithdrawal
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := lockAffiliateRewardAccount(tx, userId); err != nil {
			return err
		}
		result := tx.Model(&User{}).
			Where("id = ? AND aff_quota >= ?", userId, amountQuota).
			UpdateColumn("aff_quota", gorm.Expr("aff_quota - ?", amountQuota))
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrAffiliateWithdrawalBalance
		}

		withdrawal := &AffiliateWithdrawal{
			UserId:      userId,
			AmountQuota: amountQuota,
			Status:      AffiliateWithdrawalStatusPending,
			AccountInfo: accountInfo,
		}
		if err := tx.Create(withdrawal).Error; err != nil {
			return err
		}
		created = withdrawal
		return nil
	})
	if err != nil {
		return nil, err
	}
	if err := InvalidateUserCache(userId); err != nil {
		common.SysLog("failed to invalidate affiliate withdrawal user cache: " + err.Error())
	}
	return created, nil
}

func ListAffiliateWithdrawals(userId int, startIdx int, pageSize int) ([]AffiliateWithdrawal, int64, error) {
	var withdrawals []AffiliateWithdrawal
	var total int64
	query := DB.Model(&AffiliateWithdrawal{})
	if userId > 0 {
		query = query.Where("user_id = ?", userId)
	}
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := query.Order("id DESC").Limit(pageSize).Offset(startIdx).Find(&withdrawals).Error; err != nil {
		return nil, 0, err
	}
	return withdrawals, total, nil
}

func CancelAffiliateWithdrawal(userId int, withdrawalId int) error {
	return updateAffiliateWithdrawal(withdrawalId, userId, AffiliateWithdrawalStatusCancelled, "")
}

func CompleteAffiliateWithdrawal(withdrawalId int, status string, adminRemark string) error {
	status = strings.TrimSpace(status)
	if status != AffiliateWithdrawalStatusApproved &&
		status != AffiliateWithdrawalStatusRejected &&
		status != AffiliateWithdrawalStatusPaid {
		return ErrAffiliateWithdrawalStatus
	}
	if utf8.RuneCountInString(adminRemark) > 255 {
		return fmt.Errorf("admin remark is too long")
	}
	return updateAffiliateWithdrawal(withdrawalId, 0, status, strings.TrimSpace(adminRemark))
}

func updateAffiliateWithdrawal(withdrawalId int, userId int, status string, adminRemark string) error {
	var affectedUserId int
	err := DB.Transaction(func(tx *gorm.DB) error {
		query := tx.Where("id = ?", withdrawalId)
		if userId > 0 {
			query = query.Where("user_id = ?", userId)
		}
		if !common.UsingSQLite {
			query = query.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		var withdrawal AffiliateWithdrawal
		if err := query.First(&withdrawal).Error; err != nil {
			return err
		}
		affectedUserId = withdrawal.UserId

		now := common.GetTimestamp()
		switch status {
		case AffiliateWithdrawalStatusApproved:
			if withdrawal.Status != AffiliateWithdrawalStatusPending {
				return ErrAffiliateWithdrawalStatus
			}
		case AffiliateWithdrawalStatusPaid:
			if withdrawal.Status != AffiliateWithdrawalStatusApproved {
				return ErrAffiliateWithdrawalStatus
			}
		case AffiliateWithdrawalStatusRejected:
			if withdrawal.Status != AffiliateWithdrawalStatusPending && withdrawal.Status != AffiliateWithdrawalStatusApproved {
				return ErrAffiliateWithdrawalStatus
			}
			if err := tx.Model(&User{}).Where("id = ?", withdrawal.UserId).
				UpdateColumn("aff_quota", gorm.Expr("aff_quota + ?", withdrawal.AmountQuota)).Error; err != nil {
				return err
			}
		case AffiliateWithdrawalStatusCancelled:
			if withdrawal.Status != AffiliateWithdrawalStatusPending || userId <= 0 {
				return ErrAffiliateWithdrawalStatus
			}
			if err := tx.Model(&User{}).Where("id = ?", withdrawal.UserId).
				UpdateColumn("aff_quota", gorm.Expr("aff_quota + ?", withdrawal.AmountQuota)).Error; err != nil {
				return err
			}
		default:
			return ErrAffiliateWithdrawalStatus
		}

		return tx.Model(&withdrawal).Updates(map[string]interface{}{
			"status":       status,
			"admin_remark": adminRemark,
			"processed_at": now,
		}).Error
	})
	if err != nil {
		return err
	}
	if affectedUserId > 0 {
		if err := InvalidateUserCache(affectedUserId); err != nil {
			common.SysLog("failed to invalidate affiliate withdrawal user cache: " + err.Error())
		}
	}
	return nil
}

func lockAffiliateRewardAccount(tx *gorm.DB, userId int) error {
	query := tx.Select("id").Where("id = ?", userId)
	if !common.UsingSQLite {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var user User
	return query.First(&user).Error
}

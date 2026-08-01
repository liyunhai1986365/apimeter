package model

import (
	"errors"
	"time"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	StripeAutoRechargeStateIdle       = "idle"
	StripeAutoRechargeStateProcessing = "processing"
	StripeAutoRechargeStatePending    = "pending"
	StripeAutoRechargeStateFailed     = "failed"
	StripeAutoRechargeStateAction     = "action_required"
)

var ErrStripeAutoRechargeBusy = errors.New("stripe auto recharge is already processing")

// StripeAutoRecharge stores only Stripe identifiers and display-safe card
// metadata. Raw card data never enters new-api.
type StripeAutoRecharge struct {
	Id                     int    `json:"id"`
	UserId                 int    `json:"user_id" gorm:"uniqueIndex;not null"`
	StripeCustomerId       string `json:"-" gorm:"type:varchar(128);not null"`
	StripePaymentMethodId  string `json:"-" gorm:"type:varchar(128);not null"`
	CardBrand              string `json:"card_brand" gorm:"type:varchar(32);not null"`
	CardLast4              string `json:"card_last4" gorm:"type:varchar(4);not null"`
	CardExpMonth           int64  `json:"card_exp_month"`
	CardExpYear            int64  `json:"card_exp_year"`
	Enabled                bool   `json:"enabled" gorm:"not null;default:false;index"`
	ThresholdQuota         int64  `json:"-" gorm:"not null;default:0"`
	TopUpAmount            int64  `json:"topup_amount" gorm:"not null;default:0"`
	ConsentAt              int64  `json:"consent_at"`
	ConsentVersion         string `json:"-" gorm:"type:varchar(32);not null;default:''"`
	State                  string `json:"state" gorm:"type:varchar(32);not null;default:'idle';index"`
	LastError              string `json:"last_error,omitempty" gorm:"type:text"`
	PendingTradeNo         string `json:"-" gorm:"type:varchar(255);index"`
	PendingPaymentIntentId string `json:"-" gorm:"type:varchar(128);index"`
	PendingAmountCents     int64  `json:"-"`
	LastAttemptAt          int64  `json:"last_attempt_at"`
	LastSuccessAt          int64  `json:"last_success_at"`
	NextRetryAt            int64  `json:"-" gorm:"index"`
	CreatedAt              int64  `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt              int64  `json:"updated_at" gorm:"autoUpdateTime"`
}

func GetStripeAutoRechargeByUserId(userId int) (*StripeAutoRecharge, error) {
	config := &StripeAutoRecharge{}
	err := DB.Where("user_id = ?", userId).First(config).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return config, err
}

func GetStripeAutoRechargeByTradeNo(tradeNo string) (*StripeAutoRecharge, error) {
	config := &StripeAutoRecharge{}
	err := DB.Where("pending_trade_no = ?", tradeNo).First(config).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return config, err
}

func UpsertStripeAutoRechargePaymentMethod(config *StripeAutoRecharge) error {
	if config == nil || config.UserId <= 0 || config.StripeCustomerId == "" || config.StripePaymentMethodId == "" {
		return errors.New("invalid stripe auto recharge payment method")
	}
	if config.State == "" {
		config.State = StripeAutoRechargeStateIdle
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		existing := &StripeAutoRecharge{}
		err := lockForUpdate(tx).Where("user_id = ?", config.UserId).First(existing).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err == nil && (existing.PendingTradeNo != "" || existing.State == StripeAutoRechargeStateProcessing || existing.State == StripeAutoRechargeStatePending) {
			return ErrStripeAutoRechargeBusy
		}
		return tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "user_id"}},
			DoUpdates: clause.Assignments(map[string]interface{}{
				"stripe_customer_id":        config.StripeCustomerId,
				"stripe_payment_method_id":  config.StripePaymentMethodId,
				"card_brand":                config.CardBrand,
				"card_last4":                config.CardLast4,
				"card_exp_month":            config.CardExpMonth,
				"card_exp_year":             config.CardExpYear,
				"enabled":                   false,
				"state":                     StripeAutoRechargeStateIdle,
				"last_error":                "",
				"pending_trade_no":          "",
				"pending_payment_intent_id": "",
				"pending_amount_cents":      0,
				"next_retry_at":             0,
			}),
		}).Create(config).Error
	})
}

func UpdateStripeAutoRechargeSettings(userId int, enabled bool, thresholdQuota int64, topUpAmount int64, consentVersion string) error {
	updates := map[string]interface{}{
		"enabled":         enabled,
		"threshold_quota": thresholdQuota,
		"top_up_amount":   topUpAmount,
		"state":           StripeAutoRechargeStateIdle,
		"last_error":      "",
		"next_retry_at":   0,
	}
	if enabled {
		updates["consent_at"] = common.GetTimestamp()
		updates["consent_version"] = consentVersion
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		config := &StripeAutoRecharge{}
		if err := lockForUpdate(tx).Where("user_id = ? AND stripe_payment_method_id <> ''", userId).First(config).Error; err != nil {
			return err
		}
		if config.PendingTradeNo != "" || config.State == StripeAutoRechargeStateProcessing || config.State == StripeAutoRechargeStatePending {
			return ErrStripeAutoRechargeBusy
		}
		return tx.Model(config).Updates(updates).Error
	})
}

func DeleteStripeAutoRecharge(userId int) error {
	return DB.Where("user_id = ?", userId).Delete(&StripeAutoRecharge{}).Error
}

// ListDueStripeAutoRecharges compares the authoritative database wallet
// balance against the stored quota threshold. It intentionally does not rely
// on Redis so a stale cache cannot create a charge.
func ListDueStripeAutoRecharges(limit int) ([]*StripeAutoRecharge, error) {
	if limit <= 0 {
		limit = 100
	}
	now := time.Now().Unix()
	staleProcessing := now - 10*60
	configs := make([]*StripeAutoRecharge, 0)
	err := DB.Table("stripe_auto_recharges").
		Select("stripe_auto_recharges.*").
		Joins("JOIN users ON users.id = stripe_auto_recharges.user_id").
		Where("stripe_auto_recharges.enabled = ?", true).
		Where("stripe_auto_recharges.stripe_payment_method_id <> ''").
		Where("stripe_auto_recharges.threshold_quota > 0 AND users.quota < stripe_auto_recharges.threshold_quota").
		Where("stripe_auto_recharges.pending_trade_no = ''").
		Where("stripe_auto_recharges.next_retry_at <= ?", now).
		Where("stripe_auto_recharges.state IN ? OR (stripe_auto_recharges.state = ? AND stripe_auto_recharges.updated_at < ?)",
			[]string{StripeAutoRechargeStateIdle, StripeAutoRechargeStateFailed}, StripeAutoRechargeStateProcessing, staleProcessing).
		Order("stripe_auto_recharges.id ASC").
		Limit(limit).
		Find(&configs).Error
	return configs, err
}

func ClaimStripeAutoRecharge(id int) (bool, error) {
	now := time.Now().Unix()
	staleProcessing := now - 10*60
	result := DB.Model(&StripeAutoRecharge{}).
		Where("id = ? AND enabled = ? AND pending_trade_no = ''", id, true).
		Where("state IN ? OR (state = ? AND updated_at < ?)",
			[]string{StripeAutoRechargeStateIdle, StripeAutoRechargeStateFailed}, StripeAutoRechargeStateProcessing, staleProcessing).
		Updates(map[string]interface{}{
			"state":           StripeAutoRechargeStateProcessing,
			"last_attempt_at": now,
			"last_error":      "",
		})
	return result.RowsAffected == 1, result.Error
}

func SetStripeAutoRechargePending(id int, tradeNo string, paymentIntentId string, amountCents int64) error {
	return DB.Model(&StripeAutoRecharge{}).
		Where("id = ? AND state IN ? AND (pending_trade_no = '' OR pending_trade_no = ?)", id,
			[]string{StripeAutoRechargeStateProcessing, StripeAutoRechargeStatePending}, tradeNo).
		Updates(map[string]interface{}{
			"state":                     StripeAutoRechargeStatePending,
			"pending_trade_no":          tradeNo,
			"pending_payment_intent_id": paymentIntentId,
			"pending_amount_cents":      amountCents,
			"last_error":                "",
		}).Error
}

func ReleaseStripeAutoRechargeClaim(id int) error {
	return DB.Model(&StripeAutoRecharge{}).
		Where("id = ? AND state = ? AND pending_trade_no = ''", id, StripeAutoRechargeStateProcessing).
		Updates(map[string]interface{}{
			"state":      StripeAutoRechargeStateIdle,
			"last_error": "",
		}).Error
}

func SuspendStripeAutoRechargePending(id int, tradeNo string, paymentIntentId string, message string) error {
	return DB.Model(&StripeAutoRecharge{}).Where("id = ? AND pending_trade_no = ?", id, tradeNo).Updates(map[string]interface{}{
		"enabled":                   false,
		"state":                     StripeAutoRechargeStatePending,
		"last_error":                message,
		"pending_payment_intent_id": paymentIntentId,
	}).Error
}

func FailStripeAutoRecharge(id int, tradeNo string, message string, disable bool) error {
	updates := map[string]interface{}{
		"state":                     StripeAutoRechargeStateFailed,
		"last_error":                message,
		"pending_trade_no":          "",
		"pending_payment_intent_id": "",
		"pending_amount_cents":      0,
		"next_retry_at":             time.Now().Add(time.Hour).Unix(),
	}
	if disable {
		updates["enabled"] = false
	}
	query := DB.Model(&StripeAutoRecharge{}).Where("id = ?", id)
	if tradeNo != "" {
		query = query.Where("pending_trade_no = ?", tradeNo)
	}
	return query.Updates(updates).Error
}

func CompleteStripeAutoRecharge(tradeNo string) error {
	if tradeNo == "" {
		return errors.New("missing stripe auto recharge trade number")
	}
	return DB.Model(&StripeAutoRecharge{}).Where("pending_trade_no = ?", tradeNo).Updates(map[string]interface{}{
		"state":                     StripeAutoRechargeStateIdle,
		"last_error":                "",
		"pending_trade_no":          "",
		"pending_payment_intent_id": "",
		"pending_amount_cents":      0,
		"last_success_at":           common.GetTimestamp(),
		"next_retry_at":             0,
	}).Error
}

func FailStripeAutoRechargeByTradeNo(tradeNo string, message string, disable bool) error {
	config := &StripeAutoRecharge{}
	if err := DB.Where("pending_trade_no = ?", tradeNo).First(config).Error; err != nil {
		return err
	}
	return FailStripeAutoRecharge(config.Id, tradeNo, message, disable)
}

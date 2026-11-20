package model

import (
	"context"
	"errors"
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// ImageTaskQuotaReservation identifies the accounts backing a local async
// image task. Its initial reservation already belongs to the persisted task.
type ImageTaskQuotaReservation struct {
	UserID         int
	TokenID        int
	TokenKey       string
	SubscriptionID int
	SkipToken      bool
	TokenUnlimited bool
}

var (
	ErrImageTaskTokenReservation        = errors.New("image task token reservation failed")
	ErrImageTaskSubscriptionReservation = errors.New("image task subscription reservation failed")
)

// ReserveImageTaskQuota publishes the task CAS and additional account debits
// atomically. Extra debits deliberately bypass the batch queue: a persisted
// task must never own an additional reservation that can disappear on restart.
// publish must only use tx and stage its in-memory changes until this returns.
func ReserveImageTaskQuota(accounts ImageTaskQuotaReservation, delta int, publish func(*gorm.DB) error) error {
	if delta < 0 || publish == nil {
		return errors.New("invalid image task reservation")
	}
	// Hydration uses the global DB, so it must precede the transaction (SQLite
	// may have only one connection). Live cached balances are never overwritten.
	if delta > 0 && !accounts.SkipToken && common.RedisEnabled {
		if _, err := cacheGetTokenByKey(accounts.TokenKey); err != nil {
			_, _ = GetTokenByKey(accounts.TokenKey, true)
		}
	}
	ctx, cancel := context.WithTimeout(DB.Statement.Context, imageQuotaTransactionTimeout)
	defer cancel()
	committed := false
	var tokenCache, walletCache *imageQuotaCacheMutation
	if delta > 0 && common.RedisEnabled {
		var err error
		if !accounts.SkipToken {
			tokenCache, err = beginImageQuotaCacheMutation(ctx, getTokenCacheKey(accounts.TokenKey), accounts.TokenID, "token")
			defer func() { tokenCache.finish(committed) }()
			if err != nil {
				return fmt.Errorf("protect image task token cache: %w", err)
			}
		}
		if accounts.SubscriptionID == 0 {
			walletCache, err = beginImageQuotaCacheMutation(ctx, getUserCacheKey(accounts.UserID), accounts.UserID, "wallet")
			defer func() { walletCache.finish(committed) }()
			if err != nil {
				return fmt.Errorf("protect image task wallet cache: %w", err)
			}
		}
	}
	cacheReserved := false
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Lock/CAS the task before charging either account. Timeout either owns
		// the old reservation or observes the fully committed new reservation.
		if err := publish(tx); err != nil {
			return err
		}
		if delta == 0 {
			return nil
		}
		if tokenCache != nil {
			result, err := tokenCache.reserve(ctx, delta, accounts.TokenUnlimited)
			if err != nil {
				return fmt.Errorf("reserve image task token cache: %w", err)
			}
			if result == cacheQuotaInsufficient {
				return fmt.Errorf("%w: insufficient token quota", ErrImageTaskTokenReservation)
			}
			cacheReserved = result == cacheQuotaOK
		}
		if walletCache != nil {
			if _, err := walletCache.reserve(ctx, delta, true); err != nil {
				return fmt.Errorf("reserve image task wallet cache: %w", err)
			}
		}
		if accounts.SubscriptionID > 0 {
			var sub UserSubscription
			if err := lockForUpdate(tx).First(&sub, accounts.SubscriptionID).Error; err != nil {
				return fmt.Errorf("%w: %w", ErrImageTaskSubscriptionReservation, err)
			}
			used := sub.AmountUsed + int64(delta)
			if sub.AmountTotal > 0 && used > sub.AmountTotal {
				return fmt.Errorf("%w: subscription used exceeds total, used=%d total=%d", ErrImageTaskSubscriptionReservation, used, sub.AmountTotal)
			}
			if err := tx.Model(&sub).Update("amount_used", used).Error; err != nil {
				return fmt.Errorf("%w: %w", ErrImageTaskSubscriptionReservation, err)
			}
		} else {
			// Keep the existing additional-wallet-reserve semantics: overage is
			// debt, while limited tokens must still have sufficient quota.
			result := tx.Model(&User{}).Where("id = ?", accounts.UserID).
				Update("quota", gorm.Expr("quota - ?", delta))
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return gorm.ErrRecordNotFound
			}
		}
		if accounts.SkipToken {
			return nil
		}
		query := tx.Model(&Token{}).Where("id = ?", accounts.TokenID)
		if !accounts.TokenUnlimited && !cacheReserved {
			query = query.Where("remain_quota >= ?", delta)
		}
		result := query.Updates(map[string]any{
			"remain_quota":  gorm.Expr("remain_quota - ?", delta),
			"used_quota":    gorm.Expr("used_quota + ?", delta),
			"accessed_time": common.GetTimestamp(),
		})
		if result.Error != nil {
			return fmt.Errorf("%w: %w", ErrImageTaskTokenReservation, result.Error)
		}
		if result.RowsAffected != 1 {
			return fmt.Errorf("%w: token missing or insufficient quota", ErrImageTaskTokenReservation)
		}
		return nil
	})
	committed = err == nil
	return err
}

// UpdateWithStatusTx is the transactional form used by image reservation
// handoff. The receiver must be a staged copy until the transaction commits.
func (t *Task) UpdateWithStatusTx(tx *gorm.DB, fromStatus TaskStatus) (bool, error) {
	if t.ID <= 0 {
		return false, errors.New("invalid task ID")
	}
	return t.updateWithImageVersion(tx.Where("id = ? AND status = ?", t.ID, fromStatus))
}

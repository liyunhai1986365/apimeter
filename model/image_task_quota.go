package model

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// ImageTaskQuotaReservation identifies the accounts backing a local async
// image task. Its initial reservation already belongs to the persisted task.
type ImageTaskQuotaReservation struct {
	UserID            int
	TokenID           int
	TokenKey          string
	SubscriptionID    int
	SkipToken         bool
	TokenUnlimited    bool
	WalletReserved    bool // An unchanged paid reservation still needs a live wallet before delivery.
	RefundUndelivered bool // Return the original reservation even if its funding account was deleted.
}

type imageQuotaReceiptKey struct{}

type imageQuotaReceipt struct {
	owner       string
	taskID      int64
	baseVersion int64
}

var (
	ErrImageTaskTokenReservation        = errors.New("image task token reservation failed")
	ErrImageTaskSubscriptionReservation = errors.New("image task subscription reservation failed")
	ErrImageTaskSubscriptionLimit       = errors.New("subscription used exceeds total")
	ErrImageTaskSubscriptionDeleted     = errors.New("image task subscription no longer exists")
	ErrImageTaskWalletDeleted           = errors.New("image task wallet no longer exists")
)

// ReserveImageTaskQuota publishes the task CAS and additional account debits
// atomically. Extra debits deliberately bypass the batch queue: a persisted
// task must never own an additional reservation that can disappear on restart.
// publish must only use tx and stage its in-memory changes until this returns.
func ReserveImageTaskQuota(accounts ImageTaskQuotaReservation, delta int, publish func(*gorm.DB) error) error {
	if delta < 0 {
		return errors.New("invalid image task reservation")
	}
	return applyImageTaskQuota(accounts, delta, false, publish)
}

// SettleImageTaskQuota allows signed adjustments for work already delivered.
// Unlike a new reservation it may debit a limited token past its balance.
func SettleImageTaskQuota(accounts ImageTaskQuotaReservation, delta int, publish func(*gorm.DB) error) error {
	return applyImageTaskQuota(accounts, delta, true, publish)
}

func applyImageTaskQuota(accounts ImageTaskQuotaReservation, delta int, settlement bool, publish func(*gorm.DB) error) (err error) {
	if publish == nil {
		return errors.New("missing image task publication")
	}
	// Hydration uses the global DB, so it must precede the transaction (SQLite
	// may have only one connection). Live cached balances are never overwritten.
	if delta > 0 && !settlement && !accounts.SkipToken && common.RedisEnabled {
		if _, err := cacheGetTokenByKey(accounts.TokenKey); err != nil {
			_, _ = GetTokenByKey(accounts.TokenKey, true)
		}
	}
	ctx, cancel := context.WithTimeout(DB.Statement.Context, imageQuotaTransactionTimeout)
	defer cancel()
	committed := false
	atCommit := false
	var receipt *imageQuotaReceipt
	var tokenCache, walletCache *imageQuotaCacheMutation
	defer func() {
		if !committed && atCommit && receipt != nil {
			// Resolve while the task is still at the reservation step, before
			// recording a terminal status obscures the original CAS version.
			checkCtx, stop := context.WithTimeout(context.Background(), imageQuotaTransactionTimeout)
			var known bool
			committed, known = receipt.outcome(checkCtx)
			stop()
			if !known {
				return // Retain the holds for subsequent receipt recovery.
			}
			if settlement && committed {
				err = nil
			}
		}
		walletDone := walletCache.finish(committed)
		tokenDone := tokenCache.finish(committed)
		if !committed && receipt != nil && receipt.taskID > 0 && (!walletDone || !tokenDone) {
			receipt.recordRollback()
		}
	}()
	if settlement || (delta != 0 && common.RedisEnabled) {
		receipt = &imageQuotaReceipt{owner: common.GetUUID()}
		if settlement {
			receipt.owner = "settled:" + receipt.owner
		}
		ctx = context.WithValue(ctx, imageQuotaReceiptKey{}, receipt)
	}
	if delta != 0 && common.RedisEnabled {
		var err error
		if !accounts.SkipToken {
			tokenCache, err = beginImageQuotaCacheMutation(ctx, getTokenCacheKey(accounts.TokenKey), accounts.TokenID, "token")
			if err != nil {
				return fmt.Errorf("protect image task token cache: %w", err)
			}
		}
		if accounts.SubscriptionID == 0 {
			walletCache, err = beginImageQuotaCacheMutation(ctx, getUserCacheKey(accounts.UserID), accounts.UserID, "wallet")
			if err != nil {
				return fmt.Errorf("protect image task wallet cache: %w", err)
			}
		}
	}
	cacheReserved := false
	err = DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Lock/CAS the task before charging either account. Timeout either owns
		// the old reservation or observes the fully committed new reservation.
		if err := publish(tx); err != nil {
			return err
		}
		if delta == 0 && (!settlement || (accounts.SubscriptionID == 0 && !accounts.WalletReserved)) {
			atCommit = true
			return nil
		}
		if receipt != nil && receipt.taskID <= 0 {
			return errors.New("image quota reservation has no persisted task receipt")
		}
		if tokenCache != nil {
			result, err := tokenCache.reserve(ctx, delta, settlement || accounts.TokenUnlimited)
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
			err := lockForUpdate(tx).First(&sub, accounts.SubscriptionID).Error
			switch {
			case settlement && errors.Is(err, gorm.ErrRecordNotFound):
				if !accounts.RefundUndelivered || delta > 0 {
					return fmt.Errorf("%w: %w: %w", ErrImageTaskSubscriptionReservation, ErrImageTaskSubscriptionDeleted, err)
				}
			case err != nil:
				return fmt.Errorf("%w: %w", ErrImageTaskSubscriptionReservation, err)
			case delta != 0:
				used := max(sub.AmountUsed+int64(delta), 0)
				if delta > 0 && sub.AmountTotal > 0 && used > sub.AmountTotal {
					return fmt.Errorf("%w: %w, used=%d total=%d", ErrImageTaskSubscriptionReservation, ErrImageTaskSubscriptionLimit, used, sub.AmountTotal)
				}
				if err := tx.Model(&sub).Update("amount_used", used).Error; err != nil {
					return fmt.Errorf("%w: %w", ErrImageTaskSubscriptionReservation, err)
				}
			}
		} else {
			// Keep the existing additional-wallet-reserve semantics: overage is
			// debt, while limited tokens must still have sufficient quota.
			wallet := tx.Model(&User{}).Where("id = ?", accounts.UserID)
			refund := settlement && accounts.RefundUndelivered && delta <= 0
			if refund {
				// Credit the original row without clearing deleted_at or creating a wallet.
				wallet = wallet.Unscoped()
			}
			var result *gorm.DB
			if delta == 0 {
				// MySQL can report zero changed rows for a no-op UPDATE.
				result = lockForUpdate(wallet).Select("id").Find(&User{})
			} else {
				result = wallet.Update("quota", gorm.Expr("quota - ?", delta))
			}
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 0 && !refund {
				if settlement {
					return fmt.Errorf("%w: %w", ErrImageTaskWalletDeleted, gorm.ErrRecordNotFound)
				}
				return gorm.ErrRecordNotFound
			}
		}
		if delta == 0 || accounts.SkipToken {
			atCommit = true
			return nil
		}
		query := tx.Model(&Token{}).Where("id = ?", accounts.TokenID)
		if !settlement && !accounts.TokenUnlimited && !cacheReserved {
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
		atCommit = true
		return nil
	})
	committed = err == nil
	return err
}

func (t *Task) ImageQuotaSettled() bool {
	if receipts := t.PrivateData.ImageQuotaReceipts; receipts != nil {
		for _, id := range *receipts {
			if strings.HasPrefix(id, "settled:") {
				return true
			}
		}
	}
	return false
}

// UpdateWithStatusTx is the transactional form used by image reservation
// handoff. The receiver must be a staged copy until the transaction commits.
func (t *Task) UpdateWithStatusTx(tx *gorm.DB, fromStatus TaskStatus) (bool, error) {
	if t.ID <= 0 {
		return false, errors.New("invalid task ID")
	}
	if receipt, ok := tx.Statement.Context.Value(imageQuotaReceiptKey{}).(*imageQuotaReceipt); ok {
		receipt.taskID, receipt.baseVersion = t.ID, t.ImageBase64Version
		var ids []string
		if t.PrivateData.ImageQuotaReceipts != nil {
			ids = append(ids, (*t.PrivateData.ImageQuotaReceipts)...)
		}
		ids = append(ids, receipt.owner)
		t.PrivateData.ImageQuotaReceipts = &ids
	}
	return t.updateWithImageVersion(tx.Where("id = ? AND status = ?", t.ID, fromStatus))
}

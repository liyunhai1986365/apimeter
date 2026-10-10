package model

import (
	"context"
	"sort"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// All aliases/tasks of an asset, and all children of a group, check the same
// durable identity. Revocation changes only that identity, without scanning
// metadata or rewriting historical ownership rows while holding locks.
func revokeAssetIdentity(ctx context.Context, binding AssetBinding) error {
	select {
	case assetDeletionSlots <- struct{}{}:
		defer func() { <-assetDeletionSlots }()
	case <-ctx.Done():
		return ctx.Err()
	}
	db := DB.WithContext(ctx)
	resolve := func(tx *gorm.DB) (AssetBinding, error) {
		target := binding
		states, err := scopedOwnedAssetStates(tx, binding, false)
		if err != nil {
			return target, err
		}
		for i, state := range states {
			var stored AssetBinding
			if err := common.UnmarshalJsonStr(state.Metadata, &stored); err != nil {
				return target, err
			}
			canonical := assetCanonicalID(stored)
			if i > 0 && assetCanonicalID(target) != canonical {
				return target, ErrAssetNotOwned
			}
			target.CanonicalID = canonical
		}
		return target, nil
	}
	return retryAssetBindingWrite(db, func() error {
		target, err := resolve(db)
		if err != nil {
			return err
		}
		plan := make(assetBindingLockPlan)
		plan.includeKeys(assetOwnLifecycleKeys(target), nil)
		ids, err := prepareAssetBindingLocks(db, plan)
		if err != nil {
			return err
		}
		return assetWriteTransaction(db, func(tx *gorm.DB) error {
			locked, err := lockAssetBindings(tx, ids)
			if err != nil {
				return err
			}
			target, err = resolve(tx)
			if err != nil {
				return err
			}
			if plan.includeKeys(assetOwnLifecycleKeys(target), locked) {
				return errAssetBindingPlanChanged
			}
			updateIDs := make([]int, 0, len(ids))
			for _, state := range locked {
				if state.Status != ConfigurableResourceStateStatusInvalid {
					updateIDs = append(updateIDs, state.Id)
				}
			}
			if len(updateIDs) == 0 {
				return nil
			}
			sort.Ints(updateIDs)
			return assetBindingPrimaryRows(tx, updateIDs).Updates(map[string]any{
				"status": ConfigurableResourceStateStatusInvalid, "updated_at": common.GetTimestamp(),
			}).Error
		})
	})
}

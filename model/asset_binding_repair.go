package model

import (
	"context"
	"sort"

	"gorm.io/gorm"
)

func (node *assetIdentityNode) needsRepair() bool {
	return node.state.Id == 0 ||
		(node.state.Status == ConfigurableResourceStateStatusActive && node.state.StateValue != assetLifecycleRelated)
}

// Complete identities take the read-only path. Only an existing binding with
// missing/old relationship records needs compatibility discovery and repair.
// Unknown handles never reach here, and repair never creates ownership.
func loadAssetBindingHistoryForRead(db *gorm.DB, refs map[string]AssetBinding) (assetBindingHistory, error) {
	ctx, cancel := context.WithTimeout(db.Statement.Context, AssetOperationTimeout)
	defer cancel()
	db = db.WithContext(ctx)
	var history assetBindingHistory
	err := retryAssetBindingWrite(db, func() error {
		var err error
		history, err = loadAssetBindingHistory(db, refs)
		if err != nil {
			return err
		}
		missing := false
		for _, node := range history {
			missing = missing || node.needsRepair()
		}
		if !missing {
			return nil
		}
		// Reject cyclic dependencies before storing a completion marker.
		for key := range refs {
			if _, err := history.revoked(key, make(map[string]bool)); err != nil {
				return err
			}
		}
		return repairAssetBindingHistory(db, history)
	})
	return history, err
}

// Discovery has already finished outside the transaction. Lock every dependency
// in the writers' PK order and compare its live state to the snapshot before
// persisting anything. A concurrent alias change/deletion forces rediscovery;
// neither an old snapshot nor a repaired tombstone can restore access.
func repairAssetBindingHistory(db *gorm.DB, history assetBindingHistory) error {
	plan := make(assetBindingLockPlan, len(history))
	for key, node := range history {
		plan[key] = assetBindingSharedLock
		if node.needsRepair() {
			plan[key] = assetBindingExclusiveLock
		}
	}
	locks, err := prepareAssetBindingLocks(db, plan)
	if err != nil {
		return err
	}
	return assetWriteTransaction(db, func(tx *gorm.DB) error {
		locked, err := lockAssetBindings(tx, locks)
		if err != nil {
			return err
		}
		type repair struct {
			status, metadata string
		}
		batches := make(map[repair][]int)
		for key, node := range history {
			state, ok := locked[key]
			expectedStatus := node.state.Status
			if node.state.Id == 0 {
				// prepareAssetBindingLocks only creates inert active mutexes.
				expectedStatus = ConfigurableResourceStateStatusActive
			}
			if !ok || (node.state.Id != 0 && node.state.Id != state.Id) ||
				state.Status != expectedStatus || state.StateValue != node.state.StateValue || state.Metadata != node.state.Metadata {
				return errAssetBindingPlanChanged
			}
			if !node.needsRepair() {
				continue
			}
			metadata, err := assetIdentityParentsJSON(node.parents)
			if err != nil {
				return err
			}
			status := ConfigurableResourceStateStatusActive
			if node.revoked {
				// Preserve invalid historical aliases as durable tombstones too.
				status = ConfigurableResourceStateStatusInvalid
			}
			change := repair{status: status, metadata: metadata}
			batches[change] = append(batches[change], state.Id)
		}
		for change, ids := range batches {
			sort.Ints(ids)
			for start := 0; start < len(ids); start += assetBindingBatchSize {
				if err := assetBindingPrimaryRows(tx, ids[start:min(start+assetBindingBatchSize, len(ids))]).
					Updates(map[string]any{"state_value": assetLifecycleRelated, "metadata": change.metadata, "status": change.status}).Error; err != nil {
					return err
				}
			}
		}
		return nil
	})
}

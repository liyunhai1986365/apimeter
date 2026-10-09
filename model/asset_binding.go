package model

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const assetBindingProfile = "asset-access-v1"
const assetLibraryScopeProfile = "asset-library-scope-v1"
const assetBindingLockProfile = "asset-access-lock-v1"
const assetBindingBatchSize = 500

var ErrAssetNotOwned = errors.New("asset resource is unknown or not owned by this user")
var errAssetBindingLockRace = errors.New("asset binding mutex was concurrently initialized")

// AssetBinding reuses ConfigurableResourceState; it does not add a table or
// column. UserID/TokenID on the stored row are zero so its existing unique key
// enforces one owner per upstream handle, including concurrent registrations.
// StateValue holds the owner, allowing portable queries without JSON SQL.
type AssetBinding struct {
	ChannelID   int    `json:"channel_id"`
	UserID      int    `json:"user_id"`
	Backend     string `json:"backend"`
	Scope       string `json:"scope"`
	Kind        string `json:"kind"`
	ID          string `json:"id,omitempty"`
	CanonicalID string `json:"canonical_id,omitempty"`
	Project     string `json:"project,omitempty"`
	GroupID     string `json:"group_id,omitempty"`
	ExpiresAt   int64  `json:"expires_at,omitempty"`
}

func assetBindingKey(id string) string { return fmt.Sprintf("%x", sha256.Sum256([]byte(id))) }

// ResolveAssetLibraryScope assigns one persistent identity to a channel's asset
// endpoint. The first value is its legacy credential scope, preserving existing
// ownership without rewriting rows. Later credential rotations keep that value.
func ResolveAssetLibraryScope(channelID int, endpointKey, initialScope string) (string, error) {
	if channelID <= 0 || endpointKey == "" || initialScope == "" {
		return "", fmt.Errorf("invalid asset library scope")
	}
	var stored ConfigurableResourceState
	lookup := func() *gorm.DB {
		return DB.Where("channel_id = ? AND profile_id = ? AND resource_id = ? AND pre_request_id = ? AND user_id = 0 AND token_id = 0 AND state_key = ?",
			channelID, assetLibraryScopeProfile, "scope", "endpoint", endpointKey).Limit(1).Find(&stored)
	}
	result := lookup()
	if result.Error != nil {
		return "", result.Error
	}
	if result.RowsAffected == 0 {
		now := common.GetTimestamp()
		state := ConfigurableResourceState{
			ChannelID: channelID, ProfileID: assetLibraryScopeProfile,
			ResourceID: "scope", PreRequestID: "endpoint", StateKey: endpointKey,
			StateValue: initialScope, Status: ConfigurableResourceStateStatusActive,
			CreatedAt: now, UpdatedAt: now, LastUsedAt: now,
		}
		// Concurrent first use must agree on a single identity, never replace it.
		if err := DB.Clauses(clause.OnConflict{DoNothing: true}).Create(&state).Error; err != nil {
			return "", err
		}
		if result = lookup(); result.Error != nil {
			return "", result.Error
		}
	}
	if stored.Status != ConfigurableResourceStateStatusActive || stored.StateValue == "" {
		return "", fmt.Errorf("asset library scope is unavailable")
	}
	return stored.StateValue, nil
}

func SaveAssetBinding(binding AssetBinding, handle string) error {
	binding.ID = handle
	return SaveAssetBindings([]AssetBinding{binding})
}

// SaveAssetBindings registers or hydrates a complete alias set atomically.
func SaveAssetBindings(bindings []AssetBinding) error {
	return registerAssetBindings(bindings, false)
}

func assetBindingLockKey(kind, stateKey string) string {
	// An asset and its asynchronous task handle share the same lock namespace.
	return assetBindingKind(kind) + ":" + stateKey
}

// Prepare persistent mutex rows outside the ownership transaction. These inert
// rows contain no ownership and may survive a rejected claim. MySQL duplicate
// UPDATE must also be avoided here: its PRIMARY gap lock can block an ownership
// insert while it waits for the mutex already held by that ownership transaction.
// The existing ownership unique key alone is insufficient across channels.
func prepareAssetBindingLocks(db *gorm.DB, keys assetBindingLockPlan) ([]int, error) {
	ordered := make([]string, 0, len(keys))
	for key := range keys {
		ordered = append(ordered, key)
	}
	sort.Strings(ordered)
	storedIDs := make(map[string]int, len(ordered))
	lookup := func(keys []string) error {
		for start := 0; start < len(keys); start += assetBindingBatchSize {
			batch := keys[start:min(start+assetBindingBatchSize, len(keys))]
			var states []ConfigurableResourceState
			if err := db.Select("id", "state_key").Where("channel_id = 0 AND profile_id = ? AND resource_id = ? AND pre_request_id = ? AND user_id = 0 AND token_id = 0 AND state_key IN ?",
				assetBindingLockProfile, "handle", "global", batch).Find(&states).Error; err != nil {
				return err
			}
			for _, state := range states {
				storedIDs[state.StateKey] = state.Id
			}
		}
		return nil
	}
	if err := lookup(ordered); err != nil {
		return nil, err
	}
	var missing []string
	for _, key := range ordered {
		if storedIDs[key] == 0 {
			missing = append(missing, key)
		}
	}
	// Keep each initialization statement below SQLite's legacy parameter limit.
	// Each batch commits independently, before any ownership locks are acquired.
	const insertBatchSize = 50
	for start := 0; start < len(missing); start += insertBatchSize {
		batch := missing[start:min(start+insertBatchSize, len(missing))]
		states := make([]ConfigurableResourceState, 0, len(batch))
		for _, key := range batch {
			states = append(states, ConfigurableResourceState{ProfileID: assetBindingLockProfile,
				ResourceID: "handle", PreRequestID: "global", StateKey: key,
				Status: ConfigurableResourceStateStatusActive})
		}
		create := db.Clauses(clause.OnConflict{DoNothing: true})
		if db.Dialector.Name() == common.DatabaseTypeMySQL {
			// Under REPEATABLE READ, IGNORE retains duplicate-key shared gap
			// locks while attempting the next insert. A plain INSERT aborts on
			// the first duplicate instead; roll back and rediscover missing rows.
			create = db
		}
		if err := create.Transaction(func(tx *gorm.DB) error { return tx.Create(&states).Error }); err != nil {
			var duplicate *mysql.MySQLError
			if errors.As(err, &duplicate) && duplicate.Number == 1062 {
				return nil, fmt.Errorf("%w: %w", errAssetBindingLockRace, err)
			}
			return nil, err
		}
	}
	if err := lookup(missing); err != nil {
		return nil, err
	}
	ids := make([]int, 0, len(ordered))
	for _, key := range ordered {
		if storedIDs[key] == 0 {
			return nil, gorm.ErrRecordNotFound
		}
		ids = append(ids, storedIDs[key])
	}
	// Every writer uses the same immutable PK order, including across batches.
	sort.Ints(ids)
	return ids, nil
}

// Drive MySQL batches from a materialized, ordered list of IDs. STRAIGHT_JOIN
// forces one unique PRIMARY lookup per ID: an IN list plus FORCE INDEX can
// still become a full index scan and retain unrelated next-key locks under RR.
func assetBindingPrimaryRows(tx *gorm.DB, ids []int) *gorm.DB {
	query := tx.Model(&ConfigurableResourceState{})
	if tx.Dialector.Name() == common.DatabaseTypeMySQL {
		table := tx.NamingStrategy.TableName("ConfigurableResourceState")
		selects := make([]string, len(ids))
		args := make([]any, len(ids))
		for i, id := range ids {
			selects[i], args[i] = "SELECT ? AS binding_row_id", id
		}
		// LIMIT preserves the derived table's ORDER BY during optimization.
		rows := clause.Expr{SQL: strings.Join(selects, " UNION ALL ") + " ORDER BY binding_row_id LIMIT " + strconv.Itoa(len(ids)), Vars: args}
		// Keep an explicit row predicate for GORM's global-update guard too.
		return query.Table("(?) AS asset_binding_ids STRAIGHT_JOIN ? FORCE INDEX (PRIMARY) ON ?.id = asset_binding_ids.binding_row_id",
			rows, clause.Table{Name: table}, clause.Table{Name: table}).
			Where("id = asset_binding_ids.binding_row_id")
	}
	return query.Where("id IN ?", ids)
}

// Acquire all mutexes in primary-key order before reading an ownership snapshot.
func lockAssetBindings(tx *gorm.DB, ids []int) (map[string]ConfigurableResourceState, error) {
	states := make(map[string]ConfigurableResourceState, len(ids))
	for start := 0; start < len(ids); start += assetBindingBatchSize {
		batch := ids[start:min(start+assetBindingBatchSize, len(ids))]
		query := assetBindingPrimaryRows(tx, batch)
		if tx.Dialector.Name() == common.DatabaseTypeSQLite {
			// SQLite acquires its write reservation before any snapshot reads.
			// Lifecycle mutexes can be revoked; acquiring a lock must not reset
			// their status. SQLite's write reservation serializes later reads.
			result := query.UpdateColumn("id", gorm.Expr("id"))
			if result.Error != nil {
				return nil, result.Error
			}
			if result.RowsAffected != int64(len(batch)) {
				return nil, gorm.ErrRecordNotFound
			}
		}
		var locked []ConfigurableResourceState
		if err := lockForUpdate(query).Select("id", "state_key", "status", "state_value").Order("id").Find(&locked).Error; err != nil {
			return nil, err
		}
		if len(locked) != len(batch) {
			return nil, gorm.ErrRecordNotFound
		}
		for _, state := range locked {
			states[state.StateKey] = state
		}
	}
	return states, nil
}

func saveAssetBinding(tx *gorm.DB, binding AssetBinding, lifecycle *assetBindingLifecycle) error {
	handle := binding.ID // registerAssetBindings validates all input before locking.
	if binding.Kind == "session" {
		// BytedToken is a bearer credential. Persist only its digest and expiry.
		binding.ID, binding.CanonicalID = "", ""
	} else if binding.CanonicalID == "" {
		binding.CanonicalID = handle
	}
	metadata, err := common.Marshal(binding)
	if err != nil {
		return err
	}
	now := common.GetTimestamp()
	state := ConfigurableResourceState{
		ChannelID: binding.ChannelID, ProfileID: assetBindingProfile,
		ResourceID: binding.Kind, PreRequestID: binding.Scope,
		StateKey: assetBindingKey(handle), StateValue: strconv.Itoa(binding.UserID),
		Status: ConfigurableResourceStateStatusActive, Metadata: string(metadata),
		CreatedAt: now, UpdatedAt: now, LastUsedAt: now,
	}
	var stored ConfigurableResourceState
	// The global handle mutex serializes registrations. A nonlocking lookup
	// avoids locking a missing unique-key gap when registering a new handle.
	result := tx.Where("channel_id = ? AND profile_id = ? AND resource_id = ? AND pre_request_id = ? AND user_id = 0 AND token_id = 0 AND state_key = ?",
		binding.ChannelID, assetBindingProfile, binding.Kind, binding.Scope, state.StateKey).Limit(1).Find(&stored)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		if err := lifecycle.validate(binding); err != nil {
			return err
		}
		return tx.Create(&state).Error
	}
	// Read the current row under lock so a concurrent revocation is respected.
	if err := lockForUpdate(tx).First(&stored, stored.Id).Error; err != nil {
		return err
	}
	if stored.StateValue != state.StateValue || stored.Status != ConfigurableResourceStateStatusActive {
		return ErrAssetNotOwned
	}
	// Do not extend a session's expiry on a repeated query/registration.
	if binding.Kind == "session" {
		return nil
	}
	var existing AssetBinding
	if err := common.UnmarshalJsonStr(stored.Metadata, &existing); err != nil {
		return err
	}
	if (existing.Project != "" && binding.Project != "" && existing.Project != binding.Project) ||
		(existing.GroupID != "" && binding.GroupID != "" && existing.GroupID != binding.GroupID) {
		return ErrAssetNotOwned
	}
	if existing.Project == "" {
		existing.Project = binding.Project
	}
	if existing.GroupID == "" {
		existing.GroupID = binding.GroupID
	}
	if err := lifecycle.validate(existing); err != nil {
		return err
	}
	// An async handle can remain canonical after its final asset ID arrives.
	// Hydrating metadata must never replace that established identity.
	metadata, err = common.Marshal(existing)
	if err != nil {
		return err
	}
	if string(metadata) == stored.Metadata {
		return nil
	}
	return tx.Model(&stored).Updates(map[string]any{"metadata": string(metadata), "updated_at": now}).Error
}

// HasAssetBinding includes other owners, revoked records and old account scopes.
// None of these are unregistered historical handles eligible for first use.
func HasAssetBinding(kind, handle string) (bool, error) {
	var count int64
	err := assetHandleStates(DB, kind, handle).Count(&count).Error
	return count > 0, err
}

func assetHandleStates(tx *gorm.DB, kind, handle string) *gorm.DB {
	kinds := []string{kind}
	if kind == "asset" || kind == "task" {
		kinds = []string{"asset", "task"}
	}
	return tx.Model(&ConfigurableResourceState{}).Where("profile_id = ? AND resource_id IN ? AND state_key = ?", assetBindingProfile, kinds, assetBindingKey(handle))
}

// ClaimLegacyAssetBindings saves all verified aliases in one transaction. A
// conflicting alias must roll back the entire claim, including the supplied ID.
func ClaimLegacyAssetBindings(bindings []AssetBinding) error {
	return registerAssetBindings(bindings, true)
}

func registerAssetBindings(bindings []AssetBinding, legacy bool) error {
	plan := make(assetBindingLockPlan)
	for _, binding := range bindings {
		if binding.UserID <= 0 || binding.ChannelID <= 0 || binding.Scope == "" || binding.ID == "" {
			return fmt.Errorf("invalid asset binding")
		}
		if binding.Kind != "asset" && binding.Kind != "group" && binding.Kind != "task" && (legacy || binding.Kind != "session") {
			return fmt.Errorf("unsupported asset kind")
		}
		plan.include(binding, nil)
	}
	if len(bindings) == 0 {
		return nil
	}
	db := DB
	return retryAssetBindingWrite(db, func() error {
		ids, err := prepareAssetBindingLocks(db, plan)
		if err != nil {
			return err
		}
		// Keep the configured isolation; all mutexes precede snapshot reads.
		return db.Transaction(func(tx *gorm.DB) error {
			locked, err := lockAssetBindings(tx, ids)
			if err != nil {
				return err
			}
			resolved, err := plan.resolveStored(tx, bindings, locked)
			if err != nil {
				return err
			}
			lifecycle := newAssetBindingLifecycle(tx, locked)
			for _, binding := range resolved {
				var states []ConfigurableResourceState
				if err := assetHandleStates(tx, binding.Kind, binding.ID).Find(&states).Error; err != nil {
					return err
				}
				for _, state := range states {
					// New resources may reuse provider-local IDs in unrelated accounts.
					// Legacy claims require a globally unregistered handle; ordinary
					// writes still cannot add a second owner in the same account scope.
					if !legacy && state.PreRequestID != binding.Scope {
						continue
					}
					if state.ChannelID != binding.ChannelID || state.PreRequestID != binding.Scope || state.StateValue != strconv.Itoa(binding.UserID) || state.Status != ConfigurableResourceStateStatusActive {
						return ErrAssetNotOwned
					}
				}
				if err := saveAssetBinding(tx, binding, lifecycle); err != nil {
					return err
				}
			}
			return lifecycle.markInitialized()
		})
	})
}

func FindAssetBindings(userID int, kind, handle string) ([]AssetBinding, error) {
	var states []ConfigurableResourceState
	err := DB.Where("profile_id = ? AND resource_id = ? AND state_key = ? AND state_value = ? AND status = ?",
		assetBindingProfile, kind, assetBindingKey(handle), strconv.Itoa(userID), ConfigurableResourceStateStatusActive).Find(&states).Error
	if err != nil {
		return nil, err
	}
	return decodeAssetBindings(states)
}

func ListAssetBindings(channelID, userID int, scope, kind string) ([]AssetBinding, error) {
	var states []ConfigurableResourceState
	err := DB.Where("channel_id = ? AND profile_id = ? AND resource_id = ? AND pre_request_id = ? AND state_value = ? AND status = ?",
		channelID, assetBindingProfile, kind, scope, strconv.Itoa(userID), ConfigurableResourceStateStatusActive).Find(&states).Error
	if err != nil {
		return nil, err
	}
	return decodeAssetBindings(states)
}

func decodeAssetBindings(states []ConfigurableResourceState) ([]AssetBinding, error) {
	bindings := make([]AssetBinding, 0, len(states))
	for _, state := range states {
		var binding AssetBinding
		if err := common.UnmarshalJsonStr(state.Metadata, &binding); err != nil {
			return nil, fmt.Errorf("invalid stored asset binding: %w", err)
		}
		if binding.ExpiresAt > 0 && binding.ExpiresAt <= common.GetTimestamp() {
			continue
		}
		bindings = append(bindings, binding)
	}
	return bindings, nil
}

func InvalidateAssetBindings(binding AssetBinding, deleteGroup bool) error {
	if binding.ChannelID <= 0 || binding.UserID <= 0 || binding.Scope == "" || assetCanonicalID(binding) == "" {
		return fmt.Errorf("invalid asset binding")
	}
	if deleteGroup && binding.Kind != "group" {
		return fmt.Errorf("invalid asset group revocation")
	}
	db := DB
	return retryAssetBindingWrite(db, func() error {
		target, records, err := assetRevocationRecords(db, binding, deleteGroup)
		if err != nil {
			return err
		}
		if len(records) == 0 {
			return nil
		}
		plan := make(assetBindingLockPlan)
		// Every registration locks its effective identity before touching a
		// binding. Deletion needs those identities, not every alias handle:
		// that keeps large alias sets batched and stabilizes new aliases too.
		plan.includeKeys(assetOwnLifecycleKeys(target), nil)
		for _, record := range records {
			plan.includeKeys(assetOwnLifecycleKeys(record.binding), nil)
		}
		mutexIDs, err := prepareAssetBindingLocks(db, plan)
		if err != nil {
			return err
		}
		return db.Transaction(func(tx *gorm.DB) error {
			// Match registration's mutex order before touching any ownership row.
			// Only then can ownership updates be batched without reversing the
			// alias lock order of a concurrent poll.
			locked, err := lockAssetBindings(tx, mutexIDs)
			if err != nil {
				return err
			}
			// This is the first snapshot read, after all locks. Writers of new
			// aliases/children share the canonical/group lifecycle mutexes.
			target, records, err = assetRevocationRecords(tx, binding, deleteGroup)
			if err != nil {
				return err
			}
			missing := plan.includeKeys(assetOwnLifecycleKeys(target), locked)
			for _, record := range records {
				if plan.includeKeys(assetOwnLifecycleKeys(record.binding), locked) {
					missing = true
				}
			}
			if missing {
				return errAssetBindingPlanChanged
			}
			if len(records) == 0 {
				return nil
			}
			updates := make(map[int]bool)
			addIdentity := func(item AssetBinding) {
				for _, key := range assetOwnLifecycleKeys(item) {
					state := locked[key]
					if state.Status != ConfigurableResourceStateStatusInvalid {
						updates[state.Id] = true
					}
				}
			}
			addIdentity(target)
			for _, record := range records {
				if record.state.Status != ConfigurableResourceStateStatusInvalid {
					updates[record.state.Id] = true
				}
				addIdentity(record.binding)
			}
			ids := make([]int, 0, len(updates))
			for id := range updates {
				ids = append(ids, id)
			}
			sort.Ints(ids)
			now := common.GetTimestamp()
			for start := 0; start < len(ids); start += assetBindingBatchSize {
				batch := ids[start:min(start+assetBindingBatchSize, len(ids))]
				// Keep PK ranges bounded and all batches in the same transaction.
				if err := assetBindingPrimaryRows(tx, batch).
					Updates(map[string]any{"status": ConfigurableResourceStateStatusInvalid, "updated_at": now}).Error; err != nil {
					return err
				}
			}
			return nil
		})
	})
}

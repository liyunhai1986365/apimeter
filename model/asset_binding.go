package model

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/go-sql-driver/mysql"
	"golang.org/x/sync/singleflight"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const assetBindingProfile = "asset-access-v1"
const assetLibraryScopeProfile = "asset-library-scope-v1"
const assetBindingLockProfile = "asset-access-lock-v1"
const assetBindingBatchSize = 500

var ErrAssetNotOwned = errors.New("asset resource is unknown or not owned by this user")
var ErrAssetRevoked = fmt.Errorf("%w: ownership was revoked", ErrAssetNotOwned)
var errAssetBindingLockRace = errors.New("asset binding mutex was concurrently initialized")

// Queue deletion bookkeeping before borrowing a connection. Waiting here holds
// no transaction or row lock; durable identity locks coordinate across nodes.
var assetDeletionSlots = make(chan struct{}, 4)

var assetDeletions singleflight.Group

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
	return ResolveAssetLibraryScopeContext(context.Background(), channelID, endpointKey, initialScope)
}

func ResolveAssetLibraryScopeContext(ctx context.Context, channelID int, endpointKey, initialScope string) (string, error) {
	if channelID <= 0 || endpointKey == "" || initialScope == "" {
		return "", fmt.Errorf("invalid asset library scope")
	}
	db := DB.WithContext(ctx)
	var stored ConfigurableResourceState
	lookup := func() *gorm.DB {
		return db.Where("channel_id = ? AND profile_id = ? AND resource_id = ? AND pre_request_id = ? AND user_id = 0 AND token_id = 0 AND state_key = ?",
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
		if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&state).Error; err != nil {
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
	return SaveAssetBindingsContext(context.Background(), bindings)
}

// SaveAssetBindingsContext keeps unchanged polling reads out of the write
// transaction. Only new aliases or missing metadata need lifecycle locks.
func SaveAssetBindingsContext(ctx context.Context, bindings []AssetBinding) error {
	ctx, cancel := context.WithTimeout(ctx, AssetOperationTimeout)
	defer cancel()
	return registerAssetBindings(DB.WithContext(ctx), bindings, false)
}

// ClaimLegacyAssetBindings atomically registers aliases verified upstream.
// Unlike creation, a historical claim also rejects handles in other scopes.
func ClaimLegacyAssetBindings(bindings []AssetBinding) error {
	return ClaimLegacyAssetBindingsContext(context.Background(), bindings)
}

func ClaimLegacyAssetBindingsContext(ctx context.Context, bindings []AssetBinding) error {
	ctx, cancel := context.WithTimeout(ctx, AssetOperationTimeout)
	defer cancel()
	return registerAssetBindings(DB.WithContext(ctx), bindings, true)
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
func prepareAssetBindingLocks(db *gorm.DB, keys assetBindingLockPlan) ([]assetBindingRowLock, error) {
	ordered := make([]string, 0, len(keys))
	for key := range keys {
		ordered = append(ordered, key)
	}
	sort.Strings(ordered)
	stored := make(map[string]ConfigurableResourceState, len(ordered))
	lookup := func(keys []string) error {
		for start := 0; start < len(keys); start += assetBindingBatchSize {
			batch := keys[start:min(start+assetBindingBatchSize, len(keys))]
			var states []ConfigurableResourceState
			if err := db.Select("id", "state_key", "state_value").Where("channel_id = 0 AND profile_id = ? AND resource_id = ? AND pre_request_id = ? AND user_id = 0 AND token_id = 0 AND state_key IN ?",
				assetBindingLockProfile, "handle", "global", batch).Find(&states).Error; err != nil {
				return err
			}
			for _, state := range states {
				stored[state.StateKey] = state
			}
		}
		return nil
	}
	if err := lookup(ordered); err != nil {
		return nil, err
	}
	var missing []string
	for _, key := range ordered {
		if stored[key].Id == 0 {
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
		if err := assetWriteTransaction(create, func(tx *gorm.DB) error { return tx.Create(&states).Error }); err != nil {
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
	locks := make([]assetBindingRowLock, 0, len(ordered))
	for _, key := range ordered {
		state := stored[key]
		if state.Id == 0 {
			return nil, gorm.ErrRecordNotFound
		}
		mode := keys[key]
		if mode == assetBindingSharedLock && state.StateValue != assetLifecycleRelated {
			// Initializing an older parent writes its lifecycle marker. Take
			// the exclusive lock from the outset to avoid shared-lock upgrades.
			mode = assetBindingExclusiveLock
		}
		locks = append(locks, assetBindingRowLock{id: state.Id, mode: mode})
	}
	// Every writer uses the same immutable PK order, including across batches.
	sort.Slice(locks, func(i, j int) bool { return locks[i].id < locks[j].id })
	return locks, nil
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
func lockAssetBindings(tx *gorm.DB, locks []assetBindingRowLock) (map[string]ConfigurableResourceState, error) {
	states := make(map[string]ConfigurableResourceState, len(locks))
	for start := 0; start < len(locks); {
		// Split only consecutive locks of the same strength; collecting all
		// shared locks first would reverse PK order relative to deletion.
		end := start + 1
		for end < len(locks) && end-start < assetBindingBatchSize && locks[end].mode == locks[start].mode {
			end++
		}
		batch := make([]int, end-start)
		for i := range batch {
			batch[i] = locks[start+i].id
		}
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
		if locks[start].mode == assetBindingSharedLock {
			query = lockForShare(query)
		} else {
			query = lockForUpdate(query)
		}
		var locked []ConfigurableResourceState
		if err := query.Select("id", "state_key", "status", "state_value", "metadata").Order("id").Find(&locked).Error; err != nil {
			return nil, err
		}
		if len(locked) != len(batch) {
			return nil, gorm.ErrRecordNotFound
		}
		for _, state := range locked {
			states[state.StateKey] = state
		}
		start = end
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
	// The global handle mutex serializes registrations. A nonlocking lookup
	// avoids locking a missing unique-key gap when registering a new handle.
	stored, err := findStoredAssetBinding(tx, binding, handle)
	if err != nil {
		return err
	}
	if stored == nil {
		if err := lifecycle.validate(binding); err != nil {
			return err
		}
		return tx.Create(&state).Error
	}
	// Read the current row under lock so a concurrent revocation is respected.
	if err := lockForUpdate(tx).First(stored, stored.Id).Error; err != nil {
		return err
	}
	if stored.StateValue != state.StateValue {
		return ErrAssetNotOwned
	}
	if stored.Status != ConfigurableResourceStateStatusActive {
		return ErrAssetRevoked
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
	return tx.Model(stored).Updates(map[string]any{"metadata": string(metadata), "updated_at": now}).Error
}

// HasAssetBinding includes other owners, revoked records and old account scopes.
// None of these are unregistered historical handles eligible for first use.
func HasAssetBinding(kind, handle string) (bool, error) {
	return HasAssetBindingContext(context.Background(), kind, handle)
}

func HasAssetBindingContext(ctx context.Context, kind, handle string) (bool, error) {
	var count int64
	err := assetHandleStates(DB.WithContext(ctx), kind, handle).Count(&count).Error
	return count > 0, err
}

func assetHandleStates(tx *gorm.DB, kind, handle string) *gorm.DB {
	kinds := []string{kind}
	if kind == "asset" || kind == "task" {
		kinds = []string{"asset", "task"}
	}
	return tx.Model(&ConfigurableResourceState{}).Where("profile_id = ? AND resource_id IN ? AND state_key = ?", assetBindingProfile, kinds, assetBindingKey(handle))
}

func registerAssetBindings(db *gorm.DB, bindings []AssetBinding, legacy bool) error {
	plan := make(assetBindingLockPlan)
	refs := make(map[string]AssetBinding)
	unique := make([]AssetBinding, 0, len(bindings))
	seen := make(map[AssetBinding]bool, len(bindings))
	for _, binding := range bindings {
		if binding.UserID <= 0 || binding.ChannelID <= 0 || binding.Scope == "" || binding.ID == "" {
			return fmt.Errorf("invalid asset binding")
		}
		if binding.Kind != "asset" && binding.Kind != "group" && binding.Kind != "task" && (legacy || binding.Kind != "session") {
			return fmt.Errorf("unsupported asset kind")
		}
		if seen[binding] {
			continue
		}
		seen[binding] = true
		unique = append(unique, binding)
		plan.include(binding, nil)
		assetIdentityRefs(refs, binding)
	}
	bindings = unique
	if len(bindings) == 0 {
		return nil
	}
	// Historical claims must recheck all scopes under the writer's locks.
	// Ordinary polling of registered handles never enters this claim path.
	if !legacy {
		if current, err := assetBindingsCurrent(db, bindings); err != nil || current {
			return err
		}
	}
	return retryAssetBindingWrite(db, func() error {
		history, err := loadAssetBindingHistory(db, refs)
		if err != nil {
			return err
		}
		for key, node := range history {
			refs[key] = node.ref
			if _, ok := plan[key]; !ok {
				plan[key] = assetBindingSharedLock
			}
		}
		ids, err := prepareAssetBindingLocks(db, plan)
		if err != nil {
			return err
		}
		// Keep the configured isolation; all mutexes precede snapshot reads.
		return assetWriteTransaction(db, func(tx *gorm.DB) error {
			locked, err := lockAssetBindings(tx, ids)
			if err != nil {
				return err
			}
			resolved, err := plan.resolveStored(tx, bindings, locked, refs)
			if err != nil {
				return err
			}
			lifecycle := newAssetBindingLifecycle(tx, locked, history, refs)
			registered, err := assetRegistrationStates(tx, resolved)
			if err != nil {
				return err
			}
			for _, binding := range resolved {
				states := registered[assetBindingLockKey(binding.Kind, assetBindingKey(binding.ID))]
				for _, state := range states {
					// New resources may reuse provider-local IDs in unrelated accounts.
					// Historical claims must reject conflicting registrations in any scope.
					if !legacy && state.PreRequestID != binding.Scope {
						continue
					}
					if state.ChannelID != binding.ChannelID || state.PreRequestID != binding.Scope || state.StateValue != strconv.Itoa(binding.UserID) {
						return ErrAssetNotOwned
					}
					if state.Status != ConfigurableResourceStateStatusActive {
						return ErrAssetRevoked
					}
				}
				if err := saveAssetBinding(tx, binding, lifecycle); err != nil {
					return err
				}
				// Later members of this batch must also see registrations just
				// made in this transaction (including asset/task equivalents).
				key := assetBindingLockKey(binding.Kind, assetBindingKey(binding.ID))
				registered[key] = append(registered[key], ConfigurableResourceState{
					ChannelID: binding.ChannelID, PreRequestID: binding.Scope,
					StateValue: strconv.Itoa(binding.UserID), Status: ConfigurableResourceStateStatusActive,
				})
			}
			return lifecycle.markInitialized()
		})
	})
}

func FindAssetBindings(userID int, kind, handle string) ([]AssetBinding, error) {
	return FindAssetBindingsContext(context.Background(), userID, kind, handle)
}

func FindAssetBindingsContext(ctx context.Context, userID int, kind, handle string) ([]AssetBinding, error) {
	states, _, err := findAssetBindingStates(ctx, userID, []string{kind}, handle)
	if err != nil {
		return nil, err
	}
	bindings, err := decodeAssetBindings(states)
	if err != nil {
		return nil, err
	}
	return filterActiveAssetBindings(DB.WithContext(ctx), bindings)
}

// FindAssetAccessBindingsContext resolves access and historical-claim eligibility
// together. Registered/revoked IDs must not repeat global scans for a task
// fallback and then a separate existence check on every unsuccessful poll.
// Claim writers still revalidate under their global handle mutexes.
func FindAssetAccessBindingsContext(ctx context.Context, userID int, kind, handle string, allowTask bool) ([]AssetBinding, bool, error) {
	kinds := []string{kind}
	if assetBindingKind(kind) == "asset" {
		kinds = []string{"asset", "task"}
	}
	states, registered, err := findAssetBindingStates(ctx, userID, kinds, handle)
	if err != nil {
		return nil, registered, err
	}
	var exact, tasks []ConfigurableResourceState
	for _, state := range states {
		if state.ResourceID == kind {
			exact = append(exact, state)
		} else if kind == "asset" && allowTask && state.ResourceID == "task" {
			tasks = append(tasks, state)
		}
	}
	bindings, err := decodeAssetBindings(exact)
	if err == nil && len(bindings) == 0 && len(tasks) > 0 {
		bindings, err = decodeAssetBindings(tasks)
	}
	if err == nil {
		bindings, err = filterActiveAssetBindings(DB.WithContext(ctx), bindings)
	}
	return bindings, registered, err
}

func findAssetBindingStates(ctx context.Context, userID int, kinds []string, handle string) ([]ConfigurableResourceState, bool, error) {
	db := DB.WithContext(ctx)
	digest := assetBindingKey(handle)
	// Discover IDs from the existing covering scope index first. Filtering
	// owner/status while selecting metadata can make MySQL scan every active
	// state row (including global mutexes) for each polling request.
	ids, err := assetHandleCandidateIDs(db, kinds, []string{digest})
	if err != nil {
		return nil, false, err
	}
	if len(ids) == 0 {
		return nil, false, nil
	}
	var states []ConfigurableResourceState
	// IDs are only candidates, never authorization: recheck the live owner
	// and status so deletion between these reads cannot restore access.
	err = db.Where("id IN ? AND profile_id = ? AND resource_id IN ? AND state_key = ? AND state_value = ? AND status = ?",
		ids, assetBindingProfile, kinds, digest, strconv.Itoa(userID), ConfigurableResourceStateStatusActive).Find(&states).Error
	return states, true, err
}

func ListAssetBindings(channelID, userID int, scope, kind string) ([]AssetBinding, error) {
	return ListAssetBindingsContext(context.Background(), channelID, userID, scope, kind)
}

func ListAssetBindingsContext(ctx context.Context, channelID, userID int, scope, kind string) ([]AssetBinding, error) {
	var bindings []AssetBinding
	err := WalkAssetBindingsContext(ctx, channelID, userID, scope, kind, func(batch []AssetBinding) error {
		bindings = append(bindings, batch...)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return bindings, nil
}

// WalkAssetBindingsContext releases the connection between bounded batches.
// It never holds a read transaction while decoding history or contacting an
// upstream. Callers can build compact handle maps without retaining all rows.
func WalkAssetBindingsContext(ctx context.Context, channelID, userID int, scope, kind string, visit func([]AssetBinding) error) error {
	ctx, cancel := context.WithTimeout(ctx, AssetOperationTimeout)
	defer cancel()
	db := DB.WithContext(ctx)
	query := db.Model(&ConfigurableResourceState{}).Select("id", "metadata").Where("channel_id = ? AND profile_id = ? AND resource_id = ? AND pre_request_id = ? AND state_value = ? AND status = ?",
		channelID, assetBindingProfile, kind, scope, strconv.Itoa(userID), ConfigurableResourceStateStatusActive)
	return walkAssetStateRows(query, func(states []ConfigurableResourceState) error {
		bindings, err := decodeAssetBindings(states)
		if err != nil {
			return err
		}
		bindings, err = filterActiveAssetBindings(db, bindings)
		if err != nil {
			return err
		}
		return visit(bindings)
	})
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
	return InvalidateAssetBindingsContext(context.Background(), binding, deleteGroup)
}

func InvalidateAssetBindingsContext(ctx context.Context, binding AssetBinding, deleteGroup bool) error {
	ctx, cancel := context.WithTimeout(ctx, AssetOperationTimeout)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return err
	}
	if binding.ChannelID <= 0 || binding.UserID <= 0 || binding.Scope == "" || assetCanonicalID(binding) == "" {
		return fmt.Errorf("invalid asset binding")
	}
	if deleteGroup && binding.Kind != "group" {
		return fmt.Errorf("invalid asset group revocation")
	}
	if binding.Kind != "asset" && binding.Kind != "task" && binding.Kind != "group" {
		return fmt.Errorf("unsupported asset revocation kind")
	}
	// Only share an in-flight local revocation, including its slot wait. Include
	// the exact handle as well as the supplied canonical identity: callers may
	// still need the stored row to resolve an older alias's canonical identity.
	// Upstream DELETE responses and completed results are never shared/cached.
	key, err := common.Marshal(struct {
		ChannelID   int
		UserID      int
		Scope       string
		Kind        string
		ID          string
		CanonicalID string
		DeleteGroup bool
	}{binding.ChannelID, binding.UserID, binding.Scope, binding.Kind, binding.ID, assetCanonicalID(binding), deleteGroup})
	if err != nil {
		return err
	}
	result := assetDeletions.DoChan(string(key), func() (any, error) {
		return nil, revokeAssetIdentity(ctx, binding)
	})
	select {
	case outcome := <-result:
		return outcome.Err
	case <-ctx.Done():
		return ctx.Err()
	}
}

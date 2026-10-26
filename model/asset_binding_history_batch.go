package model

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

type assetHistoryRead struct {
	db   *gorm.DB
	ids  []string
	done chan assetHistoryResult
}

type assetHistoryResult struct {
	states []ConfigurableResourceState
	err    error
}

type assetHistoryScope struct {
	pool        gorm.ConnPool
	channel     int
	scope, kind string
}

var assetHistoryReads = struct {
	sync.Mutex
	queues map[assetHistoryScope][]*assetHistoryRead
}{queues: make(map[assetHistoryScope][]*assetHistoryRead)}

// Combine overlapping legacy discovery reads, not their authorization results.
// A burst of new IDs otherwise scans the same account once per creation. There
// is no retained cache: each frozen batch reads the database, and writers still
// validate durable lifecycle state under their existing transaction locks.
func readAssetHistoryCandidates(db *gorm.DB, channel int, scope, kind string, ids []string) ([]ConfigurableResourceState, error) {
	ctx := db.Statement.Context
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	key := assetHistoryScope{db.Statement.ConnPool, channel, scope, kind}
	r := &assetHistoryRead{db: db, ids: ids, done: make(chan assetHistoryResult, 1)}
	assetHistoryReads.Lock()
	_, running := assetHistoryReads.queues[key]
	assetHistoryReads.queues[key] = append(assetHistoryReads.queues[key], r)
	assetHistoryReads.Unlock()
	if !running {
		go runAssetHistoryReads(key)
	}
	select {
	case result := <-r.done:
		return result.states, result.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func runAssetHistoryReads(key assetHistoryScope) {
	for {
		// Also lets a small burst share one selective SQL query.
		time.Sleep(time.Millisecond)
		assetHistoryReads.Lock()
		batch := assetHistoryReads.queues[key]
		if len(batch) == 0 {
			delete(assetHistoryReads.queues, key)
			assetHistoryReads.Unlock()
			return
		}
		assetHistoryReads.queues[key] = nil
		assetHistoryReads.Unlock()
		active := batch[:0]
		for _, r := range batch {
			if r.db.Statement.Context.Err() == nil {
				active = append(active, r)
			}
		}
		if len(active) > 0 {
			executeAssetHistoryReads(key, active)
		}
	}
}

func executeAssetHistoryReads(key assetHistoryScope, batch []*assetHistoryRead) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(batch[0].db.Statement.Context), AssetOperationTimeout)
	defer cancel()
	var remaining atomic.Int32
	remaining.Store(int32(len(batch)))
	for _, r := range batch {
		stop := context.AfterFunc(r.db.Statement.Context, func() {
			if remaining.Add(-1) == 0 {
				cancel()
			}
		})
		defer stop()
	}
	wanted := make(map[string][]int)
	for i, r := range batch {
		for _, id := range r.ids {
			wanted[id] = append(wanted[id], i)
		}
	}
	results := make([]assetHistoryResult, len(batch))
	visit := func(states []ConfigurableResourceState) error {
		for _, state := range states {
			var binding AssetBinding
			if err := common.UnmarshalJsonStr(state.Metadata, &binding); err != nil {
				return err
			}
			ids := []string{assetCanonicalID(binding)}
			if binding.Kind == "group" && binding.ID != ids[0] {
				ids = append(ids, binding.ID)
			}
			for _, id := range ids {
				for _, i := range wanted[id] {
					results[i].states = append(results[i].states, state)
				}
			}
		}
		return ctx.Err()
	}
	db := batch[0].db.WithContext(ctx)
	query := db.Model(&ConfigurableResourceState{}).Select("id", "metadata", "status").Where("channel_id = ? AND profile_id = ? AND pre_request_id = ?", key.channel, assetBindingProfile, key.scope)
	kinds := []string{key.kind}
	if key.kind == "asset" {
		kinds = []string{"asset", "task"}
	}
	var err error
	if len(wanted) <= 8 {
		ids := make([]string, 0, len(wanted))
		for id := range wanted {
			ids = append(ids, id)
		}
		err = walkAssetStateRows(assetMetadataCandidates(query.Where("resource_id IN ?", kinds), ids), visit)
	} else {
		// Match identities in Go in one streaming pass, instead of evaluating
		// hundreds of leading-wildcard LIKE predicates for every historical row.
		// Only requested families are retained; unrelated history is discarded.
		for _, kind := range kinds {
			err = walkScopedAssetStateRows(query.Session(&gorm.Session{}).Where("resource_id = ?", kind), visit)
			if err != nil {
				break
			}
		}
	}
	for i, r := range batch {
		results[i].err = err
		r.done <- results[i]
	}
}

package model

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"gorm.io/gorm"
)

type assetListRead struct {
	db   *gorm.DB
	user int
	done chan assetListReadResult
}

type assetListReadResult struct {
	bindings []AssetBinding
	err      error
}

var assetListReads = struct {
	sync.Mutex
	queues map[assetHistoryScope][]*assetListRead
}{queues: make(map[assetHistoryScope][]*assetListRead)}

// Ownership is stored in the unindexed state_value column. Freeze overlapping
// readers and enumerate their account once, dispatching rows by owner in Go.
// Arrivals after the scan starts wait for a fresh scan, so no caller can receive
// a snapshot taken before its request. Only requested owners' bindings are kept;
// unrelated metadata is neither retained nor decoded. No completed cache exists.
func readAssetListBindings(db *gorm.DB, channel, user int, scope, kind string) ([]AssetBinding, error) {
	ctx := db.Statement.Context
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	key := assetHistoryScope{db.Statement.ConnPool, channel, scope, kind}
	r := &assetListRead{db: db, user: user, done: make(chan assetListReadResult, 1)}
	assetListReads.Lock()
	_, running := assetListReads.queues[key]
	assetListReads.queues[key] = append(assetListReads.queues[key], r)
	assetListReads.Unlock()
	if !running {
		go runAssetListReads(key)
	}
	select {
	case result := <-r.done:
		return result.bindings, result.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func runAssetListReads(key assetHistoryScope) {
	for {
		time.Sleep(time.Millisecond)
		assetListReads.Lock()
		batch := assetListReads.queues[key]
		if len(batch) == 0 {
			delete(assetListReads.queues, key)
			assetListReads.Unlock()
			return
		}
		assetListReads.queues[key] = nil
		assetListReads.Unlock()
		active := batch[:0]
		for _, r := range batch {
			if r.db.Statement.Context.Err() == nil {
				active = append(active, r)
			}
		}
		if len(active) > 0 {
			executeAssetListReads(key, active)
		}
	}
}

func executeAssetListReads(key assetHistoryScope, batch []*assetListRead) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(batch[0].db.Statement.Context), AssetOperationTimeout)
	defer cancel()
	var remaining atomic.Int32
	remaining.Store(int32(len(batch)))
	results := make(map[string]*assetListReadResult)
	for _, r := range batch {
		stop := context.AfterFunc(r.db.Statement.Context, func() {
			if remaining.Add(-1) == 0 {
				cancel()
			}
		})
		defer stop()
		results[strconv.Itoa(r.user)] = &assetListReadResult{}
	}
	db := batch[0].db.WithContext(ctx)
	// Do not put owner/status predicates in either scope discovery or paging:
	// discovery can use the covering index, and each page advances by state_key
	// instead of scanning all account rows to find a single owner's next page.
	query := db.Model(&ConfigurableResourceState{}).Select("id", "metadata", "state_value", "status").
		Where("channel_id = ? AND profile_id = ? AND resource_id = ? AND pre_request_id = ?", key.channel, assetBindingProfile, key.kind, key.scope)
	err := walkScopedAssetStateRows(query, func(states []ConfigurableResourceState) error {
		for _, state := range states {
			result := results[state.StateValue]
			if result == nil || result.err != nil || state.Status != ConfigurableResourceStateStatusActive {
				continue
			}
			bindings, err := decodeAssetBindings([]ConfigurableResourceState{state})
			if err != nil {
				result.err = err
				continue
			}
			for _, binding := range bindings {
				if strconv.Itoa(binding.UserID) != state.StateValue || binding.ChannelID != key.channel || binding.Scope != key.scope || binding.Kind != key.kind {
					result.err = fmt.Errorf("inconsistent stored asset ownership")
					break
				}
				result.bindings = append(result.bindings, binding)
			}
		}
		return ctx.Err()
	})
	for _, r := range batch {
		result := *results[strconv.Itoa(r.user)]
		if err != nil {
			result.err = err
		}
		r.done <- result
	}
}

package controller

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/configurable"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestAssetLoad100K1000MySQL57(t *testing.T) {
	if os.Getenv("ASSET_LOAD_100K_1000") != "1" || os.Getenv("ASSET_TEST_MYSQL_DSN") == "" {
		t.Skip("requires explicit ASSET_LOAD_100K_1000 and isolated MySQL")
	}
	assetLoadCreateAndPollMySQL57(t, 1000, 100000)
}

// Bulk-copy a binding created by the real model API, including its global
// handle mutex and v2 identity. This represents current-format inventory, not
// legacy rows which would trigger a different migration/discovery workload.
func seedAssetScaleHistory(t *testing.T, count int, groupID string) int {
	t.Helper()
	ch, err := model.GetChannelById(20, true)
	require.NoError(t, err)
	profile, ok := configurable.AssetProfile(ch.GetSetting().Protocol)
	require.True(t, ok)
	scope, err := assetAccountScope(ch, profile)
	require.NoError(t, err)
	var previous int
	require.NoError(t, model.DB.Model(&model.ConfigurableResourceState{}).Select("COALESCE(MAX(id),0)").Scan(&previous).Error)
	binding := model.AssetBinding{ChannelID: 20, UserID: 1, Backend: profile.ID, Scope: scope, Kind: "asset", ID: "history-000000", CanonicalID: "history-000000", Project: "nmyk", GroupID: groupID}
	require.NoError(t, model.SaveAssetBindings([]model.AssetBinding{binding}))
	var prototype []model.ConfigurableResourceState
	require.NoError(t, model.DB.Where("id > ?", previous).Order("id").Find(&prototype).Error)
	require.Len(t, prototype, 3, "one ownership row, global handle and identity per asset")
	hash := func(s string) string { return fmt.Sprintf("%x", sha256.Sum256([]byte(s))) }
	db := model.DB.Session(&gorm.Session{SkipDefaultTransaction: true, PrepareStmt: false, Logger: logger.Default.LogMode(logger.Silent)})
	for start := 1; start < count; start += 500 {
		rows := make([]model.ConfigurableResourceState, 0, 1500)
		for i := start; i < min(start+500, count); i++ {
			id := fmt.Sprintf("history-%06d", i)
			b := binding
			b.ID, b.CanonicalID = id, id
			metadata, e := common.Marshal(b)
			require.NoError(t, e)
			for _, original := range prototype {
				row := original
				row.Id = 0
				switch {
				case row.ProfileID == "asset-access-v1":
					row.StateKey, row.Metadata = hash(id), string(metadata)
				case strings.HasPrefix(row.StateKey, "identity:"):
					row.StateKey = "identity:" + hash(fmt.Sprintf("%d:%s:asset:%s", b.ChannelID, hash(b.Scope), hash(id)))
				default:
					row.StateKey = "asset:" + hash(id)
				}
				rows = append(rows, row)
			}
		}
		require.NoError(t, db.Create(&rows).Error)
	}
	require.NoError(t, db.Exec("ANALYZE TABLE configurable_resource_states").Error)
	var actual int64
	require.NoError(t, db.Model(&model.ConfigurableResourceState{}).Where("profile_id = ? AND resource_id = ?", "asset-access-v1", "asset").Count(&actual).Error)
	require.EqualValues(t, count, actual)
	for _, i := range []int{0, count / 2, count - 1} {
		found, e := model.FindScopedAssetBindingsContext(context.Background(), 20, 1, scope, "asset", fmt.Sprintf("history-%06d", i))
		require.NoError(t, e)
		require.Len(t, found, 1)
		require.Equal(t, groupID, found[0].GroupID)
	}
	var lastID int
	require.NoError(t, db.Model(&model.ConfigurableResourceState{}).Select("MAX(id)").Scan(&lastID).Error)
	return lastID
}

type assetScaleDBPhase struct {
	Samples                int     `json:"samples"`
	PoolPeak               int     `json:"pool_peak"`
	FullSamples            int     `json:"full_samples"`
	FullLongest            int     `json:"full_longest_samples"`
	LockWaitersPeak        int     `json:"lock_waiters_peak"`
	LockedTransactionsPeak int     `json:"locked_transactions_peak"`
	LockAgeUpperMS         float64 `json:"locked_transaction_age_upper_ms"`
	WaitAgeUpperMS         float64 `json:"lock_wait_age_upper_ms"`
	Errors                 int     `json:"monitor_errors"`
	streak                 int
}

type assetScaleMonitor struct {
	phase  atomic.Value
	stop   chan struct{}
	done   chan struct{}
	mu     sync.Mutex
	Phases map[string]*assetScaleDBPhase `json:"phases"`
}

func assetScaleStatsCopy(s *assetLoadRequests) *assetLoadRequests {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := &assetLoadRequests{Count: s.Count, HTTP: map[int]int{}, States: map[string]int{}, Errors: append([]string(nil), s.Errors...), durations: append([]float64(nil), s.durations...)}
	for k, v := range s.HTTP {
		c.HTTP[k] = v
	}
	for k, v := range s.States {
		c.States[k] = v
	}
	c.peak.Store(s.peak.Load())
	c.finish()
	return c
}

func startAssetScaleMonitor(t *testing.T, pool, monitor *sql.DB, schema string) *assetScaleMonitor {
	t.Helper()
	m := &assetScaleMonitor{stop: make(chan struct{}), done: make(chan struct{}), Phases: map[string]*assetScaleDBPhase{}}
	m.phase.Store("setup")
	var file *os.File
	if reportPath := os.Getenv("ASSET_LOAD_REPORT_PATH"); reportPath != "" {
		var err error
		file, err = os.OpenFile(filepath.Join(filepath.Dir(reportPath), "db-samples.jsonl"), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
		require.NoError(t, err)
	}
	go func() {
		defer close(m.done)
		if file != nil {
			defer file.Close()
		}
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		started := time.Now()
		for {
			phase := m.phase.Load().(string)
			inUse := pool.Stats().InUse
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			rows, err := monitor.QueryContext(ctx, "SELECT t.trx_state,t.trx_rows_locked,TIMESTAMPDIFF(MICROSECOND,t.trx_started,NOW(6)),IFNULL(TIMESTAMPDIFF(MICROSECOND,t.trx_wait_started,NOW(6)),0) FROM information_schema.innodb_trx t JOIN information_schema.processlist p ON p.ID=t.trx_mysql_thread_id WHERE p.DB=?", schema)
			waiters, locked := 0, 0
			var lockAge, waitAge int64
			if err == nil {
				for rows.Next() {
					var state string
					var locks, age, wait int64
					if err = rows.Scan(&state, &locks, &age, &wait); err != nil {
						break
					}
					if locks > 0 {
						locked++
						lockAge = max(lockAge, age)
					}
					if state == "LOCK WAIT" {
						waiters++
						waitAge = max(waitAge, wait)
					}
				}
				if err == nil {
					err = rows.Err()
				}
				_ = rows.Close()
			}
			cancel()
			m.mu.Lock()
			s := m.Phases[phase]
			if s == nil {
				s = &assetScaleDBPhase{}
				m.Phases[phase] = s
			}
			s.Samples++
			s.PoolPeak = max(s.PoolPeak, inUse)
			if inUse == 30 {
				s.FullSamples++
				s.streak++
				s.FullLongest = max(s.FullLongest, s.streak)
			} else {
				s.streak = 0
			}
			s.LockWaitersPeak = max(s.LockWaitersPeak, waiters)
			s.LockedTransactionsPeak = max(s.LockedTransactionsPeak, locked)
			s.LockAgeUpperMS = max(s.LockAgeUpperMS, float64(lockAge)/1000)
			s.WaitAgeUpperMS = max(s.WaitAgeUpperMS, float64(waitAge)/1000)
			if err != nil {
				s.Errors++
			}
			m.mu.Unlock()
			if file != nil {
				data, e := common.Marshal(map[string]any{"elapsed_ms": time.Since(started).Milliseconds(), "phase": phase, "pool_in_use": inUse, "lock_waiters": waiters, "locked_transactions": locked, "lock_age_upper_us": lockAge, "wait_age_upper_us": waitAge, "error": err != nil})
				if e == nil {
					_, e = file.Write(append(data, '\n'))
				}
				if e != nil {
					t.Errorf("write database sample: %v", e)
				}
			}
			select {
			case <-m.stop:
				return
			case <-ticker.C:
			}
		}
	}()
	return m
}

func (m *assetScaleMonitor) close() { close(m.stop); <-m.done }

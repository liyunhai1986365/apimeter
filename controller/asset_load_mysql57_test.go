package controller

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/internal/testdb"
	"github.com/QuantumNous/new-api/model"
	"github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"gorm.io/gorm"
)

type assetLoadRequests struct {
	Count        int            `json:"count"`
	HTTP         map[int]int    `json:"http"`
	Errors       []string       `json:"errors,omitempty"`
	States       map[string]int `json:"states,omitempty"`
	P50MS        float64        `json:"p50_ms"`
	P95MS        float64        `json:"p95_ms"`
	P99MS        float64        `json:"p99_ms"`
	MaxMS        float64        `json:"max_ms"`
	PeakInFlight int64          `json:"peak_in_flight"`
	mu           sync.Mutex
	inFlight     atomic.Int64
	peak         atomic.Int64
	durations    []float64
}

func (s *assetLoadRequests) record(code int, elapsed time.Duration, err error, state string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.HTTP == nil {
		s.HTTP = make(map[int]int)
		s.States = make(map[string]int)
	}
	s.Count++
	s.HTTP[code]++
	s.durations = append(s.durations, float64(elapsed)/float64(time.Millisecond))
	if err != nil && len(s.Errors) < 10 {
		s.Errors = append(s.Errors, err.Error())
	}
	if state != "" {
		s.States[state]++
	}
}

func (s *assetLoadRequests) finish() {
	sort.Float64s(s.durations)
	if len(s.durations) == 0 {
		return
	}
	percentile := func(p float64) float64 { return s.durations[int(math.Ceil(p*float64(len(s.durations))))-1] }
	s.P50MS, s.P95MS, s.P99MS, s.MaxMS = percentile(.5), percentile(.95), percentile(.99), percentile(1)
	s.PeakInFlight = s.peak.Load()
}

func assetLoadPeak(peak *atomic.Int64, value int64) {
	for previous := peak.Load(); value > previous; previous = peak.Load() {
		if peak.CompareAndSwap(previous, value) {
			return
		}
	}
}

// Opt in explicitly: real MySQL 5.7, production HTTP handlers and auth, local
// upstream only. At most one in-flight request per client; no
// overlapping rounds or unbounded request backlog. This is not a live API test.
func TestAssetLoad100CreateAndPollMySQL57(t *testing.T) {
	if os.Getenv("ASSET_LOAD_100") != "1" || os.Getenv("ASSET_TEST_MYSQL_DSN") == "" {
		t.Skip("requires ASSET_LOAD_100=1 and an isolated MySQL 5.7 test server")
	}
	assetLoadCreateAndPollMySQL57(t, 100)
}

func TestAssetLoad1000CreateAndPollMySQL57(t *testing.T) {
	if os.Getenv("ASSET_LOAD_1000") != "1" || os.Getenv("ASSET_TEST_MYSQL_DSN") == "" {
		t.Skip("requires ASSET_LOAD_1000=1 and an isolated MySQL 5.7 test server")
	}
	assetLoadCreateAndPollMySQL57(t, 1000)
}

func assetLoadCreateAndPollMySQL57(t *testing.T, clients int) {
	t.Helper()
	const rounds = 20
	const interval = 3 * time.Second
	groupID := fmt.Sprintf("group-load-%d", clients)
	var upstreamMu sync.Mutex
	upstreamQueries := make(map[string]int)
	var created int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		timer := time.NewTimer(20 * time.Millisecond)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-r.Context().Done():
			return
		}
		upstreamMu.Lock()
		defer upstreamMu.Unlock()
		id, state := gjson.GetBytes(body, "Id").String(), "Processing"
		switch r.URL.Query().Get("Action") {
		case "CreateAsset":
			created++
			id = fmt.Sprintf("asset-load-%03d", created)
			upstreamQueries[id] = 0
		case "GetAsset":
			if _, ok := upstreamQueries[id]; !ok {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			upstreamQueries[id]++
			if upstreamQueries[id] > 3 {
				state = "Active"
			}
		default:
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"Result":{"Id":%q,"GroupId":%q,"ProjectName":"nmyk","Status":%q}}`, id, groupID, state)
	}))
	defer upstream.Close()
	router := assetSecurityRouter(t, upstream.URL, "volcengine-assets", "")
	var version, schema string
	require.NoError(t, model.DB.Raw("SELECT VERSION()").Scan(&version).Error)
	require.True(t, strings.HasPrefix(version, "5.7."), "this load test is restricted to MySQL 5.7")
	require.NoError(t, model.DB.Raw("SELECT DATABASE()").Scan(&schema).Error)
	model.DB = model.DB.Session(&gorm.Session{PrepareStmt: true})
	pool, err := model.DB.DB()
	require.NoError(t, err)
	pool.SetMaxOpenConns(30)
	pool.SetMaxIdleConns(30)
	logDB := testdb.AssetOpener(t, "unused")()
	require.NoError(t, logDB.AutoMigrate(&model.Log{}))
	model.LOG_DB = logDB
	logPool, err := logDB.DB()
	require.NoError(t, err)
	logPool.SetMaxOpenConns(5)
	logPool.SetMaxIdleConns(5)
	ch, err := model.GetChannelById(20, true)
	require.NoError(t, err)
	settings := ch.GetSetting()
	settings.Protocol.ProjectName = "nmyk"
	ch.SetSetting(settings)
	require.NoError(t, model.DB.Save(ch).Error)
	seedAssetOwnershipForTest(t, 20, 1, "group", groupID)
	router.GET("/api/ready", GetReadiness)
	router.GET("/api/status", GetStatus)
	gateway := httptest.NewServer(router)
	defer gateway.Close()
	transport := &http.Transport{MaxIdleConns: clients + 16, MaxIdleConnsPerHost: clients + 16, MaxConnsPerHost: clients + 16}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 20 * time.Second}
	ctx, cancel := context.WithTimeout(context.Background(), 110*time.Second)
	defer cancel()
	var creates, polls, ready, status assetLoadRequests
	call := func(stats *assetLoadRequests, method, path, body, wantID string) (string, error) {
		active := stats.inFlight.Add(1)
		assetLoadPeak(&stats.peak, active)
		defer stats.inFlight.Add(-1)
		start := time.Now()
		req, err := http.NewRequestWithContext(ctx, method, gateway.URL+path, strings.NewReader(body))
		if err != nil {
			stats.record(0, time.Since(start), err, "")
			return "", err
		}
		req.Header.Set("Authorization", "Bearer seedancetest1")
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			stats.record(0, time.Since(start), err, "")
			return "", err
		}
		raw, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		id, state := gjson.GetBytes(raw, "Result.Id").String(), gjson.GetBytes(raw, "Result.Status").String()
		if err == nil && resp.StatusCode != http.StatusOK {
			err = fmt.Errorf("%s %s: HTTP %d: %.250s", method, path, resp.StatusCode, raw)
		}
		if err == nil && strings.HasPrefix(path, "/api/assets") && (id == "" || (wantID != "" && id != wantID) || (state != "Processing" && state != "Active")) {
			err = fmt.Errorf("invalid asset ID or state: %.250s", raw)
		}
		if err == nil && path == "/api/ready" && (!gjson.GetBytes(raw, "success").Bool() || gjson.GetBytes(raw, "data.checks.database").String() != "ok" || gjson.GetBytes(raw, "data.checks.log_database").String() != "ok") {
			err = fmt.Errorf("readiness check failed: %.250s", raw)
		}
		stats.record(resp.StatusCode, time.Since(start), err, state)
		return id, err
	}
	_, err = call(&ready, http.MethodGet, "/api/ready", "", "")
	require.NoError(t, err)
	// Monitor on a dedicated connection so collecting evidence cannot wait
	// behind the application's 30-connection pool. Never reset global counters.
	cfg, err := mysql.ParseDSN(os.Getenv("ASSET_TEST_MYSQL_DSN"))
	require.NoError(t, err)
	cfg.DBName, cfg.Timeout = schema, 2*time.Second
	monitor, err := sql.Open("mysql", cfg.FormatDSN())
	require.NoError(t, err)
	defer monitor.Close()
	monitor.SetMaxOpenConns(1)
	before, logBefore := pool.Stats(), logPool.Stats()
	var poolPeak, logPeak, waiterPeak, monitorErrors atomic.Int64
	stop := make(chan struct{})
	var background sync.WaitGroup
	background.Add(3)
	for _, probe := range []struct {
		path  string
		every time.Duration
		stats *assetLoadRequests
	}{{"/api/ready", 500 * time.Millisecond, &ready}, {"/api/status", interval, &status}} {
		go func() {
			defer background.Done()
			ticker := time.NewTicker(probe.every)
			defer ticker.Stop()
			for {
				_, _ = call(probe.stats, http.MethodGet, probe.path, "", "")
				select {
				case <-ticker.C:
				case <-stop:
					return
				}
			}
		}()
	}
	go func() {
		defer background.Done()
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			assetLoadPeak(&poolPeak, int64(pool.Stats().InUse))
			assetLoadPeak(&logPeak, int64(logPool.Stats().InUse))
			var waiters int64
			probeCtx, probeCancel := context.WithTimeout(ctx, time.Second)
			err := monitor.QueryRowContext(probeCtx, "SELECT COUNT(*) FROM information_schema.innodb_trx t JOIN information_schema.processlist p ON p.ID=t.trx_mysql_thread_id WHERE p.DB=? AND t.trx_state='LOCK WAIT'", schema).Scan(&waiters)
			probeCancel()
			if err != nil {
				monitorErrors.Add(1)
			} else {
				assetLoadPeak(&waiterPeak, waiters)
			}
			select {
			case <-ticker.C:
			case <-stop:
				return
			}
		}
	}()
	var stopOnce sync.Once
	stopBackground := func() { stopOnce.Do(func() { close(stop) }); background.Wait() }
	defer stopBackground()
	started := time.Now()
	ids := make([]string, clients)
	gate := make(chan struct{})
	var workers sync.WaitGroup
	for i := range clients {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-gate
			ids[i], _ = call(&creates, http.MethodPost, "/api/assets", `{"GroupId":"`+groupID+`","ProjectName":"nmyk","AssetType":"Image","URL":"https://example.com/load.png"}`, "")
		}()
	}
	close(gate)
	workers.Wait()
	var ownershipBefore, ownershipAfter []model.ConfigurableResourceState
	require.NoError(t, model.DB.Where("profile_id IN ?", []string{"asset-access-v1", "asset-access-lock-v1"}).Order("id").Find(&ownershipBefore).Error)
	t.Logf("creation phase: %d requests, HTTP=%v, %.3fs", creates.Count, creates.HTTP, time.Since(started).Seconds())
	pollStarted := time.Now()
	var lateRounds int
	for round := range rounds {
		if round > 0 {
			delay := time.Until(pollStarted.Add(time.Duration(round) * interval))
			if delay > 0 {
				timer := time.NewTimer(delay)
				select {
				case <-timer.C:
				case <-ctx.Done():
				}
				timer.Stop()
			} else {
				lateRounds++ // Never accumulate another wave over unfinished requests.
			}
		}
		gate := make(chan struct{})
		for _, id := range ids {
			workers.Add(1)
			go func() {
				defer workers.Done()
				<-gate
				if id != "" {
					_, _ = call(&polls, http.MethodGet, "/api/assets/"+id, "", id)
				}
			}()
		}
		close(gate)
		workers.Wait()
		if round%5 == 4 {
			t.Logf("poll round %d/%d: requests=%d HTTP=%v pool_in_use=%d", round+1, rounds, polls.Count, polls.HTTP, pool.Stats().InUse)
		}
	}
	stopBackground()
	after, logAfter := pool.Stats(), logPool.Stats()
	for _, stats := range []*assetLoadRequests{&creates, &polls, &ready, &status} {
		stats.finish()
	}
	report := map[string]any{
		"mysql_version": version, "clients": clients, "rounds": rounds, "interval_seconds": interval.Seconds(),
		"duration_seconds": time.Since(started).Seconds(), "upstream_delay_ms": 20, "redis": "disabled", "instances": 1,
		"creates": &creates, "polls": &polls, "ready": &ready, "status": &status, "late_rounds": lateRounds,
		"main_pool_limit": 30, "main_pool_peak_in_use": poolPeak.Load(), "main_pool_wait_count": after.WaitCount - before.WaitCount,
		"main_pool_wait_ms": float64(after.WaitDuration-before.WaitDuration) / float64(time.Millisecond),
		"log_pool_limit":    5, "log_pool_peak_in_use": logPeak.Load(), "log_pool_wait_count": logAfter.WaitCount - logBefore.WaitCount,
		"sampled_server_lock_waiters_peak": waiterPeak.Load(), "monitor_errors": monitorErrors.Load(),
	}
	raw, err := common.Marshal(report)
	require.NoError(t, err)
	t.Logf("ASSET_LOAD_REPORT %s", raw)
	if path := os.Getenv("ASSET_LOAD_REPORT_PATH"); path != "" {
		require.NoError(t, os.WriteFile(path, append(raw, '\n'), 0600))
	}
	require.Equal(t, clients, creates.HTTP[http.StatusOK])
	require.Equal(t, clients*rounds, polls.HTTP[http.StatusOK])
	for _, stats := range []*assetLoadRequests{&creates, &polls, &ready, &status} {
		require.Empty(t, stats.Errors)
		require.Zero(t, stats.HTTP[http.StatusServiceUnavailable])
	}
	require.Zero(t, lateRounds)
	require.Zero(t, monitorErrors.Load())
	require.Equal(t, clients*3, polls.States["Processing"])
	require.Equal(t, clients*(rounds-3), polls.States["Active"])
	require.NoError(t, model.DB.Where("profile_id IN ?", []string{"asset-access-v1", "asset-access-lock-v1"}).Order("id").Find(&ownershipAfter).Error)
	require.Equal(t, ownershipBefore, ownershipAfter, "unchanged status polling must not rewrite ownership or mutex rows")
	for _, id := range ids {
		bindings, err := model.FindAssetBindings(1, "asset", id)
		require.NoError(t, err)
		require.Len(t, bindings, 1)
	}
}

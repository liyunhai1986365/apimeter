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
	"github.com/QuantumNous/new-api/relay/channel/configurable"
	"github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
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

func assetLoadCreateAndPollMySQL57(t *testing.T, clients int, inventory ...int) {
	t.Helper()
	rounds, interval, historical := 20, 3*time.Second, 0
	upstreamDelay := 20 * time.Millisecond
	if len(inventory) > 0 {
		historical = inventory[0]
		rounds = 3
		interval = 0
		upstreamDelay = time.Millisecond
	}
	groupID := fmt.Sprintf("group-load-%d", clients)
	var upstreamMu sync.Mutex
	upstreamQueries := make(map[string]int)
	var created int
	var upstreamCreates, upstreamGets, upstreamLists atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		timer := time.NewTimer(upstreamDelay)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-r.Context().Done():
			return
		}
		id, state := gjson.GetBytes(body, "Id").String(), "Processing"
		switch r.URL.Query().Get("Action") {
		case "CreateAsset":
			upstreamCreates.Add(1)
			upstreamMu.Lock()
			created++
			id = fmt.Sprintf("asset-load-%03d", created)
			upstreamQueries[id] = 0
			upstreamMu.Unlock()
		case "GetAsset":
			upstreamGets.Add(1)
			if strings.HasPrefix(id, "history-") {
				var index int
				if _, e := fmt.Sscanf(id, "history-%d", &index); e == nil && index >= 0 && index < historical {
					state = "Active"
					break
				}
			}
			upstreamMu.Lock()
			if _, ok := upstreamQueries[id]; !ok {
				upstreamMu.Unlock()
				w.WriteHeader(http.StatusNotFound)
				return
			}
			upstreamQueries[id]++
			if upstreamQueries[id] > 3 {
				state = "Active"
			}
			upstreamMu.Unlock()
		case "ListAssets":
			upstreamLists.Add(1)
			page, size := int(gjson.GetBytes(body, "PageNumber").Int()), int(gjson.GetBytes(body, "PageSize").Int())
			if page < 1 {
				page = 1
			}
			if size < 1 {
				size = 100
			}
			upstreamMu.Lock()
			total := historical + created
			upstreamMu.Unlock()
			items := make([]map[string]any, 0, size)
			for i := (page - 1) * size; i < min(page*size, total); i++ {
				itemID := fmt.Sprintf("history-%06d", i)
				if i >= historical {
					itemID = fmt.Sprintf("asset-load-%03d", i-historical+1)
				}
				items = append(items, map[string]any{"Id": itemID, "GroupId": groupID, "ProjectName": "nmyk", "Status": "Active"})
			}
			data, e := common.Marshal(map[string]any{"Result": map[string]any{"Items": items, "TotalCount": total, "PageNumber": page, "PageSize": size}})
			if e != nil {
				w.WriteHeader(500)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(data)
			return
		default:
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"Result":{"Id":%q,"GroupId":%q,"ProjectName":"nmyk","Status":%q}}`, id, groupID, state)
	}))
	defer upstream.Close()
	backend := "volcengine-assets"
	if historical > 0 {
		backend = configurable.ArkCompatibleAssetBackend
	}
	router := assetSecurityRouter(t, upstream.URL, backend, "")
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
	historyLastID := 0
	seedStarted := time.Now()
	if historical > 0 {
		historyLastID = seedAssetScaleHistory(t, historical, groupID)
	}
	seedSeconds := time.Since(seedStarted).Seconds()
	trace := &arkLiveSQLTrace{Interface: logger.Default.LogMode(logger.Silent), errors: map[string]int{}}
	if historical > 0 {
		model.DB = model.DB.Session(&gorm.Session{Logger: trace})
		t.Logf("seeded %d assets, %d state rows, %.3fs", historical, historyLastID, seedSeconds)
	}
	router.GET("/api/ready", GetReadiness)
	router.GET("/api/status", GetStatus)
	gateway := httptest.NewServer(router)
	defer gateway.Close()
	transport := &http.Transport{MaxIdleConns: clients + 16, MaxIdleConnsPerHost: clients + 16, MaxConnsPerHost: clients + 16}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 20 * time.Second}
	testTimeout := 110 * time.Second
	if historical > 0 {
		testTimeout = 5 * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	var creates, polls, ready, status, historicalPolls, singleList, lists assetLoadRequests
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
		if err == nil && strings.HasPrefix(path, "/api/assets/") || (err == nil && path == "/api/assets" && method == http.MethodPost) {
			if id == "" || (wantID != "" && id != wantID) || (state != "Processing" && state != "Active") {
				err = fmt.Errorf("invalid asset ID or state: %.250s", raw)
			}
		}
		if err == nil && path == "/api/assets" && method == http.MethodGet && (!gjson.GetBytes(raw, "Result.Items").IsArray() || gjson.GetBytes(raw, "Result.TotalCount").Int() < int64(historical)) {
			err = fmt.Errorf("invalid list result: %.250s", raw)
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
	var scale *assetScaleMonitor
	if historical > 0 {
		scale = startAssetScaleMonitor(t, pool, monitor, schema)
		defer scale.close()
	}
	before, logBefore := pool.Stats(), logPool.Stats()
	var poolPeak, logPeak, waiterPeak, monitorErrors atomic.Int64
	stop := make(chan struct{})
	var background sync.WaitGroup
	background.Add(3)
	for _, probe := range []struct {
		path  string
		every time.Duration
		stats *assetLoadRequests
	}{{"/api/ready", 500 * time.Millisecond, &ready}, {"/api/status", 3 * time.Second, &status}} {
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
		if scale != nil {
			return
		}
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
	recoveryMS := float64(0)
	persistScale := func(phase string) {
		if scale == nil {
			return
		}
		trace.mu.Lock()
		sqlData := map[string]any{"count": trace.count, "lock_queries": trace.lockQueries, "max_lock_query_ms": trace.maxLockQueryMS, "max_ms": trace.maxMS, "slow_over_100ms": trace.slow, "errors": trace.errors}
		scale.mu.Lock()
		after := pool.Stats()
		report := map[string]any{"phase_completed": phase, "historical_assets": historical, "initial_state_rows": historyLastID, "seed_seconds": seedSeconds, "clients": clients, "rounds": rounds, "backend": backend, "mysql_version": version, "upstream": "mock", "upstream_delay_ms": upstreamDelay.Milliseconds(), "duration_seconds": time.Since(started).Seconds(), "main_pool_limit": 30, "main_pool_end_in_use": after.InUse, "main_pool_wait_count": after.WaitCount - before.WaitCount, "main_pool_wait_ms": float64(after.WaitDuration-before.WaitDuration) / float64(time.Millisecond), "database_samples": scale.Phases, "sample_interval_ms": 100, "sql_trace": sqlData, "historical_polls": assetScaleStatsCopy(&historicalPolls), "creates": assetScaleStatsCopy(&creates), "polls": assetScaleStatsCopy(&polls), "single_list": assetScaleStatsCopy(&singleList), "lists": assetScaleStatsCopy(&lists), "ready": assetScaleStatsCopy(&ready), "status": assetScaleStatsCopy(&status)}
		report["upstream_requests"] = map[string]int64{"CreateAsset": upstreamCreates.Load(), "GetAsset": upstreamGets.Load(), "ListAssets": upstreamLists.Load()}
		report["recovery_ms"] = recoveryMS
		data, e := common.Marshal(report)
		scale.mu.Unlock()
		trace.mu.Unlock()
		if e == nil {
			if path := os.Getenv("ASSET_LOAD_REPORT_PATH"); path != "" {
				e = os.WriteFile(path, append(data, '\n'), 0600)
			} else {
				t.Logf("ASSET_SCALE_REPORT %s", data)
			}
		}
		if e != nil {
			t.Errorf("persist scale report: %v", e)
		}
	}
	if scale != nil {
		scale.phase.Store("historical_poll")
		var wave sync.WaitGroup
		gate := make(chan struct{})
		for i := range clients {
			wave.Add(1)
			go func() {
				defer wave.Done()
				<-gate
				id := fmt.Sprintf("history-%06d", i*(historical/clients))
				_, _ = call(&historicalPolls, "GET", "/api/assets/"+id, "", id)
			}()
		}
		close(gate)
		wave.Wait()
		t.Logf("historical poll: HTTP=%v elapsed=%.3fs", historicalPolls.HTTP, time.Since(started).Seconds())
		persistScale("historical_poll")
		scale.phase.Store("create")
	}
	ids := make([]string, clients)
	createBody := `{"GroupId":"` + groupID + `","ProjectName":"nmyk","AssetType":"Image","URL":"https://example.com/load.png"}`
	if historical > 0 {
		createBody = `{"GroupId":"` + groupID + `","AssetType":"Image","URL":"https://example.com/load.png"}`
	}
	gate := make(chan struct{})
	var workers sync.WaitGroup
	for i := range clients {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-gate
			ids[i], _ = call(&creates, http.MethodPost, "/api/assets", createBody, "")
		}()
	}
	close(gate)
	workers.Wait()
	var ownershipBefore, ownershipAfter []model.ConfigurableResourceState
	require.NoError(t, model.DB.Where("id > ? AND profile_id IN ?", historyLastID, []string{"asset-access-v1", "asset-access-lock-v1"}).Order("id").Find(&ownershipBefore).Error)
	t.Logf("creation phase: %d requests, HTTP=%v, %.3fs", creates.Count, creates.HTTP, time.Since(started).Seconds())
	persistScale("create")
	if scale != nil {
		scale.phase.Store("new_asset_poll")
	}
	pollStarted := time.Now()
	var lateRounds int
	for round := range rounds {
		if round > 0 && interval > 0 {
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
		if scale != nil {
			t.Logf("new asset poll round=%d HTTP=%v", round+1, polls.HTTP)
			persistScale(fmt.Sprintf("poll_%d", round+1))
		}
		if round%5 == 4 {
			t.Logf("poll round %d/%d: requests=%d HTTP=%v pool_in_use=%d", round+1, rounds, polls.Count, polls.HTTP, pool.Stats().InUse)
		}
	}
	if scale != nil {
		scale.phase.Store("single_list")
		_, _ = call(&singleList, "GET", "/api/assets", `{"Filter":{"GroupType":"AIGC"},"PageNumber":1,"PageSize":20}`, "")
		t.Logf("single list: HTTP=%v", singleList.HTTP)
		persistScale("single_list")
		scale.phase.Store("list_1000")
		gate := make(chan struct{})
		for range clients {
			workers.Add(1)
			go func() {
				defer workers.Done()
				<-gate
				_, _ = call(&lists, "GET", "/api/assets", `{"Filter":{"GroupType":"AIGC"},"PageNumber":1,"PageSize":20}`, "")
			}()
		}
		close(gate)
		workers.Wait()
		t.Logf("list wave: HTTP=%v", lists.HTTP)
		persistScale("list_1000")
	}
	stopBackground()
	if scale != nil {
		scale.phase.Store("recovery")
		recoveryStarted := time.Now()
		for pool.Stats().InUse > 0 && time.Since(recoveryStarted) < 5*time.Second {
			time.Sleep(50 * time.Millisecond)
		}
		recoveryMS = float64(time.Since(recoveryStarted)) / float64(time.Millisecond)
		_, _ = call(&ready, http.MethodGet, "/api/ready", "", "")
		scale.phase.Store("finished")
		persistScale("finished")
		for _, s := range []*assetLoadRequests{&historicalPolls, &creates, &polls, &singleList, &lists, &ready, &status} {
			require.Empty(t, s.Errors)
		}
		return
	}
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

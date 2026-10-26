package controller

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/internal/testdb"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/configurable"
	relaydto "github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type arkLiveTransport func(*http.Request) (*http.Response, error)

func (f arkLiveTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type arkLiveSQLTrace struct {
	logger.Interface
	mu                       sync.Mutex
	count, slow, lockQueries int64
	maxMS, maxLockQueryMS    float64
	errors                   map[string]int
}

func (l *arkLiveSQLTrace) Trace(_ context.Context, begin time.Time, query func() (string, int64), err error) {
	elapsed := float64(time.Since(begin)) / float64(time.Millisecond)
	sqlText, _ := query()
	upper := strings.ToUpper(sqlText)
	l.mu.Lock()
	defer l.mu.Unlock()
	l.count++
	l.maxMS = max(l.maxMS, elapsed)
	if elapsed >= 100 {
		l.slow++
	}
	if strings.Contains(upper, "FOR UPDATE") || strings.Contains(upper, "LOCK IN SHARE MODE") {
		l.lockQueries++
		l.maxLockQueryMS = max(l.maxLockQueryMS, elapsed)
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		code := "other"
		var mysqlErr *mysql.MySQLError
		if errors.As(err, &mysqlErr) {
			code = fmt.Sprint(mysqlErr.Number)
		}
		if errors.Is(err, context.DeadlineExceeded) {
			code = "deadline_exceeded"
		}
		l.errors[code]++
	}
}

func arkLiveResult(body []byte) gjson.Result {
	root := gjson.ParseBytes(body)
	if result := root.Get("Result"); result.IsObject() {
		return result
	}
	return root
}

// Explicit opt-in, private credentials, random isolated MySQL schemas only.
// Creates one smoke asset and 200 simultaneous assets in one temporary group.
// Creation is never retried. A durable upstream ID journal precedes local
// binding persistence, so cleanup also covers local failures after creation.
func TestArkCompatibleAssetsLive200(t *testing.T) {
	arkCompatibleAssetsLive200(t, false)
}

// The same-asset wave also exercises contention when all readers share a
// binding, without creating another batch after a supplier creation throttle.
func TestArkCompatibleAssetQueryLive200(t *testing.T) {
	arkCompatibleAssetsLive200(t, true)
}

func arkCompatibleAssetsLive200(t *testing.T, queryOnly bool) {
	configPath, evidence := os.Getenv("ARK_ASSETS_LIVE_CONFIG"), os.Getenv("ARK_ASSETS_LIVE_EVIDENCE")
	if configPath == "" || os.Getenv("ARK_ASSETS_LIVE_200") != "1" {
		t.Skip("requires explicit ARK_ASSETS_LIVE_200=1 and private config/evidence paths")
	}
	var cfg struct {
		URL, Key, Namespace string
		AssetURL            string `json:"asset_url"`
		MySQLDSN            string `json:"mysql_dsn"`
	}
	raw, err := os.ReadFile(configPath)
	require.NoError(t, err)
	require.NoError(t, common.Unmarshal(raw, &cfg))
	require.NotEmpty(t, cfg.Key)
	require.NotEmpty(t, cfg.AssetURL)
	require.NotEmpty(t, cfg.MySQLDSN)
	require.NotEmpty(t, evidence)
	require.NoError(t, os.MkdirAll(evidence, 0700))
	t.Setenv("ASSET_TEST_MYSQL_DSN", cfg.MySQLDSN)
	r := tgxMaasResourceTestRouter(t, cfg.URL, "fixture-placeholder", tgxRegressionModel)
	registerTgxConversionTestRoutes(t, r)
	sqlTrace := &arkLiveSQLTrace{Interface: logger.Default.LogMode(logger.Silent), errors: map[string]int{}}
	model.DB = model.DB.Session(&gorm.Session{PrepareStmt: true, Logger: sqlTrace})
	ch, err := model.GetChannelById(20, true)
	require.NoError(t, err)
	settings := ch.GetSetting()
	settings.Protocol.AssetLibrary = &relaydto.AssetLibrarySettings{Backend: configurable.ArkCompatibleAssetBackend, AuthMode: "channel_key", ArkCompatible: &relaydto.ArkCompatibleAssetSettings{FixedQuery: map[string]string{"ns": cfg.Namespace}, Pagination: "page"}}
	ch.Key = cfg.Key
	ch.SetSetting(settings)
	require.NoError(t, model.DB.Save(ch).Error)
	pool, err := model.DB.DB()
	require.NoError(t, err)
	pool.SetMaxOpenConns(30)
	pool.SetMaxIdleConns(30)
	model.LOG_DB = testdb.AssetOpener(t, "unused")()
	require.NoError(t, model.LOG_DB.AutoMigrate(&model.Log{}))
	logPool, err := model.LOG_DB.DB()
	require.NoError(t, err)
	logPool.SetMaxOpenConns(5)
	logPool.SetMaxIdleConns(5)
	var version, schema string
	require.NoError(t, model.DB.Raw("SELECT VERSION()").Scan(&version).Error)
	require.True(t, strings.HasPrefix(version, "5.7."))
	require.NoError(t, model.DB.Raw("SELECT DATABASE()").Scan(&schema).Error)
	dbCfg, err := mysql.ParseDSN(cfg.MySQLDSN)
	require.NoError(t, err)
	dbCfg.DBName, dbCfg.Timeout = schema, 2*time.Second
	monitor, err := sql.Open("mysql", dbCfg.FormatDSN())
	require.NoError(t, err)
	defer monitor.Close()
	monitor.SetMaxOpenConns(1)
	r.GET("/api/ready", GetReadiness)
	r.GET("/api/status", GetStatus)
	gateway := httptest.NewServer(r)
	defer gateway.Close()
	transport := &http.Transport{MaxIdleConns: 256, MaxIdleConnsPerHost: 256, MaxConnsPerHost: 256}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 65 * time.Second}
	stats := map[string]*assetLoadRequests{}
	for _, kind := range []string{"CreateAssetGroup", "GetAssetGroup", "UpdateAssetGroup", "ListAssetGroups", "smoke_create", "CreateAsset", "GetAsset", "UpdateAsset", "ListAssets", "DeleteAsset", "DeleteAssetGroup", "ready", "status", "direct_list", "direct_delete", "direct_verify"} {
		stats[kind] = &assetLoadRequests{}
	}
	prefix := fmt.Sprintf("apimeter-live200-%d", time.Now().Unix())
	journal, err := os.OpenFile(filepath.Join(evidence, "created-resources.jsonl"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	require.NoError(t, err, "use a fresh evidence directory for every run")
	defer journal.Close()
	var resourceMu sync.Mutex
	assets, groups := map[string]bool{}, map[string]bool{}
	remember := func(kind, id string) {
		if id == "" {
			return
		}
		resourceMu.Lock()
		defer resourceMu.Unlock()
		target := assets
		if kind == "group" {
			target = groups
		}
		if target[id] {
			return
		}
		target[id] = true
		entry, e := common.Marshal(map[string]string{"kind": kind, "id": id, "prefix": prefix})
		if e == nil {
			_, e = journal.Write(append(entry, '\n'))
		}
		if e == nil {
			e = journal.Sync()
		}
		if e != nil {
			t.Errorf("write resource journal: %v", e)
		}
	}
	upstreamStats := map[string]*assetLoadRequests{}
	var upstreamMu sync.Mutex
	upstream := service.GetHttpClient()
	originalTransport, originalTimeout := upstream.Transport, upstream.Timeout
	base := originalTransport
	if base == nil {
		base = http.DefaultTransport
	}
	upstream.Timeout = 45 * time.Second
	upstream.Transport = arkLiveTransport(func(req *http.Request) (*http.Response, error) {
		action := req.URL.Query().Get("Action")
		upstreamMu.Lock()
		s := upstreamStats[action]
		if s == nil {
			s = &assetLoadRequests{}
			upstreamStats[action] = s
		}
		upstreamMu.Unlock()
		assetLoadPeak(&s.peak, s.inFlight.Add(1))
		defer s.inFlight.Add(-1)
		started := time.Now()
		resp, e := base.RoundTrip(req)
		code := 0
		if resp != nil {
			code = resp.StatusCode
			if action == "CreateAsset" || action == "CreateAssetGroup" {
				body, readErr := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
				_ = resp.Body.Close()
				resp.Body = io.NopCloser(bytes.NewReader(body))
				if readErr != nil {
					e = readErr
				}
				if e == nil && code == 200 && assetResponseSuccessful(body) {
					kind := "asset"
					if action == "CreateAssetGroup" {
						kind = "group"
					}
					remember(kind, arkLiveResult(body).Get("Id").String())
				}
			}
		}
		s.record(code, time.Since(started), e, "")
		return resp, e
	})
	defer func() { upstream.Transport, upstream.Timeout = originalTransport, originalTimeout }()
	call := func(kind, label, action string, payload map[string]any, direct, rest bool) ([]byte, int, error) {
		s := stats[kind]
		assetLoadPeak(&s.peak, s.inFlight.Add(1))
		defer s.inFlight.Add(-1)
		started := time.Now()
		body := map[string]any{}
		for k, v := range payload {
			body[k] = v
		}
		method, target, key := "POST", gateway.URL+"/?Action="+action+"&Version=2024-01-01", "seedancetest1"
		if direct {
			target, key = cfg.URL+"/api?"+url.Values{"Action": {action}, "Version": {"2024-01-01"}, "ns": {cfg.Namespace}}.Encode(), cfg.Key
		} else if kind == "ready" || kind == "status" {
			method, target = "GET", gateway.URL+"/api/"+kind
		} else {
			body["model"] = tgxRegressionModel
			if rest {
				resource := "assets"
				if strings.Contains(action, "Group") {
					resource = "groups"
				}
				target = gateway.URL + "/v1/private-avatar/" + resource
				switch {
				case strings.HasPrefix(action, "List"):
					target += "/list"
				case strings.HasPrefix(action, "Get"):
					method = "GET"
					target += "/" + fmt.Sprint(body["Id"])
				case strings.HasPrefix(action, "Update"):
					method = "PATCH"
					target += "/" + fmt.Sprint(body["Id"])
				case strings.HasPrefix(action, "Delete"):
					method = "DELETE"
					target += "/" + fmt.Sprint(body["Id"])
				}
			}
		}
		encoded, e := common.Marshal(body)
		timeout := 65 * time.Second
		if kind == "ready" || kind == "status" {
			timeout = 5 * time.Second
		}
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		var data []byte
		code := 0
		if e == nil {
			var req *http.Request
			req, e = http.NewRequestWithContext(ctx, method, target, bytes.NewReader(encoded))
			if e == nil {
				req.Header.Set("Authorization", "Bearer "+key)
				req.Header.Set("Content-Type", "application/json")
				var resp *http.Response
				resp, e = client.Do(req)
				if e == nil {
					code = resp.StatusCode
					data, e = io.ReadAll(io.LimitReader(resp.Body, 4<<20))
					_ = resp.Body.Close()
				}
			}
		}
		if e == nil && (code != 200 || !assetResponseSuccessful(data)) {
			e = fmt.Errorf("%s HTTP %d: %.400s", action, code, data)
		}
		if kind == "direct_verify" && (code == 404 || (code >= 400 && (strings.Contains(strings.ToLower(string(data)), "notfound") || strings.Contains(strings.ToLower(string(data)), "not found")))) {
			e = nil
		}
		if e == nil && kind == "ready" && (!gjson.GetBytes(data, "success").Bool() || gjson.GetBytes(data, "data.checks.database").String() != "ok" || gjson.GetBytes(data, "data.checks.log_database").String() != "ok") {
			e = fmt.Errorf("database readiness failed")
		}
		if label != "" {
			if writeErr := os.WriteFile(filepath.Join(evidence, label+".json"), data, 0600); writeErr != nil {
				t.Errorf("write evidence: %v", writeErr)
			}
		}
		if e != nil {
			e = fmt.Errorf("%s", strings.ReplaceAll(e.Error(), cfg.Key, "[redacted]"))
		}
		s.record(code, time.Since(started), e, arkLiveResult(data).Get("Status").String())
		return data, code, e
	}
	parallel := func(n, limit int, work func(int)) {
		var wg sync.WaitGroup
		gate, slots := make(chan struct{}), make(chan struct{}, limit)
		for i := range n {
			wg.Add(1)
			go func() { defer wg.Done(); <-gate; slots <- struct{}{}; defer func() { <-slots }(); work(i) }()
		}
		close(gate)
		wg.Wait()
	}
	stop := make(chan struct{})
	var background sync.WaitGroup
	var poolPeak, lockPeak, monitorErrors atomic.Int64
	var poolSamples, poolFullSamples, poolFullLongest atomic.Int64
	var lockedTransactionsPeak, lockAgeUS, waitAgeUS atomic.Int64
	samples, err := os.OpenFile(filepath.Join(evidence, "db-samples.jsonl"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	require.NoError(t, err)
	defer samples.Close()
	before := pool.Stats()
	started := time.Now()
	for _, kind := range []string{"ready", "status"} {
		background.Add(1)
		go func() {
			defer background.Done()
			interval := 500 * time.Millisecond
			if kind == "status" {
				interval = 3 * time.Second
			}
			ticker := time.NewTicker(interval)
			defer ticker.Stop()
			for {
				_, _, _ = call(kind, "", "", nil, false, false)
				select {
				case <-stop:
					return
				case <-ticker.C:
				}
			}
		}()
	}
	background.Add(1)
	go func() {
		defer background.Done()
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		var fullStreak int64
		for {
			inUse := int64(pool.Stats().InUse)
			assetLoadPeak(&poolPeak, inUse)
			poolSamples.Add(1)
			if inUse == 30 {
				poolFullSamples.Add(1)
				fullStreak++
				assetLoadPeak(&poolFullLongest, fullStreak)
			} else {
				fullStreak = 0
			}
			var waiters, locked, oldestLock, oldestWait int64
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			rows, e := monitor.QueryContext(ctx, "SELECT t.trx_state,t.trx_rows_locked,TIMESTAMPDIFF(MICROSECOND,t.trx_started,NOW(6)),IFNULL(TIMESTAMPDIFF(MICROSECOND,t.trx_wait_started,NOW(6)),0) FROM information_schema.innodb_trx t JOIN information_schema.processlist p ON p.ID=t.trx_mysql_thread_id WHERE p.DB=?", schema)
			if e == nil {
				for rows.Next() {
					var state string
					var rowLocks, age, wait int64
					if e = rows.Scan(&state, &rowLocks, &age, &wait); e != nil {
						break
					}
					if rowLocks > 0 {
						locked++
						oldestLock = max(oldestLock, age)
					}
					if state == "LOCK WAIT" {
						waiters++
						oldestWait = max(oldestWait, wait)
					}
				}
				if e == nil {
					e = rows.Err()
				}
				_ = rows.Close()
			}
			cancel()
			if e != nil {
				monitorErrors.Add(1)
			} else {
				assetLoadPeak(&lockPeak, waiters)
				assetLoadPeak(&lockedTransactionsPeak, locked)
				assetLoadPeak(&lockAgeUS, oldestLock)
				assetLoadPeak(&waitAgeUS, oldestWait)
			}
			sample, marshalErr := common.Marshal(map[string]any{"elapsed_ms": time.Since(started).Milliseconds(), "pool_in_use": inUse, "lock_waiters": waiters, "locked_transactions": locked, "locked_transaction_age_upper_bound_us": oldestLock, "lock_wait_age_upper_bound_us": oldestWait, "monitor_error": e != nil})
			if marshalErr == nil {
				_, marshalErr = samples.Write(append(sample, '\n'))
			}
			if marshalErr != nil {
				monitorErrors.Add(1)
			}
			select {
			case <-stop:
				return
			case <-ticker.C:
			}
		}
	}()
	groupID := ""
	states, ids := make([]string, 200), make([]string, 200)
	rounds := 0
	var createSeconds float64
	var cleanupFailures []string
	defer func() {
		// Reconcile timed-out creations by this run's unique name inside its
		// known group. Never delete an account-wide or pre-existing resource.
		if groupID != "" {
			for page := 1; page <= 10; page++ {
				body, _, e := call("direct_list", fmt.Sprintf("reconcile-%d", page), "ListAssets", map[string]any{"Filter": map[string]any{"GroupType": "AIGC", "GroupIds": []string{groupID}}, "PageNumber": page, "PageSize": 100}, true, false)
				if e != nil {
					t.Errorf("cleanup reconciliation: %v", e)
					break
				}
				result := arkLiveResult(body)
				items := result.Get("Items").Array()
				for _, item := range items {
					if strings.HasPrefix(item.Get("Name").String(), prefix) && item.Get("GroupId").String() == groupID {
						remember("asset", item.Get("Id").String())
					}
				}
				if len(items) < 100 || int64(page*100) >= result.Get("TotalCount").Int() {
					break
				}
			}
		}
		var assetIDs, groupIDs []string
		resourceMu.Lock()
		for id := range assets {
			assetIDs = append(assetIDs, id)
		}
		for id := range groups {
			groupIDs = append(groupIDs, id)
		}
		resourceMu.Unlock()
		var deleted, verified atomic.Int64
		var failureMu sync.Mutex
		parallel(len(assetIDs), 5, func(i int) {
			id := assetIDs[i]
			_, _, e := call("DeleteAsset", "delete-"+id, "DeleteAsset", map[string]any{"Id": id}, false, i%2 == 0)
			for attempt := 0; e != nil && attempt < 3; attempt++ {
				time.Sleep(time.Duration(attempt+1) * time.Second)
				_, code, retryErr := call("direct_delete", fmt.Sprintf("delete-retry-%s-%d", id, attempt), "DeleteAsset", map[string]any{"Id": id}, true, false)
				e = retryErr
				if code == 404 {
					e = nil
				}
			}
			if e == nil {
				deleted.Add(1)
			}
			body, code, verifyErr := call("direct_verify", "verify-"+id, "GetAsset", map[string]any{"Id": id}, true, false)
			missing := code == 404 || (code >= 400 && (strings.Contains(strings.ToLower(string(body)), "notfound") || strings.Contains(strings.ToLower(string(body)), "not found")))
			if missing {
				verified.Add(1)
			} else {
				failureMu.Lock()
				cleanupFailures = append(cleanupFailures, id)
				failureMu.Unlock()
				t.Errorf("asset cleanup unverified %s: delete=%v get=%v HTTP=%d", id, e, verifyErr, code)
			}
		})
		groupDeleted := true
		for _, id := range groupIDs {
			_, _, e := call("DeleteAssetGroup", "group-delete", "DeleteAssetGroup", map[string]any{"Id": id}, false, false)
			for attempt := 0; e != nil && attempt < 3; attempt++ {
				time.Sleep(time.Second)
				_, _, e = call("direct_delete", "group-delete-retry", "DeleteAssetGroup", map[string]any{"Id": id}, true, false)
			}
			body, code, _ := call("direct_verify", "group-verify", "GetAssetGroup", map[string]any{"Id": id}, true, false)
			missing := code == 404 || (code >= 400 && (strings.Contains(strings.ToLower(string(body)), "notfound") || strings.Contains(strings.ToLower(string(body)), "not found")))
			if e != nil || !missing {
				groupDeleted = false
				cleanupFailures = append(cleanupFailures, id)
				t.Errorf("group cleanup unverified: %s HTTP=%d", id, code)
			}
		}
		close(stop)
		background.Wait()
		for _, s := range stats {
			s.finish()
		}
		for _, s := range upstreamStats {
			s.finish()
		}
		after := pool.Stats()
		for _, kind := range []string{"ready", "status"} {
			if len(stats[kind].Errors) > 0 {
				t.Errorf("%s probe errors: %v", kind, stats[kind].Errors)
			}
		}
		report := map[string]any{"upstream": cfg.URL, "namespace": cfg.Namespace, "run_prefix": prefix, "mysql_version": version, "clients": 200, "query_only": queryOnly, "instances": 1, "redis": "disabled", "rounds": rounds, "create_wave_seconds": createSeconds, "duration_seconds": time.Since(started).Seconds(), "requests": stats, "upstream_requests": upstreamStats, "created_asset_ids": assetIDs, "group_ids": groupIDs, "final_asset_states": states, "deleted_assets": deleted.Load(), "verified_absent_assets": verified.Load(), "group_deleted": groupDeleted, "cleanup_failures": cleanupFailures, "main_pool_limit": 30, "log_pool_limit": 5, "main_pool_peak_in_use": poolPeak.Load(), "main_pool_end_in_use": after.InUse, "main_pool_wait_count": after.WaitCount - before.WaitCount, "main_pool_wait_ms": float64(after.WaitDuration-before.WaitDuration) / float64(time.Millisecond), "sampled_server_lock_waiters_peak": lockPeak.Load(), "monitor_errors": monitorErrors.Load(), "passed": !t.Failed()}
		report["pool_sample_interval_ms"] = 100
		report["pool_samples"] = poolSamples.Load()
		report["pool_full_samples"] = poolFullSamples.Load()
		report["pool_full_estimated_ms"] = poolFullSamples.Load() * 100
		report["pool_full_longest_sample_streak_ms"] = poolFullLongest.Load() * 100
		report["sampled_locked_transactions_peak"] = lockedTransactionsPeak.Load()
		// MySQL 5.7 trx_started/trx_wait_started have second resolution, so
		// these ages are conservative upper bounds, not precise lock timings.
		report["sampled_locked_transaction_age_upper_bound_ms"] = float64(lockAgeUS.Load()) / 1000
		report["sampled_lock_wait_age_upper_bound_ms"] = float64(waitAgeUS.Load()) / 1000
		sqlTrace.mu.Lock()
		report["sql_trace"] = map[string]any{"count": sqlTrace.count, "slow_over_100ms": sqlTrace.slow, "max_ms": sqlTrace.maxMS, "lock_queries": sqlTrace.lockQueries, "max_lock_query_ms": sqlTrace.maxLockQueryMS, "errors": sqlTrace.errors}
		sqlTrace.mu.Unlock()
		data, e := common.Marshal(report)
		if e == nil {
			e = os.WriteFile(filepath.Join(evidence, "report.json"), append(data, '\n'), 0600)
		}
		if e != nil {
			t.Errorf("write report: %v", e)
		}
		t.Logf("cleanup created=%d deleted=%d absent=%d group_deleted=%v lock_peak=%d pool_end=%d report=%s", len(assetIDs), deleted.Load(), verified.Load(), groupDeleted, lockPeak.Load(), after.InUse, filepath.Join(evidence, "report.json"))
	}()
	data, _, err := call("CreateAssetGroup", "group-create", "CreateAssetGroup", map[string]any{"Name": prefix, "Description": "Temporary API and 200 concurrency test; delete after test", "GroupType": "AIGC"}, false, false)
	groupID = arkLiveResult(data).Get("Id").String()
	require.NoError(t, err)
	require.NotEmpty(t, groupID)
	for _, action := range []string{"GetAssetGroup", "UpdateAssetGroup", "ListAssetGroups"} {
		payload := map[string]any{"Id": groupID}
		if action == "UpdateAssetGroup" {
			payload["Name"] = prefix + "-updated"
		}
		if action == "ListAssetGroups" {
			payload = map[string]any{"Filter": map[string]any{"GroupType": "AIGC"}, "PageNumber": 1, "PageSize": 10}
		}
		_, _, e := call(action, action, action, payload, false, true)
		if e != nil {
			t.Errorf("%s: %v", action, e)
		}
	}
	makeAsset := func(i int) map[string]any {
		return map[string]any{"GroupId": groupID, "URL": cfg.AssetURL, "AssetType": "Image", "Name": fmt.Sprintf("%s-%03d", prefix, i)}
	}
	data, _, err = call("smoke_create", "smoke-create", "CreateAsset", makeAsset(999), false, true)
	require.NoError(t, err)
	smokeID := arkLiveResult(data).Get("Id").String()
	require.NotEmpty(t, smokeID)
	_, _, err = call("UpdateAsset", "asset-update", "UpdateAsset", map[string]any{"Id": smokeID, "Name": prefix + "-smoke-updated"}, false, false)
	if err != nil {
		t.Errorf("UpdateAsset: %v", err)
	}
	data, _, err = call("GetAsset", "smoke-get", "GetAsset", map[string]any{"Id": smokeID}, false, true)
	require.NoError(t, err)
	if arkLiveResult(data).Get("Name").String() != prefix+"-smoke-updated" {
		t.Errorf("asset update did not persist")
	}
	wave := time.Now()
	if queryOnly {
		for i := range ids {
			ids[i] = smokeID
		}
		t.Log("smoke lifecycle completed; launching 200 simultaneous reads of one asset")
	} else {
		t.Log("smoke lifecycle completed; launching 200 simultaneous creates")
		parallel(200, 200, func(i int) {
			body, _, e := call("CreateAsset", fmt.Sprintf("create-%03d", i), "CreateAsset", makeAsset(i), false, i%2 == 0)
			ids[i] = arkLiveResult(body).Get("Id").String()
			if e != nil || ids[i] == "" {
				t.Errorf("create %d: %v", i, e)
			}
		})
		createSeconds = time.Since(wave).Seconds()
		t.Logf("create wave finished %.3fs HTTP=%v", createSeconds, stats["CreateAsset"].HTTP)
	}
	for round := 0; round < 10; round++ {
		wave = time.Now()
		parallel(200, 200, func(i int) {
			if ids[i] == "" {
				return
			}
			body, _, e := call("GetAsset", fmt.Sprintf("get-%02d-%03d", round, i), "GetAsset", map[string]any{"Id": ids[i]}, false, i%2 == 0)
			if e != nil {
				t.Errorf("get round %d asset %d: %v", round, i, e)
				return
			}
			if arkLiveResult(body).Get("Id").String() != ids[i] {
				t.Errorf("incorrect asset ID returned")
			}
			states[i] = arkLiveResult(body).Get("Status").String()
		})
		rounds++
		t.Logf("query round=%d elapsed=%.3fs pool_in_use=%d lock_peak=%d", rounds, time.Since(wave).Seconds(), pool.Stats().InUse, lockPeak.Load())
		if delay := 3*time.Second - time.Since(wave); delay > 0 {
			time.Sleep(delay)
		}
	}
	if queryOnly {
		return
	}
	// Concurrent list calls exercise pagination and ownership filtering too.
	resourceMu.Lock()
	expectedCount := int64(len(assets))
	resourceMu.Unlock()
	parallel(200, 200, func(i int) {
		body, _, e := call("ListAssets", fmt.Sprintf("list-%03d", i), "ListAssets", map[string]any{"Filter": map[string]any{"GroupType": "AIGC", "GroupIds": []string{groupID}}, "PageNumber": 1, "PageSize": 100}, false, i%2 == 0)
		if e != nil {
			t.Errorf("list %d: %v", i, e)
			return
		}
		if arkLiveResult(body).Get("TotalCount").Int() != expectedCount {
			t.Errorf("list total mismatch: %d", arkLiveResult(body).Get("TotalCount").Int())
		}
	})
	for _, action := range []string{"GetAssetGroup", "ListAssetGroups"} {
		parallel(200, 200, func(i int) {
			payload := map[string]any{"Id": groupID}
			if action == "ListAssetGroups" {
				payload = map[string]any{"Filter": map[string]any{"GroupType": "AIGC"}, "PageNumber": 1, "PageSize": 10}
			}
			_, _, e := call(action, "", action, payload, false, i%2 == 0)
			if e != nil {
				t.Errorf("%s %d: %v", action, i, e)
			}
		})
	}
	if stats["CreateAsset"].HTTP[200] != 200 {
		t.Errorf("not all 200 creations succeeded")
	}
	if monitorErrors.Load() != 0 {
		t.Errorf("lock monitoring errors: %d", monitorErrors.Load())
	}
}

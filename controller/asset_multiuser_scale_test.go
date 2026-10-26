package controller

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
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

type multiUserAsset struct {
	ID          string `json:"Id"`
	GroupID     string `json:"GroupId,omitempty"`
	Name        string `json:"Name"`
	ProjectName string `json:"ProjectName"`
	Status      string `json:"Status"`
	GroupType   string `json:"GroupType,omitempty"`
	Owner       int    `json:"-"`
}

type multiUserAssetMock struct {
	mu                 sync.RWMutex
	assets, groups     map[string]multiUserAsset
	assetIDs, groupIDs []string
	calls              map[string]int
	active, peak       atomic.Int64
}

func (m *multiUserAssetMock) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	active := m.active.Add(1)
	assetLoadPeak(&m.peak, active)
	defer m.active.Add(-1)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(400)
		return
	}
	timer := time.NewTimer(time.Millisecond)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-r.Context().Done():
		return
	}
	action := r.URL.Query().Get("Action")
	m.mu.Lock()
	m.calls[action]++
	m.mu.Unlock()
	group := strings.Contains(action, "AssetGroup")
	id := gjson.GetBytes(body, "Id").String()
	var result any
	code := 200
	m.mu.Lock()
	items, order := m.assets, &m.assetIDs
	if group {
		items, order = m.groups, &m.groupIDs
	}
	switch {
	case strings.HasPrefix(action, "Create"):
		// Request names encode only fixture IDs, never authentication data.
		id = gjson.GetBytes(body, "Name").String()
		var owner int
		_, _ = fmt.Sscanf(id, "load-u%d-", &owner)
		if id == "" || owner < 1 {
			code = 400
			break
		}
		item := multiUserAsset{ID: id, Name: id, Owner: owner, Status: "Active", ProjectName: "nmyk", GroupID: gjson.GetBytes(body, "GroupId").String()}
		if group {
			item.GroupType = "AIGC"
		}
		if _, exists := items[id]; !exists {
			*order = append(*order, id)
		}
		items[id] = item
		result = item
	case strings.HasPrefix(action, "List"):
		page, size := int(gjson.GetBytes(body, "PageNumber").Int()), int(gjson.GetBytes(body, "PageSize").Int())
		page, size = max(1, page), max(1, size)
		rows := make([]multiUserAsset, 0, size)
		// Lists are measured before deletion, so stable fixture order is enough.
		for i := (page - 1) * size; i < min(page*size, len(*order)); i++ {
			if item, ok := items[(*order)[i]]; ok {
				rows = append(rows, item)
			}
		}
		result = map[string]any{"Items": rows, "TotalCount": len(items), "PageNumber": page, "PageSize": size}
	case strings.HasPrefix(action, "Get"), strings.HasPrefix(action, "Update"), strings.HasPrefix(action, "Delete"):
		item, ok := items[id]
		if !ok {
			code = 404
			break
		}
		if strings.HasPrefix(action, "Update") {
			item.Name = gjson.GetBytes(body, "Name").String()
			items[id] = item
		}
		if strings.HasPrefix(action, "Delete") {
			delete(items, id)
		}
		result = item
	default:
		code = 400
	}
	m.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if code != 200 {
		_, _ = w.Write([]byte(`{"ResponseMetadata":{"Error":{"Code":"NotFound","Message":"mock resource unavailable"}}}`))
		return
	}
	data, err := common.Marshal(map[string]any{"Result": result})
	if err == nil {
		_, _ = w.Write(data)
	}
}

type multiUserSQLStats struct {
	Count, Slow, LockQueries, MetadataLikeQueries int64
	MaxMS, MaxLockQueryMS                         float64
	Errors                                        map[string]int
	Kinds                                         map[string]*multiUserQueryStats
}

type multiUserQueryStats struct {
	Count, Errors, Rows int64
	TotalMS, MaxMS      float64
	Example             string
}

type multiUserSQLTrace struct {
	logger.Interface
	mu     sync.Mutex
	phase  atomic.Value
	phases map[string]*multiUserSQLStats
}

func (l *multiUserSQLTrace) Trace(_ context.Context, begin time.Time, query func() (string, int64), err error) {
	elapsed := float64(time.Since(begin)) / float64(time.Millisecond)
	text, returnedRows := query()
	l.mu.Lock()
	defer l.mu.Unlock()
	name := l.phase.Load().(string)
	s := l.phases[name]
	if s == nil {
		s = &multiUserSQLStats{Errors: map[string]int{}, Kinds: map[string]*multiUserQueryStats{}}
		l.phases[name] = s
	}
	s.Count++
	s.MaxMS = max(s.MaxMS, elapsed)
	if elapsed >= 100 {
		s.Slow++
	}
	if strings.Contains(text, "FOR UPDATE") || strings.Contains(text, "LOCK IN SHARE MODE") {
		s.LockQueries++
		s.MaxLockQueryMS = max(s.MaxLockQueryMS, elapsed)
	}
	if strings.Contains(text, "metadata LIKE") {
		s.MetadataLikeQueries++
	}
	if name == "ListAssets" {
		normalized := strings.ReplaceAll(text, "`", "")
		kind := "other"
		switch {
		case strings.Contains(normalized, "asset_candidates"):
			kind = "ownership_recheck"
		case strings.Contains(normalized, "GROUP BY channel_id") && !strings.Contains(normalized, "state_value =") && !strings.Contains(normalized, "asset_candidates"):
			kind = "account_scope_discovery"
		case strings.Contains(normalized, "state_key >") && !strings.Contains(normalized, "state_value ="):
			kind = "account_binding_page"
		case strings.Contains(normalized, "GROUP BY channel_id") && strings.Contains(normalized, "state_value ="):
			kind = "list_owner_scope_discovery"
		case strings.Contains(normalized, "state_key >") && strings.Contains(normalized, "state_value ="):
			kind = "list_owner_page"
		case strings.Contains(normalized, "identity_rows") || strings.Contains(normalized, "asset-access-lock-v1"):
			kind = "identity_lookup"
		case strings.Contains(normalized, "configurable_resource_states"):
			kind = "other_asset_lookup"
		}
		q := s.Kinds[kind]
		if q == nil {
			q = &multiUserQueryStats{Example: text[:min(len(text), 1500)]}
			s.Kinds[kind] = q
		}
		q.Count++
		q.TotalMS += elapsed
		q.MaxMS = max(q.MaxMS, elapsed)
		if err != nil {
			q.Errors++
		}
		q.Rows += max(0, returnedRows)
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		code := "other"
		var mysqlErr *mysql.MySQLError
		if errors.As(err, &mysqlErr) {
			code = fmt.Sprint(mysqlErr.Number)
		}
		if errors.Is(err, context.Canceled) {
			code = "canceled"
		}
		if errors.Is(err, context.DeadlineExceeded) {
			code = "deadline_exceeded"
		}
		s.Errors[code]++
	}
}

func multiUserHistoryID(user, index int) string {
	return fmt.Sprintf("history-u%03d-%06d", user, index)
}
func multiUserGroupID(user int) string { return fmt.Sprintf("history-group-u%03d", user) }
func multiUserNewID(user, request int, kind string) string {
	return fmt.Sprintf("load-u%03d-%s-%04d", user, kind, request)
}

func seedMultiUserAssetHistory(t *testing.T, m *multiUserAssetMock, users, inventory int) (string, []int) {
	t.Helper()
	ch, err := model.GetChannelById(20, true)
	require.NoError(t, err)
	profile, ok := configurable.AssetProfile(ch.GetSetting().Protocol)
	require.True(t, ok)
	scope, err := assetAccountScope(ch, profile)
	require.NoError(t, err)
	counts := make([]int, users)
	weightTotal := users * (users + 1) / 2
	total := 0
	for u := range users {
		// A reproducible, uneven allocation, summing to exactly inventory.
		counts[u] = inventory * (u + 1) / weightTotal
		total += counts[u]
	}
	for i := 0; total < inventory; i, total = i+1, total+1 {
		counts[i%users]++
	}
	var newUsers []model.User
	var tokens []model.Token
	var groups []model.AssetBinding
	for user := 1; user <= users; user++ {
		if user > 2 {
			newUsers = append(newUsers, model.User{Id: user, Username: fmt.Sprintf("asset-load-user-%d", user), AffCode: fmt.Sprintf("alu%d", user), Group: "default", Quota: 10000000, Status: common.UserStatusEnabled, Role: common.RoleCommonUser})
			tokens = append(tokens, model.Token{Id: user, UserId: user, Name: "asset-load", Key: fmt.Sprintf("seedancetest%d", user), Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true, Group: "default"})
		}
		id := multiUserGroupID(user)
		groups = append(groups, model.AssetBinding{ChannelID: 20, UserID: user, Backend: profile.ID, Scope: scope, Kind: "group", ID: id, CanonicalID: id, Project: "nmyk"})
		m.groups[id] = multiUserAsset{ID: id, Name: id, Owner: user, Status: "Active", ProjectName: "nmyk", GroupType: "AIGC"}
		m.groupIDs = append(m.groupIDs, id)
	}
	require.NoError(t, model.DB.Create(&newUsers).Error)
	require.NoError(t, model.DB.Create(&tokens).Error)
	require.NoError(t, model.SaveAssetBindings(groups))
	var previous int
	require.NoError(t, model.DB.Model(&model.ConfigurableResourceState{}).Select("MAX(id)").Scan(&previous).Error)
	binding := model.AssetBinding{ChannelID: 20, UserID: 1, Backend: profile.ID, Scope: scope, Kind: "asset", ID: multiUserHistoryID(1, 0), CanonicalID: multiUserHistoryID(1, 0), Project: "nmyk", GroupID: multiUserGroupID(1)}
	require.NoError(t, model.SaveAssetBindings([]model.AssetBinding{binding}))
	var prototype []model.ConfigurableResourceState
	require.NoError(t, model.DB.Where("id > ?", previous).Order("id").Find(&prototype).Error)
	require.Len(t, prototype, 3)
	hash := func(s string) string { return fmt.Sprintf("%x", sha256.Sum256([]byte(s))) }
	db := model.DB.Session(&gorm.Session{SkipDefaultTransaction: true, PrepareStmt: false, Logger: logger.Default.LogMode(logger.Silent)})
	rows := make([]model.ConfigurableResourceState, 0, 1500)
	// Interleave owners in the supplier's order to avoid a first-page advantage.
	for index, added := 0, 0; added < inventory; index++ {
		for u, count := range counts {
			if index >= count {
				continue
			}
			user, id := u+1, multiUserHistoryID(u+1, index)
			m.assets[id] = multiUserAsset{ID: id, Name: id, Owner: user, Status: "Active", ProjectName: "nmyk", GroupID: multiUserGroupID(user)}
			m.assetIDs = append(m.assetIDs, id)
			added++
			if user == 1 && index == 0 {
				continue
			}
			b := binding
			b.UserID, b.ID, b.CanonicalID, b.GroupID = user, id, id, multiUserGroupID(user)
			metadata, err := common.Marshal(b)
			require.NoError(t, err)
			parents, err := common.Marshal(map[string]any{"parents": []string{b.GroupID}})
			require.NoError(t, err)
			for _, original := range prototype {
				row := original
				row.Id = 0
				switch {
				case row.ProfileID == "asset-access-v1":
					row.StateKey, row.Metadata, row.StateValue = hash(id), string(metadata), fmt.Sprint(user)
				case strings.HasPrefix(row.StateKey, "identity:"):
					row.StateKey = "identity:" + hash(fmt.Sprintf("%d:%s:asset:%s", b.ChannelID, hash(b.Scope), hash(id)))
					row.Metadata = string(parents)
				default:
					row.StateKey = "asset:" + hash(id)
				}
				rows = append(rows, row)
			}
			if len(rows) >= 1500 {
				require.NoError(t, db.Create(&rows).Error)
				rows = rows[:0]
			}
		}
	}
	if len(rows) > 0 {
		require.NoError(t, db.Create(&rows).Error)
	}
	require.NoError(t, db.Exec("ANALYZE TABLE configurable_resource_states").Error)
	for u, count := range counts {
		var actual int64
		require.NoError(t, db.Model(&model.ConfigurableResourceState{}).Where("profile_id = ? AND resource_id = ? AND state_value = ?", "asset-access-v1", "asset", fmt.Sprint(u+1)).Count(&actual).Error)
		require.EqualValues(t, count, actual)
	}
	return scope, counts
}

type multiUserLoadPhase struct {
	Requests   *assetLoadRequests         `json:"requests"`
	Users      map[int]*assetLoadRequests `json:"users"`
	Seconds    float64                    `json:"seconds"`
	DrainMS    float64                    `json:"drain_ms"`
	PoolWaits  int64                      `json:"pool_waits"`
	PoolWaitMS float64                    `json:"pool_wait_ms"`
}

// Explicit opt-in only: 100 real fixture users, 100,000 existing assets in one
// upstream account, 1000 simultaneous requests split evenly across those users.
func TestAssetMultiUser100K1000MySQL57(t *testing.T) {
	if os.Getenv("ASSET_LOAD_MULTIUSER_100K1000") != "1" || os.Getenv("ASSET_TEST_MYSQL_DSN") == "" {
		t.Skip("requires explicit multi-user load opt-in and isolated MySQL")
	}
	users, clients, inventory := 100, 1000, 100000
	if os.Getenv("ASSET_LOAD_MULTIUSER_SMALL") == "1" {
		users, clients, inventory = 10, 20, 1000
	}
	mock := &multiUserAssetMock{assets: map[string]multiUserAsset{}, groups: map[string]multiUserAsset{}, calls: map[string]int{}}
	upstream := httptest.NewServer(mock)
	defer upstream.Close()
	router := assetSecurityRouter(t, upstream.URL, configurable.ArkCompatibleAssetBackend, "")
	var version, schema string
	require.NoError(t, model.DB.Raw("SELECT VERSION()").Scan(&version).Error)
	require.True(t, strings.HasPrefix(version, "5.7."))
	require.NoError(t, model.DB.Raw("SELECT DATABASE()").Scan(&schema).Error)
	ch, err := model.GetChannelById(20, true)
	require.NoError(t, err)
	settings := ch.GetSetting()
	settings.Protocol.ProjectName = "nmyk"
	ch.SetSetting(settings)
	require.NoError(t, model.DB.Save(ch).Error)
	model.DB = model.DB.Session(&gorm.Session{PrepareStmt: true})
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
	seedStart := time.Now()
	scope, counts := seedMultiUserAssetHistory(t, mock, users, inventory)
	seedSeconds := time.Since(seedStart).Seconds()
	t.Logf("seed complete: %d users, %d assets, min=%d max=%d per user, %.3fs", users, inventory, counts[0], counts[len(counts)-1], seedSeconds)
	trace := &multiUserSQLTrace{Interface: logger.Default.LogMode(logger.Silent), phases: map[string]*multiUserSQLStats{}}
	trace.phase.Store("setup")
	model.DB = model.DB.Session(&gorm.Session{Logger: trace})
	cfg, err := mysql.ParseDSN(os.Getenv("ASSET_TEST_MYSQL_DSN"))
	require.NoError(t, err)
	cfg.DBName, cfg.Timeout = schema, 2*time.Second
	monitor, err := sql.Open("mysql", cfg.FormatDSN())
	require.NoError(t, err)
	defer monitor.Close()
	monitor.SetMaxOpenConns(1)
	scale := startAssetScaleMonitor(t, pool, monitor, schema)
	defer scale.close()
	router.GET("/api/ready", GetReadiness)
	var handlers atomic.Int64
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/ready" {
			handlers.Add(1)
			defer handlers.Add(-1)
		}
		router.ServeHTTP(w, r)
	}))
	defer gateway.Close()
	transport := &http.Transport{MaxIdleConns: clients + 16, MaxIdleConnsPerHost: clients + 16, MaxConnsPerHost: clients + 16}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 20 * time.Second}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	ready := &assetLoadRequests{}
	stop, stopped := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(stopped)
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		for {
			start := time.Now()
			request, _ := http.NewRequestWithContext(ctx, "GET", gateway.URL+"/api/ready", nil)
			response, e := client.Do(request)
			code := 0
			if e == nil {
				code = response.StatusCode
				body, readErr := io.ReadAll(response.Body)
				_ = response.Body.Close()
				e = readErr
				if e == nil && (code != 200 || !gjson.GetBytes(body, "success").Bool()) {
					e = fmt.Errorf("readiness HTTP %d", code)
				}
			}
			ready.record(code, time.Since(start), e, "")
			select {
			case <-stop:
				return
			case <-ticker.C:
			}
		}
	}()
	var stopOnce sync.Once
	stopReady := func() { stopOnce.Do(func() { close(stop) }); <-stopped }
	defer stopReady()
	phases := map[string]*multiUserLoadPhase{}
	phaseOrder := []string{}
	started := time.Now()
	persist := func(completed string) {
		trace.mu.Lock()
		scale.mu.Lock()
		mock.mu.RLock()
		report := map[string]any{
			"phase_completed": completed, "phase_order": phaseOrder, "users": users, "historical_assets": inventory, "assets_per_user": counts,
			"concurrency": clients, "requests_per_user_per_wave": clients / users, "upstream_accounts": 1, "upstream": "local mock", "upstream_delay_ms": 1,
			"historical_relationships": "v2 initially; one identity per user removed for explicit repair waves", "mysql_version": version,
			"main_pool_limit": 30, "main_pool_end_in_use": pool.Stats().InUse, "log_pool_limit": 5, "client_timeout_seconds": 20,
			"seed_seconds": seedSeconds, "duration_seconds": time.Since(started).Seconds(), "phases": phases, "sql": trace.phases,
			"database_samples": scale.Phases, "sample_interval_ms": 100, "upstream_calls": mock.calls, "upstream_peak_in_flight": mock.peak.Load(),
			"mock_assets_remaining": len(mock.assets), "mock_groups_remaining": len(mock.groups), "handlers_remaining": handlers.Load(), "ready": assetScaleStatsCopy(ready),
		}
		data, e := common.Marshal(report)
		mock.mu.RUnlock()
		scale.mu.Unlock()
		trace.mu.Unlock()
		require.NoError(t, e)
		if path := os.Getenv("ASSET_LOAD_REPORT_PATH"); path != "" {
			require.NoError(t, os.WriteFile(path, append(data, '\n'), 0600))
		}
	}
	type requestInput struct {
		action string
		body   map[string]any
		wantID string
		status int
	}
	runWave := func(name string, input func(int, int) requestInput, validate func(int, gjson.Result) error) {
		trace.phase.Store(name)
		scale.phase.Store(name)
		phase := &multiUserLoadPhase{Requests: &assetLoadRequests{}, Users: map[int]*assetLoadRequests{}}
		for user := 1; user <= users; user++ {
			phase.Users[user] = &assetLoadRequests{}
		}
		phases[name] = phase
		phaseOrder = append(phaseOrder, name)
		before, begin := pool.Stats(), time.Now()
		gate := make(chan struct{})
		var workers sync.WaitGroup
		for i := range clients {
			workers.Add(1)
			go func() {
				defer workers.Done()
				user := i%users + 1
				in := input(i, user)
				body, e := common.Marshal(in.body)
				<-gate
				stats := phase.Requests
				active := stats.inFlight.Add(1)
				assetLoadPeak(&stats.peak, active)
				defer stats.inFlight.Add(-1)
				begin := time.Now()
				code, state := 0, ""
				if e == nil {
					request, err := http.NewRequestWithContext(ctx, "POST", gateway.URL+"/?Action="+in.action+"&Version=2024-01-01", strings.NewReader(string(body)))
					e = err
					if e == nil {
						request.Header.Set("Authorization", fmt.Sprintf("Bearer seedancetest%d", user))
						request.Header.Set("Content-Type", "application/json")
						response, err := client.Do(request)
						e = err
						if e == nil {
							code = response.StatusCode
							raw, err := io.ReadAll(response.Body)
							_ = response.Body.Close()
							e = err
							result := arkLiveResult(raw)
							state = result.Get("Status").String()
							want := in.status
							if want == 0 {
								want = 200
							}
							if e == nil && code != want {
								e = fmt.Errorf("%s user=%d HTTP %d: %.240s", in.action, user, code, raw)
							}
							if e == nil && want == 200 && gjson.GetBytes(raw, "ResponseMetadata.Error").Exists() {
								e = fmt.Errorf("%s business error: %.240s", in.action, raw)
							}
							if e == nil && in.wantID != "" && result.Get("Id").String() != in.wantID {
								e = fmt.Errorf("%s returned wrong identity", in.action)
							}
							if e == nil && validate != nil {
								e = validate(user, result)
							}
						}
					}
				}
				elapsed := time.Since(begin)
				stats.record(code, elapsed, e, state)
				phase.Users[user].record(code, elapsed, e, state)
			}()
		}
		close(gate)
		workers.Wait()
		phase.Seconds = time.Since(begin).Seconds()
		phase.Requests.finish()
		for _, s := range phase.Users {
			s.finish()
		}
		drainStart := time.Now()
		for (handlers.Load() > 0 || mock.active.Load() > 0 || pool.Stats().InUse > 0) && time.Since(drainStart) < 20*time.Second {
			time.Sleep(25 * time.Millisecond)
		}
		phase.DrainMS = float64(time.Since(drainStart)) / float64(time.Millisecond)
		after := pool.Stats()
		phase.PoolWaits, phase.PoolWaitMS = after.WaitCount-before.WaitCount, float64(after.WaitDuration-before.WaitDuration)/float64(time.Millisecond)
		t.Logf("%s: count=%d HTTP=%v peak=%d p95=%.1fms max=%.1fms elapsed=%.2fs drain=%.1fms errors=%v", name, phase.Requests.Count, phase.Requests.HTTP, phase.Requests.PeakInFlight, phase.Requests.P95MS, phase.Requests.MaxMS, phase.Seconds, phase.DrainMS, phase.Requests.Errors)
		persist(name)
		require.Zero(t, handlers.Load(), "previous gateway wave must finish before the next begins")
		require.Zero(t, mock.active.Load(), "previous upstream wave must finish before the next begins")
	}
	assetInput := func(action string) func(int, int) requestInput {
		return func(i, user int) requestInput {
			id := multiUserNewID(user, i, "asset")
			body := map[string]any{"Id": id}
			if action == "CreateAsset" {
				body = map[string]any{"Name": id, "GroupId": multiUserGroupID(user), "AssetType": "Image", "URL": "https://example.com/load.png"}
			}
			if action == "UpdateAsset" {
				body["Name"] = "renamed-" + id
			}
			wantID := id
			if action == "DeleteAsset" || action == "UpdateAsset" {
				wantID = ""
			}
			return requestInput{action: action, body: body, wantID: wantID}
		}
	}
	groupInput := func(action string) func(int, int) requestInput {
		return func(i, user int) requestInput {
			id := multiUserNewID(user, i, "group")
			body := map[string]any{"Id": id}
			if action == "CreateAssetGroup" {
				body = map[string]any{"Name": id, "GroupType": "AIGC"}
			}
			if action == "UpdateAssetGroup" {
				body["Name"] = "renamed-" + id
			}
			wantID := id
			if action == "DeleteAssetGroup" || action == "UpdateAssetGroup" {
				wantID = ""
			}
			return requestInput{action: action, body: body, wantID: wantID}
		}
	}
	historyInput := func(i, user int) requestInput {
		id := multiUserHistoryID(user, 0)
		return requestInput{action: "GetAsset", body: map[string]any{"Id": id}, wantID: id}
	}
	runWave("CreateAsset", assetInput("CreateAsset"), nil)
	if os.Getenv("ASSET_LOAD_MULTIUSER_DIAGNOSE") == "1" {
		diagnoseMultiUserAssetQueries(t, pool, scope, users)
	}
	runWave("GetAsset", assetInput("GetAsset"), nil)
	runWave("GetHistoricalAsset", historyInput, nil)
	// Explicitly exercise the newly implemented compatibility fallback, without
	// incorrectly describing all 100,000 current-format records as unmigrated.
	trace.phase.Store("remove_test_relationships")
	keys := make([]string, 0, users)
	hash := func(s string) string { return fmt.Sprintf("%x", sha256.Sum256([]byte(s))) }
	for user := 1; user <= users; user++ {
		keys = append(keys, "identity:"+hash(fmt.Sprintf("20:%s:asset:%s", hash(scope), hash(multiUserHistoryID(user, 0)))))
	}
	require.NoError(t, model.DB.Where("channel_id = 0 AND profile_id = ? AND resource_id = ? AND pre_request_id = ? AND user_id = 0 AND token_id = 0 AND state_key IN ?", "asset-access-lock-v1", "handle", "global", keys).Delete(&model.ConfigurableResourceState{}).Error)
	runWave("GetHistoricalAssetRepair", historyInput, nil)
	runWave("GetHistoricalAssetAfterRepair", historyInput, nil)
	runWave("UpdateAsset", assetInput("UpdateAsset"), nil)
	listInput := func(action string) func(int, int) requestInput {
		return func(_, _ int) requestInput {
			return requestInput{action: action, body: map[string]any{"Filter": map[string]any{"GroupType": "AIGC"}, "PageNumber": 1, "PageSize": 20}}
		}
	}
	listValidator := func(kind string) func(int, gjson.Result) error {
		type ownedCount struct {
			StateValue string
			Total      int
		}
		var rows []ownedCount
		require.NoError(t, model.DB.Model(&model.ConfigurableResourceState{}).Select("state_value, COUNT(*) AS total").Where("profile_id = ? AND resource_id = ? AND status = ?", "asset-access-v1", kind, model.ConfigurableResourceStateStatusActive).Group("state_value").Scan(&rows).Error)
		totals := map[string]int{}
		for _, row := range rows {
			totals[row.StateValue] = row.Total
		}
		return func(user int, result gjson.Result) error {
			items := result.Get("Items")
			total := totals[fmt.Sprint(user)]
			if !items.IsArray() || int(result.Get("TotalCount").Int()) != total || len(items.Array()) != min(20, total) {
				return fmt.Errorf("wrong %s list total/page for user %d: want %d, got %s", kind, user, total, result.Raw[:min(180, len(result.Raw))])
			}
			mock.mu.RLock()
			defer mock.mu.RUnlock()
			store := mock.assets
			if kind == "group" {
				store = mock.groups
			}
			for _, row := range items.Array() {
				if store[row.Get("Id").String()].Owner != user {
					return fmt.Errorf("foreign %s leaked to user %d", kind, user)
				}
			}
			return nil
		}
	}
	runWave("ListAssets", listInput("ListAssets"), listValidator("asset"))
	runWave("CreateAssetGroup", groupInput("CreateAssetGroup"), nil)
	runWave("GetAssetGroup", groupInput("GetAssetGroup"), nil)
	runWave("UpdateAssetGroup", groupInput("UpdateAssetGroup"), nil)
	runWave("ListAssetGroups", listInput("ListAssetGroups"), listValidator("group"))
	runWave("CrossUserGetAssetDenied", func(i, user int) requestInput {
		other := user%users + 1
		return requestInput{action: "GetAsset", body: map[string]any{"Id": multiUserHistoryID(other, 0)}, status: 404}
	}, nil)
	runWave("DeleteAsset", assetInput("DeleteAsset"), nil)
	runWave("GetDeletedAssetDenied", func(i, user int) requestInput {
		in := assetInput("GetAsset")(i, user)
		in.wantID, in.status = "", 404
		return in
	}, nil)
	runWave("DeleteAssetGroup", groupInput("DeleteAssetGroup"), nil)
	stopReady()
	scale.phase.Store("finished")
	trace.phase.Store("finished")
	persist("finished")
	for name, phase := range phases {
		if len(phase.Requests.Errors) > 0 {
			t.Errorf("%s had request failures; see structured report", name)
		}
	}
	if len(ready.Errors) > 0 {
		t.Errorf("readiness failures: %v", ready.Errors)
	}
}

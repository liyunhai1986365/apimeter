package controller

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
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
	"github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"gorm.io/gorm"
)

// Explicit opt-in: creates 100 real assets and exactly two video submissions.
// Creation is never retried. Cleanup only touches IDs returned by this run;
// references for videos whose outcome remains uncertain are retained.
func TestAssetLoad100LiveMySQL57(t *testing.T) {
	configPath, evidence := os.Getenv("ASSET_LOAD_LIVE_CONFIG"), os.Getenv("ASSET_LOAD_LIVE_EVIDENCE")
	if configPath == "" || os.Getenv("ASSET_LOAD_LIVE_100") != "1" || os.Getenv("ASSET_TEST_MYSQL_DSN") == "" {
		t.Skip("requires explicit real-upstream opt-in, private config and isolated MySQL 5.7")
	}
	var cfg struct {
		URL, Key, Model, Project string
		AssetURL                 string `json:"asset_url"`
	}
	raw, err := os.ReadFile(configPath)
	require.NoError(t, err)
	require.NoError(t, common.Unmarshal(raw, &cfg))
	require.NotEmpty(t, cfg.Key)
	require.NotEmpty(t, cfg.AssetURL)
	require.NotEmpty(t, evidence)
	require.NoError(t, os.MkdirAll(evidence, 0700))
	router := tgxMaasResourceTestRouter(t, cfg.URL, cfg.Key, cfg.Model)
	ch, err := model.GetChannelById(20, true)
	require.NoError(t, err)
	settings := ch.GetSetting()
	settings.Protocol.ProjectName = cfg.Project
	ch.SetSetting(settings)
	require.NoError(t, model.DB.Save(ch).Error)
	var version, schema string
	require.NoError(t, model.DB.Raw("SELECT VERSION()").Scan(&version).Error)
	require.True(t, strings.HasPrefix(version, "5.7."))
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
	router.GET("/api/ready", GetReadiness)
	router.GET("/api/status", GetStatus)
	gateway := httptest.NewServer(router)
	defer gateway.Close()
	transport := &http.Transport{MaxIdleConns: 128, MaxIdleConnsPerHost: 128, MaxConnsPerHost: 128}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 65 * time.Second}
	ctx, cancel := context.WithTimeout(context.Background(), 18*time.Minute)
	defer cancel()
	stats := map[string]*assetLoadRequests{}
	for _, name := range []string{"group_create", "create", "poll", "video_create", "video_poll", "delete", "deleted_get", "group_delete", "ready", "status"} {
		stats[name] = &assetLoadRequests{}
	}
	call := func(kind, label, method, path string, body any, wantCode int) ([]byte, int, error) {
		s := stats[kind]
		assetLoadPeak(&s.peak, s.inFlight.Add(1))
		defer s.inFlight.Add(-1)
		started := time.Now()
		var payload []byte
		var err error
		if body != nil {
			payload, err = common.Marshal(body)
		}
		requestCtx, requestCancel := context.WithTimeout(ctx, 65*time.Second)
		defer requestCancel()
		if kind == "ready" || kind == "status" {
			requestCancel()
			requestCtx, requestCancel = context.WithTimeout(ctx, 5*time.Second)
			defer requestCancel()
		}
		var data []byte
		code := 0
		if err == nil {
			var req *http.Request
			req, err = http.NewRequestWithContext(requestCtx, method, gateway.URL+path, bytes.NewReader(payload))
			if err == nil {
				req.Header.Set("Authorization", "Bearer seedancetest1")
				req.Header.Set("Content-Type", "application/json")
				var resp *http.Response
				resp, err = client.Do(req)
				if err == nil {
					code = resp.StatusCode
					data, err = io.ReadAll(io.LimitReader(resp.Body, 2<<20))
					_ = resp.Body.Close()
				}
			}
		}
		if err == nil && (code != wantCode || (wantCode == 200 && gjson.GetBytes(data, "ResponseMetadata.Error").Exists())) {
			err = fmt.Errorf("%s HTTP %d: %.300s", label, code, data)
		}
		if err == nil && kind == "ready" && (!gjson.GetBytes(data, "success").Bool() || gjson.GetBytes(data, "data.checks.database").String() != "ok" || gjson.GetBytes(data, "data.checks.log_database").String() != "ok") {
			err = fmt.Errorf("readiness invalid: %.300s", data)
		}
		if label != "" && len(data) > 0 {
			if writeErr := os.WriteFile(filepath.Join(evidence, label+".json"), data, 0600); writeErr != nil && err == nil {
				err = writeErr
			}
		}
		state := gjson.GetBytes(data, "Result.Status").String()
		if strings.HasPrefix(kind, "video_") {
			state = gjson.GetBytes(data, "status").String()
		}
		s.record(code, time.Since(started), err, state)
		return data, code, err
	}
	cfgDB, err := mysql.ParseDSN(os.Getenv("ASSET_TEST_MYSQL_DSN"))
	require.NoError(t, err)
	cfgDB.DBName, cfgDB.Timeout = schema, 2*time.Second
	monitor, err := sql.Open("mysql", cfgDB.FormatDSN())
	require.NoError(t, err)
	defer monitor.Close()
	monitor.SetMaxOpenConns(1)
	before := pool.Stats()
	var poolPeak, lockPeak, monitorErrors atomic.Int64
	stop := make(chan struct{})
	var background sync.WaitGroup
	for _, probe := range []struct {
		kind, path string
		every      time.Duration
	}{{"ready", "/api/ready", 500 * time.Millisecond}, {"status", "/api/status", 3 * time.Second}} {
		background.Add(1)
		go func() {
			defer background.Done()
			ticker := time.NewTicker(probe.every)
			defer ticker.Stop()
			for {
				_, _, _ = call(probe.kind, "", "GET", probe.path, nil, 200)
				select {
				case <-ticker.C:
				case <-stop:
					return
				}
			}
		}()
	}
	background.Add(1)
	go func() {
		defer background.Done()
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			assetLoadPeak(&poolPeak, int64(pool.Stats().InUse))
			var waiters int64
			probeCtx, done := context.WithTimeout(ctx, time.Second)
			err := monitor.QueryRowContext(probeCtx, "SELECT COUNT(*) FROM information_schema.innodb_trx t JOIN information_schema.processlist p ON p.ID=t.trx_mysql_thread_id WHERE p.DB=? AND t.trx_state='LOCK WAIT'", schema).Scan(&waiters)
			done()
			if err != nil {
				monitorErrors.Add(1)
			} else {
				assetLoadPeak(&lockPeak, waiters)
			}
			select {
			case <-ticker.C:
			case <-stop:
				return
			}
		}
	}()
	type videoResult struct {
		ID              string `json:"id"`
		AssetID         string `json:"asset_id"`
		Status          string `json:"status"`
		MayBeRunning    bool   `json:"may_be_running"`
		DownloadedBytes int64  `json:"downloaded_bytes"`
	}
	var videos [2]videoResult
	ids, states := make([]string, 100), make([]string, 100)
	groupID := ""
	started := time.Now()
	var rounds, lateRounds int
	parallel := func(n int, work func(int)) {
		var workers sync.WaitGroup
		gate := make(chan struct{})
		for i := range n {
			workers.Add(1)
			go func() { defer workers.Done(); <-gate; work(i) }()
		}
		close(gate)
		workers.Wait()
	}
	defer func() {
		// Always attempt cleanup of known, unreferenced resources, even on assertion failure.
		retained := map[string]bool{}
		for _, v := range videos {
			if v.MayBeRunning {
				retained[v.AssetID] = true
			}
		}
		var deleted atomic.Int64
		parallel(len(ids), func(i int) {
			if ids[i] == "" || retained[ids[i]] {
				return
			}
			_, _, err := call("delete", fmt.Sprintf("asset-delete-%03d", i), "DELETE", "/v1/private-avatar/assets/"+ids[i], nil, 200)
			if err != nil {
				t.Errorf("asset cleanup %s: %v", ids[i], err)
				return
			}
			deleted.Add(1)
			bindings, err := model.FindAssetBindings(1, "asset", ids[i])
			if err != nil || len(bindings) != 0 {
				t.Errorf("deleted asset still accessible: %s", ids[i])
			}
			_, _, err = call("deleted_get", fmt.Sprintf("asset-after-delete-%03d", i), "GET", "/v1/private-avatar/assets/"+ids[i], nil, 404)
			if err != nil {
				t.Errorf("deleted asset GET %s: %v", ids[i], err)
			}
		})
		groupDeleted := false
		if groupID != "" && len(retained) == 0 {
			_, _, err := call("group_delete", "group-delete", "DELETE", "/v1/private-avatar/groups/"+groupID, nil, 200)
			groupDeleted = err == nil
			if err != nil {
				t.Errorf("group cleanup %s: %v", groupID, err)
			}
		}
		close(stop)
		background.Wait()
		after := pool.Stats()
		for name, s := range stats {
			s.finish()
			if name != "deleted_get" {
				if s.HTTP[503] > 0 {
					t.Errorf("%s returned %d HTTP 503", name, s.HTTP[503])
				}
			}
			if len(s.Errors) > 0 {
				t.Errorf("%s errors: %v", name, s.Errors)
			}
		}
		report := map[string]any{"upstream": cfg.URL, "mysql_version": version, "clients": 100, "instances": 1, "redis": "disabled", "interval_seconds": 3, "rounds": rounds, "late_rounds": lateRounds, "duration_seconds": time.Since(started).Seconds(), "requests": stats, "videos": videos, "asset_ids": ids, "final_asset_states": states, "group_id": groupID, "deleted_assets": deleted.Load(), "group_deleted": groupDeleted, "retained_assets": retained, "main_pool_limit": 30, "log_pool_limit": 5, "main_pool_peak_in_use": poolPeak.Load(), "main_pool_wait_count": after.WaitCount - before.WaitCount, "main_pool_wait_ms": float64(after.WaitDuration-before.WaitDuration) / float64(time.Millisecond), "sampled_server_lock_waiters_peak": lockPeak.Load(), "monitor_errors": monitorErrors.Load(), "passed": !t.Failed()}
		data, err := common.Marshal(report)
		if err == nil {
			err = os.WriteFile(filepath.Join(evidence, "report.json"), append(data, '\n'), 0600)
		}
		if err != nil {
			t.Errorf("write report: %v", err)
		}
		t.Logf("live cleanup: deleted=%d group_deleted=%v retained=%d; report saved", deleted.Load(), groupDeleted, len(retained))
	}()
	_, _, err = call("ready", "ready-initial", "GET", "/api/ready", nil, 200)
	require.NoError(t, err)
	data, _, err := call("group_create", "group-create", "POST", "/v1/private-avatar/groups", map[string]any{"model": cfg.Model, "Name": fmt.Sprintf("codex-load100-%d", time.Now().Unix()), "Description": "Temporary 100-client asset lifecycle test", "ProjectName": cfg.Project}, 200)
	groupID = gjson.GetBytes(data, "Result.Id").String()
	require.NoError(t, err)
	require.NotEmpty(t, groupID)
	parallel(100, func(i int) {
		data, _, err := call("create", fmt.Sprintf("asset-create-%03d", i), "POST", "/v1/private-avatar/assets", map[string]any{"model": cfg.Model, "GroupId": groupID, "ProjectName": cfg.Project, "URL": cfg.AssetURL, "AssetType": "Image", "Name": fmt.Sprintf("load100-%03d", i)}, 200)
		ids[i] = gjson.GetBytes(data, "Result.Id").String()
		if err != nil || ids[i] == "" {
			t.Errorf("asset creation %d failed: %v", i, err)
		}
	})
	t.Logf("live creation: requests=%d HTTP=%v elapsed=%.3fs", stats["create"].Count, stats["create"].HTTP, time.Since(started).Seconds())
	videoSubmitted := false
	deadline := time.Now().Add(12 * time.Minute)
	for time.Now().Before(deadline) && ctx.Err() == nil {
		waveStart := time.Now()
		parallel(100, func(i int) {
			if ids[i] == "" {
				return
			}
			data, _, err := call("poll", fmt.Sprintf("asset-latest-%03d", i), "GET", "/v1/private-avatar/assets/"+ids[i], nil, 200)
			if err == nil {
				states[i] = gjson.GetBytes(data, "Result.Status").String()
				if gjson.GetBytes(data, "Result.Id").String() != ids[i] {
					t.Errorf("query returned wrong asset ID")
				}
			}
		})
		rounds++
		active, failed := 0, 0
		var selected []string
		for i, s := range states {
			if s == "Active" {
				active++
				if len(selected) < 2 {
					selected = append(selected, ids[i])
				}
			}
			if s == "Failed" {
				failed++
			}
		}
		if !videoSubmitted && len(selected) == 2 {
			videoSubmitted = true
			parallel(2, func(i int) {
				videos[i].AssetID, videos[i].MayBeRunning = selected[i], true
				payload := map[string]any{"model": cfg.Model, "content": []map[string]any{{"type": "text", "text": "Keep the fruit arrangement from the reference image. Static camera, subtle natural lighting changes."}, {"type": "image_url", "image_url": map[string]string{"url": "asset://" + selected[i]}, "role": "first_frame"}}, "duration": 4, "resolution": "480p", "generate_audio": false}
				data, code, err := call("video_create", fmt.Sprintf("video-create-%d", i), "POST", "/api/v3/contents/generations/tasks", payload, 200)
				videos[i].ID = gjson.GetBytes(data, "id").String()
				if code >= 400 && code < 500 {
					videos[i].MayBeRunning = false
					videos[i].Status = "rejected"
				}
				if err != nil || videos[i].ID == "" {
					t.Errorf("video submission %d failed; will not resubmit: %v", i, err)
				}
			})
			t.Logf("video submissions: %s / %s", videos[0].ID, videos[1].ID)
		}
		parallel(2, func(i int) {
			v := &videos[i]
			if v.ID == "" || !v.MayBeRunning {
				return
			}
			data, _, err := call("video_poll", fmt.Sprintf("video-latest-%d", i), "GET", "/api/v3/contents/generations/tasks/"+v.ID, nil, 200)
			if err != nil {
				return
			}
			v.Status = gjson.GetBytes(data, "status").String()
			switch v.Status {
			case "succeeded", "failed", "expired", "cancelled":
				v.MayBeRunning = false
			}
			if v.Status == "succeeded" {
				url := gjson.GetBytes(data, "content.video_url").String()
				if url == "" {
					t.Errorf("video %d missing download URL", i)
					return
				}
				req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
				if err != nil {
					t.Errorf("video download request: %v", err)
					return
				}
				resp, err := client.Do(req)
				if err != nil {
					t.Errorf("video download failed: %v", err)
					return
				}
				defer resp.Body.Close()
				f, err := os.OpenFile(filepath.Join(evidence, fmt.Sprintf("video-%d.mp4", i)), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
				if err != nil {
					t.Errorf("video file: %v", err)
					return
				}
				defer f.Close()
				header := make([]byte, 64)
				n, readErr := io.ReadFull(resp.Body, header)
				if readErr != nil || resp.StatusCode != 200 || !bytes.Contains(header[:n], []byte("ftyp")) {
					t.Errorf("video %d invalid MP4 HTTP %d", i, resp.StatusCode)
					return
				}
				_, err = f.Write(header[:n])
				if err != nil {
					t.Errorf("video header write: %v", err)
					return
				}
				size, err := io.Copy(f, io.LimitReader(resp.Body, 32<<20))
				v.DownloadedBytes = size + int64(n)
				if err != nil || size >= 32<<20 {
					t.Errorf("video download truncated or too large: %v", err)
				}
			}
		})
		if rounds%5 == 0 || rounds == 1 {
			t.Logf("live round=%d Active=%d Failed=%d video=%s/%s HTTP=%v pool=%d", rounds, active, failed, videos[0].Status, videos[1].Status, stats["poll"].HTTP, pool.Stats().InUse)
		}
		if rounds >= 20 && videoSubmitted && !videos[0].MayBeRunning && !videos[1].MayBeRunning && active+failed == 100 {
			break
		}
		if !videoSubmitted && rounds >= 100 {
			break
		}
		delay := time.Until(waveStart.Add(3 * time.Second))
		if delay > 0 {
			timer := time.NewTimer(delay)
			select {
			case <-timer.C:
			case <-ctx.Done():
			}
			timer.Stop()
		} else {
			lateRounds++
		}
	}
	require.Equal(t, 100, stats["create"].HTTP[200])
	require.True(t, videoSubmitted, "not enough Active assets to submit videos")
	for _, v := range videos {
		require.Equal(t, "succeeded", v.Status)
		require.Positive(t, v.DownloadedBytes)
	}
	for _, state := range states {
		require.Equal(t, "Active", state)
	}
}

package controller

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/configurable"
	"github.com/QuantumNous/new-api/relay/channel/task/doubao"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/bytedance/gopkg/util/gopool"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// The mock is a separate HTTP process. Requests go through production token auth,
// distributor, controller, relay and persistent task lookup, not an adaptor stub.
func startSeedanceMock(t *testing.T, scenario string) string {
	t.Helper()
	cmd := exec.Command("python3", "../scripts/seedance_mock/server.py", "--port", "0", "--step-seconds", "0", "--scenario", scenario)
	out, err := cmd.StdoutPipe()
	require.NoError(t, err)
	cmd.Stderr = os.Stderr
	require.NoError(t, cmd.Start())
	t.Cleanup(func() { _ = cmd.Process.Signal(os.Interrupt); _ = cmd.Wait() })
	line := make(chan string, 1)
	go func() {
		scan := bufio.NewScanner(out)
		if scan.Scan() {
			line <- scan.Text()
		}
	}()
	select {
	case text := <-line:
		var port int
		_, err = fmt.Sscanf(text, "Seedance mock listening on ('127.0.0.1', %d)", &port)
		require.NoError(t, err)
		require.NotZero(t, port)
		return fmt.Sprintf("http://127.0.0.1:%d", port)
	case <-time.After(10 * time.Second):
		t.Fatal("mock startup timed out")
	}
	return ""
}

func seedanceTestRouter(t *testing.T, upstream, key, modelName string) *gin.Engine {
	t.Helper()
	require.NoError(t, i18n.Init())
	baselineWorkers := gopool.WorkerCount()
	originalRetries := common.RetryTimes
	common.RetryTimes = 0
	t.Cleanup(func() { common.RetryTimes = originalRetries })
	originalGroups := setting.UserUsableGroups2JSONString()
	originalRatios := ratio_setting.GroupRatio2JSONString()
	t.Cleanup(func() {
		_ = setting.UpdateUserUsableGroupsByJSONString(originalGroups)
		_ = ratio_setting.UpdateGroupRatioByJSONString(originalRatios)
	})
	db := openConfigurableResourceTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.BillingUsageItem{}, &model.AccountLedgerEntry{}))
	service.InitHttpClient()
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"default"}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1}`))
	originalPrices := ratio_setting.ModelPrice2JSONString()
	prices, err := common.Marshal(map[string]float64{modelName: 0.001})
	require.NoError(t, err)
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(string(prices)))
	t.Cleanup(func() { _ = ratio_setting.UpdateModelPriceByJSONString(originalPrices) })
	for id := 1; id <= 2; id++ {
		require.NoError(t, db.Create(&model.User{Id: id, Username: fmt.Sprintf("seedance-user-%d", id), AffCode: fmt.Sprintf("sd%d", id), Group: "default", Quota: 10000000, Status: common.UserStatusEnabled, Role: common.RoleCommonUser}).Error)
		require.NoError(t, db.Create(&model.Token{Id: id, UserId: id, Name: "seedance-test", Key: fmt.Sprintf("seedancetest%d", id), Status: common.TokenStatusEnabled, ExpiredTime: -1, RemainQuota: 10000000, Group: "default"}).Error)
	}
	require.NoError(t, db.Create(&model.Channel{Id: 20, Type: constant.ChannelTypeVolcEngine, Key: key, BaseURL: &upstream, Status: common.ChannelStatusEnabled, Name: "seedance-test", Models: modelName, Group: "default"}).Error)
	createConfigurableResourceAbility(t, db, 20, modelName, true, 1)
	r := gin.New()
	r.Use(middleware.RequestId())
	r.POST("/api/v3/contents/generations/tasks", middleware.ConfigurableNativeProfile("doubao-seedance-2", relayconstant.RelayModeVideoSubmit), middleware.TokenAuth(), middleware.Distribute(), RelayTask)
	r.GET("/api/v3/contents/generations/tasks/:task_id", middleware.ConfigurableNativeProfile("doubao-seedance-2", relayconstant.RelayModeVideoFetchByID), middleware.TokenAuth(), middleware.Distribute(), RelayTaskFetch)
	// Request-failure refunds and quota cache updates are asynchronous. Drain
	// them before the fixture restores process-global DB/cache settings.
	t.Cleanup(func() {
		require.Eventually(t, func() bool { return gopool.WorkerCount() <= baselineWorkers }, 5*time.Second, 5*time.Millisecond)
	})
	return r
}

func seedanceCall(r http.Handler, method, path, body string, user int) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Authorization", fmt.Sprintf("Bearer seedancetest%d", user))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestSeedanceGatewayMock(t *testing.T) {
	for _, scenario := range []string{"success", "failed", "expired", "cancelled", "query429", "query500", "create429", "queryinvalid", "querywrongid"} {
		t.Run(scenario, func(t *testing.T) {
			upstream := startSeedanceMock(t, scenario)
			r := seedanceTestRouter(t, upstream, "seedance-mock-key", "doubao-seedance-2-5-260628")
			payload := `{"model":"doubao-seedance-2-5-260628","content":[{"type":"text","text":"test"}],"duration":-1,"seed":0,"return_last_frame":false,"execution_expires_after":3600,"tools":[{"type":"web_search"}],"safety_identifier":"test-user","bitrate_mode":"mock-only","output_format":"mp4"}`
			created := seedanceCall(r, "POST", "/api/v3/contents/generations/tasks", payload, 1)
			if scenario == "create429" {
				require.Equal(t, 429, created.Code, created.Body.String())
				return
			}
			require.Equal(t, 200, created.Code, created.Body.String())
			id := gjson.Get(created.Body.String(), "id").String()
			require.True(t, strings.HasPrefix(id, "cgt-"), created.Body.String())
			task, exists, err := model.GetByTaskIDOrUpstreamID(1, id)
			require.NoError(t, err)
			require.True(t, exists)
			require.True(t, strings.HasPrefix(task.TaskID, "task_"))
			require.Equal(t, id, task.GetUpstreamTaskID())
			path := "/api/v3/contents/generations/tasks/" + id
			denied := seedanceCall(r, "GET", path, "", 2)
			require.NotEqual(t, 200, denied.Code)
			fetched := seedanceCall(r, "GET", path, "", 1)
			if scenario == "query429" {
				require.Equal(t, 429, fetched.Code, fetched.Body.String())
				require.Equal(t, "1", fetched.Header().Get("Retry-After"))
				return
			}
			if scenario == "query500" {
				require.Equal(t, 500, fetched.Code, fetched.Body.String())
				return
			}
			if scenario == "queryinvalid" || scenario == "querywrongid" {
				require.Equal(t, 502, fetched.Code, fetched.Body.String())
				return
			}
			require.Equal(t, 200, fetched.Code, fetched.Body.String())
			expected := scenario
			if scenario == "success" {
				expected = "succeeded"
			}
			require.Equal(t, expected, gjson.Get(fetched.Body.String(), "status").String())
			require.Equal(t, id, gjson.Get(fetched.Body.String(), "id").String())
			if scenario == "success" {
				require.Equal(t, int64(0), gjson.Get(fetched.Body.String(), "usage.mock_token_details.zero").Int())
				require.True(t, gjson.Get(fetched.Body.String(), "usage.mock_token_details.nested").Exists(), fetched.Body.String())
				require.True(t, gjson.Get(fetched.Body.String(), "mock_extension.preserve").Bool())
				require.False(t, gjson.Get(fetched.Body.String(), "content.last_frame_url").Exists())
			}
			var before, after model.User
			require.NoError(t, model.DB.First(&before, 1).Error)
			repeat := seedanceCall(r, "GET", path, "", 1)
			require.Equal(t, 200, repeat.Code)
			legacy := seedanceCall(r, "GET", "/api/v3/contents/generations/tasks/"+task.TaskID, "", 1)
			require.Equal(t, 200, legacy.Code)
			require.NoError(t, model.DB.First(&after, 1).Error)
			require.Equal(t, before.Quota, after.Quota)
			// A stale background poller loaded the task before the foreground transition.
			// It must lose the CAS and must not refund/settle or overwrite terminal data.
			stale := *task
			late := &relaycommon.TaskInfo{Status: string(model.TaskStatusFailure), Reason: "stale background result", Progress: "100%"}
			require.NoError(t, updateSeedanceVideoTask(context.Background(), &doubao.TaskAdaptor{}, &stale, late))
			var afterWorker model.User
			require.NoError(t, model.DB.First(&afterWorker, 1).Error)
			require.Equal(t, after.Quota, afterWorker.Quota)
			if scenario != "success" {
				require.Equal(t, 10000000, afterWorker.Quota)
			}
			reloaded, ok, err := model.GetByTaskIDOrUpstreamID(1, id)
			require.NoError(t, err)
			require.True(t, ok)
			require.NotEqual(t, "stale background result", reloaded.FailReason)

			req, _ := http.NewRequest("GET", upstream+"/__mock/requests", nil)
			req.Header.Set("Authorization", "Bearer seedance-mock-key")
			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			data, err := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			require.NoError(t, err)
			require.JSONEq(t, payload, gjson.GetBytes(data, "requests.0.body").Raw)
			// Denied query must not reach the upstream: create + three owner queries.
			require.Equal(t, int64(4), gjson.GetBytes(data, "requests.#").Int())
		})
	}
}

func TestSeedanceIntermediaryGateway(t *testing.T) {
	for _, scenario := range []string{"intermediary", "intermediarywrongid"} {
		t.Run(scenario, func(t *testing.T) {
			upstream := startSeedanceMock(t, scenario)
			r := seedanceTestRouter(t, upstream, "seedance-mock-key", "doubao-seedance-2-5-260628")
			payload := `{"model":"doubao-seedance-2-5-260628","content":[{"type":"text","text":"test"}],"duration":-1,"seed":0,"return_last_frame":true,"execution_expires_after":3600,"tools":[{"type":"web_search","extension":{"enabled":false}}],"safety_identifier":"test-user","bitrate_mode":"vbr","output_format":"mov"}`
			created := seedanceCall(r, "POST", "/api/v3/contents/generations/tasks", payload, 1)
			require.Equal(t, 200, created.Code)
			id := gjson.Get(created.Body.String(), "id").String()
			require.True(t, strings.HasPrefix(id, "cgt-"))
			require.Equal(t, id, gjson.Get(created.Body.String(), "upstream_task_id").String())
			task, ok, err := model.GetByTaskIDOrUpstreamID(1, id)
			require.NoError(t, err)
			require.True(t, ok)
			require.True(t, strings.HasPrefix(task.GetUpstreamTaskID(), "task_"))
			path := "/api/v3/contents/generations/tasks/" + id
			require.NotEqual(t, 200, seedanceCall(r, "GET", path, "", 2).Code)
			pending := seedanceCall(r, "GET", path, "", 1)
			require.Equal(t, 503, pending.Code)
			require.Equal(t, "task_status_pending", gjson.Get(pending.Body.String(), "code").String())
			require.Equal(t, "2", pending.Header().Get("Retry-After"))
			fetched := seedanceCall(r, "GET", path, "", 1)
			if scenario == "intermediarywrongid" {
				require.Equal(t, 502, fetched.Code)
				return
			}
			require.Equal(t, 200, fetched.Code, fetched.Body.String())
			require.Equal(t, id, gjson.Get(fetched.Body.String(), "id").String())
			require.Equal(t, "succeeded", gjson.Get(fetched.Body.String(), "status").String())
			require.Equal(t, upstream+"/media/frame", gjson.Get(fetched.Body.String(), "content.last_frame_url").String())
			require.Equal(t, upstream+"/media/video", gjson.Get(fetched.Body.String(), "official_url").String())
			require.Equal(t, "mov", gjson.Get(fetched.Body.String(), "output_format").String())
			require.Equal(t, "vbr", gjson.Get(fetched.Body.String(), "bitrate_mode").String())
			require.Equal(t, int64(0), gjson.Get(fetched.Body.String(), "seed").Int())
			require.Equal(t, int64(5), gjson.Get(fetched.Body.String(), "duration").Int())
			require.Equal(t, `{"completion_tokens":100,"total_tokens":100,"mock_token_details":{"zero":0,"nested":{"value":7}},"tool_usage":{"web_search":0}}`, strings.ReplaceAll(gjson.Get(fetched.Body.String(), "usage").Raw, " ", ""))
			var before, after model.User
			require.NoError(t, model.DB.First(&before, 1).Error)
			for _, alias := range []string{id, task.TaskID, task.GetUpstreamTaskID()} {
				repeat := seedanceCall(r, "GET", "/api/v3/contents/generations/tasks/"+alias, "", 1)
				require.Equal(t, 200, repeat.Code)
				require.Equal(t, id, gjson.Get(repeat.Body.String(), "id").String())
			}
			require.NoError(t, model.DB.First(&after, 1).Error)
			require.Equal(t, before.Quota, after.Quota)
			req, _ := http.NewRequest("GET", upstream+"/__mock/requests", nil)
			req.Header.Set("Authorization", "Bearer seedance-mock-key")
			response, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			data, err := io.ReadAll(response.Body)
			_ = response.Body.Close()
			require.NoError(t, err)
			require.JSONEq(t, payload, gjson.GetBytes(data, "requests.0.body").Raw)
			require.Equal(t, int64(6), gjson.GetBytes(data, "requests.#").Int())
			for _, request := range gjson.GetBytes(data, "requests").Array()[1:] {
				require.Equal(t, "/api/v3/contents/generations/tasks/"+task.GetUpstreamTaskID(), request.Get("path").String())
			}
		})
	}
}

func TestSeedanceBackgroundRejectsIncompleteAndMismatchedResponses(t *testing.T) {
	upstream := startSeedanceMock(t, "intermediarywrongid")
	r := seedanceTestRouter(t, upstream, "seedance-mock-key", "doubao-seedance-2-5-260628")
	created := seedanceCall(r, "POST", "/api/v3/contents/generations/tasks", `{"model":"doubao-seedance-2-5-260628","content":[{"type":"text","text":"test"}]}`, 1)
	require.Equal(t, 200, created.Code)
	task, ok, err := model.GetByTaskIDOrUpstreamID(1, gjson.Get(created.Body.String(), "id").String())
	require.NoError(t, err)
	require.True(t, ok)
	ch, err := model.GetChannelById(20, true)
	require.NoError(t, err)
	var before, after model.User
	require.NoError(t, model.DB.First(&before, 1).Error)
	for _, expected := range []string{"not available yet", "different official task ID"} {
		err := updateVideoSingleTask(context.Background(), &doubao.TaskAdaptor{}, ch, task.GetUpstreamTaskID(), map[string]*model.Task{task.GetUpstreamTaskID(): task})
		require.ErrorContains(t, err, expected)
		reloaded, ok, err := model.GetByTaskIDOrUpstreamID(1, task.GetNativeTaskID())
		require.NoError(t, err)
		require.True(t, ok)
		require.Equal(t, task.Status, reloaded.Status)
	}
	require.NoError(t, model.DB.First(&after, 1).Error)
	require.Equal(t, before.Quota, after.Quota)
}

// Explicitly opt-in; one POST only. Never included in ordinary CI runs.
func TestSeedanceGatewayLive(t *testing.T) {
	configPath := os.Getenv("SEEDANCE_LIVE_CONFIG")
	if configPath == "" {
		t.Skip("live test requires explicit SEEDANCE_LIVE_CONFIG")
	}
	var config struct {
		URL        string         `json:"url"`
		Profile    string         `json:"profile"`
		Key        string         `json:"key"`
		Model      string         `json:"model"`
		Resolution string         `json:"resolution"`
		Duration   int            `json:"duration"`
		Parameters map[string]any `json:"parameters"`
	}
	raw, err := os.ReadFile(configPath)
	require.NoError(t, err)
	require.NoError(t, common.Unmarshal(raw, &config))
	require.NotEmpty(t, config.Model)
	require.Positive(t, config.Duration)
	var r *gin.Engine
	if config.Profile == "seedance-tgxmaas" {
		r = tgxMaasResourceTestRouter(t, config.URL, config.Key, config.Model)
	} else {
		r = seedanceTestRouter(t, config.URL, config.Key, config.Model)
	}
	requestBody := map[string]any{"model": config.Model, "content": []map[string]string{{"type": "text", "text": "A red ball on a white table, static camera."}}, "duration": config.Duration, "resolution": config.Resolution}
	for key, value := range config.Parameters {
		requestBody[key] = value
	}
	payload, err := common.Marshal(requestBody)
	require.NoError(t, err)
	evidenceDir := os.Getenv("SEEDANCE_LIVE_EVIDENCE")
	require.NotEmpty(t, evidenceDir)
	require.NoError(t, os.MkdirAll(evidenceDir, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(evidenceDir, "request.json"), payload, 0600))
	created := seedanceCall(r, "POST", "/api/v3/contents/generations/tasks", string(payload), 1)
	require.NoError(t, os.WriteFile(filepath.Join(evidenceDir, "create.json"), created.Body.Bytes(), 0600))
	require.Equal(t, 200, created.Code, "creation failed; recorded in evidence, do not retry automatically")
	id := gjson.Get(created.Body.String(), "id").String()
	require.True(t, strings.HasPrefix(id, "cgt-"), created.Body.String())
	t.Logf("Created exactly one task: %s", id)
	deadline := time.Now().Add(12 * time.Minute)
	for time.Now().Before(deadline) {
		fetched := seedanceCall(r, "GET", "/api/v3/contents/generations/tasks/"+id, "", 1)
		require.NoError(t, os.WriteFile(filepath.Join(evidenceDir, "fetch.json"), fetched.Body.Bytes(), 0600))
		if fetched.Code == 429 || (fetched.Code == 503 && gjson.Get(fetched.Body.String(), "code").String() == "task_status_pending") {
			t.Logf("Query not ready (%d); retrying GET only", fetched.Code)
			time.Sleep(2 * time.Second)
			continue
		}
		require.Equal(t, 200, fetched.Code, "query failed; see evidence")
		require.Equal(t, id, gjson.Get(fetched.Body.String(), "id").String())
		status := gjson.Get(fetched.Body.String(), "status").String()
		t.Logf("Task status: %s", status)
		if status == "succeeded" {
			task, ok, err := model.GetByTaskIDOrUpstreamID(1, id)
			require.NoError(t, err)
			require.True(t, ok)
			expected, err := sjson.SetBytes(task.Data, "id", task.GetNativeTaskID())
			require.NoError(t, err)
			require.JSONEq(t, string(expected), fetched.Body.String())
			url := gjson.Get(fetched.Body.String(), "content.video_url").String()
			require.NotEmpty(t, url)
			client := &http.Client{Timeout: 60 * time.Second}
			response, err := client.Get(url)
			require.NoError(t, err)
			defer response.Body.Close()
			require.Equal(t, 200, response.StatusCode)
			video, err := io.ReadAll(io.LimitReader(response.Body, 32<<20))
			require.NoError(t, err)
			require.True(t, bytes.Contains(video[:min(len(video), 64)], []byte("ftyp")), "expected MP4/MOV header")
			require.NoError(t, os.WriteFile(filepath.Join(evidenceDir, "video.bin"), video, 0600))
			if wantFrame, _ := requestBody["return_last_frame"].(bool); wantFrame {
				frameURL := gjson.Get(fetched.Body.String(), "content.last_frame_url").String()
				require.NotEmpty(t, frameURL)
				frameResponse, err := client.Get(frameURL)
				require.NoError(t, err)
				frame, err := io.ReadAll(io.LimitReader(frameResponse.Body, 16<<20))
				_ = frameResponse.Body.Close()
				require.NoError(t, err)
				require.Equal(t, 200, frameResponse.StatusCode)
				require.NoError(t, os.WriteFile(filepath.Join(evidenceDir, "frame.bin"), frame, 0600))
				_, _, err = image.DecodeConfig(bytes.NewReader(frame))
				require.NoError(t, err)
			}
			if format, ok := requestBody["output_format"].(string); ok {
				require.Equal(t, format, gjson.Get(fetched.Body.String(), "output_format").String())
			}
			require.Positive(t, gjson.Get(fetched.Body.String(), "duration").Float())
			if expires, ok := requestBody["execution_expires_after"].(float64); ok {
				require.Equal(t, int64(expires), gjson.Get(fetched.Body.String(), "execution_expires_after").Int())
			}
			t.Logf("Parameter evidence: seed=%s output_format=%s duration=%s tools_usage=%s", gjson.Get(fetched.Body.String(), "seed").Raw, gjson.Get(fetched.Body.String(), "output_format").Raw, gjson.Get(fetched.Body.String(), "duration").Raw, gjson.Get(fetched.Body.String(), "usage.tool_usage").Raw)

			var before, after model.User
			require.NoError(t, model.DB.First(&before, 1).Error)
			repeat := seedanceCall(r, "GET", "/api/v3/contents/generations/tasks/"+id, "", 1)
			require.Equal(t, 200, repeat.Code)
			require.NoError(t, model.DB.First(&after, 1).Error)
			require.Equal(t, before.Quota, after.Quota)
			t.Logf("Live lifecycle passed; downloaded %d bytes; usage=%s", len(video), gjson.Get(fetched.Body.String(), "usage").Raw)
			return
		}
		if status == "failed" || status == "expired" || status == "cancelled" {
			t.Fatalf("upstream terminal failure %s; see evidence; no second task created", status)
		}
		time.Sleep(5 * time.Second)
	}
	t.Fatal("task did not finish within test deadline; do not resubmit")
}

func TestSeedanceCreatePersistenceFailureDoesNotReturnSuccess(t *testing.T) {
	upstream := startSeedanceMock(t, "success")
	r := seedanceTestRouter(t, upstream, "seedance-mock-key", "doubao-seedance-2-0-260128")
	// Failure happens after upstream acceptance; client must not receive a 200 ID.
	require.NoError(t, model.DB.Migrator().DropTable(&model.Task{}))
	w := seedanceCall(r, "POST", "/api/v3/contents/generations/tasks", `{"model":"doubao-seedance-2-0-260128","content":[{"type":"text","text":"test"}],"duration":4}`, 1)
	require.Equal(t, 500, w.Code)
	require.Equal(t, "task_persistence_failed", gjson.Get(w.Body.String(), "code").String())
	require.Contains(t, w.Body.String(), "do not resubmit")
}

func TestSeedanceForegroundBackgroundRace(t *testing.T) {
	for _, scenario := range []string{"success", "failed"} {
		t.Run(scenario, func(t *testing.T) {
			upstream := startSeedanceMock(t, scenario)
			r := seedanceTestRouter(t, upstream, "seedance-mock-key", "doubao-seedance-2-0-260128")
			sqlDB, err := model.DB.DB()
			require.NoError(t, err)
			sqlDB.SetMaxOpenConns(1)
			created := seedanceCall(r, "POST", "/api/v3/contents/generations/tasks", `{"model":"doubao-seedance-2-0-260128","content":[{"type":"text","text":"test"}],"duration":4}`, 1)
			require.Equal(t, 200, created.Code)
			id := gjson.Get(created.Body.String(), "id").String()
			task, ok, err := model.GetByTaskIDOrUpstreamID(1, id)
			require.NoError(t, err)
			require.True(t, ok)
			result := &relaycommon.TaskInfo{Status: string(model.TaskStatusSuccess), TotalTokens: 100, CompletionTokens: 100, Progress: "100%"}
			if scenario == "failed" {
				result.Status = string(model.TaskStatusFailure)
				result.Reason = "Mock task failed"
			}
			var wg sync.WaitGroup
			start := make(chan struct{})
			errors := make(chan error, 1)
			statuses := make(chan int, 2)
			wg.Add(3)
			go func() {
				defer wg.Done()
				<-start
				errors <- updateSeedanceVideoTask(context.Background(), &doubao.TaskAdaptor{}, task, result)
			}()
			for i := 0; i < 2; i++ {
				go func() {
					defer wg.Done()
					<-start
					statuses <- seedanceCall(r, "GET", "/api/v3/contents/generations/tasks/"+id, "", 1).Code
				}()
			}
			close(start)
			wg.Wait()
			require.NoError(t, <-errors)
			require.Equal(t, 200, <-statuses)
			require.Equal(t, 200, <-statuses)
			var user model.User
			require.NoError(t, model.DB.First(&user, 1).Error)
			expected := 10000000
			if scenario == "success" {
				expected -= task.Quota
			}
			require.Equal(t, expected, user.Quota)
		})
	}
}

func TestSeedanceBackgroundRejectsNestedTaskMismatch(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"task":{"id":"cgt-other","status":"succeeded","outputs":["https://other.example/video.mp4"]}}`))
	}))
	defer upstream.Close()
	seedanceTestRouter(t, upstream.URL, "test", "doubao-seedance-2-0-260128")
	ch, err := model.GetChannelById(20, true)
	require.NoError(t, err)
	ch.Type = constant.ChannelTypeConfigurable
	settings := dto.ChannelSettings{Protocol: &dto.ChannelProtocolSettings{ProfileID: "seedance2-service-inference"}}
	ch.SetSetting(settings)
	adaptor := &configurable.TaskAdaptor{}
	adaptor.Init(&relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelType: ch.Type, ChannelBaseUrl: upstream.URL, ChannelSetting: settings}})
	task := &model.Task{TaskID: "task_nested", UserId: 1, ChannelId: 20, Status: model.TaskStatusInProgress, PrivateData: model.TaskPrivateData{UpstreamTaskID: "cgt-owner"}}
	require.NoError(t, model.DB.Create(task).Error)
	err = updateVideoSingleTask(context.Background(), adaptor, ch, "cgt-owner", map[string]*model.Task{"cgt-owner": task})
	require.ErrorContains(t, err, "different task ID")
	var saved model.Task
	require.NoError(t, model.DB.First(&saved, task.ID).Error)
	require.Equal(t, model.TaskStatus(model.TaskStatusInProgress), saved.Status)
	require.Empty(t, saved.Data)
	require.Empty(t, task.Data)
}

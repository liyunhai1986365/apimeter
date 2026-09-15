package controller

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

const tgxRegressionModel = "doubao-seedance-2-5-260628"

func TestTgxRegressionResourceModelPermission(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_, _ = w.Write([]byte(`{"Result":{"Id":"asset_local"}}`))
	}))
	defer upstream.Close()
	r := tgxMaasResourceTestRouter(t, upstream.URL, "test", tgxRegressionModel)
	require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", 1).Updates(map[string]any{"model_limits_enabled": true, "model_limits": "allowed-other-model"}).Error)
	video := seedanceCall(r, "POST", "/api/v3/contents/generations/tasks", `{"model":"`+tgxRegressionModel+`","content":[{"type":"text","text":"test"}]}`, 1)
	require.Equal(t, 403, video.Code)
	resource := seedanceCall(r, "POST", "/v1/private-avatar/assets", `{"model":"`+tgxRegressionModel+`","GroupId":"ag_test","URL":"https://example.com/input.png","AssetType":"Image","Name":"test"}`, 1)
	t.Logf("same restricted token: video=%d asset=%d upstream calls=%d", video.Code, resource.Code, calls.Load())
	require.Equal(t, 403, resource.Code)
}

func TestTgxRegressionRequestPrecision(t *testing.T) {
	for _, path := range []string{"/api/v3/contents/generations/tasks", "/v1/private-avatar/groups"} {
		t.Run(path, func(t *testing.T) {
			var observed atomic.Value
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				require.NoError(t, err)
				observed.Store(gjson.GetBytes(body, "extension.counter").Raw)
				_, _ = w.Write([]byte(`{"id":"task_vendor","upstream_task_id":"cgt-original","Result":{"Id":"ag_local"}}`))
			}))
			defer upstream.Close()
			r := tgxMaasResourceTestRouter(t, upstream.URL, "test", tgxRegressionModel)
			response := seedanceCall(r, "POST", path, `{"model":"`+tgxRegressionModel+`","content":[{"type":"text","text":"test"}],"Name":"test","extension":{"counter":9007199254740993}}`, 1)
			require.Equal(t, 200, response.Code, response.Body.String())
			t.Logf("upstream counter=%v", observed.Load())
			require.Equal(t, "9007199254740993", observed.Load())
		})
	}
}

func TestTgxRegressionInvalidCreateID(t *testing.T) {
	for _, body := range []string{
		`{"id":123,"upstream_task_id":"cgt-original"}`,
		`{"id":{},"upstream_task_id":"cgt-original"}`,
		`{"id":[],"upstream_task_id":"cgt-original"}`,
		`{"id":null}`, `{"id":""}`, `{"id":"   "}`,
		`{"id":"task-vendor","upstream_task_id":123}`,
		`{"id":"task-vendor","upstream_task_id":null}`,
		`{"id":"task-vendor","upstream_task_id":""}`,
		`{"id":"task-vendor"} trailing`, `{"accepted":true}`,
	} {
		t.Run(body, func(t *testing.T) {
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				_, _ = w.Write([]byte(body))
			}))
			defer upstream.Close()
			r := tgxMaasResourceTestRouter(t, upstream.URL, "test", tgxRegressionModel)
			oldRetries := common.RetryTimes
			common.RetryTimes = 2
			t.Cleanup(func() { common.RetryTimes = oldRetries })
			response := seedanceCall(r, "POST", "/api/v3/contents/generations/tasks", `{"model":"`+tgxRegressionModel+`","content":[{"type":"text","text":"test"}]}`, 1)
			require.Equal(t, 502, response.Code, response.Body.String())
			require.Equal(t, "task_submission_unconfirmed", gjson.GetBytes(response.Body.Bytes(), "code").String())
			require.Contains(t, response.Body.String(), "do not resubmit")
			require.EqualValues(t, 1, calls.Load())
			var tasks []model.Task
			require.NoError(t, model.DB.Find(&tasks).Error)
			require.Empty(t, tasks)
			var user model.User
			require.NoError(t, model.DB.First(&user, 1).Error)
			require.Less(t, user.Quota, 10000000, "ambiguous acceptance must retain the reservation for reconciliation")
		})
	}
}

func TestTgxRegressionResourcesFailureIsolation(t *testing.T) {
	for _, mode := range []string{"fixed", "smart"} {
		for _, status := range []int{429, 500} {
			t.Run(mode+http.StatusText(status), func(t *testing.T) {
				var calls [2]atomic.Int32
				primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls[0].Add(1)
					w.Header().Set("Retry-After", "3")
					w.WriteHeader(status)
					_, _ = w.Write([]byte(`{"ResponseMetadata":{"Error":{"Code":"ProviderFailure","Message":"try later"}}}`))
				}))
				defer primary.Close()
				backup := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls[1].Add(1)
					_, _ = w.Write([]byte(`{"Result":{"Id":"wrong_account"}}`))
				}))
				defer backup.Close()
				r := tgxMaasResourceTestRouter(t, primary.URL, "primary", tgxRegressionModel)
				ch, err := model.GetChannelById(20, true)
				require.NoError(t, err)
				second := *ch
				second.Id = 21
				second.Key = "backup"
				second.BaseURL = &backup.URL
				second.Priority = common.GetPointer(int64(-1))
				require.NoError(t, model.DB.Create(&second).Error)
				createConfigurableResourceAbility(t, model.DB, 21, tgxRegressionModel, true, 0)
				oldRetries := common.RetryTimes
				common.RetryTimes = 3
				t.Cleanup(func() { common.RetryTimes = oldRetries })
				if mode == "smart" {
					oldAuto := setting.AutoGroups2JsonString()
					t.Cleanup(func() { require.NoError(t, setting.UpdateAutoGroupsByJsonString(oldAuto)) })
					require.NoError(t, setting.UpdateAutoGroupsByJsonString(`["default"]`))
					require.NoError(t, model.DB.AutoMigrate(&model.RoutingStrategySnapshot{}))
					require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", 1).Updates(map[string]any{"group": "auto", "group_policy": `{"type":"routing_strategy","strategy":"` + model.RoutingStrategies()[0] + `"}`}).Error)
				}
				response := seedanceCall(r, "POST", "/v1/private-avatar/groups", `{"model":"`+tgxRegressionModel+`","Name":"test"}`, 1)
				t.Logf("HTTP=%d primary=%d backup=%d Retry-After=%q", response.Code, calls[0].Load(), calls[1].Load(), response.Header().Get("Retry-After"))
				require.EqualValues(t, 1, calls[0].Load())
				require.Zero(t, calls[1].Load())
				require.GreaterOrEqual(t, response.Code, 400)
				require.Equal(t, "3", response.Header().Get("Retry-After"))
				var tasks int64
				require.NoError(t, model.DB.Model(&model.Task{}).Count(&tasks).Error)
				require.Zero(t, tasks)
			})
		}
	}
}

func TestTgxRegressionResourceGroupAndAbilityRestriction(t *testing.T) {
	for _, denied := range []string{"disabled_channel", "wrong_group", "disabled_ability"} {
		t.Run(denied, func(t *testing.T) {
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				_, _ = w.Write([]byte(`{"Result":{"Id":"asset_local"}}`))
			}))
			defer upstream.Close()
			r := tgxMaasResourceTestRouter(t, upstream.URL, "test", tgxRegressionModel)
			switch denied {
			case "disabled_channel":
				require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", 20).Update("status", common.ChannelStatusManuallyDisabled).Error)
			case "wrong_group":
				ch, err := model.GetChannelById(20, true)
				require.NoError(t, err)
				ch.Group = "not-authorized"
				require.NoError(t, model.DB.Save(ch).Error)
			case "disabled_ability":
				require.NoError(t, model.DB.Model(&model.Ability{}).Where("channel_id = ?", 20).Update("enabled", false).Error)
			}
			response := seedanceCall(r, "GET", "/v1/private-avatar/assets/asset_local", "", 1)
			require.NotEqual(t, 200, response.Code, response.Body.String())
			require.Zero(t, calls.Load())
		})
	}
}

func TestTgxRegressionAcceptedUnparseableCreateDoesNotReplay(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if n == 1 {
			_, _ = w.Write([]byte(`{"accepted":true}`))
			return
		}
		_, _ = w.Write([]byte(`{"id":"task_second","upstream_task_id":"cgt-second"}`))
	}))
	defer upstream.Close()
	r := tgxMaasResourceTestRouter(t, upstream.URL, "test", tgxRegressionModel)
	oldRetries := common.RetryTimes
	common.RetryTimes = 1
	t.Cleanup(func() { common.RetryTimes = oldRetries })
	response := seedanceCall(r, "POST", "/api/v3/contents/generations/tasks", `{"model":"`+tgxRegressionModel+`","content":[{"type":"text","text":"test"}]}`, 1)
	t.Logf("HTTP=%d upstream POST count=%d body=%s", response.Code, calls.Load(), response.Body.String())
	require.EqualValues(t, 1, calls.Load(), "HTTP 200 with an unusable body may already have created a billed task")
}

func TestTgxRegressionVideoNativeLifecycle(t *testing.T) {
	for _, terminal := range []string{"succeeded", "failed", "expired", "cancelled"} {
		t.Run(terminal, func(t *testing.T) {
			var creates, gets atomic.Int32
			var phase atomic.Int32
			var submitted atomic.Value
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, "Bearer test", r.Header.Get("Authorization"))
				if r.Method == "POST" {
					require.Equal(t, "/doubao/api/v3/contents/generations/tasks", r.URL.Path)
					creates.Add(1)
					raw, err := io.ReadAll(r.Body)
					require.NoError(t, err)
					submitted.Store(string(raw))
					_, _ = w.Write([]byte(`{"id":"task_vendor","upstream_task_id":"cgt-official"}`))
					return
				}
				gets.Add(1)
				require.Equal(t, "/doubao/api/v3/contents/generations/tasks/task_vendor", r.URL.Path)
				switch phase.Load() {
				case 0:
					_, _ = w.Write([]byte(`{"id":"task_vendor","upstream_task_id":"cgt-official"}`))
				case 1:
					w.Header().Set("Retry-After", "2")
					w.WriteHeader(429)
					_, _ = w.Write([]byte(`{"error":{"code":"Throttled","message":"busy"}}`))
				case 2:
					_, _ = w.Write([]byte(`{"id":"task_vendor","upstream_task_id":"cgt-other","status":"succeeded"}`))
				case 3:
					_, _ = w.Write([]byte(`{"id":"task_vendor","upstream_task_id":"cgt-official","status":"running"}`))
				case 4:
					_, _ = w.Write([]byte(`{"id":"task_vendor","upstream_task_id":"cgt-official","status":"` + terminal + `","content":{"video_url":"https://example.com/video.mov","last_frame_url":"https://example.com/frame.png"},"usage":{"total_tokens":9007199254740993,"completion_tokens":20,"tool_usage":{"web_search":0}},"error":{"code":"ProviderTerminal","message":"terminal"},"future":{"zero":0,"flag":false}}`))
				case 5:
					w.WriteHeader(503)
					_, _ = w.Write([]byte(`{"error":{"code":"Unavailable"}}`))
				}
			}))
			defer upstream.Close()
			r := tgxMaasResourceTestRouter(t, upstream.URL, "test", tgxRegressionModel)
			payload := `{"model":"` + tgxRegressionModel + `","content":[{"type":"text","text":"test"},{"type":"image_url","role":"first_frame","image_url":{"url":"asset://asset_local"}}],"duration":-1,"seed":0,"return_last_frame":false,"execution_expires_after":3600,"safety_identifier":"test-user","tools":[{"type":"web_search"}],"bitrate_mode":"vbr","output_format":"mov"}`
			created := seedanceCall(r, "POST", "/api/v3/contents/generations/tasks", payload, 1)
			require.Equal(t, 200, created.Code, created.Body.String())
			require.Equal(t, "cgt-official", gjson.GetBytes(created.Body.Bytes(), "id").String())
			require.JSONEq(t, payload, submitted.Load().(string))
			task, exists, err := model.GetByTaskIDOrUpstreamID(1, "cgt-official")
			require.NoError(t, err)
			require.True(t, exists)
			require.Equal(t, "task_vendor", task.PrivateData.UpstreamTaskID)
			path := "/api/v3/contents/generations/tasks/cgt-official"
			denied := seedanceCall(r, "GET", path, "", 2)
			require.NotEqual(t, 200, denied.Code)
			require.Zero(t, gets.Load())
			for p, code := range []int{503, 429, 502, 200} {
				phase.Store(int32(p))
				response := seedanceCall(r, "GET", path, "", 1)
				require.Equal(t, code, response.Code, response.Body.String())
				if p == 0 || p == 1 {
					require.Equal(t, "2", response.Header().Get("Retry-After"))
				}
			}
			phase.Store(4)
			seedancePollingFixture(t, true, 0)
			var wg sync.WaitGroup
			wg.Add(2)
			go func() { defer wg.Done(); service.RunTaskPollingOnce(context.Background(), func(int, int) {}) }()
			go func() {
				defer wg.Done()
				response := seedanceCall(r, "GET", path, "", 1)
				require.Equal(t, 200, response.Code, response.Body.String())
				require.Equal(t, terminal, gjson.GetBytes(response.Body.Bytes(), "status").String())
			}()
			wg.Wait()
			phase.Store(5)
			cached := seedanceCall(r, "GET", path, "", 1)
			require.Equal(t, 200, cached.Code, cached.Body.String())
			require.Equal(t, terminal, gjson.GetBytes(cached.Body.Bytes(), "status").String())
			require.Equal(t, "9007199254740993", gjson.GetBytes(cached.Body.Bytes(), "usage.total_tokens").Raw)
			var user model.User
			require.NoError(t, model.DB.First(&user, 1).Error)
			expected := 10000000
			if terminal == "succeeded" {
				expected -= task.Quota
			}
			require.Equal(t, expected, user.Quota)
			require.EqualValues(t, 1, creates.Load())
		})
	}
}

func TestTgxRegressionTruncatedAcceptedResponse(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Length", "1000")
		_, _ = w.Write([]byte(`{"id":"task-truncated"`))
	}))
	defer upstream.Close()
	r := tgxMaasResourceTestRouter(t, upstream.URL, "test", tgxRegressionModel)
	oldRetries := common.RetryTimes
	common.RetryTimes = 2
	t.Cleanup(func() { common.RetryTimes = oldRetries })
	response := seedanceCall(r, "POST", "/api/v3/contents/generations/tasks", `{"model":"`+tgxRegressionModel+`","content":[{"type":"text","text":"test"}]}`, 1)
	require.GreaterOrEqual(t, response.Code, 400)
	require.EqualValues(t, 1, calls.Load())
	var user model.User
	require.NoError(t, model.DB.First(&user, 1).Error)
	require.Less(t, user.Quota, 10000000)
}

func TestTgxRegressionResourceTokenPermissionsAcrossRouting(t *testing.T) {
	for _, smart := range []bool{false, true} {
		t.Run(map[bool]string{false: "fixed", true: "smart"}[smart], func(t *testing.T) {
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				_, _ = w.Write([]byte(`{"Result":{"Id":"asset_local"}}`))
			}))
			defer upstream.Close()
			r := tgxMaasResourceTestRouter(t, upstream.URL, "test", tgxRegressionModel)
			if smart {
				oldAuto := setting.AutoGroups2JsonString()
				t.Cleanup(func() { require.NoError(t, setting.UpdateAutoGroupsByJsonString(oldAuto)) })
				require.NoError(t, setting.UpdateAutoGroupsByJsonString(`["default"]`))
				require.NoError(t, model.DB.AutoMigrate(&model.RoutingStrategySnapshot{}))
				require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", 1).Updates(map[string]any{"group": "auto", "group_policy": `{"type":"routing_strategy","strategy":"` + model.RoutingStrategies()[0] + `"}`}).Error)
			}
			for _, allowed := range []string{"", "other", tgxRegressionModel} {
				require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", 1).Updates(map[string]any{"model_limits_enabled": true, "model_limits": allowed}).Error)
				for _, tc := range []struct {
					method, path, body string
					explicit           bool
				}{
					{"POST", "/v1/private-avatar/assets", `{"model":"` + tgxRegressionModel + `"}`, true},
					{"GET", "/v1/private-avatar/assets/asset_local?model=" + tgxRegressionModel, "", true},
					{"POST", "/v1/private-avatar/assets", `{}`, false},
					{"GET", "/v1/private-avatar/assets/asset_local", "", false},
				} {
					before := calls.Load()
					response := seedanceCall(r, tc.method, tc.path, tc.body, 1)
					if allowed == tgxRegressionModel && tc.explicit {
						require.Equal(t, 200, response.Code, response.Body.String())
						require.Equal(t, before+1, calls.Load())
					} else {
						require.Equal(t, 403, response.Code, response.Body.String())
						require.Equal(t, before, calls.Load())
					}
				}
			}
		})
	}
}

func TestTgxRegressionCreatedStatusPreservesAcceptance(t *testing.T) {
	for _, body := range []string{`{"id":"task-created","upstream_task_id":"cgt-created"}`, `{"accepted":true}`} {
		t.Run(body, func(t *testing.T) {
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.WriteHeader(http.StatusCreated)
				_, _ = w.Write([]byte(body))
			}))
			defer upstream.Close()
			r := tgxMaasResourceTestRouter(t, upstream.URL, "test", tgxRegressionModel)
			response := seedanceCall(r, "POST", "/api/v3/contents/generations/tasks", `{"model":"`+tgxRegressionModel+`","content":[{"type":"text","text":"test"}]}`, 1)
			if gjson.Get(body, "id").Exists() {
				require.Equal(t, 200, response.Code, response.Body.String())
				require.Equal(t, "cgt-created", gjson.GetBytes(response.Body.Bytes(), "id").String())
			} else {
				require.Equal(t, 502, response.Code, response.Body.String())
				require.Equal(t, "task_submission_unconfirmed", gjson.GetBytes(response.Body.Bytes(), "code").String())
			}
			require.EqualValues(t, 1, calls.Load())
		})
	}
}

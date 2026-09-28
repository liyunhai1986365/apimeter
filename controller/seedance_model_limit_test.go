package controller

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestSeedanceVideoFetchModelLimit(t *testing.T) {
	const customerModel = "doubao-seedance-2-0-260128"
	const upstreamModel = "doubao-seedance-2-5-260628"
	const supplierID = "task_supplier_model_limit"
	const officialID = "cgt-model-limit"
	const nativePath = "/api/v3/contents/generations/tasks/"

	for _, profile := range []string{"volcengine", "doubao-seedance-2", "seedance-tgxmaas"} {
		t.Run(profile, func(t *testing.T) {
			var submissions, queries atomic.Int32
			var completed atomic.Bool
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.Method == http.MethodPost {
					submissions.Add(1)
					body, err := io.ReadAll(r.Body)
					assert.NoError(t, err)
					assert.Equal(t, upstreamModel, gjson.GetBytes(body, "model").String())
					_, _ = w.Write([]byte(`{"id":"task_supplier_model_limit","upstream_task_id":"cgt-model-limit"}`))
					return
				}
				queries.Add(1)
				assert.Equal(t, http.MethodGet, r.Method)
				assert.True(t, strings.HasSuffix(r.URL.Path, "/contents/generations/tasks/"+supplierID), r.URL.Path)
				if completed.Load() {
					_, _ = w.Write([]byte(`{"id":"task_supplier_model_limit","upstream_task_id":"cgt-model-limit","status":"succeeded","content":{"video_url":"https://cdn.example/video.mp4"},"usage":{"completion_tokens":100,"total_tokens":100}}`))
				} else {
					_, _ = w.Write([]byte(`{"id":"task_supplier_model_limit","upstream_task_id":"cgt-model-limit","status":"running"}`))
				}
			}))
			defer upstream.Close()

			r := seedanceTestRouter(t, upstream.URL, "mock-key", customerModel)
			ch, err := model.GetChannelById(20, true)
			require.NoError(t, err)
			ch.ModelMapping = common.GetPointer(`{"doubao-seedance-2-0-260128":"doubao-seedance-2-5-260628"}`)
			if profile != "volcengine" {
				ch.Type = constant.ChannelTypeConfigurable
				ch.SetSetting(dto.ChannelSettings{Protocol: &dto.ChannelProtocolSettings{ProfileID: profile}})
			}
			require.NoError(t, model.DB.Save(ch).Error)
			for _, path := range []string{"/v1/videos/:task_id", "/v1/video/generations/:task_id"} {
				r.GET(path, middleware.TokenAuth(), middleware.DistributeVideoTaskFetch(), RelayTaskFetch)
			}
			require.NoError(t, model.DB.Create(&model.Token{
				Id: 3, UserId: 1, Name: "same-user-other-key", Key: "seedancetest3",
				Status: common.TokenStatusEnabled, ExpiredTime: -1, RemainQuota: 10000000,
				Group: "default", ModelLimitsEnabled: true, ModelLimits: customerModel,
			}).Error)
			setLimits := func(tokenID int, enabled bool, models string) {
				t.Helper()
				require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", tokenID).
					Updates(map[string]any{"model_limits_enabled": enabled, "model_limits": models}).Error)
			}
			setLimits(1, true, customerModel)
			setLimits(2, true, customerModel)

			created := seedanceCall(r, http.MethodPost, strings.TrimSuffix(nativePath, "/"),
				`{"model":"doubao-seedance-2-0-260128","content":[{"type":"text","text":"test"}],"duration":4}`, 1)
			require.Equal(t, http.StatusOK, created.Code, created.Body.String())
			require.Equal(t, officialID, gjson.Get(created.Body.String(), "id").String())
			task, exists, err := model.GetByTaskIDOrUpstreamID(1, officialID)
			require.NoError(t, err)
			require.True(t, exists)
			require.Equal(t, customerModel, task.Properties.OriginModelName)
			require.Equal(t, upstreamModel, task.Properties.UpstreamModelName)

			// Authorization uses the original model even when the provider receives a mapping.
			pending := seedanceCall(r, http.MethodGet, nativePath+officialID, "", 1)
			require.Equal(t, http.StatusOK, pending.Code, pending.Body.String())
			require.Equal(t, "running", gjson.Get(pending.Body.String(), "status").String())
			beforeQueries := queries.Load()
			foreign := seedanceCall(r, http.MethodGet, nativePath+officialID, "", 2)
			missing := seedanceCall(r, http.MethodGet, nativePath+"missing", "", 1)
			require.Equal(t, http.StatusBadRequest, foreign.Code, foreign.Body.String())
			require.Equal(t, foreign.Code, missing.Code)
			require.Equal(t, "task_not_exist", gjson.Get(foreign.Body.String(), "code").String())
			require.Equal(t, "task_not_exist", gjson.Get(missing.Body.String(), "code").String())

			// Neither an upstream-model whitelist nor a client-supplied model may override the task model.
			for _, limits := range []string{upstreamModel, "unrelated-model", ""} {
				setLimits(1, true, limits)
				for _, path := range []string{nativePath + officialID, "/v1/videos/" + task.TaskID, "/v1/video/generations/" + task.TaskID} {
					denied := seedanceCall(r, http.MethodGet, path+"?model="+upstreamModel, `{"model":"doubao-seedance-2-5-260628"}`, 1)
					require.Equal(t, http.StatusForbidden, denied.Code, denied.Body.String())
					require.Equal(t, "new_api_error", gjson.Get(denied.Body.String(), "error.type").String())
				}
			}
			require.Equal(t, beforeQueries, queries.Load(), "denied queries must never reach upstream")

			setLimits(1, true, customerModel)
			completed.Store(true)
			for _, id := range []string{officialID, supplierID, task.TaskID} {
				fetched := seedanceCall(r, http.MethodGet, nativePath+id, "", 1)
				require.Equal(t, http.StatusOK, fetched.Code, fetched.Body.String())
				require.Equal(t, "succeeded", gjson.Get(fetched.Body.String(), "status").String())
			}
			for _, path := range []string{"/v1/videos/" + task.TaskID, "/v1/video/generations/" + task.TaskID} {
				fetched := seedanceCall(r, http.MethodGet, path, "", 1)
				require.Equal(t, http.StatusOK, fetched.Code, fetched.Body.String())
			}
			// Ownership remains user-scoped, not tied to the creating Key.
			shared := seedanceCall(r, http.MethodGet, nativePath+officialID, "", 3)
			require.Equal(t, http.StatusOK, shared.Code, shared.Body.String())

			var before, after model.User
			var beforeToken, afterToken model.Token
			var beforeLogs, afterLogs int64
			require.NoError(t, model.DB.First(&before, 1).Error)
			require.NoError(t, model.DB.First(&beforeToken, 1).Error)
			require.NoError(t, model.LOG_DB.Model(&model.Log{}).Count(&beforeLogs).Error)
			beforeQueries = queries.Load()
			setLimits(1, true, upstreamModel)
			denied := seedanceCall(r, http.MethodGet, nativePath+officialID, "", 1)
			require.Equal(t, http.StatusForbidden, denied.Code, denied.Body.String())
			require.Equal(t, beforeQueries, queries.Load(), "completed tasks must also enforce model limits")
			setLimits(1, false, "")
			unrestricted := seedanceCall(r, http.MethodGet, nativePath+officialID, "", 1)
			require.Equal(t, http.StatusOK, unrestricted.Code, unrestricted.Body.String())
			setLimits(1, true, customerModel)
			repeated := seedanceCall(r, http.MethodGet, nativePath+officialID, "", 1)
			require.Equal(t, http.StatusOK, repeated.Code, repeated.Body.String())
			require.NoError(t, model.DB.First(&after, 1).Error)
			require.NoError(t, model.DB.First(&afterToken, 1).Error)
			require.NoError(t, model.LOG_DB.Model(&model.Log{}).Count(&afterLogs).Error)
			require.Equal(t, before.Quota, after.Quota)
			require.Equal(t, beforeToken.RemainQuota, afterToken.RemainQuota)
			require.Equal(t, beforeLogs, afterLogs, "repeat queries must not create billing logs")

			// Submission still checks the requested model before sending anything upstream.
			for _, path := range []string{strings.TrimSuffix(nativePath, "/"), "/v1/chat/completions"} {
				if path == "/v1/chat/completions" {
					r.POST(path, middleware.TokenAuth(), middleware.Distribute(), func(_ *gin.Context) {
						t.Error("forbidden chat model reached handler")
					})
				}
				denied := seedanceCall(r, http.MethodPost, path, `{"model":"unrelated-model"}`, 1)
				require.Equal(t, http.StatusForbidden, denied.Code, denied.Body.String())
			}
			require.EqualValues(t, 1, submissions.Load())

			beforeQueries = queries.Load()
			// Old records without a customer model cannot prove model permission.
			setLimits(1, true, customerModel+",")
			require.NoError(t, model.DB.Model(&model.Task{}).Where("id = ?", task.ID).
				Update("properties", model.Properties{UpstreamModelName: customerModel}).Error)
			unknownModel := seedanceCall(r, http.MethodGet, nativePath+officialID, "", 1)
			require.Equal(t, http.StatusForbidden, unknownModel.Code, unknownModel.Body.String())
			// An ambiguous native ID must not select an arbitrary authorized record.
			require.NoError(t, model.DB.Create(&model.Task{
				TaskID: "task_alias_collision", UserId: 1,
				Properties:  model.Properties{OriginModelName: customerModel},
				PrivateData: model.TaskPrivateData{OfficialTaskID: officialID},
			}).Error)
			ambiguous := seedanceCall(r, http.MethodGet, nativePath+officialID, "", 1)
			require.Equal(t, http.StatusInternalServerError, ambiguous.Code, ambiguous.Body.String())
			require.Equal(t, beforeQueries, queries.Load())
		})
	}
}

func TestSeedanceModelLimitRejectsRewrittenJimengQuery(t *testing.T) {
	const requestedModel = "doubao-seedance-2-5-260628"
	for _, acceptUnset := range []bool{false, true} {
		name := "default_pricing"
		if acceptUnset {
			name = "accept_unpriced_models"
		}
		t.Run(name, func(t *testing.T) {
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.WriteHeader(http.StatusMethodNotAllowed)
			}))
			defer upstream.Close()
			r := seedanceTestRouter(t, upstream.URL, "mock-key", requestedModel)
			// Same handler chain as the existing Jimeng Action route. Its adapter
			// changes relay_mode to fetch, but its handler remains RelayTask.
			r.POST("/jimeng/", middleware.JimengRequestConvert(), middleware.TokenAuth(), middleware.Distribute(), RelayTask)
			ch, err := model.GetChannelById(20, true)
			require.NoError(t, err)
			ch.SetSetting(dto.ChannelSettings{Protocol: &dto.ChannelProtocolSettings{ProfileID: "doubao-seedance-2"}})
			require.NoError(t, model.DB.Save(ch).Error)
			seedAssetOwnershipForTest(t, 20, 1, "asset", "owned-query-asset")
			require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", 1).
				Updates(map[string]any{"model_limits_enabled": true, "model_limits": "unrelated-model"}).Error)
			var before, after model.User
			require.NoError(t, model.DB.First(&before, 1).Error)
			settings := before.GetSetting()
			settings.AcceptUnsetRatioModel = acceptUnset
			before.SetSetting(settings)
			before.Quota = 1000000000
			require.NoError(t, model.DB.Save(&before).Error)
			var beforeToken, afterToken model.Token
			require.NoError(t, model.DB.First(&beforeToken, 1).Error)
			// An existing authorized task must not authorize a different model
			// supplied in the body if an adapter rewrites a submit handler to GET.
			require.NoError(t, model.DB.Create(&model.Task{TaskID: "owned-query-task", UserId: 1,
				Properties: model.Properties{OriginModelName: "unrelated-model"}}).Error)
			for _, id := range []string{"missing-query-task", "owned-query-task"} {
				body := `{"model":"doubao-seedance-2-5-260628","req_key":"doubao-seedance-2-5-260628","task_id":"` + id + `","prompt":"test","image_urls":["asset://owned-query-asset"]}`
				got := seedanceCall(r, http.MethodPost, "/jimeng/?Action=CVSync2AsyncGetResult&Version=2022-08-31", body, 1)
				require.Equal(t, http.StatusForbidden, got.Code, got.Body.String())
			}
			require.Zero(t, calls.Load(), "model rejection must precede any upstream request")
			require.NoError(t, model.DB.First(&after, 1).Error)
			require.NoError(t, model.DB.First(&afterToken, 1).Error)
			require.Equal(t, before.Quota, after.Quota)
			require.Equal(t, beforeToken.RemainQuota, afterToken.RemainQuota)
			var tasks, logs int64
			require.NoError(t, model.DB.Model(&model.Task{}).Count(&tasks).Error)
			require.EqualValues(t, 1, tasks)
			require.NoError(t, model.LOG_DB.Model(&model.Log{}).Count(&logs).Error)
			require.Zero(t, logs, "a denied query must not create consumption or refund logs")
		})
	}
}

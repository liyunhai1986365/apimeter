package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestSeedanceGenericSubmitPersistsOfficialID(t *testing.T) {
	for _, backend := range []string{"volcengine", "seedance-tgxmaas"} {
		for _, entry := range []string{"/v1/video/generations", "/v1/videos"} {
			t.Run(backend+entry, func(t *testing.T) {
				const supplierID = "task_supplier"
				const officialID = "cgt-official"
				var phase, queries, submissions atomic.Int32
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					if r.Method == http.MethodPost {
						submissions.Add(1)
						_, _ = w.Write([]byte(`{"id":"task_supplier","upstream_task_id":"cgt-official"}`))
						return
					}
					queries.Add(1)
					assert.Equal(t, http.MethodGet, r.Method)
					assert.True(t, strings.HasSuffix(r.URL.Path, "/contents/generations/tasks/"+supplierID), "must query using supplier ID: %s", r.URL.Path)
					switch phase.Load() {
					case 0:
						_, _ = w.Write([]byte(`{"id":"task_supplier","upstream_task_id":"cgt-official"}`))
					case 1:
						_, _ = w.Write([]byte(`{"id":"task_supplier","upstream_task_id":"cgt-wrong","status":"succeeded","content":{"video_url":"https://cdn.example/wrong.mp4"}}`))
					default:
						_, _ = w.Write([]byte(`{"id":"task_supplier","upstream_task_id":"cgt-official","status":"succeeded","content":{"video_url":"https://cdn.example/video.mp4"},"usage":{"completion_tokens":100,"total_tokens":100}}`))
					}
				}))
				defer upstream.Close()
				r := seedanceTestRouter(t, upstream.URL, "mock-key", "doubao-seedance-2-5-260628")
				if backend == "seedance-tgxmaas" {
					ch, err := model.GetChannelById(20, true)
					require.NoError(t, err)
					ch.Type = constant.ChannelTypeConfigurable
					ch.SetSetting(dto.ChannelSettings{Protocol: &dto.ChannelProtocolSettings{ProfileID: backend}})
					require.NoError(t, model.DB.Save(ch).Error)
				}
				r.POST(entry, middleware.TokenAuth(), middleware.Distribute(), RelayTask)
				created := seedanceCall(r, http.MethodPost, entry, `{"model":"doubao-seedance-2-5-260628","prompt":"mock video","duration":4,"metadata":{"upstream_task_id":"cgt-client-spoof"}}`, 1)
				require.Equal(t, http.StatusOK, created.Code, created.Body.String())
				localID := gjson.Get(created.Body.String(), "id").String()
				require.True(t, strings.HasPrefix(localID, "task_"), created.Body.String())
				require.NotEqual(t, supplierID, localID)
				task, exists, err := model.GetByTaskIDOrUpstreamID(1, localID)
				require.NoError(t, err)
				require.True(t, exists)
				require.Equal(t, supplierID, task.GetUpstreamTaskID())
				require.Equal(t, officialID, task.PrivateData.OfficialTaskID)
				lookup, exists, err := model.GetByTaskIDOrUpstreamID(1, officialID)
				require.NoError(t, err)
				require.True(t, exists)
				require.Equal(t, task.ID, lookup.ID)
				const queryPath = "/api/v3/contents/generations/tasks/"
				for _, id := range []string{localID, officialID} {
					denied := seedanceCall(r, http.MethodGet, queryPath+id, "", 2)
					require.NotEqual(t, http.StatusOK, denied.Code)
				}
				require.Zero(t, queries.Load(), "other users must not reach upstream")
				var before, after model.User
				require.NoError(t, model.DB.First(&before, 1).Error)
				for _, id := range []string{localID, officialID} {
					pending := seedanceCall(r, http.MethodGet, queryPath+id, "", 1)
					require.Equal(t, http.StatusServiceUnavailable, pending.Code, pending.Body.String())
					require.Equal(t, "task_status_pending", gjson.Get(pending.Body.String(), "code").String())
					require.Equal(t, "2", pending.Header().Get("Retry-After"))
				}
				phase.Store(1)
				wrong := seedanceCall(r, http.MethodGet, queryPath+officialID, "", 1)
				require.Equal(t, http.StatusBadGateway, wrong.Code, wrong.Body.String())
				var saved model.Task
				require.NoError(t, model.DB.First(&saved, task.ID).Error)
				require.Equal(t, task.Status, saved.Status, "pending and mismatched results must not settle")
				require.NoError(t, model.DB.First(&after, 1).Error)
				require.Equal(t, before.Quota, after.Quota)
				phase.Store(2)
				success := seedanceCall(r, http.MethodGet, queryPath+officialID, "", 1)
				require.Equal(t, http.StatusOK, success.Code, success.Body.String())
				require.Equal(t, officialID, gjson.Get(success.Body.String(), "id").String())
				require.Equal(t, "succeeded", gjson.Get(success.Body.String(), "status").String())
				require.NoError(t, model.DB.First(&saved, task.ID).Error)
				require.Equal(t, model.TaskStatus(model.TaskStatusSuccess), saved.Status)
				require.NoError(t, model.DB.First(&before, 1).Error)
				for _, id := range []string{localID, officialID, supplierID} {
					repeat := seedanceCall(r, http.MethodGet, queryPath+id, "", 1)
					require.Equal(t, http.StatusOK, repeat.Code, repeat.Body.String())
					require.Equal(t, officialID, gjson.Get(repeat.Body.String(), "id").String())
				}
				require.NoError(t, model.DB.First(&after, 1).Error)
				require.Equal(t, before.Quota, after.Quota, "repeated queries must not charge again")
				require.EqualValues(t, 1, submissions.Load())
				require.EqualValues(t, 7, queries.Load())
			})
		}
	}
}

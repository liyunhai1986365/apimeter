package relay

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestImageTaskFetchAfterTimeoutShowsRefundedFailure(t *testing.T) {
	_, _, task := setupImageReservationRace(t, false)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"id":"up","state":"running","progress":30,"data":{"images":[{"url":"https://example.invalid/unfinished.png"}]}}`))
	}))
	t.Cleanup(upstream.Close)
	require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", task.ChannelId).Update("base_url", upstream.URL).Error)
	task.PrivateData.UpstreamTaskID = "up"
	task.SubmitTime = time.Now().Add(-time.Hour).Unix()
	won, err := task.UpdateWithStatus(task.Status)
	require.NoError(t, err)
	require.True(t, won)
	limit, timeout := constant.TaskQueryLimit, constant.TaskTimeoutMinutes
	constant.TaskQueryLimit, constant.TaskTimeoutMinutes = 10, 1
	t.Cleanup(func() { constant.TaskQueryLimit, constant.TaskTimeoutMinutes = limit, timeout })
	service.RunTaskPollingOnce(context.Background(), func(int, int) {})
	var fresh model.Task
	require.NoError(t, model.DB.First(&fresh, task.ID).Error)
	require.Equal(t, model.TaskStatus(model.TaskStatusFailure), fresh.Status)
	require.NotEmpty(t, fresh.FailReason)
	require.Zero(t, fresh.Quota)
	require.Equal(t, "running", gjson.GetBytes(fresh.Data, "state").String())
	var user model.User
	require.NoError(t, model.DB.First(&user, task.UserId).Error)
	require.Equal(t, 100000, user.Quota)
	r := gin.New()
	r.GET("/v1/tasks/:id", func(c *gin.Context) {
		c.Set("id", task.UserId)
		if e := ImageTaskFetch(c); e != nil {
			c.JSON(e.StatusCode, e)
		}
	})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/v1/tasks/"+task.TaskID, nil))
	require.Equal(t, http.StatusOK, w.Code)
	result := gjson.ParseBytes(w.Body.Bytes())
	require.Equal(t, task.TaskID, result.Get("id").String())
	require.Equal(t, "failed", result.Get("state").String())
	require.EqualValues(t, 100, result.Get("progress").Int())
	require.Equal(t, fresh.FailReason, result.Get("error.message").String())
	require.Empty(t, result.Get("data.images").Array())
	require.NotContains(t, w.Body.String(), "unfinished.png")
}

func TestStoredImageTaskStatusCompatibility(t *testing.T) {
	for _, tt := range []struct {
		name                               string
		status                             model.TaskStatus
		body, wantState, wantURL, wantCode string
	}{
		{"upstream failure detail", model.TaskStatusFailure, `{"state":"error","error":{"code":"render_failed","message":"upstream detail"}}`, "failed", "", "render_failed"},
		{"expired failure", model.TaskStatusFailure, `{"state":"running","data":{"images":[{"url":"https://example.invalid/stale.png"}]}}`, "failed", "", ""},
		{"success", model.TaskStatusSuccess, `{"state":"succeeded","data":{"images":[{"url":"https://example.invalid/result.png"}]}}`, "succeeded", "https://example.invalid/result.png", ""},
		{"success without state", model.TaskStatusSuccess, `{"data":{"images":[{"url":"https://example.invalid/result.png"}]}}`, "succeeded", "https://example.invalid/result.png", ""},
		{"nonterminal", model.TaskStatusInProgress, `{"state":"running","progress":30}`, "running", "", ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			task := &model.Task{TaskID: "local", Status: tt.status, Data: []byte(tt.body), Progress: "100%", PrivateData: model.TaskPrivateData{UpstreamTaskID: "up"}}
			if tt.status == model.TaskStatusFailure {
				task.FailReason = "persisted reason"
			}
			if tt.name == "expired failure" {
				task.ImageBase64State = model.TaskImageCleared
			}
			body, err := storedImageTaskResponseBytes(task)
			require.NoError(t, err)
			result := gjson.ParseBytes(body)
			require.Equal(t, "local", result.Get("id").String())
			require.Equal(t, tt.wantState, result.Get("state").String())
			require.Equal(t, tt.wantURL, result.Get("data.images.0.url").String())
			require.Equal(t, tt.wantCode, result.Get("error.code").String())
			if tt.status == model.TaskStatusFailure {
				require.Equal(t, task.FailReason, result.Get("error.message").String())
			}
			if tt.status == model.TaskStatusInProgress {
				require.EqualValues(t, 30, result.Get("progress").Int())
			}
		})
	}
}

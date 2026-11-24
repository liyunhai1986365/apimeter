package relay

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"gorm.io/gorm"
)

func TestImageFetchRejectsBilledVideoWithoutSideEffects(t *testing.T) {
	_, _, task := setupImageReservationRace(t, false)
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_, _ = w.Write([]byte(`{"state":"failed"}`))
	}))
	t.Cleanup(upstream.Close)
	require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", task.ChannelId).Update("base_url", upstream.URL).Error)
	task.PrivateData.AsyncImage = false
	task.PrivateData.UpstreamTaskID = "upstream-video"
	task.PrivateData.BillingContext.OriginModelName = "sora"
	task.Properties = model.Properties{OriginModelName: "sora"}
	task.Platform = constant.TaskPlatform("1")
	task.Data = []byte(`{"video":"unchanged"}`)
	require.NotZero(t, task.Quota)
	var startUser model.User
	var startToken model.Token
	require.NoError(t, model.DB.First(&startUser, task.UserId).Error)
	require.NoError(t, model.DB.First(&startToken, task.PrivateData.TokenId).Error)
	for _, status := range []model.TaskStatus{model.TaskStatusInProgress, model.TaskStatusSuccess, model.TaskStatusFailure} {
		task.Status = status
		require.NoError(t, model.DB.Save(task).Error)
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Set("id", task.UserId)
		c.Params = gin.Params{{Key: "id", Value: task.TaskID}}
		c.Request = httptest.NewRequest(http.MethodGet, "/v1/tasks/"+task.TaskID, nil)
		e := ImageTaskFetch(c)
		require.NotNil(t, e)
		require.Equal(t, "task_not_exist", e.Code)
		require.Equal(t, http.StatusBadRequest, e.StatusCode)
		var saved model.Task
		require.NoError(t, model.DB.First(&saved, task.ID).Error)
		require.Equal(t, status, saved.Status)
		require.Equal(t, task.Quota, saved.Quota)
		require.JSONEq(t, string(task.Data), string(saved.Data))
		require.Zero(t, calls.Load())
	}
	var user model.User
	var token model.Token
	require.NoError(t, model.DB.First(&user, task.UserId).Error)
	require.NoError(t, model.DB.First(&token, task.PrivateData.TokenId).Error)
	require.Equal(t, startUser.Quota, user.Quota)
	require.Equal(t, startToken.RemainQuota, token.RemainQuota)
}

func TestImageSettlementFetchRequiresCommittedBalances(t *testing.T) {
	for _, source := range []string{"local", "upstream"} {
		t.Run(source, func(t *testing.T) {
			c, info, task := setupImageReservationRace(t, false)
			const publicURL = "https://images.invalid/output"
			const output = `{"data":[{"url":"https://images.invalid/output"},{"url":"https://images.invalid/output"}]}`
			var upstreamCalls atomic.Int32
			if source == "upstream" {
				task.PrivateData.UpstreamTaskID = "upstream"
				won, err := task.UpdateWithStatus(task.Status)
				require.NoError(t, err)
				require.True(t, won)
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					upstreamCalls.Add(1)
					_, _ = w.Write([]byte(`{"id":"upstream","data":{"images":[{"url":"https://images.invalid/output"},{"url":"https://images.invalid/output"}]}}`))
				}))
				t.Cleanup(upstream.Close)
				require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", task.ChannelId).Update("base_url", upstream.URL).Error)
			}
			var down, failed atomic.Bool
			down.Store(true)
			const hook = "test:fail_image_fetch_settlement"
			require.NoError(t, model.DB.Callback().Update().Before("gorm:update").Register(hook, func(tx *gorm.DB) {
				if tx.Statement.Table == "tokens" && down.Load() {
					failed.Store(true)
					tx.AddError(errors.New("token update unavailable"))
				}
			}))
			t.Cleanup(func() { require.NoError(t, model.DB.Callback().Update().Remove(hook)) })
			if source == "local" {
				finishLocalImageAsyncTask(c, info, task, []byte(output), nil)
			}
			router := gin.New()
			router.GET("/v1/tasks/:id", func(c *gin.Context) {
				c.Set("id", task.UserId)
				if err := ImageTaskFetch(c); err != nil {
					c.JSON(err.StatusCode, err)
				}
			})
			get := func() gjson.Result {
				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/tasks/"+task.TaskID, nil))
				require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
				return gjson.Parse(rec.Body.String())
			}
			pending := get()
			require.True(t, failed.Load())
			require.Equal(t, "processing", pending.Get("state").String())
			require.Less(t, pending.Get("progress").Int(), int64(100))
			require.Empty(t, pending.Get("data.images").Array())
			require.NotContains(t, pending.Raw, publicURL)
			down.Store(false)
			done := get()
			require.Equal(t, "succeeded", done.Get("state").String())
			require.Equal(t, task.TaskID, done.Get("id").String())
			require.Len(t, done.Get("data.images").Array(), 2)
			require.Equal(t, publicURL, done.Get("data.images.0.url").String())
			if source == "upstream" {
				require.EqualValues(t, 1, upstreamCalls.Load())
			}
			var user model.User
			var token model.Token
			require.NoError(t, model.DB.First(&user, task.UserId).Error)
			require.NoError(t, model.DB.First(&token, task.PrivateData.TokenId).Error)
			require.Equal(t, 80000, user.Quota)
			require.Equal(t, 80000, token.RemainQuota)
		})
	}
}

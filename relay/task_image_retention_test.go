package relay

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestExpiredImageTaskFetchSkipsUpstreamAndKeepsUsage(t *testing.T) {
	setupImageTaskTestDB(t)
	// An invalid channel proves this path never looks up an upstream or R2.
	task := model.Task{TaskID: "expired-image-api", UserId: 77, ChannelId: 987654, Status: model.TaskStatusSuccess,
		FinishTime: common.GetTimestamp() - 90000,
		Data:       []byte(`{"id":"upstream-id","state":"succeeded","progress":100,"usage":{"tokens":9007199254740993},"data":{"images":[{"b64_json":"PRIVATE_IMAGE"}]}}`)}
	require.NoError(t, model.DB.Create(&task).Error)
	r := gin.New()
	r.GET("/v1/tasks/:id", func(c *gin.Context) {
		c.Set("id", 77)
		require.Nil(t, ImageTaskFetch(c))
	})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/v1/tasks/expired-image-api", nil))
	require.Equal(t, 200, w.Code)
	require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	for _, value := range []string{`"state":"succeeded"`, `"image_status":"expired"`, `"image_expired":true`, `9007199254740993`, `"id":"expired-image-api"`} {
		require.Contains(t, w.Body.String(), value)
	}
	require.NotContains(t, w.Body.String(), "PRIVATE_IMAGE")
	var saved model.Task
	require.NoError(t, model.DB.First(&saved, task.ID).Error)
	require.Contains(t, string(saved.Data), "PRIVATE_IMAGE")
	// Merely fetching must not clean, settle, or rewrite the task.
	require.Zero(t, saved.ImageBase64Version)
}

func TestTaskDTOExpiresMediaWithoutLosingMetadata(t *testing.T) {
	task := &model.Task{TaskID: "dto-image", Status: model.TaskStatusSuccess, Quota: 123,
		ImageBase64State: model.TaskImagePresent, ImageExpiresAt: common.GetTimestamp() - 1,
		Data: []byte(`{"data":{"images":[{"b64_json":"SECRET"}]}}`)}
	result := TaskModel2Dto(task)
	require.Equal(t, "expired", result.ImageStatus)
	require.Equal(t, 123, result.Quota)
	require.NotContains(t, string(result.Data), "SECRET")
	require.Contains(t, string(task.Data), "SECRET")
	task.Data = nil // the list must expose expiry without fetching Data
	result = TaskModel2Dto(task)
	require.Equal(t, "expired", result.ImageStatus)
	require.Nil(t, result.Data)
}

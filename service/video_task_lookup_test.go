package service

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestVideoTaskLookupPreservesAuthorizedSnapshotAndScope(t *testing.T) {
	db := openChannelSelectTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Task{}))
	owner := &model.Task{UserId: 1, TaskID: "task_owner", Properties: model.Properties{OriginModelName: "authorized-model"},
		PrivateData: model.TaskPrivateData{UpstreamTaskID: "cgt-shared"}}
	foreign := &model.Task{UserId: 2, TaskID: "task_foreign", Properties: model.Properties{OriginModelName: "foreign-model"},
		PrivateData: model.TaskPrivateData{UpstreamTaskID: "cgt-shared"}}
	require.NoError(t, db.Create(owner).Error)
	require.NoError(t, db.Create(foreign).Error)
	newContext := func() *gin.Context {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Set("id", 1)
		c.Params = gin.Params{{Key: "task_id", Value: "cgt-shared"}}
		c.Request = httptest.NewRequest(http.MethodGet, "/api/v3/contents/generations/tasks/cgt-shared", nil)
		return c
	}
	c := newContext()
	authorized, exists, err := LookupVideoTask(c)
	require.NoError(t, err)
	require.True(t, exists)
	require.Equal(t, owner.ID, authorized.ID)
	// A record changing between authorization and result handling must not
	// replace the model/task snapshot that this request was authorized against.
	require.NoError(t, db.Model(&model.Task{}).Where("id = ?", owner.ID).
		Update("properties", model.Properties{OriginModelName: "changed-model"}).Error)
	fetched, exists, err := LookupVideoTask(c)
	require.NoError(t, err)
	require.True(t, exists)
	require.Equal(t, "authorized-model", fetched.Properties.OriginModelName)
	nextRequest, exists, err := LookupVideoTask(newContext())
	require.NoError(t, err)
	require.True(t, exists)
	require.Equal(t, "changed-model", nextRequest.Properties.OriginModelName)

	// Even if a caller reuses a Gin context, cached aliases cannot change the
	// user boundary or make a native supplier ID valid on a generic endpoint.
	c.Set("id", 2)
	fetched, exists, err = LookupVideoTask(c)
	require.NoError(t, err)
	require.True(t, exists)
	require.Equal(t, foreign.ID, fetched.ID)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/videos/cgt-shared", nil)
	_, exists, err = LookupVideoTask(c)
	require.NoError(t, err)
	require.False(t, exists)
	c.Params[0].Value = "task_owner"
	_, exists, err = LookupVideoTask(c)
	require.NoError(t, err)
	require.False(t, exists)
	c.Params = nil
	c.Set("task_id", "task_foreign")
	fetched, exists, err = LookupVideoTask(c)
	require.NoError(t, err)
	require.True(t, exists)
	require.Equal(t, foreign.ID, fetched.ID)
}

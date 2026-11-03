package controller

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/stretchr/testify/require"
)

func TestYHVideoContentUsesOwnedTaskAndStoredKey(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		require.Equal(t, "/v1/videos/cgt-owned/content", r.URL.Path)
		require.Equal(t, "Bearer original-task-key", r.Header.Get("Authorization"))
		require.Equal(t, "bytes=99-100", r.Header.Get("Range"))
		w.Header().Set("Content-Range", "bytes */10")
		w.Header().Set("Set-Cookie", "private=1")
		w.WriteHeader(416)
	}))
	defer upstream.Close()
	r := yhAssetTestRouter(t, upstream.URL+"/v1")
	r.GET("/v1/videos/:task_id/content", middleware.TokenAuth(), VideoProxy)
	setting := system_setting.GetFetchSetting()
	old := *setting
	setting.EnableSSRFProtection = false
	t.Cleanup(func() { *setting = old })
	require.NoError(t, model.DB.Create(&model.Task{TaskID: "task_gateway", UserId: 1, ChannelId: 20, Status: model.TaskStatusSuccess, PrivateData: model.TaskPrivateData{Key: "original-task-key", UpstreamTaskID: "cgt-owned"}}).Error)
	for _, id := range []string{"task_gateway", "cgt-owned"} {
		req := httptest.NewRequest("GET", "/v1/videos/"+id+"/content", nil)
		req.Header.Set("Authorization", "Bearer seedancetest1")
		req.Header.Set("Range", "bytes=99-100")
		out := httptest.NewRecorder()
		r.ServeHTTP(out, req)
		require.Equal(t, 416, out.Code, out.Body.String())
		require.Equal(t, "bytes */10", out.Header().Get("Content-Range"))
		require.Equal(t, "private, no-store", out.Header().Get("Cache-Control"))
		require.Empty(t, out.Header().Get("Set-Cookie"))
	}
	require.Equal(t, 404, seedanceCall(r, "GET", "/v1/videos/cgt-owned/content", "", 2).Code)
	require.EqualValues(t, 2, calls.Load())
}

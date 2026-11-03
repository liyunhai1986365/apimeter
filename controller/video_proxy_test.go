package controller

import (
	"net/http"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestVideoProxyGatewayLookupAndNativeFallback(t *testing.T) {
	r := tgxMaasResourceTestRouter(t, "https://example.test", "test-key", tgxRegressionModel)
	r.GET("/v1/videos/:task_id/content", middleware.TokenAuth(), VideoProxy)
	db := model.DB
	tasks := []model.Task{
		{TaskID: "task_gateway", UserId: 1, ChannelId: 20, Status: model.TaskStatusSuccess, PrivateData: model.TaskPrivateData{UpstreamTaskID: "native-owned", OfficialTaskID: "official-owned", ResultURL: "data:video/mp4;base64,YWJj"}},
		{TaskID: "task_foreign", UserId: 2, ChannelId: 20, Status: model.TaskStatusSuccess, PrivateData: model.TaskPrivateData{UpstreamTaskID: "native-foreign"}},
		{TaskID: "task_ambiguous_a", UserId: 1, ChannelId: 20, PrivateData: model.TaskPrivateData{UpstreamTaskID: "native-ambiguous"}},
		{TaskID: "task_ambiguous_b", UserId: 1, ChannelId: 20, PrivateData: model.TaskPrivateData{OfficialTaskID: "native-ambiguous"}},
		{TaskID: "task_alias_collision", UserId: 1, ChannelId: 20, PrivateData: model.TaskPrivateData{UpstreamTaskID: "task_gateway"}},
	}
	require.NoError(t, db.Create(&tasks).Error)
	var taskQueries []string
	require.NoError(t, db.Callback().Query().After("gorm:query").Register("test:video_content_lookup", func(tx *gorm.DB) {
		if tx.Statement.Table == "tasks" {
			taskQueries = append(taskQueries, tx.Statement.SQL.String())
		}
	}))
	t.Cleanup(func() { db.Callback().Query().Remove("test:video_content_lookup") })
	for _, tc := range []struct {
		id     string
		status int
	}{
		{"task_gateway", http.StatusOK},
		{"native-owned", http.StatusOK},
		{"official-owned", http.StatusOK},
		{"task_foreign", http.StatusNotFound},
		{"native-foreign", http.StatusNotFound},
		{"missing", http.StatusNotFound},
		{"native-ambiguous", http.StatusInternalServerError},
	} {
		t.Run(tc.id, func(t *testing.T) {
			taskQueries = nil
			out := seedanceCall(r, http.MethodGet, "/v1/videos/"+tc.id+"/content", "", 1)
			require.Equal(t, tc.status, out.Code, out.Body.String())
			if tc.status == http.StatusOK {
				require.Equal(t, "abc", out.Body.String())
			}
			if tc.id == "task_gateway" {
				// Avoid reintroducing a JSON scan of the user's entire task history.
				require.Len(t, taskQueries, 1)
				require.NotContains(t, strings.ToLower(taskQueries[0]), "json_extract")
			}
		})
	}
}

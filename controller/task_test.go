package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestTaskListDTOExcludesFullResults(t *testing.T) {
	data, err := common.Marshal(map[string]string{"b64_json": strings.Repeat("A", 1024*1024)})
	require.NoError(t, err)
	task := &model.Task{TaskID: "task-list", Data: data, PrivateData: model.TaskPrivateData{Key: "private-key", ResultURL: "data:image/png;base64,huge"}}
	items := taskListToDto([]*model.Task{task}, false)
	body, err := common.Marshal(items)
	require.NoError(t, err)
	require.Less(t, len(body), 2000)
	require.Contains(t, string(body), `"data_omitted":true`)
	require.NotContains(t, string(body), "private-key")
	require.NotContains(t, string(body), "base64")
	require.Equal(t, []byte(data), []byte(task.Data))
}

func TestUserTaskDetailRequiresScopeAndHidesPrivateData(t *testing.T) {
	originalDB := model.DB
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:task-detail-%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	t.Cleanup(func() { model.DB = originalDB; sqlDB, _ := db.DB(); _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&model.Task{}))
	data, err := common.Marshal(map[string]any{"images": []string{"saved-result"}})
	require.NoError(t, err)
	require.NoError(t, db.Create(&model.Task{TaskID: "task-owned", UserId: 1001, ChannelId: 9, Data: data,
		PrivateData: model.TaskPrivateData{Key: "private-api-key", ResultURL: "https://example.com/result.png"}}).Error)
	for _, tc := range []struct {
		name    string
		owner   int
		scope   bool
		status  int
		success bool
	}{
		{"owner", 1001, true, 200, true},
		{"other-owner", 2002, true, 404, false},
		{"missing-scope", 0, false, 200, false},
		{"invalid-owner", 0, true, 200, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := gin.New()
			r.GET("/self/:task_id", func(c *gin.Context) {
				if tc.scope {
					c.Set("workspace_access_scope", &service.WorkspaceAccessScope{OwnerUserId: tc.owner})
				}
				GetUserTaskDetail(c)
			})
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/self/task-owned", nil))
			require.Equal(t, tc.status, w.Code)
			var response struct {
				Success bool `json:"success"`
			}
			require.NoError(t, common.Unmarshal(w.Body.Bytes(), &response))
			require.Equal(t, tc.success, response.Success)
			require.NotContains(t, w.Body.String(), "private-api-key")
			if tc.success {
				require.Contains(t, w.Body.String(), "saved-result")
				require.Contains(t, w.Body.String(), `"channel_id":0`)
				require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
			} else {
				require.NotContains(t, w.Body.String(), "saved-result")
			}
		})
	}
}

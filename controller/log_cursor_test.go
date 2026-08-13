package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGetAllLogsCursorResponseOmitsExactTotal(t *testing.T) {
	db := openRelayRetryEventTestDB(t)
	require.NoError(t, db.Create(&[]model.Log{
		{Id: 1, UserId: 1001, Type: model.LogTypeError, CreatedAt: 100, RequestId: "req-success", Content: "recovered error"},
		{Id: 2, UserId: 1001, Type: model.LogTypeConsume, CreatedAt: 101, RequestId: "req-success", Content: "retry success"},
		{Id: 3, UserId: 1001, Type: model.LogTypeSystem, CreatedAt: 102, Content: "system log"},
	}).Error)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Set("role", common.RoleRootUser)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/log?cursor_mode=1&page_size=1", nil)

	GetAllLogs(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.JSONEq(t, `{
		"success": true,
		"message": "",
		"data": {
			"items": [{
				"id": 3,
				"user_id": 1001,
				"created_at": 102,
				"type": 4,
				"content": "system log",
				"username": "",
				"token_name": "",
				"model_name": "",
				"quota": 0,
				"prompt_tokens": 0,
				"input_tokens": 0,
				"completion_tokens": 0,
				"cache_read_tokens": 0,
				"cache_write_tokens": 0,
				"use_time": 0,
				"is_stream": false,
				"channel": 0,
				"channel_name": "",
				"token_id": 0,
				"group": "",
				"ip": "",
				"other": ""
			}],
			"page": 1,
			"page_size": 1,
			"has_more": true,
			"next_cursor": 3
		}
	}`, recorder.Body.String())
	require.NotContains(t, recorder.Body.String(), `"total"`)
}

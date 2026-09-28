package service

import (
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

const videoTaskLookupKey = "video_task_lookup"

type videoTaskLookup struct {
	userID int
	taskID string
	native bool
	task   *model.Task
}

// LookupVideoTask shares a single user-scoped task snapshot between model
// authorization and result handling. The cache lives only in this request.
func LookupVideoTask(c *gin.Context) (*model.Task, bool, error) {
	userID := c.GetInt("id")
	taskID := c.Param("task_id")
	if taskID == "" {
		taskID = c.GetString("task_id")
	}
	native := IsVolcengineVideoTaskQueryRequest(c)
	if raw, ok := c.Get(videoTaskLookupKey); ok {
		if cached, ok := raw.(videoTaskLookup); ok && cached.userID == userID && cached.taskID == taskID && cached.native == native {
			return cached.task, true, nil
		}
	}
	lookup := model.GetByTaskId
	if native {
		lookup = model.GetByTaskIDOrUpstreamID
	}
	task, exists, err := lookup(userID, taskID)
	if err == nil && exists {
		c.Set(videoTaskLookupKey, videoTaskLookup{userID: userID, taskID: taskID, native: native, task: task})
	}
	return task, exists, err
}

func IsVolcengineVideoTaskQueryRequest(c *gin.Context) bool {
	return c != nil && c.Request != nil && c.Request.URL != nil &&
		c.Request.Method == http.MethodGet && strings.HasPrefix(c.Request.URL.Path, "/api/v3/contents/generations/tasks/")
}

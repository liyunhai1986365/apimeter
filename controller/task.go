package controller

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// UpdateTaskBulk 薄入口，实际轮询逻辑在 service 层
func UpdateTaskBulk() {
	service.TaskPollingLoop()
}

func GetAllTask(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)

	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	// 解析其他查询参数
	queryParams := model.SyncTaskQueryParams{
		Platform:       constant.TaskPlatform(c.Query("platform")),
		TaskID:         c.Query("task_id"),
		TokenName:      c.Query("token_name"),
		WorkspaceName:  c.Query("workspace_name"),
		Status:         c.Query("status"),
		Action:         c.Query("action"),
		StartTimestamp: startTimestamp,
		EndTimestamp:   endTimestamp,
		ChannelID:      c.Query("channel_id"),
	}

	items := model.TaskGetAllTasks(pageInfo.GetStartIdx(), pageInfo.GetPageSize(), queryParams)
	total := model.TaskCountAllTasks(queryParams)
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(taskListToDto(items, true))
	common.ApiSuccess(c, pageInfo)
}

func GetUserTask(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	scope, err := workspaceAccessScope(c)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)

	queryParams := model.SyncTaskQueryParams{
		Platform:            constant.TaskPlatform(c.Query("platform")),
		TaskID:              c.Query("task_id"),
		TokenName:           c.Query("token_name"),
		WorkspaceName:       c.Query("workspace_name"),
		Status:              c.Query("status"),
		Action:              c.Query("action"),
		StartTimestamp:      startTimestamp,
		EndTimestamp:        endTimestamp,
		ChannelID:           c.Query("channel_id"),
		AllowedWorkspaceIds: scope.WorkspaceFilter(),
	}

	items := model.TaskGetAllUserTask(scope.OwnerUserId, pageInfo.GetStartIdx(), pageInfo.GetPageSize(), queryParams)
	total := model.TaskCountAllUserTask(scope.OwnerUserId, queryParams)
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(taskListToDto(items, false))
	common.ApiSuccess(c, pageInfo)
}

type taskListItem struct {
	*dto.TaskDto
	DataOmitted bool `json:"data_omitted"`
}

func taskListToDto(tasks []*model.Task, fillUser bool) []taskListItem {
	items := tasksToDto(tasks, fillUser)
	result := make([]taskListItem, len(items))
	for i, item := range items {
		// Keep list serialization safe even if a future caller supplies full tasks.
		item.Data = nil
		item.ResultURL = ""
		result[i] = taskListItem{TaskDto: item, DataOmitted: true}
	}
	return result
}

func GetTaskDetail(c *gin.Context) {
	getTaskDetail(c, 0, nil)
}

func GetUserTaskDetail(c *gin.Context) {
	scope, err := workspaceAccessScope(c)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if scope.OwnerUserId <= 0 {
		common.ApiError(c, ErrWorkspaceScopeMissing)
		return
	}
	getTaskDetail(c, scope.OwnerUserId, scope.WorkspaceFilter())
}

func getTaskDetail(c *gin.Context, userId int, allowedWorkspaceIds []int) {
	task, err := model.GetTaskLogDetail(c.Request.Context(), c.Param("task_id"), userId, allowedWorkspaceIds)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Task not found"})
		return
	}
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	common.ApiSuccess(c, tasksToDto([]*model.Task{task}, userId == 0)[0])
}

func tasksToDto(tasks []*model.Task, fillUser bool) []*dto.TaskDto {
	var userIdMap map[int]*model.UserBase
	if fillUser {
		userIdMap = make(map[int]*model.UserBase)
		userIds := types.NewSet[int]()
		for _, task := range tasks {
			userIds.Add(task.UserId)
		}
		for _, userId := range userIds.Items() {
			cacheUser, err := model.GetUserCache(userId)
			if err == nil {
				userIdMap[userId] = cacheUser
			}
		}
	}
	result := make([]*dto.TaskDto, len(tasks))
	for i, task := range tasks {
		if fillUser {
			if user, ok := userIdMap[task.UserId]; ok {
				task.Username = user.Username
			}
		}
		result[i] = relay.TaskModel2Dto(task)
	}
	return result
}

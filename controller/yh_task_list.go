package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/configurable"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// Contract follows arkruntime ListContentGenerationTasks (SDK v1.2.52).
// Local polling supplies state; listing never imports supplier-account tasks
// or triggers the supplier's charge-on-detail operation.
func ListYHVideoTasks(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	query := c.Request.URL.Query()
	allowed := map[string]bool{"page_num": true, "page_size": true, "filter.status": true, "filter.model": true, "filter.service_tier": true, "filter.task_ids": true}
	for key, values := range query {
		if !allowed[key] || (key != "filter.task_ids" && len(values) != 1) {
			videoProxyError(c, 400, "invalid_request_error", "unsupported or repeated query parameter: "+key)
			return
		}
	}
	page, size := 1, 10
	for key, target := range map[string]*int{"page_num": &page, "page_size": &size} {
		if raw := query.Get(key); raw != "" {
			n, err := strconv.Atoi(raw)
			if err != nil || n < 1 || n > 1000000 {
				videoProxyError(c, 400, "invalid_request_error", "invalid "+key)
				return
			}
			*target = n
		}
	}
	if size > 100 || len(query["filter.task_ids"]) > 100 {
		videoProxyError(c, 400, "invalid_request_error", "page_size and task ID filters are limited to 100")
		return
	}
	status := query.Get("filter.status")
	if status != "" && status != "queued" && status != "running" && status != "succeeded" && status != "failed" && status != "expired" && status != "cancelled" {
		videoProxyError(c, 400, "invalid_request_error", "invalid filter.status")
		return
	}
	ids := map[string]bool{}
	for _, id := range query["filter.task_ids"] {
		if !validAssetHandle(id) {
			videoProxyError(c, 400, "invalid_request_error", "invalid task ID")
			return
		}
		ids[id] = true
	}
	items := make([]json.RawMessage, 0, size)
	total := 0
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	err := model.VisitYHUserTasks(ctx, common.GetContextKeyInt(c, constant.ContextKeyUserId), func(task *model.Task) error {
		if len(ids) > 0 && !ids[task.TaskID] && !ids[task.GetNativeTaskID()] {
			return nil
		}
		if name := query.Get("filter.model"); name != "" && name != task.Properties.OriginModelName {
			return nil
		}
		if common.GetContextKeyBool(c, constant.ContextKeyTokenModelLimitEnabled) {
			raw, _ := common.GetContextKey(c, constant.ContextKeyTokenModelLimit)
			limits, _ := raw.(map[string]bool)
			if _, ok := limits[ratio_setting.FormatMatchingModelName(task.Properties.OriginModelName)]; !ok {
				return nil
			}
		}
		if raw, ok := common.GetContextKey(c, constant.ContextKeyTokenSpecificChannelId); ok {
			selected, err := parseSpecificChannelID(raw)
			if err != nil || selected != task.ChannelId {
				return nil
			}
		}
		body, err := configurable.BuildYHVideoTaskListResponse(task)
		if err != nil {
			return fmt.Errorf("invalid stored video task: %w", err)
		}
		if status != "" && gjson.GetBytes(body, "status").String() != status {
			return nil
		}
		if tier := query.Get("filter.service_tier"); tier != "" && gjson.GetBytes(body, "service_tier").String() != tier {
			return nil
		}
		index := total
		total++
		if index < (page-1)*size || len(items) >= size {
			return nil
		}
		if failure := gjson.GetBytes(body, "error"); failure.Exists() {
			body, err = sjson.SetRawBytes(body, "failure_reason", []byte(failure.Raw))
			if err != nil {
				return err
			}
			body, err = sjson.DeleteBytes(body, "error")
			if err != nil {
				return err
			}
		}
		items = append(items, body)
		return nil
	})
	if err != nil {
		videoProxyError(c, http.StatusInternalServerError, "server_error", "failed to list local video tasks")
		return
	}
	c.JSON(http.StatusOK, gin.H{"total": total, "items": items})
}

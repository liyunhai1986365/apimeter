package model

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/pkg/taskimage"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

const (
	TaskImageUnknown = iota
	TaskImageNone
	TaskImagePresent
	TaskImageCleared
	TaskImageRetentionSeconds int64 = 24 * 60 * 60
)

var ErrTaskImageVersionConflict = errors.New("task result changed concurrently")

func (t *Task) imageTerminal() bool {
	return t.Status == TaskStatusSuccess || t.Status == TaskStatusFailure
}

func (t *Task) imageTaskHint() bool {
	name := strings.ToLower(t.Properties.OriginModelName + " " + t.Properties.UpstreamModelName)
	return t.PrivateData.AsyncImage || strings.Contains(name, "gpt-image-") || strings.Contains(name, "nano-banana") || (strings.Contains(name, "gemini-") && strings.Contains(name, "image"))
}

func (t *Task) imageLegacyCompletedAt(now int64) (int64, bool) {
	var payload struct {
		UpdateTime int64 `json:"update_time"`
	}
	_ = common.Unmarshal(t.Data, &payload)
	for _, stamp := range []int64{t.FinishTime, payload.UpdateTime, t.UpdatedAt} {
		if stamp > 0 && stamp <= now && (t.SubmitTime <= 0 || stamp >= t.SubmitTime) {
			return stamp, false
		}
	}
	return now, true
}

func (t *Task) prepareImageRetention(now int64, legacy bool) error {
	if !t.imageTerminal() {
		return nil
	}
	if t.ImageBase64State == TaskImageCleared {
		return t.stripExpiredImage(now)
	}
	if !taskimage.HasPayload(t.Data) && !taskimage.IsDataURI(t.PrivateData.ResultURL) {
		if t.ImageBase64State == TaskImageUnknown {
			t.ImageBase64State = TaskImageNone
		}
		return nil
	}
	_, summary, err := taskimage.Transform(t.Data, t.imageTaskHint(), false)
	if err != nil {
		return err
	}
	if taskimage.IsDataURI(t.PrivateData.ResultURL) {
		summary.Base64++
	}
	if summary.Base64 == 0 && summary.Expired == 0 {
		if t.ImageBase64State == TaskImageUnknown {
			t.ImageBase64State = TaskImageNone
		}
		return nil
	}
	if t.ImageExpiresAt == 0 {
		completed := now
		if legacy {
			completed, _ = t.imageLegacyCompletedAt(now)
		}
		t.ImageExpiresAt = completed + TaskImageRetentionSeconds
	}
	t.ImageHasURL = summary.URLs > 0
	if summary.Base64 == 0 && summary.Expired > 0 {
		t.ImageBase64State = TaskImageCleared
	} else {
		t.ImageBase64State = TaskImagePresent
	}
	// Normal writers must never reintroduce an expired payload. Historical
	// discovery only sets metadata; physical removal belongs to the cleanup job.
	if !legacy && t.ImageContentExpired(now) {
		return t.stripExpiredImage(now)
	}
	return nil
}

func (t *Task) ImageContentExpired(now int64) bool {
	return t.imageTerminal() && (t.ImageBase64State == TaskImageCleared || (t.ImageBase64State == TaskImagePresent && t.ImageExpiresAt > 0 && now >= t.ImageExpiresAt))
}

func (t *Task) ImageAvailability(now int64) dto.TaskImageAvailability {
	result := dto.TaskImageAvailability{}
	if t.Status != TaskStatusSuccess || (t.ImageBase64State != TaskImagePresent && t.ImageBase64State != TaskImageCleared) {
		return result
	}
	result.ImageExpiresAt = t.ImageExpiresAt
	result.ImageHasURL = t.ImageHasURL
	result.ImageStatus = "available"
	if t.ImageContentExpired(now) {
		result.ImageStatus, result.ImageMessage = "expired", "图片已过期"
		if t.ImageHasURL {
			result.ImageStatus, result.ImageMessage = "partially_expired", "部分图片已过期"
		}
	}
	return result
}

func (t *Task) stripExpiredImage(now int64) error {
	data, summary, err := taskimage.Transform(t.Data, t.imageTaskHint(), true)
	if err != nil {
		return err
	}
	t.Data = data
	if taskimage.IsDataURI(t.PrivateData.ResultURL) {
		t.PrivateData.ResultURL = ""
	}
	t.ImageBase64State = TaskImageCleared
	t.ImageHasURL = summary.URLs > 0
	if t.ImageBase64ClearedAt == 0 {
		t.ImageBase64ClearedAt = now
	}
	return nil
}

// ImageRetentionView never mutates the saved task and also protects reads before
// the scheduled physical cleanup. List records have no Data and use metadata.
func (t *Task) ImageRetentionView(now int64) (*Task, error) {
	view := *t
	if len(view.Data) > 0 || taskimage.IsDataURI(view.PrivateData.ResultURL) {
		if err := view.prepareImageRetention(now, true); err != nil {
			return nil, err
		}
		if view.ImageContentExpired(now) {
			if err := view.stripExpiredImage(now); err != nil {
				return nil, err
			}
		}
	}
	return &view, nil
}

func (t *Task) updateWithImageVersion(query *gorm.DB) (bool, error) {
	updated := *t
	if err := updated.prepareImageRetention(common.GetTimestamp(), false); err != nil {
		return false, err
	}
	// Image writes advance the revision even when the execution status stays
	// SUCCESS. Leave unrelated tasks' no-op update semantics unchanged.
	if updated.ImageBase64State == TaskImagePresent || updated.ImageBase64State == TaskImageCleared {
		updated.ImageBase64Version++
	}
	// Do not use Model(t): GORM assigns update fields into that pointer even
	// when the CAS affects zero rows, allowing a stale object's version to advance.
	result := query.Model(&Task{}).Where("image_base64_version = ?", t.ImageBase64Version).Select("*").Updates(&updated)
	if result.Error != nil {
		return false, result.Error
	}
	if result.RowsAffected > 0 {
		*t = updated
	}
	return result.RowsAffected > 0, nil
}

func taskBulkVersionParams(params map[string]any) (map[string]any, error) {
	out := make(map[string]any, len(params)+1)
	for k, v := range params {
		if k == "data" || k == "private_data" || strings.HasPrefix(k, "image_") {
			return nil, errors.New("task image payloads require a versioned task update")
		}
		out[k] = v
	}
	out["image_base64_version"] = gorm.Expr("image_base64_version + 1")
	return out, nil
}

// MaintainTaskImage performs one short, conditional maintenance update. It
// never writes status, quota, timestamps, usage or any associated table.
func MaintainTaskImage(ctx context.Context, id int64, now int64) (changed bool, removedBytes int64, estimatedTime bool, err error) {
	// Slow/error SQL logging would otherwise include the old image payload.
	db := DB.WithContext(ctx).Session(&gorm.Session{Logger: gormlogger.Discard})
	var task Task
	if err = db.First(&task, id).Error; err != nil {
		return
	}
	if !task.imageTerminal() {
		return
	}
	original := task
	if err = task.prepareImageRetention(now, true); err != nil {
		return
	}
	if task.ImageContentExpired(now) {
		if err = task.stripExpiredImage(now); err != nil {
			return
		}
	}
	if task.ImageBase64State == original.ImageBase64State && task.ImageExpiresAt == original.ImageExpiresAt && task.ImageBase64ClearedAt == original.ImageBase64ClearedAt {
		return
	}
	updates := map[string]any{
		"image_base64_state": task.ImageBase64State, "image_expires_at": task.ImageExpiresAt,
		"image_base64_cleared_at": task.ImageBase64ClearedAt, "image_has_url": task.ImageHasURL,
		"image_base64_version": original.ImageBase64Version + 1,
	}
	if task.ImageBase64State == TaskImageCleared {
		updates["data"] = task.Data
		removedBytes = int64(len(original.Data) - len(task.Data))
		if original.PrivateData.ResultURL != task.PrivateData.ResultURL {
			// Preserve unknown historical private fields as well as billing data.
			var private struct {
				Raw string `gorm:"column:private_data"`
			}
			if err = db.Model(&Task{}).Select("private_data").Where("id = ?", id).Take(&private).Error; err != nil {
				return
			}
			var values map[string]json.RawMessage
			if err = common.Unmarshal([]byte(private.Raw), &values); err != nil {
				return
			}
			delete(values, "result_url")
			var body []byte
			if body, err = common.Marshal(values); err != nil {
				return
			}
			updates["private_data"] = string(body)
			removedBytes += int64(len(private.Raw) - len(body))
		}
	}
	// UpdateColumns bypasses GORM's automatic updated_at callback. Image
	// maintenance must preserve the task's original business timestamps.
	result := db.Model(&Task{}).Where("id = ? AND status = ? AND image_base64_version = ? AND image_expires_at = ?", id, original.Status, original.ImageBase64Version, original.ImageExpiresAt).UpdateColumns(updates)
	err, changed = result.Error, result.RowsAffected > 0
	if changed && original.ImageExpiresAt == 0 && task.ImageExpiresAt > 0 {
		_, estimatedTime = original.imageLegacyCompletedAt(now)
	}
	if !changed || removedBytes < 0 {
		removedBytes = 0
	}
	return
}

type TaskImageCandidate struct {
	ID             int64
	ImageExpiresAt int64
}

func FindTaskImageCandidates(ctx context.Context, now int64, backfill bool, afterTime, afterID int64, limit int) ([]TaskImageCandidate, error) {
	query := DB.WithContext(ctx).Model(&Task{}).Select("id", "image_expires_at").Where("status IN ?", []string{TaskStatusSuccess, TaskStatusFailure})
	if backfill {
		// Unknown is the migration/default state and has no deadline. Binding
		// both index prefixes lets this walk IDs without sorting the old table.
		query = query.Where("image_base64_state = ? AND image_expires_at = 0 AND id > ?", TaskImageUnknown, afterID).Order("id")
	} else {
		query = query.Where("image_base64_state = ? AND image_expires_at > 0 AND image_expires_at <= ?", TaskImagePresent, now).
			Where("image_expires_at > ? OR (image_expires_at = ? AND id > ?)", afterTime, afterTime, afterID).
			Order("image_expires_at, id")
	}
	var rows []TaskImageCandidate
	err := query.Limit(limit).Find(&rows).Error
	return rows, err
}

type TaskImageBacklog struct {
	PendingExpired  int64 `json:"pending_expired"`
	PendingBackfill int64 `json:"pending_backfill"`
	OldestExpiresAt int64 `json:"oldest_expires_at"`
}

func GetTaskImageBacklog(ctx context.Context, now int64) (TaskImageBacklog, error) {
	result := TaskImageBacklog{}
	query := DB.WithContext(ctx).Model(&Task{}).Where("status IN ?", []string{TaskStatusSuccess, TaskStatusFailure})
	if err := query.Session(&gorm.Session{}).Where("image_base64_state = ? AND image_expires_at = 0", TaskImageUnknown).Count(&result.PendingBackfill).Error; err != nil {
		return result, err
	}
	var expired struct {
		Count  int64
		Oldest int64
	}
	err := query.Session(&gorm.Session{}).Select("COUNT(*) AS count, COALESCE(MIN(image_expires_at), 0) AS oldest").
		Where("image_base64_state = ? AND image_expires_at > 0 AND image_expires_at <= ?", TaskImagePresent, now).Scan(&expired).Error
	result.PendingExpired, result.OldestExpiresAt = expired.Count, expired.Oldest
	return result, err
}

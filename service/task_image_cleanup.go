package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
)

type taskImageCleanupHandler struct{}

func init()                                  { RegisterSystemTaskHandler(taskImageCleanupHandler{}) }
func (taskImageCleanupHandler) Type() string { return model.SystemTaskTypeTaskImageCleanup }
func (taskImageCleanupHandler) Enabled() bool {
	return common.GetEnvOrDefaultBool("TASK_IMAGE_BASE64_CLEANUP_ENABLED", false)
}
func (taskImageCleanupHandler) Interval() time.Duration { return time.Minute }
func (h taskImageCleanupHandler) NewPayload() any {
	state := TaskImageCleanupState{}
	previous, err := model.GetLatestSystemTask(h.Type())
	if err == nil && previous != nil {
		_ = common.UnmarshalJsonStr(previous.State, &state)
	}
	// Keep only cursors, not the previous run's counters.
	return TaskImageCleanupState{BackfillAfterID: state.BackfillAfterID, ExpiredAfterID: state.ExpiredAfterID, ExpiredAfterTime: state.ExpiredAfterTime}
}

type TaskImageCleanupState struct {
	model.TaskImageBacklog
	BackfillAfterID   int64 `json:"backfill_after_id"`
	ExpiredAfterID    int64 `json:"expired_after_id"`
	ExpiredAfterTime  int64 `json:"expired_after_time"`
	Scanned           int   `json:"scanned"`
	Updated           int   `json:"updated"`
	Failed            int   `json:"failed"`
	RemovedBytes      int64 `json:"removed_bytes"`
	EstimatedTimeRows int   `json:"estimated_time_rows"`
	DurationMS        int64 `json:"duration_ms"`
}

func (h taskImageCleanupHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	state := TaskImageCleanupState{}
	err := task.DecodePayload(&state)
	if err == nil {
		err = RunTaskImageCleanup(ctx, &state, common.GetTimestamp(), 1000, 45*time.Second, func() error {
			return model.UpdateSystemTaskState(task.TaskID, runnerID, state)
		})
	}
	if err == nil {
		statsCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		state.TaskImageBacklog, err = model.GetTaskImageBacklog(statsCtx, common.GetTimestamp())
		cancel()
	}
	status, message := model.SystemTaskStatusSucceeded, ""
	if err != nil {
		status, message = model.SystemTaskStatusFailed, err.Error()
	}
	if state.Failed > 0 && err == nil {
		status, message = model.SystemTaskStatusFailed, fmt.Sprintf("%d image records could not be processed; will retry after the cursor wraps", state.Failed)
	}
	_ = model.UpdateSystemTaskState(task.TaskID, runnerID, state)
	RecordSystemTaskExecution(h.Type(), string(status))
	if status == model.SystemTaskStatusSucceeded {
		RecordSystemTaskSuccess(h.Type(), float64(common.GetTimestamp()))
	} else {
		RecordSystemTaskFailure(h.Type())
	}
	if err := model.FinishSystemTask(task.TaskID, runnerID, status, state, message); err != nil {
		logger.LogWarn(ctx, fmt.Sprintf("image cleanup could not finish system task: %v", err))
	}
}

// RunTaskImageCleanup reads only candidate IDs in batches, then loads at most
// one payload at a time. Each phase gets half the budget so backfill cannot
// starve newly expired images. Persisted cursors prevent poison rows starving
// later records, and wrap to retry failed or concurrently skipped records.
func RunTaskImageCleanup(ctx context.Context, state *TaskImageCleanupState, now int64, maxRows int, maxDuration time.Duration, progress func() error) (runErr error) {
	start := time.Now()
	parent := ctx
	ctx, cancel := context.WithTimeout(ctx, maxDuration)
	defer cancel()
	defer func() {
		state.DurationMS = time.Since(start).Milliseconds()
		// Exhausting this run's budget is normal; lease loss or a caller's
		// cancellation remains a failure and stops further writes immediately.
		if errors.Is(runErr, context.DeadlineExceeded) && parent.Err() == nil {
			runErr = nil
		}
	}()
	for _, backfill := range []bool{false, true} {
		processed := 0
		for processed < maxRows/2 && time.Since(start) < maxDuration {
			if err := ctx.Err(); err != nil {
				return err
			}
			afterID, afterTime := state.ExpiredAfterID, state.ExpiredAfterTime
			if backfill {
				afterID, afterTime = state.BackfillAfterID, 0
			}
			limit := min(50, maxRows/2-processed)
			rows, err := model.FindTaskImageCandidates(ctx, now, backfill, afterTime, afterID, limit)
			if err != nil {
				return err
			}
			if len(rows) == 0 {
				if backfill {
					state.BackfillAfterID = 0
				} else {
					state.ExpiredAfterID, state.ExpiredAfterTime = 0, 0
				}
				break
			}
			for _, row := range rows {
				if time.Since(start) >= maxDuration {
					break
				}
				if err := ctx.Err(); err != nil {
					return err
				}
				changed, removed, estimatedTime, err := model.MaintainTaskImage(ctx, row.ID, now)
				if err != nil {
					if ctx.Err() != nil {
						return ctx.Err()
					}
					state.Failed++
					// Never log database errors containing parameters or image JSON.
					logger.LogWarn(ctx, fmt.Sprintf("image cleanup failed for task row %d", row.ID))
				} else if changed {
					state.Updated++
					state.RemovedBytes += removed
					if estimatedTime {
						state.EstimatedTimeRows++
					}
				}
				state.Scanned++
				processed++
				if backfill {
					state.BackfillAfterID = row.ID
				} else {
					state.ExpiredAfterID, state.ExpiredAfterTime = row.ID, row.ImageExpiresAt
				}
			}
			if progress != nil {
				if err := progress(); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

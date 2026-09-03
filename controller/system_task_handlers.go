package controller

import (
	"context"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/routing_strategy_setting"
)

// RegisterScheduledSystemTasks wires periodic maintenance, async polling, and
// monthly billing jobs into the system task framework so a DB lease dedups
// execution across multiple master instances and each run is recorded as one
// task row. Call this before service.StartSystemTaskRunner.
func RegisterScheduledSystemTasks() {
	// Register channel test handler (implemented)
	service.RegisterSystemTaskHandler(channelTestHandler{})

	// Register model update handler (implemented)
	service.RegisterSystemTaskHandler(modelUpdateHandler{})

	// Register async task polling handlers (implemented)
	service.RegisterSystemTaskHandler(midjourneyPollHandler{})
	service.RegisterSystemTaskHandler(asyncTaskPollHandler{})

	service.RegisterSystemTaskHandler(routingStrategyRefreshHandler{})
	service.RegisterSystemTaskHandler(monthlyBillingStatementHandler{})
}

// channelTestHandler runs the scheduled "test all channels" job. Enablement and
// cadence still come from the monitor settings; only the execution path moved
// into the system task runner.
type channelTestHandler struct{}

func (channelTestHandler) Type() string { return model.SystemTaskTypeChannelTest }

func (channelTestHandler) Enabled() bool {
	return operation_setting.GetMonitorSetting().AutoTestChannelEnabled
}

func (channelTestHandler) Interval() time.Duration {
	minutes := operation_setting.GetMonitorSetting().AutoTestChannelMinutes
	if minutes <= 0 {
		minutes = 10
	}
	return time.Duration(minutes * float64(time.Minute))
}

func (channelTestHandler) NewPayload() any { return nil }

// channelTestTaskPayload controls one channel_test run. A nil/empty payload is a
// scheduled run, which uses the configured monitor ChannelTestMode and does not
// notify. A manual "test all channels" trigger sets Mode=scheduled_all and
// Notify=true to reproduce the legacy manual behavior (test every channel and
// notify root on completion).
type channelTestTaskPayload struct {
	Mode   string `json:"mode,omitempty"`
	Notify bool   `json:"notify,omitempty"`
}

func (channelTestHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	payload := channelTestTaskPayload{}
	if err := task.DecodePayload(&payload); err != nil {
		finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusFailed, nil, err)
		return
	}
	summary, err := runChannelTestTask(ctx, payload.Mode, payload.Notify, service.NewSystemTaskProgressReporter(task, runnerID))
	if err != nil {
		finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusFailed, nil, err)
		return
	}
	finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusSucceeded, summary, nil)
}

func finishSystemTaskHandler(task *model.SystemTask, runnerID string, status model.SystemTaskStatus, result any, runErr error) {
	errorMessage := ""
	if runErr != nil {
		errorMessage = runErr.Error()
	}

	// Record execution metrics
	statusStr := string(status)
	service.RecordSystemTaskExecution(task.Type, statusStr)

	// Record success timestamp for succeeded tasks
	if status == model.SystemTaskStatusSucceeded {
		service.RecordSystemTaskSuccess(task.Type, float64(common.GetTimestamp()))
	} else if status == model.SystemTaskStatusFailed {
		service.RecordSystemTaskFailure(task.Type)
	}

	if err := model.FinishSystemTask(task.TaskID, runnerID, status, result, errorMessage); err != nil {
		common.SysLog(fmt.Sprintf("system task %s failed to persist result: %v", task.TaskID, err))
	}
}

// modelUpdateHandler runs the scheduled upstream model update detection job.
type modelUpdateHandler struct{}

func (modelUpdateHandler) Type() string { return model.SystemTaskTypeModelUpdate }

func (modelUpdateHandler) Enabled() bool {
	return common.GetEnvOrDefaultBool("CHANNEL_UPSTREAM_MODEL_UPDATE_TASK_ENABLED", true)
}

func (modelUpdateHandler) Interval() time.Duration {
	intervalMinutes := common.GetEnvOrDefault(
		"CHANNEL_UPSTREAM_MODEL_UPDATE_TASK_INTERVAL_MINUTES",
		30, // default 30 minutes
	)
	if intervalMinutes < 1 {
		intervalMinutes = 30
	}
	return time.Duration(intervalMinutes) * time.Minute
}

func (modelUpdateHandler) NewPayload() any { return nil }

// modelUpdateTaskPayload controls one model_update run. A scheduled run
// (Manual=false) respects the per-channel minimum check interval and may
// auto-apply detected models when a channel has auto-sync enabled. A manual
// "detect all" trigger sets Manual=true to reproduce the legacy detect-all
// semantics: force a re-check regardless of the interval and never auto-apply,
// so the admin reviews and applies changes explicitly.
type modelUpdateTaskPayload struct {
	Manual bool `json:"manual,omitempty"`
}

func (modelUpdateHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	payload := modelUpdateTaskPayload{}
	if err := task.DecodePayload(&payload); err != nil {
		finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusFailed, nil, err)
		return
	}
	summary := runChannelUpstreamModelUpdateTask(ctx, payload.Manual, !payload.Manual, service.NewSystemTaskProgressReporter(task, runnerID))
	finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusSucceeded, summary, nil)
}

// midjourneyPollHandler runs one Midjourney polling pass per scheduled run.
// Enabled() folds the "are there unfinished tasks?" check into enablement so the
// scheduler creates no row when the system is idle; only when at least one
// Midjourney task is in progress does a row get scheduled.
type midjourneyPollHandler struct{}

func (midjourneyPollHandler) Type() string { return model.SystemTaskTypeMidjourneyPoll }

func (midjourneyPollHandler) Enabled() bool {
	return common.IsMasterNode && model.HasUnfinishedMidjourneyTasks()
}

func (midjourneyPollHandler) Interval() time.Duration { return 15 * time.Second }

func (midjourneyPollHandler) NewPayload() any { return nil }

func (midjourneyPollHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	summary := runMidjourneyTaskUpdateOnce(ctx, service.NewSystemTaskProgressReporter(task, runnerID))
	finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusSucceeded, summary, nil)
}

// asyncTaskPollHandler runs one async-task (Suno/video) polling pass per
// scheduled run. Like midjourneyPollHandler, Enabled() folds in the unfinished
// task existence check so an idle system schedules no rows.
type asyncTaskPollHandler struct{}

func (asyncTaskPollHandler) Type() string { return model.SystemTaskTypeAsyncTaskPoll }

func (asyncTaskPollHandler) Enabled() bool {
	return common.IsMasterNode && model.HasUnfinishedSyncTasks()
}

func (asyncTaskPollHandler) Interval() time.Duration { return 15 * time.Second }

func (asyncTaskPollHandler) NewPayload() any { return nil }

func (asyncTaskPollHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	summary := service.RunTaskPollingOnce(ctx, service.NewSystemTaskProgressReporter(task, runnerID))
	finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusSucceeded, summary, nil)
}

type routingStrategyRefreshHandler struct{}

func (routingStrategyRefreshHandler) Type() string { return model.SystemTaskTypeRoutingRefresh }

func (routingStrategyRefreshHandler) Enabled() bool {
	return routing_strategy_setting.GetSetting().Enabled
}

func (routingStrategyRefreshHandler) Interval() time.Duration {
	return time.Duration(routing_strategy_setting.GetUpdateIntervalMinutes()) * time.Minute
}

func (routingStrategyRefreshHandler) NewPayload() any { return nil }

func (routingStrategyRefreshHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	result, err := service.RefreshRoutingStrategySnapshots(ctx)
	if err != nil {
		finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusFailed, result, err)
		return
	}
	finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusSucceeded, result, nil)
}

const monthlyBillingRetryInterval = time.Hour

type monthlyBillingStatementTaskPayload struct {
	Month string `json:"month"`
}

type monthlyBillingStatementHandler struct{}

func (monthlyBillingStatementHandler) Type() string {
	return model.SystemTaskTypeMonthlyBilling
}

func (monthlyBillingStatementHandler) Enabled() bool {
	latest, err := model.GetLatestSystemTask(model.SystemTaskTypeMonthlyBilling)
	if err != nil {
		common.SysLog(fmt.Sprintf("monthly billing scheduler lookup failed: %v", err))
		return false
	}
	return monthlyBillingGenerationDue(time.Now(), latest)
}

func (monthlyBillingStatementHandler) Interval() time.Duration {
	return monthlyBillingRetryInterval
}

func (monthlyBillingStatementHandler) NewPayload() any {
	return monthlyBillingStatementTaskPayload{Month: previousClosedBillingMonth(time.Now())}
}

func (monthlyBillingStatementHandler) Run(_ context.Context, task *model.SystemTask, runnerID string) {
	payload := monthlyBillingStatementTaskPayload{}
	if err := task.DecodePayload(&payload); err != nil {
		finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusFailed, nil, err)
		return
	}
	if payload.Month == "" {
		finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusFailed, nil, fmt.Errorf("billing month is required"))
		return
	}
	result, err := model.GenerateMonthlyBillingStatementsForMonth(payload.Month)
	if err == nil && len(result.FailedUsers) > 0 {
		err = fmt.Errorf("monthly billing generation failed for %d users", len(result.FailedUsers))
	}
	if err != nil {
		finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusFailed, result, err)
		return
	}
	finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusSucceeded, result, nil)
}

func previousClosedBillingMonth(now time.Time) string {
	if now.IsZero() {
		now = time.Now()
	}
	utcNow := now.UTC()
	currentMonth := time.Date(utcNow.Year(), utcNow.Month(), 1, 0, 0, 0, 0, time.UTC)
	return currentMonth.AddDate(0, -1, 0).Format("2006-01")
}

func monthlyBillingGenerationDue(now time.Time, latest *model.SystemTask) bool {
	targetMonth := previousClosedBillingMonth(now)
	if latest == nil {
		return true
	}
	payload := monthlyBillingStatementTaskPayload{}
	if err := latest.DecodePayload(&payload); err != nil || payload.Month == "" {
		return true
	}
	if payload.Month > targetMonth {
		return false
	}
	return payload.Month != targetMonth || latest.Status != model.SystemTaskStatusSucceeded
}

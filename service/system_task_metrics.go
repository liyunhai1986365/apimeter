package service

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// systemTaskExecutionTotal counts total system task executions by type and status
	systemTaskExecutionTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "new_api_system_task_execution_total",
			Help: "Total number of system task executions",
		},
		[]string{"type", "status"},
	)

	// systemTaskExecutionDuration tracks system task execution duration in seconds
	systemTaskExecutionDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "new_api_system_task_execution_duration_seconds",
			Help:    "System task execution duration in seconds",
			Buckets: []float64{0.1, 0.5, 1, 2, 5, 10, 30, 60, 120, 300, 600},
		},
		[]string{"type"},
	)

	// systemTaskInProgress tracks currently executing system tasks
	systemTaskInProgress = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "new_api_system_task_in_progress",
			Help: "Number of system tasks currently in progress",
		},
		[]string{"type"},
	)

	// systemTaskLastExecutionTime tracks the timestamp of last successful execution
	systemTaskLastExecutionTime = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "new_api_system_task_last_execution_timestamp_seconds",
			Help: "Timestamp of the last successful system task execution",
		},
		[]string{"type"},
	)

	// systemTaskFailureTotal counts total system task failures by type
	systemTaskFailureTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "new_api_system_task_failure_total",
			Help: "Total number of system task failures",
		},
		[]string{"type"},
	)

	// systemTaskLockWaitDuration tracks time spent waiting for task lock
	systemTaskLockWaitDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "new_api_system_task_lock_wait_duration_seconds",
			Help:    "Time spent waiting for system task lock in seconds",
			Buckets: []float64{0.001, 0.01, 0.1, 0.5, 1, 2, 5, 10},
		},
		[]string{"type"},
	)

	// systemTaskInstancesActive tracks number of active system task runner instances
	systemTaskInstancesActive = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "new_api_system_task_instances_active",
			Help: "Number of active system task runner instances",
		},
	)

	// systemTaskScheduledTotal counts total scheduled tasks
	systemTaskScheduledTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "new_api_system_task_scheduled_total",
			Help: "Total number of scheduled system tasks",
		},
		[]string{"type"},
	)

	// systemTaskCancelledTotal counts total cancelled tasks
	systemTaskCancelledTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "new_api_system_task_cancelled_total",
			Help: "Total number of cancelled system tasks",
		},
		[]string{"type"},
	)
)

// RecordSystemTaskExecution records a system task execution
func RecordSystemTaskExecution(taskType, status string) {
	systemTaskExecutionTotal.WithLabelValues(taskType, status).Inc()
}

// RecordSystemTaskDuration records the duration of a system task execution
func RecordSystemTaskDuration(taskType string, durationSeconds float64) {
	systemTaskExecutionDuration.WithLabelValues(taskType).Observe(durationSeconds)
}

// RecordSystemTaskStart records the start of a system task
func RecordSystemTaskStart(taskType string) {
	systemTaskInProgress.WithLabelValues(taskType).Inc()
}

// RecordSystemTaskEnd records the end of a system task
func RecordSystemTaskEnd(taskType string) {
	systemTaskInProgress.WithLabelValues(taskType).Dec()
}

// RecordSystemTaskSuccess records a successful system task execution
func RecordSystemTaskSuccess(taskType string, timestamp float64) {
	systemTaskLastExecutionTime.WithLabelValues(taskType).Set(timestamp)
}

// RecordSystemTaskFailure records a system task failure
func RecordSystemTaskFailure(taskType string) {
	systemTaskFailureTotal.WithLabelValues(taskType).Inc()
}

// RecordSystemTaskLockWait records time spent waiting for task lock
func RecordSystemTaskLockWait(taskType string, durationSeconds float64) {
	systemTaskLockWaitDuration.WithLabelValues(taskType).Observe(durationSeconds)
}

// SetSystemTaskInstancesActive sets the number of active system task runner instances
func SetSystemTaskInstancesActive(count float64) {
	systemTaskInstancesActive.Set(count)
}

// RecordSystemTaskScheduled records a scheduled system task
func RecordSystemTaskScheduled(taskType string) {
	systemTaskScheduledTotal.WithLabelValues(taskType).Inc()
}

// RecordSystemTaskCancelled records a cancelled system task
func RecordSystemTaskCancelled(taskType string) {
	systemTaskCancelledTotal.WithLabelValues(taskType).Inc()
}

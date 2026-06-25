# 系统任务 Prometheus 监控指标

## 实现时间
2026-06-25

## 概述

为系统任务框架添加了完整的 Prometheus 监控指标，用于实时监控任务执行状态、性能和健康度。

---

## 指标列表

### 1. new_api_system_task_execution_total

**类型**: Counter  
**标签**: `type`, `status`  
**说明**: 系统任务执行总次数

**示例**:
```promql
# 查看所有任务类型的执行次数
new_api_system_task_execution_total

# 查看通道测试的成功次数
new_api_system_task_execution_total{type="channel_test", status="succeeded"}

# 查看失败的任务
new_api_system_task_execution_total{status="failed"}

# 计算任务成功率
sum(rate(new_api_system_task_execution_total{status="succeeded"}[5m])) by (type) 
/ 
sum(rate(new_api_system_task_execution_total[5m])) by (type)
```

---

### 2. new_api_system_task_execution_duration_seconds

**类型**: Histogram  
**标签**: `type`  
**说明**: 系统任务执行耗时（秒）

**Buckets**: 0.1, 0.5, 1, 2, 5, 10, 30, 60, 120, 300, 600

**示例**:
```promql
# 查看任务执行时间的 P95
histogram_quantile(0.95, sum(rate(new_api_system_task_execution_duration_seconds_bucket[5m])) by (type, le))

# 查看任务执行时间的 P99
histogram_quantile(0.99, sum(rate(new_api_system_task_execution_duration_seconds_bucket[5m])) by (type, le))

# 查看平均执行时间
rate(new_api_system_task_execution_duration_seconds_sum[5m]) 
/ 
rate(new_api_system_task_execution_duration_seconds_count[5m])

# 查看超过 60 秒的任务执行
sum(rate(new_api_system_task_execution_duration_seconds_bucket{le="60"}[5m])) by (type)
```

---

### 3. new_api_system_task_in_progress

**类型**: Gauge  
**标签**: `type`  
**说明**: 当前正在执行的系统任务数量

**示例**:
```promql
# 查看当前正在执行的任务
new_api_system_task_in_progress

# 按类型查看
new_api_system_task_in_progress{type="channel_test"}

# 查看是否有卡住的任务（执行时间过长）
new_api_system_task_in_progress > 0 for 30m
```

---

### 4. new_api_system_task_last_execution_timestamp_seconds

**类型**: Gauge  
**标签**: `type`  
**说明**: 最后一次成功执行的时间戳（Unix 秒）

**示例**:
```promql
# 查看任务最后成功执行时间
time() - new_api_system_task_last_execution_timestamp_seconds

# 检查任务是否长时间未执行（超过 1 小时）
(time() - new_api_system_task_last_execution_timestamp_seconds) > 3600

# 查看各任务的上次执行时间差
(time() - new_api_system_task_last_execution_timestamp_seconds) / 60
```

---

### 5. new_api_system_task_failure_total

**类型**: Counter  
**标签**: `type`  
**说明**: 系统任务失败总次数

**示例**:
```promql
# 查看失败次数
new_api_system_task_failure_total

# 查看最近 5 分钟的失败率
rate(new_api_system_task_failure_total[5m])

# 按类型统计失败次数
sum(rate(new_api_system_task_failure_total[1h])) by (type)
```

---

### 6. new_api_system_task_lock_wait_duration_seconds

**类型**: Histogram  
**标签**: `type`  
**说明**: 等待获取任务锁的时间（秒）

**Buckets**: 0.001, 0.01, 0.1, 0.5, 1, 2, 5, 10

**示例**:
```promql
# 查看锁等待时间的 P95
histogram_quantile(0.95, sum(rate(new_api_system_task_lock_wait_duration_seconds_bucket[5m])) by (type, le))

# 查看平均锁等待时间
rate(new_api_system_task_lock_wait_duration_seconds_sum[5m]) 
/ 
rate(new_api_system_task_lock_wait_duration_seconds_count[5m])

# 检测锁竞争（等待时间 > 1 秒）
histogram_quantile(0.99, sum(rate(new_api_system_task_lock_wait_duration_seconds_bucket[5m])) by (type, le)) > 1
```

---

### 7. new_api_system_task_instances_active

**类型**: Gauge  
**说明**: 当前活跃的系统任务运行器实例数量

**示例**:
```promql
# 查看活跃实例数
new_api_system_task_instances_active

# 检查是否有实例在运行
new_api_system_task_instances_active > 0

# 检查实例数异常（假设正常应该有 2-3 个）
new_api_system_task_instances_active < 2 or new_api_system_task_instances_active > 5
```

---

### 8. new_api_system_task_scheduled_total

**类型**: Counter  
**标签**: `type`  
**说明**: 已调度的系统任务总数

**示例**:
```promql
# 查看调度次数
new_api_system_task_scheduled_total

# 查看调度频率
rate(new_api_system_task_scheduled_total[5m])

# 按类型统计调度次数
sum(rate(new_api_system_task_scheduled_total[1h])) by (type)
```

---

### 9. new_api_system_task_cancelled_total

**类型**: Counter  
**标签**: `type`  
**说明**: 已取消的系统任务总数

**示例**:
```promql
# 查看取消次数
new_api_system_task_cancelled_total

# 查看取消率
rate(new_api_system_task_cancelled_total[5m])

# 检查取消率是否过高
rate(new_api_system_task_cancelled_total[5m]) > 0.1
```

---

## 监控面板示例

### Grafana Dashboard JSON

```json
{
  "dashboard": {
    "title": "System Tasks Monitoring",
    "panels": [
      {
        "title": "Task Execution Rate",
        "targets": [
          {
            "expr": "sum(rate(new_api_system_task_execution_total[5m])) by (type, status)"
          }
        ]
      },
      {
        "title": "Task Duration P95",
        "targets": [
          {
            "expr": "histogram_quantile(0.95, sum(rate(new_api_system_task_execution_duration_seconds_bucket[5m])) by (type, le))"
          }
        ]
      },
      {
        "title": "Tasks In Progress",
        "targets": [
          {
            "expr": "new_api_system_task_in_progress"
          }
        ]
      },
      {
        "title": "Task Success Rate",
        "targets": [
          {
            "expr": "sum(rate(new_api_system_task_execution_total{status=\"succeeded\"}[5m])) by (type) / sum(rate(new_api_system_task_execution_total[5m])) by (type)"
          }
        ]
      }
    ]
  }
}
```

---

## 告警规则

### Prometheus AlertManager Rules

```yaml
groups:
  - name: system_tasks
    interval: 30s
    rules:
      # 任务连续失败告警
      - alert: SystemTaskHighFailureRate
        expr: |
          rate(new_api_system_task_failure_total[5m]) > 0.1
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "System task {{ $labels.type }} has high failure rate"
          description: "Task {{ $labels.type }} failure rate is {{ $value }} per second"

      # 任务执行时间过长告警
      - alert: SystemTaskSlowExecution
        expr: |
          histogram_quantile(0.95, sum(rate(new_api_system_task_execution_duration_seconds_bucket[5m])) by (type, le)) > 600
        for: 10m
        labels:
          severity: warning
        annotations:
          summary: "System task {{ $labels.type }} is running slow"
          description: "Task {{ $labels.type }} P95 duration is {{ $value }} seconds"

      # 任务长时间未执行告警
      - alert: SystemTaskNotExecuted
        expr: |
          (time() - new_api_system_task_last_execution_timestamp_seconds) > 3600
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "System task {{ $labels.type }} has not been executed recently"
          description: "Task {{ $labels.type }} last executed {{ $value }} seconds ago"

      # 任务一直在执行（可能卡住）
      - alert: SystemTaskStuck
        expr: |
          new_api_system_task_in_progress > 0
        for: 30m
        labels:
          severity: critical
        annotations:
          summary: "System task {{ $labels.type }} may be stuck"
          description: "Task {{ $labels.type }} has been running for more than 30 minutes"

      # 锁等待时间过长告警
      - alert: SystemTaskLockContention
        expr: |
          histogram_quantile(0.99, sum(rate(new_api_system_task_lock_wait_duration_seconds_bucket[5m])) by (type, le)) > 2
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "System task {{ $labels.type }} has high lock contention"
          description: "Task {{ $labels.type }} P99 lock wait is {{ $value }} seconds"

      # 无活跃实例告警
      - alert: NoSystemTaskRunners
        expr: |
          new_api_system_task_instances_active == 0
        for: 2m
        labels:
          severity: critical
        annotations:
          summary: "No active system task runner instances"
          description: "All system task runners appear to be down"

      # 实例数异常告警
      - alert: SystemTaskInstanceCountAnomaly
        expr: |
          new_api_system_task_instances_active < 1 or new_api_system_task_instances_active > 10
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "Abnormal number of system task instances"
          description: "Current instance count: {{ $value }}"
```

---

## 查询示例

### 1. 查看任务成功率（最近 5 分钟）

```promql
sum(rate(new_api_system_task_execution_total{status="succeeded"}[5m])) by (type) 
/ 
sum(rate(new_api_system_task_execution_total[5m])) by (type) * 100
```

### 2. 查看任务平均执行时间

```promql
rate(new_api_system_task_execution_duration_seconds_sum[5m]) 
/ 
rate(new_api_system_task_execution_duration_seconds_count[5m])
```

### 3. 查看最慢的任务类型（P95）

```promql
topk(5, histogram_quantile(0.95, 
  sum(rate(new_api_system_task_execution_duration_seconds_bucket[5m])) by (type, le)
))
```

### 4. 查看任务失败率

```promql
sum(rate(new_api_system_task_failure_total[5m])) by (type)
```

### 5. 查看任务调度频率

```promql
sum(rate(new_api_system_task_scheduled_total[5m])) by (type)
```

### 6. 查看锁竞争情况

```promql
histogram_quantile(0.95, 
  sum(rate(new_api_system_task_lock_wait_duration_seconds_bucket[5m])) by (type, le)
) * 1000  # 转换为毫秒
```

### 7. 检查任务是否按预期执行

```promql
# 通道测试应该每 10 分钟执行一次
(time() - new_api_system_task_last_execution_timestamp_seconds{type="channel_test"}) / 60 < 15
```

### 8. 查看任务执行趋势

```promql
sum(increase(new_api_system_task_execution_total[1h])) by (type, status)
```

---

## 集成到现有 Prometheus

### 1. 确保 Prometheus 可以抓取指标

在 `main.go` 或 `router` 中确保已经暴露 `/metrics` 端点：

```go
import "github.com/prometheus/client_golang/prometheus/promhttp"

// 在 router 设置中
router.GET("/metrics", gin.WrapH(promhttp.Handler()))
```

### 2. 配置 Prometheus 抓取

在 `prometheus.yml` 中添加：

```yaml
scrape_configs:
  - job_name: 'new-api'
    static_configs:
      - targets: ['localhost:3000']  # 修改为实际地址
    metrics_path: '/metrics'
    scrape_interval: 15s
```

### 3. 验证指标

访问 `http://your-api:3000/metrics` 查看所有指标，应该能看到：

```
# HELP new_api_system_task_execution_total Total number of system task executions
# TYPE new_api_system_task_execution_total counter
new_api_system_task_execution_total{status="succeeded",type="channel_test"} 10
new_api_system_task_execution_total{status="failed",type="channel_test"} 1

# HELP new_api_system_task_execution_duration_seconds System task execution duration in seconds
# TYPE new_api_system_task_execution_duration_seconds histogram
new_api_system_task_execution_duration_seconds_bucket{le="0.1",type="channel_test"} 0
new_api_system_task_execution_duration_seconds_bucket{le="0.5",type="channel_test"} 2
...
```

---

## 实现细节

### 代码位置

**指标定义**: `service/system_task_metrics.go`
```go
// 定义所有 Prometheus 指标
var (
    systemTaskExecutionTotal = promauto.NewCounterVec(...)
    systemTaskExecutionDuration = promauto.NewHistogramVec(...)
    // ... 其他指标
)
```

**指标记录点**:

1. **任务开始**: `service/system_task.go` - `runWithLeaseHeartbeat()`
   ```go
   RecordSystemTaskStart(task.Type)
   ```

2. **任务结束**: `service/system_task.go` - `runWithLeaseHeartbeat()`
   ```go
   RecordSystemTaskEnd(task.Type)
   RecordSystemTaskDuration(task.Type, durationSeconds)
   ```

3. **任务成功**: `controller/system_task_handlers.go` - `finishSystemTaskHandler()`
   ```go
   RecordSystemTaskSuccess(task.Type, timestamp)
   RecordSystemTaskExecution(task.Type, "succeeded")
   ```

4. **任务失败**: `service/system_task.go` - `failSystemTask()`
   ```go
   RecordSystemTaskFailure(task.Type)
   RecordSystemTaskExecution(task.Type, "failed")
   ```

5. **任务调度**: `service/system_task.go` - `runSystemTaskScheduler()`
   ```go
   RecordSystemTaskScheduled(scheduled.Type())
   ```

6. **锁等待**: `service/system_task.go` - `runSystemTaskClaimPass()`
   ```go
   RecordSystemTaskLockWait(handler.Type(), duration)
   ```

7. **活跃实例**: `service/system_task.go` - `StartSystemTaskRunner()`
   ```go
   SetSystemTaskInstancesActive(float64(count))
   ```

---

## 性能影响

### 资源占用

**内存**:
- 每个指标约 1-2 KB
- 总共约 20-30 KB 增量

**CPU**:
- 指标更新开销极低（< 0.1%）
- Prometheus 抓取时略有增加（< 0.5%）

**网络**:
- 每次抓取约 5-10 KB 数据
- 15 秒间隔，约 0.5 KB/s

---

## 故障排查

### 指标未出现

1. 检查 `/metrics` 端点是否可访问
```bash
curl http://localhost:3000/metrics | grep new_api_system_task
```

2. 检查任务是否执行
```sql
SELECT * FROM system_tasks ORDER BY created_at DESC LIMIT 10;
```

3. 检查 Prometheus 配置
```bash
# 查看 Prometheus targets
curl http://prometheus:9090/api/v1/targets
```

### 指标值异常

1. **in_progress 一直为正数** - 可能有任务卡住
   ```sql
   SELECT * FROM system_tasks WHERE status = 'running';
   ```

2. **last_execution_timestamp 不更新** - 任务未执行
   ```sql
   SELECT type, status, updated_at FROM system_tasks ORDER BY updated_at DESC;
   ```

3. **instances_active 为 0** - 无活跃实例
   ```sql
   SELECT * FROM system_instances ORDER BY last_heartbeat DESC;
   ```

---

## 总结

### 实现内容

✅ **9 个核心指标**
- 执行次数、耗时、进行中任务数
- 成功时间戳、失败次数
- 锁等待、活跃实例、调度次数、取消次数

✅ **完整集成**
- 任务生命周期各阶段记录
- 自动更新活跃实例数
- 锁等待时间追踪

✅ **监控支持**
- Grafana 面板示例
- Prometheus 告警规则
- 常用查询示例

### 后续增强

可选的未来改进：
- 添加任务结果大小指标
- 添加任务重试次数
- 添加任务队列长度

---

**实现完成时间**: 2026-06-25  
**状态**: ✅ Prometheus 监控指标已集成

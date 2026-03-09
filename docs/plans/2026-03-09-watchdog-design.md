# Watchdog 巡检设计

日期: 2026-03-09
状态: draft

## 1. 目标

引入 Watchdog 定时巡检机制，检测并恢复卡死的 Run、滞留的 Issue 和泄漏的信号量槽位。解决系统长时间运行后可能出现的"僵尸"状态问题。

## 2. 现状分析

### 已有的超时机制

| 层级 | 机制 | 覆盖范围 |
|------|------|---------|
| Stage 级 | IdleTimeout (1-5 分钟) | 单个 stage 无活动超时 |
| Engine 级 | tryRecoverStuckRun() | 执行器崩溃后恢复 |
| GitHub 级 | ReconcileJob (10 分钟) | Issue 状态漂移修复 |

### 缺失的监控

- **Run 级超时**：Run 卡在 `in_progress` 超过 SLA 无自动恢复
- **Issue 队列滞留**：Issue 在 `queued`/`ready` 超过合理时间无告警
- **信号量泄漏检测**：所有 slot 满但无 running issue（已通过 R3 panic recovery 部分修复）
- **Merging 超时**：Issue 在 `merging` 状态挂死无兜底

## 3. Watchdog 设计

### 3.1 巡检项

| 巡检项 | 检测条件 | 恢复动作 | 默认阈值 |
|--------|---------|---------|---------|
| stuck_run | Run `in_progress` 且 `UpdatedAt` 超过阈值 | 标记 Run failed，释放 slot | 30 分钟 |
| stuck_merging | Issue `merging` 且无进展超过阈值 | 标记 Issue failed，释放 slot | 15 分钟 |
| queue_stale | Issue `queued`/`ready` 且 `UpdatedAt` 超过阈值 | 仅告警（日志 + 事件） | 60 分钟 |
| sem_leak | `len(sem) == cap(sem)` 但 `len(Running) < cap(sem)` | 释放多余 slot | 每次巡检 |

### 3.2 架构

Watchdog 是 DepScheduler 内部的一个 goroutine，与现有 reconcile loop 并行运行。

```
DepScheduler
  ├── dispatchLoop (主调度循环)
  ├── reconcileLoop (已有)
  └── watchdogLoop (新增)
       ├── checkStuckRuns()
       ├── checkStuckMerging()
       ├── checkQueueStale()
       └── checkSemLeak()
```

### 3.3 核心接口

```go
// 在 DepScheduler 中新增
type WatchdogConfig struct {
    Interval       time.Duration // 巡检间隔，默认 5 分钟
    StuckRunTTL    time.Duration // Run 卡死阈值，默认 30 分钟
    StuckMergeTTL  time.Duration // Merging 卡死阈值，默认 15 分钟
    QueueStaleTTL  time.Duration // 队列滞留告警阈值，默认 60 分钟
}

func (s *DepScheduler) startWatchdog(ctx context.Context, cfg WatchdogConfig)
func (s *DepScheduler) stopWatchdog()
func (s *DepScheduler) watchdogOnce() // 单次巡检，便于测试
```

### 3.4 巡检逻辑

```go
func (s *DepScheduler) checkStuckRuns() {
    s.mu.Lock()
    defer s.mu.Unlock()

    now := time.Now()
    for sessionID, rs := range s.sessions {
        for issueID, runID := range rs.Running {
            issue := rs.IssueByID[issueID]
            if issue == nil || issue.Status != IssueStatusExecuting {
                continue
            }
            // 从 store 查 Run 的 UpdatedAt
            run, _ := s.store.GetRun(runID)
            if run == nil || now.Sub(run.UpdatedAt) < s.watchdogCfg.StuckRunTTL {
                continue
            }
            // 超时 → 发 EventRunFailed
            s.mu.Unlock()
            _ = s.OnEvent(ctx, Event{
                Type:  EventRunFailed,
                RunID: runID,
                Error: fmt.Sprintf("watchdog: run stuck for %v", now.Sub(run.UpdatedAt)),
            })
            s.mu.Lock()
        }
    }
}
```

### 3.5 信号量泄漏检测

```go
func (s *DepScheduler) checkSemLeak() {
    s.mu.Lock()
    defer s.mu.Unlock()

    semUsed := len(s.sem)
    actualRunning := 0
    for _, rs := range s.sessions {
        actualRunning += len(rs.Running)
    }

    if semUsed > actualRunning {
        leaked := semUsed - actualRunning
        slog.Warn("watchdog: semaphore leak detected", "sem", semUsed, "running", actualRunning, "leaked", leaked)
        for i := 0; i < leaked; i++ {
            s.releaseSlot()
        }
    }
}
```

### 3.6 TaskStep 记录

Watchdog 恢复操作写 TaskStep：
- `action: "failed"` + `note: "watchdog: run stuck for 35m"`
- `agent_id: "watchdog"`

## 4. 配置

```toml
[scheduler.watchdog]
enabled         = true
interval        = "5m"
stuck_run_ttl   = "30m"
stuck_merge_ttl = "15m"
queue_stale_ttl = "60m"
```

## 5. 改造范围

### 新增

- `internal/teamleader/watchdog.go` — Watchdog 逻辑
- `internal/teamleader/watchdog_test.go` — 测试

### 改造

- `internal/teamleader/scheduler.go` — 启动/停止 watchdog goroutine
- `internal/config/types.go` — WatchdogConfig 配置项
- `internal/config/defaults.toml` — 默认值

### 不变

- Run 模型、Issue 模型、事件系统、Store 接口

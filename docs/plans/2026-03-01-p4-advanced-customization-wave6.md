# P4 Wave 6 — 治理能力补齐（priority/budget/trace/backup）

> **For Agent:** REQUIRED SUB-SKILL: Use executing-wave-plans to implement this wave and finalize P4.

## Wave Goal

在保持单实例、本地优先架构前提下，补齐最新 spec 的治理能力：优先级调度、Token 预算控制、统一 trace_id、数据库自动备份。

## Depends On

- `[W5-T1, W5-T2, W5-T3]`

## Wave Entry Data

- P4 Wave 1~5 已完成能力建设，Pipeline 主链路稳定可用。
- `checkpoint.tokens_used` 已可记录 Token 使用量，但尚无预算门禁。
- 现有日志具备结构化字段，但未形成 TaskPlan 级统一 trace_id。
- SQLite 处于 WAL 模式并具备崩溃恢复能力，但缺少自动备份策略。

## Tasks

### Task W6-T1: 优先级调度 + 优先级继承

**Files:**
- Modify: `internal/core/taskitem.go`
- Modify: `internal/secretary/scheduler.go`
- Modify: `internal/secretary/scheduler_test.go`
- Modify: `internal/secretary/dag.go`
- Modify: `internal/secretary/dag_test.go`
- Modify: `internal/config/types.go`
- Modify: `internal/config/defaults.go`

**Depends on:** `[]`

**Step 1: Write failing test**
```text
新增测试：
- TestScheduler_ReadyQueueSortedByPriority
- TestScheduler_PriorityInheritance_PropagatesToBlockingChain
- TestScheduler_DefaultPriority_IsP1
```

**Step 2: Run to confirm failure**
Run: `go test ./internal/secretary ./internal/core -run 'Priority|Inheritance|ReadyQueue'`
Expected: priority 字段或调度排序逻辑缺失。

**Step 3: Minimal implementation**
```text
TaskItem 增加 priority（P0/P1/P2，默认 P1）。
DAG Scheduler 对 ready 队列按 priority 排序（同优先级保持 FIFO）。
当高优先级任务被依赖链阻塞时，提升阻塞链有效优先级用于调度。
```

**Step 4: Run tests to confirm pass**
Run: `go test ./internal/secretary ./internal/core -run 'Priority|Inheritance|ReadyQueue'`
Expected: PASS。

**Step 5: Commit**
```bash
git add internal/core/taskitem.go internal/secretary/scheduler.go internal/secretary/scheduler_test.go internal/secretary/dag.go internal/secretary/dag_test.go internal/config/types.go internal/config/defaults.go
git commit -m "feat(scheduler): add priority scheduling and inheritance"
```

### Task W6-T2: Token 月度预算门禁 + 超限告警

**Files:**
- Modify: `internal/config/types.go`
- Modify: `internal/config/defaults.go`
- Modify: `internal/plugins/store-sqlite/store.go`
- Modify: `internal/plugins/store-sqlite/store_test.go`
- Modify: `internal/engine/executor.go`
- Modify: `internal/engine/executor_behavior_test.go`
- Modify: `internal/core/events.go`

**Depends on:** `[W6-T1]`

**Step 1: Write failing test**
```text
新增测试：
- TestBudgetGate_MonthlyLimitExceeded_AutoPausePipeline
- TestBudgetGate_UnderLimit_AllowsExecution
- TestBudgetGate_EmitsBudgetExceededEvent
```

**Step 2: Run to confirm failure**
Run: `go test ./internal/engine ./internal/plugins/store-sqlite -run 'Budget|MonthlyLimit|AutoPause'`
Expected: budget 配置或门禁逻辑缺失。

**Step 3: Minimal implementation**
```text
新增配置：budget.monthly_token_limit（全局 + 项目级覆盖）。
执行前读取当月累计 tokens_used，超限时将 pipeline 标记 waiting_human，并写明 budget_exceeded 原因。
发送通知事件（best-effort），但不改变已完成任务状态。
```

**Step 4: Run tests to confirm pass**
Run: `go test ./internal/engine ./internal/plugins/store-sqlite -run 'Budget|MonthlyLimit|AutoPause'`
Expected: PASS。

**Step 5: Commit**
```bash
git add internal/config/types.go internal/config/defaults.go internal/plugins/store-sqlite/store.go internal/plugins/store-sqlite/store_test.go internal/engine/executor.go internal/engine/executor_behavior_test.go internal/core/events.go
git commit -m "feat(budget): enforce monthly token limit gate"
```

### Task W6-T3: TaskPlan 级 trace_id 全链路贯通

**Files:**
- Modify: `internal/core/taskplan.go`
- Modify: `internal/secretary/agent.go`
- Modify: `internal/engine/executor.go`
- Modify: `internal/plugins/store-sqlite/store.go`
- Modify: `internal/plugins/store-sqlite/store_test.go`
- Modify: `internal/web/ws.go`
- Modify: `internal/web/handlers_pipeline.go`

**Depends on:** `[W6-T2]`

**Step 1: Write failing test**
```text
新增测试：
- TestTaskPlanTraceID_GeneratedOnPlanCreate
- TestTraceID_PropagatesToPipelineAndCheckpoint
- TestWSAndLogs_ContainTraceID
```

**Step 2: Run to confirm failure**
Run: `go test ./internal/core ./internal/engine ./internal/plugins/store-sqlite ./internal/web -run 'TraceID|Trace|Checkpoint'`
Expected: trace_id 传播链路不完整。

**Step 3: Minimal implementation**
```text
在 TaskPlan 创建时生成 trace_id（无外部值时自动生成）。
将 trace_id 传递到 TaskItem -> Pipeline -> Checkpoint -> EventBus -> 日志。
WebSocket 推送与查询 API 输出 trace_id，便于前端与运维追踪。
```

**Step 4: Run tests to confirm pass**
Run: `go test ./internal/core ./internal/engine ./internal/plugins/store-sqlite ./internal/web -run 'TraceID|Trace|Checkpoint'`
Expected: PASS。

**Step 5: Commit**
```bash
git add internal/core/taskplan.go internal/secretary/agent.go internal/engine/executor.go internal/plugins/store-sqlite/store.go internal/plugins/store-sqlite/store_test.go internal/web/ws.go internal/web/handlers_pipeline.go
git commit -m "feat(obs): propagate taskplan trace id end to end"
```

### Task W6-T4: SQLite 自动备份 + 恢复演练命令

**Files:**
- Modify: `internal/config/types.go`
- Modify: `internal/config/defaults.go`
- Create: `internal/plugins/store-sqlite/backup_job.go`
- Create: `internal/plugins/store-sqlite/backup_job_test.go`
- Create: `cmd/ai-flow/commands_store_backup.go`
- Create: `cmd/ai-flow/commands_store_backup_test.go`
- Modify: `cmd/ai-flow/commands.go`

**Depends on:** `[W6-T3]`

**Step 1: Write failing test**
```text
新增测试：
- TestBackupJob_CreatesTimestampedSnapshot
- TestBackupJob_PrunesExpiredBackups
- TestStoreBackupCommand_DryRunAndVerify
```

**Step 2: Run to confirm failure**
Run: `go test ./internal/plugins/store-sqlite ./cmd/ai-flow -run 'BackupJob|StoreBackup|DryRun|Verify'`
Expected: 自动备份任务或命令缺失。

**Step 3: Minimal implementation**
```text
新增配置：store.backup.interval、store.backup.path、store.backup.retention。
后台定时生成 SQLite 备份快照并按 retention 清理。
新增 `ai-flow store backup --dry-run --verify` 用于恢复演练前的完整性检查。
```

**Step 4: Run tests to confirm pass**
Run: `go test ./internal/plugins/store-sqlite ./cmd/ai-flow -run 'BackupJob|StoreBackup|DryRun|Verify'`
Expected: PASS。

**Step 5: Commit**
```bash
git add internal/config/types.go internal/config/defaults.go internal/plugins/store-sqlite/backup_job.go internal/plugins/store-sqlite/backup_job_test.go cmd/ai-flow/commands_store_backup.go cmd/ai-flow/commands_store_backup_test.go cmd/ai-flow/commands.go
git commit -m "feat(store): add scheduled backup and verification command"
```

## Test Strategy Per Task

| Task | Unit | Integration |
|---|---|---|
| W6-T1 | priority 排序、继承规则 | 多依赖链调度回放 |
| W6-T2 | 预算判定、暂停行为、事件 | 月度累计 token 模拟 |
| W6-T3 | trace_id 传播、日志字段 | API + WS 端到端追踪 |
| W6-T4 | 备份生成/清理、命令返回码 | 备份目录与恢复演练 smoke |

## Risks and Mitigations

- 风险：优先级继承导致“低优先级饥饿”。  
  缓解：继承仅提升阻塞链有效优先级，不改原始优先级字段；同级 FIFO 保序。
- 风险：预算门禁误判影响正常执行。  
  缓解：预算统计基于已落库 tokens_used，且提供管理员手动放行路径。
- 风险：trace_id 引入后日志字段不一致。  
  缓解：统一封装日志上下文注入，新增结构化日志回归测试。
- 风险：备份任务在高写入场景影响性能。  
  缓解：使用 WAL 安全快照策略并将备份 IO 放到后台任务，失败降级为告警。

## Wave E2E/Smoke Cases and Entry Data

### Entry Data
- 三组任务：P0 阻塞链、P1 常规链、P2 低优先级链。
- 一组月度 token 累计样本（低于阈值 / 高于阈值）。
- 一份启用 `store.backup` 的本地配置。

### Smoke Cases
- P0 任务被 P2 任务阻塞时，阻塞链在下一轮调度中被优先执行。
- 超出 `monthly_token_limit` 后，新自动任务进入 `waiting_human` 并产生预算告警事件。
- 任一 pipeline 详情可看到统一 trace_id，并在日志查询中可过滤。
- `ai-flow store backup --dry-run --verify` 返回 0 且输出最新快照信息。

## Wave Exit Gate
- Execute mandatory gate sequence via `executing-wave-plans`.
- Wave-specific acceptance:
  - [ ] 优先级调度与优先级继承行为可验证且不破坏既有 FIFO 语义。
  - [ ] Token 预算门禁可阻断新自动任务并保留审计线索。
  - [ ] trace_id 从 TaskPlan 到日志全链路可追踪。
  - [ ] 自动备份可运行，恢复演练命令可用于运维检查。
- Wave-specific verification:
  - [ ] `go test ./internal/secretary ./internal/core -run 'Priority|Inheritance|ReadyQueue'` 通过。
  - [ ] `go test ./internal/engine ./internal/plugins/store-sqlite -run 'Budget|Trace|Backup'` 通过。
  - [ ] `go test ./cmd/ai-flow -run 'StoreBackup|backup'` 通过。
  - [ ] `go test ./...` 全量通过一次。
- Boundary-change verification (if triggered):
  - [ ] 若修改配置层，执行 `go test ./internal/config -run 'Merge|Defaults|Budget|Backup'` 并确认 PASS。

## Next Wave Entry Condition
- Wave 6 为 P4 最终波次；满足 Exit Gate 后进入 P4 Done 验收。

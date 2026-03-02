# Wave 6 — P4: 治理能力

> **Wave Goal:** 补齐治理闭环：任务优先级调度与继承、Token 月度预算门禁、trace_id 全链路贯通、SQLite 自动备份与恢复。

## 任务列表

### Task W6-T1: 优先级调度 + 继承

**Files:**
- Modify: `internal/core/types.go` (TaskItem 新增 Priority 字段)
- Modify: `internal/secretary/scheduler.go` (DAG Scheduler 按优先级排序)
- Test: `internal/secretary/scheduler_test.go`

**Depends on:** `[]`

**Step 1: Write failing test**
```go
func TestSchedulerPriorityOrdering(t *testing.T) {
    items := []core.TaskItem{
        {ID: "t1", Priority: core.PriorityP2, Deps: nil},
        {ID: "t2", Priority: core.PriorityP0, Deps: nil},
        {ID: "t3", Priority: core.PriorityP1, Deps: nil},
    }
    sched := secretary.NewScheduler(items)
    ready := sched.ReadyQueue()

    // P0 排在最前
    require.Equal(t, "t2", ready[0].ID)
    require.Equal(t, "t3", ready[1].ID)
    require.Equal(t, "t1", ready[2].ID)
}

func TestSchedulerPriorityInheritance(t *testing.T) {
    // t2(P2) blocks t1(P0) → t2 应继承 P0
    items := []core.TaskItem{
        {ID: "t1", Priority: core.PriorityP0, Deps: []string{"t2"}},
        {ID: "t2", Priority: core.PriorityP2, Deps: nil},
    }
    sched := secretary.NewScheduler(items)
    ready := sched.ReadyQueue()

    // t2 继承 P0
    require.Equal(t, core.PriorityP0, ready[0].EffectivePriority)
}
```

**Step 2: Run to confirm failure**
Run: `go test ./internal/secretary/ -run TestSchedulerPriority -count=1`
Expected: `undefined: PriorityP0`

**Step 3: Minimal implementation**
```go
// core/types.go
type Priority int
const (
    PriorityP0 Priority = iota // 最高
    PriorityP1
    PriorityP2
)

// TaskItem 新增
type TaskItem struct {
    // ... existing fields
    Priority          Priority `json:"priority"`
    EffectivePriority Priority `json:"-"` // 继承后的优先级
}
```

Scheduler 改动：
- `ReadyQueue()` 按 `EffectivePriority` 排序（升序，P0 最先）
- 初始化时遍历 DAG，将下游高优先级向上游传播

**Step 4: Run tests to confirm pass**
Run: `go test ./internal/secretary/ -run TestSchedulerPriority -count=1`
Expected: `PASS`

**Step 5: Commit**
```bash
git add internal/core/types.go internal/secretary/scheduler.go internal/secretary/scheduler_test.go
git commit -m "feat(scheduler): priority-based ordering with inheritance"
```

---

### Task W6-T2: Token 预算门禁

**Files:**
- Modify: `internal/config/config.go` (新增 budget 配置)
- Create: `internal/engine/budget.go`
- Test: `internal/engine/budget_test.go`

**Depends on:** `[]`

**Step 1: Write failing test**
```go
func TestTokenBudgetGate(t *testing.T) {
    gate := engine.NewBudgetGate(engine.BudgetConfig{
        MonthlyTokenLimit: 1_000_000,
    })

    // 未超限
    err := gate.Check(500_000)
    require.NoError(t, err)

    // 超限
    err = gate.Check(1_500_000)
    require.ErrorIs(t, err, engine.ErrBudgetExceeded)
}

func TestTokenBudgetAccumulation(t *testing.T) {
    gate := engine.NewBudgetGate(engine.BudgetConfig{
        MonthlyTokenLimit: 100,
    })

    gate.Record(60) // 累计 60
    err := gate.Check(50) // 60 + 50 = 110 > 100
    require.ErrorIs(t, err, engine.ErrBudgetExceeded)
}

func TestTokenBudgetAlert(t *testing.T) {
    notifier := &mockNotifier{}
    gate := engine.NewBudgetGate(engine.BudgetConfig{
        MonthlyTokenLimit: 100,
        AlertThreshold:    0.8, // 80% 告警
        Notifier:          notifier,
    })

    gate.Record(85) // 85% > 80%
    require.True(t, notifier.called)
    require.Contains(t, notifier.lastMessage, "85%")
}
```

**Step 2: Run to confirm failure**
Run: `go test ./internal/engine/ -run TestTokenBudget -count=1`
Expected: `undefined: NewBudgetGate`

**Step 3: Minimal implementation**
```go
type BudgetGate struct {
    limit     int64
    threshold float64
    used      int64
    notifier  core.NotifierPlugin
    mu        sync.Mutex
    alerted   bool
}

func (bg *BudgetGate) Check(estimated int64) error {
    if bg.limit <= 0 { return nil } // 未配置则不限制
    if bg.used + estimated > bg.limit {
        return ErrBudgetExceeded
    }
    return nil
}

func (bg *BudgetGate) Record(tokens int64) {
    bg.mu.Lock()
    bg.used += tokens
    pct := float64(bg.used) / float64(bg.limit)
    bg.mu.Unlock()
    if pct >= bg.threshold && !bg.alerted {
        bg.notifier.Notify(...)
        bg.alerted = true
    }
}
```

Executor 在 `executeStage` 前调用 `BudgetGate.Check()`，stage 完成后 `BudgetGate.Record(result.Usage.TotalTokens)`。

**Step 4: Run tests to confirm pass**
Run: `go test ./internal/engine/ -run TestTokenBudget -count=1`
Expected: `PASS`

**Step 5: Commit**
```bash
git add internal/config/config.go internal/engine/budget.go internal/engine/budget_test.go
git commit -m "feat(engine): token budget gate with monthly limit and alerts"
```

---

### Task W6-T3: trace_id 全链路贯通

**Files:**
- Modify: `internal/core/types.go` (TaskPlan 新增 TraceID)
- Modify: `internal/engine/executor.go` (Stage/Checkpoint 注入 trace_id)
- Modify: `internal/plugins/store-sqlite/store.go` (查询支持 trace_id 过滤)
- Test: `internal/engine/executor_test.go`

**Depends on:** `[]`

**Step 1: Write failing test**
```go
func TestTraceIDPropagation(t *testing.T) {
    plan := core.TaskPlan{
        ID:      "plan-1",
        TraceID: "trace-abc-123",
        Items:   []core.TaskItem{{ID: "t1"}},
    }

    // 创建 Pipeline 时 trace_id 传播到 Pipeline 和每个 Stage
    pipeline := engine.NewPipeline(plan, plan.Items[0])
    require.Equal(t, "trace-abc-123", pipeline.TraceID)

    // Checkpoint 携带 trace_id
    cp := pipeline.NewCheckpoint()
    require.Equal(t, "trace-abc-123", cp.TraceID)
}

func TestStoreQueryByTraceID(t *testing.T) {
    store := newTestSQLiteStore(t)
    // 插入带 trace_id 的记录
    store.SavePipeline(ctx, &core.Pipeline{ID: "p1", TraceID: "trace-abc"})
    store.SavePipeline(ctx, &core.Pipeline{ID: "p2", TraceID: "trace-def"})

    // 按 trace_id 查询
    results, err := store.ListPipelines(ctx, core.PipelineFilter{TraceID: "trace-abc"})
    require.NoError(t, err)
    require.Len(t, results, 1)
    require.Equal(t, "p1", results[0].ID)
}
```

**Step 2: Run to confirm failure**
Run: `go test ./internal/engine/ -run TestTraceID -count=1`
Expected: `unknown field TraceID`

**Step 3: Minimal implementation**
- `TaskPlan` 新增 `TraceID string`
- TaskPlan 创建时自动生成 `trace_id`（UUID v4）
- Pipeline/Stage/Checkpoint 继承 `TraceID`
- slog 日志注入 `trace_id` 字段
- SQLite 表新增 `trace_id` 列（migration）

**Step 4: Run tests to confirm pass**
Run: `go test ./internal/engine/ -run TestTraceID -count=1`
Expected: `PASS`

**Step 5: Commit**
```bash
git add internal/core/types.go internal/engine/ internal/plugins/store-sqlite/
git commit -m "feat(core): trace_id propagation from TaskPlan through Pipeline/Checkpoint"
```

---

### Task W6-T4: SQLite 自动备份

**Files:**
- Create: `internal/plugins/store-sqlite/backup.go`
- Test: `internal/plugins/store-sqlite/backup_test.go`
- Modify: `internal/config/config.go`

**Depends on:** `[]`

**Step 1: Write failing test**
```go
func TestSQLiteBackup(t *testing.T) {
    store := newTestSQLiteStore(t)
    backupDir := t.TempDir()

    bk := sqlite.NewBackupManager(sqlite.BackupConfig{
        Store:    store,
        Path:     backupDir,
        Interval: 1 * time.Second,
    })

    // 触发一次备份
    err := bk.RunOnce(context.Background())
    require.NoError(t, err)

    // 验证备份文件存在
    files, _ := filepath.Glob(filepath.Join(backupDir, "*.db"))
    require.Len(t, files, 1)
}

func TestSQLiteRestore(t *testing.T) {
    store := newTestSQLiteStore(t)
    backupDir := t.TempDir()

    // 插入数据
    store.SavePipeline(ctx, &core.Pipeline{ID: "p1"})

    // 备份
    bk := sqlite.NewBackupManager(sqlite.BackupConfig{Store: store, Path: backupDir})
    bk.RunOnce(context.Background())

    // 删除原数据
    store.DeletePipeline(ctx, "p1")

    // 恢复
    err := bk.Restore(context.Background(), filepath.Join(backupDir, "*.db"))
    require.NoError(t, err)

    // 验证数据恢复
    p, err := store.GetPipeline(ctx, "p1")
    require.NoError(t, err)
    require.Equal(t, "p1", p.ID)
}
```

**Step 2: Run to confirm failure**
Run: `go test ./internal/plugins/store-sqlite/ -run TestSQLiteBackup -count=1`
Expected: `undefined: NewBackupManager`

**Step 3: Minimal implementation**
```go
type BackupManager struct {
    store    *Store
    path     string
    interval time.Duration
}

func (bm *BackupManager) RunOnce(ctx context.Context) error {
    // SQLite VACUUM INTO 或 .backup 命令
    backupFile := filepath.Join(bm.path, fmt.Sprintf("backup-%s.db", time.Now().Format("20060102-150405")))
    _, err := bm.store.db.ExecContext(ctx, fmt.Sprintf("VACUUM INTO '%s'", backupFile))
    return err
}

func (bm *BackupManager) Start(ctx context.Context) {
    // 周期性备份 goroutine
}

func (bm *BackupManager) Restore(ctx context.Context, backupFile string) error {
    // 关闭当前 db → 复制备份 → 重新打开
}
```

配置新增：
```yaml
store:
  backup:
    interval: "24h"
    path: ".ai-workflow/backups"
    max_backups: 7
```

**Step 4: Run tests to confirm pass**
Run: `go test ./internal/plugins/store-sqlite/ -run TestSQLiteBackup -count=1`
Expected: `PASS`

**Step 5: Commit**
```bash
git add internal/plugins/store-sqlite/backup.go internal/plugins/store-sqlite/backup_test.go internal/config/config.go
git commit -m "feat(store): SQLite automatic backup and restore"
```

---

## Risks and Mitigations

| Risk | Severity | Mitigation |
|------|----------|------------|
| 优先级继承在复杂 DAG 中形成环 | 中 | DAG 构建时已做环检测，继承在拓扑排序后执行 |
| Token 预算跨月清零时机 | 低 | 基于 Store 按月统计，不需内存状态持久化 |
| trace_id 新增 migration 影响现有数据 | 低 | ALTER TABLE ADD COLUMN 默认值为空字符串 |
| SQLite VACUUM INTO 在写入期间执行 | 中 | 备份使用 WAL 模式读取，不阻塞写入 |

## Wave Exit Gate
- Execute mandatory gate sequence via `executing-wave-plans`.
- Wave-specific acceptance:
  - [ ] DAG ready queue 按优先级排序，高优先级阻塞链触发继承
  - [ ] Token 预算超限时阻止 stage 执行，并发送告警通知
  - [ ] 每条 TaskPlan 创建时自动生成 trace_id，贯穿到 Pipeline/Checkpoint/日志
  - [ ] SQLite 备份可周期执行，恢复演练成功
- Wave-specific verification:
  - [ ] `go test ./internal/secretary/ -run Priority -count=1` — PASS
  - [ ] `go test ./internal/engine/ -run Budget -count=1` — PASS
  - [ ] `go test ./internal/engine/ -run TraceID -count=1` — PASS
  - [ ] `go test ./internal/plugins/store-sqlite/ -run Backup -count=1` — PASS
  - [ ] `go build ./...` — 全量编译通过
  - [ ] `go test ./... -count=1` — 全部 PASS

## Next Wave Entry Condition
- 本 Wave 为最终波次。全部 Gate 通过后项目进入 P4 完成态。
- 后续 spec-mcp、tracker-linear、GitHub P4+ 命令作为独立计划推进。

# 实施计划 v1: Merging 状态 + 冲突处理 + TL Triage

> **设计依据**: [01-PR/Merge 流程](01-pr-merge-flow.zh-CN.md)
> **升级路径**: v1(自动重试) → v2(TL ACP 决策) → v3(Escalation/Directive 泛化) → v4(A2A 表达)

## Context

当前 AutoMergeHandler 在 Run 完成后尝试 test→PR→merge。但 DepScheduler.onRunDone (`scheduler.go:507-513`) 已经把 issue 标记为 `done`。如果 merge 失败（冲突），issue 仍然是 `done`，**系统卡死没有恢复路径**。

DepScheduler 和事件循环在不同 goroutine 中并发执行，DepScheduler 的 `handleRunEventLocked` 先将 issue 标 `done`，事件循环中的 `autoMerger.OnEvent` 再尝试 merge。merge 失败发出的 `EventRunFailed` 无人处理（issue 已是终态）。

## 设计原则

1. **`merging` 占用 slot** — merge 期间 worktree 仍在使用，必须占用并发位
2. **向后兼容** — `AutoMerge=false` 的 issue 走原路径，零影响
3. **事件驱动** — 新 handler 遵循现有 `OnEvent` 模式
4. **v1 自动重试** — TL Triage 先用自动 rebase 重试，不需要 ACP session
5. **v1 就有重试上限** — 默认 3 次，避免死循环

## Wave 1: 数据层 + DepScheduler 分流

### 1.1 新增状态和事件

**`internal/core/issue.go`**
- L28 后新增: `IssueStatusMerging IssueStatus = "merging"`
- L51 的 `validIssueStatuses` map 中添加
- Issue struct 新增: `MergeRetries int` (json:"merge_retries")

**`internal/core/events.go`**
- L39 后新增:
  ```go
  EventIssueMerging       EventType = "issue_merging"
  EventIssueMerged        EventType = "issue_merged"
  EventIssueMergeConflict EventType = "issue_merge_conflict"
  EventIssueMergeRetry    EventType = "issue_merge_retry"    // TLTriage 重试时专用
  EventMergeFailed        EventType = "merge_failed"         // 非冲突的 merge 失败
  ```
- `IsIssueScopedEvent` 添加 `EventIssueMerging`
- `IsAlwaysBroadcastIssueEvent` 添加 `EventIssueMergeConflict`

**`internal/plugins/store-sqlite/migrations.go`**
- `schemaVersion` 升级（当前版本+1）
- Migration: `ALTER TABLE issues ADD COLUMN merge_retries INTEGER NOT NULL DEFAULT 0`

**`internal/plugins/store-sqlite/store.go`**
- Issue 的 scan/save SQL 加 `merge_retries`

### 1.2 DepScheduler 条件分流

**`internal/teamleader/scheduler.go`**

`handleRunEventLocked` (L507-543) 的 `EventRunDone` 分支改为:
```go
case core.EventRunDone:
    if issue.AutoMerge {
        issue.Status = core.IssueStatusMerging
        if err := s.saveIssue(issue); err != nil {
            return err
        }
        s.publishIssueEvent(core.EventIssueMerging, issue, nil, "")
        // 不释放 slot、不清理 RunIndex、不调 markReady
        // merging 期间仍占用并发位
        return nil
    }
    // 原逻辑不变
    issue.Status = core.IssueStatusDone
    // ...
```

新增 merge 结果处理（在 `OnEvent` 中添加事件监听）:
```go
case core.EventIssueMerged:
    // merge 成功: issue → done, 释放 slot
case core.EventMergeFailed:
    // 非冲突失败: issue → failed, 释放 slot
case core.EventIssueMergeRetry:
    // TLTriage 重试: 释放旧 slot + 清理 RunIndex
    // issue 已被 TLTriageHandler 设为 queued，markReady 自动捡起
case core.EventIssueMergeConflict:
    // 冲突: 不改状态不释放 slot (留给 TLTriageHandler 决策)
```

`scheduleSession` (L275) switch 添加 `case core.IssueStatusMerging:` — 恢复时保持 merging，注册为 Running。

`isIssueTerminal` 不需要改 — `merging` 不是终态。

### 验收
```bash
go build ./...
go test -count=1 ./internal/core/...
go test -count=1 ./internal/plugins/store-sqlite/...
go test -count=1 ./internal/teamleader/...  # scheduler 测试
```

## Wave 2: MergeHandler（重构 AutoMergeHandler）

**`internal/teamleader/auto_merge.go`**

1. `OnEvent` (L46-47): 监听 `EventIssueMerging` 替代 `EventRunDone`
2. 通过 `evt.IssueID` 获取 issue，再通过 `issue.RunID` 获取 run（替代从 EventRunDone 取 RunID）
3. 成功路径: 发 `EventIssueMerged`（替代 `EventAutoMerged`）
4. 失败路径拆分:
   - merge PR 失败且 `isConflictError(err)`: 发 `EventIssueMergeConflict`
   - test gate 失败 / PR 创建失败 / 非冲突 merge 失败: 发 `EventMergeFailed`

新增辅助函数:
```go
func isConflictError(err error) bool {
    msg := strings.ToLower(err.Error())
    return strings.Contains(msg, "conflict") ||
           strings.Contains(msg, "409")
}
```

**`internal/teamleader/auto_merge_test.go`**
- 输入事件从 `EventRunDone` 改为 `EventIssueMerging`
- 测试冲突错误发 `EventIssueMergeConflict`
- 测试非冲突失败发 `EventMergeFailed`

### 验收
```bash
go test -count=1 -v ./internal/teamleader/... -run TestAutoMerge
```

## Wave 3: TLTriageHandler

**新建 `internal/teamleader/tl_triage_handler.go`**

```go
type TLTriageHandler struct {
    store       core.Store
    bus         eventPublisher
    maxRetries  int  // 默认 3
    log         *slog.Logger
}
```

**OnEvent 逻辑:**
```
收到 EventIssueMergeConflict:
  → issue := store.GetIssue(evt.IssueID)
  → if issue.Status != merging → return (防重复)
  → if issue.MergeRetries < maxRetries:
      issue.MergeRetries++
      issue.Status = queued    // 回到队列，跳过 review
      issue.RunID = ""         // 清空旧 run
      store.SaveIssue(issue)
      bus.Publish(EventIssueMergeRetry)  // 专用事件，DepScheduler 释放 slot
  → else:
      issue.Status = failed
      store.SaveIssue(issue)
      bus.Publish(EventIssueFailed, error: "merge conflict retries exhausted")
```

**重试时 rebase 指示注入:**

`scheduler.go` 的 `buildRunFromIssue` (约 L962): 如果 `issue.MergeRetries > 0`，在 Run.Config 写入:
```go
run.Config["merge_conflict_hint"] = "上一次实现与主干产生合并冲突，请先 rebase 解决冲突后再实现需求。"
```

`engine/executor.go` 构建 PromptVars 时读取 `Run.Config["merge_conflict_hint"]`，写入 prompt 展示给 coder agent。

**`cmd/ai-flow/commands.go`**
- 创建 `TLTriageHandler` 并注入 store, bus
- 事件循环中添加 `tlTriageHandler.OnEvent(ctx, evt)`

**新建 `internal/teamleader/tl_triage_handler_test.go`**
- 测试: 首次冲突 → issue 回到 queued, MergeRetries=1
- 测试: 第 3 次冲突 → issue failed
- 测试: 非 merging 状态的事件 → 忽略

### 验收
```bash
go test -count=1 -v ./internal/teamleader/... -run TestTLTriage
```

## 完整事件流

```
RunDone
  ↓
DepScheduler.handleRunEventLocked
  ├─ AutoMerge=false → done (原逻辑)
  └─ AutoMerge=true  → merging + EventIssueMerging
                           ↓
                     MergeHandler.OnEvent
                       ├─ 成功 → EventIssueMerged
                       │         → DepScheduler: done, 释放 slot
                       ├─ 冲突 → EventIssueMergeConflict
                       │         → TLTriageHandler.OnEvent
                       │           ├─ retries < max → queued + EventIssueMergeRetry
                       │           │   → DepScheduler: 释放 slot, markReady, dispatch
                       │           │   → 新 Run 带 rebase hint → 重新执行
                       │           └─ retries >= max → failed + EventIssueFailed
                       │               → DepScheduler: 释放 slot
                       └─ 其他失败 → EventMergeFailed
                                    → DepScheduler: failed, 释放 slot
```

## 文件清单

| 文件 | 操作 | Wave | 说明 |
|------|------|------|------|
| `internal/core/issue.go` | 改 | 1 | +IssueStatusMerging, +MergeRetries 字段 |
| `internal/core/events.go` | 改 | 1 | +5 事件类型 |
| `internal/plugins/store-sqlite/migrations.go` | 改 | 1 | migration: merge_retries 列 |
| `internal/plugins/store-sqlite/store.go` | 改 | 1 | Issue CRUD 加 merge_retries |
| `internal/teamleader/scheduler.go` | 改 | 1 | onRunDone 分流, merge 结果处理, recovery |
| `internal/teamleader/auto_merge.go` | 改 | 2 | 监听 EventIssueMerging, 输出拆分 |
| `internal/teamleader/auto_merge_test.go` | 改 | 2 | 更新测试用例 |
| `internal/teamleader/tl_triage_handler.go` | **新建** | 3 | 冲突自动重试 handler |
| `internal/teamleader/tl_triage_handler_test.go` | **新建** | 3 | 单测 |
| `internal/teamleader/scheduler.go` | 改 | 3 | buildRunFromIssue hint 注入 |
| `cmd/ai-flow/commands.go` | 改 | 3 | 注册 TLTriageHandler |

## 风险点

1. **Slot 泄漏**: merging 期间占用 slot，如果 MergeHandler 挂死则永不释放。缓解: PR 操作加 context timeout（test gate 已有 10min timeout）。
2. **并发竞态**: TLTriageHandler 先写 store 再发事件，DepScheduler 收到事件后从 store 读取最新状态，顺序安全。
3. **EventAutoMerged 兼容**: 外部可能监听 `EventAutoMerged`。过渡期保留发布，后续用 `EventIssueMerged` 替代。

## 升级路径

```
v1 (本计划): TLTriageHandler 自动重试
  ↓ 替换 OnEvent 内部逻辑
v2: TLTriageHandler 启动 ACP session，TL 通过 MCP 工具读 PR diff 做决策
  ↓ 泛化 handler
v3: TLTriageHandler → EscalationRouter，监听所有 escalation 类型，chain 配置路由
  ↓ 映射协议
v4: Escalation = A2A INPUT_REQUIRED，Directive = A2A SendMessage
```

每一步只改一层，下层不动。

## 验证

```bash
go build ./...
go test -count=1 ./internal/core/...                          # Wave 1
go test -count=1 ./internal/plugins/store-sqlite/...          # Wave 1
go test -count=1 -v ./internal/teamleader/...                 # Wave 1-3
go test -count=1 -short ./...                                 # 全量
```

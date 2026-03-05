# 模块拆分与状态机/事件驱动缺陷分析

> 基于源码实际导入图、状态转换追踪和职责交叉分析。
> 生成时间：2026-03-05

---

## 一、状态机缺陷：遗漏与重叠

### 1.1 Issue 状态机——根本不存在

**现状**：Run 有 `ValidateTransition()` + `validTransitions` map。Issue 只有 `validIssueStatuses`（合法值集），**无转换规则，无验证函数**。

11 个状态值、15+ 个转换点散落在 4 个文件中，零个编译期或运行时约束：

```
manager.go        ← draft → reviewing, draft → decomposing, any → abandoned
scheduler.go      ← any → queued (4处!), queued → ready, ready → executing,
                     executing → done/failed, executing → ready (rollback)
decompose_handler ← decomposing → decomposed, decomposing → failed
child_completion  ← decomposed → done/failed
```

**核心问题**：

| 缺陷 | 具体 | 风险 |
|------|------|------|
| 无转换验证 | 任何代码都能把 Issue 设成任何状态 | 非法状态出现时只能靠人肉排查 |
| 4 处重复 `→ queued` | scheduler.go:278, 284, 360, 384 各有不同条件 | 逻辑分支不一致 |
| rollback 无事件 | `executing → ready` 在 scheduler.go:701 发生但不发事件 | UI 和审计无法感知回滚 |
| `draft → reviewing` 无事件 | manager.go:253 改状态但不发布 EventIssueReviewing | 前端无法实时跟踪 |

**应有的 Issue 状态机：**

```
draft ──→ reviewing ──→ queued ──→ ready ──→ executing ──→ done
  │          │                                    │         │
  │       [reject]                            [rollback]  [fail]
  │          ↓                                    ↓         ↓
  │        draft                               ready     failed
  │
  ├──→ decomposing ──→ decomposed ──→ done/failed
  │         │
  │       [fail]
  │         ↓
  │       failed
  │
  └──→ abandoned

非法转换（应被拒绝）：
  done → 任何, failed → 任何（除非显式 retry）,
  abandoned → 任何, superseded → 任何
```

### 1.2 Run 状态机——存在但几乎不执行

**现状**：`ValidateTransition()` 在整个代码库中**只被调用 1 次**（executor.go:188，`queued → in_progress`）。

其余 6 个转换点全部裸写，无验证：

| 转换 | 位置 | 验证? | 事件? |
|------|------|-------|-------|
| queued → in_progress | executor.go:191 | **是** | 无 |
| in_progress → completed(success) | executor.go:401 | 否 | EventRunDone |
| in_progress → completed(failure) | executor.go:481 | 否 | EventRunFailed |
| in_progress → action_required | executor.go:351 | 否 | EventHumanRequired |
| in_progress → action_required | executor.go:384 | 否 | EventHumanRequired |
| action_required → in_progress | actions.go (approve/resume) | 否 | EventActionApplied |
| action_required → completed | actions.go (abort) | 否 | EventRunFailed |

**缺陷**：`queued → in_progress` 是唯一被保护的转换——恰好也是最不可能出错的（Scheduler CAS 已经保证了原子性）。真正容易出错的 `in_progress → completed` 反而不验证。

### 1.3 事件与状态转换的不匹配矩阵

| 状态转换 | 对应事件 | 一致性 |
|---------|---------|--------|
| Issue: draft → reviewing | EventIssueReviewing | **缺失**——状态变了但不发事件 |
| Issue: reviewing → queued | EventIssueQueued | **条件性**——仅部分路径发布 |
| Issue: queued → ready | EventIssueReady | 一致 |
| Issue: ready → executing | EventIssueExecuting | 一致 |
| Issue: executing → done | EventIssueDone | 一致 |
| Issue: executing → failed | EventIssueFailed | **5 个发布点**——过度分散 |
| Issue: executing → ready (rollback) | 无 | **缺失**——回滚不可追溯 |
| Issue: any → abandoned | 无 | **缺失**——放弃操作无事件 |
| Run: queued → in_progress | 无 | **缺失** |
| Run: in_progress → action_required | EventHumanRequired | 一致（2 处） |
| Run: in_progress → completed | EventRunDone / EventRunFailed | 一致 |

**规律**：进入终态（done/failed/completed）的事件完备，进入中间态（reviewing/queued/in_progress）的事件大量缺失。

---

## 二、双调度器问题

### 2.1 两个调度器共存

| 调度器 | 包 | 管什么 | 并发机制 | 使用场景 |
|--------|---|--------|---------|---------|
| `engine.Scheduler` | internal/engine | 纯 Run 队列 | `maxGlobal` + `maxPerProject` 计数 + worktree 锁 | 直接创建 Run |
| `teamleader.DepScheduler` | internal/teamleader | Issue → Run 映射 + 并发 | `sem` 信号量 (buffered channel) | Issue 驱动的 Run |

**问题**：
- 两套并发控制互不感知。engine.Scheduler 的 `maxGlobal=3` 和 DepScheduler 的 `sem=N` 可能叠加超配。
- 代码无显式防护阻止两者同时调度同一项目的 Run。
- stage 默认配置不一致：executor 的 `defaultStageConfig()` 和 scheduler 的 `schedulerDefaultStageConfig()` 对 IdleTimeout 设置不同。

### 2.2 Run 创建的两条路径

```
路径 A（直接）：                        路径 B（Issue 驱动）：
  Executor.CreateRun()                   DepScheduler.buildRunFromIssue()
  → defaultStageConfig()                 → schedulerDefaultStageConfig()
  → IdleTimeout=5min, Timeout=0          → Timeout=30min, IdleTimeout 未设
```

**同一个 template 的 Run，因为创建路径不同，超时行为不一致。**

---

## 三、模块边界问题

### 3.1 Web 是 God Module

`internal/web` 导入 11 个内部包，跨越 4 个架构层：

```
web ──→ core (24 处)
    ──→ teamleader (10 处)     ← 顶层 HTTP 直接依赖业务编排
    ──→ acpclient (9 处)       ← 直接构造 ACP 会话
    ──→ engine (3 处)
    ──→ eventbus (2 处)
    ──→ github, git, mcpserver, observability, plugins/store-sqlite
```

**`web/chat_assistant_acp.go` 直接调用 `teamleader.NewACPHandler()`** ——HTTP handler 硬编码了 TL 的 ACP 交互细节。

### 3.2 接口反演模式

3 个核心接口定义在 library 包，但实现在 `cmd/ai-flow/commands.go`：

| 接口 | 定义 | 实现 |
|------|------|------|
| `ACPHandlerFactory` | engine/executor.go:29 | `acpHandlerFactoryAdapter` commands.go:1066 |
| `IssueManager` | web/server.go:61 | `teamLeaderIssueManagerAdapter` commands.go:88 |
| `A2ABridge` | web/server.go:73 | 直接注入 |

这些接口是为了打破循环依赖而创建的，但把"谁组装"的知识泄漏到了 cmd 层的 1000+ 行 commands.go 中。

### 3.3 EventBus 隐性耦合

8 个包通过 `core.EventType` 字符串常量隐式耦合：

```
发布方：engine/executor, engine/actions, github/webhook_dispatcher, teamleader/scheduler
订阅方：teamleader/auto_merge, teamleader/child_completion, teamleader/decompose_handler,
        teamleader/scheduler, github/reconnect_sync, web/handlers_webhook
```

无 schema 验证——事件的 `Data map[string]string` 字段由发布方填、订阅方猜。改错字段名编译不报错。

---

## 四、模块拆分方案

### 4.1 目标架构

```
cmd/ai-flow
  ├─ commands.go（仅组装，< 200 行）

internal/
  ├─ core/              ← 领域类型 + 状态机（含 Issue 状态机）
  │   ├─ issue.go       ← Issue + ValidateIssueTransition()
  │   ├─ run.go         ← Run + ValidateTransition()（不变）
  │   └─ events.go      ← 精简至 ~10 个事件
  │
  ├─ orchestrator/      ← 新包：统一编排（合并 engine + teamleader 调度）
  │   ├─ scheduler.go   ← 唯一调度器（合并两套并发控制）
  │   ├─ executor.go    ← Run 执行（从 engine 移入）
  │   ├─ issue_lifecycle.go ← Issue 状态流转（从 manager + scheduler 抽出）
  │   └─ handlers.go    ← auto_merge, decompose, child_completion
  │
  ├─ agent/             ← ACP 交互（从 acpclient + engine ACP 逻辑合并）
  │   ├─ client.go      ← ACP Client
  │   ├─ handler.go     ← ACP Handler（从 teamleader 移入）
  │   └─ resolver.go    ← RoleResolver
  │
  ├─ api/               ← 新包：替代 web，仅 HTTP 路由和序列化
  │   ├─ routes.go
  │   ├─ handlers_*.go
  │   └─ ws.go
  │
  ├─ github/            ← 不变
  ├─ plugins/           ← 不变
  └─ config/            ← 不变（类型去重后）
```

### 4.2 关键变更

**变更 1：统一调度器**

合并 `engine.Scheduler` 和 `teamleader.DepScheduler` 为 `orchestrator.Scheduler`：
- 单一并发控制：`maxGlobal` + `maxPerProject` + `sem` 归一
- 单一 stage 配置源：`defaultStageConfig()` 只有一个
- 支持两种入口：直接 Run 和 Issue-driven Run

**变更 2：Issue 状态机强制化**

在 `core/issue.go` 添加：
```go
var validIssueTransitions = map[IssueStatus]map[IssueStatus]bool{
    IssueStatusDraft:       {IssueStatusReviewing: true, IssueStatusDecomposing: true, IssueStatusAbandoned: true},
    IssueStatusReviewing:   {IssueStatusQueued: true, IssueStatusDraft: true, IssueStatusAbandoned: true},
    IssueStatusQueued:      {IssueStatusReady: true, IssueStatusAbandoned: true},
    IssueStatusReady:       {IssueStatusExecuting: true, IssueStatusQueued: true, IssueStatusAbandoned: true},
    IssueStatusExecuting:   {IssueStatusDone: true, IssueStatusFailed: true, IssueStatusReady: true, IssueStatusAbandoned: true},
    IssueStatusDone:        {},  // 终态
    IssueStatusFailed:      {IssueStatusDraft: true},  // 允许重试
    IssueStatusDecomposing: {IssueStatusDecomposed: true, IssueStatusFailed: true},
    IssueStatusDecomposed:  {IssueStatusDone: true, IssueStatusFailed: true},
    IssueStatusSuperseded:  {},  // 终态
    IssueStatusAbandoned:   {},  // 终态
}

func ValidateIssueTransition(from, to IssueStatus) error { ... }
```

所有状态变更点（15+ 处）改为调用此函数，非法转换 fail-fast。

**变更 3：状态转换与事件绑定**

将"改状态"和"发事件"合并为单一操作，消除不匹配：

```go
// orchestrator/issue_lifecycle.go
func (lc *IssueLifecycle) Transition(ctx context.Context, issue *Issue, to IssueStatus, reason string) error {
    if err := ValidateIssueTransition(issue.Status, to); err != nil {
        return err
    }
    old := issue.Status
    issue.Status = to
    issue.UpdatedAt = time.Now()
    if err := lc.store.SaveIssue(issue); err != nil {
        return err
    }
    lc.store.SaveIssueChange(&IssueChange{
        IssueID: issue.ID, Field: "status",
        OldValue: string(old), NewValue: string(to), Reason: reason,
    })
    lc.bus.Publish(statusToEvent(to, issue))  // 自动映射
    return nil
}
```

映射表：

| 目标状态 | 自动发布的事件 |
|---------|--------------|
| reviewing | EventIssueReviewing |
| queued | EventIssueQueued |
| ready | EventIssueReady |
| executing | EventIssueExecuting |
| done | EventIssueDone |
| failed | EventIssueFailed |
| decomposing | EventIssueDecomposing |
| decomposed | EventIssueDecomposed |
| abandoned | EventIssueAbandoned（新增） |

**效果**：15+ 个散落的状态转换点 → 全部调用 `lc.Transition()`，事件发布零遗漏。

**变更 4：Web 拆分**

```
web（当前 11 个导入） → api（3 个导入：core + orchestrator 接口 + config）
```

- `chat_assistant_acp.go` 中的 ACP 逻辑移入 `agent/` 包
- `IssueManager` / `RunExecutor` / `A2ABridge` 接口移入 `orchestrator/` 包
- `api/` 只做 HTTP 序列化和路由，不知道 teamleader/engine 的存在

**变更 5：EventBus 类型安全化**

用 typed event 替代 `map[string]string`：

```go
// core/events.go
type RunDonePayload struct {
    RunID      string        `json:"run_id"`
    Conclusion RunConclusion `json:"conclusion"`
    IssueID    string        `json:"issue_id"`
}

type IssueDonePayload struct {
    IssueID   string `json:"issue_id"`
    ProjectID string `json:"project_id"`
    ParentID  string `json:"parent_id"`
}

// Event.Payload 改为 any，由消费方类型断言
type Event struct {
    Type      EventType `json:"type"`
    Payload   any       `json:"payload"`
    Timestamp time.Time `json:"timestamp"`
}
```

编译期保证 payload 结构，消除字符串约定。

---

## 五、实施路径

| 阶段 | 内容 | 前置 | 风险 |
|------|------|------|------|
| **阶段 1** | 添加 `ValidateIssueTransition` + 在所有转换点调用 | 无 | 极低——纯增量 |
| **阶段 1** | 删除 18 个死事件 + 删除 RunEvent 类型 | 无 | 极低——删死代码 |
| **阶段 2** | 提取 `IssueLifecycle` 统一转换入口 | 阶段 1 | 低——重构不改行为 |
| **阶段 2** | 合并两套 `defaultStageConfig` 为一套 | 无 | 低 |
| **阶段 3** | 合并双调度器为 `orchestrator.Scheduler` | 阶段 2 | 中——需要全量测试 |
| **阶段 3** | 拆分 web → api + 移动 ACP 逻辑到 agent/ | 阶段 2 | 中——大量文件移动 |
| **阶段 4** | EventBus 类型安全化 | 阶段 3 | 中——所有事件消费者需要更新 |

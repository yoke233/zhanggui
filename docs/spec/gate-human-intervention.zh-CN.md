# Step MCP 工具注入 + 人类介入设计方案

> 状态：部分实现
>
> 最后按代码核对：2026-03-14
>
> 对应实现：`internal/platform/appcmd/mcp_serve.go`、`internal/adapters/http/step_signal.go`、`internal/application/flow/signal_e2e_test.go`

## 当前实现状态

本文包含两类内容：

- 已落地的后端能力：核心模型现已命名为 `ActionSignal`，并通过 `mcp-serve`、`/steps/{id}/decision`、`/steps/{id}/unblock`、`/pending-decisions` 暴露
- 尚未完全落地的产品/UI 规划：WorkItem 详情阻塞面板、统一信号时间线、Dashboard 待处理汇总

阅读时请不要把全文都当成“已上线现状”。

补充说明：

- 本文保留了 `StepSignal` / `step` 的产品与 HTTP 语义，因为对外接口当前仍是 `/steps/{id}/*`
- 但应用层执行器真实类型已经是 `WorkItemEngine`，核心对象也已切到 `Action` / `Run`

## 问题：引擎猜 Agent 意图

当前所有 step 类型都存在同一个根本问题：**Agent 没有主动表达意图的手段，引擎在替 Agent 做判断**。

### Exec Step

| Agent 实际状态 | 引擎如何判断 | 问题 |
|---------------|-------------|------|
| 完成了任务 | Session 正常结束 → `handleSuccess` → Done | 引擎不知道 agent 是否真的完成了，只是没报错 |
| 完成了任务 + 结构化结果 | LLM Collector 从 Markdown 猜 summary/files | 额外一次模型调用，可能猜错 |
| 做不了，需要帮助 | 无法表达 → Session 正常结束 → 被标记 Done | **严重**：agent 说"我做不了"被当成成功 |
| 遇到暂时性问题 | 只能 crash → 引擎侧分类 ErrorKind | Agent 没有主动分类错误的能力 |
| 做了一半，有进展 | 无法上报 | 前端看不到中间进度 |

### Gate Step

| Agent 实际状态 | 引擎如何判断 | 问题 |
|---------------|-------------|------|
| 审核通过 | 从 Markdown 正则/LLM 提取 verdict | 格式偏差 → 默认 pass（危险） |
| 审核驳回 | 从 Markdown 正则/LLM 提取 verdict + reason | 提取失败 → 被当成 pass |

### 设计原则

> **Agent 主动声明，引擎不猜。**
>
> 给每种 step 类型注入对应的 MCP 工具，让 agent 通过工具调用显式表达：完成、失败、需要帮助、审核通过/驳回。引擎只负责读取和执行。

## 一、统一信号模型：StepSignal

### 实体定义

```go
// internal/core/step_signal.go

// StepSignal captures an explicit declaration from an agent or human
// about a step's outcome or status change.
type StepSignal struct {
    ID        int64          `json:"id"`
    StepID    int64          `json:"step_id"`
    IssueID   int64          `json:"issue_id"`
    ExecID    int64          `json:"exec_id,omitempty"`
    Type      SignalType     `json:"type"`
    Source    SignalSource   `json:"source"`
    Payload   map[string]any `json:"payload"`
    Actor     string         `json:"actor"`
    CreatedAt time.Time      `json:"created_at"`
}

type SignalType string
const (
    // Exec step signals
    SignalComplete  SignalType = "complete"    // Agent 完成任务
    SignalNeedHelp  SignalType = "need_help"   // Agent 需要人类/lead 协助
    SignalBlocked   SignalType = "blocked"     // Agent 被外部依赖阻塞
    SignalProgress  SignalType = "progress"    // Agent 上报中间进度（非终态）

    // Gate step signals
    SignalApprove   SignalType = "approve"     // Gate 通过
    SignalReject    SignalType = "reject"      // Gate 驳回

    // Human/system signals
    SignalUnblock   SignalType = "unblock"     // 人类解除阻塞
    SignalOverride  SignalType = "override"    // 人类强制覆盖结果
)

type SignalSource string
const (
    SignalSourceAgent  SignalSource = "agent"
    SignalSourceHuman  SignalSource = "human"
    SignalSourceSystem SignalSource = "system"
)
```

Gate 的 verdict 和 exec 的 completion 都是 StepSignal 的特化。一张表，统一查询、统一时间线。

### Store 接口

```go
CreateStepSignal(ctx context.Context, s *StepSignal) (int64, error)
GetLatestStepSignal(ctx context.Context, stepID int64, types ...SignalType) (*StepSignal, error)
ListStepSignals(ctx context.Context, stepID int64) ([]*StepSignal, error)
ListPendingHumanSteps(ctx context.Context, issueID int64) ([]*Step, error)
```

### 新增事件类型

```go
EventStepNeedHelp  EventType = "step.need_help"       // Agent 请求人类协助
EventStepUnblocked EventType = "step.unblocked"        // 人类解除阻塞
EventGateAwaitingHuman EventType = "gate.awaiting_human"
```

说明：

- `step.need_help`、`step.unblocked` 相关事件链路已有实现
- `gate.awaiting_human` 目前仍更接近预留设计，不应视为已完成的统一产品语义

## 二、Exec Step MCP 工具

### 工具列表

Gate step 执行时注入 gate 系列工具；exec step 执行时注入 exec 系列工具。

#### `step_complete` — 标记任务完成

```json
{
  "name": "step_complete",
  "description": "Declare that you have completed the task. Provide a structured summary of what you did. Call this BEFORE ending your response.",
  "inputSchema": {
    "type": "object",
    "properties": {
      "summary": {
        "type": "string",
        "description": "One-sentence summary of what was accomplished."
      },
      "files_changed": {
        "type": "array",
        "items": { "type": "string" },
        "description": "File paths that were created or modified."
      },
      "tests_passed": {
        "type": "boolean",
        "description": "Whether you ran tests and they passed."
      },
      "details": {
        "type": "string",
        "description": "Additional details about the changes, if needed."
      }
    },
    "required": ["summary"]
  }
}
```

**副作用**：写入 `StepSignal{type: "complete", payload: {summary, files_changed, tests_passed}}`

**引擎行为**：
- `handleSuccess` 看到 `SignalComplete` → 将 payload 直接作为 artifact metadata
- **跳过 LLM Collector**（agent 已提供结构化数据）
- 转 `StepDone`

#### `step_need_help` — 请求人类协助

```json
{
  "name": "step_need_help",
  "description": "Signal that you cannot complete the task and need human assistance. Explain what you tried, what went wrong, and what kind of help you need.",
  "inputSchema": {
    "type": "object",
    "properties": {
      "reason": {
        "type": "string",
        "description": "Why you cannot proceed. Be specific about what's blocking you."
      },
      "attempted": {
        "type": "string",
        "description": "What you already tried before giving up."
      },
      "help_type": {
        "type": "string",
        "enum": ["access", "clarification", "decision", "manual_action", "other"],
        "description": "What kind of help is needed."
      }
    },
    "required": ["reason"]
  }
}
```

**副作用**：写入 `StepSignal{type: "need_help", payload: {reason, attempted, help_type}}`

**引擎行为**：
- `handleSuccess` 看到 `SignalNeedHelp` → **不标记 Done**
- 转 `StepBlocked`（复用现有状态，等同于 `ErrKindNeedHelp` 的效果）
- 发布 `EventStepNeedHelp` → WebSocket → 前端弹通知
- 人类通过 `POST /steps/{id}/unblock` 处理

**关键**：这解决了"agent 正常结束但其实没完成"的问题。以前 session 正常退出 = 成功，现在 agent 可以显式说"我没完成"。

#### `step_context` — 获取执行上下文

```json
{
  "name": "step_context",
  "description": "Get the execution context: issue details, upstream step results, and your own rework history.",
  "inputSchema": {
    "type": "object",
    "properties": {},
    "required": []
  }
}
```

**返回值**：
```json
{
  "step": { "id": 3, "name": "implement", "position": 1, "retry_count": 0 },
  "issue": { "id": 1, "title": "fix login bug", "body": "..." },
  "upstream_artifacts": [],
  "rework_history": [
    { "attempt": 1, "reason": "test coverage < 80%", "at": "..." }
  ]
}
```

### 工具注入条件

```go
func buildStepMCPFactory(step *core.Step, profileID string, resolver ...) ... {
    switch step.Type {
    case core.StepGate:
        // 注入 gate env → mcp-serve 暴露 gate_approve/reject + step_context
        return withGateEnv(resolver, step)
    case core.StepExec:
        // 注入 exec env → mcp-serve 暴露 step_complete/need_help + step_context
        return withExecEnv(resolver, step)
    default:
        return nil
    }
}
```

环境变量：

| 变量 | 含义 |
|------|------|
| `AI_WORKFLOW_STEP_ID` | 当前 step ID（所有类型共用） |
| `AI_WORKFLOW_ISSUE_ID` | 当前 issue ID |
| `AI_WORKFLOW_STEP_TYPE` | `exec` / `gate` — mcp-serve 据此决定暴露哪些工具 |

`mcp-serve` 根据 `STEP_TYPE` 动态注册工具：

```go
switch os.Getenv("AI_WORKFLOW_STEP_TYPE") {
case "gate":
    register(gateApprove, gateReject, stepContext)
case "exec":
    register(stepComplete, stepNeedHelp, stepContext)
}
```

## 三、Gate Step MCP 工具

（保持前版设计，归纳到统一 StepSignal 模型）

#### `gate_approve` — 通过 Gate

```json
{
  "name": "gate_approve",
  "description": "Approve the gate. Call this when the review passes all criteria.",
  "inputSchema": {
    "type": "object",
    "properties": {
      "reason": {
        "type": "string",
        "description": "Why the gate passes. Be specific about what was verified."
      }
    },
    "required": ["reason"]
  }
}
```

**副作用**：写入 `StepSignal{type: "approve", payload: {reason}}`

#### `gate_reject` — 驳回 Gate

```json
{
  "name": "gate_reject",
  "description": "Reject the gate. Call this when the review finds issues that must be fixed.",
  "inputSchema": {
    "type": "object",
    "properties": {
      "reason": {
        "type": "string",
        "description": "What needs to be fixed. Be specific and actionable."
      },
      "reject_targets": {
        "type": "array",
        "items": { "type": "integer" },
        "description": "Step IDs to reset for rework. Omit to reset immediate predecessors."
      }
    },
    "required": ["reason"]
  }
}
```

**副作用**：写入 `StepSignal{type: "reject", payload: {reason, reject_targets}}`

## 四、引擎改造

### `handleSuccess` 改造（exec step）

```go
func (e *WorkItemEngine) handleSuccess(ctx context.Context, action *core.Action, run *core.Run) error {
    run.Status = core.RunSucceeded
    _ = e.store.UpdateRun(ctx, run)

    // 1. 检查 agent 是否通过 MCP 工具声明了 need_help
    helpSignal, _ := e.store.GetLatestActionSignal(ctx, action.ID, core.SignalNeedHelp)
    if helpSignal != nil && helpSignal.RunID == run.ID {
        // Agent 明确表示需要帮助 → 不标记 Done
        _ = e.transitionAction(ctx, action, core.ActionBlocked)
        e.bus.Publish(ctx, core.Event{
            Type:       core.EventActionNeedHelp,
            WorkItemID: action.WorkItemID,
            ActionID:   action.ID,
            RunID:      run.ID,
            Data:       helpSignal.Payload,
        })
        return nil  // 非引擎错误，其他 step 继续
    }

    // 2. 检查 agent 是否通过 MCP 工具提交了结构化完成信号
    completeSignal, _ := e.store.GetLatestActionSignal(ctx, action.ID, core.SignalComplete)
    if completeSignal != nil && completeSignal.RunID == run.ID {
        // Agent 提供了结构化 metadata → 直接写入 artifact，跳过 Collector
        e.applySignalMetadata(ctx, action, run, completeSignal.Payload)
    } else {
        // 3. 降级：LLM Collector 提取（现有行为）
        _ = e.collectMetadata(ctx, action)
    }

    switch action.Type {
    case core.ActionGate:
        return e.finalizeGate(ctx, action)
    default:
        return e.transitionAction(ctx, action, core.ActionDone)
    }
}
```

### `finalizeGate` 改造 — 评估链模式

`finalizeGate` 使用 **evaluator chain** 模式：一组 `GateEvaluator` 按顺序求值，首个返回 `Decided=true` 的评估器决定 gate 结果。

```go
// GateVerdict 表示单个评估器的输出
type GateVerdict struct {
    Decided  bool              // 是否已作出决定
    Passed   bool              // 通过/拒绝
    Reason   string
    ResetTo  []int64           // reject 时重置的上游 step IDs
    Metadata map[string]any    // 来源上下文 (art.Metadata / signal.Payload)
    Signal   *core.StepSignal  // 信号驱动的 verdict 携带原始信号
}

// GateEvaluator 评估函数签名
type GateEvaluator func(ctx context.Context, action *core.Action) (GateVerdict, error)

func (e *WorkItemEngine) finalizeGate(ctx context.Context, action *core.Action) error {
    evaluators := e.gateEvaluators // 可通过 WithGateEvaluators() 注入自定义链
    if len(evaluators) == 0 {
        evaluators = []GateEvaluator{
            e.evalSignalVerdict,    // 1. StepSignal (MCP / HTTP)
            e.evalManifestCheck,    // 2. Feature manifest
            e.evalDeliverableMetadata, // 3. Deliverable metadata (verdict field)
        }
    }
    for _, eval := range evaluators {
        v, err := eval(ctx, action)
        if err != nil { return err }
        if v.Decided {
            return e.applyGateVerdict(ctx, action, v)
        }
    }
    return e.applyGatePass(ctx, action, GateVerdict{}) // 默认 pass
}
```

**`applyGateVerdict`** 分发决定：pass → `applyGatePass`（含 merge PR）；reject → `processGateReject`。
**`applyGatePass`** 统一处理：merge PR（如果配置）→ 发事件 → transition done。

### 评估链优先级

```
evalSignalVerdict — StepSignal (MCP tool / HTTP API)
  │ Decided → applyGateVerdict (pass: merge+done, reject: rework)
  │ 未决 ↓
evalManifestCheck — Feature manifest entries
  │ Decided → applyGateVerdict
  │ 未决 ↓
evalDeliverableMetadata — Deliverable metadata verdict field
  │ Decided → applyGateVerdict
  │ 未决 ↓
默认行为 → applyGatePass (pass)
```

评估链可通过 `WithGateEvaluators()` 扩展，例如加入 CI 状态检查、外部审批系统等自定义评估器。

## 五、人类介入

Agent 和人类使用同一张 StepSignal 表，区别只在 `source` 字段。

### HTTP API

#### 提交人工 Gate 决策

```
POST /steps/{stepID}/decision
```

当前实现的请求体使用统一 `decision` 字段，而不是 `verdict`：

```json
{
  "decision": "approve",
  "reason": "code looks good"
}
```

→ 写入 `StepSignal{type: "approve", source: "human"}`
→ 触发后续 gate 评估链

#### 解除阻塞（exec 或 gate）

```
POST /steps/{stepID}/unblock
```

当前实现的请求体更接近：

```json
{
  "reason": "manually fixed the issue",
  "instructions": "retry with the new access"
}
```

→ 写入 `StepSignal{type: "unblock", source: "human"}`
→ 如带 `instructions`，还会额外写入 `SignalInstruction`
→ step 置回 `StepPending`

#### 查询待处理列表

```
GET /pending-decisions                     — 全局待处理列表
GET /pending-decisions?issue_id={issueID}  — 某 issue 下所有等待人类的 step
```

返回 `StepBlocked` + `StepWaitingGate` 的 step，附带最新上下文信号。

### GateMode 三模式

| 模式 | Agent MCP 工具 | 人类介入 | 适用场景 |
|------|---------------|---------|---------|
| `auto` | gate_approve / gate_reject | 仅当 max_retries 耗尽 | 常规 AI 审核 |
| `manual` | 不执行 agent（或仅信息收集） | 必须人类决策 | 合规审批、发布确认 |
| `hybrid` | gate_approve / gate_reject | reject 超阈值后升级 | AI 初审 + 人类兜底 |

## 六、mcp-serve 实现

### 新增子命令

```go
// cmd/ai-flow/main.go
case "mcp-serve":
    return appcmd.RunMCPServe(args[1:])
```

### 工具注册逻辑

```go
func RunMCPServe(args []string) error {
    dbPath   := os.Getenv("AI_WORKFLOW_DB_PATH")
    stepID   := envInt64("AI_WORKFLOW_STEP_ID")
    issueID  := envInt64("AI_WORKFLOW_ISSUE_ID")
    stepType := os.Getenv("AI_WORKFLOW_STEP_TYPE")

    store := sqlite.Open(dbPath)
    server := mcpserver.New(store, stepID, issueID)

    // 基础工具（所有 step 类型）
    server.Register("step_context", server.HandleStepContext)

    // 按 step 类型注册
    switch stepType {
    case "exec":
        server.Register("step_complete",  server.HandleStepComplete)
        server.Register("step_need_help", server.HandleStepNeedHelp)
    case "gate":
        server.Register("gate_approve", server.HandleGateApprove)
        server.Register("gate_reject",  server.HandleGateReject)
    }

    // 通用查询工具（原有）
    server.Register("query_projects", ...)
    server.Register("query_issues", ...)

    return server.ServeStdio()
}
```

### 幂等性

- 终态信号（complete / need_help / approve / reject）同一次执行只接受第一个
- progress 信号可多次发送
- 后续终态调用返回 `{ "status": "already_decided", "type": "..." }`

## 七、前端 UI（规划中，非当前现状）

以下内容主要是产品/UI 规划。当前仓库没有充分证据表明这些界面已经全部落地。

### 1. FlowDetailPage — 状态区分

| 状态 | 图标 | 含义 | 用户操作 |
|------|------|------|---------|
| `waiting_gate` | ⏳ | Gate 等待决策 | [通过] / [驳回] |
| `blocked` (need_help) | 🆘 | Agent 请求帮助 | [查看详情] → [解除阻塞] |
| `blocked` (max_retries) | 🚫 | 重试耗尽 | [重试] / [跳过] |

### 2. 阻塞详情面板

当 step 被 `step_need_help` 阻塞时，显示 agent 的求助信息：

```
🆘 Step "implement" 请求人工协助
    原因: "需要访问生产数据库的权限来验证 migration"
    已尝试: "在测试数据库上验证通过，但无法连接 prod"
    帮助类型: access

    [提供指引后重试]  [跳过此步骤]  [手动完成]
```

### 3. 信号时间线

统一展示所有 StepSignal，exec 和 gate 共用同一个 UI 组件：

```
Step "implement" 信号历史:
#1  Agent (worker)    complete   "实现了登录功能，修改了 3 个文件"    3min ago   [MCP]
--- 被 gate reject，进入 rework ---
#2  Agent (worker)    need_help  "需要 prod DB 访问权限"             1min ago   [MCP]
#3  alice@example.com unblock    "已开通权限，请重试"                 just now   [HTTP]

Step "code review" 信号历史:
#1  Agent (reviewer)  reject     "test coverage < 80%"               5min ago   [MCP]
#2  Agent (reviewer)  approve    "all checks pass"                   just now   [MCP]
```

### 4. Dashboard — 待处理汇总

```
待处理 (3)
├─ 🆘 Issue #12 / Step "implement" — Agent 请求帮助: "需要 prod DB 权限"
├─ ⏳ Issue #15 / Step "code review" — 等待人工审批 (AI 2次 reject 后升级)
└─ 🚫 Issue #8  / Step "deploy" — 重试耗尽 (3/3)
```

## 八、完整流程示例

```
Issue "fix login bug" → Run
  │
  Step 1: "implement" (exec)
  │   Agent 连接 MCP，看到: step_complete, step_need_help, step_context
  │   Agent 调用 step_context() → 获取 issue body
  │   Agent 修改代码、跑测试
  │   Agent 调用 step_complete({
  │     summary: "Added login validation + unit tests",
  │     files_changed: ["src/auth.go", "src/auth_test.go"],
  │     tests_passed: true
  │   })
  │   → StepSignal 写入 DB
  │   → handleSuccess: 读到 SignalComplete → 跳过 Collector → StepDone ✓
  │
  Step 2: "code review" (gate, mode=hybrid, escalate_after=2)
  │   Agent 连接 MCP，看到: gate_approve, gate_reject, step_context
  │   Agent 调用 step_context() → 获取 Step 1 的 artifact
  │   Agent 调用 gate_reject({reason: "test coverage < 80%"})
  │   → ProcessGate(rejected) → Step 1 reset
  │
  Step 1: rework (retry_count=1)
  │   Agent 调用 step_context() → 看到 rework_history
  │   Agent 补充测试，但发现需要 prod DB 权限验证
  │   Agent 调用 step_need_help({
  │     reason: "need prod DB access to verify migration",
  │     attempted: "verified on test DB, but prod has different schema",
  │     help_type: "access"
  │   })
  │   → handleSuccess: 读到 SignalNeedHelp → StepBlocked
  │   → EventStepNeedHelp → WebSocket → 前端通知
  │
  人类介入:
  │   前端显示阻塞详情
  │   POST /steps/1/unblock {"action":"retry","reason":"已开通权限"}
  │   → StepSignal{type: "unblock"} → step → StepPending
  │
  Step 1: rework (retry_count=1, 重新调度)
  │   Agent 完成任务
  │   Agent 调用 step_complete({summary: "migration verified on prod"})
  │   → StepDone ✓
  │
  Step 2: re-evaluate
  │   Agent 调用 gate_approve({reason: "all tests pass, coverage 92%"})
  │   → ProcessGate(passed) → merge PR → StepDone ✓
  │
  Issue → Done ✓
```

## 九、向后兼容

| 场景 | 行为 |
|------|------|
| Agent 调用了 MCP 工具 | 优先使用 StepSignal |
| Agent 未调用工具，输出含 `AI_WORKFLOW_GATE_JSON` | 降级正则提取（gate only） |
| Agent 未调用工具，无标记行 | 降级 LLM Collector |
| 所有路径都没结果 | 默认行为（exec: Done, gate: pass） |

优先级：**StepSignal > 正则提取 > LLM Collector > 默认**

## 十、实现优先级（按原方案保留，需结合当前状态理解）

| 阶段 | 内容 | 工作量 |
|------|------|--------|
| **P0** | `StepSignal` 模型 + Store + SQLite 表 | 已实现 |
| **P0** | `mcp-serve` 子命令：exec 工具 (step_complete, step_need_help) + gate 工具 (gate_approve, gate_reject) + step_context | 已实现 |
| **P0** | `handleSuccess` 改造：检查 SignalNeedHelp / SignalComplete | 已实现 |
| **P0** | `finalizeGate` 改造：检查 SignalApprove / SignalReject | 已实现主体 |
| **P0** | `buildStepMCPFactory` 改造：按 step type 注入 env | 已实现 |
| **P1** | `POST /steps/{id}/decision` + `POST /steps/{id}/unblock` API | 已实现 |
| **P1** | `EventStepNeedHelp` 事件 + WebSocket 广播 | 已实现主体 |
| **P1** | 前端审批/阻塞面板 | 未确认已实现 |
| **P2** | `hybrid` gate mode（AI + 升级阈值） | 中 |
| **P2** | Dashboard 待处理汇总面板 | 未确认已实现 |
| **P2** | 信号时间线 UI | 未确认已实现 |
| **P3** | Webhook 通知（Slack/飞书） | 中 |

## 十一、设计原则

- **Agent 主动声明，引擎不猜**：所有 step 类型的 agent 都通过 MCP 工具显式表达意图
- **统一信号模型**：exec 和 gate 共用 StepSignal 表 + 时间线 UI，AI 和人类共用同一管道
- **向后兼容**：StepSignal 优先，无则降级到现有行为（Collector / 正则 / 默认）
- **幂等安全**：终态信号同一次执行只接受第一个
- **最小侵入**：`ProcessGate()` 完全不变，`handleFailure()` 完全不变；`finalizeGate()` 重构为 evaluator chain 模式，支持 `WithGateEvaluators()` 自定义扩展

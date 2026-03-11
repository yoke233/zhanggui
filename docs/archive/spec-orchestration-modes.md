# 编排模式规范：Pipeline Mode + Collaboration Mode

> **Status:** 设计阶段，未实现。
>
> 本文定义 ai-workflow 的双编排模式：固定流水线（系统驱动）与动态协作（Agent 驱动）并行共存。

---

## 1. 问题：固定流程的天花板

当前系统所有编排决策由系统代码做出：

```
DepScheduler       → 决定何时派发
EventHandlers      → 决定如何反应（auto-merge, decompose, retry）
Executor           → 决定 stage 顺序（setup → implement → review → merge）
TL                 → 一组确定性事件处理器，没有推理循环
Agent              → 在 stage 边界内执行，不知道其他 Agent 的存在
```

这在任务明确时运转良好。但存在结构性限制：

| 限制 | 场景 |
|------|------|
| Agent 不能通信 | Worker 发现 spec 有矛盾，无法告诉 TL |
| TL 不能中途调整 | 执行过程中发现需要加一个探索性任务，做不到 |
| 流程不可变 | 每个 Issue 必须走完固定 stage 链，即使某些 stage 不需要 |
| 无动态委派 | TL 不能在对话中创建 Issue 或委派 Agent |

随着 AI 能力增强，固定流程从"可靠的脚手架"变成"不必要的约束"。但固定流程的价值——可审计、可预测、资源可控——不会消失。

**解决方案：两种模式共存，共享基础设施。**

---

## 2. 双模式架构

```
┌─────────────────────────────────────────────────────┐
│                  Orchestration Layer                  │
│                                                      │
│  ┌────────────────┐       ┌─────────────────────┐   │
│  │ Pipeline Mode   │       │ Collaboration Mode   │   │
│  │ (系统驱动)       │       │ (Agent 驱动)          │   │
│  │                 │       │                      │   │
│  │ DepScheduler    │       │ TL Agent             │   │
│  │ EventHandlers   │       │ + Action MCP Tools   │   │
│  │ 固定 Stage 链    │       │ + 动态委派 / 通信      │   │
│  └───────┬────────┘       └──────────┬───────────┘   │
│          │                           │               │
│          └──────────┬────────────────┘               │
│                     ▼                                │
│           Shared Infrastructure                      │
│  ┌──────────────────────────────────────────────┐   │
│  │  Store  │ EventBus │ Executor │ MCP Server   │   │
│  │  Issue  │ Run │ Event │ ACP Session │ Git    │   │
│  └──────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────┘
```

### 核心洞察

**Pipeline Mode 是 Collaboration Mode 的一个子程序。** TL 在 Collaboration Mode 下可以选择把明确任务交给 Pipeline 自动处理。两者不是对立关系，是包含关系。

---

## 3. Pipeline Mode（现有，保留）

系统驱动的确定性流程。当前实现完整，无需改动。

```
Issue → queued → ready → executing → Run(stages) → review → merge → done
                    ↑                                              │
                    └──── DepScheduler 自动推进 ──────────────────┘
```

**特征：**
- 决策者：DepScheduler + EventHandlers（Go 代码）
- Agent 能力：stage 内完全自主，stage 间无通信
- 可预测性：高——每个 Issue 走固定路径
- 适用场景：明确任务、CI/CD 集成、合规审计、批量执行

**保留原因：**
- 可审计——每一步状态变迁有记录，满足合规需求
- 可预测——资源消耗有上限，不会因 Agent 失控跑飞
- 安全门——人类审批节点是硬约束，不可被 Agent 绕过
- 外部集成——GitHub webhook、CI pipeline 需要确定性触发点

---

## 4. Collaboration Mode（新增）

Agent 驱动的动态编排。TL 从"事件处理器"升级为"推理循环"。

```
Human ←→ TL Agent (长期 ACP session)
              │
              ├── context_overview()      ← 查项目知识
              ├── memory_search()         ← 查历史经验
              ├── create_issue()          ← 创建任务（可选交给 Pipeline）
              ├── delegate_to_agent()     ← 动态委派 Agent
              ├── send_instruction()      ← 向 Agent 发送指令
              ├── subscribe_and_wait()    ← 等一组任务完成
              └── approve_issue()         ← 审批决策
```

**特征：**
- 决策者：TL Agent（LLM 推理）
- Agent 能力：可通信、可动态委派、可读取其他 Agent 状态
- 可预测性：中——TL 有推理能力但也有不确定性
- 适用场景：探索性任务、复杂跨项目协调、需要人机深度交互

### 4.1 TL 角色升级

从确定性事件处理器升级为持久推理循环：

```
现有（Pipeline Mode）：
  EventBus → TLTriageHandler()          确定性 Go 代码
  EventBus → DecomposeHandler()         确定性 Go 代码
  EventBus → ChildCompletionHandler()   确定性 Go 代码

升级后（Collaboration Mode）：
  EventBus → TL Agent ACP Session       LLM 推理决策
  TL 收到事件 → 自行判断如何反应 → 通过 Action Tools 执行
```

TL 变成一个**长期运行的 ACP session**。不是每个事件触发一次 LLM 调用，而是维持一个持续的对话上下文，在其中接收事件、推理、行动。

### 4.2 Action MCP Tools（核心新增）

当前 TL 只有 `query_*` 工具（只读）。Collaboration Mode 需要写操作：

| 分类 | 工具 | 说明 |
|------|------|------|
| **Issue 管理** | `create_issue` | 创建新 Issue（可指定模板、标签、依赖） |
| | `approve_issue` | 审批通过（交给 Pipeline 执行） |
| | `reject_issue` | 驳回并附注原因 |
| | `abandon_issue` | 放弃 Issue |
| **Agent 委派** | `delegate_to_agent` | 动态启动一个 Agent 执行特定任务 |
| | `get_agent_status` | 查看委派 Agent 的当前状态 |
| | `send_instruction` | 向运行中的 Agent 发送指令 |
| **事件等待** | `subscribe_and_wait` | 等待一组条件满足后回调（wait-group 语义） |
| **知识查询** | `context_overview` | L1 项目概览（来自 §12 Context & Memory） |
| | `memory_search` | 经验召回 |

**与现有 query 工具的关系：** Action Tools 是新增层，现有 `query_issues` / `query_runs` 等只读工具保留不变。

### 4.3 动态委派 vs Pipeline 派发

两种方式创建和执行任务：

```
Pipeline 派发（现有）：
  create_issue() → approve_issue() → DepScheduler 自动调度
  → Executor 走完 setup → implement → review → merge
  → 适合明确、完整的开发任务

动态委派（新增）：
  delegate_to_agent(role, prompt, tools)
  → 直接启动一个 ACP session，不创建 Issue/Run
  → Agent 完成后通过 report_to_parent 回报
  → 适合探索性、一次性、轻量级任务
```

动态委派不经过 Issue 状态机和 Stage 链——它是"叫一个人帮你干个活"，不是"走一套正式流程"。

### 4.4 Agent 间通信

分阶段引入：

**Phase 1：单向汇报（TL ← Agent）**

```
Agent 完成任务 → report_to_parent(result)
  → EventBus 发布 AgentReportEvent
  → TL ACP session 收到事件
  → TL 推理下一步
```

**Phase 2：双向通信（TL ↔ Agent）**

```
Agent 遇到问题 → ask_parent("spec 有矛盾，选 A 还是 B？")
  → TL 推理 → send_instruction(agent_id, "选 A，因为...")
  → Agent 继续执行
```

**Phase 3：Agent 间感知（Agent → Agent，需要时）**

```
Agent A → read_agent_conversation(agent_b_id)
  → 了解 Agent B 做了什么，避免重复工作
```

Phase 1 和 Phase 2 覆盖绝大多数场景。Phase 3 在 Agent 数量多、需要感知彼此进度时才需要。

---

## 5. 模式选择与共存

### 5.1 模式选择

```yaml
orchestration:
  mode: auto           # auto | pipeline | collaborate
```

| 模式 | 行为 |
|------|------|
| `pipeline` | 所有 Issue 走固定流程，TL 是确定性事件处理器（当前行为） |
| `collaborate` | TL 是持久推理循环，动态决策编排方式 |
| `auto` | TL 自己判断——简单任务交 Pipeline，复杂任务自己编排 |

### 5.2 同一时刻并行共存

```
Session 1（来自 A2A 外部调用）：
  Issue-001 → Pipeline Mode → executing → review → merge
  Issue-002 → Pipeline Mode → queued → ready

Session 2（来自 Human 对话）：
  TL ACP Session 运行中（Collaboration Mode）
  ├── 已创建 Issue-003，交给 Pipeline 执行
  ├── 动态委派了 Agent-X 做探索（不走 Pipeline）
  └── 等 Agent-X 回报后决定下一步
```

两种模式共享：
- 同一个 Store（Issue / Run 都在 SQLite）
- 同一个 EventBus（事件互通）
- 同一个 Executor（ACP 执行相同）
- 同一个 MCP Server（工具集统一）

### 5.3 Collaboration Mode 下 TL 的典型行为

```
Human: "重构 project A 的认证系统"

TL 推理：这个很复杂，先了解现状
→ context_overview("project-a/src/auth/")      查项目知识
→ memory_search("auth refactor")                查历史经验

TL 推理：需要分 3 步，前两个明确，第三个需要先探索
→ create_issue("抽取 JWT middleware", ...)       明确任务
→ create_issue("统一 session 管理", ...)         明确任务
→ approve_issue(issue-1)                         交给 Pipeline
→ approve_issue(issue-2)                         交给 Pipeline
→ delegate_to_agent("researcher", "探索 OAuth 集成方案")   动态委派

Pipeline 接管 issue-1, issue-2 的执行
探索 Agent 独立运行

探索 Agent → report_to_parent("建议用 OIDC，原因...")
TL 推理：好，基于探索结果创建第三个任务
→ create_issue("实现 OIDC 集成", acceptance_criteria=[...])
→ approve_issue(issue-3)

TL → Human: "三个子任务已安排，预计..."
```

---

## 6. Issue 模型增强

为支持 Collaboration Mode 下更精确的任务描述和验收，Issue 模型新增结构化字段：

### 6.1 新增字段

```go
type Issue struct {
    // ... 现有字段 ...

    // 结构化验收条件（Collaboration Mode 使用）
    AcceptanceCriteria   []string `json:"acceptance_criteria,omitempty"`
    VerificationCommands []string `json:"verification_commands,omitempty"`

    // 编排模式标记
    OrchestrationMode string `json:"orchestration_mode,omitempty"` // "pipeline" | "collaborate" | ""(继承全局)
}
```

### 6.2 AcceptanceCriteria

Decomposer 或 TL 拆任务时定义，是"什么算完成"的结构化表达：

```json
{
  "acceptance_criteria": [
    "API 返回 JWT token，包含 user_id 和 exp",
    "密码使用 bcrypt hash，cost >= 12",
    "登录失败 5 次锁定 15 分钟",
    "所有端点有 rate limiting"
  ]
}
```

**用途：**
- Reviewer 按条目逐一检查，不遗漏
- TL 收到 review 结果后，能判断哪些条目未满足
- 比 `Issue.Body` 中的自然语言描述更精确

### 6.3 VerificationCommands

可自动执行的验证命令，Run 完成后由系统执行：

```json
{
  "verification_commands": [
    "go test ./internal/auth/...",
    "curl -s http://localhost:8080/api/v3/auth/login -d '{\"email\":\"test@test.com\"}' | jq .token"
  ]
}
```

**执行时机：** Executor 在 `implement` stage 完成后、`review` stage 之前自动执行。结果附加到 Run Events 中供 Reviewer 参考。

---

## 7. EventBus 增强

为支持 Collaboration Mode 下的异步协调，EventBus 增加订阅语义：

### 7.1 Wait-Group

等一组条件全部满足后触发回调：

```go
// 等所有子 Issue 完成后通知 TL
bus.WaitAll(ctx, []EventMatcher{
    {Type: EventIssueDone, IssueID: child1.ID},
    {Type: EventIssueDone, IssueID: child2.ID},
    {Type: EventIssueDone, IssueID: child3.ID},
}, func(events []Event) {
    // 所有子 Issue 完成，通知 TL 汇总
})
```

替代现有 `ChildCompletionHandler` 中的手动计数逻辑。

### 7.2 One-Shot

订阅一次事件后自动注销：

```go
bus.Once(ctx, EventMatcher{Type: EventRunDone, RunID: runID},
    func(e Event) {
        // Run 完成，处理结果
    })
```

### 7.3 Pre-Subscribe

先订阅再触发动作，避免竞态丢事件：

```go
// 先订阅 → 再创建 Issue → 不会丢掉创建后立即发生的状态变更
sub := bus.PreSubscribe(ctx, EventMatcher{Type: EventIssueStatusChanged, IssueID: issueID})
store.CreateIssue(issue)
event := <-sub.C  // 安全接收
```

---

## 8. Role 与 Specialist 增强

受 Routa 的 Specialist 设计启发，Role 从纯配置升级为有运行时语义的一等实体：

### 8.1 行为边界（RoleReminder）

每个 Role 声明"不做什么"，注入 ACP session 的 system prompt：

```yaml
roles:
  team_leader:
    agent: claude
    role_reminder: |
      你是 Team Leader。你的职责是规划、拆解、委派和汇总。
      你不直接写实现代码。如果需要实现，创建 Issue 或委派 Agent。
    # ...

  worker:
    agent: codex
    role_reminder: |
      你是 Worker。你的职责是按 Issue 要求完成实现。
      你不扩大任务范围。如果发现需要额外工作，通过 report_to_parent 汇报。
    # ...
```

**价值：** 防止角色塌缩——TL 不会自己去写代码，Worker 不会自己去改 spec。

### 8.2 模型分层（ModelTier）

显式声明角色使用的模型级别：

```yaml
roles:
  team_leader:
    model_tier: smart          # 需要强推理能力
  worker:
    model_tier: fast           # 需要快速执行
  reviewer:
    model_tier: smart          # 需要深度理解
```

`smart` / `fast` 映射到具体模型由 Agent Profile 决定，不在 Role 层硬编码。

**价值：** FinOps——规划者用昂贵模型（Claude Opus），执行者用快速模型（Claude Sonnet / Codex），成本分层优化。

### 8.3 Skill 复用层（预留）

Skill = 外部分发的可复用能力包，独立于角色，可被多个角色引用。

#### Skill 存储

Skill 以 GitHub 仓库为分发载体。一个仓库的 `skills/` 目录下有多个子目录，每个子目录是一个独立 Skill：

```
github.com/company/ai-workflow-skills        ← 一个 Skill 仓库
├── skills/
│   ├── code-review/                         ← 一个 Skill
│   │   ├── skill.yaml                       #   元数据：名称、描述、所需工具、参数
│   │   ├── prompt.md                        #   prompt 模板
│   │   └── examples/                        #   用例（可选）
│   ├── dependency-analysis/
│   │   ├── skill.yaml
│   │   └── prompt.md
│   └── security-audit/
│       ├── skill.yaml
│       └── prompt.md
```

`skill.yaml` 示例：

```yaml
name: code-review
description: "审查代码变更的质量、安全性和一致性"
version: "1.2.0"
required_tools: [fs_read]                     # Skill 运行需要的工具权限
parameters:                                   # 可配置参数
  severity_threshold:
    type: string
    default: "warning"
    enum: [info, warning, error]
  language:
    type: string
    default: "auto"
```

#### Skill 引用

Role 通过 `owner/repo/skill-name@version` 引用 Skill：

```yaml
roles:
  reviewer:
    skills:
      - ref: "company/ai-workflow-skills/code-review@v1.2.0"
      - ref: "company/ai-workflow-skills/security-audit@v1.0.0"
        params:
          severity_threshold: error

  decomposer:
    skills:
      - ref: "company/ai-workflow-skills/dependency-analysis@v2.0.0"

  team_leader:
    skills:                                    # TL 可复用多个 Skill
      - ref: "company/ai-workflow-skills/code-review@v1.2.0"
      - ref: "company/ai-workflow-skills/dependency-analysis@v2.0.0"
```

版本可以是：
- 语义化版本 `@v1.2.0`（推荐生产环境）
- commit hash `@abc1234`（精确锁定）
- 分支名 `@main`（开发环境）

#### Skill 管理中心（预留）

生产环境需要 Skill 治理：

| 能力 | 说明 |
|------|------|
| **注册与发现** | 浏览可用 Skill，按标签/能力搜索 |
| **版本锁定** | 锁定文件记录每个 Skill 的精确版本（类似 `go.sum`），保证可复现 |
| **审批流程** | 新 Skill 或版本升级需要审批后才能在生产环境使用 |
| **审计日志** | 记录哪个 Role 在什么时候使用了哪个版本的 Skill |
| **缓存与分发** | 本地缓存已下载的 Skill，避免每次从 GitHub 拉取 |

这部分复杂度较高，当前阶段预留设计位置，不实现。初期可以直接将 Skill 内容内联到 `configs/` 目录中使用。

#### 与现有 prompt_templates 的演进关系

```
现在：prompt_templates/implement.tmpl → Role 1:1 绑定
      （简单直接，角色少时够用）

未来：skills/code-review/prompt.md → 多个 Role 复用
      （Skill 是 prompt_template 的外部化、版本化、可组合升级）
```

> Skill 层为预留设计。当前 `prompt_template` 1:1 绑定方式在角色数量不多时仍然可行。当角色和能力增多、需要跨项目/跨组织复用时，再引入完整 Skill 体系。

---

## 9. Trace 审计增强

当前 events 表是松散的 JSON blob。为支持协作审计和调试回放，定义统一的 Trace schema：

### 9.1 Trace 字段

```go
type Trace struct {
    ID          string    `json:"id"`
    Timestamp   time.Time `json:"timestamp"`

    // 谁
    AgentRole   string    `json:"agent_role"`              // team_leader / worker / reviewer
    AgentID     string    `json:"agent_id"`                // ACP session ID
    SessionID   string    `json:"session_id,omitempty"`    // 所属对话 session

    // 什么
    EventType   string    `json:"event_type"`              // tool_call / stage_start / stage_end / delegation / report / decision
    Action      string    `json:"action"`                  // 具体动作：create_issue / approve / delegate / ...
    Input       any       `json:"input,omitempty"`         // 动作输入
    Output      any       `json:"output,omitempty"`        // 动作结果

    // 上下文
    IssueID     string    `json:"issue_id,omitempty"`
    RunID       string    `json:"run_id,omitempty"`
    ParentTrace string    `json:"parent_trace,omitempty"`  // 父 Trace ID（委派链）
    Branch      string    `json:"branch,omitempty"`        // Git 分支
    Files       []string  `json:"files,omitempty"`         // 涉及文件
}
```

### 9.2 用途

- **调试回放**：按时间线重放 TL 的决策链——它为什么创建了这个 Issue？它看了什么上下文？
- **协作审计**：跨 Agent 的委派链完整记录——谁委派了谁，结果是什么
- **性能分析**：每个 Agent 的耗时、token 消耗、重试次数

---

## 10. 实现阶段

### Phase 1：TL Action Tools（最小可用）

给 TL 加写操作 MCP 工具，让它能主动操作系统。

| 改动 | 说明 |
|------|------|
| `internal/mcpserver/tools_action.go` | 新增 `create_issue` / `approve_issue` / `reject_issue` / `abandon_issue` |
| `internal/mcpserver/server.go` | 注册 Action Tools，权限检查（只有 TL 角色可用） |
| `configs/defaults.yaml` | team_leader 角色声明 action tools 权限 |

**验收标准：** TL 在 ACP session 中可以通过 MCP 工具创建 Issue 并触发 Pipeline 执行。

### Phase 2：动态委派 + 单向汇报

| 改动 | 说明 |
|------|------|
| `internal/mcpserver/tools_delegation.go` | `delegate_to_agent` / `get_agent_status` 工具 |
| `internal/engine/delegation.go` | 动态 ACP session 创建（不经过 Issue/Run） |
| `internal/mcpserver/tools_report.go` | `report_to_parent` 工具（所有角色可用） |
| EventBus 增强 | `WaitAll` / `Once` / `PreSubscribe` |

**验收标准：** TL 可以动态委派一个 Agent 做探索性任务，Agent 完成后结果回传到 TL session。

### Phase 3：Issue 模型增强

| 改动 | 说明 |
|------|------|
| `internal/core/issue.go` | 新增 `AcceptanceCriteria` / `VerificationCommands` 字段 |
| Store 迁移 | issues 表加 `acceptance_criteria` / `verification_commands` 列 |
| `internal/engine/executor.go` | implement 完成后自动执行 VerificationCommands |
| Decomposer prompt | 拆任务时生成结构化验收条件 |

### Phase 4：Role 增强 + Trace

| 改动 | 说明 |
|------|------|
| `configs/defaults.yaml` | 每个角色加 `role_reminder` / `model_tier` |
| `internal/acpclient/role_resolver.go` | 注入 role_reminder 到 system prompt |
| `internal/core/trace.go` | Trace 结构定义 |
| Store 迁移 | 新增 `traces` 表 |
| TL Action Tools | 每次调用自动写 Trace |

### Phase 5（预留）：双向通信 + Skill 层

- `ask_parent` / `send_instruction` 工具
- `read_agent_conversation` 工具
- Skill 配置层 + 多角色复用

---

## 11. 关键设计决策

| 决策 | 选择 | 理由 |
|------|------|------|
| 两模式共存 vs 替换 | 共存 | Pipeline 的审计/安全价值不可替代 |
| TL 实现方式 | 长期 ACP session | 需要持续上下文推理，不是无状态函数调用 |
| 动态委派 vs 全走 Issue | 两者并存 | 探索性任务不需要 Issue 的完整生命周期 |
| Action Tools 权限 | 仅 TL 可用 | 防止 Worker 自己创建 Issue 导致失控 |
| Agent 通信方式 | MCP 工具 + EventBus | 复用现有基础设施，不引入新协议 |
| Trace 存储 | SQLite 表 | 与现有 Store 统一，不引入新存储 |

---

## 12. 与其他规范的关系

| 规范 | 关系 |
|------|------|
| `spec-v3-system.md` §4 Scheduling | Pipeline Mode 对应现有 DepScheduler 逻辑，不改动 |
| `spec-v3-system.md` §7 Agent Configuration | Role 增强（§8）扩展现有 Role Profile |
| `spec-v3-system.md` §12 Context & Memory | Collaboration Mode 下 TL 的 context/memory 工具来自此规范 |
| `spec-distributed-deployment.md` §4 TL Hybrid | 本地 TL 在 Collaboration Mode 下天然兼容（通过 REST 调用 Action Tools） |
| `spec-context-memory.md` | TL 在 Collaboration Mode 下是 OpenViking 的主要消费者 |

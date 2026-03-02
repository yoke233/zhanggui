# Secretary Layer — 设计文档

> 本文档是 [spec-overview.md](spec-overview.md) 中 Secretary Layer 的详细设计。整体架构、插件体系、目录结构、设计原则、实施分期见总览文档。API 端点、WebSocket 协议、SQL Schema、配置 YAML 见 [spec-api-config.md](spec-api-config.md)。Event Bus 事件定义见 [spec-pipeline-engine.md](spec-pipeline-engine.md) Section VII。ACP Client 接口见 [spec-agent-drivers.md](spec-agent-drivers.md)。

## 概述

Secretary Layer 是系统的上层编排层，位于 Orchestrator Core 之上。它负责：通过持久交互式 Agent session 与用户多轮对话理解需求、由用户指示在项目中生成计划文件、经用户勾选后由 AI 解析为结构化子任务、通过 Multi-Agent 审核委员会自动审核纠错、用 DAG Scheduler 管理子任务间的依赖并行调度。每个子任务 1:1 对应一个 Pipeline，由现有 Pipeline Engine 执行。

**关键设计决策**：Secretary Agent 是一个持久运行的交互式 session（而非一次性 LLM 调用），工作目录为项目目录，拥有文件读写权限。对话过程中不自动生成 Plan，而是由用户显式指示 Secretary 生成计划文件，文件格式由 Secretary 自由决定。

## 一、Secretary Agent

### 职责

Secretary Agent 是用户的入口。用户通过持久交互式 session 与 Secretary 多轮对话，Secretary 作为一个拥有项目文件权限的 AI Agent，帮助用户理解代码、探索项目、讨论需求。当需求明确后，用户指示 Secretary 在项目中生成计划文件，后续由用户勾选文件创建 TaskPlan。

**Secretary 不自动生成 Plan。** 对话只是对话，Plan 的创建是一个独立的、由用户发起的动作。

### 实现方式 — ACP 持久 Session

Secretary Agent 通过 ACP Client 建立持久多轮 session，天然支持对话上下文保持。通信链路统一使用 ACP `session/new`、`session/prompt`、`session/load` 语义，不再维护自定义持久 stdin/stdout 交互：

```
项目打开时:
  → acpClient, err = acpclient.New(LaunchConfig{
        Command: agentConfig.LaunchCommand,
        Args:    agentConfig.LaunchArgs,
        Env:     agentConfig.Env,
    }, handler)
  → acpClient.Initialize(ctx, clientCaps)
  → session, err = acpClient.NewSession(ctx, NewSessionRequest{
        CWD:        project.RepoPath,
        MCPServers: mcpServers,
    })
      // mcpServers 包含内嵌 MCP Server（暴露查询工具）
  → session 保持运行, 等待用户消息

用户每条消息:
  → result = acpClient.Prompt(ctx, PromptRequest{
        SessionID: session.ID,
        Prompt:    userMessage,
    })
  → 接收 session/update 事件:
      ├── text → WebSocket 流式推送到前端
      ├── tool_call → Agent 调用工具（ACP Handler 处理）
      └── end_turn → 本轮结束
  → Agent 写文件时:
      → ACP 触发 fs/write_text_file → ACPHandler.HandleWriteFile 回调
      → 后端记录变更文件 → 发 secretary_files_changed 事件

项目关闭 / 空闲超时:
  → acpClient.Close(ctx)
  → 对话历史已持久化到 Store
  → 项目重开时优先 acpClient.LoadSession(ctx, LoadSessionRequest{SessionID: session.ID, CWD: project.RepoPath}) 恢复（Agent 支持时）
  → LoadSession 不可用或恢复失败时再 NewSession
```

### 角色绑定（Secretary Role）

Secretary 通过角色绑定获取 Agent 与运行策略，默认绑定 `secretary` 角色：

```yaml
role_bindings:
  secretary:
    role: secretary
```

其中 `roles.secretary.agent` 指向具体 Agent（如 `claude` / `codex`），因此切换 Agent 只需改角色定义，不需要改 Secretary 业务逻辑。

### 权限配置

Secretary Agent 的权限统一走 ACP permission 模型（capability + request_permission），并通过角色配置下发：

1. **Capability Gate**：`initialize` 时协商 fs_read/fs_write/terminal 能力
2. **Scope**：`session/new` 的 cwd（项目目录）定义文件操作边界
3. **Runtime**：`session/request_permission` 按需授权

```yaml
roles:
  - name: secretary
    agent: claude
    capabilities:
      fs_read: true
      fs_write: true
      terminal: true
    permission_policy:
      - pattern: "fs/write_text_file"
        scope: "cwd"
        action: "allow_always"      # cwd 内写文件自动放行
      - pattern: "terminal/create"
        action: "allow_once"        # 终端操作逐次确认
```

安全边界由 ACP scope 机制保证：Agent 只能操作 cwd 范围内的文件，无需手动维护工具白名单。

### 查询工具

Secretary Agent 在对话中可通过内嵌 MCP Server 查询系统状态：

| 工具名 | 功能 | 返回 |
|--------|------|------|
| `query_plans` | 列出当前项目所有 TaskPlan | ID, name, status, task count |
| `query_plan_detail` | 查看某个 Plan 详情 | tasks, DAG, review status |
| `query_pipelines` | 列出项目下活跃 Pipeline | ID, status, current stage, progress |
| `query_pipeline_logs` | 查看某 Pipeline 的日志 | 最近 N 条日志 |
| `query_project_stats` | 项目统计 | 总 pipeline 数, 成功率, token 消耗 |

实现架构：

```
后端启动内嵌 MCP Server，通过 session/new 的 mcpServers 参数注册：

session, err := acpClient.NewSession(ctx, NewSessionRequest{
    CWD: projectDir,
    MCPServers: []MCPServerConfig{
        {Name: "workflow-query", Command: "internal", Tools: queryTools},
    },
})

Agent 通过标准 MCP 协议调用查询工具：
  Agent 发现 MCP Server 提供的工具列表
  → Agent 调用 MCP tool (如 query_plans)
  → MCP Server 执行查询 (调 Store 接口)
  → 结果通过 MCP 协议返回 Agent
  → Agent 继续生成回复

查询调用走 ACP + MCP 标准链路，不再从 Agent stdout 解析 `tool_use` 事件，也不需要 stdin 注入。
```

查询工具是**只读**的，不修改任何系统状态。工具调用和结果均记录到审计日志。

### 对话管理

对话以 ChatSession 为单位存储在 SQLite 中：

```go
type ChatSession struct {
    ID        string
    ProjectID string
    AgentSessionID string   // 最近一次可恢复的 ACP session ID（用于 LoadSession 优先恢复）
    Messages  []ChatMessage
    CreatedAt time.Time
    UpdatedAt time.Time
}

type ChatMessage struct {
    Role    string    // "user" | "assistant"
    Content string
    Time    time.Time
}
```

规则：
- 每个项目可以有多个 ChatSession（不同需求讨论）
- ChatSession 和 TaskPlan 是 **1:N** 关系（一个长期对话可产出多个计划，每个计划独立审核和执行）
- 对话历史持久化到 SQLite，支持断线续聊
- Agent session 是运行时状态。项目重开/断线重连时优先通过 `acpClient.LoadSession(ctx, LoadSessionRequest{SessionID: sessionID, CWD: cwd})` 恢复已有 session（Agent 支持时）；仅在不支持或恢复失败时才 `acpClient.NewSession(ctx, NewSessionRequest{...})` 新建 session
- 空闲超时后自动关闭 session（默认 30 分钟，可配置）

> 对应的 SQL schema 见 [spec-api-config.md](spec-api-config.md) Section IV。ID 生成规则见 Section V。

## 二、TaskPlan 数据模型

### 计划文件 → TaskPlan 流程

TaskPlan 的创建不再由 Secretary Agent 直接输出 JSON，而是基于文件驱动：

```
1. 用户在 Chat 中指示 Secretary 生成计划文件
   └── Secretary 在项目目录写入文件（格式自由：.md / .json / .yaml / 混合）
       推荐但不强制写入 .ai-workflow/plans/ 目录

2. 后端检测到 Secretary 写文件（由 ACP Handler 的 `HandleWriteFile` 回调触发）
   └── 发 WebSocket 事件 secretary_files_changed { file_paths, session_id }

3. 前端展示变更文件列表，用户勾选哪些文件作为 Plan 输入

4. 用户提交 → POST /api/v1/projects/:pid/plans/from-files
   Body: { session_id, file_paths: ["plan-auth.md", "plan-db.md"] }

5. 后端 Plan Parser 处理：
   ├── 读取所有选中文件内容
   ├── 调用 Agent（ACP）解析为结构化 TaskPlan + TaskItems
   │   Prompt: "读取以下计划文件，提取为结构化任务清单 JSON..."
   ├── 校验解析结果（必填字段、依赖引用合法性）
   └── 写入 Store，状态 draft，记录 source_files

6. 解析失败 → 返回错误详情
   └── 用户可在 Chat 中让 Secretary 调整文件格式后重试
```

> Plan Parser 是一次独立的 Agent 调用（非持久 session），只负责"文件 → 结构化 JSON"的转换。

### 核心结构

```go
type TaskPlan struct {
    ID          string
    ProjectID   string
    SessionID   string            // 关联的 ChatSession
    Name        string            // "add-oauth-login"
    SourceFiles []string          // 计划来源文件路径列表（相对于项目根目录）
    Status      TaskPlanStatus
    WaitReason  WaitReason        // waiting_human 时区分原因
    Tasks       []TaskItem
    FailPolicy  FailurePolicy     // block / skip / human
    ReviewRound int               // 当前审核轮次
    CreatedAt   time.Time
    UpdatedAt   time.Time
}

type WaitReason string
const (
    WaitNone           WaitReason = ""
    WaitFinalApproval  WaitReason = "final_approval"      // AI 审核通过，等人工最终确认
    WaitFeedbackReq    WaitReason = "feedback_required"    // AI 审核超限/DAG 校验失败，等人工反馈
)

type TaskPlanStatus string
const (
    PlanDraft      TaskPlanStatus = "draft"
    PlanReviewing  TaskPlanStatus = "reviewing"
    PlanApproved      TaskPlanStatus = "approved"
    PlanWaitingHuman  TaskPlanStatus = "waiting_human"  // AI 审核通过等最终确认 / 超限等反馈
    PlanExecuting     TaskPlanStatus = "executing"
    PlanPartial    TaskPlanStatus = "partially_done"  // 部分成功部分失败
    PlanDone       TaskPlanStatus = "done"
    PlanFailed     TaskPlanStatus = "failed"
    PlanAbandoned  TaskPlanStatus = "abandoned"        // 用户放弃
)

type TaskItem struct {
    ID          string
    PlanID      string
    Title       string
    Description string            // 任务描述（做什么、为什么）
    Inputs      []string          // 前置输入：依赖的文件/接口/数据
    Outputs     []string          // 交付产物：新建或修改的文件/接口
    Acceptance  []string          // 验收标准：可验证的检查项
    Labels      []string          // ["backend", "database"]
    DependsOn   []string          // 其他 TaskItem 的 ID
    Template    string            // pipeline 模板: quick/standard/full/hotfix
    PipelineID  string            // 审核通过后由 DAG Scheduler 创建
    ExternalID  string            // GitHub Issue # 等外部系统 ID（可选）
    Status      TaskItemStatus
    CreatedAt   time.Time
    UpdatedAt   time.Time
}

type TaskItemStatus string
const (
    ItemPending          TaskItemStatus = "pending"
    ItemReady            TaskItemStatus = "ready"
    ItemRunning          TaskItemStatus = "running"
    ItemDone             TaskItemStatus = "done"
    ItemFailed           TaskItemStatus = "failed"
    ItemSkipped          TaskItemStatus = "skipped"
    ItemBlockedByFailure TaskItemStatus = "blocked_by_failure"
)

type FailurePolicy string
const (
    FailBlock FailurePolicy = "block"   // 下游标记 blocked_by_failure
    FailSkip  FailurePolicy = "skip"    // 跳过强依赖下游，弱依赖继续
    FailHuman FailurePolicy = "human"   // 暂停整个 TaskPlan，等人工决策
)
```

> 对应的 SQL schema 见 [spec-api-config.md](spec-api-config.md) Section IV。

### 状态机

```
TaskPlan 状态流转：

draft ──► reviewing ──► waiting_human ──► executing ──► done
  │          │            (两种原因)          │
  │          │                               ├──► partially_done
  │          │ (fix: 修正后重审)               │         │
  │          ◄──────────────┘                │    人工决策后
  │                                         │    ├──► executing (重试)
  │    reviewing 内部循环：                    │    └──► failed
  │    ├─ approve → waiting_human             │
  │    │   (wait_reason=final_approval)       ▼
  │    │   人工通过 → executing              failed
  │    │   人工驳回 → reviewing (重生成)
  │    ├─ fix → 替换 TaskPlan → 下一轮 review
  │    └─ 超限(max_rounds=2) → waiting_human
  │        (wait_reason=feedback_required)
  │        人工反馈 → reviewing (重生成)
  ▼
abandoned (用户放弃)

TaskItem 状态流转：

pending ──► ready ──► running ──► done
                        │
                        ▼
                      failed ──► running (人工 retry)
                        │
                        ▼
               blocked_by_failure
                        │
                        ├──► ready (上游重试成功)
                        └──► skipped (人工 skip)
```

## 三、Multi-Agent 审核委员会

### 概述

TaskPlan 创建后（由 Plan Parser 从文件解析得到）自动进入审核流程。审核由多个专项 Agent 并行执行，各自关注不同维度，最后由 Aggregator Agent 综合研判。整个过程自动运行，只在反复修正失败时才升级到人工。

> **输入变更**：审核 Reviewer 的输入从"Secretary 直接输出的 JSON"变为"Plan Parser 解析后的结构化 TaskPlan"。Reviewer prompt 中额外注入 `SourceFiles` 的原始文件内容，使审核 Agent 能理解原始计划意图。修正循环中 Aggregator 的 `fix` 行为不变：输出修正后的 TaskItems JSON，直接替换数据库中的 TaskItems（不修改源文件）。

### 审核架构

```
TaskPlan (status: reviewing)
    │
    ▼ 并行调用 3 个 Reviewer
    │
    ├── Completeness Agent ──────────┐
    │   "需求是否全部覆盖？           │
    │    有无遗漏的功能点？"          │
    │                                │
    ├── Dependency Agent ────────────┤
    │   "依赖关系是否正确？           │ 各自输出
    │    DAG 有无环？                 │ ReviewVerdict
    │    并行度是否最大化？"          │
    │                                │
    ├── Feasibility Agent ───────────┤
    │   "每个任务描述是否足够清晰？   │
    │    AI 能独立执行吗？            │
    │    粒度是否合适？"              │
    │                                │
    ▼                                ▼
    ┌────────────────────────────────┐
    │        Aggregator Agent        │
    │                                │
    │  输入：3 个 Reviewer 的意见     │
    │  职责：                        │
    │  - 综合分析所有问题             │
    │  - 判断是否可以修正             │
    │  - 生成修正后的 TaskPlan (fix)  │
    │  - 或判定通过 (approve)        │
    │  - 或判定需要人工 (escalate)   │
    └────────────┬───────────────────┘
                 │
          ┌──────┴──────┐──────────┐
          ▼             ▼          ▼
       approve        fix       escalate
       → DAG调度    → 修正后     → 人工审批
                    重新审核
```

Reviewer/Aggregator 都通过 ACP 调用完成，会话复用策略可配置：

- 默认模式：Review Orchestrator 绑定 `reviewer` 与 `aggregator` 角色，且两者 `roles[].session.reuse=true`
- 同名 Reviewer 在同一 Plan 多轮审核中默认复用 session（可显式关闭）
- Aggregator 默认也复用 session（可显式关闭）

复用约束：

- 每轮必须注入 reset 提示（“基于当前 TaskPlan 快照重新审核，不继承过期结论”）
- 输出结构不合法或出现审查漂移时，强制回退到 fresh session
- 超过 `max_turns` 或 `session_idle_ttl` 后回收并重建

与普通 Pipeline Stage 执行的区别：不创建 worktree、不写 Checkpoint、超时更短（默认 5 分钟）、capabilities 只需 fs_read。

### Reviewer Agent 定义

```go
type ReviewAgent struct {
    Name   string   // "completeness" / "dependency" / "feasibility"
    Prompt string   // 审核 prompt 模板
}

type ReviewVerdict struct {
    Reviewer string          // Agent 名称
    Status   string          // "pass" | "issues_found"
    Issues   []ReviewIssue
    Score    int             // 0-100 评分
}

type ReviewIssue struct {
    Severity    string   // "critical" | "warning" | "suggestion"
    TaskID      string   // 涉及的 TaskItem ID（可选）
    Description string
    Suggestion  string   // 修改建议
}
```

### 各 Reviewer 的 Prompt 要点

**Completeness Agent：**
```
审核以下任务清单是否完整覆盖了用户需求。

用户对话历史：{{.Conversation}}
任务清单：{{.TasksJSON}}

检查项：
1. 对话中提到的每个功能点是否都有对应的任务
2. 是否遗漏了隐含需求（如错误处理、边界情况、测试）
3. 是否有冗余任务（做了用户没要求的事）

输出 JSON：{"status": "pass"|"issues_found", "issues": [...], "score": 0-100}
```

**Dependency Agent：**
```
审核以下任务清单的依赖关系。

任务清单：{{.TasksJSON}}

检查项：
1. 依赖图是否有环（循环依赖）
2. 依赖方向是否正确（A 确实需要 B 先完成吗？）
3. 是否有不必要的依赖（可以并行但被串行化了）
4. 并行度是否已最大化

输出 JSON：{"status": "pass"|"issues_found", "issues": [...], "score": 0-100}
```

**Feasibility Agent：**
```
审核以下任务清单中每个任务的可执行性。

任务清单：{{.TasksJSON}}
项目信息：{{.ProjectContext}}

检查项：
1. 每个任务的 description 是否清晰到 AI 编码 Agent 可以独立执行
2. inputs/outputs 是否具体到文件或接口级别
3. acceptance 中每条标准是否可机器验证（命令行可执行）
4. 任务粒度是否合适（太大需要拆分，太小可以合并）
5. 建议的模板（full/standard/quick/hotfix）是否合理

输出 JSON：{"status": "pass"|"issues_found", "issues": [...], "score": 0-100}
```

### Aggregator 逻辑

Aggregator 也是一次 Agent（ACP）调用，输入是三个 Reviewer 的意见，输出是最终决策：

```
你是任务清单审核委员会的主席。以下是三位审核专家的意见：

完整性审核：{{.CompletenessVerdict}}
依赖性审核：{{.DependencyVerdict}}
可行性审核：{{.FeasibilityVerdict}}

原始任务清单：{{.TasksJSON}}

请综合所有意见做出决策：

1. 如果所有审核都通过且无 critical 问题：输出 {"decision": "approve"}
2. 如果有问题但可以修正：输出 {"decision": "fix", "revised_tasks": [...]}
   - 直接在 revised_tasks 中给出修正后的完整任务清单
   - 确保修正后的清单解决了所有 critical 和 warning 问题
3. 如果问题严重到无法自动修正：输出 {"decision": "escalate", "reason": "..."}
```

### 审核-修正循环

```go
type ReviewOrchestrator struct {
    Reviewers    []ReviewAgent     // 3 个专项 Reviewer
    Aggregator   ReviewAgent       // 综合研判
    MaxRounds    int               // 默认 2
    MinScore     int               // 通过最低分，默认 70
}
```

循环规则：
- 每轮审核消耗 4 次 Agent 调用（3 Reviewer + 1 Aggregator）
- Aggregator 判定 `fix` 时，用修正后的 TaskPlan 替换原始版本，开始下一轮
- Aggregator 判定 `approve` 时，TaskPlan 进入 `waiting_human`（`wait_reason=final_approval`），**人工最终确认后**才进入 `executing`
- 达到 MaxRounds（默认 2）仍未 approve → 进入 `waiting_human`（`wait_reason=feedback_required`）
- 每轮审核结果写入 `review_records` 表（审计追溯）
- 人工在 `final_approval` 时可以：通过（进入 DAG 调度）、驳回（必填两段式反馈：问题类型+具体说明，Secretary 自动重生成后重新进入 AI review）
- 人工在 `feedback_required` 时可以：提交两段式反馈（Secretary 重生成后重走 AI review）、放弃（Plan → abandoned）

> 审核编排器的完整配置（`secretary.review_orchestrator` + `roles` + `role_bindings.review_orchestrator`）见 [spec-api-config.md](spec-api-config.md) Section III。

## 四、TaskPlan 细化（可选，P2a+）

### 背景

Secretary Agent 输出的 TaskItem 偏向"做什么"（需求级），但执行器需要"怎么做"（实施级）。`taskplan_refine` 是一个可选的中间步骤，在审核通过后、调度执行前，为每个 TaskItem 补充实施级细节。

### 触发条件

- TaskPlan 进入 `approved` 状态后自动触发（如果配置开启）
- 配置项：`secretary.refine_enabled: false`（默认关闭，V1 可跳过）

### 细化内容

对每个 TaskItem，通过一次 Agent（ACP）调用补充：

```go
type RefinedDetail struct {
    FilesToModify  []string   // 需要修改的具体文件路径
    FilesToCreate  []string   // 需要新建的文件
    TestCommands   []string   // 验收测试命令
    EstimatedSteps int        // 预估步骤数（供调度器参考）
}
```

细化结果合并到 TaskItem 的结构化字段中：`FilesToModify`/`FilesToCreate` 追加到 `Outputs`，`TestCommands` 追加到 `Acceptance`。不改变 TaskItem 的数据结构。

### 限制

- 细化不改变依赖关系和任务边界（不允许拆分或合并 TaskItem）
- 单个 TaskItem 细化超时 2 分钟，超时则跳过，使用原始描述执行
- 细化失败不阻塞调度——降级为使用原始 TaskItem 描述

## 五、执行期文件沉淀

### 背景

长时间运行的 implement/fixup 阶段容易"漂移"（Agent 偏离目标）。通过在 worktree 中维护结构化进度文件，Agent 可以在每个步骤后自检，也为崩溃恢复提供上下文。

### 三文件约定

Agent 在 implement 阶段启动时，在 worktree 根目录的 `.ai-workflow/` 下创建并持续更新三个文件：

| 文件 | 用途 | 更新频率 |
|------|------|---------|
| `task_plan.md` | 当前 TaskItem 的结构化信息（Description、Inputs、Outputs、Acceptance） | 开始时写入，不再更新 |
| `progress.md` | 已完成的步骤、当前步骤、下一步 | 每完成一个步骤后更新 |
| `findings.md` | 执行过程中的发现、决策记录、遇到的问题 | 随时追加 |

### 实现方式

- 在 implement 阶段的 prompt 模板中注入文件维护指令
- Agent 自行创建和更新这些文件（不需要引擎介入）
- fixup 阶段启动时读取这三个文件，获取上下文
- cleanup 阶段删除 `.ai-workflow/` 目录（不进入最终代码）

### 崩溃恢复价值

Pipeline 崩溃重启时，Agent 可以读取 `progress.md` 了解已完成的工作，避免重复执行。这比纯粹依赖 checkpoint 更细粒度——checkpoint 记录阶段级状态，进度文件记录步骤级状态。

### 配置

```yaml
secretary:
  execution_files_enabled: true   # 默认开启
```

## 六、DAG Scheduler

### 概述

DAG Scheduler 是 Secretary Layer 的核心调度器。它接收 approved 的 TaskPlan，构建依赖图，为就绪的 TaskItem 创建 Pipeline 并启动执行，监听完成事件推进后续任务。

### 核心数据结构

```go
type DepScheduler struct {
    store    Store
    bus      *EventBus
    executor *Executor
    tracker  Tracker   // 可选，同步到外部系统
    sem      chan struct{}        // 复用 max_global_agents 信号量

    mu       sync.Mutex
    plans    map[string]*runningPlan
}

type runningPlan struct {
    Plan     *TaskPlan
    Graph    *DAG
    Running  map[string]string   // taskItemID → pipelineID
}

type DAG struct {
    Nodes      map[string]*TaskItem
    Downstream map[string][]string   // taskID → 下游 taskID 列表
    InDegree   map[string]int        // 当前剩余入度（只计未完成的上游）
}
```

### 调度流程

```
1. TaskPlan approved (AI review 通过)
   │
   ▼
1.2 进入 waiting_human (wait_reason=final_approval)
   │  人工确认通过 → 继续
   │  人工驳回 → Secretary 重生成 → 回到 reviewing
   │
   ▼
1.5（可选）taskplan_refine — 为每个 TaskItem 补充实施级细节（见 Section 四）
   │
   ▼
2. DepScheduler.StartPlan(plan)
   ├── 构建 DAG（解析 DependsOn 字段）
   ├── 校验 DAG 无环（拓扑排序检测）
   ├── 计算所有节点的入度
   ├── 找出入度=0 的 TaskItem → 标记 ready
   ├── 可选：同步到 Tracker（如 GitHub Issue）
   └── 对每个 ready 的 TaskItem → dispatchTask()

3. dispatchTask(item)
   ├── 获取信号量（阻塞等待）
   ├── 创建 Pipeline（使用 item.Template 模板）
   │   Pipeline.Name = item.Title
   │   Pipeline.Description = item.Description
   │   Pipeline.TaskItemID = item.ID
   │   requirements 阶段通过 task_item_id 反查并注入 item.Description/Inputs/Outputs/Acceptance
   ├── 更新 item.PipelineID
   ├── 更新 item.Status = running
   ├── 启动 Pipeline（调用现有 Executor.Run）
   └── 记录 Running map

4. 监听 Event Bus
   │
   ├── pipeline_done 事件
   │   ├── 找到对应的 TaskItem → 标记 done
   │   ├── 释放信号量
   │   ├── 遍历该 TaskItem 的下游
   │   │   └── 对每个下游：InDegree--
   │   │       └── 如果 InDegree == 0 → 标记 ready → dispatchTask()
   │   ├── 可选：同步状态到 Tracker
   │   └── 检查是否所有 TaskItem 都完成 → 是则 Plan = done
   │
   ├── pipeline_failed 事件
   │   ├── 找到对应的 TaskItem → 标记 failed
   │   ├── 释放信号量
   │   ├── 根据 FailPolicy 处理下游：
   │   │   ├── block: 下游标记 blocked_by_failure
   │   │   ├── skip: 无强依赖的下游继续，有强依赖的 skip
   │   │   └── human: 暂停整个 Plan，等人工决策
   │   └── 可选：同步状态到 Tracker
   │
   └── 人工操作事件
       ├── retry(taskItemID): 重新创建 Pipeline → dispatchTask()
       ├── skip(taskItemID): 标记 skipped → 解除下游阻塞
       ├── replan: 回到 Secretary Agent 重新拆解
       └── abort: 终止所有运行中的 Pipeline，Plan = failed
```

### DAG 校验

在 StartPlan 时执行：

```go
func (d *DAG) Validate() error {
    // 1. 检测环：使用 Kahn 算法（拓扑排序）
    //    如果排序后节点数 < 总节点数 → 有环 → 返回错误

    // 2. 检测孤立引用：DependsOn 中引用的 ID 必须存在

    // 3. 检测自依赖：任务不能依赖自己

    return nil
}
```

### 并发控制

DAG Scheduler 复用现有的全局信号量，不引入额外的并发控制：

```
全局 Agent 并发上限 (max_global_agents，默认 3)
    │
    ├── Plan A / Task 1 → Pipeline → 占 1 个信号量
    ├── Plan A / Task 2 → Pipeline → 占 1 个信号量
    └── Plan B / Task 1 → Pipeline → 占 1 个信号量
        (其他 ready 的 Task 排队等待信号量)
```

所有 Pipeline 共享同一个信号量池，不区分来源。当前调度策略为 FIFO（先 ready 的先获取信号量）。

> **P4 演进：优先级调度**。计划为 TaskItem 增加 `priority` 字段（P0 紧急 / P1 高 / P2 普通，默认 P2），DAG Scheduler 按优先级排序 ready 队列。同时支持优先级继承：如果高优先级任务被低优先级上游阻塞，自动提升上游优先级，促使阻塞链尽快完成。当前阶段 FIFO 已满足需求，不提前引入复杂性。

### 失败处理详细规则

**block 策略（默认）：**
- 失败任务的所有直接和间接下游标记为 `blocked_by_failure`
- 不会创建这些下游的 Pipeline
- 其他无关分支的任务继续执行
- 人工可以 retry 失败任务，成功后自动解除下游阻塞

**skip 策略：**
- 失败任务标记为 `failed`
- 直接下游检查：如果该任务是下游的**唯一**上游 → 下游标记 `skipped`
- 如果下游还有其他已完成的上游 → 下游仍可执行（降级模式）
- 适用于容错要求高的场景

**human 策略：**
- 失败后暂停整个 TaskPlan
- 所有运行中的 Pipeline 继续执行直到完成（不强制终止）
- 不再启动新的 Pipeline
- 等待人工决策：retry / skip / replan / abort

### 崩溃恢复

进程重启时：

1. 扫描 Store 中 `status = executing` 的 TaskPlan
2. 重建 DAG 和 runningPlan 结构
3. 查询关联的 Pipeline 状态：
   - Pipeline running → 恢复监听（Pipeline Engine 自己会恢复执行）
   - Pipeline done → 触发下游调度
   - Pipeline failed → 根据 FailPolicy 处理
4. 重新计算各 TaskItem 的入度和状态

> DAG Scheduler 的完整配置（`secretary.dag_scheduler:`）见 [spec-api-config.md](spec-api-config.md) Section III。

## 七、基础设施抽象

### ReviewGate 插件

```go
type ReviewGate interface {
    Plugin
    // 提交 TaskPlan 进行审核，返回审核 ID
    Submit(ctx context.Context, plan *TaskPlan) (reviewID string, err error)
    // 查询审核状态
    Check(ctx context.Context, reviewID string) (*ReviewResult, error)
    // 取消审核
    Cancel(ctx context.Context, reviewID string) error
}

type ReviewResult struct {
    Status    string          // "pending" | "approved" | "rejected" | "changes_requested"
    Verdicts  []ReviewVerdict // 各 Reviewer 的意见
    Decision  string          // Aggregator 的决策
    Revised   *TaskPlan       // fix 时的修正版本
    Comments  []string
}
```

**实现：**

| 实现 | 说明 | 阶段 |
|------|------|------|
| `review-ai-panel` | Multi-Agent 审核委员会（默认） | P2b ✅ |
| `review-local` | Workbench/TUI 中人工直接审批 | P2a ✅ |
| `review-github-pr` | 创建 PR 提交 tasks.json，等 merge | P3 🔧 |

### Tracker 插件

```go
type Tracker interface {
    Plugin
    // 创建外部任务（如 GitHub Issue）
    CreateTask(ctx context.Context, item *TaskItem) (externalID string, err error)
    // 更新外部任务状态
    UpdateStatus(ctx context.Context, externalID string, status TaskItemStatus) error
    // 同步依赖关系到外部系统
    SyncDependencies(ctx context.Context, item *TaskItem, allItems []TaskItem) error
    // 外部任务完成时的回调（如 GitHub Issue closed）
    OnExternalComplete(ctx context.Context, externalID string) error
}
```

**实现：**

| 实现 | 说明 | 阶段 |
|------|------|------|
| `tracker-local` | 纯本地，不同步到外部系统（默认，空实现） | P2a ✅ |
| `tracker-github` | 同步为 GitHub Issue + Label 管理 | P3 🔧 |
| `tracker-linear` | 同步到 Linear | P4 |

`tracker-local` 是一个 no-op 实现——所有状态都在 SQLite 的 `task_items` 表中，不需要外部同步。Tracker 插件只负责**镜像**到外部系统，不是核心逻辑。

## 八、Workbench UI

### 定位

Workbench 是系统的主操作界面，基于 Web 技术构建，内嵌到 Go 二进制中（`embed.FS`）。提供五个核心视图：Chat、Plan、Board、Pipeline，以及独立的 Admin 全局管理界面。

### 技术选型

| 层级 | 选型 | 理由 |
|------|------|------|
| 框架 | React 18+ | 组件化、生态成熟 |
| 样式 | Tailwind CSS | 实用优先、开发快 |
| 构建 | Vite | 快速 HMR、打包小 |
| 实时通信 | WebSocket | 双向、低延迟 |
| 状态管理 | Zustand | 轻量、无样板代码 |
| DAG 可视化 | React Flow | 专为节点图设计 |
| 打包方式 | `embed.FS` 嵌入 Go 二进制 | 零外部依赖部署 |

### 页面结构

```
┌─ 顶栏 ───────────────────────────────────────────────────┐
│  项目选择器 ▾    │  当前计划状态指示  │  Admin │ 设置 ⚙ │
└──────────────────┴──────────────────┴────────┴───────────┘
┌─ 侧栏 ──┐  ┌─ 主面板 ──────────────────────────────────┐
│          │  │                                           │
│ 📂 项目  │  │  视图内容（根据侧栏选择切换）              │
│ ├ app-a  │  │                                           │
│ └ api-b  │  │  • Chat View    — 对话 + 文件选择         │
│          │  │  • Plan View    — DAG 可视化 + 审核状态    │
│ 📋 计划  │  │  • Board View   — 看板式任务跟踪          │
│ ├ Plan A │  │  • Pipeline View — 单个 Pipeline 详情     │
│ └ Plan B │  │  • Manage View  — 项目级管理 + 审计日志    │
│          │  │                                           │
│ ⚡ 活跃  │  │                                           │
│ 3 running│  │                                           │
│          │  │                                           │
│ 📊 历史  │  │                                           │
│          │  │                                           │
│ 🔧 管理  │  │                                           │
│          │  │                                           │
└──────────┘  └───────────────────────────────────────────┘
```

### Chat View（对话视图）

主要元素：
- **对话历史区域**：显示用户消息和 Secretary Agent 回复，支持 Markdown 渲染
- **输入框**：支持多行输入、Shift+Enter 换行、Enter 发送
- **操作按钮**：
  - 「新建对话」— 开始新的 ChatSession
  - 「结束 Session」— 关闭当前 Agent session
- **Agent 状态指示**：显示 Secretary session 是否存活、当前状态（generating / idle / tool_running）
- **流式输出**：Secretary Agent 回复时流式显示（通过 WebSocket 接收 `secretary_thinking` 事件）
- **文件变更提示区**（Secretary 写入文件后自动弹出）：
  - 列出 Secretary 新增/修改的文件路径
  - 每个文件可预览内容
  - 多选框勾选要用作 Plan 输入的文件
  - 「创建计划」按钮 → 提交选中文件创建 TaskPlan

> **注意**：Chat View 不再有"生成任务清单"按钮。Plan 的创建完全由用户主动触发——先在对话中让 Secretary 生成文件，再通过文件变更提示区勾选提交。

### Plan View（计划视图）

主要元素：
- **DAG 图**：使用 React Flow 渲染任务依赖图
  - 节点 = TaskItem，颜色表示状态
  - 边 = 依赖关系，箭头方向从上游到下游
  - 点击节点展开任务详情
- **源文件列表**：显示 Plan 来源文件，点击可查看内容
- **审核面板**（审核进行中时显示）：
  - 各 Reviewer 的状态和评分
  - Aggregator 的决策
  - 修正记录（每轮对比）
- **操作按钮**：
  - 「接受」— 人工 approve
  - 「驳回重来」— 回到 Chat 重新对话
  - 「手动编辑」— 直接修改 TaskPlan JSON

### Board View（看板视图）

主要元素：
- **四列看板**：Pending | Ready | Running | Done/Failed
- **任务卡片**：
  - 标题、标签、模板类型
  - 进度指示（running 时显示当前 Pipeline 阶段）
  - 依赖关系缩略（上游 N 个，下游 N 个）
- **操作**：
  - 点击卡片 → 跳转到 Pipeline View
  - 右键 → retry / skip / abort
- **过滤器**：按状态、标签、模板过滤

### Pipeline View（Pipeline 详情）

复用现有 Pipeline 监控能力：
- 阶段进度条（当前在哪个 Stage）
- Agent 实时输出流（WebSocket `agent_output` 事件）
- Checkpoint 列表
- 人工操作按钮（approve / reject / skip / abort）

### Manage View（项目级管理视图）

Workbench 侧栏中的"管理"Tab，项目级视角：
- **Pipeline 状态面板**：当前项目下所有 Pipeline 列表，按状态分组，显示阶段进度
- **Plan 概览**：当前项目下所有 TaskPlan，审核状态、执行进度
- **审计日志**：当前项目的操作日志，支持按操作类型/时间过滤

### Admin View（全局管理界面，独立路由 /admin）

独立于项目 Workbench，全局视角：
- **概览仪表盘**：
  - 项目总数 / 活跃项目数
  - 运行中 Pipeline / 等待中 / 今日完成 / 今日失败
  - 执行中 Plan / 审核中 Plan
  - Agent 并发使用 / 最大并发
- **全局 Pipeline 列表**：跨项目查看所有 Pipeline，支持按项目/状态/时间过滤
- **全局 Plan 列表**：跨项目查看所有 TaskPlan
- **审计日志浏览器**：全局操作日志，支持按项目/操作类型/用户/时间范围过滤
- **系统健康**：Agent CLI 可用性、SQLite 状态、磁盘使用

> REST API 端点、WebSocket 事件协议见 [spec-api-config.md](spec-api-config.md) Section I/II。完整目录结构见 [spec-overview.md](spec-overview.md)。

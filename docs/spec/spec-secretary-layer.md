# Secretary Layer — 设计文档

> 本文档是 [spec-overview.md](spec-overview.md) 中 Secretary Layer 的详细设计。整体架构、插件体系、目录结构、设计原则、实施分期见总览文档。API 端点、WebSocket 协议、SQL Schema、配置 YAML 见 [spec-api-config.md](spec-api-config.md)。Event Bus 事件定义见 [spec-pipeline-engine.md](spec-pipeline-engine.md) Section VII。ACP Client 接口见 [spec-agent-drivers.md](spec-agent-drivers.md)。

## 概述

Secretary Layer 是系统的上层编排层，位于 Orchestrator Core 之上。它负责：通过持久交互式 Agent session 与用户多轮对话理解需求、由用户指示在项目中生成计划文件、经用户批量提交为 Issue（GitHub 对齐命名，1 Issue = 1 需求单）、通过两阶段 AI 审核（单 Issue 审查 + 跨 Issue 依赖分析）自动审核、DAG Scheduler 管理 Issue 间依赖并行调度。每个 Issue 1:1 对应一个 Pipeline，由 ACP Agent 独立完成（Agent 内部处理并行执行）。

**核心设计变化**：ACP 客户端自身支持内部并行执行，系统不再将一个计划拆解为多个子任务。每个 Issue 直接关联 plan 文件，整体交给一个 Pipeline 执行。系统只管理 Issue 间的依赖调度，不管理 Issue 内部的并行。

## 一、Secretary Agent

### 职责

Secretary Agent 是用户的入口。用户通过持久交互式 session 与 Secretary 多轮对话，Secretary 作为一个拥有项目文件权限的 AI Agent，帮助用户理解代码、探索项目、讨论需求。当需求明确后，用户指示 Secretary 在项目中生成计划文件，后续由用户勾选文件创建 Issue。

**Secretary 不自动创建 Issue。** 对话只是对话，Issue 的创建是一个独立的、由用户发起的动作。

### 实现方式 — ACP 持久 Session

Secretary Agent 通过 ACP Client 建立持久多轮 session，天然支持对话上下文保持。通信链路统一使用 ACP `session/new`、`session/prompt`、`session/load` 语义：

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
| `query_issues` | 列出当前项目所有 Issue | ID, title, state, status |
| `query_issue_detail` | 查看某个 Issue 详情 | attachments, DAG, review status |
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
  → Agent 调用 MCP tool (如 query_issues)
  → MCP Server 执行查询 (调 Store 接口)
  → 结果通过 MCP 协议返回 Agent
  → Agent 继续生成回复
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
- ChatSession 和 Issue 是 **1:N** 关系（一个长期对话可产出多个 Issue）
- 对话历史持久化到 SQLite，支持断线续聊
- Agent session 是运行时状态。项目重开/断线重连时优先通过 `acpClient.LoadSession` 恢复已有 session（Agent 支持时）；仅在不支持或恢复失败时才 `NewSession`
- 空闲超时后自动关闭 session（默认 30 分钟，可配置）

> 对应的 SQL schema 见 [spec-api-config.md](spec-api-config.md) Section IV。ID 生成规则见 Section V。

## 二、Issue 数据模型

### 计划文件 → Issue 流程

Issue 的创建基于文件驱动，用户直接提交 plan 文件，不需要 AI 解析为结构化子任务：

```
1. 用户在 Chat 中指示 Secretary 生成计划文件
   └── Secretary 在项目目录写入文件（格式自由：.md / .json / .yaml / 混合）
       推荐但不强制写入 .ai-workflow/plans/ 目录

2. 后端检测到 Secretary 写文件（由 ACP Handler 的 HandleWriteFile 回调触发）
   └── 发 WebSocket 事件 secretary_files_changed { file_paths, session_id }

3. 前端展示变更文件列表，用户分组勾选
   └── 每组文件 = 1 个 Issue（一个 Issue 可关联多个 plan 文件）

4. 用户提交 → POST /api/v1/projects/:pid/issues
   Body: {
     issues: [
       { title: "用户认证", attachments: ["plan-auth.md", "plan-auth-api.md"] },
       { title: "数据库设计", attachments: ["plan-db.md"] }
     ],
     auto_review: true
   }

5. 后端处理：
   ├── 为每个 Issue 创建记录，状态 draft
   ├── 快照所有关联 plan 文件内容 → issue_attachments 表（留痕）
   └── 如果 auto_review=true，自动触发两阶段 AI 审核

6. 审核通过 → DAG Scheduler 接管
   └── 就绪 Issue → 创建 Pipeline（plan 文件内容注入 agent prompt）
```

> 不再有 Plan Parser。plan 文件直接作为 Pipeline 的输入 spec，由执行 Agent 自行理解和实施。

### 核心结构

```go
type Issue struct {
    ID           string
    ProjectID    string
    SessionID    string          // 关联的 ChatSession（可选）

    // GitHub 风格字段
    Title        string          // AI 摘要或用户填
    Body         string          // AI 生成的摘要描述
    Labels       []string        // ["backend", "database", "urgent"]
    MilestoneID  string          // 可选分组

    // Plan 文件（Issue 的核心输入）
    Attachments  []string        // plan 文件路径列表（相对于项目根目录）

    // 调度
    DependsOn    []string        // 其他 Issue ID
    Blocks       []string        // 被本 Issue 阻塞的 Issue ID（冗余，方便查询）
    Priority     int             // 0=critical 1=high 2=normal
    Template     string          // pipeline 模板: quick/standard/full/hotfix

    // 生命周期
    State        IssueState      // open / closed（GitHub 风格，对外）
    Status       IssueStatus     // 内部细分状态
    PipelineID   string          // 1:1 映射，审核通过后由 DAG Scheduler 创建

    // 变更管理
    Version      int             // 每次变更 +1
    SupersededBy string          // 被哪个新 Issue 取代

    // 外部同步
    ExternalID   string          // GitHub Issue number 等外部系统 ID（可选）
    FailPolicy   FailurePolicy   // block / skip / human

    CreatedAt    time.Time
    UpdatedAt    time.Time
    ClosedAt     *time.Time
}

// State — GitHub 风格，只有两态
type IssueState string
const (
    StateOpen   IssueState = "open"
    StateClosed IssueState = "closed"
)

// Status — 内部细分，驱动调度逻辑
type IssueStatus string
const (
    StatusDraft      IssueStatus = "draft"       // 刚创建
    StatusReviewing  IssueStatus = "reviewing"   // AI 审核中
    StatusQueued     IssueStatus = "queued"      // 审核通过，等待依赖
    StatusReady      IssueStatus = "ready"       // 依赖满足，可调度
    StatusExecuting  IssueStatus = "executing"   // Pipeline 运行中
    StatusDone       IssueStatus = "done"        // 完成 → state=closed
    StatusFailed     IssueStatus = "failed"      // 失败
    StatusSuperseded IssueStatus = "superseded"  // 被取代 → state=closed
    StatusAbandoned  IssueStatus = "abandoned"   // 放弃 → state=closed
)

type FailurePolicy string
const (
    FailBlock FailurePolicy = "block"   // 下游标记 blocked
    FailSkip  FailurePolicy = "skip"    // 跳过强依赖下游
    FailHuman FailurePolicy = "human"   // 暂停，等人工决策
)
```

> 对应的 SQL schema 见 [spec-api-config.md](spec-api-config.md) Section IV。

### State vs Status 设计

和 GitHub 一样，对外只暴露 `open/closed` 两态（`State`），`Status` 是内部调度用的细分状态：

| State | 对应 Status | 说明 |
|-------|-----------|------|
| open | draft, reviewing, queued, ready, executing, failed | 工单仍然活跃 |
| closed | done, superseded, abandoned | 工单已结束 |

`failed` 保持 `open` 状态，因为用户可能 retry。用户显式 close 后才变为 `closed`。

### 状态机

```
Issue 状态流转：

draft ──► reviewing ──► queued ──► ready ──► executing ──► done (closed)
  │          │            ▲                      │
  │          │            │                      ├──► failed (open, 可 retry)
  │          │            │                      │       │
  │          ▼            │                      │   retry → executing
  │    needs_revision     │                      │
  │    (回到 draft,       │                      │
  │     用户修改文件)      │                      │
  │                       │                      │
  │   审核通过 + 有依赖 ──┘                      │
  │   审核通过 + 无依赖 ──────► ready             │
  │                                              │
  ▼                                              ▼
abandoned (closed)                        superseded (closed)

依赖驱动的状态转换（DAG Scheduler）：
  queued: 有未完成的上游 Issue
  ready:  所有上游 done → 入度归零
  上游 failed + fail_policy=block → 本 Issue 标记 blocked (保持 queued)
```

### 人工审核动作约束（Plan/Issue 闭环）

为保证前后端状态机一致，UI 的“批准/驳回”入口由 Issue 与 Pipeline 两侧状态共同决定：

- `approve` 可在以下状态触发：
  - Issue `status=reviewing`（审核阶段）
  - Pipeline `status=waiting_human` 且 reason = `final_approval | feedback_required`（执行阶段）
- `reject` 可在以下状态触发：
  - Issue `status=reviewing`（审核阶段）
  - Pipeline `status=waiting_human` 且 reason = `final_approval | feedback_required`（执行阶段）
- 在 Issue 审核阶段执行 `reject` 时，必须包含结构化反馈
  （category + detail），执行后回到 `draft`

`approve` 的执行语义：

1. 先将 Issue 置为 `queued`（state 仍为 `open`）
2. 再调用调度器尝试进入 Pipeline
3. 如果调度器启动失败，Issue 必须回写为 `failed`，并在 `issue_changes.reason`
   记录 `approve dispatch failed: ...`（含可诊断错误）

该规则确保不会出现“前端显示 queued，但实际未进入 pipeline 且无原因”的静默卡死。

## 三、Issue 审核（两阶段）

### 概述

Issue 创建后可自动触发两阶段 AI 审核。相比原来的 3 Reviewer + 1 Aggregator (4+ 次 ACP 调用/轮)，新设计更轻量高效：

- **Phase 1**：Per-Issue Review（N 个 Issue 并行审查，N 次 ACP 调用）
- **Phase 2**：Cross-Issue Analysis（1 次 ACP 调用，分析依赖 DAG）

总计 N+1 次 ACP 调用（N=Issue 数量），无修正循环。

### 审核架构

```
用户提交 N 个 Issue (status: draft)
    │
    ▼ auto_review=true
    │
    ├─── Phase 1: Per-Issue Review (并行) ──────────┐
    │                                                │
    │    Issue A → demand_reviewer ACP 调用           │
    │    Issue B → demand_reviewer ACP 调用           │  各自输出
    │    Issue C → demand_reviewer ACP 调用           │  ReviewVerdict
    │    (N 个 Issue 并行审查)                        │
    │                                                │
    ▼                                                ▼
    ┌────────────────────────────────────────────────┐
    │  Phase 2: Cross-Issue Dependency Analysis      │
    │                                                │
    │  输入：所有 Issue 的 plan 文件 + 摘要           │
    │  输出：                                        │
    │  - 依赖 DAG: [{from, to, reason}]              │
    │  - 冲突检测: 哪些 Issue 改同一文件              │
    │  - 优先级建议                                  │
    │                                                │
    │  1 次 dependency_analyzer ACP 调用              │
    └──────────────┬─────────────────────────────────┘
                   │
            ┌──────┴──────┐
            ▼             ▼
      全部 pass       有 issues
      + 无冲突         或冲突
            │             │
            ▼             ▼
      自动批准        前端展示
      → queued/ready   DAG + issues
                       → 人工确认
```

### Phase 1: Per-Issue Review

每个 Issue 独立审查，由 `demand_reviewer` 角色执行：

```go
type IssueReviewVerdict struct {
    IssueID   string
    Verdict   string          // "pass" | "needs_revision"
    Title     string          // AI 自动摘要标题
    Summary   string          // AI 生成摘要
    Template  string          // 建议的 pipeline 模板
    Score     int             // 0-100
    Issues    []ReviewIssue
}

type ReviewIssue struct {
    Severity    string   // "critical" | "warning" | "suggestion"
    Description string
    Suggestion  string
}
```

**demand_review.tmpl** prompt 要点：

```
审查以下计划文件，评估其作为 AI 编码 Agent 输入的质量。

计划文件内容：
{{range .PlanFiles}}
--- {{.Path}} ---
{{.Content}}
{{end}}

项目上下文：{{.ProjectContext}}

检查项：
1. 需求描述是否清晰到 AI Agent 可独立执行
2. 边界和范围是否明确（做什么、不做什么）
3. 验收标准是否可机器验证
4. 建议的 pipeline 模板 (quick/standard/full/hotfix)

输出 JSON：{
  "verdict": "pass" | "needs_revision",
  "title": "自动摘要标题（20字以内）",
  "summary": "50字以内摘要",
  "template": "standard",
  "score": 0-100,
  "issues": [{"severity": "...", "description": "...", "suggestion": "..."}]
}
```

### Phase 2: Cross-Issue Dependency Analysis

当批量提交多个 Issue 时，由 `dependency_analyzer` 角色分析全局依赖关系：

```go
type DependencyAnalysis struct {
    Edges      []DependencyEdge
    Conflicts  []ConflictInfo
    Priorities []PrioritySuggestion
}

type DependencyEdge struct {
    From   string   // Issue ID
    To     string   // Issue ID（From 必须在 To 之前完成）
    Reason string   // 依赖原因
}

type ConflictInfo struct {
    IssueIDs   []string   // 冲突的 Issue
    Resource   string     // 冲突资源（如 "同一文件 src/auth.go"）
    Suggestion string     // 建议处理方式
}

type PrioritySuggestion struct {
    IssueID  string
    Priority int      // 0=critical 1=high 2=normal
    Reason   string
}
```

**dependency_analysis.tmpl** prompt 要点：

```
分析以下一批需求之间的依赖关系。

{{range .Issues}}
Issue {{.ID}}: {{.Title}}
摘要: {{.Summary}}
计划文件: {{.AttachmentPaths}}
---
{{end}}

项目结构概要：{{.ProjectTree}}

分析要求：
1. 识别需求间的依赖（A 必须在 B 之前完成的原因）
2. 检测冲突（多个需求修改同一文件/模块）
3. 最大化并行度（不要加不必要的依赖）
4. 建议执行优先级

输出 JSON：{
  "edges": [{"from": "issue-id-a", "to": "issue-id-b", "reason": "..."}],
  "conflicts": [{"issue_ids": ["id-a","id-c"], "resource": "...", "suggestion": "..."}],
  "priorities": [{"issue_id": "...", "priority": 0, "reason": "..."}]
}
```

### 单个 Issue 提交时的行为

如果只提交 1 个 Issue：
- Phase 1 正常执行（审查质量）
- Phase 2 跳过（只有 1 个 Issue，无需分析依赖）
- 但会检查与项目中**已有 open Issue** 的依赖关系（可选，`check_existing_deps: true`）

### Auto-approve 规则

```yaml
secretary:
  demand_review:
    auto_approve: true              # 开启自动批准
    auto_approve_threshold: 80      # 所有 Issue score >= 此值时自动批准
    auto_approve_on_conflict: false  # 有冲突时不自动批准，等人工
    check_existing_deps: true       # 单个 Issue 提交时检查与已有 Issue 的依赖
```

全部 pass + score >= threshold + 无冲突 → 自动写入 DAG + 更新状态为 queued/ready → DAG Scheduler 直接调度。

有 needs_revision 或冲突 → Issue 回到 draft，前端展示审核结果和 DAG，等用户修改或确认。

### 与 ReviewGate 插件的关系

两阶段审核是 `review-ai-panel` 插件的新实现。ReviewGate 接口保留但简化：

```go
type ReviewGate interface {
    Plugin
    // 提交一批 Issue 进行审核
    Submit(ctx context.Context, issues []*Issue) (reviewID string, err error)
    // 查询审核状态
    Check(ctx context.Context, reviewID string) (*ReviewResult, error)
    // 取消审核
    Cancel(ctx context.Context, reviewID string) error
}

type ReviewResult struct {
    Status     string                // "pending" | "completed"
    Verdicts   map[string]*IssueReviewVerdict  // issueID → verdict
    DAG        *DependencyAnalysis   // 跨 Issue 依赖分析结果
    AutoApproved bool               // 是否自动批准
}
```

其他 ReviewGate 实现：
- `review-local`：人工直接审批（跳过 AI 审核）
- `review-github-pr`：通过 GitHub PR 审核（P3，可选）

## 四、需求变更管理

### 背景

现实中需求经常变更。Issue 模型支持版本化变更，保证可追溯性。

### 变更场景

**场景 1：Issue 在排队中（queued/ready），尚未执行**

直接更新 plan 文件，重新快照：

```
POST /api/v1/projects/:pid/issues/:id
Body: { attachments: ["plan-auth-v2.md"] }

→ 快照新文件内容
→ Issue.Version++
→ 记录变更到 issue_changes 表
→ 如果已有 DAG，重新触发 Phase 2 分析（依赖可能变了）
→ 状态不变
```

**场景 2：Issue 正在执行（executing）**

三个选项，由用户决定：

| 选项 | 操作 | 适用场景 |
|------|------|---------|
| abort & redo | 终止当前 Pipeline，用新版本重新执行 | 需求大改 |
| supersede | 当前 Issue 标记 superseded，创建新 Issue | 需求方向完全变了 |
| wait | 等当前 Pipeline 完成，再创建补充 Issue | 小修补 |

**场景 3：Issue 已完成（done），需要改**

创建新 Issue，可选 `DependsOn` 包含原 Issue（保证执行顺序）。原 Issue 状态不变。

### 变更记录

所有变更写入 `issue_changes` 表：

```go
type IssueChange struct {
    ID        int
    IssueID   string
    Field     string    // "status" / "attachments" / "depends_on" / "priority" / ...
    OldValue  string
    NewValue  string
    Reason    string    // 变更原因
    ChangedBy string    // user / system / ai
    CreatedAt time.Time
}
```

支持按 Issue 查看完整变更历史（谁在什么时候改了什么）。

## 五、执行期文件沉淀

### 背景

长时间运行的 implement/fixup 阶段容易"漂移"（Agent 偏离目标）。通过在 worktree 中维护结构化进度文件，Agent 可以在每个步骤后自检，也为崩溃恢复提供上下文。

### 三文件约定

Agent 在 implement 阶段启动时，在 worktree 根目录的 `.ai-workflow/` 下创建并持续更新三个文件：

| 文件 | 用途 | 更新频率 |
|------|------|---------|
| `issue_spec.md` | 当前 Issue 的 plan 文件内容合并（Agent 的输入 spec） | 开始时写入，不再更新 |
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

DAG Scheduler 是 Secretary Layer 的核心调度器。它接收审核通过的 Issue，构建依赖图，为就绪的 Issue 创建 Pipeline 并启动执行，监听完成事件推进后续 Issue。

### 核心数据结构

```go
type DepScheduler struct {
    store    Store
    bus      *EventBus
    executor *Executor
    tracker  Tracker   // 可选，同步到外部系统
    sem      chan struct{}        // 复用 max_global_agents 信号量

    mu       sync.Mutex
    active   map[string]*runningIssue
}

type runningIssue struct {
    Issue      *Issue
    PipelineID string
}

type DAG struct {
    Nodes      map[string]*Issue
    Downstream map[string][]string   // issueID → 下游 issueID 列表
    InDegree   map[string]int        // 当前剩余入度（只计未完成的上游）
}
```

### 调度流程

```
1. Issue 审核通过（Phase 1 + Phase 2 完成）
   │
   ├── 自动批准 → 直接进入调度
   └── 人工确认 → 用户 approve 后进入调度
   │
   ▼
2. DepScheduler.EnqueueIssues(issues, dag)
   ├── 写入 DAG 依赖关系（Phase 2 分析结果或人工指定）
   ├── 校验 DAG 无环（拓扑排序检测）
   ├── 计算所有节点的入度
   ├── 有依赖的 Issue → 标记 queued
   ├── 入度=0 的 Issue → 标记 ready
   ├── 可选：同步到 Tracker（如 GitHub Issue）
   └── 对每个 ready 的 Issue → dispatchIssue()

3. dispatchIssue(issue)
   ├── 获取信号量（阻塞等待）
   ├── 读取 issue_attachments 快照内容
   ├── 创建 Pipeline（使用 issue.Template 模板）
   │   Pipeline.Name = issue.Title
   │   Pipeline.Description = issue.Body
   │   Pipeline.IssueID = issue.ID
   │   requirements 阶段注入 plan 文件快照内容
   ├── 更新 issue.PipelineID
   ├── 更新 issue.Status = executing
   ├── 启动 Pipeline（调用现有 Executor.Run）
   └── 记录到 active map

4. 监听 Event Bus
   │
   ├── pipeline_done 事件
   │   ├── 找到对应的 Issue → 标记 done, state=closed
   │   ├── 释放信号量
   │   ├── 遍历该 Issue 的下游
   │   │   └── 对每个下游：InDegree--
   │   │       └── 如果 InDegree == 0 → 标记 ready → dispatchIssue()
   │   ├── 可选：同步状态到 Tracker
   │   └── 检查是否所有同批 Issue 都完成
   │
   ├── pipeline_failed 事件
   │   ├── 找到对应的 Issue → 标记 failed
   │   ├── 释放信号量
   │   ├── 根据 FailPolicy 处理下游：
   │   │   ├── block: 下游标记 blocked（保持 queued）
   │   │   ├── skip: 无强依赖的下游继续，有强依赖的 skip
   │   │   └── human: 暂停调度，等人工决策
   │   └── 可选：同步状态到 Tracker
   │
   └── 人工操作事件
       ├── retry(issueID): 重新创建 Pipeline → dispatchIssue()
       ├── skip(issueID): 标记 skipped → 解除下游阻塞
       └── abort(issueID): 终止 Pipeline, Issue → abandoned
```

### Issue Timeline 聚合视图（Workbench）

Issue 详情页通过统一 Timeline 聚合展示全链路留痕，数据来源：

- `review_records`
- `issue_changes`
- `human_actions`（通过 `issue.pipeline_id` 关联）
- `checkpoints`
- `logs`
- `audit_log`（可选，kind=`audit`）

接口：

- `GET /api/v1/projects/:pid/issues/:id/timeline`
- `GET /api/v1/projects/:pid/plans/:id/timeline`（兼容别名）

分页与排序：

- 默认 `limit=50`
- 服务端按 `created_at ASC` 返回
- 前端可按产品交互需求倒序渲染

### DAG 校验

在 EnqueueIssues 时执行：

```go
func (d *DAG) Validate() error {
    // 1. 检测环：使用 Kahn 算法（拓扑排序）
    //    如果排序后节点数 < 总节点数 → 有环 → 返回错误

    // 2. 检测孤立引用：DependsOn 中引用的 ID 必须存在

    // 3. 检测自依赖：Issue 不能依赖自己

    return nil
}
```

### 并发控制

DAG Scheduler 复用现有的全局信号量，不引入额外的并发控制：

```
全局 Agent 并发上限 (max_global_agents，默认 3)
    │
    ├── Issue A → Pipeline → 占 1 个信号量
    ├── Issue B → Pipeline → 占 1 个信号量
    └── Issue C → Pipeline → 占 1 个信号量
        (其他 ready 的 Issue 排队等待信号量)
```

所有 Pipeline 共享同一个信号量池，不区分来源。当前调度策略为 FIFO（先 ready 的先获取信号量）。

> **P4 演进：优先级调度**。DAG Scheduler 按 Issue.Priority 排序 ready 队列。同时支持优先级继承：如果高优先级 Issue 被低优先级上游阻塞，自动提升上游优先级。当前阶段 FIFO 已满足需求。

### 失败处理详细规则

**block 策略（默认）：**
- 失败 Issue 的所有直接和间接下游保持 `queued`（标记 blocked 原因）
- 不会创建这些下游的 Pipeline
- 其他无关分支的 Issue 继续执行
- 人工可以 retry 失败 Issue，成功后自动解除下游阻塞

**skip 策略：**
- 失败 Issue 标记为 `failed`
- 直接下游检查：如果该 Issue 是下游的**唯一**上游 → 下游也标记 `failed`
- 如果下游还有其他已完成的上游 → 下游仍可执行（降级模式）

**human 策略：**
- 失败后暂停调度
- 所有运行中的 Pipeline 继续执行直到完成（不强制终止）
- 不再启动新的 Pipeline
- 等待人工决策：retry / skip / abort

### 崩溃恢复

进程重启时：

1. 扫描 Store 中 `state=open AND status IN (queued, ready, executing)` 的 Issue
2. 重建 DAG 结构
3. 查询关联的 Pipeline 状态：
   - Pipeline running → 恢复监听（Pipeline Engine 自己会恢复执行）
   - Pipeline done → 触发下游调度
   - Pipeline failed → 根据 FailPolicy 处理
4. 重新计算各 Issue 的入度和状态

> DAG Scheduler 的完整配置见 [spec-api-config.md](spec-api-config.md) Section III。

## 七、基础设施抽象

### ReviewGate 插件

```go
type ReviewGate interface {
    Plugin
    // 提交一批 Issue 进行审核，返回审核 ID
    Submit(ctx context.Context, issues []*Issue) (reviewID string, err error)
    // 查询审核状态
    Check(ctx context.Context, reviewID string) (*ReviewResult, error)
    // 取消审核
    Cancel(ctx context.Context, reviewID string) error
}

type ReviewResult struct {
    Status       string                           // "pending" | "completed"
    Verdicts     map[string]*IssueReviewVerdict   // issueID → verdict
    DAG          *DependencyAnalysis              // 跨 Issue 依赖分析
    AutoApproved bool
}
```

**实现：**

| 实现 | 说明 | 阶段 |
|------|------|------|
| `review-ai-panel` | 两阶段 AI 审核（Per-Issue Review + Dependency Analysis） | P2b ✅ |
| `review-local` | Workbench/TUI 中人工直接审批 | P2a ✅ |
| `review-github-pr` | 创建 PR 提交 issue 描述，等 merge | P3 🔧 |

### Tracker 插件

```go
type Tracker interface {
    Plugin
    // 创建外部任务（如 GitHub Issue）
    CreateIssue(ctx context.Context, issue *Issue) (externalID string, err error)
    // 更新外部任务状态
    UpdateStatus(ctx context.Context, externalID string, status IssueStatus) error
    // 同步依赖关系到外部系统
    SyncDependencies(ctx context.Context, issue *Issue, allIssues []Issue) error
    // 关闭外部任务
    CloseIssue(ctx context.Context, externalID string) error
}
```

**实现：**

| 实现 | 说明 | 阶段 |
|------|------|------|
| `tracker-local` | 纯本地，不同步到外部系统（默认，空实现） | P2a ✅ |
| `tracker-github` | 同步为 GitHub Issue + Label 管理 | P3 🔧 |
| `tracker-linear` | 同步到 Linear | P4 |

`tracker-local` 是一个 no-op 实现——所有状态都在 SQLite 的 `issues` 表中，不需要外部同步。Tracker 插件只负责**镜像**到外部系统，不是核心逻辑。

**命名对齐的好处**：内部 Issue 和 GitHub Issue 同名，`tracker-github` 的实现几乎变成字段直接映射（Title→Title, Body→Body, Labels→Labels, State→State），不再需要概念转换。

## 八、Workbench UI

### 定位

Workbench 是系统的主操作界面，基于 Web 技术构建，内嵌到 Go 二进制中（`embed.FS`）。提供五个核心视图：Chat、Issues、Board、Pipeline，以及独立的 Admin 全局管理界面。

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
│  项目选择器 ▾    │  活跃 Issue 状态摘要  │  Admin │ 设置 ⚙ │
└──────────────────┴──────────────────┴────────┴───────────┘
┌─ 侧栏 ──┐  ┌─ 主面板 ──────────────────────────────────┐
│          │  │                                           │
│ 📂 项目  │  │  视图内容（根据侧栏选择切换）              │
│ ├ app-a  │  │                                           │
│ └ api-b  │  │  • Chat View     — 对话 + 文件选择        │
│          │  │  • Issues View   — Issue 列表 + DAG       │
│ 📋 Issues│  │  • Board View    — 看板式任务跟踪          │
│ ├ #1 auth│  │  • Pipeline View — 单个 Pipeline 详情     │
│ └ #2 db  │  │  • Manage View   — 项目级管理 + 审计日志   │
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
- **Agent 状态指示**：显示 Secretary session 是否存活、当前状态
- **流式输出**：Secretary Agent 回复时流式显示
- **文件变更提示区**（Secretary 写入文件后自动弹出）：
  - 列出 Secretary 新增/修改的文件路径
  - 每个文件可预览内容
  - 分组勾选：用户将文件分组，每组 = 1 个 Issue
  - 「创建 Issue」按钮 → 批量提交创建 Issue

### Issues View（Issue 视图，替代原 Plan View）

主要元素：
- **Issue 列表**：按状态分组，显示 title, labels, status, priority
- **DAG 图**：使用 React Flow 渲染 Issue 依赖图
  - 节点 = Issue，颜色表示状态
  - 边 = 依赖关系，箭头方向从上游到下游
  - 点击节点展开 Issue 详情
- **审核面板**（审核进行中时显示）：
  - Per-Issue Review 结果和评分
  - Dependency Analysis 结果
  - 冲突高亮
- **操作按钮**：
  - 「批准执行」— approve 所有通过审核的 Issue
  - 「修改」— 回到 Chat 调整 plan 文件
  - 「编辑依赖」— 手动调整 DAG 边

### Board View（看板视图）

主要元素：
- **多列看板**：Draft | Queued | Ready | Executing | Done/Failed
- **Issue 卡片**：
  - 标题、标签、优先级
  - 进度指示（executing 时显示当前 Pipeline 阶段）
  - 依赖关系缩略（上游 N 个，下游 N 个）
- **操作**：
  - 点击卡片 → 跳转到 Pipeline View
  - 右键 → retry / skip / abort
- **过滤器**：按状态、标签、模板、里程碑过滤

### Pipeline View（Pipeline 详情）

复用现有 Pipeline 监控能力：
- 阶段进度条（当前在哪个 Stage）
- Agent 实时输出流（WebSocket `agent_output` 事件）
- Checkpoint 列表
- 人工操作按钮（approve / reject / skip / abort）

### Manage View（项目级管理视图）

- **Pipeline 状态面板**：当前项目下所有 Pipeline 列表，按状态分组
- **Issue 概览**：当前项目下所有 Issue，审核状态、执行进度
- **审计日志**：当前项目的操作日志，支持按操作类型/时间过滤

### Admin View（全局管理界面，独立路由 /admin）

- **概览仪表盘**：项目总数、运行中 Pipeline、执行中 Issue、Agent 并发使用
- **全局 Issue 列表**：跨项目查看所有 Issue
- **全局 Pipeline 列表**：跨项目查看所有 Pipeline
- **审计日志浏览器**：全局操作日志
- **系统健康**：Agent CLI 可用性、SQLite 状态、磁盘使用

> REST API 端点、WebSocket 事件协议见 [spec-api-config.md](spec-api-config.md) Section I/II。

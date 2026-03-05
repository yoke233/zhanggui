# AI-Workflow 逆向工程规范

> 从源码逆向提取，不依赖 docs/spec 或 docs/code-spec。
> 生成时间：2026-03-05

---

## 目录

1. [领域模型](#1-领域模型)
2. [状态机](#2-状态机)
3. [插件体系](#3-插件体系)
4. [执行引擎](#4-执行引擎)
5. [Team Leader 编排层](#5-team-leader-编排层)
6. [Web API 层](#6-web-api-层)
7. [前端架构](#7-前端架构)
8. [配置体系](#8-配置体系)
9. [数据库 Schema](#9-数据库-schema)

---

## 1. 领域模型

所有领域类型定义在 `internal/core/`，是系统的不可变内核。

### 1.1 Project

| 字段 | 类型 | JSON | 说明 |
|------|------|------|------|
| ID | string | `id` | 主键 |
| Name | string | `name` | 项目名称 |
| RepoPath | string | `repo_path` | 本地仓库路径 |
| GitHubOwner | string | `github_owner` | GitHub 组织/用户 |
| GitHubRepo | string | `github_repo` | GitHub 仓库名 |
| DefaultBranch | string | `default_branch` | 默认分支 |
| CreatedAt | time.Time | `created_at` | |
| UpdatedAt | time.Time | `updated_at` | |

### 1.2 Issue

最小交付单元，承载目标、上下文、约束与验收标准。

| 字段 | 类型 | JSON | 说明 |
|------|------|------|------|
| ID | string | `id` | 格式 `issue-YYYYMMDD-xxxxxxxx` |
| ProjectID | string | `project_id` | 所属项目 |
| SessionID | string | `session_id` | 关联聊天会话 |
| Title | string | `title` | **必填** |
| Body | string | `body` | 描述体 |
| Labels | []string | `labels` | 标签列表 |
| MilestoneID | string | `milestone_id` | 里程碑 |
| Attachments | []string | `attachments` | 附件路径 |
| DependsOn | []string | `depends_on` | V2 中已弃用 |
| Blocks | []string | `blocks` | V2 中已弃用 |
| Priority | int | `priority` | 优先级（数字越大越高） |
| Template | string | `template` | **必填**，执行模板 `full/standard/quick/hotfix` |
| AutoMerge | bool | `auto_merge` | 完成后自动合并，默认 true |
| State | IssueState | `state` | 跟踪器状态 `open/closed` |
| Status | IssueStatus | `status` | 编排进度状态 |
| RunID | string | `run_id` | 关联的运行实例 |
| Version | int | `version` | 乐观锁版本号 |
| SupersededBy | string | `superseded_by` | 被哪个 Issue 替代 |
| ParentID | string | `parent_id` | 父 Issue（分解场景） |
| ExternalID | string | `external_id` | 外部系统 ID |
| FailPolicy | FailurePolicy | `fail_policy` | `block/skip/human` |
| CreatedAt | time.Time | `created_at` | |
| UpdatedAt | time.Time | `updated_at` | |
| ClosedAt | *time.Time | `closed_at` | 关闭时间 |

**业务规则 - 分解触发条件：**
- `Template == "epic"` **或** `Labels` 包含 `"decompose"`（不区分大小写）
- 满足任一条件时 `NeedsDecomposition()` 返回 true

**验证规则：**
- Title 不能为空
- Template 不能为空且不能含空格
- State/Status 如非空必须在合法值集内

### 1.3 Run

一次执行实例，输入为 Issue + Profile，输出为事件与结果。

| 字段 | 类型 | JSON | 说明 |
|------|------|------|------|
| ID | string | `id` | 格式 `YYYYMMDD-{6随机字节}` |
| ProjectID | string | `project_id` | |
| Name | string | `name` | |
| Description | string | `description` | |
| Template | string | `template` | 执行模板 |
| Status | RunStatus | `status` | `queued/in_progress/completed/action_required` |
| Conclusion | RunConclusion | `conclusion` | `success/failure/timed_out/cancelled` |
| CurrentStage | StageID | `current_stage` | 当前执行阶段 |
| Stages | []StageConfig | `stages` | 阶段配置列表（按模板初始化） |
| Artifacts | map[string]string | `artifacts` | 产物（review_comment, modify_message 等） |
| Config | map[string]any | `config` | 运行配置（trace_id, issue_number, pr_number, base_branch） |
| BranchName | string | `branch_name` | Git 分支名 |
| WorktreePath | string | `worktree_path` | Git worktree 路径 |
| IssueID | string | `issue_id` | 关联 Issue |
| ErrorMessage | string | `error_message` | |
| MaxTotalRetries | int | `max_total_retries` | 全局重试预算，默认 5 |
| TotalRetries | int | `total_retries` | 已用重试次数 |
| RunCount | int | `run_count` | 历史执行次数 |
| LastErrorType | string | `last_error_type` | |
| QueuedAt | time.Time | `queued_at` | |
| LastHeartbeatAt | time.Time | `last_heartbeat_at` | |
| StartedAt | time.Time | `started_at` | |
| FinishedAt | time.Time | `finished_at` | |
| CreatedAt | time.Time | `created_at` | |
| UpdatedAt | time.Time | `updated_at` | |

### 1.4 StageConfig

| 字段 | 类型 | 说明 |
|------|------|------|
| Name | StageID | `setup/requirements/implement/review/fixup/test/merge/cleanup` |
| Agent | string | 代理名称 |
| Role | string | 角色 ID |
| PromptTemplate | string | 提示词模板名 |
| Timeout | time.Duration | 硬超时 |
| IdleTimeout | time.Duration | 活跃超时（默认 5min，test 3min，系统阶段 1min） |
| MaxRetries | int | 阶段级最大重试（默认 1） |
| RequireHuman | bool | 成功后是否需人工批准 |
| OnFailure | OnFailure | `retry/skip/abort/human`（默认 human） |
| ReuseSessionFrom | StageID | 跨阶段 ACP 会话复用来源（如 fixup → implement） |

### 1.5 Checkpoint

| 字段 | 类型 | 说明 |
|------|------|------|
| RunID | string | |
| StageName | StageID | |
| Status | CheckpointStatus | `in_progress/success/failed/skipped/invalidated` |
| Artifacts | map[string]string | 阶段输出 |
| StartedAt | time.Time | |
| FinishedAt | time.Time | |
| AgentUsed | string | |
| TokensUsed | int | |
| RetryCount | int | |
| Error | string | |

### 1.6 RunAction（人工操作）

| 字段 | 类型 | 说明 |
|------|------|------|
| RunID | string | 必填 |
| Type | HumanActionType | 操作类型 |
| Stage | StageID | 目标阶段（默认 CurrentStage） |
| Message | string | 批注内容 |
| Role | string | 用于 change_role |
| CreatedAt | time.Time | |

**9 种操作类型：**

| Type | 语义 | 效果 |
|------|------|------|
| `approve` | 批准 | action_required → in_progress，继续执行 |
| `reject` | 拒绝 | 清空该阶段及之后的 checkpoints |
| `modify` | 修改 | 存 `Artifacts["modify_message"]`，继续执行 |
| `skip` | 跳过 | 进入下一阶段（若无下一阶段则完成） |
| `rerun` | 重跑 | 转 in_progress 重新执行当前阶段 |
| `change_role` | 换角色 | 修改阶段 Role 后重新执行 |
| `abort` | 中止 | Run 标记为 completed + failure |
| `pause` | 暂停 | 清空 ACP 会话池，进入 action_required |
| `resume` | 恢复 | action_required → in_progress |

### 1.7 Event

| 字段 | 类型 | 说明 |
|------|------|------|
| Type | EventType | |
| RunID | string | |
| ProjectID | string | |
| IssueID | string | |
| Stage | StageID | |
| Agent | string | |
| Data | map[string]string | |
| Error | string | |
| Timestamp | time.Time | |

**事件分类：**

| 类别 | 事件 |
|------|------|
| 阶段级 | `stage_start`, `stage_complete`, `stage_failed`, `human_required` |
| Run 级 | `run_done`, `run_action_required`, `run_resumed`, `action_applied`, `agent_output`, `run_stuck` |
| TL 级 | `team_leader_thinking`, `team_leader_files_changed`, `run_started`, `run_update`, `run_completed`, `run_failed`, `run_cancelled` |
| Issue 级 | `issue_created`, `issue_reviewing`, `review_done`, `issue_approved`, `issue_queued`, `issue_ready`, `issue_executing`, `issue_done`, `issue_failed`, `issue_decomposing`, `issue_decomposed`, `issue_dependency_changed`, `auto_merged` |
| GitHub 级 | `github_webhook_received`, `github_issue_opened`, `github_issue_comment_created`, `github_pull_request_review_submitted`, `github_pull_request_closed`, `github_reconnected`, `admin_operation` |

**广播规则：**
- Issue 作用域事件仅发送给订阅了该 Issue 的客户端
- 4 个始终广播事件：`issue_created`, `issue_done`, `issue_failed`, `issue_decomposed`

### 1.8 ReviewResult / ReviewRecord

```
ReviewResult
├─ Status: string (pending/approved/rejected/changes_requested/cancelled)
├─ Decision: string (pending/approve/reject/fix/cancelled)
├─ Verdicts: []ReviewVerdict
│  ├─ Reviewer: string
│  ├─ Status: string
│  ├─ Summary: string
│  ├─ RawOutput: string
│  ├─ Issues: []ReviewIssue
│  │  ├─ Severity, IssueID, Description, Suggestion
│  └─ Score: int
└─ Comments: []string

ReviewRecord（持久化）
├─ ID: int64
├─ IssueID: string
├─ Round: int
├─ Reviewer, Verdict, Summary, RawOutput: string
├─ Issues: []ReviewIssue
├─ Fixes: []ProposedFix {IssueID, Description, Suggestion}
├─ Score: *int
└─ CreatedAt: time.Time
```

### 1.9 WorkflowProfile

| 字段 | 类型 | 说明 |
|------|------|------|
| Type | WorkflowProfileType | `normal/strict/fast_release` |
| SLAMinutes | int | 范围 [1, 60] |

### 1.10 辅助实体

**ChatSession:** `{ID, ProjectID, AgentSessionID, Messages []ChatMessage, CreatedAt, UpdatedAt}`
**ChatMessage:** `{Role: "user"|"assistant", Content, Time}`
**Notification:** `{Level, Title, Body, RunID, ProjectID, ActionURL}`
**PullRequest:** `{Title, Body, Head, Base, Draft, Reviewers}`
**WorkspaceSetupRequest:** `{RepoPath, RunID, BranchName, WorktreePath, DefaultBranch}`

---

## 2. 状态机

### 2.1 Run 状态转移

```
queued ──────────────→ in_progress
                         │
                    ┌────┴────┐
                    ▼         ▼
              completed   action_required
                 │            │
                 │      ┌─────┴─────┐
                 │      ▼           ▼
                 │  in_progress  completed
                 │
                 └──→ in_progress  (retry from failure)
```

**触发条件：**
- `queued → in_progress`：Scheduler CAS 抢占成功
- `in_progress → completed`：所有阶段完成或 failRun()
- `in_progress → action_required`：OnFailure=human / RequireHuman=true / pause 操作
- `action_required → in_progress`：approve / resume 操作
- `completed → in_progress`：retry（特定场景）

### 2.2 Issue 状态转移

```
draft ──→ reviewing ──→ queued ──→ ready ──→ executing ──→ done
  │          │                                    │
  │          ▼                                    ▼
  │       [reject]                             failed
  │          │
  │          ▼
  │       draft (回退)
  │
  ├──→ decomposing ──→ decomposed ──→ [子 Issue 完成] ──→ done/failed
  │
  └──→ abandoned (终态，State → closed)

特殊转换：
  任意状态 ──→ superseded (被替代)
  任意状态 ──→ abandoned (放弃)
```

### 2.3 Checkpoint 状态转移

```
[创建] → in_progress → success   (正常完成)
                     → failed    (执行失败)
                     → skipped   (跳过操作)
success/failed/in_progress → invalidated  (人工拒绝或恢复)
```

### 2.4 A2A 任务状态映射

| Issue Status | A2A Task State |
|---|---|
| draft | submitted |
| reviewing | input-required |
| queued / ready / executing | working |
| done | completed |
| failed | failed |
| superseded / abandoned | canceled |

---

## 3. 插件体系

### 3.1 插件接口层次

```
Plugin (base)
├─ Name() string
├─ Init(ctx) error
└─ Close() error

Store extends Plugin            — 完整数据访问层
ReviewGate extends Plugin       — Submit / Check / Cancel
Tracker extends Plugin          — CreateIssue / UpdateStatus / SyncDependencies / OnExternalComplete
SCM extends Plugin              — CreateBranch / Commit / Push / Merge / CreatePR / UpdatePR / ConvertToReady / MergePR
Notifier extends Plugin         — Notify
WorkspacePlugin extends Plugin  — Setup / Cleanup
Scheduler                       — Start / Stop / Enqueue
```

### 3.2 插件实现清单

| 槽位 | 插件名 | 默认 | 说明 |
|------|--------|------|------|
| Store | store-sqlite | 是 | SQLite WAL 模式，modernc.org 纯 Go |
| ReviewGate | review-ai-panel | 是 | AI 驱动的两阶段评审 |
| ReviewGate | review-github-pr | 否 | GitHub PR Review |
| ReviewGate | review-local | 否 | 本地评审（开发用） |
| Tracker | tracker-local | 是 | 本地 Issue 跟踪 |
| Tracker | tracker-github | 否 | GitHub Issues 同步 |
| SCM | scm-local-git | 是 | 本地 Git 操作 |
| SCM | scm-github | 否 | GitHub API 操作 |
| Workspace | workspace-worktree | 是 | Git worktree 隔离 |
| Workspace | workspace-clone | 否 | Git clone 隔离 |
| Agent | agent-claude | — | Claude ACP 适配 |
| Agent | agent-codex | — | Codex ACP 适配 |
| Notifier | notifier-desktop | 是 | 桌面通知 |

### 3.3 Factory 组装逻辑

```go
BuildFromConfig(cfg) → BootstrapSet {
    RoleResolver  // 从 agents + roles 配置构建
    Store         // sqlite
    ReviewGate    // review-ai-panel 或 review-local
    Tracker       // tracker-local 或 tracker-github（当 github.enabled=true）
    SCM           // local-git 或 scm-github（当 github.enabled=true）
    Notifier      // desktop
    Workspace     // worktree
}
```

### 3.4 Store 接口（完整方法列表）

```
// 项目
ListProjects, GetProject, CreateProject, UpdateProject, DeleteProject

// Run
ListRuns, GetRun, SaveRun, GetActiveRuns, ListRunnableRuns,
CountInProgressRunsByProject, TryMarkRunInProgress

// Checkpoint
SaveCheckpoint, GetCheckpoints, GetLastSuccessCheckpoint, InvalidateCheckpointsFromStage

// Human Action
RecordAction, GetActions

// Chat Session
CreateChatSession, GetChatSession, UpdateChatSession, ListChatSessions

// Issue
CreateIssue, GetIssue, SaveIssue, ListIssues, GetActiveIssues,
GetIssueByRun, GetChildIssues,
SaveIssueAttachment, GetIssueAttachments,
SaveIssueChange, GetIssueChanges

// Review
SaveReviewRecord, GetReviewRecords

// Events
AppendChatRunEvent, ListChatRunEvents, SaveRunEvent, ListRunEvents
```

---

## 4. 执行引擎

### 4.1 Executor 结构

```
Executor
├─ store: Store
├─ bus: EventBus
├─ roleResolver: RoleResolver
├─ stageRoles: map[StageID]string
├─ workspace: WorkspacePlugin
├─ acpHandlerFactory: ACPHandlerFactory
├─ acpPool: map[string]*acpSessionEntry    // key = "runID:stageID"
└─ testStageFunc: func(...)                // 测试 hook
```

### 4.2 主执行循环

```
executor.run(ctx, runID):
  1. 加载 Run，验证状态 queued → in_progress
  2. defer: acpPoolCleanup(runID)
  3. 从 checkpoint 恢复起始位置
  4. FOR each stage:
     a. 发布 EventStageStart
     b. FOR attempt := 0:
        i.   创建 Checkpoint(in_progress)
        ii.  executeStage(ctx, stage)
        iii. 成功 → Checkpoint(success), break
        iv.  检查 isRunActionRequired → return nil
        v.   失败 → Checkpoint(failed), TotalRetries++
        vi.  全局预算耗尽 → failRun
        vii. 评估反应规则:
             - ReactionRetry     → continue (attempt < maxRetries) 或 failRun
             - ReactionSkipStage → Checkpoint(skipped), break
             - ReactionEscalateHuman → action_required, return nil
             - ReactionAbortRun  → failRun
     c. RequireHuman → action_required, return nil
  5. 全部完成 → completed + success
```

### 4.3 阶段执行分发

| 阶段 | 执行方式 | 说明 |
|------|---------|------|
| setup | runWorktreeSetup | workspace.Setup → 创建分支和 worktree |
| merge | runMerge | git checkout + git merge |
| cleanup | runCleanup | workspace.Cleanup |
| requirements, implement, review, fixup, test | runACPStage | ACP 协议执行 |

### 4.4 ACP 执行流程

```
runACPStage(ctx, agentName, agentProfile, roleProfile, run, stage, prompt):
  1. 超时控制：
     - IdleTimeout > 0 → startIdleChecker（后台监控 lastActivity 原子变量）
     - Timeout > 0     → WithTimeout
  2. 会话复用：
     - ReuseSessionFrom → 从 acpPool 获取已有 session
     - miss → 创建新 client + session
  3. 创建 ACP 客户端：
     handler = acpHandlerFactory.NewHandler(WorktreePath, eventBus)
     SetPermissionPolicy(handler, roleProfile.PermissionPolicy)
     client = acpclient.New(LaunchConfig, handler, WithEventHandler(bridge))
     client.Initialize(ctx, roleProfile.Capabilities)
  4. 新建会话：
     sessionID = client.NewSession(ctx, {Cwd: WorktreePath})
     存入 acpPool
  5. 发送提示词：
     result = client.Prompt(ctx, {SessionId, Prompt})
     发布 EventAgentOutput(done)
```

### 4.5 事件桥接 (stageEventBridge)

实现 `acpclient.EventHandler`：
- 每收到 SessionUpdate → 更新 lastActivity（空闲超时检测）
- 若 update.Text 非空 → 发布 EventAgentOutput

### 4.6 调度器

```
Scheduler
├─ maxGlobal: int         // 全局最大并发（默认 3）
├─ maxPerProject: int     // 项目级并发限制（默认 2）
├─ pollInterval: Duration // 轮询周期（默认 200ms）

RunOnce():
  1. 获取所有 active runs
  2. 统计 runningCount 和 busyWorktrees
  3. 计算 slots = maxGlobal - runningCount
  4. 获取候选 runnable = ListRunnableRuns(maxGlobal * 4)
  5. 逐个检查:
     - 项目级限制
     - Worktree 冲突
     - CAS 抢占: TryMarkRunInProgress(id, StatusQueued)
  6. 异步启动: go s.run(ctx, runID)
```

### 4.7 恢复机制

```
RecoverActiveRuns():
  对每个 in_progress 的 Run:
  1. 查找中断的 in_progress checkpoint
  2. 标记为 failed（error="recovered: previous checkpoint interrupted by crash"）
  3. 清理 worktree（git reset --hard + git clean -fd）
  4. 重新加入调度队列
```

### 4.8 反应规则

```
stage.OnFailure → 编译为 ReactionRule:

OnFailureRetry  → ReactionRetry      (自动重试至 maxRetries)
OnFailureHuman  → ReactionEscalateHuman (立即升级)
OnFailureSkip   → ReactionSkipStage  (跳过)
OnFailureAbort  → ReactionAbortRun   (中止)
```

---

## 5. Team Leader 编排层

### 5.1 Manager

```
Manager
├─ store: Store
├─ scheduler: Scheduler (DepScheduler)
├─ reviewGate: ReviewGate
├─ twoPhaseReview: TwoPhaseReview
├─ reviewSubmitter: ReviewSubmitter
├─ pub: EventPublisher
```

**Manager 核心方法：**

| 方法 | 说明 |
|------|------|
| `Start(ctx)` | 启动 scheduler + 恢复执行中 Issue |
| `Stop(ctx)` | 停止 scheduler |
| `CreateIssues(ctx, input)` | 批量创建 Issue（验证 + 默认值 + 持久化） |
| `SubmitForReview(ctx, issueIDs)` | 提交评审（两阶段评审 → 自动批准） |
| `ApplyIssueAction(ctx, issueID, action, feedback)` | 执行 approve/reject/abandon |

### 5.2 Issue 操作流程

**CreateIssues：**
1. 验证 ProjectID 非空、Issues 非空
2. 每个 Issue：生成 ID、设置默认 Template=standard、FailPolicy=block、AutoMerge=true
3. Validate → CreateIssue → GetIssue（返回持久化版本）

**SubmitForReview：**
1. 加载 Issue 对象，记录评审基线轮次
2. 委托：twoPhaseReview > reviewSubmitter > reviewGate
3. 状态 → reviewing
4. AutoMerge=true 时检查 shouldAutoApproveIssue → applyIssueApprove

**ApplyIssueAction(approve)：**
1. NeedsDecomposition? → 转 decomposing，发布 EventIssueDecomposing
2. 常规 → 转 queued → DepScheduler.StartIssue

**ApplyIssueAction(reject)：**
- 需要 feedback 非空
- 状态回退 → draft

**ApplyIssueAction(abandon)：**
- 状态 → abandoned，State → closed，设置 ClosedAt

### 5.3 两阶段评审

```
TwoPhaseReview.Run(ctx, issues):
  Phase 1: 需求评审 (DemandReviewer)
    - Strict Profile: 3 reviewer, 阈值 ≥85
    - FastRelease: 1 reviewer, 无阈值
    - Normal: 1 reviewer, 应用配置阈值

  Phase 2: 依赖分析 (DependencyAnalyzer)
    - len(issues) > 1 且 profile != FastRelease 时执行

  决策:
    - 有冲突 → Escalate（拒绝）
    - 需修复 → Fix（变更请求）
    - 不满足阈值 → Fix
    - 否则 → Approve（自动通过）
```

**内置评审器：**
- `completeness` — 验证 Title/Body/Template 完整性
- `dependency` — 检查依赖关系（V2 简化）
- `feasibility` — 验证 template 在允许列表中（full/standard/quick/hotfix）

### 5.4 分解处理器 (DecomposeHandler)

事件驱动，监听 `EventIssueDecomposing`：
1. 加载父 Issue（验证 status=decomposing）
2. 调用 decompose(ctx, parent) → []DecomposeSpec
3. 为每个 spec 创建子 Issue（ParentID=parent.ID，继承 AutoMerge/FailPolicy）
4. 父 Issue 状态 → decomposed
5. 可选自动提交子 Issue 评审

### 5.5 自动合并处理器 (AutoMergeHandler)

事件驱动，监听 `EventRunDone`：
1. 验证 Issue.AutoMerge=true
2. 测试门控：检测修改的 Go 包 → `go test ./changed-packages...`
3. 创建 PR → 合并 PR
4. 成功 → EventAutoMerged

### 5.6 子 Issue 完成处理器 (ChildCompletionHandler)

事件驱动，监听 `EventIssueDone` + `EventIssueFailed`：
1. 检查 ParentID 非空
2. 加载父 Issue（status=decomposed）
3. 统计兄弟 Done/Failed/其他
4. 全部完成且无失败 → 父 Done
5. 有失败 → 根据 FailPolicy：
   - FailSkip → 视为成功
   - FailHuman → 等待人工
   - FailBlock → 父 Failed

### 5.7 DepScheduler（依赖感知调度器）

```
DepScheduler
├─ store, bus, tracker
├─ sem: chan struct{}         // 并发信号量
├─ sessions: map[sessionID]*runningSession
├─ RunIndex: map[runID]RunRef
├─ reconcileInterval: Duration
```

**StartIssue → 创建 Run → 获取信号量 → 异步 runRun(runID)**

**事件响应：**
- EventRunDone → Issue 状态 → Done
- EventRunFailed → Issue 状态 → Failed

### 5.8 ACP Handler（Agent 交互层）

处理 Agent 工具调用：

| 操作 | 说明 |
|------|------|
| ReadTextFile | 规范化路径 + 检查 CWD 范围 + 行窗口 |
| WriteTextFile | 规范化路径 + 创建目录 + 记录修改 + 发布 EventTeamLeaderFilesChanged |
| CreateTerminal | 验证命令 + 启动进程 + 管道 stdout/stderr |
| GetTerminalOutput | 返回缓冲区快照 |
| RequestPermission | 遍历 PermissionRule[] 匹配 pattern + scope → 决策 |

**权限规则格式：**
```yaml
permission_policy:
  - pattern: "fs/write_text_file"
    scope: "cwd"
    action: "allow_always"
  - pattern: "terminal/create"
    action: "allow_once"
```

### 5.9 A2A Bridge

| 方法 | 说明 |
|------|------|
| SendMessage | 解析项目范围 → CreateIssues(标签=["a2a"]) → 自动 approve |
| GetTask | 加载 Issue → 映射为 A2ATaskSnapshot |
| CancelTask | ApplyIssueAction("abandon") |

### 5.10 MCP 工具

角色配置声明 MCPTools 列表。两种传输模式：
- **SSE 模式**：连接到 `/mcp` 端点
- **Stdio 模式**：启动 `ai-flow mcp-serve` 子进程，传递 `AI_WORKFLOW_DB_PATH` 等环境变量

可用工具：`query_projects`, `query_project_detail`, `query_issues`, `query_issue_detail`, `query_runs`, `query_run_detail`, `query_run_events`, `query_project_stats`

---

## 6. Web API 层

### 6.1 路由总表

#### 系统
```
GET  /health
GET  /api/v1/health
GET  /api/v1/stats
```

#### 项目
```
GET   /api/v1/projects
POST  /api/v1/projects
GET   /api/v1/projects/{id}
POST  /api/v1/projects/create-requests
GET   /api/v1/projects/create-requests/{id}
```

#### 仓库
```
GET  /api/v1/projects/{projectID}/repo/tree
GET  /api/v1/projects/{projectID}/repo/status
GET  /api/v1/projects/{projectID}/repo/diff
```

#### 聊天
```
GET    /api/v1/projects/{projectID}/chat
POST   /api/v1/projects/{projectID}/chat
GET    /api/v1/projects/{projectID}/chat/{sessionID}
POST   /api/v1/projects/{projectID}/chat/{sessionID}/cancel
DELETE /api/v1/projects/{projectID}/chat/{sessionID}
GET    /api/v1/projects/{projectID}/chat/{sessionID}/events
```

#### Issue
```
GET    /api/v1/projects/{projectID}/issues
POST   /api/v1/projects/{projectID}/issues
POST   /api/v1/projects/{projectID}/issues/from-files
GET    /api/v1/projects/{projectID}/issues/{issueID}
GET    /api/v1/projects/{projectID}/issues/{issueID}/timeline
GET    /api/v1/projects/{projectID}/issues/{issueID}/dag
GET    /api/v1/projects/{projectID}/issues/{issueID}/status
PATCH  /api/v1/projects/{projectID}/issues/{issueID}/status
POST   /api/v1/projects/{projectID}/issues/{issueID}/actions
GET    /api/v1/projects/{projectID}/issues/{issueID}/auto-merge
PATCH  /api/v1/projects/{projectID}/issues/{issueID}/auto-merge
POST   /api/v1/projects/{projectID}/issues/{issueID}/review
```

#### V2 工作流
```
GET  /api/v2/workflow-profiles
GET  /api/v2/workflow-profiles/{type}
GET  /api/v2/issues
GET  /api/v2/issues/{id}
GET  /api/v2/runs
GET  /api/v2/runs/{id}
GET  /api/v2/runs/{id}/events
```

#### 管理员
```
POST  /api/v1/admin/ops/force-ready
POST  /api/v1/admin/ops/force-unblock
POST  /api/v1/admin/ops/replay-delivery
GET   /api/v1/admin/audit-log
```

#### 协议端点
```
POST  /api/v1/a2a                          [A2A JSON-RPC 2.0]
GET   /.well-known/agent-card.json         [A2A Agent Card]
GET   /mcp                                 [MCP SSE]
POST  /webhook                             [GitHub Webhook]
GET   /api/v1/ws                           [WebSocket]
```

### 6.2 中间件栈

```
RecoveryMiddleware → LoggingMiddleware → CORSMiddleware → [BearerAuthMiddleware]
```

BearerAuth 支持 `?token=` 查询参数和 `Authorization: Bearer` 头，使用 `crypto/subtle.ConstantTimeCompare`。

### 6.3 WebSocket 协议

**客户端消息：**
```json
{"type": "subscribe_run", "run_id": "..."}
{"type": "subscribe_issue", "issue_id": "..."}
{"type": "subscribe_chat_session", "session_id": "..."}
```

**服务端消息：**
```json
{
  "type": "<event_type>",
  "run_id": "...",
  "project_id": "...",
  "issue_id": "...",
  "session_id": "...",
  "data": {}
}
```

**参数：** writeWait=10s, pongWait=60s, pingPeriod=30s, maxMessage=1MB, maxBufferedChatSessionEvents=32

### 6.4 A2A JSON-RPC

方法：`message/send`, `message/stream`, `tasks/get`, `tasks/cancel`

错误码：-32600 (Invalid Request), -32602 (Invalid Params), -32603 (Internal Error), -32601 (Method Not Found), -32004 (Task Not Found), -39001 (Project Scope Mismatch)

### 6.5 核心请求/响应类型

**项目创建（异步）：**
```
POST /api/v1/projects/create-requests
Body: {name, source_type: "local_path|local_new|github_clone", repo_path?, remote_url?, ref?}
→ {request_id, status}

GET /api/v1/projects/create-requests/{id}
→ {request_id, source_type, status, project_id?, repo_path?, step?, message?, progress, error?}
```

**Issue 创建：**
```
POST /api/v1/projects/{projectID}/issues
Body: {session_id, name?, fail_policy?, auto_merge?}
→ Issue[]

POST /api/v1/projects/{projectID}/issues/from-files
Body: {session_id, name?, fail_policy?, auto_merge?, file_paths[]}
→ Issue[]
```

**Issue 操作：**
```
POST /api/v1/projects/{projectID}/issues/{issueID}/actions
Body: {action: "approve|reject|abandon", feedback?: {category, detail, expected_direction?}}
```

**限制：** 单文件 1MB，总大小 5MB，反馈最少 20 Unicode 字符

---

## 7. 前端架构

### 7.1 技术栈

React 18 + Vite 5 + TypeScript 5 + Tailwind 3 + Zustand 5

### 7.2 类型系统

前端类型定义在 `web/src/types/` 中，与后端领域模型对齐：
- `workflow.ts` — Project, Run, Issue, ChatSession 等核心实体
- `api.ts` — 所有 API 请求/响应类型 + 分页泛型
- `ws.ts` — WebSocket 消息格式
- `a2a.ts` — A2A JSON-RPC 协议类型

### 7.3 状态管理 (Zustand Stores)

| Store | 状态 | 关键操作 |
|-------|------|---------|
| ProjectsStore | `projects[], selectedProjectId, loading, error` | setProjects, upsertProjects, selectProject |
| RunsStore | `RunsByProjectId{}, selectedRunId, loading, error` | setRuns, upsertRuns, selectRun |
| ChatStore | `sessionsByProjectId{}, activeSessionId, loading, error` | setSessions, upsertSession, appendMessage, selectSession |

合并策略：ID-based Map merge（避免覆盖）

### 7.4 API 客户端

| 客户端 | 文件 | 说明 |
|--------|------|------|
| ApiClient | `lib/apiClient.ts` | REST API，类方法风格，含归一化函数 |
| WsClient | `lib/wsClient.ts` | WebSocket，指数退避重连，按 type 分发 |
| A2AClient | `lib/a2aClient.ts` | JSON-RPC 2.0，支持 SSE 流式 |

**ApiClient 归一化：**
- `normalizeApiRun` — 标准化 GitHub 字段
- `normalizeApiIssue` — 解析 github ref、数组字段
- `normalizeIssueTimelineEntry` — 验证必填字段、填充默认值

### 7.5 视图组件

| 视图 | 说明 | 关键功能 |
|------|------|---------|
| ChatView | 对话式交互 | 流式响应、文件树/Git Status 面板、从文件创建 Issue |
| BoardView | 看板视图 | 5 列 Kanban、Issue Timeline、操作按钮 |
| RunView | 运行监控 | Run 列表、事件流、GitHub 关联 |
| A2AChatView | A2A 对话 | JSON-RPC 发送/取消/查询 |

**组件树：**
```
App
├─ ProjectAdminPanel (项目创建/选择)
├─ ChatView
│  ├─ FileTree (递归目录树 + Git 状态 + 文件选择)
│  ├─ GitStatusPanel (变更分组 + Diff 查看)
│  └─ DiffViewer (unified diff 渲染)
├─ BoardView
│  └─ TimelineItem (事件时间轴)
├─ RunView
│  └─ GitHubStatusBadge
└─ A2AChatView
```

### 7.6 环境变量

```
VITE_API_BASE_URL  — API 根路径，默认 "/api/v1"
VITE_API_TOKEN     — Bearer Token
VITE_A2A_ENABLED   — A2A 开关，默认 true
```

### 7.7 数据流模式

```
用户输入 → API 调用 → Store 更新 → React 重渲染
                                      ↑
WS 推送 → 事件路由(type) → 增量更新 ──┘
```

关键模式：
- Request ID Ref 追踪，丢弃过期响应
- 409 Conflict 触发全量刷新 + 跨终端同步提示
- 历史事件加载失败不阻断主流程

---

## 8. 配置体系

### 8.1 Config 结构

```yaml
agents:              # Agent 定义列表
  - name: claude
    launch_command: "npx"
    launch_args: ["-y", "@zed-industries/claude-agent-acp@latest"]
    capabilities_max: {fs_read: true, fs_write: true, terminal: true}

roles:               # Role 定义列表
  - name: team_leader
    agent: claude
    prompt_template: secretary_system
    capabilities: {fs_read: true, fs_write: true, terminal: true}
    permission_policy:
      - pattern: "fs/write_text_file"
        scope: "cwd"
        action: "allow_always"
    mcp:
      enabled: true
      tools: [query_projects, query_issues, ...]
    session:
      reuse: true
      max_turns: 0

role_binds:          # 角色绑定
  team_leader: {role: team_leader}
  run:
    stage_roles:
      setup: worker
      implement: worker
      review: reviewer
      fixup: worker
      test: worker
      merge: worker
  review_orchestrator:
    reviewers: {demand: reviewer}
    aggregator: aggregator

run:                 # 运行行为
  default_template: standard
  global_timeout: 2h
  auto_infer_template: true

scheduler:           # 并发控制
  max_global_agents: 3
  max_project_pipelines: 2

team_leader:         # TL 配置
  review_orchestrator:
    enabled: true
    max_rounds: 2
    min_score: 70
  dag_scheduler:
    fail_policy: block
    max_concurrent_tasks: 0
    dispatch_interval: 1s

server:              # Web 服务器
  host: "127.0.0.1"
  port: 8080
  auth_enabled: false

store:               # 数据库
  driver: sqlite
  path: .ai-workflow/data.db

github:              # GitHub 集成
  enabled: false
  pr:
    auto_create: false
    draft: true
    auto_merge: false

a2a:                 # A2A 协议
  enabled: false
```

### 8.2 配置层次

```
configs/defaults.yaml          (内置默认)
  ↓ 被覆盖
.ai-workflow/config.yaml       (项目本地)
  ↓ 被覆盖
~/.ai-workflow/config.yaml     (用户级)
```

合并逻辑在 `internal/config/` 中实现。

### 8.3 预定义角色

| 角色 | Agent | 用途 | 权限特点 |
|------|-------|------|---------|
| team_leader | claude | 对话编排 | 读写+终端+MCP |
| worker | codex | 实现/测试 | 读写+终端 |
| reviewer | claude | 代码评审 | 只读 |
| aggregator | claude | 评审聚合 | 只读 |
| decomposer | claude | Issue 分解 | 只读 |
| plan_parser | claude | 计划解析 | 只读 |

### 8.4 运行模板

| 模板 | 阶段序列 |
|------|---------|
| full | setup → requirements → implement → review → fixup → test → merge → cleanup |
| standard | setup → implement → review → fixup → test → merge → cleanup |
| quick | setup → implement → test → merge → cleanup |
| hotfix | setup → implement → merge → cleanup |

---

## 9. 数据库 Schema

SQLite WAL 模式，Schema Version 3。

### 核心表

**projects**
```sql
id TEXT PK, name TEXT NOT NULL, repo_path TEXT NOT NULL UNIQUE,
github_owner TEXT, github_repo TEXT, default_branch TEXT DEFAULT '',
config_json TEXT, created_at, updated_at
```

**runs**
```sql
id TEXT PK, project_id TEXT FK→projects, name TEXT NOT NULL,
description TEXT, template TEXT NOT NULL, status TEXT DEFAULT 'queued',
conclusion TEXT, current_stage TEXT, stages_json TEXT NOT NULL,
artifacts_json TEXT DEFAULT '{}', config_json TEXT DEFAULT '{}',
branch_name TEXT, worktree_path TEXT, error_message TEXT,
max_total_retries INT DEFAULT 5, total_retries INT DEFAULT 0,
run_count INT DEFAULT 0, last_error_type TEXT, issue_id TEXT,
queued_at, last_heartbeat_at, started_at, finished_at, created_at, updated_at
```

**issues**
```sql
id TEXT PK, project_id TEXT FK→projects, session_id TEXT FK→chat_sessions,
title TEXT NOT NULL, body TEXT DEFAULT '', labels TEXT DEFAULT '[]',
milestone_id TEXT DEFAULT '', attachments TEXT DEFAULT '[]',
depends_on TEXT DEFAULT '[]', blocks TEXT DEFAULT '[]',
priority INT DEFAULT 0, template TEXT DEFAULT 'standard',
auto_merge INT DEFAULT 1, state TEXT DEFAULT 'open',
status TEXT DEFAULT 'draft', run_id TEXT, version INT DEFAULT 1,
superseded_by TEXT DEFAULT '', external_id TEXT,
fail_policy TEXT DEFAULT 'block', parent_id TEXT DEFAULT '',
created_at, updated_at, closed_at
```

**checkpoints**
```sql
id INTEGER PK AUTOINCREMENT, run_id TEXT FK→runs,
stage TEXT NOT NULL, status TEXT NOT NULL, agent_used TEXT,
artifacts_json TEXT DEFAULT '{}', tokens_used INT DEFAULT 0,
retry_count INT DEFAULT 0, error_message TEXT,
started_at DATETIME NOT NULL, finished_at, created_at
```

### 辅助表

| 表 | 说明 |
|---|---|
| `chat_sessions` | 聊天会话 |
| `chat_run_events` | 聊天运行事件 |
| `human_actions` | 人工干预记录 |
| `issue_attachments` | Issue 附件 |
| `issue_changes` | Issue 变更历史 |
| `review_records` | 审核记录 |
| `run_events` | 运行事件 |

### 迁移历史

| 版本 | 变更 |
|------|------|
| 1 | status/conclusion 状态转换、删除 logs 表 |
| 2 | 添加 projects.default_branch |
| 3 | 添加 issues.parent_id（Issue 分解支持） |

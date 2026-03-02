# Secretary Layer — 设计文档

> 本文档是 [spec-overview.md](spec-overview.md) 中 Secretary Layer 的详细设计。整体架构、插件体系、目录结构、设计原则、实施分期见总览文档。API 端点、WebSocket 协议、SQL Schema、配置 YAML 见 [spec-api-config.md](spec-api-config.md)。Event Bus 事件定义见 [spec-pipeline-engine.md](spec-pipeline-engine.md) Section VII。AgentPlugin 和 Claude Driver 接口见 [spec-agent-drivers.md](spec-agent-drivers.md)。

## 概述

Secretary Layer 是系统的上层编排层，位于 Orchestrator Core 之上。它负责：通过对话理解用户需求、将复杂需求拆解为结构化子任务、通过 Multi-Agent 审核委员会自动审核纠错、用 DAG Scheduler 管理子任务间的依赖并行调度。每个子任务 1:1 对应一个 Pipeline，由现有 Pipeline Engine 执行。

## 一、Secretary Agent

### 职责

Secretary Agent 是用户的入口。用户通过对话描述需求，Secretary Agent 理解上下文后将需求拆解为可独立执行的子任务清单（TaskPlan）。

### 实现方式

Secretary Agent **不是新的 Agent 类型**，而是复用现有 AgentPlugin（Claude Driver）的一次特殊调用：

```
用户对话历史 → 构造特殊 Prompt → Claude Driver → JSON 输出 → 解析为 TaskPlan
```

具体流程：

1. 前端积累对话历史（ChatSession）
2. 用户点击"生成任务清单"或发送 `/plan` 指令
3. Secretary Agent 构造 prompt，包含：
   - 完整对话历史
   - 项目上下文（目录结构、技术栈、现有代码摘要）
   - 输出格式要求（严格 JSON schema）
4. 通过 AgentPlugin.BuildCommand() 构造 Claude CLI 调用
5. 解析 Claude 输出为 TaskPlan 结构
6. 写入 Store，状态为 `draft`

### Prompt 构造规则

```
你是一个资深的软件架构师。请基于以下对话历史，将用户的需求拆解为可独立执行的子任务清单。

对话历史：
{{.Conversation}}

项目信息：
- 名称：{{.ProjectName}}
- 技术栈：{{.TechStack}}
- 仓库路径：{{.RepoPath}}

要求：
1. 每个任务必须足够独立，可以被 AI 编码 Agent 单独完成
2. 每个任务必须有结构化的 inputs（前置输入）、outputs（交付产物）和 acceptance（验收标准）
3. 标注任务间的依赖关系（哪些任务必须在另一些之后执行）
4. 为每个任务建议合适的执行模板（full/standard/quick/hotfix）
5. 尽量最大化可并行的任务数量

输出格式（严格 JSON）：
{
  "name": "计划名称",
  "tasks": [
    {
      "id": "task-1",
      "title": "任务标题",
      "description": "做什么、为什么",
      "inputs": ["pkg/auth/token.go — TokenService 接口"],
      "outputs": ["pkg/auth/oauth.go — 新建 OAuthProvider"],
      "acceptance": [
        "go test ./pkg/auth/... 全部通过",
        "/login 端点返回 302 重定向到 OAuth provider"
      ],
      "labels": ["backend", "database"],
      "depends_on": [],
      "template": "standard"
    }
  ]
}

只输出 JSON，不要其他内容。
```

### 项目上下文获取

Secretary Agent 在构造 prompt 前需要收集项目上下文，通过只读方式获取：

| 信息 | 获取方式 |
|------|---------|
| 目录结构 | Go `filepath.WalkDir` 遍历，按扩展名过滤（`.go`/`.ts`/`.py` 等），截取前 200 条路径 |
| 技术栈 | 检测 `go.mod` / `package.json` / `Cargo.toml` 等标记文件（Go `os.Stat` 判断存在性） |
| 现有代码摘要 | 读取 README.md、主入口文件的前 100 行（Go `bufio.Scanner`） |
| Git 状态 | `git log --oneline -20`（git CLI 跨平台通用） |

上下文总量控制在 token 预算内（默认 4000 tokens），超出时按优先级截断。这些参数可在 `secretary:` 配置段中覆盖，见 [spec-api-config.md](spec-api-config.md) Section III。

### 对话管理

对话以 ChatSession 为单位存储在 SQLite 中：

```go
type ChatSession struct {
    ID        string
    ProjectID string
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
- 对话历史发送给 LLM 时，如果超过 context window 上限，截取最近的 N 条消息（默认上限 100 条，保证最后一条用户消息完整）

> 对应的 SQL schema 见 [spec-api-config.md](spec-api-config.md) Section IV。ID 生成规则见 Section V。

## 二、TaskPlan 数据模型

### 核心结构

```go
type TaskPlan struct {
    ID          string
    ProjectID   string
    SessionID   string            // 关联的 ChatSession
    Name        string            // "add-oauth-login"
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

TaskPlan 创建后自动进入审核流程。审核由多个专项 Agent 并行执行，各自关注不同维度，最后由 Aggregator Agent 综合研判。整个过程自动运行，只在反复修正失败时才升级到人工。

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

每个 Reviewer 和 Aggregator 都是一次 AgentPlugin（Claude Driver）调用，只是 prompt 不同。不需要新的基础设施。与普通 Pipeline Stage 执行的区别：不创建 worktree、不写 Checkpoint、超时更短（默认 5 分钟）、AllowedTools 只需 Read。

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

Aggregator 也是一次 LLM 调用，输入是三个 Reviewer 的意见，输出是最终决策：

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
type ReviewPanel struct {
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

> 审核面板的完整配置（`secretary.review_panel:`）见 [spec-api-config.md](spec-api-config.md) Section III。

## 四、TaskPlan 细化（可选，P2a+）

### 背景

Secretary Agent 输出的 TaskItem 偏向"做什么"（需求级），但执行器需要"怎么做"（实施级）。`taskplan_refine` 是一个可选的中间步骤，在审核通过后、调度执行前，为每个 TaskItem 补充实施级细节。

### 触发条件

- TaskPlan 进入 `approved` 状态后自动触发（如果配置开启）
- 配置项：`secretary.refine_enabled: false`（默认关闭，V1 可跳过）

### 细化内容

对每个 TaskItem，通过一次 Agent 调用（复用 Claude Driver）补充：

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

Workbench 是系统的主操作界面，基于 Web 技术构建，内嵌到 Go 二进制中（`embed.FS`）。提供四个核心视图：Chat、Plan、Board、Pipeline。

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
│  项目选择器 ▾    │  当前计划状态指示  │  设置 ⚙       │
└──────────────────┴──────────────────┴────────────────────┘
┌─ 侧栏 ──┐  ┌─ 主面板 ──────────────────────────────────┐
│          │  │                                           │
│ 📂 项目  │  │  视图内容（根据侧栏选择切换）              │
│ ├ app-a  │  │                                           │
│ └ api-b  │  │  • Chat View    — 对话 + 任务生成         │
│          │  │  • Plan View    — DAG 可视化 + 审核状态    │
│ 📋 计划  │  │  • Board View   — 看板式任务跟踪          │
│ ├ Plan A │  │  • Pipeline View — 单个 Pipeline 详情     │
│ └ Plan B │  │                                           │
│          │  │                                           │
│ ⚡ 活跃  │  │                                           │
│ 3 running│  │                                           │
│          │  │                                           │
│ 📊 历史  │  │                                           │
│          │  │                                           │
└──────────┘  └───────────────────────────────────────────┘
```

### Chat View（对话视图）

主要元素：
- **对话历史区域**：显示用户消息和秘书 Agent 回复，支持 Markdown 渲染
- **输入框**：支持多行输入、Shift+Enter 换行、Enter 发送
- **操作按钮**：
  - 「生成任务清单」— 触发 Secretary Agent 任务拆解
  - 「清空对话」— 开始新的 ChatSession
- **流式输出**：Secretary Agent 回复时流式显示（通过 WebSocket 接收 `secretary_thinking` 事件）

### Plan View（计划视图）

主要元素：
- **DAG 图**：使用 React Flow 渲染任务依赖图
  - 节点 = TaskItem，颜色表示状态
  - 边 = 依赖关系，箭头方向从上游到下游
  - 点击节点展开任务详情
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

> REST API 端点、WebSocket 事件协议见 [spec-api-config.md](spec-api-config.md) Section I/II。完整目录结构见 [spec-overview.md](spec-overview.md)。

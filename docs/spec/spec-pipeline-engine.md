# Pipeline Engine — 设计文档

## 概述

Pipeline Engine 是系统的核心，负责定义工作流阶段、管理状态流转、处理重试/超时、协调人工介入。它是一个基于检查点的状态机，不是简单的顺序执行。

## 一、Stage 定义

### 所有可用阶段

> **设计决策**：spec 生成和审核已上提到 Secretary Layer（TaskPlan 级），Pipeline 只负责纯执行。
> "做什么"由 TaskPlan 决定，Pipeline 通过 `task_item_id` 关联 TaskItem 后进入 requirements → implement 流程。
> 详见 [spec-secretary-layer.md](spec-secretary-layer.md)。

| Stage ID | 职责 | 执行者 | 典型耗时 |
|---|---|---|---|
| `requirements` | 结构化需求描述 | Claude | 1-3 min |
| `worktree_setup` | 创建 git worktree + 复制文件 | Git | < 10s |
| `implement` | 根据需求描述实现代码 | Codex 或 Claude | 5-30 min |
| `code_review` | 审查实现质量 | Claude | 3-10 min |
| `fixup` | 修复 review 问题 | Codex 或 Claude | 3-15 min |
| `e2e_test` | 端到端测试（full 模板包含） | Codex | 2-10 min |
| `cleanup` | worktree 移除 + 分支清理 | Git | < 10s |
| `merge` | 合并到目标分支 | Git | < 30s |

### 每个 Stage 的配置项

```yaml
stage:
  name: implement
  agent: codex                   # 由谁执行
  prompt_template: "implement"   # 引用哪个 prompt 模板
  timeout: 30m                   # 超时时间
  max_retries: 1                 # 失败后重试次数
  require_human: false           # 完成后是否等人工确认
  on_failure: human              # 失败时动作: retry / skip / abort / human
  depends_on: []                 # 阶段依赖（用于未来并行执行）
  condition: ""                  # 条件表达式（空 = 始终执行）
```

### Stage 间的数据传递

每个 Stage 执行完毕会写入 `Artifacts`，后续 Stage 可以读取：

```
requirements → artifacts["requirements_doc"]
worktree     → artifacts["worktree_path"], artifacts["branch_name"]
implement    → artifacts["implement_summary"]
               artifacts["progress_md"]          -- 执行期进度文件内容（如启用，见 spec-secretary-layer.md Section 五）
               artifacts["findings_md"]          -- 执行期发现记录（如启用）
code_review  → artifacts["code_review"] (JSON: status + issues)
fixup        → artifacts["fix_summary"]
e2e_test     → artifacts["test_result"] (JSON: status + failures)
cleanup      → artifacts["cleanup_done"]
merge        → artifacts["merge_commit"]
```

规则：
- Artifact 的 key 命名统一用 snake_case
- 值为字符串，复杂结构用 JSON 序列化
- 每次写入同时持久化到 Store，用于崩溃恢复
- 人工介入时传入的反馈也写入 artifacts["human_feedback"]

## 二、Template 机制

### 预设模板

```
full:      requirements → worktree_setup → implement
           → code_review → fixup → e2e_test → merge → cleanup

standard:  requirements → worktree_setup → implement
           → code_review → fixup → merge → cleanup

quick:     requirements → worktree_setup → implement
           → code_review → merge → cleanup

hotfix:    worktree_setup → implement → merge → cleanup
```

> `full` 模板的区分点是 `e2e_test` 阶段（端到端测试），适用于需要严格验证的功能。
> 旧版的“规格生成/规格审核”阶段已移至 Secretary Layer 的 TaskPlan 级别处理。

### 动态 fixup 注入规则

quick 模板不包含 fixup 阶段，但如果 code_review 返回 `{"status": "needs_fix"}`，
Engine 自动在 code_review 和 merge 之间插入一个 fixup stage，无需人工干预。

注意：hotfix 模板没有 code_review 阶段，因此此规则对 hotfix 不会触发。hotfix 的设计意图就是最短路径上线，如果需要 review 应选择 quick 模板。

规则：
- 仅当 code_review 产出的 `artifacts["code_review"]` 包含 `"status": "needs_fix"` 时触发
- 插入的 fixup stage 使用默认配置（max_retries: 2, on_failure: human）
- fixup 完成后自动再跑一次 code_review 验证（形成 review → fix 循环）
- 每次 review-fix 循环消耗 1 次全局重试预算（见 Pipeline.max_total_retries）
- 预算耗尽 → `escalate_human`

### cleanup 阶段

full 和 standard 模板在 **merge 之后** 包含 cleanup 阶段，负责：
- 清理 worktree（`git worktree remove`）
- 清理已合并的分支（`git branch -d`）

cleanup 必须在 merge 之后，因为 merge 需要 worktree 中的代码和分支存在。cleanup 失败不影响 Pipeline 状态：Checkpoint status 仍为 `success`，但在 artifacts 中记录 `cleanup_warnings`（JSON 数组），供审计查询。

所有模板统一走 worktree 隔离（worktree 创建 < 10s，代价极低；不走 worktree 则并发安全无法保证）。

### 模板选择规则

优先级从高到低：

1. **用户显式指定** — CLI 参数 `--template quick` 或 Web 指定
2. **TaskItem.Template** — Secretary Agent 拆解时为每个子任务建议的模板（P2a）
3. **项目配置映射** — 项目配置中定义 `label_mapping: { "bug": "quick" }`（仅适用于 GitHub Issue 触发的 Pipeline，P3）
4. **AI 推断** — 将需求描述发给 Claude，返回模板名称
5. **全局默认** — 配置文件中的 `default_template`，兜底值为 `standard`

### AI 推断规则

Prompt 设计原则：
- 只让 AI 返回一个单词（模板名），不要多余解释
- 提供明确的判断标准，减少歧义
- 超时或解析失败时回退到 `standard`

判断标准给到 AI 的参考：

```
full     → 新功能、涉及多模块、需要端到端测试验证、API 变更
standard → 中等功能、单模块改动、代码审查足够保证质量
quick    → 小 bug、文案修改、样式调整、一两个文件
hotfix   → 线上紧急、改一行代码、配置修改
```

### 自定义模板

用户可在项目配置或全局配置中定义自己的模板：

```yaml
custom_templates:
  ui-only:
    stages: [requirements, worktree_setup, implement, code_review, merge]
    defaults:
      implement_agent: codex
      code_review_human: false

  with-e2e:
    stages: [requirements, worktree_setup, implement, code_review,
             fixup, e2e_test, merge, cleanup]
    defaults:
      e2e_test_timeout: 15m
```

### 动态修改

Pipeline 创建后、执行前，允许增删 Stage：

- `InsertBefore(target, newStage)` — 在某阶段前插入
- `InsertAfter(target, newStage)` — 在某阶段后插入
- `RemoveStage(name)` — 移除某阶段
- `ReplaceStage(old, new)` — 替换

执行中不允许修改未来阶段（避免状态混乱）。人工介入时可以通过 `ActionReject` 回退后再修改。动态 fixup 注入是引擎内部操作（code_review 结果为 needs_fix 时自动插入），不受此限制。

## 三、状态机

### Pipeline 状态

```
                    ┌──────────┐
          ┌────────►│  paused  │◄─── 手动暂停
          │         └────┬──┬──┘
          │    resume │  │ abort
          │              ▼  ▼
created ──►  running ──► waiting_human ──► running ──► done
  │           │              │                │
  │ abort     │              │ abort           │
  ▼           ▼              ▼                ▼
aborted    failed        aborted           failed
              │
              │ 人工重试
              ▼
           running
```

状态转换规则：
- `created → running`：Executor.Run() 被调用
- `created → aborted`：创建后未启动即取消
- `running → waiting_human`：当前 Stage 设置了 `require_human: true` 或 `on_failure: human` 触发
- `running → paused`：外部调用 Pause()
- `running → failed`：Stage 执行失败且已耗尽重试次数且 on_failure != human
- `running → done`：所有 Stage 执行完毕

注意区分 Stage failed 与 Pipeline failed：
- Stage failed：单个阶段执行失败，触发 Reactions 处理（重试/升级人工等）
- Pipeline failed：全局重试预算耗尽，或 on_failure=abort 且无 Reaction 匹配时，Pipeline 整体标记为 failed
- `running → failed` 仅在 Pipeline 级别失败时触发，单个 Stage 失败不直接导致此转换
- `paused → running`：外部调用 Resume()
- `paused → aborted`：暂停后决定取消
- `waiting_human → running`：收到人工 Action（approve/reject/modify/skip/rerun）
- `waiting_human → aborted`：收到 ActionAbort
- `failed → running`：人工决定重试（通过 action 端点发送 rerun）

非法转换一律拒绝并返回错误，状态转换在写入 Store 前用 `ValidateTransition(from, to)` 校验。

### 检查点（Checkpoint）

每个 Stage 完成后自动创建检查点：

```
Checkpoint {
    PipelineID  string
    StageName   string
    Status      "in_progress" | "success" | "failed" | "skipped" | "invalidated"
    Artifacts   map[string]string    // 该阶段的产物
    StartedAt   time.Time
    FinishedAt  time.Time
    AgentUsed   string
    TokensUsed  int
    RetryCount  int
}
```

作用：
- **崩溃恢复**：进程重启后从最后一个成功的检查点继续
- **回退**：人工 reject 时回退到指定检查点，丢弃之后的产物
- **审计**：完整记录每个阶段的执行详情

### 崩溃恢复流程

1. 进程启动时扫描 Store 中所有 `status = running | paused | waiting_human` 的 Pipeline
2. 对每条 Pipeline，重建内存结构（重新创建 humanCh、cancelFn 等）
3. 根据状态分别处理：
   - `waiting_human`：重新创建 channel，保持等待状态，推送 `human_required` 事件通知 Web/TUI/GitHub
   - `paused`：仅恢复内存结构，不启动执行，等待 Resume
   - `running`：找到最后一个 Checkpoint，根据其 Status 决定：
     - `in_progress`：该 Stage 执行中崩溃，需要清理脏状态后重新执行
       - 对 worktree 执行 `git checkout . && git clean -fd`（清理脏文件）
       - 将该 Checkpoint 标记为 `failed`
       - 从该 Stage 重新开始执行
       - Prompt 注入 "这是崩溃恢复后的重新执行，请从头开始"
     - `success`：从下一个 Stage 恢复执行
     - `failed`：根据 on_failure 策略决定是重试还是转为 waiting_human
4. Stage 开始前写入 `in_progress` Checkpoint，完成后更新为 `success` 或 `failed`

注意：humanCh 是 Go channel，进程退出后丢失。Pipeline 的等待状态通过 Store 中的 status 字段持久化，恢复时根据 status 重建 channel 即可。

## 四、人工介入

### 介入点类型

| 类型 | 触发条件 | 行为 |
|---|---|---|
| 阶段审批 | Stage.require_human = true | 阶段完成后暂停，等 approve |
| 失败升级 | Stage.on_failure = human | 阶段失败后暂停，等人工处理 |
| 手动暂停 | 用户主动 Pause | 中断当前 Agent（见下方 Pause/Resume 语义），暂停 |

### Pause/Resume 语义

- **Pause**：发送 SIGTERM（Unix）/ `GenerateConsoleCtrlEvent`（Windows）给 Agent 子进程 → 等待 30s 优雅退出 → 超时则 SIGKILL/TerminateProcess
- **Resume**：从当前 Stage 头重新执行（先清理脏状态：`git checkout . && git clean -fd`），Prompt 注入 "这是暂停恢复后的重新执行"
- Pause 期间 Pipeline status 为 `paused`，不占用调度信号量

### 人工可执行的操作

| 操作 | 效果 | 适用场景 |
|---|---|---|
| `approve` | 继续执行下一阶段 | 审批通过 |
| `reject(stage)` | 回退到指定阶段重新执行 | 实现不满意，需要重做 |
| `modify(message)` | 将反馈注入 artifacts 后重跑当前阶段 | 需求微调 |
| `skip` | 跳过当前阶段，继续 | 某个阶段不需要 |
| `rerun` | 重跑当前阶段 | 运气不好想再试一次 |
| `change_agent(agent)` | 换一个 Agent 重跑当前阶段 | Codex 效果不好换 Claude |
| `abort` | 终止整个 Pipeline | 不做了 |
| `pause` / `resume` | 暂停 / 恢复 | 需要时间思考 |

### Skip 硬依赖检查

skip 操作执行前需要检查硬依赖关系，以下组合不允许 skip：
- `worktree_setup` 不可 skip，如果后续有 `implement`（implement 需要 worktree）
- `implement` 不可 skip，如果后续有 `code_review`（review 需要代码变更）
- `merge` 不可 skip，如果后续有 `cleanup`（cleanup 需要合并完成）

违反硬依赖时返回错误提示用户，不执行 skip。

### Reject 回退规则

- 回退到目标 Stage 时，该 Stage 及之后所有 Stage 的 Checkpoint 标记为 `invalidated`
- 对应的 Artifacts 保留但 key 加前缀 `stale:` 标记（如 `stale:implement_summary`），可查历史但不参与后续执行
- 如果目标 Stage 早于 `worktree_setup`，需要清理已创建的 worktree 和分支
- 回退后重新执行时，人工反馈会注入到 prompt 中，让 AI 知道上次的问题

### 人工介入的输入通道

三个来源，统一进入同一个 channel：

```
Web API 请求    ─┐
TUI 键盘操作    ─┼──► Pipeline.humanCh ──► Executor.handleHumanAction()
GitHub 评论命令  ─┘
```

规则：
- 同一时刻只能有一个操作进入，后到的排队
- 操作需要验证权限（GitHub 场景下只有 Issue assignee 或 repo admin 可以操作）
- 每次操作都记录到审计日志

## 五、超时和重试

### 超时策略

- 每个 Stage 有独立的 timeout 配置
- 超时时通过 context.Cancel() 终止 Agent 子进程
- 超时视为一次失败，发送 `stage_failed` 事件到 EventBus → Reactions Engine 处理（不直接走 on_failure）
- 全局 Pipeline 也有一个总超时（默认 2 小时），防止卡死

### 重试策略

- 重试次数由 Stage.max_retries 控制
- 重试间隔采用指数退避：1s → 2s → 4s（上限 30s）
- 每次重试会将上次的错误信息注入到 prompt 中，让 AI 知道哪里出了问题
- 重试时使用相同的 Agent，除非人工通过 change_agent 更换

### Prompt 注入规则

重试和人工反馈都通过在原始 prompt 前追加上下文实现：

```
[如果是重试]
上次执行失败，错误信息：{error}
请避免同样的问题，重新执行以下任务：

[如果有人工反馈]
用户反馈：{human_feedback}
请根据以上反馈调整方案，重新执行以下任务：

[原始 prompt]
...
```

## 六、Reactions Engine

### 概述

借鉴 agent-orchestrator 的 Reactions 模式：不只是被动等失败，而是主动监听事件并自动响应。Reactions Engine 监听 Event Bus 上的所有事件，匹配预定义的规则，执行对应的动作。

### 事件 → 动作映射

```yaml
reactions:
  # Stage 级别的 Reactions
  stage_failed:
    - match: { stage: "*", error_type: "timeout" }
      action: retry
      max_retries: 2

    - match: { stage: "implement", error_type: "exit_nonzero" }
      action: inject_and_retry
      prompt: "上次执行失败了，错误信息：{{.Error}}。请修复后重试。"
      max_retries: 1

    - match: { stage: "*", error_type: "*" }
      action: escalate_human
      notify: [desktop, slack]

  # CI / 外部事件的 Reactions
  ci_failed:
    action: inject_to_stage
    target_stage: fixup
    prompt: "CI 失败，日志如下：{{.CILog}}。请修复。"
    auto_rerun: true

  review_comments:
    action: inject_to_stage
    target_stage: fixup
    prompt: "Reviewer 提了以下意见：{{.Comments}}。请逐条处理。"
    auto_rerun: true

  pr_approved:
    action: notify
    channels: [desktop]
    message: "PR #{{.PRNumber}} 已通过，可以合并。"

  # Pipeline 级别的 Reactions
  pipeline_stuck:
    match: { idle_minutes: 30 }
    action: notify
    channels: [desktop, slack]
    message: "Pipeline {{.PipelineID}} 已停滞 30 分钟，请检查。"
```

### 与 on_failure 的关系

Stage 配置中的 `on_failure` 是简化写法，Reactions Engine 是完整实现：

| on_failure 值 | 等价的 Reaction |
|---|---|
| `retry` | `action: retry, max_retries: N` |
| `human` | `action: escalate_human, notify: [desktop]` |
| `skip` | `action: skip_stage` |
| `abort` | `action: abort_pipeline` |

`on_failure` 是语法糖：Pipeline 创建时，Engine 将每个 Stage 的 `on_failure` 配置编译为等价的 Reaction 规则。运行时只有 Reactions Engine 处理失败，不存在两套机制。

编译规则：
- 用户显式配置的 Reactions 覆盖编译出的规则（同一 stage + error_type 匹配时）
- 执行流程：Stage 失败 → 发 `stage_failed` 事件到 EventBus → Reactions Engine 消费 → 匹配规则 → 执行动作
- 所有重试（包括 Reaction 触发的）消耗全局重试预算

### Reaction 动作类型

| 动作 | 效果 |
|---|---|
| `retry` | 直接重跑当前 Stage |
| `inject_and_retry` | 将错误/反馈注入 prompt 后重跑 |
| `inject_to_stage` | 将外部信息注入指定 Stage 后触发执行 |
| `escalate_human` | 转为 waiting_human + 发通知 |
| `notify` | 只发通知，不改变 Pipeline 状态 |
| `skip_stage` | 跳过当前 Stage 继续 |
| `abort_pipeline` | 终止 Pipeline |
| `spawn_agent` | 启动一个新的 Agent session 处理问题（用于 CI 修复等场景）— **P4 功能，P0~P2 不实现** |

### 执行规则

- Reactions 按配置顺序匹配，**第一个匹配的规则执行后停止**（不继续匹配后续规则）
- `match` 支持通配符 `*` 和字段匹配
- 每个 Reaction 执行后记录到 `human_actions` 表（source = "reaction"）
- Reaction 导致的重试也受 `max_retries` 限制，耗尽后 fallback 到 `escalate_human`
- Reactions 可在全局、项目、Pipeline 三个层级配置，下级覆盖上级
- `inject_to_stage` 对已完成（done）的 Pipeline：忽略并记录 warning 到日志，不重新激活 Pipeline

### 全局重试预算

Pipeline 级别新增 `max_total_retries` 配置（默认 5）：
- 每次重试（无论来源：Stage max_retries、Reaction retry、review-fix 循环）都消耗 1 次预算
- 预算耗尽时，任何重试请求自动转为 `escalate_human`
- 预算计数持久化到 Pipeline 记录中，崩溃恢复后继续累计
- 此预算独立于单个 Stage 的 max_retries（Stage 可能还有剩余重试次数，但全局预算耗尽则不再重试）

### Notifier 集成

Reactions 中涉及通知的动作通过 Notifier 插件发送：

```go
type Notifier interface {
    Plugin
    Notify(ctx context.Context, msg Notification) error
}

type Notification struct {
    Level      string  // "info" | "warning" | "critical"
    Title      string
    Body       string
    PipelineID string
    ProjectID  string
    PlanID     string  // 关联的 TaskPlan ID（如有），便于端到端追踪
    ActionURL  string  // 深链到 Dashboard 或 GitHub Issue
}
```

多个 Notifier 可以同时启用（desktop + slack），通知并行发送。

## 七、与 Secretary Layer 的集成

### Pipeline 的创建来源

Pipeline 可以从以下来源创建：

| 来源 | 说明 | 阶段 |
|------|------|------|
| 手动创建 | 用户通过 CLI/TUI/Web 直接创建单个 Pipeline | P0 ✅ |
| **DAG Scheduler 自动创建** | **Secretary Layer 审核通过后，为每个 TaskItem 自动创建 Pipeline** | **P2a ✅** |
| GitHub Issue 触发 | Webhook 监听 Issue 事件自动创建 | P3 🔧 |

当由 DAG Scheduler 创建时：
- Pipeline.Name = TaskItem.Title
- Pipeline.Description = TaskItem.Description
- Pipeline.TaskItemID = TaskItem.ID
- 模板由 TaskItem.Template 字段指定
- requirements 阶段通过 task_item_id 反查 TaskItem，并注入 Description/Inputs/Outputs/Acceptance
- Pipeline 完成/失败事件通过 Event Bus 通知 DAG Scheduler

### Event Bus 新增事件

以下事件由 Secretary Layer 引入。本节为这些事件的规范定义，[spec-secretary-layer.md](spec-secretary-layer.md) 引用此处。

| 事件类型 | 触发时机 | 数据 |
|---------|---------|------|
| `plan_created` | TaskPlan 创建 | plan_id, project_id |
| `plan_reviewing` | 进入审核 | plan_id, round |
| `review_agent_done` | 单个 Reviewer 完成 | plan_id, reviewer, verdict |
| `review_complete` | 审核流程完成 | plan_id, decision |
| `plan_approved` | TaskPlan 通过审核 | plan_id |
| `plan_waiting_human` | 等待人工确认或反馈 | plan_id, wait_reason |
| `task_ready` | TaskItem 变为 ready | plan_id, task_id |
| `task_running` | TaskItem 开始执行 | plan_id, task_id, pipeline_id |
| `task_done` | TaskItem 完成 | plan_id, task_id |
| `task_failed` | TaskItem 失败 | plan_id, task_id, error |
| `plan_done` | 所有 TaskItem 完成 | plan_id, stats |
| `plan_failed` | TaskPlan 失败 | plan_id, reason |
| `plan_partially_done` | 部分成功部分失败 | plan_id, stats |

这些事件和现有的 `stage_*`、`pipeline_*` 事件共存于同一个 Event Bus。

> 详细的 Secretary Layer 设计（Secretary Agent、Multi-Agent Review、DAG Scheduler、Workbench UI）见 [spec-secretary-layer.md](spec-secretary-layer.md)。

## 八、Stage 内并行执行（未来）

当前设计是串行执行所有 Stage。未来可通过 `depends_on` 字段实现 Stage 级 DAG 调度：

```yaml
stages:
  - name: unit_test
    depends_on: [implement]
  - name: lint
    depends_on: [implement]
  - name: code_review
    depends_on: [unit_test, lint]  # 等两个都完成
```

实现时 Executor 用 WaitGroup 或 errgroup 协调。但 P0 阶段不需要这个。

> 注意区分两层并行：
> - **任务级并行**（P2a）：多个 TaskItem 的 Pipeline 并行执行，由 DAG Scheduler 调度
> - **Stage 级并行**（未来）：单个 Pipeline 内的多个 Stage 并行执行

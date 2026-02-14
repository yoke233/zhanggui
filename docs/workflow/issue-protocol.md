# Issue 协作协议（协作总线）

## 目标

用 Issue（可承载在 GitHub/GitLab 或本地 SQLite）作为公共总线，让多角色协作可追溯、可路由、可并行、可验收，并且避免：

- 规格/结论在多个地方出现不同版本
- 讨论被误当成决定
- 并发开工导致互相踩踏、重复劳动、merge 地狱

本协议与项目画像文件 `docs/workflow/workflow-profile.md` 配套。

## 一、Issue 的粒度：什么时候用 Epic + 子 Issue

推荐规则（实用优先）：

- 小任务（单 repo、单角色、单 PR）：
  - 直接 1 个 Issue 即可
- 大任务（需要并发、跨 repo、或需要多角色推进）：
  - 1 个 Epic Issue（总线/决策/摘要）
  - 多个子 Issue（每个子 Issue 是一个可独立交付的 work item）

子 Issue 的约束（强烈建议）：

- 单 repo：明确写 `Repo:`（对应 `workflow.toml` 的 repos 之一）
- 单角色：明确写 `Role:`（backend/frontend/qa/…）
- 单责任：尽量只改一个 bounded area（避免多人改同一片）

补充（V1.1：PR 闭环）：

- 子 Issue/主 Issue 都应当有明确的 `IssueRef`，并要求相关 PR 在描述中引用该 `IssueRef`（便于追溯与自动化回填）。

## 二、Labels：最小集合与语义

Labels 的目的不是“分类好看”，而是作为路由与状态机，让 agent 能判断：

- 我该不该看
- 我能不能开始
- 我现在是 blocked 还是 doing

### 1) 路由标签（谁监听谁）

核心标签：`to:<role>`

建议最小集合：

- `to:architect`
- `to:backend`
- `to:frontend`
- `to:qa`
- `to:integrator`
- `to:recorder`

监听规则（默认）：

- 角色 X 监听所有带 `to:X` 的 open issues
- 如果 issue @mention 了某个角色/人：该角色也应当处理（@mention 视为路由指令）

重要说明：

- 路由的事实源是 Outbox backend 的 labels（`to:<role>`），不是正文里的自由文本。
  - GitHub/GitLab：labels
  - SQLite：`issue_labels`
- Issue 主帖模板里的 `Routing/ToRoles` 只是提示字段；创建 thread/issue 时必须同步设置对应 label（否则监听与路由不会生效）。

### 2) 状态标签（队列/开工判断）

建议最小集合：

- `state:todo`：可领取
- `state:doing`：已领取正在做
- `state:blocked`：阻塞等待
- `state:review`：待集成/待盖章/待验收（视项目而定）
- `state:done`：已完成（可与 issue close 并用，也可作为 close 前的显式状态）

注意：

- issue close 本身也是状态；但保留 `state:*` 能让队列更清晰

### 3) 决策标签（Accepted Gate）

- `decision:proposed`：存在待盖章的决策点
- `decision:accepted`：已通过（V1=any）
- `decision:rejected`：已拒绝

### 4) 控制标签（硬闸门）

- `needs-human`：必须人类介入；自动化与 agent 不应继续推进
- `autoflow:off`：关闭自动路由/自动迁移（仅人工推进）

## 三、Claim 机制：如何避免“大家都以为别人会做”

推荐“先 claim 再开工”：

- 任何执行动作（改代码、开 PR、跑集成）必须先 claim
- 未 claim 的 issue：允许讨论/澄清，但不允许推进实现

Claim 的两种实现方式（二选一或同时使用）：

- 使用 assignee（推荐，V1.1 真源）：claim = 把 issue 指派给某个 agent 身份或负责人
- 使用评论命令（可选）：claim = 评论 `/claim` 并由系统/流程把 issue assign + 状态从 `state:todo` 改为 `state:doing`

V1.1 约定（建议实现侧遵守）：

- Claim 的事实源应以 Outbox backend 的 `assignee` 字段为准（comment 文本不应被完全信任）
- `/claim` 可以保留为人工提示语，但不应仅凭文本判定领取成功

建议配套动作：

- claim 成功后补齐 `Repo:`、`Role:`、`ContractsRef:`（如果适用）

## 四、并发分组：frontend 3 / backend 4 怎么落地

并发控制分两层：

1. goclaw 运行时硬上限：
   - `agents.defaults.subagents.role_max_concurrent`
   - 这是“这个进程最多能同时跑多少个该角色 subagent”
2. 项目画像软上限：
   - `workflow.toml` 的 `groups.<name>.max_concurrent`
   - 这是“这个项目希望该角色同时最多开多少个”

建议实践：

- 硬上限可以略高（机器承受范围内）
- 软上限用于不同项目动态调度

## 五、依赖与等待：支持“等某个人进度”但不烧 token

依赖表达建议用正文固定字段（无需复杂自动化）：

- `DependsOn:` 里列出依赖 issue/PR
  - 例：`DependsOn: org/contracts#123, org/backend#45`

阻塞的处理方式：

- 发现依赖未满足时：
  - 将 issue 标为 `state:blocked`
  - 评论说明 `BlockedBy:`（指向依赖项）
  - 退出当前 subagent（不要轮询等待）

“什么时候可以 claim（领取）”判断（最小规则）：

- issue 为 open
- 有匹配自己的 `to:<role>` 标签或被 @mention/assigned
- 状态为 `state:todo`（或无状态标签且未被 claim/assigned）
- 依赖项（DependsOn）全部 closed 或已达成约定条件
- 不存在 `needs-human`

“什么时候可以开始工作（spawn Worker）”判断（最小规则）：

- issue 为 open
- 已 claim（assignee 已设置；如使用 `/claim`，必须最终落到 assignee 被设置）
- 依赖项（DependsOn）全部 closed 或已达成约定条件
- 不存在 `needs-human`

推荐（Soft）：

- 状态为 `state:doing`（缺失时不应阻塞 Phase 1；Lead/Integrator 可补齐用于队列与监听）

解除阻塞的语义（V1 建议统一）：

- 条件：依赖项满足（DependsOn 全部 resolved / closed）
- 机制：
  - 若启用 `auto_unblock_when_dependency_closed = true`：
    - assignee 仍存在：恢复到 `state:doing`（继续由当前负责人推进）
    - 无 assignee：恢复到 `state:todo`（回到队列等待领取）
  - 否则由 integrator/lead 通过 `/unblock` 手动执行恢复（同上规则）

## 六、固定 Comment 模板（你已确认愿意固定）

建议每条 agent 评论都使用固定结构，便于人读与机器解析。

Comment 模板以 `docs/workflow/templates/comment.md` 为基线，字段可扩展但不应删减关键字段（`Action`、`Status`、`IssueRef`、`Next`）。

模板真源：

- `docs/workflow/templates/comment.md`（mailbox skill 应当直接读取该文件并填充占位符）
- 不在本文再维护第二份“示例模板”，避免模板漂移导致不同角色输出结构不一致

建议：

- mailbox skill 必须负责生成/填充模板，避免人手写出不同版本
- `Action/Status` 是关键，用于去重、自动摘要、自动路由
- 推荐采用单写者：由 Lead/Integrator/Recorder 使用 mailbox skill 写回 Outbox 的结构化评论，Worker 仅回传证据与原始素材

## 七、固定 Issue 主帖模板

Issue 主帖以 `docs/workflow/templates/issue.md` 为基线。

Phase 1（人工跑通，Lean）最小要求（Hard）：

- `Goal`：至少说明“要得到什么结果”
- `Acceptance Criteria`：至少 1 条可观察条件
 - `Routing`：至少设置 1 个路由标签（Outbox label：`to:<role>`）
- `Repo/Role`：至少能推断出执行所在 repo（`Repo:` 或 `Role:` + `role_repo` 映射）
- 如涉及接口契约：必须写 `ContractsRef`（可以是 `none`，但必须明确）

推荐补齐（Soft）：

- `SpecRef`：如有外部规格/设计文档，给出链接或路径
- `Dependencies`：`DependsOn/BlockedBy`（没有就写 `none`）
- `Start Conditions`：尽量按模板填写；其中 `state:*` 用于队列清晰，Phase 1 不应作为硬闸门

Recorder/Integrator 的处理建议：

- 缺少 Hard 字段：打 `needs-human` 并要求补齐（否则容易造成理解分叉）
- 缺少 Soft 字段：评论提醒即可，不建议阻塞推进

# Lead/Worker 模型（每个角色一个 Lead，多 Worker 并发）

## 为什么需要 Lead（而不是只靠一次性 subagent）

当一个项目里同时跑多个 subagent 时，最大的体验问题通常不是“并发改代码”，而是：

- subagent 天生短命：做完即退出，很难维护持续上下文
- 没有持久记忆：跨天/跨 session 复用不到“已确认的决定、接口约定、坑位经验”
- 没有稳定 inbox：谁在等谁、谁 blocked、什么时候可以开工，很难靠临时对话维持一致

因此推荐把系统拆成两类 actor：

- Lead：常驻、持久、带 inbox + 记忆 + 光标（cursor）的“角色负责人”
- Worker：短命、纯执行、可以并发跑的“任务执行单元”

这与“主 agent 作为助理统筹”的目标不冲突：主 agent 仍然轻量，只负责启动/停止 Lead、做跨角色协调与用户交互。

## 核心概念（V1）

- Project：一个交付单元。可能是单 repo，也可能是 frontend/backend/contracts 多 repo。
- Role：项目启用的一组角色（backend/frontend/qa/integrator/architect/recorder…），由 `<outbox_repo>/workflow.toml` 决定。
- Outbox：公共总线，V1 以 Issue 作为协作真源；backend 可选 GitHub/GitLab Issues 或本地 SQLite（由 `workflow.toml` 的 `[outbox]` 段指定）。
- Mailbox：向 Outbox 投递/回复的固定模板（见 `docs/workflow/templates/*`）。
- Accepted Gate：盖章机制（见 `docs/workflow/approval-policy.md`）。

## 运行时拓扑（推荐）

推荐拓扑是“一个项目 N 个常驻 Lead，每个 Lead 再拉起多个 Worker”：

```text
Issue (GitHub/GitLab/SQLite)
  -> (issue / comment / label events)
     -> Lead(backend)  -> spawn Worker x N -> PR/CI -> 回填到 Issue
     -> Lead(frontend) -> spawn Worker x M -> PR/CI -> 回填到 Issue
     -> Lead(qa)       -> spawn Worker x K -> 证据/回归 -> 回填到 Issue
     -> Lead(integrator) 负责收敛与最终验收
     -> Lead(architect) 负责 contracts/决策盖章
```

注意：Lead 不等于“必须强技术领导”。它更像“该角色的项目经理 + 单写者”，负责让协作可回放、可接管、可验收。

## Lead 的职责边界（必须清晰）

Lead 负责“统筹与写回”，不负责“把所有实现自己写完”。

Lead 做：

- 订阅 Outbox：监听 `to:<role>`、@mention、assignee、`state:*`、`decision:*` 等（规则见 `docs/workflow/issue-protocol.md`）。
- 维护光标：记录自己已处理到的 issue/comment id，保证可回放、可去重、可恢复。
- 领取与派工：
  - 先 claim（assignee 已设置）再开工（如使用 `/claim`，必须最终落到“assignee 被设置”）
  - 把大 issue 拆成可并发的子 issue（或子任务）
  - 根据 `workflow.toml` 的 `role_repo` 把 Worker 派到正确 repo/workdir
- 阻塞管理：
  - 发现依赖未满足，置 `state:blocked` 并写 `BlockedBy/DependsOn`，然后停止 Worker（不轮询烧 token）
  - 依赖满足后再恢复（由事件触发，而不是忙等）
- 证据收敛：
  - 收 Worker 的 PR/commit/CI 证据
  - 统一按 mailbox 模板回填到 Issue（让线程可审计）
- 记忆治理：
  - 把“已 Accepted 的稳定结论”落盘到项目记忆（见 `docs/workflow/memory-layout.md`）
  - 不把“讨论”当成“记忆真相”

Lead 不做：

- 不在同一个 Issue 下与其他 Lead ping-pong 互相触发死循环
- 不把 Worker 的草稿/猜测写进共享记忆（除非已盖章 Accepted）
- 不承担跨 repo 的最终收敛（那是 integrator 的职责）

## Worker 的职责边界（强约束）

Worker 是短命执行单元，目标是“把事情做成并给出证据”，而不是“维持项目状态”。

Worker 做：

- 在单一 repo 范围内实现、写测试、跑命令、修 CI
- 输出证据：PR 链接、测试命令与结果、关键 diff 说明
- 将“原始素材”回传给本 role 的 Lead（不要求严格模板，但必须可追溯）：
  - IssueRef（对应哪个 Issue）
  - Changes（PR URL 或 commit URL，至少一个）
  - Tests（跑了什么、结果如何；或明确 `n/a`）
  - BlockedBy/疑问（如有）

Worker 不做：

- 不修改共享记忆文件夹（共享记忆由 Lead/Recorder 单写者维护）
- 不自行决定接口破坏性变更（必须走 contracts + Accepted Gate）
- 不轮询等待别人的进度（发现依赖就退出，并写清楚阻塞点）
- 不直接向 Outbox 写入“结构化事实”（固定模板 comment/labels/state 迁移）
  - 推荐做法：只有 Lead/Integrator 拥有 Outbox 写权限/凭证（例如仅 Lead 配置 `gh auth`），Worker 只做本地执行与回传证据

## “常驻”怎么实现：Lead 是独立进程还是携带 role 的 subagent？

两种实现都可行，但推荐独立进程（或至少独立 runtime 实例）。

### 方案 A（推荐）：每个角色一个常驻 Lead 进程

特点：

- Lead 有独立 workspace 与记忆目录（按角色隔离）
- 崩溃隔离：backend Lead 挂了不影响 frontend Lead
- 易于做事件驱动：每个 Lead 维护自己的 event loop 与 cursor

代价：

- 进程数增多，需要一个轻量 Supervisor 负责拉起/重启/限流

### 方案 B（备选）：主进程里维持多个“长会话 subagent”

特点：

- 部署简单：一个进程里管理多角色 session
- 容易共享某些缓存

风险：

- 容易把“统筹”和“执行”耦合在一起，主 agent 变重
- 一旦主进程退出，所有 Lead 会话同时消失

结论：如果你明确想要“一个角色一个 Lead + 多 Worker”，并且 Lead 需要长期记忆与 inbox，方案 A 更匹配。

## 事件驱动与去重（避免忙等与刷屏）

原则：Lead 永远不应靠“不断 list issue”来等待别人完成，而应由事件唤醒。

建议实现：

- GitHub/GitLab backend：优先 webhook/App 把 issue/comment/label/close 事件投递到队列
- SQLite backend：退化为定时轮询 DB（或 file/watch + cursor），只处理新增 event id
- 无论哪种 backend：必须带 cursor（只拉增量），并设置最小间隔与速率限制

去重机制建议（写在 comment 模板里，便于回放）：

- `ReadUpTo: <comment_id>`：声明“我读到哪”
- `Trigger: <stable-id>`：同一个 trigger 只能处理一次（幂等）

## 并发控制（组并发 + 角色硬上限）

并发同样分两层：

- 系统硬上限：运行时最多允许某角色并发多少 Worker（例如 `role_max_concurrent`）
- 项目软上限：`workflow.toml` 的 `groups.*.max_concurrent`（backend 4、frontend 3…）

建议：Lead 以 `min(软上限, 硬上限)` 作为实际可开工的 Worker 并发数。

## 与 proto/contracts 的关系（你更偏好 proto）

建议把 mailbox 的“结构化字段”也视作一种 contract：

- 在 contracts repo 里增加 `mailbox.proto`（例如 `MailboxEnvelope`、`IssueState`）
- Issue/Comment 模板是该 proto 的可读渲染（人可读、机可解析）
- 将来需要从 Issue 迁移到 HTTP/Queue，只需复用同一份 proto

V1 不强制实现 proto，但建议把字段设计保持稳定，避免后期迁移成本暴涨。


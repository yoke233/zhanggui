# Outbox（发件箱）与 mailbox skill（固定模板沟通）

## 背景

多 subagent/多 repo 并行时，最大的风险不是“写错代码”，而是：

- 规格/接口在不同地方被重复描述
- 讨论结论只存在于聊天里
- 角色之间靠“互相 ping”同步，导致多个版本并存

因此沟通建议从“互聊”切换为“写入一个可订阅的 outbox”。

## Outbox 的推荐载体：Issue（GitHub/GitLab）或本地 SQLite

Issue 具备天然的队列能力（无论承载在 GitHub/GitLab，还是本地 DB）：

- 可持久化、可追踪
- 支持 label（topic）、assign（领取）、close（完成）
- 可关联 PR、commit、CI 结果

V1 推荐两种 backend：

- 团队协作：GitHub/GitLab Issues（共享、可审计、权限体系成熟）
- 本地/离线启动：SQLite outbox（单机、最小依赖，见 `docs/operating-model/outbox-backends.md`）

本仓库已有内置 `github` skill（依赖 `gh` CLI），适合在 GitHub backend 下让 agent 自动创建/查询 issue。

V1 约定（工作流当前决策）：

- Outbox backend 由项目画像 `<outbox_repo>/.agents/workflow.toml` 的 `[outbox]` 段决定：
  - GitHub：`outbox.backend = "github"` + `outbox.repo = "org/repo"`
  - SQLite：`outbox.backend = "sqlite"` + `outbox.path = ".agents/state/outbox.sqlite"`
- 如果项目存在独立 `contracts` repo，推荐将 Outbox 放在 contracts（便于把“接口与决策”集中）。
- 如果项目是后端-only 单 repo，则 Outbox 可直接放在该 repo。
- Issue 是协作真源（讨论、阻塞、证据、结论都在同一线程内可回放）。
- 不使用 goclaw `task_id` 作为协作主线，避免双真源状态漂移（后续阶段可再引入 task 镜像）。

备选/降级：

- 如果暂时无法接入 GitHub/GitLab：优先使用 SQLite outbox（仍保持同一套 Thread/Event/labels/assignee/close 语义）
- 如果连本地 DB 都不可用：才使用 `message` 工具把内容发送到一个固定频道/聊天作为“弱 outbox”（但这不再是可回放真源）

## 安全与权限（最小约束）

- Outbox（Issue/Comment）是持久化记录：默认按“全团队可见、可长期回放”对待，不要粘贴密钥、token、个人隐私数据或内部敏感链接。
- 涉及敏感信息的项目，Outbox repo 应使用 private 仓库，并限制可见范围与写权限。
- 若使用 machine user/bot 账号承载角色身份（assignee/approver），建议最小权限原则：
  - 只授予需要的 repo 访问与 assign/review 权限
  - 避免让 bot 拥有全组织管理员权限

## mailbox skill 的定位

mailbox skill 的目标是同时提供“统一投递点 + 固定输出结构”：

- 讨论正文可以自由文本
- 但 Issue/Comment 的协议字段必须按固定模板输出，便于路由、回放和审计

建议暴露的“方法”（概念层，不强制具体语法）：

- `mailbox.list`：按 label/状态筛选待处理消息
- `mailbox.claim`：领取（assign）某条消息
- `mailbox.post`：发布问题/阻塞/变更请求到 outbox（推荐：Lead/Integrator/Recorder 调用）
- `mailbox.reply`：在 issue 下回复结论/追问（推荐：Lead/Integrator/Recorder 调用）
- `mailbox.draft`：生成一段符合模板的草稿文本但不投递（推荐：Worker 调用后回传给 Lead 统一投递）

建议（强烈推荐）：

- 将 `@mention` 视为路由指令，而不是聊天功能。
- mailbox skill 统一读取固定模板并填充字段，避免不同角色写出不同结构。
- 如需机器去重/回放，可额外附加元信息字段（例如 `ReadUpTo`、`Trigger`）。

## 写入权限建议（V1 推荐：结构化写回由 Lead 单写者负责）

为了保证 Issue 中的“结构化事实”稳定一致，推荐采用单写者路径：

- Worker 的输出允许不规范：更像“原始素材”（日志片段、命令结果、PR 链接）。
- Lead/Integrator/Recorder 负责把素材转换为固定模板 comment，并写回 Outbox（mailbox.reply/post）。

这样可以显著降低“不同 worker 写出不同模板版本”的漂移风险。

务实的权限隔离建议：

- 只有 Lead/Integrator 配置 Outbox 写入凭证（例如 `gh auth` token）。
- Worker 运行时不配置 Outbox 写入凭证，只能在本地执行并回传证据。

## labels（V1 固定基线）

V1 采用固定 label 基线，便于所有角色与自动化共享同一套路由语义：

- `to:architect` / `to:frontend` / `to:backend` / `to:qa` / `to:integrator` / `to:recorder`
- `state:todo` / `state:doing` / `state:blocked` / `state:review` / `state:done`
- `decision:proposed` / `decision:accepted` / `decision:rejected`
- `kind:task` / `kind:bug` / `kind:question` / `kind:proposal` / `kind:blocker`
- `prio:p0` / `prio:p1` / `prio:p2` / `prio:p3`
- `needs-human`（必须人类介入，自动化停止）
- `autoflow:off`（关闭自动路由/自动迁移，仅人工推进）

可选补充（建议但不强制）：

- `contract:breaking`：接口破坏性变更风险（通常由 architect/integrator 使用）

完整集合与监听规则以 `docs/workflow/label-catalog.md` 为准。

## 一致性底线（必须）

- Outbox 里的“决定”必须最终落盘到真源：
  - 接口相关：落盘到 contracts repo（proto/ADR/PR）
  - 代码相关：落盘到对应 repo 的 PR/commit
- Outbox 只做“协调与广播”，不作为接口规格的最终载体。

## 决策盖章（Accepted Gate）

Issue 里的“讨论”不等于“决定”。建议引入可配置的盖章人集合与最小审批策略：

- V1：`any`（任一盖章人通过即视为 Accepted）
- 盖章动作建议通过 issue 评论命令触发（例如 `/accept`），并由系统写入 label（例如 `decision:accepted`）

审批策略详见：`docs/workflow/approval-policy.md`。

## Labels / 协作协议

labels、监听规则、claim/blocked/依赖/开工条件与固定评论模板见：

- `docs/workflow/issue-protocol.md`
- `docs/workflow/label-catalog.md`
- `docs/workflow/templates/issue.md`
- `docs/workflow/templates/comment.md`

# Workflow Docs

本目录用于记录项目在「多角色 + subagent + 多仓库」场景下的工作流约定，目标是：

- subagent 各自拥有独立工作区并可直接改代码
- 主 agent 更像助理，负责统筹流程而不是事无巨细地管控
- 避免「规格/文档不一致」导致产出不一致
- 支持多 repo（frontend/backend/contracts）并行开发
- 角色沟通采用固定 Issue/Comment 模板，通过 outbox（消息队列）异步广播与领取

V1/V1.1 本项目的关键选择（先记住这几条，其它都是细节）：

- 协作真源：Issue（默认承载在 GitHub/GitLab，亦可本地 SQLite）
- 配置真源：只保留 Outbox repo 内的 `<outbox_repo>/workflow.toml`（其它 repo 不再放第二份）
- Claim 真源：Outbox 的 `assignee` 字段（GitHub assignee / SQLite issues.assignee；`/claim` 只是触发手段，文本不算事实）
- 状态标签：`state:*` 推荐使用以保持队列清晰，但 Phase 1 不应作为硬闸门；Phase 2 起由 lead 尽量自动补齐/规范化

文档列表：

- `docs/operating-model/README.md`：三层操作模型（Product Truth / Delivery Control / Quality Gate，含分阶段计划）
- `docs/operating-model/START-HERE.md`：Phase 1 本地启动清单（git + sqlite，最小可开工）
- `docs/standards/README.md`：跨文档稳定规范入口（命名、审批、标签、文档生命周期）
- `docs/features/README.md`：feature 级定稿文档入口（requirement/prd/tech-spec）
- `docs/workflow/roles-and-flow.md`：角色边界与推荐流程（主 agent/架构师/实现/测试/集成）
- `docs/workflow/integrator-role.md`：Integrator 角色说明（收敛、合并、集成验收）
- `docs/workflow/lead-worker.md`：Lead/Worker 运行模型（每个角色一个常驻 Lead，多 Worker 并发）
- `docs/workflow/v1.1.md`：V1.1 补充约定（多环境布局、Claim/Assignee、PR（forge）闭环、Outbox 抽象）
- `docs/operating-model/executor-protocol.md`：Worker 可插拔执行协议（上下文注入、WorkOrder/WorkResult、切换 worker 幂等语义）
- `docs/workflow/multi-repo-contracts.md`：多 repo + contracts(proto) 的组织方式与对齐规则
- `docs/workflow/outbox-and-mailbox-skill.md`：用 skill 将沟通写入 outbox（Outbox backend + 固定模板：GitHub Issues 或 SQLite）
- `docs/workflow/approval-policy.md`：Issue 的“盖章/Accepted”审批策略（V1: any）
- `docs/workflow/workflow-profile.md`：项目级 `workflow.toml` 规范（动态 repo/角色/并发/监听）
- `docs/workflow/issue-protocol.md`：Issue 协作协议（labels、监听、claim、blocked、依赖、开工条件、评论模板）
- `docs/workflow/label-catalog.md`：固定 label 集合、监听矩阵、状态迁移规则
- `docs/workflow/memory-layout.md`：项目级记忆目录布局（shared + roles 子目录，不额外存 L3）
- `docs/workflow/templates/workflow.toml.example`：`workflow.toml` 模板
- `docs/workflow/templates/issue.md`：Issue 主帖模板
- `docs/workflow/templates/comment.md`：Comment 模板
- `docs/workflow/templates/pr.md`：PR 描述模板（可选但推荐）
- `docs/workflow/spec-consistency.md`：如何把「规格一致性」做成机制（含已发现的漂移点）
- `docs/workflow/guardrails.md`：常见问题与护栏（并行审查、多 worker、幂等与回填）

说明：

- `docs/subagent.md` 是 subagent 功能的用户文档；本目录更偏「协作流程与约定」。
- 这里的约定会引用仓库已有能力：`sessions_spawn`（支持 `repo_dir`/`task_id`）、task 追踪、内置 `github` skill 等。


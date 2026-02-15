# Operating Model (3-Layer Truth)

本目录定义一个面向“多角色 + 多智能体 + 多仓库”的协作操作模型（Operating Model）。

它比 `docs/workflow/` 更上层：`docs/workflow/` 解决的是 **交付层（Delivery Control）** 的协议与模板；而本目录把整个系统拆成三层真源与责任边界，目的是让你可以：

- 先跑通人工闭环，再逐步自动化
- 将来替换实现框架（goclaw / agentsdk / workflowd / GitHub -> GitLab/MySQL）时，不推翻协作协议
- 把“谁对什么负责”说清楚，避免 PM/Lead/Reviewer 指挥链混乱

## 三层模型（未来长期结构）

1. 需求层（Product Truth）
- 把用户/业务输入变成可执行的 Spec
- 真源：Issue 的 Spec 区块（或链接到 repo 内 `spec.md`）

2. 交付层（Delivery Control）
- 管队列、节奏、依赖、证据、合并/发布收敛
- 真源：Issue（协作总线对象）+ PR/commit（代码真源）+ `<outbox_repo>/workflow.toml`（配置真源）

3. 质量层（Quality Gate）
- Reviewer/QA 只做判定，不派工
- 真源：PR review/CI/test 证据（可计算），或在无 forge 模式下由 Outbox 的结构化质量判定承接（见 `docs/operating-model/quality-gate.md`）

## 与现有文档的关系

- 需求层/质量层的原则与产物：本目录
- 交付层的协议与模板（Outbox、labels、claim、Lead/Worker、mailbox）：`docs/workflow/`
- 跨文档稳定规范（命名、审批、标签、文档生命周期）：`docs/standards/`
- 功能级定稿文档（requirement/prd/tech-spec）：`docs/features/`

推荐阅读顺序：

1. `docs/operating-model/START-HERE.md`（Phase 1 本地启动清单：git + sqlite）
2. `docs/operating-model/product-truth.md`
3. `docs/operating-model/meeting-mode.md`（会议模式：并行输入 + 快速收敛）
4. `docs/operating-model/delivery-control.md`
5. `docs/operating-model/quality-gate.md`
6. `docs/operating-model/outbox-backends.md`（Outbox 承载系统抽象：GitHub/GitLab/MySQL/SQLite）
7. `docs/operating-model/executor-protocol.md`（Worker 可插拔、上下文注入、切换 worker 的幂等语义）
8. `docs/operating-model/local-first.md`（只用 git + sqlite 启动项目）
9. `docs/operating-model/phases.md`
10. 然后再进入 `docs/workflow/README.md`

## 术语（本目录统一用法）

- Issue：协作总线对象。V1 的默认承载由 `<outbox_repo>/workflow.toml` 的 `[outbox]` 段决定：可以是 GitHub/GitLab Issue，也可以是本地 SQLite issue（见 `docs/operating-model/outbox-backends.md`）。
- Spec：需求层的可执行规格，必须包含验收标准与边界。
- Evidence：交付/质量层的可审计证据（PR、commit、CI、测试命令输出等）。


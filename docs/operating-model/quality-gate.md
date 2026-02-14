# Quality Gate (质量层闸门)

目标：让“是否可合并/是否可发布”的判定可计算、可审计、可回放。

本层强调：

- Reviewer 不派活，只判定
- 判定必须有证据，并且可被系统读取（GitHub PR review/CI 状态，或 Outbox 结构化字段）

## 角色与职责边界

### Reviewer（Code Reviewer）

专职做代码审查与风险识别：

- 安全：权限、注入、敏感信息、依赖风险
- 性能：明显的 O(N^2)、资源泄漏、热点路径
- 可维护性：结构、边界、命名、复杂度
- 一致性：是否符合约定（contracts 版本引用、迁移脚本、回滚策略）

Reviewer 的输出是“判定”：

- `review:approved`
- `review:changes_requested`

注意：Reviewer 不分配任务，不写“你去做 X”，而是指出问题与风险点。由 Lead 决定怎么拆/派工修复。

### QA / Test Reviewer（可选）

专职审测试与验收覆盖：

- 回归覆盖是否足够
- 契约测试/集成测试是否存在
- 数据迁移脚本是否可回滚
- 关键路径是否有可观测性（日志、指标、报警）

输出同样是“判定 + 证据”。

## 真源（可计算）

V1 的推荐真源是代码托管平台自带的计算结果：

- PR review 事件（Approved / Changes requested）
- CI checks 状态（pass/fail）
- 必要时补充：合并策略、分支保护规则

Issue 的作用是“协作与归档”，不替代 PR review 本身。

## 无 forge / 本地模式（只有 git + sqlite）

如果你一开始不接入 GitHub/GitLab（没有 PR review 事件、没有 CI checks），仍然可以跑 Phase 1：

- Reviewer 的判定需要落到 Issue 的结构化事件里（可计算、可审计）。
- 推荐由 Lead 单写者把 Reviewer 的输入规范化写回 Outbox（避免格式漂移）。

最小可行（Phase 1）：

- Reviewer 在 Outbox comment 的 `Summary` 里写：
  - `review:approved` 或 `review:changes_requested`
- Lead 将其规范化写回（Phase 2 可自动化为结构化字段）

注意：

- 本地模式的 `actor` 身份通常只能做到“约定俗成”的信任（例如 `whoami`），不具备平台级强认证。
- 当进入多人协作与合规要求阶段，建议迁移到 GitHub/GitLab 或服务端 DB。

## 与交付层的接口

建议的闭环方式：

1. Reviewer 在 PR 上给出 review（GitHub 记录为事实）
2. Lead/Integrator 将“review 判定 + 证据链接”规范化写回 Issue
3. Integrator 满足合并条件后 merge，并把 Evidence 回填 Outbox

这样可以在 Phase 1 不做复杂自动化的情况下，仍然保证判定可审计。

## 未来增强（Phase 3 方向）

当需要更强的可计算路由时，可以增加一种“结构化质量事件”：

- 在 Outbox comment 里增加字段（例如 `Review.Result`、`Review.Evidence`）
- 或者用 proto 定义 `QualityVerdict`，模板只是可读渲染

但这不应成为 Phase 1 的开工门槛。

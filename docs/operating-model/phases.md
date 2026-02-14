# Phases (分阶段落地计划)

目标：按阶段落地，不在 Phase 1 就把 Phase 3 的自动化复杂度背上。

## Phase 0: 文档与真源收敛

完成条件：

- 需求层：Issue Spec 区块（或 `SpecRef`）可执行（至少 Acceptance Criteria / Out of Scope / Risks）
- 交付层：Issue + 交付物关联规则明确（`IssueRef` 必填，Changes 可为 PR 或 commit）
- 质量层：review/测试 的证据来源明确（forge 模式下以 PR review/CI 为真源；本地模式下以 Outbox 的结构化质量判定为真源）

## Phase 1: 人工跑通闭环 (Lean)

目标：真正开始交付，不写 lead mode 自动化。

Hard（必须做到）：

- 需求层：Spec 真源在 Issue（或 spec.md 并链接）
- 交付层：
  - assignee 是 claim 真源
  - PR/commit 是交付真源（PR 描述含 `IssueRef`；本地无 PR 时用 commit hash 作为 Changes）
  - Evidence 回填 + close（建议由 Lead/Integrator 统一按模板写回）
- 质量层：
  - forge 模式：PR review 给出 approved/changes_requested + CI/test 至少一项证据
  - 本地/离线模式：Reviewer 的判定必须写入 Outbox（可计算），并且至少一项测试证据（本地命令输出摘要也可）

Soft（推荐但不阻塞开工）：

- `state:*` 标签尽量补齐（队列更清晰）
- 依赖用 `DependsOn/BlockedBy` 写清楚

## Phase 2: 最小 Lead 自动化 (polling + cursor)

目标：让每个角色的 Lead 常驻，自动做“订阅 -> 规范化 -> spawn worker -> 写回证据”的闭环。

原则：

- 不强依赖 `state:*` 作为事实源（缺失时 lead normalize）
- 仍以 assignee / labels / depends-on 为事实源
- 结构化写回仍由 Lead 单写者负责，worker 只产证据
- 必须支持切换 worker：用 `run_id`（active run）保证幂等，避免旧结果覆盖新结果（见 `docs/operating-model/executor-protocol.md`）

## Phase 3: 质量/PR/CI 自动化增强 (不改协议)

目标：把“可计算的质量信号”自动回填到 Outbox，并自动路由回责任角色。

增强项示例：

- 自动读取 PR review 结论、CI checks 结果，写入 Outbox comment
- 自动把 changes_requested/CI fail 路由回对应 role（Next + labels）
- 自动生成 release note / changelog（可选）

## 与现有交付协议的对应

交付层（Delivery Control）更细的协议与模板在：

- `docs/workflow/v1.1.md`
- `docs/workflow/issue-protocol.md`
- `docs/workflow/templates/*`

本地/离线启动路径（只用 git + sqlite）见：

- `docs/operating-model/local-first.md`

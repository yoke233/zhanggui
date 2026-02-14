# Phase 1 验收测试规格（ATDD）

版本：v1.0  
状态：Draft  
负责人：PM / Lead / QA  
目标：用可执行验收场景保证 Phase 1 闭环真实可跑

## 1. 范围与原则

范围：

- 仅覆盖 Phase 1（人工闭环），不包含 Phase 2/3 自动化能力。

原则：

- 先写失败场景（Red），再写最小实现（Green），最后重构（Refactor）。
- 验收以 Issue 时间线证据为准，不以口头描述为准。

## 2. 前置条件（测试环境）

- 已存在 `<outbox_repo>/.agents/workflow.toml`。
- 已存在 Issue/Comment 模板。
- 有可用的 outbox backend（GitHub/GitLab 或本地 sqlite）。
- 角色至少包含：Lead、Worker、Reviewer、Integrator。

## 3. 术语

- `IssueRef`：协作主键。
- `run_id`：一次执行尝试主键。
- `Evidence`：PR/commit/tests/review 的可审计证据。

## 4. 验收场景（ATDD）

### [P1-AT-001] 未 claim 禁止开工

Given:

- Issue 为 open
- 没有 assignee

When:

- 尝试触发执行（spawn worker）

Then:

- 执行被拒绝
- Issue 中记录一条可审计说明（例如 `Action: blocked`，`result_code: manual_intervention`）

### [P1-AT-002] 有 `needs-human` 禁止自动推进

Given:

- Issue 带 `needs-human`
- 其它开工条件满足

When:

- 触发自动或半自动推进

Then:

- 不进入执行态
- 仅允许人工接管

### [P1-AT-003] 依赖未满足必须 blocked

Given:

- Issue 已 claim
- `DependsOn` 存在未完成项

When:

- 尝试开工

Then:

- 状态进入 blocked 语义
- 输出 `BlockedBy` 与依赖引用

### [P1-AT-004] Worker 结果缺少 Changes 不得关闭

Given:

- Worker 回传结果中无 PR/commit

When:

- Lead 尝试推进到 close

Then:

- 关闭被拒绝
- 回填 comment 标记缺失项（Changes）

### [P1-AT-005] Worker 结果缺少 Tests 不得关闭

Given:

- Worker 回传结果中无 Tests 字段或无结果

When:

- Lead 尝试推进到 close

Then:

- 关闭被拒绝
- 回填 comment 标记缺失项（Tests）

### [P1-AT-006] 允许自然语言结果，但必须可追溯

Given:

- Worker 仅返回自然语言文本

When:

- Lead 做规范化写回

Then:

- 写回 comment 必须包含 `IssueRef`、`Changes`、`Tests`、`Next`
- 缺失关键锚点时不得推进完成

### [P1-AT-007] 成功闭环

Given:

- Issue 已 claim
- 依赖满足
- 有 Changes 与 Tests Evidence
- Reviewer 给出判定（forge 或本地结构化回填）

When:

- Integrator 执行收敛关闭

Then:

- Issue 关闭成功
- 时间线可回放：创建、claim、执行、证据、判定、关闭

### [P1-AT-008] 状态标签缺失不阻塞 Phase 1

Given:

- 无 `state:*` 标签
- Hard 条件均满足

When:

- 触发执行

Then:

- 允许开工
- 可选补齐 `state:*` 作为软约束

## 5. 验收通过标准

- 以上 8 个场景全部通过。
- 至少 1 个真实任务完成端到端闭环。
- 关键证据在 Issue 中可追溯且可复盘。

## 6. 产出物

- 测试记录（通过/失败、时间、责任人）。
- 样例 Issue 链接或本地 `IssueRef` 列表。
- 回填 comment 样本（结构化字段完整）。

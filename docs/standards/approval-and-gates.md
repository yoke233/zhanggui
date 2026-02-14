# 审批与闸门规范

版本：v1.0  
状态：Draft  
Owner：PM / Lead / Reviewer

## 1. 目标

- 统一“谁有权盖章、盖章在哪生效、失败后如何回退”。
- 让本地模式（git + sqlite）与 forge 模式（GitHub/GitLab）使用同一套判定语义。
- 保证审批结论可计算、可审计、可回放。

## 2. 配置真源

- 唯一配置位置：`<outbox_repo>/.agents/workflow.toml`
- V1 仅支持 TOML 配置；不读取 JSON/YAML。
- 审批配置统一放在 `[approval]` 段。

示例：

```toml
[approval]
enabled = true
approvers = ["lead-integrator", "lead-backend"]
mode = "any"        # any | all | quorum | sequential
quorum = 1          # mode=quorum 时生效
```

## 3. 角色职责

- `Approver`：有权给出通过/驳回结论。
- `Lead`：组织任务、汇总证据、发起审批，不绕过审批。
- `Worker`：执行任务并提交证据，不直接盖章。
- `Reviewer/QA`：给出质量判定证据，供 Approver 决策。

说明：

- 可以配置多个 `approvers`。
- 若配置了 `approvers`，则必须由其给出最终结论。
- V1 默认推荐 `mode=any`（你当前选择）。

## 4. 闸门点（Gates）

最小必需闸门：

1. `gate:spec-accepted`：需求/规格可以进入交付。
2. `gate:merge-accepted`：代码可合并。
3. `gate:release-accepted`：可发布（可选，按项目启用）。

每个闸门都要求：

- 有可追踪证据（Issue 评论、PR review、测试结果）。
- 有结构化结论（label 或结构化评论字段）。

## 5. 结论语义

统一结论集合：

- `approved`
- `changes_requested`
- `rejected`

推荐标签：

- `review:approved`
- `review:changes_requested`
- `decision:accepted`
- `decision:rejected`

## 6. 失败回路（打回）

当结论为 `changes_requested` 或 `rejected`：

- 任务状态回到 `state:doing` 或 `state:todo`（由 Lead 选择）。
- 在同一 `IssueRef` 下新增一轮执行（新 `run_id`）。
- 必须记录“打回原因 + 修复要求 + 复验条件”。

## 7. 本地模式最小执行

在无 GitHub/GitLab 的本地模式下：

- 审批证据写入 SQLite outbox 的 issue/comment。
- `Approver` 仍按同样字段给出结论。
- CI 不存在时，测试命令输出可作为临时证据，但必须可复现。

## 8. 审查检查项

- 是否只从 `workflow.toml` 读取审批配置。
- 是否遵守当前 `mode` 的通过条件。
- 是否把审批证据写回 `IssueRef` 对应线程。
- 是否在驳回后生成新的执行轮次（新 `run_id`）。

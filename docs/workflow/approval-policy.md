# Issue 盖章策略（Accepted Gate）

## 目的

在多角色协作里，“讨论结论”很容易被误当成“已决定的规格”，从而导致实现与验收分叉。

本文件定义一个最小的“盖章/Accepted”机制：

- 讨论可以自由发散
- 但只有被授权的盖章人通过后，才算“可执行的决定”

## 术语

- `Approver`：盖章人集合（可配置多个）
- `Decision`：需要被盖章的事项（接口、验收标准、合并顺序、是否 breaking 等）
- `Accepted`：已通过盖章，可作为实现/验收的依据

## V1 默认策略：any

V1 采用最简单的策略：

- `any`：任一 Approver 通过即 Accepted

理由：

- 启动成本最低，能最快把“讨论”和“决定”分离开
- 适合第一期把系统跑通，再逐步引入更复杂策略

## Approver 配置（建议）

Approver 不建议写死在代码里，建议由项目画像提供（每个项目可不同）。

建议以 “repo 层 overlay” 的方式放在项目 repo：

- `<outbox_repo>/.agents/workflow.toml` 的 `[approval]` 段

配置形式（V1 固定）：

- 仅支持 TOML：`<outbox_repo>/.agents/workflow.toml`

备注：

- JSON/YAML 可以作为后续扩展，但不应在 V1 文档里暗示“随便写都能生效”，否则会导致配置失效却难排查。

配置内容（概念）：

- `approvers`: Approver 身份字符串列表（identity list）
- `mode`: `any | all | quorum | staged`

示例（V1）：

```toml
[approval]
mode = "any"
approvers = ["agent-architect", "agent-integrator", "yoke233"]
```

安全要求：

- 盖章人身份必须以 Outbox backend 提供的“作者身份字段”为准（而不是正文文本）
  - GitHub/GitLab：comment/note author login（平台账号）
  - SQLite：`events.actor`（本地字符串 ID）
- 不允许仅凭正文出现 “/accept” 就判定通过（避免冒名或误触发）
- 本地/离线模式说明：SQLite backend 里 `actor` 通常只具备“约定俗成”的可信度，不具备平台级强认证；需要强审计时应迁移到 GitHub/GitLab 或服务端 DB（见 `docs/operating-model/outbox-backends.md`）

## 盖章动作（建议）

为了不强制评论模板，盖章建议使用“命令式短语”：

- `/accept`：通过
- `/reject <reason>`：拒绝
- `/needs-human <reason>`：需要人工介入，停止自动推进

系统侧的最小实现语义：

- 检测到 Approver 的 `/accept` 后：
   - 为 issue 增加 `decision:accepted` label
   - 可选：记录 `accepted-by:<identity>` 的 label，或在评论中回写一条机器评论用于审计

## 状态标签（建议）

为了让订阅者快速过滤，可采用一组 label：

- `decision:proposed`：存在待盖章的决策点（建议用于 `kind:proposal`，或任何需要 Approver 裁决的事项；普通 `kind:task` 不必默认打此标签）
- `decision:accepted`：已盖章，可执行
- `decision:rejected`：已拒绝
- `needs-human`：必须由人类介入（停止自动讨论/自动合并）

说明：

- label 是状态机的“结果”，不是规格本身
- 规格本身仍应落盘到真源（proto/ADR/PR）

## 未来扩展（第二期）

当 `any` 不能满足一致性与合规需求时，可扩展：

- `all`：所有 Approver 都需通过
- `quorum(k-of-n)`：至少 k 个通过
- `staged`：逐级审批（例如：先 Architect 通过，再 Integrator 通过）

扩展时仍应保持：

- 评论正文自由
- 结构化信息尽量由 mailbox skill 追加，而不是要求人手写模板

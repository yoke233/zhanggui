# Product Truth (需求层真源)

目标：把“业务输入”转化为可执行、可验收、可追溯的 Spec，避免规格散落在飞书/Notion/聊天记录里导致分叉。

本层强调 **单一真源**：

- 真源默认是 Issue里的 Spec 区块
- 若规格较大，可写到 repo 内 `spec.md`，并在 Issue 的 `SpecRef` 链接到该文件
- 不允许出现“多个地方各写一份规格”的情况

## 角色与职责

### BA（需求分析师）

负责把输入变成 Spec：

- Acceptance Criteria：可观察、可验收
- Out of Scope：明确不做什么，防止范围膨胀
- Edge Cases：边界条件/异常路径
- Non-Functional：性能/安全/合规/可用性等
- Data Semantics：数据口径（字段含义、时间窗口、统计口径）
- Risks：已知风险与未知风险

### PM/PO（产品经理/产品 Owner）

负责最终裁决：

- 做不做
- 先做什么
- 验收口径是否成立
- 重大变更的决策归档（写回 Issue，确保可追溯）

PM/PO 不直接指挥 Worker（避免指挥链混乱）；PM/PO 的输出是 Spec 与决策，而不是实现步骤。

## 输出物（Artifacts）

最小集合（Phase 1 即可要求）：

- Issue Spec 区块（或 `SpecRef` 链接的 repo 内 `spec.md`）
- Acceptance Criteria（至少 1 条，可观察）
- Out of Scope（至少 1 条，或明确 `none`）
- Risks（至少 1 条，或明确 `none`）

建议集合（可后补）：

- Metrics：上线后如何判定成功（例如转化率、延迟、错误率、成本）
- Rollout/Guardrails：灰度策略、回滚条件
- Data Semantics：字段/指标口径与样例

## Spec 模板（建议结构）

你可以把下面结构直接放进 Issue 的 Spec 区块，或作为 `spec.md` 的骨架：

1. Background / Context
2. Goal
3. Non-Goals (Out of Scope)
4. User Stories / Scenarios
5. Acceptance Criteria
6. Edge Cases
7. Non-Functional Requirements
8. Metrics
9. Risks / Open Questions

## 与交付层的接口

需求层把“做什么”定义清楚后，交付层负责：

- 将 Spec 拆成可交付的 work items（可能是 Epic + 子 Issue）
- 指派 Lead/队列优先级/里程碑
- 要求 Evidence 与 Close 形成闭环

注意：Spec 可以演进，但必须在 Issue 真源上演进（编辑 Issue Spec 区块或更新 `spec.md` 并链接）。


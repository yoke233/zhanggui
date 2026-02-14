# 文档生命周期与定稿流程

版本：v1.0  
状态：Draft  
Owner：PM / Architect / Lead

## 1. 目标

- 把“讨论稿 -> 可执行方案 -> 定稿”做成统一流程。
- 避免需求、PRD、技术方案分散在不同地方造成真源漂移。

## 2. 文档分层

- 规范层：`docs/standards/*`（长期稳定规则）
- 模型层：`docs/operating-model/*`（职责边界与阶段策略）
- 协议层：`docs/workflow/*`（交付执行协议）
- 功能层：`docs/features/<feature-id>/*`（单个 feature 的需求与方案）

## 3. 状态定义

- `Draft`：可讨论，不可作为实现依据。
- `Active`：已审批，可作为当前实现依据。
- `Superseded`：已被新版本替代。
- `Archived`：历史留存，不再维护。

## 4. 功能文档最小闭环

每个 `feature-id` 至少包含：

- `requirement.md`：业务需求与边界
- `prd.md`：交付计划与验收口径
- `tech-spec.md`：实现设计与测试策略

## 5. 定稿与生效

定稿需要同时满足：

1. 对应 `IssueRef` 已给出 `decision:accepted`（或等效审批结论）。
2. 文档头部状态从 `Draft` 更新为 `Active`。
3. 文档写明关联 `IssueRef`、`SpecRef`、版本信息。

## 6. 变更控制

- 任何 `Active` 文档变更必须附带变更说明（What/Why/Impact）。
- 若是破坏性变更，必须更新 `Out of Scope` 与迁移说明。
- 被替代文档需标记 `Superseded` 并指向新文档。

## 7. 审查检查项

- 实现前是否存在 `Active` 的 `tech-spec.md`。
- `IssueRef` 与 feature 文档路径是否一一对应。
- 当前实现证据是否回填到对应 `IssueRef`。

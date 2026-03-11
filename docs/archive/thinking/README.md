# 设计思考记录

架构设计推导过程，从具体问题到通用模式再到系统级决策。

## 文件索引

| # | 文档 | 主题 | 状态 |
|---|------|------|------|
| 01 | [PR/Merge 流程](01-pr-merge-flow.zh-CN.md) | `merging` 状态机、冲突解决、TL Triage | **已实现** |
| 06 | [Agent 工作空间](06-agent-workspace.zh-CN.md) | 方向概述：常驻 Agent + Inbox + Gateway，从固定流水线到动态协作 | 设计阶段 |
| 06-detail | [Agent 工作空间（详细）](06-agent-workspace-detail.zh-CN.md) | 完整代码、Schema、配置、IronClaw 能力吸收 | 参考文档 |
| 07 | [Thread / Message / Inbox / Bridge](07-thread-message-inbox-bridge.zh-CN.md) | 消息层收敛：thread 元数据、消息/投递分离、外部群组桥接 | 设计阶段 |
| 08 | [多 Agent 系统核心领域模型](08-multi-agent-core-domain-model.zh-CN.md) | 从消息内核走向协作操作系统：Task / Assignment / Decision / Artifact / Execution | 设计阶段 |
| 09 | [最小迁移路线图](09-migration-roadmap.zh-CN.md) | 基于当前仓库的 P0/P1/P2/P3 迁移计划，session_id 解耦 | **下一步** |
| -- | [IronClaw 架构学习](ironclaw-architecture-study.zh-CN.md) | IronClaw 项目架构分析，06 的 8 项能力吸收来源 | 参考资料 |

## 已归档（git 历史可查）

以下文档已删除，设计成果已融入 06 或已实现到代码中：

| # | 主题 | 去向 |
|---|------|------|
| 02 | Escalation/Directive 模式 | 被 06 Agent 消息模型取代 |
| 03 | A2A 协议映射 | 核心已实现 |
| 04 | A2A 对外接口与权限 | 已实现（多 token 认证） |
| 05 | 多用户多 Project 部署 | P0/P1 已实现 |
| plan-v1 | Merging + 冲突处理 | 已完成 |
| plan-v2 | TL ACP 决策 | 被 06 Agent 模型取代 |

## 实施顺序

| 顺序 | 内容 | 状态 |
|------|------|------|
| 第 1 步 | `merging` 状态 + 冲突处理 + TL Triage (01) | ✅ 已完成 |
| 第 2 步 | 跨 Project 分解 + token 权限分层 (04+05) | ✅ 已完成 |
| **下一步** | Agent 工作空间 P0: Thread + 消息 + session_id 解耦 (09) | 待实施 |

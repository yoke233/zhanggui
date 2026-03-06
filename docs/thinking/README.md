# 设计思考记录

架构设计推导过程，从具体问题到通用模式再到系统级决策。

## 文件索引

| # | 文档 | 主题 | 状态 |
|---|------|------|------|
| 01 | [PR/Merge 流程](01-pr-merge-flow.zh-CN.md) | `merging` 状态、冲突解决、TL Triage | **已实现** — merging 状态机、AutoMergeHandler、冲突检测、MergeRetries |
| 02 | [Escalation/Directive 模式](02-escalation-directive-pattern.zh-CN.md) | 通用层级决策协议，从 01 的 TL Triage 泛化而来 | **被 06 取代** — Actor 消息模型不需要硬编码类型 |
| 03 | [A2A 协议映射](03-a2a-escalation-mapping.zh-CN.md) | 02 的模式如何映射到 A2A 原语 | **核心已实现** — follow-up 回复、tasks/list；Escalation 路由待 02 |
| 04 | [A2A 对外接口与权限](04-a2a-external-access-design.zh-CN.md) | 外部操控/读取的权限分层，基于 03 的协议基础 | **已实现** — 多 token 认证(submitter/role/projects)、Agent Card skills、Task artifacts |
| 05 | [多用户多 Project 部署](05-multi-user-deployment-model.zh-CN.md) | 默认单实例多 project，三个独立问题（跨 project 分解 / 多用户 / 多实例），只在信任边界处拆实例 | **P0/P1 已实现** — 跨 Project 分解 + token 权限分层；P2 多实例联通待需求 |
| 06 | [Actor 工作空间](06-actor-workspace.zh-CN.md) | 常驻 Actor + Inbox + Gateway，从固定流水线进化到动态多 Agent 协作 | 设计阶段 |
| -- | [IronClaw 架构学习](ironclaw-architecture-study.zh-CN.md) | IronClaw 项目架构分析，06 的 8 项能力吸收来源 | 参考资料 |

## 阅读顺序（思考推导链）

```
01 (具体问题: merge 冲突怎么办)
 └→ 02 (泛化: 所有 agent 都能向上汇报) ─┐
     └→ 03 (映射: 用 A2A 协议表达)       ├→ 06 (Actor 工作空间: 统一通信模型)
         └→ 04 (扩展: 对外权限设计)       │
              05 (独立: 部署拓扑) ─────────┘
```

## 实施顺序（按系统缺口排优先级）

实施顺序不等于文件顺序。文件顺序是推导链（具体→抽象），实施顺序按"什么在阻塞系统真正跑起来"排。

| 顺序 | 来源 | 内容 | 状态 |
|------|------|------|------|
| 第 1 步 | 01 | `merging` 状态 + 冲突处理 + TL Triage | ✅ 已完成 |
| 第 2 步 | 05 P0 | 跨 Project 分解（`DecomposeSpec.ProjectID`） | ✅ 已完成 |
| 第 3 步 | 04 + 05 P1 | token 三合一模型（submitter + role + projects） | ✅ 已完成 |
| ~~不急~~ | ~~02 + 03~~ | ~~完整 Escalation/Directive 协议~~ | 被 06 Actor 模型取代 |

```
实施路径:
01 (TL 处理冲突)  ──→  发现更多异常场景  ──→  02 (抽象为通用协议)
     具体                                       抽象
```

## 实施计划

| 计划 | 文档 | 状态 |
|------|------|------|
| [plan-v1: Merging + 冲突处理 + TL Triage](plan-v1-merging-conflict-triage.zh-CN.md) | 对应第 1 步，3 个 Wave | 已完成（分支 plan-bubbly-giggling-karp） |
| [plan-v2: TL ACP 决策替代自动重试](plan-v2-tl-acp-triage.zh-CN.md) | v1→v2 升级，TL 启动 ACP session 分析冲突 | 待实施 |

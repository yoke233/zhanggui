# Thread 协同收敛到 DAG 的规划

> 状态：草案
>
> 创建日期：2026-03-13
>
> 适用范围：`Thread` / `WorkItem` / `DAG` / `ThreadAgentSession`
>
> 相关现状：
> - `Thread` 已独立建模并拥有 REST / WebSocket 协议
> - `ThreadAgentSession` 已支持多 agent 加入同一 Thread
> - `thread.send` 当前默认是 `mention_only`；仅在显式指定 `target_agent_id` 或设置 `agent_routing_mode=broadcast/auto` 时才会 fanout
> - `WorkItem + DAG` 仍是现行执行主线

## 1. 背景

当前系统已经具备两条重要能力：

1. `Thread`：用于多人、多 agent 的共享讨论。
2. `WorkItem + DAG`：用于明确任务后的依赖拆解与执行推进。

这两条能力分别解决了不同问题：

- `Thread` 擅长解决“问题还没有想清楚”的阶段。
- `WorkItem + DAG` 擅长解决“边界已经明确，如何稳定执行”的阶段。

当前缺口不在执行层，而在两者之间缺少一层清晰的“协同收敛层”：

- 如何让多个 agent 围绕一个问题分组讨论
- 如何让多个讨论小组并行思考而不互相打爆上下文
- 如何把讨论结果正式沉淀为 `WorkItem`
- 如何让这些 `WorkItem` 进入 DAG，而不是长期停留在线程聊天里

本文的目标不是替换现有 DAG 主线，而是在其前面补一层协同能力。

## 2. 核心判断

本规划采用以下固定判断：

1. `Thread` 与 `DAG` 不解决同一个问题。
2. `Thread` 负责协同收敛，不直接承担最终执行真相。
3. `WorkItem + DAG` 继续作为执行真相，不被 mesh 协作替代。
4. 多小组协同应优先建立在现有 `Thread` 模型之上，而不是立即引入一套并行的 `team/org` 控制面。
5. 跨小组传播应默认基于 `summary / artifact / decision`，而不是跨组实时消息互刷。

换句话说：

- 想清楚，用 `Thread`
- 做完它，用 `WorkItem + DAG`

## 3. 设计目标

### 3.1 要解决的问题

1. 需求模糊、边界不清时，支持多个 agent 协同讨论。
2. 支持“多小组并行思考”，例如架构组、实现组、测试组。
3. 支持从讨论结果中提炼结构化结论。
4. 支持从结构化结论生成一个或多个 `WorkItem`。
5. 支持把收敛后的工作项组织为 DAG，进入现有执行主线。
6. 支持执行结果回流到讨论层，形成闭环。

### 3.2 明确不做的事

1. 不替换现有 `WorkItem + DAG` 执行链路。
2. 不在第一阶段引入复杂的 `team/org` 文件控制面。
3. 不在第一阶段实现任意 agent 间自由私聊或跨组实时 mesh。
4. 不在第一阶段实现完整的多 assignee 状态机。
5. 不在第一阶段引入全新的执行对象或平行任务模型。

## 4. 分层模型

本规划将系统分为三层：

### 4.1 协同层

以 `Thread` 为核心，负责：

- 发起讨论
- 邀请 agent 参与
- 记录共享上下文
- 形成小组讨论空间

### 4.2 收敛层

负责把讨论内容沉淀为结构化结果，例如：

- `summary`
- `proposal`
- `decision`
- `risk`
- `todo`
- `work_item draft`

收敛层是本文新增规划的重点。

### 4.3 执行层

以 `WorkItem + DAG + Execution + Artifact` 为核心，负责：

- 任务拆解
- 依赖管理
- 状态推进
- 产物沉淀

执行层继续沿用现有主线。

## 5. 概念模型

### 5.1 Thread

`Thread` 继续作为共享讨论容器，不做重定义。

在本规划中，`Thread` 可能扮演两种角色：

1. 普通讨论线程
2. 某个协作小组的讨论空间

也就是说，一个 Thread 既可以是一场普通多人讨论，也可以代表“前端组”或“架构组”。

### 5.2 Coordination Space

这是本文新增的逻辑概念，用于表示“围绕一个主题的多 Thread 协作空间”。

第一阶段不要求独立建表，可以先作为 `Thread.Metadata` 或关联关系表达。

它的职责是：

- 标识某几个 Thread 属于同一个协作主题
- 区分“主协调 Thread”和“子小组 Thread”
- 为后续的 summary 聚合提供锚点

### 5.3 Group Thread

一个 `Coordination Space` 下可以存在多个小组 Thread，例如：

- 主协调 Thread
- 架构组 Thread
- 前端组 Thread
- 后端组 Thread
- 测试组 Thread

这些 Thread 不是新的领域对象，本质上仍然是 Thread，只是承担不同职责。

### 5.4 WorkItem

`WorkItem` 继续作为执行域的统一工作单元。

在本规划中，`WorkItem` 的职责是承接 Thread 收敛结果，而不是承接 Thread 的原始全文聊天内容。

### 5.5 Artifact

`Artifact` 继续作为显式输出对象。

在协同阶段，重要结论应优先沉淀为 artifact 或结构化 summary，而不是让关键结论长期埋在线程消息流中。

## 6. 运行流程

### 6.1 单组模式

适用于问题不大，但仍需要先讨论的场景。

流程：

1. 创建一个主 Thread
2. 邀请多个 agent 加入
3. 进行讨论与澄清
4. 产出 summary / proposal / todo
5. 从 Thread 生成一个或多个 `WorkItem`
6. 根据明确度决定是否立即转为 DAG

### 6.2 多小组模式

适用于跨职能、跨领域、边界尚未收敛的大问题。

流程：

1. 创建一个主协调 Thread
2. 为不同小组创建多个子 Thread
3. 每个小组在自己的 Thread 内讨论
4. 各小组定期提交 summary 到主协调 Thread
5. 主协调 Thread 聚合总结
6. 从聚合结果生成 `WorkItem`
7. 将 `WorkItem` 组织为 DAG
8. 执行过程中的 blocker / 结果再回流到主协调 Thread

### 6.3 执行回流模式

当 DAG 执行中出现以下情况时，需要回流协同层：

- blocker
- 方案冲突
- 新依赖暴露
- 验收标准不明确
- 风险超出原讨论范围

此时不应在执行节点里无限兜圈，而应：

1. 回写 blocker summary
2. 回到主协调 Thread 或相关小组 Thread
3. 重新收敛
4. 再更新 WorkItem / DAG

## 7. 边界策略

### 7.1 组内边界

组内 Thread 保持现有共享讨论模型。

短期内接受：

- 多个 agent 在同一 Thread 中共同收到讨论消息
- 由现有广播式 `thread.send` 驱动多 agent 响应

这是现状，不在本规划第一阶段中推翻。

### 7.2 组间边界

多小组之间不建议默认做“所有消息互通”。

推荐策略：

1. 跨组只同步 summary
2. 跨组只同步 decision
3. 跨组只同步 artifact 引用
4. 不同步完整实时消息流

这样做的原因：

- 减少上下文污染
- 降低噪声
- 保持讨论边界
- 为后续 leader / coordinator 模型预留空间

### 7.3 执行边界

一旦进入 `WorkItem + DAG`，执行层应遵守：

1. 以结构化工作项为输入
2. 以依赖关系为推进依据
3. 以 artifact 为主要结果沉淀
4. 不把开放式讨论重新塞回执行主链

## 8. 数据与对象演进建议

本节区分“远期设计”与“近期实现”。

### 8.1 远期设计

远期可考虑引入以下能力：

1. `coordination_id`
   - 将多个 Thread 归入同一协作空间

2. `parent_thread_id`
   - 支持主协调 Thread 与子 Thread 的父子关系

3. `thread summary`
   - 为每个 Thread 保持最新结构化摘要

4. `thread decision`
   - 从讨论中沉淀显式决策对象

5. `work item draft`
   - 支持 Thread 中先形成草稿，再正式落 WorkItem

6. `coordinator agent`
   - 标识某个 Thread 或某个协调空间的代表 agent

7. `work item ownership / collaborators`
   - 为后续的多 assignee 或 Assignment 铺路

### 8.2 近期实现

近期不建议一次性引入上述完整对象。

建议优先顺序：

1. 先补“Thread -> Summary -> WorkItem”
2. 再补“多 Thread 汇总”
3. 最后才考虑更正式的 coordinator / assignment 模型

## 9. 分阶段实施计划

### P0：规划与边界收口

目标：

- 在文档层明确 `Thread`、`WorkItem`、`DAG` 的关系
- 避免未来功能设计继续漂移

实现：

- 新增本规划文档
- 后续相关 spec 引用本规划

验收：

- 后续讨论以“协同层 / 收敛层 / 执行层”三层术语表达

### P1：Thread 收敛动作 MVP

目标：

- 让一个 Thread 的讨论结果可以正式进入执行层

细化文档：

- `thread-summary-workitem-mvp.zh-CN.md`

最小实现：

1. Thread summary 生成能力
2. 从 Thread 创建一个或多个 WorkItem
3. 自动建立 Thread 与 WorkItem 的关联
4. 在 UI 上展示“由哪个 Thread 收敛而来”

不做：

- 不做多小组
- 不做复杂路由策略
- 不做完整 assignment

验收：

- 一个 Thread 可以稳定地产生可执行 WorkItem

### P2：多 Thread 协同 MVP

目标：

- 支持一个主题下多个小组 Thread 协作

最小实现：

1. 主 Thread / 子 Thread 关系
2. 子 Thread 提交 summary 到主 Thread
3. 主 Thread 聚合多个 summary
4. 从聚合结果批量生成 WorkItem

不做：

- 不做实时跨组消息互通
- 不做复杂 team/org 权限

验收：

- 一个主题下至少支持“主协调 + 2 个小组”协作闭环

### P3：协同到 DAG 的自动化增强

目标：

- 从收敛结果更顺滑地进入执行编排

最小实现：

1. 从多个 WorkItem 生成初步 DAG
2. 支持在主 Thread 中查看 DAG 草案
3. 支持从执行 blocker 回写主 Thread

验收：

- 从讨论到 DAG 的链路可形成闭环

### P4：责任模型增强

目标：

- 让执行层更好承接协同层输出

最小实现建议：

1. 为 `WorkItem` 增加 owner / coordinator / collaborators 能力
2. 优先做轻量字段或 metadata
3. 暂缓完整的多 assignee claim / completion 状态机

验收：

- WorkItem 能表达“谁主负责、谁协同参与”

### P5：更正式的多小组治理

目标：

- 在验证需求成立后，再考虑更强治理能力

可能能力：

1. coordinator-only 跨组同步
2. 更正式的 group policy
3. coordinator approval
4. 显式 Assignment 对象

说明：

- 这一阶段不应前置
- 只有在前几个阶段已证明需求稳定时才值得做

## 10. 近期推荐落地顺序

如果只选最值得做的两步，推荐顺序如下：

1. `Thread -> Summary -> WorkItem`
2. `主 Thread + 子 Thread + Summary 聚合`

这两步已经足以覆盖大多数“先讨论、再执行”的真实场景。

相比之下，以下内容不建议前置：

- 全新 team/org 控制面
- 完整 leader-only 通信治理
- 完整多 assignee 状态机
- 任意 agent 间复杂路由规则

## 11. 风险与取舍

### 11.1 最大风险

一边保留现有 `Thread` 主线，一边再新做一套平行的多 agent 协作模型，会造成双轨复杂度失控。

因此本文明确要求：

- 优先扩展 `Thread`
- 暂不引入平行协作域

### 11.2 次级风险

如果过早做完整多小组治理，系统会在还没验证真实使用模式前就承担大量模型复杂度。

因此本文明确要求：

- 先验证“讨论如何收敛为 WorkItem”
- 再验证“多小组是否真有稳定需求”
- 最后再决定是否上更重的治理模型

## 12. 结论

本规划的最终方向不是“用 mesh 协作替代 DAG”，而是：

1. 用 `Thread` 承接开放式讨论
2. 用收敛动作把讨论结果结构化
3. 用 `WorkItem + DAG` 承接正式执行
4. 用 artifact 和 summary 让执行结果可回流、可复盘

这意味着系统未来的主链应是：

`Thread 协同` -> `Summary / Decision / WorkItem` -> `DAG 执行` -> `Artifact / Blocker 回流`

这条链路既能支持“大问题先协同”，也能保住现有“小步可执行”的工程优势。

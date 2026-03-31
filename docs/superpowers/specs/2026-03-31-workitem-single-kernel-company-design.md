# WorkItem 单核公司任务系统重构设计

> 日期：2026-03-31
> 状态：草案
> 类型：重构设计
> 范围：任务主模型、层级上报、CEO 入口收敛

---

## 1. 背景

当前系统已经具备 `ChatSession / Thread / WorkItem / Action / Run` 等能力，但
“谁在负责、谁该处理、谁来升级、谁拿结果”仍然分散在多个对象和零散 metadata
里：

- `WorkItem` 代表正式任务，但没有成为唯一任务真相
- `Action` 同时承担流程节点和部分责任语义
- `Thread` 既像协作空间，又被期待承载任务升级
- `Notification` 容易被误用为待办来源
- `metadata["ceo"]`、`ceo_journal` 等旁路字段持续累积业务语义

这导致 CEO 场景虽然已经能跑 MVP，但系统骨架仍偏“工具拼接”，还不是
“AI 公司”的任务操作系统。

本次重构目标不是继续缝合边界，而是直接把系统收敛成一个更简单的主模型：

> CEO 拆出多个 `WorkItem`，每个 `WorkItem` 自己闭环推进，遇阻则沿上级链逐级上报。

---

## 2. 设计目标

本次重构要达成以下目标：

1. 让 `WorkItem` 成为系统唯一任务真相
2. 让 CEO 只负责拆解、派发、跟踪、处理升级、收集结果
3. 让执行流程默认保持“执行 -> 审核”的轻量闭环
4. 让异常处理遵循“逐级上报”，而不是直接回 CEO
5. 让 `Thread` 回归协作现场，而不是任务状态容器
6. 让前后端都能围绕 `WorkItem` 直接构建“我的待办 / 我管理的任务 / 升级给我”

---

## 3. 非目标

本次重构明确不做以下内容：

- 不引入新的 `Inbox` 主实体
- 不把 `Thread` 发展成新的任务中心
- 不保留“多套并行真相”的兼容设计
- 不继续扩展 `metadata` 承载核心状态
- 不优先设计复杂的组织治理系统（部门、编制、权限树）
- 不先做完整多 human 协同产品面

---

## 4. 核心结论

### 4.1 `WorkItem` 是唯一主线

以后系统里的核心问题都应先由 `WorkItem` 回答：

- 这件事是什么
- 现在是谁负责
- 下一步谁处理
- 是否处于审核
- 是否被卡住
- 是否正在上报
- 最终结果是什么

`Action`、`Thread`、`Notification` 都不能再与 `WorkItem` 并列为任务真相来源。

### 4.2 CEO 不是执行器

CEO 的职责收敛为：

1. 接收用户高层目标
2. 拆成多个 `WorkItem`
3. 为每个 `WorkItem` 指定负责人和审核人
4. 跟踪 `WorkItem` 状态
5. 处理升级或继续上报
6. 汇总最终结果返回给用户

CEO 默认不做具体执行，不直接代替 worker/lead 干活。

### 4.3 `WorkItem` 默认且仅支持两段式

每个 `WorkItem` 默认只有一条标准流程：

```text
待执行
  -> 执行中
  -> 待审核
  -> 已完成
```

本期重构明确只落地两段式，不做可配置流程引擎。

“复杂任务允许 CEO 特批扩展”只保留为后续演进方向，不进入本期实现范围。

### 4.4 异常流转遵循逐级上报

异常路径不应默认回 CEO，而应按上级链逐级升级：

```text
执行者卡住
  -> 上报给直属上级
  -> 上级处理不了则继续上报
  -> 最终到 CEO
  -> CEO 仍处理不了再转 human
```

这意味着“待处理”首先是一个 `WorkItem` 级别的责任流转，而不是 chat 消息、
notification 或 thread 消息。

---

## 5. 新领域模型

### 5.1 `WorkItem` 需要承担的核心字段

为了避免在 `WorkItem` 内部再次长出多套“当前真相”，本设计只保留一条当前责任主线：

- `goal`：任务目标
- `status`：任务状态
- `executor_profile_id`：默认执行者
- `reviewer_profile_id`：默认审核者
- `active_profile_id`：**当前唯一责任人**
- `blocked_reason`：卡点原因
- `created_by_profile_id`：谁创建了这张任务单
- `sponsor_profile_id`：谁对结果负责，通常是 CEO 或上级管理者
- `parent_work_item_id`：父任务，用于 CEO 拆分
- `root_work_item_id`：根任务，用于整棵任务树聚合
- `escalation_path`：当前责任人的上报链缓存
- `final_deliverable_id`：最终正式交付物

字段语义约束：

- `status` 决定当前阶段
- `active_profile_id` 决定现在该谁处理
- `executor_profile_id` 与 `reviewer_profile_id` 是默认流程参与者，不代表当前责任人
- `escalation_path` 只是当前升级路径缓存，不是历史审计容器
- 历史责任变更、改派、升级记录进入正式 `WorkItem` 事件流，不再塞 metadata

这意味着：

- `pending_review` 时，`active_profile_id` 必须等于 `reviewer_profile_id`
- `needs_rework` / `in_execution` 时，`active_profile_id` 必须等于 `executor_profile_id`
- `escalated` 时，`active_profile_id` 是当前接棒上级
- 前端待办查询统一基于 `active_profile_id + status`
- `final_deliverable_id` 是唯一结果真相；摘要、卡片展示、CEO 汇总文案都从该交付物派生

### 5.2 统一 `Deliverable` 模型

本设计采用一套统一的 `Deliverable` 类型体系，同时服务于 `Thread` 与 `WorkItem`。

`Deliverable` 不是“必须是文件”，而是统一交付对象。它可以表示：

- 文档
- 代码改动
- Pull Request
- 决策结论
- 会议纪要
- 聚合报告

最小字段建议：

- `id`
- `kind`
- `title`
- `summary`
- `payload`
- `producer_type`：`run` / `thread` / `workitem`
- `producer_id`
- `status`
- `created_at`

典型 `kind`：

- `document`
- `code_change`
- `pull_request`
- `decision`
- `meeting_summary`
- `aggregate_report`

统一规则如下：

- `Thread` 可以产出协作型 `Deliverable`
- `WorkItem` 可以采纳某个 `Deliverable` 作为正式结果
- 只有 `WorkItem.final_deliverable_id` 指向的对象，才代表任务最终结果
- `Thread` 里的 `Deliverable` 不能天然关闭 `WorkItem`

例如：

- 代码改动任务可产出 `code_change` 或 `pull_request`
- 文档任务可产出 `document`
- CEO 父任务可产出 `aggregate_report`
- 会议 thread 可产出 `meeting_summary` 或 `decision`

### 5.3 `Action / Run / ActionSignal` 的新定位

`WorkItem` 是流程真相；`Action / Run / ActionSignal` 是执行底座。

- `Action`：`WorkItem` 状态推进时生成的执行节点
- `Run`：某个 `Action` 的一次真实执行尝试
- `ActionSignal`：执行中的事件信号，例如 blocked / need_help / complete

强约束如下：

- 调度入口从 `WorkItem` 发起，不再从 pending `Action` 发起
- 当前责任人、待办、人类介入、升级对象都只从 `WorkItem` 读取
- `ActionSignalStore` 不再提供主待办查询
- `Run` 继续承载执行结果，但最终正式结果必须落为一个 `Deliverable`
- `WorkItem` 只暴露 `final_deliverable_id` 作为唯一结果锚点

这不是“废掉 Action 引擎”，而是把它降为执行基础设施，不再和 `WorkItem`
竞争任务真相。

### 5.4 `Thread` 的新定位

`Thread` 只做协作现场：

- 开会
- 讨论
- 邀请额外 agent
- 临时同步上下文
- 产出协作型 `Deliverable`

`Thread` 可附着到 `WorkItem`，但不能再决定 `WorkItem` 的正式状态。

如果某个 `Thread Deliverable` 被 `WorkItem` 采纳，它可以成为最终交付物；
否则它只是一份协作产物。

### 5.5 `Notification` 的新定位

`Notification` 只负责提示和展示，不承担任何任务状态真相。

例如：

- “某个 `WorkItem` 升级给你了”
- “你有一个待审核 `WorkItem`”
- “CEO 已收齐全部子任务结果”

通知是派生物，不是源数据。

### 5.6 `WorkItem` 是流程源，运行层只做执行

新的硬边界如下：

```text
WorkItem 命令
  -> 生成/推进 Action
  -> 驱动 Run
  -> 写回 WorkItem 状态
```

也就是说：

- 运行层不能自己决定任务负责人
- 运行层不能自己生成待办真相
- 运行层只把执行事实回写给 `WorkItem`

### 5.7 正式 `WorkItem` 事件流

本设计要求一条最小可用的正式事件流，用来承载历史而不是当前真相。

优先做法是复用现有 `Journal` 基础设施，扩展为可表达以下事件：

- `workitem.created`
- `workitem.assigned`
- `workitem.escalated`
- `workitem.resumed`
- `workitem.review_requested`
- `workitem.review_rejected`
- `workitem.completed`

每条事件至少记录：

- `work_item_id`
- `event_kind`
- `actor_profile_id`
- `from_profile_id`
- `to_profile_id`
- `status_before`
- `status_after`
- `note`
- `created_at`

边界要求：

- 事件流只承载历史，不承载当前责任真相
- 详情页、审计页、升级历史统一从这里读取
- 禁止再新增 `ceo_journal` 一类 metadata 历史通道

---

## 6. 新状态机

### 6.1 主状态

建议把 `WorkItem` 主状态收敛为：

- `pending_execution`
- `in_execution`
- `pending_review`
- `needs_rework`
- `escalated`
- `completed`
- `cancelled`

### 6.2 状态语义

- `pending_execution`：等待 `active_profile_id` 开始，且该人应为执行者
- `in_execution`：执行者正在处理
- `pending_review`：执行已完成，等待 reviewer 审核
- `needs_rework`：审核退回，需要 executor 重做
- `escalated`：当前卡住，等待上级链中的 `active_profile_id` 处理
- `completed`：结果确认完成
- `cancelled`：终止，不再推进

### 6.3 典型流转

正常流：

```text
pending_execution
  -> in_execution
  -> pending_review
  -> completed
```

返工流：

```text
pending_review
  -> needs_rework
  -> in_execution
  -> pending_review
  -> completed
```

升级流：

```text
in_execution / pending_review
  -> escalated
  -> in_execution 或 pending_review
```

状态不再额外引入 `current_stage`。阶段语义直接由 `status` 表达。

---

## 7. 组织关系与上报链

### 7.1 组织关系定义位置

组织关系定义在 profile，并成为一等配置：

- `profile_id`
- `manager_profile_id`

CEO 的 `manager_profile_id` 为空。

human 不要求一定是 profile，但必须作为最终升级出口存在。

### 7.2 当前上报链缓存

系统根据当前 `active_profile_id` 所在组织关系计算：

```text
active_profile -> manager -> ... -> ceo -> human
```

并写入 `escalation_path`。

这里明确不做不可变快照。规则如下：

- 初次创建时生成一次
- 改派后立即按新的责任人重算并覆盖
- 每次升级都沿当前 `escalation_path` 取下一个接棒人
- 历史链路变化进入 `WorkItem` 事件流

这样能避免“责任人变了，但升级还发给旧上级”的问题。

### 7.3 迁移前提

逐级上报不是 `WorkItem` 单表改造，而是一次 profile 组织模型升级。

因此本次重构必须先完成：

- profile 增加 `manager_profile_id`
- profile 装载、校验、默认 seed 与管理 CLI 同步支持
- 组织链解析器成为后端一等组件

在这一步完成前，`WorkItem` 单核模型不算真正落地。

---

## 8. CEO 工作模式

### 8.1 用户与 CEO 的交互

用户仍只在 `/chat` 中与 CEO 交互。

CEO 处理一个用户目标的标准动作：

1. 理解目标
2. 判断是否需要拆分
3. 创建多个 `WorkItem`
4. 指派 executor / reviewer
5. 跟踪推进
6. 处理中途升级
7. 汇总所有已完成 `WorkItem` 的结果
8. 向用户返回整体结论

### 8.2 CEO 不应承担的动作

CEO 不应：

- 直接执行普通 worker 工作
- 自己变成 review worker
- 长期停留在 thread 里代替执行器开工
- 把所有问题都直接拉到 CEO 处理

CEO 只在上级链最终到达自己时才接管异常。

---

## 9. 待办与收件箱视图

虽然本次不引入 `Inbox` 实体，但系统仍需要提供稳定视图。

这些视图都应从 `WorkItem` 直接查询得到：

- 我的待办：`active_profile_id = me` 且 `status != completed/cancelled`
- 我的待审核：`active_profile_id = me` 且 `status = pending_review`
- 升级给我的：`active_profile_id = me` 且 `status = escalated`
- 我创建的任务：`created_by_profile_id = me`
- 我负责收口的任务：`sponsor_profile_id = me`
- CEO 总览：`sponsor_profile_id = ceo` 或位于 CEO 根任务树下的活跃 `WorkItem`

这类视图不应继续从 `ActionSignalStore` 推导。

---

## 10. 应废弃或收缩的旧边界

### 10.1 废弃“待办来自 ActionSignal”这条主线

`ActionSignal` 可以保留为事件，但不再作为“待处理事项”的核心来源。

### 10.2 收缩 `metadata["ceo"]`

`metadata["ceo"]` 不应继续积累业务真相。

允许短期保留少量创建来源字段，但以下内容必须迁出：

- 当前负责人语义
- 当前升级语义
- 过程历史
- 审批或分派日志

### 10.3 停止扩展 `ceo_journal`

过程历史应进入正式 `WorkItem` 事件记录模型，而不是继续挂在 metadata 列表里。

### 10.4 `Notification` 禁止承担待办真相

所有通知必须由 `WorkItem` 派生生成。

### 10.5 `Thread` 不再作为任务升级真相

是否开会、是否邀请 agent、是否创建 thread，都只能影响协作方式，不影响
`WorkItem` 的责任链真相。

---

## 11. 迁移策略

### 阶段 0：组织模型先落地

- profile 增加 `manager_profile_id`
- seed / sqlite / CLI / API / 前端配置统一支持组织链
- 引入组织链解析器

### 阶段 1：`WorkItem` 主字段切换

- 为 `WorkItem` 增加 `executor_profile_id`、`reviewer_profile_id`
- 增加 `active_profile_id`、`created_by_profile_id`、`sponsor_profile_id`
- 增加 `root_work_item_id`、`escalation_path`、`final_deliverable_id`
- 引入新状态机

### 阶段 1.5：历史数据回填

- 将旧 `WorkItem.status` 映射到新状态机
- 从旧 `metadata["ceo"]`、旧分派字段、旧运行记录回填 `executor_profile_id`
- 回填 `reviewer_profile_id`、`active_profile_id`、`sponsor_profile_id`
- 为已有任务树补齐 `root_work_item_id`
- 根据现存运行结果或 thread 采纳关系回填 `final_deliverable_id`
- 对无法自动回填的旧单据标记为需人工迁移，禁止静默跳过

### 阶段 2：写路径 cutover

- CEO 编排入口只写新 `WorkItem` 主字段
- `withAssignedProfile`、`appendCEOJournal` 停止写新旁路数据
- 旧 metadata 字段进入只读兼容，不再增量写入

### 阶段 3：读路径 cutover

- 待办、审核、升级列表全部切到 `WorkItem` 查询
- 前端主视图切到 `active_profile_id + status`
- `ActionSignalStore` 的 pending 查询接口下线或转兼容壳

### 阶段 4：运行层回写收口

- 运行层只通过 `WorkItem` 状态推进接口回写结果
- `final_deliverable_id` 成为最终结果唯一锚点
- `Action / Run / Signal` 不再被前端当任务真相读取

### 阶段 5：删旧

- 删除 `ceo_journal` 写路径
- 删除和待办真相冲突的旧接口
- 将旧 metadata 路由字段迁移完毕后物理移除

cutover 判定条件：

- 新建 `WorkItem` 不再写旧路由 metadata
- 历史活跃 `WorkItem` 已完成字段回填或被明确标记为人工处理
- 前端主待办不再依赖 `ActionSignalStore`
- CEO 汇总结果只读 `WorkItem + final_deliverable_id`
- 详情页、时间线、升级历史统一读取正式 `WorkItem` 事件流
- 任务详情与排障接口不再从 metadata 推断当前责任人

---

## 12. 风险与取舍

### 12.1 优点

- 系统主线会明显变清楚
- CEO 行为会更像管理者而不是执行器
- 前后端展示都更自然
- “逐级上报”可以直接落在任务流里

### 12.2 代价

- 需要修改现有 `WorkItem`、查询接口、前端视图和 CEO 编排入口
- 需要清理部分已经落地的旁路字段和旧接口
- 需要接受一次较硬的 cutover，而不是无限期兼容

### 12.3 关键取舍

本设计明确选择：

- 接受一次较大的主线收敛
- 不为了短期兼容继续保留多套任务真相
- 把复杂度集中到 `WorkItem`，换取系统整体简化

---

## 13. 验收标准

当以下条件成立时，可认为本次重构方向成功：

1. 用户给 CEO 一个大目标后，CEO 会拆成多个 `WorkItem`
2. 每个 `WorkItem` 默认按“执行 -> 审核”推进
3. 执行中断时，任务按上级链逐级升级，而不是默认直接回 CEO
4. CEO 只处理最终收口和高阶升级
5. 前端主待办只基于 `active_profile_id + status`
6. `final_deliverable_id` 成为最终结果唯一锚点
7. `Notification`、`Thread`、`ActionSignal` 不再作为任务真相来源

---

## 14. 一句话结论

这次重构的本质不是补一个新 inbox，而是把系统彻底收敛成：

> 以 `WorkItem` 为唯一任务单，以默认两段式流程推进，以逐级上报处理异常，以 CEO 做拆解和收口的 AI 公司任务系统。

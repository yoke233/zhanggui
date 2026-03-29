# P1 细化：Thread Summary 到 WorkItem 的 MVP

> 状态：草案
>
> 最后按代码核对：2026-03-29
>
> 上位规划：`thread-collaboration-to-dag-plan.zh-CN.md`
>
> 目标：在不引入新协作主模型的前提下，打通 `Thread -> Summary -> WorkItem` 最小闭环

## 1. 目标

P1 的目标不是完整实现多小组协同，而是先让一个 `Thread` 能稳定完成以下动作：

1. 沉淀当前讨论的可复用 summary
2. 基于 summary 创建一个或多个 `WorkItem`
3. 自动建立 Thread 与 WorkItem 的显式关联
4. 让用户在 UI 中明确看到“这个 WorkItem 来自哪个 Thread 收敛”

P1 是后续多小组协同的基础。如果这一层做不好，P2 的“主 Thread + 子 Thread”只会把噪声放大。

## 2. 现状基础

按 2026-03-29 当前代码核对，P1 原稿中的一部分前提并未按原方案落地。

当前真正已具备的基础能力包括：

1. `POST /threads/{id}/create-work-item` 已可从 Thread 创建单个 WorkItem
2. 当请求未显式提供 `body` 时，当前实现会回退到 `Thread.Title`，而不是 `Thread.summary`
3. `thread_work_item_links` 已支持 Thread 与 WorkItem 的显式链接
4. Thread 详情页已展示 linked work items
5. Thread 侧已具备 `Proposal -> Initiative -> WorkItem` 的审批与执行前收敛主链
6. 当前 public surface 上尚未提供 `POST /chat/sessions/{sessionID}/crystallize-thread`

也就是说，这篇草案不是“把现有 `Thread.summary` 流程正式化”，而更像是：

- 在现有 `Thread -> create-work-item` 和 `Proposal / Initiative` 主链之外，
- 重新评估是否还需要单独的 `Thread.summary -> WorkItem` MVP。

补充说明：

- `crystallize-thread` 当前不是现行能力，不能再当作本文的现状前提
- 当前现行主线更接近 `Thread -> Proposal / Initiative -> WorkItem`，而不是 `Thread.summary`

## 3. P1 的核心原则

### 3.1 复用现有对象

P1 不新增新的一级领域对象。

继续复用：

- `Thread`
- `Thread.summary`
- `Thread.metadata`
- `WorkItem`
- `ThreadWorkItemLink`

### 3.2 Summary 先于 WorkItem

P1 要强制建立一个习惯：

- 先形成 summary
- 再从 summary 产生 WorkItem

而不是直接从混乱消息流里临时抠一个任务名去创建工单。

### 3.3 单 Thread 优先

P1 只处理单个 Thread 的收敛。

不处理：

- 多 Thread 聚合
- 主 Thread / 子 Thread
- 跨组同步

这些属于 P2。

### 3.4 先单 WorkItem，再批量

P1 允许规划批量 WorkItem，但实现时建议分两步：

1. 先稳定支持“从 summary 创建 1 个高质量 WorkItem”
2. 再补“从 summary 批量创建多个 WorkItem”

## 4. 目标用户流程

P1 建议的标准流程：

1. 用户或 agent 在 Thread 中讨论
2. 用户触发“生成 summary”或“更新 summary”
3. Thread 保存结构化 summary
4. 用户基于 summary 创建 WorkItem
5. 系统自动建立 `drives + is_primary=true` 的关联
6. 用户进入 WorkItem 详情继续执行或后续生成 DAG

从产品角度，P1 解决的是：

“讨论结束后，我如何把结论正式推入执行系统？”

## 5. Summary 设计

### 5.1 Summary 的定位

`Thread.summary` 是 P1 的主入口，不是附属展示字段。

它应被视为：

- 当前 Thread 的收敛结果
- 后续创建 WorkItem 的默认输入
- 后续 P2 聚合多个子 Thread 结果的基础单元

### 5.2 Summary 的最小结构

P1 不强制数据库中新增独立结构化表，但建议在逻辑上采用以下结构：

```json
{
  "problem": "要解决的问题",
  "context": "必要背景",
  "decisions": ["已达成的关键决定"],
  "risks": ["主要风险"],
  "todos": ["待落实事项"],
  "suggested_work_items": [
    {
      "title": "建议的 WorkItem 标题",
      "body": "建议的 WorkItem 描述"
    }
  ]
}
```

P1 的存储策略建议分层处理：

1. `Thread.summary`
   - 存放面向人类可读的文本摘要

2. `Thread.metadata.summary_draft`
   - 可选，存放结构化 JSON 草稿

这样做的原因：

- 不需要立刻改 `Thread` 主字段结构
- UI 可先用文本 summary 展示
- 后续 P2/P3 可以继续复用结构化草稿

### 5.3 Summary 的来源

P1 不要求一步做到完全自动。

可接受三种来源：

1. 人工编辑
2. agent 辅助生成后由人确认
3. 直接由 agent 生成并写入，但仍允许人覆盖

推荐顺序是：

- 先支持人工可编辑
- 再支持 agent 自动生成草稿

## 6. WorkItem 生成设计

### 6.1 现有能力

当前已有：

- `POST /threads/{id}/create-work-item`
- 请求体支持 `title`、`body`、`project_id`
- 创建后自动建立 `drives + is_primary=true` 的 link

这说明 P1 不需要重新设计新的主创建链路。

### 6.2 P1 增强目标

P1 要做的是让这个现有接口更符合“从 summary 收敛”的语义。

建议增强方向：

1. 当请求未显式提供 `body` 时，可默认使用 `Thread.summary`
2. 当 `Thread.summary` 为空时，明确提示“请先生成或填写 summary”
3. 创建后在 WorkItem `metadata` 中记录来源信息

推荐来源字段：

```json
{
  "source_thread_id": 123,
  "source_type": "thread_summary"
}
```

### 6.3 批量创建的处理策略

P1 不建议立即做复杂批量接口。

如果需要预留，建议只做逻辑规划：

- 后续可新增 `POST /threads/{id}/create-work-items`
- 请求体为 `items[]`
- 每一项都会自动建 link

但本阶段先不落地。

## 7. API 草案

P1 优先复用现有接口，只做最少补充。

### 7.1 复用接口

1. `PUT /threads/{id}`
   - 用于更新 `summary`

2. `POST /threads/{id}/create-work-item`
   - 用于从 summary 创建 WorkItem

3. `GET /threads/{id}/work-items`
   - 用于查看该 Thread 已收敛出的 WorkItem

### 7.2 建议新增的轻量接口

P1 可选新增一个辅助接口：

`POST /threads/{id}/summary:generate`

用途：

- 触发基于最近消息、参与者、关联 WorkItem 的 summary 草稿生成

返回：

```json
{
  "summary": "面向人类的文本摘要",
  "structured": {
    "problem": "...",
    "decisions": [],
    "risks": [],
    "todos": [],
    "suggested_work_items": []
  }
}
```

说明：

- 这是辅助接口，不替代 `PUT /threads/{id}`
- 最终是否保存，仍应由调用方决定

如果要更小步，甚至可以先不实现这个接口，只允许客户端把生成内容写回 `PUT /threads/{id}`。

## 8. UI 动作草案

P1 建议在 `ThreadDetailPage` 上补三类动作。

### 8.1 Summary 区域

现状已经可以展示 `thread.summary`。

P1 建议增加：

1. 编辑 summary
2. 生成 summary 草稿
3. 覆盖保存 summary

### 8.2 Create WorkItem 动作

当前已有“创建 WorkItem”入口，但更像手工临时创建。

P1 建议调整为：

1. 默认从 summary 预填标题/正文草稿
2. 如果没有 summary，按钮给出明确提示
3. 创建成功后提示“已从当前 Thread 收敛为 WorkItem”

### 8.3 来源可见性

建议在两个页面都展示来源关系：

1. Thread 详情页
   - 显示“已收敛出的 WorkItems”

2. WorkItem 详情页
   - 显示“来源 Thread”
   - 若来自 summary，明确标记来源类型

## 9. 数据建议

### 9.1 第一阶段不改表的方案

P1 推荐先走不改表路线：

- `Thread.summary` 存文本
- `Thread.metadata.summary_draft` 存结构化草稿
- `WorkItem.metadata.source_thread_id` 记录来源
- `thread_work_item_links` 继续作为关系真相

优点：

- 改动小
- 兼容现有代码
- 足够支撑单 Thread 收敛

### 9.2 如果必须加字段

如果后续评审认为 `metadata` 不够稳，建议优先考虑：

1. 给 `threads` 增加 `summary_updated_at`
2. 给 `issues` 增加 `source_thread_id`

但这不应作为 P1 前置条件。

## 10. 验收标准

P1 完成后，至少要满足以下验收标准：

1. 用户能在 Thread 页面看到并编辑 summary
2. 用户能基于 summary 创建 WorkItem
3. 创建出的 WorkItem 自动与 Thread 建立显式关联
4. WorkItem 能显示其来源 Thread
5. 整个链路不需要引入新的一级对象或并行控制面

## 11. 分步实施建议

建议按三个很小的步骤做：

### P1.1 Summary 变成正式动作

实现：

- Thread 页面支持编辑与保存 summary
- 明确 summary 是收敛入口，而不是附属说明

### P1.2 Create WorkItem 与 summary 打通

实现：

- 从 Thread 创建 WorkItem 时优先使用 summary
- 创建成功后记录来源 metadata

### P1.3 来源关系展示闭环

实现：

- Thread 侧能看到收敛出的 WorkItems
- WorkItem 侧能看到来源 Thread

完成这三步，P1 就已经成立。

## 12. 不要前置的内容

P1 阶段不建议前置以下内容：

1. 多小组协同
2. 主 Thread / 子 Thread
3. 完整批量 WorkItem 创建
4. 完整 Assignment / 多 assignee 状态机
5. coordinator-only 跨组治理
6. 新的 team/org 控制面

这些都会显著增加复杂度，但对“单 Thread 收敛到 WorkItem”这个 MVP 不是必要条件。

## 13. 结论

P1 的本质不是“把 Thread 做得更像聊天工具”，而是：

- 让 Thread 成为正式的协同收敛入口
- 让 summary 成为讨论和执行之间的桥
- 让 WorkItem 成为讨论结果的正式执行载体

因此 P1 的主线应收敛为：

`Thread 消息流` -> `Thread.summary` -> `WorkItem` -> `ThreadWorkItemLink`

只要这个链路稳定，后面的多小组协同、DAG 自动生成、责任模型增强才有坚实基础。

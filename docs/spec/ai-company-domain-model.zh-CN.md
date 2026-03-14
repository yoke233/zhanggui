# AI Company Domain Model（最小落地版）

> 状态：草案
> 最后按代码核对：2026-03-14

> 目标：从当前仓库已经稳定运行的 `project / work item / action / run / deliverable / chat session / thread` 主线出发，逐步扩展为可覆盖产品、运营、财务、剪辑、销售、管理等场景的通用 AI Company 系统。

## 阅读方式

- 本文主要是领域建模与演进方向讨论，不是现行 API 契约文档。
- 当前代码已经有一部分 Thread 能力和 Thread <-> WorkItem 关联能力，但远未达到本文后半段定义的完整四层模型。
- 当前前后端对外主入口都已经使用 `/work-items`；但持久化表名与部分兼容代码仍保留 `issues` 旧命名，不要把文中的通用 `work_items` 主表/API 设计直接等同于现状。

## 当前实现对照

- 已稳定存在：`Project -> WorkItem -> Action -> Run -> Deliverable` 主链。
- 已落地基础 Thread：独立 `Thread` 实体、消息、参与者、agent 邀请，以及 Thread 与 WorkItem/Issue 的链接关系。
- 仍并行存在：`ChatSession` 继续作为 direct chat 主线，和 Thread 不是同一个对象。
- 尚未落地：本文定义的通用 `work_items` 主表、`decisions` 结构化治理层、完整的通用 `executions` / `artifacts` 新 API 面。

## 当前系统现状

在谈通用模型之前，必须先承认当前仓库已经稳定的核心不是抽象的 `work_item`，而是下面这几条主线：

### 1. Project 是组织容器，不是业务对象

当前 `Project` 的定位很清楚：

- 用于组织内容
- 不直接存 repo/workspace
- repo/workspace 通过 `ResourceBinding` 绑定

这意味着系统已经天然支持两类项目：

- `dev` 项目
- `general` 项目

所以“通用 AI Company”并不是从零开始，当前主线已经有一个跨工程/非工程的组织容器。

### 2. WorkItem 是当前最稳定的工作对象

当前系统里，`WorkItem` 已经是对外主语义；持久化层和少量兼容代码仍保留 `issue` 命名。它不是 GitHub issue 的薄壳，而是已经吸收了执行语义的统一工作单元：

- 有标题、正文、优先级、标签
- 有状态流转
- 可绑定资源
- 可声明依赖
- 可挂 steps

因此，如果要从当前系统出发，第一判断不是“先引入全新 WorkItem 替代一切”，而是：

**当前 `WorkItem` 已经是系统里最接近通用 work item 的对象。**

### 3. Action / Run / Deliverable 已经构成执行骨架

当前执行主线已经完整存在：

```text
WorkItem
  -> Action[]
  -> Run[]
  -> Deliverable[]
```

其中：

- `Action` 是编排节点
- `Run` 是一次尝试
- `Deliverable` / `Artifact` 是输出结果

这意味着系统已经有“动作”和“产物”的稳定容器，不应该在第一阶段推翻重建。

### 4. Chat 当前仍是独立于 WorkItem 的 direct chat 主线，但已具备结晶入口

当前前端和 API 已经把 direct chat 与执行工作台拆成两套入口：

- 前端：`/chat` 与 `/work-items`（旧 `/issues` / `/flows` 仅页面 redirect）
- 后端：`/api/chat/*` 与 `/api/work-items/*`

而且当前 `chat session` 的定位明显更接近：

- 一对一 lead 会话
- 持久化消息历史
- 带 project / profile / driver / workdir 上下文

它本身还不是多人协作 Thread，但当前代码已经提供：

- `POST /api/chat/sessions/{sessionID}/crystallize-thread`
- 可把 `ChatSession` 固化为独立 `Thread`
- 并可选同时创建 `WorkItem`

### 5. 当前最大缺口不是“没有通用对象”，而是“讨论层和执行层还没接上”

所以如果从现状出发，真正的问题不是：

- 先不先做 PR 抽象
- 先不先做巨大的通用业务模型

而是：

**当前 `ChatSession -> Thread -> WorkItem` 已经出现最小闭环，但多人讨论、结构化决策、治理层仍未真正接上。**

这才是第一步最该补的地方。

## 从当前系统出发的结论

### 1. 现在不要先上新的 `work_items` 主表

如果当前就新增一张全新的 `work_items` 表，并试图让 `issue` 退化成它的一个 type：

- 后端存储层会同时维护两套工作对象
- 前端 `/work-items` 路由与列表会失去稳定中心
- 调度、steps、executions、artifacts 都要重新找归属

这一步太早。

当前更稳的做法是：

- **保留 `WorkItem` 作为当前工作对象主线**
- 同时接受持久化层仍有 `issues` / `steps` / `executions` 等兼容命名
- 等 thread 这一层接好后，再决定是否真的升格出 `work_items`

### 2. 现在最应该补的是 Thread，而不是先补 Work Item

因为当前系统已经有工作对象，但缺讨论容器。

所以从现状出发的第一阶段应该是：

- 保持 `ChatSession` 作为 1:1 direct chat
- 通过 `crystallize-thread` 把需要沉淀的 direct chat 固化成 `Thread`
- 让 `WorkItem` 能拥有一个或多个讨论
- 让讨论可以先存在，再补关联

这一步补上以后，系统结构会从：

```text
chat session -> thread -> work item -> action -> run -> deliverable
   (1:1)        (共享讨论)        (执行主线)
```

变成：

```text
discussion(chat/thread) <-> work item -> action -> run -> deliverable
```

这才是当前系统最自然的第一刀。

### 3. 当前 Artifact 已经足够通用，不要重复造“交付物”

当前 `Artifact` 本身已经是比较通用的输出容器：

- 有 `result_markdown`
- 有 `metadata`
- 有 `assets`

所以从现状出发，不应该先新增另一套“通用 deliverable”模型来替代它。

更合理的是：

- 保留 `Artifact`
- 以后再逐步扩展 `Artifact` 的类型语义
- 让 `PR`、文档、报告、视频等逐渐都通过 artifact 视角表达

## 核心原则

### 1. Thread 是统一交互层，不是业务真相源

所有人和 AI 都可以在 `thread` 里：

- 讨论需求
- 开会
- 追问补充
- 汇总上下文
- 拉更多参与者进来

但 `thread` 只负责承载讨论时间线，不直接代表：

- 这件事的正式业务类型
- 当前状态
- 负责人
- 交付边界

### 2. Issue 是当前 work item，Work Item 是后续抽象目标

系统的业务中心不再是 `PR`，但在当前阶段也不应急于立刻引入一套替代 `Issue` 的全新中心对象。

更准确的说法是：

- 当前：`Issue` 是系统里最稳定的工作对象
- 后续：如果非工程场景持续扩张，再把 `Issue` 上提/泛化为更中性的 `WorkItem`

长期目标中的 `WorkItem` 可以覆盖：

- 一条产品需求
- 一次运营活动
- 一个付款申请
- 一条剪辑需求
- 一个客户跟进项
- 一个管理层决策请求

### 3. Decision 和 Execution 必须结构化

如果所有结论和动作都只留在 thread 自然语言里，AI 很快会失去判定能力：

- 哪个方案最终被采用
- 哪项任务已经开始执行
- 哪个结果已经交付

因此需要把“讨论”与“决定/执行”拆开。

### 4. Artifact 是交付物统一层

`PR` 只是交付物的一种，不应再作为整个系统的中心语义。

统一的交付物可以包括：

- `pr`
- `doc`
- `report`
- `invoice_pdf`
- `slide`
- `video_cut`
- `contract`

## 四层模型

### Layer 1: Interaction

统一交互层，负责聊天、会议、时间线与参与者。

```text
threads
messages
thread_members
thread_links
```

### Layer 2: Business

统一业务层，负责“这件事是什么”。

```text
work_items
work_item_links
```

### Layer 3: Governance

治理层，负责“做了什么决定、派发了什么动作”。

```text
decisions
executions
```

### Layer 4: Delivery

交付层，负责沉淀结果与产物。

```text
artifacts
```

## 核心关系

```text
Thread
  ├── Message[]
  ├── Member[]
  └── Link[] -> WorkItem / Decision / Artifact

WorkItem
  ├── Decision[]
  ├── Execution[]
  ├── Artifact[]
  ├── ThreadLink[]
  └── WorkItemLink[]

Decision
  ├── derived_from Thread
  └── belongs_to WorkItem

Execution
  ├── belongs_to WorkItem
  ├── optionally derived_from Decision
  └── may produce Artifact[]
```

## 最小对象定义

### Thread

`thread` 是独立讨论容器，可先创建，后关联业务对象。

建议字段：

```text
threads
- id
- title
- status              // active | closed | archived
- visibility          // private | team | org | external_shared
- source_type         // manual | inbox | meeting | imported
- owner_id
- summary
- metadata_json
- created_at
- updated_at
```

```text
messages
- id
- thread_id
- sender_type         // user | agent | external
- sender_id
- content
- content_type        // text | markdown | file | system
- reply_to_message_id
- metadata_json
- created_at
```

```text
thread_members
- id
- thread_id
- kind                // human | agent
- user_id
- agent_profile_id
- role                // owner | member | observer
- status
- agent_data_json
- joined_at
- last_active_at
```

```text
thread_links
- id
- thread_id
- target_type         // work_item | decision | artifact
- target_id
- relation_type       // primary_context | about | review_for | output_of
- is_primary
- created_at
- created_by
```

### Work Item（后续抽象目标）

`work_item` 是业务对象统一抽象，但它不是当前系统第一阶段必须落地的新主表。

从现状出发，更合理的理解是：

- 当前 `Issue` 先承担 work item 角色
- `work_item` 是未来跨部门统一建模时的抽象方向
- 是否真的单独建表，应当在 thread 与 issue 接通后再决定

建议字段：

```text
work_items
- id
- type                // engineering_task | requirement | campaign | invoice | clip_request | customer_case | objective
- domain              // engineering | product | ops | finance | content | sales | exec
- title
- summary
- status
- priority
- owner_id
- assignee_id
- due_at
- primary_thread_id   // 可空，便于 UI 默认挂载主讨论
- schema_version
- fields_json         // 各领域差异字段
- created_at
- updated_at
- archived_at
```

```text
work_item_links
- id
- source_work_item_id
- target_work_item_id
- relation_type       // depends_on | blocks | child_of | relates_to | duplicates
- is_primary
- created_at
```

### Decision

`decision` 用来把 thread 中的重要结论抽成结构化真相。

```text
decisions
- id
- work_item_id
- thread_id
- title
- decision_type       // approve | reject | choose_option | freeze_scope | assign_owner
- summary
- payload_json
- status              // proposed | confirmed | superseded | cancelled
- made_by_type        // user | agent
- made_by_id
- made_at
- created_at
```

### Execution

`execution` 是对 work item 的一次具体推进动作，可以由 AI 或人执行。

```text
executions
- id
- work_item_id
- decision_id         // 可空
- execution_type      // implement | write_doc | prepare_invoice | edit_video | call_customer | analyze_report
- status              // pending | running | blocked | done | failed | cancelled
- assigned_actor_type // user | agent
- assigned_actor_id
- input_json
- output_json
- started_at
- finished_at
- created_at
```

### Artifact

`artifact` 是统一交付物，`PR` 只是其中一种。

```text
artifacts
- id
- work_item_id
- execution_id        // 可空
- type                // pr | branch | doc | invoice_pdf | report | slide | video_cut | contract
- title
- url
- status              // draft | active | approved | delivered | archived
- metadata_json
- created_by_type
- created_by_id
- created_at
```

## Work Item 类型示例

### engineering_task

适配当前主线：

- 继续保留 repo / worktree / step / gate / PR 流程
- `artifact.type = pr`
- `execution_type = implement`

### requirement

产品需求：

- 状态示例：`draft -> refining -> approved -> planned -> delivering -> done`
- artifact 可为 `doc`、`slide`

### campaign

运营活动：

- 状态示例：`draft -> preparing -> running -> reviewing -> done`
- artifact 可为 `report`、`asset_bundle`

### invoice

财务单据：

- 状态示例：`draft -> pending_review -> approved -> paid -> archived`
- artifact 可为 `invoice_pdf`

### clip_request

剪辑需求：

- 状态示例：`draft -> editing -> review -> revision -> approved -> delivered`
- artifact 可为 `video_cut`

### customer_case

客户事项：

- 状态示例：`new -> active -> waiting_customer -> resolved -> closed`
- artifact 可为 `report`、`contract`

## API 草案

> 以下为目标 API 轮廓，不代表当前代码已经全部提供。
> 当前现状是：后端主工作对象接口已经使用 `/api/work-items`；Thread 也已有一组 `/api/threads/*` 接口，但与下述草案并不完全一致。

### Thread API

```text
POST   /api/threads
GET    /api/threads
GET    /api/threads/{id}
POST   /api/threads/{id}/messages
POST   /api/threads/{id}/participants
POST   /api/threads/{id}/links
POST   /api/threads/{id}/close
```

当前代码更接近：

```text
POST    /api/threads
GET     /api/threads
GET     /api/threads/{id}
PUT     /api/threads/{id}
DELETE  /api/threads/{id}
POST    /api/threads/{id}/messages
POST    /api/threads/{id}/participants
POST    /api/threads/{id}/agents
POST    /api/threads/{id}/create-work-item
POST    /api/threads/{id}/links/work-items
GET     /api/threads/{id}/work-items
GET     /api/work-items/{issueID}/threads
```

### Work Item API

```text
POST   /api/work-items
GET    /api/work-items
GET    /api/work-items/{id}
PATCH  /api/work-items/{id}
POST   /api/work-items/{id}/links
POST   /api/work-items/{id}/decisions
POST   /api/work-items/{id}/executions
POST   /api/work-items/{id}/artifacts
```

当前代码已经提供这组 `/api/work-items/*` REST，但 `Decision` / 通用治理层 API 仍未落地。

### Decision / Execution API

```text
PATCH  /api/decisions/{id}
PATCH  /api/executions/{id}
POST   /api/executions/{id}/artifacts
```

## UI 信息架构

### 1. Thread 视图

适合：

- 跟 AI 讨论
- 开会
- 同步上下文
- 提炼决策

关键区块：

- 时间线
- 参与者列表
- 关联对象侧栏
- “提炼为 Work Item / Decision / Artifact” 操作区

### 2. Work Item 视图

适合：

- 看这件事当前状态
- 看负责人
- 看相关 thread
- 看已做决策
- 看交付物

关键区块：

- 基本信息
- 状态流转
- 主 thread
- 关联 thread 列表
- decisions
- executions
- artifacts

### 3. Inbox / Role 视图

适合：

- 查看轮到谁处理
- 查看各角色待办
- 查看 AI/Human 协作切换点

## 与当前仓库的演进路径

### Phase 0: 先承认当前主线，不推翻 Issue

当前真正稳定的主线是：

```text
Project
  -> Issue
  -> Step
  -> Execution
  -> Artifact

Chat Session
```

所以第一原则是：

- 不推翻现有 `Issue`
- 不重写 scheduler / step / execution 主链
- 不先上新的 `work_items` 主表

### Phase 1: 新增独立 Thread，与 ChatSession 并列

目标：

- 新增独立 `Thread` 领域实体，用于多 AI + 多 human 的共享讨论
- `ChatSession` 保留为 1 AI + 1 human 的 direct chat，不做 Thread 的别名或前身
- `Thread` 具备独立存储（threads、thread_messages、thread_members）
- `Thread` 具备独立 API（`/threads`）与 WebSocket 协议（`thread.send`）
- `Thread` 可先独立存在，后关联 WorkItem
- `ChatSession` 可在需要沉淀时通过 `crystallize-thread` 固化为 `Thread`

此阶段：

- 当前 `WorkItem` 继续是工作对象真相源
- 当前 `ChatSession` 保持现有 API、存储与 runtime 模型不变
- 当前 PR bootstrap / gate 流程完全不变
- `/chat` 与 `/threads` 是两个并列入口，产品边界明确

说明：其中前两条在当前代码里已经基本成立；但后续章节中的 `WorkItem`、`Decision`、治理层 API 仍大多停留在目标设计。

### Phase 2: 让 Thread 与 WorkItem 接通

目标：

- 建立 `Thread <-> WorkItem` 的显式关联层（`thread_work_item_links`）
- 支持一个 WorkItem 关联多个 Thread
- 支持从 Thread 创建或挂载 WorkItem
- 在 WorkItem Detail 中展示关联 Thread
- 在 Thread 页面展示关联 WorkItem

此阶段：

- Thread 成为讨论层与执行层之间的连接器
- ChatSession 的 direct chat 行为继续独立运行

### Phase 3: 非工程场景扩张后，再评估 Work Item 抽象

目标：

- 当产品、运营、财务、剪辑等都开始稳定使用时
- 再决定是否把 `Issue` 上提为统一 `WorkItem`
- 或者让 `Issue` 继续作为主工作对象，只是扩 domain/type

此阶段：

- 是“是否重命名/重抽象”的决策点，不是第一阶段动作

### Phase 4: 决策与交付物进一步结构化

目标：

- 新增 `decisions`
- 继续扩展 `artifact` 语义
- 让 thread 中的重要结论能落结构化记录

## 边界与约束

### 1. 不做万能对象大爆炸

第一版不要直接引入无限泛化的 `object_links(source_type, target_type, ...)` 作为中心设计。

更稳的顺序是：

- 先做 `thread_links`
- 再做 `work_item_links`
- 最后如果确有需要，再抽象统一关系层

### 2. 不让 Thread 取代 Work Item

禁止把以下真相只留在 thread 自然语言里：

- 当前负责人
- 正式状态
- 生效决策
- 已交付产物

### 3. 不让所有领域共用一套死状态机

应采用：

- 一组基础状态：`active / done / cancelled / archived`
- 各 `work_item.type` 自定义业务状态机

### 4. 不让 PR 继续绑架通用模型

工程域保留 PR 没问题，但 PR 只能是工程域的重要 artifact，不能继续做整个系统的总主语义。

### 5. 入口演进遵循实际产品语义

当前前端已有两个入口：`/chat`（1:1 direct chat）和 `/work-items`（执行工作台，旧 `/issues` / `/flows` 仅 redirect）。

演进后新增 `/threads` 作为多人共享讨论入口，三者并列：

- `/chat` — 1:1 direct chat（保留现有 ChatSession 语义）
- `/threads` — 多人/多 AI 共享讨论（独立 Thread 实体）
- `/work-items` — 执行工作台

Thread 与 WorkItem 通过关联层连接，ChatSession 继续独立运行。

## 一句话总结

如果目标是通用 AI Company 系统，那么中心模型应从：

```text
Chat Session + WorkItem
```

升级为：

```text
Chat Session (1:1 direct) + Thread (多人共享讨论) + WorkItem + Decision + Execution + Artifact
```

其中：

- `Chat Session` 保留为 1:1 direct chat
- `Thread` 负责多人/多 AI 共享讨论，独立于 ChatSession
- `WorkItem` 先负责当前业务真相；持久化层仍可保留 `issue` 兼容命名
- `Decision` 负责结构化结论
- `Execution` 负责推进动作
- `Artifact` 负责交付结果

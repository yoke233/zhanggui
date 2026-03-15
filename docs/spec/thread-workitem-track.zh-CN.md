# Thread 内任务孵化与 WorkItemTrack 设计

> **⚠️ DEPRECATED** — 本文档已废弃。WorkItemTrack 已被 ThreadTask 替代。
> 请参阅 `thread-task-dag.zh-CN.md`。
>
> 状态：已废弃
>
> 最后按代码核对：2026-03-15
>
> 相关现状：
> - `Thread` 已独立落地：REST、WebSocket、消息、参与者、agent 邀请、WorkItem 关联都已存在
> - `thread.send` 当前已支持 `mention_only / broadcast / auto` 路由模式
> - `WorkItem` 仍是现行执行主线，状态流转为 `open -> accepted -> queued -> running -> ...`
> - `Thread -> WorkItem` 的显式关联层 `thread_work_item_links` 已存在
> - `WorkItemTrack` 第一阶段已落地：表结构、REST 路由、thread-scoped WebSocket 事件、Thread 页面 Track 区、Track 过程消息 `work_item_track_id` 归属、`生成待办 / 生成并执行`
>
> 相关文档：
> - `thread-workitem-linking.zh-CN.md`
> - `thread-summary-workitem-mvp.zh-CN.md`
> - `thread-collaboration-to-dag-plan.zh-CN.md`

## 1. 目标

本文定义一个新的轻量过程对象 `WorkItemTrack`，用于承接：

1. 在 `Thread` 中经过多轮讨论后，手动开启“任务孵化”
2. 让 planner / reviewer 围绕某个明确目标持续推进
3. 在聊天时间线中展示强可视化、强操作的流程卡片
4. 支持一个 `Thread` 孵化多个 `WorkItem`
5. 支持多个 `Thread` 共同收敛到同一个 `WorkItem`
6. 在不推翻现有 `Thread` 与 `WorkItem` 主线的前提下，为“讨论 -> 任务”的中间阶段提供稳定真相源

本文不是在重新定义执行主模型。执行主模型仍然是 `WorkItem`。

## 2. 核心判断

### 2.1 为什么需要 `WorkItemTrack`

如果系统只支持：

- 一个 `Thread` 对应一个 `WorkItem`
- 一个 `WorkItem` 只来自一个 `Thread`

那么现有 `Thread + WorkItem + thread_work_item_links` 基本够用。

但当前已经明确存在以下需求：

1. 一个 `Thread` 可能孵化多个 `WorkItem`
2. 一个 `WorkItem` 可能由多个 `Thread` 共同收敛
3. 讨论期间并不总是立即创建正式 `WorkItem`
4. 聊天中需要出现“待确认执行”“重新规划”“送审”等结构化可操作卡片

仅靠：

- `Thread.metadata`
- `WorkItem.metadata`
- `thread_work_item_links`
- 自然语言消息本身

已经不足以稳定表达：

1. 当前这轮任务孵化处于哪个阶段
2. 这轮孵化关联了哪些 `Thread`
3. 这轮 planner / reviewer 的输出分别是什么
4. 当前应该在哪个 `Thread` 中显示主操作卡片
5. 某次操作究竟是针对哪一个候选任务进行的

因此，需要一个很窄的过程对象来承担“任务孵化中的状态真相”。

### 2.2 为什么不用 `WorkflowInstance`

`WorkflowInstance` 这个名字过重，像通用 BPM / 编排系统术语，不适合当前场景。

本设计统一采用：

- 代码实体名：`WorkItemTrack`
- 产品文案名：`任务孵化`

它表达的是：

- 这是一条围绕某个候选任务持续推进的轨道
- 它最终可以落到 `WorkItem`
- 它不是比 `WorkItem` 更高一级的业务对象

## 3. 范围与非目标

## 3.1 本文范围

本文只处理：

1. 直接在 `Thread` 中发起任务孵化
2. `Thread -> WorkItemTrack -> WorkItem` 的主链
3. 聊天时间线中的流程卡片
4. planner / reviewer / 用户确认的结构化流转

## 3.2 明确不在本文范围内

以下内容暂不处理：

1. `ChatSession -> Thread` 的结晶入口细节
2. 通用 `Decision` / `Proposal` 新对象体系
3. 多 reviewer 投票
4. 一个 `WorkItemTrack` 一次性拆成多个 `WorkItem` 的正式协议
5. 任意组织树、团队树、审批树

说明：

- “一个 Thread 孵化多个 WorkItem”通过多个 `WorkItemTrack` 解决
- “多个 Thread 收敛到一个 WorkItem”通过一个 `WorkItemTrack` 关联多个 `Thread` 解决
- “一个 Track 后续拆成多个 WorkItem”不是第一期需求，如果未来需要，再引入子 Track 或显式拆分关系

## 4. 领域边界

### 4.1 Thread

`Thread` 继续是讨论真相源，负责：

- 人与 agent 的消息流
- 最近上下文
- 共享讨论空间
- 显示流程卡片和过程消息

### 4.2 WorkItemTrack

`WorkItemTrack` 是任务孵化真相源，负责：

- 标识“当前正在孵化哪一个候选任务”
- 保存该候选任务的规划、审核、确认状态
- 聚合多个 `Thread` 的输入来源
- 记录该轮产出的候选 `WorkItem`

### 4.3 WorkItem

`WorkItem` 继续是正式任务真相源，负责：

- 进入待办池
- 进入调度与执行
- 挂载 Actions / Runs / Deliverables

## 5. 数据模型

## 5.1 主表：`work_item_tracks`

建议新增表：

```sql
CREATE TABLE work_item_tracks (
    id                           INTEGER PRIMARY KEY AUTOINCREMENT,
    title                        TEXT NOT NULL,
    objective                    TEXT NOT NULL DEFAULT '',
    status                       TEXT NOT NULL,
    primary_thread_id            INTEGER,
    work_item_id                 INTEGER,
    planner_status               TEXT NOT NULL DEFAULT 'idle',
    reviewer_status              TEXT NOT NULL DEFAULT 'idle',
    awaiting_user_confirmation   BOOLEAN NOT NULL DEFAULT FALSE,
    latest_summary               TEXT NOT NULL DEFAULT '',
    planner_output_json          TEXT NOT NULL DEFAULT '{}',
    review_output_json           TEXT NOT NULL DEFAULT '{}',
    metadata_json                TEXT NOT NULL DEFAULT '{}',
    created_by                   TEXT NOT NULL DEFAULT '',
    created_at                   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at                   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
```

### 5.1.1 字段说明

| 字段 | 类型 | 说明 |
|------|------|------|
| `id` | int64 | Track 主键 |
| `title` | string | 当前候选任务标题 |
| `objective` | string | 当前孵化目标的简要描述 |
| `status` | string | 当前孵化阶段 |
| `primary_thread_id` | *int64 | 主操作 Thread |
| `work_item_id` | *int64 | 最终落地到的正式 WorkItem |
| `planner_status` | string | planner 子状态 |
| `reviewer_status` | string | reviewer 子状态 |
| `awaiting_user_confirmation` | bool | 是否正在等待用户确认 |
| `latest_summary` | string | 当前最新结构化摘要的文本版 |
| `planner_output_json` | json-string | planner 输出 |
| `review_output_json` | json-string | reviewer 输出 |
| `metadata_json` | json-string | 扩展字段 |
| `created_by` | string | 谁发起了该 Track |

## 5.2 关联表：`work_item_track_threads`

```sql
CREATE TABLE work_item_track_threads (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    track_id        INTEGER NOT NULL,
    thread_id       INTEGER NOT NULL,
    relation_type   TEXT NOT NULL DEFAULT 'source',
    created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(track_id, thread_id)
);
```

### 5.2.1 `relation_type`

建议第一期只支持：

- `primary`
  主操作 Thread，默认卡片显示和按钮操作都以这里为主
- `source`
  该 Thread 提供了输入讨论
- `context`
  仅作为补充上下文挂入

说明：

- 一个 Track 可以关联多个 Thread
- 一个 Thread 也可以关联多个 Track
- `primary_thread_id` 是主表上的快捷字段，用于高频查询与 UI 聚焦

## 5.3 与 `WorkItem` 的关系

第一期建议：

- 一个 `WorkItemTrack` 最终至多关联一个正式 `WorkItem`
- 一个 `WorkItem` 可对应多轮 `WorkItemTrack`

因此：

- `work_item_tracks.work_item_id` 允许为空
- 当进入“生成待办”或“生成并执行”阶段时，再回填 `work_item_id`

## 5.4 与 `ThreadMessage` 的关系

第一期不新增专门的 `track_messages` 表。

直接在 `thread_messages.metadata` 中补一个可选字段：

```json
{
  "work_item_track_id": 42
}
```

作用：

1. planner / reviewer 发出的过程消息能够归属于具体 Track
2. 用户可以针对某个 Track 继续讨论
3. 前端可以按 Track 高亮和折叠消息

这意味着：

- Thread 仍是一条时间线
- 但消息可选携带 Track 归属

## 6. 状态机

## 6.1 Track 主状态

建议主状态如下：

- `draft`
- `planning`
- `reviewing`
- `awaiting_confirmation`
- `materialized`
- `executing`
- `done`
- `paused`
- `cancelled`
- `failed`

## 6.2 状态语义

### `draft`

- Track 已创建
- 但还未开始分析

### `planning`

- planner 正在收敛需求
- 允许追加更多 Thread 上下文

### `reviewing`

- reviewer 正在审核 planner 输出

### `awaiting_confirmation`

- planner 与 reviewer 已完成一轮
- 当前在等待用户决定：
  - 重新规划
  - 仅生成待办
  - 生成并执行

### `materialized`

- 已生成或绑定正式 `WorkItem`
- 但不一定已经进入执行

### `executing`

- 关联的 `WorkItem` 已进入 `queued / running`

### `done`

- 该轮孵化流程已结束

### `paused`

- 由用户手动暂停

### `cancelled`

- 该轮孵化被用户放弃

### `failed`

- 该轮孵化因 agent 或系统问题失败

## 6.3 合法流转

建议第一期约束：

```text
draft -> planning
planning -> reviewing
planning -> paused
planning -> cancelled
planning -> failed

reviewing -> awaiting_confirmation
reviewing -> planning
reviewing -> paused
reviewing -> cancelled
reviewing -> failed

awaiting_confirmation -> planning
awaiting_confirmation -> materialized
awaiting_confirmation -> executing
awaiting_confirmation -> paused
awaiting_confirmation -> cancelled

materialized -> executing
materialized -> done
materialized -> paused

executing -> done
executing -> failed
executing -> paused

paused -> planning
paused -> reviewing
paused -> awaiting_confirmation
paused -> cancelled
```

## 7. 与 WorkItem 状态的映射

`WorkItemTrack` 与 `WorkItem` 状态不要求一一对应。

建议映射如下：

| Track 状态 | WorkItem 是否存在 | WorkItem 建议状态 |
|------------|-------------------|-------------------|
| `draft` | 否 | 无 |
| `planning` | 否 | 无 |
| `reviewing` | 否 | 无 |
| `awaiting_confirmation` | 否 | 无 |
| `materialized` | 是 | `accepted` |
| `executing` | 是 | `queued/running` |
| `done` | 是 | `done/closed` |
| `failed` | 可选 | `failed` 或无 |

关键原则：

1. 在 reviewer 通过前，不强制生成正式 `WorkItem`
2. 用户确认“生成待办”后，再创建 `WorkItem`
3. 用户确认“生成并执行”后，创建 `WorkItem` 并立刻进入 `queued`

## 8. 用户流程

## 8.1 单 Thread 孵化单任务

1. 用户在 Thread 内讨论
2. 用户点击 `开始孵化任务`
3. 系统创建一个 `WorkItemTrack`
4. planner 进入 `planning`
5. reviewer 进入 `reviewing`
6. 系统在 Thread 时间线中展示 Track 卡片
7. reviewer 通过后，Track 进入 `awaiting_confirmation`
8. 用户点击：
   - `生成待办`
   - 或 `生成并执行`
9. 系统创建正式 `WorkItem`
10. 建立 `Thread <-> WorkItem` 显式关联

## 8.2 单 Thread 孵化多个任务

1. Thread 中围绕多个子主题讨论
2. 用户对每个子主题分别点击“开始孵化任务”
3. 同一 Thread 下创建多个 `WorkItemTrack`
4. 时间线中出现多个卡片
5. 每个 Track 各自走 planning / reviewing / confirmation
6. 最终可分别落到多个 `WorkItem`

## 8.3 多 Thread 收敛到一个任务

1. 主 Thread 中出现一个明确目标
2. 用户创建 `WorkItemTrack`
3. 用户将其他 Thread 追加为该 Track 的 `source/context`
4. planner / reviewer 在收敛时读取多个 Thread 的 summary 与最近消息
5. 最终该 Track 只生成一个正式 `WorkItem`

## 9. 命令模型

## 9.1 为什么需要结构化命令

不建议让用户通过自然语言输入：

- “同意执行”
- “打回”
- “重新规划”

来驱动状态流转。

建议使用结构化命令，因为：

1. 后端更容易校验
2. 前端更容易复现
3. 审计更清晰
4. 不会与普通讨论文本混淆

## 9.2 当前实现与后续命令草案

第一阶段实际落地采用的是 REST 命令入口：

- `POST /threads/{threadID}/tracks`
- `POST /tracks/{trackID}/threads`
- `POST /tracks/{trackID}/submit-review`
- `POST /tracks/{trackID}/approve-review`
- `POST /tracks/{trackID}/reject-review`
- `POST /tracks/{trackID}/materialize`
- `POST /tracks/{trackID}/confirm-run`
- `POST /tracks/{trackID}/pause`
- `POST /tracks/{trackID}/cancel`

统一 `thread.command` WebSocket 命令仍可作为后续收口方向，草案如下：

```json
{
  "type": "thread.command",
  "data": {
    "thread_id": 123,
    "track_id": 42,
    "command": "confirm_execution",
    "payload": {}
  }
}
```

### 第一期建议支持的命令

- `start_track`
- `attach_thread_context`
- `restart_planning`
- `submit_for_review`
- `approve_review`
- `reject_review`
- `materialize_work_item`
- `confirm_execution`
- `pause_track`
- `cancel_track`

## 9.3 命令校验原则

每个命令至少校验：

1. `thread_id` 是否属于该 Track
2. 当前状态是否允许该命令
3. 目标 `WorkItem` 是否已存在
4. 如果存在 `WorkItem`，其状态是否与 Track 兼容

## 10. 事件模型

建议新增以下 thread-scoped 事件：

- `thread.track.created`
- `thread.track.updated`
- `thread.track.state_changed`
- `thread.track.planning_started`
- `thread.track.planning_completed`
- `thread.track.review_started`
- `thread.track.review_approved`
- `thread.track.review_rejected`
- `thread.track.materialized`
- `thread.track.run_confirmed`

这些事件应继续走现有 thread WebSocket 订阅能力，以便：

1. 当前 `ThreadDetailPage` 可直接消费
2. Timeline 中可插入卡片或局部刷新
3. 不必并行引入新的独立实时通道

## 11. 聊天卡片设计

## 11.1 卡片定位

卡片不是普通消息泡泡，也不是临时弹窗。

它是：

- `WorkItemTrack` 在 Thread 时间线中的 UI 投影

每张卡片必须绑定：

- `track_id`
- `primary_thread_id`
- 可选 `work_item_id`

## 11.2 卡片内容

建议卡片至少展示：

1. 标题
2. 当前阶段
3. 关联的 Thread 数量
4. 关联的 WorkItem
5. planner 摘要
6. reviewer 结论
7. 步骤预览
8. 风险与待确认事项
9. 当前负责人
10. 更新时间

## 11.3 卡片操作

建议第一期支持：

- `开始孵化`
- `追加当前 Thread`
- `重新规划`
- `送审`
- `审核通过`
- `打回修改`
- `生成待办`
- `生成并执行`
- `暂停`
- `取消`

## 11.4 卡片显示策略

建议同时有两层 UI：

### 时间线卡片

放在 Thread 消息流里，和过程消息交织展示。

作用：

- 保留“这件事就是在聊天里推进”的感受

### 顶部 Track 导航条

放在 `ThreadDetailPage` 顶部或侧栏，列出当前 Thread 关联的所有 Track。

作用：

- 当一个 Thread 有多个 Track 时，方便快速切换与聚焦

## 12. planner / reviewer 的消息策略

第一阶段已通过 `Thread` 内的过程消息落地 Track 归属。相关系统过程消息会附带：

```json
{
  "work_item_track_id": 42
}
```

作用：

1. 表示该消息属于哪个 Track
2. 前端可以直接在时间线中显示 Track 归属
3. 后端可以把该消息纳入该 Track 的过程审计

这意味着：

- 过程消息仍在 Thread 内
- 但过程状态不再依赖自然语言本身

## 13. 与现有能力的兼容策略

## 13.1 与 `thread_work_item_links` 的兼容

`thread_work_item_links` 继续保留，不被替代。

它表示：

- Thread 与正式 `WorkItem` 之间的显式关系

而 `WorkItemTrack` 表示：

- 任务孵化中的过程关系

两者职责不同。

## 13.2 与 `Thread.summary` 的兼容

`Thread.summary` 继续存在，用于：

- 每个 Thread 自己的讨论收敛
- planner 在跨 Thread 收敛时优先读取 summary，而不是全文消息

`WorkItemTrack.latest_summary` 表示的是：

- 该 Track 自己收敛后的当前视图

两者不要混为一谈。

## 13.3 与现有 `Thread -> CreateWorkItem` 的兼容

现有：

- `POST /threads/{id}/create-work-item`

继续保留，作为：

- 最短人工路径

而 `WorkItemTrack` 路径是：

- 更完整的多轮任务孵化路径

两者可以并存。

## 14. 第一阶段实现建议

为了避免范围失控，建议第一阶段只做：

1. 新增 `WorkItemTrack` 表与 `WorkItemTrackThread` 表
2. Thread 页面支持查看多个 Track
3. 支持从 Thread 发起 `start_track`
4. 支持 planner / reviewer 过程消息带 `work_item_track_id`
5. 支持 reviewer 通过后进入 `awaiting_confirmation`
6. 支持用户点击 `生成待办` 或 `生成并执行`
7. 生成正式 `WorkItem` 后建立 `thread_work_item_links`

第一阶段不做：

1. 多 reviewer 协同
2. Track 版本树
3. 一个 Track 同时产出多个 WorkItem
4. Track 之间依赖关系
5. 自动跨 Thread 聚类

当前代码已完成以上第一阶段 1-7 项；后续若继续推进，重点是统一命令入口、真正的 planner / reviewer 专属消息生产，以及更强的时间线聚合交互。

## 15. 为什么单文件足够

当前阶段，这个方案还处于“定义主边界”的时期。

最重要的是先把以下问题说清楚：

1. 为什么要有 `WorkItemTrack`
2. 它与 `Thread`、`WorkItem` 的职责边界是什么
3. 它的状态机是什么
4. 聊天卡片与结构化命令如何挂接

这些内容都强相关，拆成多文件反而会让边界再次模糊。

因此当前阶段建议：

- 只保留本文一个主设计文件

等后续真正进入实现阶段，再视需要拆出：

- API 细化
- 前端卡片规格
- Store / migration 细节

## 16. 最终结论

本设计采用以下稳定结论：

1. `Thread` 继续是讨论容器
2. `WorkItem` 继续是执行主对象
3. `WorkItemTrack` 是新增的轻量任务孵化对象
4. 一个 `Thread` 可以关联多个 `WorkItemTrack`
5. 一个 `WorkItemTrack` 可以关联多个 `Thread`
6. 一个 `WorkItemTrack` 最终可落到一个正式 `WorkItem`
7. 聊天中的强可视化卡片只是 `WorkItemTrack` 的 UI 投影，不是新的主对象

一句话总结：

`Thread` 负责讨论，`WorkItemTrack` 负责孵化，`WorkItem` 负责执行。

# 2026-03-15 Thread / WorkItemTrack 实施计划

## 1. 目标

基于 [thread-workitem-track.zh-CN.md](D:/project/ai-workflow/docs/spec/thread-workitem-track.zh-CN.md) 的设计，分阶段落地 `WorkItemTrack`，让系统支持：

- 在 `Thread` 中手动开启任务孵化
- 一个 `Thread` 孵化多个 `WorkItem`
- 多个 `Thread` 共同收敛到一个 `WorkItem`
- 在 Thread 时间线中展示任务孵化卡片
- 用户通过结构化命令推进 planning / review / confirm / materialize / execute

## 2. 实施原则

1. 不推翻现有 `Thread`、`WorkItem`、`thread_work_item_links`
2. `WorkItemTrack` 只负责过程，不替代正式 `WorkItem`
3. 第一阶段优先把数据模型、状态流转、基础 UI 和结构化命令打通
4. 第一阶段不做复杂自动化，不做多 reviewer 投票，不做 Track 版本树

## 3. Phase 1：领域模型与存储

目标：

- 让后端先具备 `WorkItemTrack` 的稳定持久化和基本读写能力

实施项：

1. 新增领域模型
   - `internal/core/work_item_track.go`
   - `WorkItemTrack`
   - `WorkItemTrackThread`

2. 新增状态定义与状态校验
   - Track 状态常量
   - Track 合法流转函数

3. 新增 Store 接口
   - 创建 Track
   - 读取 Track
   - 列表查询
   - 更新状态
   - 更新 planner / reviewer 输出
   - 关联 Thread
   - 按 Thread 查询 Tracks
   - 按 WorkItem 查询 Tracks

4. SQLite migration
   - 新增 `work_item_tracks`
   - 新增 `work_item_track_threads`

验收标准：

- 可以通过 store 持久化、读取、更新 `WorkItemTrack`
- 可以建立 Track 与多个 Thread 的关联
- Track 状态流转有统一校验

## 4. Phase 2：应用层与 API

目标：

- 让后端和前端协议层都可以从 `Thread` 发起、推进并实时感知 Track

实施项：

1. 新增应用服务
   - `internal/application/workitemtrackapp/`
   - 负责 `start_track`
   - 负责 `attach_thread_context`
   - 负责 `submit_for_review`
   - 负责 `approve_review`
   - 负责 `reject_review`
   - 负责 `materialize_work_item`
   - 负责 `confirm_execution`

2. REST API
   - `POST /threads/{threadID}/tracks`
   - `GET /threads/{threadID}/tracks`
   - `GET /tracks/{trackID}`
   - `POST /tracks/{trackID}/threads`
   - `POST /tracks/{trackID}/materialize`

3. WebSocket command
   - 新增 `thread.command`
   - 由统一 handler 处理针对 Track 的结构化命令

4. WebSocket events
   - `thread.track.created`
   - `thread.track.updated`
   - `thread.track.state_changed`
   - `thread.track.review_approved`
   - `thread.track.review_rejected`
   - `thread.track.materialized`

5. 实时协议接线
   - 在 `internal/core/event.go` 新增 Track 相关 `EventType`
   - 在 thread 事件过滤逻辑中把 `thread.track.*` 视为 thread-scoped event
   - 在 `web/src/types/ws.ts` 补充 `thread.command` 与 `thread.track.*` 的类型定义
   - 在 `ThreadDetailPage` 先接入 Track 事件订阅与局部状态刷新，不等到 Phase 5 再补

6. Thread 消息 metadata 契约
   - 提前把 `thread_messages.metadata.work_item_track_id` 定义为稳定字段
   - REST / WebSocket 返回 Thread 消息时保留该字段
   - 前端消息模型先识别该字段，为 Phase 4 的视觉高亮做准备

验收标准：

- 可以从 Thread 发起 Track
- 可以通过结构化命令推进 Track
- 前端可收到 Track 相关实时事件
- `work_item_track_id` 已成为稳定消息字段，后续过程消息可直接复用

## 5. Phase 3：与 WorkItem 的落地打通

目标：

- 让 Track 可以在确认后生成或绑定正式 `WorkItem`

实施项：

1. `materialize_work_item`
   - 从 Track 生成 `WorkItem`
   - 回填 `track.work_item_id`
   - 在同一事务内完成 `WorkItem` 创建 / 绑定、Track 回填、`thread_work_item_links` 建立
   - 任一步失败整体回滚，不留下“已创建 WorkItem 但 Track / Link 未完成”的半成功状态

2. 建立 `thread_work_item_links`
   - 为 Track 关联的相关 Thread 建立 WorkItem 显式链接
   - `primary_thread_id` 可优先建立 `is_primary=true`
   - 对已有 `(thread_id, work_item_id)` 链接做幂等处理，避免重复 materialize 时冲突

3. `confirm_execution`
   - 若 Track 已关联 `WorkItem`
   - 推进 `WorkItem` 到 `accepted -> queued`
   - Track 同步进入 `executing`
   - 若 Track 已是 `executing` 或 `WorkItem` 已在 `queued/running`，按幂等请求返回当前状态，不重复入队

4. Track 与 WorkItem 状态联动
   - WorkItem 完成后，Track 可进入 `done`
   - WorkItem 失败后，Track 可进入 `failed`

验收标准：

- 审核通过后可生成正式 `WorkItem`
- Thread 和 WorkItem 的链接能自动建立
- 用户点击确认后，WorkItem 能正常进入执行队列
- 重复调用 `materialize_work_item` / `confirm_execution` 不会产生重复 WorkItem、重复链接或重复入队

## 6. Phase 4：Thread 页面 UI

目标：

- 在现有 `ThreadDetailPage` 里把 Track 可视化出来

实施项：

1. 顶部 Track 导航条
   - 展示当前 Thread 关联的所有 Track
   - 可切换聚焦某个 Track

2. 时间线中的 Track 卡片
   - 每张卡片绑定 `track_id`
   - 展示当前阶段、planner 摘要、reviewer 结论、步骤预览、风险、关联 WorkItem

3. 卡片操作按钮
   - 开始孵化
   - 追加当前 Thread
   - 重新规划
   - 送审
   - 审核通过
   - 打回修改
   - 生成待办
   - 生成并执行
   - 暂停
   - 取消

4. 消息与 Track 的视觉关联
   - 识别 `thread_messages.metadata.work_item_track_id`
   - 可在 UI 中高亮其归属 Track

验收标准：

- 用户在 Thread 页面能看到多个 Track
- 用户可以从卡片触发结构化命令
- 带 `work_item_track_id` 的消息或系统事件能与 Track 对上
- planner / reviewer 的自动过程消息联动放到 Phase 5 验收

## 7. Phase 5：planner / reviewer 集成

目标：

- 让 Track 真正驱动 planner / reviewer，而不是只停留在静态对象层

实施项：

1. planner 集成
   - Track 进入 `planning` 时，向指定 planner 发送任务孵化上下文
   - planner 输出写入 `planner_output_json`

2. reviewer 集成
   - Track 进入 `reviewing` 时，向 reviewer 发送 planner 输出
   - reviewer 输出写入 `review_output_json`

3. 过程消息回写 Thread
   - planner / reviewer 的输出消息附带 `work_item_track_id`
   - 复用 Phase 2 已落地的消息 metadata 契约，不再额外引入新消息模型

4. summary 聚合规则
   - 第一阶段优先读 `Thread.summary`
   - 没有 summary 时再退回最近消息摘要

验收标准：

- planner / reviewer 的过程状态可以驱动 Track 状态变化
- 过程输出能落到 Track，也能回显到 Thread
- planner / reviewer 的自动过程消息能在 Phase 4 已存在的 UI 高亮与卡片聚合中正确归属

## 8. 第一阶段明确不做的内容

为了避免范围扩大，以下内容不进入当前实施：

1. 多 reviewer 投票
2. Track 版本树
3. 一个 Track 同时拆成多个 WorkItem
4. Track 之间依赖关系
5. 自动跨 Thread 聚类
6. 与 ChatSession crystallize 的联动补强

## 9. 推荐实施顺序

建议严格按下面顺序推进：

1. Phase 1：模型与存储
2. Phase 2：应用层与 API
3. Phase 3：WorkItem 落地
4. Phase 4：Thread UI
5. Phase 5：planner / reviewer 集成

原因：

- 先有稳定数据模型，后端和前端才能围绕同一真相源工作
- 先把实时协议和消息字段打稳，Thread UI 才不会在 Phase 4 因缺少事件或 metadata 而空转
- 先打通 `Track -> WorkItem` 的事务与幂等，再做卡片交互，能避免 UI 触发后留下半成功状态
- planner / reviewer 自动化放在最后，更容易边做边调

## 10. 最终验收口径

本计划完成后，应满足：

1. 用户可以直接在 `Thread` 中发起任务孵化
2. 一个 Thread 可以并行存在多个 `WorkItemTrack`
3. 一个 Track 可以关联多个 Thread
4. reviewer 通过后，用户可以选择“生成待办”或“生成并执行”
5. 正式 `WorkItem` 能通过现有机制进入待办池和执行主线
6. 整个过程能在 Thread 页面中通过卡片和消息看清楚

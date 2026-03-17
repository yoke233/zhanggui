# Thread Task DAG 现状规格

> 状态：部分实现
>
> 最后按代码核对：2026-03-17
>
> 替代关系：本文已取代 `thread-workitem-track.zh-CN.md`，当前代码主线以
> `ThreadTaskGroup` / `ThreadTask` 为准。
>
> 对应实现：
> - `internal/core/thread_task.go`
> - `internal/application/threadtaskapp/service.go`
> - `internal/adapters/http/thread_task.go`
> - `internal/adapters/store/sqlite/thread_task.go`
> - `web/src/lib/apiClient.ts`

## 一句话结论

Thread 内轻量 DAG 已经落地，当前系统已支持：

- 在 Thread 下创建 `task group`
- 在 group 内定义 `work` / `review` 两类 task
- 用依赖关系形成 DAG
- 通过 agent runtime 派发 task
- 通过 `thread-tasks/{id}/signal` 回报 `complete` / `reject`
- 用 Thread 消息、事件和通知回讲执行过程
- 可选在 group 完成后自动 materialize 为 WorkItem

但它仍属于“部分实现”而非完全定型，原因是前端卡片形态、
可靠投递语义和更完整的产物治理仍在演进。

## 当前定位

ThreadTask DAG 用来承接 Thread 内部的轻量协作编排。

它与 WorkItem 的关系是：

- `ThreadTask` 不是 `WorkItem`
- `ThreadTaskGroup` 是一次 Thread 内协作编排
- 成功完成后可以选择：
  - 只保留在 Thread 中
  - 或自动 materialize 为 WorkItem

当前主链路可以概括为：

```text
Thread
  -> ThreadTaskGroup
    -> ThreadTask(work/review)
      -> Agent dispatch / signal
      -> ThreadMessage + EventBus + Notification
      -> 可选 MaterializeToWorkItem
```

## 核心模型

### ThreadTaskGroup

当前 `ThreadTaskGroup` 已包含以下核心字段：

- `id`
- `thread_id`
- `status`
- `source_message_id`
- `status_message_id`
- `notify_on_complete`
- `materialize_to_workitem`
- `materialized_work_item_id`
- `created_at`
- `completed_at`

当前 group 状态包括：

- `pending`
- `running`
- `done`
- `failed`

### ThreadTask

当前 `ThreadTask` 已包含以下核心字段：

- `id`
- `group_id`
- `thread_id`
- `assignee`
- `type`
- `instruction`
- `depends_on`
- `status`
- `output_file_path`
- `output_message_id`
- `review_feedback`
- `max_retries`
- `retry_count`
- `created_at`
- `completed_at`

当前 task 类型包括：

- `work`
- `review`

当前 task 状态包括：

- `pending`
- `ready`
- `running`
- `done`
- `rejected`
- `failed`

## 当前行为规范

### 创建 group

当前入口：

- `POST /threads/{threadID}/task-groups`

创建时客户端传入：

- tasks 列表
- `depends_on_index`
- 可选 `source_message_id`
- 可选 `notify_on_complete`
- 可选 `materialize_to_workitem`

服务端当前行为：

1. 校验 `thread_id` 与 tasks
2. 创建 `ThreadTaskGroup`
3. 创建全部 `ThreadTask`
4. 将 `depends_on_index` 转成实际 task ID
5. 插入一条进度卡系统消息，回填 `status_message_id`
6. 将 group 状态切到 `running`
7. 触发第一轮 `Tick()`

### 调度循环

当前调度逻辑已在 `threadtaskapp.Service.Tick()` 中实现。

执行顺序为：

1. 找出所有 `pending` 且依赖已全部完成的 task，提升到 `ready`
2. 将所有 `ready` task 派发为 `running`
3. 检查 group 是否整体结束
4. 更新进度卡元数据事件

### 派发方式

当前默认通过 `ThreadAgentRuntime` 派发。

服务端会：

- 先 `InviteAgent`
- 再等待 `WaitAgentReady`
- 最后 `SendMessage`

如果 agent 邀请、启动或消息发送失败，task 会被标记为 `failed`，
并触发 group 失败收口。

### 输入构建

当前派发给 agent 的输入已由服务端统一拼装，包含：

- 当前 task 指令
- 上游产出文件列表
- 当前产出文件路径要求
- review 任务的审核要求
- 上一轮 reject 反馈
- `AI_WORKFLOW_TASK_ID` 等环境变量提示
- 调用 `task-signal` skill 的说明

### complete / reject 信号

当前入口：

- `POST /thread-tasks/{taskID}/signal`

当前支持动作：

- `complete`
- `reject`

行为约束：

- `complete` 适用于运行中的 task
- `reject` 仅适用于运行中的 `review` task

### review reject 行为

当前 reject 逻辑已经落地：

1. 将 review task 先标记为 `rejected`
2. 写入 reject 消息
3. 找到其依赖的上游 `work` task
4. 若上游 `RetryCount >= MaxRetries`：
   - 上游 task 标记 `failed`
   - group 标记 `failed`
5. 否则：
   - 上游 work task 重置为 `pending`
   - 写入 `review_feedback`
   - `retry_count++`
   - review task 自身也重置为 `pending`
6. 再次触发 `Tick()`

这意味着当前代码确实支持“review 打回后自动返工”的闭环。

## 当前消息、事件与通知

### Thread 消息

当前实现会在 Thread 中插入系统/agent 消息，用于讲述 DAG 过程。

当前已使用的 metadata 类型包括：

- `task_group_progress`
- `task_output`
- `task_review_approved`
- `task_review_rejected`
- `task_group_completed`
- `task_group_materialized`

这些消息已经是当前行为的一部分，不是未来规划。

### EventBus 事件

当前事件常量已经落地：

- `thread.task_group.created`
- `thread.task_group.completed`
- `thread.task.started`
- `thread.task.completed`

这些事件通过现有 EventBus 与 Thread/WebSocket 链路向前端传播。

### Notification

当前 `notify_on_complete = true` 时：

- 成功完成会发送 success notification
- 失败会发送 error notification

通知当前复用已有 `Notification` 模型，不引入独立 task 通知体系。

## 与 WorkItem 的关系

当前 ThreadTask DAG 与 WorkItem 的边界是清晰的：

- ThreadTaskGroup 是 Thread 内协作编排
- WorkItem 是全局执行主对象
- 自动收口到 WorkItem 是可选能力，不是默认强制路径

当前 materialize 行为已支持：

- 生成 WorkItem 标题与正文
- 读取输出文件内容
- 调用 materializer 创建 WorkItem
- 建立 Thread 与 WorkItem 的 link
- 回填 `materialized_work_item_id`
- 发送系统消息与事件

## API 现状

当前已实现端点：

| Method | Path | 说明 |
|------|------|------|
| `POST` | `/threads/{threadID}/task-groups` | 创建 group 与 tasks |
| `GET` | `/threads/{threadID}/task-groups` | 列出 group |
| `GET` | `/task-groups/{groupID}` | 获取 group 明细 |
| `DELETE` | `/task-groups/{groupID}` | 删除 group |
| `POST` | `/thread-tasks/{taskID}/signal` | task 完成或 reject 回报 |

前端当前已消费：

- `listThreadTaskGroups`
- `createThreadTaskGroup`
- `getThreadTaskGroup`
- `deleteThreadTaskGroup`
- `signalThreadTask`

## 当前实现边界

以下内容已经实现：

- 核心数据模型
- SQLite 存储
- HTTP API
- 基础调度
- reject 回退重试
- Thread 消息叙事
- EventBus 事件
- 完成/失败通知
- 可选 materialize 为 WorkItem

以下内容仍不应被写成“完全成熟”：

- 进度卡消息本体并不是直接原地修改消息，而是通过事件推动前端刷新
- 路由/投递仍依赖当前 Thread agent runtime 的 best-effort 语义
- 产出文件治理仍是相对轻量的 `output_file_path` 方案
- 删除 group 的语义还偏“数据删除”，不是完整取消协议

## 与旧 Track 方案的关系

`thread-workitem-track` 当前已经被替代。

当前应以 `ThreadTaskGroup` / `ThreadTask` 为准，不再把旧 Track
状态机、旧 Track API 和旧 Track skill 当成现行依据。

## 推荐搭配阅读

1. `thread-agent-runtime.zh-CN.md`
2. `thread-workspace-context.zh-CN.md`
3. `thread-workitem-linking.zh-CN.md`
4. `naming-transition-thread-workitem.zh-CN.md`

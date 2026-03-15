# Thread 内轻量 DAG：ThreadTask 设计

> 状态：草案
>
> 创建日期：2026-03-15
>
> 替代文档：本文替代 `thread-workitem-track.zh-CN.md`（WorkItemTrack 将被移除）
>
> 相关文档：
> - `thread-workspace-context.zh-CN.md`（Thread workspace 与上下文引用）
> - `thread-workitem-linking.zh-CN.md`（Thread-WorkItem 显式关联）
> - `thread-agent-runtime.zh-CN.md`（Thread agent 运行时）

## 1. 动机

### 1.1 WorkItemTrack 的问题

`WorkItemTrack` 设计了一条完整的孵化链路：

```
draft → planning → reviewing → awaiting_confirmation → materialized → executing → done
```

10 个状态、12 个 API、专用 planner/reviewer 角色、专用 `track-planning` skill、最终才产出一个正式 `WorkItem`。

这对"让 A 做个调研、B 审一下、通知我"这种轻量协作场景来说太重了。用户的真实需求是：

1. 在聊天里指派工作给 agent，像指挥同事一样自然
2. agent 之间可以串行交接、并行协作
3. 审核不合格能自动打回重做
4. 完成后通知我，不需要创建正式工单

### 1.2 设计目标

用一个轻量 DAG 替代 WorkItemTrack，直接嵌入 Thread 聊天流：

- **极简模型**：只有 `ThreadTaskGroup`（一次编排）和 `ThreadTask`（一个节点）
- **聊天即工作台**：所有过程在聊天流中可见，内联进度卡 + 产出卡片
- **产物即文件**：agent 的产出落地为 workspace 内的 markdown 文件，跨 group 可复用
- **通知收口**：完成或失败走现有 `NotificationService`
- **可选升级**：结果好的话，用户可以随时通过现有 `POST /threads/{id}/create-work-item` 转为正式 WorkItem

## 2. 核心判断

1. **ThreadTask 不是 WorkItem**。ThreadTask 是 Thread 内的轻量协作单元，完成后即结束，不进入全局待办池。
2. **产物属于 Thread，不属于 Group**。所有 task 的输出文件放在 Thread workspace 的 `outputs/` 目录下，扁平共享。Group 只是编排逻辑，不是文件隔离边界。
3. **调度器驱动执行，消息讲述故事**。agent 不需要"读聊天消息"来知道下一步干什么——调度器直接派发。聊天流中的消息是给用户看的叙事。
4. **不做模式切换**。Thread 保持聊天模式不变，TaskGroup 以内联卡片形式嵌在聊天流中。
5. **review 就是 work 的一种**。review 类型的 task 本质上是"读上游产物、给出判断"的 work，不限制 agent 能力，只是口头意义上的审核。

## 3. 数据模型

### 3.1 ThreadTaskGroup

一次编排，代表一个完整的小 DAG。

```sql
CREATE TABLE thread_task_groups (
    id                 INTEGER PRIMARY KEY AUTOINCREMENT,
    thread_id          INTEGER NOT NULL,
    status             TEXT    NOT NULL DEFAULT 'pending',
    source_message_id  INTEGER,          -- 触发创建的那条用户消息
    status_message_id  INTEGER,          -- 内联进度卡的消息 ID（持续更新）
    notify_on_complete BOOLEAN NOT NULL DEFAULT TRUE,
    created_at         DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    completed_at       DATETIME
);
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `id` | int64 | 主键 |
| `thread_id` | int64 | 所属 Thread |
| `status` | TaskGroupStatus | `pending` / `running` / `done` / `failed` |
| `source_message_id` | *int64 | 用户发起编排的那条消息 |
| `status_message_id` | *int64 | 聊天流中的内联进度卡消息（状态变更时更新此消息的 metadata） |
| `notify_on_complete` | bool | 完成时是否发通知 |

**状态流转**：

```
pending → running → done
                  → failed
```

- `pending`：刚创建，尚未开始调度
- `running`：至少一个 task 已开始执行
- `done`：所有 task 已完成
- `failed`：至少一个 task 最终失败（含 review 超限）

### 3.2 ThreadTask

DAG 中的一个节点。

```sql
CREATE TABLE thread_tasks (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    group_id          INTEGER NOT NULL,
    thread_id         INTEGER NOT NULL,   -- 冗余，查询方便
    assignee          TEXT    NOT NULL,    -- agent profile ID
    type              TEXT    NOT NULL DEFAULT 'work',
    instruction       TEXT    NOT NULL,    -- 给 agent 的指令
    depends_on_json   TEXT    NOT NULL DEFAULT '[]',  -- 上游 task ID 列表
    status            TEXT    NOT NULL DEFAULT 'pending',
    output_file_path  TEXT    NOT NULL DEFAULT '',     -- 产出文件相对路径
    output_message_id INTEGER,             -- 产出消息 ID
    review_feedback   TEXT    NOT NULL DEFAULT '',     -- reject 时的反馈
    max_retries       INTEGER NOT NULL DEFAULT 0,
    retry_count       INTEGER NOT NULL DEFAULT 0,
    created_at        DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    completed_at      DATETIME
);
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `id` | int64 | 主键 |
| `group_id` | int64 | 所属 TaskGroup |
| `thread_id` | int64 | 所属 Thread（冗余） |
| `assignee` | string | agent profile ID |
| `type` | TaskType | `work` / `review` |
| `instruction` | string | 自然语言指令 |
| `depends_on_json` | []int64 | 上游 task ID 列表（JSON 数组） |
| `status` | ThreadTaskStatus | 见下文 |
| `output_file_path` | string | 产出文件路径，相对于 thread workspace（如 `outputs/competitive-pricing-research.md`） |
| `output_message_id` | *int64 | 产出消息在聊天流中的 ID |
| `review_feedback` | string | review 打回时的反馈文字（注入给重试的 agent） |
| `max_retries` | int | 最大重试次数（review 类型默认 3，work 类型默认 0） |
| `retry_count` | int | 当前已重试次数 |

**任务类型**：

- `work`：常规工作。调研、编写、分析、汇总都算 work。
- `review`：审核上游 work 的产出。approve 则流程继续，reject 则上游重做。

说明：不设 `aggregate` 类型。汇总就是 `work`，只是 `depends_on` 包含多个上游。类型由 `depends_on` 拓扑自然表达。

**状态流转**：

```
pending → ready → running → done
                          → rejected   (仅 review 类型产生此状态给上游)
                          → failed

rejected 对上游的影响:
  上游 work: done → pending (retry_count++)
  当前 review: done → pending (等待上游重做后重新触发)
```

- `pending`：等待上游完成
- `ready`：所有上游已 done，可以被调度
- `running`：agent 正在执行
- `done`：完成
- `rejected`：review 不通过（这个状态标记在 review task 自身，同时触发上游 work 重试）
- `failed`：执行失败或重试次数耗尽

### 3.3 类型定义（Go）

```go
// internal/core/thread_task.go

type TaskGroupStatus string

const (
    TaskGroupPending TaskGroupStatus = "pending"
    TaskGroupRunning TaskGroupStatus = "running"
    TaskGroupDone    TaskGroupStatus = "done"
    TaskGroupFailed  TaskGroupStatus = "failed"
)

type TaskType string

const (
    TaskTypeWork   TaskType = "work"
    TaskTypeReview TaskType = "review"
)

type ThreadTaskStatus string

const (
    ThreadTaskPending  ThreadTaskStatus = "pending"
    ThreadTaskReady    ThreadTaskStatus = "ready"
    ThreadTaskRunning  ThreadTaskStatus = "running"
    ThreadTaskDone     ThreadTaskStatus = "done"
    ThreadTaskRejected ThreadTaskStatus = "rejected"
    ThreadTaskFailed   ThreadTaskStatus = "failed"
)

type ThreadTaskGroup struct {
    ID               int64           `json:"id"`
    ThreadID         int64           `json:"thread_id"`
    Status           TaskGroupStatus `json:"status"`
    SourceMessageID  *int64          `json:"source_message_id,omitempty"`
    StatusMessageID  *int64          `json:"status_message_id,omitempty"`
    NotifyOnComplete bool            `json:"notify_on_complete"`
    CreatedAt        time.Time       `json:"created_at"`
    CompletedAt      *time.Time      `json:"completed_at,omitempty"`
}

type ThreadTask struct {
    ID              int64            `json:"id"`
    GroupID         int64            `json:"group_id"`
    ThreadID        int64            `json:"thread_id"`
    Assignee        string           `json:"assignee"`
    Type            TaskType         `json:"type"`
    Instruction     string           `json:"instruction"`
    DependsOn       []int64          `json:"depends_on"`
    Status          ThreadTaskStatus `json:"status"`
    OutputFilePath  string           `json:"output_file_path,omitempty"`
    OutputMessageID *int64           `json:"output_message_id,omitempty"`
    ReviewFeedback  string           `json:"review_feedback,omitempty"`
    MaxRetries      int              `json:"max_retries"`
    RetryCount      int              `json:"retry_count"`
    CreatedAt       time.Time        `json:"created_at"`
    CompletedAt     *time.Time       `json:"completed_at,omitempty"`
}
```

## 4. 产出文件机制

### 4.1 目录结构

所有 task 产出放在 Thread workspace 的 `outputs/` 目录下，**扁平共享**：

```
{dataDir}/threads/{threadID}/
├── outputs/                              ← 所有任务产出，扁平共享
│   ├── competitive-pricing-research.md   ← Group 1, Task A 的产出
│   ├── competitive-pricing-review.md     ← Group 1, Task B 的审核意见
│   ├── user-feedback-analysis.md         ← Group 2, Task C 的产出
│   └── ...
├── attachments/                          ← 用户上传（已有）
├── projects/                             ← 项目挂载（已有）
└── .context.json                         ← 平台自动维护（已有）
```

### 4.2 设计原则

1. **文件属于 Thread，不属于 Group**。任何 group 的 task 都可以引用任何已有的 output 文件。
2. **文件名由 instruction 推导**，不含 groupID 或 taskID。例如 instruction 是"调研竞品定价策略"，文件名就是 `competitive-pricing-research.md`。
3. **谁产出的、属于哪个 group**，全在 `ThreadTask.OutputFilePath` 字段中追踪，不靠目录结构。
4. **agent 负责写文件**。调度器在派发 agent 时告知产出文件路径，agent 完成后将 markdown 写入该路径。
5. **下游 agent 直接读文件**。调度器在派发下游 task 时，将上游 `OutputFilePath` 列表注入 agent 的 input context，agent 读取文件内容作为工作输入。

### 4.3 与 .context.json 的关系

`outputs/` 目录中的文件在 `.context.json` 同步时自动收录。agent 读取 `.context.json` 即可发现所有可用产出。

### 4.4 与 ThreadAttachment 的关系

- `ThreadAttachment` 用于**用户上传**的讨论素材（放在 `attachments/`）
- `outputs/` 用于 **agent 产出**的工作结果
- 两者物理隔离，语义清晰

## 5. 聊天流叙事

### 5.1 设计原则

调度器驱动执行，消息讲述故事。聊天流中出现两类特殊消息：

1. **进度卡片**：TaskGroup 创建时插入，随状态变更持续更新
2. **产出卡片**：每个 task 完成时插入，展示产出摘要和文件链接

两类消息都是 `ThreadMessage`，通过 `metadata` 中的 `type` 字段区分，前端识别后渲染为卡片而非普通文字气泡。

### 5.2 进度卡片

TaskGroup 创建时在聊天流中插入一条系统消息，ID 存入 `ThreadTaskGroup.StatusMessageID`。状态变更时更新此消息的 metadata。

```json
{
  "role": "system",
  "content": "",
  "metadata": {
    "type": "task_group_progress",
    "task_group_id": 1,
    "tasks": [
      {"id": 1, "assignee": "researcher", "instruction": "调研竞品定价", "status": "running"},
      {"id": 2, "assignee": "reviewer",   "instruction": "审核调研结果", "status": "pending"}
    ],
    "edges": [[1, 2]],
    "group_status": "running"
  }
}
```

前端渲染示例（串行）：

```
┌──────────────────────────────────────┐
│ Task Group #1                        │
│                                      │
│  [researcher:调研] ──→ [reviewer:审核]│
│   ● 运行中              ○ 等待      │
│                                      │
│  进度: 1/2                           │
└──────────────────────────────────────┘
```

前端渲染示例（并行 + 汇总）：

```
┌──────────────────────────────────────┐
│ Task Group #2                        │
│                                      │
│  [A:技术调研] ──┐                    │
│   ● 运行中      ├──→ [C:汇总]       │
│  [B:市场数据] ──┘     ○ 等待        │
│   ● 运行中                           │
└──────────────────────────────────────┘
```

### 5.3 产出卡片

每个 task 完成时在聊天流中插入一条消息，ID 存入 `ThreadTask.OutputMessageID`。

**work 完成**：

```json
{
  "sender_id": "researcher",
  "role": "agent",
  "content": "已完成调研，产出文件见 outputs/competitive-pricing-research.md",
  "metadata": {
    "type": "task_output",
    "task_id": 1,
    "task_group_id": 1,
    "output_file": "outputs/competitive-pricing-research.md",
    "next_task": {"id": 2, "assignee": "reviewer", "instruction": "审核调研结果"}
  }
}
```

**review 通过**：

```json
{
  "sender_id": "reviewer",
  "role": "agent",
  "content": "审核通过。详细意见见 outputs/competitive-pricing-review.md",
  "metadata": {
    "type": "task_review_approved",
    "task_id": 2,
    "task_group_id": 1,
    "output_file": "outputs/competitive-pricing-review.md"
  }
}
```

**review 打回**：

```json
{
  "sender_id": "reviewer",
  "role": "agent",
  "content": "审核未通过，已打回修改（第 1/3 轮）。反馈: 缺少东南亚市场数据",
  "metadata": {
    "type": "task_review_rejected",
    "task_id": 2,
    "task_group_id": 1,
    "output_file": "outputs/competitive-pricing-review.md",
    "feedback": "缺少东南亚市场数据",
    "retry_round": "1/3"
  }
}
```

### 5.4 Group 完成消息

TaskGroup 完成时更新进度卡片为最终状态，并发送通知：

```json
{
  "role": "system",
  "content": "Task Group #1 已完成",
  "metadata": {
    "type": "task_group_completed",
    "task_group_id": 1,
    "final_status": "done",
    "output_files": [
      "outputs/competitive-pricing-research.md",
      "outputs/competitive-pricing-review.md"
    ]
  }
}
```

## 6. 调度器

### 6.1 调度循环

调度器是一个简单的事件驱动循环，不需要定时轮询。每当一个 task 状态变更时触发一次 `Tick`。

```go
// internal/engine/task_scheduler.go

func (s *TaskScheduler) Tick(ctx context.Context, groupID int64) error {
    tasks := s.store.ListTasksByGroup(ctx, groupID)

    // 1. 找到所有 pending 且上游全部 done 的 task → 标记 ready
    for _, t := range tasks {
        if t.Status != ThreadTaskPending { continue }
        if s.allDependsDone(t, tasks) {
            t.Status = ThreadTaskReady
            s.store.UpdateThreadTask(ctx, t)
        }
    }

    // 2. 派发所有 ready 的 task
    for _, t := range tasks {
        if t.Status != ThreadTaskReady { continue }
        s.dispatch(ctx, t, tasks)
    }

    // 3. 检查 group 是否完成
    if s.allTasksTerminal(tasks) {
        if s.anyTaskFailed(tasks) {
            s.failGroup(ctx, groupID)
        } else {
            s.completeGroup(ctx, groupID)
        }
    }

    return nil
}
```

### 6.2 派发 agent

```go
func (s *TaskScheduler) dispatch(ctx context.Context, task *ThreadTask, allTasks []*ThreadTask) {
    // 收集上游产物文件路径
    upstreamFiles := s.collectUpstreamOutputFiles(task, allTasks)

    // 构建 agent input
    input := buildTaskInput(task, upstreamFiles)

    // 派发到 agent session pool
    s.agentPool.DispatchTask(ctx, task.ThreadID, task.Assignee, input)

    task.Status = ThreadTaskRunning
    s.store.UpdateThreadTask(ctx, task)

    // 发聊天消息（更新进度卡片）
    s.updateProgressCard(ctx, task.GroupID)
}
```

### 6.3 Agent input 构建

agent 收到的 input 包含：

```markdown
## 任务

{task.Instruction}

## 上游产出

以下是上游任务的产出文件，请阅读后开展工作：

{for each upstream file}
- 文件: {file_path}
{end}

## 要求

- 将你的工作产出写入文件: {task.OutputFilePath}
- 产出格式: Markdown
{if task.Type == "review"}
- 你是审核者。请审核上游产出是否达标。
- 审核通过: 在产出文件中写明"审核通过"及具体意见
- 审核不通过: 在产出文件中写明"审核不通过"及修改建议
{end}
{if task.ReviewFeedback != ""}
## 上次审核反馈

上次提交被审核者打回，反馈如下：

{task.ReviewFeedback}

请根据反馈修改后重新提交。
{end}
```

### 6.4 Review 打回处理

```go
func (s *TaskScheduler) handleReviewReject(ctx context.Context, reviewTask *ThreadTask, feedback string) {
    // 找到被审核的上游 work task
    for _, depID := range reviewTask.DependsOn {
        workTask := s.store.GetThreadTask(ctx, depID)
        if workTask.Type != TaskTypeWork { continue }

        if workTask.RetryCount >= workTask.MaxRetries {
            workTask.Status = ThreadTaskFailed
            s.store.UpdateThreadTask(ctx, workTask)
            s.failGroup(ctx, reviewTask.GroupID)
            return
        }

        // 上游 work 重置为 pending，注入反馈
        workTask.RetryCount++
        workTask.Status = ThreadTaskPending
        workTask.ReviewFeedback = feedback
        s.store.UpdateThreadTask(ctx, workTask)

        // review task 自己也重置为 pending
        reviewTask.Status = ThreadTaskPending
        s.store.UpdateThreadTask(ctx, reviewTask)
    }

    // 触发下一轮调度
    s.Tick(ctx, reviewTask.GroupID)
}
```

### 6.5 Review 结果判定

review 类型的 task 完成后，调度器需要判定审核结果。判定方式：

1. **agent 写入产出文件**后，调度器读取文件内容
2. 解析文件中的结构化标记（如开头包含 `审核通过` 或 `审核不通过`）
3. 或者 agent 通过 task-signal API 主动报告结果（推荐）

推荐方案：复用现有 `step-signal` 思路，为 ThreadTask 提供一个轻量的信号接口：

```
POST /thread-tasks/{taskID}/signal
{
  "action": "complete",            // complete | reject
  "output_file_path": "outputs/xxx.md",
  "feedback": "缺少东南亚数据"     // 仅 reject 时需要
}
```

agent 通过 skill 脚本调用此接口来报告完成或打回。

## 7. 通知

### 7.1 完成通知

TaskGroup 完成时，若 `notify_on_complete = true`，调用现有 `NotificationService`：

```go
notification := &core.Notification{
    Level:    core.NotificationSuccess,
    Title:    "任务完成",
    Body:     fmt.Sprintf("Thread「%s」中的任务组已完成", thread.Title),
    Category: "chat",
    Channels: []core.NotificationChannel{core.ChannelBrowser, core.ChannelInApp},
    ActionURL: fmt.Sprintf("/threads/%d", threadID),
}
```

### 7.2 失败通知

TaskGroup 失败时：

```go
notification := &core.Notification{
    Level:    core.NotificationError,
    Title:    "任务失败",
    Body:     fmt.Sprintf("Thread「%s」中的任务组执行失败", thread.Title),
    Category: "chat",
    Channels: []core.NotificationChannel{core.ChannelBrowser, core.ChannelInApp},
    ActionURL: fmt.Sprintf("/threads/%d", threadID),
}
```

## 8. API 端点

### 8.1 TaskGroup CRUD

| Method | Path | 说明 |
|--------|------|------|
| `POST` | `/threads/{threadID}/task-groups` | 创建 TaskGroup（含所有 tasks） |
| `GET` | `/threads/{threadID}/task-groups` | 列出 Thread 下所有 TaskGroup |
| `GET` | `/task-groups/{groupID}` | 获取 TaskGroup 详情（含所有 tasks） |
| `DELETE` | `/task-groups/{groupID}` | 取消/删除 TaskGroup |

### 8.2 创建请求

```json
POST /threads/1/task-groups
{
  "tasks": [
    {
      "assignee": "researcher",
      "type": "work",
      "instruction": "调研竞品定价策略，重点关注东南亚市场",
      "output_file_name": "competitive-pricing-research.md"
    },
    {
      "assignee": "reviewer",
      "type": "review",
      "instruction": "审核调研报告的完整性和数据准确性",
      "depends_on_index": [0],
      "max_retries": 3,
      "output_file_name": "competitive-pricing-review.md"
    }
  ]
}
```

说明：

- `depends_on_index` 使用数组索引引用（0-based），创建时服务端转换为实际 task ID
- `output_file_name` 只提供文件名，服务端自动补全为 `outputs/{file_name}`
- `max_retries` 对 review 类型默认为 3，work 类型默认为 0
- 创建后立即触发 `Tick` 开始调度

### 8.3 创建响应

```json
201 Created
{
  "id": 1,
  "thread_id": 1,
  "status": "running",
  "tasks": [
    {"id": 1, "assignee": "researcher", "type": "work", "status": "ready", ...},
    {"id": 2, "assignee": "reviewer",   "type": "review", "status": "pending", ...}
  ]
}
```

### 8.4 Task 信号接口

```
POST /thread-tasks/{taskID}/signal
```

请求体：

```json
{
  "action": "complete",
  "output_file_path": "outputs/competitive-pricing-research.md"
}
```

或 review reject：

```json
{
  "action": "reject",
  "output_file_path": "outputs/competitive-pricing-review.md",
  "feedback": "缺少东南亚市场的定价对比数据"
}
```

此接口由 agent 通过 skill 脚本调用。调用后触发调度器 `Tick`。

## 9. 事件模型

新增 4 个事件，走现有 EventBus + Thread WebSocket 订阅：

```go
const (
    EventThreadTaskGroupCreated   = "thread.task_group.created"
    EventThreadTaskGroupCompleted = "thread.task_group.completed"
    EventThreadTaskStarted        = "thread.task.started"
    EventThreadTaskCompleted      = "thread.task.completed"
)
```

| 事件 | 触发时机 | 携带数据 |
|------|---------|---------|
| `thread.task_group.created` | TaskGroup 创建 | group_id, thread_id, tasks 概要 |
| `thread.task_group.completed` | TaskGroup 完成或失败 | group_id, final_status, output_files |
| `thread.task.started` | Task 开始执行 | task_id, group_id, assignee |
| `thread.task.completed` | Task 完成/失败/被打回 | task_id, group_id, status, output_file |

前端通过现有 Thread WebSocket 订阅即可消费这些事件，用于实时更新进度卡片。

## 10. 与现有能力的关系

### 10.1 保留：CreateWorkItemFromThread

现有 `POST /threads/{threadID}/create-work-item` **完整保留**。

用户在 ThreadTask 完成后，如果觉得结果值得正式立项，可以手动调用此接口将讨论/产出转为 WorkItem。ThreadTask 系统不自动创建 WorkItem。

### 10.2 保留：ThreadWorkItemLink

`thread_work_item_links` 表继续存在，用于 Thread 与 WorkItem 的显式关联。

### 10.3 保留：Thread workspace / 上下文引用

Thread workspace 目录结构、`.context.json`、`thread_context_refs` 完全不变。`outputs/` 是 workspace 内新增的子目录，自动被 `.context.json` 索引。

### 10.4 保留：Agent Session Pool

ThreadTask 的 agent 派发复用现有 `ThreadSessionPool`。agent 在 Thread workspace 内执行，拥有与普通 Thread agent 相同的权限和文件访问能力。

### 10.5 移除：WorkItemTrack 全套

WorkItemTrack 被 ThreadTask 完全替代。移除范围：

| 层 | 文件 |
|----|------|
| Core | `internal/core/work_item_track.go`, `work_item_track_test.go` |
| Application | `internal/application/workitemtrackapp/` 整个包 |
| HTTP | `internal/adapters/http/work_item_track.go`, `work_item_track_app.go`, `work_item_track_test.go` |
| Store | `internal/adapters/store/sqlite/work_item_track.go`, `work_item_track_test.go` |
| Schema | `sqlite/models.go` 中 `WorkItemTrackModel`, `WorkItemTrackThreadModel` |
| Events | `internal/core/event.go` 中 10 个 `EventThreadTrack*` 常量 |
| Skill | `internal/skills/builtin/track-planning/` 整个目录 |
| Frontend | `web/src/types/apiV2.ts` 中 Track 类型、`apiClient.ts` 中 Track 方法、`ThreadSidebar.tsx` Track 面板 |
| Spec | `docs/spec/thread-workitem-track.zh-CN.md` 标记为 deprecated |

数据库表 `work_item_tracks` 和 `work_item_track_threads` 在迁移后删除。

### 10.6 移除范围内无需保留的 Track 事件

以下 10 个事件全部移除，由第 9 节的 4 个新事件替代：

- `EventThreadTrackCreated`, `EventThreadTrackUpdated`, `EventThreadTrackStateChanged`
- `EventThreadTrackPlanningStarted`, `EventThreadTrackPlanningCompleted`
- `EventThreadTrackReviewStarted`, `EventThreadTrackReviewApproved`, `EventThreadTrackReviewRejected`
- `EventThreadTrackMaterialized`, `EventThreadTrackRunConfirmed`

## 11. Skill 设计

### 11.1 移除：track-planning

`track-planning` skill 随 WorkItemTrack 一起移除。

### 11.2 新增：task-signal

一个极轻量的 skill，提供给被 ThreadTask 调度的 agent 使用。包含一个脚本：

```bash
# scripts/signal.sh
# 环境变量: AI_WORKFLOW_SERVER_ADDR, AI_WORKFLOW_API_TOKEN, AI_WORKFLOW_TASK_ID

ACTION=${1}           # complete | reject
OUTPUT_FILE=${2}      # 产出文件路径
FEEDBACK=${3:-""}     # reject 时的反馈

curl -s -X POST "${AI_WORKFLOW_SERVER_ADDR}/api/v1/thread-tasks/${AI_WORKFLOW_TASK_ID}/signal" \
  -H "Authorization: Bearer ${AI_WORKFLOW_API_TOKEN}" \
  -H "Content-Type: application/json" \
  -d "{\"action\":\"${ACTION}\",\"output_file_path\":\"${OUTPUT_FILE}\",\"feedback\":\"${FEEDBACK}\"}"
```

agent 执行完毕后调用 `signal.sh complete outputs/xxx.md` 报告完成，或 `signal.sh reject outputs/xxx.md "反馈内容"` 报告打回。

### 11.3 环境变量注入

调度器派发 agent 时注入以下环境变量：

| 变量 | 说明 |
|------|------|
| `AI_WORKFLOW_TASK_ID` | 当前 ThreadTask ID |
| `AI_WORKFLOW_TASK_GROUP_ID` | 当前 TaskGroup ID |
| `AI_WORKFLOW_TASK_TYPE` | `work` 或 `review` |
| `AI_WORKFLOW_OUTPUT_FILE` | 预期产出文件路径 |

复用已有的 `AI_WORKFLOW_SERVER_ADDR` 和 `AI_WORKFLOW_API_TOKEN`。

## 12. 实施顺序

### Phase 1：核心模型 + 调度器

1. 新增 `internal/core/thread_task.go`（ThreadTaskGroup, ThreadTask 类型 + Store 接口）
2. 新增 `internal/adapters/store/sqlite/thread_task.go`（持久化）
3. 新增 `internal/engine/task_scheduler.go`（调度循环）
4. 新增 `task-signal` skill

### Phase 2：API + 聊天叙事

5. 新增 HTTP 端点（`internal/adapters/http/thread_task.go`）
6. 新增事件类型（4 个）
7. 实现进度卡片和产出卡片消息

### Phase 3：前端

8. `web/src/types/` 新增 ThreadTask 类型
9. `web/src/lib/apiClient.ts` 新增 API 方法
10. Thread 消息列表识别并渲染 task_group_progress / task_output / task_review 卡片

### Phase 4：WorkItemTrack 移除

11. 移除 WorkItemTrack 全套代码（26 文件）
12. 数据库迁移：删除 `work_item_tracks`, `work_item_track_threads` 表
13. 更新 spec 文档

Phase 1-3 不破坏现有功能，Phase 4 是纯删除操作。

## 13. 未来扩展（不在本期范围）

以下能力明确不在第一期，但设计上不阻塞后续引入：

1. **多 TaskGroup 并行**：模型已支持，调度器已独立。后续只需前端支持多个进度卡片交织显示。
2. **自然语言解析**：用 AI 将聊天中的 `@A 调研XXX, @B 审核` 自动解析为 DAG 结构。第一期用 API/表单创建。
3. **人工审核**：review task 可以 assignee 设为人类用户，由用户在聊天中点击通过/打回。
4. **跨 Thread 产出引用**：当前产出文件限于 Thread workspace 内。后续可支持引用其他 Thread 的产出。
5. **TaskGroup 模板**：将常用 DAG 模式保存为模板，一键复用。

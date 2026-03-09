# TaskStep 事件溯源设计

日期: 2026-03-09
状态: approved

## 1. 目标

在现有 Issue + Run 双模型上引入 TaskStep 事件溯源层。TaskStep 是业务事实的唯一来源，Issue.Status 变为写时派生的缓存。

### 设计原则

- **TaskStep 是事实，Issue.Status 是缓存** — 任何状态变更先写 TaskStep，再派生更新 Status
- **三层各司其职** — TaskStep（业务事实）、run_events（执行追溯）、review_records（审核细节）
- **写时派生（write-through）** — 每次写 TaskStep 在同一事务内同步更新 Issue.Status
- **向后兼容** — Issue.Status 字段不变，现有 API 和前端无感知

## 2. 新增模型：TaskStep

```go
type TaskStep struct {
    ID        string    // uuid
    IssueID   string    // 关联 Issue
    RunID     string    // 关联 Run（可选，Issue 级操作为空）
    AgentID   string    // 操作者（agent/human/system）
    Action    string    // 枚举，见 Action 表
    StageID   string    // Run 的阶段（stage 类 action 时填写）
    Input     string    // 输入摘要（JSON）
    Output    string    // 输出摘要（JSON）
    Note      string    // 人类可读备注
    RefID     string    // 关联外部记录（如 review_record_id）
    RefType   string    // 关联类型（如 "review_record"）
    CreatedAt time.Time
}
```

## 3. Action 枚举

### Issue 状态变迁（~14 种）

| Action | 触发场景 | Issue.Status 变为 |
|--------|---------|------------------|
| `created` | Issue 创建 | `draft` |
| `submitted_for_review` | 提交审核 | `reviewing` |
| `review_approved` | 审核通过 | `queued` |
| `review_rejected` | 审核打回 | `draft` |
| `queued` | 进入队列 | `queued` |
| `ready` | 获取执行槽 | `ready` |
| `execution_started` | 开始执行 | `executing` |
| `merge_started` | 开始合并 | `merging` |
| `merge_completed` | 合并完成 | `done` |
| `failed` | 执行失败 | `failed` |
| `abandoned` | 放弃 | `abandoned` |
| `decompose_started` | 开始分解 | `decomposing` |
| `decomposed` | 分解完成 | `decomposed` |
| `superseded` | 被取代 | `superseded` |

### Run 关键节点（~6 种）

| Action | 触发场景 | Issue.Status 不变 |
|--------|---------|------------------|
| `run_created` | Run 创建 | — |
| `run_started` | Run 开始执行 | — |
| `stage_started` | Stage 开始 | — |
| `stage_completed` | Stage 完成 | — |
| `stage_failed` | Stage 失败 | — |
| `run_completed` | Run 结束（含 conclusion） | — |

## 4. 写入路径

核心函数签名：

```go
// Store 接口新增
SaveTaskStep(ctx context.Context, step TaskStep) (IssueStatus, error)
ListTaskSteps(ctx context.Context, issueID string) ([]TaskStep, error)
RebuildIssueStatus(ctx context.Context, issueID string) (IssueStatus, error)
```

写入流程（SQLite 单事务）：

```
调用方写 TaskStep
  → Store.SaveTaskStep() 在同一个 SQLite 事务内：
    1. INSERT INTO task_steps
    2. 根据 action 查表派生新的 Issue.Status（仅 Issue 状态变迁类 action）
    3. UPDATE issues SET status = 新状态, updated_at = now
    4. 返回新状态
  → 调用方拿到新状态，发 EventBus 事件
```

## 5. 数据库表

```sql
CREATE TABLE task_steps (
    id         TEXT PRIMARY KEY,
    issue_id   TEXT NOT NULL,
    run_id     TEXT,
    agent_id   TEXT,
    action     TEXT NOT NULL,
    stage_id   TEXT,
    input      TEXT,
    output     TEXT,
    note       TEXT,
    ref_id     TEXT,
    ref_type   TEXT,
    created_at DATETIME NOT NULL,
    FOREIGN KEY (issue_id) REFERENCES issues(id)
);
CREATE INDEX idx_task_steps_issue ON task_steps(issue_id, created_at);
CREATE INDEX idx_task_steps_run   ON task_steps(run_id, created_at);
```

## 6. 三层数据架构

```
TaskStep (业务事实层)
  ├── Issue 状态变迁（~14 种 action）
  └── Run 关键节点（~6 种 action）
          │
          ├── run_events (执行追溯层)
          │     └── prompt / agent_message / tool_call / ...
          │
          └── review_records (审核细节)
                └── reviewer / verdict / comments / ...
```

- **TaskStep** — 记"发生了什么"（业务转折点）
- **run_events** — 记"agent 具体做了什么"（执行细节）
- **review_records** — 记"怎么审的"（审核细节）
- 通过 `issue_id` / `run_id` / `ref_id` 互相关联

## 7. Issue 详情页：流程树

### 视觉结构

```
▼ Issue: 实现用户注册 API                              [executing]
  │
  ├── ✅ draft                                            10:00
  │   └── 由 human 创建
  │
  ├── ✅ reviewing                                        10:05
  │   ▼ 审核详情
  │     ├── completeness (reviewer_code): ✅ pass
  │     ├── feasibility (reviewer_code): ✅ pass
  │     └── dependency (reviewer_code): ⚠️ 建议先做登录模块
  │
  ├── ✅ queued                                           10:08
  ├── ✅ ready                                            10:10
  │
  ▼ 🔄 executing                                         10:10 - ...
  │   │
  │   ▼ Run #run-20260309-abc
  │     │
  │     ├── ✅ setup                                      10:10 (2s)
  │     │
  │     ▼ ✅ implement                                    10:10 - 10:15
  │     │   ├── 🤖 "分析需求，准备实现用户注册..."
  │     │   ├── 🔧 write_file → src/api/register.go
  │     │   ├── 🔧 write_file → src/api/register_test.go
  │     │   ├── 🔧 run_terminal → go test ./...
  │     │   └── 🤖 "完成实现，3 个文件，测试通过"
  │     │
  │     ▼ 🔄 review                                      10:15 - ...
  │     │   ├── 🤖 "开始审阅代码..."
  │     │   └── ⏳ 进行中...
  │     │
  │     ├── ⏳ test
  │     ├── ⏳ merge
  │     └── ⏳ cleanup
  │
  ├── ⏳ merging
  └── ⏳ done
```

### 树的层级与数据来源

| 层级 | 内容 | 数据来源 |
|------|------|---------|
| 第 1 层 | Issue 状态节点 | `task_steps` (按 action 分组) |
| 第 2 层 | Run / 审核概览 | `task_steps` (run_id / ref_id) |
| 第 3 层 | Stage 列表 | `task_steps` (action=stage_*) |
| 第 4 层 | Agent 交互详情 | `run_events` (按 stage 过滤) |
|         | 审核详情 | `review_records` (按 ref_id) |

### 交互行为

- 默认只展示第 1 层，已完成的折叠，当前进行中的展开
- 点击任意节点展开/折叠下一层
- `executing` 默认展开到 Run 的 stage 列表
- 点击某个 stage 展开 agent 交互详情（run_events，懒加载）
- 点击 `reviewing` 展开审核详情（review_records）
- 实时：当前进行中的节点通过 WebSocket 推送 TaskStep + run_events 更新

### API 设计

```
GET /api/v2/issues/{id}/timeline
  → 返回第 1~3 层（TaskStep 全量，轻量数据）

GET /api/v2/runs/{id}/stages/{stage}/events?limit=50
  → 按需加载第 4 层（agent 交互详情，分页）

GET /api/v2/reviews/{id}
  → 按需加载审核详情
```

### 前端组件结构

```
<IssueFlowTree>
  ├── <FlowNode status="done">      ← 折叠，点击展开
  ├── <FlowNode status="active">    ← 默认展开
  │   └── <RunPipeline>
  │       ├── <StageNode status="done">    ← 折叠
  │       ├── <StageNode status="active">  ← 展开
  │       │   └── <EventList>              ← 懒加载 run_events
  │       └── <StageNode status="pending">
  └── <FlowNode status="pending">
```

## 8. 改造范围

### 新增

- `internal/core/task_step.go` — TaskStep 模型 + Action 常量
- `Store` 接口新增：`SaveTaskStep` / `ListTaskSteps` / `RebuildIssueStatus`
- `store-sqlite` 新增 migration + 实现
- API handler：`GET /api/v2/issues/{id}/timeline`
- 前端组件：`<IssueFlowTree>` / `<FlowNode>` / `<StageNode>` / `<EventList>`

### 改造

- `teamleader/manager.go` — Issue 状态变更改为写 TaskStep
- `teamleader/review.go` — 审核结果改为写 TaskStep
- `teamleader/scheduler.go` — queued/ready 转换改为写 TaskStep
- `engine/executor.go` — stage 开始/完成/失败写 TaskStep
- `teamleader/auto_merge.go` — merge 状态写 TaskStep
- 前端 Issue 详情页 — 集成 IssueFlowTree 组件

### 不变

- `run_events` 表和写入逻辑
- `review_records` 表和写入逻辑
- Issue.Status 字段和现有 API 响应格式
- EventBus 事件格式
- Run 模型和执行引擎核心逻辑

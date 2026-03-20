# Plan 1：跨项目跨 WorkItem 编排

> 状态：草案
>
> 创建日期：2026-03-20
>
> 依赖：无（基础层）
>
> 被依赖：Plan 2（Thread 讨论收敛与提案流程）、Plan 3（需求分析与智能路由）

## 1. 目标

让系统具备"一个需求产出多个跨项目 WorkItem，并按依赖关系协调执行"的能力。

当前现状：

- `WorkItem.DependsOn []int64` 字段已存在，但**调度器不执行**（`flow_scheduler.go` 忽略它）
- `WorkItem.DependsOn` 在 `workitemapp` 中校验时**限定同项目**
- 没有"方案"聚合实体——多个 WorkItem 属于同一个需求/方案，缺少组织容器
- 没有方案级别的审批状态——WorkItem 创建后可直接 queue 执行
- 没有跨 WorkItem 整体进度追踪

## 2. 核心判断

1. **不发明新的执行单元**。WorkItem + Action 仍是执行真相，新增的只是上层编排。
2. **`Initiative`（方案）是纯编排容器**，不参与执行调度，只做聚合、审批、进度追踪。
3. **跨项目依赖是真实需求**。一个"上线新功能"可能涉及后端项目、前端项目、基础设施项目各出一个 WorkItem，前端依赖后端完成。
4. **审批是方案级别的**，不是单个 WorkItem 级别的。用户审批的是整个方案，不是逐个批。

## 3. 新增领域模型

### 3.1 Initiative（方案）

```go
// internal/core/initiative.go

type InitiativeStatus string

const (
    InitiativeDraft     InitiativeStatus = "draft"      // 草案，还在编辑
    InitiativeProposed  InitiativeStatus = "proposed"    // 已提案，等待审批
    InitiativeApproved  InitiativeStatus = "approved"    // 已审批，可以开始执行
    InitiativeExecuting InitiativeStatus = "executing"   // 执行中
    InitiativeBlocked   InitiativeStatus = "blocked"     // 执行受阻
    InitiativeDone      InitiativeStatus = "done"        // 全部完成
    InitiativeFailed    InitiativeStatus = "failed"      // 失败
    InitiativeCancelled InitiativeStatus = "cancelled"   // 已取消
)

type Initiative struct {
    ID          int64               `json:"id"`
    Title       string              `json:"title"`
    Description string              `json:"description"`
    Status      InitiativeStatus    `json:"status"`
    CreatedBy   string              `json:"created_by"`    // user or agent ID
    ApprovedBy  *string             `json:"approved_by,omitempty"`
    ApprovedAt  *time.Time          `json:"approved_at,omitempty"`
    Metadata    map[string]any      `json:"metadata,omitempty"`
    CreatedAt   time.Time           `json:"created_at"`
    UpdatedAt   time.Time           `json:"updated_at"`
}
```

### 3.2 InitiativeItem（方案-WorkItem 关联）

```go
type InitiativeItem struct {
    ID           int64  `json:"id"`
    InitiativeID int64  `json:"initiative_id"`
    WorkItemID   int64  `json:"work_item_id"`
    Role         string `json:"role,omitempty"` // 可选标记："lead", "support", "infra" 等
    CreatedAt    time.Time `json:"created_at"`
}
```

### 3.3 Initiative 与 Thread 关联

复用已有的 `ThreadWorkItemLink` 模式，新增：

```go
type ThreadInitiativeLink struct {
    ID           int64  `json:"id"`
    ThreadID     int64  `json:"thread_id"`
    InitiativeID int64  `json:"initiative_id"`
    RelationType string `json:"relation_type"` // "source"（由此 Thread 讨论产生）
    CreatedAt    time.Time `json:"created_at"`
}
```

## 4. WorkItem.DependsOn 放开跨项目限制

### 4.1 当前限制

`internal/application/workitemapp/service.go` 中校验：

> 所有 dependency WorkItem 必须属于同一个 project

### 4.2 改动

- 移除同项目限制
- 保留其他校验：不能依赖自己、不能有重复、依赖的 WorkItem 必须存在
- 新增：WorkItem 必须属于某个 Initiative 时才允许跨项目依赖（避免散乱的跨项目引用）

## 5. 调度器支持跨 WorkItem 依赖

### 5.1 当前调度器

`internal/application/flow/flow_scheduler.go` 只关心 WorkItem 内部 Action DAG，不看 `WorkItem.DependsOn`。

### 5.2 改动

在 `flow_scheduler` 的 WorkItem 入队/启动逻辑中增加前置检查：

```
当一个 WorkItem 被 queue 请求时：
  1. 检查 WorkItem.DependsOn
  2. 若所有依赖 WorkItem 的 status == done → 允许启动
  3. 若存在未完成的依赖 → WorkItem 保持 accepted 状态，不进入 queued
  4. 当某个 WorkItem 完成时 → 检查是否有其他 WorkItem 依赖它
     → 若依赖全部满足 → 自动推进到 queued
```

### 5.3 事件驱动

- 监听 `WorkItemCompleted` 事件
- 查询 `依赖此 WorkItem 的其他 WorkItem`（反向查询）
- 逐个检查依赖是否全部满足
- 满足则自动推进

### 5.4 Store 新增查询

```go
// 查询依赖指定 WorkItem 的所有 WorkItem（反向依赖查询）
ListDependentWorkItems(ctx context.Context, workItemID int64) ([]*WorkItem, error)
```

## 6. Initiative 状态机

```
draft ──→ proposed ──→ approved ──→ executing ──→ done
  │          │            │           │
  │          │            │           └──→ blocked ──→ executing
  │          │            │           └──→ failed
  │          ↓            │
  │       (rejected       │
  │        → draft)       │
  │                       │
  └──────────────────────→ cancelled
```

### 审批流程

1. `draft`：创建方案，添加 WorkItem，编辑依赖关系。可由 Plan 2 的 Proposal materialize 自动创建，也可手动创建。
2. `proposed`：方案提交审批，不可再修改 WorkItem 列表
3. `approved`：用户审批通过，**自动进入 executing**（审批即启动，不需要二次触发）
4. `executing`：系统根据依赖关系自动推进 WorkItem 执行
   - 无依赖的 WorkItem → 立即 queue
   - 有依赖的 WorkItem → 等待依赖完成后自动 queue
5. `done`：所有 WorkItem 完成
6. `failed`：任一 WorkItem 失败且不可恢复

### 审批后自动启动

`approve` 操作一步完成 `proposed → approved → executing`：
1. 标记 approved_by、approved_at
2. 遍历 Initiative 下所有 WorkItem
3. 无依赖的 → 直接 queue
4. 有依赖的 → 标记为 accepted，等待依赖完成
5. Initiative 状态设为 executing

### 两次审批的边界（与 Plan 2 的关系）

用户目标中有两个审批节点：

- **第一次审批：审批结论**（Plan 2 的 Proposal）—— 讨论方向是否正确、结论是否合理
- **第二次审批：审批方案**（本 Plan 的 Initiative）—— 具体的 WorkItem 拆分、依赖关系、执行计划是否合理

Proposal approve 后 materialize 生成的 Initiative 状态为 `draft`，用户需要审阅具体的 WorkItem 拆分结果，确认后才 propose → approve 启动执行。

## 7. 进度追踪

### 7.1 Initiative 进度

```go
type InitiativeProgress struct {
    Total     int `json:"total"`
    Pending   int `json:"pending"`   // open + accepted
    Running   int `json:"running"`   // queued + running
    Blocked   int `json:"blocked"`
    Done      int `json:"done"`
    Failed    int `json:"failed"`
    Cancelled int `json:"cancelled"`
}
```

### 7.2 计算方式

查询 Initiative 下所有 WorkItem 的状态分布，不做缓存，实时查询。

## 8. REST API

```
# Initiative CRUD
POST   /api/initiatives                              # 创建方案
GET    /api/initiatives                              # 列表（支持 status 过滤）
GET    /api/initiatives/{id}                         # 详情（含 WorkItem 列表和进度）
PUT    /api/initiatives/{id}                         # 更新标题/描述
DELETE /api/initiatives/{id}                         # 删除（仅 draft 状态）

# Initiative WorkItem 管理
POST   /api/initiatives/{id}/items                   # 添加 WorkItem 到方案
DELETE /api/initiatives/{id}/items/{workItemID}       # 从方案移除 WorkItem
PUT    /api/initiatives/{id}/items/{workItemID}       # 更新 WorkItem 在方案中的角色

# Initiative 状态推进
POST   /api/initiatives/{id}/propose                 # draft → proposed
POST   /api/initiatives/{id}/approve                 # proposed → approved → executing（一步完成）
POST   /api/initiatives/{id}/reject                  # proposed → draft（附 feedback）
POST   /api/initiatives/{id}/cancel                  # → cancelled

# Initiative 进度
GET    /api/initiatives/{id}/progress                # 实时进度

# Initiative 与 Thread 关联
POST   /api/initiatives/{id}/threads                 # 关联 Thread
GET    /api/initiatives/{id}/threads                 # 列出关联 Thread
DELETE /api/initiatives/{id}/threads/{threadID}       # 移除关联
```

## 9. WebSocket 事件

```
initiative.created
initiative.updated
initiative.proposed           # 方案提交审批
initiative.approved           # 方案审批通过
initiative.rejected           # 方案被打回
initiative.executing          # 开始执行
initiative.progress_changed   # 进度变化（WorkItem 状态变化时触发）
initiative.done               # 全部完成
initiative.failed             # 执行失败
initiative.cancelled          # 已取消
```

## 10. 前端

### 10.1 新增页面

- **InitiativesPage** (`/initiatives`)：方案列表，按状态分组
- **InitiativeDetailPage** (`/initiatives/{id}`)：
  - 方案标题、描述、状态
  - WorkItem 列表（带状态、项目归属、依赖关系可视化）
  - 依赖关系 DAG 图（WorkItem 级别）
  - 进度条
  - 操作按钮（提案/审批/驳回/执行/取消）
  - 关联的 Thread 列表

### 10.2 现有页面增强

- **WorkItemDetailPage**：增加"所属方案"显示
- **ThreadDetailPage**：增加"关联方案"入口

## 11. 数据库 Migration

```sql
-- initiatives 表
CREATE TABLE initiatives (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    title       TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    status      TEXT NOT NULL DEFAULT 'draft',
    created_by  TEXT NOT NULL DEFAULT '',
    approved_by TEXT,
    approved_at DATETIME,
    metadata    TEXT,   -- JSON
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- initiative_items 表
CREATE TABLE initiative_items (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    initiative_id  INTEGER NOT NULL REFERENCES initiatives(id) ON DELETE CASCADE,
    work_item_id   INTEGER NOT NULL REFERENCES issues(id) ON DELETE CASCADE,
    role           TEXT NOT NULL DEFAULT '',
    created_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(initiative_id, work_item_id)
);

-- thread_initiative_links 表
CREATE TABLE thread_initiative_links (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    thread_id      INTEGER NOT NULL REFERENCES threads(id) ON DELETE CASCADE,
    initiative_id  INTEGER NOT NULL REFERENCES initiatives(id) ON DELETE CASCADE,
    relation_type  TEXT NOT NULL DEFAULT 'source',
    created_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(thread_id, initiative_id)
);
```

## 12. 实施阶段

### Phase 1：领域模型与存储

- `internal/core/initiative.go`：Initiative、InitiativeItem、ThreadInitiativeLink、InitiativeStore 接口
- SQLite migration + GORM model
- 状态校验函数

### Phase 2：应用层与 API

- `internal/application/initiativeapp/service.go`：CRUD + 状态推进
- REST handler 注册
- 放开 WorkItem.DependsOn 同项目限制

### Phase 3：调度器集成

- `flow_scheduler` 增加 WorkItem.DependsOn 前置检查
- WorkItemCompleted 事件监听 → 自动推进依赖方
- InitiativeItem 状态聚合 → Initiative 整体状态联动

### Phase 4：WebSocket + 前端

- Initiative 事件定义与发布
- InitiativesPage + InitiativeDetailPage
- 现有页面增强（WorkItem、Thread 显示方案关联）

## 13. 明确不做的内容

1. Initiative 内 WorkItem 自动拆分/生成（由 Plan 2 的 Thread 收敛流程负责）
2. Initiative 版本管理（驳回后直接回到 draft 编辑，不保留历史版本）
3. 多级审批链（第一阶段只有一个审批人）
4. Initiative 模板
5. Initiative 之间的依赖关系（方案之间不互相依赖）

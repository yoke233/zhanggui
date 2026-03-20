# Plan 2：Thread 讨论收敛与提案流程

> 状态：草案
>
> 创建日期：2026-03-20
>
> 依赖：Plan 1（跨项目跨 WorkItem 编排——Initiative 实体）
>
> 被依赖：Plan 3（需求分析与智能路由）

## 1. 目标

让 Thread 的多 Agent 讨论能够**收敛**出结构化结论，经过审批后生成可执行的 Initiative（方案）。

当前现状：

- Thread group_chat 模式支持多 Agent 轮流发言，但讨论是**开放式**的
- `[FINAL]` 机制只是停止轮次，不产出结构化结论
- Thread 所有消息地位相同，没有"这是结论"的语义标记
- `create-work-item` 直接创建 WorkItem，没有提案→审批→批量生成的流程
- Thread 可以挂载多个项目上下文，但讨论产出无法结构化地分配到各项目

## 2. 核心判断

1. **提案是 Thread 消息的一种特殊类型**，不是独立于 Thread 的新容器。提案的讨论上下文就是 Thread 本身。
2. **一个 Thread 可以产出多个提案**。需求分析可能得出多个备选方案，或者一个大需求分阶段提案。
3. **提案审批后自动生成 Initiative + WorkItems**，这是 Thread 协同层到执行层的桥梁。
4. **不引入独立的 Proposal 表**。提案就是一种带结构化 metadata 的 ThreadMessage + 一个轻量状态追踪。用 Thread 已有的 attachment/output 机制承载方案内容。

## 3. 设计方案

### 3.1 Proposal 作为 Thread 内的结构化对象

在 Thread 中新增 `ThreadProposal` 实体，它不是消息，而是从讨论中收敛出来的决策对象：

```go
// internal/core/thread_proposal.go

type ProposalStatus string

const (
    ProposalDraft    ProposalStatus = "draft"     // Agent 正在起草
    ProposalOpen     ProposalStatus = "open"      // 已提出，等待讨论/审批
    ProposalApproved ProposalStatus = "approved"  // 审批通过
    ProposalRejected ProposalStatus = "rejected"  // 被驳回
    ProposalRevised  ProposalStatus = "revised"   // 修订中（驳回后重新编辑）
    ProposalMerged   ProposalStatus = "merged"    // 已合并到 Initiative
)

type ThreadProposal struct {
    ID           int64          `json:"id"`
    ThreadID     int64          `json:"thread_id"`
    Title        string         `json:"title"`
    Summary      string         `json:"summary"`       // 结论摘要
    Content      string         `json:"content"`       // 详细方案内容（markdown）
    ProposedBy   string         `json:"proposed_by"`   // agent 或 human ID
    Status       ProposalStatus `json:"status"`

    // 审批信息
    ReviewedBy   *string    `json:"reviewed_by,omitempty"`
    ReviewedAt   *time.Time `json:"reviewed_at,omitempty"`
    ReviewNote   string     `json:"review_note,omitempty"`  // 审批/驳回意见

    // 方案中包含的 WorkItem 草案
    WorkItemDrafts []ProposalWorkItemDraft `json:"work_item_drafts,omitempty"`

    // 关联
    SourceMessageID *int64 `json:"source_message_id,omitempty"` // 触发此提案的消息
    InitiativeID    *int64 `json:"initiative_id,omitempty"`     // 审批后生成的 Initiative

    Metadata  map[string]any `json:"metadata,omitempty"`
    CreatedAt time.Time      `json:"created_at"`
    UpdatedAt time.Time      `json:"updated_at"`
}

// ProposalWorkItemDraft 是提案中对 WorkItem 的规划草案
type ProposalWorkItemDraft struct {
    TempID      string  `json:"temp_id"`                  // 临时 ID，用于草案间引用依赖
    ProjectID   *int64  `json:"project_id,omitempty"`     // 目标项目
    Title       string  `json:"title"`
    Body        string  `json:"body"`
    Priority    string  `json:"priority"`
    DependsOn   []string `json:"depends_on,omitempty"`    // 引用其他草案的 temp_id
    Labels      []string `json:"labels,omitempty"`
}
```

### 3.2 为什么不直接用 ThreadMessage

消息是讨论过程的一部分，Proposal 是讨论的结论。它们的区别：

- 消息是不可变的；Proposal 有状态流转（draft → open → approved → merged）
- 消息是线性追加的；Proposal 可以被修改、驳回、重新编辑
- 消息没有审批机制；Proposal 有
- 消息不包含结构化的 WorkItem 草案；Proposal 包含

但 Proposal 的**创建和审批事件**会以系统消息的形式出现在 Thread 时间线中，保持可追溯。

## 4. 讨论收敛机制

### 4.1 Agent 主动收敛

在 group_chat 模式中，增加收敛触发：

1. **Agent 可以发起提案**：讨论到一定程度后，某个 Agent（通常是 lead role）可以通过结构化命令创建 Proposal
2. **Agent 可以投票/评论**：其他 Agent 可以对 Proposal 发表意见
3. **人类审批**：最终由人类决定 approve 或 reject

### 4.2 收敛流程

```
Thread 多 Agent 讨论（group_chat 模式）
    ↓
某个 Agent 认为可以收敛
    ↓
Agent 调用结构化命令创建 Proposal（draft）
    ├─ 分析讨论内容，提炼结论
    ├─ 根据挂载的项目上下文，规划 WorkItem 分配
    └─ 生成 WorkItemDrafts（哪个项目做什么）
    ↓
Proposal 状态 → open
    ├─ 系统消息通知 Thread 所有参与者
    └─ 其他 Agent 可以评论补充
    ↓
人类审批
    ├─ approve → Proposal 状态 → approved
    │   ↓
    │   自动 materialize:
    │   ├─ 创建 Initiative（Plan 1 的实体）
    │   ├─ 按 WorkItemDrafts 创建 WorkItem（解析依赖关系）
    │   ├─ 建立 Initiative ↔ WorkItem 关联
    │   ├─ 建立 Initiative ↔ Thread 关联
    │   └─ Proposal 状态 → merged
    │
    └─ reject（附意见）→ Proposal 状态 → rejected
        ↓
        Agent 收到驳回意见，可以修订 → revised → 重新 open
```

## 5. REST API

```
# Proposal CRUD
POST   /api/threads/{threadID}/proposals              # 创建提案
GET    /api/threads/{threadID}/proposals              # 列出 Thread 下的提案
GET    /api/proposals/{proposalID}                    # 提案详情
PUT    /api/proposals/{proposalID}                    # 更新提案（仅 draft/revised 状态）
DELETE /api/proposals/{proposalID}                    # 删除提案（仅 draft 状态）

# Proposal 状态推进
POST   /api/proposals/{proposalID}/submit             # draft/revised → open
POST   /api/proposals/{proposalID}/approve            # open → approved → 自动 materialize
POST   /api/proposals/{proposalID}/reject             # open → rejected（附 review_note）
POST   /api/proposals/{proposalID}/revise             # rejected → revised

# Proposal WorkItem 草案管理
PUT    /api/proposals/{proposalID}/drafts             # 批量更新 WorkItem 草案
```

## 6. WebSocket 事件

```
thread.proposal.created        # 新提案创建
thread.proposal.submitted      # 提案提交审批
thread.proposal.approved       # 提案审批通过
thread.proposal.rejected       # 提案被驳回
thread.proposal.revised        # 提案修订
thread.proposal.merged         # 提案已合并为 Initiative
```

## 7. Agent 能力扩展

### 7.1 新增 Agent Action

在 `AgentProfile.ActionsAllowed` 中新增：

- `create_proposal`：允许 Agent 创建提案
- `submit_proposal`：允许 Agent 提交提案进入审批

### 7.2 Agent 收敛提示

在 group_chat 模式的上下文中，增加收敛提示：

- 当讨论轮次接近 `meeting_max_rounds` 时，提示 lead Agent 可以收敛
- Lead Agent 可以在任何时候发起收敛
- 收敛时，Agent 需要：
  1. 总结讨论要点
  2. 分析需求涉及哪些项目（基于 ThreadContextRef 挂载的项目）
  3. 为每个涉及的项目拟定 WorkItem 草案
  4. 标注 WorkItem 之间的依赖关系

## 8. Materialize 流程（Proposal → Initiative）

审批通过后的自动物化：

```go
func (s *Service) MaterializeProposal(ctx context.Context, proposalID int64) error {
    // 1. 获取 Proposal + WorkItemDrafts
    // 2. 创建 Initiative（status = approved）
    // 3. 逐个创建 WorkItem
    //    - 解析 DependsOn：将 temp_id 映射为真实 WorkItem ID
    //    - 设置 ProjectID
    // 4. 创建 InitiativeItem 关联
    // 5. 创建 ThreadInitiativeLink
    // 6. 更新 Proposal: initiative_id, status = merged
    // 7. 发布事件 + 系统消息
}
```

## 9. 前端

### 9.1 ThreadDetailPage 增强

- **提案卡片**：在 Thread 消息流中插入提案卡片
  - 显示提案标题、状态、WorkItem 草案列表
  - 操作按钮：编辑/提交/审批/驳回
- **提案侧边栏**：列出当前 Thread 的所有提案及状态
- **WorkItem 草案编辑器**：
  - 表格形式编辑多个 WorkItem 草案
  - 项目选择器（从 ThreadContextRef 挂载的项目中选）
  - 依赖关系拖拽连线

### 9.2 提案审批视图

- 并排对比多个提案（如果有备选方案）
- 查看每个 WorkItem 草案的详情
- 一键审批/驳回（附意见）

## 10. 数据库 Migration

```sql
-- thread_proposals 表
CREATE TABLE thread_proposals (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    thread_id         INTEGER NOT NULL REFERENCES threads(id) ON DELETE CASCADE,
    title             TEXT NOT NULL,
    summary           TEXT NOT NULL DEFAULT '',
    content           TEXT NOT NULL DEFAULT '',
    proposed_by       TEXT NOT NULL DEFAULT '',
    status            TEXT NOT NULL DEFAULT 'draft',
    reviewed_by       TEXT,
    reviewed_at       DATETIME,
    review_note       TEXT NOT NULL DEFAULT '',
    work_item_drafts  TEXT NOT NULL DEFAULT '[]',  -- JSON array of ProposalWorkItemDraft
    source_message_id INTEGER,
    initiative_id     INTEGER REFERENCES initiatives(id),
    metadata          TEXT,  -- JSON
    created_at        DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at        DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_thread_proposals_thread ON thread_proposals(thread_id);
CREATE INDEX idx_thread_proposals_status ON thread_proposals(status);
```

## 11. 实施阶段

### Phase 1：领域模型与存储

- `internal/core/thread_proposal.go`：ThreadProposal、ProposalWorkItemDraft
- ProposalStore 接口
- SQLite migration + GORM model
- 状态校验函数

### Phase 2：应用层与 API

- `internal/application/threadapp/proposal.go`（或独立 `proposalapp/`）
- Proposal CRUD + 状态推进
- Materialize 流程（Proposal → Initiative + WorkItems）
- REST handler

### Phase 3：Agent 集成

- Agent action 扩展：`create_proposal`, `submit_proposal`
- Group chat 收敛提示逻辑
- Agent 结构化命令处理

### Phase 4：WebSocket + 前端

- Proposal 事件定义与发布
- ThreadDetailPage 提案卡片
- WorkItem 草案编辑器
- 提案审批视图

## 12. 与已有设计的关系

### 与 WorkItemTrack 的关系

`WorkItemTrack` 已标记为 DEPRECATED，被 ThreadTask 替代。本设计中的 Proposal 与 WorkItemTrack 的定位不同：

- WorkItemTrack 侧重"单个 WorkItem 的孵化过程"
- Proposal 侧重"讨论收敛后的结构化结论，可以产出多个跨项目 WorkItem"

### 与 thread-collaboration-to-dag-plan 的关系

该 spec 描述的"收敛层"正是本 Plan 要落地的内容：

- spec 中的 `summary / proposal / decision` → 本 Plan 的 ThreadProposal
- spec 中的 P1"Thread 收敛动作 MVP" → 本 Plan 的 Phase 1-2
- spec 中的 P3"协同到 DAG 的自动化增强" → 本 Plan 的 Materialize 流程

## 13. 明确不做的内容

1. 多人投票表决（第一阶段只需一人审批）
2. 提案版本树（驳回后直接修订，不保留历史版本）
3. 提案模板
4. 自动收敛（由 Agent 主动发起，不做基于轮次的自动触发）
5. 跨 Thread 的提案合并

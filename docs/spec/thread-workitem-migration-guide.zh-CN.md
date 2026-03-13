# Thread / WorkItem 术语迁移指南

> 本指南帮助前后端开发者理解 Thread + WorkItem 术语迁移的变更范围、兼容策略及升级步骤。
>
> 状态：历史
>
> 最后按代码核对：2026-03-13
>
> 当前实现状态：本文保留原迁移设计与落地偏差，用于解释“当时计划如何迁移，以及现在实际只落地了哪些部分”。现行命名治理请以 `naming-transition-thread-workitem.zh-CN.md` 为准。
>
> 重要说明：本文最初用于描述一次迁移目标。当前仓库只完成了其中一部分，不能把全文视为“全部已落地现状”。

## 变更总览

本次迁移引入两大变化：

1. **Thread 独立领域实体**：多人（多 AI + 多 human）共享讨论容器，区别于 ChatSession（1:1 direct chat）
2. **WorkItem 路由推广**：前端页面主入口切到 `/work-items`，后端 REST 仍保留 `/issues`

## 新增数据库表

| 表名 | 用途 |
|------|------|
| `threads` | Thread 主表 |
| `thread_messages` | Thread 消息 |
| `thread_participants` | Thread 参与者 |
| `thread_work_item_links` | Thread ↔ Issue 双向关联 |
| `thread_agent_sessions` | Thread 内 Agent 会话 |

当前实现不是通过 GORM AutoMigrate 自动创建，而是由 SQLite migration SQL 创建。

## 后端 API 变更

### 新增 Thread REST 端点

| 方法 | 路由 | 说明 |
|------|------|------|
| POST | `/threads` | 创建 Thread |
| GET | `/threads` | Thread 列表 |
| GET | `/threads/{id}` | Thread 详情 |
| PUT | `/threads/{id}` | 更新 Thread |
| DELETE | `/threads/{id}` | 删除 Thread |
| POST | `/threads/{id}/messages` | 发送消息 |
| GET | `/threads/{id}/messages` | 消息列表 |
| POST | `/threads/{id}/participants` | 添加参与者 |
| GET | `/threads/{id}/participants` | 参与者列表 |
| DELETE | `/threads/{id}/participants/{userID}` | 移除参与者 |

### 新增 Thread-WorkItem 关联端点

| 方法 | 路由 | 说明 |
|------|------|------|
| POST | `/threads/{id}/links/work-items` | 创建关联 |
| GET | `/threads/{id}/work-items` | 按 Thread 查关联 |
| DELETE | `/threads/{id}/links/work-items/{workItemID}` | 删除关联 |
| GET | `/issues/{id}/threads` | 按 WorkItem 反查 Thread |
| POST | `/threads/{id}/create-work-item` | 从 Thread 创建 WorkItem（自动关联） |

### 新增 Thread Agent 端点

| 方法 | 路由 | 说明 |
|------|------|------|
| POST | `/threads/{id}/agents` | 邀请 Agent |
| GET | `/threads/{id}/agents` | Agent 列表 |
| DELETE | `/threads/{id}/agents/{sessionID}` | 移除 Agent |

### 路由重定向

| 旧路由 | 新路由 | 说明 |
|--------|--------|------|
| `/issues` | `/work-items` | 前端路由重定向 |
| `/issues/new` | `/work-items/new` | 前端路由重定向 |
| `/issues/:id` | `/work-items` | 当前实现重定向到列表页，不保留详情 ID |
| `/flows` | `/work-items` | 前端路由重定向 |
| `/flows/:id` | `/work-items` | 当前实现重定向到列表页，不保留详情 ID |

后端 `/issues` REST API 完整保留，不重定向。

## 前端变更

### 路由变更

- `/work-items`、`/work-items/new`、`/work-items/:flowId` 为新主路由
- `/issues/*`、`/flows/*` 通过 `<Navigate>` 重定向到 `/work-items` 系列路由
- 其中旧详情页当前会跳回 `/work-items` 列表，而不是映射到 `/work-items/:flowId`
- 侧边栏导航项从 "Issues" 更名为 "Work Items"

### 新增类型（`apiV2.ts`）

```typescript
ThreadWorkItemLink     // Thread ↔ WorkItem 关联
ThreadAgentSession     // Thread 内 Agent 会话
```

### 新增 API Client 方法

```typescript
createThreadWorkItemLink(threadId, req)
listWorkItemsByThread(threadId)
deleteThreadWorkItemLink(threadId, workItemId)
listThreadsByWorkItem(workItemId)
createWorkItemFromThread(threadId, req)
inviteThreadAgent(threadId, req)
listThreadAgents(threadId)
removeThreadAgent(threadId, sessionId)
```

### 页面更新

- **ThreadDetailPage**: 新增 Linked Work Items 面板，支持创建/关联 WorkItem
- **FlowDetailPage**: 新增 Linked Threads 面板，显示反向关联的 Thread
- **FlowsPage**: 内部链接更新为 `/work-items/*`

## 兼容性说明

### 不受影响

- ChatSession（`/chat`）相关 API 和 WebSocket 完全不受影响
- 后端 `/issues` REST API 保持完整可用
- Go 内部 struct 名称（`Issue`、`Step`、`Execution`、`Artifact`）和数据库表名不变
- 现有数据无需迁移

### 注意事项

- 当前代码没有在应用层显式清理 `thread_work_item_links` 和 `thread_agent_sessions`
- 当前 SQLite 打开了 `PRAGMA foreign_keys=ON`，因此删除行为依赖数据库外键约束，而不是本文原始版本描述的“应用层先清理再删除”
- 如果未来切换存储实现，删除策略需要重新明确
- `thread_work_item_links` 有 UNIQUE(thread_id, work_item_id) 约束
- `thread_agent_sessions` 有 UNIQUE(thread_id, agent_profile_id) 约束

## 升级步骤

1. **启动服务**：确认 SQLite migrations 已执行
2. **验证前端**：访问 `/work-items` 确认页面路由正常；访问 `/threads` 确认 Thread 功能可用
3. **验证后端**：继续使用 `/issues` 作为 WorkItem 的 REST 主入口
4. **兼容旧书签**：旧 `/issues`、`/flows` 页面入口会跳转到 `/work-items`

## 当前实现与原迁移目标的差异

- 已实现：Thread 相关后端 REST、WebSocket、前端页面、Thread-Agent runtime 基础能力
- 已实现：前端主工作台入口切到 `/work-items`
- 未实现：后端 `/work-items` REST alias
- 已偏离原方案：旧详情页不再重定向到 `/work-items/:id`，而是直接回到 `/work-items`
- 已偏离原方案：数据库建表方式为 migration SQL，不是 AutoMigrate

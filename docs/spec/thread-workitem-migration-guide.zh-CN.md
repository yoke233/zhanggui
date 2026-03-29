# Thread / WorkItem 术语迁移指南

> 本指南帮助前后端开发者理解 Thread + WorkItem 术语迁移的变更范围、兼容策略及升级步骤。
>
> 状态：历史
>
> 最后按代码核对：2026-03-29
>
> 当前实现状态：本文保留原迁移设计与落地偏差，用于解释“当时计划如何迁移，以及后来代码实际如何继续演进”。当前 public surface 已比本文原始迁移目标更进一步：前端与后端对外主入口都已经切到 `/work-items`；现行命名治理请以 `README.md` 与 `naming-transition-thread-workitem.zh-CN.md` 为准。
>
> 重要说明：本文最初用于描述一次迁移目标。当前仓库只完成了其中一部分，不能把全文视为“全部已落地现状”。

## 变更总览

本次迁移引入两大变化：

1. **Thread 独立领域实体**：多人（多 AI + 多 human）共享讨论容器，区别于 ChatSession（1:1 direct chat）
2. **WorkItem 路由推广**：前后端对外主入口都切到 `/work-items`

## 新增数据库表

| 表名 | 用途 |
|------|------|
| `threads` | Thread 主表 |
| `thread_messages` | Thread 消息 |
| `thread_members` | Thread 成员（human + agent 统一模型） |
| `thread_work_item_links` | Thread ↔ Issue 双向关联 |

当前实现通过 `internal/adapters/store/sqlite/schema.go` 中的 GORM `AutoMigrate` 创建。

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
| GET | `/threads/{id}/events` | Thread 事件流 |
| POST | `/threads/{id}/participants` | 添加参与者 |
| GET | `/threads/{id}/participants` | 参与者列表 |
| DELETE | `/threads/{id}/participants/{userID}` | 移除参与者 |

### 新增 Thread-WorkItem 关联端点

| 方法 | 路由 | 说明 |
|------|------|------|
| POST | `/threads/{id}/links/work-items` | 创建关联 |
| GET | `/threads/{id}/work-items` | 按 Thread 查关联 |
| DELETE | `/threads/{id}/links/work-items/{workItemID}` | 删除关联 |
| GET | `/work-items/{id}/threads` | 按 WorkItem 反查 Thread |
| POST | `/threads/{id}/create-work-item` | 从 Thread 创建 WorkItem（自动关联） |

补充说明（按 2026-03-13 当前代码）：

- `POST /threads/{id}/create-work-item` 现在要求满足以下其一：
  - 显式提供 `body`
  - 或由后端回退到 `Thread.Title`
- 当请求未提供 `body` 时，后端当前会自动使用 `Thread.Title` 作为 WorkItem `body`
- 创建出的 WorkItem 会记录 `metadata.source_thread_id`
- 当前实现中 `metadata.source_type` 统一记录为 `thread_manual`

### 新增 Thread Agent 端点

| 方法 | 路由 | 说明 |
|------|------|------|
| POST | `/threads/{id}/agents` | 邀请 Agent |
| GET | `/threads/{id}/agents` | Agent 列表 |
| DELETE | `/threads/{id}/agents/{sessionID}` | 移除 Agent |

### 原迁移设计中的路由重定向（当前已退出）

| 旧路由 | 新路由 | 说明 |
|--------|--------|------|
| `/issues` | `/work-items` | 原方案中的前端路由重定向；当前代码已移除该旧入口 |
| `/issues/new` | `/work-items/new` | 原方案中的前端路由重定向；当前代码已移除该旧入口 |
| `/issues/:id` | `/work-items/:id` | 原方案中的前端详情跳转；当前代码已移除该旧入口 |
| `/flows` | `/work-items` | 原方案中的前端路由重定向；当前代码已移除该旧入口 |
| `/flows/:id` | `/work-items/:id` | 原方案中的前端详情跳转；当前代码已移除该旧入口 |

后端对外主 REST 已经切到 `/work-items`；当前前端与 API client 也已直接以 `/work-items` 为主，不再保留 `/issues` / `/flows` 页面兼容路由。

## 前端变更

### 路由变更

- `/work-items`、`/work-items/new`、`/work-items/:workItemId` 为新主路由
- 旧 `/issues/*`、`/flows/*` 兼容页面当前已退出工作台
- 侧边栏导航项从 "Issues" 更名为 "Work Items"

### 新增类型（`apiV2.ts`）

```typescript
ThreadWorkItemLink     // Thread ↔ WorkItem 关联
ThreadMember           // Thread 内统一成员（human + agent）
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
- **WorkItemDetailPage**: 新增 Linked Threads 面板，显示反向关联的 Thread
- **WorkItemsPage**: 作为主工作台入口承载 `/work-items/*`

## 兼容性说明

### 不受影响

- ChatSession（`/chat`）相关 API 和 WebSocket 完全不受影响
- 内部持久化表名仍保留 `issues` / `steps` / `executions`
- 部分 handler/request struct 仍保留 `issue` 兼容命名
- 现有数据无需迁移

### 注意事项

- Thread 删除前当前会显式调用 `CleanupThread(threadID)` 清理 ACP runtime
- `thread_work_item_links` 的父对象删除清理当前仍未统一收口
- `thread_members(kind=agent)` 已替代本文旧版中的 `thread_agent_sessions`
- 如果未来切换存储实现，删除策略需要重新明确
- `thread_work_item_links` 有 UNIQUE(thread_id, work_item_id) 约束
- agent 成员的唯一性当前由 runtime / store 行为共同约束，不应再把 `thread_agent_sessions` 当成现行表结构

## 升级步骤

1. **启动服务**：确认 SQLite migrations 已执行
2. **验证前端**：访问 `/work-items` 确认页面路由正常；访问 `/threads` 确认 Thread 功能可用
3. **验证后端**：使用 `/work-items` 作为 WorkItem 的 REST 主入口
4. **更新旧书签**：当前工作台已不再提供 `/issues`、`/flows` 页面入口，应统一改用 `/work-items`

## 当前实现与原迁移目标的差异

- 已实现：Thread 相关后端 REST、WebSocket、前端页面、Thread-Agent runtime 基础能力
- 已实现：前后端对外主工作台入口都切到 `/work-items`
- 已偏离原方案：Thread agent 会话最终落在统一的 `thread_members` 模型，而不是独立 `thread_agent_sessions` 表
- 已偏离原方案：旧 `/issues`、`/flows` 兼容页面已退出当前工作台
- 已偏离原方案：数据库建表由 GORM AutoMigrate 驱动

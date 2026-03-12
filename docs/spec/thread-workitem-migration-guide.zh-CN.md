# Thread / WorkItem 术语迁移指南

> 本指南帮助前后端开发者理解 Thread + WorkItem 术语迁移的变更范围、兼容策略及升级步骤。

## 变更总览

本次迁移引入两大变化：

1. **Thread 独立领域实体**：多人（多 AI + 多 human）共享讨论容器，区别于 ChatSession（1:1 direct chat）
2. **WorkItem 路由推广**：`/work-items` 成为主入口，`/issues` 和 `/flows` 重定向至 `/work-items`

## 新增数据库表

| 表名 | 用途 |
|------|------|
| `threads` | Thread 主表 |
| `thread_messages` | Thread 消息 |
| `thread_participants` | Thread 参与者 |
| `thread_work_item_links` | Thread ↔ Issue 双向关联 |
| `thread_agent_sessions` | Thread 内 Agent 会话 |

所有表通过 GORM AutoMigrate 自动创建，无需手动执行 DDL。

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
| `/issues/:id` | `/work-items/:id` | 前端路由重定向 |
| `/flows` | `/work-items` | 前端路由重定向 |
| `/flows/:id` | `/work-items/:id` | 前端路由重定向 |

后端 `/issues` REST API 完整保留，不重定向。

## 前端变更

### 路由变更

- `/work-items`、`/work-items/new`、`/work-items/:flowId` 为新主路由
- `/issues/*`、`/flows/*` 通过 `<Navigate>` 重定向到 `/work-items/*`
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

- Thread 删除前需先清理 `thread_work_item_links` 和 `thread_agent_sessions`（应用层清理，非 CASCADE）
- Issue 删除前需先清理 `thread_work_item_links`
- `thread_work_item_links` 有 UNIQUE(thread_id, work_item_id) 约束
- `thread_agent_sessions` 有 UNIQUE(thread_id, agent_profile_id) 约束

## 升级步骤

1. **拉取代码**：合并 `codex/thread-workitem-terminology-transition` 分支
2. **启动服务**：GORM AutoMigrate 自动创建新表
3. **验证**：访问 `/work-items` 确认路由正常；访问 `/threads` 确认 Thread 功能可用
4. **前端书签**：如有旧 `/issues` 书签会自动重定向到 `/work-items`

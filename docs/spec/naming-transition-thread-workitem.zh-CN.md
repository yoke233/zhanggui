# 命名迁移规范：Thread / WorkItem

> 本文档定义系统对外术语升级的映射矩阵、兼容策略与淘汰周期。

## 命名映射矩阵

| 内部 Go struct / 表名 | API 外部名 | UI 显示名 | 说明 |
|----------------------|-----------|----------|------|
| `Issue` | `WorkItem` | Work Item | Issue 表/struct 暂不重命名；API 新增 `/work-items` alias |
| `Step` | `Action` | Action | Step 表/struct 暂不重命名；API payload 新增 alias 字段 |
| `Execution` | `Run` | Run | Execution 表/struct 暂不重命名；API payload 新增 alias 字段 |
| `Artifact` | `Deliverable` | Deliverable | Artifact 表/struct 暂不重命名；API payload 新增 alias 字段 |
| `ChatSession` | `ChatSession` | Chat | **不映射为 Thread**；保持 1:1 direct chat 概念 |
| `Thread`（新增） | `Thread` | Thread | 独立领域实体，多 AI + 多 human 共享讨论 |

## ChatSession 保持 direct chat 概念，不映射为 Thread

`ChatSession` 与 `Thread` 是两个并列的交互概念：

- `ChatSession`：1 AI + 1 human 的 direct chat，保留现有 `/chat` API 与 `chat.send` WebSocket 协议
- `Thread`：多 AI + 多 human 的共享讨论容器，新增 `/threads` API 与 `thread.send` WebSocket 协议

两者不共享主键、时间线或 runtime session。

## HTTP 路由兼容策略

### 新增路由

| 路由 | 说明 |
|------|------|
| `GET /threads` | Thread 列表 |
| `POST /threads` | 创建 Thread |
| `GET /threads/{id}` | Thread 详情 |
| `PUT /threads/{id}` | 更新 Thread |
| `DELETE /threads/{id}` | 删除 Thread |
| `GET /work-items` | 等价于 `GET /issues`，返回相同数据 |
| `GET /work-items/{id}` | 等价于 `GET /issues/{id}` |

### 保留路由（兼容期内继续可用）

| 路由 | 说明 |
|------|------|
| `GET /chat/sessions` | ChatSession 列表（保留） |
| `POST /chat` | 发送 chat 消息（保留） |
| `GET /issues` | Issue 列表（保留） |
| `GET /issues/{id}` | Issue 详情（保留） |

### 兼容周期

- **Phase 1（当前）**：新旧路由同时可用，旧路由不发出 deprecation 警告
- **Phase 2（未来）**：旧路由返回 `Deprecation` header
- **Phase 3（未来）**：旧路由移除

## WebSocket 协议兼容策略

### 新增消息类型

| 消息类型 | 说明 |
|---------|------|
| `thread.send` | 向 Thread 发送消息（payload 包含 `thread_id`） |
| `subscribe_thread` | 订阅 Thread 事件流 |
| `unsubscribe_thread` | 取消订阅 Thread 事件流 |

### 保留消息类型

| 消息类型 | 说明 |
|---------|------|
| `chat.send` | 向 ChatSession 发送消息（保留 `session_id` 语义） |
| `subscribe_chat_session` | 订阅 ChatSession（保留） |

### 关键区别

- `thread.send` 的 payload 使用 `thread_id`，不使用 `session_id`
- `chat.send` 的 payload 继续使用 `session_id`
- 两者不互为 alias，各走独立的处理链路

## JSON Payload 字段策略

### 新增 alias 字段（响应中同时返回新旧名）

暂不在 API 响应中同时返回新旧字段名。当前策略：

- `/issues` 返回 Issue 字段名（`issue_id`, `step_id`, `execution_id`, `artifact_id`）
- `/work-items` 返回相同数据，字段名与 `/issues` 一致（内部名保持不变）
- `/threads` 返回 Thread 独立字段名（`thread_id`, `message_id`, `participant_id`）

### 错误码策略

- Thread 相关错误使用 `THREAD_*` 前缀（`THREAD_NOT_FOUND`, `CREATE_THREAD_FAILED`）
- Issue/WorkItem 相关错误继续使用 `ISSUE_*` 前缀
- ChatSession 相关错误继续使用 `CHAT_*` / `SESSION_*` 前缀

## 内部 Go struct 重命名策略

当前阶段（Wave 1-3）**不强制**重命名内部 Go struct 和数据库表名：

- `Issue` struct 和 `issues` 表保持不变
- `Step` struct 和 `steps` 表保持不变
- `Execution` struct 和 `executions` 表保持不变
- `Artifact` struct 和 `artifacts` 表保持不变

新增的 `Thread` 直接以新名称建模，不存在旧名遗留。

## 前端类型 alias 策略

在 `web/src/types/apiV2.ts` 中新增类型 alias：

```typescript
// 新领域类型
export interface Thread { ... }
export interface ThreadMessage { ... }
export interface ThreadParticipant { ... }

// 术语 alias（指向现有类型）
export type WorkItem = Issue;
export type Action = Step;
export type Run = Execution;
export type Deliverable = Artifact;
```

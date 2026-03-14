# Thread AI Runtime 模型规格

> 状态：部分实现
>
> 最后按代码核对：2026-03-14
>
> 对应实现：`internal/runtime/agent/thread_session_pool.go`、`internal/adapters/http/thread.go`、`internal/adapters/http/event.go`
>
> 补充边界说明：`thread.send` 当前仍是 best-effort routing，不是可靠 delivery；详见 `thread-message-delivery-deferred.zh-CN.md`

## 概述

Thread 的多 agent 能力已基于现有 ACP session pool 基础设施落地，为每个 Thread 管理独立的 agent session 集合。Thread 与 ChatSession 共享同一个 ACP session pool，但 session 实例互不干扰。

当前文档描述的是“现行行为 + 少量未来预留”，不是纯设计稿。需要注意：

- Thread agent 的 CRUD、runtime 启停、事件广播已经实现
- `thread.send` 与 `POST /threads/{id}/messages` 当前都支持 `target_agent_id`
- 当前默认路由语义不是广播，而是 `mention_only`；只有显式 `target_agent_id`、或 Thread `metadata.agent_routing_mode` 设置为 `broadcast` / `auto` 时，消息才会 fanout 给 agent

## 架构模型

```
Thread
  ├── Agent Session 1 → ACP Client 1 → ACP Session (stdio)
  ├── Agent Session 2 → ACP Client 2 → ACP Session (stdio)
  └── Agent Session N → ACP Client N → ACP Session (stdio)

ChatSession (独立)
  └── LeadChatService → ACP Client → ACP Session (1:1 direct chat)
```

- 每个 Thread 可关联 N 个 agent
- 每个 agent 对应一个独立 ACP Client 实例 + 一个 ACP Session
- Thread 的 agent session 与 ChatSession 的 agent session **互不共享**

## 数据模型

```sql
CREATE TABLE thread_members (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    thread_id         INTEGER NOT NULL,
    kind              TEXT    NOT NULL,            -- human | agent
    user_id           TEXT    NOT NULL DEFAULT '',
    agent_profile_id  TEXT    NOT NULL DEFAULT '',
    role              TEXT    NOT NULL DEFAULT 'member',
    status            TEXT    NOT NULL DEFAULT '',
    agent_data        TEXT,
    joined_at         DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_active_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
```

### 字段说明

| 字段 | 类型 | 说明 |
|------|------|------|
| `id` | int64 | 自增主键 |
| `thread_id` | int64 | 关联的 Thread ID |
| `kind` | string | 当前成员类型：`human` 或 `agent` |
| `agent_profile_id` | string | agent 成员对应的 profile ID |
| `status` | string | agent 生命周期状态；human 成员通常为空 |
| `agent_data` | JSON | ACP session ID、turn_count、tokens、progress 等运行时数据 |
| `joined_at` | datetime | agent 加入时间 |
| `last_active_at` | datetime | 最后活跃时间 |

### Agent Session 状态机

```
joining → booting → active → paused → active (恢复)
                           → left   (主动退出)
                           → failed (异常退出)
```

- `joining`: 正在创建 ACP session
- `booting`: ACP session 已启动，正在发送 thread boot prompt
- `active`: ACP session 已就绪，可收发消息
- `paused`: 暂停响应（保持 session 不释放）
- `left`: 正常退出，ACP session 已释放
- `failed`: 异常退出，需人工介入或自动重连

## 消息路由策略

当前实现有三种路由模式：

1. `mention_only`（默认）
   - 没有 `target_agent_id` 时，只写入 `thread_messages`，不自动 fanout
   - 有 `target_agent_id` 时，只投递给指定 active agent
2. `broadcast`
   - 投递给该 Thread 下所有 active agent
3. `auto`
   - 先按 profile capability / skill / name / role 评分自动选路
   - 若没有命中，则回退到广播

当前实现还会把路由结果写回消息 metadata：

- 定向消息：`target_agent_id`
- 自动选路：`auto_routed_to`

补充说明：

- 当前“发送成功”只表示消息已写入 `thread_messages`
- 当前没有消息级 delivery ledger，也没有自动重试语义
- `thread.agent_failed` 是观测事件，不是可靠投递真相源

## ACP Session 生命周期

1. **Thread 创建时**：不自动启动 agent session
2. **邀请 agent 加入**：`POST /threads/{id}/agents`
   - 创建 `thread_members(kind=agent)` 记录
   - 通过 ACP session pool 创建 session
   - 更新 status / agent_data
3. **移除 agent**：`DELETE /threads/{id}/agents/{agentSessionID}`
   - 释放对应 ACP session
   - 更新 status = left
4. **Thread 删除前**：当前代码路径没有单独的 `close` 端点；如要安全删除，建议先移除相关 agent session

说明：

- runtime pool 内部具备 `CleanupThread(threadID)` 能力
- 当前 HTTP `DELETE /threads/{id}` 已经会先调用 `CleanupThread(threadID)`，再删除 Thread 记录
- 但 `Thread` 与其关联 link 的删除一致性仍未形成完整契约，详见 `thread-workitem-linking.zh-CN.md`

## API 端点

| Method | Path | 说明 |
|--------|------|------|
| `POST` | `/threads/{threadID}/agents` | 邀请 agent 加入 Thread |
| `GET` | `/threads/{threadID}/agents` | 列出 Thread 的 agent sessions |
| `DELETE` | `/threads/{threadID}/agents/{agentSessionID}` | 移除 agent |

### POST /threads/{threadID}/agents 请求体

```json
{
  "agent_profile_id": "worker-claude"
}
```

## 与 ChatSession Runtime 的关系

| 维度 | ChatSession | Thread |
|------|-------------|--------|
| 模式 | 1 AI + 1 human (direct chat) | N AI + N human (shared discussion) |
| 入口服务 | LeadChatService | ThreadAgentService |
| Session 管理 | 单一 ACP session per chat | 多个 ACP session per thread |
| 消息路由 | 直连 | 当前为广播到所有 active agent |
| Session Pool | 共享基础设施 | 共享基础设施 |
| Session 实例 | 独立，不与 Thread 共享 | 独立，不与 ChatSession 共享 |

# Thread AI Runtime 模型规格

## 概述

Thread 的多 agent 能力基于现有 ACP session pool 基础设施，为每个 Thread 管理独立的 agent session 集合。Thread 与 ChatSession 共享同一个 ACP session pool，但 session 实例互不干扰。

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
CREATE TABLE thread_agent_sessions (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    thread_id         INTEGER NOT NULL REFERENCES threads(id),
    agent_profile_id  TEXT    NOT NULL,
    acp_session_id    TEXT    NOT NULL DEFAULT '',
    status            TEXT    NOT NULL DEFAULT 'joining',
    joined_at         DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_active_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_tas_thread ON thread_agent_sessions(thread_id);
CREATE UNIQUE INDEX idx_tas_thread_profile ON thread_agent_sessions(thread_id, agent_profile_id);
```

### 字段说明

| 字段 | 类型 | 说明 |
|------|------|------|
| `id` | int64 | 自增主键 |
| `thread_id` | int64 | 关联的 Thread ID |
| `agent_profile_id` | string | 引用 `agent_profiles` 中的 profile ID |
| `acp_session_id` | string | 对应 ACP session pool 中的 session ID |
| `status` | string | 生命周期状态 |
| `joined_at` | datetime | agent 加入时间 |
| `last_active_at` | datetime | 最后活跃时间 |

### Agent Session 状态机

```
joining → active → paused → active (恢复)
                 → left   (主动退出)
                 → failed (异常退出)
```

- `joining`: 正在创建 ACP session
- `active`: ACP session 已就绪，可收发消息
- `paused`: 暂停响应（保持 session 不释放）
- `left`: 正常退出，ACP session 已释放
- `failed`: 异常退出，需人工介入或自动重连

## 消息路由策略

第一版采用显式 `target_agent_id`（@mention）：

1. 用户发送 `thread.send` 时可指定 `target_agent_id`
2. 若指定：查找对应的 `thread_agent_sessions` 记录 → 转发消息到对应 ACP session
3. 若未指定：走默认路由规则（当前版本：主 agent 优先，即第一个 active 的 agent）
4. Agent 回复统一写入 `thread_messages` 时间线，`role = "agent"`，`sender_id = agent_profile_id`

## ACP Session 生命周期

1. **Thread 创建时**：不自动启动 agent session
2. **邀请 agent 加入**：`POST /threads/{id}/agents`
   - 创建 `thread_agent_sessions` 记录（status = joining）
   - 通过 ACP session pool 创建 session
   - 更新 status = active 并记录 acp_session_id
3. **移除 agent**：`DELETE /threads/{id}/agents/{agentSessionID}`
   - 释放对应 ACP session
   - 更新 status = left
4. **Thread 关闭时**：逐个释放所有 active/paused 的 ACP session

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
| 消息路由 | 直连 | 显式 @mention / 默认路由 |
| Session Pool | 共享基础设施 | 共享基础设施 |
| Session 实例 | 独立，不与 Thread 共享 | 独立，不与 ChatSession 共享 |

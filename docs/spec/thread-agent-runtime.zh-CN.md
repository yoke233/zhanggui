# Thread Agent Runtime 现状规格

> 状态：部分实现
>
> 最后按代码核对：2026-03-17
>
> 对应实现：
> - `internal/runtime/agent/thread_session_pool.go`
> - `internal/runtime/agent/thread_boot.go`
> - `internal/adapters/http/thread.go`
> - `internal/adapters/http/event.go`
>
> 补充边界说明：`thread.send` 当前仍属于 best-effort routing，
> 不是可靠消息投递协议；可靠 delivery 不在本文范围内。

## 一句话结论

Thread agent runtime 已经是当前系统的现行能力：

- 每个 Thread 可维护多 agent session
- agent session 基于 ACP 启动
- 邀请、启动、移除、状态更新、事件广播都已落地
- Thread 与 ChatSession 共用底层 ACP 基础设施，但 session 彼此独立

它当前处于“现行主线 + 少量边界未收口”的状态，因此标记为“部分实现”。

## 当前模型

当前运行时关系可以理解为：

```text
Thread
  -> ThreadMember(kind=agent)
    -> ThreadSessionPool
      -> ACP Client
      -> ACP Session

ChatSession
  -> LeadAgent / LeadChatService
    -> ACP Client
    -> ACP Session
```

关键点：

- Thread 和 ChatSession 共享 ACP 基础设施
- 但不会共享同一个 ACP session
- Thread 内 agent 是多会话模型
- ChatSession 仍是 direct chat 的单会话模型

## 数据模型现状

当前 thread agent 仍统一落在 `thread_members` 表中。

关键字段包括：

- `thread_id`
- `kind`
- `user_id`
- `agent_profile_id`
- `role`
- `status`
- `agent_data`
- `joined_at`
- `last_active_at`

当前事实：

- `kind=human` 与 `kind=agent` 共用同一成员模型
- 前端的 `ThreadParticipant` / `ThreadAgentSession`
  只是围绕 `ThreadMember` 的使用视图
- agent runtime 状态主要保存在 `status` 与 `agent_data`

## Agent 状态机

当前文档可按下面的状态机理解：

```text
joining -> booting -> active
                    -> paused -> active
                    -> left
                    -> failed
```

其中：

- `joining`：准备创建 session
- `booting`：session 已启动，正在完成 thread boot
- `active`：可以接收 Thread 消息或 task dispatch
- `paused`：保留 session 但暂停处理
- `left`：正常离开
- `failed`：异常失败

当前实现已经覆盖：

- 邀请时进入启动流程
- 启动失败会进入失败路径
- 移除时进入离开路径

## 当前消息路由

当前 Thread 消息路由有三种模式：

- `mention_only`
- `broadcast`
- `auto`

### `mention_only`

这是当前默认模式。

行为是：

- 没有 `target_agent_id` 时，只写入 `thread_messages`
- 有 `target_agent_id` 时，只尝试投递到指定 active agent

### `broadcast`

行为是：

- fanout 到该 Thread 下所有 active agent

### `auto`

行为是：

- 尝试按 profile/capability/name/role 做选路
- 未命中时再回退到广播

### metadata 回写

当前系统会把路由结果写回消息 metadata，例如：

- `target_agent_id`
- `auto_routed_to`

因此前端已经可以展示“这条消息投给了谁”。

## Session 生命周期

### Thread 创建

当前 Thread 创建时不会自动启动 agent session。

### 邀请 agent

当前入口：

- `POST /threads/{threadID}/agents`

当前行为：

1. 校验 Thread 存在
2. 创建或更新 `thread_members(kind=agent)`
3. 通过 `ThreadSessionPool` 启动 ACP session
4. 注入 thread boot 上下文与 workspace
5. 更新成员状态
6. 通过事件广播运行时变化

### 移除 agent

当前入口：

- `DELETE /threads/{threadID}/agents/{agentSessionID}`

当前行为：

- 如果 runtime pool 可用，则优先走优雅移除
- 否则退化为 DB 状态更新
- 成员最终会进入 `left` 或移除后的静态状态

### Thread 删除

当前删除 Thread 时，代码会先尝试清理 thread runtime，
再删除 Thread 记录。

因此“先关 session，再删 Thread”已经不是纯建议，而是当前删除链路的一部分。

## 当前 boot 上下文

当前 Thread agent 启动时，运行时已经会注入：

- Thread 基础上下文
- Thread workspace cwd
- `.context.json` 相关信息
- signal token / server addr 等任务协作能力

这意味着 Thread agent 当前不是裸 session，而是带有明确 Thread 语义的
预热上下文。

## API 现状

当前已实现端点：

| Method | Path | 说明 |
|------|------|------|
| `POST` | `/threads/{threadID}/agents` | 邀请 agent |
| `GET` | `/threads/{threadID}/agents` | 列出当前 agent members |
| `DELETE` | `/threads/{threadID}/agents/{agentSessionID}` | 移除 agent |

当前已配合实现的 Thread 能力还包括：

- `POST /threads/{threadID}/messages`
- `GET /threads/{threadID}/messages`
- `GET /threads/{threadID}/events`

也就是说，agent runtime 已经与消息模型和事件流整合，不是独立孤岛。

## 与 ChatSession 的关系

| 维度 | ChatSession | Thread |
|------|------|------|
| 交互模式 | 1 human + 1 AI | N human + N agent |
| 服务入口 | LeadChatService | ThreadSessionPool / Thread runtime |
| 会话形态 | 单 session | 多 session |
| 路由方式 | direct chat | mention_only / broadcast / auto |
| 工作区 | chat 上下文为主 | Thread workspace + context refs |

重要边界：

- ChatSession 不会自动等价为 Thread
- Thread 也不是 ChatSession 的多用户版别名
- 两者只在“可 crystallize”这件事上发生显式连接

## 当前实现边界

以下内容已经落地：

- 多 agent session 模型
- ACP 启动
- agent 邀请、列出、移除
- Thread 消息路由模式
- metadata 回写
- workspace 注入
- boot 上下文注入
- 删除前 cleanup

以下内容仍不应被写成“强可靠已完成”：

- 消息投递仍是 best-effort
- 失败事件不是 delivery ledger
- 更完整的 pause/resume 管理面仍在演进

## 推荐搭配阅读

1. `thread-workspace-context.zh-CN.md`
2. `thread-task-dag.zh-CN.md`
3. `thread-workitem-linking.zh-CN.md`
4. `thread-message-delivery-deferred.zh-CN.md`

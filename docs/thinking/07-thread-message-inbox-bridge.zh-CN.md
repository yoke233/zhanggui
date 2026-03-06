# Thread / Message / Inbox / Bridge：Agent 协作消息模型收敛

> **补充**: [06-Agent 工作空间](06-agent-workspace.zh-CN.md) — 本文不替代 06，而是把其中最容易失真的”消息 / 线程 / 群组 / 外部桥接”部分单独收敛为可实现模型。
> **目标**: 在不把系统做成完整 IM 的前提下，支持多人讨论、`@human`、Thread 上下文共享、以及 Slack / Discord / Telegram 这类外部群组桥接。
> **参考**: [IronClaw 架构学习](ironclaw-architecture-study.zh-CN.md) — 本文额外吸收了 IronClaw 在 `ChannelManager`、`IncomingMessage` / `OutgoingResponse` / `StatusUpdate` 分层、conversation metadata、以及 thread hydration 上的已验证做法。
> **命名约定**: 本文写于 Actor→Agent 改名之前，文中 `actor_*` 表名/字段名在实施时统一为 `agent_*`（见 [09-migration-roadmap](09-migration-roadmap.zh-CN.md)）。

## 为什么要单独收敛这一层

06 解决了“大方向”问题：Agent 要从固定 stage 变成常驻 Actor，通过 Inbox + Gateway 协作。

但一旦进入具体交互，很快会遇到三个容易混淆的问题：

1. **`@两个人` 怎么表示** — 如果 `actor_messages.to_id` 只能写一个接收者，就必须存两条一模一样的消息。
2. **外部群组桥接会不会重复发** — 如果“多人投递”和“消息本体”是同一条记录，桥接到 Slack/Discord 时就会重复同步。
3. **Thread 元数据放哪** — 如果没有独立 thread 元数据，TL 只能靠 Memory 猜“现在有哪些讨论在进行、哪些人属于哪个讨论”。

结论不是“做复杂了”，而是：

**原来把消息内容、接收者投递、thread 元数据、外部桥接四件事揉在一起了。拆开以后，表面上多 1~2 张表，实际模型更简单。**

## 设计目标

### 要支持的能力

- 点对点消息
- 一条消息投递给多人
- `@human`（Human 作为虚拟 Actor）
- Thread 级上下文共享
- Thread 可选桥接到外部平台群组
- 默认仍然保持 `Worker -> TL` 的路由约束
- 外部平台上下文可进入 prompt，但不污染消息正文

### 明确不做的事

- 不做通用 IM 系统
- 不做任意 Actor 间自由私聊
- 不做复杂群权限、已读回执、typing、emoji reaction 等聊天产品能力
- 不要求每个 Actor 在 Slack/Discord 上都有独立机器人身份

## 从 IronClaw 学到的 5 个约束

### 1. 输入消息、最终回复、状态更新不是一回事

IronClaw 在 `src/channels/channel.rs` 里把三类东西明确拆开：

- `IncomingMessage`：进入系统的一条输入
- `OutgoingResponse`：最终要回给对端的一条回复
- `StatusUpdate`：thinking / tool started / auth required 这类过程态更新

这对 07 的启发很直接：

> `actor_messages` 只承载“会进入 thread 时间线的正式消息”，不承载“处理中状态”。

因此，thinking、tool 执行、审批请求、SSE/WebSocket 提示这类事件，应该走独立的 `StatusUpdate` 通道，而不是写入 `actor_messages`。

### 2. 外部 thread ID 不能裸用，必须带作用域

IronClaw 在 `src/agent/session_manager.rs` 里不是单独用 `external_thread_id` 做键，而是用：

`(user_id, channel, external_thread_id)`

这说明一个很重要的工程事实：

> 外部平台给你的 thread/channel ID，通常只能在某个 channel / workspace / user 作用域内解释，不能当全局主键裸用。

因此，07 的 bridge 映射不能只有一个 `bridge_id` 字段，还必须保留最小作用域信息。

### 3. thread 元数据要可扩展，别过早定死宽表

IronClaw 的 conversation 支持 metadata JSON，并在此之上轻量抽出 `thread_type`、title 等信息用于列表和行为分支。

对 07 的启发是：

- thread 应该有几个稳定主字段
- 但仍然要保留 `metadata` / `bridge_metadata` 这种扩展位

否则后面一加外部平台特有属性、桥接策略、摘要信息，就得不停改表。

### 4. channel-specific 上下文应该结构化注入，不应该塞回正文

IronClaw 的 channel 会把 `sender`、`sender_uuid`、`group` 这类信息通过 `conversation_context(...)` 注入推理上下文，而不是硬拼进用户内容。

这说明 07 也应坚持：

> 外部平台的 sender / group / mention / external_message_id 这类信息进入 `metadata` 和 session context，不回写进 message content。

### 5. thread 必须能从存储层 hydrate，不能只活在内存里

IronClaw 在处理带 `thread_id` 的消息时，会先尝试从存储层 hydrate 历史 thread，再把它注册回运行时 session。

对 07 的启发是：

> Actor runtime 不能依赖 TL 或某个 Actor 的短期 memory 去“记住有哪些 thread 存在”。thread 必须可从数据库独立恢复。

## 核心判断

### 1. Thread 是上下文容器，不是默认广播容器

`thread_id` 的含义只有一个：

> 这条消息属于哪段讨论上下文。

它**不自动意味着**“发给 thread 里的所有人”。

也就是说：

- `thread_id = T`：消息进入讨论 T 的时间线
- `to = [A, B]`：消息投递给 A、B 两个接收者

这两个维度必须分开，否则“讨论记录共享”和“消息实际送达”会再次混在一起。

### 2. 一条消息只存一次，投递可以有很多次

多人消息的正确模型不是“存多条 message”，而是：

- `actor_messages`：消息本体，只存一条
- `actor_inbox`：每个接收者一条投递记录

这样可以同时满足：

- 内容去重
- 每个接收者有自己的处理状态
- 外部桥接只同步一次

### 3. Thread 要有自己的元数据表

如果 thread 只是 `actor_messages.thread_id` 上的一个裸字段，短期看很省事，但 TL 很快会遇到两个问题：

- 我有哪些开放中的讨论？
- 这个讨论对应哪个 issue、桥接到哪个外部 channel？

因此，**`actor_threads` 应保留**，但保持轻量，只承载讨论元数据，不承载复杂成员状态机。

### 4. 外部桥接是可选通道，不是核心模型的一部分

Slack / Discord / Telegram 都是 `GroupBridge` 插件实现。

内部仍然以：

`Thread -> Message -> Inbox Delivery`

为核心。外部桥接只是：

- 把 thread 时间线同步到外部群组
- 把外部 webhook 转成内部 IncomingMessage

不桥接时，内部模型完全成立。

### 5. 状态更新是侧带流，不进入正式时间线

对外展示“正在思考 / 调工具 / 等审批 / 已同步到外部平台”这些过程态信息时，应该走 side channel：

- Web SSE / WebSocket
- MCP streaming event
- 外部 bridge 的临时状态提示

只有真正需要进入讨论上下文的内容，才进入 `actor_messages`。

## 最终模型

### 概念分层

```
Thread   = 讨论元数据（标题、参与者快照、桥接信息、状态）
Message  = 一条消息内容，属于某个 Thread 或直接消息
Inbox    = 这条消息投递给了谁，各自处理到了什么状态
Bridge   = Thread 与外部群组的可选双向同步器
Status   = 处理过程中的临时状态流，不进入正式消息存储
```

### 典型场景如何表达

| 场景 | 表达方式 | 需要新机制吗 |
|---|---|---|
| `@一个人` | `send_message(to="coder-01", thread_id=T, content=...)` | 不需要 |
| `@两个人` | `send_message(to=["coder-01", "coder-02"], thread_id=T, content=...)` | 不需要 |
| `@human` | `send_message(to="virtual:user-123", thread_id=T, content=...)` | 不需要，Human 是虚拟 Actor |
| 看完整讨论 | 加载 `thread_id=T` 下所有 `actor_messages` | 不需要 |
| 外部群组同步 | 给 thread 绑定一个 `GroupBridge` | 不需要新消息语义 |

## 数据模型

### 1. `actor_threads` — 讨论元数据

```sql
CREATE TABLE actor_threads (
    id            TEXT PRIMARY KEY,
    title         TEXT,
    issue_id      TEXT,
    created_by    TEXT NOT NULL,
    participants  TEXT,         -- JSON 快照: ["actor-tl", "coder-01", "virtual:user-123"]
    bridge_type   TEXT,         -- "slack" / "discord" / "telegram" / null
    bridge_scope  TEXT,         -- 外部作用域: workspace/team/guild/chat 等
    bridge_id     TEXT,         -- 外部 channel/thread ID（仅在 bridge_scope 内解释）
    metadata      TEXT,         -- thread 级扩展信息（JSON）
    bridge_metadata TEXT,       -- bridge 专有扩展信息（JSON）
    status        TEXT NOT NULL DEFAULT 'open',
    created_at    DATETIME NOT NULL,
    updated_at    DATETIME NOT NULL
);

CREATE UNIQUE INDEX idx_actor_threads_bridge_lookup
    ON actor_threads(bridge_type, bridge_scope, bridge_id);
```

说明：

- `participants` 在 P0/P1 是**轻量快照**，用于列出讨论和桥接同步，不是完整成员系统。
- `bridge_scope + bridge_id` 一起构成外部 thread/channel 的定位键；不能只靠 `bridge_id` 裸查。
- `metadata` / `bridge_metadata` 用于承载未来的轻量扩展，例如 thread 摘要、桥接策略、静音策略、外部群组标题缓存等。
- 如果后续真要支持 join / leave / mute / role 等复杂成员能力，再新增 `actor_thread_members` 表。

### 2. `actor_messages` — 消息本体

```sql
CREATE TABLE actor_messages (
    id           TEXT PRIMARY KEY,
    thread_id    TEXT,
    from_id      TEXT NOT NULL,
    content      TEXT NOT NULL,
    metadata     TEXT,          -- JSON: sender/group/mention/source_message_id 等结构化上下文
    created_at   DATETIME NOT NULL,

    FOREIGN KEY (thread_id) REFERENCES actor_threads(id)
);

CREATE INDEX idx_actor_messages_thread_created
    ON actor_messages(thread_id, created_at);
```

说明：

- 一条消息只存一次。
- `thread_id = NULL` 表示无 thread 的直接消息。
- 回复永远是新的 `actor_messages` 记录，不回写到原 inbox 记录里。
- channel / bridge 特有的上下文放在 `metadata`，并在 session 构建时抽成结构化上下文，不拼回 `content`。

### 3. `actor_inbox` — 投递通知

```sql
CREATE TABLE actor_inbox (
    id                TEXT PRIMARY KEY,
    message_id        TEXT NOT NULL,
    actor_id          TEXT NOT NULL,
    status            TEXT NOT NULL DEFAULT 'pending',
    claimed_at        DATETIME,
    handled_at        DATETIME,
    result_message_id TEXT,
    error             TEXT,

    FOREIGN KEY (message_id) REFERENCES actor_messages(id),
    FOREIGN KEY (result_message_id) REFERENCES actor_messages(id)
);

CREATE INDEX idx_actor_inbox_actor_status
    ON actor_inbox(actor_id, status, claimed_at);
```

说明：

- `actor_inbox` 是“某个 Actor 需要处理哪条消息”的投递层。
- 同一条消息可以有很多 inbox 记录，每个接收者状态独立。
- `result_message_id` 指向处理该消息后产生的回复消息；Inbox 不直接存回复正文。

### 4. `actor_actions` — 工具执行记录

```sql
CREATE TABLE actor_actions (
    id                    TEXT PRIMARY KEY,
    inbox_id              TEXT NOT NULL,
    sequence              INTEGER NOT NULL,
    tool_name             TEXT NOT NULL,
    input                 TEXT,
    output_raw            TEXT,
    output_sanitized      TEXT,
    sanitization_warnings TEXT,
    cost                  REAL,
    duration_ms           INTEGER,
    success               BOOLEAN,
    error                 TEXT,
    executed_at           DATETIME NOT NULL,

    FOREIGN KEY (inbox_id) REFERENCES actor_inbox(id)
);
```

说明：

- 工具调用属于“某个 Actor 处理某条投递”的过程，因此挂在 `inbox_id` 上最自然。

### 5. `actors` 的最小补充字段

```sql
ALTER TABLE actors ADD COLUMN display_name TEXT;
ALTER TABLE actors ADD COLUMN avatar_url   TEXT;
```

说明：

- 这两个字段仅用于桥接场景下的展示。
- P0 不要求“每个 Actor 对应一个外部平台机器人账号”。默认可由单一 bridge bot 发言，正文带 `[coder-01]` 前缀即可。

### 6. `StatusUpdate` — 非持久化过程态事件

这不是数据库表，而是一类明确不进入 `actor_messages` 的事件：

- thinking / planning
- tool started / tool completed
- approval needed
- auth required / auth completed
- bridge sync started / bridge sync failed

是否持久化状态事件可以单独设计事件日志，但**不能与正式 message 时间线共表**。

## API 设计

### `create_thread`

```text
create_thread(title, participants, issue_id?, bridge?) -> thread_id
```

用途：

- 创建一个讨论容器
- 指定初始参与者快照
- 可选地绑定到 Issue 或外部平台 bridge

### `list_threads`

```text
list_threads(status?, issue_id?) -> [thread]
```

用途：

- 让 TL 或外部控制面看到“当前有哪些开放中的讨论”

### `send_message`

```text
send_message(to, content, thread_id?, metadata?) -> message_id
```

其中 `to` 支持：

- 单个 Actor ID：`"coder-01"`
- 多个 Actor ID：`["coder-01", "coder-02"]`
- 虚拟 Actor ID：`"virtual:user-123"`

P0 **不强依赖** `@thread` 语义糖。原因很简单：

- `thread` 是上下文容器
- `to` 是实际投递集合

如果过早支持 `@thread`，很容易把 thread 再次误用成广播容器。等 P1/P2 确认确有需要，再把它作为“展开为 `thread.participants`”的语法糖加入即可。

### `check_inbox`

```text
check_inbox(limit?) -> [delivery]
```

用途：

- 让 Actor 或外部 Human 看到自己待处理的消息投递

### `close_thread`

```text
close_thread(thread_id) -> ok
```

用途：

- 关闭讨论，不再作为活跃 thread 列表的一部分

## 消息流

### 场景 1：TL 同时问两个人

```text
send_message(
  to=["coder-01", "coder-02"],
  thread_id="thread-auth",
  content="JWT 还是 session？说说你的看法"
)
```

Gateway 执行：

1. 插入一条 `actor_messages(msg-1)`
2. 插入两条 `actor_inbox`
   - `(msg-1 -> coder-01)`
   - `(msg-1 -> coder-02)`
3. 唤醒两个 Actor 的收件循环
4. 如果 `thread-auth` 绑定了 bridge，则 `bridge.Send(...)` **只同步一次**

### 场景 2：Actor 回复 TL

`coder-01` 处理完投递后：

1. 新增一条 `actor_messages(msg-2, from_id=coder-01, thread_id=thread-auth)`
2. 新增一条 `actor_inbox(msg-2 -> actor-tl)`
3. 将原投递 `(msg-1 -> coder-01)` 标记为 `handled`
4. `result_message_id = msg-2`
5. 如有 bridge，再同步一次回复到外部 thread

### 场景 3：Human 从外部平台发言

Slack / Discord / Telegram webhook 到达：

1. `GroupBridge.ParseIncoming(...)` 解析 payload
2. 根据 `(bridge_type, bridge_scope, bridge_id)` resolve 到内部 `thread_id`
3. 生成内部 `IncomingMessage`
4. 将 sender / group / external_message_id / mentions 等信息写入 `metadata`
5. 如能匹配到明确的内部目标 Actor，则按规则投递
6. 若不能明确 resolve，则默认路由到 TL

这保证了：

- 外部平台可以参与 thread
- 内部仍由 TL 掌控编排与决策

## GroupBridge 接口

```go
type GroupBridge interface {
    core.Plugin

    Create(ctx context.Context, thread *Thread) (externalID string, err error)
    Send(ctx context.Context, thread *Thread, from *Actor, message *Message) error
    ParseIncoming(ctx context.Context, payload []byte) (*IncomingMessage, error)
    ConversationContext(ctx context.Context, metadata map[string]any) (map[string]string, error)
    SyncMembers(ctx context.Context, thread *Thread, actors []*Actor) error
}
```

建议实现目录：

```text
plugins/
  bridge-slack/
  bridge-discord/
  bridge-telegram/
  bridge-mock/
```

### P0 的桥接约束

- 外部桥接是可选插件，不是核心依赖
- 默认单一 bot 身份即可，不强求每个 Actor 一个独立外部账号
- 外部平台的 `@mention` 解析失败时，默认回落给 TL，而不是任意猜测目标 Actor
- 同一条外部消息需要可去重；桥接层必须使用平台消息 ID 或等价键做幂等保护

## Session 上下文加载

Actor 处理一条投递时：

1. 读取该 `actor_inbox` 记录及其 `message_id`
2. 找到该消息的 `thread_id`
3. 加载同 `thread_id` 下最近的 `actor_messages`
4. 从 message / thread metadata 抽取结构化 conversation context（如 sender / group / source channel）
5. 合并 Role Prompt、MemoryStore 召回、Thread 时间线、conversation context，组成 ACP session 上下文

因此，**群组讨论的共享上下文来自 `actor_messages` 时间线，不来自 Inbox。**

Inbox 只负责“谁要处理这条消息”。

如果当前 Actor 进程或 session 内存里没有这个 thread，也必须能根据数据库中的 `actor_threads + actor_messages` 完整 hydrate 回来，而不是依赖 TL 自己记住。

## 路由规则

本设计不改变 06 的核心约束：

- 默认仍然是 `Worker -> TL -> Worker`
- Thread 共享上下文，不等于放开任意 Actor 自由互聊
- 需要跨 Worker 协调时，优先由 TL 汇总、转述、再分发

这意味着“群聊”在系统里的真实含义是：

> 多个 Actor 处在同一个讨论上下文里，但实际投递和调度仍然受 Gateway 路由规则控制。

## 实现不变量

为避免后续实现再次漂移，以下不变量必须固定：

1. **一条消息只在 `actor_messages` 中存一次**
2. **多接收者只增加 `actor_inbox` 行，不复制消息本体**
3. **外部 bridge 对每条消息最多同步一次，不按接收者重复同步**
4. **回复总是新的 `actor_messages`，Inbox 只记录处理结果引用**
5. **`thread_id` 只代表上下文归属，不代表自动广播**
6. **外部 mention 解析不确定时默认路由 TL，不做模糊猜测**
7. **状态更新不进入 `actor_messages`，只走 side channel 或独立事件流**
8. **外部 bridge 查找 thread 时必须带作用域，不能裸用 `bridge_id`**
9. **外部 sender/group 等上下文进入 metadata 与 prompt context，不拼回 message content**
10. **thread 必须可从存储层 hydrate，不能只依赖 Actor 进程内存**

## 复杂度评估

### 现在保留的复杂度

- `actor_threads`：为 thread 元数据提供稳定锚点
- `actor_messages` / `actor_inbox` 分离：避免多人消息重复和桥接重复
- `GroupBridge`：以插件方式接入外部平台
- `StatusUpdate` 侧带流：把正式消息和过程态展示分开
- `metadata` / `bridge_scope`：为外部桥接和未来扩展预留最小结构化空间

### 明确砍掉的复杂度

- 不做聊天产品级能力
- 不做复杂成员系统
- 不做开放式全员自由通信
- 不做每 Actor 一个外部 bot 身份的强约束

所以这套方案不是“复杂”，而是：

**在多人协作 + 外部群组桥接这个目标下，已经接近最小正确模型。**

## 与 06 的关系

06 仍然回答“大架构”问题：

- 为什么要 Actor
- 为什么要 Gateway / Inbox / Memory / Skills
- 为什么推荐“Actor 层叠加”而不是完全替代流水线

07 只回答“消息层到底怎么建模”这个更窄、更实现导向的问题：

- Thread 元数据放哪
- 多人投递怎么避免重复
- 外部群组桥接怎么不污染核心模型
- 状态更新、外部上下文和 thread hydrate 应该放在哪一层

因此，推荐的阅读方式是：

1. 先看 06，理解 Actor 工作空间的全局方向
2. 再看 07，理解消息 / 线程 / Inbox / Bridge 的收敛实现

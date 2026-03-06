# Actor 工作空间：动态多 Agent 协作模型

> **前置**: [02-Escalation/Directive](02-escalation-directive-pattern.zh-CN.md) 定义了纵向通信。本文将其泛化为 Actor 间任意通信。
> **相关**: [05-多用户部署](05-multi-user-deployment-model.zh-CN.md) 的单实例多 Project 模型是本设计的部署基础。

## 问题

当前系统是**固定流水线**模型：

```
Issue → Run → [setup → implement → review → merge] → Done
```

- 角色在 config 里静态定义
- ACP session 绑定 stage 生命周期，stage 结束即销毁
- Agent 之间不能直接对话，只通过 EventBus 间接协调
- 所有工作流是预定义模板（standard / quick / hotfix）

**缺什么：**

1. 不能动态创建角色 — TL 发现需要一个"数据库专家"，得改 config 重启
2. Agent 没有持久记忆 — 每次启动都是全新 session，前一个任务学到的上下文丢失
3. Agent 之间不能对话 — Coder 想问 Reviewer 一个问题，必须走完整个 stage 流转
4. 流程不能动态编排 — TL 不能说"先让 A 和 B 并行，B 做完再让 C 开始"

## 核心洞察

**Agent 不是函数，是人。**

函数调用：调用 → 等结果 → 销毁。适合确定性流水线。
人的工作方式：常驻 → 接活 → 干活 → 跟人沟通 → 汇报 → 等下一个活。

把 Agent 从"被调用的函数"变成"常驻的 Actor"，系统从"流水线调度器"变成"团队工作空间"。

## 核心概念

### Actor

一个有持久身份的 Agent 实例。

```
Actor = 角色画像 + 持久 Session + 工作目录 + 收件箱
```

| 属性 | 说明 |
|------|------|
| ID | 唯一标识，如 `actor-coder-01` |
| Role | 角色模板（画像、能力、提示词） |
| Session | ACP 持久会话（可休眠/唤醒） |
| Workspace | 独立工作目录（worktree 或自定义路径） |
| Inbox | 消息队列，按优先级排序 |
| Status | `idle` / `busy` / `sleeping` / `dead` |

### Inbox（收件箱）

每个 Actor 有一个收件箱。发消息 = 投递到收件箱，**立刻返回**，不阻塞发送方。

```
TL 给 Coder 发消息：
  TL → Gateway.Send(to=coder, msg) → Coder.Inbox.Push(msg) → 返回（TL 继续干别的）

  ...稍后...

  Coder 空闲 → Coder.Inbox.Pop() → 处理消息 → 可能回复 TL
```

**这就是 Actor 模型。** 没有同步调用，没有阻塞等待。

### Gateway（网关）

消息路由中心。所有 Actor 间通信经过 Gateway。

```
┌─────────────────────────────────────────────────┐
│                   Gateway                        │
│                                                  │
│  路由表: Actor ID → Inbox 地址                    │
│  权限: 谁能给谁发消息                              │
│  策略: 消息优先级、超时、死信队列                    │
│  日志: 全量消息审计                                │
└────┬──────────┬──────────┬──────────┬────────────┘
     │          │          │          │
 ┌───▼──┐  ┌───▼──┐  ┌───▼───┐  ┌───▼───┐
 │  TL  │  │Coder │  │Coder  │  │Review │
 │Inbox │  │#1    │  │#2     │  │er     │
 │      │  │Inbox │  │Inbox  │  │Inbox  │
 └──────┘  └──────┘  └──────┘  └───────┘
```

Gateway 不做决策，只做路由。决策是 TL（或更上层）的事。

### 消息类型（参考 IronClaw 设计）

> 以下设计直接参考 IronClaw 已验证的消息体系，而非自行发明抽象分类。
> 参考源码: `src/channels/channel.rs`, `src/agent/session.rs`, `src/context/state.rs`

IronClaw 不区分"directive / escalation / query"等抽象消息类型。它的设计更简单：

- **IncomingMessage** — 所有输入统一结构（不管来自哪个通道、谁发的）
- **StatusUpdate** — 执行过程中的实时状态推送（thinking、tool_started、tool_completed...）
- **OutgoingResponse** — 处理结果统一回复
- **Session → Thread → Turn** — 对话按层级组织，不是扁平消息队列

核心洞察：**消息的"类型"不需要在协议层硬编码。** TL 给 Coder 发的"实现这个功能"和用户给 TL 发的"帮我修个 bug"，在消息结构上完全一样 — 都是 `IncomingMessage`。语义（指令/上报/问答）由 Agent 自己从内容理解，不由消息字段定义。

#### 输入消息

```go
// IncomingMessage — 所有输入的统一结构
// 参考 IronClaw src/channels/channel.rs:14-70
type IncomingMessage struct {
    ID         string            // 唯一消息 ID
    Channel    string            // 来源通道: "mcp" / "a2a" / "web" / "telegram" / actor ID
    UserID     string            // 发送者标识（外部用户 ID 或 Actor ID）
    UserName   string            // 可选显示名
    Content    string            // 消息内容（自然语言）
    ThreadID   string            // 线程/对话 ID（串联上下文）
    ReceivedAt time.Time
    Metadata   map[string]any    // 通道特定元数据
}
```

不需要 `Type` 字段。"这是指令还是问题"由接收方 Agent 判断。

#### 状态更新

```go
// StatusUpdate — 执行过程中的实时推送
// 参考 IronClaw src/channels/channel.rs:112-166
type StatusUpdate struct {
    Type       string            // thinking / tool_started / tool_completed / tool_result /
                                 // stream_chunk / status / approval_needed / error
    ActorID    string            // 产生此状态的 Actor
    ThreadID   string            // 关联的线程
    Data       map[string]any    // 类型特定数据
    Timestamp  time.Time
}
```

StatusUpdate 不进 Inbox，走实时推送（SSE/WebSocket）。外部调用方不在线时，关键状态变化转为 IncomingMessage 投递到其 Inbox。

#### 输出响应

```go
// OutgoingResponse — 处理完成后的回复
// 参考 IronClaw src/channels/channel.rs:76-110
type OutgoingResponse struct {
    Content     string            // 回复内容
    ThreadID    string            // 回复到哪个线程
    Attachments []string          // 附件（文件路径）
    Metadata    map[string]any    // 通道特定元数据
}
```

#### 会话层级

```go
// Session → Thread → Turn 三层结构
// 参考 IronClaw src/agent/session.rs:21-192

// Session — 一个 Actor 的完整会话（可包含多个线程）
type ActorSession struct {
    ID              string
    ActorID         string
    ActiveThread    string
    Threads         map[string]*Thread
    CreatedAt       time.Time
    LastActiveAt    time.Time
    Metadata        map[string]any
    AutoApproved    map[string]bool    // 自动批准的工具
}

// Thread — 一个对话线程（一个任务/话题）
type Thread struct {
    ID        string
    SessionID string
    State     ThreadState    // idle / processing / awaiting_approval / completed / interrupted
    Turns     []Turn
    CreatedAt time.Time
    UpdatedAt time.Time
    Metadata  map[string]any
}

// Turn — 一轮交互（用户输入 → Agent 处理 → 响应）
type Turn struct {
    TurnNumber  int
    UserInput   string
    Response    string
    ToolCalls   []TurnToolCall
    State       TurnState    // processing / completed / failed / interrupted
    StartedAt   time.Time
    CompletedAt time.Time
    Error       string
}
```

#### 工具执行记录

```go
// ActionRecord — 每次工具调用的详细记录
// 参考 IronClaw src/context/memory.rs:12-93
type ActionRecord struct {
    ID                   string
    Sequence             int
    ToolName             string
    Input                map[string]any
    OutputRaw            string            // 原始输出
    OutputSanitized      map[string]any    // 清理后输出
    SanitizationWarnings []string          // 安全层警告
    Cost                 float64           // 本次调用成本
    Duration             time.Duration
    Success              bool
    Error                string
    ExecutedAt           time.Time
}
```

#### 与原有设计的对应关系

| 原 06 设计（自创） | IronClaw 设计（采用） | 说明 |
|---|---|---|
| `ActorMessage.Type = directive` | `IncomingMessage` (内容隐含指令语义) | Agent 自己理解意图，不靠字段 |
| `ActorMessage.Type = escalation` | `IncomingMessage` (内容隐含上报语义) | 同上 |
| `ActorMessage.Type = query` | `IncomingMessage` + `ThreadID` 串联 | 对话在线程内自然发生 |
| `ActorMessage.Type = notify` | `StatusUpdate` 或 `IncomingMessage` | 实时状态走 StatusUpdate，离线通知走 Inbox |
| `ActorMessage.Priority` | Inbox 排序策略（不在消息里） | 优先级是 Inbox 的事，不是消息的事 |
| `ActorMessage.ReplyTo` | `ThreadID` | 线程天然串联对话 |
| `ActorMessage.ExpiresAt` | Inbox 死信策略（不在消息里） | 超时是 Inbox 的事，不是消息的事 |

## TL 的角色管理技能

TL 是工作空间的管理者。通过对话创建、配置、管理 Actor。

### MCP 工具集

```
角色管理：
  create_role(name, base_agent, capabilities, prompt, description)
  update_role(name, ...)
  delete_role(name)
  list_roles()

Actor 生命周期：
  spawn_actor(role, workspace_path?)     → 创建并启动一个 Actor
  kill_actor(actor_id)                   → 停止并销毁
  sleep_actor(actor_id)                  → 休眠（释放进程，保留状态）
  wake_actor(actor_id)                   → 唤醒（恢复进程和状态）
  list_actors()                          → 查看所有活跃 Actor

消息：
  send_message(to, type, subject, body)  → 发送消息到 Actor
  broadcast(channel, subject, body)      → 广播
  check_inbox()                          → 查看自己的收件箱
```

### TL 对话示例

```
Human: "我们需要一个专门处理数据库迁移的角色"

TL:  → create_role(
         name="db-specialist",
         base_agent="claude",
         capabilities={fs_read: true, fs_write: true, terminal: true},
         prompt="你是数据库迁移专家，熟悉 PostgreSQL 和 SQLite...",
       )
     → spawn_actor(role="db-specialist", workspace="/projects/backend")

     "已创建 db-specialist 角色并启动了一个实例。
      他现在在 /projects/backend 工作目录待命。要给他分配任务吗？"
```

### TL 动态编排示例

```
Human: "项目 A 需要重构认证模块，让两个 coder 并行做，一个负责后端 API，一个负责前端适配"

TL:  → spawn_actor(role="worker", workspace="/projects/A")        // coder-01
     → spawn_actor(role="worker", workspace="/projects/A")        // coder-02
     → send_message(to="coder-01",
         content="重构 /src/auth/ 下的 handler，切换到 JWT...")
     → send_message(to="coder-02",
         content="后端 API 变更后，适配 /web/src/lib/auth.ts...")

     "已分配两个 coder 并行工作。coder-01 负责后端，coder-02 负责前端。
      后端完成后我会通知前端 coder 对齐接口。"

  ...coder-01 完成后...

  TL 收到 coder-01 的消息("后端认证 API 重构完成")
     → send_message(to="coder-02",
         content="后端 API 已完成，接口变更: POST /auth/login 返回格式改了...")
```

**关键区别**：TL 不是在执行预定义流水线，而是根据任务性质**即兴编排**。

## 与固定流水线的关系

### 不替代，共存

固定流水线 = 成熟的、可重复的工作流（标准开发、hotfix）。
Actor 动态编排 = 非标准、探索性、跨域协作的工作流。

```
用户说 "修一个 bug"
  → TL 判断这是标准任务
  → 走固定流水线（Issue → Run → stages → Done）
  → 流水线内部可以复用常驻 Actor 的 session（不用每次冷启动）

用户说 "重构整个认证模块，涉及三个项目"
  → TL 判断这需要自定义编排
  → 动态创建角色、分配任务、协调沟通
  → 不走预定义 stage 模板
```

### 流水线在 Actor 模型下的表达

固定流水线其实就是 TL 按照模板发的一系列消息：

```
标准流水线 =
  TL → send_message(coder, "实现这个功能: ...")
  TL ← 收到 coder 的消息("实现完成")
  TL → send_message(reviewer, "审查 coder 的改动: ...")
  TL ← 收到 reviewer 的消息("审查通过")
  TL → send_message(coder, "合并到 main")
```

消息就是消息，不需要"directive"/"notify"等类型标签。TL 从内容理解语义。流水线模板变成 TL 的**行为模式**，而不是硬编码的 stage 序列。

## Actor 生命周期

### 状态机

```
                spawn
                  │
                  ▼
idle ◄──────► busy
  │              │
  │ sleep        │ sleep（完成当前消息后）
  ▼              ▼
sleeping ───► idle（wake）
  │
  │ kill / 超时回收
  ▼
dead
```

### 休眠与唤醒

Agent 进程占资源。空闲 Actor 需要休眠：

```
休眠：
  1. 序列化 ACP session 状态（对话历史、工具状态）
  2. 保存到 Store（actor_sessions 表）
  3. 终止 ACP 进程
  4. Actor 状态 → sleeping

唤醒：
  1. 从 Store 加载 session 状态
  2. 启动新 ACP 进程
  3. 注入历史对话作为上下文
  4. Actor 状态 → idle
```

**注意**：ACP 协议目前不支持原生 session 序列化。唤醒后注入历史是近似恢复，不是精确恢复。对于大部分场景够用 — Agent 会"记得"之前的对话，但丢失进程内状态（如打开的文件句柄）。

### 自动回收策略

```toml
[actor_pool]
max_idle_duration = "30m"       # 空闲超过 30 分钟自动休眠
max_sleeping_duration = "24h"   # 休眠超过 24 小时自动销毁
max_concurrent_actors = 10      # 全局最多同时活跃 Actor
```

## Inbox 设计

### 消息队列语义

```
排序：FIFO（先进先出）

不硬编码优先级。IronClaw 的做法是：所有消息平等，Agent 自己判断轻重缓急。
如果 Gateway 需要插队（如系统告警），直接 PushFront 即可，不需要优先级字段。

Actor 消费循环：
  loop:
    msg = inbox.Pop()          // 阻塞等待
    if status == busy:
      inbox.PushFront(msg)     // 忙碌时退回队首
      wait(current_task_done)
      continue
    process(msg)               // 创建 Thread，进入 Turn 循环
```

### 死信队列

消息在 Inbox 中停留超过配置时间 → 进入死信队列 → Gateway 通知发送方：

```
TL 给 Coder 发了消息，Coder 挂了
  → 消息在 Inbox 超时（默认 10 分钟）
  → Gateway → TL.Inbox: "你给 coder-01 的消息超时未处理"
  → TL 决策：唤醒 coder-01 / 转发给 coder-02 / 放弃
```

超时策略在 Inbox 层配置，不在消息里。

### 对话串联

通过 **Thread** 串联对话（参考 IronClaw Session → Thread → Turn）：

```
Thread "实现登录功能":
  Turn 0: TL → Coder "实现登录功能"
  Turn 1: Coder → TL "数据库 schema 不确定，用 sessions 表还是 tokens 表？"
  Turn 2: TL → Coder "用 sessions 表，参考 project B 的实现"
```

同一个 Thread 内的消息自动共享上下文。新话题开新 Thread。Actor 处理消息时，自动加载同 Thread 的历史 Turn 作为对话上下文。

## Gateway 设计

### 路由规则

```go
type RoutingRule struct {
    From     string      // Actor ID / role pattern / "*"
    To       string      // Actor ID / role pattern / "*"
    Action   string      // allow / deny / redirect
    Target   string      // redirect 目标（可选）
}
```

不按消息类型过滤（消息没有类型字段）。只按**谁能给谁发**控制。

默认规则：
- TL 可以给所有人发消息
- Worker 只能给 TL 发消息（不能直接给其他 Worker 发）
- Worker 之间通信需要 TL 预先授权（防止 Agent 之间无限聊天）
- 外部（Human / MCP / A2A）只能给 TL 发消息

### 与外部的桥接

```
Human / A2A → Gateway → TL.Inbox     （外部消息统一进 TL）
              Gateway ← TL.Inbox     （TL 的回复统一出 Gateway）
                      → Human / A2A
```

TL 是外部世界的唯一接口。其他 Actor 对外不可见。
这与当前 A2A Bridge 的设计一致 — A2A 只跟"团队"对话，不跟个别成员对话。

## 存储

### 新增表

```sql
-- 角色模板（TL 动态创建）
CREATE TABLE roles (
    name        TEXT PRIMARY KEY,
    base_agent  TEXT NOT NULL,
    capabilities TEXT NOT NULL,  -- JSON (SandboxPolicy)
    prompt      TEXT,
    description TEXT,
    created_by  TEXT,            -- actor ID（通常是 TL）
    created_at  DATETIME,
    source      TEXT DEFAULT 'dynamic'  -- 'static' (config) / 'dynamic' (TL 创建)
);

-- Actor 实例
CREATE TABLE actors (
    id          TEXT PRIMARY KEY,
    role        TEXT NOT NULL REFERENCES roles(name),
    workspace   TEXT,
    status      TEXT NOT NULL DEFAULT 'idle',
    session_data TEXT,           -- 休眠时序列化的 session 状态
    budget_used REAL DEFAULT 0,  -- 已消耗预算
    last_active DATETIME,
    created_at  DATETIME
);

-- 消息（Inbox 持久化）
-- 参考 IronClaw IncomingMessage 结构，无 type/priority/subject 字段
CREATE TABLE actor_messages (
    id          TEXT PRIMARY KEY,
    channel     TEXT NOT NULL,     -- 来源: actor ID / "mcp" / "a2a" / "web"
    from_id     TEXT NOT NULL,     -- 发送者 ID
    to_id       TEXT NOT NULL,     -- 接收者 Actor ID
    thread_id   TEXT,              -- 线程 ID（对话串联）
    content     TEXT NOT NULL,     -- 消息内容
    metadata    TEXT,              -- JSON (通道特定元数据)
    status      TEXT DEFAULT 'pending',  -- pending / delivered / processed / expired
    created_at  DATETIME,
    processed_at DATETIME
);

-- 线程（对话上下文）
-- 参考 IronClaw Session → Thread → Turn 层级
CREATE TABLE actor_threads (
    id          TEXT PRIMARY KEY,
    actor_id    TEXT NOT NULL,     -- 所属 Actor
    state       TEXT DEFAULT 'idle',  -- idle / processing / completed / interrupted
    metadata    TEXT,              -- JSON (标题、标签等)
    created_at  DATETIME,
    updated_at  DATETIME
);

-- Turn（一轮交互）
CREATE TABLE actor_turns (
    id          TEXT PRIMARY KEY,
    thread_id   TEXT NOT NULL REFERENCES actor_threads(id),
    turn_number INTEGER NOT NULL,
    user_input  TEXT,              -- 输入消息
    response    TEXT,              -- Agent 回复
    state       TEXT DEFAULT 'processing',  -- processing / completed / failed / interrupted
    error       TEXT,
    started_at  DATETIME,
    completed_at DATETIME
);

-- 工具执行记录（参考 IronClaw ActionRecord）
CREATE TABLE actor_actions (
    id          TEXT PRIMARY KEY,
    turn_id     TEXT NOT NULL REFERENCES actor_turns(id),
    sequence    INTEGER NOT NULL,
    tool_name   TEXT NOT NULL,
    input       TEXT,             -- JSON
    output_raw  TEXT,
    output_sanitized TEXT,        -- 安全层清理后
    sanitization_warnings TEXT,   -- JSON array
    cost        REAL,
    duration_ms INTEGER,
    success     BOOLEAN,
    error       TEXT,
    executed_at DATETIME
);
```

### 与现有 Store 的关系

新增表，不改现有表。现有的 `issues`、`runs`、`checkpoints` 继续用于固定流水线。Actor 层是叠加的，不是替换的。

## 从 IronClaw 吸收的能力

> 参考: [IronClaw 架构学习笔记](ironclaw-architecture-study.zh-CN.md)
>
> IronClaw 是一个 ~83K 行 Rust 的个人 AI 助手框架，已在生产中验证。以下 8 项能力是 06 设计的重要补充。

### 1. 外部通道抽象（← IronClaw ChannelManager）

**问题**: Gateway 只管 Actor 间路由，但**外部世界怎么接入**没有定义。

IronClaw 的做法：`Channel` trait 产生 `MessageStream`，`ChannelManager` 合并所有通道为统一输入。

**吸收**:

```
Gateway = 外部通道管理 + 内部消息路由

外部通道:
  MCP     → /api/v1/mcp 调用方（IronClaw、Claude Code 等）
  A2A     → /api/v1/a2a 标准协议对接
  Web     → Dashboard WebSocket/SSE
  Webhook → Telegram/Slack/GitHub 等

每个通道有独立的消息流汇入 Gateway:
  Channel.MessageStream → Gateway.Inbound → 安全检查 → 路由到目标 Inbox
```

外部调用方也是 Actor —— 它们有自己的 Inbox（`check_inbox` 拉取），消息投递到 Inbox 后立刻返回，不阻塞。这解决了 MCP 调用方"连上来调一下就走"的通知问题。

### 2. Actor 执行隔离（← IronClaw WASM 沙箱）

**问题**: TL 动态创建的角色，它的 ACP session 能做什么？没有安全边界。

IronClaw 的做法：WASM 沙箱 + 能力声明 + 白名单 + 燃料计量 + 内存限制。

**吸收**:

```go
type Actor struct {
    // ... 现有字段

    // 执行沙箱
    Sandbox    SandboxPolicy
    AllowTools []string       // 工具白名单（空 = 全部）
    DenyTools  []string       // 工具黑名单
}

type SandboxPolicy struct {
    FSRead     bool           // 文件读
    FSWrite    bool           // 文件写（限定 workspace 目录）
    Terminal   bool           // shell 执行
    Network    []string       // 网络白名单
    MCP        bool           // 是否能调 MCP 工具
}
```

- TL 创建角色时设定安全边界
- 动态创建的角色默认 `restricted`（只读 + 无 shell）
- config 里静态定义的角色可以 `full`
- ACP agent 的 `capabilities` 被 Sandbox 二次约束

### 3. Skill 动态注入（← IronClaw SKILL.md + selector + attenuation）

**问题**: 角色提示词在 `create_role` 时写死。Actor 处理不同消息时无法自适应。

IronClaw 的做法：
- SKILL.md 文件定义提示词 + 激活条件（关键词、正则、标签）
- Selector 按消息内容自动匹配相关 skills，多 skill 可叠加
- 信任级别衰减：低信任 skill 激活时，自动降低可用工具上限

**吸收**:

```
Actor 处理消息时:
  1. 加载角色基础提示词（Role.Prompt）
  2. 根据消息内容匹配 Skills（关键词、标签）
  3. 叠加匹配的 Skill 上下文到系统提示
  4. 根据 Skill 信任级别衰减可用工具

Skill 来源:
  - 内置 skills（系统自带，Trusted）
  - 项目级 skills（.ai-workflow/skills/，Trusted）
  - 外部安装 skills（注册表，Installed = 低信任）
```

这让 Actor 可以是"通才 + 按需专项技能"，而不是创建时定死能力。

### 4. 成本控制（← IronClaw CostGuard）

**问题**: 开放问题 #3 "10 个常驻 Actor 的成本怎么控"没有方案。

IronClaw 的做法：`CostGuard` 跟踪日预算、小时速率、单作业上限，超限则拒绝执行。

**吸收**:

```go
type ActorPool struct {
    // 现有
    MaxConcurrentActors int
    MaxIdleDuration     time.Duration
    MaxSleepingDuration time.Duration

    // 成本控制
    DailyBudget      float64        // 全局日预算（美元）
    PerActorBudget   float64        // 单 Actor 日上限
    PerMessageBudget float64        // 单消息处理上限
}
```

- 每次 LLM 调用记录 token 消耗和估算成本
- Actor 超预算 → 自动休眠 → Gateway 通知 TL
- TL 可调整预算或决定是否继续

```toml
[actor_pool]
daily_budget        = 50.0     # 全局日预算 $50
per_actor_budget    = 10.0     # 单 Actor 日上限 $10
per_message_budget  = 2.0      # 单消息上限 $2
```

### 5. 自我修复（← IronClaw self_repair + heartbeat）

**问题**: Actor 挂了怎么办没有设计。

IronClaw 的做法：心跳检测 + 卡住检测（超时无输出）+ 自动恢复（重启 + 注入历史）。

**吸收**:

```
Gateway 健康管理:
  - 每 N 秒检查所有 busy Actor 的心跳
  - Actor 无响应超过阈值 → 标记 dead
  - 通知 TL，由 TL 决策：
    a. restart（wake 注入历史）
    b. 转发当前消息给其他 Actor
    c. 放弃并通知消息发送方

  - Actor 进程崩溃 → 自动尝试 wake（注入历史恢复上下文）
  - 连续 N 次恢复失败 → 标记 dead，不再自动恢复
```

```toml
[actor_pool.health]
heartbeat_interval   = "30s"
busy_timeout         = "10m"    # busy 超过 10 分钟无心跳 → 异常
max_auto_restarts    = 3        # 连续自动恢复上限
```

### 6. 语义记忆（← IronClaw Workspace 混合搜索）

**问题**: Actor 持久记忆只是"序列化 session_data"，休眠后只有对话历史，没有语义搜索能力。

IronClaw 的做法：向量搜索 + 全文搜索 + RRF (Reciprocal Rank Fusion) 融合，800 token 分块，15% 重叠。

**吸收**:

```
Actor Memory = 三层

1. 短期：当前 ACP session 对话历史
   - 随 session 生存
   - 休眠时序列化保存

2. 中期：session_data 序列化
   - 用于休眠/唤醒恢复上下文
   - 注入历史对话是近似恢复

3. 长期：语义记忆库（新增）
   - 每个 Actor 有独立的 memory namespace
   - 内容: 做过的任务摘要、学到的经验、项目上下文
   - 搜索: 向量索引 + 全文索引 + 混合搜索
   - 工具: memory_write / memory_search（Actor 自己可调用）
   - 持久: 休眠甚至销毁后长期记忆不丢失
   - 共享: TL 可以把一个 Actor 的记忆挂载给另一个（只读）
```

新增存储:
```sql
CREATE TABLE actor_memories (
    id         TEXT PRIMARY KEY,
    actor_id   TEXT NOT NULL,        -- 所属 Actor（namespace）
    content    TEXT NOT NULL,         -- 原文
    embedding  BLOB,                 -- 向量
    metadata   TEXT,                  -- JSON (标签、来源、时间)
    created_at DATETIME
);
CREATE INDEX idx_actor_memories_actor ON actor_memories(actor_id);
```

### 7. 例程 / 自主触发（← IronClaw routine_engine）

**问题**: Actor 纯被动——只有收到消息才工作。

IronClaw 的做法：`routine_engine.rs` 支持 Cron 表达式和事件触发，独立于用户消息。

**吸收**:

```go
type Actor struct {
    // ... 现有字段

    // 自主行为
    Routines []ActorRoutine
}

type ActorRoutine struct {
    Name    string
    Trigger RoutineTrigger   // cron / event / interval
    Action  string           // 自然语言行为描述或模板 ID
    Enabled bool
}

// 触发类型
type RoutineTrigger struct {
    Cron     string   // "0 9 * * *"
    Event    string   // "issue_created" / "run_failed"
    Interval string   // "30m"
}
```

示例:
- Reviewer Actor: `cron("0 9 * * *")` → 每天早上自动检查待审 PR
- Monitor Actor: `event("run_failed")` → Run 失败时自动分析原因
- TL: `interval("30m")` → 每半小时汇总所有项目进度

例程触发时，Gateway 生成一条 `system` 类型消息投递到 Actor Inbox，Actor 像处理普通消息一样处理。

### 8. 消息安全（← IronClaw SafetyLayer）

**问题**: Actor 间消息没有安全检查。如果一个 Actor 被提示注入，它可以给其他 Actor 发恶意指令。

IronClaw 的做法：多层防御 — Sanitizer（注入检测）+ LeakDetector（泄露扫描）+ Validator（输入验证）+ Policy（规则引擎）。

**吸收**:

```
Gateway 消息处理管道（每条消息都经过）:

  1. 接收消息
  2. Validator   — 长度、编码、格式检查
  3. Sanitizer   — 提示注入模式检测（XML 标记脱逃等）
  4. LeakDetector — 扫描 API 密钥、token 等敏感信息
  5. Policy      — 规则匹配，决定 Allow / Sanitize / Block
  6. Router      — 权限检查（路由规则）
  7. 投递到目标 Inbox

工具输出安全:
  - Actor 的工具执行结果经过 SafetyLayer 再注入对话
  - 防止工具返回值中的提示注入（间接注入攻击）
  - 检测到敏感信息 → 脱敏后再传递

Actor 间通信安全:
  - Worker 给其他 Worker 发消息时，消息内容经过注入检测
  - 防止"被污染的 Actor A"通过消息污染"干净的 Actor B"
  - 严重威胁自动 escalate 给 TL
```

## 更新后的 Actor 模型

```
                          ┌────────────────────────────────────────────────┐
                          │                  Gateway                       │
                          │                                                │
                          │  ┌───────────────────┐  外部通道              │
                          │  │  ChannelManager    │  MCP / A2A / Web /    │
                          │  │  (统一消息流汇入)  │  Telegram / Webhook   │
                          │  └────────┬──────────┘                        │
                          │           │                                    │
                          │  ┌────────▼──────────┐  安全层                │
                          │  │  SafetyLayer       │  注入检测 / 泄露扫描  │
                          │  │  (每条消息必经)     │  / 输入验证 / 策略    │
                          │  └────────┬──────────┘                        │
                          │           │                                    │
                          │  ┌────────▼──────────┐  路由                  │
                          │  │  Router            │  allow / deny /       │
                          │  │  (权限 + 目标解析)  │  redirect / transform│
                          │  └────────┬──────────┘                        │
                          │           │                                    │
                          │  ┌────────▼──────────┐  健康管理              │
                          │  │  HealthCheck       │  心跳 / 卡住检测 /   │
                          │  │                    │  自动恢复             │
                          │  └────────┬──────────┘                        │
                          │           │                                    │
                          │  ┌────────▼──────────┐  成本控制              │
                          │  │  CostGuard         │  全局预算 / 单 Actor  │
                          │  │                    │  预算 / 速率限制      │
                          │  └────────┬──────────┘                        │
                          │           │                                    │
                          │  ┌────────▼──────────┐  例程调度              │
                          │  │  RoutineEngine     │  Cron / 事件触发 /   │
                          │  │                    │  生成系统消息         │
                          │  └───────────────────┘                        │
                          └────┬──────────┬──────────┬──────────┬────────┘
                               │          │          │          │
                           ┌───▼──┐  ┌───▼──┐  ┌───▼───┐  ┌───▼───┐
                           │  TL  │  │Coder │  │Coder  │  │Review │
                           │      │  │#1    │  │#2     │  │er     │
                           ├──────┤  ├──────┤  ├──────┤  ├───────┤
                           │Inbox │  │Inbox │  │Inbox │  │Inbox  │
                           │Skills│  │Skills│  │Skills │  │Skills │
                           │Memory│  │Memory│  │Memory│  │Memory │
                           │Budget│  │Budget│  │Budget│  │Budget │
                           │Sandbox│ │Sandbox│ │Sandbox│ │Sandbox│
                           │Routine│ │Routine│ │Routine│ │Routine│
                           └──────┘  └──────┘  └──────┘  └───────┘
```

## 方案对比

### A: 纯 Actor（全部替换）

把流水线也用 Actor 重写。Run 不再是一等概念。

- 优点：统一模型，没有两套逻辑
- 缺点：改动巨大，现有测试全部失效，风险高

### B: Actor 层叠加（推荐）

保留流水线引擎。新增 Actor 层用于动态协作。流水线内部可选择复用常驻 Actor。

- 优点：增量实施，现有功能不受影响，两种模式按场景选用
- 缺点：两套执行模型并存，维护成本稍高

### C: 流水线渐进 Actor 化

先实现 Actor 基础设施（Inbox + Gateway）。然后逐步把流水线 stage 改为 Actor 消息。最终流水线变成 Actor 编排的一种"宏"。

- 优点：最终统一，但过程渐进
- 缺点：过渡期两套逻辑都在，统一时间不确定

**推荐 B，以 C 为北极星。** 先把 Actor 跑起来，验证价值，再决定是否统一。

## 实施路径

| 阶段 | 内容 | 依赖 |
|------|------|------|
| **P0: 基础通信** | | |
| P0 | Gateway + Inbox + ActorMessage 存储 + 基础路由 | 无 |
| P0 | SafetyLayer 消息管道（注入检测 + 泄露扫描） | Gateway |
| P0 | 外部通道注册（MCP / A2A / Web 作为外部 Actor） | Gateway |
| P0 | TL 常驻 Actor（第一个持久 session） | Gateway |
| P0 | TL 的 `send_message` / `check_inbox` MCP 工具 | Gateway + Inbox |
| P0 | MCP 工具优化: `project_name` resolver + `auto_approve` | MCP Server |
| **P1: Actor 生命周期** | | |
| P1 | Actor 生命周期管理（spawn / kill / sleep / wake） | P0 |
| P1 | 执行隔离: SandboxPolicy + 工具白名单/黑名单 | P0 |
| P1 | CostGuard: 全局预算 + 单 Actor 预算 + 超限休眠 | P0 |
| P1 | HealthCheck: 心跳检测 + 卡住恢复 + 自动 restart | P0 |
| P1 | TL 的角色管理工具（create_role / spawn_actor） | P0 |
| P1 | Worker Actor 复用（流水线 stage 可指向常驻 Actor） | P1 |
| **P2: 动态编排** | | |
| P2 | Skill 动态注入: 按消息匹配 skill + 信任级别衰减 | P1 |
| P2 | 语义记忆: actor_memories 表 + 向量搜索 + memory 工具 | P1 |
| P2 | RoutineEngine: Cron + 事件触发 + 系统消息生成 | P1 |
| P2 | 动态编排（TL 自由组合 Actor 完成非标任务） | P1 |
| P2 | Actor 间 query 通信（对等问答） | P1 + 权限规则 |
| **P3: 统一** | | |
| P3 | 流水线 Actor 化（stage → Actor message 宏） | P2 验证后 |

## 开放问题

### 已解答（通过 IronClaw 研究）

1. ~~**资源预算**~~ → CostGuard 方案（§4 成本控制）：日/Actor/消息三级预算 + 超限自动休眠
2. ~~**人类参与模式**~~ → 外部通道抽象（§1）：Human 是外部 Actor，通过 MCP/Web 通道接入，Inbox = Web UI 通知面板
3. ~~**与 A2A 的关系**~~ → 外部通道抽象（§1）：A2A 是一种外部通道，消息进 Gateway 统一处理，TL 不需要区分来源

### 仍然开放

4. **ACP session 休眠精度** — 注入历史对话能恢复多少上下文？三层记忆模型（§6 语义记忆）是缓解方案，但需要实验验证。可能需要 ACP 协议扩展支持 session snapshot。

5. **Actor 间聊天失控** — 两个 Agent 互相 query 可能无限循环。Gateway Router 层限制：每个 query 链最多 N 轮（默认 5），超出自动 escalate 给 TL。CostGuard 的 per_message_budget 是第二道防线。

6. **Skill 信任边界** — 外部安装的 Skill 包含提示词，可能被投毒。需要参考 IronClaw 的信任模型（Installed vs Trusted）和权限衰减机制，但 Go 侧没有 WASM 沙箱，隔离程度取决于 ACP agent 的能力约束。

7. **跨 Actor 记忆共享** — TL 把 Actor A 的记忆挂载给 Actor B（只读），如何防止信息泄露？需要 memory namespace 的 ACL 机制。

---

> **后续**: 本设计确定后，实施计划由 `plan-v3-actor-workspace` 承接。02 的 Escalation/Directive 协议作为消息类型的子集自然融入，不再需要单独实施。
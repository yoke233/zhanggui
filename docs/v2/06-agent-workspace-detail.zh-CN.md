# Agent 工作空间：动态多 Agent 协作模型

> **取代**: [02-Escalation/Directive](02-escalation-directive-pattern.zh-CN.md) — Agent 消息模型取代了硬编码的 directive/escalation 类型。
> **参考**: [IronClaw 架构学习](ironclaw-architecture-study.zh-CN.md) — 8 项能力吸收来源。
> **对接**: [spec-context-memory](../spec/spec-context-memory.md) — Agent 记忆系统的 MemoryStore 后端规范。
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

把 Agent 从"被调用的函数"变成"常驻的 Agent"，系统从"流水线调度器"变成"团队工作空间"。

## 核心概念

### 术语

| 术语 | 含义 | 对应 |
|------|------|------|
| **AgentRuntime** | 执行协议层 — 怎么启动进程、通信协议。当前实现是 ACP (stdio JSON-RPC)，未来可替换 | config `runtimes`（原 `agents.profiles`）|
| **Agent** | 画像定义 — instruction（基础指令）+ skills（技能文件）+ capabilities（能力边界）+ 引用哪个 runtime | config `agents`（原 `roles`）|
| **AgentInstance** | 运行实例 — Agent 定义 + workspace + inbox + memory + 状态。一个 Agent 可以有多个并发实例 | DB `agent_instances`（原 `agents`）|

```go
// AgentRuntime — 执行协议层（可插拔，当前 ACP）
type AgentRuntime struct {
    Name            string
    LaunchCommand   string
    LaunchArgs      []string
    CapabilitiesMax SandboxPolicy   // 该 runtime 支持的能力上限
}

// Agent — 画像定义（原 Role）
type Agent struct {
    Name         string
    Runtime      string           // 引用 AgentRuntime.Name
    Instruction  string           // 基础指令（原 prompt_template）
    Skills       []string         // 技能文件路径（configs/skills/*.md）
    Capabilities SandboxPolicy    // ≤ Runtime.CapabilitiesMax
    Description  string
}

// AgentInstance — 运行实例
type AgentInstance struct {
    ID        string
    Agent     string              // 引用 Agent.Name
    Workspace string
    Status    string              // idle / busy / sleeping / dead
    Memory    MemoryStore
    Inbox     *Inbox
    // ... 其他运行时状态
}
```

> **Skill 激活子集**: Skills 是文件资产（`configs/skills/*.md`），定义在 Agent 画像中。每次处理消息时，只根据消息内容激活相关子集注入上下文，不是全量加载。

### AgentInstance（运行实例）

一个有持久身份的 Agent 实例。

```
AgentInstance = Agent 画像 + 记忆 + 工作目录 + 收件箱
```

| 属性 | 说明 |
|------|------|
| ID | 唯一标识，如 `agent-coder-01` |
| Agent | 画像定义（instruction、skills、capabilities） |
| Memory | 持久记忆（通过 MemoryStore 接口，跨 session 存续） |
| Workspace | 独立工作目录（worktree 或自定义路径） |
| Inbox | 消息队列，FIFO |
| Status | `idle` / `busy` / `sleeping` / `dead` |

**Session 原则上是一次性的。** 每条消息处理时开 ACP session，处理完提交记忆后可销毁。同 thread 的后续消息可复用 warm session（省去重建上下文），空闲超时后回收。Agent 的持久性靠 Memory，不靠保持 session alive。

### Inbox（收件箱）

每个 Agent 有一个收件箱。发消息 = 投递到收件箱，**立刻返回**，不阻塞发送方。

```
TL 给 Coder 发消息：
  TL → Gateway.Send(to=coder, msg) → Coder.Inbox.Push(msg) → 返回（TL 继续干别的）

  ...稍后...

  Coder 空闲 → Coder.Inbox.Pop() → 处理消息 → 可能回复 TL
```

**这就是 Actor 模型。** 没有同步调用，没有阻塞等待。

### Gateway（网关）

消息路由中心。所有 Agent 间通信经过 Gateway。

```
┌─────────────────────────────────────────────────┐
│                   Gateway                        │
│                                                  │
│  路由表: Agent ID → Inbox 地址                    │
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
- **Session → Thread → Turn** — 对话按层级组织（我们只采用 Thread 层级，Session 不作为持久组织单元）

核心洞察：**消息的"类型"不需要在协议层硬编码。** TL 给 Coder 发的"实现这个功能"和用户给 TL 发的"帮我修个 bug"，在消息结构上完全一样 — 都是 `IncomingMessage`。语义（指令/上报/问答）由 Agent 自己从内容理解，不由消息字段定义。

#### 输入消息

```go
// IncomingMessage — 所有输入的统一结构
// 参考 IronClaw src/channels/channel.rs:14-70
// IncomingMessage — 消息本体（接收者在 agent_inbox 中，不在消息里）
type IncomingMessage struct {
    ID         string            // 唯一消息 ID
    Channel    string            // 来源通道: "internal" / "mcp" / "a2a" / "web"
    FromID     string            // 发送者标识（Agent ID 或外部用户 ID）
    FromName   string            // 可选显示名
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
    AgentID    string            // 产生此状态的 Agent
    ThreadID   string            // 关联的线程
    Data       map[string]any    // 类型特定数据
    Timestamp  time.Time
}
```

StatusUpdate **只推送给外部通道**（Web SSE/WebSocket、MCP 长连接），不进 Inbox，不在 Agent 间流转。Agent 间通信只用 IncomingMessage。外部调用方不在线时，关键状态变化（如任务完成、失败）转为 IncomingMessage 投递到其虚拟 Agent Inbox。

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

#### 会话模型：一次性 Session + Warm Cache

```
Session 生命周期 = ACP 进程的一次存活期。

默认流程（无 warm cache 命中）:
  消息到达 → 开 session → 注入上下文(Memory + 近期消息) → 干活 → 提交记忆 → 关 session

Warm cache 命中（同 Agent 同 thread 的后续消息）:
  消息到达 → 复用 warm session（LLM 上下文还在内存） → 干活 → 提交记忆 → 继续 warm 等待
  空闲超时 → commit memory → 销毁 session

关键: Thread 通过 Memory 跨 session 串联。即使 warm session 过期，下次从 Memory 重建即可，不丢信息。
```

同一个 Agent 可以同时开多个 session 处理不同消息（见"消费循环 — Dispatcher 模式"）。

```toml
[agent_pool.session]
max_per_instance = 3          # 单实例最多同时 3 个 warm session
idle_timeout     = "1h"       # 空闲 1 小时 → commit memory → 销毁 session
```

Session key = `(agent_id, thread_id)`。同 Agent 同 thread 的消息复用 session。无 thread 的一次性消息用完即回收。

```go
// Thread — 持久上下文容器，有独立生命周期（open/closed）
// 所有参与者共享 thread 内的消息历史作为 session 上下文

// Session 上下文构建
type SessionContext struct {
    Instruction    string            // Agent 基础指令
    MatchedSkills  []Skill           // 按消息内容匹配的 Skills
    ThreadMemory   []MemoryEntry     // 从 MemoryStore 召回的 thread 相关记忆
    RecentMessages []AgentMessage    // 同 thread 最近 N 条消息（短期上下文）
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
| `AgentMessage.Type = directive` | `IncomingMessage` (内容隐含指令语义) | Agent 自己理解意图，不靠字段 |
| `AgentMessage.Type = escalation` | `IncomingMessage` (内容隐含上报语义) | 同上 |
| `AgentMessage.Type = query` | `IncomingMessage` + `ThreadID` 串联 | 对话在线程内自然发生 |
| `AgentMessage.Type = notify` | `StatusUpdate` 或 `IncomingMessage` | 实时状态走 StatusUpdate，离线通知走 Inbox |
| `AgentMessage.Priority` | Inbox 排序策略（不在消息里） | 优先级是 Inbox 的事，不是消息的事 |
| `AgentMessage.ReplyTo` | `ThreadID` | 线程天然串联对话 |
| `AgentMessage.ExpiresAt` | Inbox 死信策略（不在消息里） | 超时是 Inbox 的事，不是消息的事 |

## TL 的 Agent 管理技能

TL 是工作空间的管理者。通过对话创建、配置、管理 Agent。

### MCP 工具集

```
Agent 画像管理：
  create_agent(name, runtime, capabilities, instruction, skills, description)
  update_agent(name, ...)
  delete_agent(name)
  list_agents()

实例生命周期：
  spawn(agent, workspace_path?)          → 从 Agent 画像创建并启动实例
  kill(instance_id)                      → 停止并销毁
  sleep(instance_id)                     → 休眠（释放进程，保留状态）
  wake(instance_id)                      → 唤醒（恢复进程和状态）
  list_instances()                       → 查看所有活跃实例

Thread（讨论组）：
  create_thread(title, participants, bridge?)  → 建讨论组，返回 thread_id
  list_threads(status?)                        → 查所有讨论组
  close_thread(thread_id)                      → 关闭讨论
  link_issue(thread_id, issue_id)              → 讨论关联到代码任务

消息：
  send_message(to, content, thread_id?)     → 发消息
                                              to = agent_id / [agent_id, ...] / "@thread"
                                              "@thread" = 展开为 thread.participants 全员
  check_inbox(limit?)                       → 查看自己的收件箱（返回 pending 消息列表）
  approve_rounds(thread_id, extra=N)   → TL 批准 worker 间继续对话 N 轮
  deny_escalation(thread_id, reason?)  → TL 拒绝继续，可附带处理指令
```

### TL 对话示例

```
Human: "我们需要一个专门处理数据库迁移的角色"

TL:  → create_agent(
         name="db-specialist",
         runtime="acp-claude",
         capabilities={fs_read: true, fs_write: true, terminal: true},
         instruction="你是数据库迁移专家，熟悉 PostgreSQL 和 SQLite...",
       )
     → spawn(agent="db-specialist", workspace="/projects/backend")

     "已创建 db-specialist Agent 画像并启动了一个实例。
      他现在在 /projects/backend 工作目录待命。要给他分配任务吗？"
```

### TL 动态编排示例

```
Human: "项目 A 需要重构认证模块，让两个 coder 并行做，一个负责后端 API，一个负责前端适配"

TL:  → spawn(agent="worker", workspace="/projects/A")              // coder-01
     → spawn(agent="worker", workspace="/projects/A")              // coder-02
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
Agent 动态编排 = 非标准、探索性、跨域协作的工作流。

```
用户说 "修一个 bug"
  → TL 判断这是标准任务
  → 走固定流水线（Issue → Run → stages → Done）
  → 流水线内部可以复用常驻 Agent（P1 实现，需要 Agent↔Run 桥接层）

用户说 "重构整个认证模块，涉及三个项目"
  → TL 判断这需要自定义编排
  → 动态创建 Agent、分配任务、协调沟通
  → 不走预定义 stage 模板
```

### 流水线在 Agent 模型下的表达

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

## 实例生命周期

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

Agent 持久性靠 Memory 而非 session，所以休眠非常简单：

```
休眠：
  1. 等待所有活跃 session 完成（warm session 提交记忆后销毁）
  2. 停止 Dispatcher 循环
  3. Agent 状态 → sleeping
  （Memory 在 MemoryStore 中持久化，不受休眠影响）

唤醒：
  1. 重启 Dispatcher 循环
  2. Agent 状态 → idle
  （下一条消息处理时从 Memory 重建上下文，或新建 session）
```

**没有"session 恢复精度"问题** — warm session 只是缓存优化，丢了就从 Memory 重建。Agent 的持久性完全靠 Memory，不靠冻结/解冻进程状态。

### 自动回收策略

```toml
[agent_pool]
max_idle_duration = "30m"       # 空闲超过 30 分钟自动休眠
max_sleeping_duration = "24h"   # 休眠超过 24 小时自动销毁
max_concurrent_instances = 10   # 全局最多同时活跃实例

[agent_pool.inbox]
dead_letter_timeout = "10m"     # 消息超时未处理 → 进入死信队列
dead_letter_notify  = true      # 超时时通知发送方
max_pending_per_instance = 100  # 单实例最大排队消息数
```

## Inbox 设计

### 消息队列语义

```
排序：FIFO（先进先出）

不硬编码优先级。所有消息平等，Agent 自己判断轻重缓急。
如果 Gateway 需要插队（如系统告警），直接 PushFront 即可，不需要优先级字段。
```

### 消费循环 — Dispatcher 模式

Agent 可以同时处理多条消息（各开独立 session），不需要串行等待。

```go
type AgentInstance struct {
    inbox      *Inbox
    control    chan ControlMsg       // cancel / sleep 等元控制，直送不排队
    memory     MemoryStore          // 可插拔记忆后端
    agent      *Agent               // 引用 Agent 画像定义
    maxSessions int                 // 最大并发 session 数（默认 3）
    activeSessions sync.WaitGroup
    semaphore  chan struct{}         // 限制并发
}

// ControlMsg — 不经过 Inbox，直达 Agent
type ControlMsg struct {
    Type string   // "cancel" / "sleep" / "kill"
    Data any
}

func (a *AgentInstance) Run(ctx context.Context) {
    a.semaphore = make(chan struct{}, a.maxSessions)
    for {
        select {
        case ctrl := <-a.control:
            a.handleControl(ctrl)
            continue
        case <-ctx.Done():
            a.activeSessions.Wait()
            return
        default:
        }

        // 获取并发 slot
        a.semaphore <- struct{}{}

        msg, err := a.inbox.Pop(ctx)
        if err != nil {
            <-a.semaphore
            return
        }

        // 每条消息在独立 goroutine + 独立 session 处理
        a.activeSessions.Add(1)
        go func() {
            defer a.activeSessions.Done()
            defer func() { <-a.semaphore }()
            a.processMessage(ctx, msg)
        }()
    }
}

func (a *AgentInstance) processMessage(ctx context.Context, msg *Message) {
    // 1. 构建上下文
    threadMemory, _ := a.memory.Search(ctx, msg.ThreadID, msg.Content)
    recentMsgs, _ := a.store.RecentMessages(msg.ThreadID, 5)

    sessionCtx := SessionContext{
        Instruction:    a.agent.Instruction,
        MatchedSkills:  matchSkills(a.agent, msg.Content),
        ThreadMemory:   threadMemory,
        RecentMessages: recentMsgs,
    }

    // 2. 获取 session（warm cache 命中则复用，否则新建）
    session := a.getOrCreateSession(sessionCtx, msg.ThreadID)
    response, err := session.Run(ctx, msg.Content)

    // 3. 回复作为新消息存入 agent_messages，inbox 记录引用
    resultMsg := a.gateway.Send(response, msg.ThreadID)
    a.store.UpdateInboxResult(inboxEntry.ID, resultMsg.ID)

    // 4. 提交记忆（session summary + 经验提取）
    a.memory.Commit(ctx, session)

    // 5. session 回收（有 thread → 放回 warm cache 等待复用，无 thread → 直接销毁）
    a.recycleSession(session, msg.ThreadID)
}
```

**Control Channel vs Inbox**:
- **Inbox**: DB 持久化，工作消息，FIFO 排队
- **Control**: 内存 channel，元控制（cancel/sleep/kill），直送不排队

```toml
[agent_pool.session]
max_per_instance = 3           # 单实例最多同时 3 个 session
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

### 实现机制

消息存储与 Inbox 投递分离。消息存一次，通知发 N 次。

```go
// Gateway 投递消息（支持多接收者）
func (gw *Gateway) Send(msg IncomingMessage, toIDs []string) error {
    // 1. 消息本体写入 DB（只写一次）
    if err := gw.store.InsertMessage(msg); err != nil {
        return err
    }

    // 2. 为每个接收者创建 inbox 条目
    for _, agentID := range toIDs {
        if err := gw.store.InsertInbox(msg.ID, agentID); err != nil {
            return err
        }
        // 3. 唤醒接收方的 Inbox 循环
        if inbox := gw.inboxes[agentID]; inbox != nil {
            select {
            case inbox.notify <- struct{}{}:
            default:
            }
        }
    }

    // 4. 桥接到外部平台（只发一次）
    if msg.ThreadID != "" {
        if thread, _ := gw.store.GetThread(msg.ThreadID); thread != nil && thread.BridgeType != "" {
            gw.bridge.Send(thread, msg)
        }
    }

    return nil
}

// Agent 消费循环（从 inbox 表取）
func (inbox *Inbox) Pop(ctx context.Context) (*InboxEntry, error) {
    for {
        // 原子操作: UPDATE agent_inbox SET status='processing' WHERE agent_id=? AND status='pending' ORDER BY claimed_at LIMIT 1
        entry, err := inbox.store.ClaimNextInbox(inbox.agentID)
        if err != nil {
            return nil, err
        }
        if entry != nil {
            return entry, nil
        }
        select {
        case <-inbox.notify:
            continue
        case <-ctx.Done():
            return nil, ctx.Err()
        }
    }
}
```

关键保证：
- **消息存一次**: 多人消息在 `agent_messages` 只有一条记录
- **通知 N 次**: 每个接收者在 `agent_inbox` 有自己的条目和状态
- **外部不重复**: 桥接到 Slack/Discord 只发一次（不按接收者数量重复）
- **崩溃安全**: 消息先写 DB 再通知，进程崩溃后 pending 条目在 DB 中

### 对话串联

通过 **Thread** 串联对话。Thread = 讨论组（群聊），TL 是默认群主：

```
Thread "认证方案讨论" (participants: [TL, coder-01, coder-02, reviewer]):
  TL → @coder-01, @coder-02:  "JWT 还是 session？说说你们的看法"
  coder-01 → TL:               "建议 JWT，原因是..."
  coder-02 → TL:               "session 更简单..."
  TL → @reviewer:              "两个 coder 意见不同，你判断一下"
  reviewer → TL:               "JWT 更合适"
  TL → @coder-01:              "方案定了用 JWT，你来实现"
```

**消息是点对点的，Thread 是共享的上下文。** 每条消息有明确的 from → to，但所有消息在同一个 thread 里。Agent 处理消息时，加载整个 thread 的消息历史（所有参与者的发言都可见）作为 ACP session 上下文。

**TL 是默认群主**: TL 创建 thread、管理参与者、做最终决策。Worker 间可以直接对话（不必经 TL 中转），但受 Router 轮次限制（默认 5 轮），超限自动 escalate 给 TL 仲裁，防止消息爆炸。

## Gateway 设计

### 路由规则

```go
// Router — 消息路由权限检查
type Router interface {
    Check(ctx context.Context, from, to, threadID string) (RouteAction, error)
}

type RouteAction int
const (
    RouteAllow    RouteAction = iota  // 放行
    RouteDeny                          // 拒绝
    RouteEscalate                      // 转 TL 仲裁
)
```

不按消息类型过滤（消息没有类型字段）。只按**谁能给谁发**控制。

默认规则：
- TL 可以给所有人发消息（不受限）
- Worker 可以给 TL 发消息（不受限）
- Worker 间可以直接通信，但受轮次限制
- 同一 thread 内 Worker 间对话最多 N 轮（默认 5），超出 → RouteEscalate
- Escalate 时 Router 自动生成摘要发给 TL，TL 判断：
  a. approve(extra_rounds=N) → 放行，计数器重置
  b. deny + 自己做决策 → TL 直接给相关 Worker 发指令
- 外部（Human / MCP / A2A）默认路由到 TL，可通过 Gateway 路由配置指定其他目标
- 计数维度: (thread_id, worker_pair)，TL 参与的对话不计数

**规则细节：**
- 计数单位: 每条 worker→worker 消息 = 1 轮（同一 worker 连续发多条只算 1 轮）
- 批准机制: TL 收到 escalation 后用 `approve_rounds(thread_id, extra=N)` 工具批准
- 重置行为: 批准后计数器清零，重新计数
- 拒绝行为: TL 用 `deny_escalation(thread_id, reason)` 拒绝，可附带处理指令发给相关 worker

```toml
[gateway.router]
worker_to_worker   = true     # 允许 worker 间直接通信
max_rounds         = 5        # 同 thread 内 worker 间最多 N 轮
escalate_to        = "tl"     # 超限时通知谁
```

### 与外部的桥接

```
Human / A2A → Gateway → TL.Inbox     （外部消息统一进 TL）
              Gateway ← TL.Inbox     （TL 的回复统一出 Gateway）
                      → Human / A2A
```

**TL 是默认的外部接口，不是唯一接口。** 外部消息默认路由到 TL，但 Gateway 支持配置将特定来源/模式的消息路由到其他 Agent 实例。

TL 的"特殊性"只体现在：
- **默认治理职责**: 仲裁、审批、预算管控、越权操作兜底
- **高权限**: 可以 spawn/kill 其他实例、管理 thread、调整 Router 规则
- **默认外部路由**: 未匹配的外部消息 fallback 到 TL

这些都是 role_binding 配置，不是硬编码架构。可以配置多个治理 Agent 分担职责。

## 存储

### 新增表

```sql
-- Agent 画像定义（原 roles，TL 动态创建）
CREATE TABLE agents (
    name         TEXT PRIMARY KEY,
    runtime      TEXT NOT NULL,           -- 引用 AgentRuntime（原 base_agent）
    capabilities TEXT NOT NULL,           -- JSON (= SandboxPolicy，不再单独定义)
    instruction  TEXT,                    -- 基础指令（原 prompt）
    skills       TEXT,                    -- JSON: 技能文件路径列表
    description  TEXT,
    created_by   TEXT,                    -- agent instance ID（通常是 TL）
    created_at   DATETIME,
    source       TEXT DEFAULT 'dynamic'   -- 'static' (config) / 'dynamic' (TL 创建)
);
-- 启动时 config 中的静态 Agent 画像 sync 到此表 (source='static')
-- 同名冲突: static 覆盖 dynamic，日志警告

-- Agent 实例（原 agents）
CREATE TABLE agent_instances (
    id           TEXT PRIMARY KEY,
    agent        TEXT NOT NULL REFERENCES agents(name),  -- 引用 Agent 画像（原 role）
    workspace    TEXT,
    status       TEXT NOT NULL DEFAULT 'idle',  -- idle / busy / sleeping / dead
    display_name TEXT,             -- 外部平台显示名
    avatar_url   TEXT,             -- 外部平台头像
    budget_used  REAL DEFAULT 0,
    last_active  DATETIME,
    created_at   DATETIME
);

-- 讨论组（Thread = 群聊，TL 是默认群主）
CREATE TABLE agent_threads (
    id           TEXT PRIMARY KEY,
    title        TEXT,                -- 讨论主题
    issue_id     TEXT,                -- 可选关联 Issue（讨论转任务时链接）
    created_by   TEXT NOT NULL,       -- 创建者（通常是 TL）
    participants TEXT,                -- JSON: ["agent-tl", "agent-coder-01", "agent-coder-02"]
    -- 外部平台桥接（可选）
    bridge_type  TEXT,                -- "slack" / "discord" / "telegram" / null
    bridge_id    TEXT,                -- 外部 channel/thread ID
    status       TEXT DEFAULT 'open', -- open / closed
    created_at   DATETIME,
    updated_at   DATETIME
);

-- 消息本体（一条消息存一次，属于 thread 或无 thread）
-- 多人消息: Gateway 展开 to 数组，消息存一次，inbox 通知 N 次
CREATE TABLE agent_messages (
    id          TEXT PRIMARY KEY,
    thread_id   TEXT,              -- 所属 thread（NULL = 无 thread 的直接消息）
    from_id     TEXT NOT NULL,     -- 发送者
    channel     TEXT NOT NULL,     -- 来源: "internal" / "mcp" / "a2a" / "web"
    content     TEXT NOT NULL,
    metadata    TEXT,              -- JSON
    created_at  DATETIME
);
CREATE INDEX idx_messages_thread ON agent_messages(thread_id, created_at);

-- Inbox 投递（每个接收者一条，指向消息本体）
CREATE TABLE agent_inbox (
    id          TEXT PRIMARY KEY,
    message_id  TEXT NOT NULL REFERENCES agent_messages(id),
    instance_id TEXT NOT NULL,       -- 接收者（引用 agent_instances.id）
    status      TEXT DEFAULT 'pending',  -- pending / processing / done / expired
    -- 处理结果（回复也是消息，引用 agent_messages）
    result_message_id TEXT REFERENCES agent_messages(id),
    error       TEXT,
    claimed_at  DATETIME
);
CREATE INDEX idx_inbox_instance ON agent_inbox(instance_id, status, claimed_at);

-- 工具执行记录（挂在 inbox 条目上）
CREATE TABLE agent_actions (
    id          TEXT PRIMARY KEY,
    inbox_id    TEXT NOT NULL REFERENCES agent_inbox(id),
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
CREATE INDEX idx_agent_actions_inbox ON agent_actions(inbox_id);
```

### 与现有 Store 的关系

新增 6 张表（agents, agent_instances, agent_threads, agent_messages, agent_inbox, agent_actions），不改现有表。现有的 `issues`、`runs`、`checkpoints` 继续用于固定流水线。Agent 层是叠加的，不是替换的。

**消息与 Inbox 分离**:
- `agent_messages` 只存消息本体（一条消息存一次）
- `agent_inbox` 存投递通知（每个接收者一条，指向消息）
- 好处: 多人消息不重复存、外部平台桥接只发一次、thread 上下文加载查 messages 表即可
- `agent_actions` 挂在 `inbox_id` 上（因为同一条消息，不同 agent 处理时的工具调用不同）

**Thread 设计**:
- Thread = 讨论组（群聊），TL 是默认群主
- `participants` 记录谁在群里，`bridge_type/bridge_id` 可选桥接到外部平台
- Thread 可选关联 Issue（`issue_id`），讨论转任务时链接
- Session 加载上下文时，查询整个 thread 的 `agent_messages`（所有参与者的消息都可见）

## 从 IronClaw 吸收的能力

> 参考: [IronClaw 架构学习笔记](ironclaw-architecture-study.zh-CN.md)
>
> IronClaw 是一个 ~83K 行 Rust 的个人 AI 助手框架，已在生产中验证。以下 8 项能力是 06 设计的重要补充。

### 1. 外部通道抽象（← IronClaw ChannelManager）

**问题**: Gateway 只管 Agent 间路由，但**外部世界怎么接入**没有定义。

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

**虚拟 Agent（外部调用方）**: 外部调用方（IronClaw、Claude Code、人类用户等）是"虚拟 Agent"— 有 Inbox、有 ID，但没有常驻进程。区别：

| | 常驻 Agent | 虚拟 Agent（外部） |
|---|---|---|
| 进程 | Dispatcher 常驻，session 按需创建/复用 | 无（调用方自己管理） |
| 消费方式 | Gateway 自动 Pop 投递 | 主动 `check_inbox()` MCP 工具拉取 |
| ID 来源 | spawn 时分配 | 从 auth token 或 A2A agent card 派生 |
| Inbox | DB + notify channel | DB only（无 notify，被动拉取） |
| 适用场景 | TL、Worker、Reviewer | MCP 调用方、A2A 对端、Web 用户 |

这解决了 MCP 调用方"连上来调一下就走"的通知问题：消息在 DB 中等待，下次 `check_inbox()` 时拉取。

#### GroupBridge — Thread 与外部平台双向桥接

Thread 可选映射到外部平台的 channel/group，消息双向同步：

```go
// GroupBridge — thread 与外部聊天平台的桥接
type GroupBridge interface {
    core.Plugin

    // 创建外部 channel/group
    Create(ctx context.Context, thread *Thread) (externalID string, err error)
    // Agent 消息 → 外部平台（一条消息只发一次）
    Send(ctx context.Context, thread *Thread, msg *IncomingMessage) error
    // 外部平台 webhook → 解析为 IncomingMessage
    ParseIncoming(ctx context.Context, payload []byte) (*IncomingMessage, error)
    // 同步成员（实例 ↔ 外部平台身份映射）
    SyncMembers(ctx context.Context, thread *Thread, instances []*AgentInstance) error
}
```

实现：
- `bridge-slack` — Slack Bot API
- `bridge-discord` — Discord Bot API
- `bridge-telegram` — Telegram Bot API

每个 Agent 通过 `display_name` / `avatar_url` 映射为外部平台的 bot 身份。消息流：

```
Agent 发消息 → Gateway 存 agent_messages + 投递 agent_inbox
                     → bridge.Send() → 外部平台 channel（一次）

外部用户在 Slack 发言 → Slack webhook → Gateway
                      → bridge.ParseIncoming() → IncomingMessage
                      → 路由到 TL.Inbox（外部消息统一进 TL）
```

### 2. Agent 执行隔离（← IronClaw WASM 沙箱）

**问题**: TL 动态创建的 Agent，它的 ACP session 能做什么？没有安全边界。

IronClaw 的做法：WASM 沙箱 + 能力声明 + 白名单 + 燃料计量 + 内存限制。

**吸收**:

```go
// SandboxPolicy = 扩展版 capabilities
// 存储在 agents.capabilities JSON 字段中，不再单独定义
// 是 ACP capabilities (fs_read/fs_write/terminal) 的超集
type SandboxPolicy struct {
    // ACP 基础能力（与 config 中 agents.capabilities 一致）
    FSRead     bool           // 文件读
    FSWrite    bool           // 文件写（限定 workspace 目录）
    Terminal   bool           // shell 执行

    // 扩展安全边界
    Network    []string       // 网络白名单（空 = 禁止）
    MCP        bool           // 是否能调 MCP 工具
    AllowTools []string       // 工具白名单（空 = 全部允许）
    DenyTools  []string       // 工具黑名单
}
```

- TL 创建 Agent 画像时设定 SandboxPolicy（= 画像的完整能力声明）
- 动态创建的 Agent 默认 `restricted`（FSRead only，无 shell/network）
- config 里静态定义的 Agent 可以 `full`
- ACP agent 的 `capabilities_max` 是上限，SandboxPolicy 不能超过它

### 3. Skill 动态注入（← IronClaw SKILL.md + selector + attenuation）

**问题**: Agent 指令在 `create_agent` 时写死。Agent 处理不同消息时无法自适应。

IronClaw 的做法：
- SKILL.md 文件定义提示词 + 激活条件（关键词、正则、标签）
- Selector 按消息内容自动匹配相关 skills，多 skill 可叠加
- 信任级别衰减：低信任 skill 激活时，自动降低可用工具上限

**吸收**:

```
Agent 处理消息时:
  1. 加载 Agent 基础指令（Agent.Instruction）
  2. 根据消息内容匹配 Skills（关键词、标签）
  3. 叠加匹配的 Skill 上下文到系统提示
  4. 根据 Skill 信任级别衰减可用工具

Skill 来源:
  - 内置 skills（系统自带，Trusted）
  - 项目级 skills（.ai-workflow/skills/，Trusted）
  - 外部安装 skills（注册表，Installed = 低信任）
```

这让 Agent 可以是"通才 + 按需专项技能"，而不是创建时定死能力。

### 4. 成本控制（← IronClaw CostGuard）

**问题**: 开放问题 #3 "10 个常驻 Agent 的成本怎么控"没有方案。

IronClaw 的做法：`CostGuard` 跟踪日预算、小时速率、单作业上限，超限则拒绝执行。

**吸收**:

```go
type AgentPool struct {
    // 现有
    MaxConcurrentInstances int
    MaxIdleDuration        time.Duration
    MaxSleepingDuration    time.Duration

    // 成本控制
    DailyBudget        float64        // 全局日预算（美元）
    PerInstanceBudget  float64        // 单实例日上限
    PerMessageBudget   float64        // 单消息处理上限
}
```

- 每次 LLM 调用记录 token 消耗和估算成本
- Agent 超预算 → 自动休眠 → Gateway 通知 TL
- TL 可调整预算或决定是否继续

```toml
[agent_pool]
daily_budget        = 50.0     # 全局日预算 $50
per_instance_budget = 10.0     # 单实例日上限 $10
per_message_budget  = 2.0      # 单消息上限 $2
```

### 5. 自我修复（← IronClaw self_repair + heartbeat）

**问题**: Agent 挂了怎么办没有设计。

IronClaw 的做法：心跳检测 + 卡住检测（超时无输出）+ 自动恢复（重启 + 注入历史）。

**吸收**:

```
Gateway 健康管理:
  - 每 N 秒检查所有 busy Agent 的心跳
  - Agent 无响应超过阈值 → 标记 dead
  - 通知 TL，由 TL 决策：
    a. restart（wake 注入历史）
    b. 转发当前消息给其他 Agent
    c. 放弃并通知消息发送方

  - Agent 进程崩溃 → 自动尝试 wake（注入历史恢复上下文）
  - 连续 N 次恢复失败 → 标记 dead，不再自动恢复
```

```toml
[agent_pool.health]
heartbeat_interval   = "30s"
busy_timeout         = "10m"    # busy 超过 10 分钟无心跳 → 异常
max_auto_restarts    = 3        # 连续自动恢复上限
```

### 6. 记忆系统（← IronClaw Workspace + spec-context-memory）

**问题**: Session 是可丢弃的（warm cache 只是优化），Agent 的跨 session 记忆靠什么？

**答案**: 可插拔的 MemoryStore 接口。不自建向量搜索引擎，而是抽象为基础设施层。

> 详细设计参考 [spec-context-memory.md](../spec/spec-context-memory.md)。本节只定义 Agent 层如何对接。

#### MemoryStore 接口

```go
// MemoryStore — Agent 记忆的通用接口
// 后端可插拔: OpenViking / SQLite / 其他
type MemoryStore interface {
    core.Plugin

    // 记忆读写
    Search(ctx context.Context, agentID string, query string, opts SearchOpts) ([]MemoryEntry, error)
    Save(ctx context.Context, agentID string, entry MemoryEntry) error

    // Session 生命周期（对接后端的 session.Commit 机制）
    CreateSession(ctx context.Context, agentID string, sessionID string) (MemorySession, error)

    // 项目知识（L0/L1 预消化）
    Overview(ctx context.Context, uri string) (string, error)
    Abstract(ctx context.Context, uri string) (string, error)

    // 资源导入
    AddResource(ctx context.Context, path string) error
}

// MemorySession — 单次消息处理期间的记忆会话
type MemorySession interface {
    // 记录对话（用于 Commit 时提取经验）
    AddMessage(role string, content string) error
    // 提交: 自动提取 cases/patterns，去重写入
    Commit() error
}

type MemoryEntry struct {
    ID        string
    Category  string            // "case" / "pattern" / "summary"
    Content   string
    Tags      []string
    ThreadID  string            // 可选，关联的 thread
    CreatedAt time.Time
}

type SearchOpts struct {
    ThreadID  string            // 按 thread 过滤
    Category  string            // 按类别过滤
    Limit     int
}
```

#### 后端实现

| 后端 | 能力 | 适用场景 |
|------|------|---------|
| **OpenViking** | L0/L1 预消化 + 语义搜索 + 自动经验提取(cases/patterns) + 向量去重 | 生产环境（推荐） |
| **SQLite** | 基础 CRUD + 全文搜索（FTS5）| 开发/测试、无外部依赖场景 |
| **Mock** | 内存 map | 单元测试 |

```toml
[memory]
provider = "openviking"          # openviking / sqlite / mock

[memory.openviking]
url     = "http://localhost:1933"
api_key = ""                     # dev 模式留空

[memory.sqlite]
path = ".ai-workflow/memory.db"  # SQLite fallback
```

降级行为: OpenViking 不可用 → 自动降级到 SQLite（Save/Search 可用，L0/L1/自动提取不可用）。

#### Agent ID 映射

```
OpenViking 三元组:
  account_id = "default"          # 部署实例
  user_id    = "system"           # ai-workflow 系统用户
  agent_id   = Agent ID           # 每个 Agent 独立 namespace

例:
  agent-tl-01      → agent_id="agent-tl-01"      (TL 的记忆)
  agent-coder-01   → agent_id="agent-coder-01"   (Coder #1 的记忆)
  agent-coder-02   → agent_id="agent-coder-02"   (Coder #2 的记忆，与 #1 隔离)
```

不再按画像名映射（同一 Agent 画像可能有多个实例，记忆需要隔离）。

#### Agent 记忆流转

```
Agent 处理消息时的完整流程:

  1. 构建上下文
     memorySession = memoryStore.CreateSession(agentID, msgID)
     threadMemory  = memoryStore.Search(agentID, msg.Content, {ThreadID: msg.ThreadID})
     recentMsgs    = store.RecentMessages(msg.ThreadID, 5)

  2. 获取或新建 ACP session（warm cache 命中复用，否则新建）
     每轮对话: memorySession.AddMessage(role, content)

  3. 提交记忆
     memorySession.Commit()
     → 后端自动: 提取 cases/patterns → 向量去重 → 写入 Agent namespace
     → 不需要手动管理 session summary 压缩 — 后端处理

  4. Session 回收（有 thread → warm cache 等复用，无 thread → 销毁）

下次同 thread 新消息到达时:
  memoryStore.Search(agentID, newMsg.Content, {ThreadID: threadID})
  → 自动召回相关 cases/patterns，不召回无关的
  → 没有"上下文爆炸"问题 — 语义搜索只返回相关的
```

#### 所有 Agent 都可查询

不再限制"只有 TL 查"。任何 Agent 都可以使用 memory_search/context_overview 工具：

| 角色 | 记忆写入 | 记忆搜索 | 项目知识(L0/L1) |
|------|---------|---------|----------------|
| TL | session.Commit() 自动 | memory_search 按需 | context_overview 按需 |
| Worker | session.Commit() 自动 | memory_search 按需 | context_overview 按需 |
| Reviewer | session.Commit() 自动 | memory_search 按需 | context_overview 按需 |
| 动态 Agent | session.Commit() 自动 | memory_search 按需 | context_overview 按需 |

区别只在于不同 Agent 的 namespace 隔离。TL 可以通过 memory_search 指定其他 Agent ID 来读取（只读）其经验 — 这是"跨 Agent 记忆共享"的实现。

### 7. 例程 / 自主触发（← IronClaw routine_engine）

**问题**: Agent 纯被动——只有收到消息才工作。

IronClaw 的做法：`routine_engine.rs` 支持 Cron 表达式和事件触发，独立于用户消息。

**吸收**:

```go
type Agent struct {
    // ... 现有字段

    // 自主行为
    Routines []AgentRoutine
}

type AgentRoutine struct {
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
- Reviewer Agent: `cron("0 9 * * *")` → 每天早上自动检查待审 PR
- Monitor Agent: `event("run_failed")` → Run 失败时自动分析原因
- TL: `interval("30m")` → 每半小时汇总所有项目进度

例程触发时，Gateway 生成一条 `system` 类型消息投递到 Agent Inbox，Agent 像处理普通消息一样处理。

### 8. 消息安全（← IronClaw SafetyLayer）

**问题**: Agent 间消息没有安全检查。如果一个 Agent 被提示注入，它可以给其他 Agent 发恶意指令。

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
  - Agent 的工具执行结果经过 SafetyLayer 再注入对话
  - 防止工具返回值中的提示注入（间接注入攻击）
  - 检测到敏感信息 → 脱敏后再传递

Agent 间通信安全:
  - Worker 给其他 Worker 发消息时，消息内容经过注入检测
  - 防止"被污染的 Agent A"通过消息污染"干净的 Agent B"
  - 严重威胁自动 escalate 给 TL
```

## 更新后的 Agent 模型

```
                          ┌──────────────────────────────────────────────────┐
                          │                    Gateway                       │
                          │                                                  │
                          │  ── 消息管道（每条消息顺序经过）──               │
                          │                                                  │
                          │  ChannelManager → SafetyLayer → Router → Inbox   │
                          │  (外部通道汇入)   (注入/泄露)   (权限)   (投递)  │
                          │                                                  │
                          │  ── 后台服务（独立 goroutine）──                 │
                          │                                                  │
                          │  HealthCheck     心跳 / 卡住检测 / 自动恢复      │
                          │  CostGuard       全局预算 / 单 Agent / 速率限制  │
                          │  RoutineEngine   Cron / 事件触发 / 生成系统消息  │
                          │  InboxReaper     死信超时检测 / 通知发送方        │
                          │                                                  │
                          └────┬──────────┬──────────┬──────────┬────────────┘
                               │          │          │          │
                           ┌───▼──┐  ┌───▼──┐  ┌───▼───┐  ┌───▼───┐
                           │  TL  │  │Coder │  │Coder  │  │Review │
                           │      │  │#1    │  │#2     │  │er     │
                           ├──────┤  ├──────┤  ├───────┤  ├───────┤
                           │Inbox │  │Inbox │  │Inbox  │  │Inbox  │
                           │Skills│  │Skills│  │Skills │  │Skills │
                           │Memory│  │Memory│  │Memory │  │Memory │
                           │Budget│  │Budget│  │Budget │  │Budget │
                           └──────┘  └──────┘  └───────┘  └───────┘
```

**Agent 属性说明**: 每个 Agent 实例携带的 Inbox/Skills/Memory/Budget 不是独立服务，而是实例的组成部分。SandboxPolicy 定义在 Agent 画像上，实例继承。Routine 定义在 Agent 配置中，由 Gateway 的 RoutineEngine 统一调度。

## 方案对比

### A: 纯 Agent（全部替换）

把流水线也用 Agent 重写。Run 不再是一等概念。

- 优点：统一模型，没有两套逻辑
- 缺点：改动巨大，现有测试全部失效，风险高

### B: Agent 层叠加（推荐）

保留流水线引擎。新增 Agent 层用于动态协作。流水线内部可选择复用常驻 Agent。

- 优点：增量实施，现有功能不受影响，两种模式按场景选用
- 缺点：两套执行模型并存，维护成本稍高

### C: 流水线渐进 Agent 化

先实现 Agent 基础设施（Inbox + Gateway）。然后逐步把流水线 stage 改为 Agent 消息。最终流水线变成 Agent 编排的一种"宏"。

- 优点：最终统一，但过程渐进
- 缺点：过渡期两套逻辑都在，统一时间不确定

**推荐 B，以 C 为北极星。** 先把 Agent 跑起来，验证价值，再决定是否统一。

## 实施路径

| 阶段 | 内容 | 依赖 |
|------|------|------|
| **P0: 基础通信** | | |
| P0 | Gateway + Inbox + AgentMessage 存储 + 基础路由 | 无 |
| P0 | SafetyLayer 消息管道（注入检测 + 泄露扫描） | Gateway |
| P0 | 外部通道注册（MCP / A2A / Web 作为外部 Agent） | Gateway |
| P0 | TL 常驻 Agent（第一个 Dispatcher 实例） | Gateway |
| P0 | TL 的 `send_message` / `check_inbox` MCP 工具 | Gateway + Inbox |
| P0 | Thread 讨论组: create_thread / list_threads / close_thread | Gateway + Inbox |
| P0 | MCP 工具优化: `project_name` resolver + `auto_approve` | MCP Server |
| **P1: 实例生命周期** | | |
| P1 | 实例生命周期管理（spawn / kill / sleep / wake） | P0 |
| P1 | 执行隔离: SandboxPolicy + 工具白名单/黑名单 | P0 |
| P1 | CostGuard: 全局预算 + 单实例预算 + 超限休眠 | P0 |
| P1 | HealthCheck: 心跳检测 + 卡住恢复 + 自动 restart | P0 |
| P1 | TL 的画像管理工具（create_agent / spawn） | P0 |
| P1 | Worker Agent 复用（流水线 stage 可复用常驻 Agent） | P1 |
| **P2: 动态编排** | | |
| P2 | Skill 动态注入: 按消息匹配 skill + 信任级别衰减 | P1 |
| P2 | 记忆系统: MemoryStore 接口 + OpenViking/SQLite 后端 + memory MCP 工具 | P1 |
| P2 | RoutineEngine: Cron + 事件触发 + 系统消息生成 | P1 |
| P2 | 动态编排（TL 自由组合 Agent 完成非标任务） | P1 |
| P2 | Agent 间 query 通信（对等问答） | P1 + 权限规则 |
| P2 | GroupBridge: Slack/Discord/Telegram 外部平台桥接 | P1 + Thread |
| **P3: 统一** | | |
| P3 | 流水线 Agent 化（stage → Agent message 宏） | P2 验证后 |

## 开放问题

### 已解答（通过 IronClaw 研究）

1. ~~**资源预算**~~ → CostGuard 方案（§4 成本控制）：日/Agent/消息三级预算 + 超限自动休眠
2. ~~**人类参与模式**~~ → 外部通道抽象（§1）：Human 是外部 Agent，通过 MCP/Web 通道接入，Inbox = Web UI 通知面板
3. ~~**与 A2A 的关系**~~ → 外部通道抽象（§1）：A2A 是一种外部通道，消息进 Gateway 统一处理，TL 不需要区分来源

### 已解答（通过 Agent 模型迭代）

4. ~~**ACP session 休眠精度**~~ → 问题消失。Session 是可丢弃的（warm cache 只是优化），不需要冻结/恢复。Agent 持久性靠 MemoryStore，不靠 session 序列化。
5. ~~**跨 Agent 记忆共享**~~ → MemoryStore.Search 可指定其他 Agent ID（只读）。ACL 由 MemoryStore 后端实现（OpenViking 原生支持 agent_id 隔离）。

### 已解答（通过 Router 重设计）

6. ~~**Agent 间聊天失控**~~ → Router 轮次限制（§Gateway 路由规则）：Worker 间通信受 max_rounds 限制，超出自动 escalate 给 TL 仲裁。TL 可 approve 继续或直接做决策。CostGuard 的 per_message_budget 是第二道防线。

### 仍然开放

7. **Skill 信任边界** — 外部安装的 Skill 包含提示词，可能被投毒。需要参考 IronClaw 的信任模型（Installed vs Trusted）和权限衰减机制，但 Go 侧没有 WASM 沙箱，隔离程度取决于 ACP agent 的能力约束。

8. **MemoryStore 降级体验** — OpenViking 不可用时降级到 SQLite，L0/L1 和自动经验提取不可用。需要评估对 Agent 智能程度的影响。

9. **外部桥接噪音控制** — Worker 直接对外桥接时（如多个 Agent 实例映射到同一个 Slack channel），如何避免噪音爆炸。可能需要 GroupBridge 层的消息过滤/聚合策略。

10. **术语与现有 config 迁移** — AgentRuntime / Agent / AgentInstance 三层术语与现有 config（`agents.profiles` / `roles`）的迁移路径。需要设计 config 向后兼容或一次性迁移方案。

---

> **后续**: 本设计确定后，实施计划由 `plan-v3-agent-workspace` 承接。02 的 Escalation/Directive 协议作为消息类型的子集自然融入，不再需要单独实施。
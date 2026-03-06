# Agent 工作空间最小迁移路线图

> **前置设计**: [06-agent-workspace](06-agent-workspace.zh-CN.md) — 方向概述
> **消息模型**: [07-thread-message-inbox-bridge](07-thread-message-inbox-bridge.zh-CN.md) — Thread/Message/Inbox 收敛
> **领域模型**: [08-multi-agent-core-domain-model](08-multi-agent-core-domain-model.zh-CN.md) — 四域划分、Task 中心
> **详细参考**: [06-agent-workspace-detail](06-agent-workspace-detail.zh-CN.md) — 完整代码、Schema

## 要解决的问题

`session_id` 同时承担三种职责，已成为架构瓶颈：

| 职责 | 当前实现 | 问题 |
|------|----------|------|
| 聊天上下文 | `Issue.SessionID` → `ChatSession` | session 同时是调度分组键 |
| 调度分组 | `groupIssuesBySession()` 按 sessionID 分组 | 无独立调度键，fallback 到 `"project:"+projectID` |
| ACP 会话 | `Checkpoint.AgentSessionID` / `acpPool` | 与上两者混淆 |

**证据**: `scheduler_helpers.go:216` `makeSessionID()` — 没有 sessionID 时用 projectID 凑；`scheduler_helpers.go:224` `groupIssuesBySession()` — 把聊天分组当调度分组用。

## 迁移原则

1. **叠加不替换** — 新表新接口叠加到现有 Issue/Run 之上，现有流水线不动
2. **session_id 解耦** — 聊天上下文 → Thread，调度分组 → 独立键，ACP 会话 → 纯协议层
3. **接口先行** — 可插拔接口先定义 + no-op 实现，后续逐步填充
4. **最小改动** — 每阶段只改必要的文件，不重构无关代码
5. **现有代码复用** — 工作域已有大量实现，扩展优先于重写

---

## 现有代码 → 08 四域映射

08 定义了四个并列域（协作 / 工作 / 执行 / 记忆）。当前代码覆盖度差异很大。

### 工作域（覆盖度: 高）

当前系统的核心链路 `Issue → Run → ReviewRecord` 已经覆盖了工作域大部分概念。

| 08 概念 | 当前代码 | 位置 | 差距 |
|---------|---------|------|------|
| **Task** | `Issue` | `core/issue.go:127` | 字段基本对齐（title, body, status, priority, depends_on, parent_id）。缺 `owner_actor_id`, `coordinator_actor_id` |
| **Assignment** | 隐式 | `Run.IssueID` + `StageConfig.Role` | **不存在为独立对象**。谁做什么由 stage role 决定 |
| **Review** | `ReviewRecord` | `core/review_record.go:6` | 已有 round, verdict, issues, fixes, score。基本够用 |
| **Decision** | `HumanAction` + `IssueChange` | `core/store.go:47` | 部分覆盖：approve/reject 在 HumanAction，字段变更在 IssueChange。不是统一对象 |
| **Artifact** | `Run.Artifacts` + `IssueAttachment` | `core/run.go:88` | **弱**: Artifacts 是 `map[string]string`，无版本、类型、来源追踪 |

### 执行域（覆盖度: 高）

Pipeline 执行完整，但绑死了 pipeline 策略。

| 08 概念 | 当前代码 | 位置 | 差距 |
|---------|---------|------|------|
| **Execution** | `Run` | `core/run.go:77` | Run = Execution(kind=pipeline)。Stages 数组硬编码了 pipeline，08 希望 kind 可选 |
| **WorkspaceLease** | `Run.WorktreePath` + `WorkspacePlugin` | `core/workspace.go` | 隐式租约，无独立记录 |
| **ToolAction** | `Checkpoint` + `RunEvent(tool_call)` | `core/stage.go:77` | stage 级记录 + 事件流，缺统一审计表 |
| **AgentDefinition** | config YAML `roles` | `configs/defaults.yaml` | 不在 DB，不可动态管理 |
| **RuntimeSession** | `acpPool` (内存) | `engine/executor_acp.go:18` | 仅运行时存在，正确——RuntimeSession 就该是临时的 |

### 协作域（覆盖度: 低 — 需要新建）

| 08 概念 | 当前代码 | 差距 |
|---------|---------|------|
| **Actor** | 不存在 | Human 散落在 `SubmittedBy`/`UserID`，Agent 在 `StageConfig.Role` |
| **Thread** | 不存在 | 讨论上下文混在 `ChatSession.Messages` JSON 里 |
| **Message** | `ChatSession.Messages` (JSON) | 嵌在 ChatSession 里，无独立表 |
| **InboxDelivery** | 不存在 | 无投递概念 |

### 记忆域（覆盖度: 最低）

| 08 概念 | 当前代码 | 差距 |
|---------|---------|------|
| **MemoryEntry** | `ContextStore` 接口 | 面向文件资产管理（URI 体系），不是 agent 记忆 |

---

## 迁移策略：不改名，渐进扩展

**现有代码的工作域和执行域已成熟，不推倒重来。**

| 08 概念 | 代码名 | 策略 |
|---------|--------|------|
| Task | `Issue` (保留) | 加 `owner_actor_id`, `coordinator_actor_id` |
| Execution | `Run` (保留) | 加 `kind` 字段 (默认 "pipeline") |
| Review | `ReviewRecord` (保留) | 已足够 |
| Decision | `Decision` (新建) | 统一 HumanAction 中的审批记录 |
| Assignment | `Assignment` (新建) | 显式化 actor-task 责任关系 |
| Artifact | `Artifact` (新建) | 从 Run.Artifacts + IssueAttachment 收敛 |
| Actor | `Actor` (新建) | 统一参与者身份 |
| AgentDefinition | `Agent` (新建) | 从 config YAML 提升到 DB |
| Thread | `Thread` (新建) | 07 设计 |
| Message | `AgentMessage` (新建) | 07 设计 |
| InboxDelivery | `InboxItem` (新建) | 07 设计 |
| WorkspaceLease | 暂不独立 | `Run.WorktreePath` 够用，P3 再考虑 |
| ToolAction | 暂不独立 | `RunEvent(tool_call)` 够用，P2 再考虑 |
| MemoryEntry | 暂不建 | P2 MemoryStore 时再加 |

### 与 08 MVP 的范围差异

08 建议 P0 包含 9 个对象（Actor, Thread, Message, InboxDelivery, Task, Assignment, Execution, Artifact, Decision）。09 做了降阶：

| 对象 | 08 建议 | 09 安排 | 原因 |
|------|---------|---------|------|
| Thread / Message / Inbox | P0 | **P0** | 协作域空白，必须首波建起 |
| Actor | P0 | **P0** (隐式) | P0 用字符串 ID 表示 actor，不建独立表 |
| Task | P0 | **已有** | Issue 就是 Task，不需要新建 |
| Execution | P0 | **已有** | Run 就是 Execution，不需要新建 |
| Assignment | P0 | **P1** | 当前 Run stage role 能跑，P0 不碰工作域 |
| Decision | P0 | **P1** | HumanAction + IssueChange 能撑住，P0 不碰工作域 |
| Artifact | P0 | **P2** | Run.Artifacts map 够用，独立化风险高 |

**原则: P0 严格收敛为"协作域落地 + session 解耦"，不顺手做工作域扩张。** 首波同时改两个域会让回归测试范围成倍增加。

### ChatSession 兼容策略

P0 引入 Thread/Message 后，ChatSession 不删除，进入双轨期：

| 入口 | P0 行为 | 后续 |
|------|---------|------|
| Web Chat | 继续写 `ChatSession.Messages` JSON | P1 改为写 `agent_messages` |
| MCP `send_message` | 写 `agent_messages` + `agent_inbox` | 新链路 |
| WebSocket 订阅 | 继续用 `session_id` 做 key | P1 增加 `thread_id` 订阅 |
| Issue 创建 | 继续关联 `SessionID` | 同时创建 Thread 关联 |

兼容映射: 对有 `SessionID` 的 Issue，自动创建对应 Thread 并保持双向引用。Web Chat 读取 Thread 时间线（如存在），fallback 到 ChatSession.Messages。

**ACP session 继续保持运行时临时态**（`acpPool` 内存缓存），不落成领域对象。Checkpoint 里的 `agent_session_id` 保留作审计引用。

---

## P0: 协作域基础 + session_id 解耦

> 目标: 协作域从零建起，调度与聊天分离。~700 行新增。
> **严格边界**: 只碰协作域 + 调度键。不碰工作域（Issue/Run/ReviewRecord）、不碰执行域（engine）。

### P0.1 新增 3 张表（协作域）

**文件**: `internal/plugins/store-sqlite/migrations.go` — migration v9

```sql
CREATE TABLE IF NOT EXISTS agent_threads (
    id          TEXT PRIMARY KEY,
    title       TEXT,
    issue_id    TEXT,
    created_by  TEXT NOT NULL,
    participants TEXT NOT NULL DEFAULT '[]',
    status      TEXT NOT NULL DEFAULT 'open',
    metadata    TEXT,
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (issue_id) REFERENCES issues(id) ON DELETE SET NULL
);

CREATE TABLE IF NOT EXISTS agent_messages (
    id         TEXT PRIMARY KEY,
    thread_id  TEXT,
    from_id    TEXT NOT NULL,
    content    TEXT NOT NULL,
    metadata   TEXT,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (thread_id) REFERENCES agent_threads(id)
);

CREATE TABLE IF NOT EXISTS agent_inbox (
    id                TEXT PRIMARY KEY,
    message_id        TEXT NOT NULL,
    actor_id          TEXT NOT NULL,
    status            TEXT NOT NULL DEFAULT 'pending',
    result_message_id TEXT,
    claimed_at        DATETIME,
    handled_at        DATETIME,
    error             TEXT,
    FOREIGN KEY (message_id) REFERENCES agent_messages(id),
    FOREIGN KEY (result_message_id) REFERENCES agent_messages(id)
);
```

### P0.2 Core 类型 + Store 扩展 + Gateway + MCP 工具

- `internal/core/thread.go` — Thread / AgentMessage / InboxItem 类型
- `internal/core/agent_interfaces.go` — SafetyLayer / Router / MemoryStore / CostGuard / GroupBridge
- `internal/plugins/agent-noop/` — 4 个 no-op 实现
- `internal/plugins/store-sqlite/store_thread.go` — DB 实现
- `internal/agent/gateway.go` — SendMessage / CheckInbox
- `internal/mcpserver/tools_agent.go` — `send_message` / `check_inbox`

### P0.3 session_id 解耦

新增 `Issue.SchedulingKey`，调度逻辑从 `groupIssuesBySession` → `groupIssuesBySchedulingKey`。

`SchedulingKey` 默认值 = `"project:"+projectID`，零回归风险。

---

## P1: 工作域补全 + Agent 画像

> 目标: Assignment 和 Decision 显式化，Agent 从 config 提升到 DB。~600 行。

### P1.1 Assignment 表

```sql
CREATE TABLE IF NOT EXISTS assignments (
    id          TEXT PRIMARY KEY,
    issue_id    TEXT NOT NULL,
    actor_id    TEXT NOT NULL,
    role        TEXT NOT NULL,     -- "implement" / "review" / "coordinate"
    status      TEXT NOT NULL DEFAULT 'pending',
    assigned_by TEXT NOT NULL,
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (issue_id) REFERENCES issues(id) ON DELETE CASCADE
);
```

Run 创建时自动从 StageConfig.Role 生成 Assignment 记录。

### P1.2 Decision 表

```sql
CREATE TABLE IF NOT EXISTS decisions (
    id           TEXT PRIMARY KEY,
    subject_type TEXT NOT NULL,    -- "issue" / "assignment" / "artifact"
    subject_id   TEXT NOT NULL,
    action       TEXT NOT NULL,    -- "approve" / "reject" / "abandon" / "accept"
    actor_id     TEXT NOT NULL,
    reason       TEXT,
    metadata     TEXT,
    created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
```

与 HumanAction 双写过渡，后续统一。

### P1.3 Agent 画像入库

`agents` + `agent_instances` 表。启动时从 config YAML 同步。

### P1.4 Issue 扩展

```sql
ALTER TABLE issues ADD COLUMN owner_actor_id TEXT NOT NULL DEFAULT '';
ALTER TABLE issues ADD COLUMN coordinator_actor_id TEXT NOT NULL DEFAULT '';
```

### P1.5 CostGuard 基础实现

日预算 + 单任务上限，替换 Unlimited。

---

## P2: 执行域多策略 + Artifact + 记忆 + 桥接

> ~1500 行。

- **Run.Kind**: `ALTER TABLE runs ADD COLUMN kind TEXT NOT NULL DEFAULT 'pipeline'`
- **Artifact 独立表**: 从 `Run.Artifacts` + `IssueAttachment` 收敛
- **MemoryStore 升级**: FTS5 → 向量搜索
- **GroupBridge**: agent_threads 加 bridge 字段，Slack/Discord adapter
- **Skill 动态注入 + RoutineEngine**

## P3: 流水线 Agent 化

`executeStage()` 变成 Agent 消息宏。`Execution(kind=interactive)` 与 pipeline 统一。

---

## 风险与缓解

| 风险 | 缓解 |
|------|------|
| session_id 改名引入回归 | SchedulingKey 默认值与旧行为一致 |
| P0 只建表不改调度 = 假迁移 | SchedulingKey 是 P0 必做项，不是可选项 |
| ChatSession 与 Thread 双轨并存 | P0 兼容映射，P1 统一读取，渐进替换 |
| Store 接口膨胀 | Thread/Message/Inbox 考虑分离为 AgentStore 子接口 |
| Issue 和 Task 语义漂移 | 不改名，只加字段 |
| Assignment/Decision 与 HumanAction 双轨 | P1 双写，P2 统一 |
| Artifact 多处收敛 | P2 渐进：新写新表，旧数据按需迁移 |
| 首波同时改多域导致回归面过大 | P0 严格只碰协作域 + 调度键 |

## 文件影响矩阵

```
                            P0    P1    P2    P3
internal/core/
  thread.go                 NEW
  agent_interfaces.go       NEW
  agent.go                        NEW
  store.go                  MOD   MOD
  issue.go                  MOD   MOD

internal/agent/
  gateway.go                NEW         MOD   MOD

internal/plugins/
  agent-noop/               NEW         DEL(部分升级)
  store-sqlite/
    migrations.go           MOD   MOD   MOD
    store_thread.go         NEW   MOD

internal/mcpserver/
  tools_agent.go            NEW   MOD
  server.go                 MOD   MOD

internal/teamleader/
  scheduler_helpers.go      MOD
  scheduler.go              MOD
  scheduler_dispatch.go     MOD
  scheduler_events.go       MOD
  manager.go                MOD   MOD

internal/engine/
  executor.go                                 MOD
  executor_acp.go                             MOD
```

# 统一活动流水（Activity Journal）设计方案

> 状态：设计中
>
> 日期：2026-03-14
>
> 关联问题：event_log / action_signals / usage_records / 磁盘审计文件 四源分散，审计和统计需跨表 JOIN

## 1. 问题

当前系统运行时产出的记录分散在 4 个存储位置：

| 存储 | 表/位置 | 内容 |
|------|---------|------|
| event_log (domain) | DB | 状态变更事件、agent 输出、线程事件、通知事件 |
| event_log (tool_audit) | DB | 工具调用记录（ToolCallAudit 编码为 Event） |
| action_signals | DB | 决策信号、反馈、上下文、阻塞/解除 |
| usage_records | DB | token 消耗（按 run 一条） |
| audit/tool-calls/ | 磁盘 | 工具调用完整输入输出原文 |

**审计困难**：还原"run X 发生了什么"需要查 4 张表 + 磁盘文件，没有统一关联键。

**重叠问题**：同一件事在 event_log 和 action_signals 各记一次（如 gate reject 同时产生 `EventGateRejected` 事件和 `SignalReject` 信号），但两者没有关联 ID。

## 2. 设计原则

- **一张流水表**：所有运行时产出的记录写入同一张 `activity_journal`
- **Append-only**：流水表只 INSERT，不 UPDATE/DELETE
- **Spine 不变**：work_items / actions / runs 三张状态表保持现状，只存当前快照
- **大内容外置**：payload 存摘要/digest（< 1KB），完整原文通过 `ref` 指向磁盘文件
- **渐进迁移**：新写入走 journal，旧表保留只读，查询层做适配

## 3. 表结构

### 3.1 activity_journal

```sql
CREATE TABLE activity_journal (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    -- 三级定位：work_item → action → run
    work_item_id  INTEGER,                -- nullable (系统级事件无 work_item)
    action_id     INTEGER,                -- nullable (work_item 级事件无 action)
    run_id        INTEGER,                -- nullable (action 级事件无 run)
    -- 内容分类
    kind          TEXT NOT NULL,           -- 活动类型 (见枚举)
    source        TEXT NOT NULL DEFAULT 'system',  -- agent / human / system
    -- 内容
    summary       TEXT NOT NULL DEFAULT '',         -- 一行摘要
    payload       TEXT,                   -- JSON 详情 (摘要/digest, < 1KB)
    ref           TEXT,                   -- 磁盘大文件引用路径 (nullable)
    -- 元数据
    actor         TEXT NOT NULL DEFAULT '',         -- 谁产生的 (agent_id / user_id / "gate" / "system")
    source_action_id INTEGER,             -- 来源 action (跨 action 反馈时使用)
    -- 时间
    created_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- 核心查询索引
CREATE INDEX idx_journal_run       ON activity_journal(run_id, created_at)       WHERE run_id IS NOT NULL;
CREATE INDEX idx_journal_action    ON activity_journal(action_id, created_at)    WHERE action_id IS NOT NULL;
CREATE INDEX idx_journal_work_item ON activity_journal(work_item_id, created_at) WHERE work_item_id IS NOT NULL;
CREATE INDEX idx_journal_kind      ON activity_journal(kind, created_at);
```

### 3.2 kind 枚举

```go
type JournalKind string

const (
    // 状态变更
    JournalStateChange   JournalKind = "state_change"    // action/run 状态转换

    // 执行过程
    JournalToolCall      JournalKind = "tool_call"       // agent 工具调用
    JournalAgentOutput   JournalKind = "agent_output"    // agent 文本输出 (聚合后)
    JournalUsage         JournalKind = "usage"           // token 消耗

    // 决策与信号
    JournalSignal        JournalKind = "signal"          // 终态信号 (complete/approve/reject/need_help)
    JournalFeedback      JournalKind = "feedback"        // rework 反馈 (gate → upstream)
    JournalContext       JournalKind = "context"         // 上下文信号 (merge_conflict 等)
    JournalProbe         JournalKind = "probe"           // probe 请求/响应

    // 人工操作
    JournalHumanAction   JournalKind = "human_action"    // unblock / override / decision

    // 合并与 SCM
    JournalMergeEvent    JournalKind = "merge_event"     // PR 合并成功/失败

    // 系统
    JournalError         JournalKind = "error"           // 执行错误
    JournalSystem        JournalKind = "system"          // 配置重载、workspace 警告等
)
```

### 3.3 source 枚举

```go
type JournalSource string

const (
    JournalSourceAgent  JournalSource = "agent"
    JournalSourceHuman  JournalSource = "human"
    JournalSourceSystem JournalSource = "system"
)
```

## 4. 现有数据映射

### 4.1 event_log (domain) → activity_journal

| 原 EventType | 新 kind | payload 内容 |
|---------------|---------|-------------|
| `work_item.queued/started/completed/failed/cancelled` | `state_change` | `{"entity":"work_item","from":"open","to":"running"}` |
| `action.ready/started/completed/failed/blocked` | `state_change` | `{"entity":"action","from":"pending","to":"running"}` |
| `run.created/started/succeeded/failed` | `state_change` | `{"entity":"run","from":"created","to":"running"}` |
| `gate.passed` | `signal` | `{"type":"approve","reason":"..."}` |
| `gate.rejected` | `signal` | `{"type":"reject","reason":"...","rework_round":1}` |
| `gate.awaiting_human` | `context` | `{"summary":"merge_conflict",...}` |
| `gate.rework_limit_reached` | `error` | `{"reason":"...","rework_count":3}` |
| `action.need_help` | `signal` | `{"type":"need_help","reason":"..."}` |
| `action.unblocked` | `human_action` | `{"type":"unblock","reason":"..."}` |
| `action.signal` | `signal` | 原始 signal payload |
| `run.agent_output` | `agent_output` | 聚合后的消息/思考/工具调用 |
| `execution.audit` | `system` | `{"kind":"...","status":"...","log_ref":"..."}` |
| `thread.*` | 不迁入 | 保留在 thread 体系（独立领域） |
| `chat.*` | 不迁入 | 保留在 chat 体系 |
| `notification.*` | 不迁入 | 保留在 notification 体系 |
| `manifest.*` | `system` | 特征清单变更 |
| `workspace.*` / `runtime.*` | `system` | 系统级事件 |

### 4.2 event_log (tool_audit) → activity_journal

| 原字段 | 新位置 |
|--------|--------|
| tool_call_id, tool_name, status, duration_ms | `payload` JSON |
| input_digest, output_digest | `payload` JSON |
| input_preview, output_preview, stdout_preview, stderr_preview | `payload` JSON |
| exit_code, redaction_level | `payload` JSON |
| (完整输入输出) | `ref` → 磁盘文件路径 |

kind = `tool_call`，一个工具调用对应一条 journal 记录。

### 4.3 action_signals → activity_journal

| 原 SignalType | 新 kind | 映射 |
|---------------|---------|------|
| `complete` | `signal` | `payload.type = "complete"` |
| `need_help` | `signal` | `payload.type = "need_help"` |
| `blocked` | `signal` | `payload.type = "blocked"` |
| `progress` | `agent_output` | 中间进度上报 |
| `approve` | `signal` | `payload.type = "approve"` |
| `reject` | `signal` | `payload.type = "reject"` |
| `unblock` | `human_action` | `payload.type = "unblock"` |
| `override` | `human_action` | `payload.type = "override"` |
| `feedback` | `feedback` | gate 反馈给上游 |
| `context` | `context` | merge_conflict 等上下文 |
| `instruction` | `human_action` | `payload.type = "instruction"` |
| `probe_request/response` | `probe` | probe 交互 |

**关键变化**：现有 `action_signals` 每条记录的 `summary` + `content` 合并为 journal 的 `summary` + `payload`。`source_step_id` 映射为 `source_action_id`。

### 4.4 usage_records → activity_journal

kind = `usage`，payload 存全部字段：

```json
{
    "agent_id": "worker",
    "profile_id": "default-worker",
    "model_id": "claude-sonnet-4-20250514",
    "input_tokens": 1200,
    "output_tokens": 800,
    "cache_read_tokens": 500,
    "cache_write_tokens": 0,
    "reasoning_tokens": 0,
    "total_tokens": 2000,
    "duration_ms": 3200
}
```

## 5. Store 接口变更

### 5.1 新增 JournalStore

```go
// JournalEntry is a single activity record in the unified journal.
type JournalEntry struct {
    ID             int64          `json:"id"`
    WorkItemID     int64          `json:"work_item_id,omitempty"`
    ActionID       int64          `json:"action_id,omitempty"`
    RunID          int64          `json:"run_id,omitempty"`
    Kind           JournalKind    `json:"kind"`
    Source         JournalSource  `json:"source"`
    Summary        string         `json:"summary"`
    Payload        map[string]any `json:"payload,omitempty"`
    Ref            string         `json:"ref,omitempty"`
    Actor          string         `json:"actor"`
    SourceActionID int64          `json:"source_action_id,omitempty"`
    CreatedAt      time.Time      `json:"created_at"`
}

// JournalFilter constrains journal queries.
type JournalFilter struct {
    WorkItemID *int64
    ActionID   *int64
    RunID      *int64
    Kinds      []JournalKind
    Sources    []JournalSource
    Since      *time.Time
    Until      *time.Time
    Limit      int
    Offset     int
}

type JournalStore interface {
    // 写入
    AppendJournal(ctx context.Context, entry *JournalEntry) (int64, error)
    BatchAppendJournal(ctx context.Context, entries []*JournalEntry) error

    // 通用查询
    ListJournal(ctx context.Context, filter JournalFilter) ([]*JournalEntry, error)
    CountJournal(ctx context.Context, filter JournalFilter) (int, error)

    // 快捷查询（替代现有 Store 方法）
    GetLatestSignal(ctx context.Context, actionID int64, signalTypes ...string) (*JournalEntry, error)
    CountSignals(ctx context.Context, actionID int64, signalTypes ...string) (int, error)
}
```

### 5.2 被替代的 Store 接口

以下接口在迁移完成后可移除（过渡期保留只读）：

```
EventStore (部分)
  - CreateEvent          → AppendJournal
  - ListEvents           → ListJournal
  - CreateToolCallAudit  → AppendJournal (kind=tool_call)
  - ListToolCallAudits   → ListJournal (kind=tool_call, run_id=X)

ActionSignalStore (全部)
  - CreateActionSignal         → AppendJournal (kind=signal/feedback/context/...)
  - GetLatestActionSignal      → GetLatestSignal
  - ListActionSignals          → ListJournal (action_id=X)
  - CountActionSignals         → CountSignals
  - ListPendingHumanActions    → ListJournal (kind=signal, source=human, ...)

UsageStore (写入部分)
  - CreateUsageRecord    → AppendJournal (kind=usage)
  - GetUsageByRun        → ListJournal (kind=usage, run_id=X)
```

### 5.3 保留不变的接口

```
UsageStore (聚合查询部分)
  - UsageByProject / UsageByAgent / UsageByProfile / UsageTotals
  → 改为从 activity_journal WHERE kind='usage' 聚合
  → SQL 查询从 usage_records 改为 activity_journal + JSON_EXTRACT

AnalyticsStore
  → 保持接口不变，底层 SQL 可选择从 journal 聚合

EventStore (只保留 EventBus 相关)
  → EventBus.Publish 仍然用于实时 WebSocket 广播
  → 持久化侧改写到 AppendJournal
  → GetLatestRunEventTime 改为 journal 查询
```

## 6. EventBus 与 Journal 的关系

```
                  ┌──────────────────────────────┐
                  │          EventBus             │
  Publish(event)  │  (内存, 实时广播)               │
  ───────────────▶│                               │
                  │  subscriber A (WebSocket)     │
                  │  subscriber B (Persister)  ───┼──▶ AppendJournal()
                  │  subscriber C (Scheduler)     │
                  └──────────────────────────────┘
```

EventBus 保持现状：内存 pub/sub，用于实时广播和引擎内部协调。

**变化点**：现有的 `Persister` subscriber（将 Event 写入 event_log）改为写入 `activity_journal`。Event 结构体保持不变（bus 内部使用），落盘时转换为 JournalEntry。

thread/chat/notification 类事件仍然走各自的存储路径，不进 journal（它们是独立的领域，不属于执行流水）。

## 7. 迁移策略

### Phase 1：双写（兼容期）

- 新建 `activity_journal` 表
- 所有写入点同时写旧表和新表
- 查询仍走旧表
- 验证新表数据完整性

### Phase 2：读切换

- 查询切换到 journal
- `ActionSignalStore` 方法改为 journal 查询
- `EventStore.ListEvents` 改为 journal 查询
- `UsageStore` 聚合查询改为 journal
- HTTP API / 前端 timeline 切换数据源

### Phase 3：清理

- 移除 `action_signals` 表写入
- 移除 `usage_records` 表写入
- `event_log` 只保留 thread/chat/notification 事件（或也迁入 journal）
- 移除旧 Store 接口

### 旧数据迁移

```sql
-- action_signals → activity_journal
INSERT INTO activity_journal (work_item_id, action_id, run_id, kind, source, summary, payload, actor, source_action_id, created_at)
SELECT
    issue_id, step_id, exec_id,
    CASE type
        WHEN 'complete' THEN 'signal'
        WHEN 'approve' THEN 'signal'
        WHEN 'reject' THEN 'signal'
        WHEN 'need_help' THEN 'signal'
        WHEN 'blocked' THEN 'signal'
        WHEN 'feedback' THEN 'feedback'
        WHEN 'context' THEN 'context'
        WHEN 'unblock' THEN 'human_action'
        WHEN 'override' THEN 'human_action'
        WHEN 'instruction' THEN 'human_action'
        ELSE 'signal'
    END,
    source, summary, payload, actor, source_step_id, created_at
FROM action_signals;

-- usage_records → activity_journal
INSERT INTO activity_journal (work_item_id, action_id, run_id, kind, source, summary, payload, actor, created_at)
SELECT
    issue_id, step_id, execution_id,
    'usage', 'system',
    'token usage: ' || total_tokens || ' tokens',
    JSON_OBJECT(
        'agent_id', agent_id, 'profile_id', profile_id, 'model_id', model_id,
        'input_tokens', input_tokens, 'output_tokens', output_tokens,
        'cache_read_tokens', cache_read_tokens, 'cache_write_tokens', cache_write_tokens,
        'reasoning_tokens', reasoning_tokens, 'total_tokens', total_tokens,
        'duration_ms', duration_ms
    ),
    agent_id, created_at
FROM usage_records;

-- event_log (domain, 执行相关) → activity_journal
INSERT INTO activity_journal (work_item_id, action_id, run_id, kind, source, summary, payload, actor, created_at)
SELECT
    issue_id, step_id, exec_id,
    CASE
        WHEN type LIKE 'work_item.%' OR type LIKE 'action.%' OR type LIKE 'run.%' THEN 'state_change'
        WHEN type LIKE 'gate.%' THEN 'signal'
        WHEN type = 'execution.audit' THEN 'system'
        ELSE 'system'
    END,
    'system', type, data, 'system', timestamp
FROM event_log
WHERE category = 'domain'
  AND type NOT LIKE 'thread.%'
  AND type NOT LIKE 'chat.%'
  AND type NOT LIKE 'notification.%';

-- event_log (tool_audit) → activity_journal
INSERT INTO activity_journal (work_item_id, action_id, run_id, kind, source, summary, payload, actor, created_at)
SELECT
    issue_id, step_id, exec_id,
    'tool_call', 'agent',
    COALESCE(JSON_EXTRACT(data, '$.tool_name'), ''),
    data, 'agent', timestamp
FROM event_log
WHERE category = 'tool_audit';
```

## 8. 查询对比（Before / After）

### "Run X 发生了什么"

**Before**（4 张表）：
```sql
SELECT * FROM executions WHERE id = ?;
SELECT * FROM event_log WHERE exec_id = ? AND category = 'domain';
SELECT * FROM event_log WHERE exec_id = ? AND category = 'tool_audit';
SELECT * FROM action_signals WHERE exec_id = ?;
SELECT * FROM usage_records WHERE execution_id = ?;
-- 再在应用层合并排序
```

**After**（1 张表）：
```sql
SELECT * FROM activity_journal WHERE run_id = ? ORDER BY created_at;
```

### "Action Y 的决策审计链"

**Before**：
```sql
SELECT * FROM action_signals WHERE step_id = ? AND type IN ('approve','reject') ORDER BY created_at;
SELECT * FROM event_log WHERE step_id = ? AND type LIKE 'gate.%' ORDER BY timestamp;
-- 应用层去重合并
```

**After**：
```sql
SELECT * FROM activity_journal
WHERE action_id = ? AND kind IN ('signal', 'human_action')
ORDER BY created_at;
```

### "项目 token 用量统计"

**Before**：
```sql
SELECT project_id, SUM(total_tokens), SUM(duration_ms) FROM usage_records
WHERE project_id = ? AND created_at BETWEEN ? AND ?
GROUP BY project_id;
```

**After**：
```sql
SELECT
    work_item_id,
    SUM(JSON_EXTRACT(payload, '$.total_tokens')),
    SUM(JSON_EXTRACT(payload, '$.duration_ms'))
FROM activity_journal
WHERE kind = 'usage' AND created_at BETWEEN ? AND ?
GROUP BY work_item_id;
```

> 注：usage 聚合查询从 JSON_EXTRACT 比列查询稍慢。如果 usage 查询是高频热点，可以考虑保留 `usage_records` 作为物化视图（由 journal 写入时同步更新），或在 journal 表上加 `total_tokens` 冗余列。

## 9. 表总数变化

| 类别 | 现有 | 合并后 | 变化 |
|------|------|--------|------|
| 状态 (Spine) | 3 (issues, steps, executions) | 3 | 不变 |
| 流水 (Journal) | 3 (event_log, action_signals, usage_records) | 1 (activity_journal) | -2 |
| 对话 | 4 (threads, messages, members, links) | 4 | 不变 |
| 配置 | 6 (projects, profiles, templates, ...) | 6 | 不变 |
| 衍生 | 5 (inspections×3, notifications, features) | 5 | 不变 |
| **总计** | **21** | **19** | **-2** |

表数量减少不多，但关键改善是：**所有执行流水归入一张表，审计和统计不再需要跨表 JOIN**。

## 10. 风险与权衡

| 风险 | 应对 |
|------|------|
| journal 表增长快（高频写入） | append-only + 按时间分区/归档；旧数据可迁移到冷存储 |
| usage 聚合查询性能（JSON_EXTRACT） | 可选方案：保留 usage_records 作为物化视图，或加冗余列 |
| tool_call 记录量大 | payload 只存 digest/preview，大内容走 ref 指向磁盘 |
| 迁移期间双写开销 | Phase 1 期间有写放大，但 SQLite 单机场景下影响可控 |
| ToolCallAudit 的 UPDATE（started → finished） | 改为两条 INSERT：`tool_call_started` + `tool_call_finished`，或合并为一条在 finished 时写入 |

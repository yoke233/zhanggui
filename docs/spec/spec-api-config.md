# API、配置与数据层 — 设计文档

## 概述

本文档定义三件事：Web API 设计（REST + WebSocket）、三级配置体系、以及数据库 Schema。这些是 Web/TUI 前端和运维需要的参考。

## 一、REST API

### 基本约定

- 基础路径：`/api/v1`
- Content-Type：`application/json`
- 认证：Bearer Token（本地使用可关闭）
- 错误格式：`{"error": "message", "code": "ERROR_CODE"}`

### 项目管理

```
GET    /api/v1/projects
  → 200: [{ id, name, repo_path, source, github_repo, pipeline_count, active_count }]

POST   /api/v1/projects
  Body: {
    name,
    source: "local" | "git",
    repo_path: "/path/to/repo",        // source=local 时必填
    git_url: "https://...",            // source=git 时必填
    git_branch: "main",               // source=git 时可选，默认 main
    github: { owner, repo }           // 可选
  }
  → 201: { id, name, repo_path, source, ... }

  source=git 时：
  - 系统 clone 到 ~/.ai-workflow/repos/{id}/
  - repo_path 设为 clone 后的绝对路径
  - git_branch 为初始 checkout 的分支
  - clone 失败返回 400 + 错误信息

GET    /api/v1/projects/:id
  → 200: { id, name, repo_path, source, git_url, config, pipelines_summary }

PUT    /api/v1/projects/:id
  Body: { name?, config? }
  → 200: { id, ... }

DELETE /api/v1/projects/:id
  → 204（需确认无活跃 Pipeline）
  → 删除项目时级联删除关联的 pipelines、checkpoints、logs、human_actions、chat_sessions、issues、issue_attachments、issue_changes、review_records、audit_log（通过应用层事务实现，不依赖 SQLite CASCADE）
  → source=git 时，同时删除 clone 的目录
```

### Pipeline 操作

所有 Pipeline 路由统一在 project scope 下，保持风格一致：

```
GET    /api/v1/projects/:pid/pipelines
  Query: ?status=running&limit=20&offset=0
  → 200: { items: [...], total, offset }

POST   /api/v1/projects/:pid/pipelines
  Body: {
    name: "add-oauth",
    description: "需求描述...",
    template: "full",          // 可选，不传则 AI 推断
    config: {}                 // 可选，Pipeline 级覆盖
  }
  → 201: { id, name, template, stages, status: "created" }

GET    /api/v1/projects/:pid/pipelines/:id
  → 200: {
    id, name, project_id, template, status,
    issue_id, current_stage, stages: [...], artifacts: {...},
    checkpoints: [...], created_at, updated_at
  }

POST   /api/v1/projects/:pid/pipelines/:id/start
  → 200: { status: "running" }

POST   /api/v1/projects/:pid/pipelines/:id/action
  Body: {
    action: "approve" | "reject" | "modify" | "skip" | "rerun"
            | "change_role" | "abort" | "pause" | "resume",
    stage: "implement",         // reject 时必填
    message: "...",             // modify/reject 时的反馈
    role: "worker"              // change_role 时必填
  }
  → 200: { status, current_stage }

GET    /api/v1/projects/:pid/pipelines/:id/logs
  Query: ?stage=implement&limit=100&offset=0
  → 200: {
    items: [{
      id, pipeline_id, stage,
      type,              // stage_start | agent_output | stage_complete
                         // | stage_failed | human_required | action_applied
      agent,             // 可能为空（系统事件）
      content,
      timestamp
    }],
    total, offset
  }

GET    /api/v1/projects/:pid/pipelines/:id/checkpoints
  → 200: [{ stage, status, started_at, finished_at, artifacts }]
```

Pipeline ID 全局唯一，因此提供一个便捷的全局查询端点（只读）：

```
GET    /api/v1/pipelines/:id
  → 200（内部查表补全 project_id，直接返回完整 Pipeline 数据）
```

### 系统信息

```
GET    /api/v1/health
  → 200: { status: "ok", agents: { "<agent_name>": true }, uptime }

GET    /api/v1/stats
  → 200: {
    total_pipelines, active_pipelines,
    success_rate, avg_duration,
    tokens_used: { by_agent: { "<agent_name>": 12345 } }
  }

GET    /api/v1/templates
  → 200: [{ name, stages, description }]
```

### Admin 管理 API（P2c 新增）

```
GET    /api/v1/admin/overview
  → 200: {
    projects: { total, active },
    pipelines: { running, waiting, done_today, failed_today },
    issues: { executing, reviewing, open_total },
    agents: { active, max }
  }

GET    /api/v1/admin/pipelines
  Query: ?project_id=&status=&limit=50&offset=0
  → 200: { items: [...], total }

GET    /api/v1/admin/issues
  Query: ?project_id=&state=&status=&limit=50&offset=0
  → 200: { items: [...], total }

GET    /api/v1/admin/audit-log
  Query: ?project_id=&action=&user=&since=&until=&limit=100&offset=0
  → 200: { items: [...], total }
```

> Issue 的领域模型、状态机和设计背景见 [spec-secretary-layer.md](spec-secretary-layer.md)。

### ChatSession — 持久 Agent Session（P2c 新增）

```
POST   /api/v1/projects/:pid/chat/sessions
  Body: { role: "secretary" }
  → 201: { session_id, role, agent, status: "active" }

POST   /api/v1/projects/:pid/chat/sessions/:sid/messages
  Body: { content: "需求描述..." }
  → 200: { message_id }

GET    /api/v1/projects/:pid/chat/sessions/:sid
  → 200: { id, project_id, agent, status, messages: [...], created_at }

GET    /api/v1/projects/:pid/chat/sessions
  → 200: [{ id, agent, status, message_count, created_at, updated_at }]

DELETE /api/v1/projects/:pid/chat/sessions/:sid
  → 204
```

### 项目文件操作（P2c 新增）

```
GET    /api/v1/projects/:pid/files
  Query: ?path=.ai-workflow/plans/&changed_since=2026-03-01T00:00:00Z
  → 200: { files: [{ path, size, modified_at, is_new }] }

GET    /api/v1/projects/:pid/files/content
  Query: ?paths=file1.md,file2.md
  → 200: { files: [{ path, content, size }] }
```

### Issue 管理（P2a 新增）

> 兼容性说明：以下 `/issues` 路径在迁移期支持 `/plans` 等价别名。

```
POST   /api/v1/projects/:pid/issues
  Body: {
    issues: [
      { title: "用户认证", attachments: ["plan-auth.md", "plan-auth-api.md"] },
      { title: "数据库设计", attachments: ["plan-db.md"] }
    ],
    session_id: "chat-xxx",
    auto_review: true,
    milestone_id: ""
  }
  → 201: { issues: [...], review_job_id: "..." }

GET    /api/v1/projects/:pid/issues
  Query: ?state=open&status=ready&labels=backend&limit=20&offset=0
  → 200: { items: [...], total, offset }

GET    /api/v1/projects/:pid/issues/:id
  → 200: { id, title, body, state, status, labels, attachments,
           depends_on, blocks, priority, template, pipeline_id,
           version, external_id, created_at, updated_at, closed_at }

PATCH  /api/v1/projects/:pid/issues/:id
  Body: { title?, labels?, priority?, attachments?, depends_on?, template? }
  → 200: { ... }

POST   /api/v1/projects/:pid/issues/:id/action
  Body: {
    action: "approve" | "reject" | "abandon",
    feedback?: {
      category: "missing_node" | "cycle" | "self_dependency"
              | "bad_granularity" | "coverage_gap" | "other",
      detail: "...",                  // reject 时必填，最少 20 字符
      expected_direction?: "..."
    }
  }
  → 200: { status }

POST   /api/v1/projects/:pid/issues/batch-action
  Body: { issue_ids: [...], action: "approve" | "abandon" }
  → 200: { results: [...] }

GET    /api/v1/projects/:pid/issues/dag
  → 200: {
    nodes: [{ id, title, state, status, pipeline_id, priority }],
    edges: [{ from, to, reason }],
    stats: { total, draft, queued, ready, executing, done, failed }
  }

GET    /api/v1/projects/:pid/issues/:id/reviews
  → 200: [{ reviewer, verdict, score, issues, created_at }]

GET    /api/v1/projects/:pid/issues/:id/attachments
  Query: ?version=2
  → 200: [{ file_path, content, hash, version, created_at }]

GET    /api/v1/projects/:pid/issues/:id/changes
  → 200: [{ field, old_value, new_value, reason, changed_by, created_at }]

GET    /api/v1/projects/:pid/issues/:id/timeline
  Query: ?kinds=checkpoint,log,action,review,change,audit&limit=50&offset=0
  → 200: {
    items: [{
      event_id,          // e.g. cp:456 / log:123 / review:789
      kind,              // checkpoint | log | action | review | change | audit
      created_at,
      actor_type,        // human | agent | system
      actor_name,
      actor_avatar_seed, // 前端 identicon 种子
      title,
      body,
      status,            // success | failed | running | info | warning
      refs: { issue_id, pipeline_id, stage },
      meta: {}
    }],
    total, offset
  }

GET    /api/v1/projects/:pid/plans/:id/timeline
  → 兼容别名，与 /issues/:id/timeline 响应一致
```

### API 设计规则

- Pipeline 的 action 端点是唯一的 Pipeline 级人工操作入口
- Issue 的 action 端点是 Issue 级操作入口
- `issue_id` 是 Pipeline 的内部关联字段：仅由 DAG Scheduler 在自动创建 Pipeline 时设置
- 分页用 limit + offset，默认 limit=20
- 列表接口支持 state/status 过滤
- Timeline 默认 `limit=50`，服务端按 `created_at ASC` 输出，前端按需倒序渲染

## 二、WebSocket

### 连接

```
WS /api/v1/ws?token={auth_token}
```

### 消息格式

服务端推送（Server → Client）：

```json
{
  "type": "stage_start" | "stage_complete" | "stage_failed"
       | "human_required" | "pipeline_done" | "pipeline_failed"
       | "action_applied"
       | "agent_output" | "system_info"
       | "secretary_thinking"
       | "issue_created" | "issue_reviewing" | "issue_review_done"
       | "issue_dag_proposed" | "issue_approved"
       | "issue_queued" | "issue_ready"
       | "issue_executing" | "issue_done" | "issue_failed"
       | "issue_closed" | "issue_changed",
  "pipeline_id": "xxx",
  "project_id": "yyy",
  "issue_id": "zzz",
  "data": { ... }
}
```

Issue 事件说明：

| 事件 | 用途 |
|------|------|
| `secretary_thinking` | Secretary Agent 流式输出 |
| `secretary_files_changed` | Secretary 写入/修改文件 |
| `secretary_tool_call` | Secretary 调用查询工具 |
| `secretary_tool_result` | 查询工具返回结果 |
| `issue_created` | 新 Issue 创建 |
| `issue_reviewing` | AI 审核开始 |
| `issue_review_done` | 单个 Issue 审核完成（含 verdict, score） |
| `issue_dag_proposed` | 依赖分析完成（含 edges, conflicts） |
| `issue_approved` | Issue 批准执行 |
| `issue_queued` | 等待依赖 |
| `issue_ready` | 可调度 |
| `issue_executing` | Pipeline 启动 |
| `issue_done` | 完成 |
| `issue_failed` | 失败 |
| `issue_closed` | 关闭 |
| `issue_changed` | 字段变更 |

### Chat 流式事件映射

| ACP 来源 | 应用层 WS 事件 | 说明 |
|------|------|------|
| `session`（开始） | `chat_run_started` | 进入运行态 |
| `update.sessionUpdate` | `chat_run_update` | 增量更新 |
| `session`（结束） | `chat_run_completed` | 完成 |
| `session`（失败） | `chat_run_failed` | 失败 |
| `session`（取消） | `chat_run_cancelled` | 已取消 |

透传策略：
- `data.session_id` 必须保留
- `data.acp` 原样透传 ACP 增量对象
- 未知 `acp.sessionUpdate` 静默忽略

持久化策略（2026-03）：
- 服务端持久化 `chat_run_update` 的非 chunk ACP 更新（如 `tool_call` / `tool_call_update` / `plan`）。
- `agent_message_chunk` / `assistant_message_chunk` / `message_chunk` 不逐条落库，仅用于流式展示与最终回复合并。
- 历史运行事件通过 `GET /api/v1/projects/{projectID}/chat/{sessionID}/events` 查询，返回按时间升序的事件数组。

客户端消息（Client → Server）：

```json
{
  "type": "subscribe_pipeline" | "unsubscribe_pipeline"
       | "subscribe_issue" | "unsubscribe_issue",
  "pipeline_id": "xxx",
  "issue_id": "zzz"
}
```

### 订阅与广播

- 状态事件（stage_start / issue_done 等）→ 广播给所有连接
- 输出事件（agent_output）→ 只发给订阅了该 Pipeline 的连接
- Issue 详情事件（review_done / dag_proposed）→ 只发给订阅了该 Issue 的连接
- 心跳：30 秒 ping，连续 2 次无响应断开
- 重连后推送当前状态快照

## 三、配置体系

### 三级配置

```
全局配置 (~/.ai-workflow/config.yaml)
  └── 项目配置 ({repo}/.ai-workflow/config.yaml)
      └── Pipeline 配置 (创建时传入)
```

### 全局配置结构

```yaml
agents:
  - name: claude
    launch_command: "npx"
    launch_args: ["-y", "@zed-industries/claude-agent-acp@latest"]
    env: {}
    capabilities_max:
      fs_read: true
      fs_write: true
      terminal: true

  - name: codex
    launch_command: "npx"
    launch_args: ["-y", "@zed-industries/codex-acp@latest"]
    env: {}
    capabilities_max:
      fs_read: true
      fs_write: true
      terminal: true

roles:
  - name: secretary
    agent: claude
    prompt_template: secretary_system
    capabilities:
      fs_read: true
      fs_write: true
      terminal: true
    permission_policy:
      - pattern: "fs/write_text_file"
        scope: "cwd"
        action: "allow_always"
      - pattern: "terminal/create"
        action: "allow_once"
    mcp:
      enabled: true
      tools: [query_issues, query_issue_detail, query_pipelines, query_pipeline_logs, query_project_stats]
    session:
      reuse: true
      prefer_load_session: true
      session_idle_ttl: 30m

  - name: worker
    agent: codex
    prompt_template: implement
    capabilities:
      fs_read: true
      fs_write: true
      terminal: true
    permission_policy:
      - pattern: "fs/write_text_file"
        scope: "cwd"
        action: "allow_always"
      - pattern: "terminal/create"
        action: "allow_always"
    session:
      reuse: true
      max_turns: 12

  - name: reviewer
    agent: claude
    prompt_template: code_review
    capabilities:
      fs_read: true
      fs_write: false
      terminal: false
    session:
      reuse: true
      max_turns: 6
      reset_prompt: true

  - name: demand_reviewer
    agent: claude
    prompt_template: demand_review
    capabilities:
      fs_read: true
      fs_write: false
      terminal: false
    session:
      reuse: false

  - name: dependency_analyzer
    agent: claude
    prompt_template: dependency_analysis
    capabilities:
      fs_read: true
      fs_write: false
      terminal: false
    session:
      reuse: false

role_bindings:
  secretary:
    role: secretary
  pipeline:
    stage_roles:
      requirements: worker
      implement: worker
      code_review: reviewer
      fixup: worker
      e2e_test: worker
  demand_review:
    reviewer: demand_reviewer
    analyzer: dependency_analyzer

pipeline:
  default_template: standard
  global_timeout: 2h
  auto_infer_template: true
  acp:
    session_strategy: hybrid
    prefer_load_session: true
    session_idle_ttl: 30m
    pause_keep_session: true
    recovery:
      fallback_rehydrate: true
      rehydrate_sources: [artifacts, progress_files, review_issues]

scheduler:
  max_global_agents: 3
  max_project_pipelines: 2

secretary:
  role: secretary
  session_idle_timeout: 30m
  context_max_tokens: 4000
  execution_files_enabled: true

  demand_review:
    enabled: true
    auto_approve: true
    auto_approve_threshold: 80
    auto_approve_on_conflict: false
    check_existing_deps: true
    timeout_per_review: 5m
    roles:
      reviewer: demand_reviewer
      analyzer: dependency_analyzer

  dag_scheduler:
    fail_policy: block
    max_concurrent_tasks: 0
    dispatch_interval: 1s
    stale_check_interval: 5m
    stale_threshold: 30m

github:
  enabled: false
  token: ""
  app_id: 0
  private_key_path: ""
  installation_id: 0
  webhook_secret: ""
  rate_limit:
    requests_per_second: 1
    burst: 5
    retry_on_limit: true
    max_retries: 3

server:
  host: "127.0.0.1"
  port: 8080
  auth_enabled: false
  auth_token: ""

store:
  driver: sqlite
  path: ~/.ai-workflow/data.db

log:
  level: info
  file: ~/.ai-workflow/logs/app.log
  max_size_mb: 100
  max_age_days: 30
```

### 配置合并规则

- 下级覆盖上级，未设置的字段继承上级值
- 指针类型（`*int`, `*string` 等），`nil` 表示未设置
- `null` 显式清空继承值，`[]` 显式清空上级数组
- 配置变更不影响已运行的 Pipeline
- 环境变量优先级最高：`AI_WORKFLOW_{SECTION}_{KEY}`

## 四、数据库 Schema

```sql
PRAGMA journal_mode=WAL;
PRAGMA busy_timeout=5000;
PRAGMA foreign_keys=ON;
```

#### projects

```sql
CREATE TABLE projects (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    repo_path   TEXT NOT NULL UNIQUE,
    source      TEXT NOT NULL DEFAULT 'local',
    git_url     TEXT,
    git_branch  TEXT,
    github_owner TEXT,
    github_repo  TEXT,
    config_json TEXT,
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

#### pipelines

```sql
CREATE TABLE pipelines (
    id          TEXT PRIMARY KEY,
    project_id  TEXT NOT NULL REFERENCES projects(id),
    name        TEXT NOT NULL,
    description TEXT,
    issue_id    TEXT,
    template    TEXT NOT NULL,
    status      TEXT NOT NULL DEFAULT 'created',
    current_stage TEXT,
    stages_json TEXT NOT NULL,
    artifacts_json TEXT DEFAULT '{}',
    config_json TEXT DEFAULT '{}',
    issue_number INTEGER,
    pr_number   INTEGER,
    branch_name TEXT,
    worktree_path TEXT,
    error_message TEXT,
    started_at  DATETIME,
    finished_at DATETIME,
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_pipelines_project ON pipelines(project_id);
CREATE INDEX idx_pipelines_status ON pipelines(status);
CREATE INDEX idx_pipelines_issue ON pipelines(issue_id) WHERE issue_id IS NOT NULL;
```

#### checkpoints

```sql
CREATE TABLE checkpoints (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    pipeline_id TEXT NOT NULL REFERENCES pipelines(id),
    stage       TEXT NOT NULL,
    status      TEXT NOT NULL,
    agent_used  TEXT,
    artifacts_json TEXT DEFAULT '{}',
    tokens_used INTEGER DEFAULT 0,
    retry_count INTEGER DEFAULT 0,
    error_message TEXT,
    started_at  DATETIME NOT NULL,
    finished_at DATETIME,
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_checkpoints_pipeline ON checkpoints(pipeline_id);
```

#### logs

```sql
CREATE TABLE logs (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    pipeline_id TEXT NOT NULL REFERENCES pipelines(id),
    stage       TEXT NOT NULL,
    type        TEXT NOT NULL,
    agent       TEXT,
    content     TEXT NOT NULL,
    timestamp   DATETIME NOT NULL
);

CREATE INDEX idx_logs_pipeline_stage ON logs(pipeline_id, stage);
```

`logs.type` 约定值（P3.5）：
- `stage_start`
- `agent_output`
- `stage_complete`
- `stage_failed`
- `human_required`
- `action_applied`

#### human_actions

```sql
CREATE TABLE human_actions (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    pipeline_id TEXT NOT NULL REFERENCES pipelines(id),
    stage       TEXT NOT NULL,
    action      TEXT NOT NULL,
    message     TEXT,
    source      TEXT NOT NULL,
    user_id     TEXT,
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_human_actions_pipeline ON human_actions(pipeline_id);
```

#### chat_sessions

```sql
CREATE TABLE chat_sessions (
    id          TEXT PRIMARY KEY,
    project_id  TEXT NOT NULL REFERENCES projects(id),
    messages    TEXT NOT NULL DEFAULT '[]',
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_chat_sessions_project ON chat_sessions(project_id);
```

#### issues

```sql
CREATE TABLE issues (
    id            TEXT PRIMARY KEY,
    project_id    TEXT NOT NULL REFERENCES projects(id),
    session_id    TEXT REFERENCES chat_sessions(id),
    title         TEXT NOT NULL,
    body          TEXT,
    labels        TEXT DEFAULT '[]',
    milestone_id  TEXT,
    attachments   TEXT NOT NULL DEFAULT '[]',
    depends_on    TEXT NOT NULL DEFAULT '[]',
    blocks        TEXT NOT NULL DEFAULT '[]',
    priority      INTEGER NOT NULL DEFAULT 2,
    template      TEXT NOT NULL DEFAULT 'standard',
    state         TEXT NOT NULL DEFAULT 'open',
    status        TEXT NOT NULL DEFAULT 'draft',
    pipeline_id   TEXT REFERENCES pipelines(id),
    version       INTEGER NOT NULL DEFAULT 1,
    superseded_by TEXT,
    external_id   TEXT,
    fail_policy   TEXT NOT NULL DEFAULT 'block',
    created_at    DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at    DATETIME DEFAULT CURRENT_TIMESTAMP,
    closed_at     DATETIME
);

CREATE INDEX idx_issues_project ON issues(project_id);
CREATE INDEX idx_issues_state ON issues(state);
CREATE INDEX idx_issues_status ON issues(status);
```

#### issue_attachments

```sql
CREATE TABLE issue_attachments (
    id          TEXT PRIMARY KEY,
    issue_id    TEXT NOT NULL REFERENCES issues(id),
    version     INTEGER NOT NULL,
    file_path   TEXT NOT NULL,
    content     TEXT NOT NULL,
    hash        TEXT NOT NULL,
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_issue_attachments_issue ON issue_attachments(issue_id);
```

#### issue_changes

```sql
CREATE TABLE issue_changes (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    issue_id    TEXT NOT NULL REFERENCES issues(id),
    field       TEXT NOT NULL,
    old_value   TEXT,
    new_value   TEXT,
    reason      TEXT,
    changed_by  TEXT DEFAULT 'system',
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_issue_changes_issue ON issue_changes(issue_id);
```

#### review_records

```sql
CREATE TABLE review_records (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    issue_id    TEXT NOT NULL REFERENCES issues(id),
    reviewer    TEXT NOT NULL,
    verdict     TEXT NOT NULL,
    issues_json TEXT DEFAULT '[]',
    score       INTEGER,
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_review_records_issue ON review_records(issue_id);
```

#### audit_log

```sql
CREATE TABLE audit_log (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    project_id  TEXT,
    action      TEXT NOT NULL,
    target_type TEXT,
    target_id   TEXT,
    user_id     TEXT DEFAULT 'system',
    detail      TEXT DEFAULT '{}',
    timestamp   DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_audit_log_project ON audit_log(project_id);
CREATE INDEX idx_audit_log_action ON audit_log(action);
CREATE INDEX idx_audit_log_timestamp ON audit_log(timestamp);
```

### Store 接口

```go
type Store interface {
    // Projects
    ListProjects(filter ProjectFilter) ([]Project, error)
    GetProject(id string) (*Project, error)
    CreateProject(p *Project) error
    UpdateProject(p *Project) error
    DeleteProject(id string) error

    // Pipelines
    ListPipelines(projectID string, filter PipelineFilter) ([]Pipeline, error)
    GetPipeline(id string) (*Pipeline, error)
    SavePipeline(p *Pipeline) error
    GetActivePipelines() ([]Pipeline, error)

    // Checkpoints
    SaveCheckpoint(cp *Checkpoint) error
    GetCheckpoints(pipelineID string) ([]Checkpoint, error)
    GetLastSuccessCheckpoint(pipelineID string) (*Checkpoint, error)

    // Logs
    AppendLog(entry LogEntry) error
    GetLogs(pipelineID string, stage string, limit int, offset int) ([]LogEntry, int, error)

    // Human Actions
    RecordAction(action HumanAction) error
    GetActions(pipelineID string) ([]HumanAction, error)

    // ChatSessions
    CreateChatSession(s *ChatSession) error
    GetChatSession(id string) (*ChatSession, error)
    UpdateChatSession(s *ChatSession) error
    ListChatSessions(projectID string) ([]ChatSession, error)

    // Issues
    CreateIssue(issue *Issue) error
    GetIssue(id string) (*Issue, error)
    SaveIssue(issue *Issue) error
    ListIssues(projectID string, filter IssueFilter) ([]Issue, int, error)
    GetActiveIssues() ([]Issue, error)
    GetIssueByPipeline(pipelineID string) (*Issue, error)

    // Issue Attachments
    SaveAttachment(att *IssueAttachment) error
    GetAttachments(issueID string, version int) ([]IssueAttachment, error)

    // Issue Changes
    RecordIssueChange(change *IssueChange) error
    GetIssueChanges(issueID string) ([]IssueChange, error)

    // ReviewRecords
    SaveReviewRecord(r *ReviewRecord) error
    GetReviewRecords(issueID string) ([]ReviewRecord, error)

    // AuditLog
    AppendAuditLog(entry AuditEntry) error
    GetAuditLogs(filter AuditFilter) ([]AuditEntry, int, error)
}
```

## 五、ID 生成规则

| 实体 | 格式 | 示例 |
|---|---|---|
| Project | 用户指定，kebab-case | `app-a` |
| Pipeline | `{日期}-{12位hex}` | `20260228-a3f1b2c0d4e6` |
| Checkpoint | 自增整数 | 1, 2, 3... |
| ChatSession | `chat-{日期}-{8位hex}` | `chat-20260301-d4e5f6a7` |
| **Issue** | `issue-{日期}-{8位hex}` | `issue-20260301-a3f1b2c0` |
| **IssueAttachment** | `att-{issue短ID}-{序号}` | `att-a3f1b2c0-1` |
| ReviewRecord | 自增整数 | 1, 2, 3... |

## 六、安全

### 本地使用

- 默认不启用认证（`auth_enabled: false`）
- Web Server 只监听 localhost

### 团队/远程使用

- 启用 Bearer Token 认证
- 配置 TLS 或通过 reverse proxy 处理
- GitHub Token 和 Auth Token 支持环境变量注入
- SQLite 数据库文件权限设为 600

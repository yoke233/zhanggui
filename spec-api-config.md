# API、配置与数据层 — 设计文档

## 概述

本文档定义三件事：Web API 设计（REST + WebSocket）、三级配置体系、以及数据库 Schema。这些是 TUI/Web 前端和运维需要的参考。

## 一、REST API

### 基本约定

- 基础路径：`/api/v1`
- Content-Type：`application/json`
- 认证：Bearer Token（本地使用可关闭）
- 错误格式：`{"error": "message", "code": "ERROR_CODE"}`

### 项目管理

```
GET    /api/v1/projects
  → 200: [{ id, name, repo_path, github_repo, pipeline_count, active_count }]

POST   /api/v1/projects
  Body: { name, repo_path, github: { owner, repo } }
  → 201: { id, name, ... }

GET    /api/v1/projects/:id
  → 200: { id, name, repo_path, config, pipelines_summary }

PUT    /api/v1/projects/:id
  Body: { name?, config? }
  → 200: { id, ... }

DELETE /api/v1/projects/:id
  → 204（需确认无活跃 Pipeline）
  → 删除项目时级联删除关联的 pipelines、checkpoints、logs、human_actions（通过应用层事务实现，不依赖 SQLite CASCADE）
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
    current_stage, stages: [...], artifacts: {...},
    checkpoints: [...], created_at, updated_at
  }

POST   /api/v1/projects/:pid/pipelines/:id/start
  → 200: { status: "running" }

POST   /api/v1/projects/:pid/pipelines/:id/action
  Body: {
    action: "approve" | "reject" | "modify" | "skip" | "rerun"
            | "change_agent" | "abort" | "pause" | "resume",
    stage: "spec_gen",          // reject 时必填
    message: "...",             // modify/reject 时的反馈
    agent: "claude"             // change_agent 时必填
  }
  → 200: { status, current_stage }

GET    /api/v1/projects/:pid/pipelines/:id/logs
  Query: ?stage=implement&limit=100&offset=0
  → 200: { items: [{ timestamp, type, content }], total, offset }

GET    /api/v1/projects/:pid/pipelines/:id/checkpoints
  → 200: [{ stage, status, started_at, finished_at, artifacts }]
```

Pipeline ID 全局唯一，因此提供一个便捷的全局查询端点（只读）：

```
GET    /api/v1/pipelines/:id
  → 200（内部查表补全 project_id，直接返回完整 Pipeline 数据）
```

GitHub Webhook 和 TUI 场景下只有 pipeline_id 没有 project_id 时使用此端点。

### 系统信息

```
GET    /api/v1/health
  → 200: { status: "ok", agents: { claude: true, codex: true }, uptime }

GET    /api/v1/stats
  → 200: {
    total_pipelines, active_pipelines,
    success_rate, avg_duration,
    tokens_used: { claude, codex }
  }

GET    /api/v1/templates
  → 200: [{ name, stages, description }]
```

### API 设计规则

- Pipeline 的 action 端点是唯一的人工操作入口，TUI/Web/GitHub 最终都调用它
- Logs 端点返回的是结构化日志，不是原始 stdout
- 分页用 limit + offset，默认 limit=20
- 列表接口支持 status 过滤

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
       | "agent_output" | "system_info",
  "pipeline_id": "xxx",
  "project_id": "yyy",
  "data": {
    "stage": "implement",
    "agent": "codex",
    "content": "...",
    "timestamp": "2026-02-28T10:30:00Z"
  }
}
```

客户端消息（Client → Server）：

```json
{
  "type": "subscribe" | "unsubscribe",
  "pipeline_id": "xxx"
}
```

### 订阅规则

- 连接后默认不推送任何 Pipeline 的详细日志
- 客户端发 `subscribe` 后开始接收指定 Pipeline 的实时 agent_output
- `stage_start`、`stage_complete`、`human_required` 等状态事件始终推送所有活跃 Pipeline
- 客户端断开后自动清理订阅

### 连接管理

- **心跳**：服务端每 30 秒发送 WebSocket ping frame，客户端必须回复 pong，连续 2 次无响应则断开连接
- **subscribe 响应**：subscribe 消息发送后服务端回复确认消息 `{"type": "subscribed", "pipeline_id": "xxx"}`
- **重连恢复**：客户端重连后发送 subscribe，服务端推送该 Pipeline 的当前状态快照（当前 stage、status、最近 N 条 agent_output）

### 广播策略

```
EventBus 事件
  ├── 状态事件（stage_start 等） → 广播给所有 WS 连接
  └── 输出事件（agent_output）   → 只发给订阅了该 Pipeline 的连接
```

## 三、配置体系

### 三级配置

```
全局配置 (~/.ai-workflow/config.yaml)
  └── 项目配置 ({repo}/.ai-workflow/config.yaml)
      └── Pipeline 配置 (创建时传入)
```

合并规则：下级覆盖上级，未设置的字段继承上级值。

### 全局配置结构

```yaml
# ~/.ai-workflow/config.yaml

# Agent 配置
agents:
  claude:
    binary: claude                  # CLI 路径
    default_max_turns: 30
    default_tools:
      - "Read(*)"
      - "Write(*)"
      - "Edit(*)"
      - "Bash(git *)"

  codex:
    binary: codex
    model: gpt-5.3-codex            # 通过 -m 标志传递
    reasoning: high                  # 通过 -c model_reasoning_effort= 传递
    sandbox: workspace-write         # 通过 --sandbox 传递
    approval: never                  # 通过 -a 传递

  openspec:
    binary: openspec
    profile: core

# 默认 Pipeline 行为
pipeline:
  default_template: standard
  global_timeout: 2h
  auto_infer_template: true        # 是否启用 AI 推断模板

# 并发控制
scheduler:
  max_global_agents: 3             # 全局最多同时运行几个 Agent
  max_project_pipelines: 2         # 每个项目最多几条活跃 Pipeline

# GitHub 全局配置
github:
  enabled: true
  token: ""                        # 或使用环境变量 AI_WORKFLOW_GITHUB_TOKEN
  app_id: 0
  private_key_path: ""
  installation_id: 0               # GitHub App 安装 ID
  webhook_secret: ""
  # Webhook 复用 server.port，路由 POST /webhook

# Web Server（同时承载 API + WebSocket + GitHub Webhook）
server:
  host: "127.0.0.1"               # 监听地址，默认只监听本地
  port: 8080
  auth_enabled: false
  auth_token: ""

# 存储
store:
  driver: sqlite
  path: ~/.ai-workflow/data.db

# 日志
log:
  level: info                      # debug / info / warn / error
  file: ~/.ai-workflow/logs/app.log
  max_size_mb: 100
  max_age_days: 30
```

### 项目配置结构

```yaml
# {repo}/.ai-workflow/config.yaml

# 项目信息（注册时自动生成）
project:
  id: app-a
  name: "前端项目"

# 覆盖 Agent 配置
agents:
  codex:
    model: gpt-5.3-codex           # 这个项目用特定模型
    reasoning: xhigh               # 复杂项目用高推理

# 覆盖 Pipeline 行为
pipeline:
  default_template: full           # 这个项目默认用 full 流程
  spec_review_human: true          # Spec 审核需要人工
  code_review_human: true          # Code Review 需要人工
  merge_human: true                # 合并需要人工

# 自定义模板
custom_templates:
  ui-change:
    stages: [requirements, worktree_setup, implement, code_review, merge]
    defaults:
      implement_agent: codex
      code_review_human: false

# GitHub 项目级配置
github:
  owner: your-username
  repo: app-a
  webhook_secret: ""               # 项目级 webhook secret，覆盖全局配置（每个仓库建议使用不同的 secret）
  label_mapping:
    "type: feature": full
    "type: bug": quick
    "type: hotfix": hotfix
  pr:
    draft: true
    reviewers: ["teammate-a"]
    labels: ["ai-generated"]

# Prompt 覆盖（可选）
prompts:
  implement: |
    你正在一个 React + TypeScript 项目中工作。
    请使用 functional component 和 hooks。
    确保添加完整的 TypeScript 类型定义。
    {默认模板继续...}
```

### Pipeline 级配置

创建 Pipeline 时可以传入临时覆盖：

```json
{
  "name": "add-oauth",
  "template": "full",
  "config": {
    "implement_agent": "claude",
    "code_review_human": false,
    "timeout": "1h"
  }
}
```

### 配置合并实现规则

- 所有可覆盖字段使用指针类型（`*int`, `*string`, `*bool`, `*Duration`），`nil` 表示"未设置"，非 nil 值覆盖上级
- `null` 在 YAML/JSON 中显式设置为 nil，用于清空继承值
- 空数组 `[]` 显式清空上级数组（区别于字段缺失时的继承）
- Duration 类型使用字符串表示（如 `"30m"`），自定义 `UnmarshalYAML` 解析
- 配置变更不影响已运行的 Pipeline，只对新创建的生效
- 不使用反射深度合并（Go 反模式），改为手写合并函数，字段显式列举

### 环境变量

所有配置项都可通过环境变量覆盖，命名规则：

```
AI_WORKFLOW_{SECTION}_{KEY}

例如：
AI_WORKFLOW_AGENTS_CLAUDE_BINARY=claude
AI_WORKFLOW_SERVER_PORT=9090
AI_WORKFLOW_GITHUB_TOKEN=ghp_xxx
```

环境变量优先级最高，覆盖所有配置文件。

**环境变量类型限制**：
- 字符串和数值类型直接映射
- 数组类型用逗号分隔（如 `AI_WORKFLOW_AGENTS_CLAUDE_DEFAULT_TOOLS="Read(*),Write(*),Edit(*)"`)
- map 类型不支持环境变量覆盖，需通过配置文件设置

## 四、数据库 Schema

### 使用 SQLite，表设计如下：

```sql
-- 建库初始化
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
    github_owner TEXT,
    github_repo  TEXT,
    config_json TEXT,               -- 运行时状态缓存（非 source of truth）
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

**配置 source of truth 规则：**
- 项目配置的 source of truth 是 `{repo}/.ai-workflow/config.yaml` 文件
- 数据库 `projects.config_json` 仅作为运行时缓存，用于快速查询
- 启动时或项目注册时从 YAML 文件加载配置写入数据库
- 如果 YAML 文件和数据库不一致，以 YAML 文件为准
- 纯运行时数据（如 pipeline_count、last_active）只存数据库，不写回 YAML
- 检测文件变更：启动时 + 每次使用配置前，计算 YAML 文件 SHA256 与数据库中缓存的 hash 对比，有变化则重新加载（不依赖 mtime，跨平台可靠）

#### pipelines

```sql
CREATE TABLE pipelines (
    id          TEXT PRIMARY KEY,
    project_id  TEXT NOT NULL REFERENCES projects(id),
    name        TEXT NOT NULL,
    description TEXT,
    template    TEXT NOT NULL,
    status      TEXT NOT NULL DEFAULT 'created',
    -- created / running / waiting_human / paused / done / failed / aborted
    current_stage TEXT,
    stages_json TEXT NOT NULL,      -- 阶段配置序列化
    artifacts_json TEXT DEFAULT '{}',
    config_json TEXT DEFAULT '{}',  -- Pipeline 级覆盖配置
    issue_number INTEGER,           -- 关联的 GitHub Issue
    pr_number   INTEGER,            -- 关联的 GitHub PR
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
CREATE INDEX idx_pipelines_issue ON pipelines(issue_number);
```

#### checkpoints

```sql
CREATE TABLE checkpoints (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    pipeline_id TEXT NOT NULL REFERENCES pipelines(id),
    stage       TEXT NOT NULL,
    status      TEXT NOT NULL,      -- in_progress / success / failed / skipped / invalidated
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
    type        TEXT NOT NULL,      -- agent_output / tool_call / error / human_action
    agent       TEXT,
    content     TEXT NOT NULL,
    timestamp   DATETIME NOT NULL
);

CREATE INDEX idx_logs_pipeline_stage ON logs(pipeline_id, stage);
CREATE INDEX idx_logs_id ON logs(id);
```

**日志批量写入策略**：高频 Agent 输出不逐条 INSERT，而是批量写入（每 100 条或每 1 秒 flush 一次，取先到者），减少 SQLite 写入压力。使用内存 buffer + 定时 flush goroutine 实现。

#### human_actions

```sql
CREATE TABLE human_actions (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    pipeline_id TEXT NOT NULL REFERENCES pipelines(id),
    stage       TEXT NOT NULL,
    action      TEXT NOT NULL,      -- approve / reject / modify / skip / ...
    message     TEXT,
    source      TEXT NOT NULL,      -- tui / web / github / reaction
    user_id     TEXT,               -- GitHub username 或 "local"
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_human_actions_pipeline ON human_actions(pipeline_id);
```

### 数据库维护规则

- 迁移文件放在 `internal/store/migrations/` 目录，按序号命名
- 使用 `golang-migrate` 或手写迁移（项目简单，手写即可）
- 已完成超过 30 天的 Pipeline 的 logs 可以归档或清理（配置项）
- 数据库文件默认位置 `~/.ai-workflow/data.db`，可配置

### updated_at 处理

SQLite 的 `DEFAULT CURRENT_TIMESTAMP` 只在 INSERT 时生效，UPDATE 时不会自动更新。

**方案：应用层处理**（不使用 trigger，避免 AFTER UPDATE 递归触发问题）：
- Store 实现的 `SaveProject()` / `SavePipeline()` 中显式设置 `updated_at = CURRENT_TIMESTAMP`
- 所有 UPDATE 语句必须包含 `updated_at = CURRENT_TIMESTAMP`

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
    GetActivePipelines() ([]Pipeline, error)  // 崩溃恢复用

    // Checkpoints
    SaveCheckpoint(cp *Checkpoint) error
    GetCheckpoints(pipelineID string) ([]Checkpoint, error)
    GetLastSuccessCheckpoint(pipelineID string) (*Checkpoint, error)

    // Logs
    AppendLog(entry LogEntry) error
    GetLogs(pipelineID string, stage string, limit int, offset int) ([]LogEntry, int, error)  // 返回 (entries, total, error)

    // Human Actions
    RecordAction(action HumanAction) error
    GetActions(pipelineID string) ([]HumanAction, error)
}
```

## 五、ID 生成规则

| 实体 | 格式 | 示例 |
|---|---|---|
| Project | 用户指定，kebab-case | `app-a`、`backend-api` |
| Pipeline | `{日期}-{12位随机hex}` | `20260228-a3f1b2c0d4e6` |
| Checkpoint | 自增整数 | 1, 2, 3... |
| Log | 自增整数 | 1, 2, 3... |

Pipeline ID 用日期前缀便于人类识别和按时间排序。12 位 hex（6 bytes = 48 bits）提供约 281 万亿种组合，碰撞概率大幅降低。

## 六、安全

### 本地使用

- 默认不启用认证（`auth_enabled: false`）
- Web Server 只监听 localhost

### 团队/远程使用

- 启用 Bearer Token 认证
- 配置 TLS 或通过 reverse proxy 处理
- GitHub Token 和 Auth Token 支持环境变量注入，避免明文写配置文件
- SQLite 数据库文件权限设为 600

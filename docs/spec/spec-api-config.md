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
  → 删除项目时级联删除关联的 pipelines、checkpoints、logs、human_actions、chat_sessions、task_plans、task_items、review_records、audit_log（通过应用层事务实现，不依赖 SQLite CASCADE）
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
    task_item_id, current_stage, stages: [...], artifacts: {...},
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
    plans: { executing, reviewing },
    agents: { active, max }
  }

GET    /api/v1/admin/pipelines
  Query: ?project_id=&status=&limit=50&offset=0
  → 200: { items: [...], total }
  注：跨项目查看所有 Pipeline

GET    /api/v1/admin/plans
  Query: ?project_id=&status=&limit=50&offset=0
  → 200: { items: [...], total }
  注：跨项目查看所有 TaskPlan

GET    /api/v1/admin/audit-log
  Query: ?project_id=&action=&user=&since=&until=&limit=100&offset=0
  → 200: { items: [{ id, timestamp, project_id, action, target_type, target_id, user_id, detail }], total }
  注：全局审计日志，支持多维过滤
```

> ChatSession、TaskPlan、TaskItem、ReviewRecord 的领域模型、状态机和设计背景见 [spec-secretary-layer.md](spec-secretary-layer.md)。

### ChatSession — 持久 Agent Session（P2c 新增）

```
POST   /api/v1/projects/:pid/chat/sessions
  Body: { role: "secretary" }          // 可选，默认使用 role_bindings.secretary.role
  → 201: { session_id, role, agent, status: "active" }
  注：后端通过 ACP Client 启动 Agent session，工作目录 = 项目目录
  注：项目重开优先尝试 `LoadSession` 恢复（Agent 支持时），失败再 `NewSession`

POST   /api/v1/projects/:pid/chat/sessions/:sid/messages
  Body: { content: "需求描述..." }
  → 200: { message_id }
  注：消息通过 acpClient.Prompt() 发给 Agent，回复通过 WebSocket 流式推送
  注：每条消息和回复均记录到 audit_log

GET    /api/v1/projects/:pid/chat/sessions/:sid
  → 200: { id, project_id, agent, status, messages: [...], created_at }

GET    /api/v1/projects/:pid/chat/sessions
  → 200: [{ id, agent, status, message_count, created_at, updated_at }]

DELETE /api/v1/projects/:pid/chat/sessions/:sid
  → 204
  注：调用 acpClient.Close() 终止 session，对话历史保留在 Store 中
```

### 项目文件操作（P2c 新增）

```
GET    /api/v1/projects/:pid/files
  Query: ?path=.ai-workflow/plans/&changed_since=2026-03-01T00:00:00Z
  → 200: { files: [{ path, size, modified_at, is_new }] }
  注：列出项目目录下指定路径的文件，支持按修改时间过滤
  注：path 是相对于项目根目录的路径前缀

GET    /api/v1/projects/:pid/files/content
  Query: ?paths=file1.md,file2.md
  → 200: { files: [{ path, content, size }] }
  注：读取指定文件内容（前端预览用），单文件上限 100KB
```

### TaskPlan 管理（P2a 新增）

```
POST   /api/v1/projects/:pid/plans/from-files
  Body: { session_id: "chat-xxx", file_paths: ["plan-auth.md", "plan-db.md"] }
  → 201: { id, name, tasks: [...], source_files: [...], status: "draft" }
  注：后端调用 AI（Plan Parser）读取选中文件内容，解析为结构化 TaskPlan + TaskItems
  注：解析失败返回 400 + 错误详情，用户可在 Chat 中调整后重试

GET    /api/v1/projects/:pid/plans
  Query: ?status=executing&limit=20&offset=0
  → 200: { items: [...], total, offset }

GET    /api/v1/projects/:pid/plans/:id
  → 200: { id, name, tasks: [...], status, review_round, fail_policy, ... }

GET    /api/v1/projects/:pid/plans/:id/dag
  → 200: {
    nodes: [{ id, title, status, pipeline_id }],
    edges: [{ from, to }],
    stats: { total, pending, ready, running, done, failed }
  }

POST   /api/v1/projects/:pid/plans/:id/review
  → 200: { status: "reviewing" }

POST   /api/v1/projects/:pid/plans/:id/action
  Body: {
    action: "approve" | "reject" | "abort",
    feedback: {                  // reject 时必填（两段式反馈）
      category: "cycle" | "missing_node" | "bad_granularity" | "coverage_gap" | "other",
      detail: "至少 20 字的说明",
      expected_direction: "可选"
    }
  }
  → 200: { status }
  注：reject 触发 Secretary 自动重生成并重新进入 AI review

POST   /api/v1/projects/:pid/plans/:id/tasks/:tid/action
  Body: {
    action: "retry" | "skip" | "abort"
  }
  → 200: { status }
```

### API 设计规则

- Pipeline 的 action 端点是唯一的 Pipeline 级人工操作入口，TUI/Web/GitHub 最终都调用它
- TaskPlan 的 action 端点是 TaskPlan 级操作入口
- TaskItem 的 action 端点用于单个子任务的 retry/skip/abort
- `task_item_id` 是 Pipeline 的内部关联字段：仅由 DAG Scheduler 在自动创建 Pipeline 时设置，手动创建接口不接受
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
       | "agent_output" | "system_info"
       | "secretary_thinking" | "plan_created" | "plan_reviewing"
       | "review_agent_done" | "review_complete" | "plan_approved"
       | "plan_waiting_human"
       | "task_ready" | "task_running" | "task_done" | "task_failed"
       | "plan_done" | "plan_failed" | "plan_partially_done",
  "pipeline_id": "xxx",
  "project_id": "yyy",
  "plan_id": "zzz",
  "data": {
    "stage": "implement",
    "agent": "codex",
    "content": "...",
    "timestamp": "2026-02-28T10:30:00Z"
  }
}
```

Secretary Layer 新增事件说明：

| 事件 | 用途 |
|------|------|
| `secretary_thinking` | Secretary Agent 流式输出（对话回复） |
| `secretary_files_changed` | Secretary 写入/修改文件（含 file_paths, session_id） |
| `secretary_tool_call` | Secretary 调用查询工具（含 tool_name, input） |
| `secretary_tool_result` | 查询工具返回结果（含 tool_name, output） |
| `plan_created` | 新 TaskPlan 创建（从文件解析得到） |
| `plan_reviewing` | 进入审核 |
| `review_agent_done` | 单个 Reviewer 完成（含 verdict） |
| `review_complete` | 审核流程完成（含 decision: approve/fix/escalate） |
| `plan_approved` | TaskPlan 通过审核 |
| `plan_waiting_human` | 等待人工（含 wait_reason: final_approval / feedback_required） |
| `task_ready` | TaskItem 变为 ready（可调度） |
| `task_running` | TaskItem 开始执行（含 pipeline_id） |
| `task_done` | TaskItem 完成 |
| `task_failed` | TaskItem 失败（含 error） |
| `plan_done` | 所有 TaskItem 完成 |
| `plan_failed` | TaskPlan 失败 |
| `plan_partially_done` | 部分成功部分失败（含 stats） |

客户端消息（Client → Server）：

```json
{
  "type": "subscribe_pipeline" | "unsubscribe_pipeline"
       | "subscribe_plan" | "unsubscribe_plan",
  "pipeline_id": "xxx",
  "plan_id": "zzz"
}
```

### 订阅规则

- 连接后默认不推送任何 Pipeline 的详细日志
- 客户端发 `subscribe_pipeline` 后开始接收指定 Pipeline 的实时 agent_output
- 客户端发 `subscribe_plan` 后开始接收指定 TaskPlan 的所有事件（审核进度、任务状态变更等）
- `stage_start`、`stage_complete`、`human_required` 等状态事件始终推送所有活跃 Pipeline
- `plan_created`、`plan_done`、`plan_failed` 等 Plan 级状态事件始终推送
- 客户端断开后自动清理订阅

### 连接管理

- **心跳**：服务端每 30 秒发送 WebSocket ping frame，客户端必须回复 pong，连续 2 次无响应则断开连接
- **subscribe 响应**：subscribe 消息发送后服务端回复确认消息 `{"type": "subscribed", "pipeline_id": "xxx"}`
- **重连恢复**：客户端重连后发送 `subscribe_pipeline`/`subscribe_plan`，服务端推送对应的当前状态快照（当前 stage、status、最近 N 条 agent_output）

### 广播策略

```
EventBus 事件
  ├── 状态事件（stage_start / plan_done 等） → 广播给所有 WS 连接
  ├── 输出事件（agent_output）               → 只发给 subscribe_pipeline 了该 Pipeline 的连接
  └── 计划事件（review_progress / task_status_changed 等） → 只发给 subscribe_plan 了该 Plan 的连接
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

# Agent Profile（统一启动配置）
agents:
  - name: claude                              # Agent 标识符
    launch_command: "claude-agent-acp"        # npm install -g @zed-industries/claude-agent-acp
    launch_args: []                           # 启动参数
    env: {}                                   # 环境变量
    capabilities_max:                         # 能力上限（角色配置不能超出此上限）
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

# 配置严格校验：未定义字段一律报错并终止启动（fail-fast）

# Role Profile（统一角色行为配置）
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
      tools: [query_plans, query_plan_detail, query_pipelines, query_pipeline_logs, query_project_stats]
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

  - name: aggregator
    agent: claude
    prompt_template: review_aggregator
    capabilities:
      fs_read: true
      fs_write: false
      terminal: false
    session:
      reuse: true
      max_turns: 6
      reset_prompt: true

  - name: plan_parser
    agent: claude
    prompt_template: plan_parser
    capabilities:
      fs_read: true
      fs_write: false
      terminal: false
    session:
      reuse: false

# 角色绑定（调用方只关心 role，不直接关心 agent）
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
  review_orchestrator:
    reviewers:
      completeness: reviewer
      dependency: reviewer
      feasibility: reviewer
    aggregator: aggregator
  plan_parser:
    role: plan_parser

# Spec 插件配置（仅 Secretary Layer 使用，Pipeline 不读取）
spec:
  provider: openspec                 # openspec | mcp | none | custom
  enabled: true
  on_failure: warn                   # warn | fail
  openspec:
    binary: openspec
    profile: core
  # mcp:                             # provider=mcp 时启用（P4 预留）
  #   endpoint: http://127.0.0.1:8081/spec
  #   api_key: ${MCP_API_KEY}
  #   timeout: 15s
  #   context_limit: 20

# 默认 Pipeline 行为
pipeline:
  default_template: standard
  global_timeout: 2h
  auto_infer_template: true        # 是否启用 AI 推断模板
  acp:
    session_strategy: hybrid       # per_stage | hybrid | per_pipeline
    prefer_load_session: true      # 崩溃恢复优先 LoadSession，失败再 NewSession
    session_idle_ttl: 30m          # 保活 session 空闲回收时间
    pause_keep_session: true       # pause 后是否保留 session（便于 resume）
    # 具体角色能力/权限/prompt 由 roles + role_bindings 决定
    recovery:
      fallback_rehydrate: true     # LoadSession/NewSession 失败时上下文补水
      rehydrate_sources: [artifacts, progress_files, review_issues]

# 并发控制
scheduler:
  max_global_agents: 3             # 全局最多同时运行几个 Agent
  max_project_pipelines: 2         # 每个项目最多几条活跃 Pipeline

# Secretary Layer 配置（P2a 新增）
secretary:
  role: secretary                  # 引用 role_bindings.secretary.role
  session_idle_timeout: 30m        # 空闲超时自动关闭 session
  # 具体 ACP 权限、MCP 工具、会话策略由 roles.secretary 定义
  context_max_tokens: 4000         # 项目上下文 token 预算（Plan Parser 用）
  refine_enabled: false            # TaskPlan 细化（补充实施级细节），V1 默认关闭
  refine_timeout: 2m               # 单个 TaskItem 细化超时
  execution_files_enabled: true    # 执行期三文件沉淀（task_plan.md/progress.md/findings.md）

  # Multi-Agent 审核配置（P2b 新增）
  review_orchestrator:
    enabled: true
    max_rounds: 2                  # 审核-修正最大循环次数（超限进入 waiting_human）
    min_score: 70                  # 通过最低分
    roles:                         # 引用 role_bindings.review_orchestrator，可在项目级覆写
      completeness: reviewer
      dependency: reviewer
      feasibility: reviewer
      aggregator: aggregator
    reviewers:
      - name: completeness
        prompt_template: review_completeness
      - name: dependency
        prompt_template: review_dependency
      - name: feasibility
        prompt_template: review_feasibility
    aggregator:
      prompt_template: review_aggregator
    timeout_per_reviewer: 5m       # 每个 Reviewer 的超时

  # DAG 调度配置（P2a 新增）
  dag_scheduler:
    fail_policy: block             # 默认失败策略: block / skip / human
    max_concurrent_tasks: 0        # 0 = 不额外限制，使用全局 max_global_agents
    dispatch_interval: 1s          # 调度检查间隔
    stale_check_interval: 5m       # 停滞检测间隔
    stale_threshold: 30m           # 超过此时间无进展视为停滞

# GitHub 全局配置（P3，可选）
github:
  enabled: false                   # 默认关闭，核心功能不依赖 GitHub
  token: ""                        # 或使用环境变量 AI_WORKFLOW_GITHUB_TOKEN
  app_id: 0
  private_key_path: ""
  installation_id: 0               # GitHub App 安装 ID
  webhook_secret: ""
  # Webhook 复用 server.port，路由 POST /webhook
  rate_limit:                      # GitHub 写操作限流（P3 新增）
    requests_per_second: 1         # 每秒最大写请求数（5000/h ≈ 1.39/s，留余量）
    burst: 5                       # 突发容量
    retry_on_limit: true           # 收到 429/403 自动退避重试
    max_retries: 3                 # 限流重试上限

# Token 预算控制（P4 预留，当前不生效）
# budget:
#   monthly_token_limit: 0         # 0 = 不限制；正数 = 月度 Token 上限
#   warn_threshold: 0.8            # 消耗达 80% 时告警
#   action_on_exceed: warn         # warn = 仅告警 / pause = 暂停自动任务

# 数据库备份（P4 预留，当前不生效）
# 归属 store 段，由 Store 插件负责执行
# store:
#   backup:
#     enabled: false
#     interval: 24h                # 备份间隔
#     path: ~/.ai-workflow/backups/  # 备份目录
#     max_copies: 7                # 保留最近 N 份

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

### ACP 配置校验规则

- 配置解析采用严格 schema：未知字段、类型不匹配、非法枚举值直接报错并终止启动（fail-fast）
- 角色语义统一走 `roles + role_bindings`，不在各业务段重复维护独立 ACP 权限块
- Agent 启动参数统一走 `agents[].launch_command/launch_args/env/capabilities_max`

### 项目配置结构

```yaml
# {repo}/.ai-workflow/config.yaml

# 项目信息（注册时自动生成）
project:
  id: app-a
  name: "前端项目"

# 覆盖 Agent 配置（项目级，合并到全局 agents 列表中同名条目）
agents:
  - name: codex
    env:                            # 项目级环境变量覆盖
      CODEX_MODEL: "gpt-5.3-codex"

# 覆盖角色配置（项目级）
roles:
  - name: worker
    agent: codex                    # 此项目让 worker 固定用 codex
    session:
      max_turns: 16
  - name: reviewer
    session:
      reuse: true
      reset_prompt: true

# 覆盖 Pipeline 行为
pipeline:
  default_template: full           # 这个项目默认用 full 流程
  code_review_human: true          # Code Review 需要人工
  merge_human: true                # 合并需要人工

# 覆盖角色绑定（项目级）
role_bindings:
  pipeline:
    stage_roles:
      implement: worker
      code_review: reviewer
  secretary:
    role: secretary

# 覆盖 Spec 插件配置（仅影响 Secretary 上下文增强）
spec:
  provider: openspec                 # openspec | mcp | none | custom
  enabled: true
  openspec:
    profile: app-a
  # mcp:
  #   endpoint: http://127.0.0.1:8081/spec
  #   timeout: 10s
  #   context_limit: 10

# 自定义模板
custom_templates:
  ui-change:
    stages: [requirements, worktree_setup, implement, code_review, merge]
    defaults:
      implement_role: worker
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
    "implement_role": "worker",
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
AI_WORKFLOW_AGENTS_CLAUDE_LAUNCH_COMMAND=claude-agent-acp
AI_WORKFLOW_SERVER_PORT=9090
AI_WORKFLOW_GITHUB_TOKEN=ghp_xxx
```

环境变量优先级最高，覆盖所有配置文件。

**环境变量类型限制**：
- 字符串和数值类型直接映射
- 数组类型用逗号分隔（如 `AI_WORKFLOW_AGENTS_CLAUDE_CAPABILITIES_FS_READ=true`)
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
    source      TEXT NOT NULL DEFAULT 'local',  -- 'local' | 'git'
    git_url     TEXT,                            -- source=git 时的 clone URL
    git_branch  TEXT,                            -- source=git 时的初始分支
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
    task_item_id TEXT,              -- 逻辑外键: task_items.id（DAG 创建时设置；手动创建为 NULL）
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
CREATE INDEX idx_pipelines_task_item ON pipelines(task_item_id)
  WHERE task_item_id IS NOT NULL;
CREATE UNIQUE INDEX idx_pipelines_project_issue ON pipelines(project_id, issue_number)
  WHERE issue_number IS NOT NULL;  -- 部分唯一索引：同一项目下 Issue 不重复关联（幂等），无 Issue 的 Pipeline 不受约束
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

> 以下 4 张表对应 Secretary Layer 引入的领域模型，Go struct 定义见 [spec-secretary-layer.md](spec-secretary-layer.md) Section I/II。

#### chat_sessions（P2c 新增）

```sql
CREATE TABLE chat_sessions (
    id          TEXT PRIMARY KEY,
    project_id  TEXT NOT NULL REFERENCES projects(id),
    messages    TEXT NOT NULL DEFAULT '[]',   -- JSON array of ChatMessage
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_chat_sessions_project ON chat_sessions(project_id);
```

#### task_plans（P2a 新增）

```sql
CREATE TABLE task_plans (
    id          TEXT PRIMARY KEY,
    project_id  TEXT NOT NULL REFERENCES projects(id),
    session_id  TEXT REFERENCES chat_sessions(id),
    name        TEXT NOT NULL,
    status      TEXT NOT NULL DEFAULT 'draft',
    -- draft / reviewing / waiting_human / approved / executing / partially_done / done / failed / abandoned
    wait_reason TEXT NOT NULL DEFAULT '',         -- '' / final_approval / feedback_required
    fail_policy TEXT NOT NULL DEFAULT 'block',   -- block / skip / human
    review_round INTEGER DEFAULT 0,
    source_files TEXT DEFAULT '[]',              -- JSON array: 计划来源文件路径列表
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_task_plans_project ON task_plans(project_id);
CREATE INDEX idx_task_plans_status ON task_plans(status);
```

#### task_items（P2a 新增）

```sql
CREATE TABLE task_items (
    id          TEXT PRIMARY KEY,
    plan_id     TEXT NOT NULL REFERENCES task_plans(id),
    title       TEXT NOT NULL,
    description TEXT NOT NULL,
    inputs      TEXT DEFAULT '[]',            -- JSON array: 前置输入（文件/接口/数据）
    outputs     TEXT DEFAULT '[]',            -- JSON array: 交付产物（文件/接口）
    acceptance  TEXT DEFAULT '[]',            -- JSON array: 可验证的验收标准
    labels      TEXT DEFAULT '[]',            -- JSON array
    depends_on  TEXT DEFAULT '[]',            -- JSON array of task_item IDs
    template    TEXT NOT NULL DEFAULT 'standard',
    pipeline_id TEXT REFERENCES pipelines(id),
    external_id TEXT,                         -- GitHub Issue # 等外部系统 ID（可选）
    status      TEXT NOT NULL DEFAULT 'pending',
    -- pending / ready / running / done / failed / skipped / blocked_by_failure
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_task_items_plan ON task_items(plan_id);
CREATE INDEX idx_task_items_status ON task_items(status);
```

#### review_records（P2b 新增）

```sql
CREATE TABLE review_records (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    plan_id     TEXT NOT NULL REFERENCES task_plans(id),
    round       INTEGER NOT NULL,
    reviewer    TEXT NOT NULL,                -- "completeness" / "dependency" / "feasibility" / "aggregator"
    verdict     TEXT NOT NULL,                -- "pass" / "issues_found" / "approve" / "fix" / "escalate"
    issues      TEXT DEFAULT '[]',            -- JSON array of ReviewIssue
    fixes       TEXT DEFAULT '[]',            -- JSON array of ProposedFix
    score       INTEGER,                      -- 0-100 评分（Reviewer 用）
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_review_records_plan ON review_records(plan_id);
```

#### audit_log（P2c 新增）

```sql
CREATE TABLE audit_log (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    project_id  TEXT,                    -- 可为空（系统级操作）
    action      TEXT NOT NULL,           -- 操作类型（见下方枚举）
    target_type TEXT,                    -- project / pipeline / plan / task / chat
    target_id   TEXT,
    user_id     TEXT DEFAULT 'system',   -- 操作者：'system' / 'user' / GitHub username
    detail      TEXT DEFAULT '{}',       -- JSON: 操作详情
    timestamp   DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_audit_log_project ON audit_log(project_id);
CREATE INDEX idx_audit_log_action ON audit_log(action);
CREATE INDEX idx_audit_log_timestamp ON audit_log(timestamp);
```

操作类型枚举：
- `project_created`, `project_deleted`
- `chat_session_started`, `chat_session_ended`, `chat_message_sent`
- `plan_files_generated` — Secretary 生成计划文件
- `plan_created_from_files` — 用户提交文件创建 Plan
- `plan_review_started`, `plan_review_completed`, `plan_approved`, `plan_rejected`
- `task_dispatched`, `task_completed`, `task_failed`, `task_retried`, `task_skipped`
- `pipeline_created`, `pipeline_started`, `pipeline_completed`, `pipeline_failed`
- `human_action` — 人工操作（approve/reject/modify/skip/abort）
- `agent_session_started`, `agent_session_ended`
- `secretary_tool_call` — Secretary 查询工具调用

> **与 logs 表的区别**：`audit_log` 记录操作级事件（谁在什么时候做了什么），`logs` 记录 Agent 的详细输出流。审计日志量远小于 Agent 输出日志。

### 数据库维护规则

- 迁移由各 Store 插件自行管理（如 `store-sqlite/migrations.go` 中嵌入 schema），不使用独立迁移目录
- 项目简单，直接在 `Open()` 时执行建表（IF NOT EXISTS），无需版本化迁移工具
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

    // ChatSessions（P2c 新增）
    CreateChatSession(s *ChatSession) error
    GetChatSession(id string) (*ChatSession, error)
    UpdateChatSession(s *ChatSession) error
    ListChatSessions(projectID string) ([]ChatSession, error)

    // TaskPlans（P2a 新增）
    CreateTaskPlan(p *TaskPlan) error
    GetTaskPlan(id string) (*TaskPlan, error)
    SaveTaskPlan(p *TaskPlan) error
    ListTaskPlans(projectID string, filter TaskPlanFilter) ([]TaskPlan, error)
    GetActiveTaskPlans() ([]TaskPlan, error)  // 崩溃恢复用

    // TaskItems（P2a 新增）
    CreateTaskItem(item *TaskItem) error
    GetTaskItem(id string) (*TaskItem, error)
    SaveTaskItem(item *TaskItem) error
    GetTaskItemsByPlan(planID string) ([]TaskItem, error)
    GetTaskItemByPipeline(pipelineID string) (*TaskItem, error)  // 反查

    // ReviewRecords（P2b 新增）
    SaveReviewRecord(r *ReviewRecord) error
    GetReviewRecords(planID string) ([]ReviewRecord, error)

    // AuditLog（P2c 新增）
    AppendAuditLog(entry AuditEntry) error
    GetAuditLogs(filter AuditFilter) ([]AuditEntry, int, error)  // 返回 (entries, total, error)
}
```

## 五、ID 生成规则

| 实体 | 格式 | 示例 |
|---|---|---|
| Project | 用户指定，kebab-case | `app-a`、`backend-api` |
| Pipeline | `{日期}-{12位随机hex}` | `20260228-a3f1b2c0d4e6` |
| Checkpoint | 自增整数 | 1, 2, 3... |
| Log | 自增整数 | 1, 2, 3... |
| **ChatSession** | `chat-{日期}-{8位hex}` | `chat-20260301-d4e5f6a7` |
| **TaskPlan** | `plan-{日期}-{8位hex}` | `plan-20260301-a3f1b2c0` |
| **TaskItem** | `task-{plan短ID}-{序号}` | `task-a3f1b2c0-1` |
| **ReviewRecord** | 自增整数 | 1, 2, 3... |

Pipeline ID 用日期前缀便于人类识别和按时间排序。12 位 hex（6 bytes = 48 bits）提供约 281 万亿种组合，碰撞概率大幅降低。

TaskItem ID 使用 plan 短 ID + 序号，便于从 ID 直接看出所属计划。

## 六、安全

### 本地使用

- 默认不启用认证（`auth_enabled: false`）
- Web Server 只监听 localhost

### 团队/远程使用

- 启用 Bearer Token 认证
- 配置 TLS 或通过 reverse proxy 处理
- GitHub Token 和 Auth Token 支持环境变量注入，避免明文写配置文件
- SQLite 数据库文件权限设为 600

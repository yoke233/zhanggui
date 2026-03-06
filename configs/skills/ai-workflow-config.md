# AI Workflow 配置技能

你正在操作 ai-workflow 编排器的配置。配置格式为 **TOML**，文件位于 `.ai-workflow/config.toml`。

## 配置文件位置

- **项目配置**: `.ai-workflow/config.toml`（每个项目独立）
- **机密文件**: `.ai-workflow/secrets.yaml`（token 等敏感数据，自动生成）
- **JSON Schema**: 可通过 `go run ./cmd/gen-schema` 生成最新 schema

## 核心概念

### Agents（agents.profiles）
ACP agent 启动配置，定义如何启动外部 agent 进程。

```toml
[[agents.profiles]]
name            = "claude"
launch_command  = "npx"
launch_args     = ["-y", "@zed-industries/claude-agent-acp"]
[agents.profiles.capabilities_max]
fs_read  = true
fs_write = true
terminal = true
```

### Roles（roles）
角色绑定 agent + 能力 + 提示词模板。角色的 capabilities 不能超过 agent 的 capabilities_max。

```toml
[[roles]]
name             = "worker"
agent            = "codex"          # 引用 agents.profiles 中的 name
prompt_template  = "implement"      # 对应 prompt_templates/implement.tmpl
[roles.capabilities]
fs_read  = true
fs_write = true
terminal = true
[roles.session]
reuse     = true
max_turns = 12
[roles.mcp]
enabled = true
tools   = ["query_projects", "query_issues"]
```

### Role Bindings（role_bindings）
将系统功能映射到角色名。

```toml
[role_bindings.team_leader]
role = "team_leader"

[role_bindings.run.stage_roles]
requirements = "worker"
implement    = "worker"
review       = "reviewer"
fixup        = "worker"
test         = "worker"

[role_bindings.review_orchestrator]
aggregator = "aggregator"
[role_bindings.review_orchestrator.reviewers]
completeness = "reviewer"
dependency   = "reviewer"
feasibility  = "reviewer"

[role_bindings.plan_parser]
role = "plan_parser"
```

### Run（run）
Run 执行默认值。

```toml
[run]
default_template    = "standard"    # standard / fast_release
global_timeout      = "2h"          # 支持 Go duration 格式
auto_infer_template = true
max_total_retries   = 5
```

### Scheduler（scheduler）
并发调度。

```toml
[scheduler]
max_global_agents = 3     # 全局最多同时运行的 agent
max_project_runs  = 2     # 每个项目最多并发 run
```

### Team Leader（team_leader）

```toml
[team_leader]
review_gate_plugin = "review-ai-panel"   # review-ai-panel / review-local / review-github-pr

[team_leader.review_orchestrator]
max_rounds = 2                           # 审核-修正最大循环次数

[team_leader.dag_scheduler]
max_concurrent_tasks = 2
```

### Server（server）

```toml
[server]
host = "127.0.0.1"
port = 8080
```

### Store（store）

```toml
[store]
driver = "sqlite"
path   = ".ai-workflow/data.db"
```

### GitHub（github）
完整的 GitHub 集成配置（默认关闭）。

```toml
[github]
enabled          = true
owner            = "your-org"
repo             = "your-repo"
webhook_enabled  = true
pr_enabled       = true
auto_trigger     = true

[github.pr]
auto_create   = true
draft         = true
branch_prefix = "ai/"
```

### A2A（a2a）

```toml
[a2a]
enabled = true
token   = ""        # 自动生成到 secrets.yaml
version = "0.3"
```

### Log（log）

```toml
[log]
level        = "info"       # debug / info / warn / error
file         = ".ai-workflow/logs/app.log"
max_size_mb  = 100
max_age_days = 30
```

## 验证规则

1. 角色的 `capabilities` 不能超过所引用 agent 的 `capabilities_max`
2. `a2a.enabled = true` 时 `a2a.token` 不能为空（secrets.yaml 自动生成）
3. `role_bindings` 引用的角色名必须在 `roles` 中定义
4. agent 名称必须唯一，role 名称必须唯一
5. 未知字段会报错并终止启动（strict parsing）

## 常见操作

- **初始化**: `ai-flow config init`（生成默认 config.toml）
- **重置**: `ai-flow config init --force`
- **环境变量覆盖**: `AI_WORKFLOW_SERVER_PORT=9090` 等

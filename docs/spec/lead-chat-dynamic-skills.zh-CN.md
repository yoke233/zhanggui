# Lead Chat 动态 Skills 设计方案

> 状态：草案
>
> 最后按代码核对：2026-03-13
>
> 重要说明：当前仓库已经有通用 skill 系统，但并未落地本文这套 `sys-*` Lead 动态技能体系。

## 当前已实现范围

当前仓库中已存在的能力：

- 通用 skills 目录、`SKILL.md` 解析与校验
- skills CRUD / GitHub import HTTP API
- sandbox 内 skill linking
- builtin skills，例如 `step-signal` 与 `step-context`

当前尚未落地的能力：

- 默认 Lead profile 预装 `sys-*` 系统 skills
- `AI_WORKFLOW_API_BASE` / `AI_WORKFLOW_PROJECT_ID` 这组专门为本文脚本体系准备的环境变量注入
- `load_mode: on_demand` 的完整动态加载协议
- 文中列出的 `sys-issue-manage` / `sys-step-manage` 等系统 skill 实体

## 1. 背景与目标

当前 Lead Agent 的聊天框仍主要依赖通用文件读写、终端能力和少量已注入 skills。本文描述的是一套未来可能扩展的“系统技能层”。

**核心原则：Skills + 脚本，动态加载，而非直接暴露 MCP tools。**

这意味着：
- 每个系统能力被封装为一个 **Skill 目录**（SKILL.md + 配套脚本）
- Lead Agent 根据上下文按需加载/卸载 skills
- 脚本通过调用内部 REST API 完成操作（非 MCP 直连）
- Agent 看到的是"技能描述 + 可调用脚本"，而非原始 HTTP 端点

## 2. 架构概览

```
用户聊天消息
    ↓
Lead Agent (claude-acp)
    ↓ 读取 SKILL.md 获得能力描述
    ↓ 调用 scripts/*.sh 执行操作
    ↓
scripts/*.sh → curl http://localhost:$PORT/api/...
    ↓
ai-workflow backend (REST API)
    ↓
SQLite / 系统状态
```

### Skill 目录结构扩展

```
skills/
├── sys-issue-manage/
│   ├── SKILL.md              # 技能描述 + 使用指南
│   └── scripts/
│       ├── create-issue.sh    # 创建 Issue
│       ├── list-issues.sh     # 列出 Issues
│       ├── update-issue.sh    # 更新 Issue
│       └── run-issue.sh       # 触发执行
├── sys-progress-monitor/
│   ├── SKILL.md
│   └── scripts/
│       ├── issue-status.sh    # 查看 Issue 执行状态
│       ├── step-status.sh     # 查看 Step 状态
│       └── recent-events.sh   # 近期事件流
├── sys-cron-scheduler/
│   ├── SKILL.md
│   └── scripts/
│       ├── setup-cron.sh
│       ├── list-cron.sh
│       └── disable-cron.sh
└── ...
```

脚本运行时环境变量（由 sandbox 注入）：
- `AI_WORKFLOW_API_BASE` — 后端 API 地址（如 `http://127.0.0.1:8080/api`）
- `AI_WORKFLOW_API_TOKEN` — 认证 token
- `AI_WORKFLOW_PROJECT_ID` — 当前会话关联的项目 ID（如有）

## 3. 推荐的系统 Skills 清单

按功能域分组，标注优先级（P0 = 核心能力，P1 = 重要，P2 = 增强）。

### 3.1 Issue 管理 — `sys-issue-manage` (P0)

| 脚本 | 功能 | 对应 API |
|------|------|---------|
| `create-issue.sh` | 创建 Issue（标题/描述/优先级/标签） | `POST /api/issues` |
| `list-issues.sh` | 列出/搜索 Issues（按状态/项目/优先级过滤） | `GET /api/issues` |
| `get-issue.sh` | 查看 Issue 详情 | `GET /api/issues/{id}` |
| `update-issue.sh` | 更新 Issue（状态/优先级/描述） | `PUT /api/issues/{id}` |
| `archive-issue.sh` | 归档 Issue | `POST /api/issues/{id}/archive` |
| `run-issue.sh` | 触发 Issue 执行 | `POST /api/issues/{id}/run` |
| `cancel-issue.sh` | 取消执行中的 Issue | `POST /api/issues/{id}/cancel` |
| `generate-title.sh` | AI 生成 Issue 标题 | `POST /api/issues/generate-title` |

**典型对话场景**：
> "帮我创建一个 Issue，实现用户登录功能，优先级 high"
> "当前项目有哪些还在 running 的 Issue？"
> "把 Issue #12 的优先级改成 urgent"

### 3.2 Step 管理与 AI 分解 — `sys-step-manage` (P0)

| 脚本 | 功能 | 对应 API |
|------|------|---------|
| `create-step.sh` | 为 Issue 创建 Step | `POST /api/issues/{id}/steps` |
| `list-steps.sh` | 查看 Issue 下的所有 Steps | `GET /api/issues/{id}/steps` |
| `get-step.sh` | 查看 Step 详情 | `GET /api/steps/{id}` |
| `update-step.sh` | 更新 Step | `PUT /api/steps/{id}` |
| `generate-steps.sh` | AI 自动分解 Issue 为 Steps | `POST /api/issues/{id}/generate-steps` |

**典型对话场景**：
> "帮我把 Issue #5 拆解成具体的执行步骤"
> "Issue #5 下面的步骤执行到哪了？"

### 3.3 进度监控 — `sys-progress-monitor` (P1)

| 脚本 | 功能 | 对应 API |
|------|------|---------|
| `issue-status.sh` | 查看 Issue 执行状态概览 | `GET /api/issues/{id}` + `GET /api/issues/{id}/steps` |
| `execution-detail.sh` | 查看具体执行详情 | `GET /api/executions/{id}` |
| `recent-events.sh` | 获取近期系统事件 | `GET /api/events` |
| `issue-events.sh` | 获取特定 Issue 的事件流 | `GET /api/issues/{id}/events` |
| `artifact-view.sh` | 查看执行产物 | `GET /api/artifacts/{id}` |
| `probe-execution.sh` | 对运行中的 Execution 发送健康探测 | `POST /api/executions/{id}/probe` |

**典型对话场景**：
> "Issue #3 现在进展如何？"
> "最近 10 分钟有什么执行失败了吗？"
> "看看 Step #7 的输出结果"

### 3.4 定时任务 — `sys-cron-scheduler` (P1)

| 脚本 | 功能 | 对应 API |
|------|------|---------|
| `setup-cron.sh` | 为 Issue 设置 Cron 定时执行 | `POST /api/issues/{id}/cron` |
| `list-cron.sh` | 列出所有定时任务 | `GET /api/cron/issues` |
| `get-cron.sh` | 查看特定 Issue 的 Cron 状态 | `GET /api/issues/{id}/cron` |
| `disable-cron.sh` | 禁用定时任务 | `DELETE /api/issues/{id}/cron` |

**典型对话场景**：
> "帮我设置 Issue #8 每天凌晨 2 点自动运行"
> "现在有哪些定时任务在跑？"
> "停掉 Issue #8 的定时执行"

### 3.5 项目管理 — `sys-project-manage` (P1)

| 脚本 | 功能 | 对应 API |
|------|------|---------|
| `list-projects.sh` | 列出所有项目 | `GET /api/projects` |
| `get-project.sh` | 查看项目详情 | `GET /api/projects/{id}` |
| `create-project.sh` | 创建项目 | `POST /api/projects` |
| `update-project.sh` | 更新项目信息 | `PUT /api/projects/{id}` |
| `list-resources.sh` | 列出项目资源绑定 | `GET /api/projects/{id}/resources` |

**典型对话场景**：
> "现在有哪些项目？"
> "创建一个新项目叫 mobile-app，类型 dev"

### 3.6 模板管理 — `sys-template-manage` (P1)

| 脚本 | 功能 | 对应 API |
|------|------|---------|
| `list-templates.sh` | 列出 DAG 模板 | `GET /api/templates` |
| `get-template.sh` | 查看模板详情 | `GET /api/templates/{id}` |
| `save-as-template.sh` | 将 Issue 保存为模板 | `POST /api/issues/{id}/save-as-template` |
| `create-from-template.sh` | 从模板创建 Issue | `POST /api/templates/{id}/create-issue` |

**典型对话场景**：
> "把 Issue #5 保存成模板，以后可以复用"
> "用 full-stack-feature 模板创建一个新 Issue"

### 3.7 分析报表 — `sys-analytics` (P1)

| 脚本 | 功能 | 对应 API |
|------|------|---------|
| `summary.sh` | 系统总览统计 | `GET /api/analytics/summary` |
| `bottlenecks.sh` | 瓶颈分析（最慢/失败率最高的 Step） | `GET /api/analytics/bottlenecks` |
| `recent-failures.sh` | 近期失败记录 | `GET /api/analytics/recent-failures` |
| `error-breakdown.sh` | 错误类型分布 | `GET /api/analytics/error-breakdown` |
| `usage-summary.sh` | Token 用量汇总 | `GET /api/analytics/usage` |
| `usage-by-project.sh` | 按项目查看用量 | `GET /api/analytics/usage/by-project` |
| `status-distribution.sh` | Issue 状态分布 | `GET /api/analytics/status-distribution` |

**典型对话场景**：
> "目前系统整体运行情况怎么样？"
> "哪些步骤执行最慢，是瓶颈？"
> "这个月 token 用了多少？哪个项目用得最多？"

### 3.8 协作讨论 — `sys-thread-manage` (P2)

| 脚本 | 功能 | 对应 API |
|------|------|---------|
| `create-thread.sh` | 创建讨论 Thread | `POST /api/threads` |
| `list-threads.sh` | 列出 Threads | `GET /api/threads` |
| `post-message.sh` | 发送消息到 Thread | `POST /api/threads/{id}/messages` |
| `list-messages.sh` | 查看 Thread 消息 | `GET /api/threads/{id}/messages` |
| `link-work-item.sh` | 关联 Thread 与 Issue | `POST /api/threads/{id}/links/work-items` |
| `invite-agent.sh` | 邀请 Agent 加入 Thread | `POST /api/threads/{id}/agents` |

**典型对话场景**：
> "创建一个讨论，关于 Issue #5 的架构方案"
> "把 worker agent 邀请进 Thread #2 一起讨论"

### 3.9 Feature 追踪 — `sys-feature-tracking` (P2)

| 脚本 | 功能 | 对应 API |
|------|------|---------|
| `feature-summary.sh` | 查看项目功能清单概览 | `GET /api/projects/{id}/manifest/summary` |
| `list-entries.sh` | 列出所有 Feature 条目 | `GET /api/projects/{id}/manifest/entries` |
| `update-entry-status.sh` | 更新 Feature 状态 | `PATCH /api/manifest/entries/{id}/status` |
| `create-entry.sh` | 新增 Feature 条目 | `POST /api/projects/{id}/manifest/entries` |

**典型对话场景**：
> "项目功能完成度怎么样了？"
> "把用户登录功能标记为 pass"

### 3.10 Agent 配置 — `sys-agent-config` (P2)

| 脚本 | 功能 | 对应 API |
|------|------|---------|
| `list-profiles.sh` | 列出 Agent Profiles | `GET /api/agents/profiles` |
| `get-profile.sh` | 查看 Profile 详情 | `GET /api/agents/profiles/{id}` |
| `list-drivers.sh` | 列出 Agent Drivers | `GET /api/agents/drivers` |
| `list-skills.sh` | 列出已安装的 Skills | `GET /api/admin/skills` |

**典型对话场景**：
> "目前有哪些 agent 可以用？各自什么能力？"
> "现在装了哪些 skills？"

## 4. 优先级总结

| 优先级 | Skill | 核心价值 |
|--------|-------|---------|
| **P0** | `sys-issue-manage` | 最核心的工作单元管理 |
| **P0** | `sys-step-manage` | Issue 分解与执行控制 |
| **P1** | `sys-progress-monitor` | 实时掌握执行进度 |
| **P1** | `sys-cron-scheduler` | 自动化定时执行 |
| **P1** | `sys-project-manage` | 项目上下文切换 |
| **P1** | `sys-template-manage` | 复用工作流模式 |
| **P1** | `sys-analytics` | 系统洞察与优化 |
| **P2** | `sys-thread-manage` | 多方协作讨论 |
| **P2** | `sys-feature-tracking` | 功能完成度追踪 |
| **P2** | `sys-agent-config` | Agent 能力自查 |

## 5. 动态加载机制设计

### 5.1 当前 Skills 加载流程

```
Profile.Skills = ["...existing skills..."]
    ↓
Sandbox.Prepare() → EnsureSkillsLinked()
    ↓
Symlink: <skills-root>/sys-issue-manage → <agent-home>/skills/sys-issue-manage
    ↓
ACP Agent 读取 skills/ 目录中的 SKILL.md
```

当前问题：Skills 主要在 Session 创建时静态绑定，缺少本文想要的 Lead 专属系统技能动态装载层。

### 5.2 动态加载方案

引入 **Skill Registry + 热加载** 机制：

```
                        ┌─────────────────────────┐
                        │   Lead Agent Session     │
                        │                          │
 用户: "帮我看进度"  ──→ │  1. 意图识别             │
                        │  2. 查询 SkillRegistry   │
                        │  3. 动态 link skill      │
                        │  4. 读取 SKILL.md        │
                        │  5. 执行 scripts/...     │
                        └─────────────────────────┘
```

#### 方案 A: Profile 预装 + 按需激活（推荐，尚未落地）

在 Lead Profile 中预配全部 sys-* skills，Agent 根据 SKILL.md 中的 `assign_when` 字段判断何时使用：

```toml
[[runtime.agents.profiles]]
id = "lead"
skills = [
  "sys-issue-manage",
  "sys-step-manage",
  "sys-progress-monitor",
  "sys-cron-scheduler",
  "sys-project-manage",
  "sys-template-manage",
  "sys-analytics",
  "sys-thread-manage",
  "sys-feature-tracking",
  "sys-agent-config",
]
```

**优点**: 实现简单，当前架构原则上可承载
**缺点**: 当前默认配置并未这样做，且上下文占用较大

#### 方案 B: 分层加载（Lazy Skill Loading，未实现）

扩展 Metadata 和加载机制：

```yaml
---
name: sys-issue-manage
description: "管理系统 Issues（工作单元）"
assign_when: "用户需要创建、查看、更新或执行 Issue 时"
version: 1
category: system           # 新增：skill 分类
load_mode: on_demand       # 新增：lazy | eager | on_demand
---
```

- `eager`: Session 启动时立即加载（默认，向后兼容）
- `on_demand`: 首次需要时加载，Agent 先看到 skill 索引（名称+描述+assign_when），需要时再读取完整 SKILL.md 和 scripts
- `lazy`: 不出现在索引中，由系统/其他 skill 调用时才加载

**实现要点**:
1. Session 启动时只传给 Agent 一个**技能索引**（名称 + 一行描述 + 触发条件）
2. Agent 判断需要某个 skill 时，通过 `load_skill <name>` 动作加载完整内容
3. 后端 `EnsureSkillsLinked()` 改为支持增量 link

**优点**: Context 占用小，可支持大量 skills
**缺点**: 需要扩展 Metadata + ACP 交互协议

#### 方案 C: Skill Group（组合方案，未实现）

定义 skill group，按场景批量加载：

```toml
[runtime.skill_groups]
core = ["sys-issue-manage", "sys-step-manage"]
ops = ["sys-progress-monitor", "sys-cron-scheduler", "sys-analytics"]
collab = ["sys-thread-manage", "sys-feature-tracking"]
admin = ["sys-agent-config"]
```

Lead Agent 启动时加载 `core` 组，按需追加其他组。

### 5.3 推荐路径

**Phase 1 (快速验证)**: 方案 A — 先实现 10 个 sys-* skills，全量预装到 lead profile
**Phase 2 (优化)**: 方案 B — 引入 `load_mode: on_demand`，按需加载减少 context 占用
**Phase 3 (扩展)**: 方案 C — Skill Group 分组，支持更多自定义场景

## 6. 脚本规范

### 6.1 脚本协议

所有 `scripts/*.sh` 遵循统一协议：

```bash
#!/usr/bin/env bash
# 输入: 通过命令行参数或 stdin (JSON)
# 输出: stdout 输出 JSON 结果
# 错误: stderr 输出错误信息, 非零退出码
# 环境变量: AI_WORKFLOW_API_BASE, AI_WORKFLOW_API_TOKEN, AI_WORKFLOW_PROJECT_ID

set -euo pipefail

API_BASE="${AI_WORKFLOW_API_BASE:-http://127.0.0.1:8080/api}"
TOKEN="${AI_WORKFLOW_API_TOKEN:-}"

# 通用 API 调用函数
api_call() {
  local method="$1" path="$2"
  shift 2
  curl -sf -X "$method" \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer ${TOKEN}" \
    "${API_BASE}${path}" "$@"
}

# 脚本逻辑...
```

### 6.2 SKILL.md 中的脚本声明

```markdown
---
name: sys-issue-manage
description: "管理系统中的 Issues（创建/查看/更新/执行/归档工作单元）"
assign_when: "用户需要创建、查看、搜索、更新、执行或归档 Issue 时"
version: 1
---

# Issue 管理

你可以通过以下脚本来管理系统中的 Issue（工作单元）：

## 可用操作

### 创建 Issue
`./scripts/create-issue.sh <json-payload>`
参数 (JSON stdin):
- title (必需): Issue 标题
- project_id (可选): 关联项目 ID
- body (可选): 详细描述
- priority (可选): low/medium/high/urgent，默认 medium
- labels (可选): 标签数组

### 列出 Issues
`./scripts/list-issues.sh [--project-id=N] [--status=STATUS] [--priority=PRIORITY]`

### 查看 Issue 详情
`./scripts/get-issue.sh <issue-id>`

### 更新 Issue
`./scripts/update-issue.sh <issue-id> <json-payload>`

### 触发执行
`./scripts/run-issue.sh <issue-id>`

### 取消执行
`./scripts/cancel-issue.sh <issue-id>`

## 注意事项
- 创建 Issue 前先确认用户的需求，确保标题和描述清晰
- 优先级默认 medium，除非用户明确指定
- 执行前确认用户意图，避免误触发
```

## 7. 需要的代码改动（未来工作）

### 7.1 Metadata 扩展（Phase 2）

```go
// internal/skills/skillset.go
type Metadata struct {
    Name        string `json:"name" yaml:"name"`
    Description string `json:"description" yaml:"description"`
    AssignWhen  string `json:"assign_when" yaml:"assign_when"`
    Version     int    `json:"version" yaml:"version"`
    Category    string `json:"category,omitempty" yaml:"category,omitempty"`       // 新增
    LoadMode    string `json:"load_mode,omitempty" yaml:"load_mode,omitempty"`     // 新增
}
```

### 7.2 Sandbox 环境变量注入

```go
// internal/adapters/sandbox/home_dir.go — 未来若采用本文方案，可在 Prepare() 中追加
env["AI_WORKFLOW_API_BASE"] = fmt.Sprintf("http://127.0.0.1:%d/api", serverPort)
env["AI_WORKFLOW_API_TOKEN"] = sessionToken
env["AI_WORKFLOW_PROJECT_ID"] = strconv.FormatInt(projectID, 10)
```

### 7.3 Lead Profile 配置更新

```toml
# defaults.toml
[[runtime.agents.profiles]]
id = "lead"
name = "Lead Agent"
driver = "claude-acp"
role = "lead"
capabilities = ["planning", "review", "fullstack"]
skills = ["sys-issue-manage", "sys-step-manage", "sys-progress-monitor", "sys-cron-scheduler", "sys-project-manage", "sys-template-manage", "sys-analytics"]
```

## 8. 建议与当前实现的衔接方式

- 不要把本文当成“现有 skill 系统说明”，它描述的是未来扩展层
- 如果要说明现有系统，请补一篇独立文档描述 skills CRUD、builtin skills、ephemeral skills
- 如果未来继续推进本文方案，建议先从 `sys-issue-manage`、`sys-step-manage` 两个最小 skill 开始

## 9. 待讨论项

1. **脚本语言选择** — 全用 Bash 还是部分用 Python/Node？Bash 更轻量但处理 JSON 不方便
2. **权限控制** — 是否需要按 skill 做权限细分？比如普通用户不能用 `sys-agent-config`
3. **Phase 1 范围** — 先实现哪几个 skills？建议 P0（issue + step）+ `sys-progress-monitor`
4. **脚本输出格式** — JSON 还是人类可读文本？建议 JSON，由 Agent 转为自然语言
5. **错误反馈** — 脚本失败时如何让 Agent 理解原因并提供有用的回复
6. **Skill 版本管理** — 系统 skills 随代码发布还是独立管理？
7. **方案 B 的 `load_skill` 动作** — 是否需要扩展 ACP 协议？还是通过现有 terminal action 调用

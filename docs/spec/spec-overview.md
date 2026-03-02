# AI Workflow Orchestrator — 系统总览

## 项目定位

一个用 Go 编写的**智能任务分解与并行执行平台**。用户通过对话描述开发需求，Secretary Agent 利用 LLM 自动拆解为结构化子任务，经 Multi-Agent 审核委员会自动审核纠错后，DAG Scheduler 管理依赖关系并行调度多个 Pipeline 执行。每个子任务由 Claude Code、Codex CLI 等 AI 编码工具独立完成。提供 Web Workbench 作为主操作界面，GitHub 集成作为可选增强。

## 要解决的问题

当前手工流程的痛点：

- 每次开发都要在多个 CLI 之间手动切换，重复执行相似的命令序列
- 小 bug 和大 feature 走同样的重流程，效率浪费
- 上下文（需求、spec、review 结果）散落在不同的终端会话中，无法追溯
- 多项目切换时要记住每个项目的路径和配置差异
- 复杂需求只能串行处理，无法将独立子任务并行分配给多个 AI Agent
- 任务拆解质量靠人脑把关，缺乏自动审核纠错机制
- GitHub Issue/PR 联动是可选需求而非必需，但缺乏灵活的集成方式

## 整体架构

```
┌──────────────────────────────────────────────────────────────┐
│                       接入层（Ingress）                        │
│  Web Workbench (主)  │  TUI (轻量备选)  │  GitHub Webhook(可选)│
└──────────┬───────────┴────────┬─────────┴────────┬───────────┘
           │                    │                   │
┌──────────▼────────────────────▼───────────────────▼───────────┐
│                    Secretary Layer                              │
│  ┌──────────────┐  ┌───────────────┐  ┌───────────────────┐   │
│  │ Secretary    │  │  TaskPlan     │  │  DAG              │   │
│  │ Agent        │  │  Manager      │  │  Scheduler        │   │
│  │ (LLM 拆解)  │  │ (CRUD+审核)   │  │  (依赖并行调度)    │   │
│  └──────┬───────┘  └───────┬───────┘  └────────┬──────────┘   │
│         │                  │                    │              │
│  ┌──────▼──────────────────▼────────────────────▼───────────┐  │
│  │              Multi-Agent Review Panel                      │  │
│  │   completeness │ dependency │ feasibility │ aggregator    │  │
│  └───────────────────────────┬──────────────────────────────┘  │
│                              │ 审核通过 → 每个 TaskItem        │
│                              │            创建一个 Pipeline     │
└──────────────────────────────┼─────────────────────────────────┘
                               │
┌──────────────────────────────▼─────────────────────────────────┐
│                    Orchestrator Core                             │
│  ┌────────────┐  ┌────────────┐  ┌─────────────────────┐       │
│  │  Pipeline   │  │  Scheduler │  │  Project Manager    │       │
│  │  Engine     │  │  (并发控制) │  │  (多项目 + 配置)    │       │
│  └─────┬──────┘  └─────┬──────┘  └──────────┬──────────┘       │
│        │               │                     │                  │
│  ┌─────▼───────────────▼─────────────────────▼──────────────┐   │
│  │              Reactions Engine                              │   │
│  │   事件驱动：CI 失败/Review 评论/Stage 完成 → 自动响应       │   │
│  └───────────────────────┬──────────────────────────────────┘   │
│                          │                                      │
│  ┌───────────────────────▼──────────────────────────────────┐   │
│  │                  Event Bus                                │   │
│  │         Go channels → 广播到所有消费者                      │   │
│  └───────────────────────┬──────────────────────────────────┘   │
│                          │                                      │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │              Plugin 层（10 个可插拔槽位）                   │   │
│  │                                                          │   │
│  │  ┌─ Agent ──────┐  ┌─ Runtime ────┐  ┌─ Workspace ────┐ │   │
│  │  │ claude-code  │  │ process      │  │ worktree       │ │   │
│  │  │ codex        │  │ tmux         │  │ clone          │ │   │
│  │  │ aider (扩展) │  │ docker (扩展)│  │                │ │   │
│  │  └──────────────┘  └──────────────┘  └────────────────┘ │   │
│  │                                                          │   │
│  │  ┌─ Spec ───────┐  ┌─ SCM ────────┐  ┌─ Notifier ────┐ │   │
│  │  │ openspec     │  │ github       │  │ desktop       │ │   │
│  │  │              │  │              │  │ slack (扩展)  │ │   │
│  │  └──────────────┘  └──────────────┘  │ webhook       │ │   │
│  │                                      └────────────────┘ │   │
│  │  ┌─ Store ──────┐  ┌─ Terminal ───┐                     │   │
│  │  │ sqlite       │  │ web (主)     │                     │   │
│  │  │              │  │ tui (备选)   │                     │   │
│  │  └──────────────┘  └──────────────┘                     │   │
│  │                                                          │   │
│  │  ┌─ ReviewGate (新增) ──┐  ┌─ Tracker (新增) ─────┐ │   │
│  │  │ ai-panel (默认)      │  │ local-db (默认)          │ │   │
│  │  │ local-approval       │  │ github-issue (可选)      │ │   │
│  │  │ github-pr (可选)     │  │ linear (扩展)            │ │   │
│  │  └──────────────────────┘  └──────────────────────────┘ │   │
│  └──────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────┘
```

### Plugin 槽位说明

| 槽位 | 职责 | 默认实现 | 可扩展 |
|---|---|---|---|
| Agent | AI 编码工具封装 | claude-code, codex | aider, opencode |
| Runtime | Agent 执行环境 | process（直接子进程） | tmux, docker, k8s |
| Workspace | 代码隔离方式 | worktree | clone |
| Spec | 项目规格上下文（供 Secretary Agent 使用） | openspec | mcp（P4）, 自定义 spec 源（Notion, Confluence 等） |
| SCM | 代码托管操作 | local-git（本地分支/commit/push） | github（PR/Issue 同步） |
| Notifier | 人工通知渠道 | desktop | slack, webhook |
| Store | 状态持久化 | sqlite | postgres（远程） |
| Terminal | Agent 输出的实时渲染适配器 | web (WebSocket) | tui (Bubble Tea) |
| **ReviewGate** | **TaskPlan 审核机制** | **ai-panel（Multi-Agent 审核委员会）** | **local-approval, github-pr** |
| **Tracker** | **子任务外部系统同步** | **local-db（纯本地，空实现）** | **github-issue, linear** |

> **Terminal 插件**只负责将 Agent 的流式输出适配到不同渲染目标（WebSocket / TUI 终端），不包含完整的 UI 框架逻辑。Web Workbench 和 TUI 是独立的接入层。
>
> **ReviewGate 和 Tracker** 是 Secretary Layer 引入的两个新插件槽位。ReviewGate 负责 TaskPlan 的审核机制（默认使用 Multi-Agent 自动审核）；Tracker 负责将子任务状态镜像到外部系统（默认纯本地，GitHub Issue 为可选增强）。

每个插件实现一个 Go interface + 注册函数。Plugin 之间通过 Event Bus 解耦。

```go
// 插件注册
type PluginModule struct {
    Name     string
    Slot     PluginSlot  // "agent" | "runtime" | "workspace" | ...
    Factory  func(cfg map[string]any) (Plugin, error)
}

// 所有插件的公共接口
type Plugin interface {
    Name() string
    Init(ctx context.Context) error
    Close() error
}
```

### 设计原则

- **默认实现够用，不强制扩展** — P0 阶段只需 process + worktree + sqlite，不需要 tmux/docker
- **换任何一个插件不影响其他插件** — Agent 换成 Aider 不需要改 Pipeline Engine
- **配置驱动** — YAML 中声明使用哪个插件，启动时动态加载
- **本地优先，外部增强** — 核心功能（任务拆解、审核、调度、执行）完全在本地 SQLite + Git 上运行，GitHub/Linear 等外部系统是可选的状态镜像
- **1 TaskItem = 1 Pipeline** — 每个子任务对应一个独立 Pipeline，执行器只管自己的 Pipeline，依赖调度交给 DAG Scheduler
- **审核默认自动** — Multi-Agent 审核委员会自动审核纠错，人工是兜底而非默认
- **P0 接口先行，实现从简** — P0 阶段定义 Go interface（AgentPlugin, RuntimePlugin 等）约束设计质量，但只实现一个 concrete 实现（process + worktree + sqlite），直接 `New()` 构造，不引入 factory 注册和动态加载。P1 再引入 factory + 配置驱动。

借鉴来源：ComposioHQ/agent-orchestrator 的 6 槽位 Plugin 架构（Agent、Runtime、Workspace、SCM、Notifier、Terminal），调整为 Go interface + factory 模式，新增 Spec、Store、ReviewGate、Tracker 四个槽位，共 10 个。

## 技术选型

| 层级 | 选型 | 理由 |
|---|---|---|
| 语言 | Go 1.22+ | 子进程管理成熟、goroutine 天然并发、单二进制分发 |
| TUI | Bubble Tea + Lip Gloss | Go 生态最成熟的 TUI 框架 |
| Web 后端 | net/http + nhooyr.io/websocket | 标准库够用，WebSocket 用 nhooyr（gorilla 已归档停维） |
| Web 前端 | React + Tailwind（内嵌到 Go 二进制） | 用 embed.FS 打包，零外部依赖部署 |
| 存储 | SQLite (via modernc.org/sqlite) | 纯 Go 实现、零 CGO、单文件数据库 |
| Git 操作 | os/exec 调用 git CLI | 比 go-git 可靠，worktree 支持完整 |
| GitHub | google/go-github + webhook 库 | 成熟稳定 |
| 配置 | YAML (gopkg.in/yaml.v3) | 人类可读，三级覆盖 |
| 日志 | slog（标准库） | Go 1.21 内置，结构化日志 |
| 通知 | beeep (桌面) / slack-go (Slack) | Notifier 插件用，按需引入 |

## 项目目录结构

```
ai-workflow/
├── cmd/
│   ├── server/main.go          # Web server + API 入口
│   └── ai-flow/main.go         # TUI + CLI 入口（轻量备选）
│
├── internal/
│   ├── core/                   # 核心领域模型
│   │   ├── project.go          # Project 实体
│   │   ├── pipeline.go         # Pipeline 实体 + 状态定义
│   │   ├── stage.go            # Stage 定义 + Template
│   │   ├── taskplan.go         # TaskPlan + TaskItem 实体
│   │   ├── events.go           # 事件类型定义
│   │   ├── store.go            # Store 接口
│   │   └── plugin.go           # Plugin 接口 + 槽位定义 + 注册表
│   │
│   ├── engine/                 # Pipeline 执行引擎
│   │   ├── executor.go         # 阶段调度 + 执行
│   │   ├── checkpoint.go       # 检查点 + 人工介入
│   │   ├── scheduler.go        # 并发调度（信号量）
│   │   ├── reactions.go        # Reactions 事件响应引擎
│   │   └── infer.go            # 模板自动推断
│   │
│   ├── secretary/              # Secretary Layer（任务拆解 + 审核 + DAG 调度）
│   │   ├── agent.go            # Secretary Agent（对话 → 任务拆解）
│   │   ├── taskplan.go         # TaskPlan 管理
│   │   ├── review.go           # Multi-Agent 审核流程编排
│   │   ├── scheduler.go        # DAG Scheduler（依赖并行调度）
│   │   └── dag.go              # DAG 数据结构 + 校验
│   │
│   ├── plugins/                # 所有插件实现
│   │   ├── agent-claude/       # Agent 插件：Claude Code
│   │   ├── agent-codex/        # Agent 插件：Codex CLI
│   │   ├── spec-openspec/      # Spec 插件：OpenSpec
│   │   ├── spec-mcp/           # Spec 插件：MCP（P4）
│   │   ├── runtime-process/    # Runtime 插件：直接子进程
│   │   ├── runtime-tmux/       # Runtime 插件：tmux 会话
│   │   ├── workspace-worktree/ # Workspace 插件：git worktree
│   │   ├── scm-github/         # SCM 插件：GitHub PR/分支
│   │   ├── review-ai-panel/    # ReviewGate 插件：Multi-Agent 审核委员会
│   │   ├── review-local/       # ReviewGate 插件：本地人工审批
│   │   ├── tracker-local/      # Tracker 插件：本地（空实现）
│   │   ├── tracker-github/     # Tracker 插件：GitHub Issue
│   │   ├── tracker-linear/     # Tracker 插件：Linear（P4）
│   │   ├── notifier-desktop/   # Notifier 插件：桌面通知
│   │   ├── notifier-slack/     # Notifier 插件：Slack
│   │   ├── notifier-webhook/   # Notifier 插件：通用 Webhook
│   │   ├── store-sqlite/       # Store 插件：SQLite
│   │   └── terminal-web/       # Terminal 插件：WebSocket
│   │
│   ├── git/                    # Git 操作（被 workspace 插件调用）
│   │   ├── worktree.go
│   │   ├── branch.go
│   │   └── ops.go
│   │
│   ├── config/                 # 配置管理
│   │   ├── config.go           # 三级配置合并 + 插件声明解析
│   │   └── defaults.go
│   │
│   ├── eventbus/               # 事件总线
│   │   └── bus.go
│   │
│   ├── web/                    # Web 层
│   │   ├── server.go           # HTTP 服务 + 路由
│   │   ├── handlers_project.go # 项目 API
│   │   ├── handlers_pipeline.go# Pipeline API
│   │   ├── handlers_chat.go    # Chat API
│   │   ├── handlers_plan.go    # TaskPlan API
│   │   └── ws.go               # WebSocket 管理
│   │
│   └── tui/                    # TUI 层（轻量备选）
│       ├── app.go
│       ├── views/
│       └── styles.go
│
├── web/dashboard/              # 前端源码（编译后 embed）
│   ├── src/
│   │   ├── views/
│   │   │   ├── ChatView.tsx    # 对话视图
│   │   │   ├── PlanView.tsx    # 计划视图 + DAG 可视化
│   │   │   ├── BoardView.tsx   # 看板视图
│   │   │   └── PipelineView.tsx# Pipeline 详情
│   │   ├── components/
│   │   │   ├── DAGGraph.tsx    # React Flow DAG 组件
│   │   │   ├── TaskCard.tsx    # 任务卡片
│   │   │   ├── ReviewPanel.tsx # 审核面板
│   │   │   └── ChatMessage.tsx # 聊天消息气泡
│   │   ├── stores/
│   │   │   └── useStore.ts     # Zustand 状态管理
│   │   └── App.tsx
│   ├── package.json
│   └── vite.config.ts
│
├── configs/
│   ├── defaults.yaml           # 全局默认配置
│   └── prompts/                # Prompt 模板
│       ├── requirements.tmpl
│       ├── implement.tmpl
│       ├── code_review.tmpl
│       ├── fixup.tmpl
│       ├── e2e_test.tmpl
│       ├── secretary.tmpl      # Secretary Agent 任务拆解
│       ├── review_completeness.tmpl
│       ├── review_dependency.tmpl
│       ├── review_feasibility.tmpl
│       └── review_aggregator.tmpl
│
├── go.mod
└── go.sum
```

## 核心数据流

```
主流程（Secretary Layer → Pipeline Engine）：

1. 用户通过 Workbench 对话描述需求
   └── Web Chat / TUI / GitHub Issue（可选）

2. Secretary Agent 理解需求 → 调用 LLM 拆解为 TaskPlan
   └── TaskPlan = 多个 TaskItem + 依赖关系 DAG

3. Multi-Agent Review Panel 自动审核
   ├── 完整性 Agent + 依赖性 Agent + 可行性 Agent 并行审核
   ├── Aggregator 综合研判 → approve / fix / escalate
   └── fix 时自动修正并重新审核（最多 N 轮）

4. DAG Scheduler 接管已审核的 TaskPlan
   ├── 构建依赖图，找出无依赖的 TaskItem → 标记 ready
   ├── 为每个 ready 的 TaskItem 创建 Pipeline（1:1）
   └── 并行启动多个 Pipeline

5. Pipeline Engine 逐阶段执行每个 Pipeline（现有逻辑不变）
   ├── 通过 Agent Driver 调用外部 CLI
   ├── 流式输出经 Event Bus 广播到 Workbench
   ├── 遇到人工检查点 → 暂停等待
   └── 完成 → 更新 Store + 通知 DAG Scheduler

6. DAG Scheduler 推进后续任务
   ├── Pipeline 完成 → 标记 TaskItem done → 检查下游依赖
   ├── 下游所有上游 done → 创建并启动下游 Pipeline
   └── 所有 TaskItem done → TaskPlan done

7. 产出
   ├── Git: 每个子任务一个 worktree + 分支 + 合并
   ├── GitHub: PR + Issue 状态同步（可选）
   ├── Spec: Secretary 读取项目规格上下文（可选）
   └── Store: 完整执行记录 + 对话历史 + 审核记录

直接模式（跳过 Secretary Layer，兼容 P0 行为）：
用户也可以直接创建单个 Pipeline，不经过任务拆解。
此时行为和 P0 完全一致，Secretary Layer 不参与。
```

## 实施分期

| 阶段 | 范围 | 产出 |
|---|---|---|
| P0 ✅ | Agent Driver + Pipeline Engine + CLI/TUI | 本地单任务自动化工具 |
| P1 ✅ | 多项目调度 + 配置驱动工厂 + 崩溃恢复 | Scheduler + Registry + 三级配置 + Reactions V1 |
| P2-Foundation ✅ | 插件接口 + API 基础设施 | ReviewGate/Tracker/SCM/Notifier 接口 + local 默认实现 + REST/WS |
| P2a ✅ | Secretary Agent + TaskPlan + DAG Scheduler | 对话 → 任务拆解 → 依赖并行执行（纯后端） |
| P2b ✅ | Multi-Agent Review Panel | AI 强门禁审核（3 Reviewer + Aggregator，max_rounds=2） |
| P2c ✅ | Workbench UI (Web) | Chat + Plan + Board + Pipeline 四视图，Web 为主界面 |
| P3 🔧 | GitHub 集成（**可选增强**） | github-issue Tracker + github-pr ReviewGate + Webhook |
| P4 | 高级定制 + MCP 扩展 + 通知 | 自定义 Template + Slack/Webhook 通知 |

> **关键变化**：GitHub 从 P2 推迟到 P3，变为可选增强而非必需。P2 专注于让 Secretary Layer（任务拆解 + 审核 + DAG 调度）+ Workbench UI 在纯本地跑通。详见 [spec-secretary-layer.md](spec-secretary-layer.md)。
>
> **当前进度**：P3 Wave1（GitHub 基础设施）已完成——包括认证客户端、GitHub Service 操作层、Webhook 端点与签名验证、多项目路由、配置与工厂选择逻辑。Wave2（核心业务：tracker-github、scm-github、Issue 触发、斜杠命令）进行中。review-github-pr 为可选增强，不阻塞 P3 Done。

## 架构边界与演进方向

### 当前定位

本系统是**单用户、本地优先、单进程**的智能任务编排工具，不是多团队分布式平台。这是刻意的设计选择：

- 核心功能完全在本地 SQLite + Git 上运行，不依赖外部服务
- 单进程内 goroutine 并发，不引入分布式协调（etcd / Saga / 消息队列）
- GitHub 是可选镜像，不是 source of truth
- 面向个人开发者或小团队的单实例部署

### 已覆盖的健壮性设计

以下常见系统风险在当前 spec 中已有对应设计：

| 风险 | 覆盖位置 |
|------|---------|
| 任务拆解粒度不一 | Secretary Agent Prompt 强制 JSON schema + 审核委员会三维校验（完整性/依赖性/可行性） |
| 依赖循环 | DAG.Validate() Kahn 算法检测环 + 自依赖 + 孤立引用（[spec-secretary-layer.md](spec-secretary-layer.md) §VI） |
| 执行器崩溃 | Pipeline 崩溃恢复（Checkpoint 恢复）+ Activity Detection 30s 轮询（[spec-agent-drivers.md](spec-agent-drivers.md) §VII） |
| 人工逃生舱 | 8 种人工操作 + FailPolicy 三策略 + waiting_human 两种原因（[spec-pipeline-engine.md](spec-pipeline-engine.md) §IV） |
| 权限与安全 | GitHub App vs PAT 双模认证 + per-repo webhook secret + author_association 权限矩阵（[spec-github-integration.md](spec-github-integration.md) §VII） |
| 数据一致性 | "GitHub 是镜像不是 source of truth" + 离线降级 + 恢复后最终状态同步（[spec-github-integration.md](spec-github-integration.md) §IX） |
| 审核版本追溯 | review_records 每轮审核快照 + human_actions 审计日志（[spec-api-config.md](spec-api-config.md) §IV） |
| 多语言支持 | AllowedTools 按阶段+语言栈配置 + 项目级 stage_tools 覆盖（[spec-agent-drivers.md](spec-agent-drivers.md) §III） |
| 外部系统集成 | 10 插件槽位（Notifier/Tracker/SCM 各有多种实现）（本文档"Plugin 槽位说明"） |

### P3 补强项

以下在 P3 实现中一并落地：

- **GitHub 写操作限流器**：所有 GitHub API 写操作经过令牌桶限流（`golang.org/x/time/rate`），确保不超配额。详见 [spec-github-integration.md](spec-github-integration.md) §VII。
- **GitHub 链路 Trace（入口级）**：P3 覆盖 webhook / slash command / pipeline 入口 trace 贯通，目标是排障与回放。

### P4 规划项

以下功能纳入 P4 阶段考虑：

| 功能 | 说明 |
|------|------|
| **优先级调度** | TaskItem 增加 `priority` 字段（P0/P1/P2），DAG Scheduler 按优先级排序 ready 队列，支持优先级继承（高优任务的阻塞依赖自动提升） |
| **Token 预算控制** | 全局和项目级月度 Token 上限，超限暂停自动任务并通知。配置项：`budget.monthly_token_limit` |
| **统一 Trace ID** | TaskPlan 级生成 trace_id，传递到 TaskItem → Pipeline → Checkpoint → 日志，便于端到端追踪（该项补齐 P3 入口级 trace 未覆盖的 Secretary→DAG→Review 路径） |
| **数据库备份** | 支持定时自动备份 SQLite 数据库。配置项：`store.backup.interval`、`store.backup.path`（见 [spec-api-config.md](spec-api-config.md) §III） |

### 不在路线图中的设计

以下"多团队分布式"方案在当前架构下不适用，**刻意不引入**以避免过度工程：

- 分布式调度器（etcd / PostgreSQL 集群）— 单进程 + 崩溃恢复已满足可靠性
- 事件溯源 / Saga 模式 — 本地 SQLite 事务 + "GitHub 是镜像"设计已避免一致性问题
- 跨团队依赖协调 — 所有依赖在单个 DAG 内解析，不存在跨组问题
- mTLS 服务间认证 — 单进程，无服务间通信
- 角色化权限与数据隔离 — 单用户工具，无跨团队数据泄露风险

> **演进原则**：如果未来需要扩展为团队级平台，优先通过**多实例部署 + 共享 Git 仓库**的方式横向扩展，而非将单进程改造为分布式系统。每个团队运行自己的 Orchestrator 实例，通过 Git 和 GitHub 作为天然的协调层。

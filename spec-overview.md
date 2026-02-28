# AI Workflow Orchestrator — 系统总览

## 项目定位

一个用 Go 编写的 AI 开发流程编排引擎，将 Claude Code、Codex CLI、OpenSpec 三个工具串联为自动化 Pipeline，支持多项目管理、人工介入、GitHub 集成，提供 TUI 和 Web 两种操作界面。

## 要解决的问题

当前手工流程的痛点：

- 每次开发都要在多个 CLI 之间手动切换，重复执行相似的命令序列
- 小 bug 和大 feature 走同样的重流程，效率浪费
- 上下文（需求、spec、review 结果）散落在不同的终端会话中，无法追溯
- 多项目切换时要记住每个项目的路径和配置差异
- 缺乏和 GitHub Issue/PR 的联动，状态靠人脑同步

## 整体架构

```
┌──────────────────────────────────────────────────────────┐
│                    接入层（Ingress）                       │
│   TUI (Bubble Tea)  │  Web Dashboard  │  GitHub Webhook  │
└──────────┬───────────┴────────┬────────┴────────┬────────┘
           │                    │                  │
┌──────────▼────────────────────▼──────────────────▼────────┐
│                    Orchestrator Core                       │
│  ┌────────────┐  ┌────────────┐  ┌─────────────────────┐  │
│  │  Pipeline   │  │  Scheduler │  │  Project Manager    │  │
│  │  Engine     │  │  (并发控制) │  │  (多项目 + 配置)    │  │
│  └─────┬──────┘  └─────┬──────┘  └──────────┬──────────┘  │
│        │               │                     │             │
│  ┌─────▼───────────────▼─────────────────────▼──────────┐  │
│  │              Reactions Engine                          │  │
│  │   事件驱动：CI 失败/Review 评论/Stage 完成 → 自动响应   │  │
│  └───────────────────────┬──────────────────────────────┘  │
│                          │                                 │
│  ┌───────────────────────▼──────────────────────────────┐  │
│  │                  Event Bus                            │  │
│  │         Go channels → 广播到所有消费者                  │  │
│  └───────────────────────┬──────────────────────────────┘  │
│                          │                                 │
│  ┌──────────────────────────────────────────────────────┐  │
│  │              Plugin 层（9 个可插拔槽位）                │  │
│  │                                                      │  │
│  │  ┌─ Agent ──────┐  ┌─ Runtime ────┐  ┌─ Workspace ┐  │  │
│  │  │ claude-code  │  │ process      │  │ worktree   │  │  │
│  │  │ codex        │  │ tmux         │  │ clone      │  │  │
│  │  │ aider (扩展) │  │ docker (扩展)│  │            │  │  │
│  │  └──────────────┘  └──────────────┘  └────────────┘  │  │
│  │                                                      │  │
│  │  ┌─ Spec ───────┐  ┌─ Tracker ────┐  ┌─ SCM ──────┐  │  │
│  │  │ openspec     │  │ github       │  │ github     │  │  │
│  │  │              │  │ linear (扩展) │  │            │  │  │
│  │  └──────────────┘  └──────────────┘  └────────────┘  │  │
│  │                                                      │  │
│  │  ┌─ Notifier ───┐  ┌─ Store ──────┐  ┌─ Terminal ─┐  │  │
│  │  │ desktop      │  │ sqlite       │  │ tui        │  │  │
│  │  │ slack (扩展) │  │              │  │ web        │  │  │
│  │  │ webhook      │  │              │  │            │  │  │
│  │  └──────────────┘  └──────────────┘  └────────────┘  │  │
│  └──────────────────────────────────────────────────────┘  │
└────────────────────────────────────────────────────────────┘
```

### Plugin 槽位说明

| 槽位 | 职责 | 默认实现 | 可扩展 |
|---|---|---|---|
| Agent | AI 编码工具封装 | claude-code, codex | aider, opencode |
| Runtime | Agent 执行环境 | process（直接子进程） | tmux, docker, k8s |
| Workspace | 代码隔离方式 | worktree | clone |
| Spec | 规格文档生命周期 | openspec | 自定义 spec 工具 |
| Tracker | 任务来源 | github（Issue） | linear |
| SCM | 代码托管操作 | github（PR/分支） | — |
| Notifier | 人工通知渠道 | desktop | slack, webhook |
| Store | 状态持久化 | sqlite | postgres（远程） |
| Terminal | Agent 输出的实时渲染适配器 | tui (Bubble Tea) | web (xterm.js) |

> **Terminal 插件**只负责将 Agent 的流式输出适配到不同渲染目标（TUI 终端 / WebSocket），不包含完整的 UI 框架逻辑。TUI 和 Web Dashboard 是独立的接入层。

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
- **P0 接口先行，实现从简** — P0 阶段定义 Go interface（AgentPlugin, RuntimePlugin 等）约束设计质量，但只实现一个 concrete 实现（process + worktree + sqlite），直接 `New()` 构造，不引入 factory 注册和动态加载。P1 再引入 factory + 配置驱动。

借鉴来源：ComposioHQ/agent-orchestrator 的 8 槽位 Plugin 架构，但调整为 Go interface + factory 模式，并新增 Spec 和 Store 两个槽位。

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
│   └── ai-flow/main.go         # TUI + CLI 入口
│
├── internal/
│   ├── core/                   # 核心领域模型
│   │   ├── project.go          # Project 实体
│   │   ├── pipeline.go         # Pipeline 实体 + 状态定义
│   │   ├── stage.go            # Stage 定义 + Template
│   │   ├── events.go           # 事件类型定义
│   │   └── plugin.go           # Plugin 接口 + 槽位定义 + 注册表
│   │
│   ├── engine/                 # Pipeline 执行引擎
│   │   ├── executor.go         # 阶段调度 + 执行
│   │   ├── checkpoint.go       # 检查点 + 人工介入
│   │   ├── scheduler.go        # 并发调度（信号量）
│   │   ├── reactions.go        # Reactions 事件响应引擎
│   │   └── infer.go            # 模板自动推断
│   │
│   ├── plugins/                # 所有插件实现
│   │   ├── agent-claude/       # Agent 插件：Claude Code
│   │   ├── agent-codex/        # Agent 插件：Codex CLI
│   │   ├── spec-openspec/      # Spec 插件：OpenSpec
│   │   ├── runtime-process/    # Runtime 插件：直接子进程
│   │   ├── runtime-tmux/       # Runtime 插件：tmux 会话
│   │   ├── workspace-worktree/ # Workspace 插件：git worktree
│   │   ├── tracker-github/     # Tracker 插件：GitHub Issue
│   │   ├── scm-github/         # SCM 插件：GitHub PR/分支
│   │   ├── notifier-desktop/   # Notifier 插件：桌面通知
│   │   ├── notifier-slack/     # Notifier 插件：Slack
│   │   ├── notifier-webhook/   # Notifier 插件：通用 Webhook
│   │   ├── store-sqlite/       # Store 插件：SQLite
│   │   └── terminal-tui/       # Terminal 插件：Bubble Tea
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
│   │   ├── server.go
│   │   ├── handlers.go
│   │   └── ws.go
│   │
│   └── tui/                    # TUI 层
│       ├── app.go
│       ├── views/
│       └── styles.go
│
├── web/dashboard/              # 前端源码（编译后 embed）
│
├── configs/
│   ├── defaults.yaml           # 全局默认配置
│   └── prompts/                # Prompt 模板
│
├── go.mod
└── go.sum
```

## 核心数据流

```
1. 输入源（三选一）
   TUI 手动创建 / Web 创建任务 / GitHub Issue 触发

2. Project Manager 定位项目 → 解析配置

3. Engine 推断或使用指定的 Template → 创建 Pipeline

4. Executor 逐阶段执行
   ├── 通过 Agent Driver 调用外部 CLI
   ├── 流式输出经 Event Bus 广播到 TUI / Web / GitHub
   ├── 遇到人工检查点 → 暂停等待
   └── 完成 → 更新 Store + GitHub 状态

5. 产出
   ├── Git: worktree、分支、合并
   ├── GitHub: PR（带 spec 摘要 + review 结果）
   ├── OpenSpec: spec 文件归档
   └── Store: 完整执行记录
```

## 实施分期

| 阶段 | 范围 | 产出 |
|---|---|---|
| P0 | Agent Driver + Pipeline Engine + CLI/TUI | 本地可用的自动化工具 |
| P1 | 多项目 + 并发调度 + SQLite | 支持日常多项目使用 |
| P2 | GitHub Webhook + Issue/PR 联动 | 团队可用，GitHub 驱动 |
| P3 | Web Dashboard + WebSocket | 可视化操作、进度监控 |
| P4 | 自定义 Template + MCP 扩展 + 通知 | 高级定制 |

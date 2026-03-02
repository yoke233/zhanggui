# AI Workflow Orchestrator — 系统总览

## 项目定位

一个用 Go 编写的**智能任务分解与并行执行平台**，通过 ACP（Agent Client Protocol）统一 Secretary 与执行 Agent 的通信。用户通过 Web Workbench 与 Secretary Agent 进行多轮对话，Secretary 作为持久交互式 Agent（工作目录即项目目录，拥有文件读写权限）理解需求并探索代码。当需求明确后，用户指示 Secretary 在项目中生成计划文件（格式自由），经用户勾选后提交为结构化 TaskPlan，再由 Multi-Agent 审核委员会自动审核纠错，DAG Scheduler 管理依赖关系并行调度多个 Pipeline 执行。每个子任务由 Claude Code、Codex CLI 等 AI 编码工具（均通过 ACP 协议驱动）独立完成。提供 Web Workbench 作为主操作界面（含全局 Admin 管理视图），GitHub 集成作为可选增强。

## 要解决的问题

当前手工流程的痛点：

- 每次开发都要在多个 CLI 之间手动切换，重复执行相似的命令序列
- 小 bug 和大 feature 走同样的重流程，效率浪费
- 上下文（需求、spec、review 结果）散落在不同的终端会话中，无法追溯
- 多项目切换时要记住每个项目的路径和配置差异
- 复杂需求只能串行处理，无法将独立子任务并行分配给多个 AI Agent
- 任务拆解质量靠人脑把关，缺乏自动审核纠错机制
- 项目导入缺乏便捷方式，需要手动管理本地路径和 git clone
- 执行全过程缺乏审计追溯，操作记录分散
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
│                                                                │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │ Secretary Agent (持久交互 session)                        │   │
│  │ 工作目录=项目目录 │ 文件读写权限 │ Agent 可切换            │   │
│  │ 多轮对话 │ 生成计划文件(格式自由) │ 查询工具(进度/状态)    │   │
│  └──────────────────────┬──────────────────────────────────┘   │
│                         │ 用户勾选文件 → 创建 Plan              │
│  ┌──────────────────────▼──────────────────────────────────┐   │
│  │ Plan Parser (AI 解析文件 → 结构化 TaskPlan + TaskItems)   │   │
│  └──────────────────────┬──────────────────────────────────┘   │
│                         │                                      │
│  ┌──────────────────────▼──────────────────────────────────┐   │
│  │          Multi-Agent Review Orchestrator                   │   │
│  │   completeness │ dependency │ feasibility │ aggregator    │   │
│  └───────────────────────────┬──────────────────────────────┘   │
│                              │ 审核通过 → 每个 TaskItem          │
│  ┌───────────────┐  ┌───────▼───────┐  创建一个 Pipeline       │
│  │  TaskPlan     │  │  DAG          │                          │
│  │  Manager      │  │  Scheduler    │                          │
│  │ (CRUD+状态)   │  │ (依赖并行调度) │                          │
│  └───────────────┘  └───────────────┘                          │
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
│  │              ACP Client 层                                │   │
│  │  internal/acpclient/ — 统一 Agent 通信（JSON-RPC 2.0）    │   │
│  │  启动命令配置驱动，不区分 Claude/Codex                      │   │
│  └──────────────────────────────────────────────────────────┘   │
│                          │                                      │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │              Plugin 层（7 个可插拔槽位）                   │   │
│  │                                                          │   │
│  │  ┌─ Workspace ────┐  ┌─ SCM ────────┐                   │   │
│  │  │ worktree       │  │ github       │                   │   │
│  │  │ clone          │  │              │                   │   │
│  │  └────────────────┘  └──────────────┘                   │   │
│  │                                                          │   │
│  │  ┌─ Notifier ────┐  ┌─ Store ──────┐  ┌─ Terminal ───┐ │   │
│  │  │ desktop       │  │ sqlite       │  │ web (主)     │ │   │
│  │  │ slack (扩展)  │  │              │  │ tui (备选)   │ │   │
│  │  │ webhook       │  │              │  │              │ │   │
│  │  └────────────────┘  └──────────────┘  └──────────────┘ │   │
│  │                                                          │   │
│  │  ┌─ ReviewGate ───────────┐  ┌─ Tracker ─────────────┐ │   │
│  │  │ ai-panel (默认)        │  │ local-db (默认)        │ │   │
│  │  │ local-approval         │  │ github-issue (可选)    │ │   │
│  │  │ github-pr (可选)       │  │ linear (扩展)          │ │   │
│  │  └────────────────────────┘  └────────────────────────┘ │   │
│  └──────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────┘
```

### Plugin 槽位说明

| 槽位 | 职责 | 默认实现 | 可扩展 |
|---|---|---|---|
| Workspace | 代码隔离方式 | worktree | clone |
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
    Factory  func(cfg map[string]any) (Plugin, error)
}

// 所有插件的公共接口
type Plugin interface {
    Name() string
    Init(ctx context.Context) error
    Close() error
}

// ACP Client — 统一 Agent 通信层（非插件，核心基础设施）
// 详见 spec-agent-drivers.md（ACP Client 章节）
// agents 定义启动参数，roles 定义角色行为，role_bindings 定义调用方绑定
// ACPClient 处理所有 Agent 交互
```

### 设计原则

- **默认实现够用，不强制扩展** — P0 阶段只需 process + worktree + sqlite，不需要 tmux/docker
- **换任何一个插件不影响其他插件** — 换 SCM 插件不需要改 Pipeline Engine
- **配置驱动** — YAML 中声明使用哪个插件，启动时动态加载
- **本地优先，外部增强** — 核心功能（任务拆解、审核、调度、执行）完全在本地 SQLite + Git 上运行，GitHub/Linear 等外部系统是可选的状态镜像
- **ACP 协议统一通信** — 所有 Agent 交互通过 ACP（JSON-RPC 2.0 over stdio），模型实现与调用方解耦
- **Agent/Role 解耦配置** — `agents` 只定义启动参数与能力上限，`roles` 定义行为策略，`role_bindings` 负责把业务调用映射到角色
- **Bootstrap 统一接入 ACP Client** — 启动阶段按 `agents + roles + role_bindings` 构建 RoleResolver，不再维护 runtime 段
- **1 TaskItem = 1 Pipeline** — 每个子任务对应一个独立 Pipeline，执行器只管自己的 Pipeline，依赖调度交给 DAG Scheduler
- **审核默认自动** — Multi-Agent 审核委员会自动审核纠错，人工是兜底而非默认
- **Secretary 是持久 Agent** — Secretary 不是一次性 LLM 调用，而是一个持久运行的交互式 Agent session，拥有项目文件读写权限，对话不自动生成 Plan
- **计划文件驱动** — TaskPlan 的来源是 Secretary 生成的项目文件（格式自由），由用户勾选后经 AI 解析为结构化任务，而非 Secretary 直接输出 JSON
- **全链路可审计** — 每个操作（对话、文件生成、审批、执行、人工操作）均记录审计日志
- **P0 接口先行，实现从简** — P0 阶段定义 Go interface（Store, ReviewGate 等）约束设计质量，但只实现一个 concrete 实现（process + worktree + sqlite），直接 `New()` 构造，不引入 factory 注册和动态加载。P1 再引入 factory + 配置驱动。

借鉴来源：ComposioHQ/agent-orchestrator 的 6 槽位 Plugin 架构（Agent、Runtime、Workspace、SCM、Notifier、Terminal），调整为 Go interface + factory 模式，删除 Agent + Runtime 槽位（ACP Client 接管），新增 Store、ReviewGate、Tracker 三个槽位，共 7 个。

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
│   ├── acpclient/                # ACP Client — 统一 Agent 通信
│   │   ├── client.go             # ACPClient 主结构
│   │   ├── handler.go            # Client 侧回调 Handler
│   │   └── protocol.go           # JSON-RPC 消息定义
│   │
│   ├── plugins/                # 所有插件实现
│   │   ├── workspace-worktree/ # Workspace 插件：git worktree
│   │   ├── scm-github/         # SCM 插件：GitHub PR/分支
│   │   ├── review-ai-panel/    # ReviewGate 插件：Multi-Agent 审核委员会
│   │   ├── review-local/       # ReviewGate 插件：本地人工审批
│   │   ├── tracker-local/      # Tracker 插件：本地（空实现）
│   │   ├── tracker-github/     # Tracker 插件：GitHub Issue
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
│   │   │   ├── ReviewOrchestratorPanel.tsx # 审核编排面板
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
│       ├── secretary_system.tmpl
│       ├── plan_parser.tmpl
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

1. 创建项目
   ├── 选择本地目录 → 指向已有 git 仓库
   └── 输入 Git URL → 系统 clone 到 ~/.ai-workflow/repos/{project-id}/

2. 进入 Workbench → Chat View，启动 Secretary Agent 持久 session
   ├── Secretary = 选定 Agent（默认 claude，可切换），工作目录 = 项目目录
   ├── 拥有文件读写权限，可探索代码、运行命令
   └── 可通过查询工具实时查看项目进度、Pipeline 状态

3. 多轮对话
   ├── 用户描述需求，Secretary 理解上下文
   ├── Secretary 读写项目文件、分析代码结构
   └── 用户指示 Secretary 生成计划文件

4. Secretary 在项目目录生成计划文件
   ├── 格式自由（.md / .json / .yaml / 混合），由 Secretary 决定
   ├── 写入路径由 Secretary 决定（推荐 .ai-workflow/plans/）
   └── 前端收到文件变更通知，展示新增/修改的文件列表

5. 用户在前端勾选文件 → 提交创建 Plan
   ├── 后端调用 AI（Plan Parser）解析选中文件为结构化 TaskPlan + TaskItems
   └── TaskPlan 记录源文件路径列表（source_files），写入 Store，状态 draft

6. Multi-Agent Review Orchestrator 自动审核
   ├── 完整性 Agent + 依赖性 Agent + 可行性 Agent 并行审核
   ├── Aggregator 综合研判 → approve / fix / escalate
   └── fix 时自动修正并重新审核（最多 N 轮）

7. DAG Scheduler 接管已审核的 TaskPlan
   ├── 构建依赖图，找出无依赖的 TaskItem → 标记 ready
   ├── 为每个 ready 的 TaskItem 创建 Pipeline（1:1）
   └── 并行启动多个 Pipeline

8. Pipeline Engine 逐阶段执行每个 Pipeline（现有逻辑不变）
   ├── 通过 ACP Client 调用 Agent
   ├── 流式输出经 Event Bus 广播到 Workbench
   ├── 遇到人工检查点 → 暂停等待
   └── 完成 → 更新 Store + 通知 DAG Scheduler

9. DAG Scheduler 推进后续任务
   ├── Pipeline 完成 → 标记 TaskItem done → 检查下游依赖
   ├── 下游所有上游 done → 创建并启动下游 Pipeline
   └── 所有 TaskItem done → TaskPlan done

10. 收尾
    ├── Git: 每个子任务一个 worktree + 分支 + 合并
    ├── GitHub: PR + Issue 状态同步（可选）
    └── Store: 完整执行记录 + 对话历史 + 审核记录 + 审计日志

全程审计日志：每个步骤（对话、文件生成、审批、执行、人工操作）
均记录到 audit_log 表，支持按项目/操作类型/时间范围查询。

直接模式（跳过 Secretary Layer）：
用户也可以直接创建单个 Pipeline，不经过任务拆解。
此时 Secretary Layer 不参与。
```

## 实施分期

| 阶段 | 范围 | 产出 |
|---|---|---|
| P0 ✅ | ACP Client（Agent 通信） + Pipeline Engine + CLI/TUI | 本地单任务自动化工具 |
| P1 ✅ | 多项目调度 + 配置驱动工厂 + 崩溃恢复 | Scheduler + Registry + 三级配置 + Reactions V1 |
| P2-Foundation | 插件接口 + API 基础设施 | ReviewGate/Tracker/SCM/Notifier 接口 + local 默认实现 + REST/WS |
| P2a | Secretary Agent + TaskPlan + DAG Scheduler | 对话 → 任务拆解 → 依赖并行执行（纯后端） |
| P2b | Multi-Agent Review Orchestrator | AI 强门禁审核（3 Reviewer + Aggregator，max_rounds=2） |
| P2c | Workbench UI (Web) | Chat + Plan + Board + Pipeline 四视图，Web 为主界面 |
| P3 | GitHub 集成（**可选增强**） | github-issue Tracker + github-pr ReviewGate + Webhook |
| P4 | 高级定制 + MCP 扩展 + 通知 | 自定义 Template + Slack/Webhook 通知 |

> **关键变化**：GitHub 从 P2 推迟到 P3，变为可选增强而非必需。P2 专注于让 Secretary Layer（任务拆解 + 审核 + DAG 调度）+ Workbench UI 在纯本地跑通。详见 [spec-secretary-layer.md](spec-secretary-layer.md)。

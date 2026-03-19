# AI Workflow

**智能 AI Agent 编排平台 — 在一个面板中规划、执行和监控多 Agent 工作流。**

AI Workflow 将你的需求转化为结构化的执行管线。你只需将要完成的事项描述为 Work Item（工作项），系统会将其拆解为 Action（步骤 DAG），将每个 Action 调度给最合适的 AI Agent，并跟踪每次 Run（运行）直至完成 — 内置质量门禁、自动重试和人工介入节点。

## 核心特性

- **工作项管理** — 创建、排序、跟踪工作项的完整生命周期（open → accepted → queued → running → done）。
- **DAG 执行引擎** — 工作项内的 Action 构成依赖图。无依赖的步骤并行执行；Gate 类型步骤作为质量关卡。
- **多 Agent 运行时** — 配置多种 AI Agent 驱动（Claude、Codex 等），每个驱动声明不同的能力集。调度器根据 Action 需求自动匹配最佳 Agent。
- **实时监控** — 仪表盘、分析统计、用量追踪、定期巡检，以及统一的 Activity Journal（活动日志）实现全链路审计。
- **项目组织** — 按项目分组管理工作项，绑定 Git 仓库，独立管理资源。
- **对话线程** — 与 AI Agent 的对话可关联到具体工作项，保持上下文连贯。
- **桌面 & Web** — Go 后端内嵌 Web 控制台；可选 Tauri 封装为本地桌面应用。

## 系统架构

```
┌─────────────────────────────────────────┐
│             Web 控制台                   │
│         React · Vite · Tailwind         │
├─────────────────────────────────────────┤
│           REST / WebSocket API          │
├─────────────────────────────────────────┤
│             Go 后端服务                  │
│  ┌───────────┐ ┌──────────┐ ┌────────┐ │
│  │  DAG 调度  │ │ 执行引擎  │ │ 质量门禁│ │
│  │ Scheduler │ │  Engine  │ │  Gate  │ │
│  └───────────┘ └──────────┘ └────────┘ │
│  ┌───────────┐ ┌──────────┐ ┌────────┐ │
│  │ 活动日志   │ │ Agent   │ │  技能   │ │
│  │ Journal   │ │ Runtime  │ │ Skills │ │
│  └───────────┘ └──────────┘ └────────┘ │
├─────────────────────────────────────────┤
│   SQLite · ACP（Agent 通信协议）         │
└─────────────────────────────────────────┘
```

## 快速开始

### 环境要求

- Go 1.23+
- Node.js 20+
- Git

### 1. 安装前端依赖

```bash
npm --prefix web install
```

### 2. 启动后端服务

```bash
go run ./cmd/ai-flow server --port 8080
```

服务启动后会自动：
- 在 `.ai-workflow/config.toml` 生成默认配置（如不存在）
- 暴露健康检查接口 `/health`
- 在 `/api` 前缀下提供后端 API

### 3. 启动前端开发服务器

```bash
npm --prefix web run dev
```

### 4. 访问地址

- 前端：`http://localhost:5173`
- API：`http://localhost:8080/api`

## 质量门禁

本地推荐直接使用原生 `go` / `npm` 命令，和 GitHub Actions 保持一致：

```bash
gofmt -w $(git ls-files '*.go')
go vet ./...
go test -p 4 -timeout 20m ./...
npm --prefix web ci
npm --prefix web run lint
npm --prefix web run test
npm --prefix web run build
CGO_ENABLED=0 go build -o ./dist/ai-flow ./cmd/ai-flow
```

`scripts/test/` 下的 PowerShell 脚本仍可用于 Windows 本地冒烟和手动回归，但 CI 已不再依赖它们。

## CI/CD

现在仓库的 GitHub Actions 已覆盖前后端主流水线：

| 工作流 | 作用 | 触发条件 |
|--------|------|----------|
| `CI` | 后端 `gofmt` / `go vet` / `go test`，前端 `lint` / `test` / `build`，以及内嵌前端发布包校验 | Pull Request、推送到 `main` |
| `Docker` | 在 PR 上校验 Docker 构建；在 `main` 和版本标签上发布多架构 GHCR 镜像 | Pull Request、推送到 `main`、标签 `v*` |
| `Release` | 构建带内嵌前端的多平台二进制，并发布 GitHub Release 附件 | 标签 `v*`、手动触发 |

## 配置说明

运行时配置位于 `.ai-workflow/config.toml`（首次启动自动创建）。可通过环境变量 `AI_WORKFLOW_DATA_DIR` 覆盖数据目录。

配置文件支持热重载 — 修改后无需重启服务即可生效。

### Agent 驱动

在 `[runtime.agents.drivers]` 下配置 Agent 驱动，每个驱动指向一个 AI Agent 可执行文件（如 Claude CLI、Codex），并声明其能力（文件系统读写、终端访问等）。

### Agent 配置档

`[runtime.agents.profiles]` 定义执行角色 — 包括角色定义、允许的能力集和会话策略。调度器根据 Action 的需求匹配最合适的 Agent。

## 核心概念

| 概念 | 说明 |
|------|------|
| **Work Item（工作项）** | 工作的基本单元，包含标题、优先级、标签和依赖关系。状态：`open` → `accepted` → `queued` → `running` → `done`。 |
| **Action（步骤）** | 工作项执行管线中的一个节点。类型：`exec`（执行任务）、`gate`（质量检查）、`plan`（生成子步骤）。多个 Action 构成 DAG。 |
| **Run（运行）** | 对一个 Action 的单次执行尝试。支持按错误类型（transient / permanent / need_help）自动重试。 |
| **Project（项目）** | 工作项的组织容器，用于分组管理。 |
| **Thread（对话线程）** | 人与 AI Agent 之间的对话，可关联到具体工作项。 |
| **Inspection（巡检）** | 定期或手动的项目健康检查（支持 cron 调度）。 |
| **Activity Journal（活动日志）** | 统一的追加写入审计日志，记录所有状态变更、工具调用、Agent 输出、信号和用量。 |

## 桌面应用

可选的 Tauri 桌面封装：

```bash
npm install
npm run tauri:dev     # 开发模式
npm run tauri:build   # 构建安装包
```

## 许可

私有仓库，保留所有权利。

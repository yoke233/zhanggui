# AI Workflow

本项目包含 Go 后端和 React 前端，默认通过 `http://localhost:5173`（前端）+ `http://127.0.0.1:8080`（后端）联调。

## 环境要求

- Go 1.23+
- Node.js 20+
- Git

## 启动方式

### 1. 启动后端

```powershell
go run ./cmd/ai-flow server --port 8080
```

### 2. 启动前端

首次安装依赖：

```powershell
npm --prefix web install
```

启动开发服务器：

```powershell
npm --prefix web run dev -- --strictPort
```

### 3. 打开页面

- 前端地址：`http://localhost:5173`
- 后端健康检查：`http://127.0.0.1:8080/health`

## 项目创建（前端）

页面顶部「创建项目」支持三种来源：

- `local_path`：填写项目名 + 本地仓库路径。
- `local_new`：填写项目名，后端自动在 `~/.ai-workflow/repos/<slug>` 创建并 `git init`。
- `github_clone`：填写项目名 + GitHub Owner/Repo（本轮未做真实账号联调）。

创建流程是异步的，前端会通过 WS（断开时轮询）显示请求状态并刷新项目列表。

## Chat 模型接入（真实 CLI）

`/chat` 已支持真实模型多轮会话，后端会持久化 provider 的 session id 并在下一轮继续对话。

- 默认使用 `claude`
- 可切换为 `codex`：

```powershell
$env:AI_WORKFLOW_CHAT_PROVIDER="codex"
go run ./cmd/ai-flow server --port 8080
```

可用取值：

- `claude`（默认）
- `codex`

## 测试命令

常用脚本位于 `scripts/test`：

```powershell
pwsh -NoProfile -File .\scripts\test\backend-all.ps1
pwsh -NoProfile -File .\scripts\test\frontend-unit.ps1
pwsh -NoProfile -File .\scripts\test\frontend-build.ps1
pwsh -NoProfile -File .\scripts\test\p3-integration.ps1
```

## 浏览器联调（agent-browser）

本轮已用 `agent-browser` 实测并跑通：

- `local_path` 创建
- `local_new` 创建
- `github_clone` 表单显示与校验（未做真实 clone）

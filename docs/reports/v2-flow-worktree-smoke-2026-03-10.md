# V2 Flow + git worktree 简单流程跑通记录（2026-03-10）

## 目标

- 验证可以从 GitHub 拉取仓库到本地
- 在 v2 后端条件下，通过 **ResourceBinding(kind=git)** 自动创建并使用 **git worktree** 执行 Flow
- 在没有外部 ACP/LLM 凭证的环境下，也能跑通一个最小闭环（用于开发/冒烟）

## 仓库与权限检查

- 远程仓库：`git@github.com:yoke233/test-workflow.git`
- 使用 `git ls-remote`（BatchMode）确认 SSH 访问可用
- 该仓库是空仓库（clone 时提示 empty repository）

## 关键问题与处理

### 1) 空仓库无法创建 worktree

现象：
- `git worktree add` 需要至少一个 commit（unborn HEAD 时会失败）

处理：
- 在本地 clone 后创建 `main` 分支并提交一个初始 commit（不需要 push）

### 2) v2 默认 StepExecutor 依赖 ACP agent/LLM 凭证

现象：
- v2 引擎默认使用 `NewACPStepExecutor`，会 spawn 外部 ACP 进程
- 本机没有 `OPENAI_API_KEY/ANTHROPIC_API_KEY` 等凭证时，真实执行链路会卡在 agent 侧

处理：
- 增加 v2 的 **mock step executor**：
  - 配置：`v2.mock_executor = true`（TOML）或环境变量 `AI_WORKFLOW_V2_MOCK_EXECUTOR=1`
  - mock executor 会：
    - 发布一条 `exec.agent_output` 的 `done` 事件
    - 写入一个简短 artifact（markdown）
    - 让 step 正常成功，从而打通调度/事件/存储/WS 等主链路

相关代码：
- `internal/v2/engine/mock_executor.go`
- `cmd/ai-flow/server.go`（v2 bootstrap 选择 ACP 或 mock）
- `internal/config/types.go`（新增 `v2.mock_executor` 字段）

## 跑通步骤（示例）

1. 启动服务（示例端口 8082）并开启 mock：
   - `AI_WORKFLOW_V2_MOCK_EXECUTOR=1 go run ./cmd/ai-flow server --port 8082`

2. 走 v2 API 最小闭环：
   - `POST /api/v2/projects` 创建项目（kind=dev）
   - `POST /api/v2/projects/{id}/resources` 绑定本地 git repo（kind=git, uri=<repoPath>）
   - `POST /api/v2/flows` 创建 flow（project_id=<id>）
   - `POST /api/v2/flows/{flowID}/steps` 创建 1 个 `exec` step
   - `POST /api/v2/flows/{flowID}/run` 触发执行
   - 轮询 `GET /api/v2/flows/{flowID}` 直到 `status=done`

验证点：
- worktree 路径：`<repoPath>/.worktrees/flow-<flowID>` 在执行时创建，Flow 完成后被清理
- execution 与 artifact 可通过 v2 API 拉取

## 额外备注（secrets.toml）

- 当前 `.ai-workflow/secrets.toml` 里出现的 `merge_pat` / `commit_pat` 字段 **不会被读取**（未在 `internal/config/secrets.go` 的结构体中声明）
- 若希望后端 GitHub 集成使用 PAT，请写在：
  - `[github].token = \"...\"`
- Token 只建议保存在 `secrets.toml`，不要提交到版本库；如已泄露，建议立即在 GitHub 侧吊销并重置。


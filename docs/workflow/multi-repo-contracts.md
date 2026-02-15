# 多仓库（frontend/backend/contracts）协作约定

## 仓库划分（建议）

- `frontend`：前端代码与 UI
- `backend`：后端服务实现
- `contracts`：接口契约（proto）与生成规则（buf/插件等）

说明：

- `contracts` 是唯一接口真源；其它 repo 不应复制一份接口文档。
- 多 repo 的并行开发依赖 contracts 的版本引用来对齐。

## 协作总线（Outbox）

V1 约定：

- Outbox backend 由 `<outbox_repo>/workflow.toml` 的 `[outbox]` 段决定（GitHub/GitLab Issues 或本地 SQLite）。
- 多 repo 项目推荐把 Outbox 放在 `contracts` repo（集中接口/决策/证据）。
- 不使用 goclaw `task_id` 作为协作主线，避免 task 与 issue 双真源导致状态漂移。

## contracts（proto）为什么适合当真源

- proto 可作为跨语言契约，天然适配多 repo/多语言团队。
- proto 可以生成服务端/客户端代码，也可以通过网关映射到 HTTP。

HTTP 方案（不在本文件强制选型）：

- `grpc-gateway`：gRPC 转 REST/JSON
- `connectrpc`：面向 HTTP 的现代方案
- 其它方案可后续由架构师统一决定并落盘到 ADR

## 版本对齐规则（核心）

- 每次并行工作都必须引用同一个 contracts 版本，例如：
  - `contracts@<git-sha>`（最精确）
  - 或 `contracts@vX.Y.Z`（tag/发布版本）
- Frontend/Backend 的实现与测试都以该版本为基准。
- 如果需要改动接口：
  - 先在 contracts 提 PR（或 outbox 提变更请求）
  - 合并后再推进实现 repo 的适配

推荐流程图（Mermaid，突出 “ContractsRef 是锚点” 与 “contracts 优先” 的顺序）：

```mermaid
flowchart TD
  A[选择 ContractsRef<br/>contracts@sha|tag] --> B{需要修改接口契约?}

  B -->|否| Impl[并行实现<br/>frontend + backend + qa<br/>都引用同一 ContractsRef]
  B -->|是| CPR[contracts repo: PR]
  CPR --> AR[architect/approver: review + merge]
  AR --> A2[生成新的 ContractsRef<br/>contracts@new_sha]
  A2 --> Impl

  Impl --> INT[integrator: 拉齐版本并集成验收]
  INT --> D[state:done + evidence]
```

## 本地目录建议（减少工具约束冲突）

建议将多个 repo 都放在 goclaw 的 workspace 目录下，例如：

- `<workspace>\\frontend`
- `<workspace>\\backend`
- `<workspace>\\contracts`

原因：

- goclaw 的部分路径管理工具对 `repo_dir` 有“必须位于 workspace 内”的限制（例如 `agent/tools/agents_target.go` 的校验逻辑）。
- 统一放 workspace 能减少路径/权限相关的意外失败。

V1.1 补充（多环境一致性）：

- 建议把 outbox repo 作为“项目锚点目录”，并要求其它 repo 以兄弟目录方式存在。
- `workflow.toml` 的 `[repos]` 建议使用相对路径（以 `workflow.toml` 所在目录为基准解析）。
- 这样可以在不同机器上复用同一份 `workflow.toml`，无需本地覆盖文件。

## subagent 如何在多 repo 下工作

关键点：

- `sessions_spawn` 支持指定 `repo_dir`，因此不同 subagent 可以指向不同 repo。
- `repo_dir` 必须是已存在的目录（否则会直接报错）。
- 推荐每个 subagent 只在自己的 repo 内改动与提交，减少冲突。

建议的启动方式（语义示例）：

- Backend subagent：`repo_dir=<workspace>\\backend`，任务里包含 `contracts@...` 引用
- Frontend subagent：`repo_dir=<workspace>\\frontend`，任务里包含同一个 `contracts@...` 引用
- Contracts/Architect：`repo_dir=<workspace>\\contracts`

## 集成工作区（Integrator）

Integrator 可以使用一个独立目录（例如 `<workspace>\\integration\\run-<id>`）：

- checkout 指定版本的 frontend/backend/contracts
- 按 DoD 运行构建/测试/E2E
- 通过 outbox 反馈失败原因与责任归属

## Forge（GitHub/GitLab）能提供的“机制化一致性”

建议在 contracts repo 启用：

- `CODEOWNERS`：强制架构师 review proto/生成规则
- Branch protection：必须通过 `buf lint` / `buf breaking` / 生成检查
- Actions：在 PR 自动输出 breaking change 报告、生成产物差异
- Issue/PR 关联：实现 PR 必须链接到 contracts 版本或接口变更 PR


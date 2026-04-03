# Backend WorkItem Action CLI Surface Map

> 状态：部分实现
>
> 最后按代码核对：2026-04-03

本文整理当前 `ai-workflow` 后端的控制面入口与分层边界，用于支撑后续将系统内部控制节点逐步统一到 Cobra CLI。

## 1. 当前控制面概览

当前后端实际存在四类入口：

1. `cmd/ai-flow`
   进程级入口，当前已经切换到 Cobra，负责承载本地命令树。
2. HTTP API
   对外主控制面，Web 前端、人类操作、远程调用主要经过这里。
3. 内部 MCP
   面向 agent 的 stdio/SSE 工具面，适合执行期的上下文读取与内部控制。
4. builtin skill 脚本
   当前主要是包装层，很多脚本仍然通过 HTTP 回调系统。

这四类入口的真正问题不是“入口太多”，而是部分业务逻辑仍然散落在 transport 层，没有统一复用。

## 2. 命名原则

后续统一使用当前领域模型命名：

- `WorkItem`
- `Action`
- `Run`

不再把 `step` 作为新设计、新命令、新文档的主命名。

当前现状里，`step` 主要只剩持久化表名、旧脚本、旧文档中的历史残留。
HTTP 主路径、运行时环境变量、builtin skills 的对外主命名已经统一到 `action`。

## 3. 主要代码分层

### 3.1 进程入口层

- `cmd/ai-flow`
  Cobra 根命令，当前承接：
  - `version`
  - `server`
  - `executor`
  - `quality-gate`
  - `mcp-serve`
  - `orchestrate`
  - `runtime`
  - `profile`

### 3.2 命令实现层

- `internal/platform/appcmd`
  目前承载本地命令实现。

当前已经落地的命令实现除了基础运行命令，还包括：

- `orchestrate.go`
  任务编排控制面，已覆盖 create / follow-up / adopt-deliverable /
  assign-profile / reassign / decompose / escalate-thread / ceo.submit
- `runtime.go`
  当前至少承接 `ensure-execution-profiles`
- `profile.go`
  当前承接 runtime profile 的 list / get / create / set-base /
  add-skill / remove-skill / delete

建议定位：

- `appcmd`
  负责命令参数到 service 输入的翻译。
- `application/*`
  负责业务规则与数据写入。
- `adapters/http`
  负责 HTTP 参数解析、鉴权、错误映射。
- `adapters/mcp` / `appcmd/mcp_serve.go`
  负责 MCP tool 参数映射。

### 3.3 HTTP transport 层

- `internal/adapters/http/handler.go`
  全量路由注册中心。
- `internal/adapters/http/*.go`
  各业务 handler。

当前问题：

- 部分 handler 已经走 service 模式，例如：
  - `llmconfig.ControlService`
  - `sandbox.ControlService`
- 但另一些关键控制点仍然在 handler 中直接操作 store/bus，例如：
  - `step_signal.go`（文件名仍待后续整理，语义已是 action signal）

这会导致 CLI、HTTP、MCP 很容易各写一套。

### 3.4 应用服务层

- `internal/application/flow`
  DAG、执行、gate、signal 关联逻辑核心。
- `internal/application/proposalapp`
  proposal 审批与物化入口。
- `internal/application/initiativeapp`
  initiative 执行前批准与物化协调。
- `internal/application/probe`
  运行期探针与 side-channel。

这里已经是很多业务规则的真实归属层，但“控制命令型能力”还没有完全抽齐。

### 3.5 运行时与基础设施层

- `internal/runtime/agent`
  ACP session、thread pool、executor worker。
- `internal/platform/bootstrap`
  系统启动与依赖装配。
- `internal/platform/configruntime`
  runtime 配置热更新与内部 MCP 注入。
- `internal/adapters/store/sqlite`
  SQLite 存储实现。

## 4. 已识别的内部控制节点

以下能力适合进入统一 CLI 面：

### 4.1 action signal

当前来源：

- agent skill: `internal/skills/builtin/action-signal`
- HTTP handler: `internal/adapters/http/step_signal.go`
- MCP 内部实现: `internal/platform/appcmd/mcp_serve.go`

现状：

- skill 仍主要走 HTTP 回调。
- MCP 已经能直接写 `ActionSignal`。
- HTTP handler 里仍有业务逻辑。

目标：

- 统一成 `ActionSignalService`
- 由 Cobra / HTTP / MCP 复用

建议 CLI 形态：

- `ai-flow action signal complete`
- `ai-flow action signal need-help`
- `ai-flow action signal approve`
- `ai-flow action signal reject`
- `ai-flow action unblock`

### 4.2 action manage

当前来源：

- skill: `internal/skills/builtin/sys-action-manage`
- HTTP routes:
  - `POST /work-items/{workItemID}/actions`
  - `GET /work-items/{workItemID}/actions`
  - `GET /actions/{actionID}`
  - `PUT /actions/{actionID}`
  - `DELETE /actions/{actionID}`
  - `POST /work-items/{workItemID}/generate-actions`

目标：

- 统一成 `ActionManageService`

建议 CLI 形态：

- `ai-flow action create`
- `ai-flow action list`
- `ai-flow action get`
- `ai-flow action update`
- `ai-flow action delete`
- `ai-flow action generate`

### 4.3 thread task signal

该能力对应的 `task-signal` skill、HTTP handler 与 `threadtaskapp` 已从当前代码中移除。

因此这里不再作为现行 CLI 抽象目标；线程讨论后的计划与执行统一走
`proposal / initiative / work item` 主链。

### 4.4 runtime config

当前来源：

- `internal/adapters/llmconfig/service.go`
- `internal/adapters/sandbox/support.go`
- HTTP admin routes

这类能力已经具备良好的 service 形态，是最容易映射到 Cobra 的一组。

目标：

- `ai-flow runtime llm get|set`
- `ai-flow runtime sandbox get|set`

补充当前已实现能力：

- `ai-flow runtime ensure-execution-profiles`
- `ai-flow profile list|get|create|set-base|add-skill|remove-skill|delete`

## 5. 当前最值得优先抽象的共享服务

建议优先形成三类 command service：

1. `ActionSignalService`
   - decision
   - unblock
   - pending decision query
2. `ActionManageService`
   - CRUD
   - generate
3. `RuntimeConfigCommandService`
    - llm config inspect/update
    - sandbox inspect/update

原则：

- CLI 不调用本机 HTTP
- HTTP 不直接承担业务规则
- MCP 不单独复制 signal 写入逻辑

## 6. CLI 化的推荐迁移顺序

### 阶段 1

完成项：

- `cmd/ai-flow` 切换到 Cobra
- 保持现有命令兼容

状态：

- 已完成

### 阶段 2

完成项：

- 抽 `ActionSignalService`
- 让 `step_signal.go`（语义为 action signal）与 `mcp_serve.go` 共用
- 新增：
  - `ai-flow action signal complete`
  - `ai-flow action signal need-help`
  - `ai-flow action signal approve`
  - `ai-flow action signal reject`
  - `ai-flow action unblock`

### 阶段 3

完成项：

- 抽 `ActionManageService`
- 改造 `sys-action-manage` skill 脚本为 CLI wrapper

### 阶段 4

完成项：

- 评估其余 admin / probe / run / skill 管理能力进入 CLI 的边界
- 决定哪些保留 HTTP-only，哪些同时暴露为 CLI

## 7. 关键设计原则

1. 不追求 HTTP 全删除。
   HTTP 仍然是 Web 与远程操作的主入口。
2. 内部控制优先 CLI 本地直连。
   只要已有 `AI_WORKFLOW_DB_PATH` 或 runtime manager 上下文，就不应让内部脚本再回打本机 HTTP。
3. fallback 保留到迁移稳定。
   `AI_WORKFLOW_SIGNAL` / `AI_WORKFLOW_TASK_SIGNAL` 输出回退链路先保留，直到 CLI 路径完全稳定。
4. 先统一业务层，再扩命令树。
   否则 Cobra 只会把重复逻辑从 HTTP 复制到 CLI。
5. 新命令只使用 `action` 命名。
   不新增 `step` alias，不在新文档中继续扩散旧命名。

## 8. 下一步建议

下一阶段建议直接做：

1. `ActionSignalService`
2. `ai-flow action signal ...`
3. `ai-flow action unblock`
4. `action-signal` skill 脚本改为调用 CLI

这是最小、最关键、收益最高的一刀，因为它同时连接了：

- agent 执行闭环
- gate verdict
- artifact metadata 传递
- 人工 unblock / decision
- 后续所有内部控制节点 CLI 化的模式

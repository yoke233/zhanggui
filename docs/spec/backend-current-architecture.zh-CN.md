# 后端现状总览

> 状态：现行
>
> 最后按代码核对：2026-04-03
>
> 适用范围：本文只描述当前仓库中已经落地的后端实现，不描述未来规划。

## 一句话结论

当前后端主线已经稳定为：

`WorkItem 执行引擎 + Thread 协作域 + Proposal / Initiative 审批链 + Deliverable 输出层 + CEO / Orchestrate 控制面 + ACP 运行时`

如果要理解“现在代码怎么工作”，应以 `internal/core`、
`internal/application`、`internal/adapters`、`internal/platform`、
`internal/runtime`、`internal/threadctx` 为主，并结合 `internal/audit`、
`internal/usecase` 这类辅助包阅读，而不是旧的
`engine/secretary/web/github/plugins/git/tui` 分法。

## 运行入口

主入口见 `cmd/ai-flow/main.go`，当前命令面为：

- `ai-flow server`
- `ai-flow executor`
- `ai-flow quality-gate`
- `ai-flow mcp-serve`
- `ai-flow orchestrate`
- `ai-flow runtime`
- `ai-flow profile`
- `ai-flow version`

这意味着项目当前不是单一 HTTP 服务，而是带有主服务、执行器、
质量门、MCP 服务以及本地控制面命令树的多入口运行体。

补充现状：

- `orchestrate` 已承接任务编排控制面，包含 `task create/follow-up/adopt-deliverable/assign-profile/reassign/decompose/escalate-thread`
- `orchestrate ceo submit` 已承接 CEO 单入口需求提交
- `runtime ensure-execution-profiles` 已承接运行期执行档案初始化
- `profile` 已承接 runtime profile 的增删改查与 skill 绑定

## 当前后端分层

### 1. `cmd/*`

负责 CLI 与不同运行模式入口。

### 2. `internal/core`

定义统一领域模型与存储接口，是当前主领域层。

关键对象包括：

- `WorkItem`、`Action`、`Run`、`Deliverable`
- `Thread`、`ThreadMessage`、`ThreadMember`
- `ThreadProposal`、`Initiative`
- `ResourceSpace`、`Resource`、`ActionIODecl`
- `Event`、`Notification`、`Inspection`

### 3. `internal/application/*`

实现用例层服务与编排逻辑。

当前主应用服务包括：

- `flow`：WorkItem 执行引擎、调度器、gate、恢复、workspace 编排
- `workitemapp`：WorkItem CRUD 与运行入口
- `threadapp`：Thread CRUD、context ref、work item linking、workspace sync、从 Thread 创建 WorkItem
- `chat`：direct chat / lead session 入口
- `orchestrateapp`：任务创建、follow-up、profile 指派/改派、deliverable 采纳、升级为协作 Thread
- `ceoapp`：单入口 requirement intake，根据分析结果分流到 direct execution 或 discussion thread
- `proposalapp`、`initiativeapp`：计划审批链与执行前物化
- `requirementapp`：需求分析、建议线程、创建 Thread
- `agent`：运行期 registry / profile 配置读取与归一化
- `planning`：LLM 规划并 materialize 为 Action
- `probe`：运行探针
- `inspection`：巡检与自演进检查
- `runtime`：会话获取、运行期 session manager 抽象

### 4. `internal/adapters/*`

提供外部适配层，当前主要包括：

- `http`：REST / WebSocket 接口
- `store/sqlite`：主持久化实现
- `agent/acpclient`、`agent/acp`：ACP 协议适配
- `executor`：ACP + builtin executor 组合执行器
- `workspace`：本地目录、Git 工作区、clone 逻辑
- `resource`：文件与外部资源
- `sandbox`：Litebox / Docker / bwrap 等沙箱能力
- `scm`：GitHub / Codeup

### 5. `internal/platform/*`

负责启动装配、配置加载、runtime config manager、server/executor
命令入口支撑。

关键装配入口见：

- `internal/platform/bootstrap/bootstrap.go`
- `internal/platform/bootstrap/bootstrap_api.go`
- `internal/platform/bootstrap/bootstrap_engine.go`
- `internal/platform/appcmd/server.go`

### 6. `internal/runtime/agent`

负责 agent 运行时，尤其是：

- ACP 会话池
- 线程内 agent session 生命周期
- thread boot prompt 与上下文注入

### 7. `internal/threadctx`

负责线程工作区目录结构与 `.context.json` 维护。

### 8. `internal/audit` / `internal/usecase`

当前仓库还包含：

- `internal/audit`：tool call / 运行审计能力
- `internal/usecase`：局部用例接口与兼容承接

它们不是新的主领域分层，但属于当前代码树中的实际组成部分。

## 核心领域事实

### WorkItem 是统一业务主轴

`internal/core/workitem.go` 中的 `WorkItem` 已经把“计划意图”和
“执行上下文”合并为统一对象。

当前主生命周期包括：

- `pending_execution`
- `in_execution`
- `pending_review`
- `needs_rework`
- `escalated`
- `completed`
- `cancelled`

补充边界：

- `open` / `accepted` / `queued` 仍然保留为迁移兼容状态
- `running` / `blocked` / `failed` / `done` / `closed` 当前主要作为 legacy alias 读取

当前主执行链路是：

`WorkItem -> Action -> Run`

### Deliverable 是统一输出对象

`internal/core/deliverable.go` 中的 `Deliverable` 已经成为现行输出模型。

当前事实包括：

- `Run` 可以沉淀 deliverable
- `Thread` 消息可以归档为 deliverable
- `WorkItem` 可以绑定 `final_deliverable_id`
- HTTP 已公开 `work-item deliverables`、`thread deliverables`、`artifact/deliverable` 查询与 `final deliverable` 采纳入口

因此当前系统已经不只是“run 产出 artifact”，而是进入了统一 deliverable 模型。

### Action 是领域名，旧 Step 只剩历史残留

当前内部核心模型、HTTP 主路径与前端契约都已经统一使用 `Action`。
`Step` 主要只剩持久化层、旧测试说明和历史文档中的残留描述。

因此当前约束应理解为：

- 领域模型主名：`Action`
- Public/API 主名：`Action`
- `Step` 不是新的主领域对象，只是历史残留词

### Thread 是一等协作模型

Thread 不是 Chat 的附属视图，而是当前后端的一等实体。

Thread 相关核心对象包括：

- `Thread`
- `ThreadMessage`
- `ThreadMember`
- `ThreadContextRef`
- `ThreadAttachment`
- `ThreadWorkItemLink`
- `ThreadWorkspaceContext`

Thread 当前已拥有独立 REST、WebSocket、存储与运行时链路。

### Proposal / Initiative 是当前计划审批主链

当前 thread 讨论后的结构化计划，不再落到独立的 `ThreadTask DAG`。

现行主链是：

- `Thread` 负责讨论与收敛
- `Proposal` 负责计划提交、驳回、返修、审批
- `Initiative` 负责执行前批准与物化 work item 关系组
- `WorkItem` 负责实际调度与执行

因此任何仍把 `task group / thread task` 当作现行线程协作主链的描述，
都已经落后于当前代码。

### 统一资源模型已进入现状实现

当前统一资源主线包括：

- `ResourceSpace`：项目级资源空间
- `Resource`：具体文件或对象
- `ActionIODecl`：Action 输入输出声明

SQLite 中已经存在统一资源迁移逻辑，说明这不是未来设计，而是
正在承接旧数据结构的现状能力。

## 当前主链路

启动装配由 `bootstrap.Build()` 串起以下核心组件：

- SQLite store
- EventBus 与持久化
- runtime config manager
- flow engine
- scheduler
- lead chat agent
- thread agent runtime
- API handler

可以把后端当前数据流简化理解为：

```text
HTTP / CLI
  -> application service / flow engine
  -> core.Store interfaces
  -> adapters/store/sqlite
  -> EventBus + event log
  -> ACP agent / builtin executor / SCM / sandbox / workspace
```

## HTTP 与 Public Surface

HTTP 总注册位于 `internal/adapters/http/handler.go`。

当前已落地的 public surface 至少包括：

- projects
- resource spaces
- action io decls
- work items / pending inbox / deliverables
- actions / runs / events / artifacts
- analytics / usage
- templates
- action signals / pending decisions
- threads / messages / participants / agents / deliverables
- proposals / initiatives
- thread work item linking
- thread context refs
- thread attachments / files
- requirements / ceo submit
- chat
- notifications
- themes
- inspections
- admin controls

因此任何把当前系统描述成“只有 workitem/action/run 单线工作流”的文档
都已经落后于代码。

## ACP 与 builtin executor

ACP 当前已经是执行与协作的主协议层。

当前实现中：

- WorkItem/Action 执行默认可走 ACP executor
- Thread agent runtime 也基于 ACP session pool
- 平台内建执行器只拦截少数动作

builtin executor 当前覆盖的典型动作包括：

- `git_commit_push`
- `scm_open_pr` / `github_open_pr`
- `self_upgrade`

因此执行层并不是“全部交给 agent 自由发挥”，而是
“ACP 为主 + builtin executor 补平台能力”。

## 线程工作区事实

`internal/threadctx/workspace.go` 表明每个 thread 都维护自己的
专属工作区目录。

当前关键目录/文件包括：

- `threads/{threadID}/projects`
- `threads/{threadID}/attachments`
- `.context.json`

`.context.json` 当前会记录至少以下信息：

- mounts
- attachments
- members
- workspace root

因此 thread workspace 不是纯逻辑概念，而是明确的文件系统事实。

## 现状与兼容边界

当前仍保留以下兼容残留：

- SQLite 主表仍多为 `issues` / `steps` / `executions`
- 这些属于持久化兼容残留，不再代表对外主命名
- 持久化与 request struct 中仍有少量旧 `issue` / `step` 命名

这些残留不代表主设计方向，只表示当前为了兼容而保留的实现细节。

## 推荐搭配阅读

若要继续深入，建议按这个顺序看：

1. `execution-context-building.zh-CN.md`
2. `thread-agent-runtime.zh-CN.md`
3. `thread-workspace-context.zh-CN.md`
4. `thread-workitem-linking.zh-CN.md`
5. `naming-transition-thread-workitem.zh-CN.md`

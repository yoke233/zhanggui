# docs/spec 状态索引

> 最后按代码核对：2026-03-17

本目录同时包含 4 类文档：

- `现行`：可按当前实现阅读
- `部分实现`：一部分已落地，另一部分仍是目标设计
- `草案`：未来方向，不代表当前代码行为
- `历史`：迁移记录或被现状取代的旧方案

## 当前代码基线

- 当前系统已经形成稳定的多入口运行体：`ai-flow server`、`executor`、`quality-gate`、`mcp-serve`、`version`
- 后端真实分层以 `internal/core`、`internal/application`、`internal/adapters`、`internal/platform`、`internal/runtime`、`internal/threadctx` 为主；`internal/legacy` 属于历史兼容层
- 主工作对象的对外 Public REST 已切到 `/api/work-items/*`
- 前端主路由已切到 `/work-items`，旧 `/issues/*`、`/flows/*` 仅保留页面 redirect
- 前端工作台已经分成 3 个能力域：通用工作台、`/monitoring/*` 监控域、`/runtime/*` 运行时域
- `ChatSession` 与 `Thread` 已明确分离，当前是两套并行交互入口：`ChatSession` 用于 direct chat，`Thread` 用于多人/多 agent 协作
- `Thread` 已独立落地：REST、WebSocket、消息、参与者、agent 邀请、WorkItem 关联都已存在
- Thread 当前公开能力已经包含：`context-refs`、附件上传/下载、workspace/project/attachment 文件检索、task groups / thread-task-dag
- Thread agent 当前使用统一的 `thread_members` 模型；前端 `ThreadParticipant` / `ThreadAgentSession` 只是 alias
- `thread.send` 已支持 `target_agent_id`；默认路由模式是 `mention_only`，并支持 `broadcast` / `auto`
- `ChatSession` 可通过 `POST /chat/sessions/{sessionID}/crystallize-thread` 结晶为 `Thread`，并可选同步创建 `WorkItem`
- 技能系统当前由 builtin `step-signal` + 运行期临时 `step-context` 组成；实现类型名仍保留 `ActionContextBuilder`
- ThreadTask DAG 已落地：后端已有 `task-groups` / `thread-tasks` 路由、存储、应用服务与前端消费面
- 统一资源模型已进入现状实现：`ResourceSpace`、`Resource`、`ActionIODecl` 与 SQLite migration 已存在
- ACP 已经是当前执行与线程协作的主协议层；builtin executor 只接管少数平台内建动作
- 当前前端已落地的现行产品面不仅包括 WorkItem / Thread，也包括 Analytics、Usage、Inspection、Scheduled Tasks、Agents、Skills、Templates、Sandbox、Feature Manifest、Git Tags
- Tauri 桌面端已实现；`desktop_bootstrap` 会返回 `api_v1_base_url` / `api_base_url` / `ws_base_url`，但当前主工作台实际统一使用 `api_base_url`
- 持久化层仍保留兼容命名：主表仍是 `issues` / `steps` / `executions`，部分 handler / request struct 也仍沿用 `issue` 命名

## 推荐阅读顺序

如果你要理解“现在系统怎么工作”，优先看：

1. `backend-current-architecture.zh-CN.md`
2. `web-product-surface.zh-CN.md`
3. `execution-context-building.zh-CN.md`
4. `thread-agent-runtime.zh-CN.md`
5. `thread-workitem-linking.zh-CN.md`
6. `thread-task-dag.zh-CN.md`
7. `tauri-desktop.md`

如果你要理解“现状与未来规划的边界”，再看：

1. `naming-transition-thread-workitem.zh-CN.md`
2. `step-context-progressive-loading.zh-CN.md`
3. `thread-collaboration-to-dag-plan.zh-CN.md`
4. `ai-company-domain-model.zh-CN.md`
5. `lead-chat-dynamic-skills.zh-CN.md`
6. `spec-unified-resource-model.zh-CN.md`

## 当前文档状态

### 现行

- `backend-current-architecture.zh-CN.md`
- `web-product-surface.zh-CN.md`
- `execution-context-building.zh-CN.md`
- `gate-human-intervention.zh-CN.md`
- `gate-merge-failure-handling.zh-CN.md`
- `tauri-desktop.md`

### 部分实现

- `thread-agent-runtime.zh-CN.md`
- `thread-task-dag.zh-CN.md`
- `thread-workitem-linking.zh-CN.md`
- `naming-transition-thread-workitem.zh-CN.md`
- `spec-unified-resource-model.zh-CN.md`
- `step-context-progressive-loading.zh-CN.md`
- `thread-summary-workitem-mvp.zh-CN.md`
- `thread-workspace-context.zh-CN.md`

### 草案

- `ai-company-domain-model.zh-CN.md`
- `lead-chat-dynamic-skills.zh-CN.md`
- `activity-journal-consolidation.zh-CN.md`
- `thread-collaboration-to-dag-plan.zh-CN.md`
- `spec-context-memory.md`

### 历史

- `design-issue-centric-model.md`
- `complete-step-mcp.md`
- `thread-workitem-track.zh-CN.md`
- `thread-workitem-migration-guide.zh-CN.md`
- `thread-message-delivery-deferred.zh-CN.md`

## 维护约定

- 文档顶部必须写明 `状态` 与 `最后按代码核对`
- 任何“未来方案”都不能写成当前时态
- 当 public surface 已经变化时，优先更新 `README` 中的基线说明，再回补各专题文档

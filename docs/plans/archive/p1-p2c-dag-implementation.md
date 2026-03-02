# P2 DAG 任务拆解计划

> 生成时间：2026-03-01
> 修订时间：2026-03-01（综合两轮审查修正）
> 范围：P2-Foundation（插件接口+API 基础设施）+ P2a（Secretary+DAG）+ P2b（审核面板）+ P2c（Workbench UI）
> 前置：P0 ✅ + P1 ✅ 已验收通过（见 `2026-02-28-p1-implementation.md`）
> 不含：P3（GitHub 集成）、P4（高级功能）
> 规格依据：`spec\spec-secretary-layer.md`、`spec\spec-api-config.md`、`2026-03-01-dag-conversion-minimal-rules.md`

## Context

P0（Pipeline Engine、Agent Driver、Store、CLI/TUI、EventBus）和 P1（多项目调度器、配置驱动工厂、Registry、三级配置合并、人工动作、崩溃恢复、Reactions V1）均已完成并验收通过。

本文档将 P2 全阶段（Foundation + P2a + P2b + P2c）拆解为 **33 个**可并行执行的 DAG 任务。任务拆解本身也是 Secretary Agent 任务拆解能力的实战验证。

### 与已完成 P1 的关系

P1 实现了调度器、配置、工厂等核心基础设施。本计划的 **P2-Foundation** 部分是 P2 所需的新增基础设施（插件接口、REST API、WebSocket），与已完成 P1 无重叠。具体区别：

| 已完成 P1 | P2-Foundation（本计划） |
|-----------|----------------------|
| `internal/engine/scheduler.go` — FIFO 调度器 | fnd-7~9 — REST API + WebSocket 服务 |
| `internal/plugins/factory/factory.go` — 工厂 | fnd-6 — 扩展现有工厂注册新插件 |
| `internal/core/registry.go` — 插件注册表 | fnd-1 — 定义 4 个新插件接口 |
| `internal/config/` — 三级配置 | fnd-2~5 — 4 个插件的具体实现 |

### 外部依赖清单

本计划引入的新外部依赖（需 `go get` 或 `npm install`）：

| 依赖 | 用途 | 涉及任务 |
|------|------|---------|
| `github.com/go-chi/chi/v5` | REST API 路由 | fnd-7 |
| `nhooyr.io/websocket` | WebSocket 服务 | fnd-9 |
| `react` + `react-dom` | 前端框架 | p2c-1 |
| `vite` + `@vitejs/plugin-react` | 前端构建 | p2c-1 |
| `tailwindcss` | CSS 框架 | p2c-1 |
| `zustand` | 状态管理 | p2c-2 |
| `@xyflow/react` (React Flow) | DAG 可视化 | p2c-4 |

## DAG 总览（33 个任务，7 波并行）

```
Wave 1 ─ 无依赖，立即启动（5 并行）
│
├─ fnd-1  定义新插件接口 (ReviewGate/Tracker/SCM/Notifier)
├─ fnd-7  REST API 服务骨架 (chi router + middleware)
├─ p2a-1  Secretary Layer 领域实体 (ChatSession/TaskPlan/TaskItem)
├─ p2b-2  审核 Prompt 模板 (4 个 .tmpl)
└─ p2c-1  React 前端项目初始化 (Vite+TS+Tailwind)

Wave 2 ─ 基础接口就绪后（12 并行）
│
├─ fnd-2  review-local 插件
├─ fnd-3  tracker-local 插件
├─ fnd-4  local-git SCM 插件
├─ fnd-5  desktop notifier 插件
├─ fnd-8  REST API handlers (Project + Pipeline)
├─ fnd-9  WebSocket 服务
├─ p2a-2  Store 接口扩展
├─ p2a-3  SQLite 新表迁移
├─ p2a-5  DAG 数据结构 + 校验
├─ p2a-6  Secretary Agent 驱动
├─ p2b-1  ReviewRecord 实体 + Store + DB
└─ p2c-2  Zustand + API/WS 客户端

Wave 3 ─ 实现层（6 并行）
│
├─ fnd-6  BootstrapSet 扩展（注册全部新插件到现有工厂）
├─ p2a-4  SQLite Store 新实体实现
├─ p2c-3  Chat View（Mock Data 开发）
├─ p2c-4  Plan View (React Flow，Mock Data 开发)
├─ p2c-5  Board View (Kanban，Mock Data 开发)
└─ p2c-6  Pipeline View（Mock Data 开发）

Wave 4 ─ 核心调度器（3 并行）
│
├─ fnd-10 CLI server 命令 + 集成
├─ p2a-7  DAG Scheduler（调度/事件/失败处理）
└─ p2b-3  ReviewPanel 编排引擎（含强门禁状态机）

Wave 5 ─ 管理层（2 并行）
│
├─ p2a-8  TaskPlan 管理层
└─ p2b-4  review-ai-panel 插件

Wave 6 ─ 端点与集成（4 并行）
│
├─ p2a-9  REST API (chat + plans)
├─ p2a-10 WebSocket Secretary 事件
├─ p2a-11 执行期文件沉淀
└─ p2b-5  ReviewPanel 接入 TaskPlan 生命周期

Wave 7 ─ 最终集成
│
└─ p2c-7  前端接入真实 API + embed.FS 打包
```

## 完整任务清单

### Phase P2-Foundation — 插件接口 + API 基础设施（10 个任务）

> 注：这些任务扩展已完成的 P1 基础设施，为 P2a/P2b/P2c 提供前置能力。

| ID | 标题 | description（含验收标准） | depends_on | 模板 | 标签 | 规模 | 测试 |
|----|------|--------------------------|-----------|------|------|------|------|
| fnd-1 | 定义新插件接口 | 在 `plugin.go` 注册 ReviewGate 槽位。创建 `ReviewGate`/`Tracker`/`SCM`/`Notifier` 四个接口文件，签名对齐 `spec\spec-secretary-layer.md` Section 七。**验收**：`go build ./...` 通过，接口可被 mock 实现。 | [] | quick | backend, core | S | 编译验证 |
| fnd-2 | review-local 插件 | 实现 `ReviewGate` 的本地人工审批版本：Submit 写入 DB 等待人工，Check 查询状态，Cancel 标记取消。**验收**：单测覆盖 Submit/Check/Cancel 正常流和边界。 | [fnd-1] | quick | backend, plugin | S | 单测 |
| fnd-3 | tracker-local 插件 | 实现 `Tracker` 的 no-op 版本：所有方法返回成功，不同步外部系统。**验收**：单测验证所有接口方法不报错。 | [fnd-1] | quick | backend, plugin | S | 单测 |
| fnd-4 | local-git SCM 插件 | 实现 `SCM`：复用现有 `internal/git/` 包的 branch/commit/push 能力，封装为插件接口。**验收**：单测验证 CreateBranch/Commit 在临时 git repo 中正确执行。 | [fnd-1] | standard | backend, plugin | M | 单测 |
| fnd-5 | desktop notifier 插件 | 实现 `Notifier`：使用 `os/exec` 调用系统通知（Windows toast / macOS osascript）。**验收**：单测验证 Notify 方法不 panic；集成测试在 CI 中跳过。 | [fnd-1] | quick | backend, plugin | S | 单测 |
| fnd-6 | BootstrapSet 扩展 | **扩展现有** `factory.go` 中的 `BootstrapSet` struct，添加 `ReviewGate`、`Tracker`、`SCM`、`Notifier` 字段。在 `newDefaultRegistry()` 中注册 fnd-2~5 的插件。`buildWithRegistry()` 增加新插件构建逻辑。**验收**：`BuildFromConfig` 返回包含所有新插件的 BootstrapSet，单测覆盖。 | [fnd-2, fnd-3, fnd-4, fnd-5] | standard | backend, config | M | 单测 |
| fnd-7 | REST API 服务骨架 | 创建 `internal/web/server.go`（chi 路由、Start/Shutdown）、`middleware.go`（Bearer auth、CORS、logging、panic recovery）、`handlers_system.go`（GET /health, GET /api/v1/stats）。**验收**：`go test` 覆盖 /health 返回 200、无 token 返回 401。 | [] | standard | backend, api | M | 单测 |
| fnd-8 | Project + Pipeline API handlers | `handlers_project.go`（CRUD）+ `handlers_pipeline.go`（list/get/create）。对齐 `spec\spec-api-config.md` Section I 端点。**验收**：单测覆盖 JSON 序列化、路由参数解析、Store mock 验证。 | [fnd-7] | standard | backend, api | M | 单测 |
| fnd-9 | WebSocket 服务 | `internal/web/ws.go`：WebSocket 升级、连接管理（hub 模式）、EventBus 订阅转发。对齐 `spec\spec-api-config.md` Section II。**验收**：单测验证连接/断开/消息广播。 | [fnd-7] | standard | backend, websocket | M | 单测 |
| fnd-10 | CLI server 命令 + 全局集成 | 在 `cmd/ai-flow/commands.go` 添加 `server` 子命令：启动 HTTP+WS 服务器 + 调度器。**验收**：`ai-flow server --port 8080` 启动后 `/health` 返回 200，Ctrl+C 优雅关闭。 | [fnd-6, fnd-8, fnd-9] | quick | backend, integration | S | 集成测试 |

### Phase P2a — Secretary Agent + DAG Scheduler（11 个任务）

| ID | 标题 | description（含验收标准） | depends_on | 模板 | 标签 | 规模 | 测试 |
|----|------|--------------------------|-----------|------|------|------|------|
| p2a-1 | Secretary Layer 领域实体定义 | 创建 `internal/core/chat.go`（ChatSession/ChatMessage）和 `internal/core/taskplan.go`（TaskPlan/TaskItem/TaskPlanStatus/TaskItemStatus/FailurePolicy + ID 生成函数）。所有字段严格对齐 `spec\spec-secretary-layer.md` Section 二，TaskItem.Description 为必填字段。新增 `WaitReason` 字段（`feedback_required` / `final_approval`）。**验收**：类型编译通过，状态枚举覆盖 spec 中全部值。 | [] | quick | backend, core | S | 编译验证 |
| p2a-2 | Store 接口扩展 | 在 `internal/core/store.go` 添加 ChatSession/TaskPlan/TaskItem 的 CRUD 方法。**验收**：接口编译通过，方法签名与领域实体匹配。 | [p2a-1] | quick | backend, interfaces | S | 编译验证 |
| p2a-3 | SQLite 新表迁移 | 在 `migrations.go` 添加 `chat_sessions`（messages 为 JSON 字段）、`task_plans`、`task_items` 三张表 DDL + 索引，对齐 `spec\spec-api-config.md` Section IV。`task_items.description` 为 NOT NULL。**验收**：`applyMigrations` 执行无错，`hasColumn` 验证新字段存在。 | [p2a-1] | quick | backend, database | S | 单测 |
| p2a-4 | SQLite Store 新实体实现 | 在 `store-sqlite/` 中实现 p2a-2 定义的全部 Store 方法。**验收**：单测覆盖 CRUD 正常路径 + 不存在记录返回 nil/empty。 | [p2a-2, p2a-3] | standard | backend, database | M | 单测 |
| p2a-5 | DAG 数据结构 + 环检测 | 创建 `internal/secretary/dag.go`：DAG struct、Build()、Validate()（Kahn 算法检测环 + 缺失引用 + 自依赖，错误码 `DAG_CYCLE_DETECTED`/`DAG_MISSING_NODE`/`DAG_SELF_DEPENDENCY`，对齐 `2026-03-01-dag-conversion-minimal-rules.md` Section 4）、ReadyNodes()、TransitiveReduce()（冗余边约简，对齐 rules Section 5）。**验收**：单测覆盖正常图、有环、孤立引用、自依赖、空图、冗余边约简共 8 个边界用例。 | [p2a-1] | standard | backend, core | M | 全覆盖单测 |
| p2a-6 | Secretary Agent 驱动 | 创建 `internal/secretary/agent.go`：复用 AgentPlugin（Claude Driver）构造 secretary prompt，解析 JSON 输出为 TaskPlan。Prompt 模板 `configs/prompts/secretary.tmpl` 对齐 `spec\spec-secretary-layer.md` Section 一。重生成 prompt 须包含四段输入（对齐 rules Section 10.3）。**验收**：单测用 mock AgentPlugin 验证 prompt 构造和 JSON 解析。 | [p2a-1] | standard | backend, agent | M | 单测 |
| p2a-7 | DAG Scheduler | 创建 `internal/secretary/scheduler.go`：DepScheduler struct。StartPlan() 构建 DAG → 校验 → 约简冗余边 → 计算入度 → dispatch ready。dispatchTask() 获取信号量 → 创建 Pipeline → Executor.Run。EventBus 监听 pipeline_done/pipeline_failed。失败策略 block/skip/human（skip 含 hard/weak 依赖判定，对齐 rules Section 11-12）。崩溃恢复：扫描 executing plans → 重建 DAG → 恢复状态。**验收**：单测覆盖正常推进、三种失败策略、崩溃恢复重建。 | [p2a-4, p2a-5, p2a-6] | standard | backend, core | **L** | 全覆盖单测 |
| p2a-8 | TaskPlan 管理层 | 创建 `internal/secretary/manager.go`：编排 TaskPlan 完整生命周期（draft → reviewing → approved/waiting_human → executing → done/failed）。协调 Secretary Agent + ReviewPanel + DAG Scheduler 的调用顺序。**验收**：单测用 mock 组件验证状态流转路径覆盖 spec 状态机。 | [p2a-7] | standard | backend, core | M | 单测 |
| p2a-9 | REST API (chat + plans + tasks) | `handlers_chat.go`（POST /chat, GET /chat/:sid）、`handlers_plan.go`（GET/POST /plans, POST /plans/:id/review, POST /plans/:id/action）、`handlers_task.go`（POST /tasks/:tid/action）。端点设计对齐 `spec\spec-api-config.md` Section I。reject 动作须验证两段式必填反馈。**验收**：单测覆盖请求/响应 JSON 格式、必填字段校验。 | [p2a-8, fnd-8] | standard | backend, api | M | 单测 |
| p2a-10 | WebSocket Secretary 事件 | 在 EventBus 和 WS hub 中添加 Secretary Layer 事件类型（`plan_created`/`plan_reviewing`/`plan_approved`/`plan_waiting_human`/`task_ready`/`task_running`/`task_done`/`task_failed`/`plan_done`/`secretary_thinking`），对齐 `spec\spec-pipeline-engine.md` Section VII。**验收**：单测验证事件类型注册和 WS 广播。 | [fnd-9, p2a-7] | quick | backend, websocket | S | 单测 |
| p2a-11 | 执行期文件沉淀 | 在 implement 阶段 prompt 模板中注入 `.ai-workflow/task_plan.md`/`progress.md`/`findings.md` 维护指令，对齐 `spec\spec-secretary-layer.md` Section 五。**验收**：用 mock executor 验证 prompt 包含文件维护指令。 | [p2a-8] | standard | backend, agent | M | 单测 |

### Phase P2b — Multi-Agent 审核面板（5 个任务）

| ID | 标题 | description（含验收标准） | depends_on | 模板 | 标签 | 规模 | 测试 |
|----|------|--------------------------|-----------|------|------|------|------|
| p2b-1 | ReviewRecord 实体 + Store + DB 表 | 创建 `ReviewRecord` struct（Round/Verdicts/Decision/RevisedPlan）、扩展 Store 接口、添加 `review_records` 表 DDL。**验收**：单测验证 ReviewRecord CRUD。 | [p2a-1] | standard | backend, database | M | 单测 |
| p2b-2 | 审核 Prompt 模板 | 创建 `configs/prompts/review_completeness.tmpl`、`review_dependency.tmpl`、`review_feasibility.tmpl`、`review_aggregator.tmpl`。模板变量对齐 `spec\spec-secretary-layer.md` Section 三。**验收**：Go `template.ParseFiles` 解析无错，变量占位完整。 | [] | quick | backend, prompts | M | 模板解析测试 |
| p2b-3 | ReviewPanel 编排引擎 | 创建 `internal/secretary/review.go`：ReviewPanel struct。**强门禁状态机**（对齐 `2026-03-01-dag-conversion-minimal-rules.md`）：①AI review 强制门禁，`max_rounds=2`；②3 Reviewer 并行调用（goroutine + WaitGroup）→ Aggregator；③Aggregator 决策 approve → Plan 进入 `waiting_human`+`wait_reason=final_approval`（人工最终确认）；④Aggregator 决策 fix → 替换 TaskPlan 重新审核（消耗一轮）；⑤超过 2 轮未通过 → Plan 进入 `waiting_human`+`wait_reason=feedback_required`；⑥人工驳回须提交两段式反馈（问题类型+具体说明），自动触发 Secretary 重生成新版本并重新进入 AI review；⑦人工通过后全部任务立即进入 DAG 调度队列。每轮结果写入 `review_records` 表。**验收**：单测覆盖 approve/fix/escalate 三条路径、max_rounds 超限、人工反馈触发重生成、人工最终确认后进入调度。 | [p2b-1, p2b-2, p2a-6] | standard | backend, core | **L** | 全覆盖单测 |
| p2b-4 | review-ai-panel 插件 | 将 ReviewPanel 包装为 `ReviewGate` 实现：Submit → 启动审核循环，Check → 查询当前状态，Cancel → 中止审核。**验收**：单测验证 ReviewGate 接口契约。 | [p2b-3, fnd-1] | standard | backend, plugin | M | 单测 |
| p2b-5 | ReviewPanel 接入 TaskPlan 生命周期 | 将 review-ai-panel 插件接入 `p2a-8 TaskPlan 管理层`：draft → 自动触发 ReviewGate.Submit → 审核通过 + 人工确认 → 触发 DAG Scheduler.StartPlan。状态流转须通过 `waiting_human` 中间态（final_approval）。**验收**：集成测试覆盖完整流程 draft → reviewing → waiting_human(final_approval) → executing。 | [p2b-4, p2a-8] | standard | backend, core | M | 集成测试 |

### Phase P2c — Workbench Web UI（7 个任务）

> 注：Wave 3 的 p2c-3~6 在后端 API（Wave 6）就绪前使用 **Mock Data** 开发。Wave 7 的 p2c-7 负责接入真实 API 并最终打包。

| ID | 标题 | description（含验收标准） | depends_on | 模板 | 标签 | 规模 | 测试 |
|----|------|--------------------------|-----------|------|------|------|------|
| p2c-1 | React 项目初始化 | 在 `web/` 目录初始化 Vite + React 18 + TypeScript + Tailwind CSS 项目。配置 proxy 到后端 API。**验收**：`npm run dev` 启动无错，显示默认页面。 | [] | quick | frontend, setup | S | 手工验证 |
| p2c-2 | Zustand + API/WS Client | 创建 Zustand stores（projects/pipelines/chat/plans）。API client 封装 fetch + Bearer token。WebSocket client 封装连接/重连/消息分发。**验收**：单元测试覆盖 store actions、API client mock 调用。 | [p2c-1] | standard | frontend, state | M | 单测 |
| p2c-3 | Chat View | 对话历史区域（Markdown 渲染）、输入框（多行）、「生成任务清单」按钮。**Mock 阶段**：使用硬编码对话数据开发 UI。**验收**：组件渲染无报错，输入交互正常。 | [p2c-2] | standard | frontend, ui | M | 组件测试 |
| p2c-4 | Plan View (React Flow DAG) | React Flow 渲染任务依赖图（节点=TaskItem 按状态着色，边=依赖关系）。审核面板（Reviewer 状态/评分/决策）。操作按钮（通过/驳回/编辑）。**Mock 阶段**：使用示例 DAG 数据。**验收**：图渲染正确，节点可点击展开详情。 | [p2c-2] | standard | frontend, ui | **L** | 组件测试 |
| p2c-5 | Board View (看板) | 四列看板（Pending/Ready/Running/Done）、任务卡片（标题/标签/进度）、右键菜单（retry/skip/abort）。**Mock 阶段**：使用示例任务数据。**验收**：卡片拖拽交互正常，列过滤正常。 | [p2c-2] | standard | frontend, ui | M | 组件测试 |
| p2c-6 | Pipeline View | 阶段进度条、Agent 输出流显示区、Checkpoint 列表、人工操作按钮。**Mock 阶段**：使用示例 Pipeline 数据。**验收**：进度条渲染正确，操作按钮可点击。 | [p2c-2] | standard | frontend, ui | M | 组件测试 |
| p2c-7 | 前端接入真实 API + embed.FS 打包 | 将 p2c-3~6 的 Mock Data 替换为真实 API 调用。WebSocket 接入真实事件流。Go 端使用 `embed.FS` 嵌入前端构建产物。**验收**：`ai-flow server` 启动后浏览器可访问 Chat/Plan/Board/Pipeline 四个视图，数据来自后端。 | [p2c-3, p2c-4, p2c-5, p2c-6, p2a-9] | quick | integration | S | 端到端手工验证 |

## 关键路径

```
p2a-1 → p2a-2+p2a-3 → p2a-4 → p2a-7 → p2a-8 → p2b-5
  S        S+S          M        L        M        M
  1h       1h           3h       6h       3h       3h  = ~17h（串行瓶颈）
```

最长链路经过 **p2a-7 (DAG Scheduler)** — 整个计划最复杂的单体任务。

次关键路径（审核门禁）：
```
p2b-2 → p2b-3 → p2b-4 → p2b-5
  M        L        M        M
  2h       6h       3h       3h  = ~14h
```

## 并行度统计

| 波次 | 并行任务数 | 预估耗时 |
|------|----------|---------|
| Wave 1 | 5 | 2-3h |
| Wave 2 | 12 | 3-4h |
| Wave 3 | 6 | 4-6h |
| Wave 4 | 3 | 6-8h |
| Wave 5 | 2 | 3-4h |
| Wave 6 | 4 | 3-4h |
| Wave 7 | 1 | 1h |

**串行执行**: ~80-100h | **最大并行 (5 workers)**: ~22-29h | **加速比**: ~3.5x

## 任务详细描述

### fnd-1: 定义新插件接口

在 `plugin.go` 注册 ReviewGate 槽位，创建 4 个接口文件：

- `review_gate.go` — `ReviewGate` (Submit/Check/Cancel) + `ReviewResult`
- `tracker.go` — `Tracker` (CreateTask/UpdateStatus/SyncDependencies/OnExternalComplete)
- `scm.go` — `SCM` (CreateBranch/Commit/Push/Merge/CreatePR)
- `notifier.go` — `Notifier` (Notify) + `Notification`

签名对齐 `spec\spec-secretary-layer.md` Section 七。

### fnd-7: REST API 服务骨架

- `internal/web/server.go` — Server struct，chi 路由注册，Start/Shutdown
- `internal/web/middleware.go` — auth (Bearer token)，CORS，request logging，panic recovery
- `internal/web/handlers_system.go` — GET /health, GET /api/v1/stats

### p2a-1: Secretary Layer 领域实体

- `internal/core/chat.go` — ChatSession, ChatMessage structs
- `internal/core/taskplan.go` — TaskPlan, TaskItem, TaskPlanStatus, TaskItemStatus, FailurePolicy enums, ID 生成函数
- TaskItem.Description 为必填字段（对齐 spec\spec-secretary-layer.md:148）
- TaskPlan 新增 `WaitReason` 字段（`feedback_required` / `final_approval`），用于区分两种 waiting_human 场景

### p2a-5: DAG 数据结构

- `internal/secretary/dag.go` — DAG struct (Nodes/Downstream/InDegree)
- Build() 从 TaskItem[] 构建图
- Validate() 用 Kahn 算法检测：
  - 环（`DAG_CYCLE_DETECTED`）
  - 孤立引用（`DAG_MISSING_NODE`）
  - 自依赖（`DAG_SELF_DEPENDENCY`）
  - 重复边自动去重（不报错）
- TransitiveReduce() 传递约简（删除冗余边，保证可达关系不变）
- ReadyNodes() 返回入度=0 的节点
- 全覆盖单元测试（对齐 `2026-03-01-dag-conversion-minimal-rules.md` Section 8 的 8 个边界用例）

### p2a-7: DAG Scheduler（关键路径，L 级）

- `internal/secretary/scheduler.go` — DepScheduler struct
- StartPlan(): 构建 DAG → 校验 → 传递约简 → 计算入度 → dispatch ready
- dispatchTask(): 获取信号量 → 创建 Pipeline → Executor.Run
- EventBus 监听: pipeline_done → 标记 done → 解锁下游; pipeline_failed → FailPolicy 处理
- 失败策略:
  - **block**: 下游标记 `blocked_by_failure`
  - **skip**: hard/weak 依赖判定（LLM 辅助 + 规则兜底，对齐 rules Section 11-12），hard 依赖下游 skipped，weak 依赖下游可继续
  - **human**: Plan 进入 `waiting_human`
- 崩溃恢复: 扫描 executing plans → 重建 DAG → 恢复状态

### p2b-3: ReviewPanel 编排引擎（L 级，强门禁状态机）

- `internal/secretary/review.go` — ReviewPanel struct
- **强门禁流程**（对齐 `2026-03-01-dag-conversion-minimal-rules.md` Section 1）：
  1. AI review 为强制门禁，`max_rounds = 2`
  2. 3 个 Reviewer Agent 并行调用（goroutine + WaitGroup）→ 收集 ReviewVerdict → 送 Aggregator
  3. Aggregator 决策 `approve` → Plan 进入 `waiting_human` + `wait_reason=final_approval`（**AI 通过后仍需人工最终确认**）
  4. Aggregator 决策 `fix` → 替换 TaskPlan 重审（消耗一轮）
  5. 超过 2 轮未通过 → Plan 进入 `waiting_human` + `wait_reason=feedback_required`
  6. 人工驳回 → 必填两段式反馈（问题类型 + 具体说明，最少 20 字）→ Secretary 自动重生成 → 重新进入 AI review
  7. 人工通过 → 全部任务立即进入 DAG 调度队列
- 每轮结果写入 `review_records` 表

## 关键文件清单（需修改的现有文件）

| 文件 | 修改内容 | 涉及任务 |
|------|---------|---------|
| `internal/core/plugin.go` | 注册 ReviewGate 槽位 | fnd-1 |
| `internal/core/store.go` | 扩展 Store 接口方法（ChatSession/TaskPlan/TaskItem/ReviewRecord） | p2a-2, p2b-1 |
| `internal/core/events.go` | 添加 Secretary Layer 事件类型 | p2a-10 |
| `internal/plugins/factory/factory.go` | 扩展 BootstrapSet + 注册新插件到 newDefaultRegistry() | fnd-2~fnd-6 |
| `internal/config/types.go` | 添加 `secretary`/`review_panel`/`dag_scheduler` 配置段 | p2a-7, p2b-3 |
| `internal/engine/executor.go` | 执行期文件沉淀 prompt 注入 | p2a-11 |
| `internal/engine/prompts.go` | 新增 secretary prompt 变量 | p2a-6, p2a-11 |
| `internal/plugins/store-sqlite/migrations.go` | 4 张新表 DDL（chat_sessions/task_plans/task_items/review_records） | p2a-3, p2b-1 |

## DAG 拆解模式总结（可复用规则）

1. **按数据流向拆分** — 类型定义 → 接口 → DB → Store 实现 → 业务逻辑 → API → UI
2. **横向并行** — 同层级的独立模块并行（如 4 个插件、4 个 UI 视图）
3. **纵向串行** — 上下游有数据依赖的严格串行（Store 接口 → 实现 → API handler）
4. **每个任务可独立测试** — 有明确的输入/输出和验收标准
5. **L 级任务是瓶颈** — 关键路径上的 L 级任务决定总工期，优先分配资源
6. **前端先 Mock 后接入** — UI 层可用 Mock Data 提前开发，最终集成时替换为真实 API

## 验证方式

1. 每个任务完成后运行 `go test ./...` 确保不破坏现有测试
2. P2-Foundation 完成后: `ai-flow server` 可启动，`/health` 返回 200
3. P2a 完成后: POST /chat → Secretary Agent 返回 TaskPlan JSON → DAG Scheduler 创建 Pipeline
4. P2b 完成后: TaskPlan 自动经过 AI review 强门禁 → 人工最终确认 → 进入 DAG 调度
5. P2c 完成后: `ai-flow server` 打开浏览器可见 Chat/Plan/Board/Pipeline 四个视图
6. 全部完成: 端到端 — 对话 → 拆解 → AI 审核(≤2轮) → 人工确认 → 并行执行 → 完成

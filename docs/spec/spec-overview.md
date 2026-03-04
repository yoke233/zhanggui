# V2 总览规范（Issue / WorkflowProfile / WorkflowRun / Team Leader）

## 目标

本规范定义 V2 的唯一主链路：`issue -> workflow_profile -> workflow_run`，
并由 Team Leader 负责用户入口与编排。

- `issue`：唯一需求/交付单元。
- `workflow_profile`：流程档位与审核策略（`normal | strict | fast_release`）。
- `workflow_run`：一次执行实例及其状态机。
- `Team Leader`：统一对话与调度入口（替代旧 Secretary 语义）。

## 非目标（断代声明）

- 不保留 `task/plan` 业务实体与 API 兼容层。
- 不保留 DAG 运行时依赖调度能力。
- 不保留旧事件名与旧路由别名。

## 核心领域模型

### Project

关键字段：

- `id`
- `name`
- `repo_path`：本地仓库绝对路径
- `default_branch`：主分支名（创建时自动检测，fallback `main`）
- `github_owner` / `github_repo`（可选）
- `created_at` / `updated_at`

约束：

- `repo_path` 在系统内唯一。
- `default_branch` 在项目创建时由 `git rev-parse --abbrev-ref HEAD` 自动检测并持久化；
  后续所有 run 的 workspace setup 和 merge stage 以此值作为 base branch，
  不再依赖运行时主仓库 HEAD 状态。
- API 创建时可显式指定 `default_branch`，为空则自动检测。

### Issue

关键字段：

- `id`: `issue-*`
- `project_id`
- `session_id`
- `title`
- `body`
- `status`: `draft | reviewing | ready | executing | done | failed | abandoned`
- `workflow_profile`: `normal | strict | fast_release`
- `input_mode`: `text | files`
- `review_scope.files`（文件模式时必填）
- `auto_merge`
- `created_at` / `updated_at`

约束：

- `title` 为必填字段，去除首尾空白后长度必须在 `1..120`。
- 任意状态变化必须记录到 issue 时间线。
- `executing` 状态必须可关联至少一个 `workflow_run`。
- `timeout` 由 run 层处理，issue 最终落为 `failed` 并记录超时原因。
- 创建 issue 时必须在 `text/files` 模式中二选一，不允许混合主输入。
- 文件模式下 review 仅覆盖 `review_scope.files`，范围变更必须显式留痕。

### WorkflowProfile

关键字段：

- `id`: `normal | strict | fast_release`
- `review_policy`（评审人数、聚合策略、通过阈值）
- `sla_minutes`（默认 60）
- `concurrency_limit`（可选）
- `retry_policy`（可选）

约束：

- `sla_minutes` 必须大于 0，V2 默认 60 分钟。
- `strict` 必须支持并行 reviewer + aggregator。
- `fast_release` 必须走轻量审核路径，但仍要留痕。

### WorkflowRun

关键字段：

- `id`: `run-*`
- `issue_id`
- `workflow_profile`
- `status`: `created | running | waiting_review | done | failed | timeout | cancelled`
- `started_at` / `finished_at`
- `summary`
- `error`

约束：

- 每个 run 必须有明确结束态（`done/failed/timeout/cancelled`）。
- run 事件必须可按时间排序查询。
- 取消与超时必须产出显式结束事件。

## 架构分层

1. `Web API`：提供项目、issue、workflow profile、run、事件查询接口。
2. `Team Leader`：维护会话上下文，决定 issue 推进与 profile 选择。
3. `Issue Service`：issue 生命周期与时间线写入。
4. `Run Engine`：基于 profile 规则执行 run（ACP 协议为唯一执行路径）。
5. `Event Bus + Event Store`：解耦触发与推进，持久化 run/review 事件。EventBus 订阅者将带 `run_id` 的事件写入 `run_events` 表。
6. `Auto-Merge`：监听 `EventRunDone`，执行 test gate → PR 创建 → PR 合并。
7. `A2A Bridge`：接收外部 agent 任务，自动创建 issue 并 approve。
8. `Integrations`：GitHub 等外部系统对接。

## 标准主链路

1. 用户向 Team Leader 发送消息。
2. Team Leader 创建或更新 issue，并确定 `workflow_profile`。
3. Scheduler 将 issue 推入 profile 队列并创建 run。
4. Run Engine 执行 run，并持续发布 `run_*` 事件。
5. Review Orchestrator 按 `workflow_profile` 产生评审结论。
6. issue 时间线写入 review/action/checkpoint 事件并推进状态。
7. 用户或系统可通过 API 统一查询 run 与 review 轨迹。

## 事件观测基线

- 会话运行事件：`GET /api/v2/sessions/{sessionID}/runs/events?project_id={projectID}`
- issue 时间线：`GET /api/v2/issues/{issueID}/timeline?project_id={projectID}`
- run 详情：`GET /api/v2/runs/{runID}?project_id={projectID}`
- run 事件流：`GET /api/v2/runs/{runID}/events?project_id={projectID}`

新增事件类型：

- `auto_merged`：auto-merge 完成后发布，`data` 含 `branch` 和 `pr_url`（可选）。

## 验收基线

- `task/plan` 与 DAG 语义不再作为主路径出现。
- Team Leader 命名在接口与文案中统一。
- `v2-smoke` 可验证 issue/profile/run 全链路可用。

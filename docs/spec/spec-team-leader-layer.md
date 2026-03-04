# Team Leader 层规范

## 角色定位

Team Leader 是系统唯一用户入口，负责把用户目标持续收敛为可执行 issue，
并驱动 issue/profile/run 主链路。

核心职责：

- 维护会话上下文与用户意图。
- 创建/更新 issue，推进 issue 生命周期。
- 选择 `workflow_profile`（`normal | strict | fast_release`）。
- 协调 run 与 review，向用户回显阶段性结果。

## 输入与输出

### 输入

- 用户消息
- 项目仓库上下文
- 历史 issue 与时间线
- 最近 run 事件
- 外部触发（如 GitHub 命令）

### 输出

- issue 变更（字段或状态）
- run 启停命令
- review 结论摘要
- 用户可读反馈与下一步建议

## Issue 创建与 Review 范围（严格定义）

Team Leader 创建 issue 时，必须在以下两种输入模式中二选一：

- 文本模式：提交 `title + body`，`file_paths` 必须为空。
- 文件模式：提交 `title + file_paths[]`，`body` 可为空或作为补充说明。

强约束：

- `title` 必填；去除首尾空白后长度必须在 `1..120`。
- `body` 与 `file_paths[]` 不允许同时为空。
- `body` 与 `file_paths[]` 不允许同时作为主输入（即不支持 mixed 模式）。
- `file_paths[]` 至少包含 1 个文件，且必须去重。
- `file_paths[]` 必须是仓库内相对路径，禁止绝对路径与越界路径（如 `../`）。

Review 范围规则：

- 文件模式创建时，系统必须生成 `review_scope.files = file_paths[]` 快照。
- 首次 review 仅允许覆盖 `review_scope.files` 中的文件。
- 任何扩展或缩小 review 范围必须走显式 issue action，并写入时间线留痕。
- run 进入 `executing` 前，必须存在与当前 `review_scope.files` 对应的 review 结论。

时间线留痕：

- 初始化范围：`review_scope_initialized`
- 变更范围：`review_scope_changed`
- 记录字段至少包含：`before_files`、`after_files`、`actor`、`reason`

## Issue 生命周期

`draft -> reviewing -> ready -> executing -> done/failed/abandoned`

约束：

- 状态转换必须写 `issue_changes`。
- review 结论必须写 `review_records`。
- `executing` 期间必须关联可追踪 run。
- run `timeout` 时，issue 必须进入 `failed` 并记录超时原因。

## Profile 选择规则

### Workflow Profile（流程档位）

- `normal`：1 reviewer + 1 aggregator。
- `strict`：3 reviewers 并行 + 1 aggregator。
- `fast_release`：轻量审核 + 快速结论。

### Agent Role（执行角色）

- `team_leader`：需求澄清、计划分解、上下文维护。
- `implementer`：代码与脚本执行。
- `reviewer`：结果评审与风险审计。

约束：

- 流程档位决定审核编排策略。
- 执行角色决定会话能力边界。
- 禁止把两者混为同一字段。

## Run 协调规则

- 每次用户提交至多触发一个活跃 run。
- 同一 session 同时只允许一个 `running/waiting_review` run。
- 重复触发请求应返回”已有运行态”并附 run 标识。
- 取消 run 后必须写入 `run_cancelled` 并同步 issue 时间线。

## A2A 集成

通过 A2A 协议（JSON-RPC）接收外部 agent 任务：

- A2A 创建的 issue 默认 `auto_merge = true`。
- 创建后自动执行 approve action，使 run 立即启动（无需人工审批）。
- A2A 端点默认启用（`a2a.enabled: true`），需配置 token。

## Auto-Merge 流程

`AutoMergeHandler` 监听 `EventRunDone`，当 `issue.auto_merge = true` 时：

1. Test gate：仅测试变更的 Go package，无变更则 `go build`，10 分钟超时。
2. 创建 PR（draft → ready）。
3. 合并 PR。
4. 发布 `auto_merged` 事件。

失败时发布 `EventRunFailed`，`data.phase` 标识失败阶段
（`auto_merge_test_gate` / `auto_merge_create_pr` / `auto_merge_merge_pr`）。

## 观测与可追溯性

Team Leader 至少支持以下查询视图：

- 会话级 run 事件流；
- issue 时间线（review/log/checkpoint/action）；
- run 详情（输入、输出摘要、错误）。

## 失败处理

- profile 不可用：返回明确错误，issue 保持原状态。
- run 异常中断：写入 `run_failed`，并附错误摘要。
- review 写入失败：返回 5xx，禁止静默吞错。
- 事件存储失败：允许重试，但必须记录告警日志。

## 断代约束

- 对外语义统一 Team Leader，不再暴露 Secretary 命名。
- 不保留 Plan/Task 行为入口与文案。

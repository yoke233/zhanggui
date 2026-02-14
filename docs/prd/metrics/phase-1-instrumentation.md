# PRD - 提前埋点规范（Phase 1 启动，支撑 Phase 4/5）

版本：v1.0  
状态：Draft  
负责人：PM / Lead / Platform  
适用阶段：Phase 1-3（必须兼容），为 Phase 4/5 预留

## 1. 目标

在不增加流程复杂度的前提下，提前埋好最小可用数据，确保后续可以直接做：

- 产能与时效看板
- 质量闭环分析
- 自动路由优化
- 成本与风险治理

核心原则：

- 埋点是“事实记录”，不是新流程。
- 埋点不改变现有协议主键：`IssueRef` + `run_id`。
- 先小后大：P0 必埋，P1 推荐，P2 可选。

## 2. 非目标

- 不在本阶段实现复杂 BI 平台。
- 不要求引入外部时序数据库。
- 不要求所有 CLI 立即输出结构化 JSON。

## 3. 统一事件模型（最低要求）

每条事件建议具备以下字段（P0）：

- `project_id`：项目标识（字符串）
- `issue_ref`：协作主键（canonical IssueRef）
- `run_id`：执行尝试主键（无执行时可为 `none`）
- `role`：角色（backend/frontend/qa/integrator/architect/recorder）
- `lead_id`：当前统筹者身份
- `worker_id`：执行者身份（可为 CLI 名称或实例 ID）
- `event_type`：事件类型（见第 4 节枚举）
- `status`：`ok|fail|blocked|n/a`
- `result_code`：标准原因码（见第 5 节）
- `occurred_at`：UTC 时间（ISO8601）

约束：

- `issue_ref` 必须是 canonical 值，不允许平台内部 id/node_id。
- `occurred_at` 必须使用 UTC，禁止本地时区混写。
- `result_code` 不可自由发挥，必须来自枚举。

## 4. 事件类型枚举（P0）

- `issue_created`
- `issue_claimed`
- `work_started`
- `work_result_received`
- `result_normalized`
- `blocked`
- `unblocked`
- `review_recorded`
- `merged`
- `issue_closed`
- `stale_result_discarded`
- `needs_human_raised`

## 5. 原因码枚举（P0）

- `dep_unresolved`
- `test_failed`
- `ci_failed`
- `review_changes_requested`
- `env_unavailable`
- `permission_denied`
- `output_unparseable`
- `stale_run`
- `manual_intervention`

## 6. 分层埋点清单

P0（Phase 1 必须）：

- 主键字段：`issue_ref`、`run_id`
- 生命周期事件：创建、claim、开始、结果、关闭
- 失败原因：`result_code`
- 结构化回填：Issue comment 中可审计字段

P1（Phase 2 前完成）：

- `queue_wait_ms`：open -> claim
- `exec_duration_ms`：work_started -> work_result_received
- `blocked_duration_ms`：blocked -> unblocked
- `retry_count`：同 Issue 的重试次数
- `active_run_switch_count`：worker 切换次数

P2（Phase 3/4 可选）：

- `cost_estimate`：token/调用成本估算
- `artifact_size`：日志/报告体积
- `command_count`：执行命令数量
- `security_flag`：敏感事件标记

## 7. 数据落点（低成本方案）

优先顺序：

1. Issue comment（结构化字段）  
说明：这是协作真源，必须保留审计证据。

2. 本地 sqlite/ndjson（分析副本）  
说明：用于聚合统计，可重建，不替代真源。

建议：

- 本地模式可先用 `events` 表追加记录。
- 若暂时无独立分析库，可在 `state/` 下保存 `metrics.ndjson`。

## 8. 指标口径（PM 可直接使用）

建议首批看板指标：

- `T_first_claim`：Issue 创建到 claim 的中位时长
- `T_first_execution`：Issue 创建到首次 `work_started` 的中位时长
- `T_cycle`：Issue 创建到关闭的中位时长
- `R_blocked`：进入 blocked 的 Issue 比例
- `R_rework`：出现 `review_changes_requested` 或 `ci_failed` 的 Issue 比例
- `R_stale_run`：`stale_result_discarded` 占比

## 9. 质量门槛（按阶段）

Phase 1：

- 每个关闭 Issue 至少有 1 条结构化证据 comment。
- 每个失败事件都必须带 `result_code`。

Phase 2：

- 过期 run 覆盖事件数应为 0（必须被丢弃并记录 `stale_run`）。

Phase 3：

- review/CI 失败自动回流成功率 >= 95%。

## 10. Phase 4/5 触发条件（非当前范围）

满足任意 2-3 条时，建议正式开启 Phase 4（规模化治理）：

- 同时运行项目数 >= 3
- 角色 Lead 总数 >= 6
- 平均每日活跃 Issue >= 30
- 因路由延迟导致的阻塞比例连续两周 > 15%
- 质量失败回流时延 P95 > 2 小时

满足任意 2 条时，建议开启 Phase 5（平台化与策略引擎）：

- 多 backend 并行接入（GitHub + GitLab + DB）
- 审批从 `any` 演进到 `all/quorum/staged`
- 明确需要统一租户隔离、审计与成本配额

## 11. 依赖文档

- `docs/operating-model/outbox-backends.md`
- `docs/operating-model/executor-protocol.md`
- `docs/operating-model/phases.md`
- `docs/workflow/issue-protocol.md`
- `docs/workflow/templates/comment.md`

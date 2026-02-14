# PRD - Phase 2.5 Reviewer-Lead 调度增强

版本：v1.0  
状态：Draft  
负责人：PM / Integrator / Reviewer Lead / Platform  
目标阶段：Phase 2.5（承接 Phase 2 与 Phase 2.1，先于 Phase 3）

## 1. 背景与问题

Phase 2 已完成最小 Lead 自动化，Phase 2.1 提供了 Lead 控制台能力，但“评审队列”仍存在空档：

- review 相关任务主要靠人工盯 `state:review` 与评论线程，回流慢。
- backend lead、qa lead、integrator 对 `state:review` 的关注重叠，容易出现重复处理或无人负责。
- 缺少 reviewer 角色的正式调度链路，评审结论到路由动作之间仍有人工搬运。

因此需要在不改变既有协议字段的前提下，引入 reviewer-lead 作为正式调度角色。

## 2. 目标与非目标

目标：

- 将 Reviewer 作为正式角色加入 `roles.enabled`（并补齐 `role_repo`、`groups`、`executors`）。
- 由 reviewer-lead 监听 review 队列（`to:reviewer` / `state:review`）并派发 reviewer worker。
- reviewer-lead 自动写回结构化评审结论，并把结果回流到责任角色。
- 保持 Lead 为“规则调度器”，不做智能技术裁决。

非目标：

- 不替代 GitHub/GitLab 的代码审查界面与审批能力。
- 不引入新的协作真源（仍是 Issue/Event + assignee + labels）。
- 不扩展复杂审批模式（`all/quorum/staged` 留给后续阶段）。
- 不在本阶段实现 PR/CI 平台全量自动集成（属于 Phase 3 范畴）。

## 3. 用户与场景

用户：

- Reviewer Lead（新）
- Integrator
- Backend/Frontend Lead
- PM（观察与追踪）

核心场景：

- 场景 A：Issue 进入 `state:review` 后，reviewer-lead 自动接管并派工。
- 场景 B：review 结论为 changes requested，自动回流到实现责任角色。
- 场景 C：review 结论为 approved，自动路由到 integrator 收敛。
- 场景 D：review worker 失败或结果不可解析，自动写回 blocked 并给出 Next。

## 4. 范围（In Scope）

- `workflow.toml` 增量配置：
  - `roles.enabled` 增加 `reviewer`
  - `role_repo.reviewer` 映射
  - `groups.reviewer`（含 `max_concurrent`、`listen_labels`）
  - `executors.reviewer`
- reviewer-lead 轮询 + cursor + run_id 幂等逻辑复用 Phase 2 主链路。
- reviewer worker 结果标准化写回 comment（使用既有模板字段）。
- 根据评审结论回流：
  - `approved` -> `Next: @integrator ...`
  - `changes_requested` -> `Next: @backend|@frontend ...`
- 与 `needs-human`、`depends-on`、`active_run_id` 语义兼容。

## 5. 功能需求（PRD 级）

- FR-2.5-01：系统必须支持 `reviewer` 作为正式 role 被启用与调度。
- FR-2.5-02：reviewer-lead 必须可监听 review 队列（`to:reviewer` / `state:review`）并产生候选任务。
- FR-2.5-03：reviewer-lead 派工必须生成 reviewer run_id，并遵循 `active_run_id` 幂等检查。
- FR-2.5-04：reviewer worker 的结果必须写回结构化 comment（`IssueRef/RunId/Action/Status/Next/Changes/Tests`）。
- FR-2.5-05：`changes_requested` 必须自动回流到责任角色并给出可执行 Next。
- FR-2.5-06：`approved` 必须自动路由到 integrator，不越过审批策略。
- FR-2.5-07：存在 `needs-human` 或未满足依赖时，必须停止自动推进并写回 blocked。
- FR-2.5-08：同一触发事件不得重复写回噪声 comment（需幂等与去重）。

## 6. 关键策略（边界约束）

- 责任真源仍是 `assignee`：claim/接管以 assignee 状态为准。
- reviewer-lead 仅做规则判断与流程推进，不做“是否技术正确”的智能判定。
- 若多角色都监听 `state:review`，需以“当前 assignee + role 路由”保证单写者，避免冲突。
- 迟到结果（`run_id != active_run_id`）仅记录，不自动推进状态。

## 7. 验收标准（DoD）

- AC-2.5-01：`reviewer` 可在 `workflow.toml` 中完整启用并正常运行 lead loop。
- AC-2.5-02：`to:reviewer` 与 `state:review` 场景下，reviewer-lead 均可触发派工与写回。
- AC-2.5-03：`changes_requested` 可自动回流到实现角色，无需人工提醒。
- AC-2.5-04：`approved` 可自动路由到 integrator 收敛，并保持审批边界不被绕过。
- AC-2.5-05：重启后 cursor 可续跑，且不会因重复事件产生重复写回。

## 8. 成功指标

- 指标 1：`state:review` 到首次有效评审写回的中位时延下降 >= 50%。
- 指标 2：review 结果人工搬运次数下降 >= 70%。
- 指标 3：review 相关回填 comment 结构化字段完整率 >= 98%。
- 指标 4：review 阶段重复处理/重复写回事件数趋近 0。

## 9. 风险与缓解

- 风险：`state:review` 被多角色同时消费，导致重复动作。  
  缓解：明确单写者优先级（assignee + role 路由 + 幂等键）。

- 风险：评审结论映射不一致，回流方向错误。  
  缓解：限定结论枚举（approved/changes_requested）并保留 `needs-human` 兜底。

- 风险：reviewer 角色新增后配置不完整（无 executor / 无 role_repo）。  
  缓解：启动时做配置校验，缺项即 fail-fast。

## 10. 依赖

- `docs/prd/phases/phase-2-prd.md`
- `docs/prd/phases/phase-2-1-prd.md`
- `docs/prd/phases/phase-3-prd.md`
- `docs/workflow/lead-worker.md`
- `docs/workflow/workflow-profile.md`
- `docs/workflow/issue-protocol.md`
- `docs/workflow/label-catalog.md`
- `docs/operating-model/quality-gate.md`

# PRD - Phase 2.1 Lead 控制台（Bubble Tea TUI）

版本：v1.0  
状态：Draft  
负责人：PM / Tech Lead / Platform  
目标阶段：Phase 2.1（承接 Phase 2，先于 Phase 3）

## 1. 背景与问题

Phase 2 已引入最小 Lead 自动化（polling + cursor + run_id 幂等），但日常运维仍偏“日志驱动”：

- Lead/Integrator 需要在多个窗口手工观察队列、阻塞、回填状态。
- 关键动作（claim、重试、切换 worker、标记 blocked/needs-human）缺少统一操作面。
- 人工判断与系统状态之间缺少同屏反馈，容易发生重复处理或漏处理。

因此需要一个轻量的本地控制台，用于“看板 + 调度 + 闭环操作”，而不改变现有协议真源。

## 2. 目标与非目标

目标：

- 提供一个基于 Bubble Tea 的 Lead/Operator TUI。
- 以 `IssueRef` 为主键展示队列状态、责任归属、活跃 run。
- 支持最小可用操作：claim、开工、切换 worker、写回标准 comment、close。
- 所有动作仍落到既有真源（Issue/outbox + `workflow.toml`），不引入第二状态机。

非目标：

- 不替代 GitHub/GitLab Web 界面做完整代码审查。
- 不在本阶段实现复杂 dashboard（跨项目 BI、趋势预测等）。
- 不引入新的协作对象（保持 Issue/Event 模型）。
- 不改动 `docs/workflow/templates/*` 的协议字段定义。

## 3. 用户与场景

用户：

- Role Lead（backend/frontend/qa/integrator）
- Integrator
- PM（只读观察为主）

核心场景：

- 场景 A：查看待处理队列并快速 claim。
- 场景 B：Issue 进入 blocked 后，定位依赖并等待解除。
- 场景 C：worker 失败后切换执行器并维持 `active_run_id` 幂等。
- 场景 D：把 worker 的自然语言输出规范化写回 comment 模板。
- 场景 E：本地模式（git + sqlite）下完成闭环操作。

## 4. 范围（In Scope）

- 队列视图：按 role/group/state/priority 过滤 Issue。
- 详情视图：展示 Issue 主帖、最近事件、`active_run_id`、DependsOn/BlockedBy。
- 操作命令：
  - claim/unclaim
  - spawn worker（生成新 `run_id`）
  - switch worker（生成新 `run_id` 并替换 active）
  - normalize + reply（按 comment 模板写回）
  - set blocked/unblock
  - close issue（满足闸门条件时）
- 幂等防护：迟到结果默认不推进状态。
- 本地优先：先支持 sqlite outbox，再兼容 GitHub/GitLab backend。

## 5. 功能需求（PRD 级）

- FR-2.1-01：TUI 必须从 `<outbox_repo>/workflow.toml` 加载项目画像。
- FR-2.1-02：队列列表必须可按 `to:*`、`state:*`、assignee 过滤。
- FR-2.1-03：详情页必须展示当前 `IssueRef` 的 `active_run_id`。
- FR-2.1-04：执行“切换 worker”时必须生成新 `run_id` 并原子更新 active。
- FR-2.1-05：对迟到结果（`run_id != active_run_id`）必须提供“仅查看，不自动写回”。
- FR-2.1-06：写回 comment 必须使用模板字段（`IssueRef`、`RunId`、`Action`、`Status`、`Next` 等）。
- FR-2.1-07：当存在 `needs-human` 时，TUI 不应提供自动推进动作。
- FR-2.1-08：关键操作需写审计日志（最少含时间、actor、IssueRef、Action、结果）。

## 6. 交互与信息架构（MVP）

视图分层：

1. Queue List：任务列表（默认按优先级 + 更新时间排序）
2. Issue Detail：主帖 + 最近事件 + 依赖与阻塞 + 活跃 run
3. Action Panel：可执行动作与确认提示
4. Log Panel：最近操作结果与错误信息

关键快捷动作（建议）：

- `c`：claim / unclaim
- `s`：spawn worker
- `w`：switch worker
- `r`：写回规范化 comment
- `b`：blocked / unblock
- `x`：close issue（仅在条件满足时启用）

## 7. 验收标准（DoD）

- AC-2.1-01：Operator 可在 TUI 内完成“claim -> spawn -> writeback -> close”的最小闭环。
- AC-2.1-02：TUI 对迟到结果不自动推进状态，幂等行为与 Phase 2 语义一致。
- AC-2.1-03：写回 comment 字段完整且符合 `docs/workflow/templates/comment.md`。
- AC-2.1-04：遇到 `needs-human` 时，自动动作被阻止并给出明确提示。
- AC-2.1-05：本地 sqlite 模式可独立运行，不依赖 GitHub/GitLab。

## 8. 成功指标

- 指标 1：Lead 日常队列操作平均耗时下降 >= 40%。
- 指标 2：因“操作遗漏/错序”导致的状态回滚次数下降 >= 60%。
- 指标 3：结构化 comment 完整率达到 95% 以上。
- 指标 4：切换 worker 场景下的幂等错误数为 0（以抽样验收统计）。

## 9. 风险与缓解

- 风险：TUI 功能膨胀，变成“第二个平台”。  
  缓解：坚持 MVP，只做 Phase 2 的操作闭环，不做 BI/分析平台。

- 风险：UI 交互复杂导致学习成本高。  
  缓解：保持命令集最小化，增加内置 help 和默认安全确认。

- 风险：本地与 forge backend 行为差异。  
  缓解：先实现统一 Outbox 抽象层，再在 adapter 层处理差异。

## 10. 依赖

- `docs/operating-model/executor-protocol.md`
- `docs/operating-model/outbox-backends.md`
- `docs/workflow/issue-protocol.md`
- `docs/workflow/lead-worker.md`
- `docs/workflow/workflow-profile.md`
- `docs/workflow/templates/comment.md`

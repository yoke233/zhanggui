# PRD - Phase 2.8 并行审查的协调者模型（Assignee 固定 + 单写者）

版本：v1.0  
状态：Draft  
负责人：PM / Integrator / Reviewer Lead / QA Lead / Platform  
目标阶段：Phase 2.8（承接 Phase 2.5/2.6/2.7，先于 Phase 3）

## 1. 背景与问题

Phase 2.5 引入 reviewer 角色与 reviewer-lead，使 review 队列可被自动调度。  
但当前实现为了保证“单写者 + 不互相踩踏”，通常依赖 `assignee == lead` 才允许 Lead 处理 Issue，这会带来几个副作用：

- **责任归属抖动**：review 阶段需要把 assignee 在多个 lead 之间频繁交接（backend -> reviewer -> integrator），TUI/审计与人类理解成本上升。
- **并行审查受限**：当 assignee 只允许单一 lead 时，reviewer 与 QA 很难在同一时刻并行产出结论。
- **与协议建议不一致**：协作协议与护栏更推荐 “assignee=协调者（通常 integrator-lead） + to:* 订阅并行关注 + 状态单写者” 的模型。

因此需要在 Phase 3（PR/CI 自动化）之前，先把“并行审查如何在本地 sqlite/outbox 模式下正确运转”的职责边界钉住。

## 2. 目标与非目标

目标：

- 引入 **Coordinator Assignee Model**：在 `state:review` 阶段保持 `assignee` 为协调者（默认 `lead-integrator`），避免 ownership ping-pong。
- 支持 **reviewer-lead / qa-lead 作为订阅者并行工作**：即使不是 assignee，也能监听命中队列、派 reviewer/qa worker、写回结构化“判定 + 证据”。
- 强制 **状态机单写者**：`state:*` 的推进与 close/done 仅允许协调者 lead（通常 integrator-lead）写回。
- 将 reviewer/qa 的输出沉淀为可计算信号（结构化字段或 label），为 Phase 3 的 PR/CI 读取打底。

非目标：

- 不在本阶段接入 GitHub/GitLab PR review/CI checks（属于 Phase 3）。
- 不引入复杂审批模式（`all/quorum/staged` 仍留给后续阶段）。
- 不引入新的协作真源对象（仍然是 Issue/outbox + assignee + labels + comment）。

## 3. 用户与场景

用户：

- Integrator Lead（协调者 / 单写者）
- Reviewer Lead（订阅者 / 判定者）
- QA Lead（订阅者 / 判定者）
- PM（全局观察）

核心场景：

- 场景 A：Issue 进入 `state:review`，assignee=integrator；reviewer 与 QA 并行产出判定与证据。
- 场景 B：reviewer 判定 `changes_requested` 或 QA fail；协调者汇总为 FixList，并回流到实现角色。
- 场景 C：reviewer `approved` 且 QA pass；协调者满足闸门后推进 `decision:accepted` 与 `state:done/close`。
- 场景 D：订阅者 worker 结果不可解析或证据缺失；写回“需要人工接管”，但不破坏状态机。

## 4. 范围（In Scope）

- **订阅者 Lead 的选题规则**：
  - 允许在不匹配 assignee 的情况下命中队列（依赖 `to:<role>`、`state:*` 等 listen labels）。
  - 仍必须尊重：`needs-human`、`autoflow:off`、DependsOn 规则、`active_run_id` 幂等。
- **写回权限收敛**：
  - 订阅者（reviewer/qa）只能写“判定 + 证据”的结构化 comment（不改 assignee、不推进 `state:*`、不 close）。
  - 协调者（integrator）负责状态推进与最终路由/关闭。
- **判定信号的规范化**：
  - reviewer：`review:approved` / `review:changes_requested`
  - QA：至少区分 `pass/fail`，并附证据（测试命令、日志片段或链接）
  - V1 允许“先结构化 comment，后续再补 label 映射”，但字段/枚举必须固定。
- **TUI/控制台显示**（若已存在全局控制台）：
  - 能在一个 Issue 的详情中看到 reviewer/qa 的最新判定与证据（按 `IssueRef + role` 聚合）。

## 5. 功能需求（PRD 级）

- FR-2.8-01：系统必须支持将 role group 标记为“订阅者模式”（subscriber），允许其在 `assignee != lead` 时仍可派 worker。
- FR-2.8-02：订阅者 lead 写回必须是“comment-only”，不得修改 `state:*`、assignee、close。
- FR-2.8-03：协调者 lead（默认 integrator）必须是 `state:*` 与 close 的唯一自动写者（single writer）。
- FR-2.8-04：同一 Issue 允许多个 role 同时有各自 `active_run_id`（按 `IssueRef + role` 幂等），迟到结果不得覆盖当前角色的 active run。
- FR-2.8-05：reviewer/qa 判定必须可被系统稳定读取（固定字段/枚举），不可只依赖自由文本。
- FR-2.8-06：当订阅者结果缺字段/不可解析/证据不足时，必须写回明确原因与 Next，但不得推进状态机。
- FR-2.8-07：必须有去重策略，避免订阅者在无新信息时反复写回噪声 comment。

## 6. 验收标准（DoD）

- AC-2.8-01：在 sqlite outbox 模式下，assignee=integrator 且存在 `to:reviewer`/`to:qa` 的 Issue，reviewer-lead 与 qa-lead 均可并行派工并写回判定 comment。
- AC-2.8-02：reviewer/qa 写回不会改变 `state:*` 与 assignee；只有 integrator-lead 能推进 `state:*`/close。
- AC-2.8-03：重启后 cursor 可续跑，且不会因重复事件产生重复写回。
- AC-2.8-04：迟到结果（`run_id != active_run_id`）不会覆盖当前判定；可被查看但不影响闸门。
- AC-2.8-05：在“review changes requested / QA fail”场景下，协调者可基于判定生成唯一 FixList，并回流到实现角色。

## 7. 成功指标

- 指标 1：review 阶段 assignee 变更次数下降 >= 80%（从多次交接趋近 0）。
- 指标 2：`state:review` 到首次有效 reviewer/qa 判定的中位时延下降 >= 50%。
- 指标 3：因“多角色写回互相覆盖”导致的状态污染事件为 0（抽样验收）。

## 8. 风险与缓解

- 风险：订阅者 lead 可以处理非 assignee issue，容易产生“多头写回”。  
  缓解：订阅者写回强制 comment-only + 幂等去重；状态推进仍只有协调者。

- 风险：判定枚举不一致导致无法计算。  
  缓解：固定 reviewer/qa 判定枚举与字段；对未知值直接 `needs-human`。

- 风险：与既有 Phase 2.5 “assignee 交接模型”并存导致混乱。  
  缓解：通过配置显式选择模式，并在文档中给出迁移路径（先跑通 2.5，再升级 2.8）。

## 9. 依赖

- `docs/prd/phases/phase-2-5-prd.md`
- `docs/prd/phases/phase-2-6-prd.md`
- `docs/prd/phases/phase-2-7-prd.md`
- `docs/prd/phases/phase-3-prd.md`
- `docs/workflow/issue-protocol.md`
- `docs/workflow/guardrails.md`
- `docs/operating-model/quality-gate.md`
- `docs/workflow/label-catalog.md`

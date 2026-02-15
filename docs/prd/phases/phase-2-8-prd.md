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
- **与协议建议不一致**：协作协议与护栏更推荐 “assignee=协调者（通常 `lead-integrator`） + to:* 订阅并行关注 + 状态单写者” 的模型。

因此需要在 Phase 3（PR/CI 自动化）之前，先把“并行审查如何在本地 sqlite/outbox 模式下正确运转”的职责边界钉住。

命名约定（避免文档/实现分叉）：

- 本 PRD 中的 “reviewer-lead / qa-lead / integrator-lead” 作为**概念角色**使用。
- 在实现与配置中，建议统一采用 `lead-<role>` 的 actor 命名：
  - reviewer-lead => `lead-reviewer`
  - qa-lead => `lead-qa`
  - integrator-lead => `lead-integrator`

## 2. 目标与非目标

目标：

- 引入 **Coordinator Assignee Model**：在 `state:review` 阶段保持 `assignee` 为协调者（默认 `lead-integrator`），避免 ownership ping-pong。
- 支持 **reviewer-lead / qa-lead 作为订阅者并行工作**：即使不是 assignee，也能监听命中队列、派 reviewer/qa worker、写回结构化“判定 + 证据”。
- 强制 **状态机单写者**：`state:*` 的推进与 close/done 仅允许协调者 lead（通常 `lead-integrator`）写回。
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

- 场景 A：Issue 进入 `state:review`，assignee=`lead-integrator`；reviewer 与 QA 并行产出判定与证据。
- 场景 B：reviewer 判定 `changes_requested` 或 QA fail；协调者汇总为 FixList，并回流到实现角色。
- 场景 C：reviewer `approved` 且 QA pass；协调者满足闸门后推进 `decision:accepted` 与 `state:done/close`。
- 场景 D：订阅者 worker 结果不可解析或证据缺失；写回“需要人工接管”，但不破坏状态机。

## 4. 范围（In Scope）

### 4.1 配置（workflow.toml v2，破坏性变更）

本阶段将 `workflow.toml` 升级为 **version = 2**，并将其作为新的基线语义。  
该升级允许破坏性变更：**version = 1 的配置不再支持**（系统必须 fail-fast，并给出明确迁移提示）。

V2 的关键变化：

- `groups.<name>.mode` / `groups.<name>.writeback` 成为工作流协议的一部分（不再依赖“隐式默认行为”）。
- `subscriber/comment-only` 使 reviewer/qa 可以在 `assignee != lead-<role>` 时并行产出判定与证据。
- `lead-integrator` 作为协调者保持 assignee 稳定；`state:*` 与 close 仍由单写者推进。

建议的最小配置形态（V2 必填字段；字段命名与语义固定）：

- 在 `workflow.toml` 的 `groups.<name>` 下新增字段：
  - `mode = "owner" | "subscriber"`
  - `writeback = "full" | "comment-only"`

约束：

- 当 `mode="subscriber"` 时，`writeback` **必须**为 `"comment-only"`（禁止 subscriber 全量写回造成多头推进）。
- 监听语义保持 AND：issue 必须同时包含 `listen_labels` 中的所有 labels 才算命中。
  - 若后续确实需要 OR（降低漏单风险），应以显式配置 `listen_mode="or"` 作为 V2 的增量扩展，而不是在代码里写死角色特判。

Claim 语义（与 `require_claim_before_work` 兼容）：

- subscriber 仍要求 issue **已被 claim**（`assignee` 非空），但不要求 assignee 等于 subscriber 自己。
- 推荐：在 `state:review` 阶段由协调者 `lead-integrator` claim 并保持稳定；reviewer/qa 仅并行产出判定与证据。

- **订阅者 Lead 的选题规则**：
  - 允许在不匹配 assignee 的情况下命中队列（依赖 `to:<role>`、`state:*` 等 listen labels）。
  - 仍必须尊重：`needs-human`、`autoflow:off`、DependsOn 规则、`require_claim_before_work`、`active_run_id` 幂等。
- **写回权限收敛**：
  - 订阅者（reviewer/qa）只能写“判定 + 证据”的结构化 comment（不改 assignee、不推进 `state:*`、不 close）。
  - 协调者（integrator）负责状态推进与最终路由/关闭。

### 4.2 判定信号的规范化（可计算）

本阶段的“判定”必须可被系统稳定读取与聚合，避免只靠自然语言猜测。

最小真源（必须）：

- 订阅者写回的结构化 comment（必须遵守 `docs/workflow/templates/comment.md` 关键字段）。
- 在 comment 的 `Summary`（或等价固定字段）中写入固定 marker（枚举固定）：
  - reviewer：`review:approved` / `review:changes_requested`
  - QA：`qa:pass` / `qa:fail`（并附证据：测试命令、日志片段或链接）

可选索引（建议但不强制）：

- 可将上述 marker 同步为 labels（例如 `review:*`、`qa:*`）以便队列筛选/看板统计。
- 若启用此能力，必须同步更新：
  - `<outbox_repo>/workflow.toml` 的 `[labels]` 定义（作为标签真源）
  - `docs/workflow/label-catalog.md`（作为目录与说明）

### 4.3 Comment-only 写回边界（硬约束）

订阅者的 comment-only 写回必须满足：

- 允许：追加结构化 comment（用于“判定 + 证据 + Next 建议”）。
- 禁止：
  - 修改 `assignee`（claim/unclaim/接管）
  - 修改 `state:*`（状态迁移）
  - close issue（或写入等价的 done/close 动作）
  - 写入 `decision:*`（属于协调者/approver 的收敛动作）

- **TUI/控制台显示**（若已存在全局控制台）：
  - 能在一个 Issue 的详情中看到 reviewer/qa 的最新判定与证据（按 `IssueRef + role` 聚合）。

## 5. 功能需求（PRD 级）

- FR-2.8-01：系统必须支持 `workflow.toml version = 2`，并对 `version = 1` fail-fast（给出明确迁移提示）。
- FR-2.8-02：系统必须支持将 role group 标记为“订阅者模式”（subscriber），并要求在 V2 配置中显式声明 `groups.*.mode/writeback`。
- FR-2.8-03：订阅者模式下，系统必须允许在 `assignee != lead-<role>` 时仍可派 worker（用于并行 reviewer/qa）。
- FR-2.8-04：订阅者 lead 写回必须是“comment-only”，不得修改 `state:*`、assignee、close、`decision:*`。
- FR-2.8-05：协调者 lead（默认 integrator）必须是 `state:*` 与 close 的唯一自动写者（single writer）。
- FR-2.8-06：同一 Issue 允许多个 role 同时有各自 `active_run_id`（按 `IssueRef + role` 幂等），迟到结果不得覆盖当前角色的 active run。
- FR-2.8-07：reviewer/qa 判定必须可被系统稳定读取（固定字段/枚举），不可只依赖自由文本。
- FR-2.8-08：当订阅者结果缺字段/不可解析/证据不足时，必须写回明确原因与 Next，但不得推进状态机。
- FR-2.8-09：必须有去重策略，避免订阅者在无新信息时反复写回噪声 comment。

## 6. 验收标准（DoD）

- AC-2.8-01：在 sqlite outbox 模式下，assignee=`lead-integrator` 且存在 `to:reviewer`/`to:qa` 的 Issue，reviewer-lead 与 qa-lead 均可并行派工并写回判定 comment。
- AC-2.8-02：reviewer/qa 写回不会改变 `state:*` 与 assignee；只有 `lead-integrator` 能推进 `state:*`/close。
- AC-2.8-03：重启后 cursor 可续跑，且不会因重复事件产生重复写回。
- AC-2.8-04：迟到结果（`run_id != active_run_id`）不会覆盖当前判定；可被查看但不影响闸门。
- AC-2.8-05：在“review changes requested / QA fail”场景下，协调者可基于判定生成唯一 FixList，并回流到实现角色。
- AC-2.8-06：当 `workflow.toml version = 1` 时，系统拒绝启动/拒绝运行 lead loop，并输出明确迁移提示。

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
- `docs/workflow/integrator-role.md`
- `docs/operating-model/quality-gate.md`
- `docs/workflow/label-catalog.md`

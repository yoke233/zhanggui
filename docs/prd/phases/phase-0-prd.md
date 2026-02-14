# PRD - Phase 0 文档与真源收敛

版本：v1.0  
状态：Draft  
负责人：PM / Lead / Architect  
目标阶段：Phase 0

## 1. 背景与问题

当前系统已经有完整讨论成果，但存在“术语与对象边界被误读”的风险：

- 协作对象、执行对象、代码对象、质量对象混在同一语境中。
- 部分字段曾出现命名漂移（例如 `IssueRef` 与历史命名残留）。
- 配置、模板、协议之间若缺乏一致约束，后续实现会出现双真源。

Phase 0 的目标不是交付自动化功能，而是先把真源和规则固定下来，确保 Phase 1 能稳定开工。

## 2. 目标与非目标

目标：

- 明确单一协作主键：`IssueRef`。
- 明确执行主键：`run_id`（而不是第二个 issue）。
- 明确配置真源：`<outbox_repo>/workflow.toml`。
- 明确模板真源：Issue/Comment/PR 模板文件。
- 输出“无歧义”的术语表与跨文档引用链路。

非目标：

- 不实现 Lead 常驻自动化。
- 不实现 PR/CI 自动回填。
- 不做多 backend 的代码适配，仅完成协议定义。

## 3. 目标用户与使用场景

目标用户：

- PM/PO：定义阶段目标、范围和验收。
- Tech Lead / Integrator：执行流程和证据回填。
- Worker 执行者：按模板回传产物。
- Reviewer：给出可审计判定。

场景：

- 团队首次启动该协作模型，需要一份“读完可直接执行”的基线文档集。

## 4. 范围（In Scope）

- 术语标准化：Issue、IssueRef、run_id、Evidence、Approver。
- 协议收敛：`docs/operating-model/*` 与 `docs/workflow/*` 的引用一致。
- 模板收敛：Issue/Comment/PR 模板字段定义稳定。
- 审批策略 V1：`approval.mode = any`。
- 本地模式最小启动路径：git + sqlite（文档级）。

## 5. 功能需求（PRD 级）

- FR-0-01：系统必须有唯一协作主键定义，并可跨 backend 映射。
- FR-0-02：系统必须明确执行主键 `run_id` 与协作主键解耦。
- FR-0-03：系统必须定义 Claim 真源为 `assignee` 字段。
- FR-0-04：系统必须给出本地模式与 forge 模式的一致语义映射。
- FR-0-05：系统必须给出阶段边界，防止 Phase 1 背负 Phase 3 复杂度。

## 6. 验收标准（DoD）

- AC-0-01：`IssueRef` 的来源和格式在文档中可精确定位并唯一解释。
- AC-0-02：文档中不存在“执行面第二个 issue”描述。
- AC-0-03：配置真源位置在文档中仅有一处定义，不冲突。
- AC-0-04：模板字段（IssueRef/Changes/Tests/Next）在协议文档与模板一致。
- AC-0-05：Phase 0/1/2/3 的边界在 `phases.md` 与 PRD 一致。

## 7. 里程碑

- M0.1：术语与主键定义冻结。
- M0.2：模板字段冻结。
- M0.3：阶段计划与启动路径冻结。
- M0.4：完成文档审查并进入 Phase 1。

## 8. 成功指标

- 指标 1：关键术语歧义问题数 = 0（评审结论）。
- 指标 2：跨文档冲突条目数 = 0（人工审查清单）。
- 指标 3：新成员阅读后可在 30 分钟内说明“IssueRef 与 run_id 区别”。

## 9. 风险与缓解

- 风险：术语继续漂移导致实现偏差。  
  缓解：将术语定义集中在 `outbox-backends.md` 与 `executor-protocol.md`，并在模板中直接引用。

- 风险：团队把 Phase 0 当“文档写作任务”而非“开工前置条件”。  
  缓解：把 AC 绑定到 Phase 1 开工门槛。

## 10. 依赖

- `docs/operating-model/outbox-backends.md`
- `docs/operating-model/executor-protocol.md`
- `docs/workflow/issue-protocol.md`
- `docs/workflow/templates/*`

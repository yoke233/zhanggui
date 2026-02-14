# PRD 文档总览

本目录用于沉淀产品经理视角的阶段性 PRD，目标是把现有协议文档转成可排期、可验收、可复盘的交付计划。

当前阶段 PRD：

- `docs/prd/phases/phase-0-prd.md`：Phase 0 文档与真源收敛
- `docs/prd/phases/phase-1-prd.md`：Phase 1 人工闭环（Local-First）
- `docs/prd/phases/phase-2-prd.md`：Phase 2 最小 Lead 自动化
- `docs/prd/phases/phase-3-prd.md`：Phase 3 质量与 PR/CI 自动化增强

埋点 PRD：

- `docs/prd/metrics/phase-1-instrumentation.md`：提前埋点规范（Phase 1 启动，支撑 Phase 4/5）

TDD 交付计划：

- `docs/prd/tdd/phase-1-test-spec.md`：Phase 1 验收测试规格（ATDD）
- `docs/prd/tdd/contract-tests.md`：协议契约测试规格
- `docs/prd/tdd/slicing-plan.md`：Red/Green/Refactor 实施切片计划

关联协议（真源）：

- `docs/operating-model/phases.md`
- `docs/operating-model/START-HERE.md`
- `docs/operating-model/executor-protocol.md`
- `docs/operating-model/outbox-backends.md`
- `docs/workflow/issue-protocol.md`
- `docs/workflow/templates/issue.md`
- `docs/workflow/templates/comment.md`

文档使用说明：

- PRD 负责回答“为什么做、做什么、怎么验收”。
- 协议文档负责回答“字段与流程如何定义”。
- 实现设计与代码任务拆分应从 PRD 派生，不反向替代 PRD。

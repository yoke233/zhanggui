# 阶段 PRD 索引

## 概览

- `Phase 0`：文档与真源收敛（先把概念和边界钉住）
- `Phase 1`：人工闭环（先交付，不自动化）
- `Phase 2`：最小 Lead 自动化（降人力、保一致）
- `Phase 2.1`：Lead 控制台（Bubble Tea TUI，提升运维效率）
- `Phase 2.5`：Reviewer-Lead 调度增强（补齐 review 自动回流）
- `Phase 3`：质量与 PR/CI 自动化增强（提效与可计算）

## 文档清单

- `docs/prd/phases/phase-0-prd.md`
- `docs/prd/phases/phase-1-prd.md`
- `docs/prd/phases/phase-2-prd.md`
- `docs/prd/phases/phase-2-1-prd.md`
- `docs/prd/phases/phase-2-5-prd.md`
- `docs/prd/phases/phase-3-prd.md`

## 评审建议顺序

1. `Phase 0`（定义是否清晰、有没有双真源）
2. `Phase 1`（是否能今天开工并闭环）
3. `Phase 2`（自动化边界是否克制）
4. `Phase 2.1`（操作面是否提升且不引入第二状态机）
5. `Phase 2.5`（reviewer-lead 调度边界是否清晰）
6. `Phase 3`（质量增强是否复用既有协议）

## 与协议文档的关系

- PRD 回答：为什么做、做什么、什么时候验收。
- 协议回答：字段是什么、流程怎么跑、系统如何互通。
- 代码实现必须同时满足 PRD 的 AC 与协议约束。

## 配套埋点

- `docs/prd/metrics/phase-1-instrumentation.md`

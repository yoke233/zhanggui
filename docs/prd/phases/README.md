# 阶段 PRD 索引

## 概览

- `Phase 0`：文档与真源收敛（先把概念和边界钉住）
- `Phase 1`：人工闭环（先交付，不自动化）
- `Phase 2`：最小 Lead 自动化（降人力、保一致）
- `Phase 2.1`：Lead 控制台（Bubble Tea TUI，操作面收敛）
- `Phase 2.5`：Reviewer-Lead 调度增强（评审队列收敛）
- `Phase 2.6`：并发执行隔离（Git Worktree，运行面稳定）
- `Phase 2.7`：真实 Worker 接入与 Wrapper 归一化（执行面统一）
- `Phase 2.8`：并行审查的协调者模型（Assignee 固定 + 单写者）
- `Phase 3`：质量与 PR/CI 自动化增强（提效与可计算）

## 文档清单

- `docs/prd/phases/phase-0-prd.md`
- `docs/prd/phases/phase-1-prd.md`
- `docs/prd/phases/phase-2-prd.md`
- `docs/prd/phases/phase-2-1-prd.md`
- `docs/prd/phases/phase-2-5-prd.md`
- `docs/prd/phases/phase-2-6-prd.md`
- `docs/prd/phases/phase-2-7-prd.md`
- `docs/prd/phases/phase-2-8-prd.md`
- `docs/prd/phases/phase-3-prd.md`

## 当前进度（建议维护在此处）

说明：

- PRD 的 `状态：Draft/Active/...` 表示文档生命周期，不直接等价于“功能是否已实现”。
- 若某阶段已在代码实现，建议在这里标记 `已实现`，并补充证据链接（PR/commit/issue）。

- 已实现：Phase 2.1、Phase 2.5、Phase 2.6（SQLite backend + Git worktree，首批 rollout=backend）
- 未实现：Phase 2.7、Phase 2.8（计划中）

## 评审建议顺序

1. `Phase 0`（定义是否清晰、有没有双真源）
2. `Phase 1`（是否能今天开工并闭环）
3. `Phase 2`（自动化边界是否克制）
4. `Phase 2.1`（操作面是否实用、是否保持单一真源）
5. `Phase 2.5`（review 队列是否可自动收敛）
6. `Phase 2.6`（并发隔离是否稳定、是否保持 run 幂等）
7. `Phase 2.7`（多 worker 输出是否可统一归一化）
8. `Phase 2.8`（并行审查是否保持单写者、是否能稳定汇总）
9. `Phase 3`（质量增强是否复用既有协议）

## 与协议文档的关系

- PRD 回答：为什么做、做什么、什么时候验收。
- 协议回答：字段是什么、流程怎么跑、系统如何互通。
- 代码实现必须同时满足 PRD 的 AC 与协议约束。

## 配套埋点

- `docs/prd/metrics/phase-1-instrumentation.md`

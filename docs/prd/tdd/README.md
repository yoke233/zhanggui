# TDD 文档索引

本目录用于把阶段 PRD 转成可执行的测试驱动交付计划。

文档清单：

- `docs/prd/tdd/phase-1-test-spec.md`：Phase 1 验收测试规格（ATDD）
- `docs/prd/tdd/contract-tests.md`：协议契约测试规格（IssueRef/run_id/assignee 等）
- `docs/prd/tdd/slicing-plan.md`：Red/Green/Refactor 实施切片计划

建议使用顺序：

1. 先读 `phase-1-test-spec.md`，冻结验收场景。
2. 再读 `contract-tests.md`，冻结字段语义与边界。
3. 最后按 `slicing-plan.md` 逐切片实现。

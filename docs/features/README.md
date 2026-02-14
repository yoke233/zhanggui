# Feature 文档入口

本目录用于存放“可执行、可定稿、可追溯”的功能文档。每个功能一个目录，不混写。

目录规则：

- 模板：`docs/features/_template/`
- 功能文档：`docs/features/<feature-id>/`

每个 `feature-id` 至少包含：

- `requirement.md`：需求与业务边界
- `prd.md`：交付计划与验收口径
- `tech-spec.md`：技术设计与测试方案

推荐流程：

1. 从模板复制到 `docs/features/<feature-id>/`
2. 先写 `requirement.md`，确认业务口径
3. 再写 `prd.md`，确定范围、里程碑、验收
4. 最后写 `tech-spec.md`，作为实现依据
5. 审批通过后将文档状态更新为 `Active`

关联规范：

- `docs/standards/naming-and-ids.md`
- `docs/standards/doc-lifecycle.md`
- `docs/standards/repo-conventions.md`

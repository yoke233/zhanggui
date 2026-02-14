# 项目规范总览

本目录用于存放 outbox_repo 的长期规范文件，避免规则散落在聊天或临时文档中。

文档清单：

- `docs/standards/naming-and-ids.md`：命名与标识规则（含 IssueRef / run_id 约定）
- `docs/standards/approval-and-gates.md`：审批与闸门规则
- `docs/standards/label-taxonomy.md`：标签分类与治理
- `docs/standards/doc-lifecycle.md`：文档生命周期与定稿流程
- `docs/standards/repo-conventions.md`：outbox_repo 目录与仓库约定

规则优先级建议：

1. `workflow.toml`（项目运行配置真源）
2. `docs/standards/*`（跨文档稳定规则）
3. `docs/features/*`（具体 feature 的需求/设计）
4. 其它说明文档

配套目录：

- `docs/features/README.md`：功能级文档入口（每个 feature 一套 requirement/prd/tech-spec）
- `docs/features/_template/`：功能文档模板

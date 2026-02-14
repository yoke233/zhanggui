# Outbox Repo 目录约定

版本：v1.0  
状态：Draft  
Owner：Lead / Architect

## 1. 目标

- 明确 outbox_repo 应承载哪些内容。
- 明确“定稿需求、PRD、技术 spec”的固定落位。
- 支持单仓和多仓项目复用同一套目录约定。

## 2. 推荐目录骨架

```text
<outbox_repo>/
  workflow.toml              # 项目运行配置真源
  mailbox/
    issue.md                 # Issue 模板
    comment.md               # Comment 模板
  state/
    outbox.sqlite            # SQLite outbox（本地模式；建议不提交）
  docs/
    operating-model/         # 三层模型与阶段策略
    workflow/                # 协作协议与模板
    standards/               # 跨项目稳定规范
    features/                # 每个 feature 的需求/PRD/技术方案
      _template/             # feature 文档模板
      <feature-id>/          # 单个功能目录（定稿产物）
        requirement.md
        prd.md
        tech-spec.md
  contracts/                 # 可选：接口契约（proto/openapi/jsonschema）
  scripts/                   # 可选：本地自动化脚本
```

## 3. 文档落位规则

- 长期稳定规则写入 `docs/standards/*`。
- 功能级定稿文档只放在 `docs/features/<feature-id>/*`。
- 讨论记录与执行证据回填到对应 `IssueRef`，不替代文档定稿。

## 4. feature-id 命名

推荐格式：

- `<domain>-<short-name>`
- 示例：`billing-refund-v1`、`agent-lead-mode`

约束：

- 全小写
- 允许连字符 `-`
- 禁止空格和中文路径名

## 5. 与代码仓关系

- 单仓项目：`role_repo` 只指向一个代码仓，未启用角色不派发。
- 多仓项目：由 `workflow.toml` 的 `role_repo` 指向不同仓（如 frontend/backend）。
- outbox_repo 负责协作与配置，不强制承载所有业务代码。

## 6. 本地模式（git + sqlite）

- 即使不接 GitHub/GitLab，也保持同一目录结构。
- `IssueRef` 可使用 `local#<issue_id>`。
- 审批/状态/证据仍写回 outbox 线程，保证可回放。

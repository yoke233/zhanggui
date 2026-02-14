# Local-First Bootstrap (只用 git + sqlite 启动项目)

目标：在不接入 GitHub/GitLab 的情况下，用本地 git + sqlite 跑通 Phase 1 闭环，并为 Phase 2 自动化预留接口。

适用项目类型：A（单仓、后端-only）

## 1) 真源选择（本地模式）

- Spec 真源：Issue 的 Spec 区块（或 `spec.md` + `SpecRef`）
- Outbox backend：SQLite（见 `docs/operating-model/outbox-backends.md`）
- 交付真源：git commit（本地没有 PR URL 时，用 commit hash/branch 作为 Changes）
- 质量真源（本地替代）：Outbox 的结构化质量判定 comment（见 `docs/operating-model/quality-gate.md` 的无 forge 模式）

## 2) 目录与文件（最小集合）

在项目 repo 根目录（也是 `<outbox_repo>`）准备：

- `<outbox_repo>/.agents/workflow.toml`（唯一配置真源）
- `<outbox_repo>/.agents/mailbox/issue.md`（Issue 模板）
- `<outbox_repo>/.agents/mailbox/comment.md`（Comment 模板）
- `<outbox_repo>/.agents/state/outbox.sqlite`（SQLite outbox，建议 gitignore，不提交）

推荐 `.gitignore` 添加：

```text
.agents/state/
*.sqlite
```

## 3) workflow.toml（本地 sqlite outbox 示例）

最小示例（单仓）：

```toml
version = 1

[outbox]
backend = "sqlite"
path = ".agents/state/outbox.sqlite"

[approval]
mode = "any"
approvers = ["lead-backend"] # 本地模式用 actor id 字符串即可

[roles]
enabled = ["backend", "qa", "integrator", "recorder"]

[repos]
main = "."

[role_repo]
backend = "main"
qa = "main"
integrator = "main"
recorder = "main"

[groups.backend]
role = "backend"
max_concurrent = 4
listen_labels = ["to:backend"]

[groups.qa]
role = "qa"
max_concurrent = 2
listen_labels = ["to:qa", "state:review"]

[groups.integrator]
role = "integrator"
max_concurrent = 1
listen_labels = ["to:integrator", "state:review"]

[groups.recorder]
role = "recorder"
max_concurrent = 1
listen_labels = ["to:recorder"]

[flow]
require_claim_before_work = true
close_issue_requires_decision = true
auto_block_when_dependency_open = true
auto_unblock_when_dependency_closed = true
```

说明：

- 本地模式下 `outbox.backend=sqlite`，`path` 相对路径以 `workflow.toml` 所在目录解析。
- `approvers` 在本地模式下是 actor id（字符串），不再限定为 GitHub username。

## 4) Issue 的“创建/领取/写回”怎么做（本地）

本地 outbox 的 Issue/Event 语义与 GitHub 一致，只是落在 SQLite：

1. 创建 issue
2. 设置 labels（`to:*`、可选 `state:*`）
3. claim：设置 `assignee`（事实源）
4. 追加事件：按 comment 模板追加 `events.body`
5. close：将 `is_closed=1`

你可以先手工操作（例如用 sqlite CLI 或简单脚本），等 Phase 2 再做自动化。

## 5) 没有 PR 时如何记录 Changes（本地 git）

在 `docs/workflow/templates/comment.md` 的 `Changes` 区块中：

- `PR: none`
- `Commit: <commit hash>`（推荐附带分支名）

示例：

- `Commit: git:1a2b3c4 (branch: feature/foo)`

Integrator 合并后再写一条事件：

- `Commit: git:<merge commit sha> (branch: main)`

## 6) Reviewer 结论如何记录（无 forge 模式）

本地没有 GitHub PR review 事件时，Reviewer 的判定必须写成可计算事件：

- Reviewer 在 Issue 追加一条结构化 comment（由 Lead 单写者规范化写回也可）
- 使用 comment 模板的 `Review.Result = approved|changes_requested`（见模板扩展建议）

Phase 1 最小可行做法：

- Reviewer 在 comment 的 `Summary` 中明确写 `review:approved` 或 `review:changes_requested`
- Phase 2 再由控制平面统一成结构化字段

## 7) 切换 worker（本地模式同样要做幂等）

即使本地跑，也必须遵守：

- 每次 spawn 生成新 `RunId`：`<YYYY-MM-DD>-<role>-<seq>`
- 只接受当前 `active_run_id` 的结果写回 Outbox

见：`docs/operating-model/executor-protocol.md`

# START HERE (Phase 1: Local-First, git + sqlite)

目标：用最小依赖把协作闭环跑起来（本地只有 git + sqlite；不接 GitHub/GitLab；不写自动化代码）。

真源（必须记住）：
- 配置真源：`<outbox_repo>/workflow.toml`（只保留这一份）
- 协作真源：Issue（SQLite；`IssueRef = local#<issue_id>`）
- 交付真源：git commit（没有 PR 时用 `git:<sha>`）
- 质量真源：可审计证据（本地无 PR review/CI 时，用 Outbox comment 承接 Reviewer 判定）

准备文件（只在 outbox repo）：
- `<outbox_repo>/workflow.toml`（SQLite 例子见 `docs/operating-model/local-first.md`）
- `<outbox_repo>/mailbox/issue.md`（基线：`docs/workflow/templates/issue.md`）
- `<outbox_repo>/mailbox/comment.md`（基线：`docs/workflow/templates/comment.md`）
- 建议不提交：`<outbox_repo>/state/outbox.sqlite`（并 gitignore `state/` 与 `*.sqlite`）

开一个任务 Issue（本地 Outbox）：
- Issue body 用 issue 模板；labels 至少一个 `to:<role>`；得到 `IssueRef = local#<id>`
- SQLite schema/语义：`docs/operating-model/outbox-backends.md`

领取与开工（Hard 条件，避免死锁）：
- issue open + `assignee` 已设置 + 无 `needs-human` + `DependsOn` 已满足（或 `none`）
- `state:*` 在 Phase 1 是 Soft（推荐但不阻塞开工）

Worker 回传什么（允许自然语言，但必须可追溯）：
- `IssueRef` + `Commit/PR` + `Tests`（或明确 `n/a`）+ 阻塞/问题（如有）

Lead/Integrator/Recorder 写回什么（单写者规范化）：
- 用 comment 模板写回结构化事件；`Trigger` 推荐 `workrun:<run_id>`（见 `docs/operating-model/executor-protocol.md`）

Done / Close（最小闭环）：
- Outbox 有结构化 comment（Changes + Tests Evidence；必要时含 review 判定）+ close issue


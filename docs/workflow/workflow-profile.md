# 项目画像：`.agents/workflow.toml`

## 目标

让“这个项目有哪些 repo、有哪些角色、谁监听谁、并发上限是多少、Outbox 放哪、谁能盖章”由**项目自己声明**，而不是写死在系统或固定某个仓库结构里。

这使得：

- 后端-only 项目可以只有一个 repo，且不需要 frontend 角色
- 多 repo 项目可以启用 `frontend/backend/contracts/...` 等角色与路径映射
- 同一套 goclaw 系统可以服务多个项目，按项目动态加载工作流约定

## 文件位置与作用域

- V1.1 约定放在 Outbox repo 根目录的：`<outbox_repo>/.agents/workflow.toml`
  - 它属于“repo 层 overlay”，优先级最高（符合 `.agents/` 的设计哲学）
  - V1.1 约定：本项目只保留 Outbox repo 内的这一份 `workflow.toml` 作为唯一配置真源；其它 repo 不应再放 `workflow.toml`（避免分叉）

说明：

- 本文中的 `<outbox_repo>` 指包含 `.agents/workflow.toml` 的那个 repo 的本地目录。
  - 它是项目的“配置锚点目录”（single source of config truth）。
  - 在 GitHub/GitLab backend 下，它通常也就是承载 Issue 的 repo（例如 `contracts/`）。
  - 在 SQLite backend 下，它通常就是你的项目仓库根目录（单仓项目），Outbox DB 的相对路径以此目录解析。

## 多环境运行（重要说明）

本次讨论明确：Lead 允许在多环境运行（不同机器/不同 OS/本地或服务器）。

因此在落地时建议把配置拆成两类：

- 项目级稳定配置（适合提交到 repo）：`outbox/approval/roles/labels/groups/flow`
- 环境相关配置（通常不适合提交）：`repos`（本地路径映射）、凭证、缓存目录

V1 文档仍使用绝对路径作为示例，是为了表达 `repo_dir` 的语义；但实际项目建议至少做到：

- V1.1 选择：不引入本地覆盖文件（例如 `workflow.local.toml`），而是通过“标准目录布局 + 相对路径”实现多环境一致。
- `repos.*` 建议写相对路径，并约定“相对路径以 `workflow.toml` 所在目录为基准解析”。

多 repo 的推荐布局（Outbox repo 通常是 `contracts`）：

```text
<project_root>/
  contracts/   # outbox repo (contains .agents/workflow.toml)
  backend/
  frontend/
```

示例（在 `contracts/.agents/workflow.toml`）：

```toml
[repos]
contracts = "."
backend = "../backend"
frontend = "../frontend"
```

更完整的 V1.1 说明见：`docs/workflow/v1.1.md`。

## 推荐字段（V1）

下面不是“必须实现的代码接口”，而是工作流协议的最小集合；未来可以增量扩展。

```toml
version = 1

[outbox]
# Outbox backend（承载系统）：github | gitlab | sqlite | ...
backend = "sqlite"
path = ".agents/state/outbox.sqlite"
#
# backend=github|gitlab 时必须：
# repo = "org/contracts"   # 例："org/contracts" 或 "org/backend"
#
# backend=sqlite 时必须：path（建议放在 `.agents/state/` 并 gitignore）

[memory]
# 项目级记忆根目录（建议与 repo 解耦：单 repo/多 repo 都共用一个 root）
# 参考：docs/workflow/memory-layout.md
root = "D:\\workspace\\_goclaw_memory\\my-project"
layout = "project+roles" # 预留：project-only | project+roles
auto_inject_topk = 3

[approval]
mode = "any" # V1 只实现 any；后续可扩展 all/quorum/staged
approvers = ["agent-architect", "agent-integrator", "yoke233"]

[roles]
# 本项目启用的角色集合；不需要的角色不要写进来
enabled = ["architect", "backend", "frontend", "qa", "integrator", "recorder"]

[repos]
# 逻辑 repo 名 -> 本地路径（给 sessions_spawn 的 repo_dir 用）
# V1.1 推荐：使用相对路径，并以 workflow.toml 所在目录为基准解析
contracts = "."
backend = "../backend"
frontend = "../frontend"

[role_repo]
# 角色 -> 逻辑 repo 名（从 [repos] 映射到 repo_dir）
architect = "contracts"
backend = "backend"
frontend = "frontend"
qa = "backend"
integrator = "backend"
recorder = "contracts"

[labels]
# 路由标签：决定“谁监听谁”
routing = ["to:architect", "to:backend", "to:frontend", "to:qa", "to:integrator", "to:recorder"]
# 状态标签：用于队列与开工判断
states = ["state:todo", "state:doing", "state:blocked", "state:review", "state:done"]
# 决策标签：用于 Accepted Gate
decisions = ["decision:proposed", "decision:accepted", "decision:rejected"]
# 控制标签：需要人类介入时的硬闸门
controls = ["needs-human", "autoflow:off"]
# 类型与优先级：用于队列管理与统计（建议启用，字段稳定）
types = ["kind:task", "kind:bug", "kind:question", "kind:proposal", "kind:blocker"]
priority = ["prio:p0", "prio:p1", "prio:p2", "prio:p3"]

[groups.architect]
role = "architect"
max_concurrent = 1
listen_labels = ["to:architect", "decision:proposed"]

[groups.backend]
role = "backend"
max_concurrent = 4
listen_labels = ["to:backend"]

[groups.frontend]
role = "frontend"
max_concurrent = 3
listen_labels = ["to:frontend"]

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
# 是否强制“先 claim（assignee 已设置）再开工（spawn worker/开 PR）”
require_claim_before_work = true
# 若为 true：当 issue 需要盖章（例如带 `decision:proposed`）时，必须先 `decision:accepted` 才允许 close/done。
# 普通 `kind:task` 若不打 `decision:*`，则不受此项影响。
close_issue_requires_decision = true
# 依赖未满足时是否自动转入 blocked
auto_block_when_dependency_open = true
# 依赖满足后是否自动解除 blocked（恢复到 doing 或 todo，规则见 issue-protocol）
auto_unblock_when_dependency_closed = true
```

说明：

- `groups.*.max_concurrent` 是项目层的“软并发上限”（用于调度/统筹）。
- goclaw 运行时本身也有“硬并发上限”（见 `agents.defaults.subagents.role_max_concurrent`）。
- 推荐做法：硬上限设为你机器能承受的最大值；项目画像里再做“软限制”，以适配不同项目的规模。
- `groups.*.listen_labels` 是“订阅过滤器”，用于决定该 group 要监听哪些 issue：
  - V1 建议采用 AND 语义：issue 必须同时包含列表中的所有 labels 才算命中。
  - 额外路由仍然生效：被 assignee 指派、或被直接 @mention 的 issue/comment，应当视为命中（见 `docs/workflow/issue-protocol.md`）。
- V1 建议做一致性校验（避免调度歧义）：
  - `roles.enabled` 为真源：未启用的角色不应被任何 group 激活/监听。
  - 对每个 `roles.enabled` 的 role：必须存在 `role_repo.<role>` 映射。
  - 对每个 `roles.enabled` 的 role：建议至少存在一个 `groups.*` 且其 `role = "<role>"`（缺失会导致并发/监听语义不明确）。
  - 存在 `groups.*.role` 但该 role 未在 `roles.enabled` 中：建议视为配置错误（或至少忽略并告警），避免误激活未启用角色。

## 后端-only 项目示例（单 repo）

```toml
version = 1

[outbox]
backend = "sqlite"
path = ".agents/state/outbox.sqlite"

[approval]
mode = "any"
approvers = ["agent-architect", "yoke233"]

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

## 多 repo + contracts 项目示例

```toml
version = 1

[outbox]
backend = "github"
repo = "org/contracts"

[approval]
mode = "any"
approvers = ["agent-architect", "agent-integrator"]

[roles]
enabled = ["architect", "backend", "frontend", "qa", "integrator", "recorder"]

[repos]
contracts = "."
backend = "../backend"
frontend = "../frontend"

[role_repo]
architect = "contracts"
backend = "backend"
frontend = "frontend"
qa = "backend"
integrator = "backend"
recorder = "contracts"

[groups.architect]
role = "architect"
max_concurrent = 1
listen_labels = ["to:architect", "decision:proposed"]

[groups.backend]
role = "backend"
max_concurrent = 4
listen_labels = ["to:backend"]

[groups.frontend]
role = "frontend"
max_concurrent = 3
listen_labels = ["to:frontend"]

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

## 模板文件（V1 固定要求）

因为 mailbox 采用固定模板，模板文件应当存放在 repo 的 `.agents/` 下并受代码评审：

- Issue 主帖模板：`<outbox_repo>/.agents/mailbox/issue.md`
- Comment 模板：`<outbox_repo>/.agents/mailbox/comment.md`

mailbox skill 必须读取模板并填充占位符，保证所有人看到的结构一致。

可以先用 `docs/workflow/templates/issue.md` 与 `docs/workflow/templates/comment.md` 作为初始模板拷贝源。

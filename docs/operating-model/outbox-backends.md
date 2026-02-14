# Outbox Backends (承载系统抽象)

目标：将“协作真源线程（Issue）”与具体承载系统解耦，允许在不同阶段选择不同 backend：

- 本地/离线：SQLite（单机、最小依赖）
- 团队协作：GitHub/GitLab Issues（共享、审计、权限）
- 更强系统：MySQL/Postgres（自建、可扩展）

本文件只定义**协议需要的最小能力集合**与 V1 推荐的 SQLite schema。

## 1) Issue / Event 抽象

- Issue：一个可追加事件的协作条目（唯一主键；语义等价于“issue 线程”）
- Event：Issue 中的一条追加记录（append-only）

### 1.1 IssueRef（规范定义，V1 固定）

`IssueRef` 不是“随便一个 id”，而是跨 backend 的统一引用字符串。V1 约定如下：

- GitHub：`<owner>/<repo>#<number>`
  - `number` 使用 GitHub Issue 的编号（网页显示的 `#123`）
  - 不使用 REST `id`、GraphQL `node_id`
- GitLab：`<group>/<project>#<iid>`
  - 使用 Issue 的 `iid`（项目内编号）
  - 不使用全局 `id`
- SQLite：`local#<issue_id>`
  - `issue_id` 对应 `issues.issue_id`

唯一性与范围：

- GitHub/GitLab：`owner/repo(or project)+编号` 组合保证全局可定位
- SQLite：`local#<issue_id>` 在单个 outbox DB（`outbox.path`）内唯一
- 迁移 backend 时，`IssueRef` 可能变化，但语义保持“同一个协作 Issue”

映射示例：

- GitHub：Issue=Issue，Event=Comment
- GitLab：Issue=Issue，Event=Note
- SQLite：Issue=issues 表一行，Event=events 表一行

## 2) 最小能力集合（接口语义）

控制平面只依赖以下语义（不要求具体 API 形态）：

- `ListIssues(filter)`：按 labels/state/assignee/open 过滤
- `GetIssue(issue_ref)`：取 issue body、labels、assignee、open/closed
- `CreateIssue(title, body, labels)`：创建新 issue
- `AppendEvent(issue_ref, actor, body)`：追加事件（comment/note）
- `MutateIssue(issue_ref, patch)`：修改 labels、assignee、open/close

约束：

- Event 必须是 append-only（不可修改历史事件），保证可回放。
- Issue 的“当前状态”可以通过字段/labels 表示，但不应破坏可回放性。

## 3) 本地 backend：SQLite (V1 推荐)

适用场景：

- 你刚开始想“先跑通协议与闭环”，不对接 GitHub/GitLab。
- 你只有 git + sqlite，仍然希望协作线程可回放、有 cursor、有去重。

限制：

- SQLite outbox 默认是单机状态，不是共享真源（除非你额外做同步/共享文件系统）。
- 多人协作时建议尽早切换到 GitHub/GitLab 或自建 DB。

### 3.1 IssueRef / EventRef 格式

推荐格式（ASCII、可读）：

- `IssueRef`：`local#<issue_id>`
- `EventRef`：`local#<issue_id>/e<event_id>`

示例：

- `local#12`
- `local#12/e34`

### 3.2 最小 SQLite schema（建议）

控制平面实现可以用任意等价 schema；下面是 V1 推荐 schema（便于查询与索引）。

表：`issues`

- `issue_id INTEGER PRIMARY KEY AUTOINCREMENT`
- `title TEXT NOT NULL`
- `body TEXT NOT NULL`  (Issue 主帖 Markdown)
- `assignee TEXT NULL`  (claim 真源：谁负责，语义等价于 GitHub assignee)
- `is_closed INTEGER NOT NULL DEFAULT 0`
- `created_at TEXT NOT NULL`  (ISO8601)
- `updated_at TEXT NOT NULL`  (ISO8601)
- `closed_at TEXT NULL`       (ISO8601)

表：`issue_labels`

- `issue_id INTEGER NOT NULL`
- `label TEXT NOT NULL`
- `PRIMARY KEY(issue_id, label)`
- `FOREIGN KEY(issue_id) REFERENCES issues(issue_id) ON DELETE CASCADE`

表：`events`

- `event_id INTEGER PRIMARY KEY AUTOINCREMENT`
- `issue_id INTEGER NOT NULL`
- `actor TEXT NOT NULL`        (事件作者；本地可用 `whoami` 或自定义 ID)
- `body TEXT NOT NULL`         (Comment 模板渲染后的 Markdown)
- `created_at TEXT NOT NULL`   (ISO8601)
- `FOREIGN KEY(issue_id) REFERENCES issues(issue_id) ON DELETE CASCADE`

可选表：`outbox_kv`（用于 cursor / active_run_id 等少量控制平面状态）

- `key TEXT PRIMARY KEY`
- `value TEXT NOT NULL`
- `updated_at TEXT NOT NULL`

推荐索引（可选）：

- `CREATE INDEX idx_issues_closed ON issues(is_closed);`
- `CREATE INDEX idx_issues_assignee ON issues(assignee);`
- `CREATE INDEX idx_issue_labels_label ON issue_labels(label);`
- `CREATE INDEX idx_events_issue ON events(issue_id, event_id);`

### 3.3 identity（actor）说明

本地模式没有 GitHub 的 `user.login`，因此：

- `actor` 只是一个字符串 ID
- V1 本地模式默认信任本机执行环境（不做签名/强认证）

建议：

- Lead/控制平面进程的 actor 使用固定值（例如 `lead-backend`）
- 人类 reviewer 使用 `whoami` 或配置的名字

未来增强：

- 可以引入 GPG 签名、或将 outbox 放到支持认证/审计的服务端 DB。

## 4) 迁移建议：SQLite -> GitHub/GitLab

当需要多人协作或需要外部审计时，建议迁移到 Issues 系统：

- IssueRef 会变化（`local#12` -> `org/repo#123`）
- 但协议不变：Issue/Event/labels/assignee/close 语义保持一致

迁移最低要求：

- 将 `issues.title/body` 转成 Issue title/body
- 将 `events.body` 按顺序追加为 Comment/Note
- 将 labels/assignee/close 状态同步过去

# GitHub 集成 — 设计文档

> **重要：GitHub 集成是可选增强，不是必需组件。** 系统的核心功能（任务拆解、审核、DAG 调度、Pipeline 执行）完全在本地运行（SQLite + 本地 Git）。GitHub 集成提供双向状态同步：GitHub Issue/PR 的变化驱动 Pipeline，Pipeline 的执行进度反映到 Issue/PR。启用后，用户可以在 GitHub 上通过评论驱动整个开发流程。
>
> **阶段：P3**（P0~P2 已完成，不需要 GitHub 即可完整运行）
>
> **当前进度**：Wave1（基础设施）✅ 已完成——GitHub 认证客户端、Service 操作层（Issue/PR/Label CRUD）、Webhook 端点与 HMAC-SHA256 签名验证、多项目路由、配置体系与工厂选择逻辑。Wave2（核心业务：tracker-github、scm-github、Issue 触发、斜杠命令）🔧 进行中。review-github-pr 为可选增强，不阻塞 P3 Done——默认审核仍使用 review-ai-panel。

## 前置条件：基础设施抽象

GitHub 集成通过两个插件槽位接入系统，不侵入核心逻辑：

| 插件槽位 | GitHub 实现 | 默认实现（无 GitHub） | 职责 |
|---------|------------|---------------------|------|
| Tracker | `tracker-github` | `tracker-local`（空实现） | 将内部 Issue 同步为 GitHub Issue + Label 管理 |
| ReviewGate | `review-github-pr` | `review-ai-panel`（两阶段 AI 审核） | 将 Issue 审核通过 GitHub PR 进行 |

> 注：`tracker-linear` 作为 Tracker 扩展实现属于 P4 范围，见 [spec-overview.md](spec-overview.md) 的插件槽位与路线图；本文件聚焦 GitHub 场景，不展开 Linear 细节。

核心逻辑（DAG Scheduler、Pipeline Engine）只依赖抽象接口，不直接调用 GitHub API。

### 与 Secretary Layer 的关系

启用 GitHub 集成后，Secretary Layer 的行为增强如下：

| 功能 | 无 GitHub | 有 GitHub |
|------|----------|----------|
| Issue 存储 | SQLite `issues` 表 | SQLite + 同步为 GitHub Issue |
| 依赖管理 | DAG Scheduler 本地管理 | 本地管理 + Label 镜像（`depends-on-#N`、`ready`、`blocked`） |
| 审核方式 | 两阶段 AI 审核 | 可选：AI 审核 或 GitHub PR 审核 |
| 状态可视化 | Workbench Board View | Workbench + GitHub Issue 标签 |
| Pipeline 触发 | DAG Scheduler 自动创建 | DAG Scheduler 或 GitHub Issue 事件触发 |

**GitHub Issue 和 Label 是内部 Issue 状态的镜像，不是 source of truth。** DAG Scheduler 在本地完成所有调度决策后，通过 Tracker 插件（`tracker-github`）将状态同步到 GitHub。

## 一、Webhook 监听

### 需要监听的事件

| 事件 | 触发动作 |
|---|---|
| `issues.opened` | 如果 auto_trigger 开启且标签匹配，创建 Pipeline |
| `issues.labeled` | 检查标签映射，决定是否创建 Pipeline |
| `issue_comment.created` | 解析斜杠命令，执行对应操作（只处理 Issue 主评论区，不处理 PR review comment） |
| `pull_request_review.submitted` | 如果 PR 关联了 Pipeline，处理 review 结果 |
| `pull_request.closed` + merged | Pipeline 标记完成 |

### Webhook 安全

- 使用 `X-Hub-Signature-256` 验证请求来源
- webhook_secret 配置在项目级配置中（每个仓库可使用不同的 secret，避免单一 secret 多仓库共用的安全隐患）
- 验证失败返回 401，不处理

### 多项目路由

一个 Orchestrator 实例可能管理多个 GitHub 仓库。路由规则：

```
Webhook 请求
  → 提取 repository.full_name (如 "user/app-a")
  → 在 Project 注册表中查找匹配的项目
  → 找到 → 路由到该项目的处理逻辑
  → 未找到 → 忽略（记录日志）
```

### Webhook 事件序列化

同一 Issue 的 webhook 事件必须串行处理，避免并发竞态（如两次 `issues.labeled` 同时触发导致重复创建 Pipeline）：

- 使用 per-issue mutex：`sync.Map` 存储 `{repo}#{issue_number}` → `*sync.Mutex`
- 同一 Issue 的事件排队处理，不同 Issue 的事件可并发
- mutex 在 Issue 关闭或 Pipeline 完成后清理（延迟 5 分钟，防止尾部事件）

项目配置中绑定 GitHub 仓库：

```yaml
projects:
  - id: app-a
    repo: /home/you/projects/app-a
    github:
      owner: your-username
      repo: app-a
      webhook_enabled: true
```

## 二、Issue → Pipeline 触发

### 两种模式

GitHub Issue 可以在两种模式下触发执行：

**模式 A — 独立 Issue（不经过 Secretary Layer）：**
外部创建的 GitHub Issue（人工创建或其他工具创建），直接映射为单个 Pipeline。适合简单任务。

**模式 B — Tracker 同步的 Issue（经过 Secretary Layer）：**
由 `tracker-github` 插件从内部 Issue 同步创建的 GitHub Issue。内部 Issue 来源于用户在 Chat 中指示 Secretary 生成计划文件后，经两阶段 AI 审核（Per-Issue 审核 + Cross-Issue 依赖分析）通过的需求。这类 Issue 的生命周期由 DAG Scheduler 管理，GitHub 上的状态变更会通过 Webhook 回传给 Scheduler。详见 [spec-secretary-layer.md](spec-secretary-layer.md)。

### 模式 A 的触发条件

满足以下任一条件时自动创建 Pipeline：

1. **标签触发**（推荐）：Issue 被打上配置中映射的标签
2. **评论触发**：有人在 Issue 中评论 `/run` 或 `/run {template}`
3. **手动触发**：通过 Workbench 或 TUI 手动关联 Issue 创建 Pipeline

### 标签 → 模板映射

```yaml
github:
  label_mapping:
    "type: feature": full
    "type: enhancement": standard
    "type: bug": quick
    "type: hotfix": hotfix
    "priority: urgent": hotfix    # 也可以用优先级标签
```

规则：
- 如果 Issue 同时有多个匹配标签，取最重的模板（full > standard > quick > hotfix）
- 如果没有匹配标签但评论了 `/run`，用 AI 推断模板
- 已有关联 Pipeline 的 Issue 不重复创建（幂等）：数据库用 `UNIQUE(project_id, issue_number)` 约束 + `INSERT ... ON CONFLICT IGNORE`，确保并发 webhook 不会创建重复 Pipeline

### Pipeline 创建后的 Issue 更新

创建成功后在 Issue 中添加评论：

```markdown
🤖 **AI Workflow Orchestrator**

Pipeline 已创建：`pipeline-20260228-abc123`
模板：`quick`（由标签 `type: bug` 推断）

阶段：requirements → worktree_setup → implement → code_review → fixup → merge → cleanup

使用斜杠命令控制流程：
- `/approve` — 审批当前阶段
- `/reject {stage} {reason}` — 回退到指定阶段
- `/status` — 查看当前状态
- `/abort` — 终止
```

同时给 Issue 加上标签 `pipeline: active`。

## 三、斜杠命令

### 命令列表

| 命令 | 效果 | 示例 |
|---|---|---|
| `/run [template]` | 创建 Pipeline | `/run full`、`/run`（自动推断） |
| `/approve` | 审批当前等待中的阶段 | `/approve` |
| `/reject <stage> [reason]` | 回退到指定阶段 | `/reject implement 数据模型需要重新设计` |
| `/status` | 查看当前状态 | `/status` |
| `/abort` | 终止 Pipeline | `/abort` |

以上 5 个命令为 **P3** 核心命令。以下命令延后到 P4+：

| 命令 | 效果 | 目标阶段 |
|---|---|---|
| `/modify <feedback>` | 带反馈重跑当前阶段 | P4 |
| `/skip` | 跳过当前阶段 | P4 |
| `/rerun` | 重跑当前阶段 | P4 |
| `/switch <role>` | 切换角色并重跑 | P4 |
| `/pause` / `/resume` | 暂停/恢复 Pipeline | P4 |
| `/logs [stage]` | 查看某阶段日志摘要（底层使用 Pipeline Logs API） | P4 |

### 命令解析规则

- 只识别评论第一行以 `/` 开头的内容
- 命令不区分大小写
- 默认按 `author_association` 做权限判定；若评论者在 `authorized_usernames` 白名单中则覆盖放行
- 无法识别的命令回复帮助信息

### 权限控制

优先使用 `issue_comment` 事件中的 `author_association` 字段进行权限判断，无需额外 API 调用：

| `author_association` 值 | 含义 | 可用命令 |
|---|---|---|
| `OWNER` / `MEMBER` | 仓库 owner 或组织成员 | 所有命令 |
| `COLLABORATOR` | 仓库协作者 | 所有命令 |
| `CONTRIBUTOR` | 曾提交过代码 | `/approve` `/reject` `/status` |
| `NONE` / `FIRST_TIMER` | 无关联 | 仅 `/status` |

此外支持显式白名单覆盖：

```yaml
github:
  authorized_usernames:
    - your-username
    - teammate-a
```

白名单中的用户拥有所有命令权限，不受 `author_association` 限制。

## 四、Pipeline → Issue 状态同步

### 标签更新

每个阶段变化时更新 Issue 标签：

```
进入阶段 → 先添加新 status 标签 → 再移除旧 status 标签（容错顺序）
```

标签更新失败只记录 warning 日志，不阻塞 Pipeline 执行（标签是辅助信息，不影响核心流程）。

标签命名规范：`status: {stage_name}`

```
status: requirements
status: worktree_setup
status: implement
status: code_review
status: fixup
status: e2e_test
status: merge
status: cleanup
pipeline: active        ← Pipeline 运行中始终保留
pipeline: done          ← 完成后替换 active
pipeline: failed        ← 失败后替换 active
```

### 评论更新

不是每个阶段都评论（避免刷屏），只在关键节点：

| 时机 | 评论内容 |
|---|---|
| 等待人工审批 | 提示需要 `/approve` 或 `/reject`，列出审核结果 |
| PR 创建 | 链接到 PR |
| Review 完成 | review 结果摘要（通过/问题列表） |
| Pipeline 完成 | 完成摘要（耗时、阶段数、Token 消耗） |
| Pipeline 失败 | 错误信息 + 建议的操作（重试/修改/中止） |

说明：
- `/status` 与 `/logs` 的内容来源于 Workbench HTTP API，而非临时内存状态
- Pipeline 日志查询使用：`GET /api/v1/projects/{projectID}/pipelines/{id}/logs`
- Issue 全链路聚合查询使用：`GET /api/v1/projects/{projectID}/issues/{id}/timeline`

### 评论格式

统一使用折叠块避免过长：

```markdown
🤖 **Implementation 完成** — `implement` ✅

<details>
<summary>📋 变更摘要</summary>

{implement_summary 的前 500 字}

</details>

⏳ 下一步：`code_review`（Agent (ACP) 审查中...）
```

## 五、自动 PR 创建

### 触发时机

在 `implement` 阶段完成后、`code_review` 阶段开始前创建 Draft PR。

理由：
- 先创建 Draft PR 让人可以看到代码变更
- Review 结果和 fixup 记录都追加到 PR 评论中
- 最终合并时将 Draft 转为 Ready

### PR 内容模板

```markdown
## 🔗 关联

Closes #{issue_number}

## 📋 变更说明

{proposal 摘要，最多 300 字}

## ✅ 实现任务

- [x] {task 1}
- [x] {task 2}
- [ ] {task 3 — 如果有未完成的}

## 🔍 AI Review

{review 结果 — PR 创建时可能还没 review，后续追加}

## 📊 执行信息

| 项目 | 值 |
|---|---|
| Pipeline | `{pipeline_id}` |
| 模板 | `{template}` |
| 实现 Agent（ACP） | agent={agent_name} |
| 耗时 | {duration} |

---
> 🤖 由 AI Workflow Orchestrator 自动创建
```

### PR 后续更新

| 事件 | 操作 |
|---|---|
| code_review 完成 | PR 评论贴 review 结果 |
| fixup 完成 | push 新 commit + PR 评论 |
| 人工 approve merge | 将 Draft PR 转为 Ready + merge |
| 人工 abort | 关闭 PR |
| 用户在 GitHub 直接 merge PR | 收到 `pull_request.closed` + `merged=true` webhook，将关联 Pipeline 标记 `done`，跳过剩余 Stage（包括 cleanup），记录"用户直接合并" |

### Pipeline 事件到 GitHub 可视化的映射（P3.5）

Pipeline 运行中的关键事件会被镜像到 GitHub（标签或评论），并在本地日志留痕：

| 事件 | GitHub 同步行为 | 本地留痕 |
|---|---|---|
| `stage_start` | 更新 `status: pipeline_active:{stage}` 标签 | `logs.type=stage_start` |
| `human_required` | 追加评论，提示 `/approve`、`/reject`、`/abort` | `logs.type=human_required` |
| `pipeline_done` | 更新为 `status: pipeline_done` | （可同时有 checkpoint/log） |
| `pipeline_failed` | 更新为 `status: pipeline_failed` | `logs.type=stage_failed`/失败上下文 |
| `action_applied` | 可选评论（实现侧按噪音控制） | `logs.type=action_applied` |

注意：
- GitHub 同步失败不阻塞 Pipeline 主流程（降级为 warning）
- 审计与排障以本地 Store（checkpoints/actions/logs/issue_changes）为准

### PR 配置

```yaml
github:
  pr:
    auto_create: true
    draft: true                    # 先创建 Draft
    auto_merge: false              # 不自动合并，等人工确认
    reviewers: []                  # 自动添加 reviewer
    labels: ["ai-generated"]       # 自动标签
    branch_prefix: "feature/"      # 分支前缀
```

## 六、多项目路由

### 路由架构

```
GitHub Webhook (POST /webhook)
  │
  ├── 验证签名
  ├── 解析 repository.full_name
  │
  ▼
ProjectRouter
  ├── "user/app-a" → Project{app-a} → 处理
  ├── "user/api-b" → Project{api-b} → 处理
  └── "user/lib-c" → 未注册 → 忽略
```

### 单 Webhook URL vs 多 Webhook URL

**推荐单 URL**：所有仓库的 Webhook 都指向同一个 `/webhook` 端点，由 ProjectRouter 分发。

理由：
- 管理简单，只需要一个 ngrok/域名
- 新增项目只需要在配置中注册，再到 GitHub 仓库 Settings 添加 Webhook
- Webhook 与 API/WebSocket 共用 `server.port`（默认 8080），路由为 `POST /webhook`，无需额外端口

### 项目自动注册（可选未来功能）

```bash
# 手动注册
ai-flow project add --id app-a --repo /path/to/app-a --github user/app-a

# 自动注册：扫描某个目录下所有 git 仓库
ai-flow project scan ~/projects/
```

## 七、GitHub API 使用约束

### Rate Limit

- GitHub API 认证用户限制 5000 req/hour
- 每次 Pipeline 大约消耗 10-20 个 API 请求（标签更新 + 评论 + PR 操作）
- 按此计算，单小时可支撑约 250-500 个 Pipeline 操作，完全够用

### 写操作限流器

所有 GitHub API 写操作（创建 Issue、更新标签、添加评论、创建/合并 PR）经过统一的令牌桶限流器，防止批量操作耗尽配额：

```go
type RateLimitedService struct {
    inner   *GitHubService
    limiter *rate.Limiter  // golang.org/x/time/rate
}
```

限流规则：
- GitHub 认证用户主配额 5000 req/h ≈ 1.39 req/s；此外还有 secondary rate limit（短时间突发会触发 HTTP 403）
- 默认速率：1 req/s，burst 5（对齐主配额，短暂突发不超 secondary limit）
- 写操作前调用 `limiter.Wait(ctx)`，阻塞等待令牌
- 读操作不限流（GET 有独立的更高配额，且本系统读操作频率很低）
- 收到 HTTP 429 或 403（secondary limit）时，解析 `Retry-After` header，退避后重试（最多 3 次）
- 限流导致的延迟只记录 warning 日志，不阻塞 Pipeline 执行（标签/评论是辅助信息）

配置项（`github:` 段）：

```yaml
github:
  rate_limit:
    requests_per_second: 1     # 写操作每秒最大请求数（5000/h ≈ 1.39/s，留余量）
    burst: 5                   # 突发容量（应对短时集中操作，如创建 Plan 的多个 Issue）
    retry_on_limit: true       # 收到 429/403 时自动退避重试
    max_retries: 3             # 限流重试上限
```

### Token 配置

```yaml
github:
  # 方式一：Personal Access Token
  token: ghp_xxxxx
  # 方式二：GitHub App（推荐，权限更细）
  app_id: 12345
  private_key_path: ~/.ai-workflow/github-app.pem
  installation_id: 67890
```

推荐使用 GitHub App，因为：
- 权限范围更精确（只需要 issues:write, pull_requests:write, contents:write）
- 评论显示为 Bot 身份，更清晰
- 不受个人 Token 过期影响

### 必要的 GitHub 权限

| 权限 | 范围 | 用途 |
|---|---|---|
| issues | write | 读写 Issue、添加评论、管理标签 |
| pull_requests | write | 创建/更新 PR、添加评论 |
| contents | write | push 代码到分支 |
| metadata | read | 读取仓库基本信息 |

> 注意：权限检查使用 webhook event 中的 `author_association` 字段，不需要额外的 collaborator API 调用（该 API 需要 admin 权限）。

## 八、tracker-github 插件实现

`tracker-github` 是 Tracker 接口的 GitHub 实现，负责将内部 Issue 状态镜像到 GitHub Issue：

```go
// tracker-github 实现 Tracker 接口
type GitHubTracker struct {
    client *github.Client
    owner  string
    repo   string
}

func (t *GitHubTracker) CreateIssue(ctx context.Context, issue *Issue) (string, error) {
    // 创建 GitHub Issue
    // Issue title = issue.Title
    // Issue body = issue.Body + 附件摘要
    // Labels = issue.Labels + 依赖标签 (depends-on-#N) + 状态标签 (ready/blocked)
    // 返回 GitHub Issue number 作为 ExternalID
}

func (t *GitHubTracker) UpdateStatus(ctx context.Context, externalID string, status IssueStatus) error {
    // 更新 Issue 标签：移除旧状态标签，添加新状态标签
    // done → 关闭 GitHub Issue
    // failed → 添加 "failed" 标签
}

func (t *GitHubTracker) SyncDependencies(ctx context.Context, issue *Issue, allIssues []*Issue) error {
    // 为 GitHub Issue 添加 depends-on-#N 标签（N = 上游 GitHub Issue number）
    // 无依赖 → 添加 "ready" 标签
    // 有未完成依赖 → 添加 "blocked" 标签
}
```

### Label 命名规范

```
status: ready               ← 可执行
status: blocked             ← 等待依赖
status: in-progress         ← 执行中
status: done                ← 完成
status: failed              ← 失败
depends-on-#123             ← 依赖 Issue #123
session: {session-id}       ← 所属会话批次
template: standard          ← 使用的 Pipeline 模板
```

## 九、离线/降级模式

GitHub 集成完全关闭时（默认）：

- 所有功能通过 Workbench / TUI 完成
- Tracker 使用 `tracker-local`（空实现），不同步到外部系统
- ReviewGate 使用 `review-ai-panel`（两阶段 AI 审核）
- 执行日志和结果保存在本地 SQLite Store

GitHub 集成启用但不可达时：

- Pipeline 正常执行，只是不同步状态到 GitHub
- 恢复连接后做一次"最终状态同步"：只同步 Pipeline 当前状态到 Issue 标签和评论（不回溯中间状态变化）

配置方式：

```yaml
github:
  enabled: false    # 完全关闭 GitHub 集成（默认）
  # 或者只关闭 Webhook 触发，保留 PR 创建
  webhook_enabled: false
  pr_enabled: true
```

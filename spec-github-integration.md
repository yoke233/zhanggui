# GitHub 集成 — 设计文档

## 概述

GitHub 集成层实现双向联动：GitHub Issue/PR 的变化驱动 Pipeline，Pipeline 的执行进度反映到 Issue/PR。目标是让用户可以完全在 GitHub 上通过评论驱动整个开发流程。

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

### 触发条件

满足以下任一条件时自动创建 Pipeline：

1. **标签触发**（推荐）：Issue 被打上配置中映射的标签
2. **评论触发**：有人在 Issue 中评论 `/run` 或 `/run {template}`
3. **手动触发**：通过 TUI 或 Web 手动关联 Issue 创建 Pipeline

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
模板：`standard`（由标签 `type: bug` 推断）

阶段：requirements → worktree → implement → review → fixup → merge

使用斜杠命令控制流程：
- `/approve` — 审批当前阶段
- `/reject {stage} {reason}` — 回退到指定阶段
- `/pause` / `/resume` — 暂停/恢复
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
| `/reject <stage> [reason]` | 回退到指定阶段 | `/reject spec_gen 数据模型需要重新设计` |
| `/status` | 查看当前状态 | `/status` |
| `/abort` | 终止 Pipeline | `/abort` |

以上 5 个命令为 P2 核心命令。以下命令延后到 P3+：

| 命令 | 效果 | 目标阶段 |
|---|---|---|
| `/modify <feedback>` | 带反馈重跑当前阶段 | P3 |
| `/skip` | 跳过当前阶段 | P3 |
| `/rerun` | 重跑当前阶段 | P3 |
| `/switch <agent>` | 换 Agent 重跑 | P4 |
| `/pause` / `/resume` | 暂停/恢复 Pipeline | P3 |
| `/logs [stage]` | 查看某阶段日志摘要 | P3 |

### 命令解析规则

- 只识别评论第一行以 `/` 开头的内容
- 命令不区分大小写
- 如果评论者不在 authorized_users 列表中，忽略并回复无权限提示
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
status: generating-spec
status: awaiting-review
status: implementing
status: code-review
status: fixing
status: ready-to-merge
pipeline: active        ← Pipeline 运行中始终保留
pipeline: done          ← 完成后替换 active
pipeline: failed        ← 失败后替换 active
```

### 评论更新

不是每个阶段都评论（避免刷屏），只在关键节点：

| 时机 | 评论内容 |
|---|---|
| Spec 生成完成 | spec 摘要（proposal 主要内容 + 任务列表） |
| 等待人工审批 | 提示需要 `/approve` 或 `/reject`，列出审核结果 |
| PR 创建 | 链接到 PR |
| Review 完成 | review 结果摘要（通过/问题列表） |
| Pipeline 完成 | 完成摘要（耗时、阶段数、Token 消耗） |
| Pipeline 失败 | 错误信息 + 建议的操作（重试/修改/中止） |

### 评论格式

统一使用折叠块避免过长：

```markdown
🤖 **Spec 生成完成** — `spec_gen` ✅

<details>
<summary>📋 Proposal 摘要</summary>

{proposal 的前 500 字}

</details>

<details>
<summary>✅ 任务列表（{n} 项）</summary>

{tasks.md 内容}

</details>

⏳ 下一步：`spec_review`（Claude 审核中...）
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
| 实现 Agent | Codex (gpt-5.3-codex) |
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

## 八、离线/降级模式

如果 GitHub 不可达或 Webhook 配置未启用：

- Pipeline 正常执行，只是不同步状态到 GitHub
- 所有操作通过 TUI 或 Web 完成
- 执行日志和结果仍保存在本地 Store
- 恢复连接后做一次"最终状态同步"：只同步 Pipeline 当前状态到 Issue 标签和评论（不回溯中间状态变化），确保 GitHub 侧反映最终结果

配置方式：

```yaml
github:
  enabled: false    # 完全关闭 GitHub 集成
  # 或者只关闭 Webhook 触发，保留 PR 创建
  webhook_enabled: false
  pr_enabled: true
```

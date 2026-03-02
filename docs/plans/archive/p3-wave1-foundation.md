# P3 Wave 1 — 基础设施（gh-1~4）

> **For Agent:** REQUIRED SUB-SKILL: Use executing-wave-plans to implement this wave and gate it before Wave 2.

## Wave Goal

建立 GitHub 集成最小基础设施：可认证客户端、可合并配置、可验签 Webhook 入站、可复用 GitHub 操作层。

## Depends On

- `[]`

## Wave Entry Data

- 已完成 P2/P2b 基础能力（TaskPlan、DAG、Pipeline、EventBus）。
- 当前仓库存在 `internal/config`、`internal/web`、`internal/plugins/factory`、`internal/core`。
- 尚无 `internal/github` 目录，需要本 Wave 创建。

## Tasks

### Task W1-T1 (gh-1): GitHub 客户端 + 认证封装

**Files:**
- Create: `internal/github/client.go`
- Create: `internal/github/client_test.go`
- Modify: `internal/config/types.go`
- Modify: `internal/config/defaults.go`
- Test: `internal/github/client_test.go`
- Test: `internal/config/config_test.go`

**Depends on:** `[]`

**Step 1: Write failing test**
```text
新增测试：
- TestNewGitHubClient_PAT_Success
- TestNewGitHubClient_AppAuth_Success
- TestNewGitHubClient_MissingCredentials_Error
```

**Step 2: Run to confirm failure**
Run: `go test ./internal/github ./internal/config -run 'TestNewGitHubClient_|TestConfig_Defaults'`
Expected: 编译失败或测试失败（client 未实现/配置字段未就绪）。

**Step 3: Minimal implementation**
```text
实现 NewClient(cfg config.GitHubConfig)，支持 PAT 与 App 两条认证路径。
保持零值安全：enabled=false 或凭据为空时返回明确错误，不 panic。
```

**Step 4: Run tests to confirm pass**
Run: `go test ./internal/github ./internal/config -run 'TestNewGitHubClient_|TestConfig_Defaults'`
Expected: PASS。

**Step 5: Commit**
```bash
git add internal/github/client.go internal/github/client_test.go internal/config/types.go internal/config/defaults.go internal/config/config_test.go
git commit -m "feat(github): add authenticated github client"
```

### Task W1-T2 (gh-2): 事件类型 + 配置模型 + 工厂选择骨架

**Files:**
- Modify: `internal/core/events.go`
- Modify: `internal/config/types.go`
- Modify: `internal/config/merge.go`
- Modify: `internal/config/merge_hierarchy_test.go`
- Modify: `internal/plugins/factory/factory.go`
- Modify: `internal/plugins/factory/factory_test.go`
- Test: `internal/core/action_test.go`
- Test: `internal/plugins/factory/factory_test.go`

**Depends on:** `[W1-T1]`

**Step 1: Write failing test**
```text
新增测试：
- TestEventTypes_GitHubConstants_Defined
- TestGitHubConfig_MergeHierarchy_Works
- TestFactory_GitHubEnabled_SelectsGitHubPluginNames
```

**Step 2: Run to confirm failure**
Run: `go test ./internal/core ./internal/config ./internal/plugins/factory -run 'GitHub|Factory'`
Expected: 事件常量缺失/配置合并失败/工厂选择断言失败。

**Step 3: Minimal implementation**
```text
补齐 GitHub 事件常量；扩展 GitHub 配置结构（`enabled`、认证字段、`owner/repo`、`webhook_secret`、`webhook_enabled`、`pr_enabled`、`label_mapping`、`authorized_usernames`、`auto_trigger`、`pr.*`）；
在工厂中引入按 github.enabled 与显式覆盖选择插件名的决策函数（仅决策，不要求插件已实现）。
```

**Step 4: Run tests to confirm pass**
Run: `go test ./internal/core ./internal/config ./internal/plugins/factory -run 'GitHub|Factory'`
Expected: PASS。

**Step 5: Commit**
```bash
git add internal/core/events.go internal/config/types.go internal/config/merge.go internal/config/merge_hierarchy_test.go internal/plugins/factory/factory.go internal/plugins/factory/factory_test.go
git commit -m "feat(github): extend config and factory selection"
```

### Task W1-T3 (gh-3): Webhook 端点骨架 + HMAC 验签 + 多项目路由

**Files:**
- Create: `internal/web/handlers_webhook.go`
- Create: `internal/web/handlers_webhook_test.go`
- Modify: `internal/web/server.go`
- Modify: `internal/web/server_test.go`
- Create: `internal/web/testdata/github_issues_opened.json`
- Create: `internal/web/testdata/github_issue_comment_created.json`
- Test: `internal/web/handlers_webhook_test.go`

**Depends on:** `[W1-T1]`

**Step 1: Write failing test**
```text
新增测试：
- TestWebhook_VerifySignature_Success
- TestWebhook_VerifySignature_Invalid_Returns401
- TestWebhook_ProjectRouting_UsesOwnerRepo
- TestWebhook_UnsupportedEvent_Returns202
```

**Step 2: Run to confirm failure**
Run: `go test ./internal/web -run 'TestWebhook_'`
Expected: 路由不存在或验签逻辑未实现。

**Step 3: Minimal implementation**
```text
新增 POST /webhook 路由（置于 auth middleware 之外）。
校验 X-Hub-Signature-256，按 owner/repo 映射项目，识别 X-GitHub-Event，发布标准入站事件。
```

**Step 4: Run tests to confirm pass**
Run: `go test ./internal/web -run 'TestWebhook_'`
Expected: PASS。

**Step 5: Commit**
```bash
git add internal/web/handlers_webhook.go internal/web/handlers_webhook_test.go internal/web/server.go internal/web/server_test.go internal/web/testdata/github_issues_opened.json internal/web/testdata/github_issue_comment_created.json
git commit -m "feat(github): add webhook endpoint and signature verification"
```

### Task W1-T4 (gh-4): GitHub 通用操作层（Issue/Label/Comment/PR）

**Files:**
- Create: `internal/github/service.go`
- Create: `internal/github/service_test.go`
- Modify: `internal/github/client.go`
- Test: `internal/github/service_test.go`

**Depends on:** `[W1-T1]`

**Step 1: Write failing test**
```text
新增测试：
- TestGitHubService_CreateIssue_Success
- TestGitHubService_UpdateLabels_ReplaceStatusLabel
- TestGitHubService_AddIssueComment_Success
- TestGitHubService_CreateDraftPR_Success
```

**Step 2: Run to confirm failure**
Run: `go test ./internal/github -run 'TestGitHubService_'`
Expected: service API 未实现。

**Step 3: Minimal implementation**
```text
封装 GitHubService：CreateIssue/UpdateIssueLabels/AddIssueComment/CreatePR/UpdatePR/MergePR/ClosePR。
统一错误包装与上下文日志字段，避免业务层直接拼 REST 请求。
```

**Step 4: Run tests to confirm pass**
Run: `go test ./internal/github -run 'TestGitHubService_'`
Expected: PASS。

**Step 5: Commit**
```bash
git add internal/github/service.go internal/github/service_test.go internal/github/client.go
git commit -m "feat(github): add reusable github service operations"
```

## Test Strategy Per Task

| Task | 单测 | 集成验证 |
|---|---|---|
| W1-T1 | 认证路径、错误路径、零值安全 | 使用 `httptest` 模拟 API 根端点连通 |
| W1-T2 | 配置合并与工厂决策 | `github.enabled` true/false 切换行为 |
| W1-T3 | 验签、事件解析、项目路由 | 真实 fixture 回放 `issues.opened` |
| W1-T4 | Issue/PR 操作序列 | 标签替换与评论追加契约 |

## Risks and Mitigations

- 风险：Webhook 验签实现错误导致误拒绝。  
  缓解：固定 fixture + 金标准签名测试（含大小写、前缀、空体）。
- 风险：配置层改动破坏现有加载路径。  
  缓解：补充 `merge_hierarchy` 回归测试，保持零值兼容。
- 风险：服务层方法粒度过粗后续难扩展。  
  缓解：先做最小接口，避免提前抽象 Provider。

## Wave E2E/Smoke Cases and Entry Data

### Entry Data
- 一个测试项目：`github_owner=acme`, `github_repo=demo`。
- Webhook secret：`test-secret-123456`。
- Webhook fixtures：`issues.opened` / `issue_comment.created`。

### Smoke Cases
- `POST /webhook`（合法签名）返回 `202` 且发布入站事件。
- `POST /webhook`（非法签名）返回 `401`。
- GitHubService 能创建 issue 并回写 label/comment（通过 `httptest` 断言请求体）。

## Wave Exit Gate
- Execute mandatory gate sequence via `executing-wave-plans`.
- Wave-specific acceptance:
  - [ ] `internal/github` 基础客户端与通用操作层可用，相关测试通过。
  - [ ] `internal/web` 已支持 `/webhook` 验签与项目路由。
  - [ ] GitHub 配置模型与 `docs/spec` 语义一致（无 `plugins` 伪字段漂移）。
  - [ ] `github.enabled=false` 下全量核心测试不回归失败。
- Wave-specific verification:
  - [ ] `go test ./internal/github ./internal/web ./internal/config ./internal/plugins/factory -run 'GitHub|Webhook|Factory'` 通过。
  - [ ] `go test ./...` 至少一次全量通过。
- Boundary-change verification (if triggered):
  - [ ] 若改动了路由中间件顺序，执行 `go test ./internal/web -run 'Server|Auth|Webhook'` 并确认全部通过。

## Next Wave Entry Condition
- Governed by `executing-wave-plans` verdict rule (`Go` / satisfied `Conditional Go` only).

# P3 Wave 2 — 插件实现（gh-5~7）

> **For Agent:** REQUIRED SUB-SKILL: Use executing-wave-plans to implement this wave and gate it before Wave 2.5.

## Wave Goal

交付 GitHub 插件层：`tracker-github`、`scm-github`、Webhook 事件分发器，形成可复用的 GitHub 适配基础。

## Depends On

- `[W1-T1, W1-T2, W1-T3, W1-T4]`

## Wave Entry Data

- Wave 1 产出的 GitHub 客户端、Webhook 端点、通用操作层已可用。
- `internal/core/tracker.go`、`internal/core/scm.go` 接口已存在。
- 仍保持默认插件为 `tracker-local` / `scm-local-git`（未切换）。

## Tasks

### Task W2-T1 (gh-5): `tracker-github` 插件

**Files:**
- Create: `internal/plugins/tracker-github/tracker.go`
- Create: `internal/plugins/tracker-github/tracker_test.go`
- Create: `internal/plugins/tracker-github/module.go`
- Modify: `internal/core/tracker.go`
- Modify: `internal/secretary/scheduler_test.go`
- Test: `internal/plugins/tracker-github/tracker_test.go`

**Depends on:** `[W1-T4]`

**Step 1: Write failing test**
```text
新增测试：
- TestGitHubTracker_CreateTask_CreatesIssueAndExternalID
- TestGitHubTracker_UpdateStatus_Done_ClosesIssue
- TestGitHubTracker_UpdateStatus_Failed_SetsFailedLabel
- TestGitHubTracker_SyncDependencies_ReadyAndBlockedLabels
```

**Step 2: Run to confirm failure**
Run: `go test ./internal/plugins/tracker-github -run 'TestGitHubTracker_'`
Expected: 包不存在或接口未实现。

**Step 3: Minimal implementation**
```text
实现 core.Tracker：CreateTask/UpdateStatus/SyncDependencies/OnExternalComplete。
策略：GitHub API 失败记录 warning 并降级，不阻塞 Task 执行主流程。
```

**Step 4: Run tests to confirm pass**
Run: `go test ./internal/plugins/tracker-github -run 'TestGitHubTracker_'`
Expected: PASS。

**Step 5: Commit**
```bash
git add internal/plugins/tracker-github/tracker.go internal/plugins/tracker-github/tracker_test.go internal/plugins/tracker-github/module.go internal/core/tracker.go internal/secretary/scheduler_test.go
git commit -m "feat(tracker): add github tracker plugin"
```

### Task W2-T2 (gh-6): `scm-github` 插件

**Files:**
- Create: `internal/plugins/scm-github/scm.go`
- Create: `internal/plugins/scm-github/scm_test.go`
- Create: `internal/plugins/scm-github/module.go`
- Modify: `internal/core/scm.go`
- Modify: `internal/plugins/scm-local-git/scm.go`
- Test: `internal/plugins/scm-github/scm_test.go`

**Depends on:** `[W1-T4]`

**Step 1: Write failing test**
```text
新增测试：
- TestGitHubSCM_CreatePR_DraftSuccess
- TestGitHubSCM_UpdatePR_AddComment
- TestGitHubSCM_ConvertToReady_Success
- TestGitHubSCM_MergePR_Success
```

**Step 2: Run to confirm failure**
Run: `go test ./internal/plugins/scm-github -run 'TestGitHubSCM_'`
Expected: 包不存在或方法未实现。

**Step 3: Minimal implementation**
```text
实现 core.SCM 的 GitHub 版本：本地 git 操作可委托给 local-git，PR 生命周期操作走 GitHubService。
支持项目配置中的 draft/reviewers。
```

**Step 4: Run tests to confirm pass**
Run: `go test ./internal/plugins/scm-github -run 'TestGitHubSCM_'`
Expected: PASS。

**Step 5: Commit**
```bash
git add internal/plugins/scm-github/scm.go internal/plugins/scm-github/scm_test.go internal/plugins/scm-github/module.go internal/core/scm.go internal/plugins/scm-local-git/scm.go
git commit -m "feat(scm): add github scm plugin"
```

### Task W2-T3 (gh-7): Webhook 事件分发 + per-issue 串行化

**Files:**
- Create: `internal/github/webhook_dispatcher.go`
- Create: `internal/github/webhook_dispatcher_test.go`
- Modify: `internal/web/handlers_webhook.go`
- Modify: `internal/eventbus/bus.go`
- Test: `internal/github/webhook_dispatcher_test.go`

**Depends on:** `[W1-T3]`

**Step 1: Write failing test**
```text
新增测试：
- TestWebhookDispatcher_IssueEvents_SerializedByIssueNumber
- TestWebhookDispatcher_DifferentIssues_CanRunInParallel
- TestWebhookDispatcher_DeduplicatesDeliveryID
- TestWebhookDispatcher_PublishesEventGitHubWebhookReceived
- TestWebhookDispatcher_CleansIssueMutexAfterCloseOrPipelineDone
```

**Step 2: Run to confirm failure**
Run: `go test ./internal/github -run 'TestWebhookDispatcher_'`
Expected: 分发器缺失，串行化断言失败。

**Step 3: Minimal implementation**
```text
实现按 issue number 的串行执行器 + 短期去重缓存（delivery id）。
实现 mutex 生命周期管理：Issue 关闭或 Pipeline 完成后延迟 5 分钟清理该 issue 锁（防止尾部事件并发竞态）。
将 handlers_webhook 入站 payload 统一交给 dispatcher。
```

**Step 4: Run tests to confirm pass**
Run: `go test ./internal/github -run 'TestWebhookDispatcher_'`
Expected: PASS。

**Step 5: Commit**
```bash
git add internal/github/webhook_dispatcher.go internal/github/webhook_dispatcher_test.go internal/web/handlers_webhook.go internal/eventbus/bus.go
git commit -m "feat(github): add webhook dispatcher with per-issue serialization"
```

## Test Strategy Per Task

| Task | 单测 | 集成验证 |
|---|---|---|
| W2-T1 | 状态映射、依赖标签、降级策略 | Scheduler 调用 tracker 时不阻塞执行 |
| W2-T2 | PR 创建/更新/合并路径 | Draft + reviewers 配置生效 |
| W2-T3 | 串行化、并发、幂等去重 | webhook handler 到 dispatcher 的端到端调用 |

## Risks and Mitigations

- 风险：同 Issue 并发事件导致重复动作。  
  缓解：per-issue lock + delivery-id 去重缓存。
- 风险：tracker/scm 与核心接口不匹配。  
  缓解：先补接口契约测试，再实现。
- 风险：GitHub API 抖动导致大量错误日志。  
  缓解：统一错误分类 + 节流日志 + warning 降级。

## Wave E2E/Smoke Cases and Entry Data

### Entry Data
- 两个 issue：`#101`、`#102`（同项目）。
- 同一 delivery-id 的重复 webhook payload。
- 配置：`github.enabled=true`，但 factory 默认仍可选本地插件。

### Smoke Cases
- 同一 `issue_number=101` 的两次事件串行执行且顺序可断言。
- `issue_number=101/102` 两组事件可并行处理。
- tracker 创建任务时回写 `external_id`，失败时仅 warning。
- `issue_number=101` 关闭后，5 分钟延迟窗口过后其 mutex 被回收；新事件可重新创建锁并继续串行。

## Wave Exit Gate
- Execute mandatory gate sequence via `executing-wave-plans`.
- Wave-specific acceptance:
  - [ ] `tracker-github` 与 `scm-github` 包均可独立通过测试。
  - [ ] Webhook dispatcher 实现 per-issue 串行与 delivery 去重。
  - [ ] Webhook dispatcher 实现 issue 级 mutex 延迟回收策略（5 分钟）。
  - [ ] GitHub API 调用失败不阻塞主执行链路。
- Wave-specific verification:
  - [ ] `go test ./internal/plugins/tracker-github ./internal/plugins/scm-github ./internal/github -run 'GitHubTracker|GitHubSCM|WebhookDispatcher'` 通过。
  - [ ] `go test ./internal/secretary -run 'Scheduler|Tracker'` 通过。
- Boundary-change verification (if triggered):
  - [ ] 若修改了 EventBus 行为，执行 `go test ./internal/eventbus ./internal/engine -run 'Event|Reaction'` 并确认 PASS。

## Next Wave Entry Condition
- Governed by `executing-wave-plans` verdict rule (`Go` / satisfied `Conditional Go` only).

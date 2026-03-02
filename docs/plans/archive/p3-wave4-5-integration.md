# P3 Wave 4+5 — 集成收口（gh-12~16）

> **For Agent:** REQUIRED SUB-SKILL: Use executing-wave-plans to implement Wave 4 first, then Wave 5.

## Wave 4 Goal (gh-12~13)

增强稳态能力：GitHub 不可达时主流程不受影响；工厂与 CLI 的 GitHub 配置集成闭环。

## Wave 4 Depends On

- `[W3-T1, W3-T2, W3-T3, W3-T4, W25-T3, W25-T5, W25-T6]`

## Wave 4 Entry Data

- 双向同步主链路已可用。
- Wave 2.5 的 queue/DLQ/reconcile/trace/admin/permission gate 已通过。
- 当前默认插件仍可退回 local 模式。
- CLI 已有基础命令框架（`cmd/ai-flow/commands.go`）。

## Wave 4 Tasks

### Task W4-T1 (gh-12): 离线降级 + 重连补偿同步

**Files:**
- Create: `internal/github/resilient_client.go`
- Create: `internal/github/resilient_client_test.go`
- Create: `internal/github/reconnect_sync.go`
- Create: `internal/github/reconnect_sync_test.go`
- Modify: `internal/github/status_syncer.go`
- Test: `internal/github/reconnect_sync_test.go`

**Depends on:** `[W3-T3, W3-T4]`

**Step 1: Write failing test**
```text
新增测试：
- TestResilientClient_NetworkError_DegradesToNoop
- TestReconnectSync_OnRecovered_PublishesGitHubReconnected
- TestReconnectSync_ReplaysLatestPipelineStateOnly
```

**Step 2: Run to confirm failure**
Run: `go test ./internal/github -run 'TestResilientClient_|TestReconnectSync_'`
Expected: 降级与重连逻辑未实现。

**Step 3: Minimal implementation**
```text
实现 ResilientClient 包装器：网络错误 -> warning + no-op。
实现健康检查与恢复回调：恢复后发布重连事件并执行最终态补偿同步。
```

**Step 4: Run tests to confirm pass**
Run: `go test ./internal/github -run 'TestResilientClient_|TestReconnectSync_'`
Expected: PASS。

**Step 5: Commit**
```bash
git add internal/github/resilient_client.go internal/github/resilient_client_test.go internal/github/reconnect_sync.go internal/github/reconnect_sync_test.go internal/github/status_syncer.go
git commit -m "feat(github): add resilient mode and reconnect sync"
```

### Task W4-T2 (gh-13): 工厂注册 + CLI 配置集成

**Files:**
- Modify: `internal/plugins/factory/factory.go`
- Modify: `internal/plugins/factory/factory_test.go`
- Modify: `internal/config/types.go`
- Modify: `cmd/ai-flow/commands.go`
- Modify: `cmd/ai-flow/commands_test.go`
- Create: `cmd/ai-flow/commands_github_validate.go`
- Create: `cmd/ai-flow/commands_github_validate_test.go`

**Depends on:** `[W2-T1, W2-T2, W4-T1]`

**Step 1: Write failing test**
```text
新增测试：
- TestFactory_GitHubEnabled_SelectsTrackerAndSCM
- TestFactory_GitHubDisabled_UsesLocalDefaults
- TestFactory_GitHubExplicitOverride_Wins
- TestCommand_GitHubValidate_InvalidConfig_Fails
```

**Step 2: Run to confirm failure**
Run: `go test ./internal/plugins/factory ./cmd/ai-flow -run 'GitHub|Validate'`
Expected: 注册缺失或命令缺失。

**Step 3: Minimal implementation**
```text
注册 github tracker/scm 插件模块，保留 local 默认与显式覆盖优先级。
新增 `ai-flow github validate`，统一校验 token/app/webhook_secret。
```

**Step 4: Run tests to confirm pass**
Run: `go test ./internal/plugins/factory ./cmd/ai-flow -run 'GitHub|Validate'`
Expected: PASS。

**Step 5: Commit**
```bash
git add internal/plugins/factory/factory.go internal/plugins/factory/factory_test.go internal/config/types.go cmd/ai-flow/commands.go cmd/ai-flow/commands_test.go cmd/ai-flow/commands_github_validate.go cmd/ai-flow/commands_github_validate_test.go
git commit -m "feat(github): wire plugins into factory and cli validation"
```

## Wave 4 Test Strategy Per Task

| Task | 单测 | 集成验证 |
|---|---|---|
| W4-T1 | 降级策略、恢复事件、补偿回放 | 断网->恢复模拟回放 |
| W4-T2 | 插件选择优先级、CLI 校验错误提示 | 真实配置文件加载 + validate 命令 |

## Wave 4 Risks and Mitigations

- 风险：降级逻辑吞掉真实错误。  
  缓解：仅网络类错误降级，业务错误继续返回并打结构化日志。
- 风险：工厂选择规则复杂化。  
  缓解：抽单独决策函数并覆盖优先级测试。

## Wave 4 E2E/Smoke Cases and Entry Data

### Entry Data
- 一个启用 GitHub 的项目配置（token 或 app 至少一套）。
- 故障注入：GitHub API 429 / timeout / connection reset。

### Smoke Cases
- GitHub API timeout 时 pipeline 继续执行且日志为 warning。
- 网络恢复后自动发布 `github_reconnected` 并完成最终态补偿同步。
- `ai-flow github validate` 对无凭据配置返回非零退出码。

## Wave 4 Exit Gate
- Execute mandatory gate sequence via `executing-wave-plans`.
- Wave-specific acceptance:
  - [ ] GitHub 不可达时主链路不断，恢复后可补偿同步。
  - [ ] 工厂按配置稳定选择 github/local 插件。
  - [ ] CLI validate 能覆盖凭据与 webhook secret 校验。
- Wave-specific verification:
  - [ ] `go test ./internal/github ./internal/plugins/factory ./cmd/ai-flow -run 'GitHub|Reconnect|Validate'` 通过。
  - [ ] `go test ./...` 全量通过一次。
- Boundary-change verification (if triggered):
  - [ ] 若修改 config merge，执行 `go test ./internal/config -run 'Merge|Layer|GitHub'` 并确认 PASS。

## Next Wave Entry Condition
- Governed by `executing-wave-plans` verdict rule (`Go` / satisfied `Conditional Go` only).

---

## Wave 5 Goal (gh-14~16)

完成集成收口：可选 `review-github-pr`、端到端场景回归、Web UI GitHub 状态展示。

## Wave 5 Depends On

- `[W4-T1, W4-T2]`

## Wave 5 Entry Data

- Wave 4 已通过，系统具备稳定降级能力。
- 前端已有 `PlanView/PipelineView/BoardView` 与 API 客户端能力。

## Wave 5 Tasks

### Task W5-T1 (gh-14): `review-github-pr` 插件（可选）

**Files:**
- Create: `internal/plugins/review-github-pr/review.go`
- Create: `internal/plugins/review-github-pr/review_test.go`
- Create: `internal/plugins/review-github-pr/module.go`
- Modify: `internal/core/review_gate.go`
- Test: `internal/plugins/review-github-pr/review_test.go`

**Depends on:** `[W3-T2, W2-T2, W4-T2]`

**Step 1: Write failing test**
```text
新增测试：
- TestGitHubPRReview_Submit_CreatesReviewPR
- TestGitHubPRReview_Check_MapsReviewStates
- TestGitHubPRReview_Cancel_ClosesPR
```

**Step 2: Run to confirm failure**
Run: `go test ./internal/plugins/review-github-pr -run 'TestGitHubPRReview_'`
Expected: 包不存在或映射逻辑未实现。

**Step 3: Minimal implementation**
```text
实现可选 ReviewGate，不改默认 review-ai-panel 路径。
当启用 github-pr 时，Review 状态映射 approved/changes_requested/pending。
```

**Step 4: Run tests to confirm pass**
Run: `go test ./internal/plugins/review-github-pr -run 'TestGitHubPRReview_'`
Expected: PASS。

**Step 5: Commit**
```bash
git add internal/plugins/review-github-pr/review.go internal/plugins/review-github-pr/review_test.go internal/plugins/review-github-pr/module.go internal/core/review_gate.go
git commit -m "feat(review): add optional github pr review gate"
```

### Task W5-T2 (gh-15): 端到端集成测试套件

**Files:**
- Create: `internal/github/e2e_github_integration_test.go`
- Create: `internal/github/testdata/issues_opened.json`
- Create: `internal/github/testdata/issue_comment_created.json`
- Create: `internal/github/testdata/pull_request_closed_merged.json`
- Modify: `internal/web/test_helpers_test.go`
- Test: `internal/github/e2e_github_integration_test.go`

**Depends on:** `[W4-T1, W4-T2]`

**Step 1: Write failing test**
```text
新增场景：
- Scenario A: issues.opened -> pipeline create -> status sync
- Scenario B: slash /reject implement -> pipeline action
- Scenario C: implement complete -> draft PR -> merged webhook -> pipeline done
- Scenario D: github outage -> degrade -> recover
```

**Step 2: Run to confirm failure**
Run: `go test ./internal/github -run 'TestE2E_GitHub_'`
Expected: 场景尚未闭环。

**Step 3: Minimal implementation**
```text
使用 httptest.Server 模拟完整 GitHub API，统一 fixture 装载。
确保每个场景都断言最终状态而非中间日志。
```

**Step 4: Run tests to confirm pass**
Run: `go test ./internal/github -run 'TestE2E_GitHub_'`
Expected: PASS。

**Step 5: Commit**
```bash
git add internal/github/e2e_github_integration_test.go internal/github/testdata/issues_opened.json internal/github/testdata/issue_comment_created.json internal/github/testdata/pull_request_closed_merged.json internal/web/test_helpers_test.go
git commit -m "test(github): add end-to-end integration scenarios"
```

### Task W5-T3 (gh-16): Web UI GitHub 状态展示

**Files:**
- Modify: `web/src/types/api.ts`
- Modify: `web/src/lib/apiClient.ts`
- Modify: `web/src/lib/apiClient.test.ts`
- Modify: `web/src/views/PipelineView.tsx`
- Modify: `web/src/views/PipelineView.test.tsx`
- Modify: `web/src/views/PlanView.tsx`
- Modify: `web/src/views/PlanView.test.tsx`
- Modify: `web/src/views/BoardView.tsx`
- Modify: `web/src/views/BoardView.test.tsx`
- Create: `web/src/components/GitHubStatusBadge.tsx`
- Create: `web/src/components/GitHubStatusBadge.test.tsx`

**Depends on:** `[W3-T3, W3-T4, W5-T2]`

**Step 1: Write failing test**
```text
新增测试：
- TestGitHubStatusBadge_RendersConnectedDegradedDisconnected
- TestPipelineView_ShowsIssueAndPRLinks
- TestPlanView_ShowsTaskItemGitHubIssueLinks
- TestBoardView_ShowsGitHubIssueIcon
```

**Step 2: Run to confirm failure**
Run: `npm --prefix web test -- --run GitHubStatusBadge PipelineView PlanView BoardView`
Expected: 组件或字段缺失导致失败。

**Step 3: Minimal implementation**
```text
扩展 API 类型与客户端字段映射；新增状态徽标组件；在 Pipeline/Plan/Board 视图增加 Issue/PR 链接与空态处理。
```

**Step 4: Run tests to confirm pass**
Run: `npm --prefix web test -- --run GitHubStatusBadge PipelineView PlanView BoardView`
Expected: PASS。

**Step 5: Commit**
```bash
git add web/src/types/api.ts web/src/lib/apiClient.ts web/src/lib/apiClient.test.ts web/src/views/PipelineView.tsx web/src/views/PipelineView.test.tsx web/src/views/PlanView.tsx web/src/views/PlanView.test.tsx web/src/views/BoardView.tsx web/src/views/BoardView.test.tsx web/src/components/GitHubStatusBadge.tsx web/src/components/GitHubStatusBadge.test.tsx
git commit -m "feat(web): surface github issue pr and connection status"
```

## Wave 5 Test Strategy Per Task

| Task | 单测 | 集成验证 |
|---|---|---|
| W5-T1 | Review 状态映射、取消路径 | 可选启用下的评审流转 |
| W5-T2 | 多场景 e2e 回放 | 含故障与恢复完整闭环 |
| W5-T3 | 组件渲染、字段映射、空态 | 前端构建与关键页面渲染 |

## Wave 5 Risks and Mitigations

- 风险：可选 ReviewGate 侵入默认路径。  
  缓解：feature flag 控制，默认保持 `review-ai-panel`。
- 风险：e2e 场景脆弱导致 CI 不稳定。  
  缓解：固定 fixture + 稳定断言最终态。
- 风险：前端字段演进破坏兼容。  
  缓解：API 类型向后兼容，字段缺失走空态 UI。

## Wave 5 E2E/Smoke Cases and Entry Data

### Entry Data
- 完整 webhook fixtures（issue/comment/pr/review）。
- 两个项目配置：GitHub enabled/disabled 各一套。

### Smoke Cases
- GitHub disabled 项目 UI 不显示错误，只显示空态。
- GitHub enabled 项目能看到 issue/pr 链接和连接状态。
- e2e 场景 C（Draft PR -> Merge）结束后 pipeline 状态为 done。

## Wave Exit Gate
- Execute mandatory gate sequence via `executing-wave-plans`.
- Wave-specific acceptance:
  - [ ] gh-15 的四个 e2e 场景稳定通过。
  - [ ] Web UI 三个视图均正确展示 GitHub 状态与链接。
  - [ ] gh-14 即使关闭也不影响 P3 主 Gate；开启时行为正确。
- Wave-specific verification:
  - [ ] `go test ./internal/github -run 'TestE2E_GitHub_|TestGitHubPRReview_'` 通过。
  - [ ] `npm --prefix web test -- --run GitHubStatusBadge PipelineView PlanView BoardView` 通过。
  - [ ] `npm --prefix web run build` 通过。
- Boundary-change verification (if triggered):
  - [ ] 若改动 API schema，执行 `go test ./internal/web ./internal/config` 与前端类型检查并确认无回归。

## Next Wave Entry Condition
- Wave 5 为 P3 最终波次；满足 Exit Gate 后进入 P3 Done 验收。

# P3 Wave 2.5 — 可靠性硬化门禁（gh-7a~7f）

> **For Agent:** REQUIRED SUB-SKILL: Use executing-wave-plans to implement this wave and gate it before Wave 3.

## Wave Goal

在不改变 P3 功能范围的前提下，先补齐生产可用性底座：GitHub 写入限流、Webhook 失败补偿、状态对账修复、链路可观测、管理员逃生舱与 GitHub App 权限门禁。

## Depends On

- `[W2-T1, W2-T2, W2-T3]`

## Wave Entry Data

- `tracker-github`、`scm-github`、Webhook dispatcher 已存在，且可处理基础事件。
- 当前系统仍允许 `github.enabled=false` 回退 local 模式。
- Wave 3 尚未开始，允许在双向主链路前插入硬化门禁。

## Tasks

### Task W25-T1 (gh-7a): GitHub 出站写操作队列 + 令牌桶限流

**Files:**
- Create: `internal/github/outbound_queue.go`
- Create: `internal/github/outbound_queue_test.go`
- Modify: `internal/github/service.go`
- Modify: `internal/github/service_test.go`
- Test: `internal/github/outbound_queue_test.go`

**Depends on:** `[W2-T2, W2-T3]`

**Step 1: Write failing test**
```text
新增测试：
- TestOutboundQueue_RespectsTokenBucketRate
- TestOutboundQueue_RetryWithBackoffOn429
- TestOutboundQueue_RetryWithBackoffOn403SecondaryLimit
- TestOutboundQueue_RetryAtMost3TimesOnRateLimit
- TestOutboundQueue_PreservesPerIssueOrdering
```

**Step 2: Run to confirm failure**
Run: `go test ./internal/github -run 'TestOutboundQueue_'`
Expected: 出站队列或限流逻辑缺失。

**Step 3: Minimal implementation**
```text
实现统一出站写队列（Issue/Comment/Label/PR 写入都经过该队列）。
限流参数对齐 spec：`github.rate_limit.rps=1`、`github.rate_limit.burst=5`。
收到 429/403（secondary limit）时按 `Retry-After` 退避重试（最多 3 次）。
保留对 5xx 的指数退避重试；支持 idempotency key 去重。
```

**Step 4: Run tests to confirm pass**
Run: `go test ./internal/github -run 'TestOutboundQueue_'`
Expected: PASS。

**Step 5: Commit**
```bash
git add internal/github/outbound_queue.go internal/github/outbound_queue_test.go internal/github/service.go internal/github/service_test.go
git commit -m "feat(github): add outbound queue and rate limiting"
```

### Task W25-T2 (gh-7b): Webhook 死信队列（DLQ）+ replay 工具

**Files:**
- Create: `internal/github/dlq_store.go`
- Create: `internal/github/dlq_store_test.go`
- Modify: `internal/github/webhook_dispatcher.go`
- Create: `cmd/ai-flow/commands_github_replay.go`
- Create: `cmd/ai-flow/commands_github_replay_test.go`
- Modify: `cmd/ai-flow/commands.go`
- Test: `internal/github/dlq_store_test.go`

**Depends on:** `[W2-T3]`

**Step 1: Write failing test**
```text
新增测试：
- TestWebhookDispatcher_FailedEvent_PushedToDLQ
- TestGitHubReplayCommand_ReplaysByDeliveryID
- TestGitHubReplayCommand_IdempotentNoDoubleApply
```

**Step 2: Run to confirm failure**
Run: `go test ./internal/github ./cmd/ai-flow -run 'DLQ|Replay|Delivery'`
Expected: DLQ 与 replay 命令缺失。

**Step 3: Minimal implementation**
```text
dispatcher 处理失败时写入 DLQ（保留 delivery_id、event_type、issue_number、trace_id）。
新增 `ai-flow github replay --delivery-id <id>`，回放时强制幂等检查。
```

**Step 4: Run tests to confirm pass**
Run: `go test ./internal/github ./cmd/ai-flow -run 'DLQ|Replay|Delivery'`
Expected: PASS。

**Step 5: Commit**
```bash
git add internal/github/dlq_store.go internal/github/dlq_store_test.go internal/github/webhook_dispatcher.go cmd/ai-flow/commands.go cmd/ai-flow/commands_github_replay.go cmd/ai-flow/commands_github_replay_test.go
git commit -m "feat(github): add webhook dlq and replay command"
```

### Task W25-T3 (gh-7c): 周期性对账修复任务（reconcile）

**Files:**
- Create: `internal/github/reconcile_job.go`
- Create: `internal/github/reconcile_job_test.go`
- Modify: `internal/secretary/scheduler.go`
- Modify: `internal/github/status_syncer.go`
- Test: `internal/github/reconcile_job_test.go`

**Depends on:** `[W25-T1, W25-T2]`

**Step 1: Write failing test**
```text
新增测试：
- TestReconcileJob_FixesBlockedTaskWhenDependencyAlreadyDone
- TestReconcileJob_RepairsIssueLabelDrift
- TestReconcileJob_MissedWebhookRecoveredWithinInterval
```

**Step 2: Run to confirm failure**
Run: `go test ./internal/github -run 'TestReconcileJob_'`
Expected: 对账任务不存在或修复逻辑缺失。

**Step 3: Minimal implementation**
```text
每 10 分钟扫描 `blocked` 与 `depends-on-#N` 镜像标签任务，与本地 DAG 状态进行差异修复。
仅修复最终态漂移，不回放全部历史中间态。
```

**Step 4: Run tests to confirm pass**
Run: `go test ./internal/github -run 'TestReconcileJob_'`
Expected: PASS。

**Step 5: Commit**
```bash
git add internal/github/reconcile_job.go internal/github/reconcile_job_test.go internal/secretary/scheduler.go internal/github/status_syncer.go
git commit -m "feat(github): add reconcile job for drift recovery"
```

### Task W25-T4 (gh-7d): Trace ID 贯通 + 结构化日志统一字段

**Files:**
- Create: `internal/observability/trace.go`
- Create: `internal/observability/trace_test.go`
- Modify: `internal/web/handlers_webhook.go`
- Modify: `internal/github/webhook_dispatcher.go`
- Modify: `internal/engine/executor.go`
- Test: `internal/observability/trace_test.go`

**Depends on:** `[W2-T3]`

**Step 1: Write failing test**
```text
新增测试：
- TestTraceContext_FromWebhookDeliveryID
- TestTraceContext_PropagatesToPipelineEvents
- TestStructuredLog_ContainsTraceAndOperation
```

**Step 2: Run to confirm failure**
Run: `go test ./internal/observability ./internal/github ./internal/web -run 'Trace|StructuredLog'`
Expected: trace 传播或日志字段不完整。

**Step 3: Minimal implementation**
```text
为 webhook/command/pipeline 三类入口生成或继承 trace_id。
统一日志字段：trace_id, project_id, pipeline_id, issue_number, op, latency_ms。
```

**Step 4: Run tests to confirm pass**
Run: `go test ./internal/observability ./internal/github ./internal/web -run 'Trace|StructuredLog'`
Expected: PASS。

**Step 5: Commit**
```bash
git add internal/observability/trace.go internal/observability/trace_test.go internal/web/handlers_webhook.go internal/github/webhook_dispatcher.go internal/engine/executor.go
git commit -m "feat(obs): propagate trace id and structured logs"
```

### Task W25-T5 (gh-7e): 管理员逃生舱操作（force-ready/force-unblock/replay-delivery）

**Files:**
- Create: `internal/web/handlers_admin_ops.go`
- Create: `internal/web/handlers_admin_ops_test.go`
- Modify: `internal/web/server.go`
- Modify: `internal/core/events.go`
- Test: `internal/web/handlers_admin_ops_test.go`

**Depends on:** `[W25-T2, W25-T3, W25-T4]`

**Step 1: Write failing test**
```text
新增测试：
- TestAdminOps_ForceReady_Audited
- TestAdminOps_ForceUnblock_Audited
- TestAdminOps_ReplayDelivery_TriggersDispatcher
```

**Step 2: Run to confirm failure**
Run: `go test ./internal/web -run 'AdminOps|ForceReady|ForceUnblock|ReplayDelivery'`
Expected: 管理员接口不存在或审计记录缺失。

**Step 3: Minimal implementation**
```text
新增受保护管理接口，仅允许 admin token 或本地受信通道调用。
所有强制操作必须写入 human_actions/source=admin 并带 trace_id。
```

**Step 4: Run tests to confirm pass**
Run: `go test ./internal/web -run 'AdminOps|ForceReady|ForceUnblock|ReplayDelivery'`
Expected: PASS。

**Step 5: Commit**
```bash
git add internal/web/handlers_admin_ops.go internal/web/handlers_admin_ops_test.go internal/web/server.go internal/core/events.go
git commit -m "feat(admin): add escape hatch operations with audit trail"
```

### Task W25-T6 (gh-7f): GitHub App 权限探测与启动门禁

**Files:**
- Create: `internal/github/permissions_probe.go`
- Create: `internal/github/permissions_probe_test.go`
- Modify: `cmd/ai-flow/commands_github_validate.go`
- Modify: `internal/config/types.go`
- Test: `internal/github/permissions_probe_test.go`

**Depends on:** `[W2-T2]`

**Step 1: Write failing test**
```text
新增测试：
- TestPermissionsProbe_MissingIssueWrite_FailsValidation
- TestPermissionsProbe_MissingPRWrite_FailsValidation
- TestPermissionsProbe_GitHubAppInstallationPasses
```

**Step 2: Run to confirm failure**
Run: `go test ./internal/github ./cmd/ai-flow -run 'PermissionsProbe|Validate'`
Expected: 权限探测或门禁校验缺失。

**Step 3: Minimal implementation**
```text
validate 命令新增权限探测：issues/pr/contents/metadata。
配置要求优先 GitHub App；PAT 模式保留但提示风险并要求显式确认。
```

**Step 4: Run tests to confirm pass**
Run: `go test ./internal/github ./cmd/ai-flow -run 'PermissionsProbe|Validate'`
Expected: PASS。

**Step 5: Commit**
```bash
git add internal/github/permissions_probe.go internal/github/permissions_probe_test.go cmd/ai-flow/commands_github_validate.go internal/config/types.go
git commit -m "feat(github): add app permission probe and startup gate"
```

## Test Strategy Per Task

| Task | 单测 | 集成验证 |
|---|---|---|
| W25-T1 | 限流、重试、顺序性 | tracker/scm/status_sync 出站统一走队列 |
| W25-T2 | DLQ 入队、replay 幂等 | webhook 异常后可人工回放恢复 |
| W25-T3 | 状态漂移修复 | 漏事件后 10 分钟内收敛 |
| W25-T4 | trace 传播、日志字段 | issue->pipeline->pr 链路可串联 |
| W25-T5 | 强制操作与审计 | 故障演练中人工解锁可追溯 |
| W25-T6 | 权限探测、失败门禁 | 配置错误时启动前失败 |

## Risks and Mitigations

- 风险：补偿机制过重导致主链路变慢。  
  缓解：对账任务异步执行，主链路只做常数级入队动作。
- 风险：管理员接口被滥用。  
  缓解：严格鉴权 + 强审计 + 默认关闭。
- 风险：replay 导致重复写入。  
  缓解：delivery_id + idempotency key 双层去重。

## Wave E2E/Smoke Cases and Entry Data

### Entry Data
- 一组正常 webhook fixtures（issues.opened / issue_comment.created / pull_request.closed）。
- 一组故障 fixtures（429、timeout、dispatcher panic）。
- 一个启用 GitHub App 的项目配置样本。

### Smoke Cases
- 出站连续高压写入时，队列限流生效且无 GitHub 429 风暴。
- dispatcher 失败事件进入 DLQ，`github replay` 后状态收敛。
- 人工 `force-unblock` 后，下游任务可继续推进且审计可追溯。
- 通过 trace_id 可串联一次完整链路：Issue 触发 -> Pipeline -> PR 回写。

## Wave Exit Gate
- Execute mandatory gate sequence via `executing-wave-plans`.
- Wave-specific acceptance:
  - [ ] 所有 GitHub 写操作统一走出站队列并启用限流。
  - [ ] 限流参数与 spec 一致：`1 req/s + burst 5`，且 429/403 退避重试上限为 3 次。
  - [ ] Webhook DLQ + replay 可恢复单条失败事件且不重复执行。
  - [ ] reconcile 任务可修复漏事件导致的 blocked/label 漂移。
  - [ ] 管理员逃生舱与审计日志可用。
  - [ ] GitHub App 权限探测纳入启动/validate 门禁。
- Wave-specific verification:
  - [ ] `go test ./internal/github ./internal/observability ./internal/web ./cmd/ai-flow -run 'OutboundQueue|DLQ|Replay|Reconcile|Trace|AdminOps|PermissionsProbe|Validate'` 通过。
  - [ ] `go test ./internal/engine ./internal/secretary -run 'Action|Scheduler|Recovery'` 通过。
  - [ ] `go test -race ./internal/github/...` 通过。
- Boundary-change verification (if triggered):
  - [ ] 若改动了 webhook 事件契约，执行 `go test ./internal/web ./internal/eventbus -run 'Webhook|Event'` 并确认 PASS。

## Next Wave Entry Condition
- Governed by `executing-wave-plans` verdict rule (`Go` / satisfied `Conditional Go` only).

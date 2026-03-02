# P3 Wave 3 — 双向同步逻辑（gh-8~11）

> **For Agent:** REQUIRED SUB-SKILL: Use executing-wave-plans to implement this wave and gate it before Wave 4.

## Wave Goal

打通 GitHub 与本地执行引擎的双向主链路：Issue 触发 Pipeline、Slash 命令控制、Pipeline 状态回写、Draft PR 生命周期联动。

## Depends On

- `[W2-T1, W2-T2, W2-T3, W25-T1, W25-T2, W25-T3, W25-T4, W25-T6]`

## Wave Entry Data

- `tracker-github`、`scm-github`、Webhook dispatcher 已可用。
- GitHub 出站写操作统一经过 queue（带限流与幂等键）。
- Webhook 失败事件已进入 DLQ，支持 replay；对账任务可修复状态漂移。
- Trace ID 可从 webhook/命令入口贯穿到 Pipeline 与日志。
- Pipeline 主阶段命名以新 spec 为准：`requirements/worktree_setup/implement/code_review/fixup/e2e_test/merge/cleanup`。
- Pipeline 与 TaskItem 通过 `task_item_id` 关联，GitHub 层不得复制任务契约。

## Tasks

### Task W3-T1 (gh-8): Issue -> Pipeline 触发器

**Files:**
- Create: `internal/github/pipeline_trigger.go`
- Create: `internal/github/pipeline_trigger_test.go`
- Modify: `internal/web/handlers_webhook.go`
- Modify: `internal/engine/scheduler.go`
- Test: `internal/github/pipeline_trigger_test.go`

**Depends on:** `[W2-T1, W2-T3]`

**Step 1: Write failing test**
```text
新增测试：
- TestPipelineTrigger_LabelMapping_SelectsTemplate
- TestPipelineTrigger_Idempotent_NoDuplicatePipelineForSameIssue
- TestPipelineTrigger_CommandRun_UsesExplicitTemplate
```

**Step 2: Run to confirm failure**
Run: `go test ./internal/github -run 'TestPipelineTrigger_'`
Expected: 触发服务不存在或幂等断言失败。

**Step 3: Minimal implementation**
```text
实现 TriggerFromIssue / TriggerFromCommand。
保证幂等：同 project_id + issue_number 不重复创建 pipeline。
所有回帖/标签写入必须通过 outbound queue，携带 issue 维度 idempotency key 与 trace_id。
创建成功后在 Issue 回帖写入 pipeline_id 与可用命令。
```

**Step 4: Run tests to confirm pass**
Run: `go test ./internal/github -run 'TestPipelineTrigger_'`
Expected: PASS。

**Step 5: Commit**
```bash
git add internal/github/pipeline_trigger.go internal/github/pipeline_trigger_test.go internal/web/handlers_webhook.go internal/engine/scheduler.go
git commit -m "feat(github): add issue to pipeline trigger"
```

### Task W3-T2 (gh-9): Slash 命令解析 + 权限控制

**Files:**
- Create: `internal/github/slash_command.go`
- Create: `internal/github/slash_command_test.go`
- Modify: `internal/web/handlers_webhook.go`
- Modify: `internal/engine/actions.go`
- Test: `internal/github/slash_command_test.go`

**Depends on:** `[W2-T3]`

**Step 1: Write failing test**
```text
新增测试：
- TestParseSlashCommand_Approve
- TestParseSlashCommand_Reject_WithStageAndReason  # 示例: /reject implement 数据模型需要重做
- TestParseSlashCommand_Reject_CodeReview
- TestSlashACL_UnauthorizedUser_Denied
- TestSlashACL_AuthorAssociationMatrix_AppliesDefaultPermissions
- TestSlashACL_WhitelistOverridesAuthorAssociation
```

**Step 2: Run to confirm failure**
Run: `go test ./internal/github -run 'TestParseSlashCommand_|TestSlashACL_'`
Expected: 解析器缺失或旧 stage 名导致断言失败。

**Step 3: Minimal implementation**
```text
实现命令解析：/approve /reject /status /abort /run。
reject 仅允许当前模板中存在的 stage id；错误 stage 返回提示评论。
ACL 规则按 spec 执行：优先读取 `author_association` 映射命令权限，再由 `authorized_usernames` 白名单做覆盖放行。
命令执行日志必须落 trace_id + actor + association + action，便于审计与追踪。
```

**Step 4: Run tests to confirm pass**
Run: `go test ./internal/github -run 'TestParseSlashCommand_|TestSlashACL_'`
Expected: PASS。

**Step 5: Commit**
```bash
git add internal/github/slash_command.go internal/github/slash_command_test.go internal/web/handlers_webhook.go internal/engine/actions.go
git commit -m "feat(github): add slash commands and acl"
```

### Task W3-T3 (gh-10): Pipeline -> Issue 状态同步器

**Files:**
- Create: `internal/github/status_syncer.go`
- Create: `internal/github/status_syncer_test.go`
- Modify: `internal/engine/reactions.go`
- Modify: `internal/core/stage.go`
- Test: `internal/github/status_syncer_test.go`

**Depends on:** `[W2-T1, W2-T3]`

**Step 1: Write failing test**
```text
新增测试：
- TestStatusSyncer_StageStart_UpdatesStatusLabelByStageID
- TestStatusSyncer_HumanRequired_PostsActionComment
- TestStatusSyncer_Done_ReplacesPipelineActiveWithDone
- TestStatusSyncer_NoIssueNumber_SkipSilently
```

**Step 2: Run to confirm failure**
Run: `go test ./internal/github -run 'TestStatusSyncer_'`
Expected: 状态映射未实现或 stage label 不一致。

**Step 3: Minimal implementation**
```text
监听 stage_start/stage_complete/human_required/pipeline_done/pipeline_failed。
标签只使用新 stage id；同步失败记录 warning，不阻塞 pipeline。
所有状态写回统一提交 outbound queue；对账任务负责补偿修复漏写或乱序。
```

**Step 4: Run tests to confirm pass**
Run: `go test ./internal/github -run 'TestStatusSyncer_'`
Expected: PASS。

**Step 5: Commit**
```bash
git add internal/github/status_syncer.go internal/github/status_syncer_test.go internal/engine/reactions.go internal/core/stage.go
git commit -m "feat(github): sync pipeline status to issue"
```

### Task W3-T4 (gh-11): Draft PR 自动创建与生命周期处理

**Files:**
- Create: `internal/github/pr_lifecycle.go`
- Create: `internal/github/pr_lifecycle_test.go`
- Modify: `internal/engine/executor.go`
- Modify: `internal/web/handlers_webhook.go`
- Test: `internal/github/pr_lifecycle_test.go`

**Depends on:** `[W2-T2, W3-T3]`

**Step 1: Write failing test**
```text
新增测试：
- TestPRLifecycle_ImplementComplete_CreatesDraftPR
- TestPRLifecycle_MergeApproved_ConvertReadyThenMerge
- TestPRLifecycle_PullRequestClosedMerged_PipelineDone
- TestPRLifecycle_PullRequestClosedNotMerged_PipelineFailed
```

**Step 2: Run to confirm failure**
Run: `go test ./internal/github -run 'TestPRLifecycle_'`
Expected: 生命周期编排缺失。

**Step 3: Minimal implementation**
```text
implement 阶段完成后创建 Draft PR；code_review/fixup 评论持续追加。
PR 创建/评论/合并写入统一走 outbound queue，确保限流和幂等。
收到 pull_request.closed(merged=true) 时将 pipeline 置 done 并跳过剩余阶段。
```

**Step 4: Run tests to confirm pass**
Run: `go test ./internal/github -run 'TestPRLifecycle_'`
Expected: PASS。

**Step 5: Commit**
```bash
git add internal/github/pr_lifecycle.go internal/github/pr_lifecycle_test.go internal/engine/executor.go internal/web/handlers_webhook.go
git commit -m "feat(github): add draft pr lifecycle orchestration"
```

## Test Strategy Per Task

| Task | 单测 | 集成验证 |
|---|---|---|
| W3-T1 | 模板选择、幂等、防重复创建 | issues.opened + /run 回放 |
| W3-T2 | 命令解析、ACL、stage 校验 | issue_comment.created 回放 |
| W3-T3 | 标签替换、评论模板、无 issue 跳过 | pipeline 事件流回放 |
| W3-T4 | PR 创建、外部 merge、失败分支 | pull_request.closed/review 回放 |

## Risks and Mitigations

- 风险：Slash 命令与 Web UI action 语义分叉。  
  缓解：Slash 最终调用同一 Pipeline action API。
- 风险：旧 stage 名残留导致 reject 失效。  
  缓解：集中 stage 白名单，测试覆盖 `implement/code_review/fixup/e2e_test`。
- 风险：PR 外部动作与本地状态竞态。  
  缓解：Webhook 分发按 pipeline 或 issue 串行化 + 幂等检查。

## Wave E2E/Smoke Cases and Entry Data

### Entry Data
- Issue `#201` 带标签 `type: feature`。
- authorized user：`maintainer-a`。
- 两类评论者样本：`author_association=CONTRIBUTOR` 与 `author_association=NONE`。
- PR webhook fixtures：`pull_request.closed`, `pull_request_review.submitted`。

### Smoke Cases
- `issues.opened` 触发 pipeline 创建并在 issue 回帖。
- `/reject implement xxx` 触发 Pipeline action，错误 stage 给出失败提示。
- pipeline 完成后 issue 标签从 `pipeline: active` 切换为 `pipeline: done`。
- 对同一 delivery-id 回放不会产生重复 pipeline/评论/标签写入。
- 人工 replay 一条失败 webhook 后，状态在一次 reconcile 周期内收敛。

## Wave Exit Gate
- Execute mandatory gate sequence via `executing-wave-plans`.
- Wave-specific acceptance:
  - [ ] Issue/Slash/Status/PR 四条主链路全部打通。
  - [ ] Slash ACL 与 spec 权限矩阵一致（`author_association` 基线 + 白名单覆盖）。
  - [ ] 所有 stage 同步标签使用新 stage id（无旧规格阶段命名残留）。
  - [ ] 外部 merge 和本地 merge 两条路径都能收敛到一致状态。
  - [ ] GitHub 写操作无直写路径，全部通过 outbound queue。
  - [ ] Webhook replay + reconcile 在主链路中可验证且可恢复。
- Wave-specific verification:
  - [ ] `go test ./internal/github -run 'TestPipelineTrigger_|TestParseSlashCommand_|TestStatusSyncer_|TestPRLifecycle_'` 通过。
  - [ ] `go test ./internal/github -run 'TestOutboundQueue_|TestReconcileJob_|TestGitHubReplay'` 通过。
  - [ ] `go test ./internal/web ./internal/engine -run 'Webhook|Action|Reaction|Executor'` 通过。
- Boundary-change verification (if triggered):
  - [ ] 若修改了 pipeline 状态流转，执行 `go test ./internal/core ./internal/engine -run 'Pipeline|Stage|Recovery'` 并确认 PASS。

## Next Wave Entry Condition
- Governed by `executing-wave-plans` verdict rule (`Go` / satisfied `Conditional Go` only).

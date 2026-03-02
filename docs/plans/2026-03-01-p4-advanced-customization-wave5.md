# P4 Wave 5 — 外部联动收口（GitHub P4+ + Linear）

> **For Agent:** REQUIRED SUB-SKILL: Use executing-wave-plans to implement this wave and gate it before Wave 6.

## Wave Goal

完成 P4 对外能力收口：GitHub P4+ 指令、日志查询闭环、`tracker-linear` 插件接入。

## Depends On

- `[W4-T1, W4-T2, W4-T3]`

## Wave Entry Data

- P3 已具备 `/run`、`/approve`、`/reject`、`/status`、`/abort` 基础命令。
- Store 已支持日志写入/读取接口，但 API 和 GitHub `/logs` 指令未闭环。
- tracker 目前默认仅 local/github，不含 linear 实现。

## Tasks

### Task W5-T1: GitHub P4+ slash commands（modify/skip/rerun/switch/pause/resume）

**Files:**
- Modify: `internal/github/slash_command.go`
- Modify: `internal/github/slash_command_test.go`
- Modify: `internal/web/handlers_webhook.go`
- Modify: `internal/engine/actions.go`
- Modify: `internal/web/handlers_pipeline.go`
- Modify: `internal/web/handlers_pipeline_test.go`

**Depends on:** `[W4-T2]`

**Conflict Scope (for executor scheduling):**
- Files: `[internal/github/slash_command.go, internal/engine/actions.go, internal/web/handlers_pipeline.go]`
- Shared Critical File: `yes`

**Step 1: Write failing test**
```text
新增测试：
- TestParseSlashCommand_Modify_WithFeedback
- TestParseSlashCommand_Switch_WithAgent
- TestParseSlashCommand_PauseResume
- TestSlashCommand_ToPipelineActionMapping_P4Set
- TestSlashACL_P4Commands_UsesAuthorAssociationAndWhitelist
```

**Step 2: Run to confirm failure**
Run: `go test ./internal/github ./internal/web ./internal/engine -run 'SlashCommand|PauseResume|Modify|Switch|PipelineActionMapping'`
Expected: 命令解析或 action 映射缺失。

**Step 3: Minimal implementation**
```text
扩展 slash parser 支持：
- /modify <feedback>
- /skip
- /rerun
- /switch <agent>
- /pause
- /resume
统一映射到现有 pipeline action API，保持 ACL 矩阵与白名单覆盖规则。
```

**Step 4: Run tests to confirm pass**
Run: `go test ./internal/github ./internal/web ./internal/engine -run 'SlashCommand|PauseResume|Modify|Switch|PipelineActionMapping'`
Expected: PASS。

**Step 5: Commit**
```bash
git add internal/github/slash_command.go internal/github/slash_command_test.go internal/web/handlers_webhook.go internal/engine/actions.go internal/web/handlers_pipeline.go internal/web/handlers_pipeline_test.go
git commit -m "feat(github): add p4 slash commands mapped to pipeline actions"
```

### Task W5-T2: `/logs [stage]` 查询与 Issue 评论摘要

**Files:**
- Create: `internal/github/log_summary.go`
- Create: `internal/github/log_summary_test.go`
- Modify: `internal/github/slash_command.go`
- Modify: `internal/web/handlers_pipeline.go`
- Modify: `internal/web/handlers_pipeline_test.go`
- Modify: `internal/plugins/store-sqlite/store_test.go`

**Depends on:** `[W5-T1]`

**Conflict Scope (for executor scheduling):**
- Files: `[internal/github/slash_command.go, internal/web/handlers_pipeline.go, internal/plugins/store-sqlite/store_test.go]`
- Shared Critical File: `yes`

**Step 1: Write failing test**
```text
新增测试：
- TestParseSlashCommand_Logs_WithOptionalStage
- TestPipelineLogsEndpoint_FilterByStageAndLimit
- TestLogSummary_TruncatesLongContentAndGroupsByStage
- TestSlashLogs_PostsIssueCommentWithSummary
```

**Step 2: Run to confirm failure**
Run: `go test ./internal/github ./internal/web ./internal/plugins/store-sqlite -run 'Logs|LogSummary|SlashLogs'`
Expected: /logs 指令或查询接口不存在。

**Step 3: Minimal implementation**
```text
新增/补齐 pipeline logs API：支持按 stage/limit 查询。
实现 `/logs [stage]`：读取最近日志，生成摘要并评论到 issue。
摘要策略：按 stage 分组，单段截断，避免超长评论刷屏。
```

**Step 4: Run tests to confirm pass**
Run: `go test ./internal/github ./internal/web ./internal/plugins/store-sqlite -run 'Logs|LogSummary|SlashLogs'`
Expected: PASS。

**Step 5: Commit**
```bash
git add internal/github/log_summary.go internal/github/log_summary_test.go internal/github/slash_command.go internal/web/handlers_pipeline.go internal/web/handlers_pipeline_test.go internal/plugins/store-sqlite/store_test.go
git commit -m "feat(github): add slash logs and pipeline log summary"
```

### Task W5-T3: `tracker-linear` 插件与工厂接入

**Files:**
- Create: `internal/plugins/tracker-linear/tracker.go`
- Create: `internal/plugins/tracker-linear/tracker_test.go`
- Create: `internal/plugins/tracker-linear/module.go`
- Modify: `internal/config/types.go`
- Modify: `internal/config/defaults.go`
- Modify: `internal/config/merge.go`
- Modify: `internal/plugins/factory/factory.go`
- Modify: `internal/plugins/factory/factory_test.go`
- Modify: `internal/core/tracker.go`

**Depends on:** `[W2-T1, W5-T2]`

**Conflict Scope (for executor scheduling):**
- Files: `[internal/plugins/factory/factory.go, internal/config/types.go, internal/core/tracker.go]`
- Shared Critical File: `yes`

**Step 1: Write failing test**
```text
新增测试：
- TestLinearTracker_CreateTask_ReturnsExternalID
- TestLinearTracker_UpdateStatus_MapsTaskStates
- TestFactory_TrackerPlugin_LinearWhenConfigured
- TestFactory_TrackerPlugin_FallbackLocalWhenLinearDisabled
```

**Step 2: Run to confirm failure**
Run: `go test ./internal/plugins/tracker-linear ./internal/plugins/factory ./internal/config -run 'LinearTracker|TrackerPlugin|Factory'`
Expected: 插件未实现或 factory 选择失败。

**Step 3: Minimal implementation**
```text
实现 tracker-linear 最小闭环：CreateTask/UpdateStatus/SyncDependencies/OnExternalComplete。
新增 linear 配置段（api_url/token/team_id/state_mapping）。
factory 支持 tracker 选择：local/github/linear，未配置时回退 local。
```

**Step 4: Run tests to confirm pass**
Run: `go test ./internal/plugins/tracker-linear ./internal/plugins/factory ./internal/config -run 'LinearTracker|TrackerPlugin|Factory'`
Expected: PASS。

**Step 5: Commit**
```bash
git add internal/plugins/tracker-linear/tracker.go internal/plugins/tracker-linear/tracker_test.go internal/plugins/tracker-linear/module.go internal/config/types.go internal/config/defaults.go internal/config/merge.go internal/plugins/factory/factory.go internal/plugins/factory/factory_test.go internal/core/tracker.go
git commit -m "feat(tracker): add linear tracker plugin and factory selection"
```

## Test Strategy Per Task

| Task | Unit | Integration |
|---|---|---|
| W5-T1 | 命令解析与 action 映射 | webhook -> action API 闭环 |
| W5-T2 | 日志查询参数、摘要格式 | slash /logs 评论输出回放 |
| W5-T3 | tracker-linear 状态映射 | factory 选择 + scheduler 调用 smoke |

## Risks and Mitigations

- 风险：P4 命令扩展破坏 P3 命令兼容。  
  缓解：保留 P3 命令回归测试，新增命令不改旧路径。
- 风险：`/logs` 评论过长导致 API 失败。  
  缓解：摘要长度上限 + 分段截断 + 错误兜底提示。
- 风险：linear API 抖动影响调度主流程。  
  缓解：tracker 插件保持 warning 降级，不阻塞本地状态流转。

## Wave E2E/Smoke Cases and Entry Data

### Entry Data
- GitHub issue/comment fixtures：包含 `/modify`、`/switch codex`、`/logs implement`。
- 一条包含多 stage 日志的 pipeline fixture。
- Linear 假服务（创建任务、更新状态、网络失败）。

### Smoke Cases
- `/switch codex` 能触发 `change_agent` 并继续执行。
- `/logs implement` 能返回实现阶段摘要评论。
- tracker-linear 失败时仅 warning，本地 task 状态仍推进。

## Wave Exit Gate
- Execute mandatory gate sequence via `executing-wave-plans`.
- Wave-specific acceptance:
  - [ ] GitHub P4+ slash commands 全部可用并与 action API 对齐。
  - [ ] `/logs` 查询与评论摘要闭环可用。
  - [ ] `tracker-linear` 插件可配置可降级。
- Wave-specific verification:
  - [ ] `go test ./internal/github ./internal/web ./internal/engine -run 'Slash|Logs|Action'` 通过。
  - [ ] `go test ./internal/plugins/tracker-linear ./internal/plugins/factory ./internal/config -run 'LinearTracker|Factory|TrackerPlugin'` 通过。
- Boundary-change verification (if triggered):
  - [ ] 若修改 webhook 命令分发，执行 `go test ./internal/web -run 'Webhook|Server|Auth'` 并确认 PASS。

## Next Wave Entry Condition
- Governed by `executing-wave-plans` verdict rule (`Go` / satisfied `Conditional Go` only), then enter Wave 6.

# P4 Wave 3 — Reactions V2

> **For Agent:** REQUIRED SUB-SKILL: Use executing-wave-plans to implement this wave and gate it before Wave 4.

## Wave Goal

将当前“on_failure 语法糖 + 简化匹配”升级为可配置 Reactions V2，并引入 `notify` / `spawn_agent` 动作。

## Depends On

- `[W2-T1, W2-T2, W2-T3]`

## Wave Entry Data

- 现有 `internal/engine/reactions.go` 仅支持固定动作：retry/escalate/skip/abort。
- 通知能力在 Wave 2 后具备多通道发信底座。
- 当前 Reaction 规则不支持配置层覆盖和动作参数。
- 执行器构造入口位于 `cmd/ai-flow/commands.go`，并有 `internal/engine/executor_behavior_test.go` 覆盖构造行为。

## Tasks

### Task W3-T1: Reactions 配置模型与解析器

**Files:**
- Create: `internal/core/reaction.go`
- Modify: `internal/config/types.go`
- Modify: `internal/config/defaults.go`
- Modify: `internal/config/merge.go`
- Modify: `internal/config/config_test.go`
- Modify: `internal/config/merge_hierarchy_test.go`
- Create: `internal/config/reactions_config_test.go`
- Modify: `configs/defaults.yaml`

**Depends on:** `[]`

**Conflict Scope (for executor scheduling):**
- Files: `[internal/config/types.go, internal/config/merge.go, configs/defaults.yaml]`
- Shared Critical File: `yes`

**Step 1: Write failing test**
```text
新增测试：
- TestReactionsConfig_DefaultRules_Loaded
- TestMergeHierarchy_Reactions_ProjectOverridesGlobal
- TestReactionRule_Validation_RejectsUnknownAction
- TestReactionRule_Validation_RequiresConditions
```

**Step 2: Run to confirm failure**
Run: `go test ./internal/config ./internal/core -run 'ReactionsConfig|ReactionRule|MergeHierarchy'`
Expected: Reactions 结构体或校验逻辑缺失。

**Step 3: Minimal implementation**
```text
定义 ReactionRule 配置结构（event/stage/error_type/action/params/priority）。
支持 global/project/pipeline 三层覆盖合并。
实现配置级校验：action 枚举、条件最小集、参数类型合法。
```

**Step 4: Run tests to confirm pass**
Run: `go test ./internal/config ./internal/core -run 'ReactionsConfig|ReactionRule|MergeHierarchy'`
Expected: PASS。

**Step 5: Commit**
```bash
git add internal/core/reaction.go internal/config/types.go internal/config/defaults.go internal/config/merge.go internal/config/config_test.go internal/config/merge_hierarchy_test.go internal/config/reactions_config_test.go configs/defaults.yaml
git commit -m "feat(reactions): add config schema and validation for reactions v2"
```

### Task W3-T2: 执行引擎支持 `notify` / `spawn_agent` 动作

**Files:**
- Modify: `internal/engine/reactions.go`
- Create: `internal/engine/reaction_dispatcher.go`
- Create: `internal/engine/reaction_dispatcher_test.go`
- Modify: `internal/engine/executor.go`
- Modify: `internal/engine/executor_behavior_test.go`
- Modify: `internal/engine/reactions_test.go`
- Modify: `cmd/ai-flow/commands.go`
- Modify: `cmd/ai-flow/commands_test.go`
- Test: `internal/engine/integration_p1_test.go`

**Depends on:** `[W3-T1]`

**Conflict Scope (for executor scheduling):**
- Files: `[internal/engine/reactions.go, internal/engine/executor.go, cmd/ai-flow/commands.go]`
- Shared Critical File: `yes`

**Step 1: Write failing test**
```text
新增测试：
- TestEvaluateReactionRules_FirstMatchWins_WithPriority
- TestReactionDispatcher_NotifyAction_InvokesNotifier
- TestReactionDispatcher_SpawnAgentAction_PublishesStructuredEvent
- TestExecutor_ReactionConfig_OverridesOnFailureSugar
- TestBootstrapWithEventBus_ExecutorGetsNotifierDependency
- TestExecutor_ReactionNotify_WithoutNotifier_DoesNotPanic
```

**Step 2: Run to confirm failure**
Run: `go test ./internal/engine ./cmd/ai-flow -run 'ReactionDispatcher|EvaluateReactionRules|OverridesOnFailure|ExecutorGetsNotifier|ReactionNotify'`
Expected: 新动作未定义或执行器未接入。

**Step 3: Minimal implementation**
```text
扩展 ReactionAction：notify/spawn_agent。
新增 dispatcher：
- notify -> 调用注入的 notifier。
- spawn_agent -> 发布结构化事件 + 记录 action，默认不阻塞当前 pipeline。
扩展 `Executor` 依赖：显式持有 `core.Notifier`（可为 nil），并在 `NewExecutor(...)` 构造时注入。
更新 `bootstrapWithEventBus` 与相关测试，确保 notifier 由 `pluginfactory.BootstrapSet` 传入执行器。
保留兼容：无配置时继续走 on_failure 编译规则。
```

**Step 4: Run tests to confirm pass**
Run: `go test ./internal/engine ./cmd/ai-flow -run 'ReactionDispatcher|EvaluateReactionRules|OverridesOnFailure|ExecutorGetsNotifier|ReactionNotify'`
Expected: PASS。

**Step 5: Commit**
```bash
git add internal/engine/reactions.go internal/engine/reaction_dispatcher.go internal/engine/reaction_dispatcher_test.go internal/engine/executor.go internal/engine/executor_behavior_test.go internal/engine/reactions_test.go internal/engine/integration_p1_test.go cmd/ai-flow/commands.go cmd/ai-flow/commands_test.go
git commit -m "feat(engine): add reactions v2 actions notify and spawn_agent"
```

### Task W3-T3: Reactions 与通知/审计链路打通

**Files:**
- Modify: `internal/core/events.go`
- Modify: `internal/engine/executor.go`
- Modify: `internal/plugins/store-sqlite/store.go`
- Modify: `internal/plugins/store-sqlite/store_test.go`
- Modify: `internal/web/ws.go`
- Create: `internal/engine/reactions_integration_test.go`

**Depends on:** `[W3-T2, W2-T1]`

**Conflict Scope (for executor scheduling):**
- Files: `[internal/engine/executor.go, internal/plugins/store-sqlite/store.go, internal/core/events.go]`
- Shared Critical File: `yes`

**Step 1: Write failing test**
```text
新增测试：
- TestReactionsIntegration_NotifyAction_WritesHumanActionSourceReaction
- TestReactionsIntegration_SpawnAgentAction_EmitsEventBusPayload
- TestWSBroadcast_ReactionEvents_AreVisibleToSubscribers
```

**Step 2: Run to confirm failure**
Run: `go test ./internal/engine ./internal/plugins/store-sqlite ./internal/web -run 'ReactionsIntegration|ReactionEvents|sourceReaction'`
Expected: 审计记录或广播事件缺失。

**Step 3: Minimal implementation**
```text
为 reaction 动作统一写 `human_actions`（source="reaction"）。
新增事件类型：`reaction_notify_sent`、`reaction_spawn_agent_requested`。
确保 ws 广播可见，用于 UI 追踪自动化动作。
```

**Step 4: Run tests to confirm pass**
Run: `go test ./internal/engine ./internal/plugins/store-sqlite ./internal/web -run 'ReactionsIntegration|ReactionEvents|sourceReaction'`
Expected: PASS。

**Step 5: Commit**
```bash
git add internal/core/events.go internal/engine/executor.go internal/plugins/store-sqlite/store.go internal/plugins/store-sqlite/store_test.go internal/web/ws.go internal/engine/reactions_integration_test.go
git commit -m "feat(reactions): persist and broadcast reaction side effects"
```

## Test Strategy Per Task

| Task | Unit | Integration |
|---|---|---|
| W3-T1 | 规则模型校验、merge 优先级 | 配置层叠加回放 |
| W3-T2 | 动作匹配、执行器行为 | executor 失败路径回放 |
| W3-T3 | 存储审计与事件广播 | ws + sqlite 端到端链路 |

## Risks and Mitigations

- 风险：规则优先级不清导致动作误触发。  
  缓解：引入 priority + first-match-wins 测试。
- 风险：`spawn_agent` 行为过重影响主链路时延。  
  缓解：默认异步发布请求事件，不同步等待子流程完成。

## Wave E2E/Smoke Cases and Entry Data

### Entry Data
- 一条 `stage_failed` 事件样例（network timeout）。
- 一条 `pipeline_stuck` 事件样例。
- 多通道 notifier 配置（desktop + webhook）。

### Smoke Cases
- `stage_failed` 匹配到 `notify` 规则后，能看到通知 + reaction action 记录。
- `pipeline_stuck` 匹配到 `spawn_agent` 后，能看到结构化事件被广播。
- 未配置 reactions 时行为与旧 on_failure 语义一致。

## Wave Exit Gate
- Execute mandatory gate sequence via `executing-wave-plans`.
- Wave-specific acceptance:
  - [ ] Reactions V2 规则可配置且可覆盖 on_failure 语法糖。
  - [ ] `notify` / `spawn_agent` 动作具备稳定审计与广播链路。
- Wave-specific verification:
  - [ ] `go test ./internal/config ./internal/core ./internal/engine ./cmd/ai-flow -run 'Reaction|ReactionsConfig|Dispatcher|ExecutorGetsNotifier'` 通过。
  - [ ] `go test ./internal/plugins/store-sqlite ./internal/web -run 'Reaction|WS|HumanAction'` 通过。
- Boundary-change verification (if triggered):
  - [ ] 若修改了事件类型，执行 `go test ./internal/eventbus ./internal/web -run 'Event|Broadcast'` 并确认 PASS。

## Next Wave Entry Condition
- Governed by `executing-wave-plans` verdict rule (`Go` / satisfied `Conditional Go` only).

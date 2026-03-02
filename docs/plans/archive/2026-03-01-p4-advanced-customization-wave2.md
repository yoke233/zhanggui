# P4 Wave 2 — 通知插件与装配

> **For Agent:** REQUIRED SUB-SKILL: Use executing-wave-plans to implement this wave and gate it before Wave 3.

## Wave Goal

把 Notifier 从单一 desktop 插件升级为可配置、多通道、可降级的通知体系，为 Reactions V2 提供稳定通知底座。

## Depends On

- `[W1-T1, W1-T2, W1-T3]`

## Wave Entry Data

- 当前仅 `notifier-desktop` 可用，factory 固定选择 desktop。
- `core.Notifier` 仅支持单实例接口，缺少 fan-out 组合器。
- 配置层无 notifier 通道列表与参数结构。

## Tasks

### Task W2-T1: Notifier 配置模型 + fan-out 组合器 + factory 装配

**Files:**
- Modify: `internal/config/types.go`
- Modify: `internal/config/defaults.go`
- Modify: `internal/config/merge.go`
- Modify: `internal/config/config_test.go`
- Modify: `internal/config/merge_hierarchy_test.go`
- Create: `internal/plugins/notifier-fanout/notifier.go`
- Create: `internal/plugins/notifier-fanout/notifier_test.go`
- Create: `internal/plugins/notifier-fanout/module.go`
- Modify: `internal/plugins/factory/factory.go`
- Modify: `internal/plugins/factory/factory_test.go`
- Modify: `configs/defaults.yaml`

**Depends on:** `[]`

**Conflict Scope (for executor scheduling):**
- Files: `[internal/config/types.go, internal/plugins/factory/factory.go, configs/defaults.yaml]`
- Shared Critical File: `yes`

**Step 1: Write failing test**
```text
新增测试：
- TestDefaults_NotifierConfig_HasDesktopChannel
- TestMergeHierarchy_NotifierChannels_ProjectOverridesGlobal
- TestFactory_BuildsFanoutNotifier_WhenMultipleChannelsEnabled
- TestFanoutNotifier_OneChannelFail_DoesNotBreakOthers
```

**Step 2: Run to confirm failure**
Run: `go test ./internal/config ./internal/plugins/notifier-fanout ./internal/plugins/factory -run 'Notifier|Fanout|MergeHierarchy'`
Expected: 配置结构与装配逻辑缺失。

**Step 3: Minimal implementation**
```text
新增 notifier 配置模型：enabled/channels/slack/webhook/desktop。
实现 fan-out Notifier：并行发送，聚合错误，默认 best-effort。
factory 根据配置构建单通道或 fan-out 实例；零配置回退 desktop。
```

**Step 4: Run tests to confirm pass**
Run: `go test ./internal/config ./internal/plugins/notifier-fanout ./internal/plugins/factory -run 'Notifier|Fanout|MergeHierarchy'`
Expected: PASS。

**Step 5: Commit**
```bash
git add internal/config/types.go internal/config/defaults.go internal/config/merge.go internal/config/config_test.go internal/config/merge_hierarchy_test.go internal/plugins/notifier-fanout/notifier.go internal/plugins/notifier-fanout/notifier_test.go internal/plugins/notifier-fanout/module.go internal/plugins/factory/factory.go internal/plugins/factory/factory_test.go configs/defaults.yaml
git commit -m "feat(notifier): add config-driven fanout notifier composition"
```

### Task W2-T2: `notifier-webhook` 插件

**Files:**
- Create: `internal/plugins/notifier-webhook/notifier.go`
- Create: `internal/plugins/notifier-webhook/notifier_test.go`
- Create: `internal/plugins/notifier-webhook/module.go`
- Modify: `internal/plugins/factory/factory.go`
- Modify: `internal/plugins/factory/factory_test.go`

**Depends on:** `[W2-T1]`

**Conflict Scope (for executor scheduling):**
- Files: `[internal/plugins/factory/factory.go, internal/plugins/notifier-webhook/notifier.go]`
- Shared Critical File: `yes`

**Step 1: Write failing test**
```text
新增测试：
- TestWebhookNotifier_Notify_SendsJSONPayload
- TestWebhookNotifier_Timeout_ReturnsCategorizedError
- TestFactory_BuildWebhookNotifier_FromConfig
```

**Step 2: Run to confirm failure**
Run: `go test ./internal/plugins/notifier-webhook ./internal/plugins/factory -run 'WebhookNotifier|BuildWebhookNotifier'`
Expected: 插件未注册或请求契约未实现。

**Step 3: Minimal implementation**
```text
实现 webhook notifier：POST JSON，支持 header 注入、超时、重试(最小 1 次)。
错误分类为 network/http_status/serialization，供上层日志与统计使用。
注册 `notifier-webhook` 到默认 registry。
```

**Step 4: Run tests to confirm pass**
Run: `go test ./internal/plugins/notifier-webhook ./internal/plugins/factory -run 'WebhookNotifier|BuildWebhookNotifier'`
Expected: PASS。

**Step 5: Commit**
```bash
git add internal/plugins/notifier-webhook/notifier.go internal/plugins/notifier-webhook/notifier_test.go internal/plugins/notifier-webhook/module.go internal/plugins/factory/factory.go internal/plugins/factory/factory_test.go
git commit -m "feat(notifier): add webhook notifier plugin"
```

### Task W2-T3: `notifier-slack` 插件

**Files:**
- Create: `internal/plugins/notifier-slack/notifier.go`
- Create: `internal/plugins/notifier-slack/notifier_test.go`
- Create: `internal/plugins/notifier-slack/module.go`
- Modify: `internal/plugins/factory/factory.go`
- Modify: `internal/plugins/factory/factory_test.go`

**Depends on:** `[W2-T1]`

**Conflict Scope (for executor scheduling):**
- Files: `[internal/plugins/factory/factory.go, internal/plugins/notifier-slack/notifier.go]`
- Shared Critical File: `yes`

**Step 1: Write failing test**
```text
新增测试：
- TestSlackNotifier_Notify_BuildsExpectedMessage
- TestSlackNotifier_Non2xx_ReturnsError
- TestFactory_BuildSlackNotifier_FromConfig
```

**Step 2: Run to confirm failure**
Run: `go test ./internal/plugins/notifier-slack ./internal/plugins/factory -run 'SlackNotifier|BuildSlackNotifier'`
Expected: 插件不存在或装配失败。

**Step 3: Minimal implementation**
```text
实现 slack notifier（incoming webhook 模式）：
- 标题/级别/pipeline 信息拼接到 text blocks。
- 非 2xx 返回错误但不 panic。
注册 `notifier-slack` 到默认 registry。
```

**Step 4: Run tests to confirm pass**
Run: `go test ./internal/plugins/notifier-slack ./internal/plugins/factory -run 'SlackNotifier|BuildSlackNotifier'`
Expected: PASS。

**Step 5: Commit**
```bash
git add internal/plugins/notifier-slack/notifier.go internal/plugins/notifier-slack/notifier_test.go internal/plugins/notifier-slack/module.go internal/plugins/factory/factory.go internal/plugins/factory/factory_test.go
git commit -m "feat(notifier): add slack notifier plugin"
```

## Test Strategy Per Task

| Task | Unit | Integration |
|---|---|---|
| W2-T1 | 配置 merge、fan-out 并发/容错 | BuildFromConfig 多通道组合回放 |
| W2-T2 | webhook payload、超时、错误分类 | httptest 验证 header/body/status |
| W2-T3 | slack 消息格式与错误分支 | httptest 验证非 2xx 处理 |

## Risks and Mitigations

- 风险：多通道并行导致通知风暴或重复。  
  缓解：fan-out 提供通道级幂等 key 与最小节流窗口。
- 风险：外部通知失败污染主链路错误语义。  
  缓解：通知错误单独归档，不覆盖 Pipeline 主错误。

## Wave E2E/Smoke Cases and Entry Data

### Entry Data
- 配置 A：仅 desktop。
- 配置 B：desktop + webhook。
- 配置 C：desktop + slack + webhook。

### Smoke Cases
- 配置 C 下触发 `pipeline_failed` 通知时，三通道并行发送。
- 其中 webhook 失败时，desktop/slack 仍成功，主流程状态不变。
- 配置 A 行为与 P3 基线一致。

## Wave Exit Gate
- Execute mandatory gate sequence via `executing-wave-plans`.
- Wave-specific acceptance:
  - [ ] Notifier 装配可支持单通道和多通道 fan-out。
  - [ ] Slack/Webhook 插件具备最小可用错误处理与测试覆盖。
- Wave-specific verification:
  - [ ] `go test ./internal/config ./internal/plugins/factory ./internal/plugins/notifier-fanout ./internal/plugins/notifier-slack ./internal/plugins/notifier-webhook -run 'Notifier|Fanout|Slack|Webhook'` 通过。
  - [ ] `go test ./internal/plugins/notifier-fanout ./internal/plugins/notifier-slack ./internal/plugins/notifier-webhook -run 'OneChannelFail|Timeout|Non2xx|Notify'` 至少一条 smoke 用例通过。
- Boundary-change verification (if triggered):
  - [ ] 若修改了工厂默认选择规则，执行 `go test ./internal/plugins/factory -run 'BuildFromConfig|Defaults|Override'` 并确认 PASS。

## Next Wave Entry Condition
- Governed by `executing-wave-plans` verdict rule (`Go` / satisfied `Conditional Go` only).

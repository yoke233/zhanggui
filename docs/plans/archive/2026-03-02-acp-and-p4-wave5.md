# Wave 5 — P4: 模板 + 通知 + Reactions

> **Wave Goal:** 在 ACP 基础上交付 P4 第一批功能：自定义模板体系、Notifier 多通道 fan-out、Reactions V2 规则引擎。

## 任务列表

### Task W5-T1: custom_templates 配置与解析

**Files:**
- Modify: `internal/config/config.go`
- Create: `internal/engine/templates.go`
- Test: `internal/engine/templates_test.go`

**Depends on:** `[]`

**Step 1: Write failing test**
```go
func TestCustomTemplateConfigParsing(t *testing.T) {
    cfg := loadTestConfig(t, `
custom_templates:
  - name: "security-review"
    stages:
      - name: requirements
        prompt_template: "configs/prompts/security_requirements.tmpl"
      - name: implement
        prompt_template: "configs/prompts/security_implement.tmpl"
      - name: code_review
        prompt_template: "configs/prompts/security_review.tmpl"
`)
    require.Len(t, cfg.CustomTemplates, 1)
    require.Equal(t, "security-review", cfg.CustomTemplates[0].Name)
    require.Len(t, cfg.CustomTemplates[0].Stages, 3)
}

func TestCustomTemplateResolution(t *testing.T) {
    // 三级合并：global < project < pipeline
    resolver := engine.NewTemplateResolver(globalCfg, projectCfg)
    tmpl, err := resolver.Resolve("security-review")
    require.NoError(t, err)
    require.Equal(t, 3, len(tmpl.Stages))
}

func TestBuiltinTemplatesStillWork(t *testing.T) {
    // full / standard / quick / hotfix 保持可用
    resolver := engine.NewTemplateResolver(defaultCfg, nil)
    for _, name := range []string{"full", "standard", "quick", "hotfix"} {
        tmpl, err := resolver.Resolve(name)
        require.NoError(t, err)
        require.NotEmpty(t, tmpl.Stages)
    }
}
```

**Step 2: Run to confirm failure**
Run: `go test ./internal/engine/ -run TestCustomTemplate -count=1`
Expected: `compilation error`

**Step 3: Minimal implementation**
- 配置新增 `CustomTemplates []TemplateConfig`
- `TemplateResolver` 实现三级合并（global → project → pipeline）
- 4 个预设模板保持不变，自定义模板叠加

**Step 4: Run tests to confirm pass**
Run: `go test ./internal/engine/ -run TestCustomTemplate -count=1`
Expected: `PASS`

**Step 5: Commit**
```bash
git add internal/config/config.go internal/engine/templates.go internal/engine/templates_test.go
git commit -m "feat(engine): custom_templates configuration and resolution"
```

---

### Task W5-T2: Notifier 多通道 + fan-out

**Files:**
- Create: `internal/plugins/notifier-fanout/fanout.go`
- Create: `internal/plugins/notifier-fanout/fanout_test.go`
- Create: `internal/plugins/notifier-slack/slack.go`
- Create: `internal/plugins/notifier-slack/slack_test.go`
- Create: `internal/plugins/notifier-webhook/webhook.go`
- Create: `internal/plugins/notifier-webhook/webhook_test.go`
- Modify: `internal/plugins/factory/factory.go`

**Depends on:** `[]`

**Step 1: Write failing test**
```go
// fanout_test.go
func TestFanoutNotifier(t *testing.T) {
    ch1 := &mockNotifier{name: "slack"}
    ch2 := &mockNotifier{name: "webhook"}
    fanout := notifierfanout.New(ch1, ch2)

    err := fanout.Notify(context.Background(), core.Notification{
        Type:    "pipeline_completed",
        Message: "Pipeline #42 完成",
    })
    require.NoError(t, err)
    require.True(t, ch1.called)
    require.True(t, ch2.called)
}

func TestFanoutIsolation(t *testing.T) {
    // ch1 失败不影响 ch2
    ch1 := &mockNotifier{name: "slack", shouldFail: true}
    ch2 := &mockNotifier{name: "webhook"}
    fanout := notifierfanout.New(ch1, ch2)

    err := fanout.Notify(context.Background(), core.Notification{Message: "test"})
    // fanout 本身不返回错误（失败记录日志）
    require.NoError(t, err)
    require.True(t, ch2.called)
}

// slack_test.go
func TestSlackNotifier(t *testing.T) {
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(200)
    }))
    defer server.Close()

    n := notifierslack.New(notifierslack.Config{WebhookURL: server.URL})
    err := n.Notify(context.Background(), core.Notification{Message: "test"})
    require.NoError(t, err)
}

// webhook_test.go
func TestWebhookNotifier(t *testing.T) {
    var received []byte
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        received, _ = io.ReadAll(r.Body)
        w.WriteHeader(200)
    }))
    defer server.Close()

    n := notifierwebhook.New(notifierwebhook.Config{URL: server.URL})
    err := n.Notify(context.Background(), core.Notification{Message: "hello"})
    require.NoError(t, err)
    require.Contains(t, string(received), "hello")
}
```

**Step 2: Run to confirm failure**
Run: `go test ./internal/plugins/notifier-fanout/ -count=1`
Expected: `package does not exist`

**Step 3: Minimal implementation**
- `notifier-fanout`: 并发发送到所有子 notifier，单通道失败不阻断
- `notifier-slack`: Slack Incoming Webhook POST（JSON payload）
- `notifier-webhook`: Generic HTTP POST（JSON body）
- Factory 注册 fan-out 装配逻辑

**Step 4: Run tests to confirm pass**
Run:
```bash
go test ./internal/plugins/notifier-fanout/ -count=1
go test ./internal/plugins/notifier-slack/ -count=1
go test ./internal/plugins/notifier-webhook/ -count=1
```
Expected: `PASS`

**Step 5: Commit**
```bash
git add internal/plugins/notifier-fanout/ internal/plugins/notifier-slack/ internal/plugins/notifier-webhook/ internal/plugins/factory/factory.go
git commit -m "feat(notifier): multi-channel fanout with slack and webhook plugins"
```

---

### Task W5-T3: Reactions V2 规则引擎

**Files:**
- Modify: `internal/engine/reactions.go` (或新建)
- Test: `internal/engine/reactions_test.go`

**Depends on:** `[W5-T2]`

**Step 1: Write failing test**
```go
func TestReactionsV2RuleMatching(t *testing.T) {
    rules := []engine.ReactionRule{
        {
            Event:     "stage_failed",
            Condition: `event.retry_count < 3`,
            Action:    "retry",
        },
        {
            Event:     "pipeline_completed",
            Condition: `true`,
            Action:    "notify",
            ActionConfig: map[string]string{"message": "Pipeline {{.PipelineID}} 完成"},
        },
    }

    re := engine.NewReactionsEngine(rules, engine.ReactionsConfig{
        Notifier: testNotifier,
    })

    // 测试 stage_failed 匹配 retry
    action := re.Match(core.Event{Type: "stage_failed", Data: map[string]any{"retry_count": 1}})
    require.Equal(t, "retry", action.Type)

    // 测试 pipeline_completed 匹配 notify
    action2 := re.Match(core.Event{Type: "pipeline_completed", Data: map[string]any{"pipeline_id": "42"}})
    require.Equal(t, "notify", action2.Type)
}

func TestReactionsV2SpawnAgent(t *testing.T) {
    rules := []engine.ReactionRule{
        {Event: "review_rejected", Action: "spawn_agent", ActionConfig: map[string]string{"prompt": "修复审核问题"}},
    }
    re := engine.NewReactionsEngine(rules, engine.ReactionsConfig{
        ACPClient: testACPClient,
    })
    action := re.Match(core.Event{Type: "review_rejected"})
    require.Equal(t, "spawn_agent", action.Type)
}
```

**Step 2: Run to confirm failure**
Run: `go test ./internal/engine/ -run TestReactionsV2 -count=1`
Expected: `undefined: ReactionRule`

**Step 3: Minimal implementation**
```go
type ReactionRule struct {
    Event        string            `yaml:"event"`
    Condition    string            `yaml:"condition"`
    Action       string            `yaml:"action"`       // retry|escalate_human|skip|abort|notify|spawn_agent
    ActionConfig map[string]string `yaml:"action_config"`
}

type ReactionsEngine struct {
    rules    []ReactionRule
    notifier core.NotifierPlugin
    acpClient *acpclient.Client
}

func (re *ReactionsEngine) Match(evt core.Event) *ReactionAction { ... }
func (re *ReactionsEngine) Execute(ctx context.Context, action *ReactionAction) error { ... }
```

- `notify` 动作调用 Notifier fan-out
- `spawn_agent` 动作创建一次性 ACP session
- 条件表达式用简单 Go 模板或 map 匹配

**Step 4: Run tests to confirm pass**
Run: `go test ./internal/engine/ -run TestReactionsV2 -count=1`
Expected: `PASS`

**Step 5: Commit**
```bash
git add internal/engine/reactions.go internal/engine/reactions_test.go
git commit -m "feat(engine): Reactions V2 with notify and spawn_agent actions"
```

---

## Risks and Mitigations

| Risk | Severity | Mitigation |
|------|----------|------------|
| 自定义模板与预设模板冲突 | 低 | 名称唯一性校验，预设模板不可被覆盖 |
| Slack webhook 认证/格式问题 | 低 | 使用 httptest mock，不依赖真实 Slack |
| Reactions 条件表达式注入 | 中 | 限制表达式语法为简单比较，不执行任意代码 |
| spawn_agent 资源泄漏 | 中 | 一次性 session 自动关闭，设超时保护 |

## Wave Exit Gate
- Execute mandatory gate sequence via `executing-wave-plans`.
- Wave-specific acceptance:
  - [ ] 自定义模板可三级合并解析并执行
  - [ ] 预设模板（full/standard/quick/hotfix）不受影响
  - [ ] Notifier fan-out 可并行发送到 slack + webhook
  - [ ] 单通道失败不阻断主流程
  - [ ] Reactions V2 支持 6 种动作：retry/escalate_human/skip/abort/notify/spawn_agent
- Wave-specific verification:
  - [ ] `go test ./internal/engine/... -count=1 -v` — PASS
  - [ ] `go test ./internal/plugins/notifier-*/ -count=1` — 全部 PASS
  - [ ] `go build ./...` — 全量编译通过

## Next Wave Entry Condition
- Governed by `executing-wave-plans` verdict rule (`Go` / satisfied `Conditional Go` only).

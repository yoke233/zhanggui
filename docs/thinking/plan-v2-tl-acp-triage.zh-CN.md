# 实施计划 v2: TL ACP 决策替代自动重试

> **前置**: [plan-v1](plan-v1-merging-conflict-triage.zh-CN.md) 已实现自动重试（3 Wave 全部完成）
> **设计依据**: [01-PR/Merge 流程](01-pr-merge-flow.zh-CN.md) + [02-Escalation/Directive 模式](02-escalation-directive-pattern.zh-CN.md)
> **升级路径**: v1(自动重试) → **v2(TL ACP 决策)** → v3(EscalationRouter 泛化) → v4(A2A 表达)

## Context

v1 的 TLTriageHandler 在合并冲突时盲目重试：递增 `MergeRetries`，用固定提示"请先 rebase 解决冲突"重新入队。coder 不知道冲突的具体原因，可能反复尝试同样的策略，浪费执行时间。

v2 的核心变化：**TLTriageHandler 启动一个 TL ACP session，TL 分析冲突上下文后做出有针对性的决策**。

## 设计原则

1. **事件流不变** — EventIssueMergeConflict → TLTriageHandler → EventIssueMergeRetry/EventIssueFailed，scheduler 逻辑零改动
2. **单轮会话** — TL session 发一次 prompt，得一次响应，不做多轮对话
3. **优雅降级** — ACP session 启动失败或超时，回退到 v1 的自动重试行为
4. **只读分析** — TL 只分析不修改代码，capabilities: fs_read + terminal（git diff），无 fs_write
5. **重试上限保留** — v1 的 maxRetries=3 仍然是硬限制，TL 不能绕过

## v1 → v2 变化对比

| 维度 | v1 | v2 |
|------|----|----|
| 决策方式 | `if retries < max → retry` | TL ACP session 分析后决策 |
| coder 提示 | 固定: "请先 rebase 解决冲突" | TL 分析后的具体指示 |
| 冲突上下文 | 无 | merge 错误信息 + worktree 路径（TL 可 git diff） |
| 升级路径 | retries >= max → failed | TL 可主动决定 escalate（不等到用完重试次数） |
| 事件流 | 不变 | 不变 |
| scheduler | 不变 | 不变 |

## Wave 1: 数据层 + Prompt 模板

### 1.1 Issue 新增字段

**`internal/core/issue.go`**
- Issue struct 新增: `TriageInstructions string` (json:"triage_instructions")

**`internal/plugins/store-sqlite/migrations.go`**
- `schemaVersion` 升级（当前版本+1）
- Migration: `ALTER TABLE issues ADD COLUMN triage_instructions TEXT NOT NULL DEFAULT ''`

**`internal/plugins/store-sqlite/store.go`**
- Issue 的 scan/save SQL 加 `triage_instructions`

### 1.2 buildRunFromIssue 读取 TriageInstructions

**`internal/teamleader/scheduler.go`**

`buildRunFromIssue` 中的 merge_conflict_hint 逻辑改为:
```go
if issue.MergeRetries > 0 {
    hint := "上一次实现与主干产生合并冲突，请先 rebase 解决冲突后再实现需求。"
    if instructions := strings.TrimSpace(issue.TriageInstructions); instructions != "" {
        hint = instructions
    }
    config["merge_conflict_hint"] = hint
}
```

### 1.3 TL Triage Prompt 模板

**新建 `internal/engine/prompt_templates/tl_triage.tmpl`**

```
你是 Team Leader，负责分析合并冲突并做出决策。

## Issue 信息
- Issue ID: {{.IssueID}}
- Title: {{.IssueTitle}}
- Requirements: {{.Requirements}}

## 冲突信息
- 重试次数: {{.MergeRetries}}/{{.MaxRetries}}
- 错误信息: {{.MergeError}}
- Worktree: {{.WorktreePath}}

## 你的任务

1. 分析冲突原因（可使用 git diff main...HEAD、git status 等命令查看 worktree）
2. 如果需要更多上下文，使用 MCP 工具查询 issue 和 run 详情
3. 做出决策

## 输出格式（严格遵守）

DECISION: RETRY 或 ESCALATE
INSTRUCTIONS: <给 coder 的具体指示，包括需要修改的文件和策略>
REASON: <决策原因>

如果冲突可以通过 rebase + 局部修改解决，选择 RETRY 并给出具体指示。
如果冲突涉及架构性矛盾或需要人类判断，选择 ESCALATE。
```

### 1.4 Triage Prompt 渲染

**`internal/teamleader/tl_triage_handler.go`** 新增:

```go
type TriagePromptVars struct {
    IssueID       string
    IssueTitle    string
    Requirements  string
    MergeRetries  int
    MaxRetries    int
    MergeError    string
    WorktreePath  string
}
```

渲染使用 `text/template`，模板嵌入 `//go:embed` 或直接从 `prompt_templates/tl_triage.tmpl` 读取（复用 engine 的 `RenderPrompt` 或 独立渲染）。

### 验收
```bash
go build ./...
go test -count=1 ./internal/core/...
go test -count=1 ./internal/plugins/store-sqlite/...
```

## Wave 2: TLTriageHandler v2 核心逻辑

### 2.1 新增依赖

**`internal/teamleader/tl_triage_handler.go`**

```go
type TLTriageHandler struct {
    store             core.Store
    bus               eventPublisher
    maxRetries        int
    log               *slog.Logger
    // v2 新增
    roleResolver      *acpclient.RoleResolver
    acpHandlerFactory ACPHandlerFactory  // 复用 engine 定义的接口
    mcpEnv            MCPEnvConfig
    triageRoleID      string  // 默认 "team_leader"
    triageTimeout     time.Duration  // 默认 2 分钟
}
```

`NewTLTriageHandler` 签名扩展（向后兼容）:
```go
type TLTriageOption func(*TLTriageHandler)

func WithTriageACP(
    resolver *acpclient.RoleResolver,
    factory ACPHandlerFactory,
    mcpEnv MCPEnvConfig,
) TLTriageOption

func WithTriageRoleID(roleID string) TLTriageOption
func WithTriageTimeout(d time.Duration) TLTriageOption
```

当 `roleResolver == nil` 时，保持 v1 行为（自动重试）。

### 2.2 OnEvent 重构

```go
func (h *TLTriageHandler) OnEvent(ctx context.Context, evt core.Event) {
    // ... 前置检查不变 ...

    if h.roleResolver != nil {
        h.handleWithACP(ctx, issue, evt)
    } else {
        h.handleAutoRetry(issue, evt)  // v1 逻辑提取为方法
    }
}
```

### 2.3 ACP Triage 核心流程

```go
func (h *TLTriageHandler) handleWithACP(ctx context.Context, issue *core.Issue, evt core.Event) {
    // 1. 获取冲突上下文
    run, _ := h.store.GetRun(strings.TrimSpace(issue.RunID))
    worktreePath := ""
    if run != nil {
        worktreePath = run.WorktreePath
    }

    // 2. 渲染 TL prompt
    prompt := renderTriagePrompt(TriagePromptVars{
        IssueID:      issue.ID,
        IssueTitle:   issue.Title,
        Requirements: issue.Body,
        MergeRetries: issue.MergeRetries,
        MaxRetries:   h.maxRetries,
        MergeError:   evt.Error,
        WorktreePath: worktreePath,
    })

    // 3. 启动 TL ACP session（带 timeout）
    triageCtx, cancel := context.WithTimeout(ctx, h.triageTimeout)
    defer cancel()

    response, err := h.runTriageSession(triageCtx, worktreePath, prompt)
    if err != nil {
        h.log.Warn("tl_triage: ACP session failed, falling back to auto-retry",
            "issue_id", issue.ID, "error", err)
        h.handleAutoRetry(issue, evt)  // 优雅降级
        return
    }

    // 4. 解析决策
    decision := parseTriageDecision(response)

    // 5. 执行决策
    switch decision.Action {
    case "RETRY":
        if issue.MergeRetries+1 >= h.maxRetries {
            // 硬限制：即使 TL 说 retry，超出上限仍 fail
            h.markFailed(issue, evt, "merge conflict retries exhausted (TL wanted retry)")
            return
        }
        issue.TriageInstructions = decision.Instructions
        h.retryIssue(issue, evt)
    case "ESCALATE":
        h.markFailed(issue, evt, "TL escalated: "+decision.Reason)
    default:
        // 无法解析 → 降级
        h.handleAutoRetry(issue, evt)
    }
}
```

### 2.4 ACP Session 启动

```go
func (h *TLTriageHandler) runTriageSession(ctx context.Context, worktreePath, prompt string) (string, error) {
    agent, role, err := h.roleResolver.Resolve(h.triageRoleID)
    if err != nil {
        return "", fmt.Errorf("resolve triage role: %w", err)
    }

    cwd := worktreePath
    if cwd == "" {
        return "", fmt.Errorf("worktree path is empty")
    }

    launchCfg := acpclient.LaunchConfig{
        Command: agent.LaunchCommand,
        Args:    append([]string(nil), agent.LaunchArgs...),
        WorkDir: cwd,
        Env:     cloneEnvMap(agent.Env),
    }

    handler := h.acpHandlerFactory.NewHandler(cwd, h.bus)
    h.acpHandlerFactory.SetPermissionPolicy(handler, role.PermissionPolicy)

    client, err := acpclient.New(launchCfg, handler)
    if err != nil {
        return "", fmt.Errorf("create acp client: %w", err)
    }
    defer func() {
        closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
        _ = client.Close(closeCtx)
        cancel()
    }()

    if err := client.Initialize(ctx, role.Capabilities); err != nil {
        return "", fmt.Errorf("initialize: %w", err)
    }

    mcpServers := MCPToolsFromRoleConfig(role, h.mcpEnv)
    session, err := client.NewSession(ctx, acpproto.NewSessionRequest{
        Cwd:        cwd,
        McpServers: mcpServers,
    })
    if err != nil {
        return "", fmt.Errorf("new session: %w", err)
    }

    result, err := client.Prompt(ctx, acpproto.PromptRequest{
        SessionId: session,
        Prompt:    []acpproto.ContentBlock{{Text: &acpproto.ContentBlockText{Text: prompt}}},
    })
    if err != nil {
        return "", fmt.Errorf("prompt: %w", err)
    }
    if result == nil {
        return "", nil
    }
    return strings.TrimSpace(result.Text), nil
}
```

### 2.5 响应解析

```go
type TriageDecision struct {
    Action       string // "RETRY" | "ESCALATE"
    Instructions string
    Reason       string
}

func parseTriageDecision(response string) TriageDecision {
    // 解析 DECISION: / INSTRUCTIONS: / REASON: 格式
    // 容错：如果格式不对，返回空 Action（触发降级）
}
```

### 验收
```bash
go test -count=1 -v ./internal/teamleader/... -run TestTLTriage
```

## Wave 3: 集成 + 降级 + 测试

### 3.1 commands.go 注入

**`cmd/ai-flow/commands.go`**

```go
// 原: tlTriageHandler := teamleader.NewTLTriageHandler(store, bus, 3)
// 改:
tlTriageHandler := teamleader.NewTLTriageHandler(store, bus, 3,
    teamleader.WithTriageACP(roleResolver, acpHandlerFactory, mcpEnv),
)
```

### 3.2 角色配置建议

**config.yaml 参考配置**（用户手动添加）:

```yaml
agents:
  - id: claude-agent
    launch_command: claude
    launch_args: ["--dangerously-skip-permissions"]
    capabilities_max:
      fs_read: true
      fs_write: true
      terminal: true

roles:
  team_leader:
    agent_id: claude-agent
    capabilities:
      fs_read: true
      terminal: true    # git diff, git log
    permission_policy:
      - pattern: "*"
        scope: cwd
        action: allow_once
    mcp_tools:
      - ai-workflow-query
    session_policy:
      max_turns: 1
      reuse: false
```

### 3.3 降级策略

| 场景 | 行为 |
|------|------|
| `roleResolver == nil` | v1 自动重试 |
| ACP 启动失败 | warn 日志 + v1 自动重试 |
| ACP 超时（2min） | warn 日志 + v1 自动重试 |
| TL 响应格式无法解析 | warn 日志 + v1 自动重试 |
| TL 决定 RETRY 但已达上限 | 硬限制 → failed |
| worktree 已不存在 | v1 自动重试 |

### 3.4 测试清单

**`internal/teamleader/tl_triage_handler_test.go`** 扩展:

| 测试 | 场景 |
|------|------|
| `TestTLTriage_ACPRetryWithInstructions` | TL 返回 RETRY + 指示 → issue.TriageInstructions 被设置 |
| `TestTLTriage_ACPEscalate` | TL 返回 ESCALATE → issue failed |
| `TestTLTriage_ACPFallbackOnError` | ACP 启动失败 → 降级到 v1 重试 |
| `TestTLTriage_ACPFallbackOnTimeout` | ACP 超时 → 降级到 v1 重试 |
| `TestTLTriage_ACPFallbackOnParseError` | TL 返回垃圾 → 降级到 v1 重试 |
| `TestTLTriage_ACPRetryExceedsMax` | TL 说 RETRY 但已达上限 → failed |
| `TestTLTriage_NoResolverFallsBackToV1` | roleResolver=nil → v1 行为 |
| `TestTLTriage_InstructionsPropagateToRun` | issue.TriageInstructions → Run.Config["merge_conflict_hint"] |
| `TestParseTriageDecision_*` | 各种响应格式的解析测试 |

### 验收
```bash
go build ./...
go test -count=1 ./internal/core/...
go test -count=1 ./internal/plugins/store-sqlite/...
go test -count=1 -v ./internal/teamleader/... -run TestTLTriage
go test -count=1 -short ./...
```

## 完整事件流（v2）

```
RunDone
  ↓
DepScheduler (不变)
  └─ AutoMerge=true → merging + EventIssueMerging
                          ↓
                    MergeHandler (不变)
                      └─ 冲突 → EventIssueMergeConflict
                                  ↓
                        TLTriageHandler v2
                          ├─ roleResolver 可用?
                          │   ├─ 是 → 启动 TL ACP session
                          │   │        TL 分析 worktree (git diff)
                          │   │        TL 查询 MCP (issue/run 详情)
                          │   │        TL 返回 DECISION
                          │   │   ├─ RETRY + instructions
                          │   │   │   → issue.TriageInstructions = TL 指示
                          │   │   │   → queued + EventIssueMergeRetry
                          │   │   │   → 新 Run 带 TL 具体指示
                          │   │   └─ ESCALATE
                          │   │       → failed + EventIssueFailed
                          │   └─ ACP 失败 → 降级到 v1
                          └─ 否 → v1 自动重试（向后兼容）
```

## 文件清单

| 文件 | 操作 | Wave | 说明 |
|------|------|------|------|
| `internal/core/issue.go` | 改 | 1 | +TriageInstructions 字段 |
| `internal/plugins/store-sqlite/migrations.go` | 改 | 1 | migration: triage_instructions 列 |
| `internal/plugins/store-sqlite/store.go` | 改 | 1 | Issue CRUD 加 triage_instructions |
| `internal/teamleader/scheduler.go` | 改 | 1 | buildRunFromIssue 读取 TriageInstructions |
| `internal/engine/prompt_templates/tl_triage.tmpl` | **新建** | 1 | TL triage prompt 模板 |
| `internal/teamleader/tl_triage_handler.go` | 改 | 2 | v2 核心：ACP session + 决策解析 + 降级 |
| `internal/teamleader/tl_triage_handler_test.go` | 改 | 3 | 扩展测试覆盖 |
| `cmd/ai-flow/commands.go` | 改 | 3 | 注入 ACP 依赖 |

## 风险点

1. **事件循环阻塞**: TL ACP session 最多阻塞 2 分钟。AutoMergeHandler 已有 10 分钟阻塞先例，可接受。未来如需并发处理，再改为异步。
2. **TL 响应格式不稳定**: LLM 输出可能不符合预期格式。缓解: 解析容错 + 降级到 v1。
3. **Worktree 状态**: merge 失败后 worktree 可能处于 dirty 状态。TL 读取 git diff 时可能看到中间状态。缓解: TL prompt 中说明 worktree 当前状态。
4. **MCP 服务可用性**: TL session 依赖 MCP 服务器。如果 SSE 端点不可达，stdio fallback 会启动子进程。缓解: MCPToolsFromRoleConfig 已有 fallback 逻辑。

## 升级路径

```
v2 (本计划): TLTriageHandler 启动 TL ACP session 做决策
  ↓ 泛化触发条件
v3: TLTriageHandler → EscalationRouter
    - 监听所有 escalation 类型（不只是 merge conflict）
    - chain 配置路由（coder → TL → human）
    - 复用同一个 ACP session launcher
  ↓ 映射协议
v4: Escalation = A2A INPUT_REQUIRED，Directive = A2A SendMessage
```

v2 → v3 的改动集中在:
- `OnEvent` 从只监听 EventIssueMergeConflict 扩展为监听多种事件
- 增加 chain 路由配置
- TL session 变为 EscalationRouter 的一个实例

v2 的 ACP session 启动逻辑（`runTriageSession`）在 v3 中可直接复用。

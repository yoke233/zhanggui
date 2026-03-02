# P4 Wave 4 — MCP 扩展入口

> **For Agent:** REQUIRED SUB-SKILL: Use executing-wave-plans to implement this wave and gate it before Wave 5.

## Wave Goal

在不重写 Agent 驱动的前提下，交付最小可用 MCP 扩展能力：`spec-mcp` provider + Agent 执行策略配置化 + CLI 校验链路。

## Depends On

- `[W3-T1, W3-T2, W3-T3]`

## Wave Entry Data

- `SpecPlugin` 已支持 `noop`，但无 MCP 来源 provider。
- `CodexAgent`/`ClaudeAgent` 对配置项（tools/sandbox/approval）支持不对齐。
- 缺少 MCP 配置合法性检查命令。

## Tasks

### Task W4-T1: `spec-mcp` provider 插件与配置模型

**Files:**
- Modify: `internal/config/types.go`
- Modify: `internal/config/defaults.go`
- Modify: `internal/config/merge.go`
- Modify: `internal/config/config_test.go`
- Create: `internal/plugins/spec-mcp/spec.go`
- Create: `internal/plugins/spec-mcp/spec_test.go`
- Create: `internal/plugins/spec-mcp/client.go`
- Create: `internal/plugins/spec-mcp/module.go`
- Modify: `internal/plugins/factory/factory.go`
- Modify: `internal/plugins/factory/factory_test.go`

**Depends on:** `[]`

**Conflict Scope (for executor scheduling):**
- Files: `[internal/config/types.go, internal/plugins/factory/factory.go]`
- Shared Critical File: `yes`

**Step 1: Write failing test**
```text
新增测试：
- TestSpecMCP_GetContext_Success
- TestSpecMCP_GetContext_TimeoutFallback
- TestBuildWithRegistry_SpecProviderMCP_LoadsWhenEnabled
- TestBuildWithRegistry_SpecProviderMCP_MissingConfig_Fails
```

**Step 2: Run to confirm failure**
Run: `go test ./internal/plugins/spec-mcp ./internal/plugins/factory ./internal/config -run 'SpecMCP|SpecProviderMCP'`
Expected: provider 未注册或配置字段缺失。

**Step 3: Minimal implementation**
```text
新增 spec.mcp 配置段：endpoint/api_key/timeout/context_limit。
实现 spec-mcp：通过 HTTP 请求 MCP gateway 获取 summary/references。
接入 factory 的 spec provider 分支并复用 on_failure 策略。
```

**Step 4: Run tests to confirm pass**
Run: `go test ./internal/plugins/spec-mcp ./internal/plugins/factory ./internal/config -run 'SpecMCP|SpecProviderMCP'`
Expected: PASS。

**Step 5: Commit**
```bash
git add internal/config/types.go internal/config/defaults.go internal/config/merge.go internal/config/config_test.go internal/plugins/spec-mcp/spec.go internal/plugins/spec-mcp/spec_test.go internal/plugins/spec-mcp/client.go internal/plugins/spec-mcp/module.go internal/plugins/factory/factory.go internal/plugins/factory/factory_test.go
git commit -m "feat(spec): add spec-mcp provider and config wiring"
```

### Task W4-T2: Agent 执行策略配置化（tools/sandbox/approval）

**Files:**
- Modify: `internal/plugins/agent-codex/codex.go`
- Modify: `internal/plugins/agent-codex/codex_test.go`
- Modify: `internal/plugins/agent-claude/claude.go`
- Modify: `internal/plugins/agent-claude/claude_test.go`
- Modify: `internal/plugins/factory/factory.go`
- Modify: `internal/core/agent.go`
- Modify: `internal/engine/executor.go`
- Test: `internal/engine/executor_behavior_test.go`

**Depends on:** `[W4-T1]`

**Conflict Scope (for executor scheduling):**
- Files: `[internal/plugins/factory/factory.go, internal/engine/executor.go, internal/core/agent.go]`
- Shared Critical File: `yes`

**Step 1: Write failing test**
```text
新增测试：
- TestCodexBuildCommand_UsesConfiguredSandboxApproval
- TestCodexBuildCommand_IncludesAllowedToolsWhenProvided
- TestExecutor_BuildExecOpts_UsesAgentDefaultToolsAndTurns
- TestClaudeBuildCommand_AllowedToolsPreserved
```

**Step 2: Run to confirm failure**
Run: `go test ./internal/plugins/agent-codex ./internal/plugins/agent-claude ./internal/engine -run 'BuildCommand|ExecOpts|AllowedTools|Sandbox'`
Expected: Codex 命令参数与配置脱节。

**Step 3: Minimal implementation**
```text
扩展 agent 插件构造参数：读取 config 中 model/reasoning/sandbox/approval/default_tools/max_turns。
executor 构建 ExecOpts 时使用每个 agent 的默认策略，而非硬编码 MaxTurns=30。
保持未配置时回退当前默认行为。
```

**Step 4: Run tests to confirm pass**
Run: `go test ./internal/plugins/agent-codex ./internal/plugins/agent-claude ./internal/engine -run 'BuildCommand|ExecOpts|AllowedTools|Sandbox'`
Expected: PASS。

**Step 5: Commit**
```bash
git add internal/plugins/agent-codex/codex.go internal/plugins/agent-codex/codex_test.go internal/plugins/agent-claude/claude.go internal/plugins/agent-claude/claude_test.go internal/plugins/factory/factory.go internal/core/agent.go internal/engine/executor.go internal/engine/executor_behavior_test.go
git commit -m "feat(agent): make execution policy config-driven"
```

### Task W4-T3: MCP 诊断与校验命令

**Files:**
- Create: `cmd/ai-flow/commands_mcp_validate.go`
- Create: `cmd/ai-flow/commands_mcp_validate_test.go`
- Modify: `cmd/ai-flow/commands.go`
- Modify: `cmd/ai-flow/main.go`
- Modify: `cmd/ai-flow/commands_test.go`

**Depends on:** `[W4-T1]`

**Conflict Scope (for executor scheduling):**
- Files: `[cmd/ai-flow/main.go, cmd/ai-flow/commands.go]`
- Shared Critical File: `yes`

**Step 1: Write failing test**
```text
新增测试：
- TestCommand_MCPValidate_MissingEndpoint_Fails
- TestCommand_MCPValidate_HealthCheckOK_Passes
- TestCommand_MCPValidate_Timeout_ReturnsNonZero
```

**Step 2: Run to confirm failure**
Run: `go test ./cmd/ai-flow -run 'MCPValidate|mcp validate'`
Expected: 子命令不存在或参数检查未实现。

**Step 3: Minimal implementation**
```text
新增 `ai-flow mcp validate`：
- 校验 spec.provider=mcp 时必要字段。
- 执行一次 MCP endpoint 健康探测并输出诊断信息。
失败时返回非零退出码。
```

**Step 4: Run tests to confirm pass**
Run: `go test ./cmd/ai-flow -run 'MCPValidate|mcp validate'`
Expected: PASS。

**Step 5: Commit**
```bash
git add cmd/ai-flow/commands_mcp_validate.go cmd/ai-flow/commands_mcp_validate_test.go cmd/ai-flow/commands.go cmd/ai-flow/main.go cmd/ai-flow/commands_test.go
git commit -m "feat(cli): add mcp validate command"
```

## Test Strategy Per Task

| Task | Unit | Integration |
|---|---|---|
| W4-T1 | spec-mcp 请求/超时/回退 | factory provider 装配与 on_failure 策略 |
| W4-T2 | agent 命令参数生成、默认值回退 | executor 构建 ExecOpts 链路 |
| W4-T3 | CLI 参数与返回码 | 假 MCP 服务健康检查 smoke |

## Risks and Mitigations

- 风险：MCP 上下游协议差异导致实现耦合。  
  缓解：P4 仅实现 HTTP gateway 适配层，协议细节下沉到 `client.go`。
- 风险：Agent 参数配置化后影响现有稳定命令行参数。  
  缓解：维持默认值，新增仅在显式配置时生效。

## Wave E2E/Smoke Cases and Entry Data

### Entry Data
- 一份 `spec.provider=mcp` 的全局配置。
- 一个可控 MCP 假服务（success/timeout/error 三种模式）。

### Smoke Cases
- Secretary 请求 Spec 上下文时可拿到 MCP 返回摘要。
- MCP 服务超时时遵循 `spec.on_failure` 策略（warn/fail）。
- `ai-flow mcp validate` 对错误配置与错误连通性返回非零。

## Wave Exit Gate
- Execute mandatory gate sequence via `executing-wave-plans`.
- Wave-specific acceptance:
  - [ ] `spec-mcp` provider 可配置并可回退。
  - [ ] Agent 执行策略从硬编码迁移为配置驱动。
- Wave-specific verification:
  - [ ] `go test ./internal/plugins/spec-mcp ./internal/plugins/agent-codex ./internal/plugins/agent-claude ./internal/plugins/factory ./internal/engine -run 'SpecMCP|BuildCommand|ExecOpts|Policy'` 通过。
  - [ ] `go test ./cmd/ai-flow -run 'MCPValidate|mcp validate'` 通过。
- Boundary-change verification (if triggered):
  - [ ] 若修改了 spec provider 装配，执行 `go test ./internal/plugins/factory -run 'Spec|Bootstrap|Registry'` 并确认 PASS。

## Next Wave Entry Condition
- Governed by `executing-wave-plans` verdict rule (`Go` / satisfied `Conditional Go` only).

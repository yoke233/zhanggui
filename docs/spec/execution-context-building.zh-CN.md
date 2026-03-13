# 执行上下文构建流程

> 状态：现行
>
> 最后按代码核对：2026-03-13
>
> 当前实现状态：本文描述的是当前已落地的 Step briefing 组装主链。对外产品语义虽然逐步统一为 `Work Item`，但内部实现仍以 `IssueEngine`、`Issue`、`Step` 为主；后端 REST 现状仍是 `/issues/*`。

## 概述

Step 执行时，上下文通过 `IssueEngine` 的三阶段管道（Prepare → Execute → Finalize）逐层组装，最终以 Markdown 形式发送给 Agent。

## 流程图

```
┌─────────────────────────────────────────────────────────────┐
│                        Issue 层                              │
│  Title, Body, Priority, Labels, ProjectID, ResourceBinding   │
└──────────────────────────┬──────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────┐
│                        Step 层                               │
│  Name, Description, Config["objective"],                     │
│  AcceptanceCriteria, AgentRole, RequiredCapabilities          │
└──────────────────────────┬──────────────────────────────────┘
                           │
          IssueEngine.Run() │  准备 Workspace (git worktree)
                           │  ctx = ContextWithWorkspace(ctx, ws)
                           ▼
┌═════════════════════════════════════════════════════════════┐
║              Phase 1: PREPARE                               ║
║                                                             ║
║  ┌─────────────────────────────────────────────────────┐   ║
║  │ AgentResolver.Resolve(step)                          │   ║
║  │   step.AgentRole + RequiredCapabilities → AgentID    │   ║
║  └──────────────────────┬──────────────────────────────┘   ║
║                         │                                   ║
║  ┌──────────────────────▼──────────────────────────────┐   ║
║  │ BriefingBuilder.Build(step)                          │   ║
║  │                                                      │   ║
║  │  ① Objective ← step.Config["objective"] | step.Name  │   ║
║  │                                                      │   ║
║  │  ② Constraints ← step.AcceptanceCriteria             │   ║
║  │                                                      │   ║
║  │  ③ ContextRefs (按优先级排列):                        │   ║
║  │     ┌──────────────────────────────────────────┐     │   ║
║  │     │ CtxIssueSummary                          │     │   ║
║  │     │  ← Issue Title + Body (≤500 字符)        │     │   ║
║  │     ├──────────────────────────────────────────┤     │   ║
║  │     │ CtxUpstreamArtifact (L2 直接前置)        │     │   ║
║  │     │  ← 直接前置 Step 的完整 ResultMarkdown   │     │   ║
║  │     ├──────────────────────────────────────────┤     │   ║
║  │     │ CtxUpstreamArtifact (L0 远处前置)        │     │   ║
║  │     │  ← Metadata["summary"] 或前 300 字符     │     │   ║
║  │     ├──────────────────────────────────────────┤     │   ║
║  │     │ CtxFeatureManifest                       │     │   ║
║  │     │  ← Project 的功能清单 (fail/pending 详细, │     │   ║
║  │     │    pass/skipped 仅 key+status)           │     │   ║
║  │     ├──────────────────────────────────────────┤     │   ║
║  │     │ CtxProjectBrief   (预留)                 │     │   ║
║  │     │ CtxAgentMemory    (预留)                 │     │   ║
║  │     └──────────────────────────────────────────┘     │   ║
║  └──────────────────────┬──────────────────────────────┘   ║
║                         │                                   ║
║  ┌──────────────────────▼──────────────────────────────┐   ║
║  │ renderBriefingSnapshot(briefing)                     │   ║
║  │                                                      │   ║
║  │  ┌────────────────────────────────────────────┐      │   ║
║  │  │ # Task                                     │      │   ║
║  │  │ {Objective}                                │      │   ║
║  │  │                                            │      │   ║
║  │  │ # Context                                  │      │   ║
║  │  │ ## work item                               │      │   ║
║  │  │ **{Issue.Title}** + {Issue.Body}           │      │   ║
║  │  │ ## upstream step N output (L2)             │      │   ║
║  │  │ {Artifact.ResultMarkdown}                  │      │   ║
║  │  │ ## upstream step M summary (L0)            │      │   ║
║  │  │ {Metadata["summary"] 或前300字符}          │      │   ║
║  │  │ ## feature manifest                        │      │   ║
║  │  │ {compact JSON}                             │      │   ║
║  │  │                                            │      │   ║
║  │  │ # Acceptance Criteria                      │      │   ║
║  │  │ - criterion 1                              │      │   ║
║  │  │ - criterion 2                              │      │   ║
║  │  └────────────────────────────────────────────┘      │   ║
║  │  限制: 整体 ≤12000 字符, 按类型分配预算:              │   ║
║  │    IssueSummary ≤800, Manifest ≤2000,                │   ║
║  │    UpstreamArtifact ≤4000                            │   ║
║  └──────────────────────┬──────────────────────────────┘   ║
║                         │                                   ║
║                         ▼  BriefingSnapshot (Markdown)      ║
║              存入 Execution.BriefingSnapshot                ║
╚═══════════════════════════╤═════════════════════════════════╝
                            │
╔═══════════════════════════▼═════════════════════════════════╗
║              Phase 2: EXECUTE                               ║
║                                                             ║
║  ┌─────────────────────────────────────────────────────┐   ║
║  │ ACPExecutor                                          │   ║
║  │                                                      │   ║
║  │  ① WorkspaceFromContext(ctx) → workDir, env          │   ║
║  │                                                      │   ║
║  │  ② SessionManager.Acquire()                          │   ║
║  │     profile + driver + MCP tools + workDir           │   ║
║  │     → ACP Session (新建 or 复用)                      │   ║
║  │                                                      │   ║
║  │  ③ BuildExecutionInputForStep()                      │   ║
║  │     ┌───────────────────────────────────────────┐    │   ║
║  │     │ Gate 步骤?  → 总是完整 prompt              │    │   ║
║  │     │                                           │    │   ║
║  │     │ 复用会话 + 有前置回合?                      │    │   ║
║  │     │   有 Gate 反馈? → Rework 跟进消息          │    │   ║
║  │     │   无反馈?       → Continue 跟进消息        │    │   ║
║  │     │                                           │    │   ║
║  │     │ 新会话?                                    │    │   ║
║  │     │   有 Gate 反馈? → 完整 prompt + 反馈章节   │    │   ║
║  │     │   无反馈?       → 完整 prompt              │    │   ║
║  │     └───────────────────────────────────────────┘    │   ║
║  │                                                      │   ║
║  │  ④ Token 预算检查 (复用会话时)                        │   ║
║  │     OK      → 继续                                   │   ║
║  │     Warning → slog 告警, 继续                        │   ║
║  │     Exceeded → 返回 ErrTokenBudgetExceeded           │   ║
║  │                                                      │   ║
║  │  ⑤ SessionManager.StartExecution(input)              │   ║
║  │     → Agent 接收最终 prompt                           │   ║
║  │                                                      │   ║
║  │  ⑥ WatchExecution() → result.Text                    │   ║
║  │     → NoteTokens(input, output) 记录累积用量         │   ║
║  └──────────────────────┬──────────────────────────────┘   ║
╚═══════════════════════════╤═════════════════════════════════╝
                            │
╔═══════════════════════════▼═════════════════════════════════╗
║              Phase 3: FINALIZE                              ║
║                                                             ║
║  ┌─────────────────────────────────────────────────────┐   ║
║  │ Artifact 存储                                        │   ║
║  │   ResultMarkdown ← Agent 输出                        │   ║
║  │   Metadata ← Collector(LLM) 提取结构化数据           │   ║
║  └──────────────────────┬──────────────────────────────┘   ║
║                         │                                   ║
║  ┌──────────────────────▼──────────────────────────────┐   ║
║  │ Gate 处理 (仅 gate 步骤)                              │   ║
║  │                                                      │   ║
║  │   pass → 步骤完成, 推进下一步                         │   ║
║  │                                                      │   ║
║  │   reject → recordGateRework():                       │   ║
║  │     upstream.Config["last_gate_feedback"] = reason    │   ║
║  │     upstream.Config["rework_history"] += entry        │   ║
║  │     upstream.Status = pending (等待重新执行)           │   ║
║  │              │                                        │   ║
║  │              └──── 反馈回流 ──→ 下次 Prepare 时注入   │   ║
║  └─────────────────────────────────────────────────────┘   ║
╚═════════════════════════════════════════════════════════════╝
```

## 关键要点

### 1. Briefing ≠ Prompt

Briefing 是结构化对象（Objective + ContextRefs + Constraints），经 `renderBriefingSnapshot()` 序列化为 Markdown 后才成为 prompt。

### 2. 上下文来源

| 来源 | 状态 | 说明 |
|------|------|------|
| Step 自身配置 | ✅ 已接入 | `Config["objective"]`, `AcceptanceCriteria` |
| Issue 摘要 | ✅ 已接入 | `CtxIssueSummary` — Title + Body (≤500 字符) |
| 上游 Artifact (L2) | ✅ 已接入 | 直接前置 Step 的完整 `ResultMarkdown` |
| 上游 Artifact (L0) | ✅ 已接入 | 远处前置 Step 的 `Metadata["summary"]` 或前 300 字符 |
| 项目功能清单 | ✅ 已接入 | `FeatureManifest` (fail/pending 详细, pass/skipped 精简) |
| Gate 反馈 | ✅ 已接入 | `last_gate_feedback` / `rework_history` |
| 项目简报 | 🔲 预留 | `CtxProjectBrief` |
| Agent 记忆 | 🔲 预留 | `CtxAgentMemory` |

### 3. 会话复用时的 Prompt 变体

- **首次执行**: 完整 BriefingSnapshot
- **复用会话 + 无反馈**: Continue 跟进消息（避免重复注入）
- **复用会话 + Gate 拒绝**: Rework 跟进消息（仅包含反馈）
- **新会话 + Gate 拒绝**: 完整 prompt + `# Gate Feedback (Rework)` 章节
- **Gate 步骤**: 总是完整 prompt（确保输出确定性）

### 4. Gate 反馈闭环

```
Gate reject
  → 写入 upstream Step.Config["last_gate_feedback"]
    → 下次 Prepare 时 BriefingBuilder 自动拼入
      → BuildExecutionInputForStep 检测到反馈
        → Agent 看到反馈并修正
```

### 5. Token 限制策略

**Briefing 字符预算** (renderBriefingSnapshot):
- 整个 BriefingSnapshot: ≤ 12000 字符
- 按 ContextRef 类型分配预算:
  - `CtxIssueSummary` / `CtxProjectBrief`: ≤ 800 字符
  - `CtxAgentMemory`: ≤ 1500 字符
  - `CtxFeatureManifest`: ≤ 2000 字符
  - `CtxUpstreamArtifact`: ≤ 4000 字符
- 超限自动截断，末尾添加 `[truncated]`

**Session Token 预算** (ACPSessionPool):
- 配置: `ProfileSession.MaxContextTokens` + `ContextWarnRatio` (默认 0.8)
- 累积 input + output tokens 在 `pooledACPSession` 中追踪
- 执行前检查三级状态:
  - OK: 正常执行
  - Warning (≥80%): slog 告警, 继续执行
  - Exceeded (≥100%): 返回 `ErrTokenBudgetExceeded`, 步骤进入 blocked/retry

## 相关代码文件

| 文件 | 职责 |
|------|------|
| `internal/core/briefing.go` | Briefing / ContextRef 领域模型 |
| `internal/core/execution.go` | Execution 领域模型 (含 BriefingSnapshot) |
| `internal/core/step.go` | Step 领域模型 (含 Config, AcceptanceCriteria) |
| `internal/core/errors.go` | 领域错误 (含 ErrTokenBudgetExceeded) |
| `internal/application/flow/briefing_builder.go` | BriefingBuilder — 上下文组装核心 (Issue 注入 + 分层 Artifact + Manifest) |
| `internal/application/flow/briefing_builder_test.go` | BriefingBuilder 单元测试 (11 cases) |
| `internal/application/flow/pipeline.go` | renderBriefingSnapshot + refBudget 按类型分配字符预算 |
| `internal/application/flow/engine.go` | IssueEngine — 三阶段管道 (prepare/execute/finalize) |
| `internal/application/flow/execution_input.go` | BuildExecutionInputForStep — prompt 变体选择 |
| `internal/application/flow/gate.go` | Gate 处理 + recordGateRework 反馈回流 |
| `internal/application/flow/dag.go` | 前置步骤查询 (predecessorStepIDs / immediatePredecessorStepIDs) |
| `internal/application/flow/workspace.go` | Workspace context 注入 |
| `internal/adapters/executor/acp.go` | ACPExecutor — 实际发送给 Agent |
| `internal/runtime/agent/acp_session_pool.go` | ACPSessionPool — 会话复用 + Token 预算检查 (CheckTokenBudget / NoteTokens) |
| `internal/runtime/agent/session_manager_local.go` | 本地会话管理 — 执行前/后 token 预算 hook |
| `internal/runtime/agent/token_budget_test.go` | Token 预算单元测试 (8 cases) |
| `internal/platform/bootstrap/bootstrap_engine.go` | 启动时装配 BriefingBuilder / Collector 等 |

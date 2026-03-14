# complete_step MCP 工具设计（Step 完成 + Gate 完成）

> 状态：已被后续方案吸收/替代
>
> 最后按代码核对：2026-03-13
>
> 说明：当前系统主路径不是本文的 `complete_step` 单工具方案，而是 `StepSignal + step_complete / step_need_help / gate_approve / gate_reject` 组合；主工作对象对外路由也已切到 `/api/work-items/*`。

## 背景

本文保留为历史设计记录，用来解释为什么后来演进成 `StepSignal` 体系。

当前运行时在 `WatchExecution` 返回后，会直接把执行结果写入 artifact，并在流程引擎成功分支将 step 标记为 `done`（gate step 走额外的 gate finalize 逻辑）。

这意味着“完成语义”主要来自执行返回，而不是 agent 的显式协议。

## 当前落地情况

当前代码中的实现对应关系如下：

- 本文的 `complete_step(kind=exec, status=done)` 已被 `step_complete` 替代
- 本文的 `complete_step(status=blocked|failed)` 已拆分为 `step_need_help` 与现有错误分类链路
- 本文的 `complete_step(kind=gate)` 已被 `gate_approve` / `gate_reject` 替代
- gate 文本兜底 `AI_WORKFLOW_GATE_JSON` 仍然保留

因此，本文不应再被视为当前协议定义。

## 目标

引入一个明确的 MCP 工具 `complete_step`，把“我已经完成/阻塞/失败”的语义从自然语言中剥离出来，提升确定性与可审计性。

同时支持两类完成：

1. **exec step 完成**（普通执行型 step）
2. **gate step 完成**（评审/门禁 step）

## 方案概览

采用 **工具优先 + 文本兜底**：

- 主路径：agent 调用 `complete_step`（通过 MCP 注入）。
- 兜底路径：若未调用工具，保留当前逻辑（`WatchExecution` 返回 + artifact 输出），保证兼容现有 agent。

## 工具协议草案（历史方案）

工具名：`complete_step`

输入建议：

```json
{
  "kind": "exec|gate",
  "status": "done|blocked|failed",
  "summary": "string",
  "error_kind": "transient|permanent|need_help",
  "gate": {
    "verdict": "pass|reject",
    "reason": "string",
    "reject_targets": [1, 2]
  }
}
```

约束：

- `kind=exec` 时不要求 `gate`。
- `kind=gate` 时必须有 `gate.verdict`。
- `status=blocked|failed` 时建议携带 `error_kind`。

## 与现有状态机映射（历史方案）

- `status=done` -> `StepDone`
- `status=blocked` -> `StepBlocked`
- `status=failed` -> `StepFailed`
- `kind=gate` + `gate.verdict=reject` -> 走现有 gate reject 流程（复用 gate finalize）

## MCP 注入路径（仅在执行 step 时注入，设计思路仍有参考价值）

> 核心约束：`complete_step` 能力**不应作为常驻 MCP 能力长期绑定给 profile**，而应在每次 step execution 会话创建时按需注入，执行结束即失效。

### 1) 配置层声明 MCP server（不做全局常驻绑定）

使用现有 `runtime.mcp.servers` 声明 `complete_step` 所在 server。

`runtime.mcp.profile_bindings` 可保留给通用工具；`complete_step` 建议走执行期动态拼装，避免在非执行场景（如闲聊/探索）误暴露“结束 step”能力。

### 2) 运行时解析 MCP server

运行时可通过 `configruntime.Manager.ResolveMCPServers(profileID, agentSupportsSSE)` 输出 ACP 侧 `McpServer` 列表。

### 3) ACP 会话注入（按 execution 动态注入）

`SessionAcquireInput` 的 `MCPFactory` 在创建执行会话时动态返回 MCP server 列表，并在 `NewSession` 时下发到 ACP agent。

推荐策略：

- 仅当存在 `step_id/exec_id` 的真实执行上下文时注入 `complete_step` server。
- 对同一 profile 的非执行会话返回空列表（不注入该能力）。
- 执行结束（成功/失败/取消）后会话释放，能力随会话失效。

### 4) 执行器消费工具调用结果

执行器监听工具调用输出，识别 `complete_step` 调用结果并落入 execution output / artifact metadata，供引擎 finalize 使用。

当前实现并未消费 `complete_step`，而是直接消费 `StepSignal` 与对应 MCP 工具。

## 推荐分阶段实施（已结束）

### Phase 1（低风险）

- 定义 `complete_step` 协议并在提示词中引导优先调用。
- 执行器仅“记录并校验”该工具调用（不改变最终判定）。
- 保持当前 `WatchExecution` 成功即完成逻辑。

### Phase 2（切主路径）

- 对可控 profile 启用“必须调用 `complete_step` 才视为完成”。
- 未调用时按策略：超时 probe、重试或失败。

### Phase 3（gate 收敛）

- gate 的 JSON 行协议原计划迁移到 `complete_step.kind=gate`
- 实际演进为 `gate_approve` / `gate_reject`，并保留原 `AI_WORKFLOW_GATE_JSON` 兜底

## 风险与应对

1. **模型未调用工具**：使用“工具优先 + 文本兜底”灰度。
2. **工具参数不合法**：服务端 schema 校验 + 回写错误给 agent。
3. **多次调用 complete_step**：采用“首次成功写入生效，后续幂等拒绝”。
4. **NATS 分布式一致性**：将工具调用结果写入 execution output，并在 result message 中回传最终规范化状态。
5. **能力越权暴露**：通过“仅 execution 注入 + 会话生命周期回收”避免 `complete_step` 在非执行链路可见。

## 对当前文档的使用建议

- 如果要理解现行协议，请优先看 `docs/spec/gate-human-intervention.zh-CN.md`
- 如果要理解现行工具注册，请以 `internal/platform/appcmd/mcp_serve.go` 为准
- 本文适合作为历史演进记录，不适合作为当前实现规范

## 验收标准

- exec step 能通过 `complete_step` 显式进入 `done/blocked/failed`。
- gate step 能通过 `complete_step.kind=gate` 进入 pass/reject 分支。
- 关闭该工具时，系统仍按现有行为正常完成（兼容性通过）。
- 事件流中可检索到显式完成证据（tool call + step transition）。

# 2026-03-13 OTel / Audit Log Alignment Plan

## 1. 结论

这份计划仍然适用，但目标已经收口，不再按“完整 OTel 审计链路”推进。

当前确定的方向是：

- 不接所有应用日志，不做“所有日志都接 OTel”。
- 不保存 ACP `tool_call` 原始 payload。
- 不提供 `tool_call payload` 的回读接口。
- 不推进 OTLP 审计导出。
- 审计重点放在可索引摘要和业务决策结果。

## 2. 审计边界

### 2.1 保留什么

- `tool_call_audits`
  - 保存一次工具调用的轻量摘要。
  - 包含 tool name、status、started_at、finished_at、duration_ms、exit_code。
  - 包含 input/output/stdout/stderr 的 digest 与 redacted preview。
- `execution.audit`
  - 保存执行期关键过程节点的本地 JSONL 记录。
  - 用于排查 session 获取、dispatch、watch、deliverable persist、fallback 等关键流程。
- `ActionSignal`
  - 保存 ACP skill 或 MCP 路径产出的决策性数据。
  - 例如 `step-signal` 产生的 `complete / need_help / reject / approve`。

### 2.2 明确不保留什么

- ACP `tool_call` 的 input/output/stdout/stderr 原始 payload。
- `/tool-calls/{auditID}/payload` 这类 payload 回读接口。
- OTLP exporter。
- “把所有 slog / runtime log 都接到审计系统”的方案。

## 3. 数据模型

### 3.1 `tool_call_audits`

用途：

- 给管理端和排障接口提供结构化索引。
- 不承担原文归档职责。

字段原则：

- 保留摘要字段。
- 保留 redaction level，保证 preview 的口径可解释。
- 不保留 `log_ref`。

### 3.2 `ActionSignal`

用途：

- 表达 agent 在执行过程中产出的业务判断和决策结果。
- 这是审计中真正值得长期保留的“结果层信号”。

## 4. 查询面

保留接口：

- `GET /executions/{execID}/tool-calls`
- `GET /tool-calls/{auditID}`
- `GET /executions/{execID}/audit-timeline`

删除接口：

- `GET /tool-calls/{auditID}/payload`

`audit-timeline` 聚合范围：

- `events`
- `execution_probes`
- `tool_call_audits`
- `ActionSignal`

## 5. 实施阶段

### Phase 1: 审计索引

已完成：

- 新增 `tool_call_audits` 表与 store。
- ACP executor 接入结构化工具调用摘要写入。
- 管理端提供 tool call 列表与详情查询。

### Phase 2: 执行审计

已完成：

- 新增 `audit.Logger`。
- 新增 `execution.audit` 事件。
- 本地 JSONL exporter 已接入执行期关键节点。

### Phase 3: 决策信号收口

已完成：

- `audit-timeline` 聚合 `ActionSignal`。
- `step-signal` 这类 ACP skill 产出的决策结果进入统一调查视图。

### Phase 4: payload 删除收口

已完成：

- 删除 `tool_call payload` 本地落盘。
- 删除 payload 回读 API。
- 删除 `tool_call_audits.log_ref`。
- 删除 payload 相关配置项。

## 6. 验收标准

- 一次 tool call 会生成一条可查询的 `tool_call_audits` 摘要记录。
- 摘要 preview 必须经过脱敏。
- `execution.audit` 可落到本地 JSONL。
- `audit-timeline` 能同时看到事件、probe、tool call 摘要和 `ActionSignal`。
- 仓库中不存在 tool call payload 的持久化与查询入口。

## 7. 后续不做项

以下内容明确不在本轮范围：

- OTLP exporter
- payload 分片与回读
- payload retention
- 全量日志接入 OTel

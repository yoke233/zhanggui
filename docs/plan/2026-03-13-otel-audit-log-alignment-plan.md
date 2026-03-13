# OTel 审计日志对齐计划（仅覆盖审计链路）

## 摘要

本期不做“全项目所有日志全面 OTel 化”，只覆盖审计与调查最关键的链路：工具调用原文、执行调查日志、关键执行节点。  
方案采用“数据库索引 + OTel 日志原文”双层设计：

- 数据库继续保存结构化事实与检索索引，作为 UI/API/筛选/聚合的数据源。
- 原始审计明细统一对齐到 OpenTelemetry Logs 语义，优先通过 OTLP 导出；Collector 不可用时，降级写本地 OTel JSON/JSONL 文件。
- `chat.permission_request` 不纳入本期数据库审计范围，只保留现有实时聊天链路，不作为执行审计目标。

默认策略：

- 范围：仅审计日志
- 传输：`OTLP + 本地降级文件`
- 保留：`30 天`
- 脱敏：`基础脱敏`

## 关键变更

### 1. 数据库存储边界

保留现有结构化表作为主审计索引，不用 OTel 替代数据库：

- 继续使用现有 `executions / artifacts / events / execution_probes / usage_records / threads / thread_messages / thread_agent_sessions`
- 新增轻量审计索引表 `tool_call_audits`
- 该表只存摘要和定位信息，不存完整原文

`tool_call_audits` 需要包含这些字段：

- `id`
- `issue_id`
- `step_id`
- `execution_id`
- `session_id`
- `tool_call_id`
- `tool_name`
- `status`
- `started_at`
- `finished_at`
- `duration_ms`
- `exit_code`
- `input_digest`
- `output_digest`
- `stdout_preview`
- `stderr_preview`
- `log_ref`
- `created_at`

索引要求：

- `(execution_id, id)`
- `(tool_call_id)`
- `(step_id, id)`
- `(issue_id, id)`

### 2. OTel 审计日志模型

新增一套统一审计写入器，负责把审计明细组织成 OTel Logs 风格的结构化记录。  
不要求本期把所有 `slog` 改写，只要求审计事件使用统一 schema。

统一资源属性：

- `service.name=ai-workflow`
- `service.version`
- `deployment.environment`
- `aiworkflow.component`
- `host.name` 或实例标识

统一日志属性：

- `aiworkflow.issue_id`
- `aiworkflow.step_id`
- `aiworkflow.execution_id`
- `aiworkflow.thread_id`
- `aiworkflow.session_id`
- `aiworkflow.tool_call_id`
- `aiworkflow.audit.kind`
- `aiworkflow.log_ref`
- `aiworkflow.redaction.level`

本期定义 4 类审计日志事件：

- `execution.audit`
  记录执行关键节点，如 acquire session、start execution、watch result、fallback signal、probe request
- `tool.call.started`
  记录工具名、输入摘要、关联 execution/session
- `tool.call.finished`
  记录状态、耗时、退出码、输出摘要、日志定位
- `tool.call.payload`
  记录完整输入/完整输出/完整 stdout/stderr 的原文载荷，仅写 OTel 日志，不进数据库

原文载荷分片规则：

- 单条超过阈值时拆成多条 `tool.call.payload`
- 每条带 `tool_call_id + chunk_index + chunk_total`
- 数据库仅保存 digest、preview 和最终 `log_ref`

### 3. 导出与降级策略

新增审计 exporter 抽象，按下面优先级工作：

1. 如果配置了 OTLP endpoint，则发送 OTel Logs
2. 如果 OTLP 不可用或发送失败，则写本地降级文件
3. 不允许因为审计写失败阻塞主执行链路；最多记录内部错误并继续执行

本地降级文件策略：

- 根目录放在 `dataDir/audit/tool-calls/`
- 按日期分层：`YYYY/MM/DD/`
- 默认按 execution 分文件：`exec-<execution_id>.jsonl`
- `log_ref` 使用相对路径 + 可选偏移信息
- 轮转与清理按 30 天 retention 执行

### 4. 脱敏与内容边界

基础脱敏必须在进入 OTel 日志前执行，数据库和文件保持一致的脱敏结果。

本期基础脱敏规则：

- 屏蔽常见密钥字段：`token`、`api_key`、`authorization`、`password`、`secret`
- 屏蔽 Bearer、PAT、Git token、Cookie 等典型值
- 对 stdout/stderr 做基于模式的替换，不尝试语义理解
- 数据库 `preview` 使用脱敏后内容
- `digest` 基于原始内容计算，便于对账；原始内容只进入 OTel 审计载荷，不进入数据库

明确不做：

- 不做逐 token/流式思考持久化
- 不做全量 chat permission 审计
- 不做全项目普通应用日志统一迁移到 OTel
- 不做 traces 全链路接线

### 5. 查询与调查接口

本期提供后台调查接口，不做完整 UI 设计。

新增只读接口：

- `GET /executions/{execID}/tool-calls`
  返回某次 execution 的工具调用摘要列表
- `GET /tool-calls/{auditID}`
  返回单条工具调用摘要详情
- `GET /tool-calls/{auditID}/payload`
  按 `log_ref` 读取对应 OTel 原文载荷；只读、管理员可见
- `GET /executions/{execID}/audit-timeline`
  聚合 `events + probes + tool_call_audits`，用于事故调查时间线

返回体中统一包含：

- 结构化摘要
- `log_ref`
- `redacted=true/false`
- 原文读取失败时的明确错误码

## 实现变化

### 核心新增

- 新增审计模块，提供：
  - `AuditLogger`
  - `AuditExporter`
  - `OTLP exporter`
  - `Fallback file exporter`
  - `Redactor`
- 在执行器的工具调用事件汇聚点接入审计写入
- 在运行时关键节点补 `execution.audit` 事件
- 在 sqlite migration 中新增 `tool_call_audits`

### 现有链路接入点

优先改这几处：

- 执行器到事件桥接链路：从现有 `tool_call / tool_call_completed` 事件里补建数据库摘要 + OTel 原文
- 执行运行主线：execution start/watch/finalize/probe/fallback signal 写 `execution.audit`
- HTTP 后台接口层：暴露工具调用摘要与 payload 查询接口

### 配置新增

在现有配置中新增审计节，而不是复用普通 `[log]`：

- `audit.enabled`
- `audit.otlp.endpoint`
- `audit.otlp.headers`
- `audit.fallback.dir`
- `audit.retention_days`
- `audit.redaction.level`
- `audit.max_payload_bytes`
- `audit.payload_chunk_bytes`

默认值：

- `enabled = true`
- `retention_days = 30`
- `redaction.level = "basic"`
- `fallback.dir = "<dataDir>/audit/tool-calls"`

## 测试计划

### 单元测试

- `Redactor` 能正确屏蔽常见 token/password/bearer/cookie
- `tool.call.payload` 超长内容能正确分片
- OTLP exporter 失败时自动降级到本地文件
- `log_ref` 生成稳定且可回读
- `tool_call_audits` digest/preview/status/exit_code 写入正确

### 集成测试

- 一次成功的工具调用会：
  - 写数据库摘要
  - 写 OTel 审计原文
  - API 可查到摘要
  - API 可按 `log_ref` 取到 payload
- 一次失败的工具调用会：
  - 记录 `stderr_preview`
  - 记录 `exit_code`
  - payload 中包含脱敏后的 stderr
- Collector 不可用时：
  - 主执行仍成功
  - 本地降级文件存在
  - 数据库 `log_ref` 指向降级文件
- retention 清理任务只删除过期本地 payload，不误删数据库索引
- `chat.permission_request` 不会被错误写入新的执行审计表

### 回归测试

- 现有 `executions / artifacts / events / probes / usage / threads` 行为不变
- 现有 WebSocket 与事件流仍可用
- 现有 analytics 与 usage API 不受影响

## 假设与已定默认值

- 本期只做审计日志 OTel 化，不做全量应用日志 OTel 化
- 本期不做 OTel Trace 设计与接线
- 本期不把 `chat.permission_request` 纳入执行审计
- 工具调用完整输入输出进入 OTel 审计日志，不进入数据库
- 数据库必须新增轻量索引表，不能只靠日志
- 默认保留 30 天，采用基础脱敏
- 默认优先 OTLP，失败降级本地文件

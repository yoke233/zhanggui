# 00 范围与原则（不落库、不写代码阶段）

## 我们要解决的现实问题
- 多 Agent 并行时，最容易“看起来并行，实际串行”，最后卡在组长/主编。
- 多交付物（报告 + PPT + 合同/脚本）会出现口径不一致、重复劳动、强协议输出难控（JSON→HTML/PPTX）。
- 上下文膨胀：组长合并时 token 爆炸、跑偏、输出截断。
- 权限与隔离：不同 agent 不应互写文件；渲染器/工具不应篡改语义。

## 本阶段的边界
- 不做数据库设计、不做后端实现。
- 只讨论：最小规范、交互协议、产物格式、并行/调度策略。
- 一切以“文件系统约定 + 工具网关边界”作为落地载体（轻量）。

## 核心设计原则（精简版）
1. **最小内核**：固定少量 Core 字段，其他全部进 extensions（命名空间）。
2. **渐进式加载**：技能与产物都按需展开，不把全文塞进上下文。
3. **模块化交付**：每个执行单元（MPU）输出独立文件，天然无冲突。
4. **强协议边界**：Adapter/Renderer 只做转换/表现，不做语义决策；做不了就出 issue_list。
5. **并行当资源**：全局并行配额可回收再分配；谁能拿走由调度策略决定（不是抢）。

## 额外收敛约定（本轮讨论结论）
- **位置锚点使用 HTML Anchor**：在 Markdown 中插入 `<a id=...></a>` + `<!--meta ...-->`，由生成器在区块前自动写入，支持稳定跳转与追溯。
- **两级版本**：Major(vN) 表示用户/VP级“推翻重来”；Revision(rN) 表示同一 task 内返工迭代。
- **目录扁平化**：team/agent 等治理信息放在 TaskSpec（spec.yaml）里，不用目录层级表达。

## 最小工具网关/沙盒/审计（系统边界，不可自由发挥）
即便不落库，也必须把“能做什么/不能做什么”写死在工具网关层。

最小要求：
- **ACL（写权限）**：每个 task 只能写 `TaskSpec.outputs.*` 指定路径（或允许前缀）；禁止写其他文件。
- **配额**：每个 role/task 有 token/工具调用/并行数上限；超限必须降级或请求裁决。
- **审计字段**（至少记录到文件或日志）：
  - who: agent_id / role / team_id
  - what: tool_name + args 摘要（脱敏）
  - where: 读写的路径 / 外部连接域名
  - when: timestamp
  - result: success/fail + error
- **沙盒阶段**（最小三档）：local → container → vm（逐步强化隔离），不影响上层契约。

## X) 最小 Tool Gateway / 审计 / 索引边界（必须）
为避免实现者自由发挥导致越权/不可追溯，本节为硬性系统边界。

### X.1 写入 ACL（硬限制）
- Agent/合并器任何写入必须经过 Tool Gateway
- Tool Gateway 必须根据 TaskSpec 的 `outputs` 与 `policy.allowed_prefixes` 校验路径
- 校验失败：直接拒绝写入（不得自动改写路径）

### X.2 配额与并行（硬限制）
每个 run 必须同时满足：
- `max_parallel_units`（全局并行上限）
- `per_agent_max_parallel_subtasks`（单 agent 可开分身上限）
- `tool_calls_budget` / `token_budget`（到达即降级或停止）

### X.3 审计字段（硬要求）
每次 tool 调用必须记录：
- who: `agent_id`, `role`
- what: tool_name, action
- where: file_path（若写文件）
- when: timestamp
- result: ok/error + error_code
- linkage: `run_id`, `task_id`, `rev`

### X.4 检索索引边界（硬要求）
- 索引 corpus 只能来自 TaskSpec.retrieval.corpus_globs 匹配的文件
- `deliver/**` 永不纳入索引（禁止自引用闭环）
- 跨 case/version 检索默认禁止（除非显式配置 allowlist）


## 会议模式（可插拔）

- 会议不是默认路径；仅在需要澄清/裁决/对齐时触发。
- 会议产物也是文件（whiteboard/decision/transcript），可回灌 TaskSpec，且同样遵循“锚点 meta 由程序生成、Agent 不编字段”。

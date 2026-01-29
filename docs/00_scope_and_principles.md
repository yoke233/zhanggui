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
6. **审计先行**：事实写 ledger（append-only），证据走内容寻址；快照可覆盖但不作为真相源。

## 额外收敛约定（本轮讨论结论）
- **位置锚点使用 HTML Anchor**：在 Markdown 中插入 `<a id=...></a>` + `<!--meta ...-->`，由生成器在区块前自动写入，支持稳定跳转与追溯。
- **两级版本**：Major(vN) 表示用户/VP级“推翻重来”；Revision(rN) 表示同一 task 内返工迭代。
- **目录扁平化**：team/agent 等治理信息放在 TaskSpec（spec.yaml）里，不用目录层级表达。
- **ID 统一**：系统生成的运行时 ID（如 `task_id/run_id/pack_id/...`）统一使用 **UUIDv7**（可排序）。
- **审计单元（Bundle）不可变**：以 `run_id` 或 `pack_id` 为边界，Bundle 内 `ledger/evidence/report/zip` 只追加/只新建。

## 落盘真相源：Bundle + 三件套（v1）
本系统把“可恢复”“可复核”“可观测”拆开，避免一个文件承担所有职责：

- **Bundle（审计单元）**：一次运行/一次打包形成一个不可变边界。
  - `zhanggui`：`run_id` 天然是 Bundle
  - `taskctl`：每次 `VERIFY+PACK` 生成新的 `pack_id`（`packs/{pack_id}/...`），`pack/latest.json` 仅作“最新指针”
- **State（快照）**：`state.json` 负责“当前长什么样”（可覆盖；用于 UI/断点恢复）。
- **Ledger（账本）**：`ledger/events.jsonl` 记录“发生过什么”（append-only；审计真相源）。
- **Evidence（证据库/证据包）**：
  - `evidence/files/{sha256}`：内容寻址（create-only），用于冻结验收标准、报告、审批记录等结构化证据
  - `pack/evidence.zip`：离线复核包（默认 **nested** 包含 `pack/artifacts.zip`）
- **Approvals（人工裁决）**：`APPROVAL_*` 写入 ledger，并引用 `evidence/files/{sha256}`（v1 允许“先打包后审批”，详见提案）。

落盘布局与事件规范以 `FILE_STRUCTURE.md` 与 `docs/proposals/audit_acceptance_ledger_v1.md` 为准。

## 最小工具网关/沙盒/审计（系统边界，不可自由发挥）
即便不落库，也必须把“能做什么/不能做什么”写死在工具网关层。

最小要求：
- **ACL（写权限）**：每个 task 只能写 `TaskSpec.outputs.*` 指定路径（或允许前缀）；禁止写其他文件。
- **写入语义**：区分 `append-only / create-only / single-writer`（把“可改”和“不可改”写死到路径级规则）。
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
- linkage: `run_id`, `task_id`, `rev`（Bundle 场景额外通过 ledger 的 `correlation.pack_id/bundle_id` 对齐）

### X.4 检索索引边界（硬要求）
- 索引 corpus 只能来自 TaskSpec.retrieval.corpus_globs 匹配的文件
- `deliver/**` 永不纳入索引（禁止自引用闭环）
- 跨 case/version 检索默认禁止（除非显式配置 allowlist）


## 会议模式（可插拔）

- 会议不是默认路径；仅在需要澄清/裁决/对齐时触发。
- 会议产物也是文件（whiteboard/decision/transcript），可回灌 TaskSpec，且同样遵循“锚点 meta 由程序生成、Agent 不编字段”。

# PRD - Phase 2.7 真实 Worker 接入与 Wrapper 归一化

版本：v1.0  
状态：Draft  
负责人：Tech Lead / Platform / Integrator  
目标阶段：Phase 2.7（承接 Phase 2.6，先于 Phase 3）

## 1. 背景与问题

Phase 2 已完成 Lead 自动化闭环，Phase 2.6 已处理并发工作目录隔离。  
接下来进入“真实 worker 接入”阶段，会遇到输出形态不一致问题：

- 有的 worker 能输出结构化 JSON。
- 有的 worker 只能输出文本（甚至只有 stdout/stderr）。
- 不同 CLI 的字段命名、证据位置、错误表达不一致。

如果不统一包装层（wrapper），Lead 很难稳定做幂等判定和结构化写回。

## 2. 目标与非目标

目标：

- 通过 wrapper 统一接入不同 worker 执行器。
- 建立“多输入编码 -> 单一归一化结果”的处理链路。
- 保持 `IssueRef/run_id` 幂等规则不变。
- 保证 JSON worker 与 text worker 都能纳入同一流程。

非目标：

- 不要求所有 worker 原生支持 proto/binary。
- 不在本阶段统一所有工具的日志格式。
- 不改动协作协议字段（仍使用既有 comment 模板与 Outbox 语义）。

## 3. 用户与场景

用户：

- Role Lead / Integrator
- Worker 执行器维护者
- 平台开发者

核心场景：

- 场景 A：接入支持 JSON 输出的 worker（快速直通）。
- 场景 B：接入仅文本输出的 worker（通过 text envelope 解析）。
- 场景 C：接入仅有日志的 CLI（wrapper 负责组装最小结果并触发人工闸门）。
- 场景 D：切换 worker 类型后，Lead 仍可用同一套规则判定与写回。

## 4. 范围（In Scope）

- Worker Wrapper 抽象：统一 `WorkOrder` 输入与 `WorkResult` 归一化输出。
- 输出优先级策略：
  1. `work_result.json`（优先）
  2. `work_result.txt`（可解析 Header + 自由正文）
  3. stdout/stderr（仅降级，可能触发 `needs-human`）
- 解析与校验规则：
  - 必填：`IssueRef`、`RunId`、`Status`
  - 证据：`PR/Commit` 至少一个，`Tests` 必须显式存在（可 `n/a`）
- 失败策略：
  - 字段缺失、解析失败、`run_id` 不匹配时，不自动推进状态
  - 写回 `blocked` 或 `needs-human` 并附原因
- 适配器注册机制：按 worker 类型选择 wrapper（如 codex-cli、go-cli、自定义脚本）

## 5. 功能需求（PRD 级）

- FR-2.7-01：Lead 必须通过 wrapper 调用 worker，禁止直接依赖 worker 原始输出格式。
- FR-2.7-02：wrapper 必须支持 JSON 与 text 两种标准返回形态。
- FR-2.7-03：text 解析必须基于固定 Header Key（`IssueRef/RunId/Status/PR|Commit/Tests`）。
- FR-2.7-04：若 JSON 与 text 同时存在，必须以 JSON 为准并记录来源。
- FR-2.7-05：`run_id != active_run_id` 时，结果仅归档不推进状态。
- FR-2.7-06：归一化结果必须映射到 comment 模板关键字段后再写回 Outbox。
- FR-2.7-07：解析失败或证据缺失时必须进入人工接管路径（`needs-human`）。
- FR-2.7-08：wrapper 必须产出执行审计信息（worker 类型、耗时、退出码、结果编码、解析状态）。

## 6. 归一化策略（MVP）

### 6.1 输入

- 输入真源：`WorkOrder`（由 Lead 生成，含 `IssueRef`、`RunId`、`repo_dir`、约束信息）
- wrapper 负责把输入注入实际 worker（文件、参数或环境变量）

### 6.2 输出判定顺序

1. 读取 `work_result.json`，校验 schema 与关键字段。
2. 若无 JSON，读取 `work_result.txt`，解析 Header。
3. 若仅有 stdout/stderr：
   - 尝试提取最小锚点（IssueRef/RunId/Status）
   - 若不足则进入 `needs-human`

### 6.3 输出归一化字段

- `issue_ref`
- `run_id`
- `status` (`ok|fail|blocked`)
- `changes`（PR/Commit）
- `tests`
- `summary`
- `blocked_by`（可选）
- `questions`（可选）

### 6.4 写回策略

- 仅当 `run_id == active_run_id` 且关键字段校验通过时，允许自动写回。
- 写回统一走 `docs/workflow/templates/comment.md`。
- 对不合格结果，写“错误原因 + 下一步”而不是伪造成功状态。

## 7. 验收标准（DoD）

- AC-2.7-01：至少接入 1 个 JSON worker 与 1 个 text worker，均可完成闭环。
- AC-2.7-02：text worker 在字段齐全时可自动归一化并写回 Outbox。
- AC-2.7-03：字段缺失时不会误推进 `review/done`，会触发 `needs-human`。
- AC-2.7-04：旧 run 迟到结果不会覆盖当前 active run 状态。
- AC-2.7-05：审计日志可追溯每次 wrapper 解析路径与判定结果。

## 8. 成功指标

- 指标 1：新增 worker 接入平均时间下降 >= 50%。
- 指标 2：因输出格式不一致导致的手工修复次数下降 >= 70%。
- 指标 3：自动写回 comment 的字段完整率达到 95% 以上。
- 指标 4：`run_id` 幂等违规导致的状态污染事件为 0。

## 9. 风险与缓解

- 风险：不同 CLI 输出漂移频繁，解析规则脆弱。  
  缓解：固定 text Header 协议 + 版本化解析器 + 回归测试。

- 风险：stdout 自动提取误判。  
  缓解：stdout 仅作降级兜底，默认走 `needs-human`，不自动推进关键状态。

- 风险：wrapper 逻辑过重，影响执行性能。  
  缓解：wrapper 保持薄层，聚焦协议映射，不承载业务逻辑。

## 10. 依赖

- `docs/operating-model/executor-protocol.md`
- `docs/workflow/templates/comment.md`
- `docs/workflow/lead-worker.md`
- `docs/prd/phases/phase-2-prd.md`
- `docs/prd/phases/phase-2-6-prd.md`

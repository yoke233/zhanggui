# 15 回合制交付（Round-based Delivery）与 Profile（统一代码/文稿任务）

> 本文目标：把“文本任务 vs 代码任务”统一抽象到同一条主线：**回合制交付**。  
> 本文不是实现细节，而是用于：对齐术语、对齐状态机、对齐 UI 展示与后续扩展方向。  
>
> 与本仓库关系：  
> - 真相源：文件系统约定（见 `FILE_STRUCTURE.md`；审计/验收以 Bundle 的 `ledger/events.jsonl` 为准）  
> - 对外承载：AG-UI（见 `docs/11_ag_ui_integration.md`）  
> - Thread 状态：以 `fs/threads/{thread_id}` 的落盘快照/事件为准（见 `docs/12_runtime_and_input_model.md`）

---

## 0) 结论（先讲清楚）

1) **代码任务与文稿任务不需要两套系统**：它们都是“从立意到交付的一串回合（Rounds）”。  
2) 差异不在流程，而在“门槛/制度”：验收更严格、回合更多、Gate 更重。  
3) 因此我们引入两个概念：
- **Round（回合）**：一次推进循环（派发→执行→收集→验收→裁决）。  
- **Profile（画像/制度参数）**：不同任务类型的规则集合（产物要求、验收规则、Gate 策略、interrupt 策略）。

> v1 实践：Round 可直接映射为一次 `Run`（AG-UI run lifecycle），Profile 先作为配置/契约存在（不要求运行态实现）。

---

## 1) 统一主线（七步链）

建议对外统一叙事链路（便于 UI 展示与团队沟通）：

> 立意 → 立契 → 立据 → 立行 → 立验 → 立交 → 立账

对应到 v1 的“落盘/可复核”概念（最小映射）：
- 立意：`spec.md`（北极星：目标/约束/验收）+ `fs/threads/{thread_id}/state.json`（线程级公共状态快照）
- 立契：ChangeSet（变更单；写入 Thread 事件流/输入清单，并驱动 replan/re-run）
- 立据：`fs/threads/{thread_id}/inputs/manifest.json` + `inputs/files/{sha256}`（资料包/CAS）
- 立行：`fs/runs/{run_id}`（一次执行 = 一回合；AG-UI 事件流承载 run 生命周期）
- 立验：`revs/<rev>/issues.json`（最小门禁）→ 进入归档/发布前生成 `verify/report.json`（Bundle 内 create-only）
- 立交：Bundle（`packs/{pack_id}`；产物包/证据包/ledger）
- 立账：`ledger/events.jsonl` + `logs/tool_audit.jsonl`（事实层/审计层）

---

## 2) Round（回合）定义（v1：映射为 Run）

### 2.1 回合循环（固定节奏）
一个回合建议包含以下阶段（可作为 run 的 stepName 口径）：
- `ROUND_START`
- `DISPATCH`（派发 work units/agents）
- `EXECUTE`（并行执行）
- `COLLECT`（收集产物/证据）
- `EVALUATE`（评审/验收：由 Profile 决定规则）
- `DECIDE`（裁决：继续/返工/暂停/结束）
- `ROUND_END`

### 2.2 v1 的最小落地方式
- Round 不需要引入额外资源模型才能跑：**将每次推进做成一次 `Run`**。  
  - 优点：天然复用 AG-UI run lifecycle + interrupt/resume；日志与审计自然落到 `fs/runs/{run_id}`。  
  - 后续若要更强表达力，可在 v2 引入更明确的 Round 资源/视图（不影响 v1）。

---

## 3) Profile（画像/制度参数）

### 3.1 为什么需要 Profile
Profile 用于把“任务类型差异”收敛到制度参数，避免出现两套流程：
- 文稿任务：回合少、验收偏格式/事实/引用一致性
- 代码任务：回合多、验收偏 lint/test/coverage/security/merge/gate

### 3.2 Profile 最小字段建议（契约层）
> v1 建议先以配置文件/contract 存在（后续可升级为 `kind=TaskProfile`）。

- `round_policy`：回合推进策略（并行数、checkpoint 频率、超时）
- `artifact_policy`：必须产出哪些 artifacts（summary/issues/patch/test_report/coverage...）
- `evaluation_policy`：验收规则集合（schema 校验、CI、静态分析、安全扫描等）
- `gate_policy`：Gate 的收敛策略（合并方式、审批人、阻断条件）
- `interrupt_policy`：哪些节点必须 interrupt（发布/合并/对外出货）
- `definition_of_done`：DoD（通过条件）

---

## 4) 与 ThreadSnapshot 的关系（UI 如何展示）

UI 不需要理解“内部执行细节”，只需要：
- 从 Thread 快照读“北极星”（`spec.md` 或 thread state 中的当前目标/约束）与输入清单（inputs manifest）
- 从 Thread 快照读“全员进度”（可由系统汇总到 thread state）
- 从 `Run` 的事件流读“当前回合细节”（step/text/tool/interrupt）

建议 UI 分两层：
1) **Thread 面板（全局）**：北极星/资料包/进度/控制状态
2) **Run 面板（当前回合）**：本回合的 step timeline、日志、工具交互与 interrupt

---

## 5) v1 兼容性（不引入破坏性变更）

- 不要求新增 kind 即可落地：Round=Run、Profile=contract/config。  
- 协议扩展一律走新增 optional 字段；破坏性变更升 v2。  
- 现有 AG-UI demo 与 Tool Gateway 机制保持可用（无需重写）。

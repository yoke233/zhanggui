# 06 会议模式（Meeting Mode v2：上下文工程版）

> 本文是会议模式（Meeting Mode v2）的**权威规范入口**。  
> 目标：在不牺牲可控性/可追溯性的前提下，实现“发言串行 + 思考并行”的会议协作，并与 TaskSpec / Artifact Pipeline / Gate Node 无缝复用。  
> 关键约束：**共享文件单写者**（避免并行写入冲突）；参与者只提交提案，不直接改共享区。

---

## 0) 在定义协议前：先把边界写清楚（必须）

### 0.1 本文解决什么（范围）
- 把“会议”定义为一个可插拔拓扑：当出现分歧/不确定/阻塞时，以最小回合完成裁决与信息补全。
- 明确：会议的**文件结构**、**写入权限**、**提案/发言/收敛**的协议、以及会议结束后的**输出物**如何注入任务流。

### 0.2 本文不解决什么（非目标）
- 不规定具体 UI（聊天/网页/命令行均可）。
- 不规定具体实现语言/框架/数据库（默认仍为文件系统）。
- 不把会议变成“产出大段正文”的主路径；会议产物以 decision / action_items / brief 为主。

### 0.3 规范用语（为了可执行）
本文使用以下关键词：
- **必须**：不满足即视为协议违规（应被工具网关/Verifier 拒绝或报错）。
- **禁止**：出现即视为越权或不可验收。
- **建议**：默认策略；可以改，但要在 MeetingSpec/decision 中记录理由。

### 0.4 核心不变式（必须始终成立）
1) **单写者共享区**：`fs/meetings/{meeting_id}/shared/**` 仅允许 Recorder 写入。  
2) **可追溯**：进入 whiteboard/decision 的结论必须能回链到提案与 sources（会议内或外部）。  
3) **可控上下文**：运行时上下文装配只加载“必要片段”（白板 + 最近发言 + 必要证据），而不是全文 transcript。  
4) **不靠 Agent 自由发挥元字段**：锚点 id / meta 字段尽量由生成器或 Recorder 统一生成；参与者只填内容字段。

### 0.5 ID 约定（必须）
- `meeting_id`、`decision_id`、`item_id` 等**系统生成 ID** 统一使用 **UUIDv7**（可排序，便于按时间切片与检索）。
- `agent_id/role` 允许使用短 alias（例如 `planner`/`a03`），但必须在 MeetingSpec.participants 中显式声明（避免“口头角色”）。

> 落地计划见：`docs/08_development_plan.md`（语言无关、多阶段）。

---

## 1) 会议在流程中的位置（触发条件）

会议不是默认步骤，仅在以下触发条件出现时介入：

- **任务之前**（planning / clarification）
  - 需求不清晰、存在多条路线、必须明确 `acceptance.must_answer`
- **任务过程中**（fork / blocker）
  - 出现方向分叉（fork_detected）、阻塞（blocker_issue）、验证失败（verifier_conflict）
- **任务之后**（review / retro）
  - 验收争议、用户打回大版本、需要沉淀可复用结论

默认原则：**不开会；满足触发条件才开会；能用 Gate Node 解决的优先 Gate**（见 `docs/07_convergence_gates.md`）。

---

## 2) 上下文分层（会议记忆模型）

为控制 token 与避免角色混淆，会议上下文分三层：

### Layer A：工作草稿（Agent 私有，短命）
- `fs/meetings/{meeting_id}/agents/{agent_id}/vault/scratchpad.md`（可选）
- 内容：草稿/待发言点/局部推理；**不进入共享结论**

### Layer B：会议共享记录（全员可读）
- `fs/meetings/{meeting_id}/shared/transcript.log`（append-only）
- `fs/meetings/{meeting_id}/shared/whiteboard.md`（single-writer，可覆盖写）
- `fs/meetings/{meeting_id}/shared/hand_queue.json`（可选，single-writer）
- `fs/meetings/{meeting_id}/shared/decisions.md`（append-only，可选）
- `fs/meetings/{meeting_id}/shared/compaction/snap_0001.md`（可选，append-only 新文件）

> 说明：**“只保留最近 N 条发言”是上下文装配策略，不是文件截断策略。**  
> 文件本身保持 append-only；过长时由 Recorder 产出 compaction snapshot，运行时只读取 snapshot + 最近窗口。

### Layer C：归档结论（跨会议可检索）
- `fs/archive/meetings/{meeting_id}/decision.md`
- `fs/archive/meetings/{meeting_id}/action_items.md`
- `fs/archive/meetings/{meeting_id}/meeting_brief.md`

---

## 3) 会议拓扑：Think 并行 / Speak 串行 / Settle 串行

- **Think（并行）**：参与者围绕 focus 产出 Proposal Block（短、结构化、可追溯）。
- **Speak（串行）**：Moderator 依据 hand queue 选择发言者；Recorder 记录到 transcript。
- **Settle（串行）**：Recorder 将可采纳内容写入 whiteboard（Settled Block）。

> 并行只发生在“思考/提案生成”；共享区写入永远由单写者完成（见第 4 节）。

---

## 4) 文件与权限（强约束）

### 4.1 允许写入的路径（必须）

**参与者 Agent（architect/cost/security/writer 等）**
- ✅ 允许写：`fs/meetings/{meeting_id}/agents/{agent_id}/**`（如需落盘）
- ✅ 建议：不落盘，直接把提案内容返回给 Recorder 统一写入共享区
- ❌ 禁止写：`fs/meetings/{meeting_id}/shared/**`、`fs/archive/**`、`fs/cases/**`、`deliver/**`

**Recorder（共享区唯一写者）**
- ✅ 允许写：`fs/meetings/{meeting_id}/shared/**`、`fs/meetings/{meeting_id}/artifacts/**`
- ✅ 允许写：`fs/archive/meetings/{meeting_id}/**`（会议结束归档阶段）
- ❌ 禁止写：非授权的 case/task 目录（除非 action_items 明确要求且走 Tool Gateway）

### 4.2 写入类型（必须）
- `shared/transcript.log`：append-only（禁止覆盖写；内容必须按 Speak Block 追加，见 §5.2）
- `shared/decisions.md`：append-only（如存在）
- `shared/whiteboard.md`：single-writer（可覆盖写）
- `shared/hand_queue.json`：single-writer（可覆盖写）
- `shared/compaction/*.md`：append-only（只新增文件）

---

## 5) 资产与锚点协议（统一可追溯语法）

会议中所有可引用片段统一使用：**HTML anchor + HTML meta 注释**（rg 可定位、生成器可解析）。

### 5.1 Proposal Block（提案块，参与者产出）

**命名规则（必须）**
- `prop-{meeting_id}-{agent_id}-r{round}-{seq}`  
- `round` 从 1 开始；`seq` 建议 3 位补零（001, 002...）。

**最小结构（必须）**
```md
<a id="prop-{meeting_id}-{agent_id}-r{round}-{seq}"></a>
<!--meta
type: proposal
meeting_id: 019c0b5a-7c1d-7f3a-9d8c-0f2e3d4c5b6a
agent_id: a03
round: 2
intent: rebut|propose|question|evidence|synthesize
confidence: 0.72
needs_sources: true
-->
**point**：一句话观点/结论  
**why**：核心理由（2~5条）  
**evidence_refs**：S2,S5（可空）  
**conditions**：适用前提/边界  
**tradeoffs**：取舍（成本/质量/风险/时间）  
**ask**：需要主持人裁决的问题（可空）
```

### 5.2 Speak Block（发言记录块，Recorder 记录）

**命名规则（必须）**
- `spk-{meeting_id}-r{round}-{seq}`

**最小结构（必须）**
```md
<a id="spk-{meeting_id}-r{round}-{seq}"></a>
<!--meta
type: speak
meeting_id: 019c0b5a-7c1d-7f3a-9d8c-0f2e3d4c5b6a
round: 2
speaker: a03
intent: propose|rebut|question|summary
from_prop: prop-019c0b5a-7c1d-7f3a-9d8c-0f2e3d4c5b6a-a03-r2-001
ts: 2026-01-28T09:00:00+08:00
-->
内容：……（允许被自动截断；截断必须记录在 meta 中，例如 truncated=true）
```

### 5.3 Settled Block（白板收敛块，Recorder 维护）

whiteboard 只写“已收敛”的结论，并绑定来源提案：

```md
<a id="wb-{meeting_id}-{seq}"></a>
<!--meta
type: settled
meeting_id: 019c0b5a-7c1d-7f3a-9d8c-0f2e3d4c5b6a
from_props: [prop-019c0b5a-7c1d-7f3a-9d8c-0f2e3d4c5b6a-a03-r2-001, prop-019c0b5a-7c1d-7f3a-9d8c-0f2e3d4c5b6a-a01-r2-004]
sources: [S2,S5]
-->
结论：选择批处理作为当前版本，保留流处理升级接口。
```

---

## 6) 会议协议（一步一步）

> 本节把会议当作“可执行的状态机”，每一步都有输入/输出与责任人。

### Step 1：创建 MeetingSpec（程序/Moderator）
必须生成 `fs/meetings/{meeting_id}/spec.yaml`，并写入：会议类型、参与者、context_refs、limits、outputs、policy。

### Step 2：初始化目录（程序）
必须创建：`shared/`、`agents/`、`artifacts/`（以及可选的 `events/`、`shared/compaction/`）。

### Step 3：打开 Round（Moderator 指令，Recorder 落盘）
Moderator 给出本轮 `focus`（一句话问题）；Recorder 必须将其写入 `whiteboard.md` 顶部（或 decisions.md），并标注 round 编号。

### Step 4：Think 并行产出提案（参与者）
参与者必须在时限内提交 Proposal Block（短、结构化）。
- 建议通过消息提交给 Recorder；如需落盘，则写入 `agents/{agent_id}/outbox/turn_XXXX.md`。

### Step 5：举手与排队（可选，Recorder 单写）
若启用 `hand_queue.json`：参与者只“请求发言”；Recorder 负责更新队列。

### Step 6：Speak 串行发言（Moderator 选择，Recorder 记录）
Moderator 从队列中挑选 1 人发言；Recorder 将发言写入 `shared/transcript.log`（Speak Block）。

### Step 7：Settle 收敛（Recorder 单写）
Recorder 将达成一致/可裁决的内容写入 `shared/whiteboard.md`（Settled Block），并回链到提案锚点与 sources。

### Step 8：必要时 Compaction（Recorder）
当 transcript 过长或出现重要阶段性结论：Recorder 新建 `shared/compaction/snap_000N.md`，摘要历史发言并列出覆盖到的 spk id 范围。

### Step 9：结束会议并产出三件套（Recorder）
会议结束必须产出：
1) `artifacts/export_minutes.md`（会议纪要，可引用锚点）
2) `artifacts/action_items.yaml`（可执行变更：TaskSpec patch / 新任务建议）
3) `artifacts/citations.yaml`（sources 清单与外部引用）

### Step 10：归档（程序/Recorder）
将最小可复用输入写入：`fs/archive/meetings/{meeting_id}/meeting_brief.md` + `decision.md` + `action_items.md`（可由 `action_items.yaml` 渲染生成）。

---

## 7) MeetingSpec（spec.yaml）协议（最小可执行）

> MeetingSpec 是会议的“唯一真相源”，用于约束写入 ACL、上下文装配范围与输出路径。

**必填字段（必须）**
```yaml
schema_version: 1
meeting_id: 019c0b5a-7c1d-7f3a-9d8c-0f2e3d4c5b6a
type: planning   # planning/fork/blocker/review
topic: "数据处理架构选型"
participants:
  - { role: moderator, agent_id: pm }
  - { role: recorder, agent_id: recorder-01 }
context_refs:
  - { path: fs/cases/{case_id}/versions/v2/spec.md }
limits:
  think_timeout_s: 12
  speak_max_chars: 600
  max_rounds: 5
  max_minutes: 15
outputs:
  transcript_path: shared/transcript.log
  whiteboard_path: shared/whiteboard.md
  queue_path: shared/hand_queue.json
  decisions_path: shared/decisions.md
  minutes_path: artifacts/export_minutes.md
  action_items_path: artifacts/action_items.yaml
  citations_path: artifacts/citations.yaml
policy:
  # 约定：policy/outputs 中的路径，均以 “spec.yaml 所在目录（meeting root）” 为基准。
  # allowed_write_prefixes 负责把写入边界圈定在 meeting root 内；角色级规则见 §4 与 `docs/10_tool_gateway_acl.md`。
  allowed_write_prefixes: [""]
  append_only_files: ["shared/transcript.log","shared/decisions.md"]
  single_writer_prefixes: ["shared/","artifacts/"]
  single_writer_roles: ["recorder"]
  lock_file: "shared/.writer.lock"
```

**可选字段（建议）**
- `focus`：本次会议首轮 focus（一句话）
- `must_answer_refs`：受影响的 must-answer 列表（引用 task/case 的锚点）
- `evidence_policy`：是否允许外部资料、是否需要 citations 完整度
- `queue_policy`：队列优先级公式与 intents 列表

---

### 7.1 hand_queue.json（可选，single-writer）

> 用途：把“举手/排队”从自由群聊变成可控队列；参与者只提出请求，Recorder 负责更新与选中记录。

**最小结构（必须）**
```json
{
  "schema_version": 1,
  "meeting_id": "019c0b5a-7c1d-7f3a-9d8c-0f2e3d4c5b6a",
  "round": 2,
  "queue": [
    {
      "agent_id": "a03",
      "intent": "question",
      "requested_at": "2026-01-28T09:00:00+08:00",
      "note": "只写一句话摘要（可选）"
    }
  ]
}
```

**规则（必须）**
- 只能由 Recorder 覆盖写（single-writer）。
- 每次选中发言者后，Recorder 必须在 `shared/transcript.log` 写入对应 Speak Block（`from_prop` 可为空）。

---

## 8) 会议输出（注入任务流）

会议结束至少要能回答两件事：
1) **裁决了什么**（decision）  
2) **下一步怎么执行**（action_items）

因此会议关闭时必须产出两类输出：

### 8.1 会议内输出（fs/meetings/{meeting_id}/artifacts，必须）
- `export_minutes.md`：会议纪要（可引用 prop/spk/wb 锚点）
- `action_items.yaml`：可执行变更（TaskSpec patch / 新任务 / 请求用户裁决）
- `citations.yaml`：sources 清单（会议内外证据的索引）

### 8.2 归档输出（fs/archive/meetings/{meeting_id}，必须）
- `meeting_brief.md`：后续 Planner/Editor 的最小上下文（替代全文 transcript）
- `decision.md`：最终裁决（必须回链到 wb/prop/source）
- `action_items.md`：从 `action_items.yaml` 导出的可读版（必须；用于审计/复盘）

> 注意：Planner/Editor 默认只读 brief/decision/action_items；全文 transcript 仅用于审计与争议回放。

### 8.3 action_items.yaml（最小 schema，必须）

```yaml
schema_version: 1
meeting_id: 019c0b5a-7c1d-7f3a-9d8c-0f2e3d4c5b6a
generated_at: 2026-01-28T09:30:00+08:00
items:
  - item_id: 019c0b5b-0d2e-7f3b-8a9b-1c2d3e4f5a6b
    kind: task_patch          # task_patch|new_task|terminate_team|major_restart_request|ask_user
    summary: "把验收 must_answer 补齐到 10 条"
    need_approval_by: planner # planner|editor|user
    priority: P0              # P0|P1|P2
    rationale:
      from_wb: wb-019c0b5a-7c1d-7f3a-9d8c-0f2e3d4c5b6a-001
      from_props: [prop-019c0b5a-7c1d-7f3a-9d8c-0f2e3d4c5b6a-a03-r2-001]
    patch:                    # kind=task_patch 时必填（其余 kind 可省略）
      patch_spec_version: 1
      target:
        file: fs/cases/{case_id}/versions/v2/spec.md
        selector:
          kind: md_heading    # md_anchor|md_heading
          value: "acceptance.must_answer"
      ops:
        - op: md_append_lines # md_append_lines|md_replace_section|md_insert_after
          lines:
            - "- 11) ……"
            - "- 12) ……"
```

**规则（必须）**
- `items[].rationale` 必须能回链到 whiteboard/props（至少一个）。
- 任何涉及“改 case/spec”的 patch，必须声明 `need_approval_by`，并在决策链路中可审计。

#### 8.3.1 PatchSpec v1（语言无关，必须）

> 目的：把“会议结论要改哪些内容”表达成**可实现、可审计、可拒绝**的补丁；具体实现语言/工具不在本文范围。

**通用规则（必须）**
- Patch 必须是 **atomic**：任一 op 失败 → 整个 patch 失败，不得部分成功。
- Patch 不得隐式创建路径：selector/目标缺失必须失败，并产出 issue_list 或转为 `ask_user`。
- Patch 应支持“安全前置条件”（可选但建议）：例如 section/base hash 不一致则拒绝，避免静默覆盖。

**target.selector（必须）**
- `kind=md_heading`：`value` 必须与目标 Markdown 中的标题文本**完全一致**（不含 `#`）。选中范围为：
  - 从该标题行开始，到**下一个同级或更高层级标题**之前（若没有则到文件末尾）。
- `kind=md_anchor`：`value` 是 `<a id="..."></a>` 中的 `id`（不含 `#`）。选中范围为：
  - 从该 anchor 行开始，找到其后第一个标题行作为 section 起点；section 终止规则同 `md_heading`。
  - 若 anchor 后找不到标题行 → 失败（防止把整文件当 section）。

**ops（必须）**
- `md_append_lines`：把 `lines[]` 追加到选中 section 的末尾（在 section 结束边界之前）。
- `md_insert_after`：把 `lines[]` 插入到 selector 定位点之后（对 `md_anchor` 最常用）。
- `md_replace_section`：替换选中 section 的正文；可选字段：
  - `keep_heading: true|false`（默认 true；保留标题行，仅替换正文）

**安全前置条件（建议）**
```yaml
preconditions:
  section_sha256: "..."  # 以目标 section 原文计算；不一致则拒绝
```

### 8.4 citations.yaml（最小 schema，必须）

```yaml
schema_version: 1
meeting_id: 019c0b5a-7c1d-7f3a-9d8c-0f2e3d4c5b6a
sources:
  - id: S1
    kind: file     # file|url|note|meeting_anchor
    ref: "fs/cases/{case_id}/versions/v2/spec.md#constraints"
    title: "当前版本约束"
    captured_at: 2026-01-28T09:10:00+08:00
  - id: S2
    kind: url
    ref: "https://example.com/spec"
    title: "外部规范（示例）"
```

**规则（必须）**
- `sources[].id` 必须全局唯一（同一 meeting 内）。
- Proposal 中的 `evidence_refs` 必须引用 `sources[].id`；若 `needs_sources=true` 则 `evidence_refs` 不得为空（否则必须转为 issue/ask_user）。

### 8.5 decision.md（归档，最小模板，必须）

路径：`fs/archive/meetings/{meeting_id}/decision.md`

```md
# Decision — {topic}

<a id="dec-{decision_id}"></a>
<!--meta
type: decision
meeting_id: 019c0b5a-7c1d-7f3a-9d8c-0f2e3d4c5b6a
decision_id: 019c0b5c-2a3b-7f3c-8b9c-2d3e4f5a6b7c
ts: 2026-01-28T09:30:00+08:00
from_wb: wb-019c0b5a-7c1d-7f3a-9d8c-0f2e3d4c5b6a-001
from_props: [prop-019c0b5a-7c1d-7f3a-9d8c-0f2e3d4c5b6a-a03-r2-001]
sources: [S1,S2]
-->

decision: choose_a            # choose_a|choose_b|keep_parallel|defer_to_user|no_decision
summary: "一句话裁决结果"
rationale:
  - "理由1（应可回链到 from_props/sources）"
  - "理由2"
action_items:
  - 019c0b5b-0d2e-7f3b-8a9b-1c2d3e4f5a6b
open_questions:
  - "仍需用户确认的点（如有）"
dissent:
  - agent_id: a01
    note: "异议摘要（可选）"
```

**规则（必须）**
- `decision` 必须能回链到 `from_wb`（至少一个 wb）。
- `action_items` 必须引用 `action_items.yaml` 的 `items[].item_id`。

### 8.6 meeting_brief.md（归档，最小模板，必须）

路径：`fs/archive/meetings/{meeting_id}/meeting_brief.md`

```md
# Meeting Brief — {topic}

<a id="brief-{meeting_id}"></a>
<!--meta
type: meeting_brief
meeting_id: 019c0b5a-7c1d-7f3a-9d8c-0f2e3d4c5b6a
ts: 2026-01-28T09:30:00+08:00
-->

## 1) 一句话结论
- …

## 2) 本次 focus（问题）
- …

## 3) 已收敛结论（指向白板/决策）
- wb: wb-019c0b5a-7c1d-7f3a-9d8c-0f2e3d4c5b6a-001 → dec: dec-019c0b5c-2a3b-7f3c-8b9c-2d3e4f5a6b7c

## 4) 下一步（行动项）
- 019c0b5b-0d2e-7f3b-8a9b-1c2d3e4f5a6b：…（need_approval_by=planner）

## 5) 关键背景（仅保留必要上下文）
- …
```

**规则（必须）**
- meeting_brief 必须在不读全文 transcript 的情况下，支持 Planner/Editor 继续工作（只保留最小必要上下文）。

### 8.7 action_items.md（归档，渲染规则，必须）

路径：`fs/archive/meetings/{meeting_id}/action_items.md`

**规则（必须）**
- 必须由 `fs/meetings/{meeting_id}/artifacts/action_items.yaml` 渲染生成（不可人工改写语义）。
- 每条必须包含：`item_id`、`kind`、`summary`、`need_approval_by`、`priority`、以及 `rationale.from_wb`（或 from_props）。

---

## 9) 最小实现契约（给工程落地）

必须写死的两条契约（不满足即不可验收）：
1) Proposal/Speak/Settled 的**最小字段 + 锚点命名规则**（rg 可定位、解析器可解析）
2) Recorder 单写者 + **ACL 路径白名单**（共享区与归档区只允许 Recorder 写）

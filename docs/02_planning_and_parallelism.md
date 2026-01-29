# 02 计划与并行（Delivery Plan + MPU + spawn + 调度配额）

## 1) Delivery Plan：让“动态组团”可执行
最小字段：
- case_id / goal / deliverables[]
- teams[]（可选；分叉才需要多个）
- roles[]（每个 Team 的角色实例）
- quality[]（校验节点）
- budgets（可选）
- audit_policy（可选；例如 `approval_policy=always|warn|gate|never`）

## 2) MPU：并行最小单元（Minimum Parallel Unit）
一个 MPU 必须满足：
- 单一目标（一句话说明）
- 输入边界清晰（读哪些文件/节点）
- 输出是一个文件（模块文件）
- 与其他 MPU 弱依赖（可最后合并）

## 3) spawn（分身）制度化：允许，但有硬规则
允许 spawn 的条件：
- 子任务可模块化输出（独立文件）
- 子任务之间弱依赖
- 工具/权限需要隔离
禁止 spawn：
- 本质是单线程裁决（需要统一决策）
- 会写同一文件/同一资源

建议限制：
- spawn 深度 ≤ 2
- 每 agent 同时 spawn ≤ 3~5

## 4) 并行度作为“资源池”（Global Concurrency Pool）
- GLOBAL_MAX：全局并行上限（例如 10）
- TEAM_MAX：每 Team 上限（防某路线吃光）
- AGENT_MAX：每角色/agent 上限（防无限分身）

slot 以 Lease（租约）形式发放，可续租、可回收。
运行单元 DONE/CANCELLED → slot 回收 → 再分配。

## 5) “谁能拿走”并行资源？
建议默认：**中心化分配**
- 角色/agent 只能 request_slots(n, tasks, reason)
- 调度中心按优先级/关键路径/截止时间/公平性分配

可选加速通道：主编/监督者可对某交付物倾斜预算（拨款），但仍由调度中心执行发放。

## 6) 抢占（Preemption）策略（轻量）
仅对可重跑/低价值/卡死任务抢占：
- non_preemptible / soft_preemptible / hard_preemptible 三档
做不了就降级：出 issue_list 或把任务切小重跑。

## 7) 文件布局建议（扁平化）
为了避免目录爆炸：不再用多层 team/agent 目录表达治理，而是把治理字段放进 TaskSpec。

推荐（单 case）：
```
fs/cases/{case_id}/
  current.yaml                 # 指向当前 vN（程序/Planner 单写者维护）
  versions/
    v2/
      spec.md                  # Master IR / 需求与验收（Planner/Editor 单写者）
      tasks/
        task-000123/
          spec.yaml
          current.yaml         # 指向当前 rN（程序维护）
          revs/
            r1/ (summary.md, cards.md, issues.md)
            r2/ ...
          packs/               # 审计单元（Bundle；不可变，按 pack_id 切片归档）
            {pack_id}/...
          pack/                # latest 指针/快捷入口（可覆盖；不作为审计依据）
            latest.json
          review.md
      deliver/
        report.md
        slides.md
```

## 8) 版本两级：Major(vN) vs Revision(rN)
- **Major(vN)**：用户/VP层面推翻重来 → 新建 `vN+1/`，更新 `fs/cases/{case_id}/current.yaml`
- **Revision(rN)**：组长/审核打回返工 → 同一 task 下新建 `revs/rN+1/`，不覆盖旧版本
读取最新：通过 `task/current.yaml` 指针定位当前 rN。

## 9) TaskSpec 引用（进一步减负）
Agent 产物文件只写：
- `task_spec_ref: ../../tasks/task-000123/spec.yaml`（或 task_id）
其余元字段全部由程序/调度中心在 spec.yaml 中维护。

## 10) 从需求到计划：Team Builder / Role Selector 的“契约”
目标：把“动态组团”从口号变成可实现的输入输出；实现者按契约做即可，不自由发挥。

### 输入（来自用户需求 + Master IR）
```yaml
goal: "..."
deliverables: [report, ppt]           # 需要产出哪些交付物
constraints:
  time: "..."
  budget: "..."
  tech_boundary: ["..."]
acceptance:
  must_answer: [1,2,3]
  must_not: ["不要引入新事实", "不要改动范围"]
context_refs:
  - global/requirement.md
  - global/master_ir.yaml
```

### 输出（Delivery Plan 片段：可直接喂给调度中心）
```yaml
teams:
  - team_id: team_a
    intent: "主方案（稳健）"
  - team_id: team_b
    intent: "备选方案（激进/更快/更省成本）"
roles:
  # 角色不是“固定编队”，而是按交付物链路与风险点最少集
  - role: planner_editor
    count: 1
    owns: ["master_ir", "outline", "acceptance_gate"]
  - role: domain_writer
    count: 2
    owns: ["cards", "sections"]
  - role: ppt_transformer
    count: 1
    owns: ["ppt_ir"]
  - role: verifier
    count: 1
    owns: ["coverage_map", "issue_list"]
quality:
  - gate: "must_answer_coverage"
  - gate: "no_new_facts"
  - gate: "cross_deliverable_consistency"
budgets:
  max_parallel: 10
  per_role_parallel_cap:
    domain_writer: 3
    ppt_transformer: 1
```

### 选择器（收益 > 成本）的最小启发式
- **需要多个 Team 的信号**：方向不确定/有明显分叉/代价差异大/需要对比（最多 3 个 Team）。
- **需要新增角色的信号**：出现强协议输出（JSON schema）→ 必有 Adapter/Verifier；涉及安全/权限 → 必有 Tool Gateway/审计。
- **避免过度编队**：能用“Verifier + cards”解决的，不要再加“二次专家审查”。

### 最小例子：报告 + PPT
- Planner/Editor：产出 Master IR + 章节/页大纲（锚点）
- Writers：按大纲节点并行产出 cards/sections（一个节点一个文件）
- PPT Transformer/Adapter：把 Master IR 投影为 PPT_IR → ppt_renderer_input.json
- Verifier：覆盖度 + 一致性 + 长度/丢失风险，必要时生成 issue_list

## X) 检索优先（BM25/向量）与 Cards 触发规则（写死，不模糊）

本系统默认采用 **“检索找材料 +（必要时）Cards 结构化合并”** 的策略：
- 检索（BM25/可选向量）解决 **“找得到”**
- Cards 解决 **“合并不跑偏 + 可验收 + 可追溯”**
- 任何情况下，都必须保留 **引用协议（manifest/引用块）**，否则后续无法审计与回放

### X.1 TaskSpec 必填字段（与检索/合并相关）
在 `tasks/<task_id>/spec.yaml` 中，必须包含以下字段（缺一不可）：

```yaml
artifacts_required: ["summary"]         # 默认只要 summary
retrieval:
  enabled: true
  mode: "bm25"                          # bm25 | hybrid
  top_k: 25                             # 合并器检索返回片段数量
  chunking:
    unit: "md_block"                    # md_block(按标题/段落块) | paragraph
    max_chars: 1200                     # 单 chunk 最大字符数（硬上限）
    overlap_chars: 120                  # chunk 重叠字符数
  corpus_globs:                         # 只在本 case/version 内检索（禁止跨 case）
    - "tasks/**/revs/**/summary.md"
    - "tasks/**/revs/**/full.md"
  deny_globs:                           # 明确禁止纳入索引的路径
    - "deliver/**"
    - "log/**"
cards_policy:
  required_when:                        # 触发 Cards 的硬规则（满足任一则必须产 Cards）
    - "parallel_teams>=2"
    - "must_answer_count>=6"
    - "has_tradeoffs=true"
    - "needs_comparison_matrix=true"
    - "conflict_detected=true"          # 检索结果出现互斥结论/相反建议（由 verifier 标注）
    - "evidence_required=true"          # 需要明确证据链（合同/合规/投研等）
  optional_when:
    - "parallel_teams==1 && must_answer_count<=5 && has_tradeoffs=false"
citation_policy:
  manifest_required: true
  cite_fields: ["task_id","rev","file","chunk_id","start_line","end_line","sha256"]
```

> 解释：  
> - `mode=bm25`：只使用 BM25（默认，先落地）；  
> - `mode=hybrid`：BM25 + 向量召回（需要时再开），但引用协议不变；  
> - `deny_globs` 硬限制：交付物与日志永不入索引，避免“合并器拿交付物当证据”自循环。

### X.2 Cards 是否需要：最终判定规则（完全确定）
- 若 `cards_policy.required_when` 任一条件为真：`artifacts_required` 必须包含 `"cards"`
- 否则：`artifacts_required` 仅包含 `"summary"`（可选 `"full"`，见下条）
- 若任务属于“内容长且可能被摘抄”场景（报告/论文/脚本）：建议加 `"full"`，但不是强制

### X.3 检索输出格式（合并器/主编必须按此消费）
检索服务/本地脚本输出必须是严格结构化对象（JSON/YAML 皆可），每个命中片段必须包含：

- `task_id`, `rev`
- `file`
- `chunk_id`（稳定：`<file>#<n>` 或 hash）
- `start_line`, `end_line`
- `score_bm25`（若 hybrid 再加 `score_vec`）
- `text`（命中正文片段，允许截断但必须可回溯定位）

合并器不得把“检索命中片段”当最终内容直接改写为新事实；任何新增断言必须能回指到某个 `chunk_id`。

### X.4 Cards 最小字段（如果触发 Cards，必须包含这些）
Cards 文件中每张卡必须包含（缺一不可）：

- `claim`：一句主张（不可超过 200 字）
- `evidence`：引用列表（至少 1 条，使用 citation_policy 的字段）
- `conditions`：适用条件/边界（至少 1 条）
- `tradeoffs`：代价/风险（至少 1 条；没有则写 `tradeoffs: none`）
- `links_to_outline`：绑定到 `assigned_outline_nodes`（至少 1 个 node）
- `confidence`：0~1（可选，但建议提供）

### X.5 “检索替代 Cards”的边界（写死）
允许“只用检索 + summary”而不产 Cards 的前提同时满足：
1) `parallel_teams==1`  
2) `must_answer_count<=5`  
3) `has_tradeoffs=false && needs_comparison_matrix=false`  
4) verifier 未标记 `conflict_detected`  
否则必须产 Cards。

## Y) Planner/PM：从需求到 Team/MPU 的决策契约（写死）

本系统区分两层“并行”：
- **Team 并行（宏观分支）**：不同方案/路线/交付路径并行探索（上限通常 3）
- **MPU 并行（微观单元）**：同一方案下，按大纲节点/必须回答拆分的执行单元（可更高并行）

### Y.1 谁决定派几个 Team？
唯一决策者：**PM/Planner（同一角色的两个面向）**
- Planner 负责生成 Master IR、Delivery Plan、TaskSpec
- PM 负责对用户汇报、处理中途指令、裁决分叉、最终签字

> 实现上可以是一个 Agent（带不同 sub-role prompt），也可以是两个 Agent，但决策权必须集中，不可让 Team 自行增殖。

### Y.2 派 Team 的硬规则（不靠模型随意判断）
默认：`team_count=1`。只有满足以下条件才允许派生额外 Team（每条是硬触发）：

- `has_competing_strategies=true`（至少两条路线在成本/风险/收益上显著不同）
- `decision_is_directional=true`（方向性选择，不做会影响整体交付）
- `user_explicitly_requests_options=true`（用户明确要多方案对比）
- `uncertainty_high=true` 且 `time_budget_allows_parallel=true`

并行 Team 上限：
- `max_parallel_teams` 默认 **3**（与我们此前讨论一致）
- 若达到上限且仍需分叉：必须 terminate 一个 Team 或请求用户裁决（参见 `05_user_interaction.md`）

### Y.3 分叉与资源分配（写死）
- 新 Team 必须有 `fork_reason` 和 `fork_point`（写入 decisions/forks）
- 每个 Team 初始配额默认均分；可由 PM/Planner 明确倾斜
- Team 结束或被 terminate 后，其配额回收至资源池，由 scheduler 再分配

### Y.4 Master IR 到 TaskSpec 的生成链路（回答“TaskSpec 谁生成”）
1. PM/Planner 生成 `spec.md`（Master IR：goal/constraints/deliverables/must-answer/outline）
2. Planner 生成 `Delivery Plan`（teams/roles/quality/budgets）
3. Planner 为每个 MPU 生成 `tasks/<task_id>/spec.yaml`
4. 若 Team 内需要更细拆分：**只能由 Planner** 生成子 TaskSpec（Team Lead 只能提出建议，不得自行创建）

### Y.5 主编（Editor）是谁？
- **Editor=合并器决策者**：负责 Normalize/Assemble 的“最后口径”
- 默认由 PM/Planner 兼任（小规模）；当并行 Team >=2 或交付物 >=2 时，建议拆分为独立 Editor
- Verifier 不等于 Editor：Verifier 只做规则检查与出 issue_list，不做业务裁决


## 11) 审计单元（Bundle）与审批策略（可控）
为避免“无限长总账”，同时保证可归档/可复核，本系统把审计边界定义为 Bundle：

- **Bundle 边界**：以 `run_id` 或 `pack_id` 做不可变边界；每次验收/打包生成新的 Bundle；`latest` 指针允许覆盖写。
- **证据链分层**：
  - `state.json`：快照（可覆盖；用于 UI/恢复）
  - `ledger/events.jsonl`：事实账本（append-only；审计真相源）
  - `evidence/files/{sha256}`：内容寻址证据（create-only；冻结标准/报告/审批记录）
  - `pack/evidence.zip`：离线复核包（默认 nested 包含 `pack/artifacts.zip`）
- **审批策略默认“所有都要”**：可下沉到 `audit_policy` 里做可控开关：
  - `always`：每次 `VERIFY+PACK` 都写 `APPROVAL_REQUESTED`
  - `warn`：仅当存在 `warn` 类问题/门禁时才请求审批
  - `gate`：仅当命中特定门禁（例如按 `issues.where` 匹配）才请求审批
  - `never`：不自动请求审批（仍允许手工写审批事件）
- **v1（B 方案）允许“先打包后审批”**：审批事件写入 ledger，但已生成的 `evidence.zip` 不回写；复核以 `ledger + evidence/files` 为准。

具体落盘与事件规范见：`FILE_STRUCTURE.md`、`docs/proposals/audit_acceptance_ledger_v1.md`。

## 12) 会议模式（可插拔拓扑）
- 会议是一个 Topology Plugin：用于“决策收敛/分歧处理/信息补全”，不替代 MPU 产出流水线。
- 会议的并行：Think 并行占用 slots；Speak 串行不等人。
- 触发点与产物格式见：`docs/06_meeting_mode.md`。

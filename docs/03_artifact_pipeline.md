# 03 多交付物与强协议流水线（Artifact Pipeline + 插件契约）

## 1) 为什么要分 Master IR / View IR
- Master IR：全局口径与动态大纲的“真相源”（少字段，灵活）
- View IR：面向具体交付物的投影（PPT_IR / REPORT_IR / CONTRACT_IR）
这样一个需求可以有多个不同大纲（PPT 一页一页；报告一章一章），但口径来自同一 Master IR。

## 2) 强协议节点：JSON→HTML（PPT）谁负责？
结论：由插件链负责，主编负责语义签字。

- Transformer：Master IR → PPT_IR（语义页结构）
- Adapter：PPT_IR → ppt_renderer_input.json（严格 schema）
- Renderer：ppt_renderer_input.json → slides.html（表现层）
- Verifier：覆盖度/一致性/格式/损失校验

## 2.5) 审计/验收闭环（Bundle + ledger + evidence）
强协议流水线一旦进入“可交付/可发布”动作，就必须具备可复核证据链（否则后续无法审计与回放）：

- **Bundle（审计单元）**：以 `run_id/pack_id` 做不可变边界；除 `state.json` 外，Bundle 内产物尽量 `append-only/create-only`。
- **Ledger（真相源）**：`ledger/events.jsonl` 记录关键事实（step/产物/审批），大内容只通过 `refs` 引用证据文件。
- **Evidence（证据库/证据包）**：
  - `evidence/files/{sha256}`：内容寻址（create-only），用于冻结标准、报告、审批记录等结构化证据
  - `pack/evidence.zip`：离线复核包（默认 nested 包含 `pack/artifacts.zip`），至少包含 ledger、tool_audit、verify report、manifest
- **Approvals（裁决）**：`APPROVAL_REQUESTED/GRANTED/DENIED` 写入 ledger，并引用 `evidence/files/{sha256}` 的审批记录。

> v1 允许“先打包后审批”（B 方案）：审批事件不会回写已生成的 `evidence.zip`；审计以 `ledger + evidence/files` 为准（详见 `docs/proposals/audit_acceptance_ledger_v1.md`）。

## 3) 插件契约（必须守边界）
### Transformer
- 允许：重组/映射/裁剪
- 禁止：引入新事实/新假设（除非 issue_list 请求裁决）
- 必产出：coverage_map + transform_log

### Adapter
- 责任：schema 校验、默认值、长度限制处理、字段归一化
- 禁止：改变语义；必须提示“损失风险”
- 必产出：transform_log

### Renderer
- 仅表现层生成；不得做内容决策
- 建议产出：渲染摘要（页数/失败原因）

### Verifier
- 输出：verify_report + issue_list
- 关注：must-answer 覆盖、report/ppt 口径一致、压缩丢失

## 4) issue_list：统一“做不了/有冲突”的反馈
字段建议：
- severity: blocker/warn/info
- where: transform/adapter/render/verify
- what: 问题描述
- options: 可选策略（裁剪/拆分/回问用户/降级）
- need_decision_by: Editor/User/Planner
- suggested_patch: 可选

约定：
- `where` 建议使用稳定枚举，便于做“门禁/审批”策略（例如按 `where` 匹配触发 `APPROVAL_REQUESTED`）。
- issue 内容必须脱敏；需要引用原文/附件时，写入证据库并以 `sha256` 绑定。

> 重要：强协议输出的最终节点可以是“PPT 生成器/渲染器”，但**交付责任仍在主编**（语义签字）。

## 5) 自动锚点与可追溯（Markdown 内嵌 HTML Anchor，不靠 Agent）
- 最终交付物（report/ppt）由生成器拼装时，**自动在每个区块前插入 HTML Anchor**，实现稳定跳转。
- 紧跟一行 `<!--meta ...-->` 记录 `task@rev` 与 sources，保证可追溯，不要求 Agent 写任何标记。
- 组长阅读时，anchor/注释默认不可见；需要定位时直接使用 `path#id`。

区块模板：
```md
<a id="block-deliver-report-2"></a>
<!--meta task=task-000123@r2 sources=task-000123@r2-->
## 2. 并行与资源池
...
```

引用示例：
```md
见：[报告第2章](deliver/report.md#block-deliver-report-2)
```

## 6) 合并器协议（Normalize → Assemble → Verify）
目标：让“主编合并”可实现、可验收、可回滚，避免合并器随意引入新事实。

### 6.1 Cards 最小字段（可合并协议）
每张卡至少包含：
- `card_id`（程序生成或合并器补齐）
- `links_to_outline`（绑定到大纲节点，如 report:2 / ppt:s05）
- `claim`（主张/结论）
- `evidence`（依据：引用/数据来源/推理链简述）
- `conditions`（适用条件/假设）
- `tradeoffs`（取舍/副作用）
- `confidence`（可选：0~1，自评）

### 6.2 阶段产物（合并过程必须留下脚印）
- `deliver/manifest.md`：本次交付引用了哪些 `task@rev`、哪些 `card_id`
- `deliver/coverage_map.yaml`：must-answer 与 outline 节点的覆盖映射（Verifier/合并器产出）
- `deliver/issue_list.md`：无法解决/需裁决的事项（blocker 优先）

### 6.3 规则：防止“合并器引入新事实”
- 合并器只能“重排/裁剪/归并/改写表达”，**不得新增事实**；
- 若必须新增（例如为了连贯补充一句假设），必须写入 `issue_list` 并标注 `need_decision_by`；
- Verifier 对比 cards 与最终 deliver：出现“无来源句子”即报警。

### 6.4 绑定到位置锚点（Anchor + meta）
- 每个交付区块前的 `<a id=...></a>` + `<!--meta ...-->` 由生成器写；
- `meta` 的 `sources=` 应从 manifest 自动生成（task@rev + card_id 可选）。

## X) 合并器协议（必须遵守，不得自由发挥）

合并器（Editor/Assembler）只做三件事：**归一化 → 组装 → 验证**。任何偏离都会导致不可验收或不可追溯。

### X.1 Normalize（归一化）
输入：各任务产物（summary / optional cards / optional full）与检索命中列表。  
输出：`deliver/manifest.(md|yaml)` 的草稿与候选引用集合。

硬规则：
- 不得改写事实：Normalize 只允许“摘取/重排/去重/格式统一”
- 任何句子若新增断言（新事实/新数字/新因果），必须标记为 `NEW_ASSERTION` 并进入 `issue_list`
- 任何引用必须携带 citation_policy 规定字段（见下）

### X.2 Assemble（组装）
输入：Normalize 的候选集合 + `spec.md` 的结构（outline/slide 计划）。  
输出：最终交付物（例：`deliver/report.md`, `deliver/slides.md`）与 `coverage_map.yaml`。

硬规则：
- 交付物每个章节/slide 区块前必须插入锚点与 meta（生成器写）：
  ```md
  <a id="block-deliver-report-2"></a>
  <!--meta task=task-000123@r2 sources=task-000120@r1,task-000121@r1-->
  ```
- `id` 命名必须使用：`block-<scope>-<deliverable>-<node>`
- 区块内容只允许来自：
  1) 任务产物（summary/cards/full）的可回溯片段
  2) 用户输入的原始需求/补充信息
- 合并器不得把“交付物旧版本”当来源（禁止自引用闭环）

### X.3 Verify（验证）
输入：交付物 + manifest + coverage_map + issue_list  
输出：`deliver/issue_list.md`（若无问题可为空）与“通过/不通过”结论（供调度中心决定是否返工）。

必须检查：
- must-answer 覆盖：每条 must-answer 都必须在 coverage_map 中有 >=1 个证据引用
- 冲突检测：同一 must-answer 下若出现互斥结论，必须进 issue_list 并触发 cards_policy.required_when: conflict_detected=true
- 引用可回放：manifest 中每条引用都能定位到原文件的 `start_line~end_line`

---

## Y) 合并阶段必须产物（固定三件套）

### Y.1 manifest（引用清单，必须有）
路径：`deliver/manifest.yaml`（或 .md，但字段必须等价）

每条引用必须包含（缺一不可）：
- `task_id`
- `rev`
- `file`
- `chunk_id`
- `start_line`, `end_line`
- `sha256`（对引用片段文本计算）
- `used_in`：被用于哪个区块（例：`block-deliver-report-2`）

### Y.2 coverage_map（验收映射，必须有）
路径：`deliver/coverage_map.yaml`

字段要求：
- `must_answer_id` → `evidence_refs[]`（每个 evidence_ref 指向 manifest 的一条引用）
- 同时记录 `status: covered|missing|conflicted`

### Y.3 issue_list（问题单，必须有）
路径：`deliver/issue_list.md`

issue 最小字段：
- `severity: blocker|warn|info`
- `where: normalize|assemble|verify|source`
- `what`
- `evidence`（若相关）
- `action`（建议修复方式）

## Z) 失败处理与回滚（必须，写死）

### Z.1 issue_list 出现 blocker 后怎么走？
若 `deliver/issue_list.md` 中存在 `severity=blocker`：
1. Verify 结论必须为 **NOT_PASS**
2. PM/Planner 必须选择以下之一并记录 decision（不可跳过）：
   - `request_rework`：打回到相关 task，创建新 Revision `r+1`
   - `terminate_branch`：若 blocker 来源于某个 Team/分支，则 terminate 该 Team
   - `major_restart`：若 blocker 表示需求/方向错误或用户推翻 → 升级 Major（v+1）

### Z.2 回滚到 r1 还是继续修？
规则：
- **不覆盖旧 revision**：永远创建新 `revs/r(N+1)/`
- “回滚”只是一种读取视角（把 current 指向旧 rev），但最终修复仍应产生新 rev
- 由 PM/Planner 决策（必要时回问用户）

### Z.3 Team 失败怎么处理？
- Team 内连续两次 NOT_PASS 或成本超预算 → PM/Planner 可 terminate_team
- 被 terminate 的 Team 产物进入 archived 只读，供复盘与证据引用（不得删）

### Z.4 自动降级策略（允许，但必须可审计）
当资源耗尽（token/tool_calls/时间）：
- 允许从 `full` 降级为 `summary-only` 交付
- 允许降低 top_k 或关闭向量，仅保留 BM25
但必须：
- 在 decisions 中记录 `degrade_reason` 与影响范围
- issue_list 记录“降级导致的缺口”


## 7) 会议模式的注入点（可选，但建议）
- 当出现 fork/blocker/Verify 不通过，可触发会议模式做“快速收敛”。
- 会议产物（decision/action_items）写回 decisions 与 TaskSpec，避免靠主编脑补。
- 详见：`docs/06_meeting_mode.md`。


## 3.x 会议产物作为上游输入

- 会议（06_meeting_mode.md）产出的 whiteboard/decision/action_items 可作为 TaskSpec.master 或 TaskSpec.patch 的输入。
- 会议 transcript 默认不进入执行上下文，仅用于审计/引用验证（按需检索）。


---

## 补充：Gate Node 与流水线的关系

- Gate 不产出大段正文，它产出 **decision + TaskSpec 更新**。
- 触发条件（写死）：分叉影响交付 / Verifier blocker / 口径冲突 / 高风险定稿 / 用户大版本打回。
- 执行模式：并行提交 position_packet → Moderator 单写 gate_decision。

详见：`docs/07_convergence_gates.md`。

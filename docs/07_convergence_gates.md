# 07 收敛门 Convergence Gate：把“会议式收敛”嵌入任务流水线（不牺牲吞吐）

> 目的：任务系统保持“并行产出 + 合并收敛”的高吞吐；仅在必要时引入“会议式回合”作为 **收敛门（Gate Node）** 解决分歧与口径一致。

---

## 7.1 结论判定（强约束）

- **不建议**把整个任务流改成会议式轮次（会显著降低吞吐）。
- **推荐**把会议抽象为一种通用节点：**Convergence Gate（收敛门）**，在少数关键点触发，用极短回合完成裁决，再回到并行生产。

---

## 7.2 三类节点（统一到同一条 TaskSpec 语义）

任务流水线只需要三类节点即可覆盖绝大多数场景：

1) **Work Node（默认）**  
- 目标：并行生产内容（每个 Agent 写自己的文件，互不冲突）。  
- 产物：Summary / Full / 可选 Cards（按触发规则）。

2) **Gate Node（收敛门 / 微型会议）**  
- 目标：裁决分歧、锁定口径、更新计划（不是产出大段正文）。  
- 产物：`gate_decision.md`（单写者：Moderator/Editor/Planner）  
- 约束：最多 1~2 轮，输入极少，输出极硬。

3) **Review Node（Verifier/QA）**  
- 目标：质量与一致性检查；失败触发返工或 Gate。  
- 产物：`issue_list.md` / `verification_report.md`

> 说明：会议模式（06_meeting_mode.md）是一种“泛化的 Gate Node”，适用于复杂、开放、争议大的场景；但在任务流水线中默认使用“微型 Gate”。

---

## 7.3 Gate Node 的输入/输出契约（最小可执行）

### 输入（程序下发，不靠 Agent 自由扩写）
- `focus`: 当前必须收敛的问题（一个句子）
- `options`: 允许的选项列表（A/B/C...）
- `must_answer`: 受影响的 must-answer 列表（引用其锚点）
- `evidence_refs`: 允许引用的证据范围（文件路径 + 锚点）
- `constraints`: 时间/成本/合规/风格等硬约束
- `round_budget`: 轮次与长度上限（默认 1 轮；必要时 2 轮）

### 输出（唯一产物：gate_decision）
`gate_decision.md` 必须包含：

- `decision`: 选项（choose_a / choose_b / keep_parallel / defer_to_user）
- `rationale`: 3~5 条理由（必须可追溯到 evidence_refs）
- `updates`: 对 TaskSpec 的更新（新增/修改 must-answer、约束、分工、并行度）
- `action_items`: 下一步要补的证据/任务（生成新的 Work Nodes 或返工）
- `dissent`: 可选异议记录（用于审计与后续复盘）

---

## 7.4 Gate Node 的“发言串行 + 思考并行”但不拖慢

### 7.4.1 “投递包”代替自由群聊
Gate 不做自由聊天，参与者仅提交短包（每人一个文件）：

- `position_packet_<agent>.md`（很短）
  - 立场（支持 A/B/并行）
  - 关键证据锚点（只引用，不复制长文）
  - 风险点（最多 3 条）
  - 需要的裁决点（如有）

Moderator 只读这些短包 + 证据锚点，然后单写 `gate_decision.md`。

### 7.4.2 回合限制（防“会议化拖慢”）
- 默认 **1 轮**：所有人并行投递 position_packet → Moderator 裁决  
- 仅在证据不足时进入 **第 2 轮**：补证据/反驳（仍是短包）  
- 超过 2 轮必须升级为完整会议模式（06_meeting_mode.md）

---

## 7.5 何时触发 Gate（程序规则：写死，避免模型分裂）

以下任一条件满足即触发 Gate Node：

1) **Fork 影响交付**：出现方向性分叉，且会影响成本/时间/交付物结构  
2) **Verifier blocker**：Review Node 给出 `severity=blocker`  
3) **口径冲突**：同一 must-answer 下出现互斥 claim（可用本地检索判定）  
4) **高风险输出进入定稿**：合同/财务/对外材料在“final 前”必须过 Gate  
5) **用户打回（大版本）**：用户否决方向，需要重新确立口径（见 05_user_interaction.md）

---

## 7.6 与“并行产出 + 合并器”的关系（关键回答）

- **并行产出**负责“生产内容”
- **Gate**负责“裁决与更新计划”
- **合并器**负责“组装成 IR/交付物”
- **Verifier**负责“质量与一致性兜底”

> 直观理解：Gate 是控制面；Work+Merge 是数据面。控制面少用但必须硬，数据面高吞吐。

---

## 7.7 目录与写权限（避免并行写入冲突）

- position_packet：各 Agent 写各自文件（并行安全）
- gate_decision：**只允许 Moderator 单写**
- TaskSpec 更新：由程序写入（或由 Moderator 写“patch 提案”，程序应用）

推荐路径：

- `runs/<run_id>/gate/<gate_id>/position_packets/position_packet_<agent>.md`
- `runs/<run_id>/gate/<gate_id>/gate_decision.md`

---

## 7.8 示例：报告+PPT 中的 Gate 放在哪里（最小例）

- Gate#1（早期）：确认报告结构与 PPT 口径是否一致（锁定 must-answer 与 outline）
- Work：并行撰写报告章节、并行准备 PPT 节点素材
- Review：检查事实引用与覆盖率
- Gate#2（定稿前）：对外口径/数据一致性裁决
- Merge/Transform：组装报告 → 生成 PPT IR → 渲染


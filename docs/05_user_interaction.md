# 05 用户中途指令处理（硬规则）

> 目标：不中断并行执行的前提下，将用户指令“吸收进系统”，并且可审计、可回滚、可裁决。

---

## 1) 指令分类（必须先分类）
用户消息进入系统后，PM/Planner 必须在写入日志前完成分类（不可跳过）：

| 类型 | 典型例子 | 对正在跑的 MPU/Team 的影响 | 默认动作 |
|---|---|---|---|
| 补充信息 | “我们用 MySQL 5.7” | 不改变目标，仅补充约束 | 写入 `spec.md#constraints`，广播相关 Team |
| 询问进度 | “现在到哪了？” | 无 | 返回 status（从 progress/manifest 推导） |
| 追加需求 | “顺便支持导出 PDF” | 可能改变交付物/范围 | 进入 `change_request` 流程（见 §3） |
| 方向调整 | “移动端先不做了” | 会废弃部分工作 | 触发 `replan`（见 §4） |
| 方案选择 | “就用方案 B” | 砍掉分支 | 触发 `terminate_team`（见 §5） |
| 推翻重来 | “全部作废，重做一版” | 新 Major | 触发 `major_restart`（见 §6） |
| 质量/格式要求 | “PPT 必须 10 页以内” | 改验收与产出格式 | 更新 TaskSpec/transformer 约束 |

---

## 2) 统一写入：interaction_log（必须）
任何用户消息必须写入（文件或 DB）并带上以下字段（缺一不可）：

- `interaction_id`
- `timestamp`
- `raw_user_message`
- `classified_type`
- `pm_understanding`（PM/Planner 对意图的单句复述）
- `action_taken`（触发了哪些系统动作）
- `affected_major`（vN）
- `affected_tasks`（task_id 列表）
- `affected_teams`（team_id 列表）

---

## 3) change_request（追加需求）的硬流程
触发条件：classified_type = 追加需求

流程：
1. PM/Planner 生成 `change_impact`（必须包含：新增交付物/改动范围/新增 must-answer/预计新增 MPU）
2. 若影响满足任一：`deliverables_changed=true` 或 `scope_changed=true` → 必须升级 Major（见 §6）
3. 否则：仅产生新 task（Revision 内追加），并写入 `spec.md#delta`

硬规则：
- 不允许“直接塞进正在写的段落”而不记录变更。
- 变更后必须重新跑 Verify（coverage_map/issue_list）。

---

## 4) replan（方向调整）的硬流程
触发条件：classified_type = 方向调整

流程：
1. PM/Planner 评估已有产物可复用性（可复用：保留；不可复用：deprecated）
2. 更新 `spec.md#constraints/#deliverables/#must_answer`（delta 方式记录）
3. 对受影响的 tasks：
   - 若仅变更输出格式 → 更新 transformer/adapter 约束，保持 task
   - 若目标/结论方向变更 → 创建新 task，旧 task 标记 deprecated（Revision 内）

---

## 5) terminate_team（用户选方案/砍分支）
触发条件：classified_type = 方案选择

规则：
- `teams` 并行上限受 `max_parallel_teams` 控制（默认 3）
- 被砍 team 必须：
  - 标记 `terminated`
  - 保留已产出（summary/cost/notes）进入 archived（只读）
  - 释放配额回资源池（scheduler 做）

---

## 6) major_restart（用户推翻重来）
触发条件：classified_type = 推翻重来 或 change_request 导致 scope/deliverables 变化

规则（写死）：
- 创建新目录 `fs/cases/{case_id}/versions/v(N+1)/`
- 复制上一版的 `spec.md` 到新版本，作为历史基线，新增一节 `# restart_reason`
- 新版本必须重新生成所有 TaskSpec（旧 TaskSpec 不复用，只作为参考）
- 更新 `fs/cases/{case_id}/current.yaml` 指向 `v(N+1)`

---

## 7) 何时必须“回问用户”（不要模型自由发挥）
若满足任一，PM/Planner 必须回问用户并暂停新任务派发：
- 用户指令自相矛盾（例如：既要“只要方案A”又要“方案B细化”）
- 影响范围不清（无法判断是否要升级 Major）
- 验收标准缺失（必须回答的问题 must-answer 无法列出）

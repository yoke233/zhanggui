# PRD - Phase 2 最小 Lead 自动化（Polling + Cursor）

版本：v1.0  
状态：Draft  
负责人：PM / Tech Lead / Platform  
目标阶段：Phase 2

## 1. 背景与问题

Phase 1 已证明流程可跑通，但仍依赖人工盯盘与手工写回。  
Phase 2 的目标是引入“最小自动化”，把重复动作系统化，同时保持协议不变。

## 2. 目标与非目标

目标：

- 每个角色有常驻 Lead（或等价常驻控制单元）。
- 自动订阅可处理 Issue，自动做规范化写回与 worker 派工。
- 支持 worker 切换并保证幂等（`active_run_id` 语义）。

非目标：

- 不做 webhook 强依赖（可先轮询）。
- 不做复杂 PR 语义解析。
- 不更改 Phase 1 的协议字段。

## 3. 用户与场景

用户：

- PM：关注阶段产能、阻塞与 SLA。
- Lead：从“手工协调者”转为“自动化运营者”。
- Worker：继续专注执行，减少手工沟通。

场景：

- 角色并发增长（backend 4、frontend 3 等）。
- 同时存在多个活动 Issue，需要自动分配与去重。

## 4. 范围（In Scope）

- 轮询 + cursor 增量消费事件。
- 基于 labels/assignee/depends-on 的候选 Issue 筛选。
- 自动 spawn worker 并分配 `run_id`。
- 仅接受 `active_run_id` 结果写回。
- 失败/阻塞自动写回 `blocked` 与 Next 建议。

## 5. 功能需求（PRD 级）

- FR-2-01：Lead 必须记录消费 cursor，支持重启恢复。
- FR-2-02：Lead 必须按 `IssueRef` 维持唯一 active run。
- FR-2-03：迟到结果（`run_id != active_run_id`）不得自动覆盖状态。
- FR-2-04：Lead 必须继续作为结构化事实单写者。
- FR-2-05：当 DependsOn 未满足时，自动进入 blocked 语义并停止当前 worker。

## 6. 核心流程

1. Lead 拉取增量 Issue/Event。
2. 过滤可处理任务（role 路由 + assignee + 依赖条件）。
3. 生成 WorkOrder 与 `run_id`，spawn worker。
4. 接收 WorkResult（JSON 或可解析文本）。
5. 判定 active run 后 Normalize 写回 Comment。
6. 需要切换 worker 时生成新 `run_id` 并接管。

## 7. 验收标准（DoD）

- AC-2-01：至少 1 个角色可持续自动处理 Issue 队列（无人工轮询）。
- AC-2-02：系统可正确丢弃过期 run 结果（幂等验证通过）。
- AC-2-03：自动写回的结构化 comment 字段完整。
- AC-2-04：worker 切换后责任归属不混乱（assignee 不抖动）。
- AC-2-05：重启后可从 cursor 续跑，不重复处理已消费事件。

## 8. 成功指标

- 指标 1：人工路由动作减少 >= 50%。
- 指标 2：重复处理/状态回滚事件数趋近 0。
- 指标 3：从 Issue open 到首次有效执行的中位时长下降 >= 30%。

## 9. 风险与缓解

- 风险：轮询延迟影响响应。  
  缓解：设置可配置轮询周期，后续 Phase 3 再接 webhook。

- 风险：不同 worker 输出格式不一致。  
  缓解：执行器协议允许文本降级，但锚点字段必须强校验。

- 风险：自动化误操作推进状态。  
  缓解：`needs-human` 与审批闸门保持最高优先级。

## 10. 依赖

- `docs/operating-model/executor-protocol.md`
- `docs/workflow/lead-worker.md`
- `docs/workflow/issue-protocol.md`
- `docs/workflow/workflow-profile.md`

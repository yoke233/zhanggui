# PRD - Phase 1 人工闭环（Local-First）

版本：v1.0  
状态：Draft  
负责人：PM / Lead / Integrator  
目标阶段：Phase 1

## 1. 背景与问题

在没有自动化调度和平台集成的前提下，团队仍需要尽快开始真实交付。  
Phase 1 的核心是“先跑通一次完整闭环”，而不是追求自动化完美。

## 2. 目标与非目标

目标：

- 在本地或 forge 环境下完成 Issue 从创建到关闭的端到端闭环。
- 固化最小事实链：IssueRef -> Changes(PR/commit) -> Tests/Review Evidence -> Close。
- 让 PM 能用同一套模板追踪交付进度和责任归属。

非目标：

- 不实现常驻 Lead 进程。
- 不实现 webhook、PR/CI 自动回填。
- 不追求全量状态标签自动维护。

## 3. 用户与场景

用户：

- PM：发起任务、定义验收标准、检查闭环证据。
- Lead：领取任务、派工、规范化写回。
- Worker：执行变更并回传证据。
- Reviewer：给出质量结论。
- Integrator：收敛与关闭。

场景：

- 单仓项目先行试点（git + sqlite）。
- 多仓项目按同一模板执行，但先人工路由。

## 4. 范围（In Scope）

- 使用固定模板创建 Issue 与 Comment。
- Claim 真源为 `assignee`。
- Worker 产出 commit/PR + tests evidence。
- Lead/Integrator 单写者回填结构化 comment。
- 完成 close 并可回放。

## 5. 功能需求（PRD 级）

- FR-1-01：Issue 必须包含可执行 Goal 与 Acceptance Criteria。
- FR-1-02：开工前必须满足 Hard 条件：
  - Issue open
  - assignee 已设置
  - 无 `needs-human`
  - DependsOn 已满足
- FR-1-03：Changes 必须包含 PR 或 commit 至少一个。
- FR-1-04：Tests 必须显式给出 `pass|fail|n/a` 与证据。
- FR-1-05：关闭前必须有至少一条结构化回填 comment。

## 6. 流程（最小闭环）

1. PM/Lead 创建 Issue（模板化）。
2. Lead claim（设置 assignee）。
3. Worker 执行并回传证据。
4. Lead 规范化写回 Comment（IssueRef/Changes/Tests/Next）。
5. Reviewer 给判定（forge 读 PR review；本地写结构化 comment）。
6. Integrator 关闭 Issue。

## 7. 验收标准（DoD）

- AC-1-01：至少完成 1 个真实任务从 open 到 close。
- AC-1-02：该任务的 Issue 时间线可完整回放。
- AC-1-03：Issue 内可找到 Changes 与 Tests Evidence。
- AC-1-04：责任人（assignee）与 Next 指派清晰。
- AC-1-05：`state:*` 缺失不会阻塞开工，但不影响闭环成立。

## 8. 成功指标

- 指标 1：首次试点任务闭环成功率 >= 90%。
- 指标 2：人工沟通往返次数较基线下降（目标 30%）。
- 指标 3：每个已关闭 Issue 都包含结构化证据回填。

## 9. 风险与缓解

- 风险：Worker 输出格式不稳定。  
  缓解：坚持 Lead 单写者规范化回填。

- 风险：本地模式缺乏平台级认证。  
  缓解：Phase 1 接受约定式信任；进入多人协作后迁移 forge/服务端 DB。

- 风险：团队误把 Soft 标签当硬门槛。  
  缓解：在 PRD 与流程图中明确 Hard/Soft 边界。

## 10. 依赖

- `docs/operating-model/START-HERE.md`
- `docs/operating-model/local-first.md`
- `docs/workflow/templates/issue.md`
- `docs/workflow/templates/comment.md`

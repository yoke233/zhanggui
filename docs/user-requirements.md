# 用户需求文档（简版）：Zhanggui 协作控制平面（Issue/Outbox 驱动）

版本：v0.1  
状态：Draft（由 `docs/operating-model/`、`docs/workflow/`、`docs/prd/` 汇总整理）  
适用阶段：Phase 1-3（路线图见 `docs/operating-model/phases.md` 与 `docs/prd/phases/`）  
读者：希望快速理解“这个系统解决什么问题、怎么用、边界是什么”的评审/协作者  

---

## 0. 一句话摘要

Zhanggui 的目标是把“多角色 + 多智能体 + 多仓库”的研发协作，收敛到一个可回放、可审计、可逐步自动化的闭环：  
用 **Issue 作为协作真源线程（Outbox）**，用 **`workflow.toml` 作为唯一配置真源**，用 **固定模板 Comment 记录 Evidence 与 Next**，在本地（仅 `git + sqlite`）也能开工并完成交付闭环。

---

## 1. 背景与问题

在多 subagent/多人并行开发时，团队常见痛点是“协作与一致性”，而不是单点编码能力：

- 规格散落在聊天/文档/PR 描述里，导致多份版本并存（双真源、甚至多真源）。
- 并行推进时缺少稳定的“队列 + 责任归属 + 阻塞管理”，容易漏单、抢单、重复劳动。
- worker（人/脚本/LLM）输出形态不统一，导致证据回填不可计算、不可审计。
- 没有统一的质量判定入口：Reviewer/QA 的结论经常停留在口头或散落在不同线程里，难以复盘。
- 过早引入重型自动化会放大复杂度与失败面，反而阻碍真实交付启动。

---

## 2. 目标与非目标

目标（我们希望系统稳定满足的“用户需求”）：

- 需求能被写成可执行、可验收、可追溯的 Spec，并且只有一个真源位置（需求层真源）。
- 交付能形成最小事实链：`IssueRef -> Changes(PR/commit) -> Tests/Review Evidence -> Close`（交付层真源）。
- 质量判定可计算、可审计：Reviewer/QA 只做判定并附证据，不派工；判定能回填到协作真源线程（质量层真源）。
- 先人工闭环，再逐步自动化：Phase 1 就能“今天开工交付”；Phase 2/3 再增强自动化。
- 支持本地/离线启动：最小依赖是 `git + sqlite`，不要求一开始接 GitHub/GitLab。

非目标（系统明确不做，避免误解）：

- 不替代 GitHub/GitLab 的 PR review 界面与平台级权限体系（在 forge 模式下以平台事实为真源）。
- 不在 Phase 1 强制实现 webhook/CI 自动回填、复杂调度、复杂审批策略。
- 不引入“第二条协作主线”：不使用另一个 task_id/单独系统来当协作真源线程。

---

## 3. 核心原则（评审请重点看）

### 3.1 三层真源（3-Layer Truth）

- 需求层（Product Truth）：Spec 真源在 Issue 的 Spec 区块，或 repo 内 `spec.md` 并在 Issue 里用 `SpecRef` 链接。见 `docs/operating-model/product-truth.md`。
- 交付层（Delivery Control）：协作真源是 Issue（Outbox），代码真源是 PR/commit，配置真源是 `<outbox_repo>/workflow.toml`。见 `docs/operating-model/delivery-control.md`。
- 质量层（Quality Gate）：Reviewer/QA 的判定必须可审计（PR review/CI，或本地模式下写入结构化 comment）。见 `docs/operating-model/quality-gate.md`。

### 3.2 单一真源（Single Source of Truth）

- 配置真源：只保留 `<outbox_repo>/workflow.toml` 这一份。见 `docs/workflow/v1.1.md`、`docs/workflow/workflow-profile.md`。
- Claim 真源：Outbox backend 的 `assignee` 字段（而不是 `/claim` 文本）。见 `docs/workflow/issue-protocol.md`。
- 结构化写回真源：固定模板 comment（`docs/workflow/templates/comment.md`），避免自由格式漂移。

### 3.3 单写者（Single Writer）

- Worker 的输出允许不规范，但必须可追溯（至少能提供锚点与证据）。
- Lead/Integrator/Recorder 负责把“原始素材”规范化为结构化 comment 写回 Outbox，减少格式分叉风险。见 `docs/workflow/lead-worker.md`。
- `state:*` 状态推进建议由单一协调者写回（推荐 `lead-integrator`），尤其在并行 Review + QA 时。见 `docs/workflow/guardrails.md`、`docs/workflow/integrator-role.md`。

### 3.4 幂等与可切换执行器（run_id / active run）

- 协作主键是 `IssueRef`，执行主键是 `run_id`（每次 spawn 都生成新的 `run_id`）。
- 必须丢弃过期结果：`run_id != active_run_id` 的迟到结果不应自动推进 Outbox 事实。见 `docs/operating-model/executor-protocol.md`。

---

## 4. 目标用户与角色（谁需要什么）

| 角色 | 主要诉求 | 主要产物/写回 | 不做什么（边界） |
| --- | --- | --- | --- |
| BA（需求分析） | 把输入写成可执行 Spec，避免口头需求漂移 | Issue Spec 区块（AC/Out of Scope/Risks） | 不派工、不决定实现细节 |
| PM/PO（产品 Owner） | 裁决“做不做/先做什么/验收口径” | 决策写回 Issue，必要时走 `/accept` | 不直接指挥 Worker |
| Lead（各角色负责人） | 队列路由、派工、阻塞管理、证据收敛 | 结构化 comment（Changes/Tests/Next） | 不要求“自己写完所有实现” |
| Worker（执行器：人/LLM/脚本/CLI） | 专注实现与产证据，减少沟通开销 | PR/commit + Tests evidence（回传给 Lead） | 不写 Outbox 的结构化事实、不写共享记忆 |
| Reviewer（代码审查） | 只做判定与风险识别，输出可审计证据 | `review:approved`/`review:changes_requested` + evidence | 不派活 |
| QA（测试/验收） | 给出验收判定与证据，区分功能失败与 flaky | `qa:pass`/`qa:fail` + evidence | 不推进全局状态机 |
| Integrator（收敛者） | 最终合并与集成验收，单写者推进 `state:*` | FixList vN、merge/验收证据、done/close | 不替代 Worker 写实现 |
| Recorder（记录者，可选） | 把线程变成“可回放状态机”，减少二次沟通 | 滚动 Summary、补齐缺失字段提示 | 不裁决、不写实现 |

说明：本表是“职责边界”视角；更详细定义见 `docs/workflow/roles-and-flow.md`。

---

## 5. 核心对象与协议（系统对外的“接口”）

### 5.1 Outbox / Issue / Event

- Issue：协作总线对象，一条可追加事件的线程。
- Event：Issue 下的追加记录（append-only），用于回放发生过什么。
- Outbox backend：Issue 的承载系统可切换（SQLite / GitHub / GitLab / DB），协议只依赖最小能力集合。见 `docs/operating-model/outbox-backends.md`。

### 5.2 IssueRef（协作主键）

V1 统一格式（必须是 canonical string）：

- GitHub：`<owner>/<repo>#<number>`
- GitLab：`<group>/<project>#<iid>`
- SQLite：`local#<issue_id>`

### 5.3 run_id（执行主键）

推荐格式：`<YYYY-MM-DD>-<role>-<seq>`，例如 `2026-02-14-backend-0001`。  
用于切换 worker、去重、幂等写回。见 `docs/operating-model/executor-protocol.md`。

### 5.4 workflow.toml（唯一配置真源）

`<outbox_repo>/workflow.toml` 声明：

- Outbox backend（sqlite/github/gitlab/...）与位置
- Approvers（Accepted Gate）
- 启用的 roles、每个 role 对应的 repo（多 repo 映射）
- groups 并发上限、监听标签、写回权限模型（owner/subscriber, full/comment-only）
- flow 规则（claim before work、依赖自动 block/unblock 等）

见 `docs/workflow/workflow-profile.md` 与 `docs/workflow/v1.1.md`。

### 5.5 固定模板（mailbox）

模板基线：

- Issue 主帖模板：`docs/workflow/templates/issue.md`
- Comment 模板：`docs/workflow/templates/comment.md`
- PR 描述模板（推荐）：`docs/workflow/templates/pr.md`

---

## 6. 典型使用流程（Phase 1：最小闭环）

本节描述“今天就能跑通”的最小流程（对应 `docs/operating-model/START-HERE.md`）。

1. 创建 Issue（协作真源线程）
2. 在 Issue 主帖写清 Goal 与 Acceptance Criteria，并设置至少一个 `to:<role>` 路由标签
3. Claim：将 Issue `assignee` 设置为责任人身份（事实源）
4. 满足开工 Hard 条件后启动执行（手工或工具）
5. Worker 输出 Changes 与 Tests Evidence（PR/commit + 命令/CI/日志）
6. Lead/Integrator 用固定 comment 模板写回结构化事实（IssueRef/RunId/Changes/Tests/Next）
7. Reviewer/QA 给出判定与证据
8. Integrator 收敛：必要时汇总 FixList vN；通过后写回 done 并关闭 Issue

本地/离线模式（仅 `git + sqlite`）的差异点：

- Changes 可用 `Commit: git:<sha>` 代替 PR URL。
- Reviewer 判定通过 Outbox comment 落盘（至少写 `review:approved` 或 `review:changes_requested`）。
见 `docs/operating-model/local-first.md`、`docs/operating-model/quality-gate.md`。

---

## 7. 需求清单（对系统的能力要求，简化版）

### 7.1 必须具备（Phase 1 硬需求）

1. 支持 Issue 作为协作总线：可创建、追加事件、设置 labels、设置 assignee、close（backend 可为 SQLite 或 forge）。
2. `IssueRef` 必须是跨 backend 的 canonical string，并且在所有证据与写回中一致出现。
3. 开工 Hard 条件可被检查：Issue open、assignee 已设置、无 `needs-human`、DependsOn 已满足。
4. 固定模板写回（comment）必须覆盖最小事实链：Changes、Tests、Next。
5. 质量判定必须可审计：forge 模式读取 PR review/CI；本地模式写入结构化 comment 承接。

### 7.2 自动化增强（Phase 2/2.x 需求）

1. 常驻 Lead 自动化：轮询 + cursor 增量消费事件，重启可恢复。见 `docs/prd/phases/phase-2-prd.md`。
2. 幂等与切换 worker：维护 `active_run_id`，丢弃过期结果。见 `docs/operating-model/executor-protocol.md`。
3. 操作面（TUI）：提供队列、详情、claim/spawn/switch/writeback/close 等最小闭环操作。见 `docs/prd/phases/phase-2-1-prd.md`。
4. 评审队列调度：将 reviewer 角色纳入调度链路，并可自动回流 changes requested。见 `docs/prd/phases/phase-2-5-prd.md`。
5. 同仓并发隔离：`run_id <-> workdir` 一对一映射，默认 git worktree。见 `docs/prd/phases/phase-2-6-prd.md`。
6. 真实 worker 接入：wrapper 归一化 `work_result.json|txt`，并产出审计信息。见 `docs/prd/phases/phase-2-7-prd.md`。
7. 并行审查协调者模型：assignee 固定为协调者；reviewer/qa 以 subscriber/comment-only 并行产出判定；状态机仍单写者推进。见 `docs/prd/phases/phase-2-8-prd.md`。

### 7.3 质量自动化（Phase 3 需求）

1. 自动读取 PR review 与 CI checks，并写回 Outbox 质量证据。见 `docs/prd/phases/phase-3-prd.md`。
2. changes_requested / CI fail 自动回流到责任角色，减少人工搬运。
3. 去重与聚合：同一事件不重复写回造成噪声。

---

## 8. 路线图（Phase 概览，便于评审）

- Phase 0：文档与真源收敛（固定术语、主键、模板与阶段边界）。见 `docs/prd/phases/phase-0-prd.md`。
- Phase 1：人工闭环（Local-First），最小依赖跑通端到端。见 `docs/prd/phases/phase-1-prd.md`。
- Phase 2：最小 Lead 自动化（polling + cursor + run_id 幂等）。见 `docs/prd/phases/phase-2-prd.md`。
- Phase 2.1：Lead 控制台（Bubble Tea TUI）。见 `docs/prd/phases/phase-2-1-prd.md`。
- Phase 2.5：Reviewer-Lead 调度增强。见 `docs/prd/phases/phase-2-5-prd.md`。
- Phase 2.6：同仓并发隔离（Git worktree）。见 `docs/prd/phases/phase-2-6-prd.md`。
- Phase 2.7：真实 Worker 接入与 Wrapper 归一化。见 `docs/prd/phases/phase-2-7-prd.md`。
- Phase 2.8：并行审查协调者模型（Assignee 固定 + 单写者，workflow.toml v2）。见 `docs/prd/phases/phase-2-8-prd.md`。
- Phase 3：质量与 PR/CI 自动化增强。见 `docs/prd/phases/phase-3-prd.md`。

文档中维护的“当前进度”见 `docs/prd/phases/README.md`。

---

## 9. 成功指标与埋点（建议口径）

为了支撑后续 Phase 4/5 的规模化治理，建议从 Phase 1 起保留最小可用埋点模型：

- 事件主键：`issue_ref` + `run_id` + `role`
- 事件类型：issue_created、issue_claimed、work_started、work_result_received、blocked、review_recorded、merged、issue_closed 等
- 原因码：dep_unresolved、test_failed、ci_failed、review_changes_requested、output_unparseable、stale_run 等

首批看板指标建议：

- `T_first_claim`：创建到 claim 的中位时长
- `T_first_execution`：创建到首次开工的中位时长
- `T_cycle`：创建到关闭的中位时长
- `R_blocked`：进入 blocked 的比例
- `R_stale_run`：过期 run 被丢弃的占比

详见 `docs/prd/metrics/phase-1-instrumentation.md`。

---

## 10. 非功能要求（简版）

- 可审计：所有关键事实必须能在 Outbox 线程回放（append-only events）。
- 可迁移：Outbox backend 可替换（SQLite -> GitHub/GitLab/DB），协议语义不变。
- 可恢复：自动化 Lead 必须有 cursor，重启不重复处理、不丢事件。
- 安全：Outbox 视为长期可回放记录，默认不写入密钥/隐私；敏感项目应使用 private repo 并最小权限。
- 兼容本地：本地模式接受“约定式信任”，但进入多人协作与合规阶段应迁移到 forge/服务端 DB。

---

## 11. 评审希望你重点反馈的问题

1. 三层真源划分是否清晰，是否会在真实团队里减少“指挥链混乱”？
2. `IssueRef` 与 `run_id` 的双主键设计是否足够支撑幂等与切换 worker？
3. 单写者规则在并行 Review + QA 场景下是否可行？哪里会卡住产能？
4. 本地模式（git + sqlite）是否真的能作为“启动路径”，还是会误导团队？
5. `workflow.toml` 作为唯一配置真源的约束是否过强？哪些项目会被阻挡？

---

## 12. 参考入口（进一步阅读）

- 快速启动清单：`docs/operating-model/START-HERE.md`
- 本地启动：`docs/operating-model/local-first.md`
- 协议抽象与主键：`docs/operating-model/outbox-backends.md`
- Worker 执行协议：`docs/operating-model/executor-protocol.md`
- 三层模型总览：`docs/operating-model/README.md`
- Issue 协作协议：`docs/workflow/issue-protocol.md`
- labels 与状态机：`docs/workflow/label-catalog.md`
- Lead/Worker 边界：`docs/workflow/lead-worker.md`
- Integrator 收敛：`docs/workflow/integrator-role.md`
- 护栏（并行审查/幂等/并发）：`docs/workflow/guardrails.md`
- 阶段 PRD 索引：`docs/prd/phases/README.md`


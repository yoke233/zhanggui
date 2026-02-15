# 常见问题与护栏（并行审查 + 多 Worker + 幂等）

目标：把高频“坑位”写成明确护栏，避免实现与协作在不同人手里分叉。

本文件不引入新的真源对象，仍遵守：

- 协作真源：Issue（Outbox）
- 交付真源：PR/commit + 可审计证据
- 配置真源：`<outbox_repo>/workflow.toml`

适用阶段：Phase 2+（Phase 1 人工模式也建议参考）。

## 1) 并行 Review + QA 的坑

### 1.1 版本漂移（Reviewer 与 QA 看的是不同代码）

症状：

- Reviewer 给了 `review:approved`，QA 也通过，但两者对应的 PR head 已经变化。
- 合并后出现“明明都通过了，为何还炸”的争议。

护栏（建议写死为规则）：

- 任何审查结论必须绑定一个可追溯的代码版本（建议=PR 的 `head_sha`）。
- 当 PR head 变化（push 新提交）后，旧结论必须视为 `stale`，不得作为合并闸门证据。

落盘方式（不改模板即可做）：

- 在 Outbox comment 的 `Changes` 中同时写：
  - `PR: <url>`
  - `Commit: git:<head_sha>`

### 1.2 结论互相覆盖（谁推进 state）

症状：

- Reviewer/QA 都在 Outbox 写状态迁移，互相把 issue 推进/打回。

护栏：

- `state:*` 的推进只允许一个角色写（推荐：`integrator-lead` 单写者）。
- Reviewer/QA 只产出“判定事实 + 证据”，不直接推进状态机。

### 1.3 都打回时的综合处理（避免多头指挥）

症状：

- Reviewer 写一堆规范问题，QA 写一堆失败用例，worker 不知道先改啥，出现反复试错。

护栏：

- `integrator-lead` 必须把各方反馈汇总成唯一的 `FixList vN`，作为下一轮执行的唯一指令。
- worker 只按 FixList 执行，不要求从多条评论里“自己综合”。

建议 FixList 的最小结构（写在 comment 的 `Summary` 或 `Notes` 中即可）：

```text
FixList vN (for git:<head_sha>):
1) MUST: <item> (owner: <role>) (acceptance: <condition>)
2) MUST: <item> ...
3) SHOULD: <item> ...
```

### 1.4 Flaky 与基础设施失败

症状：

- CI/测试不稳定导致频繁打回，返工噪音极高。

护栏：

- 将 QA 失败分为两类：`functional`（功能/验收失败）与 `infra/flaky`（环境/偶发）。
- `infra/flaky` 默认不打回到 worker 大改，而是进入“重跑策略 + 证据归档 + 阈值触发 needs-human”。

### 1.5 重跑策略失控（无限 rerun）

护栏：

- 为 CI/用例重跑设置 `max_rerun` 阈值；超过阈值必须 `needs-human`。
- 重跑必须记录原因与证据（否则会变成“赌运气”）。

## 2) Issue 的 assignee 与 to:* 的关系

核心语义：

- `to:<role>`：路由意图（谁应该看、进入谁的队列；可多个）
- `assignee`：责任归属事实（谁在协调/对结果负责；建议单一）

并行审查如何处理：

- PR 进入审查阶段时：
  - `assignee` 指向“协调者”（通常是 `integrator-lead` 或对应 role lead）
  - 同时打 `to:qa` 与 `to:reviewer`（或等价角色）以并行启动两条线

冲突优先级建议：

- 是否允许开工：以 `assignee` 为准（未 claim 不开工）。
- 谁应关注：以 `to:*` 为准（订阅队列）。
- 状态推进：以“单写者规则”为准（见 1.2）。

## 3) PR/CI 的并行闸门（建议）

并行闸门建议采用 AND 语义：

- Reviewer：`review:approved`（绑定 `git:<head_sha>`）
- QA：用例通过 + 证据链接（同样绑定 `git:<head_sha>`）
- CI：checks pass（或明确无 CI 的替代证据）

任一失败：

- 不允许合并
- 进入 FixList 汇总（见 1.3）

## 4) 同仓并发的工作目录隔离（worktree）

风险：

- 多个 worker 同时改同一 repo，必然互相污染工作区。

护栏：

- `run_id <-> workdir` 一对一，禁止复用旧 run 的工作目录。
- 清理必须安全：只允许在白名单根目录下删除；脏目录默认不删，转 `needs-human`。

对应阶段：

- 该能力在 PRD 中定义为 Phase 2.6（见 `docs/prd/phases/phase-2-6-prd.md`）。

## 5) 真实 Worker 接入：JSON/Text 归一化

风险：

- 不同 worker 输出格式不一致，Lead 无法稳定写回。
- 解析 stdout/stderr 容易误判。

护栏：

- 结构化结果只接受两种来源：
  1. `work_result.json`
  2. `work_result.txt`（Header + 空行 + Body）
- stdout/stderr 只作为归档与调试，不作为结构化事实来源（缺锚点直接 `needs-human`）。
- 必填锚点缺失时不推进状态：`IssueRef`、`RunId`、`Status`、`PR/Commit`、`Tests`。

对应阶段：

- 该能力在 PRD 中定义为 Phase 2.7（见 `docs/prd/phases/phase-2-7-prd.md`）。

## 6) 权限与身份（容易在真实项目里踩坑）

常见问题：

- GitHub/GitLab 无法 assign（claim 真源失效）
- 无法读取 PR checks/review（质量闸门不可计算）

护栏：

- Phase 1/2 可以在本地 sqlite 模式先跑通，但进入多人协作前必须把最低权限需求写清楚。
- 任何“需要平台强审计”的项目，应尽早迁移到有认证/审计能力的 backend（GitHub/GitLab 或服务端 DB）。

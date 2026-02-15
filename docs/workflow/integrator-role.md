# Integrator 角色说明（收敛、合并、集成验收）

本文件定义 **Integrator（集成者）** 在本项目工作流中的职责、触发条件、可执行动作与边界，目标是让：

- 多角色并行（backend/frontend/qa/architect）最终能在 **同一条 Issue 线程**上收敛为“可合并、可验收、可关闭”的结论。
- `state:*` 状态推进不再多头写入，避免“互相覆盖”“互相打回”“无人收敛”的分叉。
- 在 **本地 sqlite**（Phase 1）与 **GitHub/GitLab**（forge）两种承载下，都能得到一致的语义。

关联文档（本文件不重复造轮子，引用其细节作为权威）：

- `docs/workflow/lead-worker.md`：Lead/Worker 边界（推荐：`lead-integrator` 单写者）
- `docs/workflow/v1.1.md`：PR（forge）闭环、merge 策略与退化路径
- `docs/workflow/guardrails.md`：并行 Review + QA 的护栏（FixList vN、head_sha 绑定等）
- `docs/workflow/label-catalog.md`：标签集合、监听矩阵与状态迁移
- `docs/workflow/issue-protocol.md`：Issue 协作协议（claim、blocked、依赖、写回模板）
- `docs/workflow/approval-policy.md`：Approver/Accepted Gate（Integrator 不自动拥有盖章权）
- `docs/operating-model/quality-gate.md`：Reviewer/QA 的“判定”语义与证据要求
- `docs/operating-model/executor-protocol.md`：Worker 可插拔执行协议（RunId、切换 worker 幂等）

---

## 1. 术语与定位

### 1.1 Integrator 是谁

- **Integrator（role）**：项目启用的一类角色（`integrator`），由 `<outbox_repo>/workflow.toml` 的 `roles.enabled` 决定，并由 `role_repo.integrator` 决定其默认执行 repo。
- **Integrator Lead（actor）**：该角色的常驻负责人（推荐实现：独立进程或独立 session），负责协调多个 worker，并作为 Outbox 的“单写者”写回结构化事实。

命名约定：

- 本仓库现行建议采用 `lead-<role>` 的 actor 命名（例如 `lead-integrator`、`lead-reviewer`、`lead-qa`），并以 `workflow.toml` 中的实际配置为准。

> 本文件中的 “Integrator 做 X” 默认指 “Integrator Lead（通常为 `lead-integrator`）负责协调与写回”，而不是让 integrator 自己去写所有实现代码。

### 1.2 Integrator 的核心职责（一句话）

Integrator = **最终收敛点**：负责 `state:review` 阶段的合并与集成验收、统一写回证据、统一推进 `state:*`，并在 Reviewer/QA 并行打回时产出唯一 `FixList vN` 再派回。

---

## 2. Integrator 监听与触发（什么时候进入队列）

### 2.1 默认监听矩阵（建议 AND 语义）

在 `workflow.toml` 的典型配置中：

- `groups.integrator.listen_labels = ["to:integrator", "state:review"]`

含义：

- 只有当某个 Issue 同时具备 `to:integrator` 与 `state:review`，才进入 integrator 队列。
- 额外路由信号仍然成立：被 `@mention` 或被 assign（assignee）同样应当被处理（见 `docs/workflow/label-catalog.md`、`docs/workflow/issue-protocol.md`）。

### 2.2 Integrator 常见被叫起的场景

- Worker/role lead 已产出 PR/commit 证据，并将 Issue 推进到 `state:review`，希望 integrator 做最终合并与集成验收。
- Reviewer/QA 并行给出结论后，需要一个角色把“多条反馈”收敛为“下一步可执行指令（FixList vN）”。
- 多 repo 变更（contracts + backend + frontend）需要明确合并顺序并跑跨 repo 验收。

---

## 3. Integrator 能做什么（动作集合）

### 3.1 协调与归属（assignee 与 to:*）

**语义原则**（见 `docs/workflow/guardrails.md`）：

- `to:*`：路由意图，可多选（并行关注/并行处理）。
- `assignee`：责任归属事实，建议单一（“谁对当前阶段交付闭环负责”）。

Integrator 在 `state:review` 阶段的推荐做法：

- 若需要明确“谁在收敛”：将 Issue 的 `assignee` 设置为 `lead-integrator`（或项目约定的 integrator 身份）。
- 同时保留必要的 `to:*` 以并行启动/维持关注：
  - 例：`to:qa`（验收）、`to:backend`（实现修复）、`to:architect`（contracts/决策）、`to:integrator`（收敛）

> 注意：Claim 的真源是 `assignee`（见 `docs/workflow/v1.1.md`）。但 “谁能执行集成验收/写回” 不应该只由 assignee 限制；应以“单写者规则 + 权限”来保证一致性。

实现提示（Phase 2.5 -> Phase 2.8 演进）：

- Phase 2.5（当前常见实现）：为避免多头写回，Lead 通常要求 `assignee == lead-<role>` 才会处理 Issue，因此 review 阶段可能会出现 assignee 在多个 lead 间交接（例如 `lead-backend -> lead-reviewer -> lead-integrator`）。
- Phase 2.8（目标模型）：引入“订阅者模式（subscriber / comment-only）”，允许 reviewer/qa 在 `assignee = lead-integrator` 的前提下并行产出判定与证据，但不推进 `state:*`，由协调者统一收敛与推进。

### 3.2 状态推进（state:* 单写者）

Integrator 的关键能力之一是 **推进状态机** 并保持可计算：

- 推荐规则：`state:*` 的推进只允许一个角色写（推荐 `lead-integrator` 单写者）。见 `docs/workflow/guardrails.md`。
- 当发现 `state:*` 冲突（同一 Issue 同时存在多个 `state:*`）时：Integrator/Lead 负责 normalize（见 `docs/workflow/label-catalog.md`）。

### 3.3 合并（merge）与多 repo 收敛

#### 主策略（推荐 A）

见 `docs/workflow/v1.1.md`：

- Integrator 在满足合并条件后执行 merge（最终收敛点）。
- 多 repo 时，Integrator 同样负责按依赖顺序收敛（典型：`contracts -> backend -> frontend`）。

#### 退化路径（权限/策略限制）

如果 integrator 无法在某个 repo 执行 merge（权限或仓库策略）：

- 对应 repo 的 role lead 可以代为 merge；
- 但 integrator 仍负责：
  - 集成验收（拉齐指定版本跑验证）
  - Outbox 的状态迁移（`review -> done`）与证据回填

### 3.4 集成验收（构建/测试/E2E）

Integrator 的“验收”不是凭感觉判断，而是要产出可审计证据：

- 构建、测试、E2E（或项目定义的验收动作）
- 失败：写明失败类型（功能 vs infra/flaky）与最小复现/证据，并阻塞或打回
- 通过：写明证据，并进入 `state:done`（或请求 Approver close）

证据写回建议使用 comment 模板：`docs/workflow/templates/comment.md`。

### 3.5 并行 Reviewer + QA 都打回时的收敛（FixList vN）

见 `docs/workflow/guardrails.md`：

- Reviewer 与 QA 允许并行，但如果都打回（或反馈分散），worker 不应从多条评论中“自行综合”。
- 必须由 `lead-integrator` 汇总为唯一的 `FixList vN`，作为下一轮执行的唯一指令，并明确 `Next:` 指派。

建议最小格式（可直接写在 comment 的 `Summary` 或 `Notes`）：

```text
FixList vN (for git:<head_sha or merge_sha>):
1) MUST: <item> (owner: <role>) (acceptance: <condition>)
2) MUST: <item> ...
3) SHOULD: <item> ...
```

### 3.6 阻塞与解除阻塞（/unblock）

见 `docs/workflow/issue-protocol.md`、`docs/workflow/label-catalog.md`：

- 依赖未满足：进入 `state:blocked`，写清 `BlockedBy/DependsOn`，停止自动推进。
- 解除阻塞：
  - 若启用 `auto_unblock_when_dependency_closed = true`：依赖满足后自动恢复（assignee 存在则回 `doing`，否则回 `todo`）。
  - 否则由 integrator/lead 通过 `/unblock` 手动恢复（同上语义）。

### 3.7 标签与决策闸门（Accepted Gate）

Integrator 可以做的：

- 标记契约风险：例如 `contract:breaking`（建议由 architect/integrator 使用）。见 `docs/workflow/label-catalog.md`。
- 对 `decision:*` 冲突做 normalize（流程层面），但 **不自动拥有盖章权**。

Integrator 不能自动做的：

- 不得绕过 `docs/workflow/approval-policy.md` 的 Approver 集合去“自说自话 accepted”。
- Integrator 只有在被配置到 `<outbox_repo>/workflow.toml` 的 `[approval].approvers` 中时，才可执行 `/accept` 并使其生效。

### 3.8 写回与可审计性（Outbox 单写者）

Integrator Lead（通常为 `lead-integrator`）的写回应遵守：

- 结构化事实写回：使用固定 Comment 模板（`docs/workflow/templates/comment.md`）
- 重要字段必须可追溯：
  - `IssueRef`（协作主键）
  - `Changes`（PR/commit）
  - `Tests`（命令/结果/证据）
  - 必要时加 `RunId/Trigger` 做幂等（见 `docs/operating-model/executor-protocol.md`）

---

## 4. Integrator 不做什么（边界）

为避免角色混用，Integrator 的硬边界如下：

- 不替代 Worker 写实现代码（Integrator 可以派工/协调，但不要求“自己写完一切”）。
- 不替代 Reviewer/QA 的判定：
  - Reviewer/QA 的输出是“判定 + 证据”，Integrator 负责收敛、合并与推进状态，但不凭主观覆写判定。见 `docs/operating-model/quality-gate.md`。
- 不把“讨论/猜测”写入共享记忆当作事实：
  - 只有已 Accepted 的稳定结论才可进入共享记忆（见 `docs/workflow/lead-worker.md`、`docs/workflow/memory-layout.md`）。

---

## 5. Integrator Runbook（收到 `to:integrator + state:review` 后怎么做）

下面是一个建议的最小操作清单（Phase 1/2 都适用，区别只是“证据来源”不同）：

1) 归属与去重

- 确认该 Issue 是否已经有明确协调者（assignee）。
- 如果需要你来收敛：将 assignee 指向 `lead-integrator`，并声明 `ReadUpTo/Trigger` 防止重复处理。

2) 证据完整性检查（不满足就不推进状态）

- 是否有 `Changes`：PR 或 commit 至少一个。
- 是否有 `Tests`：至少一项证据或明确 `n/a`。
- 是否有 QA/Reviewer 的判定与证据（forge 模式优先以平台真源为准）。

3) 并行结论的收敛

- 若 Reviewer/QA 任一打回：进入 FixList 汇总（输出唯一 `FixList vN`），并明确 `Next:` 指派给对应 role/worker。
- 若存在版本漂移风险：要求所有结论绑定同一代码版本（例如 PR head_sha）。见 `docs/workflow/guardrails.md`。

4) 合并与集成验收

- 满足合并条件后执行 merge（或协调 role lead 代 merge）。
- 跑集成验收（build/test/e2e），输出证据。

5) 写回与收敛

- 使用 comment 模板写回最终证据与结论。
- 推进 `state:done`。
- 若配置要求 Approver close：@Approver 请求关闭（或你本身是 Approver 则直接 close）。

---

## 6. Phase 1（本地 sqlite）与 forge（GitHub/GitLab）差异点

### 6.1 本地 sqlite（无 PR/CI）

- `Changes` 用 `Commit: git:<sha>` 作为真源。
- Reviewer/QA 的判定需要以 Outbox comment 的结构化事件落盘（至少能写清 `review:approved/changes_requested`），并由 Lead/Integrator 规范化写回。见 `docs/operating-model/quality-gate.md`。

### 6.2 GitHub/GitLab（有 PR/Review/CI）

- Reviewer 的判定与 CI 的 pass/fail 优先以平台事件为真源。
- Issue 的作用是“协作与归档”，不替代 PR review 本身。见 `docs/operating-model/quality-gate.md`。

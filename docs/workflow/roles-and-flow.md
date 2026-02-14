# 角色与流程（多 subagent / 多 repo）

## 目标

- 并行推进：多个 subagent 在不同 repo/workdir 中直接改代码，减少互相踩踏。
- 一致性优先：把“接口/规格”集中成单一真源，避免分叉。
- 主 agent 轻量：主 agent 作为助理做统筹，不承担所有技术决策。

## 角色定义（建议）

### 1) 主 agent（Orchestrator / 助理）

只做流程统筹与信息路由，不充当强势技术领导。

职责：

- 保证每次并行都有一个可追踪的“主键”（V1 选用 Issue 的编号；backend 可为 GitHub/GitLab Issue 或本地 SQLite thread）
- 启动执行单元时明确 `repo_dir`（多 repo 时每个执行单元指向自己的 repo；contracts 相关任务指向 contracts repo）
- 确保任务描述中包含「当前 contracts 版本引用」（例如 `contracts@<sha|tag>`，或 issue 中约定的版本）
- 发现阻塞时触发 outbox（发 issue/消息），并把问题路由给合适角色
- 协调集成（Integrator）跑验收并回填结果

不做：

- 不随意改写 contracts/proto（除非由“架构师”角色兼任）
- 不在多个地方写不同版本的规格文档

补充说明（与 Lead/Worker 的关系）：

- 手动模式：主 agent 可以直接并行启动多个 worker（subagent）去实现/测试。
- Lead 模式：主 agent 更像 Supervisor，只负责拉起/停止各 role 的常驻 Lead；日常派工与回填由 Lead 完成（见 `docs/workflow/lead-worker.md`）。

### 2) 架构师（Architect / Contracts Owner）

职责：

- 维护 contracts repo（proto、buf 配置、生成策略、ADR/决策记录）
- 对外提供“权威接口契约”，并对 breaking change 做把关
- 对实现提出的“改规格请求”做裁决并落盘（合并 PR 或写 ADR）

强约束（推荐）：

- 单写者：contracts/proto 只由架构师合并（可通过 CODEOWNERS 强制）

### 3) 实现者（Workers：Frontend / Backend）

职责：

- 在各自 repo 中直接实现功能与修复问题
- 以 contracts repo 的 proto 为唯一接口真源
- 需要改接口时，通过 outbox/PR 提出变更请求（而不是在本地口头改）

不做：

- 不复制接口规格到自己的 repo（避免文档分叉）

### 4) 测试者（QA）

职责：

- 基于同一份 contracts 版本写集成/E2E 回归场景
- 发现不一致与边界问题时写入 outbox（并附上可复现实验/日志）

### 5) 集成者（Integrator）

职责：

- 拉齐 frontend/backend/contracts 的指定版本
- 运行构建、测试、E2E，输出可执行的验收结果
- 失败时把阻塞写入 outbox 并指派责任方

### 6) 记录者（Recorder / Summarizer，可选但强烈推荐）

Recorder 的价值在于把“线程”变成“可回放的状态机”，减少信息分散导致的二次沟通成本。

职责：

- 维护 Epic Issue（或 Outbox 线程）的滚动摘要（Summary）
- 把已 Accepted 的结论提取成事实列表（带 IssueRef/PRRef），避免被讨论淹没
- 提醒缺失字段并做轻量“质量门槛”：
  - 缺少 Hard 字段（Goal/Acceptance Criteria/Routing/Repo/Role；以及涉及接口时的 ContractsRef）：建议打 `needs-human` 要求补齐
  - 缺少 Soft 字段（SpecRef/DependsOn/Start Conditions/`state:*`）：建议仅评论提醒，不阻塞推进

边界：

- Recorder 不裁决接口（由 Approver/Architect 盖章）
- Recorder 不写实现（由 Backend/Frontend/QA workers 交付）

## 推荐流程（不强制）

1. 需求进入：在 Outbox repo（由 `workflow.toml` 指定）创建一个 Issue（GitHub Issue 或本地 SQLite thread），写“目标 + 验收标准 + contracts 引用/版本（如适用）”。
2. contracts 就绪：架构师确认 proto/版本；必要时先合入 contracts 变更。
3. 并行实现（两种模式）：
   手动模式：主 agent 分别对 frontend/backend/qa 启动 worker（subagent，各自 `repo_dir`）。
   Lead 模式：主 agent 拉起各 role 的 Lead；由 Lead 按 `groups.*.max_concurrent` 派生多个 worker 并行执行。
4. 异步沟通：任何阻塞/疑问通过 outbox 发布，感兴趣角色自行领取与回应。
5. 集成验收：Integrator 拉齐版本跑一遍，并回填结果到 issue/outbox。
6. 收敛关闭：完成后关闭 issue，并保留 contracts/ADR 作为最终依据。

V1 约定（本仓库当前决策）：

- 不使用 goclaw `task_id` 作为协作主线；以 Outbox repo 的 Issue（`IssueRef`）作为唯一协作真源（Outbox + 决策 + 证据）。
- `task_id` 可在后续阶段作为“执行镜像/统计面板”引入，但不应与 Issue 双真源并行。

## 动态角色与分组并发（项目可变）

- 角色是否存在、是否启用，不由系统写死，而由 `<outbox_repo>/.agents/workflow.toml` 的 `roles.enabled` 决定。
- 每个角色对应哪个 repo，由 `role_repo` 决定；后端-only 项目可以全部映射到 `main`。
- 角色组并发由 `groups.<name>.max_concurrent` 决定（项目软限制）。
- 运行时硬限制由 `agents.defaults.subagents.role_max_concurrent` 决定（系统上限）。
- 实际可用并发建议取两者较小值：`min(项目软限制, 系统硬限制)`。

## 最小一致性约束（必须做到）

- 接口真源只能有一个：contracts repo 的 proto。
- 任何接口决策必须落盘：PR/ADR/commit，而不是聊天记录。

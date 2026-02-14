# Executor Protocol (Worker 可插拔执行协议)

目标：允许 Worker 由不同的执行器实现（LLM subagent、Go 工具链、buf、前端工具链、脚本等），而不破坏协作真源与闭环。

本协议属于 **控制平面协议**：

- Lead/控制平面负责：注入上下文、路由执行、规范化写回 Outbox（单写者）、维护 cursor/幂等
- Worker/执行器负责：在 repo 内执行动作并产出可追溯证据（PR/commit/test/CI）

关键原则：

- Issue 里的“结构化事实”由 Lead 单写者写回（见 `docs/workflow/lead-worker.md`）
- Worker 的输出允许不规范，但必须可被规范化（至少提供锚点）

## 1) 核心抽象

### WorkUnit

一个可交付的工作单元。推荐与一个 Issue 绑定：

- `WorkUnitId`：稳定 ID（建议 = `IssueRef`）
- 真源线程：Issue

并行建议：

- 需要并行时，优先拆成子 Issue（多个 WorkUnit），而不是对同一个 Issue 同时跑多个 Worker。

### WorkRun (Attempt)

一次具体的执行尝试（spawn 一个 Worker）：

- `RunId`：每次 spawn 生成一个新 ID
- `Attempt`：可选整数递增（或用 `RunId` 即可）
- `LeaseTtl`：可选，声明“这个 run 预计占用多久”，用于超时/切换策略

`WorkUnit` 可有多个 `WorkRun`（用于切换 worker、失败重试、换执行器）。

#### RunId 格式（你选择：A）

为保证跨平台、可读、可用于文件名与 Outbox 去重，推荐 `RunId` 采用 ASCII、无空格格式：

```text
<YYYY-MM-DD>-<role>-<seq>
```

示例：

- `2026-02-14-backend-0001`
- `2026-02-14-qa-0002`

约束（控制平面必须保证）：

- `seq` 建议用 4 位零填充十进制（`0001` 开始递增）
- `seq` 建议按 `IssueRef`（WorkUnit）维度递增，而不是全局递增
- 同一个 `IssueRef` 同一时刻只能有一个 `active_run_id`
- 每次切换 worker / 重试 / 换执行器，都必须生成新的 `RunId`（不要复用旧值）

与 Outbox 的绑定（推荐）：

- 当 Lead 将本次 run 的结果规范化写回 Outbox 时，建议将 comment 模板中的 `Trigger` 设为：
  - `workrun:<RunId>`
  - 用于幂等去重（同一个 RunId 不应写回两次“结构化事实”）

### Executor Adapter（执行器适配器 / wrapper）

很多情况下 “Worker” 其实只是一个普通 CLI（`go test` / `buf` / `pnpm` / 自研脚本），它不会也不应该关心：

- `IssueRef` / `RunId`
- Outbox 写回格式
- 协作协议字段

因此本协议建议把“执行器”拆成两层：

- Tool CLI（不可控或不想改的工具）：输出自然语言日志 + exit code
- Executor Adapter（你可控的薄 wrapper）：读取 WorkOrder/Context Pack，运行 Tool CLI，并生成 WorkResult（JSON 或可解析文本）

结论：

- 不要求底层 CLI 输出它不关心/Lead 已知的元信息。
- 但要求 **Executor Adapter 最终产出** 可被控制平面消费的 WorkResult（用于幂等、切换 worker、规范化写回）。

## 2) 输入输出契约（WorkOrder / WorkResult）

本项目选择：**契约用 proto 定义**（可转 HTTP），但为了让不同 CLI 好接入，允许多种编码：

- Canonical schema: `.proto`
- 传输编码（建议优先级）：
  1. proto-json（最易被各种 CLI 处理）
  2. textproto（调试友好）
  3. binary protobuf（高效，但 CLI 生态不一定方便）

说明：

- “选择 proto”不等于强制每个 CLI 都链接 protobuf 库。
- Worker 可以只处理 proto-json；控制平面负责 schema 版本与兼容性。

### WorkOrder (Lead -> Worker)

最小字段（MVP）：

- `issue_ref`：IssueRef（必须）
  - 必须使用 canonical IssueRef 字符串（见 `docs/operating-model/outbox-backends.md`）
  - 不允许传 GitHub/GitLab 的内部 `id/node_id` 作为 `issue_ref`
- `run_id`：本次 WorkRun 的唯一 ID（必须）
- `role`：本次执行的角色（必须）
- `repo_dir`：执行目录（必须）
- `spec_ref` 或 `spec_snapshot`（二选一，建议至少有一个）
- `contracts_ref`：涉及接口时必须；不涉及可填 `none`
- `constraints`：硬约束（不可改路径、必须跑的命令、目标分支等）
- `depends_on`：已知依赖（可选）

### WorkResult (Worker -> Lead)

最小字段（MVP）：

- `issue_ref`（必须）
  - 必须回显 WorkOrder 中的 canonical IssueRef
- `run_id`（必须，回显 WorkOrder 的 run_id）
- `status`：`ok|fail|blocked`（必须）
- `changes`：PR URL 或 commit URL，至少一个（必须）
- `tests`：至少一项证据，或显式 `n/a`（必须）
- `summary`：一句话即可（必须）

可选字段（强烈建议）：

- `blocked_by`：依赖/阻塞项
- `questions`：需要 Lead/PM/Reviewer 回答的问题（不直接写 Outbox，由 Lead 规范化后写回）
- `artifacts`：日志/报告位置（文件路径或 URL）

### 编码降级：允许自然语言文本，但必须有锚点

现实里不是所有 Worker 都能“按 schema 输出 JSON”：

- 有的 worker 只是运行 `go test` / `buf` / `pnpm test` 的 CLI
- 有的 worker 是人类或脚本，只能回传一段自然语言描述

为了保持“可插拔”，V1 允许 Worker 回传 **文本结果**，但必须满足：

- 必须包含可解析的锚点（否则控制平面无法做幂等与闭环）
- 文本仅作为传输编码的降级，不改变 canonical schema（仍以 proto 定义 WorkResult）

推荐做法：

- Preferred（结构化）：写出 `work_result.json`（proto-json of WorkResult）
- Fallback（可解析文本 + 自然语言正文）：写出 `work_result.txt`
  - Header 部分：若干行 `Key: Value`（直到第一行空行结束）
  - Body 部分：任意自然语言（不解析，只做归档/调试）

硬规则（V1，便于接入各种工具链）：

- 底层 Tool CLI 的 stdout/stderr **不做任何结构化要求**（允许完全自然语言日志）。
- 所有结构化约束只作用于 Executor Adapter 生成的结果文件：`work_result.json` 或 `work_result.txt`。
  - Adapter 可以将 Tool CLI 的 stdout/stderr 原样附在 `work_result.txt` 的 Body 中用于归档与调试。

`work_result.txt` 的最小必填 Header Keys（MVP）：

- `IssueRef`：必须（例：`org/contracts#123` 或 `local#12`）
- `RunId`：必须（用于切换 worker/幂等；必须回显当前 active run）
- `Status`：必须（`ok|fail|blocked`）
- `PR` 或 `Commit`：至少一个（可追溯）
- `Tests`：必须（可以是 `n/a`，但必须显式写出）

示例：

```text
IssueRef: org/contracts#123
RunId: 2026-02-14-backend-0003
Status: ok
PR: https://github.com/org/backend/pull/45
Commit: none
Tests: go test ./... => pass
Evidence: https://github.com/org/backend/actions/runs/123456

Notes:
I updated the handler to use the new contracts. The existing integration test was flaky; I kept it
unchanged and added a deterministic unit test instead.
```

降级语义（必须遵守）：

- 若缺少 `RunId`：Lead 不应自动写回 Outbox（无法做 active run 去重）；应打 `needs-human` 或要求补齐锚点。
- 若缺少 `PR/Commit`：Lead 不应把状态推进到 `review/done`，最多写 `blocked` 并要求提供可追溯证据。

## 3) Context Pack（上下文注入包）

为了避免把上下文塞进命令行参数，推荐 Lead 为每次 WorkRun 生成一个目录（Context Pack），Worker 只需要读取该目录即可开工。

推荐布局：

```text
<context_pack>/
  work_order.json         # proto-json of WorkOrder (recommended)
  work_order.textproto    # optional
  work_result.json        # proto-json of WorkResult (preferred)
  work_result.txt         # fallback: parseable text envelope + natural language body
  spec_snapshot.md        # optional: Issue spec 的快照（防止线程过长或内容变更导致误读）
  links.md                # Issue/PR/CI/依赖链接汇总
  constraints.md          # 约束与 DoD（必须/禁止/推荐）
  manifest.txt            # 可选：建议关注的文件/目录清单
```

说明：

- Worker 执行器是 CLI 时，读文件比解析复杂参数更稳定。
- `spec_snapshot.md` 用于“执行时刻一致性”：避免 worker 在长线程里读到争议讨论。

## 4) Worker 切换（Switch Worker / Re-run）语义

你提出的“切换 worker”必须被协议化，否则会出现：

- 老 worker 迟到的结果覆盖新 worker 的结论
- 两个 worker 同时推进同一个 Issue，Outbox 出现互相打架的事实

### 4.1 事实源：assignee 属于 Lead，不属于 worker

推荐做法：

- Issue 的 assignee 表示“交付责任人”（通常是 role lead 或 integrator）
- Worker 是执行器实例，可能频繁切换，不应成为 Outbox claim 的事实源

这样切换 worker 时：

- 不需要反复 re-assign issue
- Outbox 的责任归属稳定，内部执行随时可换

### 4.2 幂等与去重：只接受当前 active run

Lead 必须维护“每个 WorkUnit 的当前 active run_id”。

规则（建议）：

- Lead spawn 新 Worker 时生成新 `run_id`，并将其设为 active
- Lead 收到 WorkResult 时：
  - `run_id != active_run_id`：视为过期结果，不自动写回 Outbox（可人工审阅）
  - `run_id == active_run_id`：进入 Normalize -> 写回 Outbox

这样可以避免“旧 worker 的迟到输出把状态冲回去”。

### 4.3 切换触发点（建议）

允许切换 worker 的常见原因：

- Worker 执行超时/卡死（超过 LeaseTtl 或人工判断）
- PR review `changes_requested` 且需要换更合适的执行器/人员
- 执行环境不匹配（例如需要 Linux 才能跑的测试）
- 需要不同工具链（Go -> buf -> 前端）

### 4.4 交接要求（handover）

为了让“换 worker”不丢进度，要求 worker 尽量把进度变成可接管的真源：

- 最推荐：尽早 push 分支并开 draft PR（哪怕未完成）
- 次推荐：至少 push commits（提供 commit hash）
- 最后手段：生成 patch/bundle 并作为 artifact 提供

Lead 在 Outbox 里写回的规范化事件应当包含：

- 旧 run 的引用（例如 `Trigger` 或 `Summary` 中标注）
- 新 run 的启动说明（为什么切换、下一步谁做）

## 5) 与 Outbox 的映射（单写者写回）

控制平面（Lead/Integrator）负责把 WorkResult 映射到 Outbox comment 模板：

- `docs/workflow/templates/comment.md`

并遵守：

- worker 不直接写 Outbox 结构化事实
- Outbox 中的结构化事件必须可回放、可审计（含 IssueRef/PR/Tests/Next）

## 6) Phase 1 如何落地（不写自动化也能跑）

Phase 1 可以“人肉模拟协议”：

- WorkOrder = Issue + Lead 的指令（可选手工生成 context pack）
- WorkResult = worker 把 PR 链接与测试结果发给 Lead
- Lead 按 comment 模板规范化写回 Outbox

你仍然获得了：

- 单一真源线程（Outbox）
- 可切换执行器（人/LLM/CLI 都行）
- 可审计闭环（PR/Review/CI 证据）

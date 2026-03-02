# P4 高级定制与扩展 Implementation Plan

> **For Agent:** REQUIRED SUB-SKILL: Use executing-wave-plans to implement this plan wave-by-wave.

**Goal:** 在不破坏 P0~P3 主链路稳定性的前提下，交付可配置模板体系、多通道通知、Reactions V2、MCP 扩展入口、P4 指令增强与治理能力补齐（priority/budget/trace/backup）。  
**Architecture:** P4 采用“先能力底座、后外部集成、最后治理收口”的分层推进：先完成模板与通知配置的内核改造，再在 Reactions 中引入 `notify/spawn_agent`，随后以 `spec-mcp` 扩展 Spec 上下文来源，收口 GitHub P4+ 指令与 Linear Tracker，最后补齐调度优先级、Token 预算、trace_id 贯通与备份策略。核心状态机（Pipeline/TaskPlan）保持不变，新增能力全部通过插件和配置装配。  
**Tech Stack:** Go 1.23+, chi, SQLite, EventBus, Slack Webhook API, Generic HTTP Webhook, React + Vitest。

---

## 0. Entry Preconditions (Hard Gate)

`P4-ENTRY` 作为 Wave 1 的外部门禁，必须全部满足：

- [ ] P3 已达到执行门禁结论：`Go`（或满足条件后转 `Go` 的 `Conditional Go`），且证据已落在 `docs/plans/`。
- [ ] P3 的 GitHub 主链路可用：`/run`、`/approve`、`/reject`、`/status`、`/abort` 冒烟验证通过并有日志证据。
- [ ] P3 Wave 2.5 硬化门禁已完成：queue / DLQ+replay / reconcile / trace / admin-ops / permission-probe 具备证据。
- [ ] P3 限流策略参数与 spec 一致：`github.rate_limit.rps=1`、`burst=5`、`retry_on_limit=true`（429/403 最多 3 次退避重试）。
- [ ] P3 文档已完成单实例边界收敛：不包含跨团队锚点/跨组依赖设计，且门禁证据齐全。
- [ ] 当前分支不存在未决的 P3 阻塞缺陷（Sev-1/Sev-2）。

## 1. Context and Scope

### In Scope
- 自定义模板（`custom_templates`）从配置层到执行层全链路落地（global/project/pipeline）。
- Notifier 多通道能力：`desktop` + `slack` + `webhook`，支持 fan-out 与失败隔离。
- Reactions V2：规则化匹配与动作扩展（`notify`、`spawn_agent`）。
- MCP 扩展入口：`spec-mcp` provider（作为 SpecPlugin 的一个实现）。
- GitHub P4+ 指令扩展：`/modify`、`/skip`、`/rerun`、`/switch`、`/pause`、`/resume`、`/logs`。
- `tracker-linear` 插件最小可用实现与工厂接入。
- 治理增强：任务优先级调度（P0/P1/P2）与优先级继承。
- 治理增强：Token 月度预算门禁与超限告警（`budget.monthly_token_limit`）。
- 治理增强：统一 `trace_id` 从 TaskPlan 贯穿到 Pipeline/Checkpoint/日志。
- 治理增强：SQLite 自动备份（`store.backup.interval`、`store.backup.path`）。

### Out of Scope
- 改写 Pipeline 核心状态机语义或引入分布式调度系统。
- 让 GitHub/Linear 成为调度真相源（本地仍是 source of truth）。
- 引入新的前端框架或重写 Workbench 页面结构。
- 一次性支持多种 MCP 传输协议全覆盖（P4 只做最小可用入口）。

### 关键约束
- 保持 `executing-wave-plans` 波次门禁：未达 `Go`（或满足条件的 `Conditional Go`）不得进入下一波。
- 新增能力必须可降级：配置关闭时行为回退到 P3 基线。
- 所有外部依赖（Slack/Webhook/MCP/Linear）失败时，不阻塞 Pipeline 主链路执行。

## 2. Dependency DAG Overview and Critical Path

```text
Wave 1 (模板底座)
  W1-T1 custom_templates 配置模型与合并
  W1-T2 模板解析与执行层接入
  W1-T3 模板目录 API/CLI 能见性

Wave 2 (通知底座)
  W2-T1 Notifier 配置与 fan-out 装配
  W2-T2 notifier-webhook 插件
  W2-T3 notifier-slack 插件

Wave 3 (Reactions V2)
  W3-T1 Reactions 规则配置模型
  W3-T2 `notify`/`spawn_agent` 动作执行器
  W3-T3 Reactions 与 Notifier/审计链路打通

Wave 4 (MCP 扩展)
  W4-T1 spec-mcp 插件与配置模型
  W4-T2 Agent 执行策略（tools/sandbox/approval）配置化
  W4-T3 CLI 校验与故障排查命令

Wave 5 (外部联动收口)
  W5-T1 GitHub P4+ slash commands
  W5-T2 `/logs` 查询与评论摘要
  W5-T3 tracker-linear 插件与工厂选择

Wave 6 (治理能力补齐)
  W6-T1 任务优先级调度 + 优先级继承
  W6-T2 Token 月度预算门禁 + 超限告警
  W6-T3 TaskPlan 级 trace_id 全链路贯通
  W6-T4 SQLite 自动备份与恢复演练命令
```

### Critical Path
- `P4-ENTRY -> W1-T1 -> W1-T2 -> W2-T1 -> W3-T2 -> W4-T1 -> W5-T1 -> W6-T1 -> W6-T2`

## 3. Wave Map

| Wave | 任务范围 | depends_on | 关键产出 | 文件 |
|---|---|---|---|---|
| Wave 1 | 模板体系底座 | P4-ENTRY | custom template 全链路可解析可执行 | [2026-03-01-p4-advanced-customization-wave1.md](2026-03-01-p4-advanced-customization-wave1.md) |
| Wave 2 | 通知插件与装配 | Wave 1 | 多通道 notifier 可并行发送且失败隔离 | [2026-03-01-p4-advanced-customization-wave2.md](2026-03-01-p4-advanced-customization-wave2.md) |
| Wave 3 | Reactions V2 | Wave 2 | 规则匹配 + notify/spawn_agent 动作可控 | [2026-03-01-p4-advanced-customization-wave3.md](2026-03-01-p4-advanced-customization-wave3.md) |
| Wave 4 | MCP 扩展入口 | Wave 3 | spec-mcp + agent 执行策略配置化 | [2026-03-01-p4-advanced-customization-wave4.md](2026-03-01-p4-advanced-customization-wave4.md) |
| Wave 5 | GitHub P4+ + Linear 收口 | Wave 4 | P4+ slash + logs + tracker-linear | [2026-03-01-p4-advanced-customization-wave5.md](2026-03-01-p4-advanced-customization-wave5.md) |
| Wave 6 | 治理能力补齐 | Wave 5 | 优先级调度 + 预算门禁 + trace_id + backup | [2026-03-01-p4-advanced-customization-wave6.md](2026-03-01-p4-advanced-customization-wave6.md) |

## 4. Global Quality Gates (F/Q/C/D)

### F - Functional
- [ ] `custom_templates` 在 API/Scheduler/Executor 三条创建路径行为一致。
- [ ] `desktop/slack/webhook` 可按配置并行通知，单通道失败不阻断主流程。
- [ ] Reactions V2 可执行 `retry/escalate_human/skip/abort/notify/spawn_agent`。
- [ ] GitHub P4+ slash 命令与本地 Pipeline action 语义一致。
- [ ] `tracker-linear` 可创建外部任务并同步状态（最小闭环）。
- [ ] DAG ready 队列按 `priority` 调度并支持“高优先级阻塞链继承提升”。
- [ ] Token 超预算时自动任务暂停，且可通过通知与状态页看到原因。
- [ ] 每条 TaskPlan 均有统一 `trace_id`，可跨 TaskItem/Pipeline/Checkpoint 检索。
- [ ] 数据库备份任务可按周期执行并支持一次恢复演练。

### Q - Quality
- [ ] 新增包单测覆盖核心路径 >= 80%。
- [ ] 至少一次 `go test -race`（核心包）无新增竞态（环境允许时）。
- [ ] 每个 Wave 至少 1 条 smoke/e2e 用例通过。

### C - Compatibility
- [ ] 不配置 P4 新字段时，行为与 P3 基线一致。
- [ ] 旧配置文件可加载（未知字段不导致启动崩溃）。
- [ ] Web UI 在未启用新能力时保持空态与兼容展示。
- [ ] `budget`/`store.backup` 新增配置均有安全默认值，不影响已有部署启动。

### D - Documentation
- [ ] `docs/spec` 与 `docs/plans` 的 P4 命令、模板、通知配置口径一致。
- [ ] 提供 Slack/Webhook/MCP/Linear 最小可运行配置样例。
- [ ] 故障排查文档可独立执行。

## 5. Per-Wave Output Links

- [Wave 1 — 模板体系底座](2026-03-01-p4-advanced-customization-wave1.md)
- [Wave 2 — 通知插件与装配](2026-03-01-p4-advanced-customization-wave2.md)
- [Wave 3 — Reactions V2](2026-03-01-p4-advanced-customization-wave3.md)
- [Wave 4 — MCP 扩展入口](2026-03-01-p4-advanced-customization-wave4.md)
- [Wave 5 — 外部联动收口](2026-03-01-p4-advanced-customization-wave5.md)
- [Wave 6 — 治理能力补齐](2026-03-01-p4-advanced-customization-wave6.md)

## 6. Task Dependency Completeness Rule

- 每个任务必须声明 `Depends on`，禁止隐式依赖。
- 依赖关系必须无环，跨 wave 依赖只能指向前序 wave。
- 外部依赖（如 `P4-ENTRY`）必须在主计划中定义，并且只能被 Wave 1 引用。
- 若同一 wave 内存在共享关键文件冲突（如 `factory.go`、`types.go`、`executor.go`），执行阶段必须串行化。

## 7. Full Regression Command Set

```powershell
# P4 入口门禁检查（P3 证据）
rg -n 'Go|Conditional Go|Wave Exit Gate|Wave 2.5|single-process|单实例|不在路线图|跨团队依赖协调' docs/spec/spec-overview.md docs/plans/2026-03-01-p3-github-integration.md docs/plans/p3-wave25-hardening.md docs/plans/p3-wave4-5-integration.md
rg -n 'rate_limit|1 req/s|burst 5|429/403|Retry-After|最多 3 次' docs/spec/spec-github-integration.md docs/spec/spec-api-config.md docs/plans/p3-wave25-hardening.md
rg -n '/run|/approve|/reject|/status|/abort' docs/spec/spec-github-integration.md docs/plans/2026-03-01-p3-github-integration.md

# 全量构建 + 测试
go build ./...
go test ./...

# 核心包竞态（环境允许时）
go test -race ./internal/engine ./internal/plugins/factory ./internal/web ./internal/secretary

# P4 目标包
go test ./internal/plugins/notifier-fanout ./internal/plugins/notifier-slack ./internal/plugins/notifier-webhook -count=1
go test ./internal/plugins/spec-mcp ./internal/plugins/tracker-linear -count=1
go test ./internal/github ./internal/engine ./internal/web -run 'Slash|Reaction|Logs|Template' -count=1

# 前端（若波次触达 web）
npm --prefix web test -- --run
npm --prefix web run build

# 文档一致性（P4 命令/配置口径）
rg -n '/modify|/skip|/rerun|/switch|/pause|/resume|/logs|custom_templates|notifier|tracker-linear|spec-mcp|priority|monthly_token_limit|trace_id|store.backup' docs/spec docs/plans
```

## 8. Test Policy

- 每个任务遵循 TDD：先失败测试，再最小实现，再回归。
- 每个 Wave 必须包含：
  - 至少 1 条 wave 级 smoke/e2e。
  - 至少 1 条边界触发 integration/contract 验证（按变更类型选择）。
- 所有外部依赖测试默认使用 fake server / mock client，不依赖真实网络。

## 9. Assumptions

- P3 门禁证据可从已有计划/执行记录中直接复用，不要求为 P4 新建重复报告。
- 当前环境可新增插件目录与配置字段，不要求对旧字段做强制迁移脚本。
- MCP 具体上游实现差异较大，P4 仅交付 `spec-mcp` 最小可用 provider 与校验链路。
- Wave 6 的治理能力在配置缺省下应保持“默认关闭或保守模式”，避免影响已有项目。
- `go test -race` 若受本地编译环境限制，可走“环境豁免 + 证据记录”流程。

## 10. Execution Handoff

- 当前会话：按 Wave 1 → Wave 6 顺序执行，每波输出门禁证据。
- 并行会话：仅允许在同一 wave 内、且无冲突文件的任务并行。
- 任意波次触发高风险回归时，必须回到上一波通过基线再继续推进。

# P3 GitHub 集成 Implementation Plan

> **For Agent:** REQUIRED SUB-SKILL: Use executing-wave-plans to implement this plan wave-by-wave.

**Goal:** 在不改变本地调度真相源（TaskPlan/TaskItem + Pipeline）的前提下，交付可开关、可降级、可回放的 GitHub 双向集成能力。  
**Architecture:** P3 通过 `tracker-github`、`scm-github`、Webhook 分发器和状态同步器把本地状态镜像到 GitHub。所有核心决策仍在本地执行：Pipeline 通过 `task_item_id` 关联 TaskItem，GitHub 只接收镜像和人工指令。异常路径采用 best-effort + no-op 降级，保证 GitHub 故障不阻塞执行主链路。  
**Tech Stack:** Go 1.23+, chi, go-github/v68, ghinstallation/v2, SQLite, EventBus, React + Vitest。

## 0. Entry Preconditions

- 启动本计划前，必须先通过前置计划的 [P3 Entry Checklist](archive/2026-03-01-p3-prerequisites-entry-checklist.md)。
- 仅当 checklist 的 `Status=Ready`，才允许进入本计划 Wave 1（`gh-1`）。
- 若 checklist 为 `Not Ready`，需先修复对应门禁失败项并重新记录证据。

---

## 1. Context and Scope

### In Scope
- GitHub 客户端、认证、Webhook 接入与事件分发。
- `tracker-github`：TaskItem 状态与 GitHub Issue 的双向镜像。
- `scm-github`：PR 生命周期（创建、更新、Ready、Merge、Close）。
- Issue 触发 Pipeline、斜杠命令控制、Pipeline 到 Issue 状态回写。
- 离线降级与重连同步、CLI 配置校验、端到端集成测试、Web UI GitHub 状态展示。

### Out of Scope
- 改变 Secretary / DAG / Pipeline 的核心状态机语义。
- 引入 GitHub 作为调度真相源（禁止）。
- 强制启用 `review-github-pr`（该能力保持可选，不阻塞 P3 Done）。
- 不在 P3 引入 `/pause`、`/resume`、`/skip`、`/rerun`、`/switch`、`/logs` 命令（维持 P4+ 范围）。
- 多代码托管平台（GitLab/Bitbucket）统一抽象。

### 关键架构约束（与最新 spec 对齐）
- Pipeline 不包含“规格生成/规格审核”阶段，P3 不得引入旧阶段命名。
- Pipeline 阶段基线顺序保持 `requirements -> worktree_setup -> implement -> ...`，不得反转前两阶段。
- Pipeline 关联 TaskItem 统一使用 `task_item_id`，不复制 TaskItem 契约到 Pipeline 字段。
- Spec 仅作为 Secretary 上下文增强；GitHub 同步层不得承担 Spec 生命周期管理。

## 2. Dependency DAG Overview

```text
Wave 1 (基础设施)
  gh-1 GitHub 客户端 + 认证
  gh-2 配置模型 + 事件类型 + 工厂选择
  gh-3 Webhook 端点 + 签名验证 + 项目路由
  gh-4 GitHub 通用操作层 (Issue/Label/Comment/PR)

Wave 2 (插件实现)
  gh-5 tracker-github            depends: gh-2, gh-4
  gh-6 scm-github                depends: gh-2, gh-4
  gh-7 webhook dispatcher        depends: gh-3

Wave 2.5 (硬化门禁)
  gh-7a outbound queue + 限流      depends: gh-4, gh-7
  gh-7b webhook DLQ + replay       depends: gh-3, gh-7
  gh-7c reconcile 对账修复         depends: gh-7a, gh-7b
  gh-7d trace/log 贯通            depends: gh-7
  gh-7e 管理员逃生舱              depends: gh-7b, gh-7c
  gh-7f GitHub App 权限探测门禁    depends: gh-2, gh-4

Wave 3 (双向同步)
  gh-8 Issue -> Pipeline 触发     depends: gh-5, gh-7, gh-7a, gh-7b, gh-7d
  gh-9 Slash 命令 + ACL          depends: gh-7, gh-7d, gh-7f
  gh-10 Pipeline -> Issue 同步   depends: gh-5, gh-7, gh-7a, gh-7c, gh-7d
  gh-11 Draft PR 生命周期         depends: gh-6, gh-10, gh-7a

Wave 4 (稳态能力)
  gh-12 降级/重连/补偿同步         depends: gh-8, gh-10, gh-11, gh-7c
  gh-13 工厂注册 + CLI 集成       depends: gh-2, gh-6, gh-12, gh-7f

Wave 5 (集成收口)
  gh-14 review-github-pr (可选)    depends: gh-6, gh-9
  gh-15 端到端集成测试             depends: gh-12, gh-13
  gh-16 Web UI GitHub 状态         depends: gh-10, gh-11, gh-15
```

### Critical Path
- `gh-1 -> gh-4 -> gh-7 -> gh-7a -> gh-8 -> gh-10 -> gh-12 -> gh-15 -> gh-16`

## 3. Wave Map

| Wave | 任务范围 | depends_on | 产出 | 文件 |
|---|---|---|---|---|
| Wave 1 | gh-1~4 | [] | GitHub 基础设施可用，Webhook 可验签入站 | [p3-wave1-foundation.md](p3-wave1-foundation.md) |
| Wave 2 | gh-5~7 | Wave 1 | tracker/scm 插件与分发器落地 | [p3-wave2-plugins.md](p3-wave2-plugins.md) |
| Wave 2.5 | gh-7a~7f | Wave 2 | 可靠性硬化（队列、补偿、可观测、运维逃生） | [p3-wave25-hardening.md](p3-wave25-hardening.md) |
| Wave 3 | gh-8~11 | Wave 2.5 | 双向同步主链路闭环（Issue/Slash/PR） | [p3-wave3-sync.md](p3-wave3-sync.md) |
| Wave 4 | gh-12~13 | Wave 3 | 降级恢复 + 工厂集成 | [p3-wave4-5-integration.md](p3-wave4-5-integration.md) |
| Wave 5 | gh-14~16 | Wave 4 | 可选评审能力 + e2e + UI 收口 | [p3-wave4-5-integration.md](p3-wave4-5-integration.md) |

## 4. Global Quality Gates (F/Q/C/D)

### F - Functional
- [ ] `github.enabled=false` 时，行为与当前默认本地插件模式一致。
- [ ] Issue 可触发 Pipeline（自动或命令触发）且幂等（同 Issue 不重复建 Pipeline）。
- [ ] Slash 命令可控制 Pipeline：`/run [template]`、`/approve`、`/reject <stage> <reason>`、`/status`、`/abort`。
- [ ] Slash ACL 与 spec 一致：`author_association` 权限矩阵生效，`authorized_usernames` 可覆盖放行。
- [ ] Pipeline 阶段状态可回写 Issue（标签 + 评论），并使用新阶段命名（`requirements/worktree_setup/implement/code_review/fixup/e2e_test/merge/cleanup`）。

### Q - Quality
- [ ] 新增 GitHub 相关 Go 包单测覆盖核心路径 >= 80%。
- [ ] 关键并发点（同 Issue 串行、Webhook 幂等）有稳定测试。
- [ ] Webhook dispatcher 具备 per-issue mutex 延迟回收（Issue close/pipeline done 后 5 分钟）测试。
- [ ] 所有 GitHub 写操作统一经过 outbound queue，具备限流与重试退避。
- [ ] Webhook 失败事件进入 DLQ，支持 replay 并验证幂等。
- [ ] 无 `go test -race` 新增数据竞争。

### C - Compatibility
- [ ] 旧项目无 GitHub 配置可正常启动、执行、回归。
- [ ] 数据库变更向后兼容；旧记录读取不 panic。
- [ ] GitHub 配置字段与 `docs/spec` 一致（移除计划内未定义字段，避免 `plugins` 漂移）。
- [ ] Web UI 在无 GitHub 数据时显示为空态，不抛异常。

### D - Documentation
- [ ] `docs/spec` 与 `docs/plans` 的阶段命名和数据契约一致。
- [ ] 配置示例包含顶层 `github` 与项目级覆盖示例。
- [ ] Webhook 接入与故障排查文档可独立执行。
- [ ] 运行手册包含管理员逃生舱（force-ready / force-unblock / replay-delivery）操作说明。

## 5. Per-Wave Output Links

- [Wave 1 — 基础设施](p3-wave1-foundation.md)
- [Wave 2 — 插件实现](p3-wave2-plugins.md)
- [Wave 2.5 — 可靠性硬化门禁](p3-wave25-hardening.md)
- [Wave 3 — 双向同步](p3-wave3-sync.md)
- [Wave 4 + 5 — 集成收口](p3-wave4-5-integration.md)

## 6. Full Regression Command Set

```powershell
# Backend 全量
$env:GOFLAGS=''
go test ./...

# Backend 竞态
$env:GOFLAGS=''
go test -race ./internal/...

# Frontend 单测
npm --prefix web test -- --run

# Frontend 构建
npm --prefix web run build

# 针对 GitHub 模块（按需）
go test ./internal/github/...
go test ./internal/plugins/...
go test ./internal/web/... -run Webhook
```

## 7. Test Policy

- 每个任务遵循 TDD：先写失败测试，再最小实现，再回归。
- 每个 Wave 必须包含：
  - 至少 1 个 wave 级 smoke/e2e 用例。
  - 边界变更触发时的 integration/contract 验证。
- Wave 间门禁遵循 `executing-wave-plans`，不满足 `Go`（或满足条件的 `Conditional Go`）不得进入下一波。

## 8. Assumptions

- 当前仓库尚未落地 `internal/github/*` 目录，相关模块将在 P3 创建。
- 当前可用并行能力按 2 条独立子线估算：
  - 子线 A：客户端/插件/同步。
  - 子线 B：Webhook/命令解析/UI。
- `review-github-pr` 保持可选特性，默认不进入主执行路径。

## 9. Execution Handoff

- 当前会话执行：按 Wave 顺序落地，每波结束执行 Exit Gate。
- 并行会话执行：新会话使用 `executing-wave-plans`，按本计划 Gate 驱动推进。

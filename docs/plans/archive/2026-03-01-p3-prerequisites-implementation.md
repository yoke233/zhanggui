# P3 Prerequisites Implementation Plan

> **For Agent:** REQUIRED SUB-SKILL: Use executing-wave-plans to implement this plan wave-by-wave.

**Goal:** 在进入 P3 GitHub 集成前，完成“Spec 上提到 Plan 级、Pipeline 纯执行”的代码底座重构，消除 `spec_gen/spec_review` 旧语义残留。  
**Architecture:** 先清理执行引擎阶段定义与模板，再把 TaskPlan/TaskItem 契约结构化并打通存储，随后补齐 SpecPlugin + 配置 + 工厂装配，最后做 API/文档/测试一致性收口。整体采用“可破坏性重构 + 全量回归”策略，不保留旧阶段兼容桥。  
**Tech Stack:** Go 1.23+, SQLite, chi/websocket, text/template, React/Vitest（接口联调层）。

---

## 1. Context and Scope

### In Scope
- 删除 Pipeline 级 `spec_gen/spec_review` 执行路径（常量、模板、switch、PromptVars、默认行为）。
- 将 TaskItem 扩展为结构化契约（inputs/outputs/acceptance），并以 `task_item_id` 连接 Pipeline 与 TaskItem。
- 引入最小 SpecPlugin 合同（读取/上下文增强），配置迁移到顶层 `spec` 语义。
- SQLite schema 与 store 查询写入同步升级（task_plans/task_items/pipelines）。
- Secretary 输出解析、Factory bootstrap 预留、API 文档与测试收口。

### Out of Scope
- P3 GitHub 功能开发（tracker-github/scm-github/webhook/sync/PR 生命周期）。
- 新增第二个 Spec provider 的完整实现（仅保留接口与配置入口）。
- 保持旧阶段运行时兼容（本计划允许直接破坏旧路径）。

### 关键约束
- 不再允许 `spec_gen/spec_review` 出现在可执行阶段集合。
- Pipeline 不复制 TaskItem 契约字段，统一通过 `task_item_id` 反查 TaskItem。
- Spec 仅用于 Secretary 上下文增强，不属于 Pipeline cleanup 责任域。

### 破坏性切换策略
- 本计划默认 **Destructive Cutover**：允许删除旧代码路径、旧字段和旧数据，不保留运行时兼容窗口。
- 切换窗口安排在 **Wave 2 开始前**：先执行数据快照（可选），再清理/重建 `task_plans`、`task_items` 中无法满足新契约的数据。
- 切换后语义：仅保证新模型（结构化 TaskItem + task_item_id）与新阶段模型可用，不保证旧 `spec_gen/spec_review` 路径可回放。

## 2. Dependency DAG Overview and Critical Path

```text
Wave 1: 执行引擎阶段重构
  W1-T1 core stage/template cleanup
  W1-T2 executor/web/scheduler stage switch cleanup
  W1-T3 prompt vars/template cleanup

Wave 2: 计划契约数据模型 + 存储升级
  W2-T0 destructive cutover (legacy data purge/freeze)
  W2-T1 core.TaskPlan/TaskItem 扩展
  W2-T2 sqlite migration + store SQL 全链路升级
  W2-T3 secretary agent 输出解析映射升级

Wave 3: Spec 插件与配置装配
  W3-T1 core.SpecPlugin + plugin slot contract
  W3-T2 config 顶层 spec + merge/defaults/defaults.yaml 升级
  W3-T3 factory BootstrapSet + buildWithRegistry 预留 spec 装配

Wave 4: API/文档/回归收口
  W4-T1 pipeline/task API 与 handlers 对齐新契约
  W4-T2 docs/spec + docs/plans 同步（无旧阶段语义）
  W4-T3 全量回归、搜索门禁、进入 P3 验收
```

Critical Path:
`W1-T1 -> W1-T2 -> W1-T3 -> W2-T0 -> W2-T1 -> W2-T2 -> W3-T1 -> W3-T2 -> W3-T3 -> W4-T1 -> W4-T2 -> W4-T3`

## 3. Wave Map

| Wave | 内容 | depends_on | 产出文件 |
|---|---|---|---|
| Wave 1 | 清理旧阶段与执行路径 | [] | [2026-03-01-p3-prerequisites-wave1.md](2026-03-01-p3-prerequisites-wave1.md) |
| Wave 2 | 扩展契约模型与 SQLite/store | Wave 1 | [2026-03-01-p3-prerequisites-wave2.md](2026-03-01-p3-prerequisites-wave2.md) |
| Wave 3 | Spec 插件、配置、工厂预留 | Wave 2 | [2026-03-01-p3-prerequisites-wave3.md](2026-03-01-p3-prerequisites-wave3.md) |
| Wave 4 | API/文档/回归与 P3 入场 Gate | Wave 3 | [2026-03-01-p3-prerequisites-wave4.md](2026-03-01-p3-prerequisites-wave4.md) |

## 4. Global Quality Gates

### F - Functional
- [ ] `full/standard/quick/hotfix` 模板运行不再依赖 `spec_gen/spec_review`。
- [ ] requirements 阶段能基于 `task_item_id` 读取 TaskItem 结构化契约。
- [ ] Secretary 输出可稳定解析 `inputs/outputs/acceptance`。

### Q - Quality
- [ ] 关键改动路径单测通过（engine/config/store/secretary/web）。
- [ ] `go test ./...` 通过，且无新增 race（核心包至少一次 `-race`）。
- [ ] 搜索门禁通过：`spec_gen/spec_review/StageSpec*` 仅允许出现在“历史说明”文档语境。

### C - Compatibility
- [ ] 无 GitHub 配置、无 spec 配置的项目可正常启动并执行。
- [ ] SQLite 旧库自动迁移新增列，并完成 `task_items.pipeline_id -> pipelines.task_item_id` 回填，不丢失历史关联语义。

### D - Documentation
- [ ] `docs/spec` 与代码语义一致（Plan-level spec, Pipeline pure execution）。
- [ ] `docs/plans` 中 P3 主计划与前置计划边界清晰。

## 5. Per-Wave Output File Links

- [Wave 1 — Stage/Template Cleanup](2026-03-01-p3-prerequisites-wave1.md)
- [Wave 2 — Contract/Store Upgrade](2026-03-01-p3-prerequisites-wave2.md)
- [Wave 3 — Spec Plugin/Config/Factory](2026-03-01-p3-prerequisites-wave3.md)
- [Wave 4 — API/Docs/Regression Gate](2026-03-01-p3-prerequisites-wave4.md)
- [P3 Entry Checklist](2026-03-01-p3-prerequisites-entry-checklist.md)

## 5.1 P3 Entry Rule

- 进入 P3 主计划前，必须先更新并确认 [P3 Entry Checklist](2026-03-01-p3-prerequisites-entry-checklist.md)。
- 仅当 checklist `Status=Ready` 时允许启动 `2026-03-01-p3-github-integration.md` Wave 1。
- 若任一门禁失败，状态保持 `Not Ready`，并阻断进入 P3。

## 6. Full Regression Command Set

```powershell
# 全量编译与测试
go build ./...
go test ./...

# 核心包竞态回归
go test -race ./internal/engine ./internal/secretary ./internal/plugins/store-sqlite ./internal/web

# 前端（若 API schema 改动触达 web）
npm --prefix web test -- --run
npm --prefix web run build

# 语义残留搜索门禁（自动化判定）
rg -n 'StageSpecGen|StageSpecReview|spec_gen|spec_review' internal cmd configs docs/spec
rg -n 'StageSpecGen|StageSpecReview|spec_gen|spec_review' docs/plans -g '!2026-03-01-p3-prerequisites-*.md'
```

## 7. Test Policy

- 每个任务遵循 TDD 5 步（先失败测试，后最小实现）。
- 每个 Wave 至少包含一条 smoke/e2e 命令验证。
- 涉及边界变化（stage model、schema migration、config merge）时，必须执行对应 integration/contract 验证。
- Wave Gate 统一由 `executing-wave-plans` 决策，未达 `Go` 不进入下一波。

## 8. Assumptions

- 当前仓库允许一次性破坏旧阶段语义，不要求运行时兼容窗口。
- 现有未提交改动较多，执行时采用“按文件审阅 + 局部提交”避免互相覆盖。
- SpecPlugin 先以接口预留为主，默认实现可保持 noop/open-spec context reader。

## 9. Execution Handoff

1. 当前会话：按本计划 wave-by-wave 直接实施。  
2. 新会话并行：使用 `executing-wave-plans`，每波结束提交 Gate 证据后再推进。

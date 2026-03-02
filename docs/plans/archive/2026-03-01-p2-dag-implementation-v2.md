# P2 DAG 任务拆解计划 V2（Wave Gate 版）

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 在不扩大 P2 范围的前提下，重写为“可执行、可追踪、可阻断”的多 Wave 计划，并强制每个 Wave 完成提交、评审、修复、复测后再进入下一 Wave。  
**Architecture:** 延续原 P2 的 33 任务 DAG 与 7 个 Wave 分层，不改任务 ID 与依赖语义；新增 Wave 级质量闸门、跨波准入条件、可追踪工件要求。  
**Tech Stack:** Go（chi/websocket/sqlite/eventbus）、React/Vite/TypeScript/Tailwind/Zustand/React Flow。

---

## 1. Context

P0/P1 已完成并验收，P2 目标仍为：
- P2-Foundation：插件接口 + API 基础设施
- P2a：Secretary Agent + DAG Scheduler
- P2b：Multi-Agent ReviewPanel
- P2c：Workbench UI

本版本是原计划的执行增强版，聚焦两点：
1. 强化 Wave 级收口，不允许“最后统一 review/统一修复”。
2. 强化跨波准入，防止质量债跨波累积。

## 2. Scope / Non-Goal

### In Scope
- 保持原 33 任务及依赖框架。
- 增加每个 Wave 的交付工件、评审和验证门禁。
- 增加 Wave 间准入规则与阻断条件。

### Out of Scope
- 不新增 P3（GitHub 集成）范围。
- 不重定义 P2 各任务的业务目标与接口设计。
- 不在本文引入 P4 高级能力。

## 3. 核心执行规则（Hard Gate）

### 3.1 每个 Wave 的强制收口序列

每个 Wave 必须依次完成以下 5 步：
1. **提交代码**：至少 1 个可追踪 commit（或 PR）与该 Wave 任务绑定。  
2. **代码评审**：给出结论（`Approve` / `Request Changes`）。  
3. **修复问题**：评审提出的问题完成修复（High/Medium 清零或书面豁免）。  
4. **修复后验证**：运行本 Wave 规定命令并记录结果。  
5. **Wave 结论**：明确 `Go / Conditional Go / No-Go`。

### 3.2 跨波准入

- 仅当 `Wave N` 结论为 `Go`，或 `Conditional Go` 的前置条件全部满足时，允许进入 `Wave N+1`。
- `Wave N` 若为 `No-Go`，下一波不得启动。

### 3.3 必备证据工件

每个 Wave 必须沉淀以下证据：
- Commit/PR 列表（含任务 ID 对应关系）
- Review 记录（问题清单 + 严重级别 + 结论）
- 修复记录（问题 -> commit 映射）
- 验证记录（命令 + exit code + 关键结果）

## 4. DAG 总览（沿用原 33 任务 / 7 Waves）

### Wave 1（5）
- `fnd-1` 定义新插件接口  
- `fnd-7` REST API 服务骨架  
- `p2a-1` Secretary 领域实体  
- `p2b-2` 审核 Prompt 模板  
- `p2c-1` React 前端初始化

### Wave 2（12）
- `fnd-2` review-local  
- `fnd-3` tracker-local  
- `fnd-4` local-git SCM  
- `fnd-5` notifier desktop  
- `fnd-8` project/pipeline handlers  
- `fnd-9` websocket hub  
- `p2a-2` store 接口扩展  
- `p2a-3` SQLite 新表迁移  
- `p2a-5` DAG 数据结构 + 校验  
- `p2a-6` Secretary Agent 驱动  
- `p2b-1` ReviewRecord 实体 + 存储  
- `p2c-2` Zustand + API/WS client

### Wave 3（6）
- `fnd-6` 工厂 BootstrapSet 扩展  
- `p2a-4` SQLite Store 实现  
- `p2c-3` Chat View（Mock）  
- `p2c-4` Plan View（Mock）  
- `p2c-5` Board View（Mock）  
- `p2c-6` Pipeline View（Mock）

### Wave 4（3）
- `fnd-10` CLI server 集成  
- `p2a-7` DAG Scheduler  
- `p2b-3` ReviewPanel 引擎

### Wave 5（2）
- `p2a-8` TaskPlan 管理层  
- `p2b-4` review-ai-panel 插件

### Wave 6（4）
- `p2a-9` REST API chat/plans/tasks  
- `p2a-10` WebSocket Secretary 事件  
- `p2a-11` 执行期文件沉淀  
- `p2b-5` ReviewPanel 接入生命周期

### Wave 7（1）
- `p2c-7` 前端接入真实 API + embed.FS

## 5. Wave 执行模板（每波都必须复制）

```markdown
## Wave Exit Gate
- [ ] Code committed（commit/PR 链接）
- [ ] Review completed（Approve / Request Changes）
- [ ] Findings fixed（High/Medium 清零或书面豁免）
- [ ] Post-fix verification passed（命令 + 输出）
- [ ] Wave verdict：Go / Conditional Go / No-Go

## Next Wave Entry Condition
- 仅当当前 Wave 为 Go，或 Conditional Go 条件全部完成时，允许进入下一 Wave。
```

---

## 6. Wave-by-Wave 执行计划

## Wave 1：基础骨架落地

**目标**：先把 P2 基础接口、服务骨架与前端工程脚手架搭起来。  
**任务**：`fnd-1, fnd-7, p2a-1, p2b-2, p2c-1`

**建议验证命令**
- `go test ./internal/core/... -count=1`
- `go test ./internal/web/... -count=1`
- `go test ./internal/engine/... -count=1`
- `cd web && npm test`
- `cd web && npm run typecheck`

**推荐提交节奏**
- 提交 1：`fnd-1 + p2a-1`
- 提交 2：`fnd-7`
- 提交 3：`p2b-2 + p2c-1`
- 提交 4：Wave1 修复提交（只修 review 问题）

**Wave 1 Exit Gate**
- [ ] 所有提交可映射到任务 ID
- [ ] 至少 1 轮 review 已完成
- [ ] 修复提交已补齐
- [ ] 以上命令全部 exit code 0
- [ ] Verdict：`Go / Conditional Go / No-Go`

## Wave 2：插件实现 + DAG/Store 核心能力

**目标**：补齐基础插件和 Secretary 关键数据结构。  
**任务**：`fnd-2, fnd-3, fnd-4, fnd-5, fnd-8, fnd-9, p2a-2, p2a-3, p2a-5, p2a-6, p2b-1, p2c-2`

**建议验证命令**
- `go test ./internal/plugins/... -count=1`
- `go test ./internal/secretary/dag_test.go -count=1`
- `go test ./internal/web/... -count=1`
- `go test ./internal/config/... -count=1`
- `cd web && npm test`
- `cd web && npm run typecheck`

**推荐提交节奏**
- 提交 1：`fnd-2~fnd-5`
- 提交 2：`fnd-8 + fnd-9`
- 提交 3：`p2a-2 + p2a-3 + p2a-5 + p2a-6 + p2b-1`
- 提交 4：`p2c-2`
- 提交 5：Wave2 修复提交

**Wave 2 Exit Gate**
- [ ] DAG 校验测试通过（环/缺失/自依赖/约简边界）
- [ ] API + WS handler 测试通过
- [ ] Review 问题已修复并复测
- [ ] Verdict：`Go / Conditional Go / No-Go`

## Wave 3：实现层与前端 Mock 视图

**目标**：完成后端工厂整合与四个 UI 视图的 Mock 可用版本。  
**任务**：`fnd-6, p2a-4, p2c-3, p2c-4, p2c-5, p2c-6`

**建议验证命令**
- `go test ./internal/plugins/factory/... -count=1`
- `go test ./internal/plugins/store-sqlite/... -count=1`
- `cd web && npm test`
- `cd web && npm run typecheck`

**推荐提交节奏**
- 提交 1：`fnd-6 + p2a-4`
- 提交 2：`p2c-3 + p2c-4`
- 提交 3：`p2c-5 + p2c-6`
- 提交 4：Wave3 修复提交

**Wave 3 Exit Gate**
- [ ] 工厂可构建完整 BootstrapSet
- [ ] 四个视图可渲染 + 对应测试通过
- [ ] Review 闭环完成
- [ ] Verdict：`Go / Conditional Go / No-Go`

## Wave 4：核心编排引擎

**目标**：打通最关键的调度与审核引擎。  
**任务**：`fnd-10, p2a-7, p2b-3`

**建议验证命令**
- `go test ./internal/secretary/... -count=1`
- `go test ./cmd/ai-flow/... -count=1`
- `go test ./internal/web/... -count=1`

**推荐提交节奏**
- 提交 1：`p2a-7`
- 提交 2：`p2b-3`
- 提交 3：`fnd-10`
- 提交 4：Wave4 修复提交

**Wave 4 Exit Gate**
- [ ] DAG Scheduler 核心用例全绿（推进/失败策略/恢复）
- [ ] ReviewPanel 强门禁状态机用例全绿
- [ ] CLI server 命令可启动并优雅关闭
- [ ] Verdict：`Go / Conditional Go / No-Go`

## Wave 5：管理层 + ReviewGate 插件化

**目标**：将审核能力接入管理层生命周期。  
**任务**：`p2a-8, p2b-4`

**建议验证命令**
- `go test ./internal/secretary/manager_test.go -count=1`
- `go test ./internal/plugins/review-ai-panel/... -count=1`

**推荐提交节奏**
- 提交 1：`p2a-8`
- 提交 2：`p2b-4`
- 提交 3：Wave5 修复提交

**Wave 5 Exit Gate**
- [ ] 生命周期路径验证通过（draft->reviewing->waiting_human->executing）
- [ ] ReviewGate 契约测试通过
- [ ] Verdict：`Go / Conditional Go / No-Go`

## Wave 6：端点与联动

**目标**：打通 chat/plan/task API 与 WS Secretary 事件。  
**任务**：`p2a-9, p2a-10, p2a-11, p2b-5`

**建议验证命令**
- `go test ./internal/web/... -count=1`
- `go test ./internal/engine/... -count=1`
- `go test ./internal/secretary/... -count=1`

**推荐提交节奏**
- 提交 1：`p2a-9 + p2a-10`
- 提交 2：`p2a-11 + p2b-5`
- 提交 3：Wave6 修复提交

**Wave 6 Exit Gate**
- [ ] API 输入输出与 reject 两段式校验通过
- [ ] WS Secretary 事件广播通过
- [ ] ReviewPanel 生命周期接入集测通过
- [ ] Verdict：`Go / Conditional Go / No-Go`

## Wave 7：最终集成收口

**目标**：前端真实 API 接入 + embed.FS 打包 + 端到端验证。  
**任务**：`p2c-7`

**建议验证命令**
- `go test ./... -count=1`
- `cd web && npm test`
- `cd web && npm run typecheck`
- 手工 E2E：`ai-flow server` 后验证 Chat/Plan/Board/Pipeline 四视图

**推荐提交节奏**
- 提交 1：`p2c-7`
- 提交 2：Wave7 修复提交

**Wave 7 Exit Gate**
- [ ] 四视图均接入真实数据并可用
- [ ] 全量回归全绿
- [ ] Gate 记录更新为 PASS/READY（若通过）
- [ ] Verdict：`Go / Conditional Go / No-Go`

---

## 7. 全局回归命令（每波建议至少跑一次）

```bash
go test ./... -count=1
cd web && npm test
cd web && npm run typecheck
```

## 8. 最终 Gate（P2 收口）

### Functional
- [ ] P2-Foundation、P2a、P2b、P2c 全部任务完成且有证据工件。
- [ ] 对话 -> 拆解 -> AI 审核 -> 人工确认 -> DAG 调度 -> 完成 全链路可运行。

### Quality
- [ ] 后端全量测试通过。
- [ ] 前端测试 + 类型检查通过。
- [ ] 无阻塞级已知缺陷。

### Process
- [ ] 每个 Wave 都有独立收口记录。
- [ ] 无跨波跳关执行记录。

---

## 9. 执行交接

计划文件已生成，后续执行有两种方式：
1. **Subagent-Driven（当前会话）**：逐 Task/逐 Wave 执行，每波结束做 review 和修复闭环。  
2. **Parallel Session（独立会话）**：新会话使用 `executing-plans` 批量推进，按 Wave 打检查点。

建议优先按 Wave 执行，不要跨波并行推进高耦合任务。


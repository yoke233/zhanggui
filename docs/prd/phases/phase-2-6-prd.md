# PRD - Phase 2.6 并发执行隔离（Git Worktree）

版本：v1.0  
状态：Draft  
负责人：Tech Lead / Platform / Integrator  
目标阶段：Phase 2.6（承接 Phase 2.1，先于 Phase 3）

## 1. 背景与问题

Phase 2 已具备 Lead 自动化调度与 `run_id` 幂等语义，Phase 2.1 提供了操作控制台。  
但在“同角色同仓库并发”场景下，如果多个 worker 共享同一工作目录，会出现以下问题：

- 互相污染工作区（未提交改动覆盖、分支状态混乱）。
- `run_id` 与实际文件状态无法一一对应，导致审计与回滚困难。
- 重试/切换 worker 后，迟到结果可能混入当前执行上下文。

因此需要引入“每次 run 独立工作目录”的执行隔离能力，默认实现为 `git worktree`。

## 2. 目标与非目标

目标：

- 建立 `run_id <-> workdir` 的一对一映射。
- 支持创建、绑定、回收 worktree 的完整生命周期。
- 保证切换 worker 时上下文隔离与幂等一致。
- 在不改协作协议的前提下提升同仓并发稳定性。

非目标：

- 不在本阶段实现完整发布流水线。
- 不替代现有分支策略与 PR 审核策略。
- 不要求所有后端都必须是 GitHub（本地 git 也可运行）。

## 3. 用户与场景

用户：

- Role Lead（backend/frontend/qa/integrator）
- Worker 执行器
- Integrator

核心场景：

- 场景 A：同一 repo 并发 2-4 个 worker 执行不同 Issue。
- 场景 B：worker 失败后切换执行器，旧 run 不污染新 run。
- 场景 C：任务完成后自动清理临时 worktree，减少磁盘与状态垃圾。

## 4. 范围（In Scope）

- workdir 分配策略：按 `IssueRef + RunId` 唯一分配目录。
- 默认实现：`git worktree add/remove/prune`。
- 生命周期管理：
  - create（spawn 前）
  - attach（执行中）
  - detach/cleanup（完成、失败、超时）
- 安全校验：
  - 目录冲突检查
  - 脏目录保护（禁止误删）
  - 迟到 run 结果不复用旧 workdir
- 降级策略：无 worktree 能力时使用独立 clone。

## 5. 功能需求（PRD 级）

- FR-2.6-01：每次新 `run_id` 必须绑定唯一 `workdir`。
- FR-2.6-02：Lead 在 spawn 前必须确保 workdir 可用且干净。
- FR-2.6-03：切换 worker 时必须创建新 workdir，不得复用旧 run 工作目录。
- FR-2.6-04：完成/失败后必须支持可配置清理策略（立即清理或 TTL 清理）。
- FR-2.6-05：清理动作必须有审计日志（包含 `IssueRef`、`RunId`、目录路径、结果）。
- FR-2.6-06：当 worktree 操作失败时，任务自动进入 `blocked` 或 `needs-human`。

## 6. 验收标准（DoD）

- AC-2.6-01：同仓并发执行时，各 worker 工作目录互不污染。
- AC-2.6-02：切换 worker 后，旧 run 无法影响新 run 的工作目录状态。
- AC-2.6-03：清理策略可执行且无误删主工作目录事故。
- AC-2.6-04：至少覆盖 1 个本地 sqlite 项目和 1 个 forge 项目场景。
- AC-2.6-05：异常分支（worktree 创建失败、清理失败）有明确恢复路径。

## 7. 成功指标

- 指标 1：同仓并发导致的工作区冲突事件下降 >= 80%。
- 指标 2：`run_id` 对应工作目录追溯成功率达到 100%。
- 指标 3：切换 worker 后的幂等异常数为 0（抽样验收口径）。

## 8. 风险与缓解

- 风险：不同平台 git 行为差异导致 worktree 命令不稳定。  
  缓解：统一封装 workdir adapter，并提供 clone 降级路径。

- 风险：清理策略误删目录。  
  缓解：引入路径白名单 + 二次确认 + 审计日志。

- 风险：磁盘占用增长。  
  缓解：TTL 清理 + 定时 prune + 最大并发阈值控制。

## 9. 依赖

- `docs/operating-model/executor-protocol.md`
- `docs/workflow/lead-worker.md`
- `docs/workflow/workflow-profile.md`
- `docs/prd/phases/phase-2-prd.md`
- `docs/prd/phases/phase-2-1-prd.md`

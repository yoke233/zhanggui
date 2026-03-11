# v3 演化路线图 — 功能清单与优先级

> 日期: 2026-03-09（最后更新: 2026-03-09 evening）
>
> 基于 v3 架构设计文档，对照当前 ai-workflow 项目实现状态，梳理全部功能项并规划优先级。

## 评判标准

引用设计反思文档的核心准则：

> **结构服务于 Prompt 质量，不服务于模型完备性。**
>
> 每次要加新结构时先问：加了之后 PromptBuilder 组装出来的 prompt 会明显变好吗？

优先级综合考虑：实用价值、对后续功能的解锁作用、实现难度。

---

## 一、总览

```
 已完成 ██████████████████████░░░░░░░░░░░ 进行中 ███░ 待做
```

| 类别 | 已完成 | 进行中 | 待做 |
|------|--------|--------|------|
| 核心链路 | 6 | 1 | 0 |
| 可靠性 | 3 | 0 | 1 |
| 决策与门禁 | 2 | 0 | 0 |
| 上下文与记忆 | 2 | 0 | 1 |
| Issue 模型增强 | 1 | 0 | 3 |
| 通信层 | 0 | 0 | 3 |
| Agent 能力 | 0 | 0 | 3 |
| 自进化 | 0 | 0 | 4 |
| 生产化 | 2 | 0 | 4 |

---

## 二、功能清单

### 核心链路（v3 Phase 0-2）

| # | 功能 | 状态 | 说明 | 关键文件 |
|---|------|------|------|---------|
| C1 | Supervisor + Worker 闭环 | ✅ 完成 | TeamLeader 编排 Issue→Run→Done 完整链路 | `teamleader/manager.go`, `engine/executor.go` |
| C2 | Reviewer 角色 | ✅ 完成 | 两阶段审核 + 3 种门禁插件 (ai-panel/local/github-pr) | `teamleader/review.go`, `plugins/review-*` |
| C3 | 多 Agent 配置 | ✅ 完成 | claude/codex/openspec 三个 agent，角色绑定 stage→agent | `config/defaults.toml`, `acpclient/` |
| C4 | 并发调度 | ✅ 完成 | Semaphore 限流，多 Run 并行 | `teamleader/scheduler.go` |
| C5 | TaskStep 事件溯源 | ✅ 完成 | Issue.Status 从 TaskStep 派生，Timeline API，IssueFlowTree 组件 | `core/task_step.go`, `store-sqlite/`, `web/IssueFlowTree.tsx` |
| C6 | 子任务合并 | ✅ 完成 | auto_merge PR + child_completion 子任务完成处理 | `teamleader/auto_merge.go`, `child_completion.go` |
| C7 | Issue DAG 拆解 | 🔧 进行中 | 一句话→TL 拆解→DAG 预览→批量创建→严格依赖调度 | `docs/plans/2026-03-09-issue-dag-decompose-plan.md` |

### 可靠性（v3 已知问题解决方案）

| # | 功能 | 状态 | v3 设计 | 优先级 | 说明 |
|---|------|------|---------|--------|------|
| R1 | 错误分类 | ✅ 完成 | transient/permanent/need_help 三类型 | — | Run 失败有 conclusion 区分 |
| R2 | Watchdog 巡检 | ✅ 完成 | 定时扫描 stuck issue/run，超时升级 | — | `01e1d51` scheduler health checks + recovery loop |
| R3 | Scheduler 信号量修复 | ✅ 完成 | run 失败/取消后释放 slot | — | `8768ea0` panic recovery 防泄漏 |
| R4 | 幂等消息处理 | ❌ 待做 | idempotency_key + at-least-once 投递 | P3 | 当前 EventBus 是 in-process，暂无丢消息风险 |

### 决策与门禁（v3 核心理念）

| # | 功能 | 状态 | v3 设计 | 优先级 | 说明 |
|---|------|------|---------|--------|------|
| D1 | Decision 版本化 | ✅ 完成 | 记录每个 AI 决策的 prompt/model/reasoning，可追溯 | — | `f57d220` Decision model + `df23f12` 后端基础 + `a5bd848` 审核决策串联 + `4f02e09` decompose/stage 决策追踪 |
| D2 | Gate 门禁 | ✅ 完成 | 后端已实现 Gate/GateCheck/GateChain、四种 gate type、fallback、人工 resolve API，并接入 Decision + TaskStep + SQLite | — | 当前实现挂在 `Issue + WorkflowProfile` 语义上，能力已具备，模型语义仍是过渡态 |

### 上下文与记忆（v3 Prompt 质量核心）

| # | 功能 | 状态 | v3 设计 | 优先级 | 说明 |
|---|------|------|---------|--------|------|
| M1 | PromptBuilder 分层拼装 | ✅ 完成 | 冷→温→热三层注入 prompt，4 个模板全部改造，prefix cache 友好排列 | — | `7529b6b` prompt builder + 模板改造 + executor 集成，27 个测试全通过 |
| M2 | SQLite 记忆召回 | ✅ 完成 | 冷(Issue背景)/温(父兄弟任务)/热(TaskStep+RunEvents+Reviews) 三层 | — | `1d4648f` SQLiteMemory 269 行 + `bfc95ac` bootstrap 自动注入，11 个测试全通过 |
| M3 | Memory Compact | ❌ 待做 | 超过阈值时压缩历史为摘要，fingerprint 控制冷层缓存 | P3 | 长期任务才需要 |

### Issue 模型增强（v3 Task 字段）

| # | 功能 | 状态 | v3 设计 | 优先级 | 说明 |
|---|------|------|---------|--------|------|
| I1 | Tags 标签 | ❌ 待做 | `tags []string` 自由标签，看板分组 | P3 | 简单，按需加 |
| I2 | acceptance_criteria | ❌ 待做 | 验收条件（自然语言），写进 prompt | **P2** | Gate 需要，提升 prompt 质量 |
| I3 | participants 参与者 | ❌ 待做 | owner 之外的协作者列表 | P3 | 多 agent 讨论场景需要 |
| I4 | children_mode | ✅ 完成 | parallel / sequential 子任务执行模式 | — | 字段、校验、SQLite 持久化、decompose / DAG 路径均已接入 |

### 通信层（v3 Message/Thread/Bus）

| # | 功能 | 状态 | v3 设计 | 优先级 | 说明 |
|---|------|------|---------|--------|------|
| T1 | Thread 会话容器 | ❌ 待做 | 消息归属地，解决闲聊无归属 + 同 task 讨论混杂 | P3 | 提升热上下文精度，依赖 M1 |
| T2 | 闲聊→任务结晶 | ❌ 待做 | supervisor 将闲聊 crystallize 为正式 Task | P3 | 当前 ChatView 已有雏形 |
| T3 | Bus 群聊广播 | ❌ 待做 | msg.to 填 thread_id 时广播给所有 participants | P4 | 多 agent 讨论场景 |

### Agent 能力（v3 Agent 模型）

| # | 功能 | 状态 | v3 设计 | 优先级 | 说明 |
|---|------|------|---------|--------|------|
| A1 | 动态 Agent 创建 | ❌ 待做 | 运行时创建新 agent，需上级 approval | P4 | 当前配置驱动够用 |
| A2 | Prompt 即 Artifact | ❌ 待做 | Agent system prompt 存为 Artifact，可追溯迭代 | P3 | 依赖 Decision 版本化 |
| A3 | Agent 权限与配额 | ❌ 待做 | AgentPermission + ResourceQuota | P4 | 多人/生产环境需要 |

### 自进化（v3 Phase 3）

| # | 功能 | 状态 | v3 设计 | 优先级 | 说明 |
|---|------|------|---------|--------|------|
| E1 | Analyst Agent | ❌ 待做 | 扫描 TaskStep 发现重复模式，提议 Pattern | P4 | 远期 |
| E2 | Pattern 模板 | ❌ 待做 | 从成功经验中归纳可复用模板 | P4 | 依赖 E1 |
| E3 | 授权衰减 | ❌ 待做 | 审批分级，信任积累后减少人工审批 | P4 | 依赖 Gate |
| E4 | Dashboard Agent | ❌ 待做 | 定期生成简报，专属 KV 存储区 | P4 | 可观测性增强 |

### 生产化（v3 Phase 4）

| # | 功能 | 状态 | v3 设计 | 优先级 | 说明 |
|---|------|------|---------|--------|------|
| P1 | Web Dashboard | ✅ 完成 | React + Tailwind + WebSocket | — | BoardView/ChatView/RunView 已有 |
| P2 | Desktop 通知 | ✅ 完成 | notifier-desktop 插件 | — | |
| P3 | 定时任务 Schedule | ❌ 待做 | cron 触发，幂等键防重复 | P3 | 日报等场景 |
| P4 | PostgreSQL 迁移 | ❌ 待做 | SQLite → PG | P4 | 多实例部署需要 |
| P5 | Docker 化 | ❌ 待做 | 容器部署 + workspace 隔离 | P4 | |
| P6 | 企业 IM 通知 | ❌ 待做 | 飞书/钉钉 Notifier | P4 | |

---

## 三、优先级排序与推荐执行顺序

### P1 — 立即做 ✅ 已全部完成

```
R3 Scheduler 信号量修复    ✅ 8768ea0
R2 Watchdog 巡检           ✅ 01e1d51
D1 Decision 版本化         ✅ f57d220 + df23f12 + a5bd848
```

### 当前优先级 — 下一步做（v3.1 语义差距补齐）

```
T1 Thread 会话容器          热上下文从 issue/chat_session 升级到 thread
I2 acceptance_criteria      验收条件进入主模型并写入 prompt / gate
I3 participants             支持多 agent 讨论与 gate 参与者语义
M3 Memory Compact           长任务冷层摘要与 fingerprint
C7 Issue DAG 拆解收尾       剩余问题偏流程加固，不是后端能力缺失
```

**依赖关系:** M1✅ M2✅ → T1 | D1✅ D2✅ → I2 | C7🔧 → 更完整的子任务 / 讨论语义

### P3 — 按需推进（丰富功能）

```
I1 Tags 标签               简单
T2 闲聊→任务结晶            依赖 T1
A2 Prompt 即 Artifact       依赖 D1
P3 定时任务 Schedule         独立
R4 幂等消息处理             当前不急
```

### P4 — 远期规划（自进化 + 生产化）

```
A1 动态 Agent 创建
A3 Agent 权限与配额
T3 Bus 群聊广播
E1 Analyst Agent
E2 Pattern 模板
E3 授权衰减
E4 Dashboard Agent
P4 PostgreSQL 迁移
P5 Docker 化
P6 企业 IM 通知
```

---

## 四、依赖关系图

```
C7(DAG 拆解, 🔧进行中)
  └→ I4(children_mode)

R3(信号量修复) ✅ ──┐
R2(Watchdog)   ✅ ──┤── 可靠性基础 ✅
                    │
D1(Decision)   ✅ ──┤── 决策基础 ✅
  └→ D2(Gate)  ✅ ──┤
      └→ I2    ❌ ──┘   ← 当前结构性缺口之一
      └→ E3(授权衰减)

M1(PromptBuilder)  ✅
  ├→ M2(记忆召回)  ✅ → M3(Compact)
  └→ T1(Thread) → T2(结晶) → T3(群聊)

D1(Decision) ✅ → A2(Prompt 即 Artifact)
E1(Analyst) → E2(Pattern)
```

---

## 五、与 v3 原始 Phase 的映射

| v3 Phase | 原始目标 | 我们的做法 |
|----------|---------|-----------|
| Phase 0 | 最小闭环 | ✅ 已超额完成（含 Web UI、GitHub 集成） |
| Phase 1 | Reviewer + 动态创建 + Validator | ✅ Reviewer ✅ / Decision 版本化 ✅ / 动态创建 P4 |
| Phase 2 | 子任务拆分 + Merger + Watchdog | ⚠️ DAG 拆分后端骨架 ✅，产品闭环仍在收尾 / Merger ✅ / Watchdog ✅ |
| Phase 3 | Analyst + Pattern + 授权衰减 + Dashboard | ❌ 全部 P4 远期 |
| Phase 4 | 三级记忆 + PG + Docker + 企业 IM | ⚠️ PromptBuilder+Memory ✅已完成(含冷温热三层，但仍是 issue-centered) / Memory Compact+PG+Docker P4 |

**我们的演化路径不是照搬 v3 Phase 顺序，而是按「实用价值 × 解锁后续」的乘积排序。** v3 的 Phase 是从零建系统的路线，我们在一个已有完整链路的项目上渐进注入 v3 理念。

Phase 0-1 已基本达成，Phase 2 的后端主骨架也已就位。当前真正的结构性差距不再是 Gate，而是 **v3.1 语义补齐**：`Thread`、`acceptance_criteria`、`participants`、`Memory Compact`，以及从 `Issue/ChatSession` 逐步过渡到 `Task/Thread` 语义。

---

## 六、代码现状校准（2026-03-09）

本节用于纠正文档与后端实现之间的时间差，尤其是当前仓库里仍保留较多 `v1/v2` 术语和结构。

### 已明显领先原 roadmap 的部分

| 项目 | 原状态 | 当前判断 | 证据 |
|------|--------|---------|------|
| D2 Gate 门禁 | ❌ 待做 | ✅ 后端已落地 | `internal/core/gate.go`, `internal/teamleader/gate_chain.go`, `internal/web/handlers_gate.go`, `cmd/ai-flow/server.go` |
| I4 children_mode | ❌ 待做 | ✅ 字段和主链路已落地 | `internal/core/issue.go`, `internal/plugins/store-sqlite/store.go`, `internal/web/handlers_decompose.go` |
| C7 Issue DAG 拆解 | 🔧 进行中 | 🔧 仍在收尾，但后端骨架已完整 | `internal/teamleader/scheduler_dispatch.go`, `internal/teamleader/child_completion.go`, `internal/web/handlers_decompose.go` |
| D1 Decision 版本化 | ✅ 完成 | ✅ 完成且覆盖更广 | `internal/core/decision.go`, `internal/web/handlers_decisions.go`, `internal/teamleader/gate_chain.go` |

### 当前代码的过渡态特征

当前后端并不是“落后到还没做 v3”，而是：

- 执行能力已经显著吸收了 v3 思路
- 领域模型语义仍以 `Issue` 为核心，而不是纯 `Task`
- 会话模型仍是 `ChatSession`，而不是 `Thread`
- Gate 已存在，但是挂在 `Issue + WorkflowProfile` 语义上运行

一句话概括：

**当前后端是“用 v2 壳承载了相当一部分 v3 能力”。**

---

## 七、从当前后端到 v3.1 的差距清单

以下清单不是重新罗列全部愿景，而是只列当前代码仍明显缺失、且会影响 v3.1 语义完整性的部分。

### G1. Thread 会话容器

当前状态：

- 聊天仍使用 `ChatSession`
- 热上下文仍按 `issueID/runID` 召回
- 没有 `thread_id`、`reply_to_msg_id`

差距：

- 无法把“任务前闲聊”和“任务内讨论”统一成会话容器
- 无法把同一任务下的不同讨论主题拆开
- 无法做 reply chain 级别的热上下文提取

影响文件：

- `internal/core/chat.go`
- `internal/core/memory.go`
- `internal/engine/prompt_builder.go`
- `internal/web/handlers_chat*`

### G2. Issue/Task 主模型仍缺 `acceptance_criteria`

当前状态：

- Gate 已存在
- 但主模型里没有显式 `acceptance_criteria`

差距：

- Gate 规则和任务“什么算完成”仍然没有统一领域字段
- PromptBuilder 也无法稳定注入验收条件

影响：

- 这是把 Gate 从“可运行”推进到“v3 语义完整”的关键一步

### G3. Issue/Task 主模型仍缺 `participants`

当前状态：

- 后端已有 reviewer / role binding / gate runner
- 但主模型里没有 owner 之外的 `participants`

差距：

- 多 agent 讨论缺少领域层参与者表达
- peer_review / vote 只能靠流程逻辑，不是靠显式协作者模型

### G4. 缺少 `tags`

当前状态：

- MCP 工具和部分外围已有 tags 痕迹
- 但核心 Issue 模型未把它作为主字段

差距：

- 看板视图、归类、后续 Pattern 提炼会缺少稳定标签面

### G5. Memory Compact 尚未实现

当前状态：

- 已有 cold / warm / hot recall
- 没有 compact、fingerprint、冷层失效控制

差距：

- 长任务上下文会继续膨胀
- 还没达到 v3 文档要求的长期可控记忆模型

### G6. 通信层仍非 v3.1 语义

当前状态：

- EventBus 是 in-process `MemoryBus`
- 没有 `idempotency_key`
- 没有 thread broadcast

差距：

- 还没进入 `Thread + Message + Bus` 的目标形态
- `R4` 仍然成立

### G7. Schedule 尚未落地

当前状态：

- 有 scheduler，但这是 run/issue 调度器
- 没有 v3 中 cron 驱动的 `Schedule` 模型

差距：

- 日报、周期提醒、定时触发等场景还不能复用统一领域模型

### G8. Artifact 仍不是统一交付中心

当前状态：

- 运行时有 run artifacts / PR / merge 结果
- 但还没有 v3 语义下统一的 Artifact 版本化交付中心

差距：

- 代码和非代码结果还没完全统一到同一套可审阅、可返工、可追溯模型
- 这也是后续文档/PPT/图片交付设计要接入的地方

---

## 八、推荐更新后的判断

如果只看“后端距离 v3 还有多远”，当前更准确的判断是：

- **能力层**：已达到或超过 roadmap 原先预期的 60%-70%
- **领域语义层**：仍处在 `Issue/ChatSession -> Task/Thread` 的迁移中
- **真正该优先补的不是 Gate，而是 v3.1 语义关键字段和会话模型**

下一阶段建议按这个顺序推进：

1. `Thread`
2. `acceptance_criteria`
3. `participants`
4. `Memory Compact`
5. `tags`
6. `Schedule`
7. 统一 Artifact 交付模型

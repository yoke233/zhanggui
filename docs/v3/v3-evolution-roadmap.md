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
| 决策与门禁 | 1 | 0 | 1 |
| 上下文与记忆 | 2 | 0 | 1 |
| Issue 模型增强 | 0 | 0 | 4 |
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
| D2 | Gate 门禁 | ❌ 待做 | 替代固定 ReviewGate，支持 auto/owner_review/peer_review/vote，可串联多道 | **P1** | D1 已就绪，Gate 是下一个解锁项 |

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
| I4 | children_mode | ❌ 待做 | parallel / sequential 子任务执行模式 | **P2** | DAG 完成后自然需要 |

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

### 当前优先级 — 下一步做（门禁 + Prompt 质量 + Issue 增强）

```
D2 Gate 门禁               替代固定 ReviewGate，D1 已就绪      ← 最高优先
C7 Issue DAG 拆解收尾      decompose 流程加固中（95%）
I2 acceptance_criteria      Gate 的验收条件，写进 prompt
I4 children_mode            DAG 之后的自然延伸
```

**依赖关系:** D1✅ → D2 → I2 | C7🔧 → I4 | M1✅ M2✅ 已完成

### P3 — 按需推进（丰富功能）

```
I1 Tags 标签               简单
I3 participants 参与者      多 agent 协作
M2 三级记忆                 依赖 M1
M3 Memory Compact           长期任务需要
T1 Thread 会话容器          依赖 M1
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
  └→ D2(Gate)  ❌ ──┤   ← 下一个重点
      └→ I2    ❌ ──┘
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
| Phase 2 | 子任务拆分 + Merger + Watchdog | ⚠️ DAG 拆分 🔧收尾中 / Merger ✅ / Watchdog ✅ |
| Phase 3 | Analyst + Pattern + 授权衰减 + Dashboard | ❌ 全部 P4 远期 |
| Phase 4 | 三级记忆 + PG + Docker + 企业 IM | ⚠️ PromptBuilder+Memory ✅已完成(含冷温热三层) / Memory Compact+PG+Docker P4 |

**我们的演化路径不是照搬 v3 Phase 顺序，而是按「实用价值 × 解锁后续」的乘积排序。** v3 的 Phase 是从零建系统的路线，我们在一个已有完整链路的项目上渐进注入 v3 理念。

Phase 0-1 已基本达成，Phase 2 仅差 DAG 拆解收尾。当前瓶颈转移到 **Gate 门禁（D2）** — 所有前置依赖（Decision、Reviewer、Watchdog）均已就位。

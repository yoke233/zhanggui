# Agent 工作空间：动态多 Agent 协作模型

> **详细设计**: [06-agent-workspace-detail](06-agent-workspace-detail.zh-CN.md) — 完整代码、Schema、配置、IronClaw 能力吸收
> **消息模型**: [07-thread-message-inbox-bridge](07-thread-message-inbox-bridge.zh-CN.md) — Thread/Message/Inbox/Bridge 收敛
> **领域模型**: [08-multi-agent-core-domain-model](08-multi-agent-core-domain-model.zh-CN.md) — 四域划分、Task 中心、Authority 模型
> **迁移路线**: [09-migration-roadmap](09-migration-roadmap.zh-CN.md) — 基于当前仓库的最小迁移计划
> **参考**: [IronClaw 架构学习](ironclaw-architecture-study.zh-CN.md) — 8 项能力吸收来源
> **对接**: [spec-context-memory](../spec/spec-context-memory.md) — MemoryStore 后端规范

## 问题

当前系统是固定流水线：`Issue → Run → [stages] → Done`。角色静态、session 一次性、Agent 不能对话、流程不能动态编排。

## 核心洞察

**Agent 不是函数，是人。** 把 Agent 从"被调用的函数"变成"常驻的 Agent"，系统从"流水线调度器"变成"团队工作空间"。

## 术语

| 术语 | 含义 | 对应 |
|------|------|------|
| **AgentRuntime** | 执行协议层（当前 ACP，可替换） | config `runtimes`（原 `agents.profiles`） |
| **Agent** | 画像定义 — instruction + skills + capabilities + runtime 引用 | config `agents`（原 `roles`） |
| **AgentInstance** | 逻辑实例（不绑定进程）— Agent 画像 + 状态 + inbox，RuntimeSession 按需创建，Workspace 按需租赁 | DB `agent_instances` |

> **Skill 激活子集**: Skills 是文件资产（`configs/skills/*.md`），定义在 Agent 画像中。每次处理消息时只激活相关子集注入上下文，不是全量加载。

## 关键设计决策

### 消息模型
- **统一 IncomingMessage** — 不硬编码 directive/escalation/query 类型，语义由 Agent 自己理解
- **消息/Inbox 分离** — `agent_messages` 存一次，`agent_inbox` 通知 N 次，外部桥接不重复
- **结果引用化** — inbox 的处理结果是 `result_message_id`（指向 agent_messages），回复也是消息

### Session 模型
- **一次性 + Warm Cache** — 默认每条消息开新 session，同 thread 可复用 warm session（idle 1h 回收）
- **持久性靠显式状态 + Memory 辅助** — 跨 session 连续性靠 Thread + Store 中的显式状态，Memory 辅助语义召回，不是主状态源

### Thread
- **持久上下文容器** — 有独立生命周期（open/closed），不是裸消息分组
- **TL 是默认群主** — Worker 间可直接对话，受 Router 轮次限制（默认 5 轮），超限 escalate 给 TL

### TL 定位
- **高权限 Agent，不是唯一对外通道** — 特殊性只在默认治理职责 + 高权限 + 默认外部路由
- 仲裁/审批/预算/越权操作默认由 TL 兜底，但都是 role_binding 配置，不是硬编码架构

### Router
- Worker 间直接通信 + `max_rounds` 轮次限制 + 超限 TL 仲裁
- `approve_rounds(thread_id, extra=N)` / `deny_escalation(thread_id, reason)` 工具
- 计数维度: `(thread_id, worker_pair)`，TL 参与的对话不计数

## 可插拔接口（P0 用 no-op 实现，后续扩展）

| 接口 | P0 实现 | 后续扩展 |
|------|---------|---------|
| **SafetyLayer** | Passthrough | 注入检测、泄露扫描、Policy 规则 |
| **Router** | DefaultRouter（TL↔全员，Worker 间限 5 轮） | 动态规则、ACL |
| **MemoryStore** | RecentMessages（查最近 N 条） | SQLite FTS5 → OpenViking 语义搜索 |
| **CostGuard** | Unlimited | 日/实例/消息三级预算 |
| **GroupBridge** | nil | Slack / Discord / Telegram |

## 存储（6 张新表，叠加不替换）

`agents`（画像）、`agent_instances`（实例）、`agent_threads`、`agent_messages`、`agent_inbox`（含 `result_message_id`）、`agent_actions`

> 详细 Schema 见 [detail 文档](06-agent-workspace-detail.zh-CN.md#新增表)

## MCP 工具集

```
画像管理:   create_agent / update_agent / delete_agent / list_agents
实例管理:   spawn / kill / sleep / wake / list_instances
Thread:     create_thread / list_threads / close_thread / link_issue
消息:       send_message(to, content, thread_id?) / check_inbox
仲裁:       approve_rounds / deny_escalation
```

## 方案选择

**推荐 B（叠加），以 C（渐进统一）为北极星。**

- A: 纯 Agent 替换 — 风险太大
- **B: Agent 层叠加** — 增量实施，现有流水线不受影响
- C: 流水线渐进 Agent 化 — 最终目标，验证 B 后再推进

## 实施路径

| 阶段 | 内容 | 依赖 |
|------|------|------|
| **P0** | Gateway + Inbox + 消息存储 + TL 常驻实例 + send/check MCP 工具 + Thread | 无 |
| **P1** | 实例生命周期（spawn/kill/sleep/wake）+ SandboxPolicy + CostGuard + HealthCheck | P0 |
| **P2** | Skill 动态注入 + MemoryStore + RoutineEngine + GroupBridge + 动态编排 | P1 |
| **P3** | 流水线 Agent 化（stage → Agent message 宏） | P2 验证后 |

## 开放问题

| # | 问题 | 备注 |
|---|------|------|
| 7 | Skill 信任边界 | 外部 Skill 投毒风险，Go 侧无 WASM 沙箱 |
| 8 | MemoryStore 降级体验 | OpenViking 不可用时 L0/L1 不可用 |
| 9 | 外部桥接噪音控制 | 多实例映射同一 Slack channel 时的过滤策略 |
| 10 | 术语与现有 config 迁移 | AgentRuntime/Agent/AgentInstance 的迁移路径 |

---

> **后续**: 实施计划由 `plan-v3-agent-workspace` 承接。详细代码和 Schema 参考 [detail 文档](06-agent-workspace-detail.zh-CN.md)。

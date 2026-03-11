# IronClaw 架构学习笔记

> **项目**: [IronClaw](D:\project\ironclaw) — Rust 编写的安全个人 AI 助手框架
> **代码量**: ~83K 行 Rust，100+ 模块
> **学习目的**: 吸收其能力到 [06-Actor 工作空间](06-actor-workspace.zh-CN.md) 设计

## 项目定位

多通道、可扩展的个人 AI 助手，强调：
- 用户数据安全和隐私（本地存储、加密）
- 防提示注入和数据泄露的多层防御
- 动态扩展能力（WASM tools、MCP、skills）
- 多通道可用性（TUI、HTTP、Web、Telegram、WASM 通道）

## 整体数据流

```
User Input
  |
[Channel] (Telegram/HTTP/CLI/WebSocket)
  |
[ChannelManager] (合并所有通道的消息流)
  |
[Agent Loop] → Router (命令 vs 意图)
  |
[Session State] → 加载对话记忆
  |
[LLM Provider] + [Skills] → 推理 (多轮)
  |
[Tool Dispatch]
  |- Built-in (echo, time, memory, shell, file)
  |- WASM tools (沙箱隔离)
  |- MCP tools (外部服务器)
  |
[Safety Layer] → 注入检测、泄露扫描、输出清理
  |
[Database] → 持久化 job、对话、action 记录
  |
[Channel Response] → 回复用户
```

## 核心模块

### 1. Agent 层 (`src/agent/`)

**Agent** — 主循环，拥有所有依赖：
- `config: AgentConfig` — 并发限制、超时、迭代数
- `deps: AgentDeps` — LLM、工具、DB 的共享引用
- `channels: ChannelManager` — 消息来源
- `scheduler: Scheduler` — 作业调度（信号量并发控制）
- `session_manager: SessionManager` — 线程/会话生命周期

**核心循环**:
1. 接收来自任何通道的消息
2. Router 检测命令 (`/status`, `/cancel`)
3. 创建或恢复会话
4. **选择活跃 Skills** + 检查权限衰减
5. 构建系统提示 + skill 上下文
6. 进入 agentic 循环: LLM → 工具调用 → 收集结果 → 安全层清理 → 重复
7. 发送回复，持久化到 DB

**子模块**:
- `dispatcher.rs` — agentic 循环（LLM→tools→重复）
- `scheduler.rs` — 并行作业调度，信号量控制
- `session.rs` — 会话状态机 (pending → in_progress → completed)
- `router.rs` — 显式命令路由
- `self_repair.rs` — 心跳检测卡住的 Job，自动恢复
- `routine_engine.rs` — Cron + 事件触发的后台任务
- `cost_guard.rs` — 日预算 + 小时速率限制

### 2. 通道系统 (`src/channels/`)

**Channel trait** → `MessageStream` (Pin<Box<dyn Stream<Item=IncomingMessage>>>)

| 通道 | 实现 | 说明 |
|------|------|------|
| CLI/REPL | `repl.rs` | 交互式命令行 |
| HTTP | `http.rs` | Axum webhook 服务，HMAC-SHA256 验证 |
| Web Gateway | `web/` | Axum 路由 40+ API，WebSocket + SSE 实时推送 |
| WASM 通道 | `wasm/` | Wasmtime 沙箱，`channel.wit` 接口 |
| Telegram/Slack/Discord | 内置 WASM 通道 | `bundled.rs` 预编译 |

**ChannelManager** 合并所有通道的消息流为统一输入。

**WASM 通道安全模型**:
- 每个回调一个新 WASM 实例（无共享状态）
- HTTP 端点白名单 + 凭证注入（主机边界）
- 工作区路径前缀隔离
- 消息速率限制（100 msg/执行）

### 3. LLM 层 (`src/llm/`)

**LlmProvider trait** — 多厂商抽象:
- NEAR AI（默认）、OpenAI、Anthropic、Ollama、OpenRouter、Tinfoil

**装饰器堆栈**（组合模式）:
```
SmartRoutingProvider  ← 13 维复杂度评分，4 层路由 (Flash/Standard/Pro/Frontier)
  → RetryProvider     ← 指数退避重试
    → CircuitBreakerProvider ← 故障熔断
      → FailoverProvider     ← 多提供商故障转移
        → ResponseCacheProvider ← 响应缓存
```

**Reasoning** (`reasoning.rs`): 封装系统提示 + skill 上下文 + 安全层，执行推理循环。

### 4. 工具系统 (`src/tools/`)

**ToolRegistry** — 统一注册表，保护内置工具名不被覆盖。

| 类型 | 实现 | 隔离 |
|------|------|------|
| Built-in | Rust 原生 | 无（信任） |
| WASM | Wasmtime 沙箱 | 燃料计量、内存限制、白名单、凭证注入 |
| MCP | 外部 JSON-RPC | OAuth 2.1 认证 |

**WASM 工具沙箱** (`wit/tool.wit`):
- 导入: `log`, `now-millis`, `workspace-read`, `http-request`, `tool-invoke`, `secret-exists`
- 导出: `execute`, `schema`, `description`
- 威胁缓解: CPU(燃料) / 内存(10MB) / 网络(白名单) / FS(无) / 凭证(注入) / 输出(泄露扫描)

### 5. Skill 系统 (`src/skills/`)

**SKILL.md** 格式: YAML frontmatter + Markdown 提示内容

```yaml
---
name: my-skill
activation:
  keywords: ["keyword1"]
  patterns: ["regex.*"]
  tags: ["tag"]
  max_context_tokens: 2000
---
# 提示内容被注入到 LLM 上下文中
```

**信任模型**:
- `Installed` (低信任) — 注册表/外部来源，工具受限
- `Trusted` (高信任) — 用户本地，所有工具

**权限衰减** (`attenuation.rs`): 活跃 skills 的最低信任级别 = 有效工具上限。混合信任级别时自动降级。

**选择器** (`selector.rs`): 13 维评分（关键词匹配、regex、标签、token 预算），确定性排序。

### 6. 安全层 (`src/safety/`)

**SafetyLayer** — 统一接口:
- `sanitize_tool_output()` — 工具输出清理
- `scan_inbound_for_secrets()` — 输入泄露扫描
- `validate_input()` — 输入验证

**组件**:
| 组件 | 职责 |
|------|------|
| Sanitizer | 模式检测（XML 标记脱逃）、注入警告 |
| Validator | 输入长度、编码、合理性检查 |
| LeakDetector | 正则库检测 API 密钥、令牌模式 |
| Policy | 规则引擎，Block/Sanitize/Warn 操作 |

### 7. 数据库层 (`src/db/`)

**Database trait** (~60 个方法): `ConversationStore + JobStore + WorkspaceStore + SettingsStore + ...`

实现: PostgreSQL (deadpool-postgres + pgvector) 或 libSQL (嵌入式 SQLite)

**主要表**: conversations, conversation_messages, jobs, job_actions, llm_calls, wasm_tools, workspace_entries, settings

### 8. 记忆系统 (`src/workspace/`)

**混合搜索**: 全文搜索 + 向量搜索 + RRF (Reciprocal Rank Fusion) 融合
**分块**: 800 tokens，15% 重叠
**身份文件**: AGENTS.md (身份) / SOUL.md (价值观) / MEMORY.md (个人上下文)

### 9. 例程系统 (`src/agent/routine_engine.rs`)

- **计划触发**: Cron 表达式
- **事件触发**: Webhook 事件匹配
- **持久化**: Routine + RoutineRun 记录

### 10. 成本控制 (`src/agent/cost_guard.rs`)

```rust
CostGuard {
    daily_budget: f64,      // 日预算（美元）
    hourly_rate_limit: f64, // 小时速率限制
    per_job_limit: f64,     // 单作业上限
}
```

### 11. 自我修复 (`src/agent/self_repair.rs`)

- 心跳检测: 定期检查 Agent 是否响应
- 卡住检测: 超时无输出 → 标记异常
- 自动恢复: 重启进程 + 注入历史

## 能力对比：IronClaw vs ai-workflow 06 设计

| IronClaw 能力 | IronClaw 实现 | 06 现有设计 | 差距 |
|---|---|---|---|
| 常驻 Agent 循环 | `Agent.run()` 无限循环 | Actor `idle/busy` 状态机 | 概念一致，06 更泛化 |
| 多通道消息汇聚 | `ChannelManager` 合并 N 个流 | Gateway 路由 | 06 缺少外部通道抽象 |
| WASM 沙箱 | Wasmtime + tool.wit/channel.wit | 无 | 06 没有 Actor 执行隔离 |
| Skill 动态注入 | SKILL.md + 关键词匹配 + 权限衰减 | TL create_role(prompt=...) | 06 角色提示词静态 |
| 安全层 | 注入检测、泄露扫描、输出清理 | 无 | 06 无消息安全检查 |
| 成本控制 | CostGuard 日/时/作业预算 | 开放问题无方案 | 06 缺成本方案 |
| 智能路由 | 13 维评分 Flash→Frontier | 无 | 不同 Actor 可能需要不同模型 |
| 自我修复 | 心跳 + 卡住检测 + 自动恢复 | 开放问题无方案 | 06 缺健康管理 |
| 语义记忆 | 向量搜索 + 全文 + RRF | session_data 序列化 | 06 缺长期语义搜索 |
| 例程/Cron | routine_engine 定时+事件触发 | 无 | Actor 无自主触发 |
| 凭证隔离 | 主机边界注入、泄露扫描 | 无 | 06 无敏感数据保护 |
| 权限衰减 | Skill 信任级别决定工具上限 | Gateway allow/deny | 06 缺动态衰减 |

## 关键文件路径

```
src/
  agent/agent_loop.rs ......... Agent 主循环
  agent/dispatcher.rs ......... Agentic 循环
  agent/scheduler.rs .......... 作业调度
  agent/self_repair.rs ........ 自我修复
  agent/routine_engine.rs ..... 例程引擎
  agent/cost_guard.rs ......... 成本控制
  channels/channel.rs ......... Channel trait
  channels/manager.rs ......... ChannelManager
  channels/web/server.rs ...... Web 网关
  channels/web/sse.rs ......... SSE 实时推送
  channels/wasm/runtime.rs .... WASM 通道运行时
  llm/provider.rs ............. LlmProvider trait
  llm/smart_routing.rs ........ 智能路由
  llm/reasoning.rs ............ 推理循环
  tools/tool.rs ............... Tool trait
  tools/registry.rs ........... ToolRegistry
  tools/wasm/runtime.rs ....... WASM 工具沙箱
  tools/mcp/client.rs ......... MCP 客户端 (OAuth)
  skills/selector.rs .......... Skill 选择器
  skills/attenuation.rs ....... 权限衰减
  safety/mod.rs ............... SafetyLayer
  safety/leak_detector.rs ..... 泄露检测
  workspace/mod.rs ............ 混合搜索记忆
  db/mod.rs ................... Database trait
  config/mod.rs ............... Config 加载
  app.rs ...................... AppBuilder 初始化
```

## 对接可行性

| 对接方式 | 改动量 | 说明 |
|----------|--------|------|
| MCP 桥接 | 零改动 | IronClaw MCP client → ai-workflow /api/v1/mcp |
| HTTP webhook | ~150 行 Go | ai-workflow 加 notifier-webhook 插件 |
| WASM A2A 通道 | ~300 行 Rust | 给 IronClaw 写 channel.wit A2A 通道 |

**IronClaw 无 A2A 协议**，有完整 MCP 客户端。最简对接: MCP 桥接 + check_inbox。

# Harness Engineering & Agent Harness 学习笔记

> 整理时间：2026-03-12
> 来源：OpenAI、Anthropic、Martin Fowler、Phil Schmid、Inngest 等

---

## 一、背景：从 Agent 到 Agent Harness

2025 年证明了 AI Agent 能写代码；2026 年行业共识转向：**Agent 不是难点，Harness（驾驭层）才是**。

OpenAI Codex 团队用 3→7 名工程师，5 个月让 Agent 生成了 **100 万行生产代码**，人类零手写，累计 ~1500 PR（人均 3.5 PR/天）。他们把这套方法论称为 **Harness Engineering**。

核心命题：不是写代码，而是**工程化地构建"让 Agent 可靠写代码"的系统**。

---

## 二、什么是 Agent Harness

### 2.1 Phil Schmid 的类比

| 比喻 | 对应 |
|---|---|
| Model = CPU | 原始算力 |
| Context Window = RAM | 有限的易失工作记忆 |
| **Agent Harness = 操作系统** | 上下文策划、启动序列、标准驱动 |
| Agent = 应用程序 | 在操作系统之上运行的用户逻辑 |

> Harness 不是 Agent 本身，而是**治理 Agent 如何运行的软件系统**——确保可靠、高效、可操控。

### 2.2 Harness vs Framework

| | Agent Framework (LangChain 等) | Agent Harness |
|---|---|---|
| 关注点 | Agent 逻辑的构建块和库 | Agent 运行时的基础设施 |
| 职责 | 推理链、Prompt 模板、工具定义 | 持久化、重试、并发、可观测性、生命周期 |
| 状态管理 | 每个 framework 自己重造 | 统一的 durable execution 基础设施 |
| 设计哲学 | "给你积木，你来搭" | "基础设施已就位，Agent 专注推理" |

Inngest 核心观点：**基础设施问题（持久化、重试、并发、可观测性）应该委托给专门系统，而不是在每个 agent framework 里重新实现。**

---

## 三、Harness 的核心组件

### 3.1 Context Engineering（上下文工程）

这是 Harness 最核心的能力，解决 Agent 跨 session 的记忆连续性问题。

**Anthropic 方案：**
- `claude-progress.txt` — Agent 每次 session 结束写入进度摘要
- Git history — 代码快照提供变更记录
- Feature Manifest (JSON) — 200+ 功能的端到端场景清单，标记 pass/fail
- JSON 优于 Markdown：Agent 不太会篡改结构化的 JSON 文件

**Inngest Utah 架构的 Context 管理：**
- **两级裁剪**：soft trim（保留头尾、截断中间）→ hard clear（用占位符替换冗长工具返回）
- **强制保留**：最近 3 轮 assistant 对话始终保留
- **跨 session 压缩**：token 估算超限时自动摘要
- **预算警告**：剩余 iteration 不足时注入系统消息，防止无限循环
- **溢出恢复**：运行中 context overflow → 强制压缩 → 重试（不消耗 iteration）

### 3.2 Lifecycle Management（生命周期管理）

**双 Agent 模式（Anthropic）：**
- **Initializer Agent** — 首次运行，建立基础设施
- **Coding Agent** — 后续 session，增量推进

**Session 启动仪式（Boot Sequence）：**
每次 session 开始必须执行的标准步骤：
1. `pwd` 确认工作目录
2. 读取 progress 文档和 git log
3. 选择下一个最高优先级的未完成功能
4. 通过 `init.sh` 启动开发服务器
5. 执行基础端到端验证，确认已有功能未损坏
6. 然后才开始新工作

> 这个"仪式"消耗 token 但战略价值极高——在新工作叠加问题之前先发现已有破损。

### 3.3 Tool Orchestration（工具编排）

**原子化执行（Inngest）：**
- 每次 LLM 调用和工具执行都是独立可重试的原子单元
- 失败在第 5 步？前 4 步已持久化，不会重新执行
- 事件驱动架构：触发源（webhook/cron/子 agent）与执行解耦

**Provider 抽象：**
- 配置级切换 Anthropic/OpenAI/Google
- 子 Agent 可使用不同 Provider/Model

### 3.4 Sub-Agent Coordination（子 Agent 协调）

**Inngest 模式：**
- 复杂任务通过 `step.invoke()` fork 独立子 session
- 子 Agent 拥有独立工具集（去掉递归委派能力，防止无限递归）
- 父 Agent 收到摘要结果作为 tool output
- 各子 Agent 独立重试策略和持久化保证

**Anthropic 观点：**
- 当前用 prompt 角色分化而非架构分离
- 未来方向：专门的 testing agent、QA agent、cleanup agent

### 3.5 Architectural Constraints（架构约束）

**OpenAI Codex 团队实践：**
- 自定义 linter + 结构化测试强制执行架构边界
- 功能清单文件强制 Agent 按序推进
- **强制规则**："不允许删除或编辑测试条目"——防止 Agent 通过删测试来"修复"问题
- Browser automation (Puppeteer MCP) 做端到端验证

**Martin Fowler 分析：**
- Harness 可能演化为"服务模板"——常见应用拓扑的预置 harness
- 约束解决空间换取可维护性和可信度
- 给遗留系统加装 Harness 可能不现实（已有熵太高）

### 3.6 Entropy Management / Garbage Collection（熵管理）

- 定期"垃圾回收"扫描文档不一致、架构违反
- 主动对抗系统退化
- 这是 OpenAI 原文中明确提到的三大支柱之一

### 3.7 Error Handling & Recovery

**Inngest Utah 架构的 6 个专门函数：**
1. Main agent loop（消息处理）
2. Reply dispatch（渠道特定格式化）
3. Immediate ack（打字指示器）
4. Error recovery（全局失败处理）
5. Periodic checks（心跳监控）
6. Sub-agent delegation

每个函数独立的重试策略、并发边界、触发条件。单个函数失败不级联。

**并发控制：**
```
singleton: { key: 'event.data.sessionKey', mode: 'cancel' }
```
- 每个 session 同时只有一个 agent run
- 新消息到来取消当前运行，以新上下文重新开始

---

## 四、OpenAI Codex 团队的关键数据

| 指标 | 数据 |
|---|---|
| 团队规模 | 3→7 名工程师 |
| 开发周期 | 5 个月 |
| 代码量 | ~100 万行 |
| PR 数量 | ~1500 |
| 人均吞吐 | 3.5 PR/工程师/天 |
| 人类手写代码 | 0 行 |

---

## 五、行业关键经验总结

1. **Start Simple** — 避免庞大控制流，提供原子工具让模型自己规划
2. **Build to Delete** — 模块化架构，随时准备替换（Manus 6 个月重构 5 次）
3. **Harness as Dataset** — 失败轨迹数据是训练迭代的竞争优势
4. **Agent 挣扎 = Harness 缺失** — 不是 Agent 不行，是缺工具/护栏/文档
5. **模型漂移检测** — 长时间运行后 Agent 不遵循指令，Harness 是检测和纠正的主要手段
6. **JSON > Markdown** — 结构化格式更不容易被 Agent 意外篡改

---

## 六、对 ai-workflow 项目的启示

### 6.1 对齐分析

| Harness 核心能力 | ai-workflow 现状 | 评估 |
|---|---|---|
| Context Engineering | Briefing 机制 + Artifact 收集 | 可增强 |
| 双 Agent 模式 (Init + Worker) | AgentDriver + AgentProfile 分离 | 架构已具备基础 |
| Session Boot Sequence | Session reuse + max_turns | 可增强 |
| 工具执行原子化 + 持久化 | Step + Execution 持久化 | 已覆盖核心 |
| 子 Agent 协调 | Composite 步骤 + DAG Scheduler | 已有基础 |
| 架构约束强制 | Gate 自动验收 + Action 白名单 | **强项** |
| 熵管理 / GC | 无 | 新机会 |
| Durable Execution | SQLite 持久化 + Recovery | 已覆盖 |
| 可观测性 | Event 系统 + ExecutionProbe | 可增强 |
| Provider 抽象 | agentsdk-go + ACP over stdio | 已有 |

**核心结论：** ai-workflow 的 **"引擎管约束，Agent 管执行"** 设计哲学与 Harness Engineering 高度一致。项目已具备 Harness 大部分骨架，主要差距在 **Context Engineering** 和 **Entropy Management**。

### 6.2 具体增强建议

#### P0: Feature Manifest 机制（高价值，低改动）
在 Briefing 之外增加 JSON 格式功能清单：
- 每个特性是端到端场景描述
- 标记 pass/fail/pending 状态
- Agent 不允许删除条目，只能更新状态
- 与 Gate 验收联动

#### P0: Context Compaction 策略（高价值）
长运行 Agent 的 context window 溢出是已知问题：
- Session 管理层增加 token budget 监控
- 预算不足时注入系统消息提醒 Agent 收尾
- 两级压缩：soft trim (保留头尾) → hard clear (占位符替换)
- 工具返回结果自动截断策略

#### P1: Session Boot Sequence 标准化（中等改动）
在 `prompt_template` 中增加 `boot_sequence` 段：
- 定义 session 开始时 Agent 必须执行的标准步骤
- 读 progress → 查 git log → 跑 smoke test → 开始工作
- 可按 Role 定制不同的 boot sequence

#### P1: Progress 文件机制（中等改动）
- 每次 session/execution 结束自动写入结构化 progress 文件
- 下次 session 开始时自动注入上下文
- 与现有 Briefing 系统互补（Briefing = 目标，Progress = 状态）

#### P2: 熵管理定时任务（新增能力）
增加定期扫描：
- 文档与代码一致性
- 架构约束违反检测
- 死代码 / 未使用的 resource binding
- 可作为特殊的 maintenance flow 实现

#### P2: Trajectory 数据收集（长期竞争力）
- 每次 Run 的完整执行轨迹结构化存储
- Agent 决策、工具调用、失败恢复路径
- 作为未来优化 prompt/harness 的数据集

---

## 参考资料

- [Harness engineering: leveraging Codex in an agent-first world — OpenAI](https://openai.com/index/harness-engineering/)
- [Effective harnesses for long-running agents — Anthropic](https://www.anthropic.com/engineering/effective-harnesses-for-long-running-agents)
- [Harness Engineering — Martin Fowler / Birgitta Böckeler](https://martinfowler.com/articles/exploring-gen-ai/harness-engineering.html)
- [The importance of Agent Harness in 2026 — Phil Schmid](https://www.philschmid.de/agent-harness-2026)
- [Your Agent Needs a Harness, Not a Framework — Inngest](https://www.inngest.com/blog/your-agent-needs-a-harness-not-a-framework)
- [2025 Was Agents. 2026 Is Agent Harnesses — Aakash Gupta](https://aakashgupta.medium.com/2025-was-agents-2026-is-agent-harnesses-heres-why-that-changes-everything-073e9877655e)
- [OpenAI Introduces Harness Engineering — InfoQ](https://www.infoq.com/news/2026/02/openai-harness-engineering-codex/)
- [What Is an Agent Harness? — Salesforce](https://www.salesforce.com/agentforce/ai-agents/agent-harness/)

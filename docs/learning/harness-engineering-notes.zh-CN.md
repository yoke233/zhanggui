# Harness Engineering & Agent Harness 学习笔记

> 整理时间：2026-03-12（v2，融合深度研究）
> 来源：OpenAI、Anthropic、Martin Fowler、Phil Schmid、Inngest、LangChain、Phodal、Charlie Guo、CNCF 等

---

## 一、背景：从 Agent 到 Agent Harness

2025 年证明了 AI Agent 能写代码；2026 年行业共识转向：**Agent 不是难点，Harness（驾驭层）才是**。

OpenAI Codex 团队用 3→7 名工程师，5 个月让 Agent 生成了 **100 万行生产代码**，人类零手写，累计 ~1500 PR（人均 3.5 PR/天）。他们把这套方法论称为 **Harness Engineering**。

LangChain 在 Terminal Bench 2.0 上给出了最有力的量化证据：**仅改 harness、不换模型（GPT-5.2-Codex），得分从 52.8% 跃升至 66.5%，排名从第 30 名升至第 5 名**。这证明了底层模型远不如围绕模型的系统重要。

核心命题：不是写代码，而是**工程化地构建"让 Agent 可靠写代码"的系统**。

---

## 二、什么是 Agent Harness

### 2.1 定义

> **Harness Engineering 是设计约束（constraints）、反馈回路（feedback loops）、文档结构（documentation）、代码检查器（linters）以及生命周期管理系统的工程学科，使 AI 编码 Agent 能够在规模化场景下可靠运行。**

LangChain 给出了一个精炼的公式：**Agent = Model + Harness**。模型提供智能，harness 让智能变得实用。Harness 涵盖了"模型本身之外的所有代码、配置和执行逻辑"。

"Harness"（马具）隐喻是刻意选择的——缰绳、鞍具、嚼子——将强大但不可预测的动物引向正确方向：
- **马 = AI 模型**：强大、快速，但自身不知道该往哪走
- **马具 = 基础设施**：约束、护栏、反馈回路，将模型的力量引导到生产性方向

### 2.2 Phil Schmid 的类比

| 比喻 | 对应 |
|---|---|
| Model = CPU | 原始算力 |
| Context Window = RAM | 有限的易失工作记忆 |
| **Agent Harness = 操作系统** | 上下文策划、启动序列、标准驱动 |
| Agent = 应用程序 | 在操作系统之上运行的用户逻辑 |

> Harness 不是 Agent 本身，而是**治理 Agent 如何运行的软件系统**——确保可靠、高效、可操控。

### 2.3 Harness vs Framework

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

这是 Harness 最核心的能力，解决"让 Agent 知道该做什么"以及跨 session 的记忆连续性问题。OpenAI 将其类比为"为新队友进行入职培训——介绍产品原则、工程规范和团队文化"。

**机器可读的知识库。** 内部文档组织在结构化的 `docs/` 目录中，包含地图（maps）、执行计划（execution plans）和设计规格（design specifications）。通过 linter 和 CI 验证强制执行交叉引用，作为 Agent 的"唯一真相来源"。

**AGENTS.md 作为活文档。** 多个组织采用 `AGENTS.md` 文件作为 Agent 的操作指南。Charlie Guo 指出，这些文件"每当 Agent 遇到困难或失败时就会更新"，不是静态文档而是主动的反馈回路，防止同类错误反复出现。

**渐进式上下文披露。** 不是一次性向 Agent 提供所有信息，而是根据当前任务动态注入相关上下文。Phodal 也强调"逐步披露上下文而非一次性提供全部信息"。Martin Fowler 指出，代码的架构设计本身就成为了上下文的一部分。

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

**LangChain 环境上下文注入：**
- `LocalContextMiddleware` 映射目录结构和可用工具，使 Agent 具备环境感知能力
- 注入关于编写可测试代码的指导、自动化评估标准、时间预算警告

### 3.2 Architectural Constraints（架构约束）

架构约束解决的是"收敛 Agent 的行动空间"的问题。

**核心发现：约束即生产力。** OpenAI 发现了一个反直觉的规律：**约束解空间反而让 Agent 更高效，而非更低效**。当 Agent 可以生成任何东西时，它会浪费 token 探索死胡同。当 harness 定义了清晰边界，Agent 会更快地收敛到正确解。

**分层依赖强制（OpenAI）：**
每个业务领域内强制执行固定的依赖层级：
```
Types → Config → Repo → Service → Runtime → UI
```
代码只能沿这个方向"向前"依赖。跨切面关注点（认证、连接器、遥测、功能标志）通过单一显式接口（Providers）进入，其他一切被禁止并通过机械化方式强制执行。

**机械化执行原则。** 架构意图必须通过 linter 和 CI 机械化执行，而不仅仅是写在文档里。因为 Agent 会在规模上复制模式——如果一个反模式被允许一次，它就会被复制几十次。

**自定义 linter 的教学功能。** 自定义 linter 不仅报告违规，还提供修复指导。linter 的错误信息本身成为了 Agent 的"教学工具"——Agent 在工作中学习。

**更多实践：**
- 功能清单文件强制 Agent 按序推进
- **强制规则**："不允许删除或编辑测试条目"——防止 Agent 通过删测试来"修复"问题
- Browser automation (Puppeteer MCP) 做端到端验证
- 结构测试（structural tests）验证合规性，防止模块化崩溃

**Martin Fowler 分析：**
- Harness 可能演化为"服务模板"——常见应用拓扑的预置 harness
- 约束解决空间换取可维护性和可信度
- 给遗留系统加装 Harness 可能不现实（已有熵太高）
- 随着模型能力提升，**harness 应该变得更薄而非更复杂**——更强的模型需要更好的护栏，而非更多的护栏
- 未来技术栈的选择标准将从"开发者偏好"转变为"AI 友好度"和"可用 harness 的成熟度"

### 3.3 Feedback Loops（反馈回路）

反馈回路解决的是"验证 Agent 做对了"和"纠正 Agent 做错了"的问题。OpenAI 的核心理念是：

> "当 Agent 遇到困难时，我们将其视为信号：找出缺失的东西——工具、护栏、文档——然后反馈回去。"

**LangChain Build-and-Verify Loop：**
Agent 最常见的失败模式是"写完代码就停下，没有充分测试"。解决方案：
1. 规划与发现阶段
2. 带测试的实现阶段
3. 对照规格的验证阶段
4. 错误分析与修复阶段

`PreCompletionChecklistMiddleware` 在 Agent 完成前拦截，要求通过所有验证检查才能标记为完成。

**LangChain Loop Detection：**
`LoopDetectionMiddleware` 监控文件编辑，在 N 次重复性修改后建议重新考虑方案，防止"厄运循环"（doom loops）——Agent 反复应用无效修复。

**LangChain 推理三明治（Reasoning Sandwich）：**
在规划和验证阶段分配最大推理资源，实现阶段使用适度推理，在准确性与 token 消耗/超时之间取得平衡。

**LangChain Trace-Based 分析：**
自动化 trace 分析创建了一个 agent skill，可以抓取 LangSmith 实验 trace、派生并行的错误分析 agent、将发现综合为有针对性的改进。类似于 boosting 算法——每次迭代聚焦于上一次的失败模式。

**Phodal 自动化反馈回路原则：**
"AI 每完成一次修改都会立即获得反馈"——来自开发环境的实时检查、CI 系统的验证结果、运行期监控和日志。

### 3.4 Lifecycle Management（生命周期管理）

**双 Agent 模式（Anthropic）：**
- **Initializer Agent** — 首次运行，建立基础设施（`init.sh`、`claude-progress.txt`、初始 Git 提交）
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

**功能列表驱动（Anthropic）：**
超过 200 个详细功能项，初始全部标记为"failing"，每个包含具体步骤和验收标准。JSON 格式优于 Markdown——Agent 更不容易篡改结构化数据。每个 session 只处理一个功能，防止上下文耗尽。

### 3.5 Tool Orchestration（工具编排）

**原子化执行（Inngest）：**
- 每次 LLM 调用和工具执行都是独立可重试的原子单元
- 失败在第 5 步？前 4 步已持久化，不会重新执行
- 事件驱动架构：触发源（webhook/cron/子 agent）与执行解耦

**Provider 抽象：**
- 配置级切换 Anthropic/OpenAI/Google
- 子 Agent 可使用不同 Provider/Model
- LangChain 发现不同模型需要不同的 harness 调优——Claude Opus 4.6 用早期 harness 得分 59.6%，说明 harness 需要针对模型特性优化

### 3.6 Sub-Agent Coordination（子 Agent 协调）

**Inngest 模式：**
- 复杂任务通过 `step.invoke()` fork 独立子 session
- 子 Agent 拥有独立工具集（去掉递归委派能力，防止无限递归）
- 父 Agent 收到摘要结果作为 tool output
- 各子 Agent 独立重试策略和持久化保证

**Anthropic 观点：**
- 当前用 prompt 角色分化而非架构分离
- 未来方向：专门的 testing agent、QA agent、cleanup agent

**多 Agent 验证工作流（行业趋势）：**
多 Agent 工作流正在替代单 Agent 代码生成——一个 Agent 写代码、一个评审、一个测试、一个验证合规性和架构对齐。

### 3.7 Entropy Management / Garbage Collection（熵管理）

这是 OpenAI 原文中明确提到的三大支柱之一，Martin Fowler 称其为"垃圾回收"。

- Agent 生成的代码会以不同于人类代码的方式积累"杂质"（cruft）
- OpenAI 建立了 **Golden Principles（黄金原则）**——一组有态度的、机械化的规则，保持代码库对未来 Agent 运行的可读性和一致性
- 后台 Codex 任务定期扫描偏差、更新质量评分、开启有针对性的重构 PR（可在一分钟内审查并自动合并）
- Charlie Guo 指出这仍是开放问题："Agent 代码以不同于人类代码的方式积累杂质；持续的垃圾回收仍然是实验性的"

### 3.8 Error Handling & Recovery

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

## 四、Harness 解剖结构（LangChain 系统分析）

一个完整的 Agent Harness 包含以下核心组件：

| 组件 | 功能 | 关键实现 |
|---|---|---|
| 文件系统 | 持久化存储、跨会话状态、多 Agent 协作 | 工作空间、Git 集成、tool call offloading |
| 代码执行 | Bash/通用代码执行能力 | 沙盒环境，允许模型自主设计方案 |
| 沙盒环境 | 安全隔离的执行空间 | 预配置运行时、CLI、浏览器 |
| 记忆/知识注入 | 超越训练数据的信息获取 | 持久化记忆文件、Web 搜索、MCP 工具 |
| 上下文管理 | 处理上下文窗口限制 | Compaction（智能摘要）、Skills（渐进披露）|
| 长期执行模式 | 多步骤、跨会话的复杂任务 | 进度追踪、Ralph Loop 模式、自验证回路 |

LangChain 的设计原则：**从期望行为反向工程**——先确定 Agent 应该实现什么，再工程化实现使其成为可能的 harness 功能。

---

## 五、CNCF Agent 平台控制四大支柱

2026 年 CNCF 提出的 Agent 平台标准化控制框架：

| 支柱 | 含义 | 实践 |
|---|---|---|
| **Golden Paths** | 预审批的标准化配置 | 预设模型/提供商组合，团队继承而非发明 |
| **Guardrails** | 硬性策略强制 | 成本上限、持续时间限制、阻止模式、工具白名单 |
| **Safety Nets** | 自动恢复机制 | 指数退避、降级响应、熔断器 |
| **Manual Review** | 高风险人工审批 | 关键决策前的 human-in-the-loop 门控 |

核心洞察："没有 harness 的 Agent 只是原型。" 生产级 Agent 系统需要与容器编排同等的治理纪律。

---

## 六、关键数据与行业案例

### 6.1 OpenAI Codex 内部实验

| 指标 | 数据 |
|---|---|
| 团队规模 | 3→7 名工程师 |
| 开发周期 | 5 个月 |
| 代码量 | ~100 万行 |
| PR 数量 | ~1500 |
| 人均吞吐 | 3.5 PR/工程师/天 |
| 人类手写代码 | 0 行 |

最大挑战不在代码生成本身，而在于"设计环境、反馈回路和控制系统"。

### 6.2 LangChain Terminal Bench 2.0

| 指标 | 数据 |
|---|---|
| 模型 | GPT-5.2-Codex（不变） |
| 改进前得分 | 52.8%（排名 30） |
| 改进后得分 | 66.5%（排名 5） |
| 提升幅度 | +13.7 分 |
| 改变内容 | 仅 harness，未换模型 |
| Claude Opus 4.6（早期 harness） | 59.6% |

关键技术：Build-and-Verify Loop、环境上下文注入、循环检测中间件、推理三明治策略、Trace-Based 分析。Claude Opus 4.6 的数据说明不同模型需要专属 harness 调优。

### 6.3 更多行业案例

| 组织/个人 | 数据 | 方法 |
|---|---|---|
| **Stripe Minions** | 每周 1000+ 合并 PR | 沙盒化开发环境 |
| **OpenClaw / Peter Steinberger** | 月均 6600+ 提交 | 同时运行 5-10 个 Agent，不逐行阅读代码 |
| **Manus** | 6 个月重构 5 次 | Build to Delete 理念 |

Charlie Guo 观察到两种并行策略：
- **有人值守**（attended）：3-4 个活跃 session，需主动管理
- **无人值守**（unattended）：任务委派后仅在 PR 阶段人工审查

---

## 七、Phodal 的三大落地原则

Phodal 是国内最早系统性引介 Harness Engineering 的技术作者，将核心方法论凝练为三大原则：

### 原则一：系统可读性（让 AI 理解系统）
"系统的结构、架构原则和领域概念需要被明确表达，并以机器可读的形式组织起来。"
- 将隐性知识显性化（架构文档、编码规范）
- 逐步披露上下文而非一次性提供全部信息
- 将工程能力暴露为 CLI 或 API 接口而非仅限 GUI

### 原则二：防御机制（收敛 AI 行动空间）
建立工程约束作为"物理定律"来约束 AI 行为：
- 静态分析和自动化测试
- 架构验证工具（如 ArchGuard）
- 代码评审自动化

### 原则三：自动化反馈回路（持续学习）
"AI 每完成一次修改都会立即获得反馈"：
- 开发环境的实时检查
- CI 系统验证结果
- 运行期监控和日志反馈

**Routa.js 实践案例：** 工程规范文档定义标准（可读性）、双层 Git Hooks 提供防御（防御机制）、Issue 流程触发自动化循环（反馈回路）。

**防御视角（Codex Security）：** AI 没有发明新漏洞，而是作为"缺陷放大器"——一旦模型生成了某种反模式，就会在多个模块中被系统性复制。此外，AI 工具的危险使用模式（如 `--dangerously-skip-permissions`、`bypassPermissions:true`）代表了一类新的风险——工具行为与系统环境的不匹配。

核心洞察："AI Coding 的突破往往来自工程系统，而不仅仅是模型能力。"

---

## 八、行业关键经验总结

1. **模型 < 系统** — LangChain 证明：仅改 harness 不换模型，得分提升 13.7 个百分点
2. **约束即生产力** — 约束解空间不降低效率反而提高——Agent 不再浪费 token 探索死胡同
3. **Agent 挣扎 = Harness 缺失** — 不是 Agent 不行，是缺工具/护栏/文档
4. **Start Simple** — 避免庞大控制流，提供原子工具让模型自己规划
5. **Build to Delete** — 模块化架构，随时准备替换（Manus 6 个月重构 5 次）
6. **Harness as Dataset** — 失败轨迹数据是训练迭代的竞争优势
7. **模型漂移检测** — 长时间运行后 Agent 不遵循指令，Harness 是检测和纠正的主要手段
8. **JSON > Markdown** — 结构化格式更不容易被 Agent 意外篡改
9. **机械化执行 > 文档约定** — 架构规则写在文档里没用，必须通过 linter/CI/结构测试强制执行
10. **模型特异性** — 不同模型需要不同的 harness 调优策略，没有通用方案

---

## 九、已知局限与开放问题

| 问题 | 现状 | 影响 |
|---|---|---|
| **绿地偏见** | 所有成功案例都是从零开始的项目 | 存量系统改造路径不明确 |
| **功能正确性缺口** | 架构约束丰富但端到端测试验证不足 | Agent 倾向于"写完就说 done" |
| **熵管理实验性** | 持续垃圾回收仍在探索阶段 | Agent 杂质与人类杂质形态不同 |
| **文化阻力** | 需要工程师重新定义专业身份 | "喜欢算法谜题的人适应困难，优先交付的人适应快" |
| **模型特异性** | 不同模型需要不同 harness 调优 | 缺乏通用方法论 |
| **成本未量化** | OpenAI 投入 5 个月开发 harness | 投入产出比不明 |
| **技术栈收敛** | 未来选技术栈看"AI 友好度" | 可能限制技术选择自由度 |

---

## 十、对 ai-workflow 项目的启示

### 10.1 对齐分析

| Harness 核心能力 | ai-workflow 现状 | 评估 |
|---|---|---|
| Context Engineering | Briefing 机制 + Artifact 收集 | 可增强（渐进披露、AGENTS.md） |
| 双 Agent 模式 (Init + Worker) | AgentDriver + AgentProfile 分离 | 架构已具备基础 |
| Session Boot Sequence | Session reuse + max_turns | 可增强（标准化启动仪式） |
| 工具执行原子化 + 持久化 | Step + Execution 持久化 | 已覆盖核心 |
| 子 Agent 协调 | Composite 步骤 + DAG Scheduler | 已有基础 |
| 架构约束强制 | Gate 自动验收 + Action 白名单 | **强项** |
| 反馈回路 | Gate reject → 自动重建 SubFlowID | 可增强（Loop Detection、Trace 分析） |
| 熵管理 / GC | 无 | 新机会 |
| Durable Execution | SQLite 持久化 + Recovery | 已覆盖 |
| 可观测性 | Event 系统 + ExecutionProbe | 可增强 |
| Provider 抽象 | agentsdk-go + ACP over stdio | 已有 |
| CNCF 四大支柱 | Golden Paths 部分覆盖（config.toml） | Guardrails/Safety Nets 可增强 |

**核心结论：** ai-workflow 的 **"引擎管约束，Agent 管执行"** 设计哲学与 Harness Engineering 高度一致。项目已具备 Harness 大部分骨架，主要差距在 **Context Engineering 深度**、**Loop/Trace 反馈机制** 和 **Entropy Management**。

### 10.2 具体增强建议

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

#### P1: Loop Detection 机制（LangChain 启发）
- 在 Step 执行层监控重复修改模式
- 检测到 doom loop 时建议 Agent 重新规划
- 与现有 Recovery 机制集成

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
- 参考 LangChain 的 Trace-Based 分析模式

#### P2: Guardrails / Safety Nets 增强（CNCF 对齐）
- 成本上限：token/时间 budget 强制执行
- 熔断器：连续失败 N 次自动停止
- 降级响应：Agent 不可用时的回退策略

---

## 参考资料

- [Harness engineering: leveraging Codex in an agent-first world — OpenAI](https://openai.com/index/harness-engineering/)
- [Harness Engineering — Martin Fowler / Birgitta Böckeler](https://martinfowler.com/articles/exploring-gen-ai/harness-engineering.html)
- [OpenAI Introduces Harness Engineering — InfoQ](https://www.infoq.com/news/2026/02/openai-harness-engineering-codex/)
- [The Anatomy of an Agent Harness — LangChain](https://blog.langchain.com/the-anatomy-of-an-agent-harness/)
- [Harness Engineering: The Complete Guide — NxCode](https://www.nxcode.io/resources/news/harness-engineering-complete-guide-ai-agent-codex-2026)
- [Effective harnesses for long-running agents — Anthropic](https://www.anthropic.com/engineering/effective-harnesses-for-long-running-agents)
- [The Emerging "Harness Engineering" Playbook — Charlie Guo](https://www.ignorance.ai/p/the-emerging-harness-engineering)
- [Improving Deep Agents with Harness Engineering — LangChain](https://blog.langchain.com/improving-deep-agents-with-harness-engineering/)
- [Harness Engineering 实践指南：三大原则 — Phodal](https://www.phodal.com/blog/harness-engineering/)
- [The importance of Agent Harness in 2026 — Phil Schmid](https://www.philschmid.de/agent-harness-2026)
- [Agent Harnesses: Why 2026 Is About Controlling Them — DEV Community](https://dev.to/htekdev/agent-harnesses-why-2026-isnt-about-more-agents-its-about-controlling-them-1f24)
- [Your Agent Needs a Harness, Not a Framework — Inngest](https://www.inngest.com/blog/your-agent-needs-a-harness-not-a-framework)
- [What is AI Harness Engineering? — Mohit Sewak / Medium](https://medium.com/be-open/what-is-ai-harness-engineering-your-guide-to-controlling-autonomous-systems-30c9c8d2b489)
- [Harness Engineering 深度解析 — 知乎](https://zhuanlan.zhihu.com/p/2014014859164026634)
- [State of AI Code Quality in 2025 — Qodo](https://www.qodo.ai/reports/state-of-ai-code-quality/)

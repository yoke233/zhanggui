# Harness Engineering 与 ai-workflow 对齐讨论

> 日期：2026-03-12
> 背景：基于 Harness Engineering 深度研究 + 后端全量代码扫描的综合分析
> 目的：识别项目已有的 Harness 能力、差距、以及可落地的增强方向

---

## 一、总体判断

**ai-workflow 在设计哲学上与 Harness Engineering 完全对齐，且在约束执行层面领先于多数开源方案。**

"引擎管约束，Agent 管执行"本质上就是 Harness Engineering 的核心命题——只是项目在 OpenAI 提出术语之前就已经在做这件事了。

项目的**强项**是架构约束和生命周期管理（Gate/Signal/Recovery/Composite），**弱项**是上下文工程的深度和运行时反馈的智能化。下面逐一展开。

---

## 二、已有的 Harness 能力（按 HE 四大支柱映射）

### 2.1 架构约束（Architectural Constraints）— 强

| HE 要求 | ai-workflow 实现 | 状态 |
|---------|-----------------|------|
| 能力层级约束 | `DriverCapabilities.Covers()` → Profile ≤ Driver | 已有，机械化强制 |
| 动作白名单 | `AgentProfile.ActionsAllowed` + `DefaultActions(role)` | 已有，按角色预设 |
| 步骤类型分离 | `exec / gate / composite` 三种 StepType | 已有 |
| 角色权限边界 | Lead 可 create_step/expand_flow，Worker 只能 fs_write/terminal，Gate 只能 approve/reject | 已有 |
| 依赖方向强制 | Step.Position + DAG 验证 | 已有 |
| Composite 不占信号量 | 防止子 Issue 步骤饿死（死锁预防） | 已有，巧妙设计 |

**评价：** 这是项目最成熟的部分。三层能力模型（Driver → Profile → Step）比 OpenAI 公开描述的单层 linter 约束更精细。`Covers()` 验证是真正的机械化执行，不依赖文档约定。

### 2.2 上下文工程（Context Engineering）— 中等

| HE 要求 | ai-workflow 实现 | 状态 |
|---------|-----------------|------|
| 任务描述注入 | Briefing.Objective + ContextRefs | 已有 |
| 上游制品注入 | L2（直接前序完整）/ L0（远前序摘要）分层注入 | 已有，带距离加权 |
| Feature 清单注入 | ContextRefType = "feature_manifest" | 已有 |
| 上下文预算控制 | 12000 chars 总预算，按类型分配 | 已有 |
| Gate 反馈注入 | `last_gate_feedback` → `rework_history` → 执行输入 | 已有 |
| 会话复用 | `ProfileSession.Reuse` + AgentContext 跨 Step | 已有 |
| Token 预算监控 | `MaxContextTokens` + `ContextWarnRatio` + `CheckTokenBudget()` | 已有 |
| 渐进式上下文披露 | — | **缺失** |
| AGENTS.md / 活文档 | — | **缺失** |
| 环境感知上下文 | — | **缺失** |

**评价：** Briefing 系统设计完善（预算、分层、距离加权），但上下文来源局限于"引擎已知信息"（Issue 描述、上游 Artifact、Feature 清单）。缺少项目级工程知识的注入（架构规范、编码规则、团队约定）——这正是 AGENTS.md 和环境上下文要解决的。

### 2.3 反馈回路（Feedback Loops）— 中等偏强

| HE 要求 | ai-workflow 实现 | 状态 |
|---------|-----------------|------|
| Gate 验收 | `ProcessGate()` → pass/reject → 上游重做 | 已有，完整闭环 |
| 重做反馈传递 | `recordGateRework()` → `last_gate_feedback` + `rework_history` | 已有 |
| 重试预算 | `MaxRetries` / `RetryCount` + `ErrMaxRetriesExceeded` | 已有 |
| 错误分类 | `ErrorKind`（transient/permanent/need_help）驱动重试策略 | 已有 |
| 信号系统 | `SignalComplete/NeedHelp/Blocked/Approve/Reject` | 已有 |
| Manifest Gate 检查 | Gate 步骤可启用 `manifest_check`，按标签过滤 | 已有 |
| 诊断探针 | `ExecutionProbe`（watchdog/manual → alive/blocked/hung/dead） | 已有 |
| 循环检测（Doom Loop） | — | **缺失** |
| Trace 分析（失败模式识别） | — | **缺失** |
| Build-and-Verify 强制 | — | **部分**（Gate 做验证，但无 PreCompletion 拦截） |

**评价：** Gate 机制是项目的一大亮点——拒绝后自动重置上游、反馈自动注入下一次执行、重试预算控制。ExecutionProbe 的 watchdog 探针也是 Harness 层面的独特实现。缺的是 LangChain 风格的"Agent 行为级"反馈（循环检测、完成前检查清单）。

### 2.4 熵管理 / 垃圾回收（Entropy Management）— 缺失

| HE 要求 | ai-workflow 实现 | 状态 |
|---------|-----------------|------|
| 定期架构合规扫描 | — | **缺失** |
| 文档-代码一致性检查 | — | **缺失** |
| 死代码/废弃资源清理 | — | **缺失** |
| Golden Principles 自动执行 | — | **缺失** |

**评价：** 这是最大空白。不过考虑到 OpenAI 自己也说这是"实验性的"，且 ai-workflow 当前聚焦于执行引擎而非代码库维护，这个缺失是合理的优先级选择。

---

## 三、额外的 Harness 能力（超出 HE 四大支柱）

ai-workflow 有一些能力在 Harness Engineering 文献中较少提及但同样重要：

| 能力 | 实现 | HE 对应 |
|------|------|---------|
| **Recovery（进程重启恢复）** | Running/Queued 步骤自动重置为 Pending + 重新提交 | Anthropic 的 Boot Sequence 的系统级版本 |
| **Composite 步骤（递归分解）** | 子 Issue 创建 + 递归执行 + Gate 拒绝时清除重建 | 多 Agent 协调的强实现 |
| **Builtin 执行器** | git_commit_push / scm_open_pr 不走 Agent | 将确定性操作从 Agent 中剥离（约束最大化） |
| **权限代理（Permission Broker）** | Chat 交互中拦截权限请求 → 转发前端 → 60 秒超时 | CNCF Manual Review 支柱 |
| **事件分层** | 持久化 / 流式 / 聚合三层 | 可观测性工程 |
| **Thread 多人讨论** | 多 Agent 同时参与线程 + Token 追踪 | 多 Agent 协作超出单一 Harness 范畴 |

**Builtin 执行器的 Harness 意义：** 将 git commit/push 和 PR 创建从 Agent 手中拿走，交给确定性的代码路径，是"约束即生产力"的极致体现——Agent 不需要知道怎么推代码，引擎替它做。

---

## 四、差距分析与增强方向

### 4.1 P0: 项目级工程知识注入（上下文工程深化）

**现状：** Briefing 只注入 Issue 摘要、上游 Artifact、Feature 清单。Agent 对项目的架构规范、编码规则、技术栈约定一无所知。

**问题：** 如果 Agent 不知道"我们用 slog 不用 logrus"、"HTTP handler 放 internal/adapters/http/"、"Go 测试文件必须在同包"，它生成的代码可能在功能上正确但在工程上不合规。

**建议方案：**

在 `ContextRefType` 中新增：
```go
CtxProjectRules  ContextRefType = "project_rules"   // AGENTS.md / 工程规范
CtxArchGuide     ContextRefType = "arch_guide"       // 架构指南（模块职责、依赖方向）
```

在 `BriefingBuilder.Build()` 中，自动从项目 workspace 中读取 `AGENTS.md`（或 `.ai-workflow/agents.md`），截断后作为 ContextRef 注入。

**预算分配建议：**
```
project_rules:  2000 chars（最重要的约束规则）
arch_guide:     1500 chars（模块边界描述）
```

**关键：** 这些文件需要在 Agent 每次犯错时更新（Charlie Guo 的"活文档"模式），形成闭环。可以在 Gate 拒绝时自动追加一条规则到 `rework_history` → 下次 Briefing 注入。

---

### 4.2 P0: Context Compaction（上下文压缩策略）

**现状：** `MaxContextTokens` + `ContextWarnRatio` 已经追踪 token 消耗，`CheckTokenBudget()` 返回 OK/Warning/Exceeded。但 Exceeded 时没有自动处理。

**问题：** 长会话复用时（`ProfileSession.Reuse = true`），context 可能溢出。目前只有警告，没有压缩。

**建议方案：**

在 `session_manager_local.go` 的 Acquire 阶段增加：

```
if budget == TokenBudgetExceeded:
    → 强制创建新 session（放弃旧上下文）
    → 在新 session 的 system prompt 中注入 AgentContext.Summary

if budget == TokenBudgetWarning:
    → 在 execution input 末尾注入警告：
      "Context window approaching limit. Prioritize completing current task.
       Summarize progress before ending."
```

这利用了已有的 `AgentContext.Summary` 字段——目前这个字段似乎未被充分利用。

---

### 4.3 P1: Loop Detection（循环检测中间件）

**现状：** Gate 有 `MaxRetries` 限制，但没有检测 Agent 在单次执行内的循环行为（反复编辑同一文件、反复运行同一命令失败）。

**问题：** Agent 可能陷入 doom loop——反复应用无效修复，浪费 token 和时间，最终超时失败。

**建议方案：**

在 ACP 执行器的事件监听层（`WatchExecution` → `EventBridge`）增加检测逻辑：

```go
type LoopDetector struct {
    fileEditCounts map[string]int  // 文件编辑次数
    commandCounts  map[string]int  // 命令执行次数
    threshold      int             // 触发阈值（如 5）
}

// 在 EventBridge 的 chunk 聚合回调中：
func (ld *LoopDetector) OnToolCall(toolName, args string) {
    if toolName == "fs_write" || toolName == "edit" {
        filePath := extractFilePath(args)
        ld.fileEditCounts[filePath]++
        if ld.fileEditCounts[filePath] > ld.threshold {
            // 注入系统消息：建议重新规划
        }
    }
}
```

**注意：** 这需要在 ACP 协议层面有注入系统消息的能力。如果 ACP 不支持中途注入，可以退而求其次在 ExecutionProbe 的 watchdog 中检测（"Agent 在过去 N 分钟内对同一文件编辑了 5 次，是否陷入循环？"）。

---

### 4.4 P1: Boot Sequence 标准化

**现状：** `ProfileSession.ThreadBootTemplate` 字段存在但未在代码中看到实际使用。会话开始时直接发送 Briefing，没有"启动仪式"。

**问题：** Anthropic 发现 Boot Sequence（读进度 → 查 git log → 跑 smoke test → 开始工作）对长期运行 Agent 至关重要。ai-workflow 的 Session Reuse 场景同样需要。

**建议方案：**

在 `BuildExecutionInputForStep()` 中，当 `handle.HasPriorTurns == true`（复用会话）时，在 Briefing 前注入 Boot 指令：

```
## Session Resumed — Status Check
Before starting new work:
1. Review your previous progress (check git log if available)
2. Verify no regressions from previous changes
3. Read the Gate feedback below if this is a rework
4. Then proceed with the objective
```

这利用了已有的 `reworkFollowupTemplate` / `continueFollowupTemplate` 机制——Boot Sequence 本质上是另一种 followup 模板。

---

### 4.5 P1: PreCompletion 验证

**现状：** Agent 通过 `SignalComplete` MCP 工具报告完成。引擎在 Finalize 阶段运行 Collector 提取元数据，然后在 Gate 步骤做验收。

**问题：** Agent 倾向于"写完就说 done"（LangChain 发现的最常见失败模式）。当前的验证在 Agent 完成之后（Gate 阶段），不在 Agent 完成之前。

**建议方案：**

在 `SignalComplete` MCP 工具的实现中增加前置检查：

```go
func handleSignalComplete(ctx context.Context, step *Step, payload map[string]any) error {
    // 检查 AcceptanceCriteria 中定义的必要条件
    if len(step.AcceptanceCriteria) > 0 {
        // 要求 Agent 在 payload 中提供每个 criteria 的验证结果
        for _, criteria := range step.AcceptanceCriteria {
            if _, ok := payload["criteria_"+criteria]; !ok {
                return fmt.Errorf("completion rejected: criteria '%s' not addressed in payload", criteria)
            }
        }
    }
    return nil
}
```

这让 `AcceptanceCriteria`（目前主要用于 Gate 步骤）也在 exec 步骤的完成时生效——Agent 必须逐项确认自己满足了要求才能说"done"。

---

### 4.6 P2: Trace 数据收集

**现状：** Event 系统记录了生命周期事件（started/completed/failed），但没有结构化的"Agent 决策轨迹"。

**问题：** 无法回答"为什么 Agent 在第 3 步选择了这种实现方式？"、"哪类任务失败率最高？"等分析问题。

**建议方案：**

在 `Execution.Output` 中已有 `text` / `stop_reason` / `tokens` 字段。可以扩展为：

```go
type ExecutionTrace struct {
    ToolCalls     []ToolCallRecord  // Agent 调用的每个工具
    TurnCount     int               // 对话轮数
    TokensInput   int64             // 输入 token
    TokensOutput  int64             // 输出 token
    Duration      time.Duration     // 执行时长
    RetryReason   string            // 如果是重试，原因是什么
}
```

存储在 `Execution.Output["trace"]` 中（利用已有的 `map[string]any` 结构）。这样不需要新建表，直接复用现有持久化。

---

### 4.7 P2: 熵管理（Maintenance Flow）

**现状：** 无。

**建议方案：**

将熵管理实现为特殊类型的 Flow——"maintenance flow"：

```toml
[[maintenance_flows]]
name = "weekly_hygiene"
schedule = "0 2 * * 1"  # 每周一凌晨 2 点
steps = [
    { name = "scan_dead_code", type = "exec", agent_role = "worker" },
    { name = "check_doc_consistency", type = "exec", agent_role = "worker" },
    { name = "review_findings", type = "gate", agent_role = "gate" },
]
```

这完全复用了现有的 Issue/Step/Gate/Execution 机制——不需要新的执行路径。唯一新增的是一个定时触发器。

---

## 五、CNCF 四大支柱对齐

| CNCF 支柱 | ai-workflow 现状 | 差距 |
|-----------|-----------------|------|
| **Golden Paths** | config.toml 预定义角色/模板；`default_template = "standard"` | 可增强：预设"黄金路径"模板库 |
| **Guardrails** | capabilities ≤ capabilities_max；Action 白名单 | 可增强：**成本上限**（token budget per Issue）、**时间上限**（global_timeout 已有但非强制） |
| **Safety Nets** | Recovery 机制；ErrorKind 分类重试；ExecutionProbe watchdog | 可增强：**熔断器**（连续 N 次失败自动暂停） |
| **Manual Review** | Permission Broker（Chat 场景）；`SignalNeedHelp` | 可增强：统一到 Gate 的 `awaiting_human` 路径 |

**最值得做的：** 为 Issue 增加 token/cost budget 上限。当前 `global_timeout` 控制时间，但没有控制成本。添加 `Issue.Metadata["token_budget"]` + 在 SessionPool 的 token 追踪中检查。

---

## 六、独特优势（HE 文献未覆盖）

ai-workflow 有几个能力在 Harness Engineering 主流文献中没有被讨论，但非常有价值：

### 6.1 Builtin 执行器（确定性操作剥离）

将 git commit/push 和 PR 创建实现为 builtin 而非 Agent 行为，意味着这些关键操作不受 Agent 幻觉影响。Agent 写代码，引擎推代码——职责完全分离。这比 OpenAI 让 Agent 自己跑 git 命令的做法更安全。

### 6.2 ExecutionProbe（运行时诊断）

通过 ACP 侧信道询问 Agent "你还活着吗？"、"你被阻塞了吗？"，得到 alive/blocked/hung/dead 的判决。这是 Harness Engineering 文献中几乎没有讨论的**运行时健康检查**能力。LangChain 的 LoopDetectionMiddleware 只在 harness 侧检测，不直接询问 Agent。

### 6.3 Signal 系统（Agent 主动反馈）

Agent 不只是被动接受 Briefing 和输出 Artifact，还可以主动发信号——`NeedHelp`（我遇到困难）、`Blocked`（我被阻塞了）、`Progress`（中间进度）。这比 OpenAI 的单向 "Agent 生成 PR" 模型更丰富。

### 6.4 Gate 的 Merge 自动化

Gate 不仅做代码审查判决，还负责 PR 合并。合并失败时自动记录 `MergeError`（含 PR#、状态、提示），触发拒绝让 Agent 修复冲突。这把 SCM 集成内化到了反馈回路中。

---

## 七、优先级总结

| 优先级 | 方向 | 预估改动量 | 价值 |
|--------|------|-----------|------|
| **P0** | 项目级工程知识注入（ContextRefType + BriefingBuilder） | 小（~100 行） | 高：Agent 代码合规性 |
| **P0** | Context Compaction（session 溢出处理） | 小（~50 行） | 高：长会话稳定性 |
| **P1** | Boot Sequence（复用会话时注入恢复指令） | 小（~30 行） | 中：复用会话质量 |
| **P1** | PreCompletion 验证（SignalComplete 前置检查） | 小（~40 行） | 中：减少"假完成" |
| **P1** | Loop Detection（事件流中检测循环行为） | 中（~150 行） | 中：减少 token 浪费 |
| **P2** | Trace 数据收集（Execution.Output 扩展） | 小（~60 行） | 长期：优化数据 |
| **P2** | Token/Cost Budget per Issue | 中（~100 行） | 中：成本控制 |
| **P2** | 熵管理 Maintenance Flow | 大（~300 行） | 低（当前）：代码库健康 |
| **P2** | 熔断器（连续失败自动暂停） | 小（~50 行） | 中：异常保护 |

---

## 八、结论

ai-workflow 不是"需要加装 Harness"的项目——**它本身就是一个 Harness**。Gate/Signal/Briefing/Capabilities/Recovery/Composite 构成了完整的约束-反馈-生命周期管理系统。

与 OpenAI Codex 团队的做法比较：
- **OpenAI 的 Harness 面向代码库**（linter、结构测试、文档规范）
- **ai-workflow 的 Harness 面向执行流**（Briefing、Gate、信号量、Recovery）

两者互补而非替代。最有价值的融合方向是**将 OpenAI 风格的代码库级约束（AGENTS.md、工程规范）注入到 ai-workflow 的 Briefing 系统中**——让执行流 Harness 也感知到代码库 Harness 的存在。

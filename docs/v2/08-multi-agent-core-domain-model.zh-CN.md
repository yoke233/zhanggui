# 多 Agent 系统核心领域模型：从消息内核到协作操作系统

> **前置**: [06-agent-workspace](06-agent-workspace.zh-CN.md) — Agent 从固定 stage 走向常驻协作主体  
> **前置**: [07-thread-message-inbox-bridge](07-thread-message-inbox-bridge.zh-CN.md) — 交互层收敛为 Thread / Message / Inbox / Bridge  
> **目标**: 定义多 Agent 系统在未来长期演进中的**核心领域模型**，避免系统退化成“高级群聊 + 隐式工作流”  
> **结论**: pipeline 可以退居为一种内部执行 playbook，但系统中心不能是 pipeline，也不能只是消息系统，而必须是**协作中的工作本体**。

## 为什么需要 08

06 解决了一个方向问题：

- Agent 不应只是一次性函数调用
- 系统不应只会跑固定流水线
- 协作需要 Inbox、Gateway、Thread、Memory、Skills

07 又解决了一个实现问题：

- Thread 是上下文容器
- Message 是正式时间线
- Inbox 是投递层
- StatusUpdate 是侧带过程态
- Bridge 是外部平台适配层

但如果系统未来真的要成为“多 Agent 交互系统”，还缺一个更关键的层：

> **这些 Agent 到底是在围绕什么进行协作？**

如果没有一个显式的工作模型，系统最后很容易退化成：

- 大家都在 thread 里发消息
- 工作状态埋在自然语言里
- 决策靠 TL 或某个 agent 的短期记忆维持
- 产物散落在 message content / metadata / tool output 中
- 回放、治理、授权、审计越来越困难

所以，08 的任务不是再发明一种消息类型，而是要回答：

> **一个多 Agent 系统的核心对象应该是什么，它们的边界在哪里，它们之间怎样协作？**

## 核心判断

### 1. 系统中心不是 pipeline

pipeline 可以保留，但它只能是：

- 一种内部执行策略
- 一种可复用的 playbook
- 一种在标准任务上很高效的执行器

它不应该继续是整个系统的中心抽象。

未来系统的中心应该是：

```text
Actor 围绕 Work 在 Thread 中协作，
通过 Execution 产生 Artifact，
并由 Review / Decision 改变正式状态。
```

### 2. 系统中心也不是消息系统

消息系统是必要基础，但不是业务中心。

如果只把系统设计成：

- Actor
- Thread
- Message
- Inbox

那最后得到的只是一个很强的“agent 群聊平台”。

真正可持续的多 Agent 系统，还必须显式拥有：

- **Work**：大家在做什么
- **Decision**：谁做了什么权威判断
- **Artifact**：协作产出了什么可复用结果
- **Execution**：这些工作是如何被执行的

### 3. 真正的中心是“协作中的工作”

因此，未来的核心模型应该围绕四个并列域展开：

1. **协作域**：谁在和谁交流，交流发生在什么上下文里
2. **工作域**：大家共同推进什么任务，状态如何变化
3. **执行域**：任务如何被真正执行，消耗什么资源，产生什么过程记录
4. **记忆域**：系统如何把长期有价值的信息沉淀下来，为后续协作服务

这四个域中，**工作域**才是系统的业务中心；其余三个域为其服务。

## 设计目标

### 要支持的能力

- Human / Agent / Service 在统一模型中协作
- Thread 共享上下文，但不等于自动广播
- Task 显式存在，不隐藏在自然语言里
- Assignment / Review / Decision 显式存在，不依赖隐式约定
- Artifact 作为共享产物被跟踪、引用、复用
- Execution 可替换：pipeline 只是其中一种
- 关键状态必须可从存储层恢复，而不是依赖运行中进程记忆
- 外部平台桥接不污染核心领域模型

### 明确不做的事

- 不做聊天产品级能力中心
- 不做“所有状态都交给 LLM 自由推理”
- 不把 memory 当成主状态源
- 不要求每个 Actor 都必须是一个长期常驻进程
- 不把 pipeline 强行塞回核心领域模型中心

## 边界与术语

为避免未来语义继续混乱，本设计统一以下术语：

| 术语 | 含义 | 不是 |
|------|------|------|
| **Actor** | 逻辑参与者身份，可是 human / agent / service | 不是进程实例 |
| **Thread** | 一段协作上下文 | 不是任务本身 |
| **Message** | 进入正式时间线的一条沟通记录 | 不是任务状态 |
| **Task** | 一项可被分配、推进、验收的工作单元 | 不是聊天串 |
| **Execution** | 完成 Task 的一次执行尝试 | 不是 Task 本身 |
| **Artifact** | 协作产生的共享产物 | 不是 message 附注 |
| **Decision** | 对某个对象产生正式约束的判断 | 不是普通意见 |
| **Memory** | 可复用经验与摘要 | 不是唯一真相源 |

## 核心领域划分（Bounded Contexts）

### 1. 协作域（Collaboration Context）

负责：

- 参与者身份
- 讨论上下文
- 消息时间线
- 投递和桥接

核心对象：

- `Actor`
- `Thread`
- `Message`
- `InboxDelivery`

### 2. 工作域（Work Context）

负责：

- 工作项建模
- 分工与责任归属
- 审核与决策
- 共享产物的业务归属

核心对象：

- `Task`
- `Assignment`
- `Review`
- `Decision`
- `Artifact`

### 3. 执行域（Execution Context）

负责：

- 某个 Task 的实际执行过程
- 执行策略（pipeline / interactive / delegated / batch）
- 工作目录和工具调用
- 运行时过程记录

核心对象：

- `AgentDefinition`
- `Execution`
- `WorkspaceLease`
- `ToolAction`

### 4. 记忆域（Memory Context）

负责：

- Thread / Task / Execution 的摘要压缩
- 可复用经验沉淀
- 偏好与长期约束记录

核心对象：

- `MemoryEntry`

## 核心对象定义（v1）

本节定义 v1 版本建议保留的 14 个核心对象。

### 1. `Actor`

**定义**：系统中的逻辑参与者身份。

可取类型：

- `human`
- `agent`
- `service`

关键职责：

- 持有稳定身份
- 参与 thread
- 接收 assignment
- 发起或响应 decision / review

关键属性：

- `id`
- `type`
- `display_name`
- `capability_profile`
- `authority_profile`
- `status`

注意：

> `Actor` 是“谁”，不是“怎么运行”。

### 2. `AgentDefinition`

**定义**：对 `actor.type = agent` 的运行画像定义。

内容包括：

- runtime 绑定
- instruction
- skills
- capability 上限
- session policy
- cost / safety / approval policy

注意：

- `AgentDefinition` 是静态画像
- 一个 agent actor 可以对应多个执行中的 runtime session
- 它不是一条 thread，也不是一个 task

### 3. `Thread`

**定义**：一段协作上下文的容器。

职责：

- 承载正式消息时间线
- 维护参与者快照
- 作为外部 bridge 的挂载点
- 允许 future summary / hydration

非职责：

- 不负责任务完成状态
- 不自动广播给所有参与者
- 不直接代表 authority

一句话：

> `Thread` 管上下文，不管工作完成。

### 4. `Message`

**定义**：正式进入 thread 时间线的一条消息。

职责：

- 记录明确沟通内容
- 保存结构化上下文 metadata
- 作为回复、转述、决议通知的载体

非职责：

- 不保存 tool 过程态
- 不等于任务状态机
- 不应该独自承担 artifact 存储

### 5. `InboxDelivery`

**定义**：某条消息被投递给某个 actor 的待处理记录。

职责：

- 记录送达关系
- 记录 claim / handled / failed 等处理状态
- 引用处理结果消息或错误

非职责：

- 不复制消息本体
- 不代表 thread 成员关系
- 不代表任务指派

### 6. `Task`

**定义**：一项可被明确拥有、执行、审核、关闭的工作单元。

这是未来系统的**业务中心对象**。

职责：

- 表达“要完成什么”
- 拥有正式状态机
- 作为 assignment / review / execution / artifact 的锚点

建议关键属性：

- `id`
- `title`
- `goal`
- `status`
- `owner_actor_id`
- `coordinator_actor_id`
- `parent_task_id`
- `depends_on`
- `acceptance_criteria`
- `priority`

注意：

> `Task` 才是“工作是否完成”的真相源，不是 thread。

### 7. `Assignment`

**定义**：一个 actor 对某个 task 所承担的一段责任关系。

职责：

- 表达谁负责做什么
- 表达角色：implement / review / coordinate / support
- 表达接受、拒绝、完成、移交等状态

关键属性：

- `task_id`
- `actor_id`
- `assignment_role`
- `scope`
- `status`
- `assigned_by`

注意：

> `Assignment` 是责任关系，不是消息，也不是执行过程。

### 8. `Review`

**定义**：对某个 Task / Artifact / Execution 的结构化质量审查。

职责：

- 保存 reviewer 身份
- 给出 verdict / findings / fixes
- 成为 decision 的输入之一

它应该是显式对象，而不是散落在 message 里的自然语言评论。

### 9. `Decision`

**定义**：对某个主题对象产生正式约束的判断结果。

例如：

- 接受一个 Task
- 批准一个 Assignment
- 通过一个 Review
- 关闭一个 Thread
- 接受一个 Artifact 版本
- 允许继续执行超预算任务

职责：

- 显式记录 authority
- 记录谁做出判断
- 记录依据、范围、时间

注意：

> 普通 message 是意见；`Decision` 才是正式约束。

### 10. `Artifact`

**定义**：协作过程中产生的共享产物。

典型包括：

- 代码改动
- 计划文档
- 设计稿
- 评审结论
- 需求摘要
- 数据集、截图、链接、补丁、PR、commit、测试报告

职责：

- 成为可引用对象
- 与 task / execution / review 建立清晰关联
- 支持版本与摘要

注意：

> Artifact 不应长期隐藏在 message body 或 tool output 里。

### 11. `Execution`

**定义**：为完成某个 task 而发起的一次执行尝试。

`Execution` 是对“怎么做”的抽象，不预设必须是 pipeline。

建议执行类型：

- `interactive`
- `pipeline`
- `delegated`
- `batch`
- `manual`

职责：

- 追踪执行状态
- 记录执行策略与输入输出
- 连接 workspace、tool actions、runtime session
- 产出 artifact

注意：

> `Execution` 是 Task 的执行载体；pipeline 只是 `Execution.kind = pipeline` 的一种情况。

### 12. `WorkspaceLease`

**定义**：一次 execution 在某个工作目录 / worktree / 沙箱上的临时占用关系。

这样建模的目的，是避免系统未来误把“workspace”设计成某个 actor 的永久附属物。

职责：

- 记录工作目录租约
- 记录租约开始/释放
- 绑定 execution，而非永远绑定 actor

一句话：

> workspace 是资源租约，不是人格属性。

### 13. `ToolAction`

**定义**：execution 内部发生的一次可观察工具操作。

职责：

- 审计工具调用
- 成本与耗时统计
- 故障排查与回放依据

非职责：

- 不直接决定 task 状态
- 不自动成为正式 artifact

### 14. `MemoryEntry`

**定义**：从 thread / task / execution / decision 中提炼出的长期可复用记忆。

类型可以包括：

- `summary`
- `pattern`
- `preference`
- `lesson`
- `constraint`

关键原则：

- 必须能追溯来源对象
- 不能成为唯一真相源
- 只能辅助理解与召回，不能覆盖显式状态

## 聚合边界（Aggregate Boundaries）

为了让系统长期可维护，必须明确哪些对象是聚合根，哪些是附属或引用对象。

### 1. `Thread` 聚合

包含：

- `Thread`
- `Message`
- `InboxDelivery`

规则：

- thread 管上下文一致性
- message append-only
- inbox 只表达投递，不表达工作状态

### 2. `Task` 聚合

包含：

- `Task`
- `Assignment`
- `Review`
- `Decision`（以 task 为主题时）

规则：

- task 状态变化必须显式
- assignment 变化不能只靠 message 推断
- review / decision 不应散落在 thread 自然语言中作为唯一记录

### 3. `Execution` 聚合

包含：

- `Execution`
- `WorkspaceLease`
- `ToolAction`

规则：

- execution 只记录“怎么执行”
- 不直接取代 task
- 不直接取代 artifact

### 4. `Actor` / `AgentDefinition` 聚合

包含：

- `Actor`
- `AgentDefinition`（对 agent actor）

规则：

- actor 是稳定身份
- agent definition 是静态能力与指令画像
- 运行时 session 属于 execution，不属于 actor 的永久实体状态

### 5. `MemoryEntry` 聚合

包含：

- `MemoryEntry`

规则：

- 一律追溯来源
- 一律低于 task / decision / artifact 的真相等级

## 真相源（Source of Truth）分配

这是未来最重要的不变量之一。

| 关心的问题 | 真相源对象 | 不应该依赖 |
|-----------|-----------|-----------|
| 当前讨论上下文是什么 | `Thread` | Agent 内存 |
| 某条正式沟通说了什么 | `Message` | Tool log |
| 谁需要处理这条消息 | `InboxDelivery` | Thread participants |
| 任务当前状态是什么 | `Task` | Message 文本 |
| 谁对任务负责 | `Assignment` | Thread 中的口头约定 |
| 审核结论是什么 | `Review` | Chat 回复 |
| 正式批准/驳回是什么 | `Decision` | 普通意见 |
| 产物是什么 | `Artifact` | Message 附带长正文 |
| 这次执行做了什么 | `Execution` + `ToolAction` | 只靠 thread 回忆 |
| 长期经验是什么 | `MemoryEntry` | 临时 session |

## 建议状态机（v1）

### `Task`

```text
proposed -> accepted -> assigned -> in_progress -> in_review -> done
                 \-> rejected
assigned -> blocked
in_progress -> blocked
blocked -> assigned / in_progress
任何阶段 -> cancelled
```

说明：

- `proposed`：工作被提出，但尚未成为正式承诺
- `accepted`：已进入正式工作池
- `assigned`：责任人明确，但未真正启动
- `in_progress`：有 execution 在推进
- `in_review`：核心产物已形成，等待 review / decision
- `done`：经过明确 decision 完成

### `Assignment`

```text
pending -> accepted -> active -> completed
pending -> rejected
active -> handed_off
active -> blocked
```

### `Execution`

```text
queued -> running -> completed
queued -> cancelled
running -> paused
running -> failed
paused -> running
failed -> queued (retry)
```

### `Thread`

```text
open -> closed -> archived
open -> archived
```

`Thread` 的关闭不等于 `Task` 完成；只是该讨论不再活跃。

## 关键关系

### 1. Thread 与 Task 是多对多关联，不应互相吞并

一个 task 可能关联多个 thread：

- 一个主讨论 thread
- 一个 reviewer thread
- 一个外部 bridge thread

一个 thread 也可能讨论多个紧密相关 task。

所以：

- `thread` 不是 `task`
- `task` 也不是 `thread`

推荐做法：

- `Task.primary_thread_id` 可选
- `Thread.related_task_ids` 或单独关联表更稳妥

### 2. Task 与 Execution 是一对多

一个 task 在生命周期中可以经历多次 execution：

- 第一次实现尝试失败
- 第二次 fixup 重试
- 第三次人工执行

这让系统天然支持：

- retry
- fallback
- 多策略执行
- pipeline 归档为老策略后继续保留历史兼容性

### 3. Artifact 不是 Message 的附件，而是显式对象

message 里可以引用 artifact，但 artifact 不应只活在 message 里。

否则后面会出现：

- 无法精确找到最新版本
- 无法把一个 artifact 复用于多个 task
- 无法把 review 精确挂到某个产物版本上

### 4. Decision 改变正式状态

未来应坚持：

- message 可以提出建议
- review 可以给出 verdict
- 但真正改变正式状态的，是 `Decision`

例如：

- `Task.done`
- `Assignment.accepted`
- `Artifact.accepted`
- `Execution.allowed_to_continue`

都应有显式 decision 或可审计的自动 decision 记录。

## 未来的 pipeline 放在哪里

pipeline 不再是系统中心，而是 execution 的一种策略。

### 推荐表达方式

```text
Task
  └── Execution(kind = "pipeline", playbook = "standard")
        ├── WorkspaceLease
        ├── ToolAction...
        └── Artifact...
```

同样，一个更开放的协作执行也可以是：

```text
Task
  └── Execution(kind = "interactive")
        ├── coordinator = actor-tl
        ├── assignments = [coder-01, reviewer-01]
        ├── thread = thread-auth-refactor
        └── artifacts = [...]
```

这就实现了两点：

1. **标准 pipeline 仍然保留**
2. **系统未来可以脱离 pipeline 继续演化**

## Authority 模型：未来不能只靠 TL 人工兜底

06 中 TL 作为默认治理中心是合理的，但未来如果系统要长大，authority 不能只靠“TL 记住一切”。

推荐把 authority 明确建模为：

- `owner_actor_id`：业务拥有者
- `coordinator_actor_id`：协作协调者
- `reviewer_actor_id` / `review_policy`
- `decision_policy`

这意味着：

- TL 可以是默认 coordinator
- 但架构上不应是唯一 coordinator
- 后续也能支持多个 team lead、多个 review board、甚至 service actor 自动做一部分 formal decision

## Memory 的正确位置

Memory 很重要，但不能放错位置。

### Memory 应该做的事

- thread 摘要
- task 背景压缩
- 用户偏好沉淀
- 执行经验复用
- 失败模式总结

### Memory 不应该做的事

- 代替 task 当前状态
- 代替 decision 正式结果
- 代替 artifact 真正内容
- 代替 thread 时间线

一句话：

> Memory 是“理解增强层”，不是“主状态层”。

## Ports / Adapters 建议

如果未来按 Ports & Adapters 演化，建议把端口大致拆成：

### 协作端口

- `ThreadRepository`
- `MessageRepository`
- `InboxRepository`
- `BridgePort`

### 工作端口

- `TaskRepository`
- `AssignmentRepository`
- `ReviewRepository`
- `DecisionRepository`
- `ArtifactRepository`

### 执行端口

- `ExecutionRepository`
- `WorkspacePort`
- `RuntimePort`
- `ToolAuditPort`

### 记忆端口

- `MemoryRepository`
- `SummaryCompressor`
- `RecallPort`

### 外部适配器

- Slack / Discord / Telegram bridge adapter
- Git / GitHub / PR adapter
- ACP / A2A / MCP runtime adapter
- Web / TUI / CLI / API adapter

关键原则：

> 外部系统永远是 adapter，不是核心领域模型的一部分。

## 未来最重要的不变量

为防止系统再次漂移，建议固定以下不变量：

1. **Thread 管上下文，不管完成状态**
2. **Task 管工作状态，不管完整对话历史**
3. **Message 进入正式时间线，StatusUpdate 不进入正式时间线**
4. **InboxDelivery 只管投递，不复制消息本体**
5. **Assignment 必须显式存在，不能只靠消息约定责任**
6. **Decision 才能改变正式状态，普通 message 不直接改状态**
7. **Artifact 必须是显式对象，不能长期埋在 message / tool output 中**
8. **Execution 是一次尝试，不能与 Task 合并**
9. **Workspace 是租约资源，不是 actor 的永久人格附属物**
10. **Memory 必须可追溯来源，且优先级低于显式状态对象**
11. **Bridge 是 adapter，不反向塑造领域核心**
12. **pipeline 只是 execution strategy，不再是系统根模型**

## 最小可行核心（MVP Core）

如果未来要分阶段推进，一个足够稳的最小核心可以是：

### P0 必须有

- `Actor`
- `Thread`
- `Message`
- `InboxDelivery`
- `Task`
- `Assignment`
- `Execution`
- `Artifact`
- `Decision`

### P1 强烈建议补齐

- `AgentDefinition`
- `Review`
- `WorkspaceLease`
- `ToolAction`
- `MemoryEntry`

### 可以后置

- 更复杂的 authority policy
- 更复杂的成员系统
- 更复杂的 bridge identity
- 更复杂的 artifact version graph

## 这份模型回答了什么

08 试图明确以下事情：

- 多 Agent 系统的中心不应再是 pipeline
- 也不应只是 thread/message/inbox 这类交互内核
- 真正的系统中心应该是**围绕 Task 的协作工作模型**
- pipeline 未来可以被归档为 execution strategy / internal playbook
- TL 可以继续作为默认协调者，但不应成为唯一架构中心

## 与 06 / 07 的关系

### 06 回答的是

- 为什么要从固定流水线走向常驻 Agent 协作
- 为什么需要 Gateway / Inbox / Memory / Skills

### 07 回答的是

- 交互层到底如何最小正确建模
- Thread / Message / Inbox / Bridge 怎么拆

### 08 回答的是

- 当系统真正走向多 Agent 协作时，**业务中心对象**到底是什么
- 消息、任务、执行、记忆、决策之间的边界如何固定
- pipeline 在未来总体架构中应该退到什么位置

因此，推荐阅读顺序是：

1. 先看 06，理解为什么要从 pipeline 走向多 Agent 协作
2. 再看 07，理解消息层如何收敛为最小正确模型
3. 最后看 08，理解未来系统应该围绕什么核心领域继续演化

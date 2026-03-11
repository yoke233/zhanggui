# v3 主文档 — 完整架构设计

> 来源：Notion MCP 抓取
>
> 页面：`v3 主文档 — 完整架构设计`
>
> URL：`https://www.notion.so/31d4a9d94a35812485d4de2cbbdb7845`
>
> 抓取时间：`2026-03-08T22:57:04.080Z`
>
> 说明：以下内容按 MCP 返回结果落地；其中 `2.7 Pattern` 之后到第 3 节之间存在 Notion MCP 返回的截断标记，本文件原样保留，不做补写。

## 1. 设计哲学

本系统的核心理念：**控制面极简，AI自主决策，接口隔离变化。**

控制面只当“邮局”——收消息、存状态、转消息。所有业务判断、任务分解、协作方式都由AI Agent自行决定。

**四条基本原则：**
- **任务是骨架**：所有Agent围绕Task工作，Task是一棵可递归分解的树
- **消息是血液**：Agent之间不直接调用，只通过收件箱异步通信
- **步骤是真相**：TaskStep是系统的唯一事实来源（Event Sourcing），Task状态从步骤序列派生
- **接口隔离变化**：在“会变的地方”切接口，初期用最简实现

**两条架构约束：**
- **不依赖消息顺序**：任何消息乱序到达都不会导致系统状态错误
- **至少一次投递**：消息可以重复投递，但不能丢失，通过幂等键去重

---

## 2. 领域模型

### 2.1 Agent（智能体）

```javascript
Agent
├── id                  唯一标识
├── name                名称
├── role                角色标签（AI自定义，非枚举）
├── parent_id           创建者，形成组织树
├── capabilities        能力描述（自然语言）
├── permissions         权限配置
├── status              idle / busy / waiting_approval
├── prompt_artifact_id  system prompt，存为Artifact可追溯
└── created_at
```

组织结构示例：

```javascript
human (老板)
├── supervisor_tech (技术主管)
│   ├── worker_dev_1
│   └── reviewer_code
├── supervisor_ops (运营主管)
├── supervisor_finance (财务主管)
└── analyst (模式分析师)
```

关键规则：Agent可动态创建，创建前必须向上级发approval_request。role和capabilities是AI自写的自然语言描述。Agent的system prompt本身是一个Artifact，可追溯、可审阅、可迭代。

### 2.2 Task（任务）

任务天然是一棵树——任何任务都可以被分解为子任务。

```javascript
Task
├── id / title / description
├── creator_id          谁创建的
├── owner_id            当前负责人（随流转变化）
├── status              状态机（物化视图，从TaskStep派生）
├── parent_task_id      父任务ID
├── children_mode       parallel / sequential / null
├── merge_strategy      子任务交付物汇总方式（自然语言描述）
├── participants        参与者（owner之外的协作者）
├── tags                自由标签
├── acceptance_criteria 验收条件（自然语言）
└── gates               [门禁规则]
```

**事件溯源规则：** Task.status不是独立存储的字段，而是从该Task最新的TaskStep推导出来的物化视图。**TaskStep是事实，Task.status是缓存。**

完成判定的递归规则：一个任务完成 = 自身工作完成 + 所有子任务完成。parallel模式下所有done才算完成；sequential模式下第N个done触发第N+1个。

### 2.3 Workspace（工作区）

系统不预定义类型，由AI自行描述。

```javascript
Workspace
├── id / agent_id / task_id
├── description       自然语言描述
├── resources         [{kind, uri}, ...]  AI自定义的资源引用
└── credentials_ref   凭证引用
```

resources中的kind是AI自己写的语义标签，不是枚举。系统不对kind做switch-case，只原样存储。

### 2.4 Artifact（交付物）

```javascript
Artifact
├── id / task_id / producer_id
├── description       自然语言描述
├── content_ref       [{kind, uri}, ...]  AI自定义的内容引用
├── summary           AI自己写的摘要
└── status            draft / submitted / approved / rejected / merged
```

没有Artifact的任务不算完成。Agent的system prompt也是Artifact。

### 2.5 Message（消息）

```javascript
Message
├── id / idempotency_key
├── from_agent_id / to_agent_id
├── type              枚举：chat / task_assign / task_complete / need_help / help_reply / approval_request / approval_granted / approval_rejected / review_request / sign_off
├── task_id           关联任务（闲聊时为空）
├── pre_condition     期望的task status前置条件
├── content / artifact_id / actions
└── created_at
```

系统不依赖消息到达顺序。每条Message通过pre_condition声明它期望的task状态，不匹配则重新排队。idempotency_key保证同一消息只处理一次。

### 2.6 TaskStep（系统唯一事实来源）

```javascript
TaskStep
├── id / task_id / agent_id
├── action            created / assigned / started / need_help / help_replied / completed / review_requested / review_passed / review_rejected / signed_off / gate_check
├── input / output / artifact_id / note
├── decision_ref      关联的Decision记录
├── duration_ms
└── created_at
```

写入路径：任何状态变更 → 写TaskStep → 派生更新Task.status缓存 → 发Message通知

### 2.7 Pattern（模式模板）

由analyst agent从历史…1605 chars truncated…td>三级记忆+RAG</td>
</tr>
<tr>
<td>Executor</td>
<td>活怎么干</td>
<td>LLM直接生成</td>
<td>工具链+专业系统</td>
</tr>
<tr>
<td>Merger</td>
<td>结果怎么合</td>
<td>简单拼接</td>
<td>AI整合+专业工具</td>
</tr>
<tr>
<td>Store</td>
<td>数据存哪</td>
<td>SQLite</td>
<td>PostgreSQL</td>
</tr>
<tr>
<td>Bus</td>
<td>消息怎么投递</td>
<td>Go channel</td>
<td>Redis Stream</td>
</tr>
<tr>
<td>Notifier</td>
<td>通知怎么发</td>
<td>控制台日志</td>
<td>飞书/钉钉</td>
</tr>
<tr>
<td>WorkspaceProvider</td>
<td>环境怎么建</td>
<td>本地目录</td>
<td>Docker/K8s</td>
</tr>
<tr>
<td>DecisionValidator</td>
<td>决策怎么校验</td>
<td>状态机校验</td>
<td>可插拔规则引擎</td>
</tr>
<tr>
<td>Watchdog</td>
<td>异常怎么发现</td>
<td>定时扫描</td>
<td>实时监控+告警</td>
</tr>
</table>

核心接口签名：

```go
type Thinker interface {
    Decide(ctx AgentContext, msg Message) (Decision, error)
}

type Memory interface {
    Recall(agentID string, taskID string, limit int) ([]MemoryItem, error)
    Store(agentID string, item MemoryItem) error
    Compact(agentID string) error
}

type Executor interface {
    Execute(workspace Workspace, instruction string, checkpoint *Checkpoint) (Artifact, *Checkpoint, error)
}

type Bus interface {
    Register(agentID string) (<-chan Message, error)
    Send(msg Message) error
    Ack(idempotencyKey string) error
    Unregister(agentID string) error
}

type Store interface {
    SaveTaskStep(step TaskStep) error      // 写入步骤，内部自动更新Task.status缓存
    RebuildTaskStatus(taskID string) (string, error)  // 从TaskStep重建状态
    IsMessageProcessed(key string) (bool, error)
    MarkMessageProcessed(key string) error
    // ... 其他实体的CRUD
}
```

---

## 4. Agent角色体系

<table header-row="true">
<tr>
<td>角色</td>
<td>职责</td>
</tr>
<tr>
<td>Supervisor</td>
<td>接收指令，分解任务，分配执行者，最终签收</td>
</tr>
<tr>
<td>Worker</td>
<td>在workspace中完成具体工作，产出artifact</td>
</tr>
<tr>
<td>Reviewer</td>
<td>审阅artifact，给出通过/打回</td>
</tr>
<tr>
<td>Analyst</td>
<td>扫描TaskStep，发现重复模式，提议Pattern</td>
</tr>
</table>

角色不是枚举，是语义标签。AI可以自己定义新角色。

---

## 5. 类型演化策略

**AI自由填写的字段（不枚举）：** Agent.role, capabilities, Workspace.description, resources\[\].kind, Artifact.description, content_ref\[\].kind, merge_strategy.description

**系统必须枚举的字段：** Task.status, children_mode, Message.type, Artifact.status, Agent.status, AgentError.type

原则：系统需要做分支判断的必须枚举，统AI看的语义标签用自然语言。

---

## 6. 已知问题与解决方案

<table header-row="true">
<tr>
<td>问题</td>
<td>解法</td>
</tr>
<tr>
<td>上下文窗口爆炸</td>
<td>三级记忆架构（热/温/冷），初期只取最近N条</td>
</tr>
<tr>
<td>LLM不确定性</td>
<td>Decision结构化 + DecisionValidator硬规则校验</td>
</tr>
<tr>
<td>成本失控</td>
<td>TieredThinker分级 + 确定性路径不调LLM</td>
</tr>
<tr>
<td>死循环和死锁</td>
<td>Watchdog定时巡检 + 超时升级</td>
</tr>
<tr>
<td>自然语言描述熵增</td>
<td>Analyst归纳推荐标签，延迟解决</td>
</tr>
<tr>
<td>Merge质量</td>
<td>分离合并和验证，确定性校验脚本</td>
</tr>
<tr>
<td>Agent质量</td>
<td>Prompt即Artifact + Decision版本化追溯</td>
</tr>
<tr>
<td>错误恢复</td>
<td>幂等 + Checkpoint + AgentError三类型</td>
</tr>
<tr>
<td>权限边界</td>
<td>AgentPermission + ResourceQuota</td>
</tr>
<tr>
<td>Human瓶颈</td>
<td>审批分级 + 授权衰减</td>
</tr>
<tr>
<td>测试验证</td>
<td>行为录制 + 回放对比</td>
</tr>
<tr>
<td>可观测性</td>
<td>Dashboard Agent定期生成简报</td>
</tr>
</table>

---

## 7. 实施路径

<table header-row="true">
<tr>
<td>阶段</td>
<td>目标</td>
</tr>
<tr>
<td>Phase 0</td>
<td>最小闭环：1个supervisor + 1个worker，跑通完整链路</td>
</tr>
<tr>
<td>Phase 1</td>
<td>多agent协作：加Reviewer + 动态创建 + Validator</td>
</tr>
<tr>
<td>Phase 2</td>
<td>并行与合并：子任务拆分 + Merger + Watchdog</td>
</tr>
<tr>
<td>Phase 3</td>
<td>自进化：Analyst + Pattern + 授权衰减 + Dashboard</td>
</tr>
<tr>
<td>Phase 4</td>
<td>生产化：三级记忆 + PG + Docker + 企业IM</td>
</tr>
</table>

---

## 8. 设计决策备忘

<table header-row="true">
<tr>
<td>决策</td>
<td>结论</td>
<td>理由</td>
</tr>
<tr>
<td>要不要用工作流引擎</td>
<td>不用</td>
<td>太重，AI自主决策不需要外部编排</td>
</tr>
<tr>
<td>要不要提前定义DSL</td>
<td>不做</td>
<td>模式还没稳定，过早固化会限制演化</td>
</tr>
<tr>
<td>Task.status怎么维护</td>
<td>事件溯源</td>
<td>TaskStep是唯一事实来源</td>
</tr>
<tr>
<td>消息能不能乱序</td>
<td>可以</td>
<td>pre_condition保障，未来可换任何消息中间件</td>
</tr>
<tr>
<td>消息投递语义</td>
<td>at-least-once + 幂等</td>
<td>不丢消息比不重复更重要</td>
</tr>
<tr>
<td>Decision要不要版本化</td>
<td>要</td>
<td>行为异常时必须能追溯到具体prompt和模型</td>
</tr>
<tr>
<td>错误怎么分类</td>
<td>transient/permanent/need_help</td>
<td>runtime需要知道该重试、该放弃、还是该求助</td>
</tr>
</table>

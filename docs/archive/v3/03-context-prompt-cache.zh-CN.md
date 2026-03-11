# 上下文与Prompt缓存策略详细设计

> 来源：Notion MCP 抓取
>
> 页面：`上下文与Prompt缓存策略详细设计`
>
> URL：`https://www.notion.so/31d4a9d94a358123ae4ff53a64c7e23c`
>
> 抓取时间：`2026-03-08T23:30:54.614Z`

## 1. 问题

Agent每次决策都需要调LLM，每次调用都需要拼装一段prompt作为上下文。这里有两层缓存机会：
- **LLM Prefix Cache**：大部分LLM API支持prefix caching，如果两次请求的prompt前缀相同，前缀部分的计算可以复用，省时省钱
- **应用层Memory Cache**：Memory.Recall每次都查数据库，相同查询可以缓存

核心挑战：prompt中间插了动态内容，会把后面所有内容的prefix cache打碎。

---

## 2. Prompt排列原则：按变化频率从低到高

**错误的排列（cache命中率低）：**

```javascript
1. Agent身份（固定）         ← cache命中 ✓
2. 任务信息（较稳定）        ← cache命中 ✓
3. 当前消息（每次不同）      ← 从这里开始cache全部失效 ✗
4. 热上下文（动态）          ← 失效 ✗
5. 温上下文（动态）          ← 失效 ✗
6. 冷上下文（极少变）        ← 明明很稳定但也失效了 ✗
```

当前消息插在中间，把后面所有内容的cache都打碎了。

**正确的排列（最大化cache命中）：**

```javascript
1. Agent身份                几乎不变      ← cache命中
   system prompt、角色、能力、权限
── cache boundary 1 ──
2. 冷上下文                 很少变        ← cache命中
   压缩后的历史摘要
── cache boundary 2 ──
3. 任务信息                 任务周期内稳定  ← cache命中
   title, description,
   acceptance_criteria, gates
── cache boundary 3 ──
4. 温上下文                 偶尔变
   父任务摘要、兄弟任务状态
5. 热上下文                 经常变
   本task最近N条消息和TaskStep
6. 当前消息                 每次都变
   触发本次决策的message
```

同一个agent处理同一个task的连续多次决策，1-3的cache几乎都能命中。以128k context window为例，1-3可能占30-40%，这部分全部cache命中，成本和延迟直接省掉一大块。

---

## 3. 应用层分层缓存

Memory.Recall的分层缓存策略：

<table header-row="true">
<tr>
<td>层级</td>
<td>内容</td>
<td>缓存策略</td>
<td>失效条件</td>
</tr>
<tr>
<td>冷层</td>
<td>agent的历史摘要（Compact产出）</td>
<td>几乎永远命中</td>
<td>下一次Compact执行</td>
</tr>
<tr>
<td>温层</td>
<td>父任务摘要、兄弟任务状态</td>
<td>版本号控制失效</td>
<td>相关task的状态变更</td>
</tr>
<tr>
<td>热层</td>
<td>当前task最近N条消息和TaskStep</td>
<td>不缓存，每次实时查</td>
<td>—</td>
</tr>
</table>

```go
type CachedMemory struct {
    // 冷层缓存：Compact之后就不变了
    coldCache   map[string][]MemoryItem    // key: agentID

    // 温层缓存：版本号控制失效
    warmCache   map[string][]MemoryItem    // key: agentID+parentTaskID
    warmVersion map[string]int64           // 版本号

    // 热层不缓存：每次实时查数据库
}
```

---

## 4. 版本号传播机制

温层缓存的失效由版本号驱动，版本号跟着TaskStep的写入自然递增。

```javascript
task_001 (version: 5)
├── task_001_1 (version: 3)
├── task_001_2 (version: 7)   ← 状态变更
└── task_001_3 (version: 2)

task_001_2写入新TaskStep时：
  task_001_2.version = 8
  task_001.version = 6        ← 父任务版本也递增
```

温层缓存检查的是父任务的版本号。任何子任务变更都会让父任务版本递增，温层缓存自动失效，下次Recall时重建。

**不需要定时过期、不需要手动清缓存、不需要广播失效通知。** 版本号跟着TaskStep的写入自然递增，缓存跟着版本号自然失效。和事件溯源一脉相承——写入是唯一的变更来源，缓存的失效也由写入驱动。

Store接口新增：

```go
// 每次SaveTaskStep时，递增该task及其父任务的版本号
GetTaskTreeVersion(taskID string) (int64, error)
```

---

## 5. Compact指纹

冷层缓存的前提是Compact之后摘要不变。需要知道什么时候该重新Compact：

```go
type CompactMeta struct {
    AgentID       string
    LastCompactAt time.Time
    RecordCount   int       // Compact时的记录数
    Fingerprint   string    // 摘要内容的hash
}
```

每次Recall冷层时，检查当前记录数是否超过上次Compact时的记录数。超过阈值则触发重新Compact，Compact后更新fingerprint，冷层缓存随之刷新。

触发条件：该agent在该task下的记录超过50条时，压缩前40条为摘要，保留最近10条原文。

---

## 6. PromptBuilder

把所有缓存策略封装成一个PromptBuilder，对Thinker透明：

```go
type PromptBuilder struct {
    memory    Memory
    maxTokens int
}

func (b *PromptBuilder) Build(agent Agent, task *Task, msg Message) Prompt {
    // 固定部分
    identity := b.buildIdentity(agent)

    // 按变化频率从低到高拼装
    cold     := b.memory.RecallCold(agent.ID)
    taskInfo := b.buildTaskInfo(task)
    warm     := b.memory.RecallWarm(agent.ID, task.ID)
    hot      := b.memory.RecallHot(agent.ID, task.ID)
    current  := b.buildCurrentMessage(msg)

    // token预算分配
    budget := b.maxTokens - b.countTokens(identity) - b.countTokens(current)
    cold     = b.truncate(cold,     int(float64(budget) * 0.15))
    taskInfo = b.truncate(taskInfo,  int(float64(budget) * 0.10))
    warm     = b.truncate(warm,     int(float64(budget) * 0.25))
    hot      = b.truncate(hot,      int(float64(budget) * 0.50))

    return Prompt{
        System:   identity,
        Messages: concat(cold, taskInfo, warm, hot, current),
    }
}
```

`Thinker`只调`PromptBuilder.Build`拿到拼好的prompt，不关心缓存策略。换缓存方案只改`PromptBuilder`，`Thinker`接口不变。

---

## 7. token预算分配

<table header-row="true">
<tr>
<td>层级</td>
<td>占比</td>
<td>内容</td>
</tr>
<tr>
<td>冷上下文</td>
<td>15%</td>
<td>压缩后的历史摘要</td>
</tr>
<tr>
<td>任务信息</td>
<td>10%</td>
<td>title, description, criteria, gates</td>
</tr>
<tr>
<td>温上下文</td>
<td>25%</td>
<td>父任务摘要、兄弟任务状态</td>
</tr>
<tr>
<td>热上下文</td>
<td>50%</td>
<td>当前task最近N条消息和TaskStep</td>
</tr>
</table>

identity（system prompt）和当前消息不计入动态预算，作为固定开销单独扣除。

初期比例硬编码，后期可根据任务类型动态调整（如讨论型task热上下文占比更高，长期跟踪型task温上下文占比更高）。

---

## 8. 三层保障总结

```javascript
1. Prompt排列顺序
   稳定的放前面，动态的放后面 → 最大化LLM prefix cache命中率

2. 应用层分层缓存
   冷层永远命中、温层版本号控制失效、热层不缓存实时查

3. 版本号传播
   子任务变更 → 父任务版本递增 → 温层缓存自动失效
   Compact执行 → fingerprint更新 → 冷层缓存自动刷新
```

不需要定时过期、不需要手动清缓存、不需要广播失效通知。写入驱动一切。

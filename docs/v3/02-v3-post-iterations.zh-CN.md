# 补充文档 — v3之后的迭代

> 来源：Notion MCP 抓取
>
> 页面：`补充文档 — v3之后的迭代`
>
> URL：`https://www.notion.so/31d4a9d94a35813da4c1c1921acf8f46`
>
> 抓取时间：`2026-03-08T22:59:40.671Z`

## 1. 架构简化：Session合并入Task

原计划为“会议/讨论”场景引入独立的Session模型。经分析发现，Session和Task结构几乎一样——“开会”和“干活”对系统来说没有区别，都是“一群agent协作产出一个artifact”。

**删除的模型：** Session、SessionRound、Message.session_id

讨论就是Task：
- “写后端API” = Task → owner干活 → 产出代码artifact
- “讨论技术方案” = Task → 多个agent交换chat消息 → 产出方案文档artifact
- “辩论主账号方案” = Task → 正反方发言 → 产出决策文档artifact

并行讨论直接使用Task的`children_mode: parallel`：

```javascript
task_000 (根任务: "用户注册功能")
├── task_001 (需求讨论阶段, children_mode: parallel)
│   ├── task_001_1 "技术方案讨论"
│   ├── task_001_2 "需求细节梳理"
│   └── task_001_3 "测试策略讨论"
│   → 全部完成后 merge → 产出完整计划 → 发给human审批
├── task_002 (开发阶段, children_mode: parallel)
└── task_003 (审阅)
```

Human的完整体验：一句话发需求 → AI自己讨论+规划 → 收到“讨论完了，要开始吗？” → 点approve → 每天收日报 → 一周后收到“完成”通知

---

## 2. 从闲聊到任务的结晶

human与supervisor闲聊，Message.task_id为空。到结晶点，supervisor的Decision为`crystallize`：同时生成需求文档Artifact、创建Task、挂载附件。

触发方式：human主动（“就这么定了”）或supervisor主动提议（附需求文档，带actions: `["approve_and_start", "modify"]`）。

---

## 3. 定时任务（Schedule）

```javascript
Schedule
├── id / name
├── cron                "0 18 * * *"
├── owner_id
├── template            触发时生成的消息模板
│   ├── to / type / content / task_ref
├── status              active / paused / expired
├── start_date / end_date / last_triggered
└── created_at
```

示例：跟踪一件事情3个月，每天发日报。每天18点系统自动给supervisor发一条消息，supervisor查TaskStep表汇总当天进展，整理成日报artifact发给human。

触发机制：一个Scheduler组件（goroutine + ticker），幂等键带日期防重复触发。

---

## 4. 任务标签（Tags）

Task新增`tags []string`字段，如`["战略:用户增长", "产品线:主站", "Q2"]`。

不做层级分类，只做自由标签。层级会变，标签足够灵活，分组方式是看板的事不是数据的事。

human手动打或AI自动打。Analyst可归纳标签规范为Pattern。

---

## 5. 门禁（Gate）

### 5.1 Gate结构

```javascript
Gate
├── name              "方案完整性" / "单测通过" / "代码审阅"
├── type              auto / owner_review / peer_review / vote
├── rules             自然语言描述
├── config            规则参数
├── max_attempts      最多检查几次
└── fallback          escalate / force_pass / abort
```

### 5.2 四种门禁类型

<table header-row="true">
<tr>
<td>类型</td>
<td>说明</td>
<td>适用场景</td>
</tr>
<tr>
<td>auto</td>
<td>owner自己判断是否达标</td>
<td>简单任务，如单测通过</td>
</tr>
<tr>
<td>owner_review</td>
<td>主持人/负责人判定</td>
<td>讨论类task的默认模式</td>
</tr>
<tr>
<td>peer_review</td>
<td>所有参与者互审</td>
<td>需要共识的讨论</td>
</tr>
<tr>
<td>vote</td>
<td>投票表决</td>
<td>有明确分歧时</td>
</tr>
</table>

### 5.3 门禁串联

一个task可以有多道门禁，按顺序检查：

```javascript
gates: [
    Gate{name: "单测", type: "auto", rules: "所有单元测试通过"},
    Gate{name: "代码审阅", type: "peer_review", rules: "reviewer通过"},
    Gate{name: "主管签收", type: "owner_review", rules: "确认符合需求"},
]
```

### 5.4 门禁与状态机的关系

gate_check发生在in_progress内部，不是新状态：
- pass → pending_review
- continue → 留在in_progress（继续干/讨论）
- fail → rejected或escalate

TaskStep记录为`action: "gate_check"`，完整可追溯。

max_attempts + fallback组合兆底，防止无限循环。

### 5.5 讨论场景示例

```javascript
Task{
    title: "讨论用户注册技术方案",
    participants: [dev_1, dev_2, test_1],
    acceptance_criteria: "产出技术方案文档，包含注册方式、认证方案、工期估算",
    gates: [
        {name: "方案完整性", type: "owner_review", max_attempts: 5, fallback: "escalate"},
        {name: "参与者认可", type: "peer_review", config: {require: "all"}}
    ]
}

执行过程:
  第1轮讨论 → gate_check → continue（缺工期估算）
  第2轮讨论 → gate_check → continue（认证方案有分歧）
  第3轮讨论 → gate_check → pass（3项都有结论）
  → 进入peer_review → 全票通过 → 产出artifact
```

---

## 6. Dashboard数据区

Dashboard agent不直接读写业务表，拥有专属KV存储区：

```javascript
DashboardData
├── key               "daily_summary_2026-03-09"
├── value             JSON（dashboard agent自己定义结构）
├── producer_id       必须是dashboard agent
└── expires_at        过期时间
```

权限：对业务表只读，对DashboardData读写。最坏情况就是看板数据不对，删掉重新生成，不污染业务数据。

向Dashboard提需求完全走现有架构，它就是一个普通的agent，配合Schedule定时触发。

---

## 7. 上下文管理

### 7.1 隔离单位

上下文的隔离边界是 **Agent × Task** 的交叉点：

```javascript
ContextID = AgentID + TaskID
```

一个agent可能同时参与多个task，一个task可能有多个agent参与。每个格子是一个独立的上下文空间。

### 7.2 上下文不是“一直存在的”

LLM没有持久记忆。每次调用Thinker.Decide，上下文都是从零拼装的。Memory接口从数据库里捣出相关记录，塞进prompt里。

### 7.3 Prompt结构（按变化频率从低到高排列，最大化prefix cache命中率）

```javascript
1. Agent身份       几乎不变     ← cache命中
2. 冷上下文       很少变       ← cache命中
3. 任务信息       任务周期内稳定 ← cache命中
4. 温上下文       偶尔变
5. 热上下文       经常变
6. 当前消息       每次都变
```

### 7.4 任务分裂时上下文流转

子任务不继承父任务的完整上下文。子任务的agent拿到的是一个精炼过的输入（task_assign消息的内容），而不是父任务的全部讨论历史。

信息跨task的流转靠消息传递，不靠共享上下文。父任务的信息对子任务是摘要式可见，不是全量可见。

### 7.5 缓存策略

- **冷层**：几乎永远命中，Compact后才失效
- **温层**：版本号控制失效（子任务变更 → 父任务版本递增 → 温层缓存失效）
- **热层**：不缓存，每次实时查

版本号跟着TaskStep的写入自然递增，缓存的失效也由写入驱动。和事件溯源一脉相承。

### 7.6 token预算分配

60%给当前task的直接记录，25%给父任务和兄弟任务的摘要，15%给跨task的关联信息。初期硬编码，后期可动态调整。

---

## 8. 完整模型变更清单

### 新增模型

<table header-row="true">
<tr>
<td>模型</td>
<td>用途</td>
</tr>
<tr>
<td>Schedule</td>
<td>定时任务触发</td>
</tr>
<tr>
<td>DashboardData</td>
<td>Dashboard agent专属数据区</td>
</tr>
<tr>
<td>Gate</td>
<td>任务门禁规则</td>
</tr>
</table>

### 修改模型

<table header-row="true">
<tr>
<td>模型</td>
<td>变更</td>
</tr>
<tr>
<td>Task</td>
<td>新增 participants, tags, acceptance_criteria, gates</td>
</tr>
</table>

### 删除模型

<table header-row="true">
<tr>
<td>模型</td>
<td>原因</td>
</tr>
<tr>
<td>Session</td>
<td>合并入Task</td>
</tr>
<tr>
<td>SessionRound</td>
<td>合并入TaskStep</td>
</tr>
</table>

### 不变的部分

状态机、Bus接口、Store接口核心、Agent运行时、消息流转、错误分类、Decision版本化、所有接口定义——全部不变。

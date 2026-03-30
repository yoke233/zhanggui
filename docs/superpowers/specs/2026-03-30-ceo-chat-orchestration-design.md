# CEO Chat Orchestration MVP 设计稿

> 日期：2026-03-30
> 状态：草案
> 类型：设计文档
> 范围：CEO 入口第一阶段 MVP

---

## 1. 背景

当前系统已经具备以下稳定主线：

- `/chat`：`ChatSession` direct chat
- `/threads`：多人 / 多 agent 协作容器
- `/work-items`：正式执行主线
- `Proposal / Initiative`：计划审批与执行前收敛
- `ACP`：agent 执行与线程协作协议层

系统最初目标是向“AI 公司”演进。当前新的产品目标是：

在不新增重产品面的前提下，引入一个 `CEO` 入口，让用户可以在 `/chat`
中直接向 CEO 交办目标，由 CEO 负责：

- 将目标任务化
- 拆解执行
- 按 profile 分派
- 跟踪推进
- 催办或改派
- 在必要时升级到 `Thread` 协作

第一阶段要求保持足够轻量，不引入新的主业务对象，不把系统改造成
“会议优先”的重流程。

---

## 2. 设计目标

第一阶段 CEO MVP 的目标如下：

1. 将 `CEO` 作为一个可在 `/chat` 中选择的 `profile`
2. 通过一份管理型 `skill` 教会 CEO 如何执行经营调度
3. 提供一组稳定的 `CLI` 管理动作，替代 skill 中直接写 `curl`
4. 默认走“任务编排优先”，仅在复杂时升级到 `Thread`
5. 复用现有 `WorkItem / Action / Thread / Proposal / Initiative` 主线

---

## 3. 非目标

第一阶段明确不做以下内容：

- 不新增独立 `/ceo` 页面
- 不新增 `CEO` 专属数据库主实体
- 不建立完整组织治理模型（部门、汇报线、岗位体系）
- 不默认先开会、先起 `Thread`
- 不允许 CEO 自动给真人做正式强制分派
- 不让 CEO 直接面向具体 agent session 做调度
- 不通过 skill 中的 `curl` 直接拼接 REST 调用
- 不做跨会话隐式记忆共享

---

## 4. 核心定位

### 4.1 CEO 的系统定位

第一阶段 CEO 不是新的领域对象，而是：

- 一个 `chat profile`
- 挂载一份 `CEO management skill`
- 通过窄 `CLI` 调用系统管理动作

一句话定义：

> CEO 是一个存在于 `/chat` 的调度型管理代理，不是新的系统核心对象。

### 4.2 CEO 的职责

第一阶段 CEO 负责：

- 接收用户的高层目标
- 判断是否可直接任务化
- 创建 `WorkItem`
- 在必要时触发任务拆解
- 按 `profile` 分派任务
- 跟踪推进状态
- 在停滞时催办
- 在角色不匹配时改派
- 在复杂度过高时升级到 `Thread`

### 4.3 CEO 的边界

CEO 第一阶段不负责：

- 代替 engineer / reviewer / planner 做具体执行
- 直接审批高风险或正式治理动作
- 直接操控具体 session 级运行时细节
- 利用跨会话隐式记忆维持长期上下文

---

## 5. 核心原则

第一阶段 CEO 必须遵守以下原则：

### 5.1 任务编排优先

默认流程：

```text
用户在 chat 提目标
  -> CEO 判断
  -> 先任务化
  -> 先拆解 / 分派 / 跟踪
  -> 只有在复杂时才升级 Thread
```

### 5.2 按 profile 分派

CEO 的分派目标是 `profile`，不是具体 agent session。

例如：

- `planner`
- `engineer`
- `reviewer`
- `secretary`

CEO 只负责“交给什么角色”，运行时如何找到具体会话由系统后续解决。

### 5.3 新会话重新加载上下文

你与 CEO 的主对话只在 `/chat` 中进行。

如果 CEO 自己创建 `Thread`，那是 CEO 开启的一个新会话，必须：

- 重新加载上下文
- 重新组织 boot 信息
- 不能隐式继承 chat 会话中的私有运行记忆

### 5.4 透明汇报

CEO 每次做出调度动作后，都应向用户说明：

- 做了什么
- 为什么这么做
- 当前状态如何
- 下一步是什么

CEO 不能成为黑箱调度器。

---

## 6. 架构方案

本设计采用如下方案：

```text
/chat
  -> CEO profile
    -> CEO management skill
      -> orchestration CLI（CEO alias）
        -> 现有 WorkItem / Action / Thread / Proposal / Initiative 能力
```

### 6.1 为什么不直接在 skill 中写 curl

不采用 skill 直接写 `curl` 的原因：

- 绑定 HTTP 细节过重
- 参数和鉴权难稳定
- 错误处理与重试不统一
- 输出格式不适合 agent 稳定消费
- 后续其他管理角色难复用

### 6.2 为什么要引入窄 CLI

CLI 的角色不是给人手工操作为主，而是给 CEO skill 提供稳定动作面：

- 对 skill 暴露业务动作语义
- 屏蔽底层 API 细节
- 提供稳定结构化输出
- 为后续 CTO / COO / PM 等角色复用奠定基础

---

## 7. CEO CLI 最小动作面

第一阶段 CLI 只覆盖 CEO 的最小管理闭环。

底层规范命名建议使用中性 orchestration 命名空间：

```text
orchestrate task create
orchestrate task decompose
orchestrate task assign-profile
orchestrate task follow-up
orchestrate task reassign
orchestrate task escalate-thread
```

为保持 CEO 视角自然，可在上层提供 persona alias：

```text
ceo task ...
```

但 `ceo` 只作为 alias，不应成为底层控制面的唯一命名空间。

### 7.1 `orchestrate task create`

作用：

- 将 CEO 在 chat 中收到的目标落成 `WorkItem`

输入建议：

- `title`
- `body`
- `project`
- `priority`
- `labels`
- `source_chat_session_id`
- `source_goal_ref`
- `dedupe_key`（可选）

语义约束：

- 该命令必须具备基础幂等语义
- 当 `dedupe_key` 或 `source_goal_ref` 与现有未关闭任务命中时，优先返回已有
  `WorkItem`，而不是重复建单
- 返回结果应显式说明是“新建”还是“命中已有”

输出建议：

- `ok`
- `action`
- `work_item_id`
- `status`
- `created`
- `summary`

### 7.2 `orchestrate task decompose`

作用：

- 为 `WorkItem` 生成或补充执行拆解
- 第一阶段可先对齐到现有 Action 生成能力

输入建议：

- `work_item_id`
- `objective`
- `overwrite_existing`

语义约束：

- `overwrite_existing=false` 时，只允许补充缺失拆解，不得破坏已有 Action DAG
- `overwrite_existing=true` 时，仅允许在以下情况执行：
  - 任务尚未开始执行
  - 或显式进入后续 `replan` / `archive-and-rebuild` 流程
- 若当前已存在 `running / done / waiting_gate` 等 Action，则应直接失败并返回
  `DECOMPOSE_CONFLICT` 一类错误，而不是静默覆盖

输出建议：

- `ok`
- `action`
- `work_item_id`
- `action_count`
- `summary`

### 7.3 `orchestrate task assign-profile`

作用：

- 按 `profile` 为任务设置“首选执行角色”
- 该动作不直接绑定具体 session，而是设置显式 profile override

输入建议：

- `work_item_id`
- `profile`
- `reason`
- `expected_output`

语义约束：

- 第一阶段的分派落点分两层：
  - 管理可见性层：写入 `WorkItem.metadata.ceo.assigned_profile`
  - 执行绑定层：对尚未启动的可执行 Action 写入
    `Action.Config["preferred_profile_id"]`
- `preferred_profile_id` 是对当前 Action resolver 的显式 override：
  - 命中时优先使用指定 profile
  - 未命中或 profile 不可用时再按现有 `AgentRole + RequiredCapabilities`
    回退
- 若当前任务还未拆解出 Actions，则先写入 `WorkItem.metadata`，待后续
  `decompose` / materialize 时再传播到新建 Actions
- 第一阶段默认只作用于执行类 Action，不强制覆盖 gate/review 类 Action，
  避免和现有质量门角色模型冲突

输出建议：

- `ok`
- `action`
- `work_item_id`
- `profile`
- `summary`

### 7.4 `orchestrate task follow-up`

作用：

- 查询任务推进状态，返回适合 CEO 汇报的摘要

输入建议：

- `work_item_id`

输出建议：

- `ok`
- `action`
- `work_item_id`
- `status`
- `blocked`
- `latest_run_summary`
- `recommended_next_step`

### 7.5 `orchestrate task reassign`

作用：

- 在当前 profile 不合适或持续停滞时改派

输入建议：

- `work_item_id`
- `new_profile`
- `reason`

输出建议：

- `ok`
- `action`
- `work_item_id`
- `old_profile`
- `new_profile`
- `summary`

### 7.6 `orchestrate task escalate-thread`

作用：

- 当任务复杂度超阈值时升级到 `Thread`

输入建议：

- `work_item_id`
- `reason`
- `thread_title`
- `invite_profiles[]`
- `invite_humans[]`

语义约束：

- `invite_humans[]` 表示“邀请参与会议/协作 Thread”，不表示“自动给真人正式派单”
- 该动作必须建立 `Thread <-> WorkItem` 显式关联
- 若已存在与当前 `WorkItem` 对应的活跃协作 Thread，应优先返回已有 Thread
  或要求显式 `force_new=true`，避免重复升级

输出建议：

- `ok`
- `action`
- `work_item_id`
- `thread_id`
- `link_created`
- `summary`

### 7.7 输出格式约束

第一阶段 CLI 输出统一采用机器友好的 JSON。

示例：

```json
{
  "ok": true,
  "action": "task.assign-profile",
  "work_item_id": 123,
  "profile": "engineer",
  "summary": "已将任务分派给 engineer profile"
}
```

CLI 的目标不是“人类阅读舒适”，而是“对 agent 稳定可解析”。

---

## 8. CEO Skill 设计

CEO skill 第一阶段不是 API 说明书，而是轻量管理编排规则。

### 8.1 Skill 主要负责什么

CEO skill 负责：

- 理解用户目标
- 判断能否直接任务化
- 判断是否需要拆解
- 判断分派给哪个 profile
- 判断当前是催办、改派，还是升级 Thread
- 组织向用户汇报的管理话术

### 8.2 Skill 不负责什么

CEO skill 不负责：

- 暴露底层 REST 参数细节
- 直接实现 HTTP 细节调用
- 管理具体 agent session
- 引入整套公司治理知识库

### 8.3 CEO 的判断链

第一阶段建议将 CEO skill 收敛成固定判断链：

```text
收到目标
  -> 判断是否可直接任务化
  -> 如果可以，创建 WorkItem
  -> 判断是否需要拆解
  -> 选择 profile 分派
  -> 周期性 follow-up
  -> 如停滞，优先催办
  -> 如 profile 不匹配，执行改派
  -> 如复杂度过高或持续卡住，升级 Thread
```

### 8.4 规则建议

#### 直接任务化条件

满足以下条件时优先直接创建任务：

- 目标边界比较清楚
- 预期产出明确
- 单个 profile 可主导完成
- 不需要先多人讨论才能开工

#### 先拆解条件

满足以下条件时先拆解：

- 任务较大
- 存在多个阶段
- 有明显前后依赖
- 不是一步完成型工作

#### 催办条件

满足以下条件时先催办：

- 已有明确 profile
- 最近推进停滞
- 没有明确 profile 错配证据

#### 改派条件

满足以下条件时改派：

- 当前 profile 明显不合适
- 多次 follow-up 仍无有效推进
- 任务性质发生变化

#### 升级 Thread 条件

满足以下任一条件时允许升级：

- 任务规模过大
- 依赖不清或冲突较多
- 需要多人 / 多 agent 协同
- 已催办或改派后仍无法推进
- 需要正式协作收敛

### 8.5 Skill 结构建议

第一阶段的 `SKILL.md` 建议仅包含：

1. CEO 角色定位
2. 默认工作原则
3. 任务编排判断链
4. 各 CLI 动作的调用时机
5. 升级到 Thread 的判断条件
6. 汇报透明度要求

不应写成大而全的公司治理手册。

---

## 9. 数据流与对象关系

第一阶段不新增新的主业务对象。

### 9.1 现有对象继续承担真相源职责

- `ChatSession`：用户与 CEO 的 direct chat 入口
- `WorkItem`：正式任务真相源
- `Action`：执行拆解节点
- `Run`：一次执行尝试
- `Thread`：复杂协作升级容器
- `Proposal / Initiative`：计划审批与执行前收敛

### 9.2 默认数据流

```text
用户
  -> ChatSession(CEO)
    -> CEO skill 判断
      -> CEO CLI
        -> WorkItem / Action / assignment 变化
          -> CEO 汇报用户
```

### 9.3 升级路径数据流

```text
用户
  -> ChatSession(CEO)
    -> CEO skill 判断复杂度过高
      -> CEO CLI escalate-thread
        -> 创建 Thread
        -> 建立 Thread <-> WorkItem 关联
        -> 邀请 profile / human
        -> CEO 在线程中开启新会话
```

### 9.4 上下文重建要求

当 CEO 从 chat 升级到 Thread 时，新的会话上下文必须重建。

Thread boot context 至少需要包含：

- 当前任务摘要
- 当前 `WorkItem` 状态
- 为什么升级
- 本次会议目标
- 需要邀请的角色或成员

CEO 不能依赖 chat 会话中的隐式运行时记忆。

### 9.5 来源与可追溯性

第一阶段建议至少记录以下来源关系：

- 哪个 `WorkItem` 由哪个 chat 目标触发
- 哪次改派的原因是什么
- 哪次升级 `Thread` 的原因是什么
- 哪个 `Thread` 来源于哪个 `WorkItem`

第一阶段可先落在 metadata 或现有 link 关系上，不强制新增表，但必须遵守
append-only 约束，不能只保留“最新状态”。

建议最小结构：

```json
{
  "ceo_journal": [
    {
      "ts": "2026-03-30T12:00:00Z",
      "actor_profile": "ceo",
      "action": "task.assign-profile",
      "reason": "需要 backend engineer 执行",
      "source_chat_session_id": 42,
      "before": { "assigned_profile": "planner" },
      "after": { "assigned_profile": "engineer" }
    }
  ]
}
```

可选增强：

- 同步写入现有 event / audit 链
- 在 `Thread.metadata` 中记录 escalation 摘要
- 在 `WorkItem.metadata` 中记录来源 chat 引用

---

## 10. 第一阶段最小落地范围

第一阶段仅交付以下内容：

### 10.1 一个 CEO profile

- 可在 `/chat` 中选择
- 作为普通 profile 使用
- 挂载 CEO management skill

### 10.2 一份 CEO management skill

- 聚焦任务编排判断链
- 不写 `curl`
- 不写重治理手册

### 10.3 一个窄 orchestration CLI（CEO alias）

仅覆盖：

- `task create`
- `task decompose`
- `task assign-profile`
- `task follow-up`
- `task reassign`
- `task escalate-thread`

### 10.4 一条完整管理闭环

```text
用户给 CEO 一个目标
  -> CEO 创建任务
  -> CEO 拆解任务
  -> CEO 分派 profile
  -> CEO 跟踪状态
  -> CEO 催办或改派
  -> 必要时升级 Thread
  -> CEO 向用户汇报
```

---

## 11. 测试与验收

第一阶段重点验证 CEO 是否真的能调度系统，而不是只会管理话术。

### 11.1 核心验收场景

#### 场景 A：直接任务化

- 用户在 `/chat` 中给 CEO 一个清晰目标
- CEO 创建 `WorkItem`
- CEO 给出汇报
- 不升级 `Thread`

#### 场景 B：拆解并分派

- 用户给出一个中等复杂任务
- CEO 先拆解，再按 `profile` 分派

#### 场景 C：跟踪与催办

- 已有任务推进停滞
- CEO 执行 `follow-up`
- 识别停滞并优先催办

#### 场景 D：改派

- 当前 profile 明显不适合
- CEO 解释原因并执行改派

#### 场景 E：升级 Thread

- 任务复杂、依赖不清或持续卡住
- CEO 说明理由
- 创建 `Thread`
- 建立 `Thread <-> WorkItem` 关联

### 11.2 CLI 验收重点

- 输入输出稳定
- 返回 JSON 结构稳定
- 错误信息可解析
- 同一动作重复调用行为可预测
- `task create` 具备基础幂等语义
- `task decompose` 在冲突状态下会显式拒绝覆盖

### 11.3 Skill 验收重点

- 默认先任务编排
- 不默认先开会
- 能区分催办与改派
- 升级 Thread 前有明确理由
- 不依赖 skill 中的 `curl`

### 11.4 第一阶段通过标准

满足以下条件即可视为 MVP 成立：

- CEO 可在 `/chat` 中作为 profile 使用
- CEO 能通过 skill + CLI 触发现有系统动作
- CEO 能完成任务编排闭环
- CEO 默认任务优先，复杂时才升级 Thread
- 整个过程可追溯、可解释

---

## 12. 风险与取舍

### 12.1 风险一：CEO 退化成普通聊天助手

表现：

- 只会提建议
- 不稳定触发系统动作

缓解：

- 强化 skill 中的动作触发规则
- CLI 保持结构化输出
- 要求 CEO 回答必须说明已执行的系统动作

### 12.2 风险二：CEO 退化成重流程入口

表现：

- 动不动就升级 Thread
- 一切都先开会

缓解：

- 将“任务编排优先”写成硬规则
- 为 `escalate-thread` 设置严格触发条件
- 规定 follow-up / reassign 优先于 escalate-thread

### 12.3 风险三：CLI 只是换皮 curl

表现：

- CLI 只包一层 REST，仍暴露底层细节

缓解：

- 采用管理动作语义命名
- 输入输出坚持业务语义
- 不让 skill 直接关心 REST 路径、header、鉴权细节

---

## 13. 结论

第一阶段 CEO 入口的最优方案是：

> `CEO = Chat Profile + CEO Management Skill + 窄 Orchestration CLI`

并坚持以下路线：

- `/chat` 是唯一入口
- 默认任务编排优先
- 按 `profile` 分派
- `Thread` 仅作为复杂任务升级路径
- 复用现有 `WorkItem / Action / Thread / Proposal / Initiative`

这个方案足够轻，不会引入新的重产品面，同时能够为后续 CTO / COO / PM
等管理角色沉淀一套可复用的调度模式。

# OpenViking聊天存储设计 — Thread在OV中的落地方案

> 来源：Notion MCP 抓取
>
> 页面：`OpenViking聊天存储设计 — Thread在OV中的落地方案`
>
> URL：`https://www.notion.so/31e4a9d94a3581879e26f14664e23413`
>
> 抓取时间：`2026-03-09T00:01:51.916Z`

## 1. 定位

OpenViking是存储和检索系统，不是通信系统。消息的实时投递走Bus，OpenViking解决的是“聊完之后怎么存、下次怎么找到”。

```javascript
实时通信：Bus投递消息 → agent收到 → 做决策 → 回复
持久化：消息同时写入OV → 生成L0/L1摘要 → 建立索引
下次使用：PromptBuilder调Memory.Recall → Memory内部用OV检索 → 返回相关上下文
```

---

## 2. Thread在OV中的目录结构

每个Thread就是一个目录，消息是目录里的文件：

```javascript
viking://session/{thread_id}/
├── .abstract.md          L0：一句话摘要（自动生成）
├── .overview.md          L1：概览（参与者、关键决策、未解决问题）
├── .relations.json       关联（task_id、participants、crystallized_to）
├── msg_001.md            L2：原始消息
├── msg_002.md
└── ...
```

---

## 3. Agent怎么知道它和别人聊过什么

Agent的所有对话经历存在它的专属目录下：

```javascript
viking://agent/{agent_id}/
├── .abstract.md              "技术主管，管理3个开发者"
├── .overview.md              能力、当前任务、最近活跃thread列表
├── threads/                  参与过的所有对话索引
│   ├── thread_001.ref        → 指向 viking://session/thread_001/
│   └── thread_002.ref
├── experience/               经验沉淀（Gate通过后触发）
│   ├── exp_001.md            "JWT比Session更适合前后端分离"
│   └── exp_002.md            "退款金额字段用decimal(10,2)"
└── decisions/                重要决策记录
```

Memory.Recall检索路径：
1. 查 `viking://agent/{agent_id}/threads/` → 拿到所有thread引用
2. 查每个thread的 `.abstract.md`（L0）→ 快速过滤相关的
3. 对相关thread读 `.overview.md`（L1）→ 理解聊了什么
4. 只在需要时读具体消息（L2）

---

## 4. 求助场景怎么记录

求助本质上就是一段Thread里的对话。“求助”的语义通过L1摘要的生成逻辑承载，不需要额外数据结构：

```javascript
viking://session/thread_003/.overview.md

## 对话概要
- 类型：求助
- 发起者：worker_dev_1
- 回答者：supervisor_tech

## 问题
退款金额字段该用什么类型？

## 结论
使用decimal(10,2)

## 关联
- Task: task_001_1
```

下次worker遇到类似问题，Memory.Recall用OV检索“字段类型”，命中这个thread的L0/L1摘要，把结论带进prompt。不需要重新问。

---

## 5. 经验沉淀：从对话到可复用知识

两个触发时机：

**Thread关闭时：**

```javascript
Thread closed
  → 异步：LLM读取全部消息
  → 提取结论写入 viking://agent/{agent_id}/experience/
  → 更新 viking://agent/{agent_id}/.overview.md
```

**Task的Gate通过时：**

```javascript
Gate passed
  → 该task下所有thread的经验统一提炼
  → 写入 viking://agent/{agent_id}/experience/
  → 写入 viking://resources/{project}/decisions/
```

---

## 6. PromptBuilder如何利用

```javascript
PromptBuilder.Build:

1. Agent身份（固定）
   → 读 viking://agent/{agent_id}/.abstract.md

2. 冷上下文
   → 读 viking://agent/{agent_id}/experience/ 下相关经验
   → 用L0向量检索，只带相关的

3. 任务信息
   → 读 viking://resources/{task_id}/.overview.md

4. 温上下文
   → 读 viking://resources/{parent_task_id}/.abstract.md
   → 读兄弟task的.abstract.md

5. 热上下文
   → 读 viking://session/{current_thread_id}/ 下最近消息
   → 有reply_to时沿对话链提取

6. 当前消息
```

---

## 7. 核心原则

**Agent不知道OV的存在。** 它只知道“我之前讨论过JWT方案，结论是用JWT+refresh token”——这是自然语言出现在prompt里的。OV是系统层面的事，AI层面看到的永远是自然语言。

符合设计反思中确立的原则：**结构服务于Prompt质量，不服务于模型完备性。**

# OpenViking 与本系统的关联分析

> 来源：Notion MCP 抓取
>
> 页面：`OpenViking 与本系统的关联分析`
>
> URL：`https://www.notion.so/31d4a9d94a3581fb8fb2e067310c0a3a`
>
> 抓取时间：`2026-03-08T23:38:31.820Z`

## 1. 结论

OpenViking 最适合作为我们 **Memory 接口的底层实现**。不用全盘接入，而是用它的核心设计思想来增强我们已有的架构。接口层不用变——Memory.Recall/Store/Compact 的签名不改，内部实现从 SQLite 查询换成 OpenViking 的 find/search。符合“接口隔离变化”原则。

---

## 2. 高度契合的设计

### 2.1 L0/L1/L2 分层 ≈ 我们的冷/温/热记忆

几乎是同一个思路，但切入角度不同：

<table header-row="true">
<tr>
<td>我们的设计</td>
<td>OpenViking</td>
<td>切入角度</td>
</tr>
<tr>
<td>冷/温/热</td>
<td>L0/L1/L2</td>
<td>我们按变化频率分层，OV按信息密度分层</td>
</tr>
</table>

**两者可以组合**：我们的每一层内部都可以用 L0/L1/L2 策略：

```javascript
我们的冷层（历史摘要）  → 本身就是 L0 级别，已压缩
我们的温层（父/兄弟任务）→ 只带 L0 摘要做过滤，需要时升级到 L1
我们的热层（当前task消息）→ 最近几条带 L2 全文，稍早的只带 L1 概览
```

直接改善了 token 预算分配——不是简单截断，而是 **先用 L0 过滤候选，再用 L1 做决策，仅必要时加载 L2**。

### 2.2 viking:// URI ≈ 我们的 Artifact content_ref

我们的 Artifact 用 `{kind, uri}` 引用内容，OpenViking 用 `viking://{scope}/{path}`。直接映射：

```javascript
viking://resources/{project}/  →  Workspace + Artifact（项目资源和交付物）
viking://user/{profile}/       →  human 的偏好和画像
viking://agent/{agent_id}/     →  Agent 的 prompt、能力、经验记忆
viking://session/{task_id}/    →  Task 下的消息和步骤记录
```

如果用 OpenViking 做底层，Artifact 的 content_ref 可以直接用 viking URI。

### 2.3 “目录即容器” ≈ 我们的 Task 树

OpenViking 的目录层级天然对应 Task 树。每个 Task 就是一个目录，子 Task 就是子目录。每个目录自带 `.abstract.md`（摘要）和 `.overview.md`（概览）——这就是我们 Memory.Compact 产出的东西，只是 OV 给了一个更标准化的存储格式。

---

## 3. 能补上我们设计盲区的部分

### 3.1 检索从“截断”升级为“导航”

我们现在的 Memory.Recall 是简单的取最近 N 条然后截断。OV 的层级递归检索思路更好——先定位到正确的“目录”（Task），再在目录里精细探索：

```javascript
supervisor 处理一条消息时：
  1. 先用 L0 摘要在所有 active task 里快速过滤相关 task
  2. 对相关 task 读 L1 概览，理解大致情况
  3. 只对当前 task 读 L2 全文
```

比我们现在的60%热+25%温+15%冷的固定比例灵活得多。

### 3.2 检索可观测性

我们完全没考虑过的。OV 的检索轨迹可以保留——“我找了哪些目录、打了多少分、最终用了哪些内容”。

如果我们在 Decision 里记录这个轨迹，当 agent 做了错误决策时，可以追溯“它当时看到了什么上下文”，而不只是“它做了什么决策”。直接增强了测试验证能力。

Decision 模型可以新增：

```javascript
Decision
├── ...现有字段
├── retrieval_trace    检索轨迹：查了哪些task、用了哪些内容、各1层级分数
```

### 3.3 写入和语义分离

OV 的 Parser → TreeBuilder → SemanticQueue 流水线启发：**TaskStep 的写入和 L0/L1 摘要的生成应该异步解耦**。

```javascript
现在：TaskStep 写入（同步）→ Memory.Compact 定时跑（批量压缩）
更好：TaskStep 写入（同步，快）→ 异步触发摘要生成（慢但不阻塞）
```

agent 的决策循环不会被摘要生成拖慢。先可用，再渐进增强。

### 3.4 记忆自迭代的触发时机

学习记录第12节提到“记忆抽取的触发时机与门禁”——这和我们的 Gate 设计可以结合。当 Task 通过门禁完成时，就是触发记忆沉淀的最佳时机：

```javascript
Task done（门禁通过）
  → 触发记忆抽取：
    - 经验沉淀到 viking://agent/{agent_id}/experience/
    - 用户偏好沉淀到 viking://user/preferences/
    - 讨论结论沉淀到 viking://resources/{project}/decisions/
```

---

## 4. 建议的接入方式

**不是全盘替换，而是逐步替换 Memory 接口的内部实现：**

<table header-row="true">
<tr>
<td>现有设计</td>
<td>用 OpenViking 替换</td>
<td>优先级</td>
</tr>
<tr>
<td>Artifact 的松散 \{kind, uri\}</td>
<td>viking URI 统一引用</td>
<td>高，规范化存储</td>
</tr>
<tr>
<td>简单截断取最近N条</td>
<td>L0/L1/L2 分层检索</td>
<td>高，直接提升质量</td>
</tr>
<tr>
<td>Memory.Recall 查 SQLite</td>
<td>OV 的 find/search</td>
<td>中，Phase 4 再换</td>
</tr>
<tr>
<td>TaskStep 同步压缩</td>
<td>异步语义队列</td>
<td>中，性能优化</td>
</tr>
<tr>
<td>无检索轨迹</td>
<td>Decision 加 retrieval_trace</td>
<td>低，可观测性增强</td>
</tr>
</table>

接口层不变——Memory.Recall/Store/Compact 的签名不改，内部实现替换。符合“接口隔离变化”原则。

---

## 5. 实施路径建议

<table header-row="true">
<tr>
<td>阶段</td>
<td>接入内容</td>
</tr>
<tr>
<td>Phase 0-2</td>
<td>不接 OV，用 SQLite 简单实现跑通流程</td>
</tr>
<tr>
<td>Phase 3</td>
<td>引入 L0/L1/L2 分层思想到 Memory 实现中，不一定用 OV，但用它的设计</td>
</tr>
<tr>
<td>Phase 4</td>
<td>评估是否部署 OV 服务作为统一上下文底座，替换 Memory 接口实现</td>
</tr>
</table>

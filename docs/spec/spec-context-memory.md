# Context & Memory 规范（OpenViking 集成）

## 目标

定义 ai-workflow 的上下文理解与经验记忆系统，基于 OpenViking 实现。
解决两个核心问题：

1. **TL 跨项目快速理解** — TL 管理多个 repo，需要快速了解任意项目的设计和实现，而不是每次从源码读起
2. **执行经验沉淀** — Agent 会话中积累的隐性知识（踩过的坑、解决方案、项目约定）需要持久化和召回

## 非目标

- 不通过 OpenViking 管理 per-issue spec 流转（Issue.Body + ACP prompt 直接传递）
- 不为 Worker/Reviewer 提供查询/阅读类 MCP tools（它们在 worktree 里，项目文件直接可用），但保留 session.Commit() 写入通道用于经验沉淀
- 不自建 L0/L1 摘要生成（交给 OpenViking）
- 不自建语义检索引擎（交给 OpenViking）
- 不自建记忆提取逻辑（交给 OpenViking session.commit）

## 设计决策：为什么不用 grep 代替

项目 spec 和代码确实在文件系统里，grep 能找到文件。但 TL 的场景不是"找文件"而是"快速理解"：

```
没有 OpenViking：
  Human: "project B 的认证是怎么设计的？"
  TL: → 读 auth/ 目录下 20 个文件 → 花几分钟 + 大量 token → 拼出回答

有 OpenViking：
  Human: "project B 的认证是怎么设计的？"
  TL: → overview("viking://resources/project-b/src/auth/") → L1 摘要 ~1k tokens → 秒答
```

**OpenViking 的核心价值是 L0/L1 预消化，不是替代 grep。**

## 两个功能

### 1. 项目知识预消化层

项目注册到系统时，自动导入 OpenViking。OpenViking 异步解析生成 L0/L1 摘要。
TL 随时可通过 overview/abstract 快速了解任何项目的任何模块。

```
项目注册 → AddResource(project.RepoPath) → OpenViking 异步解析
                                            ├── 每个目录生成 .abstract.md (L0, ~100 tokens)
                                            └── 每个目录生成 .overview.md (L1, ~1k tokens)

Issue 完成(merge 事件) → 增量更新变更的文件
```

### 2. 执行经验沉淀池

Agent 会话结束后，session.Commit() 自动提取经验。这些经验不在任何文件里，
是执行过程中产生的隐性知识。

```
ACP session 结束 → session.Commit()
                    ├── 归档消息到 history/
                    ├── 生成会话摘要
                    └── 自动提取记忆：
                        ├── cases（问题-方案对，不可变）
                        └── patterns（行为模式，可追加）
```

经验示例：
- "project B 的数据库迁移上次搞了 3 轮才通过，原因是 X"
- "这个项目的 CI 必须先跑 X 再跑 Y，否则会超时"
- "用户偏好用 bun 不用 npm"

## 核心映射：OpenViking 三元组

OpenViking 的 `UserIdentifier(account_id, user_id, agent_id)` 映射：

```
account_id  =  "default"          部署实例（单租户）
user_id     =  "system"           ai-workflow 系统用户
agent_id    =  角色名              tl / coder / reviewer
```

不同 agent_id → 自动物理隔离。TL 的记忆和 Coder 的记忆互不干扰。

## 目录结构

使用 OpenViking 默认目录，只定义 resources/ 下的项目组织方式：

```
viking://
├── resources/                          # account 级共享
│   ├── {project-name}/                 # 导入的项目 repo（自动生成 L0/L1）
│   │   ├── src/
│   │   ├── docs/
│   │   └── ...                         # 完整 repo 结构
│   └── shared/                         # 全局共享（编码规范、模板）
│
├── agent/                              # 按 agent_id 自动隔离
│   ├── memories/                       #   session.commit() 自动提取
│   │   ├── cases/                      #     问题-方案对（不可变）
│   │   └── patterns/                   #     行为模式（可追加）
│   └── instructions/                   #   角色指令（预留，P1）
│
└── session/{sid}/                      # 会话级
    ├── messages.jsonl
    ├── .abstract.md                    #   会话 L0 摘要
    └── history/                        #   归档历史
```

## L0/L1/L2 分层加载

每个目录节点由 OpenViking 自动生成摘要。子节点的 L0 聚合成父节点的 L1。

```
L0 (Abstract)   ~100 tokens   快速过滤（"这个目录大概讲什么"）
L1 (Overview)   ~1k tokens    理解概览（"这个模块的设计和关键组件"）
L2 (Detail)     无限制         原始内容（按需读完整文件）
```

TL 的典型查询路径：

```
Human: "project B 的认证模块怎么设计的？"
  1. abstract("viking://resources/project-b/src/")     → L0 扫描顶层目录
  2. overview("viking://resources/project-b/src/auth/") → L1 理解认证模块
  3. read("viking://resources/project-b/src/auth/middleware.go") → L2 看具体实现（如需要）
```

## TL 的 MCP 工具（按需调用）

TL 在 ACP session 中获得以下工具，**由 LLM 自行判断何时调用**。
简单任务不搜，跨项目设计问题才用。

```
工具                          用途                           调用时机
─────────────────────────────────────────────────────────────────────
context_overview(uri)         L1 概览                        "这个模块怎么设计的？"
context_abstract(uri)         L0 摘要                        快速扫描多个目录
context_read(uri)             L2 全文                        需要看具体文件
context_search(query)         跨项目语义搜索                  "哪个项目做过类似的？"
memory_search(query, pid?)    搜索执行经验                    "上次遇到过什么问题？"
memory_save(content, tags)    手动保存观察                    TL 主动记录用户偏好等
```

### 为什么是按需而不是预加载

- 预加载：每次对话都注入大量上下文 → token 浪费、噪声干扰
- 按需工具：LLM 根据对话内容自行判断是否需要搜索 → 零噪声
- Human 说"给 project A 加个按钮" → TL 不会去搜
- Human 说"参考 project B 怎么做的" → TL 自然会调用 overview

## 谁使用 OpenViking

| 角色 | 是否使用 | 原因 |
|------|---------|------|
| **TL** | **是（主要消费者）** | 管理多项目，需要快速理解 + 经验召回 |
| Worker | 否 | 在 worktree 里工作，项目文件直接可用 |
| Reviewer | 否 | 在 worktree 里审查，代码直接可读 |
| Decomposer | 否 | 读 Issue.Body 拆分，不需要跨项目知识 |
| Aggregator | 否（预留） | 未来可能需要读项目 source of truth |

Worker/Reviewer 的执行经验通过 session.Commit() 自动沉淀到各自的 agent/memories/，
但它们不主动查询 OpenViking。TL 是唯一主动查询的角色。

## Go 接口定义

### ContextStore

系统与 OpenViking 交互的核心接口。接口保持完整以支持未来扩展，
但 P0 阶段只有 TL 相关路径实际使用。

```go
type ContextStore interface {
    Plugin

    // 基础 CRUD
    Read(ctx context.Context, uri string) ([]byte, error)
    Write(ctx context.Context, uri string, content []byte) error
    List(ctx context.Context, uri string) ([]ContextEntry, error)
    Remove(ctx context.Context, uri string) error

    // 分层查询（L0/L1）— TL 快速理解项目的关键能力
    Abstract(ctx context.Context, uri string) (string, error)
    Overview(ctx context.Context, uri string) (string, error)

    // 语义搜索
    Find(ctx context.Context, query string, opts FindOpts) ([]ContextResult, error)
    Search(ctx context.Context, query string, sessionID string, opts SearchOpts) ([]ContextResult, error)

    // 资源导入（项目注册时调用）
    AddResource(ctx context.Context, path string, opts AddResourceOpts) error

    // 会话
    CreateSession(ctx context.Context, id string) (ContextSession, error)
    GetSession(ctx context.Context, id string) (ContextSession, error)
}
```

相比旧设计移除了：
- `Link()` — 不再管理 spec 间关联
- `Materialize()` — Worker 不需要从 OpenViking 物化文件到 worktree

### ContextSession

ACP session 期间与 OpenViking 交互的会话接口。

```go
type ContextSession interface {
    ID() string
    AddMessage(role string, parts []MessagePart) error
    Used(contexts []string) error
    Commit() (CommitResult, error)
}
```

## 项目同步策略

### 导入时机

```
事件                              动作
──────────────────────────────────────────────────
项目注册到系统（POST /projects）   AddResource(project.RepoPath, wait=false)
Issue merge 完成（EventIssueMerged）增量更新变更文件
手动触发                           POST /api/v3/projects/{id}/sync-context
```

### 导入范围

导入整个 repo。OpenViking 自动处理：
- 代码文件：语法感知解析
- Markdown/文档：原生支持
- 配置文件：结构化解析
- 二进制文件：跳过或 VLM 描述

### 增量更新

Issue merge 后，系统通过 git diff 确定变更文件列表，只更新这些文件。
不需要全量重新导入。

## 记忆系统

### 记忆提取（全自动）

所有 ACP session（不仅 TL）结束时调用 `session.Commit()`：

```
对话消息 → LLM 提取候选记忆
    → 向量预过滤（找同分类相似记忆）
    → LLM 去重决策（CREATE / UPDATE / MERGE / SKIP）
    → 写入 agent/memories/（cases 或 patterns）
    → 向量化建索引
```

### 记忆分类（使用两种）

| 分类 | 位置 | 归属 | 可更新 | 说明 |
|------|------|------|--------|------|
| **cases** | `agent/memories/cases/` | agent | 不可变 | 问题-方案对 |
| **patterns** | `agent/memories/patterns/` | agent | 追加 | 行为模式 |

其他 4 种 OpenViking 记忆分类（profile/preferences/entities/events）暂不使用。

### 经验召回

TL 通过 memory_search 工具按需召回：

```
# TL 接到项目 A 的任务，搜索相关经验
results = client.Search(
    query="数据库迁移注意事项",
    session=currentSession,           # 带当前对话上下文
    target_uri="viking://agent/memories/",
)
// agent_id="tl" → 只搜 TL 自己的记忆
```

跨项目经验自动可用 — 扁平记忆池，不按项目分子目录。
语义搜索自然按相关性召回。

## ACP 集成流程

### TL Session（完整集成）

```
TL ACP session 启动
│
├── 1. 创建 OpenViking client
│      client = ov.NewHTTPClient(agentID="tl")
│
├── 2. 创建 OpenViking session
│      session = client.CreateSession("tl-session-{id}")
│
├── 3. 注册 MCP tools（按需调用，不预加载内容）
│      tools = [context_overview, context_abstract, context_read,
│               context_search, memory_search, memory_save]
│
├── 4. 运行 ACP session
│      Human 对话 → TL 按需调用工具
│      session.AddMessage() 自动记录对话
│
└── 5. session 结束 → commit
       result = session.Commit()
       // OpenViking 自动提取 cases/patterns
```

### Worker/Reviewer Session（仅记忆提交）

```
Worker ACP session 启动
│
├── 1. 创建 OpenViking client
│      client = ov.NewHTTPClient(agentID="coder")
│
├── 2. 创建 OpenViking session
│      session = client.CreateSession("run-{runID}-{stage}")
│
├── 3. 不注册 context MCP tools（Worker 直接读 worktree 文件）
│
├── 4. 运行 ACP session
│      session.AddMessage() 自动记录对话
│
└── 5. session 结束 → commit
       result = session.Commit()
       // 自动提取 coder 的执行经验
```

## 配置

```yaml
context:
  provider: openviking          # openviking | context-sqlite | mock | "" (disabled)
  openviking:
    url: "http://localhost:1933"
    api_key: ""                 # dev 模式留空
  fallback: context-sqlite      # OpenViking 不可用时降级
  path: ".ai-workflow/context.db"  # SQLite fallback 路径
```

降级到 SQLite 时：
- Read / Write / List 可用
- L0/L1 不可用（返回空）→ TL 退化为直接读文件
- Search 不可用（返回空）→ 经验召回不可用
- Session.Commit() 仅记录消息，不提取记忆

## 实现阶段

### P0：最小闭环

1. OpenViking HTTP client 实现（`context-openviking` 插件）
2. 项目注册时 AddResource 导入 repo
3. TL 的 MCP tools 注册（overview / abstract / read / search）
4. 所有 ACP session 的 session.Commit()（记忆提取）
5. 配置扩展（openviking URL / API key）

### P1：经验召回 + 增量同步

1. memory_search / memory_save MCP tools
2. Issue merge 后增量更新 OpenViking
3. TL 启动时可选加载最近相关经验（轻量，非预加载全部）
4. Instructions 加载（角色 prompt 从 OpenViking 读取，预留）

### P2：质量 + 监控

1. 记忆质量监控（定期 review cases/patterns 的有效性）
2. 项目同步状态 dashboard
3. SQLite fallback 完整实现
4. 多用户 account_id 支持（预留）
# Memory System Design — 分层递进实现方案

> 基于 `spec-context-memory.md` 的愿景，制定可落地的实现路径。
> 核心原则：先补齐本地管道，再接外部服务；被动注入优先于主动查询。

## 现状

### 已有基础

| 组件 | 位置 | 状态 |
|------|------|------|
| Briefing 结构体 | `core/briefing.go` | 运行中，5 种 ContextRefType，其中 3 种已填充 |
| BriefingBuilder | `application/flow/briefing_builder.go` | 运行中，收集 Issue 摘要 + 分层上游 Artifact (L0/L2) + Feature Manifest |
| renderBriefingSnapshot | `application/flow/pipeline.go` | 运行中，按类型分配字符预算 (`refBudget()`) |
| ExecutionInput | `application/flow/execution_input.go` | 运行中，渲染 Briefing 快照为 prompt |
| Token 预算监控 | `runtime/agent/acp_session_pool.go` | 运行中，会话级累积 token 追踪 + 三级预算检查 (OK/Warning/Exceeded) |
| AgentProfile.Skills | `core/agent.go` | 字段存在，Skills 元数据可解析，但未注入 prompt |
| AgentProfile.PromptTemplate | `core/agent.go` | 字段存在，未使用 |
| Legacy Memory 接口 | `legacy/core/memory.go` | 废弃，Cold/Warm/Hot 三层模型 |
| OpenViking 规范 | `docs/spec/spec-context-memory.md` | 设计完成，未实现 |
| ContextRef 类型 | `core/briefing.go` | ✅ `issue_summary` / `upstream_artifact` / `feature_manifest` 已填充；🔲 `project_brief` / `agent_memory` 未填充 |

### 关键缺口

~~Agent 执行时除了上游步骤的输出，**不知道项目是什么、Flow 到了哪一步、历史上遇到过什么问题**。~~

> **更新 (2026-03-12):** P0 和 P1 优化已部分落地：Agent 现在能看到 Issue 摘要、分层上游 Artifact（L0 摘要 / L2 全文）、Feature Manifest，并有 Session Token 预算监控。剩余缺口：ProjectBrief、FlowSummary、AgentMemory、Skills 注入。

## 分层实现路径

### Phase 1：补齐 Briefing 管道（纯本地，0 外部依赖）

**目标：** 让每个 Agent 启动时自动获得项目上下文和 Flow 状态。

> **落地进度 (2026-03-12):**
> - ✅ Issue 摘要注入 (`CtxIssueSummary`) — `briefing_builder.go` `injectIssueContext()`
> - ✅ 上游 Artifact 分层注入 (L0 summary / L2 full) — `briefing_builder.go` `injectUpstreamContext()`
> - ✅ Feature Manifest 注入 (`CtxFeatureManifest`) — `briefing_builder.go` `injectManifestContext()`
> - ✅ 按类型分配字符预算 — `pipeline.go` `refBudget()`
> - ✅ Session Token 预算监控 — `acp_session_pool.go` `CheckTokenBudget()` / `NoteTokens()`
> - 🔲 1.1 `CtxProjectBrief` — 未实现
> - 🔲 1.2 `CtxFlowSummary` — 未实现
> - 🔲 1.3 Skills 内容注入 — 未实现

#### 1.1 填充 `CtxProjectBrief`

BriefingBuilder 在 Build() 时，通过 Store 查询 Step 所属 Flow → Flow 所属 Project → 拼接项目摘要：

```go
// briefing_builder.go — Build() 内新增
project, err := b.store.GetProject(ctx, flow.ProjectID)
if err == nil && project != nil {
    brief := renderProjectBrief(project, bindings)
    briefing.ContextRefs = append(briefing.ContextRefs, core.ContextRef{
        Type:   core.CtxProjectBrief,
        RefID:  project.ID,
        Label:  "project: " + project.Name,
        Inline: brief,
    })
}
```

`renderProjectBrief` 组装：
- Project.Name + Description
- ResourceBinding 列表（repo URL、类型）
- Project.Kind（dev / general）

预算：~200-500 tokens，固定开销。

#### 1.2 填充 `CtxFlowSummary`

把 Flow 的已完成步骤列表 + 状态 + 当前步骤位置拼成摘要：

```go
// briefing_builder.go — Build() 内新增
steps, _ := b.store.ListStepsByFlow(ctx, step.FlowID)
summary := renderFlowSummary(flow, steps, step.ID)
briefing.ContextRefs = append(briefing.ContextRefs, core.ContextRef{
    Type:   core.CtxFlowSummary,
    RefID:  flow.ID,
    Label:  "flow progress",
    Inline: summary,
})
```

`renderFlowSummary` 输出示例：
```
Flow: "Add OAuth login" (3/5 steps completed)
- [done] plan: decompose requirements
- [done] implement: auth middleware
- [done] implement: login page
- [running] gate: code review        ← current
- [pending] implement: merge & deploy
```

预算：~100-300 tokens。

#### 1.3 Skills 内容注入

当前 Skills 的 SKILL.md 元数据已被解析但未注入。在 ExecutionInput 构建时，如果 Profile 配置了 Skills，将 Skill 的 description + assign_when 以简短说明形式追加到 prompt：

```go
// execution_input.go — BuildExecutionInputFromBriefing 扩展
if profile != nil && len(profile.Skills) > 0 {
    sb.WriteString("\n\n# Available Skills\n\n")
    for _, skill := range resolvedSkills {
        sb.WriteString("- **" + skill.Name + "**: " + skill.Description + "\n")
    }
}
```

#### 1.4 改动范围估算

| 文件 | 改动 |
|------|------|
| `application/flow/briefing_builder.go` | 新增 ProjectBrief + FlowSummary 填充 |
| `application/flow/execution_input.go` | Skills 注入 |
| `application/flow/store.go` (接口) | 可能需要新增 GetProject / GetFlow 方法 |
| 无新文件，无新依赖 | |

### Phase 2：本地记忆存储（SQLite）

**目标：** Agent 执行经验可持久化、可召回，无外部依赖。

#### 2.1 数据模型

```go
// internal/core/memory.go（替代 legacy 版本）

type MemoryKind string

const (
    MemoryCase       MemoryKind = "case"        // 问题-方案对，不可变
    MemoryPattern    MemoryKind = "pattern"      // 行为模式，可追加
    MemoryPreference MemoryKind = "preference"   // 用户/项目偏好
)

type MemoryEntry struct {
    ID        int64      `json:"id"`
    AgentID   string     `json:"agent_id"`    // "tl" / "coder" / "reviewer"
    ProjectID int64      `json:"project_id"`  // 0 = 跨项目通用
    FlowID    int64      `json:"flow_id"`     // 来源 flow（溯源用）
    Kind      MemoryKind `json:"kind"`
    Content   string     `json:"content"`     // 自然语言描述
    Tags      []string   `json:"tags"`        // 粗筛标签
    Source    string     `json:"source"`      // "auto:session-xxx" / "manual"
    CreatedAt time.Time  `json:"created_at"`
}
```

#### 2.2 存储接口

```go
// internal/core/memory_store.go

type MemoryStore interface {
    SaveMemory(ctx context.Context, entry *MemoryEntry) error
    SearchMemories(ctx context.Context, opts MemorySearchOpts) ([]*MemoryEntry, error)
    DeleteMemory(ctx context.Context, id int64) error
}

type MemorySearchOpts struct {
    AgentID   string   // 必填，角色隔离
    ProjectID int64    // 0 = 不限项目
    Kind      MemoryKind // 空 = 全部
    Keywords  []string // content LIKE 匹配
    Tags      []string // tag 交集过滤
    Limit     int      // 默认 10
}
```

SQLite 实现放在 `internal/store/sqlite/memory.go`，复用现有 SQLite 基础设施。

#### 2.3 记忆写入（执行后自动提取）

在 Execution 完成后（Artifact 已生成），新增一个 `MemoryExtractor`：

```
Execution 完成
  → Artifact.ResultMarkdown 交给 Collector（已有的小模型调用）
  → 扩展 Collector prompt：除了提取 metadata，同时提取 memories
  → 返回 []MemoryEntry 候选
  → 查重（同 agent_id + 相似 content → 跳过）
  → 写入 SQLite
```

查重初期用简单策略：按 agent_id + project_id + kind 查出最近 N 条，让 LLM 判断是否重复。不需要向量搜索。

#### 2.4 记忆注入（BriefingBuilder 扩展）

```go
// briefing_builder.go — Build() 内新增
memories, _ := b.memoryStore.SearchMemories(ctx, core.MemorySearchOpts{
    AgentID:   profile.Name,
    ProjectID: project.ID,
    Limit:     5,
})
if len(memories) > 0 {
    briefing.ContextRefs = append(briefing.ContextRefs, core.ContextRef{
        Type:   core.CtxAgentMemory,
        RefID:  0,
        Label:  "relevant experience",
        Inline: renderMemories(memories),
    })
}
```

预算：~500-1000 tokens（5 条记忆，每条 ~100-200 tokens）。

#### 2.5 改动范围

| 文件 | 改动 |
|------|------|
| `internal/core/memory.go` | 新模型（替代 legacy） |
| `internal/core/memory_store.go` | 新接口 |
| `internal/store/sqlite/memory.go` | SQLite 实现 |
| `internal/store/sqlite/migrations/` | 新建 memory_entries 表 |
| `application/flow/briefing_builder.go` | 注入 MemoryStore，召回记忆 |
| `application/flow/engine.go` | 执行后调用 MemoryExtractor |
| `internal/application/flow/memory_extractor.go` | 新文件，LLM 提取逻辑 |

### Phase 3：MCP 工具注入（Agent 主动查询）

**目标：** TL 等需要跨项目探索的角色可以按需搜索知识和记忆。

#### 3.1 工具清单

| 工具 | 用途 | 首要使用者 |
|------|------|-----------|
| `memory_search(query, project_id?)` | 搜索执行经验 | TL |
| `memory_save(content, kind, tags)` | 手动保存观察 | TL |
| `project_info(project_id)` | 查看项目详情 | TL |
| `flow_status(flow_id)` | 查看 Flow 进度 | TL |

#### 3.2 注册方式

通过 AgentProfile.MCP.Tools 配置哪些角色拥有哪些工具。Engine 启动 ACP session 时注册：

```toml
[runtime.agents.profiles.team_leader]
skills = ["planning"]
mcp.tools = ["memory_search", "memory_save", "project_info", "flow_status"]

[runtime.agents.profiles.implement]
# Worker 不需要主动查询工具，靠 Briefing 被动注入即可
mcp.tools = []
```

#### 3.3 与 Phase 1/2 的关系

Phase 3 是**增量能力**，不是替代：
- Phase 1（被动注入）对所有 Agent 生效，保证基线上下文
- Phase 3（主动查询）仅对 TL 等角色开放，用于探索性任务

两者互补，不冲突。

### Phase 4：OpenViking 集成（可选加速器）

**目标：** 用 OpenViking 替换本地实现中能力有限的部分。

#### 4.1 替换矩阵

| 本地能力 | OpenViking 替换 | 价值 |
|---------|----------------|------|
| `renderProjectBrief()` 手工拼接 | L0/L1 自动摘要 | 摘要质量大幅提升 |
| `SearchMemories` 关键词匹配 | 语义向量搜索 | 召回率提升 |
| `MemoryExtractor` 手工 LLM 调用 | `session.Commit()` 自动提取 | 开发量减少 |
| 无跨项目语义搜索 | `context_search(query)` | 新能力 |

#### 4.2 接入策略

通过 `ContextStore` 接口抽象（你 spec 里已经定义了）。本地 SQLite 实现和 OpenViking 实现共享接口：

```go
type ContextStore interface {
    // Phase 1-2 本地实现用 SQLite
    // Phase 4 替换为 OpenViking client
    Abstract(ctx context.Context, uri string) (string, error)
    Overview(ctx context.Context, uri string) (string, error)
    Search(ctx context.Context, query string, opts SearchOpts) ([]ContextResult, error)
}
```

配置切换：
```toml
[context]
provider = "sqlite"      # Phase 1-2
# provider = "openviking"  # Phase 4
```

## 关于"要不要搭知识库"的决策

### 不建议自建 RAG 知识库

理由：
1. OpenViking 已经提供了 L0/L1 摘要 + 语义搜索，自建是重复造轮子
2. 维护 embedding 模型 + 向量索引 + 分块策略的成本很高
3. 你的场景是"项目理解 + 经验召回"，不是通用 RAG

### 建议路径

```
Phase 1 (1-2 天)     Phase 2 (3-5 天)      Phase 3 (2-3 天)     Phase 4 (按需)
补齐 Briefing 管道 → 本地记忆存储 SQLite → MCP 工具注入 TL  → OpenViking 集成
- ProjectBrief       - MemoryEntry 模型     - memory_search      - L0/L1 替换手工摘要
- FlowSummary        - 自动提取             - memory_save        - 语义搜索替换关键词
- Skills 注入        - 关键词召回            - project_info       - session.Commit
                     - Briefing 注入
```

Phase 1 投入产出比最高：改动 2-3 个文件，0 新依赖，所有 Agent 立即受益。

## Token 预算控制

### Briefing 字符预算（已实现，`pipeline.go` `refBudget()`）

| 注入项 | 字符预算 | 状态 |
|--------|----------|------|
| IssueSummary / ProjectBrief | ≤ 800 字符 | ✅ IssueSummary 已填充, ProjectBrief 待实现 |
| FeatureManifest | ≤ 2000 字符 | ✅ 已填充 |
| AgentMemory | ≤ 1500 字符 | 🔲 待实现 |
| UpstreamArtifact | ≤ 4000 字符/ref | ✅ 已填充 (L0 summary / L2 full) |
| **整体 BriefingSnapshot** | **≤ 12000 字符** | ✅ 超限自动截断 |

### Session Token 预算（已实现，`acp_session_pool.go`）

| 级别 | 条件 | 行为 |
|------|------|------|
| OK | < warn ratio (默认 80%) | 正常执行 |
| Warning | ≥ warn ratio | slog 告警, 继续执行 |
| Exceeded | ≥ 100% | 返回 `ErrTokenBudgetExceeded`, 步骤进入 blocked/retry |

配置: `ProfileSession.MaxContextTokens` + `ContextWarnRatio` (默认 0.8)。

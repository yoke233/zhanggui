# PromptBuilder + Memory 分层上下文设计

日期: 2026-03-09
状态: approved

## 1. 目标

将当前平铺的 prompt 模板渲染升级为分层上下文拼装，让 AI agent 在执行 Stage 和 Review 时能看到任务背景、兄弟任务状态、最近事件和审查记录，显著提升 prompt 质量。

### 设计原则

- **渐进替换** — 保留现有 `.tmpl` 模板，通过新增字段注入分层上下文
- **接口隔离** — Memory 接口定义在 core/，实现可替换（SQLite / OpenViking）
- **不增加 LLM 调用** — 所有上下文从现有数据库查询获得，无额外 AI 开销
- **向后兼容** — Memory 返回空时，prompt 退化为原来的行为

## 2. 现状

### 当前 Prompt 构建

- `engine/prompts.go` — 42 行，Go template 平铺渲染
- `engine/prompt_templates/*.tmpl` — 4 个模板（implement/code_review/fixup/requirements）
- `PromptVars` — 10 个字段，无分层概念
- 无记忆、无缓存、每次从零构建

### AI 当前看到的信息

- 项目名、仓库路径、工作目录
- 需求描述（Issue body）
- 重试错误、合并冲突提示
- **看不到**：兄弟任务状态、历史事件、审查详情

## 3. Memory 接口

```go
// internal/core/memory.go

// Memory provides layered context for PromptBuilder.
type Memory interface {
    // RecallCold returns rarely-changing background (Issue title + body preview).
    RecallCold(issueID string) (string, error)

    // RecallWarm returns parent + sibling issue summaries.
    RecallWarm(issueID string) (string, error)

    // RecallHot returns recent TaskSteps + filtered RunEvents + ReviewRecords.
    RecallHot(issueID string, runID string) (string, error)
}
```

### 三层含义

| 层级 | 变化频率 | 内容 | 数据来源 |
|------|---------|------|---------|
| 冷层 | 几乎不变 | Issue title + body 前 500 字符 | `store.GetIssue()` |
| 温层 | 偶尔变 | 父 Issue title/body + 兄弟 Issue title/status | `store.GetIssue()` + `store.GetChildIssues()` |
| 热层 | 经常变 | 最近 TaskSteps + 过滤后 RunEvents + ReviewRecords | `store.ListTaskSteps()` + `store.ListRunEvents()` + `store.GetReviewRecords()` |

## 4. SQLite Memory 实现

放在 `internal/plugins/store-sqlite/`，在现有 SQLiteStore 基础上实现。

### 4.1 RecallCold

```go
func (m *SQLiteMemory) RecallCold(issueID string) (string, error) {
    issue, err := m.store.GetIssue(issueID)
    // → "## 任务背景\n标题: {title}\n描述: {body前500字符}"
}
```

### 4.2 RecallWarm

```go
func (m *SQLiteMemory) RecallWarm(issueID string) (string, error) {
    issue, _ := m.store.GetIssue(issueID)
    if issue.ParentID == "" { return "", nil }  // 无父任务 → 空
    parent, _ := m.store.GetIssue(issue.ParentID)
    siblings, _ := m.store.GetChildIssues(issue.ParentID)
    // → "## 父任务\n标题: {parent.Title}\n描述: {parent.Body前300字符}\n\n"
    //   "## 兄弟任务\n- {sibling.Title} [状态: done]\n..."
    // 过滤掉自己，最多列 10 个兄弟
}
```

### 4.3 RecallHot

```go
func (m *SQLiteMemory) RecallHot(issueID string, runID string) (string, error) {
    steps, _ := m.store.ListTaskSteps(issueID)     // 最近 20 条
    events, _ := m.store.ListRunEvents(runID)       // 过滤 type=prompt|done|agent_message，最后 5 条
    reviews, _ := m.store.GetReviewRecords(issueID) // 全部
    // → "## 最近事件\n{step.CreatedAt} {step.Action}: {step.Note}\n..."
    //   "## 最近执行\n{event.Type}: {event.Data前200字符}\n..."
    //   "## 审查记录\n第{round}轮 {reviewer}: {verdict} - {summary}\n..."
}
```

### 数据量控制

- 冷层: ~500 字符（固定）
- 温层: ~800 字符（与兄弟数量相关）
- 热层: ~2000-5000 字符（与历史深度相关）
- 初期靠条目数限制控制大小，不做 token 计数截断

## 5. PromptBuilder

放在 `internal/engine/prompt_builder.go`，包装现有 `RenderPrompt()`。

```go
type PromptBuilder struct {
    memory core.Memory
}

func (b *PromptBuilder) Build(issue *core.Issue, run *core.Run, stage string, baseVars PromptVars) (string, error) {
    cold, _ := b.memory.RecallCold(issue.ID)
    warm, _ := b.memory.RecallWarm(issue.ID)
    hot, _  := b.memory.RecallHot(issue.ID, run.ID)

    baseVars.ColdContext = cold
    baseVars.WarmContext = warm
    baseVars.HotContext  = hot

    return RenderPrompt(stage, baseVars)
}
```

### PromptVars 新增字段

```go
type PromptVars struct {
    // ...现有 10 个字段不变
    ColdContext  string  // 任务背景（Issue 描述）
    WarmContext  string  // 父/兄弟任务概览
    HotContext   string  // 最近事件、执行记录、审查记录
}
```

### 模板改造

以 `implement.tmpl` 为例，在模板头部插入上下文占位符：

```
{{if .ColdContext}}
{{.ColdContext}}

{{end}}
{{if .WarmContext}}
{{.WarmContext}}

{{end}}
你正在项目 {{.ProjectName}} 的 worktree ({{.WorktreePath}}) 中工作。
...（现有内容不变）...

{{if .HotContext}}
{{.HotContext}}
{{end}}
```

排列顺序：冷 → 温 → 现有模板内容 → 热。稳定内容在前（利于 LLM prefix cache），动态内容在后。

4 个模板（implement/code_review/fixup/requirements）统一加上这三个占位符。

## 6. 集成点

### Executor 注入

```go
// engine/executor.go
type Executor struct {
    // ...现有字段
    promptBuilder *PromptBuilder
}

func NewExecutor(..., memory core.Memory) *Executor {
    return &Executor{
        // ...
        promptBuilder: &PromptBuilder{memory: memory},
    }
}
```

### executor_stages.go 改造

```go
// 原来
prompt, err := RenderPrompt(promptStage, vars)

// 改为
prompt, err := e.promptBuilder.Build(issue, run, promptStage, vars)
```

需要在 executeStage() 中获取 Issue 对象（通过 `store.GetIssueByRun(run.ID)` 或 Run.IssueID 查）。

### Review 侧改造

`teamleader/review.go` 中构建审查 prompt 时，同样通过 PromptBuilder 注入上下文。

### 启动链路

```go
// cmd/ai-flow/commands.go
store := storesqlite.New(...)
memory := storesqlite.NewSQLiteMemory(store)
executor := engine.NewExecutor(..., memory)
```

## 7. OpenViking 预留

```
internal/core/memory.go              ← Memory 接口（本次）
internal/plugins/store-sqlite/       ← SQLiteMemory 实现（本次）
internal/plugins/context-viking/     ← VikingMemory 实现（后续）
```

后续切换：

```go
var memory core.Memory
if cfg.Context.Provider == "viking" {
    memory = viking.NewVikingMemory(cfg.Context.Path)
} else {
    memory = storesqlite.NewSQLiteMemory(store)
}
```

与已有 `ContextStore` 接口的关系：
- `ContextStore`（core/context.go）— OV 完整能力（CRUD + 语义搜索 + Session）
- `Memory`（core/memory.go）— PromptBuilder 需要的最小接口（3 个方法）
- `VikingMemory` 内部调 `ContextStore` 实现 `Memory` 接口

### 本次不做

- 不实现 VikingMemory
- 不实现 ContextStore
- 不加 token 计数和精细预算分配
- 不做 Memory Compact（冷层压缩）
- 不加 retrieval_trace

## 8. 改造范围

### 新增

- `internal/core/memory.go` — Memory 接口定义
- `internal/engine/prompt_builder.go` — PromptBuilder
- `internal/plugins/store-sqlite/memory.go` — SQLiteMemory 实现

### 改造

- `internal/engine/prompts.go` — PromptVars 新增 3 个字段
- `internal/engine/prompt_templates/*.tmpl` — 4 个模板加上下文占位符
- `internal/engine/executor.go` — 注入 Memory，构造 PromptBuilder
- `internal/engine/executor_stages.go` — 调用 PromptBuilder.Build 替代 RenderPrompt
- `internal/teamleader/review.go` — 审查 prompt 注入上下文
- `cmd/ai-flow/commands.go` — 启动链路创建 SQLiteMemory

### 不变

- Store 接口（不加新方法）
- ACP 通信层
- RenderPrompt() 函数（保留，PromptBuilder 内部调用）
- 数据库 schema（不加新表）

## 9. 测试策略

### 单元测试

1. **SQLiteMemory 测试**（`store-sqlite/memory_test.go`）
   - RecallCold — 正常 / Issue 不存在
   - RecallWarm — 有父兄弟 / 无父任务
   - RecallHot — 有完整数据 / 空数据

2. **PromptBuilder 测试**（`engine/prompt_builder_test.go`）
   - 三层都有内容 / Memory 全返回空（向后兼容） / 部分有内容

3. **模板渲染测试** — 验证新字段在模板中正确渲染

## 10. 未来演进

- **M3 Memory Compact** — 冷层从 Issue 描述升级为 LLM 压缩摘要
- **Token 预算** — 加 token 计数，按 冷15%/任务10%/温25%/热50% 分配
- **版本号缓存** — 温层加版本号驱动的应用层缓存
- **OpenViking 集成** — 实现 VikingMemory，用 L0/L1/L2 分层检索替代简单查询
- **retrieval_trace** — Decision 记录"AI 看到了什么上下文"

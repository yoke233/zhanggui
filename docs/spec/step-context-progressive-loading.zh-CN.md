# Step Context 渐进式加载

> 状态：部分实现
>
> 最后按代码核对：2026-03-14
>
> 当前实现状态：当前主链仍以 `BriefingSnapshot` 为 prompt 主体，但 `step-context` 的临时技能注入已经部分落地。需要注意当前代码存在两层兼容命名：`ACPExecutorConfig.StepContextBuilder` 的真实类型是 `*skills.ActionContextBuilder`，生成的 `SKILL.md` frontmatter 名称仍是 `action-context`，但运行期挂载目录和 prompt 提示统一使用 `skills/step-context/`。

## 概述

当前 Action 执行时，所有上下文（WorkItem 摘要、上游 Deliverable、Feature Manifest 等）被塞入一个 ≤12000 字符的 BriefingSnapshot prompt，一次性发送给 Agent。这带来两个问题：

1. **字符预算硬限** — 上游 Deliverable 限 4000 字符、WorkItem 限 800 字符，大量有价值的信息被截断
2. **一次性加载** — Agent 不一定需要所有信息，但必须承受全量 prompt 的 token 开销

**Step Context 渐进式加载**利用现有的 Skill 注入机制，在 Agent 执行前将完整的参考材料写入磁盘文件，Agent 在 SKILL.md 索引的引导下按需读取。Briefing prompt 精简为摘要 + 引用指针，不再承载完整内容。

## 与现有上下文构建的关系

本方案是 [execution-context-building.zh-CN.md](execution-context-building.zh-CN.md) 的**增强层**，不替代现有的三阶段管道（Prepare → Execute → Finalize）。

```
现有流程:
  Briefing (≤12000 字符) ──全量塞入──→ Prompt ──发给──→ Agent

增强后:
  Briefing (精简摘要 ~3000 字符) ──摘要──→ Prompt ──发给──→ Agent
  StepContextBuilder ──完整材料──→ skills/step-context/ ──按需读取──→ Agent
```

## 设计原则

1. **引擎准备，Agent 取用** — 引擎负责在执行前把材料写好，Agent 通过文件系统按需读取
2. **向后兼容** — BriefingSnapshot 仍然包含摘要级上下文，即使 Agent 不读磁盘文件也能工作
3. **复用现有基础设施** — 使用 Skill 的 symlink 机制分发文件，使用 Sandbox 的 scope 隔离目录
4. **临时生命周期** — step-context 是 per-execution 的一次性目录，执行完自动清理

## 流程图

```
┌─────────────────────────────────────────────────────────────────────┐
│                    WorkItemEngine.prepare()                          │
│                                                                      │
│  BriefingBuilder.Build()  →  Briefing (结构化对象, 同现有逻辑)         │
│  renderBriefingSnapshot() →  BriefingSnapshot (精简 Markdown)         │
│                              ↓                                       │
│                              存入 Run.BriefingSnapshot                │
└──────────────────────────────┬──────────────────────────────────────┘
                               │
╔══════════════════════════════▼══════════════════════════════════════╗
║                     ACPStepExecutor.Execute()                       ║
║                                                                     ║
║  ┌─────────────────────────────────────────────────────────────┐   ║
║  │ Phase A: 材料预投放 (新增)                                    │   ║
║  │                                                              │   ║
║  │  ActionContextBuilder.Build(action, run)                    │   ║
║  │    │                                                         │   ║
║  │    ├── 读取 WorkItem (完整 body, 不截断)                      │   ║
║  │    ├── 读取所有上游 Deliverable (完整 ResultMarkdown)          │   ║
║  │    ├── 读取 AcceptanceCriteria                                │   ║
║  │    ├── 读取 Gate Feedback + Rework History                    │   ║
║  │    ├── 读取 Feature Manifest (完整 JSON)                      │   ║
║  │    │                                                         │   ║
║  │    └── 写入磁盘:                                              │   ║
║  │         step-context/                                         │   ║
║  │         ├── SKILL.md          ← 动态生成的索引                  │   ║
║  │         ├── issue.md          ← 完整 WorkItem                  │   ║
║  │         ├── upstream/                                          │   ║
║  │         │   ├── requirements.md                                │   ║
║  │         │   └── design.md                                      │   ║
║  │         ├── acceptance.md                                      │   ║
║  │         ├── gate-feedback.md                                   │   ║
║  │         └── manifest.json                                      │   ║
║  └─────────────────────────────────────────────────────────────┘   ║
║                               │                                     ║
║  ┌────────────────────────────▼────────────────────────────────┐   ║
║  │ Phase B: Session 获取                                        │   ║
║  │                                                              │   ║
║  │  SessionManager.Acquire(SessionAcquireInput{                  │   ║
║  │    ExtraSkills:    ["step-signal", "step-context"],           │   ║
║  │    EphemeralSkills: {"step-context": stepContextDir},         │   ║
║  │  })                                                           │   ║
║  │    │                                                         │   ║
║  │    └── Sandbox.Prepare()                                      │   ║
║  │         ├── link step-signal   (全局 skill, symlink)           │   ║
║  │         └── link step-context  (临时 skill, symlink 到生成目录) │   ║
║  └─────────────────────────────────────────────────────────────┘   ║
║                               │                                     ║
║  ┌────────────────────────────▼────────────────────────────────┐   ║
║  │ Phase C: 发送 Prompt                                         │   ║
║  │                                                              │   ║
║  │  BuildRunInputForAction()                                     │   ║
║  │    → "# Task"                                                 │   ║
║  │    → "{精简 BriefingSnapshot}"                                 │   ║
║  │    → "# Reference Materials"                                  │   ║
║  │    → "> 完整材料在 skills/step-context/, 按需读取"              │   ║
║  │    → "# Acceptance Criteria"                                  │   ║
║  │    → "- criterion 1 ..."                                      │   ║
║  └─────────────────────────────────────────────────────────────┘   ║
║                               │                                     ║
║  ┌────────────────────────────▼────────────────────────────────┐   ║
║  │ Phase D: Agent 执行                                          │   ║
║  │                                                              │   ║
║  │  Agent 启动 → 自动读取 skills/step-context/SKILL.md           │   ║
║  │    → 看到材料索引表                                            │   ║
║  │    → 按需 Read issue.md / upstream/*.md / ...                 │   ║
║  │    → 完成后 signal.sh complete "..."                          │   ║
║  └─────────────────────────────────────────────────────────────┘   ║
║                               │                                     ║
║  defer: Cleanup(stepContextDir)  ← 执行完自动清理临时目录           ║
╚═════════════════════════════════════════════════════════════════════╝
```

## 材料类型

### 固定材料

以下材料在每次 Action 执行时自动生成（如果数据存在）：

| 文件 | 来源 | 内容 | 生成条件 |
|------|------|------|----------|
| `issue.md` | `Store.GetWorkItem()` | WorkItem 完整 Title + Body + Priority + Labels | WorkItem 存在 |
| `upstream/<name>.md` | `Store.GetLatestRunWithResult()` | 前置 Action 的完整 `ResultMarkdown` | 有上游 Deliverable |
| `acceptance.md` | `action.AcceptanceCriteria` | 编号列表的验收标准 | AcceptanceCriteria 非空 |
| `gate-feedback.md` | `action.Config["last_gate_feedback"]` + `action.Config["rework_history"]` | Gate 反馈 + 历史 | 存在反馈 |
| `manifest.json` | `Store.GetFeatureManifestByProject()` | 完整的 Feature Manifest JSON | Project 有 Manifest |

### 预留材料（未来扩展）

| 文件 | 用途 | 对应 ContextRefType |
|------|------|---------------------|
| `project-brief.md` | 项目简报 | `CtxProjectBrief` |
| `agent-memory.md` | Agent 跨 Step 记忆 | `CtxAgentMemory` |
| `codebase/file-map.md` | 代码库关键文件路径 | 新增 |
| `codebase/architecture.md` | 项目架构描述 | 新增 |
| `related-work-items.md` | 关联 WorkItem 列表 | 新增 |

## SKILL.md 动态生成

`SKILL.md` 由 `ActionContextBuilder` 在运行时动态生成，内容根据实际写入的材料文件生成索引表：

```markdown
---
name: step-context
description: Pre-loaded reference materials for the current step execution
---

# Step Context

Reference materials for your current task are in this directory.
**Read files on demand** — you don't need to load everything upfront.

## Available Materials

| File | Description | When to read |
|------|-------------|--------------|
| `issue.md` | Full work item details (title, body, priority, labels) | At start — understand the full task |
| `upstream/requirements.md` | Full output of upstream step "requirements" | When you need context from this predecessor step |
| `upstream/design.md` | Full output of upstream step "design" | When you need context from this predecessor step |
| `acceptance.md` | Detailed acceptance criteria for this step | Before signaling completion — verify all criteria |
| `gate-feedback.md` | Previous review feedback and rework history | Immediately — understand what to fix |

## How to use

1. Your task prompt already contains the objective and a brief summary
2. Read individual files above when you need more detail
3. Check `acceptance.md` before signaling completion (if present)
4. If `gate-feedback.md` exists, read it first — it contains rework instructions
```

**注意**：step-context 是临时 skill，不需要额外的 frontmatter 字段；只保留 `name` 和 `description` 即可。

## 代码变更

### 新增文件

| 文件 | 职责 |
|------|------|
| `internal/skills/context_builder.go` | `ActionContextBuilder` — 根据 Action/WorkItem/Deliverables 生成 step-context 目录 |
| `internal/skills/context_builder_test.go` | 单元测试 |

### 修改文件

| 文件 | 变更 |
|------|------|
| `internal/adapters/executor/acp.go` | `ACPExecutorConfig` 保留 `StepContextBuilder` 字段名，但其真实类型已是 `*skills.ActionContextBuilder`；executor 中调用 Build + 注入 extraSkills + defer Cleanup |
| `internal/adapters/sandbox/sandbox.go` | `PrepareInput` 新增 `EphemeralSkills map[string]string` 字段 |
| `internal/adapters/sandbox/home_dir.go` | `Prepare()` 中处理 `EphemeralSkills` — 直接 link 预生成目录 |
| `internal/application/runtime/session.go` | `SessionAcquireInput` 新增 `EphemeralSkills` 字段，透传到 Sandbox |
| `internal/application/flow/execution_input.go` | `BuildRunInputForAction` / `BuildRunInputFromSnapshot` 新增 action-context 引导文案 |
| `internal/platform/bootstrap/bootstrap_engine.go` | 装配 `ActionContextBuilder` 并注入 `ACPExecutorConfig.StepContextBuilder` |

## ActionContextBuilder 接口

```go
package skills

// ContextMaterial represents one reference file in the action-context skill
// (mounted at runtime as skills/step-context/).
type ContextMaterial struct {
    Path        string // relative path within skill dir, e.g. "issue.md"
    Description string // one-line description for the index table
    Hint        string // when the agent should read this
    Content     string // file content
}

// ActionContextBuilder generates a per-execution action-context skill directory
// containing full reference materials that the agent can read on demand.
type ActionContextBuilder struct {
    store core.Store
}

func NewActionContextBuilder(store core.Store) *ActionContextBuilder

// Build generates the action-context directory under parentDir/run-<id>/action-context/
// and returns the full path. The caller must defer Cleanup(dir).
//
// Materials generated (when data exists):
//   - issue.md:           full WorkItem title + body + metadata
//   - upstream/<name>.md: full ResultMarkdown of each predecessor action
//   - acceptance.md:      numbered acceptance criteria
//   - gate-feedback.md:   last_gate_feedback + rework_history
//   - manifest.json:      full feature manifest entries
//   - SKILL.md:           auto-generated index of all above
func (b *ActionContextBuilder) Build(ctx context.Context, parentDir string,
    action *core.Action, run *core.Run) (dir string, err error)

// Cleanup removes the step-context directory. Safe to call with empty string.
func Cleanup(dir string)
```

## Executor 集成

### ACPExecutorConfig 变更

```go
type ACPExecutorConfig struct {
    // ... 现有字段不变 ...

    // StepContextBuilder generates per-execution reference materials.
    // When nil, step-context is not injected (graceful degradation).
    StepContextBuilder *skills.ActionContextBuilder
}
```

### Execute 函数内的注入逻辑

在现有 `extraSkills` 赋值之后、`SessionManager.Acquire()` 之前插入：

```go
// --- step-context: progressive loading ---
var stepContextDir string
var ephemeralSkills map[string]string
if cfg.StepContextBuilder != nil &&
    (step.Type == core.ActionExec || step.Type == core.ActionGate) {

    ctxParentDir := filepath.Join(workDir, ".ai-workflow", "step-contexts")
    dir, buildErr := cfg.StepContextBuilder.Build(ctx, ctxParentDir, step, exec)
    if buildErr != nil {
        slog.Warn("step-context: build failed, proceeding without",
            "step_id", step.ID, "error", buildErr)
    } else if dir != "" {
        stepContextDir = dir
        extraSkills = append(extraSkills, "step-context")
        ephemeralSkills = map[string]string{"step-context": stepContextDir}
        slog.Info("step-context: materials prepared",
            "step_id", step.ID, "dir", dir)
    }
}

defer func() {
    if stepContextDir != "" {
        skills.Cleanup(stepContextDir)
    }
}()
```

## Sandbox 变更

### PrepareInput

```go
type PrepareInput struct {
    Profile *core.AgentProfile
    Driver  *core.AgentDriver
    Launch  acpclient.LaunchConfig
    Scope   string

    ExtraSkills []string

    // EphemeralSkills maps skill names to pre-built directories on disk.
    // These directories are linked directly into the agent's skills dir,
    // bypassing the global skillsRoot. Used for per-execution materials.
    EphemeralSkills map[string]string
}
```

### HomeDirSandbox.Prepare()

在现有 `EnsureSkillsLinked` 之后追加：

```go
// Link ephemeral skills (pre-built directories, e.g. step-context).
for name, srcDir := range in.EphemeralSkills {
    dst := filepath.Join(skillsDir, name)
    // Ephemeral skills are per-execution; remove stale link if exists.
    if _, statErr := os.Lstat(dst); statErr == nil {
        _ = os.RemoveAll(dst)
    }
    if err := linkDir(dst, srcDir); err != nil {
        slog.Warn("link ephemeral skill failed",
            "skill", name, "src", srcDir, "error", err)
        // Non-fatal: agent can still work without the materials.
    }
}
```

其中 `linkDir` 复用现有的 `linkPathIfMissing` 内部逻辑（symlink + Windows junction fallback）。

## Prompt 变更

### BuildRunInputForAction 签名变更

```go
func BuildRunInputForAction(
    profile *core.AgentProfile,
    snapshot string,
    action *core.Action,
    hasPriorTurns bool,
    feedback string,
    reworkTmpl string,
    continueTmpl string,
    hasActionContext bool,      // ← 新增
) string
```

### 新增 Reference Materials 章节

当 `hasActionContext == true` 时，在 Acceptance Criteria 之前插入：

```markdown
# Reference Materials

> Full details (issue body, upstream outputs, feature manifest) are pre-loaded
> in `skills/step-context/`. Read the `SKILL.md` there for an index of
> available files. Read individual files on demand — do not load everything.
```

## 与现有 Briefing 预算的协作

step-context 不改变现有 BriefingBuilder 的逻辑和字符预算。两层并存：

| 层 | 内容 | 字符限制 | 用途 |
|----|------|----------|------|
| **Prompt 层** (BriefingSnapshot) | 摘要：Objective + WorkItem 摘要 + Upstream 摘要 + Manifest 精简 | ≤12000 字符 | Agent 的即时任务理解 |
| **文件层** (step-context) | 完整版：WorkItem 全文 + Deliverable 全文 + Manifest 全量 JSON | 无限制 | Agent 按需深入 |

当前阶段 Briefing 预算不变；未来可以考虑在有 step-context 时**主动缩减** Briefing 预算（例如 WorkItem 摘要改为仅 Title），以节省 prompt token。

## 生命周期管理

```
ActionContextBuilder.Build()
    → 写入 .ai-workflow/step-contexts/run-<id>/action-context/
    → Sandbox 链接到 agent home: skills/step-context → 上述目录

Agent 执行中
    → 通过文件系统读取 skills/step-context/ 下的文件
    → 写入的文件为只读（Agent 不应修改）

ACPStepExecutor defer
    → Cleanup(stepContextDir)
    → 删除 .ai-workflow/step-contexts/run-<id>/ 整个目录

Sandbox 中的 symlink 自动失效（指向已删除目录）
    → 下次 Sandbox.Prepare() 时会重新创建
```

**异常安全**：
- `Build()` 失败 → 不注入 step-context，Agent 仍使用 Briefing prompt（降级）
- 链接失败 → 非致命，slog.Warn 后继续
- 清理失败 → 忽略，目录在 `.ai-workflow/step-contexts/` 下，可手动清理

## 前提条件

- Agent 必须具备 `fs_read` 能力才能读取 step-context 文件
- Agent 运行时必须支持 SKILL.md 自动发现（Claude Code / Codex 均支持）
- 如果 Agent 不支持 fs_read，仍可使用 Briefing prompt 中的摘要信息（降级模式）

## 对比：当前 vs 渐进式加载

| 维度 | 当前方案 | 渐进式加载 |
|------|----------|-----------|
| **信息量** | ≤12000 字符，超限截断 | 无限制，磁盘文件 |
| **加载方式** | 全量塞入 prompt | Agent 按需读取 |
| **Token 开销** | 固定：全量 prompt 消耗 | 弹性：Agent 只读需要的文件 |
| **复用基础设施** | — | 复用 Skill symlink + Sandbox scope |
| **新增代码** | — | ~300 行 Go (builder + 修改) |
| **向后兼容** | — | 完全兼容，Briefing 仍存在 |
| **清理** | — | defer 自动清理，无残留 |

## 测试策略

### 单元测试 (`context_builder_test.go`)

| 用例 | 验证点 |
|------|--------|
| `TestBuild_FullMaterials` | WorkItem + upstream + acceptance + manifest 均生成 |
| `TestBuild_NoIssue` | WorkItem 不存在时跳过 issue.md |
| `TestBuild_NoUpstream` | 无上游 Action 时不生成 upstream/ |
| `TestBuild_GateFeedback` | 有 gate 反馈时生成 gate-feedback.md |
| `TestBuild_SkillMD_Index` | SKILL.md 索引表包含所有实际文件 |
| `TestBuild_LargeArtifact` | 大体量 Deliverable 不被截断 |
| `TestCleanup` | 清理后目录不存在 |
| `TestSanitizeFileName` | 特殊字符被正确处理 |

### 集成测试

| 场景 | 验证点 |
|------|--------|
| E2E Action 执行 | step-context 目录在执行前存在，执行后被清理 |
| Agent 实际读取 | Agent 的输出引用了 step-context 中的文件内容 |
| 降级模式 | `StepContextBuilder = nil` 时行为与现有完全一致 |
| Session 复用 | 复用会话时 step-context 被刷新（新 run 的材料） |

## 相关文件

| 文件 | 职责 |
|------|------|
| `internal/skills/context_builder.go` | **新增** — ActionContextBuilder 核心实现 |
| `internal/skills/context_builder_test.go` | **新增** — 单元测试 |
| `internal/adapters/executor/acp.go` | **修改** — 注入 step-context |
| `internal/adapters/sandbox/sandbox.go` | **修改** — PrepareInput 新增 EphemeralSkills |
| `internal/adapters/sandbox/home_dir.go` | **修改** — 处理 EphemeralSkills 链接 |
| `internal/application/runtime/session.go` | **修改** — SessionAcquireInput 透传 EphemeralSkills |
| `internal/application/flow/execution_input.go` | **修改** — 追加 Reference Materials 提示 |
| `internal/platform/bootstrap/bootstrap_engine.go` | **修改** — 装配 `ActionContextBuilder` 到 `ACPExecutorConfig.StepContextBuilder` |
| `docs/spec/execution-context-building.zh-CN.md` | **参考** — 现有上下文构建流程 |

# P1 Implementation Plan (多项目调度 + 配置驱动插件)

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 在已完成 P0 的基础上，实现可日常使用的 P1 能力：多项目并发调度、崩溃恢复、人工动作流转、三级配置合并，以及插件工厂化初始化。

**Architecture:** 以 `Scheduler + Executor + Store` 三层协作实现异步编排。`Scheduler` 负责 FIFO 队列与并发配额（全局/项目/worktree 锁）；`Executor` 负责单条 Pipeline 的阶段执行与动作处理；`Store` 持久化状态与检查点，用于恢复。启动流程改为“配置驱动插件工厂”，不再在 `bootstrap()` 硬编码具体实现。

**Tech Stack:** Go 1.25+, SQLite (modernc.org/sqlite), Bubble Tea, gopkg.in/yaml.v3, os/exec, slog, Go channel/Mutex/semaphore。

**Spec Sources (P1 重点):**
- `spec-overview.md`（P1 范围、P1 引入 factory + 配置驱动）
- `spec-pipeline-engine.md`（崩溃恢复、人工动作、重试预算、Reactions）
- `spec-agent-drivers.md`（并发调度资源模型、worktree 排他）
- `spec-api-config.md`（三级配置、环境变量覆盖、调度参数）

**Out of Scope (本计划不做):**
- GitHub Webhook / Issue/PR 双向联动（P2）
- Web API / WebSocket / Dashboard（P3）
- Notifier 多通道、自定义模板高级能力（P4）

---

## Task 1: 定义 P1 领域模型与调度接口

**Files:**
- Create: `internal/core/action.go`
- Create: `internal/core/scheduler.go`
- Modify: `internal/core/pipeline.go`
- Test: `internal/core/action_test.go`

**Step 1: 先写失败测试（动作与状态约束）**

```go
// internal/core/action_test.go
package core

import "testing"

func TestHumanActionTypeValidate(t *testing.T) {
    actions := []HumanActionType{
        ActionApprove, ActionReject, ActionModify, ActionSkip,
        ActionRerun, ActionChangeAgent, ActionAbort, ActionPause, ActionResume,
    }
    for _, a := range actions {
        if err := a.Validate(); err != nil {
            t.Fatalf("expected valid action %s, got %v", a, err)
        }
    }
}
```

**Step 2: 运行测试确认失败**

Run: `go test ./internal/core -run TestHumanActionTypeValidate -v`  
Expected: FAIL（类型或方法尚未定义）

**Step 3: 实现动作模型与调度接口**

```go
// internal/core/action.go
type HumanActionType string
const (
  ActionApprove HumanActionType = "approve"
  ActionReject HumanActionType = "reject"
  // ...
)
type PipelineAction struct {
  PipelineID string
  Type HumanActionType
  Stage StageID
  Message string
  Agent string
}
```

```go
// internal/core/scheduler.go
type Scheduler interface {
  Start(ctx context.Context) error
  Stop(ctx context.Context) error
  Enqueue(pipelineID string) error
}
```

**Step 4: 更新 Pipeline 元数据字段（为 P1 调度与恢复预留）**
- 添加 `LastHeartbeatAt`、`QueuedAt`、`RunCount`、`LastErrorType` 等必要字段。
- 保持 JSON 向后兼容（旧数据可反序列化）。

**Step 5: 跑核心测试**

Run: `go test ./internal/core -v`  
Expected: PASS

**Step 6: Commit**

```bash
git add internal/core/action.go internal/core/scheduler.go internal/core/pipeline.go internal/core/action_test.go
git commit -m "feat(core): add p1 human-action and scheduler contracts"
```

---

## Task 2: 完成三级配置合并与环境变量覆盖

**Files:**
- Modify: `internal/config/types.go`
- Create: `internal/config/project.go`
- Create: `internal/config/env.go`
- Modify: `internal/config/loader.go`
- Modify: `internal/config/merge.go`
- Test: `internal/config/merge_hierarchy_test.go`

**Step 1: 写失败测试（global + project + pipeline 三层）**

```go
func TestMergeHierarchy_GlobalProjectPipeline(t *testing.T) {
  // 断言下级覆盖上级，nil 继承，空数组清空
}
```

**Step 2: 运行失败测试**

Run: `go test ./internal/config -run TestMergeHierarchy_GlobalProjectPipeline -v`  
Expected: FAIL

**Step 3: 扩展配置结构（对齐 spec-api-config）**
- `agents` / `pipeline` / `scheduler` 使用可判空字段（指针或专用可选类型）。
- 增加 `project` 层配置结构，支持 `{repo}/.ai-workflow/config.yaml`。

**Step 4: 实现分层加载**
- 新增入口：
  - `LoadGlobal(path string)`
  - `LoadProject(repoPath string)`
  - `MergeForPipeline(global, project, override)`
- pipeline override 用 `map[string]any` 解码后合并到可识别字段。

**Step 5: 实现环境变量覆盖**
- 规则：`AI_WORKFLOW_{SECTION}_{KEY}`
- 最低覆盖：
  - `AI_WORKFLOW_AGENTS_CLAUDE_BINARY`
  - `AI_WORKFLOW_SERVER_PORT`
  - `AI_WORKFLOW_SCHEDULER_MAX_GLOBAL_AGENTS`
  - `AI_WORKFLOW_GITHUB_TOKEN`（先落字段，P2 才真正使用）

**Step 6: 回归测试**

Run: `go test ./internal/config -v`  
Expected: PASS

**Step 7: Commit**

```bash
git add internal/config/types.go internal/config/project.go internal/config/env.go internal/config/loader.go internal/config/merge.go internal/config/merge_hierarchy_test.go
git commit -m "feat(config): add 3-layer merge and env override for p1"
```

---

## Task 3: 插件注册表与工厂化启动（P1 关键）

**Files:**
- Create: `internal/core/registry.go`
- Create: `internal/plugins/factory/factory.go`
- Create: `internal/plugins/factory/factory_test.go`
- Modify: `cmd/ai-flow/commands.go`

**Step 1: 写失败测试（未知插件与已注册插件构建）**

```go
func TestFactoryBuildKnownPlugin(t *testing.T) {}
func TestFactoryBuildUnknownPlugin(t *testing.T) {}
```

**Step 2: 运行失败测试**

Run: `go test ./internal/plugins/factory -v`  
Expected: FAIL

**Step 3: 实现 registry + factory**
- `registry.Register(module PluginModule)`
- `registry.Get(slot, name)`
- `factory.BuildFromConfig(cfg config.Config) (*BootstrapSet, error)`

**Step 4: 将 `bootstrap()` 改为配置驱动**
- 用配置声明 agent/runtime/store 实例名。
- 默认仍保持 P0 行为（claude/codex/process/sqlite）。

**Step 5: 测试与构建**

Run:
- `go test ./internal/plugins/factory -v`
- `go test ./cmd/ai-flow -v`
- `go build ./cmd/ai-flow`

Expected: PASS

**Step 6: Commit**

```bash
git add internal/core/registry.go internal/plugins/factory/factory.go internal/plugins/factory/factory_test.go cmd/ai-flow/commands.go
git commit -m "feat(bootstrap): introduce plugin registry and config-driven factory"
```

---

## Task 4: 扩展 Store 查询能力（为 Scheduler/恢复提供原语）

**Files:**
- Modify: `internal/core/store.go`
- Modify: `internal/plugins/store-sqlite/migrations.go`
- Modify: `internal/plugins/store-sqlite/store.go`
- Create: `internal/plugins/store-sqlite/scheduler_store_test.go`

**Step 1: 先写失败测试（调度队列查询）**

```go
func TestListRunnablePipelinesFIFO(t *testing.T) {}
func TestCountRunningByProject(t *testing.T) {}
func TestMarkRunningCAS(t *testing.T) {}
```

**Step 2: 运行失败测试**

Run: `go test ./internal/plugins/store-sqlite -run Scheduler -v`  
Expected: FAIL

**Step 3: 扩展 Store 接口与 SQLite 实现**
- 新增接口方法：
  - `ListRunnablePipelines(limit int) ([]Pipeline, error)`
  - `CountRunningPipelinesByProject(projectID string) (int, error)`
  - `TryMarkPipelineRunning(id string, from ...PipelineStatus) (bool, error)`
  - `InvalidateCheckpointsFromStage(pipelineID string, stage StageID) error`
- 为 FIFO 增加 `queued_at` 字段（或复用 created_at，建议显式 `queued_at`）。

**Step 4: migration 与索引**
- 增加 `idx_pipelines_status_queued_at`
- 增加 `idx_pipelines_project_status`

**Step 5: 回归测试**

Run: `go test ./internal/plugins/store-sqlite -v`  
Expected: PASS

**Step 6: Commit**

```bash
git add internal/core/store.go internal/plugins/store-sqlite/migrations.go internal/plugins/store-sqlite/store.go internal/plugins/store-sqlite/scheduler_store_test.go
git commit -m "feat(store): add scheduler query primitives and fifo indexes"
```

---

## Task 5: 实现 P1 Scheduler（全局/项目/worktree 三层约束）

**Files:**
- Create: `internal/engine/scheduler.go`
- Create: `internal/engine/scheduler_test.go`
- Modify: `internal/engine/executor.go`

**Step 1: 先写失败测试（并发配额 + FIFO + worktree 排他）**

```go
func TestScheduler_RespectGlobalLimit(t *testing.T) {}
func TestScheduler_RespectPerProjectLimit(t *testing.T) {}
func TestScheduler_WorktreeExclusive(t *testing.T) {}
func TestScheduler_FIFO(t *testing.T) {}
```

**Step 2: 运行失败测试**

Run: `go test ./internal/engine -run Scheduler -v`  
Expected: FAIL

**Step 3: 实现调度器**
- 按 spec 使用两级并发门控：
  - `max_global_agents`
  - `max_project_pipelines`
- 增加 `worktree lock`（同 worktree 同时最多 1 个 agent stage）。
- `waiting_human` / `paused` 不占用信号量。

**Step 4: 集成 Executor**
- `pipeline start` 不再直接阻塞执行全部阶段（改为 enqueue + scheduler 驱动）。
- 保留兼容路径：单测可直接调用 executor 运行。

**Step 5: 回归测试**

Run: `go test ./internal/engine -v`  
Expected: PASS

**Step 6: Commit**

```bash
git add internal/engine/scheduler.go internal/engine/scheduler_test.go internal/engine/executor.go
git commit -m "feat(engine): add p1 scheduler with fifo and multi-level limits"
```

---

## Task 6: 人工动作与 Pause/Resume 语义落地

**Files:**
- Create: `internal/engine/actions.go`
- Create: `internal/engine/actions_test.go`
- Modify: `internal/engine/executor.go`
- Modify: `internal/core/events.go`

**Step 1: 写失败测试（approve/reject/modify/skip/rerun/change_agent/abort/pause/resume）**

```go
func TestActionApprove_ContinueNextStage(t *testing.T) {}
func TestActionReject_InvalidateFollowingCheckpoints(t *testing.T) {}
func TestActionPauseResume_ReRunCurrentStage(t *testing.T) {}
```

**Step 2: 跑失败测试**

Run: `go test ./internal/engine -run Action -v`  
Expected: FAIL

**Step 3: 实现动作处理器**
- `Approve`：`waiting_human -> running`
- `Reject(stage)`：目标 stage 及之后 checkpoint 标记 `invalidated`
- `Modify(message)`：写入 artifacts + prompt 注入后 rerun
- `Pause`：终止当前 session 并置 `paused`
- `Resume`：清理 worktree 脏状态后重跑当前 stage

**Step 4: 事件补齐**
- 增加 `pipeline_paused` / `pipeline_resumed` / `action_applied` 等事件，供 TUI 与后续 Web 使用。

**Step 5: 回归**

Run: `go test ./internal/engine -v`  
Expected: PASS

**Step 6: Commit**

```bash
git add internal/engine/actions.go internal/engine/actions_test.go internal/engine/executor.go internal/core/events.go
git commit -m "feat(engine): implement human actions and pause/resume semantics"
```

---

## Task 7: 崩溃恢复（running/paused/waiting_human）

**Files:**
- Create: `internal/engine/recovery.go`
- Create: `internal/engine/recovery_test.go`
- Modify: `cmd/ai-flow/commands.go`

**Step 1: 写失败测试（恢复策略）**

```go
func TestRecovery_RestoreWaitingHuman(t *testing.T) {}
func TestRecovery_ReRunInProgressCheckpoint(t *testing.T) {}
func TestRecovery_ResumeFromNextAfterSuccessCheckpoint(t *testing.T) {}
```

**Step 2: 运行失败测试**

Run: `go test ./internal/engine -run Recovery -v`  
Expected: FAIL

**Step 3: 实现恢复流程**
- 启动时扫描 `running|paused|waiting_human`。
- `in_progress` checkpoint 处理：
  - 标记 failed
  - 清理 worktree
  - 从当前 stage 重跑

**Step 4: CLI 启动接线**
- 在 `bootstrap` 后执行一次 `RecoverActivePipelines(ctx)`。
- 恢复逻辑仅启动一次，不阻塞主命令响应。

**Step 5: 回归**

Run:
- `go test ./internal/engine -v`
- `go test ./cmd/ai-flow -v`

Expected: PASS

**Step 6: Commit**

```bash
git add internal/engine/recovery.go internal/engine/recovery_test.go cmd/ai-flow/commands.go
git commit -m "feat(engine): add crash recovery for active pipelines"
```

---

## Task 8: Reactions Engine V1（替换分散 on_failure 逻辑）

**Files:**
- Create: `internal/engine/reactions.go`
- Create: `internal/engine/reactions_test.go`
- Modify: `internal/engine/executor.go`

**Step 1: 先写失败测试（规则匹配与动作执行）**

```go
func TestReactions_FirstMatchWins(t *testing.T) {}
func TestReactions_CompileOnFailureSugar(t *testing.T) {}
func TestReactions_RetryConsumesGlobalBudget(t *testing.T) {}
```

**Step 2: 跑失败测试**

Run: `go test ./internal/engine -run Reaction -v`  
Expected: FAIL

**Step 3: 实现 Reactions V1**
- 编译 `on_failure` 为 reaction 规则（P1 先实现最小动作集）：
  - `retry`
  - `escalate_human`
  - `skip_stage`
  - `abort_pipeline`
- 执行顺序：`stage_failed event -> reaction match -> action`
- 第一条匹配后停止。

**Step 4: 回归测试**

Run: `go test ./internal/engine -v`  
Expected: PASS

**Step 5: Commit**

```bash
git add internal/engine/reactions.go internal/engine/reactions_test.go internal/engine/executor.go
git commit -m "feat(engine): introduce reactions v1 and compile on_failure rules"
```

---

## Task 9: CLI P1 命令面补齐（多项目+调度控制）

**Files:**
- Modify: `cmd/ai-flow/main.go`
- Modify: `cmd/ai-flow/commands.go`
- Create: `cmd/ai-flow/commands_test.go`

**Step 1: 写失败测试（新增命令解析）**

```go
func TestCLI_PipelineActionCommand(t *testing.T) {}
func TestCLI_SchedulerCommand(t *testing.T) {}
func TestCLI_ProjectScanCommand(t *testing.T) {}
```

**Step 2: 运行失败测试**

Run: `go test ./cmd/ai-flow -run CLI -v`  
Expected: FAIL

**Step 3: 实现命令**
- `project scan <root>`（扫描 git repo 批量注册）
- `pipeline action <id> <approve|reject|...> [args]`
- `pipeline list [project-id]`
- `scheduler run`（前台运行调度循环）
- `scheduler once`（单次调度，便于调试）

**Step 4: 回归与构建**

Run:
- `go test ./cmd/ai-flow -v`
- `go build ./cmd/ai-flow`

Expected: PASS

**Step 5: Commit**

```bash
git add cmd/ai-flow/main.go cmd/ai-flow/commands.go cmd/ai-flow/commands_test.go
git commit -m "feat(cli): add p1 scheduler and pipeline action commands"
```

---

## Task 10: TUI P1 交互升级（多项目、动作、队列状态）

**Files:**
- Modify: `internal/tui/app.go`
- Modify: `internal/tui/views/pipeline_list.go`
- Create: `internal/tui/views/project_list.go`
- Create: `internal/tui/views/project_list_test.go`
- Modify: `internal/tui/app_test.go`

**Step 1: 写失败测试（多项目切换 + 动作触发）**

```go
func TestTUI_ProjectSwitchChangesPipelineContext(t *testing.T) {}
func TestTUI_ActionApproveCommand(t *testing.T) {}
```

**Step 2: 运行失败测试**

Run: `go test ./internal/tui/... -v`  
Expected: FAIL

**Step 3: 实现 UI**
- 顶部显示当前项目与调度状态（running/queued/waiting_human）。
- 支持项目切换。
- 在聊天模式下展示“自动匹配项目结果”。
- 增加动作快捷输入（如 `/pipeline action ...` 的提示与回显）。

**Step 4: 回归测试**

Run: `go test ./internal/tui/... -v`  
Expected: PASS

**Step 5: Commit**

```bash
git add internal/tui/app.go internal/tui/views/pipeline_list.go internal/tui/views/project_list.go internal/tui/views/project_list_test.go internal/tui/app_test.go
git commit -m "feat(tui): add p1 multi-project controls and action panel"
```

---

## Task 11: P1 集成与压力回归（必须）

**Files:**
- Create: `internal/engine/integration_p1_test.go`
- Modify: `docs/plans/2026-02-28-p1-implementation.md`（只在执行后追加“验收记录”）

**Step 1: 写集成测试**
- 三项目并发（至少 5 条 pipeline）：
  - 验证全局并发上限
  - 验证项目并发上限
  - 验证 worktree 排他
- 恢复测试：
  - 模拟中途崩溃，重启后恢复。

**Step 2: 执行验证**

Run:
- `go test ./internal/engine -run IntegrationP1 -v`
- `go test ./... -v`
- `go build ./cmd/ai-flow`

Expected: PASS

**Step 3: 手工 smoke**

Run:
```bash
./ai-flow project add demo-a /path/to/demo-a
./ai-flow project add demo-b /path/to/demo-b
./ai-flow pipeline create demo-a p1-a "需求A" quick
./ai-flow pipeline create demo-b p1-b "需求B" quick
./ai-flow scheduler run
./ai-flow tui
```

Expected: 可见队列推进、状态变更、人工动作可用。

**Step 4: Final Commit**

```bash
git add -A
git commit -m "feat: complete p1 multi-project scheduler and config-driven orchestration"
```

**验收记录（2026-03-01）**

- 第四批（Task 10-11）已先执行代码 review，再执行修复：
  - 修复 `internal/engine/integration_p1_test.go` 在 runner goroutine 调用 `t.Fatalf` 的并发测试风险，改为 error channel 回传并由主测试 goroutine 失败。
  - 增加并发观测断言（`maxGlobalObserved >= 2`）以避免“退化串行仍通过”的漏报。
  - 提取并复用恢复错误常量，恢复测试改为结构化断言，降低文案变更导致的脆弱性。
  - 修复 TUI `/clear` 命令别名不一致问题，并补单测。
  - 修复自动创建项目失败时错误被吞的问题，并补单测。
  - 补充 selected project 优先级单测，覆盖多项目路由关键分支。
- 执行验证结果：
  - `go test ./internal/engine -run IntegrationP1 -v` -> PASS
  - `go test ./... -v` -> PASS
  - `go build ./cmd/ai-flow` -> PASS

---

Plan complete and saved to `docs/plans/2026-02-28-p1-implementation.md`.


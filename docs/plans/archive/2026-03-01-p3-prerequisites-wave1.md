# P3 Prerequisites Wave 1 — Stage/Template Cleanup

> **For Agent:** REQUIRED SUB-SKILL: Use executing-wave-plans to implement this wave and gate it before Wave 2.

## Wave Goal

彻底移除 Pipeline 级 `spec_gen/spec_review` 执行语义，统一到“requirements -> worktree_setup -> implement -> code_review/fixup/e2e_test”的新阶段模型。

## Depends On

- `[]`

## Wave Entry Data

- 旧阶段仍存在于 `internal/core/stage.go`、`internal/engine/templates.go`、`executor/handlers/scheduler` switch 分支。
- `core.StageE2ETest` 已存在，但 `full` 模板尚未稳定使用。

## Tasks

### Task W1-T1: 清理 Stage 常量与模板定义

**Files:**
- Modify: `internal/core/stage.go`
- Modify: `internal/engine/templates.go`
- Modify: `internal/core/pipeline_test.go`
- Modify: `internal/engine/integration_p1_test.go`
- Test: `internal/engine/scheduler_test.go`

**Depends on:** `[]`

**Step 1: Write failing test**
```text
新增/调整测试：
- 验证 Templates["full"] 不含 spec_gen/spec_review，且含 e2e_test。
- 验证 stage 常量集合不再包含 StageSpecGen/StageSpecReview。
- 验证 full 模板顺序为 requirements -> worktree_setup -> implement。
```

**Step 2: Run to confirm failure**
Run: `go test ./internal/core ./internal/engine -run 'Template|Stage'`
Expected: 断言失败（旧阶段仍存在）。

**Step 3: Minimal implementation**
```text
删除 StageSpecGen/StageSpecReview 常量。
重写 full 模板：requirements -> worktree_setup -> implement -> code_review -> fixup -> e2e_test -> merge -> cleanup。
同步修正 requirements 运行前置约束：requirements 不再要求 worktree_path，worktree 相关依赖统一在 worktree_setup/implement 阶段消费。
同步修正受影响的模板测试和默认推断逻辑。
```

**Step 4: Run tests to confirm pass**
Run: `go test ./internal/core ./internal/engine -run 'Template|Stage'`
Expected: PASS。
Run: `go test ./internal/engine -run 'Integration|Full.*Order|Requirements.*Worktree'`
Expected: PASS（真实执行路径证明 requirements 可先于 worktree_setup 执行）。

**Step 5: Commit**
```bash
git add internal/core/stage.go internal/engine/templates.go internal/core/pipeline_test.go internal/engine/integration_p1_test.go internal/engine/scheduler_test.go
git commit -m "refactor(stage): remove spec stages and align templates"
```

### Task W1-T2: 清理执行分支 switch（executor/web/scheduler）

**Files:**
- Modify: `internal/engine/executor.go`
- Modify: `internal/web/handlers_pipeline.go`
- Modify: `internal/secretary/scheduler.go`
- Modify: `internal/engine/executor_behavior_test.go`
- Modify: `internal/web/handlers_pipeline_test.go`
- Modify: `internal/secretary/scheduler_test.go`

**Depends on:** `[W1-T1]`

**Step 1: Write failing test**
```text
新增/调整测试：
- requirements/code_review 路径 agent 默认值正确。
- e2e_test 默认 agent=codex、timeout=15m。
- 不存在 spec_gen/spec_review 分支调用。
```

**Step 2: Run to confirm failure**
Run: `go test ./internal/engine ./internal/web ./internal/secretary -run 'Stage|E2E|DefaultAgent'`
Expected: 仍命中旧分支或 e2e 配置未生效。

**Step 3: Minimal implementation**
```text
删除所有 StageSpecGen/StageSpecReview 分支。
补齐 e2e_test 在 executor/handler/scheduler 的默认行为配置。
```

**Step 4: Run tests to confirm pass**
Run: `go test ./internal/engine ./internal/web ./internal/secretary -run 'Stage|E2E|DefaultAgent'`
Expected: PASS。

**Step 5: Commit**
```bash
git add internal/engine/executor.go internal/web/handlers_pipeline.go internal/secretary/scheduler.go internal/engine/executor_behavior_test.go internal/web/handlers_pipeline_test.go internal/secretary/scheduler_test.go
git commit -m "refactor(stage): drop spec stage branches and wire e2e defaults"
```

### Task W1-T3: 清理 PromptVars 旧字段与模板引用

**Files:**
- Modify: `internal/engine/prompts.go`
- Modify: `internal/engine/executor.go`
- Modify: `internal/engine/prompt_templates/implement.tmpl`
- Modify: `internal/engine/prompt_templates/requirements.tmpl`
- Modify: `internal/engine/executor_test.go`

**Depends on:** `[W1-T2]`

**Step 1: Write failing test**
```text
新增测试：
- RenderPrompt 不再依赖 SpecPath/TasksMD。
- implement 模板在无 TasksMD 时仍能正确渲染。
```

**Step 2: Run to confirm failure**
Run: `go test ./internal/engine -run 'Prompt|Render'`
Expected: 模板变量缺失或断言失败。

**Step 3: Minimal implementation**
```text
从 PromptVars 强制删除 ChangeName/SpecPath/TasksMD 三个字段，并同步修复所有调用方与模板引用（不得保留“临时兼容字段”）。
executor 传参改为 Requirements + 结构化上下文（后续波次补齐）。
移除 implement.tmpl 的 TasksMD 条件块。
```

**Step 4: Run tests to confirm pass**
Run: `go test ./internal/engine -run 'Prompt|Render'`
Expected: PASS。

**Step 5: Commit**
```bash
git add internal/engine/prompts.go internal/engine/executor.go internal/engine/prompt_templates/implement.tmpl internal/engine/prompt_templates/requirements.tmpl internal/engine/executor_test.go
git commit -m "refactor(prompt): remove pipeline spec artifacts from prompt vars"
```

## Test Strategy Per Task

| Task | Unit | Integration |
|---|---|---|
| W1-T1 | 模板和 stage 常量断言 | 模板推断路径 smoke |
| W1-T2 | 默认 agent/timeout 行为 | handler->executor 路径联测 |
| W1-T3 | prompt 渲染契约测试 | stage 执行提示词 smoke |

## Risks and Mitigations

- 风险：移除旧常量引发编译雪崩。  
  缓解：先改常量再立即全局 `rg` + 修复引用。
- 风险：e2e 默认策略不一致。  
  缓解：三处 switch 统一通过同一 helper 决策。

## Wave E2E/Smoke Cases and Entry Data

### Entry Data
- 一个 `full` 模板 pipeline。
- 一次 code_review 返回 needs_fix 的场景。

### Smoke Cases
- `full` 模板会包含 `e2e_test` 且无 spec 阶段。
- `full` 执行时，`requirements` 先于 `worktree_setup` 且不依赖 `worktree_path`。
- `quick` 模板在 needs_fix 时仍能动态插入 fixup。

## Wave Exit Gate
- Execute mandatory gate sequence via `executing-wave-plans`.
- Wave-specific acceptance:
  - [ ] 代码中不再存在可执行 `StageSpecGen/StageSpecReview` 分支。
  - [ ] `full` 模板已使用 `e2e_test` 并可执行。
  - [ ] PromptVars 已无 Pipeline spec 产物字段。
- Wave-specific verification:
  - [ ] `go build ./...` 通过。
  - [ ] `go test ./internal/core ./internal/engine ./internal/web ./internal/secretary` 通过。
  - [ ] `go test ./internal/engine -run 'Integration|Full.*Order|Requirements.*Worktree'` 通过（非纯单测 smoke）。
- Boundary-change verification (if triggered):
  - [ ] `rg -n 'StageSpecGen|StageSpecReview|spec_gen|spec_review' internal cmd` 结果仅允许测试说明/注释语境。

## Next Wave Entry Condition
- Governed by `executing-wave-plans` verdict rule (`Go` / satisfied `Conditional Go` only).

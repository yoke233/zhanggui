# P4 Wave 1 — 模板体系底座

> **For Agent:** REQUIRED SUB-SKILL: Use executing-wave-plans to implement this wave and gate it before Wave 2.

## Wave Goal

把 `custom_templates` 从“文档能力”变成“运行时能力”：配置可合并、模板可校验、Pipeline 创建路径可统一解析。

## Depends On

- `[P4-ENTRY]`

## Wave Entry Data

- 当前模板定义仅在 `internal/engine/templates.go` 静态 map，未支持项目自定义。
- `internal/web/handlers_pipeline.go`、`internal/secretary/scheduler.go` 均直接读取 `engine.Templates`。
- 配置模型无 `custom_templates` 字段。
- P3 执行门禁结论与 GitHub 主链路冒烟证据已满足主计划 `P4-ENTRY` 要求。
- P3 Wave 2.5 硬化证据已可复用；跨团队依赖相关设计已从 P3 路线图中移除，不在 P4 复用范围内。

## Tasks

### Task W1-T1: 配置模型新增 `custom_templates` 并打通 merge

**Files:**
- Modify: `internal/config/types.go`
- Modify: `internal/config/defaults.go`
- Modify: `internal/config/merge.go`
- Modify: `internal/config/config_test.go`
- Modify: `internal/config/merge_hierarchy_test.go`
- Modify: `configs/defaults.yaml`
- Test: `internal/config/custom_templates_test.go`

**Depends on:** `[]`

**Conflict Scope (for executor scheduling):**
- Files: `[internal/config/types.go, internal/config/merge.go, configs/defaults.yaml]`
- Shared Critical File: `yes`

**Step 1: Write failing test**
```text
新增测试：
- TestLoadDefaults_CustomTemplates_DefaultEmptyMap
- TestMergeHierarchy_CustomTemplates_ProjectOverridesGlobal
- TestMergeHierarchy_CustomTemplates_PipelineOverrideWins
- TestCustomTemplateConfig_RejectsUnknownStageID
```

**Step 2: Run to confirm failure**
Run: `go test ./internal/config -run 'CustomTemplate|MergeHierarchy|Defaults'`
Expected: 字段缺失或 merge 断言失败。

**Step 3: Minimal implementation**
```text
在 PipelineConfig / PipelineLayer 增加 custom_templates 字段。
实现 global/project/pipeline 三层 merge 规则：下层同名模板覆盖上层。
增加模板结构校验：stage 不能为空、stage id 必须在受支持集合中。
```

**Step 4: Run tests to confirm pass**
Run: `go test ./internal/config -run 'CustomTemplate|MergeHierarchy|Defaults'`
Expected: PASS。

**Step 5: Commit**
```bash
git add internal/config/types.go internal/config/defaults.go internal/config/merge.go internal/config/config_test.go internal/config/merge_hierarchy_test.go configs/defaults.yaml internal/config/custom_templates_test.go
git commit -m "feat(config): add custom_templates schema and merge rules"
```

### Task W1-T2: 引入模板解析器并替换静态读取路径

**Files:**
- Create: `internal/engine/template_registry.go`
- Create: `internal/engine/template_registry_test.go`
- Modify: `internal/engine/templates.go`
- Modify: `internal/engine/executor.go`
- Modify: `internal/web/handlers_pipeline.go`
- Modify: `internal/secretary/scheduler.go`
- Test: `internal/engine/executor_test.go`
- Test: `internal/web/handlers_pipeline_test.go`
- Test: `internal/secretary/scheduler_test.go`

**Depends on:** `[W1-T1]`

**Conflict Scope (for executor scheduling):**
- Files: `[internal/engine/templates.go, internal/web/handlers_pipeline.go, internal/secretary/scheduler.go]`
- Shared Critical File: `yes`

**Step 1: Write failing test**
```text
新增测试：
- TestTemplateRegistry_ResolveBuiltInTemplate
- TestTemplateRegistry_ResolveCustomTemplate
- TestTemplateRegistry_RejectsInvalidStageSequence
- TestCreatePipeline_UsesResolvedCustomTemplateStages
```

**Step 2: Run to confirm failure**
Run: `go test ./internal/engine ./internal/web ./internal/secretary -run 'TemplateRegistry|CustomTemplate|CreatePipeline'`
Expected: 解析器不存在或创建路径仍使用静态模板。

**Step 3: Minimal implementation**
```text
新增 TemplateRegistry：输入内置模板 + custom_templates，输出最终 stage 序列。
替换 executor/web/scheduler 的模板读取方式，统一经 registry.Resolve(templateName)。
保持 built-in 模板在无自定义配置时行为不变。
```

**Step 4: Run tests to confirm pass**
Run: `go test ./internal/engine ./internal/web ./internal/secretary -run 'TemplateRegistry|CustomTemplate|CreatePipeline'`
Expected: PASS。

**Step 5: Commit**
```bash
git add internal/engine/template_registry.go internal/engine/template_registry_test.go internal/engine/templates.go internal/engine/executor.go internal/web/handlers_pipeline.go internal/secretary/scheduler.go internal/engine/executor_test.go internal/web/handlers_pipeline_test.go internal/secretary/scheduler_test.go
git commit -m "feat(engine): resolve templates via registry with custom support"
```

### Task W1-T3: 暴露模板目录能力（API/CLI）

**Files:**
- Create: `internal/web/handlers_template.go`
- Create: `internal/web/handlers_template_test.go`
- Modify: `internal/web/server.go`
- Modify: `cmd/ai-flow/main.go`
- Modify: `cmd/ai-flow/commands.go`
- Modify: `cmd/ai-flow/commands_test.go`

**Depends on:** `[W1-T2]`

**Conflict Scope (for executor scheduling):**
- Files: `[internal/web/server.go, cmd/ai-flow/commands.go, cmd/ai-flow/main.go]`
- Shared Critical File: `yes`

**Step 1: Write failing test**
```text
新增测试：
- TestListTemplates_IncludesBuiltinAndCustom
- TestListTemplates_InvalidConfig_Returns400
- TestCLI_TemplateList_PrintsResolvedTemplates
```

**Step 2: Run to confirm failure**
Run: `go test ./internal/web ./cmd/ai-flow -run 'TemplateList|ListTemplates'`
Expected: 路由/命令不存在。

**Step 3: Minimal implementation**
```text
新增 `GET /api/v1/templates` 返回 built-in + custom 模板与 stage 列表。
新增 `ai-flow template list` 命令读取有效配置并输出解析后的模板目录。
错误配置场景返回明确错误码与字段提示。
```

**Step 4: Run tests to confirm pass**
Run: `go test ./internal/web ./cmd/ai-flow -run 'TemplateList|ListTemplates'`
Expected: PASS。

**Step 5: Commit**
```bash
git add internal/web/handlers_template.go internal/web/handlers_template_test.go internal/web/server.go cmd/ai-flow/main.go cmd/ai-flow/commands.go cmd/ai-flow/commands_test.go
git commit -m "feat(api,cli): expose resolved template catalog"
```

## Test Strategy Per Task

| Task | Unit | Integration |
|---|---|---|
| W1-T1 | 配置结构与 merge 断言 | 全局+项目+pipeline 三层叠加回放 |
| W1-T2 | 模板解析与非法 stage 拦截 | web/scheduler 创建路径一致性 |
| W1-T3 | API/CLI 输出结构与错误码 | 实际配置加载 + HTTP 路由 smoke |

## Risks and Mitigations

- 风险：模板解析路径分叉导致行为不一致。  
  缓解：以 `TemplateRegistry` 作为唯一入口，所有调用方复用。
- 风险：用户配置 stage 拼写错误导致运行时才失败。  
  缓解：在加载/创建阶段提前校验并返回结构化错误。

## Wave E2E/Smoke Cases and Entry Data

### Entry Data
- 全局配置定义 `custom_templates.ui_only`。
- 项目配置定义同名模板覆盖 + 新增模板 `hotfix_safe`。

### Smoke Cases
- `POST /api/v1/projects/:id/pipelines` 使用自定义模板创建成功。
- `ai-flow template list` 可看到 built-in + custom 合并结果。
- scheduler 从 TaskItem.Template 读取自定义模板时可正常排程。

## Wave Exit Gate
- Execute mandatory gate sequence via `executing-wave-plans`.
- Wave-specific acceptance:
  - [ ] `custom_templates` 三层配置 merge 行为确定且可测试。
  - [ ] executor/web/scheduler 均经统一模板解析器。
- Wave-specific verification:
  - [ ] `go test ./internal/config ./internal/engine ./internal/web ./internal/secretary -run 'CustomTemplate|TemplateRegistry|TemplateList'` 通过。
  - [ ] `go test ./cmd/ai-flow -run 'TemplateList'` 通过。
- Boundary-change verification (if triggered):
  - [ ] 若改动模板默认顺序，执行 `go test ./internal/engine -run 'Template|Integration'` 并确认 PASS。

## Next Wave Entry Condition
- Governed by `executing-wave-plans` verdict rule (`Go` / satisfied `Conditional Go` only).

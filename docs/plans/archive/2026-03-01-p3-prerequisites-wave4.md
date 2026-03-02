# P3 Prerequisites Wave 4 — API/Docs/Regression Gate

> **For Agent:** REQUIRED SUB-SKILL: Use executing-wave-plans to implement this wave and produce P3 entry evidence.

## Wave Goal

完成外部接口、文档和回归门禁的最终对齐，产出“可进入 P3”的客观证据包。

## Depends On

- `[W3-T1, W3-T2, W3-T3]`

## Wave Entry Data

- 代码内部语义已迁移到新阶段与新契约。
- 仍需要确保 API/前端类型、文档、计划文件与代码一致。

## Tasks

### Task W4-T1: API/Handler/前端类型对齐新契约

**Files:**
- Modify: `internal/web/handlers_pipeline.go`
- Modify: `internal/web/handlers_pipeline_test.go`
- Modify: `internal/web/handlers_plan.go`
- Modify: `internal/web/handlers_plan_test.go`
- Modify: `web/src/types/api.ts`
- Modify: `web/src/lib/apiClient.ts`
- Modify: `web/src/lib/apiClient.test.ts`

**Depends on:** `[W2-T2, W3-T2, W3-T3]`

**Step 1: Write failing test**
```text
新增测试：
- TestGetPipeline_IncludesTaskItemID
- TestPlanTaskPayload_IncludesInputsOutputsAcceptance
- 前端 apiClient 对 task_item_id 和结构化字段序列化测试
```

**Step 2: Run to confirm failure**
Run: `go test ./internal/web -run 'Pipeline|Plan|TaskItemID|Acceptance'`
Expected: 响应结构断言失败。
Run: `npm --prefix web test -- --run apiClient`
Expected: 类型或字段映射失败。

**Step 3: Minimal implementation**
```text
统一 API 输出模型：pipeline 带 task_item_id，task 带 inputs/outputs/acceptance/constraints。
同步前端类型与客户端映射。
```

**Step 4: Run tests to confirm pass**
Run: `go test ./internal/web -run 'Pipeline|Plan|TaskItemID|Acceptance'`
Expected: PASS。
Run: `npm --prefix web test -- --run apiClient`
Expected: PASS。

**Step 5: Commit**
```bash
git add internal/web/handlers_pipeline.go internal/web/handlers_pipeline_test.go internal/web/handlers_plan.go internal/web/handlers_plan_test.go web/src/types/api.ts web/src/lib/apiClient.ts web/src/lib/apiClient.test.ts
git commit -m "feat(api): align pipeline and task payloads with plan-level contract"
```

### Task W4-T2: 文档同步（spec + plans）

**Files:**
- Modify: `docs/spec/spec-overview.md`
- Modify: `docs/spec/spec-pipeline-engine.md`
- Modify: `docs/spec/spec-secretary-layer.md`
- Modify: `docs/spec/spec-agent-drivers.md`
- Modify: `docs/spec/spec-api-config.md`
- Modify: `docs/plans/2026-03-01-p3-github-integration.md`

**Depends on:** `[W4-T1]`

**Step 1: Write failing test**
```text
文档一致性检查脚本（可作为测试）：
- docs/spec 内部不得出现可执行语义的 spec_gen/spec_review/StageSpec*。
- docs/plans 采用 allowlist：仅前置迁移计划文件允许出现历史术语，其他计划文件禁止。
- docs/spec 与 docs/plans 的 stage 命名一致。
```

**Step 2: Run to confirm failure**
Run: `rg -n 'spec_gen|spec_review|StageSpec' docs/spec`
Expected: 有命中即失败（docs/spec 需 0 命中）。
Run: `rg -n 'spec_gen|spec_review|StageSpec' docs/plans -g '!2026-03-01-p3-prerequisites-*.md'`
Expected: 有命中即失败（非 allowlist 计划文件需 0 命中）。

**Step 3: Minimal implementation**
```text
按当前代码语义修正文档，保留“历史说明”仅用于迁移背景，不作为现行行为描述。
```

**Step 4: Run tests to confirm pass**
Run: `rg -n 'spec_gen|spec_review|StageSpec' docs/spec`
Expected: 无输出（0 命中）。
Run: `rg -n 'spec_gen|spec_review|StageSpec' docs/plans -g '!2026-03-01-p3-prerequisites-*.md'`
Expected: 无输出（0 命中）。

**Step 5: Commit**
```bash
git add docs/spec/spec-overview.md docs/spec/spec-pipeline-engine.md docs/spec/spec-secretary-layer.md docs/spec/spec-agent-drivers.md docs/spec/spec-api-config.md docs/plans/2026-03-01-p3-github-integration.md
git commit -m "docs: sync spec and plans with plan-level spec architecture"
```

### Task W4-T3: P3 入场回归 Gate 与证据固化

**Files:**
- Create: `docs/plans/2026-03-01-p3-prerequisites-entry-checklist.md`
- Modify: `docs/plans/2026-03-01-p3-prerequisites-implementation.md`
- Modify: `docs/plans/2026-03-01-p3-github-integration.md`

**Depends on:** `[W4-T1, W4-T2]`

**Step 1: Write failing test**
```text
定义 checklist 断言项：
- build/test/race/search 四项必须有结果记录。
- 若任一失败则状态标记为 Not Ready。
```

**Step 2: Run to confirm failure**
Run: `go build ./...; if ($LASTEXITCODE -ne 0) { exit 1 }`
Expected: 若仍有残留问题，此处失败并阻断。

**Step 3: Minimal implementation**
```text
生成 entry checklist，记录命令、日期、结果、负责人。
将 P3 主计划中的“开始条件”改为引用该 checklist。
```

**Step 4: Run tests to confirm pass**
Run: `go test ./...`
Expected: PASS。
Run: `go test -race ./internal/engine ./internal/secretary ./internal/plugins/store-sqlite ./internal/web`
Expected: PASS。
Run: `rg -n 'StageSpecGen|StageSpecReview|spec_gen|spec_review' internal cmd configs docs/spec`
Expected: 无输出（0 命中）。
Run: `rg -n 'StageSpecGen|StageSpecReview|spec_gen|spec_review' docs/plans -g '!2026-03-01-p3-prerequisites-*.md'`
Expected: 无输出（0 命中）。

**Step 5: Commit**
```bash
git add docs/plans/2026-03-01-p3-prerequisites-entry-checklist.md docs/plans/2026-03-01-p3-prerequisites-implementation.md docs/plans/2026-03-01-p3-github-integration.md
git commit -m "chore: add p3 entry checklist and prerequisite gate evidence"
```

## Test Strategy Per Task

| Task | Unit | Integration |
|---|---|---|
| W4-T1 | handlers + apiClient | web/build + API 回归 |
| W4-T2 | 文档规则 grep | spec/plans 语义对照审阅 |
| W4-T3 | checklist 规则 | build/test/race/search 四门禁 |

## Risks and Mitigations

- 风险：文档与代码再次漂移。  
  缓解：将 grep 门禁写入 entry checklist，作为 P3 启动前必跑步骤。
- 风险：大改后回归时间长。  
  缓解：先跑核心包，最后跑全量；失败按模块并行修复。

## Wave E2E/Smoke Cases and Entry Data

### Entry Data
- 一条 `full` 模板 pipeline 样例。
- 一份包含结构化 TaskItem 的 plan fixture。

### Smoke Cases
- 通过 API 读取 pipeline 时可拿到 `task_item_id` 并关联到 TaskItem。
- Web 前端显示结构化任务字段不报错。
- checklist 记录四门禁命令结果后状态为 Ready。

## Wave Exit Gate
- Execute mandatory gate sequence via `executing-wave-plans`.
- Wave-specific acceptance:
  - [ ] API/前端类型与新契约完全对齐。
  - [ ] docs/spec 与 docs/plans 与代码语义一致。
  - [ ] 已生成 P3 入场 checklist 并附完整命令证据。
- Wave-specific verification:
  - [ ] `go build ./...` 通过。
  - [ ] `go test ./...` 通过。
  - [ ] `npm --prefix web test -- --run` 与 `npm --prefix web run build` 通过（若本波触达前端）。
- Boundary-change verification (if triggered):
  - [ ] `go test -race ./internal/engine ./internal/secretary ./internal/plugins/store-sqlite ./internal/web` 通过。

## Next Wave Entry Condition
- Wave 4 为前置收口终波；达到 Exit Gate 后方可进入 P3 主计划 Wave 1。

# P3 Prerequisites Wave 2 — Contract/Store Upgrade

> **For Agent:** REQUIRED SUB-SKILL: Use executing-wave-plans to implement this wave and gate it before Wave 3.

## Wave Goal

把 Plan 级契约从“弱文本”升级为“结构化字段”，并完成 SQLite + Store 全链路持久化，确保 DAG/requirements 可直接消费。

## Depends On

- `[W1-T1, W1-T2, W1-T3]`

## Wave Entry Data

- `core.TaskPlan/TaskItem` 仍是精简字段，尚未承载结构化契约。
- `store-sqlite` 的 `task_plans/task_items/pipelines` 表结构和 SQL 尚未同步新字段。
- 采用 Destructive Cutover：允许清理无法满足新契约的历史 TaskPlan/TaskItem 数据。

## Tasks

### Task W2-T0: 破坏性切换窗口（旧数据清理/冻结）

**Files:**
- Modify: `internal/plugins/store-sqlite/migrations.go`
- Create: `internal/plugins/store-sqlite/cutover_test.go`
- Modify: `docs/plans/2026-03-01-p3-prerequisites-wave2.md`

**Depends on:** `[]`

**Step 1: Write failing test**
```text
新增测试：
- TestCutover_PurgeLegacyTaskPlanRows_WhenMissingStructuredContract
- TestCutover_PreserveDonePipelines_AndResetDanglingTaskRelations
```

**Step 2: Run to confirm failure**
Run: `go test ./internal/plugins/store-sqlite -run 'Cutover|PurgeLegacy'`
Expected: 当前无切换策略实现，测试失败。

**Step 3: Minimal implementation**
```text
实现一次性切换策略：
- 删除/归档不满足新结构化契约的 task_plans/task_items（如 acceptance 为空且状态非 done）。
- 清理旧阶段残留 artifacts 与无效 pipeline->task 关系。
- 在 entry checklist 中新增“已执行切换窗口”标记（`migration_flags.wave2_destructive_cutover_done=1`）。
```

**Step 4: Run tests to confirm pass**
Run: `go test ./internal/plugins/store-sqlite -run 'Cutover|PurgeLegacy'`
Expected: PASS。

**Step 5: Commit**
```bash
git add internal/plugins/store-sqlite/migrations.go internal/plugins/store-sqlite/cutover_test.go docs/plans/2026-03-01-p3-prerequisites-wave2.md
git commit -m "chore(cutover): apply destructive data cleanup before contract migration"
```

### Task W2-T1: 扩展 core.TaskPlan / TaskItem 契约字段

**Files:**
- Modify: `internal/core/taskplan.go`
- Modify: `internal/core/taskplan_test.go`
- Modify: `internal/core/store.go`
- Modify: `internal/core/pipeline.go`

**Depends on:** `[W2-T0]`

**Step 1: Write failing test**
```text
新增测试：
- TestTaskItemValidate_RequiresAcceptanceWhenStructuredEnabled
- TestNewTaskItemID_StableWithPlanPrefix
- TestTaskPlanJSON_RoundTrip_WithContractFields
```

**Step 2: Run to confirm failure**
Run: `go test ./internal/core -run 'TaskPlan|TaskItem|Contract'`
Expected: 新字段缺失或校验逻辑不满足。

**Step 3: Minimal implementation**
```text
TaskPlan 新增：SpecProfile/ContractVersion/ContractChecksum。
TaskItem 新增：Inputs/Outputs/Acceptance/Constraints。
Pipeline 新增：TaskItemID（仅关联，不复制契约字段）。
```

**Step 4: Run tests to confirm pass**
Run: `go test ./internal/core -run 'TaskPlan|TaskItem|Contract'`
Expected: PASS。

**Step 5: Commit**
```bash
git add internal/core/taskplan.go internal/core/taskplan_test.go internal/core/store.go internal/core/pipeline.go
git commit -m "feat(contract): expand task plan and task item structured fields"
```

### Task W2-T2: SQLite migration + Store SQL 全链路升级

**Files:**
- Modify: `internal/plugins/store-sqlite/migrations.go`
- Modify: `internal/plugins/store-sqlite/store.go`
- Create: `internal/plugins/store-sqlite/migrations_test.go`
- Modify: `internal/plugins/store-sqlite/secretary_store_test.go`
- Modify: `internal/plugins/store-sqlite/scheduler_store_test.go`
- Modify: `internal/plugins/store-sqlite/store_test.go`

**Depends on:** `[W2-T1]`

**Step 1: Write failing test**
```text
新增测试：
- TestMigration_AddsTaskContractColumns_BackwardCompatible
- TestMigration_BackfillPipelineTaskItemID_FromLegacyTaskItems
- TestTaskPlanRoundTrip_PersistsContractMeta
- TestTaskItemRoundTrip_PersistsInputsOutputsAcceptanceConstraints
- TestPipelineRoundTrip_PersistsTaskItemID
```

**Step 2: Run to confirm failure**
Run: `go test ./internal/plugins/store-sqlite -run 'Migration|RoundTrip|TaskItemID|Contract'`
Expected: 列缺失或 scan/insert 字段不匹配导致失败。

**Step 3: Minimal implementation**
```text
migrations 新增列：
- pipelines.task_item_id
- task_plans.spec_profile/contract_version/contract_checksum
- task_items.inputs/outputs/acceptance/constraints
并补 ALTER TABLE 兼容迁移。

新增历史关系回填策略（迁移阶段执行）：
- 从旧关系 `task_items.pipeline_id -> pipelines.id` 回填 `pipelines.task_item_id`。
- 回填 SQL 原则：每个 pipeline 选 created_at 最早的 task_item 作为 canonical 关联；若出现多条冲突，记录 warning 并保持确定性选择。
- 回填条件：仅对 `pipelines.task_item_id IS NULL` 且存在旧关联记录的行执行。

store.go 统一升级 task_plan/task_item/pipeline 相关 INSERT/SELECT/UPDATE/UPSERT 与 JSON 序列化反序列化。
```

**Step 4: Run tests to confirm pass**
Run: `go test ./internal/plugins/store-sqlite -run 'Migration|RoundTrip|TaskItemID|Contract'`
Expected: PASS。

**Step 5: Commit**
```bash
git add internal/plugins/store-sqlite/migrations.go internal/plugins/store-sqlite/store.go internal/plugins/store-sqlite/secretary_store_test.go internal/plugins/store-sqlite/scheduler_store_test.go internal/plugins/store-sqlite/store_test.go
git commit -m "feat(store): persist structured plan contract fields"
```

### Task W2-T3: Secretary 输出解析与映射升级

**Files:**
- Modify: `internal/secretary/agent.go`
- Modify: `internal/secretary/agent_test.go`
- Modify: `internal/secretary/review.go`
- Modify: `internal/secretary/review_test.go`

**Depends on:** `[W2-T1]`

**Step 1: Write failing test**
```text
新增测试：
- TestParseTaskPlan_IncludesInputsOutputsAcceptance
- TestToTaskItem_MapsStructuredFields
- TestReviewAgent_RejectsMissingAcceptance
```

**Step 2: Run to confirm failure**
Run: `go test ./internal/secretary -run 'ParseTaskPlan|Structured|Acceptance|Review'`
Expected: 解析字段丢失或 review 规则未覆盖。

**Step 3: Minimal implementation**
```text
更新 taskItemOutput DTO 与 toTaskItem 映射。
review 提示词与规则聚焦结构化字段（inputs/outputs/acceptance）可执行性。
过渡策略（与切换窗口配合）：
- 新生成/重生成计划：缺 acceptance 一律 reject。
- 切换窗口前历史数据：不进入新 review 流程；在 W2-T0 中清理或归档后再处理。
```

**Step 4: Run tests to confirm pass**
Run: `go test ./internal/secretary -run 'ParseTaskPlan|Structured|Acceptance|Review'`
Expected: PASS。

**Step 5: Commit**
```bash
git add internal/secretary/agent.go internal/secretary/agent_test.go internal/secretary/review.go internal/secretary/review_test.go
git commit -m "feat(secretary): parse and validate structured task contracts"
```

## Test Strategy Per Task

| Task | Unit | Integration |
|---|---|---|
| W2-T1 | 模型字段/校验 | JSON 契约 round-trip |
| W2-T2 | 迁移与 SQL 映射 | 老库迁移 + 新数据读写 |
| W2-T3 | Secretary 解析/评审规则 | review + scheduler 流程 smoke |

## Risks and Mitigations

- 风险：SQL 字段升级遗漏导致运行时 scan 崩溃。  
  缓解：列出所有 CRUD 函数清单逐项对照改。
- 风险：旧数据库迁移失败。  
  缓解：migration 测试覆盖“已有库 + 新列增补”。
- 风险：历史数据存在“一条 pipeline 对多条 task_item”的脏数据。  
  缓解：迁移回填使用确定性选择 + warning 日志 + 回归测试覆盖冲突场景。
- 风险：严格 acceptance 校验导致历史数据批量失败。  
  缓解：先执行 W2-T0 切换窗口，历史数据不走新校验链路。

## Wave E2E/Smoke Cases and Entry Data

### Entry Data
- 旧 schema sqlite 文件（不含新增列）。
- 一份包含结构化字段的 TaskPlan JSON fixture。
- 一份“缺 acceptance 的历史 TaskPlan”fixture（用于验证切换窗口清理）。

### Smoke Cases
- 先执行 W2-T0 后，历史脏数据已清理，不再触发新校验失败。
- 旧库启动自动迁移成功。
- 旧库已有 `task_items.pipeline_id` 时，迁移后 `pipelines.task_item_id` 可查询且稳定。
- 结构化 TaskPlan 创建后，DAG 可读取 TaskItem.Acceptance 不为空。

## Wave Exit Gate
- Execute mandatory gate sequence via `executing-wave-plans`.
- Wave-specific acceptance:
  - [ ] 已完成 Destructive Cutover（W2-T0），历史脏数据不再进入新契约校验链路。
  - [ ] 核心模型、存储、Secretary 解析全部支持结构化契约字段。
  - [ ] pipelines 与 task_items 通过 `task_item_id` 建立持久化关联。
  - [ ] 历史数据已完成 `pipeline_id -> task_item_id` 回填，不丢失既有关联语义。
  - [ ] 旧库迁移路径稳定可回归。
- Wave-specific verification:
  - [ ] `go test ./internal/core ./internal/plugins/store-sqlite ./internal/secretary` 通过。
  - [ ] `go test ./internal/plugins/store-sqlite -run 'Migration'` 通过。
- Boundary-change verification (if triggered):
  - [ ] `go test ./internal/engine ./internal/web -run 'Pipeline|Task|Plan'` 通过，确保 API 层未被 schema 变化破坏。

## Next Wave Entry Condition
- Governed by `executing-wave-plans` verdict rule (`Go` / satisfied `Conditional Go` only).

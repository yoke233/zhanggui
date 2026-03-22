# Resource Model Cleanup Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Consolidate three overlapping resource model layers (legacy ResourceBinding/ActionResource, new ResourceSpace/Resource/ActionIODecl, Thread-private) into a clean two-layer architecture.

**Architecture:** The legacy layer (ResourceBinding + ActionResource) is fully superseded by the new layer (ResourceSpace + Resource + ActionIODecl) — migration code already exists and runs on startup. The Thread-private layer (ThreadContextRef + ThreadAttachment) serves a different purpose (access grants + conversation attachments) and is kept separate but gets a naming cleanup. The field `WorkItem.ResourceBindingID` already points to a ResourceSpace ID at runtime and needs renaming.

**Tech Stack:** Go (domain model + SQLite store + HTTP handlers), TypeScript/React (frontend types), GORM (ORM)

**Commit Strategy:** Three green commits — each leaves the build passing:
1. Commit A (Task 1): All Go-side legacy removal (types + stores + tests) — atomic
2. Commit B (Tasks 2+3): Rename ResourceBindingID → ResourceSpaceID across all layers
3. Commit C (Task 4): Thread naming cleanup

---

## File Map

### Files to DELETE (Task 1)
| File | Reason |
|------|--------|
| `internal/core/resource.go` | Legacy types: ResourceBinding, ActionResource, ResourceBindingStore, ActionResourceStore, ResourceProvider |
| `internal/adapters/store/sqlite/resource_binding.go` | Legacy ResourceBindingStore implementation |
| `internal/adapters/store/sqlite/action_resource.go` | Legacy ActionResourceStore implementation |
| `internal/adapters/store/sqlite/unified_resource_migration.go` | One-time migration, already served its purpose |
| `internal/adapters/store/sqlite/unified_resource_migration_test.go` | Tests for deleted migration |

### Files to MODIFY — Legacy removal (Task 1)
| File | Change |
|------|--------|
| `internal/adapters/store/sqlite/models.go` | Remove ResourceBindingModel (lines 59-71), ActionResourceModel (lines 1286-1339), and their conversion functions |
| `internal/adapters/store/sqlite/schema.go` | Remove `&ResourceBindingModel{}` (line 16), `&ActionResourceModel{}` (line 39) from AutoMigrate; remove `migrateUnifiedResources` call (lines 71-73) |
| `internal/adapters/store/sqlite/workitem_cleanup.go` | Remove `DeleteResourceBindingsByWorkItem` (lines 106-113), `DeleteActionResourcesByWorkItem` (lines 23-34) |
| `internal/core/helpers_test.go` | Remove `TestResourceBindingAttachmentHelpers` |
| `internal/adapters/store/sqlite/store_test.go` | Remove `TestResourceBindingCRUD` |
| `internal/adapters/store/sqlite/json_models_test.go` | Remove ResourceBinding/ActionResource test fixtures |
| `internal/adapters/executor/builtin_helpers_test.go` | Remove legacy store mock methods from noopStore |
| `internal/adapters/http/integration_test.go` | Remove `TestIntegration_ResourceBindingCRUD` |
| `internal/application/workitemapp/service_test.go` | Rewrite `createWorkItemFixture` to use ResourceSpace/Resource/ActionIODecl instead of legacy types |

### Files to MODIFY — Rename (Tasks 2+3)
| File | Change |
|------|--------|
| `internal/core/workitem.go:42` | Field rename `ResourceBindingID` → `ResourceSpaceID` + JSON tag |
| `internal/adapters/store/sqlite/models.go:76,931,952` | GORM model + conversions |
| `internal/adapters/store/sqlite/schema.go` | Add column-existence-checked data migration |
| `internal/adapters/store/sqlite/workitem.go:125` | Store usage |
| `internal/application/workitemapp/contracts.go:84,96` | Input struct fields |
| `internal/application/workitemapp/service.go:54,68,117-121` | Usage sites |
| `internal/application/workitemapp/errors.go:7,13` | Error code constants |
| `internal/application/workitemapp/service_test.go` | Test references |
| `internal/application/flow/engine.go:179-188` | Workspace resolution |
| `internal/application/cron/trigger.go:295` | Cron clone |
| `internal/adapters/http/workitem.go:14,25,43,140` | HTTP request/mapping |
| `internal/adapters/http/workitem_app.go:75-76,81-82` | HTTP error code strings + messages |
| `internal/adapters/http/handler_test.go:522` | Test JSON key |
| `internal/adapters/http/flow_pr_bootstrap.go:302-304` | PR bootstrap |
| `internal/adapters/http/flow_pr_bootstrap_test.go:207` | Test |
| `internal/adapters/workspace/provider/composite.go:12` | Comment |
| `web/src/types/apiV2.ts:18,33` | Frontend types |

### Files to MODIFY — Thread naming (Task 4)
| File | Change |
|------|--------|
| `internal/core/thread.go:199-205,212` | Rename `ThreadWorkspaceAttachmentRef` → `AttachmentRef` |
| `internal/threadctx/workspace.go:205` | Usage site |

---

## Task 1: Remove all legacy ResourceBinding/ActionResource code (atomic)

This is one atomic commit covering deletion of types, stores, schema entries, and tests.

**Files to delete:**
- `internal/core/resource.go`
- `internal/adapters/store/sqlite/resource_binding.go`
- `internal/adapters/store/sqlite/action_resource.go`
- `internal/adapters/store/sqlite/unified_resource_migration.go`
- `internal/adapters/store/sqlite/unified_resource_migration_test.go`

**Files to modify:**
- `internal/adapters/store/sqlite/models.go`
- `internal/adapters/store/sqlite/schema.go`
- `internal/adapters/store/sqlite/workitem_cleanup.go`
- `internal/core/helpers_test.go`
- `internal/adapters/store/sqlite/store_test.go`
- `internal/adapters/store/sqlite/json_models_test.go`
- `internal/adapters/executor/builtin_helpers_test.go`
- `internal/adapters/http/integration_test.go`
- `internal/application/workitemapp/service_test.go`

- [ ] **Step 1: Delete the five legacy files**

```bash
rm internal/core/resource.go
rm internal/adapters/store/sqlite/resource_binding.go
rm internal/adapters/store/sqlite/action_resource.go
rm internal/adapters/store/sqlite/unified_resource_migration.go
rm internal/adapters/store/sqlite/unified_resource_migration_test.go
```

Note: `ActionResourceDirection`, `ResourceInput`, `ResourceOutput` constants and `ResourceKind*` constants are already defined in `internal/core/unified_resource.go` — no duplication loss.

- [ ] **Step 2: Remove ResourceBindingModel from models.go (lines 59-71)**

Delete from `type ResourceBindingModel struct {` through `func (ResourceBindingModel) TableName() string { return "resource_bindings" }`.

- [ ] **Step 3: Remove ActionResourceModel + conversion functions from models.go (lines 1286-1339)**

Delete from the `// ActionResource` section header through the closing brace of `toCore()`.

- [ ] **Step 4: Remove legacy models from AutoMigrate in schema.go**

Remove `&ResourceBindingModel{},` (line 16) and `&ActionResourceModel{},` (line 39) from the AutoMigrate call.

- [ ] **Step 5: Remove migrateUnifiedResources call from schema.go (lines 71-73)**

Delete:
```go
if err := migrateUnifiedResources(ctx, orm); err != nil {
    return fmt.Errorf("migrate unified resources: %w", err)
}
```

- [ ] **Step 6: Remove legacy deletion functions from workitem_cleanup.go**

Remove `DeleteActionResourcesByWorkItem` (lines 23-34) and `DeleteResourceBindingsByWorkItem` (lines 106-113).

- [ ] **Step 7: Remove TestResourceBindingAttachmentHelpers from helpers_test.go**

Delete the entire test function.

- [ ] **Step 8: Remove TestResourceBindingCRUD from store_test.go**

Delete the entire test function.

- [ ] **Step 9: Remove legacy fixtures from json_models_test.go**

Remove all `ResourceBindingModel` / `ActionResourceModel` / `core.ResourceBinding` / `core.ActionResource` references and any test cases that depend on them.

- [ ] **Step 10: Remove legacy mock methods from builtin_helpers_test.go**

Remove all `ResourceBinding*` and `ActionResource*` methods from the noopStore struct (lines 145-173 area).

- [ ] **Step 11: Remove TestIntegration_ResourceBindingCRUD from integration_test.go**

Delete the entire test function.

- [ ] **Step 12: Rewrite service_test.go fixture to use new resource types**

The `createWorkItemFixture` function (lines 154-172) currently calls `store.CreateResourceBinding()` and `store.CreateActionResource()`. Rewrite to use `store.CreateResourceSpace()` / `store.CreateResource()` / `store.CreateActionIODecl()`. Ensure the cascade deletion test still exercises `DeleteResourcesByWorkItem` and `DeleteActionIODeclsByWorkItem`.

- [ ] **Step 13: Build and test**

Run: `go build ./... && go test ./internal/...`
Expected: Clean build, all tests pass.

- [ ] **Step 14: Commit**

```bash
git add -A
git commit -m "refactor: remove legacy ResourceBinding/ActionResource layer

Delete ResourceBinding, ActionResource, ResourceBindingStore, ActionResourceStore,
ResourceProvider types and all implementations. The unified ResourceSpace/Resource/
ActionIODecl layer is now the sole resource model. Migration code removed as it has
already served its purpose."
```

---

## Task 2: Rename WorkItem.ResourceBindingID → ResourceSpaceID (core + app + store)

**Files:**
- Modify: `internal/core/workitem.go:42`
- Modify: `internal/application/workitemapp/contracts.go:84,96`
- Modify: `internal/application/workitemapp/service.go:54,68,117-121`
- Modify: `internal/application/workitemapp/errors.go:7,13`
- Modify: `internal/application/flow/engine.go:179-188`
- Modify: `internal/application/cron/trigger.go:295`
- Modify: `internal/adapters/store/sqlite/models.go:76,931,952`
- Modify: `internal/adapters/store/sqlite/schema.go` (add data migration)
- Modify: `internal/adapters/store/sqlite/workitem.go:125`
- Modify: `internal/adapters/http/workitem.go:14,25,43,140`
- Modify: `internal/adapters/http/workitem_app.go:75-76,81-82`
- Modify: `internal/adapters/http/handler_test.go:522`
- Modify: `internal/adapters/http/flow_pr_bootstrap.go:302-304`
- Modify: `internal/adapters/http/flow_pr_bootstrap_test.go:207`
- Modify: `internal/adapters/workspace/provider/composite.go:12`
- Modify: `internal/application/workitemapp/service_test.go`

- [ ] **Step 1: Rename field in core/workitem.go**

```go
// Before:
ResourceBindingID *int64 `json:"resource_binding_id,omitempty"` // which repo/resource to work on
// After:
ResourceSpaceID *int64 `json:"resource_space_id,omitempty"` // which resource space to work on
```

- [ ] **Step 2: Rename in workitemapp/contracts.go**

`CreateWorkItemInput.ResourceBindingID` → `ResourceSpaceID` (line 84).
`UpdateWorkItemInput.ResourceBindingID` → `ResourceSpaceID` (line 96).

- [ ] **Step 3: Rename in workitemapp/service.go**

Replace all `input.ResourceBindingID` → `input.ResourceSpaceID`, `workItem.ResourceBindingID` → `workItem.ResourceSpaceID`.

- [ ] **Step 4: Rename error codes in workitemapp/errors.go**

```go
// Before:
CodeInvalidResourceBinding  = "INVALID_RESOURCE_BINDING"
CodeResourceBindingNotFound = "RESOURCE_BINDING_NOT_FOUND"
// After:
CodeInvalidResourceSpace  = "INVALID_RESOURCE_SPACE"
CodeResourceSpaceNotFound = "RESOURCE_SPACE_NOT_FOUND"
```

- [ ] **Step 5: Rename in flow/engine.go (lines 179, 182, 188)**

`workItem.ResourceBindingID` → `workItem.ResourceSpaceID`

- [ ] **Step 6: Rename in cron/trigger.go (line 295)**

`source.ResourceBindingID` → `source.ResourceSpaceID`

- [ ] **Step 7: Rename in models.go — WorkItemModel field + conversions**

Line 76: GORM column tag `"column:resource_binding_id"` → `"column:resource_space_id"`, field name `ResourceBindingID` → `ResourceSpaceID`.
Lines 931, 952: conversion functions.

- [ ] **Step 8: Add column-existence-checked data migration to schema.go**

Add after AutoMigrate, before index creation:
```go
// Migrate legacy column data (idempotent, safe for fresh DBs).
var colCount int64
orm.WithContext(ctx).Raw(
    `SELECT COUNT(*) FROM pragma_table_info('work_items') WHERE name = 'resource_binding_id'`,
).Scan(&colCount)
if colCount > 0 {
    orm.WithContext(ctx).Exec(
        `UPDATE work_items SET resource_space_id = resource_binding_id WHERE resource_space_id IS NULL AND resource_binding_id IS NOT NULL`,
    )
}
```

- [ ] **Step 9: Update workitem.go store usage (line 125 area)**

- [ ] **Step 10: Rename HTTP request structs in workitem.go**

Lines 14, 25: field `ResourceBindingID` → `ResourceSpaceID`, JSON tag `"resource_binding_id"` → `"resource_space_id"`.
Lines 43, 140: mapping to app input.

- [ ] **Step 11: Update HTTP error strings in workitem_app.go**

```go
// Before:
case workitemapp.CodeResourceBindingNotFound:
    writeError(w, http.StatusNotFound, "resource binding not found", "RESOURCE_BINDING_NOT_FOUND")
...
case workitemapp.CodeInvalidResourceBinding:
    writeError(w, http.StatusBadRequest, err.Error(), "INVALID_RESOURCE_BINDING")
// After:
case workitemapp.CodeResourceSpaceNotFound:
    writeError(w, http.StatusNotFound, "resource space not found", "RESOURCE_SPACE_NOT_FOUND")
...
case workitemapp.CodeInvalidResourceSpace:
    writeError(w, http.StatusBadRequest, err.Error(), "INVALID_RESOURCE_SPACE")
```

- [ ] **Step 12: Update handler_test.go (line 522)**

`"resource_binding_id"` → `"resource_space_id"`

- [ ] **Step 13: Update flow_pr_bootstrap.go (lines 302-304)**

- [ ] **Step 14: Update flow_pr_bootstrap_test.go (line 207)**

- [ ] **Step 15: Update composite.go comment (line 12)**

- [ ] **Step 16: Update service_test.go references**

- [ ] **Step 17: Run backend tests**

Run: `go build ./... && go test ./internal/...`
Expected: All pass.

- [ ] **Step 18: Commit (do not push yet — frontend update next)**

---

## Task 3: Rename ResourceBindingID → ResourceSpaceID (frontend)

**Files:**
- Modify: `web/src/types/apiV2.ts:18,33`

- [ ] **Step 1: Update frontend types**

`resource_binding_id` → `resource_space_id` on lines 18 and 33.

- [ ] **Step 2: Search for any other frontend references**

Run: `grep -r "resource_binding" web/src/ --include="*.ts" --include="*.tsx"`
Expected: Zero matches.

- [ ] **Step 3: Run frontend typecheck**

Run: `npm --prefix web run typecheck`
Expected: Pass.

- [ ] **Step 4: Commit Tasks 2+3 together**

```bash
git add -A
git commit -m "refactor: rename resource_binding_id → resource_space_id

WorkItem.ResourceBindingID was already pointing to a ResourceSpace ID at runtime.
This rename aligns the field name with the actual semantics. Includes SQLite data
migration (column-existence-checked, idempotent) and frontend type update."
```

---

## Task 4: Thread naming cleanup — ThreadWorkspaceAttachmentRef → AttachmentRef

**Files:**
- Modify: `internal/core/thread.go:199-205,212`
- Modify: `internal/threadctx/workspace.go:205`

- [ ] **Step 1: Rename the type in thread.go**

```go
// Before:
// ThreadWorkspaceAttachmentRef is the .context.json representation of an attachment.
type ThreadWorkspaceAttachmentRef struct {
// After:
// AttachmentRef is the .context.json representation of an attachment.
type AttachmentRef struct {
```

- [ ] **Step 2: Update ThreadWorkspaceContext field type (line 212)**

```go
// Before:
Attachments []ThreadWorkspaceAttachmentRef `json:"attachments,omitempty"`
// After:
Attachments []AttachmentRef `json:"attachments,omitempty"`
```

- [ ] **Step 3: Update usage in threadctx/workspace.go (line 205)**

`core.ThreadWorkspaceAttachmentRef{...}` → `core.AttachmentRef{...}`

- [ ] **Step 4: Run tests**

Run: `go test ./internal/core/... ./internal/threadctx/...`
Expected: Pass.

- [ ] **Step 5: Commit**

```bash
git add internal/core/thread.go internal/threadctx/workspace.go
git commit -m "refactor(core): rename ThreadWorkspaceAttachmentRef → AttachmentRef"
```

---

## Task 5: Final verification

- [ ] **Step 1: Full backend build + test**

Run: `go build ./... && go test ./...`
Expected: Clean.

- [ ] **Step 2: Frontend typecheck + build**

Run: `npm --prefix web run typecheck && npm --prefix web run build`
Expected: Both pass.

- [ ] **Step 3: Grep for any remaining legacy references**

Run: `grep -r "ResourceBinding" internal/ web/src/ --include="*.go" --include="*.ts" --include="*.tsx"`
Expected: Zero matches (except possibly `archive-src/` which is frozen).

Run: `grep -r "ActionResource[^D]" internal/ --include="*.go"`
Expected: Only `ActionResourceDirection` in `unified_resource.go`.

Run: `grep -r "resource_binding" internal/ web/src/ --include="*.go" --include="*.ts" --include="*.tsx"`
Expected: Zero matches.

- [ ] **Step 4: Final fixup commit if needed**

```bash
git add -A
git commit -m "chore: resource model cleanup — final fixups"
```

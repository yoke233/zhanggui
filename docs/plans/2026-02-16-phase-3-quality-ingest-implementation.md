# Phase 3 Quality Ingest Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 在 sqlite outbox 模式下实现可审计、可去重的质量事件接入，并自动写回结构化质量 comment 与责任路由建议。  
**Architecture:** 在 persistence 层新增 `quality_events` 表做幂等与审计真源；在 usecase 层新增 `IngestQualityEvent` 完成事件规范化、去重、回写；在 CLI 暴露 `outbox quality ingest/list` 命令。  
**Tech Stack:** Go, Cobra, GORM(SQLite), 现有 outbox 协议与结构化 comment 渲染。

---

### Task 1: Repository Contract 扩展

**Files:**
- Modify: `internal/ports/outbox_repository.go`

**Step 1: Write the failing test**

- 依赖后续 Task 2/3 测试触发编译失败（新增接口方法未实现）。

**Step 2: Run test to verify it fails**

Run: `go test ./internal/usecase/outbox ./internal/infrastructure/persistence/sqlite/repository`  
Expected: 编译失败，提示 `OutboxRepository` 缺少质量事件方法实现。

**Step 3: Write minimal implementation**

- 在 ports 增加：
  - `OutboxQualityEvent`
  - `OutboxQualityEventCreate`
  - `CreateQualityEvent(...) (bool, error)`
  - `ListQualityEvents(...)`

**Step 4: Run test to verify it passes**

Run: 同 Step 2  
Expected: 进入下一阶段失败（尚未实现 repository/model）。

### Task 2: SQLite Model 与 Repository 实现

**Files:**
- Create: `internal/infrastructure/persistence/sqlite/model/quality_event.go`
- Modify: `internal/infrastructure/persistence/sqlite/repository/outbox_repository.go`
- Modify: `internal/infrastructure/persistence/sqlite/repository/outbox_repository_test.go`
- Modify: `internal/bootstrap/app.go`
- Modify: `internal/usecase/outbox/service_test.go`

**Step 1: Write the failing test**

- 在 `outbox_repository_test.go` 新增去重测试：
  - 首次 `CreateQualityEvent` 返回 `true`
  - 同 key 再次写入返回 `false`
  - `ListQualityEvents` 长度为 1

**Step 2: Run test to verify it fails**

Run: `go test ./internal/infrastructure/persistence/sqlite/repository -run QualityEvent`  
Expected: 方法未实现或表不存在导致失败。

**Step 3: Write minimal implementation**

- 新增 `quality_events` GORM model。
- repository 中实现插入与查询，插入使用 `ON CONFLICT DO NOTHING`。
- 自动迁移加入 `QualityEvent`。

**Step 4: Run test to verify it passes**

Run: `go test ./internal/infrastructure/persistence/sqlite/repository -run QualityEvent`  
Expected: PASS。

### Task 3: 用例层质量事件接入（TDD）

**Files:**
- Modify: `internal/usecase/outbox/service.go`
- Create: `internal/usecase/outbox/quality_event_ingest.go`
- Create: `internal/usecase/outbox/quality_event_ingest_test.go`

**Step 1: Write the failing test**

- `review changes_requested` 写回：
  - 结构化 comment 含 `review:changes_requested`
  - `ResultCode: review_changes_requested`
  - `Next` 路由到责任角色
- 去重：
  - 同 key 第二次写入 `Duplicate=true`
  - 不新增 comment
- `ci fail` 无 evidence 直接报错
- `ci pass` 写回 `qa:pass` 并路由 `@integrator`

**Step 2: Run test to verify it fails**

Run: `go test ./internal/usecase/outbox -run QualityEvent`  
Expected: 编译失败或断言失败。

**Step 3: Write minimal implementation**

- 增加输入输出类型与 `IngestQualityEvent`、`ListQualityEvents`。
- 统一映射逻辑、失败证据校验、幂等键生成、结构化 comment 回写。
- 保守模式：不设置 state，不改 assignee/close。

**Step 4: Run test to verify it passes**

Run: `go test ./internal/usecase/outbox -run QualityEvent`  
Expected: PASS。

### Task 4: CLI 暴露

**Files:**
- Create: `cmd/outbox_quality.go`
- Modify: `README.md`

**Step 1: Write the failing test**

- 本仓库无 cmd 单测，改为命令 smoke 验证。

**Step 2: Run test to verify it fails**

Run: `go run . outbox quality ingest --help`  
Expected: 现状命令不存在或无子命令。

**Step 3: Write minimal implementation**

- 增加：
  - `outbox quality ingest`
  - `outbox quality list`
- 支持 `--payload` / `--payload-file`、`--evidence`、`--event-key`。

**Step 4: Run test to verify it passes**

Run:
- `go run . outbox quality ingest --help`
- `go run . outbox quality list --help`
Expected: 命令可用，帮助文本正常。

### Task 5: 全量验证

**Files:**
- Modify: none (verification only)

**Step 1: Run focused tests**

Run:
- `go test ./internal/infrastructure/persistence/sqlite/repository -run QualityEvent`
- `go test ./internal/usecase/outbox -run QualityEvent`

Expected: PASS。

**Step 2: Run full tests**

Run: `go test ./...`  
Expected: PASS。

**Step 3: Smoke CLI**

Run:
- `go run . outbox quality ingest --help`
- `go run . outbox quality list --help`

Expected: PASS。

# Demo04 测试工作记录（2026-01-29）

本文记录 Demo04（`taskctl run --workflow demo04`）相关测试补强与“并行可验收”工作，便于本地复盘与后续扩展。

## 一、当前新增/补强的测试

### 1) `internal/execution`（单元 + 验收）
- Registry：
  - 未知 workflow 报错、非法注册（nil/空名）、重复注册、Get 成功：`internal/execution/registry_test.go`
- Result：
  - `HasBlocker()`（空/仅 warn/大小写+空白兼容 blocker）：`internal/execution/workflow_test.go`
- 并行 Runner：
  - `RunMPUs` 遵守 `GlobalMax` / `PerRole` / `PerTeam`，以及“首个 error 触发 cancel 且后续 MPU 不应继续执行 fn”：`internal/execution/runner_test.go`
  - `RunMPUs` 在 acquire 前后都检查 `ctx`（harden cancel path）：`internal/execution/runner.go`
- Caps：
  - `CapsFromPlan` 默认值（`MaxParallel=0` → `GlobalMax=4`）、映射（PerRole/PerTeam）、非法输入：`internal/execution/caps_test.go`
- PPT Adapter：
  - 坏 JSON → `where=adapter` blocker
  - 合法 IR → 生成 `deliver/ppt_renderer_input.json`
  - slides 为空 → blocker 且不生成输出：`internal/execution/ppt_adapter_test.go`
- PPT Renderer：
  - demo04 路径：生成 `deliver/slides.html`
  - 缺输入 → `where=renderer` blocker 且不生成输出
  - HTML 输出对用户内容做 escape（避免注入）：`internal/execution/ppt_renderer_test.go`
- Demo04 基础行为：
  - 产物存在（`deliver/*` + `issues.json`），并默认无 blocker：`internal/execution/demo04_test.go`
- Demo04 并行“验收测试”（关键）：
  - 通过落盘 `revs/{rev}/run_stats.json` 记录 `max_in_flight`
  - 用不同 DeliveryPlan（`max_parallel: 1/2`）断言 `max_in_flight == 1/2`：`internal/execution/demo04_parallel_acceptance_test.go`

### 2) `internal/cli`（CLI smoke）
- `taskctl run --workflow demo04` smoke
- `--delivery-plan` 生效且会快照到 `revs/r1/delivery_plan.yaml`
- 错误用法：
  - 仅传 `--delivery-plan`（无 `--workflow`）应报错
  - `--workflow` 与 `--entrypoint` 互斥：`internal/cli/run_test.go`

## 二、用于测试/验收的可观测产物

- `revs/r1/run_stats.json`：demo04 运行统计（目前用于并行验收）
- `revs/r1/delivery_plan.yaml`：当 CLI 传入 `--delivery-plan` 时写入的快照（进入 manifest/evidence）

## 三、本地运行方式（建议）

### 1) 全量
```bash
go test ./...
```

### 2) 并行验收（推荐）
```bash
go test ./internal/execution -run TestDemo04_Run_RespectsMaxParallel_Acceptance -count=1
```

### 3) CLI smoke
```bash
go test ./internal/cli -run TestRunCmd_ -count=1
```

## 四、相关提交（便于追溯）
- `02eb805`：扩展 execution/cli 单元测试覆盖；强化 `RunMPUs` 的 cancel 路径
- `af4b82e`：新增 `run_stats.json` 并行观测 + 并行验收测试（max_parallel=1/2）


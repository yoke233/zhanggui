# P3 Prerequisites Wave 3 — Spec Plugin/Config/Factory

> **For Agent:** REQUIRED SUB-SKILL: Use executing-wave-plans to implement this wave and gate it before Wave 4.

## Wave Goal

建立 Plan-level Spec 能力的基础装配：定义 SpecPlugin 接口、引入顶层 `spec` 配置、让 factory/bootstrap 能按配置加载 Spec 插槽。

## Depends On

- `[W2-T1, W2-T2, W2-T3]`

## Wave Entry Data

- `SlotSpec` 常量存在，但没有 `core.SpecPlugin` 合同。
- 配置仍倾向 `agents.openspec` 表达，未形成独立 spec provider 语义。
- factory `BootstrapSet` 尚未承载 Spec 插件实例。

## Tasks

### Task W3-T1: 定义 SpecPlugin 最小合同 + noop 实现

**Files:**
- Modify: `internal/core/plugin.go`
- Create: `internal/plugins/spec-noop/spec.go`
- Create: `internal/plugins/spec-noop/spec_test.go`
- Create: `internal/plugins/spec-noop/module.go`
- Modify: `internal/core/doc.go`

**Depends on:** `[]`

**Step 1: Write failing test**
```text
新增测试：
- TestSpecPluginInterface_CompileGuard
- TestNoopSpec_IsInitializedFalse
- TestNoopSpec_GetContext_ReturnsEmptyContext
```

**Step 2: Run to confirm failure**
Run: `go test ./internal/core ./internal/plugins/spec-noop -run 'Spec|Noop'`
Expected: 接口或实现缺失导致编译失败。

**Step 3: Minimal implementation**
```text
新增 core.SpecPlugin（必须嵌入 core.Plugin）：IsInitialized/GetContext。
新增 SpecContext 结构和 spec-noop 默认实现。
```

**Step 4: Run tests to confirm pass**
Run: `go test ./internal/core ./internal/plugins/spec-noop -run 'Spec|Noop'`
Expected: PASS。

**Step 5: Commit**
```bash
git add internal/core/plugin.go internal/plugins/spec-noop/spec.go internal/plugins/spec-noop/spec_test.go internal/plugins/spec-noop/module.go internal/core/doc.go
git commit -m "feat(spec): add spec plugin contract and noop provider"
```

### Task W3-T2: 配置模型迁移到顶层 `spec`

**Files:**
- Modify: `internal/config/types.go`
- Modify: `internal/config/defaults.go`
- Modify: `internal/config/merge.go`
- Modify: `internal/config/config_test.go`
- Modify: `internal/config/merge_hierarchy_test.go`
- Modify: `configs/defaults.yaml`

**Depends on:** `[W3-T1]`

**Step 1: Write failing test**
```text
新增测试：
- TestLoadDefaults_IncludesSpecConfig
- TestMergeHierarchy_SpecLayerOverridesGlobal
- TestConfigZeroValue_SpecSafeWhenMissing
```

**Step 2: Run to confirm failure**
Run: `go test ./internal/config -run 'Spec|Merge|Defaults'`
Expected: spec 结构不存在或 merge 行为不正确。

**Step 3: Minimal implementation**
```text
Config 顶层新增 SpecConfig/SpecLayer：provider/enabled/on_failure/openspec 子段。
defaults + merge + defaults.yaml 同步更新。
保留 agents.openspec 仅作 agent binary 配置，不作为 spec provider 入口。
明确 on_failure 语义：warn=回退 noop 且继续；fail=返回错误并中止启动/构建。
```

**Step 4: Run tests to confirm pass**
Run: `go test ./internal/config -run 'Spec|Merge|Defaults'`
Expected: PASS。

**Step 5: Commit**
```bash
git add internal/config/types.go internal/config/defaults.go internal/config/merge.go internal/config/config_test.go internal/config/merge_hierarchy_test.go configs/defaults.yaml
git commit -m "feat(config): add top-level spec config and merge semantics"
```

### Task W3-T3: Factory/Bootstrap 装配 Spec 插槽

**Files:**
- Modify: `internal/plugins/factory/factory.go`
- Modify: `internal/plugins/factory/factory_test.go`
- Modify: `cmd/ai-flow/commands.go`
- Modify: `cmd/ai-flow/commands_test.go`

**Depends on:** `[W3-T1, W3-T2]`

**Step 1: Write failing test**
```text
新增测试：
- TestBuildWithRegistry_LoadsSpecWhenEnabled
- TestBuildWithRegistry_UsesNoopSpecWhenDisabled
- TestBootstrapSet_ContainsSpecPlugin
- TestBuildWithRegistry_SpecProviderMissing_OnFailureWarn_FallbackNoop
- TestBuildWithRegistry_SpecProviderMissing_OnFailureFail_ReturnsError
- TestBuildWithRegistry_SpecInitError_OnFailureWarn_FallbackNoop
- TestBuildWithRegistry_SpecInitError_OnFailureFail_ReturnsError
```

**Step 2: Run to confirm failure**
Run: `go test ./internal/plugins/factory ./cmd/ai-flow -run 'Spec|Bootstrap|Registry'`
Expected: BootstrapSet 缺字段或装配路径缺失。

**Step 3: Minimal implementation**
```text
BootstrapSet 增加 Spec core.SpecPlugin。
buildWithRegistry 按 cfg.Spec.Enabled/provider/on_failure 装配 spec 插件。
缺 provider、未注册 provider、provider.Init 失败时，按 on_failure 执行回退或 fail-fast。
commands 启动编排链路透传 Spec 实例给 secretary 相关组件（按需）。
```

**Step 4: Run tests to confirm pass**
Run: `go test ./internal/plugins/factory ./cmd/ai-flow -run 'Spec|Bootstrap|Registry'`
Expected: PASS。

**Step 5: Commit**
```bash
git add internal/plugins/factory/factory.go internal/plugins/factory/factory_test.go cmd/ai-flow/commands.go cmd/ai-flow/commands_test.go
git commit -m "feat(factory): wire spec plugin into bootstrap"
```

## Spec Failure Policy Matrix

| spec.enabled | provider | on_failure | 行为 |
|---|---|---|---|
| false | 任意 | 任意 | 强制使用 `spec-noop`，不报错 |
| true | `noop` 或空 | 任意 | 使用 `spec-noop` |
| true | 已注册 provider，Init/GetContext 成功 | 任意 | 使用目标 provider |
| true | provider 缺失/未注册/Init 失败 | `warn` | warning + 回退 `spec-noop` |
| true | provider 缺失/未注册/Init 失败 | `fail` | 直接返回错误（fail-fast） |

## Test Strategy Per Task

| Task | Unit | Integration |
|---|---|---|
| W3-T1 | 接口编译约束 + noop 行为 | secretary 调用 spec 上下文 smoke |
| W3-T2 | defaults/merge/零值安全 | global+project 配置叠加验证 |
| W3-T3 | registry/factory 装配分支 | CLI 启动 bootstrap 路径 |

## Risks and Mitigations

- 风险：配置迁移后旧配置文件失效。  
  缓解：保留零值兼容 + 明确迁移提示测试。
- 风险：factory 装配失败影响启动。  
  缓解：spec provider 装配失败回退 noop 并 warning。

## Wave E2E/Smoke Cases and Entry Data

### Entry Data
- 全局配置含 `spec.provider=openspec` 与项目级覆盖 `spec.provider=noop` 两套样例。

### Smoke Cases
- `spec.enabled=false` 时系统仍能正常启动并执行计划。
- `spec.enabled=true` 且 provider=noop 时不会阻塞 secretary。

## Wave Exit Gate
- Execute mandatory gate sequence via `executing-wave-plans`.
- Wave-specific acceptance:
  - [ ] `core.SpecPlugin` 与默认 noop provider 可用。
  - [ ] 顶层 `spec` 配置生效且 merge 规则明确。
  - [ ] factory/bootstrap 已具备 Spec 插槽装配能力。
- Wave-specific verification:
  - [ ] `go test ./internal/core ./internal/config ./internal/plugins/spec-noop ./internal/plugins/factory ./cmd/ai-flow` 通过。
  - [ ] `go build ./...` 通过。
- Boundary-change verification (if triggered):
  - [ ] `go test ./internal/secretary ./internal/web -run 'Config|Bootstrap|Plan'` 通过。

## Next Wave Entry Condition
- Governed by `executing-wave-plans` verdict rule (`Go` / satisfied `Conditional Go` only).

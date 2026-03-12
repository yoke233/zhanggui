# Test Scripts

This folder stores reusable one-shot test scripts for current backend/frontend regression checks.

## Safety Defaults

- No infinite loops.
- No background jobs or daemon processes.
- Sequential execution only.
- `go test` includes explicit timeout (`GOTEST_TIMEOUT`, default `20m`).
- Backend parallelism is limited (`-p 4`, `GOMAXPROCS=4` by default).
- Frontend unit tests run in one-shot mode (`vitest run --run`, no watch).

## Scripts

- `backend-all.ps1`: run current backend full suite (`go test ./...`).
- `backend-e2e.ps1`: run backend lifecycle / integration suites on the current codebase:
  - `internal/application/flow` `TestIssueE2E_*`
  - `internal/adapters/http` `TestAPI_ExecutionProbeLifecycle|TestAPI_E2E_IssueLifecycle|TestIntegration_*`
  - `internal/adapters/agent/acpclient` lifecycle tests (default skip; enable with `-IncludeACPClientIntegration`)
- `frontend-unit.ps1`: run frontend unit tests.
- `frontend-build.ps1`: run frontend production build.
- `p35-terminology-gate.ps1`: enforce P3.5 terminology gate (fail on `review_panel|change_agent|implement_agent`, require role-driven terms).
- `p3-integration.ps1`: run current smoke baseline sequence.
- `project-admin-e2e.ps1`: run browser E2E for project admin (`local_path` + `local_new` flows) via Playwright.
- `codeup-cr-smoke.ps1`: minimal Codeup API smoke for create CR, with optional merge.
- `codeup-resource-binding.example.json`: minimal v2 git resource binding example for Codeup.

## Usage

Run from repository root:

```powershell
pwsh -NoProfile -File .\scripts\test\backend-all.ps1
pwsh -NoProfile -File .\scripts\test\backend-e2e.ps1
```

Run backend E2E with ACP client integration enabled:

```powershell
pwsh -NoProfile -File .\scripts\test\backend-e2e.ps1 -IncludeACPClientIntegration
```

Run browser E2E (headful):

```powershell
pwsh -NoProfile -File .\scripts\test\project-admin-e2e.ps1 -Headed
```

Run browser E2E (headless):

```powershell
pwsh -NoProfile -File .\scripts\test\project-admin-e2e.ps1
```

Run Codeup create-only smoke:

```powershell
pwsh -NoProfile -File .\scripts\test\codeup-cr-smoke.ps1 `
  -Token "pt-xxxx" `
  -OrganizationId "5f6ea0829cffa29cfdd39a7f" `
  -RepositoryId "5f6ea0829cffa29cfdd39a7f/xiaoin/xiaoin-rag-service" `
  -ProjectId 2369234 `
  -SourceBranch "feature/smoke" `
  -TargetBranch "main"
```

Run Codeup create + merge smoke:

```powershell
pwsh -NoProfile -File .\scripts\test\codeup-cr-smoke.ps1 `
  -Token "pt-xxxx" `
  -OrganizationId "5f6ea0829cffa29cfdd39a7f" `
  -RepositoryId "5f6ea0829cffa29cfdd39a7f/xiaoin/xiaoin-rag-service" `
  -ProjectId 2369234 `
  -SourceBranch "feature/smoke" `
  -TargetBranch "main" `
  -AutoMerge `
  -MergeType "no-fast-forward"
```

Use the Codeup binding example when creating a v2 git resource binding. `base_branch` is the default target branch for PR bootstrap, and step-level config can still override it.

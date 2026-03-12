# Test Scripts

This folder stores reusable one-shot test scripts for P3 frontend/backend integration.

## Safety Defaults

- No infinite loops.
- No background jobs or daemon processes.
- Sequential execution only.
- `go test` includes explicit timeout (`GOTEST_TIMEOUT`, default `20m`).
- Backend parallelism is limited (`-p 4`, `GOMAXPROCS=4` by default).
- Frontend unit tests run in one-shot mode (`vitest run --run`, no watch).

## Scripts

- `backend-all.ps1`: run backend full suite (`go test ./...`).
- `backend-github.ps1`: run GitHub-focused backend suites (plugins, dispatcher/e2e, webhook).
- `p35-terminology-gate.ps1`: enforce P3.5 terminology gate (fail on `review_panel|change_agent|implement_agent`, require role-driven terms).
- `frontend-unit.ps1`: run frontend unit tests.
- `frontend-build.ps1`: run frontend production build.
- `p3-integration.ps1`: run all suites above in sequence.
- `ai-flow quality-gate`: built-in local quality gate command (backend `go test ./...`, frontend `npm test` + `npm build`).
- `project-admin-e2e.ps1`: run browser E2E for project admin (`local_path` + `local_new` flows) via Playwright.
- `codeup-cr-smoke.ps1`: minimal Codeup API smoke for create CR, with optional merge.
- `codeup-resource-binding.example.json`: minimal v2 git resource binding example for Codeup.

## Usage

Run from repository root:

```powershell
pwsh -NoProfile -File .\scripts\test\p3-integration.ps1
```

Run individually:

```powershell
pwsh -NoProfile -File .\scripts\test\backend-all.ps1
pwsh -NoProfile -File .\scripts\test\backend-github.ps1
pwsh -NoProfile -File .\scripts\test\p35-terminology-gate.ps1
pwsh -NoProfile -File .\scripts\test\frontend-unit.ps1
pwsh -NoProfile -File .\scripts\test\frontend-build.ps1
```

Run built-in quality gate (cross-platform):

```bash
go run ./cmd/ai-flow quality-gate
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

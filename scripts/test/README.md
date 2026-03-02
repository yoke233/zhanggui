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
- `frontend-unit.ps1`: run frontend unit tests.
- `frontend-build.ps1`: run frontend production build.
- `p3-integration.ps1`: run all suites above in sequence.
- `project-admin-e2e.ps1`: run browser E2E for project admin (`local_path` + `local_new` flows) via Playwright.

## Usage

Run from repository root:

```powershell
pwsh -NoProfile -File .\scripts\test\p3-integration.ps1
```

Run individually:

```powershell
pwsh -NoProfile -File .\scripts\test\backend-all.ps1
pwsh -NoProfile -File .\scripts\test\backend-github.ps1
pwsh -NoProfile -File .\scripts\test\frontend-unit.ps1
pwsh -NoProfile -File .\scripts\test\frontend-build.ps1
```

Run browser E2E (headful):

```powershell
pwsh -NoProfile -File .\scripts\test\project-admin-e2e.ps1 -Headed
```

Run browser E2E (headless):

```powershell
pwsh -NoProfile -File .\scripts\test\project-admin-e2e.ps1
```

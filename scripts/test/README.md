# Test Scripts

This folder stores reusable one-shot test scripts for local backend/frontend regression checks.

GitHub Actions now runs native `go` / `npm` commands directly. These PowerShell scripts remain as a Windows-friendly local regression layer and are no longer the CI source of truth.

## Safety Defaults

- No infinite loops.
- No background jobs or daemon processes.
- Sequential execution only.
- `go test` includes explicit timeout (`GOTEST_TIMEOUT`, default `20m`).
- Backend parallelism is limited (`-p 4`, `GOMAXPROCS=4` by default).
- Frontend unit tests run in one-shot mode (`vitest run`, no watch).

## Test Types

- `backend-unit.ps1`: run backend tests excluding `TestIntegration_*`, `TestE2E_*`, and `TestReal_*`.
- `backend-integration.ps1`: run backend `TestIntegration_*` suites. Use `-IncludeACPClientIntegration` to enable ACP client integration cases.
- `backend-e2e.ps1`: run backend `TestE2E_*` suites.
- `backend-real.ps1`: run backend `TestReal_*` suites with `-tags real`.
- `frontend-unit.ps1`: run frontend unit tests.
- `frontend-e2e.ps1`: run Playwright browser E2E for project admin (`local_path` + `local_new` flows).
- `frontend-build.ps1`: run frontend production build.

## Suite Scripts

- `suite-smoke.ps1`: terminology gate, test naming gate, and backend build smoke.
- `suite-p3.ps1`: backend unit/integration/e2e + frontend unit/build + smoke baseline.
- `issue-e2e-github.ps1`: full Issue E2E smoke against a local GitHub repo (server → project → issue → exec+gate steps → ACP agent → done).
- `issue-e2e-codeup.ps1`: full Issue E2E smoke against a Codeup repo (auto-clones if needed, same flow as above).
- `codeup-cr-smoke.ps1`: minimal Codeup API smoke for create CR, with optional merge.
- `codeup-resource-binding.example.json`: minimal v2 git resource binding example for Codeup.

## Usage

Run from repository root:

```powershell
pwsh -NoProfile -File .\scripts\test\backend-unit.ps1
pwsh -NoProfile -File .\scripts\test\backend-integration.ps1
pwsh -NoProfile -File .\scripts\test\backend-e2e.ps1
pwsh -NoProfile -File .\scripts\test\backend-real.ps1
```

Run backend integration with ACP client cases enabled:

```powershell
pwsh -NoProfile -File .\scripts\test\backend-integration.ps1 -IncludeACPClientIntegration
```

Run browser E2E (headful):

```powershell
pwsh -NoProfile -File .\scripts\test\frontend-e2e.ps1 -Headed
```

Run browser E2E (headless):

```powershell
pwsh -NoProfile -File .\scripts\test\frontend-e2e.ps1
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

### Issue E2E (GitHub)

Starts a server, creates a project with local GitHub repo, creates an issue with exec+gate steps, runs ACP agents, and waits until done.

```powershell
pwsh -NoProfile -File .\scripts\test\issue-e2e-github.ps1
```

With custom options:

```powershell
pwsh -NoProfile -File .\scripts\test\issue-e2e-github.ps1 `
  -RepoPath "D:\project\test-workflow" `
  -Port 8083 `
  -TimeoutSeconds 600
```

### Issue E2E (Codeup)

Same flow as GitHub but against a Codeup (Alibaba Cloud) repo. Auto-clones the repo if not already present. Reads `[codeup].pat` from secrets.toml.

```powershell
pwsh -NoProfile -File .\scripts\test\issue-e2e-codeup.ps1
```

With custom options:

```powershell
pwsh -NoProfile -File .\scripts\test\issue-e2e-codeup.ps1 `
  -CodeupRepoUrl "https://codeup.aliyun.com/org/repo" `
  -LocalClonePath "D:\project\codeup-test-workflow" `
  -OrganizationId "5f6ea0829cffa29cfdd39a7f" `
  -BaseBranch "master" `
  -Port 8084 `
  -TimeoutSeconds 600
```

Use the Codeup binding example when creating a v2 git resource binding. `base_branch` is the default target branch for PR bootstrap, and step-level config can still override it.

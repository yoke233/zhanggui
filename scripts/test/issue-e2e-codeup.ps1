<#
.SYNOPSIS
  Issue-based E2E smoke test against a Codeup (Alibaba Cloud) repository.
  Clones the Codeup repo locally, then: server → project → resource binding → issue → exec+gate steps → run → poll.

.DESCRIPTION
  This script tests the full Issue → Steps → ACP Agent execution pipeline using a Codeup repository.
  It reads the Codeup PAT from secrets.toml [codeup] section and clones the repo to a local temp directory.
  The resource binding uses provider=codeup with organization_id and base_branch=master (Codeup default).

.EXAMPLE
  pwsh -NoProfile -File .\scripts\test\issue-e2e-codeup.ps1
  pwsh -NoProfile -File .\scripts\test\issue-e2e-codeup.ps1 -CodeupRepoUrl "https://codeup.aliyun.com/org/repo" -Port 8084
#>
[CmdletBinding()]
param(
  [int]$Port = 8084,
  [string]$SecretsPath = (Join-Path $PSScriptRoot "..\\..\\.ai-workflow\\secrets.toml"),
  [string]$CodeupRepoUrl = "https://codeup.aliyun.com/5f6ea0829cffa29cfdd39a7f/test-workflow",
  [string]$LocalClonePath = "D:\\project\\codeup-test-workflow",
  [string]$OrganizationId = "5f6ea0829cffa29cfdd39a7f",
  [string]$BaseBranch = "master",
  [string]$ExecProfileId = "worker",
  [string]$GateProfileId = "reviewer",
  [int]$TimeoutSeconds = 600
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

function Read-AdminTokenFromSecrets {
  param([Parameter(Mandatory = $true)][string]$Path)
  $raw = Get-Content -Raw -LiteralPath $Path
  $pattern = "(?ms)\[tokens\.admin\].*?token\s*=\s*'([^']+)'"
  if ($raw -match $pattern) {
    return $Matches[1]
  }
  $pattern2 = '(?ms)\[tokens\.admin\].*?token\s*=\s*"([^"]+)"'
  if ($raw -match $pattern2) {
    return $Matches[1]
  }
  throw "failed to extract [tokens.admin].token from secrets.toml"
}

function Read-CodeupPatFromSecrets {
  param([Parameter(Mandatory = $true)][string]$Path)
  $raw = Get-Content -Raw -LiteralPath $Path
  $pattern = "(?ms)\[codeup\].*?pat\s*=\s*'([^']+)'"
  if ($raw -match $pattern) {
    return $Matches[1]
  }
  $pattern2 = '(?ms)\[codeup\].*?pat\s*=\s*"([^"]+)"'
  if ($raw -match $pattern2) {
    return $Matches[1]
  }
  throw "failed to extract [codeup].pat from secrets.toml"
}

function Start-Server {
  param([int]$Port)
  $logDir = Join-Path (Get-Location) ".tmp"
  New-Item -ItemType Directory -Force -Path $logDir | Out-Null
  $stdout = Join-Path $logDir "issue-e2e-codeup-$Port.out.log"
  $stderr = Join-Path $logDir "issue-e2e-codeup-$Port.err.log"

  $p = Start-Process -FilePath "go" `
    -ArgumentList @("run", "./cmd/ai-flow", "server", "--port", "$Port") `
    -WorkingDirectory (Get-Location) `
    -RedirectStandardOutput $stdout `
    -RedirectStandardError $stderr `
    -PassThru
  return @{ Proc = $p; Stdout = $stdout; Stderr = $stderr }
}

function Wait-Health {
  param([int]$Port, [int]$Seconds = 90)
  $deadline = (Get-Date).AddSeconds($Seconds)
  while ((Get-Date) -lt $deadline) {
    try {
      $resp = Invoke-WebRequest -UseBasicParsing -TimeoutSec 2 "http://127.0.0.1:$Port/health"
      if ($resp.StatusCode -eq 200) { return $true }
    } catch {}
    Start-Sleep -Milliseconds 500
  }
  return $false
}

function Api {
  param(
    [Parameter(Mandatory)][string]$Method,
    [Parameter(Mandatory)][string]$Url,
    [Parameter(Mandatory)][string]$Token,
    [object]$Body = $null
  )
  $headers = @{ Authorization = "Bearer $Token" }
  if ($null -eq $Body) {
    return Invoke-RestMethod -Method $Method -Uri $Url -Headers $headers
  }
  $json = $Body | ConvertTo-Json -Depth 20
  return Invoke-RestMethod -Method $Method -Uri $Url -Headers $headers `
    -ContentType "application/json" -Body $json
}

# ---------------------------------------------------------------------------
# Pre-checks
# ---------------------------------------------------------------------------

if (-not (Test-Path -LiteralPath $SecretsPath)) {
  throw "secrets.toml not found: $SecretsPath"
}

$adminToken = Read-AdminTokenFromSecrets -Path $SecretsPath
$codeupPat = Read-CodeupPatFromSecrets -Path $SecretsPath
$ts = Get-Date -Format "yyyyMMdd-HHmmss"

# ---------------------------------------------------------------------------
# Clone Codeup repo if needed
# ---------------------------------------------------------------------------

if (-not (Test-Path -LiteralPath (Join-Path $LocalClonePath ".git"))) {
  Write-Host "Cloning Codeup repo to $LocalClonePath ..."
  $cloneUrl = $CodeupRepoUrl -replace "^https://", "https://oauth2:$codeupPat@"
  & git clone $cloneUrl $LocalClonePath
  if ($LASTEXITCODE -ne 0) {
    throw "Failed to clone Codeup repo."
  }
} else {
  Write-Host "Using existing Codeup clone at $LocalClonePath"
  & git -C $LocalClonePath fetch --all --prune 2>$null
}

# ---------------------------------------------------------------------------
# Start server
# ---------------------------------------------------------------------------

Write-Host "Starting server on port $Port ..."
$server = Start-Server -Port $Port
try {
  if (-not (Wait-Health -Port $Port -Seconds 90)) {
    throw "Server health check failed. See logs: $($server.Stderr)"
  }
  Write-Host "Server ready." -ForegroundColor Green

  $base = "http://127.0.0.1:$Port/api"

  # -------------------------------------------------------------------------
  # 1. Create project
  # -------------------------------------------------------------------------
  $proj = Api -Method Post -Url "$base/projects" -Token $adminToken -Body @{
    name = "codeup-issue-e2e-$ts"
    kind = "dev"
  }
  $projectId = [int64]$proj.id
  Write-Host "project_id=$projectId"

  # -------------------------------------------------------------------------
  # 2. Create resource binding (Codeup)
  # -------------------------------------------------------------------------
  $rb = Api -Method Post -Url "$base/projects/$projectId/resources" -Token $adminToken -Body @{
    kind   = "git"
    uri    = (Resolve-Path -LiteralPath $LocalClonePath).Path
    label  = "codeup-repo"
    config = @{
      provider        = "codeup"
      organization_id = $OrganizationId
      base_branch     = $BaseBranch
    }
  }
  Write-Host "resource_id=$($rb.id)"

  # -------------------------------------------------------------------------
  # 3. Create issue
  # -------------------------------------------------------------------------
  $issue = Api -Method Post -Url "$base/issues" -Token $adminToken -Body @{
    project_id = $projectId
    title      = "E2E smoke: add greeting util ($ts)"
    body       = "Create pkg/greeting/hello.go with Hello(name) returning 'Hello, <name>!'. Add hello_test.go with TestHello. Run go test ./... to verify."
    priority   = "medium"
  }
  $issueId = [int64]$issue.id
  Write-Host "issue_id=$issueId"

  # -------------------------------------------------------------------------
  # 4. Create exec step (implement)
  # -------------------------------------------------------------------------
  $step1 = Api -Method Post -Url "$base/issues/$issueId/steps" -Token $adminToken -Body @{
    name        = "implement"
    type        = "exec"
    position    = 0
    max_retries = 2
    config      = @{
      objective  = "Create pkg/greeting/hello.go with Hello(name string) string that returns 'Hello, <name>!'. Create pkg/greeting/hello_test.go with TestHello. Initialize go module if needed. Run go test ./... to verify."
      profile_id = $ExecProfileId
    }
  }
  Write-Host "step_implement_id=$($step1.id)"

  # -------------------------------------------------------------------------
  # 5. Create gate step (review)
  # -------------------------------------------------------------------------
  $step2 = Api -Method Post -Url "$base/issues/$issueId/steps" -Token $adminToken -Body @{
    name        = "review"
    type        = "gate"
    position    = 1
    max_retries = 0
    config      = @{
      objective  = "Review the greeting utility code. Check if tests pass and code quality is acceptable. Output AI_WORKFLOW_GATE_JSON with verdict."
      profile_id = $GateProfileId
    }
  }
  Write-Host "step_review_id=$($step2.id)"

  # -------------------------------------------------------------------------
  # 6. Run issue
  # -------------------------------------------------------------------------
  Write-Host "Running issue $issueId ..."
  Api -Method Post -Url "$base/issues/$issueId/run" -Token $adminToken -Body @{} | Out-Null

  # -------------------------------------------------------------------------
  # 7. Poll until done
  # -------------------------------------------------------------------------
  $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
  $status = ""
  while ((Get-Date) -lt $deadline) {
    $iss = Api -Method Get -Url "$base/issues/$issueId" -Token $adminToken
    $status = [string]$iss.status
    if ($status -in @("done", "closed", "failed", "blocked", "cancelled")) { break }
    Start-Sleep -Seconds 5
  }
  Write-Host "issue_status=$status"

  # -------------------------------------------------------------------------
  # 8. Report step results
  # -------------------------------------------------------------------------
  $steps = Api -Method Get -Url "$base/issues/$issueId/steps" -Token $adminToken
  foreach ($s in $steps) {
    Write-Host "  step=$($s.name) type=$($s.type) status=$($s.status)"
  }

  if ($status -ne "done") {
    Write-Host ""
    Write-Host "FAILED: issue did not complete (status=$status)." -ForegroundColor Red
    Write-Host "Server stderr log: $($server.Stderr)"
    exit 1
  }

  Write-Host ""
  Write-Host "Codeup Issue E2E smoke PASSED." -ForegroundColor Green

} finally {
  if ($server -and $server.Proc -and -not $server.Proc.HasExited) {
    Stop-Process -Id $server.Proc.Id -Force -ErrorAction SilentlyContinue
  }
}

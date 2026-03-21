<#
.SYNOPSIS
  E2E test: PR merge conflict → gate rework limit → blocked.
  Creates a 4-action PR work item, injects a conflict after PR opens, verifies gate blocks after max_rework_rounds.

.EXAMPLE
  pwsh -NoProfile -File .\scripts\test\merge-conflict-e2e.ps1
#>
[CmdletBinding()]
param(
  [int]$Port = 8084,
  [string]$SecretsPath = (Join-Path $PSScriptRoot "..\..\\.ai-workflow\\secrets.toml"),
  [string]$RepoPath = "D:\\project\\test-workflow",
  [string]$BaseBranch = "main",
  [int]$TimeoutSeconds = 900
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

function Read-AdminTokenFromSecrets {
  param([Parameter(Mandatory)][string]$Path)
  $raw = Get-Content -Raw -LiteralPath $Path
  $pattern = "(?ms)\[tokens\.admin\].*?token\s*=\s*'([^']+)'"
  if ($raw -match $pattern) { return $Matches[1] }
  $pattern2 = '(?ms)\[tokens\.admin\].*?token\s*=\s*"([^"]+)"'
  if ($raw -match $pattern2) { return $Matches[1] }
  throw "failed to extract [tokens.admin].token from secrets.toml"
}

function Start-Server {
  param([int]$Port)
  $logDir = Join-Path (Get-Location) ".tmp"
  New-Item -ItemType Directory -Force -Path $logDir | Out-Null
  $stdout = Join-Path $logDir "merge-conflict-e2e-$Port.out.log"
  $stderr = Join-Path $logDir "merge-conflict-e2e-$Port.err.log"
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

function Push-ConflictToMain {
  param([string]$RepoPath, [string]$Branch, [string]$ConflictFile)
  Write-Host "[CONFLICT] Pushing conflicting change to $Branch ..." -ForegroundColor Yellow

  # Save current branch
  $origBranch = git -C $RepoPath rev-parse --abbrev-ref HEAD

  # Checkout main, modify the same file the agent will touch
  git -C $RepoPath checkout $Branch 2>&1 | Out-Null
  $content = "# Conflict injection $(Get-Date -Format 'yyyy-MM-dd HH:mm:ss')`nThis line conflicts with the agent's changes.`n"
  Set-Content -Path (Join-Path $RepoPath $ConflictFile) -Value $content
  git -C $RepoPath add $ConflictFile
  git -C $RepoPath commit -m "test: inject merge conflict for E2E" 2>&1 | Out-Null
  git -C $RepoPath push origin $Branch 2>&1 | Out-Null

  Write-Host "[CONFLICT] Conflict pushed to $Branch ($ConflictFile)" -ForegroundColor Yellow

  # Return to original branch (if different)
  if ($origBranch -ne $Branch) {
    git -C $RepoPath checkout $origBranch 2>&1 | Out-Null
  }
}

function Cleanup-ConflictCommit {
  param([string]$RepoPath, [string]$Branch)
  Write-Host "[CLEANUP] Reverting conflict commit on $Branch ..." -ForegroundColor Cyan
  git -C $RepoPath checkout $Branch 2>&1 | Out-Null
  git -C $RepoPath revert HEAD --no-edit 2>&1 | Out-Null
  git -C $RepoPath push origin $Branch 2>&1 | Out-Null
  Write-Host "[CLEANUP] Conflict reverted." -ForegroundColor Cyan
}

# ---------------------------------------------------------------------------
# Pre-checks
# ---------------------------------------------------------------------------

if (-not (Test-Path -LiteralPath $SecretsPath)) {
  throw "secrets.toml not found: $SecretsPath"
}
if (-not (Test-Path -LiteralPath $RepoPath)) {
  throw "repo path not found: $RepoPath"
}

$adminToken = Read-AdminTokenFromSecrets -Path $SecretsPath
$ts = Get-Date -Format "yyyyMMdd-HHmmss"
$conflictFile = "README.md"
$conflictPushed = $false

# ---------------------------------------------------------------------------
# Cleanup: fresh runtime DB + stale worktrees + old remote branches
# ---------------------------------------------------------------------------

$runtimeDB = Join-Path $PSScriptRoot "..\..\\.ai-workflow\\data_runtime.db"
if (Test-Path -LiteralPath $runtimeDB) {
  try {
    Remove-Item -LiteralPath $runtimeDB -Force -ErrorAction Stop
    Write-Host "[CLEANUP] Removed stale runtime DB" -ForegroundColor Cyan
  } catch {
    Write-Host "[CLEANUP] Could not remove runtime DB (locked?), continuing with existing DB" -ForegroundColor Yellow
  }
}

# Prune worktrees
Push-Location $RepoPath
git worktree prune 2>&1 | Out-Null
# Remove leftover worktree directories
$wtDir = Join-Path $RepoPath ".worktrees"
if (Test-Path $wtDir) {
  Get-ChildItem -Path $wtDir -Directory | ForEach-Object {
    git worktree remove --force $_.FullName 2>&1 | Out-Null
  }
}
git worktree prune 2>&1 | Out-Null

# Delete stale remote ai-flow/* branches
$remoteBranches = git branch -r --list "origin/ai-flow/*" 2>&1
foreach ($rb in $remoteBranches) {
  $branchName = $rb.Trim() -replace "^origin/", ""
  if ($branchName) {
    Write-Host "[CLEANUP] Deleting remote branch: $branchName" -ForegroundColor Cyan
    git push origin --delete $branchName 2>&1 | Out-Null
  }
}
# Delete local ai-flow/* branches
$localBranches = git branch --list "ai-flow/*" 2>&1
foreach ($lb in $localBranches) {
  $branchName = $lb.Trim()
  if ($branchName) {
    git branch -D $branchName 2>&1 | Out-Null
  }
}
Pop-Location
Write-Host "[CLEANUP] Worktrees and branches cleaned." -ForegroundColor Cyan

# ---------------------------------------------------------------------------
# Start server
# ---------------------------------------------------------------------------

Write-Host "=== Merge Conflict E2E Test ===" -ForegroundColor Magenta
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
    name = "merge-conflict-e2e-$ts"
    kind = "dev"
  }
  $projectId = [int64]$proj.id
  Write-Host "project_id=$projectId"

  # -------------------------------------------------------------------------
  # 2. Create resource binding (enable_scm_flow!)
  # -------------------------------------------------------------------------
  $rb = Api -Method Post -Url "$base/projects/$projectId/resources" -Token $adminToken -Body @{
    kind   = "git"
    uri    = (Resolve-Path -LiteralPath $RepoPath).Path
    label  = "github-test"
    config = @{
      provider        = "github"
      default_branch  = $BaseBranch
      enable_scm_flow = $true
    }
  }
  Write-Host "resource_id=$($rb.id) (enable_scm_flow=true)"

  # -------------------------------------------------------------------------
  # 3. Create work item
  # -------------------------------------------------------------------------
  $workItem = Api -Method Post -Url "$base/work-items" -Token $adminToken -Body @{
    project_id = $projectId
    title      = "Merge conflict test ($ts)"
    body       = "Modify README.md: append a section '## Auto Test $ts' with a greeting message. This is a test for merge conflict handling."
    priority   = "medium"
  }
  $workItemId = [int64]$workItem.id
  Write-Host "work_item_id=$workItemId"

  # -------------------------------------------------------------------------
  # 4. Verify auto-bootstrapped actions (enable_scm_flow triggers auto-bootstrap)
  # -------------------------------------------------------------------------
  $actions = Api -Method Get -Url "$base/work-items/$workItemId/actions" -Token $adminToken
  Write-Host "auto-bootstrapped $($actions.Count) actions:"
  $gateActionId = $null
  foreach ($action in $actions) {
    Write-Host "  action=$($action.name) type=$($action.type) id=$($action.id)"
    if ($action.name -eq "review_merge_gate") { $gateActionId = $action.id }
  }
  if ($actions.Count -ne 4) {
    Write-Host "WARNING: expected 4 auto-bootstrapped actions, got $($actions.Count)" -ForegroundColor Yellow
  }

  # -------------------------------------------------------------------------
  # 5. Run work item
  # -------------------------------------------------------------------------
  Write-Host "Running work item $workItemId ..."
  Api -Method Post -Url "$base/work-items/$workItemId/run" -Token $adminToken -Body @{} | Out-Null

  # -------------------------------------------------------------------------
  # 6. Poll — inject conflict when open_pr action completes
  # -------------------------------------------------------------------------
  $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
  $status = ""
  $prevActionStates = @{}

  while ((Get-Date) -lt $deadline) {
    $currentWorkItem = Api -Method Get -Url "$base/work-items/$workItemId" -Token $adminToken
    $status = [string]$currentWorkItem.status

    # Fetch action statuses
    $actions = Api -Method Get -Url "$base/work-items/$workItemId/actions" -Token $adminToken
    foreach ($action in $actions) {
      $actionName = [string]$action.name
      $actionStatus = [string]$action.status
      $prevStatus = $prevActionStates[$actionName]
      if ($prevStatus -ne $actionStatus) {
        Write-Host "  action=$actionName status=$actionStatus" -ForegroundColor $(
          switch ($actionStatus) {
            "done"    { "Green" }
            "blocked" { "Yellow" }
            "failed"  { "Red" }
            "running" { "Cyan" }
            default   { "White" }
          }
        )
        $prevActionStates[$actionName] = $actionStatus
      }

      # Inject conflict right after open_pr succeeds (PR is open, but gate hasn't run yet)
      if ($actionName -eq "open_pr" -and $actionStatus -eq "done" -and -not $conflictPushed) {
        Push-ConflictToMain -RepoPath $RepoPath -Branch $BaseBranch -ConflictFile $conflictFile
        $conflictPushed = $true
      }
    }

    # Track gate action ID
    foreach ($action in $actions) {
      if ([string]$action.name -eq "review_merge_gate" -and -not $gateActionId) {
        $gateActionId = $action.id
      }
    }

    # Check gate's rework count
    if ($gateActionId) {
      try {
        $gateAction = Api -Method Get -Url "$base/actions/$gateActionId" -Token $adminToken
        $reworkCount = 0
        if ($gateAction.config -and $gateAction.config.rework_count) {
          $reworkCount = [int]$gateAction.config.rework_count
        }
        if ($reworkCount -gt 0) {
          Write-Host "  gate rework_count=$reworkCount / max=$($gateAction.config.max_rework_rounds)" -ForegroundColor Magenta
        }
      } catch {}
    }

    if ($status -in @("done", "closed", "failed", "blocked", "cancelled")) { break }
    Start-Sleep -Seconds 8
  }

  # -------------------------------------------------------------------------
  # 7. Final report
  # -------------------------------------------------------------------------
  Write-Host ""
  Write-Host "=== Final State ===" -ForegroundColor Magenta
  Write-Host "work_item_status=$status"

  $actions = Api -Method Get -Url "$base/work-items/$workItemId/actions" -Token $adminToken
  foreach ($action in $actions) {
    $extra = ""
    try {
      if ($action.config -and $null -ne $action.config.PSObject.Properties['rework_count']) {
        $extra = " rework_count=$($action.config.rework_count)"
      }
    } catch {}
    try {
      if ($action.retry_count -gt 0) {
        $extra += " retry_count=$($action.retry_count)"
      }
    } catch {}
    Write-Host "  action=$($action.name) type=$($action.type) status=$($action.status)$extra"
  }

  # Check if gate blocked (expected outcome)
  $gateAction = $actions | Where-Object { $_.name -eq "review_merge_gate" }
  if ($gateAction -and $gateAction.status -eq "blocked") {
    Write-Host ""
    Write-Host "SUCCESS: Gate blocked after rework limit reached (expected behavior)." -ForegroundColor Green
  } elseif ($status -eq "done") {
    Write-Host ""
    Write-Host "NOTE: Work item completed successfully (no conflict encountered or agent resolved it)." -ForegroundColor Yellow
  } else {
    Write-Host ""
    Write-Host "RESULT: work_item_status=$status (check logs for details)" -ForegroundColor Yellow
    Write-Host "Server stderr log: $($server.Stderr)"
  }

} finally {
  # Cleanup: revert conflict commit if we pushed one
  if ($conflictPushed) {
    try {
      Cleanup-ConflictCommit -RepoPath $RepoPath -Branch $BaseBranch
    } catch {
      Write-Host "[CLEANUP] Failed to revert conflict: $_" -ForegroundColor Red
    }
  }

  if ($server -and $server.Proc -and -not $server.Proc.HasExited) {
    Stop-Process -Id $server.Proc.Id -Force -ErrorAction SilentlyContinue
  }
}

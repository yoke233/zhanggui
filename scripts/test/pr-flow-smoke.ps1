[CmdletBinding()]
param(
  [int]$Port = 8083,
  [string]$SecretsPath = (Join-Path $PSScriptRoot "..\\..\\.ai-workflow\\secrets.toml"),
  [string]$RepoPath = (Join-Path (Get-Location) ".ai-workflow\\repos\\github.com__yoke233__test-workflow"),
  [string]$BaseBranch = "main",
  [int]$TimeoutSeconds = 1800
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

function Read-AdminTokenFromSecrets {
  param([Parameter(Mandatory = $true)][string]$Path)
  $raw = Get-Content -Raw -LiteralPath $Path
  $pattern = '(?ms)\\[tokens\\.admin\\].*?^token\\s*=\\s*\"([^\"]+)\"'
  if ($raw -notmatch $pattern) { throw "failed to extract [tokens.admin].token from secrets.toml" }
  return $Matches[1]
}

function Start-Server {
  param([int]$Port)
  $stdout = Join-Path (Get-Location) (".tmp\\pr-flow-" + $Port + ".out.log")
  $stderr = Join-Path (Get-Location) (".tmp\\pr-flow-" + $Port + ".err.log")
  New-Item -ItemType Directory -Force -Path (Split-Path $stdout) | Out-Null

  $env:AI_WORKFLOW_MOCK_EXECUTOR = "0"
  $p = Start-Process -FilePath "go" -ArgumentList @("run","./cmd/ai-flow","server","--port", "$Port") -WorkingDirectory (Get-Location) -RedirectStandardOutput $stdout -RedirectStandardError $stderr -PassThru
  return @{ Proc = $p; Stdout = $stdout; Stderr = $stderr }
}

function Wait-Health {
  param([int]$Port, [int]$Seconds = 60)
  $deadline = (Get-Date).AddSeconds($Seconds)
  while ((Get-Date) -lt $deadline) {
    try {
      $resp = Invoke-WebRequest -UseBasicParsing -TimeoutSec 2 "http://127.0.0.1:$Port/health"
      if ($resp.StatusCode -eq 200) { return $true }
    } catch {}
    Start-Sleep -Milliseconds 400
  }
  return $false
}

function Api {
  param(
    [Parameter(Mandatory = $true)][string]$Method,
    [Parameter(Mandatory = $true)][string]$Url,
    [Parameter(Mandatory = $true)][string]$Token,
    [object]$Body = $null
  )
  $headers = @{ Authorization = "Bearer $Token" }
  if ($null -eq $Body) {
    return Invoke-RestMethod -Method $Method -Uri $Url -Headers $headers
  }
  $json = $Body | ConvertTo-Json -Depth 20
  return Invoke-RestMethod -Method $Method -Uri $Url -Headers $headers -ContentType "application/json" -Body $json
}

if (-not (Test-Path -LiteralPath $SecretsPath)) { throw "secrets.toml not found: $SecretsPath" }
if (-not (Test-Path -LiteralPath $RepoPath)) { throw "repo path not found: $RepoPath" }

$adminToken = Read-AdminTokenFromSecrets -Path $SecretsPath

$server = Start-Server -Port $Port
try {
  if (-not (Wait-Health -Port $Port -Seconds 90)) {
    throw "server health check failed. stderr log: $($server.Stderr)"
  }

  $base = "http://127.0.0.1:$Port/api"
  $ts = Get-Date -Format "yyyyMMdd-HHmmss"

  $proj = Api -Method Post -Url "$base/projects" -Token $adminToken -Body @{
    name = "pr-flow-smoke-$ts"
    kind = "dev"
  }
  $projectId = [int64]$proj.id
  Write-Host "project_id=$projectId"

  $rb = Api -Method Post -Url "$base/projects/$projectId/resources" -Token $adminToken -Body @{
    kind = "git"
    uri  = (Resolve-Path -LiteralPath $RepoPath).Path
    label = "local repo"
    config = @{ default_branch = $BaseBranch }
  }
  Write-Host "resource_id=$($rb.id)"

  $workItem = Api -Method Post -Url "$base/work-items" -Token $adminToken -Body @{
    project_id = $projectId
    title = "pr-flow-smoke-$ts"
    body = "Append one smoke-test line to README.md, push a branch, open a PR, and pass review_merge_gate."
    priority = "medium"
  }
  $workItemId = [int64]$workItem.id
  Write-Host "work_item_id=$workItemId"

  $implement = Api -Method Post -Url "$base/work-items/$workItemId/actions" -Token $adminToken -Body @{
    name = "implement"
    type = "exec"
    position = 0
    agent_role = "worker"
    max_retries = 3
    config = @{
      objective = "在该仓库中做一个最小变更：在 README.md 末尾追加一行 'pr flow smoke $ts'。不要 git commit/push（后续 actions 会处理）。如果存在 Go 测试则运行 go test ./...（失败则说明原因）。"
    }
  }
  $implementId = [int64]$implement.id
  Write-Host "action_implement_id=$implementId"

  $commit = Api -Method Post -Url "$base/work-items/$workItemId/actions" -Token $adminToken -Body @{
    name = "commit_push"
    type = "exec"
    position = 1
    agent_role = "worker"
    depends_on = @($implementId)
    max_retries = 0
    config = @{
      builtin = "git_commit_push"
      commit_message = "test: pr flow smoke $ts"
    }
  }
  $commitId = [int64]$commit.id
  Write-Host "action_commit_id=$commitId"

  $openPR = Api -Method Post -Url "$base/work-items/$workItemId/actions" -Token $adminToken -Body @{
    name = "open_pr"
    type = "exec"
    position = 2
    agent_role = "worker"
    depends_on = @($commitId)
    max_retries = 0
    config = @{
      builtin = "github_open_pr"
      base = $BaseBranch
      title = "pr flow smoke $ts"
      body  = "Automated PR created by pr flow smoke."
    }
  }
  $openPrId = [int64]$openPR.id
  Write-Host "action_open_pr_id=$openPrId"

  $gate = Api -Method Post -Url "$base/work-items/$workItemId/actions" -Token $adminToken -Body @{
    name = "review_merge_gate"
    type = "gate"
    position = 3
    agent_role = "gate"
    depends_on = @($openPrId)
    max_retries = 0
    config = @{
      objective = "你是代码审查员。请在当前 worktree 中执行 git status、git diff $BaseBranch...HEAD，评估变更是否合理且无明显问题。若通过则 verdict=pass；否则 verdict=reject 并说明 reason。最后必须输出一行 AI_WORKFLOW_GATE_JSON，包含 verdict 和 reason。"
      merge_on_pass = $true
      merge_method = "squash"
      reset_upstream_closure = $true
    }
  }
  $gateId = [int64]$gate.id
  Write-Host "action_gate_id=$gateId"

  Api -Method Post -Url "$base/work-items/$workItemId/run" -Token $adminToken | Out-Null

  $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
  $status = ""
  while ((Get-Date) -lt $deadline) {
    $currentWorkItem = Api -Method Get -Url "$base/work-items/$workItemId" -Token $adminToken
    $status = [string]$currentWorkItem.status
    if ($status -in @("done","blocked","failed","cancelled")) { break }
    Start-Sleep -Milliseconds 750
  }
  Write-Host "work_item_status=$status"

  $prArtifact = Api -Method Get -Url "$base/actions/$openPrId/artifact/latest" -Token $adminToken
  if ($prArtifact -and $prArtifact.metadata -and $prArtifact.metadata.pr_url) {
    Write-Host "pr_url=$($prArtifact.metadata.pr_url)"
    Write-Host "pr_number=$($prArtifact.metadata.pr_number)"
  }

  if ($status -ne "done") {
    throw "work item did not complete successfully (status=$status). See logs: $($server.Stderr)"
  }
} finally {
  if ($server -and $server.Proc -and -not $server.Proc.HasExited) {
    Stop-Process -Id $server.Proc.Id -Force -ErrorAction SilentlyContinue
  }
}

[CmdletBinding()]
param(
  [string]$SecretsPath = (Join-Path $PSScriptRoot "..\\..\\.ai-workflow\\secrets.toml"),
  [string]$Owner = "yoke233",
  [string]$Repo = "test-workflow",
  [string]$BaseBranch = "main",
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

function Read-SecretFromToml {
  param(
    [Parameter(Mandatory = $true)][string]$Path,
    [Parameter(Mandatory = $true)][string]$Key
  )
  $raw = Get-Content -Raw -LiteralPath $Path
  $pattern = "(?m)^\s*" + [regex]::Escape($Key) + "\s*=\s*`"([^`"]+)`"\s*$"
  $m = [regex]::Match($raw, $pattern)
  if (-not $m.Success) {
    throw "secrets.toml missing key: $Key"
  }
  return $m.Groups[1].Value
}

function New-GitAskPass {
  param(
    [Parameter(Mandatory = $true)][string]$Token
  )
  $dir = Join-Path $PWD ".tmp\\askpass"
  New-Item -ItemType Directory -Force -Path $dir | Out-Null
  $cmdPath = Join-Path $dir ("git-askpass-" + [guid]::NewGuid().ToString("n") + ".cmd")
  @"
@echo off
set prompt=%~1
echo %prompt% | findstr /i "username" >nul
if %errorlevel%==0 (
  echo x-access-token
  exit /b 0
)
echo %prompt% | findstr /i "password" >nul
if %errorlevel%==0 (
  echo $Token
  exit /b 0
)
echo $Token
"@ | Set-Content -LiteralPath $cmdPath -Encoding ASCII
  return $cmdPath
}

function Invoke-GitWithPat {
  param(
    [Parameter(Mandatory = $true)][string]$Token,
    [Parameter(Mandatory = $true)][string[]]$Args,
    [string]$WorkDir = ""
  )
  $askpass = New-GitAskPass -Token $Token
  try {
    $env:GIT_ASKPASS = $askpass
    $env:GIT_TERMINAL_PROMPT = "0"
    if ($WorkDir -ne "") {
      & git -C $WorkDir @Args
    } else {
      & git @Args
    }
    if ($LASTEXITCODE -ne 0) {
      throw "git failed: git $($Args -join ' ')"
    }
  } finally {
    Remove-Item -LiteralPath $askpass -Force -ErrorAction SilentlyContinue
    Remove-Item -LiteralPath (Split-Path $askpass) -Force -Recurse -ErrorAction SilentlyContinue
    Remove-Item Env:GIT_ASKPASS -ErrorAction SilentlyContinue
    Remove-Item Env:GIT_TERMINAL_PROMPT -ErrorAction SilentlyContinue
  }
}

function Invoke-GHApi {
  param(
    [Parameter(Mandatory = $true)][string]$Token,
    [Parameter(Mandatory = $true)][string]$Method,
    [Parameter(Mandatory = $true)][string]$Url,
    [object]$Body = $null
  )
  $headers = @{
    "Authorization"        = "Bearer $Token"
    "Accept"               = "application/vnd.github+json"
    "X-GitHub-Api-Version" = "2022-11-28"
    "User-Agent"           = "ai-workflow-pat-smoke"
  }
  $json = $null
  if ($null -ne $Body) {
    $json = $Body | ConvertTo-Json -Depth 20
  }

  $resp = $null
  if ($null -eq $json) {
    $resp = Invoke-WebRequest -Method $Method -Uri $Url -Headers $headers -SkipHttpErrorCheck
  } else {
    $resp = Invoke-WebRequest -Method $Method -Uri $Url -Headers $headers -ContentType "application/json" -Body $json -SkipHttpErrorCheck
  }

  if ($resp.StatusCode -lt 200 -or $resp.StatusCode -ge 300) {
    $bodyText = [string]$resp.Content
    throw "GitHub API failed: $Method $Url (code=$($resp.StatusCode) body=$bodyText)"
  }

  if ([string]::IsNullOrWhiteSpace($resp.Content)) {
    return $null
  }
  return $resp.Content | ConvertFrom-Json
}

if (-not (Test-Path -LiteralPath $SecretsPath)) {
  throw "secrets.toml not found: $SecretsPath"
}

# Read PAT from [github] section: pat = "..."
$raw = Get-Content -Raw -LiteralPath $SecretsPath
$patMatch = [regex]::Match($raw, '(?ms)\[github\].*?pat\s*=\s*"([^"]+)"')
if (-not $patMatch.Success) {
  throw "secrets.toml missing [github] pat field"
}
$pat = $patMatch.Groups[1].Value
$gitPat = $pat
$prPat = $pat
$mergeToken = $pat

$remoteUrl = "https://github.com/$Owner/$Repo.git"
$apiBase = "https://api.github.com/repos/$Owner/$Repo"

$repoRoot = Join-Path $PWD ".tmp\\pat-smoke\\$Owner-$Repo"
New-Item -ItemType Directory -Force -Path (Split-Path $repoRoot) | Out-Null

if (-not (Test-Path -LiteralPath (Join-Path $repoRoot ".git"))) {
  Write-Host "Cloning $Owner/$Repo to $repoRoot"
  Invoke-GitWithPat -Token $gitPat -Args @("clone", $remoteUrl, $repoRoot)
} else {
  Write-Host "Using existing clone at $repoRoot"
  Invoke-GitWithPat -Token $gitPat -Args @("fetch", "--all", "--prune") -WorkDir $repoRoot
}

# Ensure base branch exists on remote (for empty repos).
$hasRemoteBase = $false
try {
  $refs = & git -C $repoRoot ls-remote --heads origin $BaseBranch 2>$null
  if ($LASTEXITCODE -eq 0 -and ($refs | Measure-Object).Count -gt 0) { $hasRemoteBase = $true }
} catch {}

if (-not $hasRemoteBase) {
  Write-Host "Remote base branch '$BaseBranch' not found. Creating initial commit and pushing..."
  & git -C $repoRoot checkout -B $BaseBranch | Out-Null
  "# $Repo" | Out-File -Encoding utf8 -NoNewline -LiteralPath (Join-Path $repoRoot "README.md")
  & git -C $repoRoot add README.md | Out-Null
  & git -C $repoRoot -c user.name="ai-flow" -c user.email="ai-flow@local" commit -m "chore: initial commit" | Out-Null
  Invoke-GitWithPat -Token $gitPat -Args @("push", "origin", $BaseBranch) -WorkDir $repoRoot
}

$ts = Get-Date -Format "yyyyMMdd-HHmmss"
$branch = "ai-flow/pat-smoke-$ts"

Write-Host "Creating change branch $branch"
& git -C $repoRoot checkout -B $branch | Out-Null
"smoke $ts" | Out-File -Encoding utf8 -NoNewline -Append -LiteralPath (Join-Path $repoRoot "README.md")
& git -C $repoRoot add README.md | Out-Null
& git -C $repoRoot -c user.name="ai-flow" -c user.email="ai-flow@local" commit -m "test: pat smoke $ts" | Out-Null

Write-Host "Pushing branch via github.pat"
Invoke-GitWithPat -Token $gitPat -Args @("push", "-u", "origin", $branch) -WorkDir $repoRoot

Write-Host "Creating PR via github.pat"
$pr = Invoke-GHApi -Token $prPat -Method "POST" -Url "$apiBase/pulls" -Body @{
  title = "PAT smoke $ts"
  head  = $branch
  base  = $BaseBranch
  body  = "Automated PAT smoke: push -> PR -> merge"
  draft = $false
}
$prNumber = [int]$pr.number
if ($prNumber -le 0) { throw "Failed to read PR number from API response." }
Write-Host "PR #$prNumber created: $($pr.html_url)"

Write-Host "Merging PR via github.pat"
$merge = Invoke-GHApi -Token $mergeToken -Method "PUT" -Url "$apiBase/pulls/$prNumber/merge" -Body @{
  merge_method   = "squash"
  commit_title   = "merge: PAT smoke $ts"
  commit_message = "squash merge by ai-workflow pat smoke"
}
if (-not $merge.merged) {
  $message = [string]$merge.message
  throw "Merge failed for PR #${prNumber}: $message"
}
Write-Host "PR #$prNumber merged successfully."

Write-Host "Deleting remote branch $branch"
Invoke-GitWithPat -Token $gitPat -Args @("push", "origin", "--delete", $branch) -WorkDir $repoRoot

Write-Host "PAT smoke completed."

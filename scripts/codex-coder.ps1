param(
  [ValidateSet('pass', 'fail')]
  [string]$Status = 'pass',
  [string]$Summary = '',
  [string]$ResultCode = '',
  [string]$Commit = '',
  [string]$Evidence = ''
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

if (-not [string]::IsNullOrWhiteSpace($env:CODEX_CODER_STATUS)) {
  $Status = $env:CODEX_CODER_STATUS.Trim().ToLowerInvariant()
}
if ($Status -notin @('pass', 'fail')) {
  throw "invalid status: $Status (allowed: pass|fail)"
}

if ([string]::IsNullOrWhiteSpace($Summary) -and -not [string]::IsNullOrWhiteSpace($env:CODEX_CODER_SUMMARY)) {
  $Summary = $env:CODEX_CODER_SUMMARY.Trim()
}
if ([string]::IsNullOrWhiteSpace($ResultCode) -and -not [string]::IsNullOrWhiteSpace($env:CODEX_CODER_RESULT_CODE)) {
  $ResultCode = $env:CODEX_CODER_RESULT_CODE.Trim()
}
if ([string]::IsNullOrWhiteSpace($Commit) -and -not [string]::IsNullOrWhiteSpace($env:CODEX_CODER_COMMIT)) {
  $Commit = $env:CODEX_CODER_COMMIT.Trim()
}
if ([string]::IsNullOrWhiteSpace($Evidence) -and -not [string]::IsNullOrWhiteSpace($env:CODEX_CODER_EVIDENCE)) {
  $Evidence = $env:CODEX_CODER_EVIDENCE.Trim()
}

if ([string]::IsNullOrWhiteSpace($Summary)) {
  if ($Status -eq 'pass') {
    $Summary = 'coding completed'
  } else {
    $Summary = 'coding failed'
  }
}

if ([string]::IsNullOrWhiteSpace($ResultCode)) {
  if ($Status -eq 'pass') {
    $ResultCode = 'none'
  } else {
    $ResultCode = 'manual_intervention'
  }
}

if ([string]::IsNullOrWhiteSpace($Commit)) {
  $sha = ''
  $hasCommit = $false
  try {
    $shaOutput = & git rev-parse HEAD 2>$null
    if ($LASTEXITCODE -eq 0) {
      $sha = [string]$shaOutput
      $hasCommit = -not [string]::IsNullOrWhiteSpace($sha)
    }
  } catch {
    $hasCommit = $false
  }
  if ($hasCommit) {
    $Commit = 'git:' + $sha.Trim()
  } else {
    $Commit = 'none'
  }
}

$runID = if ([string]::IsNullOrWhiteSpace($env:ZG_RUN_ID)) {
  (Get-Date).ToUniversalTime().ToString('yyyyMMddHHmmss')
} else {
  $env:ZG_RUN_ID.Trim()
}

if ([string]::IsNullOrWhiteSpace($Evidence)) {
  $Evidence = "codex-coder://$runID"
}

$result = [ordered]@{
  status      = $Status
  summary     = $Summary
  result_code = $ResultCode
  commit      = $Commit
  evidence    = $Evidence
}

$result | ConvertTo-Json -Compress

if ($Status -eq 'fail') {
  exit 1
}

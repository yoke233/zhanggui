param(
  [ValidateSet('pass', 'fail')]
  [string]$Status = 'pass',
  [string]$Summary = '',
  [string]$ResultCode = '',
  [string]$Evidence = ''
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

if (-not [string]::IsNullOrWhiteSpace($env:CODEX_TEST_STATUS)) {
  $Status = $env:CODEX_TEST_STATUS.Trim().ToLowerInvariant()
}
if ($Status -notin @('pass', 'fail')) {
  throw "invalid status: $Status (allowed: pass|fail)"
}

if ([string]::IsNullOrWhiteSpace($Summary) -and -not [string]::IsNullOrWhiteSpace($env:CODEX_TEST_SUMMARY)) {
  $Summary = $env:CODEX_TEST_SUMMARY.Trim()
}
if ([string]::IsNullOrWhiteSpace($ResultCode) -and -not [string]::IsNullOrWhiteSpace($env:CODEX_TEST_RESULT_CODE)) {
  $ResultCode = $env:CODEX_TEST_RESULT_CODE.Trim()
}
if ([string]::IsNullOrWhiteSpace($Evidence) -and -not [string]::IsNullOrWhiteSpace($env:CODEX_TEST_EVIDENCE)) {
  $Evidence = $env:CODEX_TEST_EVIDENCE.Trim()
}

if ([string]::IsNullOrWhiteSpace($Summary)) {
  if ($Status -eq 'pass') {
    $Summary = 'tests passed'
  } else {
    $Summary = 'tests failed'
  }
}

if ([string]::IsNullOrWhiteSpace($ResultCode)) {
  if ($Status -eq 'pass') {
    $ResultCode = 'none'
  } else {
    $ResultCode = 'ci_failed'
  }
}

$runID = if ([string]::IsNullOrWhiteSpace($env:ZG_RUN_ID)) {
  (Get-Date).ToUniversalTime().ToString('yyyyMMddHHmmss')
} else {
  $env:ZG_RUN_ID.Trim()
}

if ([string]::IsNullOrWhiteSpace($Evidence)) {
  $Evidence = "codex-test://$runID"
}

$result = [ordered]@{
  status      = $Status
  summary     = $Summary
  result_code = $ResultCode
  evidence    = $Evidence
}

$result | ConvertTo-Json -Compress

if ($Status -eq 'fail') {
  exit 1
}

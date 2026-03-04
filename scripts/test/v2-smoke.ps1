[CmdletBinding()]
param(
    [switch]$SkipTerminologyGate,
    [switch]$SkipGoTests
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

. (Join-Path $PSScriptRoot "common.ps1")

$repoRoot = Enter-RepoRoot -ScriptRoot $PSScriptRoot
Set-SafeTestEnvironment

Write-Host "RepoRoot: $repoRoot"
Write-Host "Smoke target: issue -> profile -> run -> run/review events"
Write-Host "GOMAXPROCS=$env:GOMAXPROCS, GOTEST_TIMEOUT=$env:GOTEST_TIMEOUT"

if (-not $SkipTerminologyGate) {
    Invoke-Step -Name "Terminology gate (README + docs/spec)" -Command {
        $legacyPattern = '\\b(plan|plans|task|tasks|Run|Runs|dag|secretary)\\b'
        $hits = & rg -n --ignore-case $legacyPattern README.md docs/spec

        if ($LASTEXITCODE -eq 0) {
            Write-Host $hits
            throw "Legacy terminology found in README/docs/spec."
        }
        if ($LASTEXITCODE -gt 1) {
            throw "Failed to run terminology gate with rg."
        }

        Write-Host "Terminology gate passed."
    }
}

if (-not $SkipGoTests) {
    Invoke-Step -Name "Issue/profile/run API smoke" -CheckLastExitCode -Command {
        go test -count=1 -timeout $env:GOTEST_TIMEOUT ./internal/web -run 'TestPlanCreateFromFilesPassesSourceFilesAndReviewInput|TestPlanHistoryEndpointsReturnReviewRecordsAndChanges|TestPlanTimelineAggregatesAndSupportsFiltersPaginationAndAliases|TestChatSessionCreateWithRole|TestListChatSessionEvents'
    }

    Invoke-Step -Name "Run event persistence smoke" -CheckLastExitCode -Command {
        go test -count=1 -timeout $env:GOTEST_TIMEOUT ./internal/plugins/store-sqlite -run 'TestChatRunEventCRUD'
        go test -count=1 -timeout $env:GOTEST_TIMEOUT ./internal/teamleader -run 'TestHandleSessionUpdatePersistsNonChunkEvent'
    }
}

Write-Host ""
Write-Host "V2 smoke completed." -ForegroundColor Green

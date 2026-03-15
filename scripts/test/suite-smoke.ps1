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
Write-Host "Smoke target: buildable current baseline"
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

Invoke-Step -Name "Test naming gate" -Command {
    $legacyPattern = 'TestWorkItemE2E_|TestAPI_E2E_|Test.*_E2E\b|real_integration_test\.go|TODO.*integration|needs integration|补集成测试|后续补 E2E'
    $hits = & rg -n --hidden -S $legacyPattern internal cmd web

    if ($LASTEXITCODE -eq 0) {
        Write-Host $hits
        throw "Legacy test naming or legacy test TODO markers found."
    }
    if ($LASTEXITCODE -gt 1) {
        throw "Failed to run test naming gate with rg."
    }

    Write-Host "Test naming gate passed."
}

if (-not $SkipGoTests) {
    Invoke-Step -Name "Current backend build smoke" -CheckLastExitCode -Command {
        go build ./...
    }
}

Write-Host ""
Write-Host "Smoke completed." -ForegroundColor Green

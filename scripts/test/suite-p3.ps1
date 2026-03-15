[CmdletBinding()]
param()

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

. (Join-Path $PSScriptRoot "common.ps1")

$repoRoot = Enter-RepoRoot -ScriptRoot $PSScriptRoot
Set-SafeTestEnvironment

Write-Host "RepoRoot: $repoRoot"
Write-Host "Run mode: sequential, no background jobs, no loops."
Write-Host "GOMAXPROCS=$env:GOMAXPROCS, GOTEST_TIMEOUT=$env:GOTEST_TIMEOUT"

Invoke-Step -Name "Backend unit baseline" -Command {
    & (Join-Path $PSScriptRoot "backend-unit.ps1")
}

Invoke-Step -Name "Backend integration baseline" -Command {
    & (Join-Path $PSScriptRoot "backend-integration.ps1")
}

Invoke-Step -Name "Backend E2E baseline" -Command {
    & (Join-Path $PSScriptRoot "backend-e2e.ps1")
}

Invoke-Step -Name "Frontend unit baseline" -Command {
    & (Join-Path $PSScriptRoot "frontend-unit.ps1")
}

Invoke-Step -Name "Frontend build baseline" -Command {
    & (Join-Path $PSScriptRoot "frontend-build.ps1")
}

Invoke-Step -Name "Smoke suite baseline" -Command {
    & (Join-Path $PSScriptRoot "suite-smoke.ps1")
}

Write-Host ""
Write-Host "P3 suite completed." -ForegroundColor Green

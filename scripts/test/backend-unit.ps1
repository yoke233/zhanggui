[CmdletBinding()]
param()

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

. (Join-Path $PSScriptRoot "common.ps1")

$repoRoot = Enter-RepoRoot -ScriptRoot $PSScriptRoot
Set-SafeTestEnvironment

Write-Host "RepoRoot: $repoRoot"
Write-Host "GOMAXPROCS=$env:GOMAXPROCS, GOTEST_TIMEOUT=$env:GOTEST_TIMEOUT"
Write-Host "Backend unit target: exclude integration/e2e/real suites"

Invoke-Step -Name "Backend unit suites" -CheckLastExitCode -Command {
    go test -p 4 -count=1 -timeout $env:GOTEST_TIMEOUT ./... -skip '^Test(Integration|E2E|Real)_'
}

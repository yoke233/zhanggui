[CmdletBinding()]
param()

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

. (Join-Path $PSScriptRoot "common.ps1")

$repoRoot = Enter-RepoRoot -ScriptRoot $PSScriptRoot
Set-SafeTestEnvironment

Write-Host "RepoRoot: $repoRoot"
Write-Host "Backend real target: TestReal_* with -tags real"
Write-Host "GOMAXPROCS=$env:GOMAXPROCS, GOTEST_TIMEOUT=$env:GOTEST_TIMEOUT"

Invoke-Step -Name "Backend real suites" -CheckLastExitCode -Command {
    go test -tags real -p 4 -count=1 -timeout $env:GOTEST_TIMEOUT ./... -run '^TestReal_'
}

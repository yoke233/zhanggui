[CmdletBinding()]
param(
    [switch]$IncludeACPClientIntegration
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

. (Join-Path $PSScriptRoot "common.ps1")

$repoRoot = Enter-RepoRoot -ScriptRoot $PSScriptRoot
Set-SafeTestEnvironment

Write-Host "RepoRoot: $repoRoot"
Write-Host "E2E target: current backend lifecycle and integration suites"
Write-Host "GOMAXPROCS=$env:GOMAXPROCS, GOTEST_TIMEOUT=$env:GOTEST_TIMEOUT"

if ($IncludeACPClientIntegration) {
    $env:AI_WORKFLOW_RUN_ACPCLIENT_INTEGRATION = "1"
    Write-Host "ACP client integration: enabled via AI_WORKFLOW_RUN_ACPCLIENT_INTEGRATION=1"
} else {
    Remove-Item Env:AI_WORKFLOW_RUN_ACPCLIENT_INTEGRATION -ErrorAction SilentlyContinue
    Write-Host "ACP client integration: skipped by default (use -IncludeACPClientIntegration to enable)"
}

Invoke-Step -Name "Flow engine E2E suites" -CheckLastExitCode -Command {
    go test -p 4 -count=1 -timeout $env:GOTEST_TIMEOUT ./internal/application/flow -run 'TestIssueE2E_'
}

Invoke-Step -Name "HTTP adapter integration and API lifecycle suites" -CheckLastExitCode -Command {
    go test -p 4 -count=1 -timeout $env:GOTEST_TIMEOUT ./internal/adapters/http -run 'Test(API_ExecutionProbeLifecycle|API_E2E_IssueLifecycle|Integration_)'
}

Invoke-Step -Name "ACP client lifecycle suites" -CheckLastExitCode -Command {
    go test -p 4 -count=1 -timeout $env:GOTEST_TIMEOUT ./internal/adapters/agent/acpclient -run 'Test(ClientLifecycle|FullLifecycleWithRoleMetadata)'
}

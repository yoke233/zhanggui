#!/usr/bin/env pwsh
# generate-actions.ps1 — AI auto-decompose a task description into actions.
#
# Usage:
#   pwsh -NoProfile -File generate-actions.ps1 <work-item-id> '<description>'
#
# The backend uses the plan-actions planning service to generate a DAG
# and materializes the actions into the work item.

param(
  [Parameter(Mandatory = $true, Position = 0)]
  [string]$WorkItemId,

  [Parameter(Mandatory = $true, Position = 1)]
  [string]$Description
)

$ErrorActionPreference = "Stop"

$server = $env:AI_WORKFLOW_SERVER_ADDR
if (-not $server) {
  Write-Error "AI_WORKFLOW_SERVER_ADDR is required"
  exit 1
}

$headers = @{ "Content-Type" = "application/json" }
$token = $env:AI_WORKFLOW_API_TOKEN
if ($token) {
  $headers["Authorization"] = "Bearer $token"
}

$body = @{ description = $Description } | ConvertTo-Json -Compress

try {
  $response = Invoke-WebRequest `
    -Method Post `
    -Uri "$server/api/work-items/$WorkItemId/generate-actions" `
    -Headers $headers `
    -Body $body `
    -TimeoutSec 120

  Write-Output $response.Content
} catch {
  Write-Error "Error generating actions: $($_.Exception.Message)"
  exit 1
}

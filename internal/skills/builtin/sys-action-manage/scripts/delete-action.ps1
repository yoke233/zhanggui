#!/usr/bin/env pwsh
# delete-action.ps1 — Delete a pending action.
#
# Usage:
#   pwsh -NoProfile -File delete-action.ps1 <action-id>
#
# Only pending actions can be deleted.

param(
  [Parameter(Mandatory = $true, Position = 0)]
  [string]$ActionId
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

try {
  $null = Invoke-WebRequest `
    -Method Delete `
    -Uri "$server/api/actions/$ActionId" `
    -Headers $headers `
    -TimeoutSec 30

  Write-Output "{`"deleted`":true,`"action_id`":$ActionId}"
} catch {
  Write-Error "Error deleting action: $($_.Exception.Message)"
  exit 1
}

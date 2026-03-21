#!/usr/bin/env pwsh
# update-action.ps1 — Update a pending action.
#
# Usage:
#   pwsh -NoProfile -File update-action.ps1 <action-id> '<json-payload>'
#
# Only pending actions can be edited.

param(
  [Parameter(Mandatory = $true, Position = 0)]
  [string]$ActionId,

  [Parameter(Mandatory = $true, Position = 1)]
  [string]$Payload
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
  $response = Invoke-WebRequest `
    -Method Put `
    -Uri "$server/api/actions/$ActionId" `
    -Headers $headers `
    -Body $Payload `
    -TimeoutSec 30

  Write-Output $response.Content
} catch {
  Write-Error "Error updating action: $($_.Exception.Message)"
  exit 1
}

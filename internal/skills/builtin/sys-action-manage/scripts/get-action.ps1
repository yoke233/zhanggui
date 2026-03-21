#!/usr/bin/env pwsh
# get-action.ps1 — Get details of a specific action.
#
# Usage:
#   pwsh -NoProfile -File get-action.ps1 <action-id>

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
  $response = Invoke-WebRequest `
    -Method Get `
    -Uri "$server/api/actions/$ActionId" `
    -Headers $headers `
    -TimeoutSec 30

  Write-Output $response.Content
} catch {
  Write-Error "Error getting action: $($_.Exception.Message)"
  exit 1
}

#!/usr/bin/env pwsh
param(
  [Parameter(Mandatory = $true, Position = 0)]
  [string]$Decision,

  [Parameter(Mandatory = $true, Position = 1)]
  [string]$Reason,

  [Parameter(Mandatory = $false, Position = 2)]
  [string]$MetadataJson
)

$ErrorActionPreference = "Stop"

$validDecisions = @("complete", "need_help", "approve", "reject")
if ($validDecisions -notcontains $Decision) {
  Write-Error "decision must be one of: complete, need_help, approve, reject"
  exit 1
}

$payloadObject = @{
  decision = $Decision
  reason   = $Reason
}
if ($MetadataJson) {
  $extra = ConvertFrom-Json $MetadataJson -AsHashtable
  foreach ($entry in $extra.GetEnumerator()) {
    $payloadObject[$entry.Key] = $entry.Value
  }
}
$payload = $payloadObject | ConvertTo-Json -Compress

$serverAddr = $env:AI_WORKFLOW_SERVER_ADDR
$stepID = $env:AI_WORKFLOW_STEP_ID
$apiToken = $env:AI_WORKFLOW_API_TOKEN

if ($serverAddr -and $stepID -and $apiToken) {
  try {
    $response = Invoke-WebRequest `
      -Method Post `
      -Uri "$serverAddr/api/steps/$stepID/decision" `
      -Headers @{ Authorization = "Bearer $apiToken" } `
      -ContentType "application/json" `
      -Body $payload `
      -TimeoutSec 10

    if ($response.StatusCode -ge 200 -and $response.StatusCode -lt 300) {
      Write-Output "Signal sent via HTTP ($($response.StatusCode)): $Decision"
      exit 0
    }

    Write-Warning "HTTP signal failed ($($response.StatusCode)), falling back to output."
  } catch {
    Write-Warning "HTTP signal failed ($($_.Exception.Message)), falling back to output."
  }
}

Write-Output "AI_WORKFLOW_SIGNAL: $payload"

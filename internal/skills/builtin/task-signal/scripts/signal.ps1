# signal.ps1 — Signal ThreadTask completion or rejection to the AI Workflow engine.
#
# Usage:
#   pwsh -NoProfile -File signal.ps1 <action> <output_file> [feedback]
#
# Actions: complete | reject

param(
    [Parameter(Mandatory=$true, Position=0)]
    [ValidateSet("complete", "reject")]
    [string]$Action,

    [Parameter(Mandatory=$true, Position=1)]
    [string]$OutputFile,

    [Parameter(Mandatory=$false, Position=2)]
    [string]$Feedback = ""
)

$ErrorActionPreference = "Stop"

$payload = @{
    action          = $Action
    output_file_path = $OutputFile
    feedback        = $Feedback
} | ConvertTo-Json -Compress

$serverAddr = $env:AI_WORKFLOW_SERVER_ADDR
$taskId     = $env:AI_WORKFLOW_TASK_ID
$token      = $env:AI_WORKFLOW_API_TOKEN

if ($serverAddr -and $taskId -and $token) {
    try {
        $uri = "${serverAddr}/api/v1/thread-tasks/${taskId}/signal"
        $headers = @{
            "Authorization" = "Bearer $token"
            "Content-Type"  = "application/json"
        }
        $response = Invoke-RestMethod -Uri $uri -Method Post -Headers $headers -Body $payload -ErrorAction Stop
        Write-Host "Signal sent via HTTP: $Action"
        exit 0
    } catch {
        Write-Warning "HTTP signal failed: $_. Falling back to output."
    }
}

# Fallback: output the signal line for engine to parse.
Write-Output "AI_WORKFLOW_TASK_SIGNAL: $payload"

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

param(
  [string]$Config = 'configs/config.yaml',
  [string]$Workflow = 'workflow.toml',
  [string]$Role = 'backend',
  [string]$Assignee = '',
  [int]$EventBatch = 200,
  [string]$WorkdirCleanup = 'manual',
  [switch]$NoCache
)

function Invoke-Logged {
  param(
    [Parameter(Mandatory = $true)][string]$Name,
    [Parameter(Mandatory = $true)][string]$FilePath,
    [Parameter(Mandatory = $true)][scriptblock]$Command
  )

  Write-Host "==> $Name"
  $out = & $Command 2>&1 | Tee-Object -FilePath $FilePath
  $exitCode = $LASTEXITCODE
  if ($exitCode -ne 0) {
    throw "$Name failed with exit code $exitCode. See log: $FilePath"
  }
  return ,$out
}

$repoRoot = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..')).Path
Push-Location -LiteralPath $repoRoot
try {
  $ts = Get-Date -Format 'yyyyMMdd-HHmmss'
  $runLogDir = Join-Path $repoRoot (Join-Path 'state\\e2e' ("phase26-full-$ts"))
  New-Item -ItemType Directory -Force -Path $runLogDir | Out-Null

  $configPath = (Resolve-Path -LiteralPath $Config).Path
  $workflowPath = ''

  $normalizedCleanup = $WorkdirCleanup.Trim().ToLowerInvariant()
  if ([string]::IsNullOrWhiteSpace($normalizedCleanup)) {
    $normalizedCleanup = 'manual'
  }
  if ($normalizedCleanup -notin @('manual', 'immediate')) {
    throw "invalid -WorkdirCleanup '$WorkdirCleanup' (allowed: manual|immediate)"
  }

  $executorArgs = @('test', './...')
  if ($NoCache) {
    $executorArgs += '-count=1'
  }
  $executorArgsRaw = ($executorArgs | ConvertTo-Json -Compress)

  $workdirRoot = (Join-Path $repoRoot '.worktrees\\runs')
  $e2eWorkflowPath = Join-Path $runLogDir 'workflow.e2e.toml'
  @"
version = 1

[outbox]
backend = "sqlite"
path = "state/outbox.sqlite"

[workdir]
enabled = true
backend = "git-worktree"
root = '$workdirRoot'
cleanup = "$normalizedCleanup"
roles = ["backend"]

[roles]
enabled = ["backend"]

[repos]
main = '$repoRoot'

[role_repo]
backend = "main"

[groups.backend]
role = "backend"
max_concurrent = 1
listen_labels = ["to:backend"]

[executors.backend]
program = "go"
args = $executorArgsRaw
timeout_seconds = 1800
"@ | Set-Content -LiteralPath $e2eWorkflowPath -Encoding UTF8
  $workflowPath = $e2eWorkflowPath

  if ([string]::IsNullOrWhiteSpace($Assignee)) {
    $Assignee = "lead-$Role-e2e-$ts"
  }

  # NOTE: 该脚本默认生成一个独立的 workflow.e2e.toml（放到本次 run 的日志目录下），
  # 用于强制启用 workdir 并控制 cleanup 策略（manual/immediate）。

  Invoke-Logged -Name 'init-db' -FilePath (Join-Path $runLogDir 'init-db.log') -Command {
    go run . --config $configPath init-db
  } | Out-Null

  $createOut = Invoke-Logged -Name 'outbox create' -FilePath (Join-Path $runLogDir 'outbox-create.log') -Command {
    go run . --config $configPath outbox create `
      --title 'e2e phase2.6 full worktree run' `
      --body 'body' `
      --label "to:$Role" `
      --label 'state:todo'
  }
  $createLine = ($createOut | Select-Object -Last 1)
  if ($createLine -notmatch 'created issue:\s+(\S+)') {
    throw "failed to parse issue ref from output: $createLine"
  }
  $issueRef = $Matches[1]

  Invoke-Logged -Name 'outbox claim' -FilePath (Join-Path $runLogDir 'outbox-claim.log') -Command {
    go run . --config $configPath outbox claim `
      --issue $issueRef `
      --assignee $Assignee `
      --actor $Assignee
  } | Out-Null

  $leadOut = Invoke-Logged -Name 'lead run --once' -FilePath (Join-Path $runLogDir 'lead.log') -Command {
    go run . --config $configPath lead run `
      --once `
      --role $Role `
      --assignee $Assignee `
      --workflow $workflowPath `
      --event-batch $EventBatch
  }

  $sanIssue = $issueRef.Replace('/', '_').Replace('#', '_').Replace(':', '_').Replace('\', '_')
  $packsRoot = Join-Path $repoRoot (Join-Path 'state\\context_packs' $sanIssue)
  if (!(Test-Path -LiteralPath $packsRoot)) {
    throw "context packs root not found: $packsRoot"
  }

  $runDir = Get-ChildItem -LiteralPath $packsRoot -Directory | Sort-Object Name | Select-Object -Last 1
  if ($null -eq $runDir) {
    throw "no run directory found under: $packsRoot"
  }
  $runID = $runDir.Name
  $contextPackDir = $runDir.FullName

  $orderPath = Join-Path $contextPackDir 'work_order.json'
  $order = Get-Content -LiteralPath $orderPath -Raw | ConvertFrom-Json
  $workdir = [string]$order.RepoDir

  $stdoutPath = Join-Path $contextPackDir 'stdout.log'
  $stderrPath = Join-Path $contextPackDir 'stderr.log'
  $resultJSONPath = Join-Path $contextPackDir 'work_result.json'
  $resultTextPath = Join-Path $contextPackDir 'work_result.txt'

  $workdirExists = Test-Path -LiteralPath $workdir
  $wtList = git -C $repoRoot worktree list
  $wtHas = ($wtList | Select-String -SimpleMatch $workdir) -ne $null

  Invoke-Logged -Name 'outbox show' -FilePath (Join-Path $runLogDir 'outbox-show.log') -Command {
    go run . --config $configPath outbox show --issue $issueRef
  } | Out-Null

  Write-Host ''
  Write-Host '==== E2E Summary ===='
  Write-Host "RepoRoot:        $repoRoot"
  Write-Host "Config:          $configPath"
  Write-Host "Workflow:        $workflowPath"
  Write-Host "IssueRef:        $issueRef"
  Write-Host "Assignee:        $Assignee"
  Write-Host "RunID:           $runID"
  Write-Host "ContextPackDir:  $contextPackDir"
  Write-Host "Workdir(RepoDir): $workdir"
  Write-Host "WorkdirExists:   $workdirExists"
  Write-Host "GitWorktreeHas:  $wtHas"
  Write-Host ''
  Write-Host '==== Logs (重要) ===='
  Write-Host "Wrapper logs dir: $runLogDir"
  Write-Host "Context pack stdout: $stdoutPath"
  Write-Host "Context pack stderr: $stderrPath"
  Write-Host "Work result json: $resultJSONPath"
  Write-Host "Work result text: $resultTextPath"
  Write-Host ''
  Write-Host '==== Tail(stdout.log) ===='
  if (Test-Path -LiteralPath $stdoutPath) {
    Get-Content -LiteralPath $stdoutPath | Select-Object -Last 60
  } else {
    Write-Host "(missing) $stdoutPath"
  }

  Write-Host ''
  Write-Host '==== Tail(stderr.log) ===='
  if (Test-Path -LiteralPath $stderrPath) {
    Get-Content -LiteralPath $stderrPath | Select-Object -Last 60
  } else {
    Write-Host "(missing) $stderrPath"
  }

  Write-Host ''
  Write-Host '提示：如果出现 blocked(workdir-cleanup)，对应 workdir 通常会保留在 .worktrees\\runs\\... 下，方便你手工检查/清理。'
  Write-Host '提示：Lead/FX/slog 的输出在 wrapper log：state\\e2e\\phase26-full-<ts>\\lead.log'
  Write-Host ''
  Write-Host '==== 后续操作建议 ===='
  Write-Host "1) 打开 TUI 查看过程/结果："
  Write-Host "   go run . console pm --workflow $workflowPath --refresh-interval 2s"
  Write-Host '2) 检查完成后：在 TUI 中选中对应 issue，按 `D` 清理 workdir（worktree）。'
  Write-Host '3) 若你想删除本次 e2e 的 wrapper 日志目录（可选）：'
  Write-Host "   Remove-Item -Recurse -Force -LiteralPath $runLogDir"
  Write-Host '4) 若你想删除本次 e2e 的 context-pack 目录（可选）：'
  Write-Host "   Remove-Item -Recurse -Force -LiteralPath $contextPackDir"
} finally {
  Pop-Location
}

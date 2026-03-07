<#
.SYNOPSIS
    构建并推送 ai-workflow Docker 镜像
.DESCRIPTION
    支持 dev / prod 两种构建目标，自动打时间标签并推送到阿里云 ACR。
.PARAMETER Target
    构建目标: dev (默认, 含 Go + Node 完整开发环境) 或 prod (仅运行时)
.PARAMETER Push
    是否推送到远程仓库 (默认不推送)
.PARAMETER Registry
    镜像仓库前缀
.EXAMPLE
    .\scripts\docker-build.ps1                         # 仅本地构建 prod
    .\scripts\docker-build.ps1 -Push                   # 构建 prod 并推送
    .\scripts\docker-build.ps1 -Target dev -Push       # 构建 dev 并推送
#>
param(
    [ValidateSet("dev", "prod")]
    [string]$Target = "prod",
    [switch]$Push,
    [string]$Registry = "registry.cn-shanghai.aliyuncs.com/xiaoin"
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

. (Join-Path $PSScriptRoot "test\common.ps1")
# Enter-RepoRoot expects scripts two levels deep (scripts/test/); we are one level (scripts/).
$repoRoot = Resolve-Path -LiteralPath (Join-Path $PSScriptRoot "..")
Set-Location -LiteralPath $repoRoot

# ── 变量 ──
$projectName = "ai-workflow"
$tag = Get-Date -Format "yyyyMMdd-HHmm"
$commitShort = git rev-parse --short HEAD 2>$null
if (-not $commitShort) { $commitShort = "unknown" }
$imageTag = "${tag}-${commitShort}"

$dockerfileMap = @{
    dev  = "Dockerfile.dev"
    prod = "Dockerfile"
}
$dockerfile = $dockerfileMap[$Target]

if ($Target -eq "dev" -and -not (Test-Path $dockerfile) -and (Test-Path "Dockerfile")) {
    Write-Host "Dockerfile.dev not found, fallback to Dockerfile for dev build." -ForegroundColor Yellow
    $dockerfile = "Dockerfile"
}

if (-not (Test-Path $dockerfile)) {
    Write-Host "Dockerfile not found: $dockerfile" -ForegroundColor Red
    exit 1
}

$localImage = "${projectName}:${Target}"
$remoteBase = "${Registry}/${projectName}"

# ── 构建 ──
Invoke-Step "Docker Build ($Target)" {
    docker build -f $dockerfile -t $localImage . --progress=plain
} -CheckLastExitCode

Write-Host ""
Write-Host "Local image: $localImage" -ForegroundColor Cyan
Write-Host "Commit:      $commitShort" -ForegroundColor Cyan

# ── 推送 ──
if ($Push) {
    $remoteTagged = "${remoteBase}:${imageTag}"
    $remoteLatest = "${remoteBase}:${Target}-latest"

    Invoke-Step "Tag images" {
        docker tag $localImage $remoteTagged
        docker tag $localImage $remoteLatest
    }

    Invoke-Step "Push $remoteTagged" {
        docker push $remoteTagged
    } -CheckLastExitCode

    Invoke-Step "Push $remoteLatest" {
        docker push $remoteLatest
    } -CheckLastExitCode

    Write-Host ""
    Write-Host "Pushed:" -ForegroundColor Green
    Write-Host "  $remoteTagged"
    Write-Host "  $remoteLatest"
} else {
    Write-Host ""
    Write-Host "跳过推送 (加 -Push 参数启用)" -ForegroundColor Yellow
}

Write-Host ""
Write-Host "Done! tag=$imageTag target=$Target" -ForegroundColor Green

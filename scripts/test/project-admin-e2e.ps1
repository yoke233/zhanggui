param(
  [string]$AppUrl = "http://localhost:5173",
  [switch]$Headed
)

$ErrorActionPreference = "Stop"
$scriptRoot = Split-Path -Parent $MyInvocation.MyCommand.Path
$repoRoot = Resolve-Path (Join-Path $scriptRoot "..\\..")
Set-Location -LiteralPath $repoRoot

$env:APP_URL = $AppUrl

$args = @(
  "-y",
  "@playwright/test",
  "test",
  "scripts/test/project-admin.e2e.spec.ts",
  "--workers=1",
  "--reporter=line"
)

if ($Headed) {
  $args += "--headed"
}

Write-Host "[e2e] running playwright project-admin flow..."
Write-Host "[e2e] APP_URL=$env:APP_URL"

& npx @args
if ($LASTEXITCODE -ne 0) {
  throw "Playwright e2e failed with exit code $LASTEXITCODE"
}

Write-Host "[e2e] completed successfully."

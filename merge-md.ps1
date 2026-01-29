[CmdletBinding()]
param(
  [Parameter()]
  [string]$Root = (Get-Location).Path,

  [Parameter()]
  [string]$OutputFile = 'merged.md',

  [Parameter()]
  [string[]]$ExcludeDirectoryNames = @('.git', 'fs', 'node_modules', 'dist', 'build', '.next', 'out', 'coverage', 'bin', 'obj'),

  [Parameter()]
  [switch]$Force
)

$rootPath = (Resolve-Path -LiteralPath $Root).Path

$outPath = $OutputFile
if (-not [System.IO.Path]::IsPathRooted($outPath)) {
  $outPath = Join-Path -Path $rootPath -ChildPath $outPath
}
$outPath = [System.IO.Path]::GetFullPath($outPath)

if ((Test-Path -LiteralPath $outPath) -and -not $Force) {
  throw "输出文件已存在：$outPath。用 -Force 覆盖，或改 -OutputFile。"
}

$files = Get-ChildItem -LiteralPath $rootPath -Recurse -File -Filter '*.md' -Force

$dirSep = [System.IO.Path]::DirectorySeparatorChar
$excludedSegments = $ExcludeDirectoryNames | ForEach-Object { "$dirSep$_$dirSep" }

$files = $files | Where-Object {
  $full = [System.IO.Path]::GetFullPath($_.FullName)
  if ($full -eq $outPath) { return $false }
  foreach ($seg in $excludedSegments) {
    if ($full -like "*$seg*") { return $false }
  }
  return $true
}

if (-not $files) {
  throw "在 $rootPath 下没有找到任何 .md 文件。"
}

$items = foreach ($f in $files) {
  $relRaw = [System.IO.Path]::GetRelativePath($rootPath, $f.FullName)
  $rel = $relRaw.Replace('\', '/')
  [pscustomobject]@{
    File      = $f
    Rel       = $rel
    SortGroup = if ($relRaw -ieq 'README.md') { 0 } elseif ($relRaw -notmatch '[\\/]' ) { 1 } else { 2 }
  }
}

$items = @($items | Sort-Object SortGroup, Rel)

$sb = [System.Text.StringBuilder]::new()
$now = Get-Date
$null = $sb.AppendLine('# 合并的 Markdown')
$null = $sb.AppendLine()
$null = $sb.AppendLine("> 生成时间: $($now.ToString('yyyy-MM-dd HH:mm:ss K'))")
$null = $sb.AppendLine("> Root: $rootPath")
$null = $sb.AppendLine()

foreach ($it in $items) {
  $rel = $it.Rel
  $path = $it.File.FullName

  $null = $sb.AppendLine('---')
  $null = $sb.AppendLine()
  $null = $sb.AppendLine("## 文件名：$rel")
  $null = $sb.AppendLine()

  $content = Get-Content -LiteralPath $path -Raw
  $null = $sb.Append($content)
  if (-not $content.EndsWith("`n")) {
    $null = $sb.AppendLine()
  }
  $null = $sb.AppendLine()
}

$outDir = Split-Path -Parent $outPath
if ($outDir -and -not (Test-Path -LiteralPath $outDir)) {
  New-Item -ItemType Directory -Path $outDir | Out-Null
}

[System.IO.File]::WriteAllText($outPath, $sb.ToString(), [System.Text.UTF8Encoding]::new($false))

Write-Host "已生成: $outPath"
Write-Host ("包含文件数: {0}" -f $items.Count)
Write-Host ("输出大小: {0:n0} bytes" -f ((Get-Item -LiteralPath $outPath).Length))

[CmdletBinding()]
param(
    # Keep the version filesystem-safe: it becomes both a folder and archive name.
    [string]$Version = (Get-Date -Format 'yyyyMMdd-HHmmss'),
    [switch]$KeepDirectory
)

$ErrorActionPreference = 'Stop'

if ($Version -notmatch '^[A-Za-z0-9][A-Za-z0-9._-]*$') {
    throw 'Version 只能包含字母、数字、点、下划线和连字符。'
}

$projectRoot = Split-Path -Parent $PSScriptRoot
$frontendRoot = Join-Path $projectRoot 'frontend'
$frontendDist = Join-Path $frontendRoot 'dist'
$templatesRoot = Join-Path $projectRoot 'templates'
$packagingRoot = Join-Path $projectRoot 'packaging'
$releaseRoot = Join-Path $projectRoot 'release'
$packageName = "LocalDream-windows-amd64-$Version"
$stage = Join-Path $releaseRoot $packageName
$archive = Join-Path $releaseRoot "$packageName.zip"

foreach ($path in @($frontendRoot, $templatesRoot, $packagingRoot)) {
    if (-not (Test-Path -LiteralPath $path -PathType Container)) {
        throw "缺少发布所需目录：$path"
    }
}

New-Item -ItemType Directory -Path $releaseRoot -Force | Out-Null
# Only remove the exact staging directory for this requested version; other releases stay intact.
if (Test-Path -LiteralPath $stage) {
    Remove-Item -LiteralPath $stage -Recurse -Force
}
if (Test-Path -LiteralPath $archive) {
    Remove-Item -LiteralPath $archive -Force
}
New-Item -ItemType Directory -Path $stage | Out-Null

Push-Location $frontendRoot
try {
    & npm.cmd run build
    if ($LASTEXITCODE -ne 0) {
        throw "前端构建失败（退出码 $LASTEXITCODE）。"
    }
}
finally {
    Pop-Location
}

if (-not (Test-Path -LiteralPath $frontendDist -PathType Container)) {
    throw "前端构建后未找到：$frontendDist"
}

# modernc.org/sqlite is pure Go. Disabling CGO makes the executable self-contained
# with respect to the Go runtime and avoids a C toolchain dependency at build time.
$previousCGO = $env:CGO_ENABLED
$env:CGO_ENABLED = '0'
try {
    Push-Location $projectRoot
    try {
        & go build -trimpath -ldflags '-s -w' -o (Join-Path $stage 'localdream.exe') .
        if ($LASTEXITCODE -ne 0) {
            throw "Go 后端构建失败（退出码 $LASTEXITCODE）。"
        }
    }
    finally {
        Pop-Location
    }
}
finally {
    $env:CGO_ENABLED = $previousCGO
}

New-Item -ItemType Directory -Path (Join-Path $stage 'frontend') | Out-Null
Copy-Item -LiteralPath $frontendDist -Destination (Join-Path $stage 'frontend') -Recurse
Copy-Item -LiteralPath $templatesRoot -Destination $stage -Recurse
Copy-Item -LiteralPath (Join-Path $packagingRoot 'start-localdream.cmd') -Destination $stage
Copy-Item -LiteralPath (Join-Path $packagingRoot 'config.example.cmd') -Destination $stage
Copy-Item -LiteralPath (Join-Path $packagingRoot '发布说明.md') -Destination $stage

# Intentionally do not copy data/. It can contain SQLite databases, WAL/SHM files,
# uploaded assets, provider credentials, and ComfyUI logs. The program creates fresh
# data/assets and a fresh empty SQLite database upon its first launch.
$forbidden = @(
    '^data/',
    '(^|/)localdream\\.sqlite($|[-.])',
    '(^|/)provider\\.json$'
)
$stageFiles = Get-ChildItem -LiteralPath $stage -File -Recurse
foreach ($file in $stageFiles) {
    $relative = $file.FullName.Substring($stage.Length + 1).Replace('\', '/')
    if ($forbidden | Where-Object { $relative -match $_ }) {
        throw "发布目录包含禁止的运行时数据：$relative"
    }
}

Compress-Archive -LiteralPath $stage -DestinationPath $archive -CompressionLevel Optimal

Add-Type -AssemblyName System.IO.Compression.FileSystem
$zip = [System.IO.Compression.ZipFile]::OpenRead($archive)
try {
    $entries = @($zip.Entries | ForEach-Object FullName)
    $required = @(
        "$packageName/localdream.exe",
        "$packageName/start-localdream.cmd",
        "$packageName/config.example.cmd",
        "$packageName/发布说明.md",
        "$packageName/frontend/dist/index.html",
        "$packageName/templates/h3/workflow.json",
        "$packageName/templates/h3/manifest.json"
    )
    $missing = @($required | Where-Object { $_ -notin $entries })
    if ($missing.Count -ne 0) {
        throw "ZIP 缺少发布文件：$($missing -join ', ')"
    }
    foreach ($entry in $zip.Entries) {
        if ($forbidden | Where-Object { $entry.FullName -match $_ }) {
            throw "ZIP 包含禁止的运行时数据：$($entry.FullName)"
        }
    }
}
finally {
    $zip.Dispose()
}

if (-not $KeepDirectory) {
    Remove-Item -LiteralPath $stage -Recurse -Force
}

[PSCustomObject]@{
    Package = $archive
    SizeMB = [math]::Round((Get-Item -LiteralPath $archive).Length / 1MB, 2)
    StagingDirectoryKept = [bool]$KeepDirectory
    DatabaseIncluded = $false
}

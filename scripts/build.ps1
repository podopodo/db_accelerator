param(
    [string]$Version = "dev",
    [string]$Commit = "unknown",
    [string]$BuildDate = "unknown",
    [string]$Output = "bin\accelerator.exe"
)

$ErrorActionPreference = "Stop"
$projectRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$buildCache = Join-Path $projectRoot "tmp\go-build"
$goTemp = Join-Path $projectRoot "tmp\go-tmp"
$moduleCache = Join-Path ([System.IO.Path]::GetTempPath()) "databaseaccelerator-go-mod"
New-Item -ItemType Directory -Force -Path $buildCache, $goTemp, $moduleCache | Out-Null
$env:GOCACHE = $buildCache
$env:GOTMPDIR = $goTemp
$env:GOMODCACHE = $moduleCache

$outputDirectory = Split-Path -Parent $Output
if ($outputDirectory) {
    New-Item -ItemType Directory -Force -Path $outputDirectory | Out-Null
}

$linkFlags = "-s -w -X github.com/podopodo/db_accelerator/internal/buildinfo.Version=$Version -X github.com/podopodo/db_accelerator/internal/buildinfo.Commit=$Commit -X github.com/podopodo/db_accelerator/internal/buildinfo.BuildDate=$BuildDate"
Push-Location $projectRoot
try {
    go build -trimpath -ldflags $linkFlags -o $Output ./cmd/accelerator
} finally {
    Pop-Location
}
if ($LASTEXITCODE -ne 0) {
    throw "go build failed with exit code $LASTEXITCODE"
}

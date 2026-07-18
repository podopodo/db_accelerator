$ErrorActionPreference = "Stop"
$projectRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$buildCache = Join-Path $projectRoot "tmp\go-build"
$goTemp = Join-Path $projectRoot "tmp\go-tmp"
$moduleCache = Join-Path ([System.IO.Path]::GetTempPath()) "databaseaccelerator-go-mod"
New-Item -ItemType Directory -Force -Path $buildCache, $goTemp, $moduleCache | Out-Null
$env:GOCACHE = $buildCache
$env:GOTMPDIR = $goTemp
$env:GOMODCACHE = $moduleCache

Push-Location $projectRoot
try {
    go generate ./...
    if ($LASTEXITCODE -ne 0) { throw "go generate failed with exit code $LASTEXITCODE" }
    git diff --exit-code -- . ':!plans/**'
    if ($LASTEXITCODE -ne 0) { throw "generated files are out of date" }
} finally {
    Pop-Location
}

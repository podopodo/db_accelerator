param(
    [switch]$SkipRace
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

$unformatted = @(gofmt -l (Join-Path $projectRoot "cmd") (Join-Path $projectRoot "internal"))
if ($unformatted.Count -gt 0) {
    throw "Go files require formatting: $($unformatted -join ', ')"
}

Push-Location $projectRoot
try {
    go vet ./...
    if ($LASTEXITCODE -ne 0) { throw "go vet failed with exit code $LASTEXITCODE" }
    go test '-coverprofile=coverage.txt' ./...
    if ($LASTEXITCODE -ne 0) { throw "go test failed with exit code $LASTEXITCODE" }
    if (-not $SkipRace) {
        go test -race ./...
        if ($LASTEXITCODE -ne 0) { throw "go test -race failed with exit code $LASTEXITCODE" }
    }
    & (Join-Path $PSScriptRoot "check-dependencies.ps1")
    if ($LASTEXITCODE -ne 0) { throw "dependency check failed with exit code $LASTEXITCODE" }
    go build -trimpath -o "tmp\accelerator-check.exe" ./cmd/accelerator
    if ($LASTEXITCODE -ne 0) { throw "go build failed with exit code $LASTEXITCODE" }
} finally {
    Pop-Location
}

$ErrorActionPreference = "Stop"
$projectRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$allowPath = Join-Path $projectRoot "build\dependencies.allow"
$buildCache = Join-Path $projectRoot "tmp\go-build"
$goTemp = Join-Path $projectRoot "tmp\go-tmp"
$moduleCache = Join-Path ([System.IO.Path]::GetTempPath()) "databaseaccelerator-go-mod"
New-Item -ItemType Directory -Force -Path $buildCache, $goTemp, $moduleCache | Out-Null
$env:GOCACHE = $buildCache
$env:GOTMPDIR = $goTemp
$env:GOMODCACHE = $moduleCache
$allowed = @(Get-Content -LiteralPath $allowPath | Where-Object { $_ -and -not $_.StartsWith("#") } | Sort-Object -Unique)

Push-Location $projectRoot
try {
    $actual = @(go list -m -f '{{if not .Main}}{{.Path}}{{end}}' all | Where-Object { $_ } | Sort-Object -Unique)
    if ($LASTEXITCODE -ne 0) {
        throw "go list failed with exit code $LASTEXITCODE"
    }
} finally {
    Pop-Location
}

$difference = @(Compare-Object -ReferenceObject $allowed -DifferenceObject $actual)
if ($difference.Count -gt 0) {
    $formatted = $difference | ForEach-Object { "$($_.SideIndicator) $($_.InputObject)" }
    throw "Dependency allowlist mismatch: $($formatted -join '; ')"
}

Write-Output "Dependency allowlist matches $($actual.Count) external modules."

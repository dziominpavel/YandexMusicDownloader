#Requires -Version 5.1
# Builds the distributable single-file Windows exe.
# Wails REQUIRES the desktop,production build tags — plain `go build`
# produces an exe that only shows an error dialog. Always build via this script.
#
# The script also generates a temporary Windows resource object (.syso) with
# the application icon, version metadata and a PerMonitorV2 DPI manifest.
$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot

$versionFile = Join-Path $root 'version'
if (-not (Test-Path $versionFile)) { throw "missing version file: $versionFile" }
$version = (Get-Content $versionFile -Raw).Trim()
if ($version -notmatch '^\d+\.\d+\.\d+$') {
    throw "bad version '$version' (expected MAJOR.MINOR.PATCH)"
}

$winresDir = Join-Path $root 'assets\winres'
$winresTemplate = Join-Path $winresDir 'winres.json'
$winresTmp = Join-Path $winresDir '.winres-build.json'
$syso = Join-Path $root 'src\cmd\app\rsrc_windows_amd64.syso'
$version4 = "$version.0"

if (-not (Test-Path $winresTemplate)) { throw "missing winres config: $winresTemplate" }

# The checked-in config has placeholders so the version never drifts from the
# `version` file. The temporary copy stays next to the icons so relative paths work.
$json = Get-Content $winresTemplate -Raw
$json = $json.Replace('__VERSION_4__', $version4).Replace('__VERSION__', $version)
$utf8NoBom = New-Object System.Text.UTF8Encoding($false)
[System.IO.File]::WriteAllText($winresTmp, $json, $utf8NoBom)

try {
    Push-Location $root
    try {
        go run github.com/tc-hib/go-winres@v0.3.3 make `
            --in $winresTmp `
            --out (Join-Path $root 'src\cmd\app\rsrc') `
            --arch amd64
        if ($LASTEXITCODE -ne 0) { throw "go-winres failed with exit code $LASTEXITCODE" }

        Push-Location (Join-Path $root 'src')
        try {
            go build -tags 'desktop,production' -ldflags '-H=windowsgui' -o (Join-Path $root 'app.exe') ./cmd/app
            if ($LASTEXITCODE -ne 0) { throw "go build failed with exit code $LASTEXITCODE" }
        } finally {
            Pop-Location
        }
    } finally {
        Pop-Location
    }
    'built: ' + (Join-Path $root 'app.exe')
} finally {
    Remove-Item $syso -Force -ErrorAction SilentlyContinue
    Remove-Item $winresTmp -Force -ErrorAction SilentlyContinue
}

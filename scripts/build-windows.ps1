#Requires -Version 5.1
# Builds the distributable single-file Windows exe.
# Wails REQUIRES the desktop,production build tags — plain `go build`
# produces an exe that only shows an error dialog. Always build via this script.
$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
Push-Location (Join-Path $root 'src')
try {
    go build -tags 'desktop,production' -ldflags '-H=windowsgui' -o (Join-Path $root 'app.exe') ./cmd/app
    'built: ' + (Join-Path $root 'app.exe')
} finally {
    Pop-Location
}

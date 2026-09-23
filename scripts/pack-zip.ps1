#Requires -Version 5.1
# Packs the distributable zip for the store: app.exe + ffmpeg.exe
#   -> dist/YandexMusicDownloader-<version>-win-x64.zip
# ffmpeg.exe is intentionally NOT in git (sidecar, see README); place it in the
# repo root before release. Fails with a clear error when anything is missing.
$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot

$versionFile = Join-Path $root 'version'
if (-not (Test-Path $versionFile)) { Write-Host "[ERROR] missing version file: $versionFile"; exit 1 }
$version = (Get-Content $versionFile -Raw).Trim()
if ($version -notmatch '^\d+\.\d+\.\d+$') { Write-Host "[ERROR] bad version '$version' (expected MAJOR.MINOR.PATCH)"; exit 1 }

$appExe = Join-Path $root 'app.exe'
if (-not (Test-Path $appExe)) {
    Write-Host "[ERROR] app.exe not found - run scripts\build-windows.ps1 first."
    exit 1
}
$ffmpeg = Join-Path $root 'ffmpeg.exe'
if (-not (Test-Path $ffmpeg)) {
    Write-Host "[ERROR] ffmpeg.exe not found in repo root (sidecar, not in git; copy it locally before release)."
    exit 1
}

$dist = Join-Path $root 'dist'
New-Item -ItemType Directory -Force -Path $dist | Out-Null
$zip = Join-Path $dist "YandexMusicDownloader-$version-win-x64.zip"
if (Test-Path $zip) { Remove-Item $zip -Force }

Compress-Archive -Path $appExe, $ffmpeg -DestinationPath $zip
$size = [math]::Round((Get-Item $zip).Length / 1MB, 1)
Write-Host "packed: $zip ($size MB)"

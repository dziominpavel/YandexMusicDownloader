#Requires -Version 5.1
# Packs the distributable zip for the store: app.exe + ffmpeg.exe
#   -> dist/YandexMusicDownloader-<version>-win-x64.zip
# Both files live under the single root directory YandexMusicDownloader/.
# The archive structure is verified after packing.
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

Add-Type -AssemblyName System.IO.Compression
Add-Type -AssemblyName System.IO.Compression.FileSystem

$entries = @(
    @{ Path = $appExe; Name = 'YandexMusicDownloader/app.exe' },
    @{ Path = $ffmpeg; Name = 'YandexMusicDownloader/ffmpeg.exe' }
)
$archive = $null
$packError = $null
try {
    $archive = [System.IO.Compression.ZipFile]::Open($zip, [System.IO.Compression.ZipArchiveMode]::Create)
    foreach ($entry in $entries) {
        $zipEntry = $archive.CreateEntry($entry.Name, [System.IO.Compression.CompressionLevel]::Optimal)
        $source = [System.IO.File]::OpenRead($entry.Path)
        try {
            $target = $zipEntry.Open()
            try {
                $source.CopyTo($target)
            } finally {
                $target.Dispose()
            }
        } finally {
            $source.Dispose()
        }
    }
} catch {
    $packError = $_.Exception.Message
} finally {
    if ($archive) { $archive.Dispose() }
}

if ($packError) {
    if (Test-Path $zip) { Remove-Item $zip -Force -ErrorAction SilentlyContinue }
    Write-Host "[ERROR] failed to pack zip: $packError"
    exit 1
}

$expected = @(
    'YandexMusicDownloader/app.exe',
    'YandexMusicDownloader/ffmpeg.exe'
) | Sort-Object

$actual = @()
$checkError = $null
$readArchive = $null
try {
    $readArchive = [System.IO.Compression.ZipFile]::OpenRead($zip)
    $actual = @($readArchive.Entries | ForEach-Object { $_.FullName } | Sort-Object)
} catch {
    $checkError = $_.Exception.Message
} finally {
    if ($readArchive) { $readArchive.Dispose() }
}

if ($checkError) {
    if (Test-Path $zip) { Remove-Item $zip -Force -ErrorAction SilentlyContinue }
    Write-Host "[ERROR] failed to verify zip: $checkError"
    exit 1
}

$diff = @(Compare-Object -ReferenceObject $expected -DifferenceObject $actual)
if ($diff.Count -gt 0 -or $actual.Count -ne $expected.Count) {
    $actualText = if ($actual.Count) { $actual -join ', ' } else { '<empty>' }
    Write-Host "[ERROR] zip structure mismatch: expected YandexMusicDownloader/app.exe and YandexMusicDownloader/ffmpeg.exe; found $actualText"
    Remove-Item $zip -Force -ErrorAction SilentlyContinue
    exit 1
}

$size = [math]::Round((Get-Item $zip).Length / 1MB, 1)
Write-Host "packed: $zip ($size MB)"

<#
.SYNOPSIS
    Try Rearview in a few minutes on three 20-second sample clips: no dashcam, GPU or build needed.

.DESCRIPTION
    Downloads the models (once, about 615 MB) and the sample clips, imports the clips into a
    scratch library, runs detection, tracking, distance and time-to-collision on them, indexes
    them for plain-English search, runs a search, and writes a video with what the engine sees
    drawn on it. Everything goes into -Demo (default ~/rearview-demo); delete it to start over.

    Works from the release folder (rearview.exe and engine.exe next to this script) or from a
    repo checkout where both are built. Needs ffmpeg and ffprobe on PATH.

.EXAMPLE
    .\quickstart.ps1
    .\quickstart.ps1 -Query "a white truck" -Device dml
#>
param(
    [string]$Demo = (Join-Path $HOME 'rearview-demo'),
    [string]$Query = 'a traffic light',
    [ValidateSet('cpu', 'dml')][string]$Device = 'cpu',
    [string]$Rearview,
    [string]$Engine,
    [string]$SamplesUrl = 'https://github.com/ousmanebarry/rearview/releases/latest/download/rearview-samples.zip'
)
$ErrorActionPreference = 'Stop'
$here = $PSScriptRoot

function Find-First([string[]]$Paths, [string]$Command) {
    foreach ($p in $Paths) { if ($p -and (Test-Path -PathType Leaf $p)) { return (Resolve-Path $p).Path } }
    $c = Get-Command $Command -ErrorAction SilentlyContinue
    if ($c) { return $c.Source }
    return $null
}

function Step([string]$Title) { Write-Host "`n== $Title" -ForegroundColor Cyan }

function Run([string]$Exe) {
    & $Exe @args
    if ($LASTEXITCODE -ne 0) { throw "$(Split-Path -Leaf $Exe) $($args -join ' ') failed (exit $LASTEXITCODE)" }
}

$Rearview = Find-First @($Rearview, "$here\rearview.exe", "$here\..\go\bin\rearview.exe") 'rearview'
$Engine = Find-First @($Engine, "$here\engine.exe",
    "$here\..\engine\build\dev-dml\RelWithDebInfo\engine.exe",
    "$here\..\engine\build\dev\RelWithDebInfo\engine.exe") 'engine'
if (-not $Rearview) { throw 'rearview.exe not found: run this from the release folder, or build go/ (see README) or pass -Rearview' }
if (-not $Engine) { throw 'engine.exe not found: run this from the release folder, or build engine/ (see README) or pass -Engine' }
foreach ($tool in 'ffmpeg', 'ffprobe') {
    if (-not (Get-Command $tool -ErrorAction SilentlyContinue)) {
        throw "$tool is not on PATH. Install it with: winget install Gyan.FFmpeg  (then open a new terminal)"
    }
}
Write-Host "rearview  $Rearview"
Write-Host "engine    $Engine"
Write-Host "demo      $Demo"

New-Item -ItemType Directory -Force $Demo | Out-Null
$fwd = { param($p) $p -replace '\\', '/' }
$cfg = Join-Path $Demo 'config.yaml'
@"
# Written by quickstart.ps1: a scratch library for the sample clips.
library_path: '$(& $fwd "$Demo\library")'
database_path: '$(& $fwd "$Demo\rearview.db")'
clips_path: '$(& $fwd "$Demo\clips")'
engine:
  path: '$(& $fwd $Engine)'
  device: $Device
"@ | Set-Content -Encoding utf8 $cfg

Step 'Models (downloaded once, checksums verified)'
Run $Rearview models download --config $cfg
$modelsDir = ((& $Rearview models --config $cfg | Select-Object -First 1) -replace '^models ', '')

Step 'Sample clips'
$samples = @(Get-ChildItem "$here\samples\*.mp4" -ErrorAction SilentlyContinue)
if ($samples.Count -eq 0) {
    # A repo checkout has them in testdata/clips (Git LFS; small files are LFS pointers).
    $samples = @(Get-ChildItem "$here\..\testdata\clips\*.mp4" -ErrorAction SilentlyContinue | Where-Object Length -gt 1MB)
}
if ($samples.Count -eq 0) {
    $samples = @(Get-ChildItem "$Demo\samples\*.mp4" -ErrorAction SilentlyContinue)
}
if ($samples.Count -eq 0) {
    Write-Host "downloading $SamplesUrl (about 200 MB)"
    $zip = Join-Path $Demo 'samples.zip'
    Invoke-WebRequest -UseBasicParsing $SamplesUrl -OutFile $zip
    Expand-Archive -Force $zip (Join-Path $Demo 'samples')
    Remove-Item $zip
    $samples = @(Get-ChildItem "$Demo\samples\*.mp4")
}
# Give the clips the camera's file names (consecutive 20-second loops of one trip) so
# import treats them like recordings off the SD card. They carry no GPS, so speed-based
# events (hard brakes, following distance) stay empty; vision and search work.
$card = Join-Path $Demo 'card\DCIM\Movie'
New-Item -ItemType Directory -Force $card | Out-Null
$start = [datetime]'2026-10-23 15:11:53'
for ($i = 0; $i -lt $samples.Count; $i++) {
    $name = '{0:yyyy_MMdd_HHmmss}_{1:D6}F.MP4' -f $start.AddSeconds(20 * $i), ($i + 1)
    $dest = Join-Path $card $name
    if (-not (Test-Path $dest)) { Copy-Item $samples[$i].FullName $dest }
    Write-Host "$($samples[$i].Name) -> $name"
}

Step 'Import (file names, GPS, trips)'
Run $Rearview import (Join-Path $Demo 'card') --config $cfg

Step "Detect, track, distance and time-to-collision ($Device)"
Run $Rearview process --plain --config $cfg

Step 'Index keyframes for search (CLIP)'
Run $Rearview process --embed --plain --config $cfg

Step "Search: `"$Query`""
Run $Rearview search $Query --limit 3 --clip --config $cfg

Step 'What the engine sees (boxes, track ids, distance, TTC drawn on the video)'
$first = Join-Path $card ('{0:yyyy_MMdd_HHmmss}_000001F.MP4' -f $start)
$raw = Join-Path $Demo 'boxes_raw.mp4'
$boxes = Join-Path $Demo 'boxes.mp4'
Run $Engine detect $first --features 'detect,track,distance,ttc' --device $Device --models $modelsDir --debug-video $raw
# Re-encode to H.264 at 720p so any player (and a browser) can show it.
Run (Get-Command ffmpeg).Source -y -loglevel error -i $raw -vf 'scale=1280:-2' '-c:v' libx264 -pix_fmt yuv420p -crf 26 $boxes
Remove-Item $raw

Write-Host "`nDone. In $Demo :" -ForegroundColor Green
Write-Host "  boxes.mp4   the first clip with every tracked object, its distance and time-to-collision"
Write-Host "  clips\      15-second clips around the search results"
Write-Host "`nTry more:"
Write-Host "  & '$Rearview' search `"a white truck`" --config '$cfg'"
Write-Host "  & '$Rearview' trips --config '$cfg'"
Write-Host "  & '$Rearview' --help"

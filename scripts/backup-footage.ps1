<#
.SYNOPSIS
    Backs up dashcam footage to a raw folder and keeps a SHA-256 manifest of it.

.DESCRIPTION
    With -Source (the SD card root), copies every file not yet in -Dest, keeping the
    card's folder layout, and checks each copy's hash against the source.
    Every run adds any file in -Dest that is not yet in the manifest
    (<Dest>\SHA256SUMS.txt, sha256sum format), then prints a summary with hours of
    front-camera footage per day.

    -VerifyOnly re-hashes every file in the manifest and fails on any mismatch or
    missing file. Exit code: 0 = ok, 1 = verification or copy failure.

.EXAMPLE
    powershell -ExecutionPolicy Bypass -File scripts\backup-footage.ps1 -Source E:\
.EXAMPLE
    powershell -ExecutionPolicy Bypass -File scripts\backup-footage.ps1 -VerifyOnly
#>
[CmdletBinding()]
param(
    [string]$Source,
    [string]$Dest = (Join-Path $HOME 'rearview-raw'),
    [switch]$VerifyOnly,
    [string]$FrontPattern = '*F.MP4',
    [double]$MinHours = 0
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

$ManifestName = 'SHA256SUMS.txt'
$SkipDirs = @('System Volume Information', '$RECYCLE.BIN')

function Get-RelativePath([string]$Root, [string]$Path) {
    $rootFull = [IO.Path]::GetFullPath($Root).TrimEnd('\') + '\'
    return ([IO.Path]::GetFullPath($Path).Substring($rootFull.Length)) -replace '\\', '/'
}

function Get-Files([string]$Root) {
    Get-ChildItem -LiteralPath $Root -Recurse -File -Force | Where-Object {
        $rel = Get-RelativePath $Root $_.FullName
        $top = ($rel -split '/')[0]
        ($SkipDirs -notcontains $top) -and ($rel -ne $ManifestName) -and ($_.Name -notlike '*.partial')
    }
}

function Get-Sha256([string]$Path) {
    return (Get-FileHash -LiteralPath $Path -Algorithm SHA256).Hash.ToLowerInvariant()
}

function Read-Manifest([string]$Path) {
    $map = [ordered]@{}
    if (Test-Path -LiteralPath $Path) {
        foreach ($line in Get-Content -LiteralPath $Path) {
            if ($line -match '^([0-9a-f]{64}) \*(.+)$') { $map[$Matches[2]] = $Matches[1] }
        }
    }
    return $map
}

function Add-ManifestLine([string]$Path, [string]$Hash, [string]$Rel) {
    # ASCII keeps the file readable by sha256sum; footage file names are ASCII.
    Add-Content -LiteralPath $Path -Value "$Hash *$Rel" -Encoding Ascii
}

function Get-DurationSeconds([string]$Path) {
    if (-not (Get-Command ffprobe -ErrorAction SilentlyContinue)) { return 0 }
    $ErrorActionPreference = 'Continue'
    $out = & ffprobe -v error -show_entries format=duration -of csv=p=0 $Path 2>$null
    $d = 0.0
    if ($out -and [double]::TryParse("$out".Trim(), [Globalization.NumberStyles]::Float, [Globalization.CultureInfo]::InvariantCulture, [ref]$d)) {
        return $d
    }
    return 0
}

if (-not (Test-Path -LiteralPath $Dest)) {
    if ($VerifyOnly) { throw "Destination not found: $Dest" }
    New-Item -ItemType Directory -Path $Dest | Out-Null
}
$manifestPath = Join-Path $Dest $ManifestName
$manifest = Read-Manifest $manifestPath
$failures = @()

if ($VerifyOnly) {
    $i = 0
    foreach ($rel in @($manifest.Keys)) {
        $i++
        Write-Progress -Activity 'Verifying' -Status $rel -PercentComplete (100 * $i / [Math]::Max(1, $manifest.Count))
        $path = Join-Path $Dest $rel
        if (-not (Test-Path -LiteralPath $path)) {
            $failures += "missing: $rel"
        } elseif ((Get-Sha256 $path) -ne $manifest[$rel]) {
            $failures += "hash mismatch: $rel"
        }
    }
    Write-Progress -Activity 'Verifying' -Completed
    $extra = @(Get-Files $Dest | Where-Object { -not $manifest.Contains((Get-RelativePath $Dest $_.FullName)) })
    foreach ($f in $extra) { Write-Warning "not in manifest: $(Get-RelativePath $Dest $f.FullName)" }
    Write-Host "Verified $($manifest.Count - $failures.Count) of $($manifest.Count) files."
} else {
    $copied = 0
    $skipped = 0
    if ($Source) {
        if (-not (Test-Path -LiteralPath $Source)) { throw "Source not found: $Source" }
        foreach ($f in Get-Files $Source) {
            $rel = Get-RelativePath $Source $f.FullName
            $target = Join-Path $Dest $rel
            if ((Test-Path -LiteralPath $target) -and (Get-Item -LiteralPath $target).Length -eq $f.Length) {
                $skipped++
                continue
            }
            New-Item -ItemType Directory -Path (Split-Path $target) -Force | Out-Null
            # Copy under a temp name so an interrupted copy never looks like a finished file.
            $partial = "$target.partial"
            Copy-Item -LiteralPath $f.FullName -Destination $partial -Force
            $srcHash = Get-Sha256 $f.FullName
            if ((Get-Sha256 $partial) -ne $srcHash) {
                Remove-Item -LiteralPath $partial -Force
                $failures += "copy hash mismatch: $rel"
                continue
            }
            Move-Item -LiteralPath $partial -Destination $target -Force
            if ($manifest.Contains($rel)) {
                Write-Warning "replaced a different file with the same name: $rel"
                $manifest.Remove($rel)
            }
            $manifest[$rel] = $srcHash
            Add-ManifestLine $manifestPath $srcHash $rel
            $copied++
        }
        Write-Host "Copied $copied new files, skipped $skipped already present."
    }

    $added = 0
    foreach ($f in Get-Files $Dest) {
        $rel = Get-RelativePath $Dest $f.FullName
        if ($manifest.Contains($rel)) { continue }
        Write-Progress -Activity 'Hashing' -Status $rel
        $h = Get-Sha256 $f.FullName
        $manifest[$rel] = $h
        Add-ManifestLine $manifestPath $h $rel
        $added++
    }
    Write-Progress -Activity 'Hashing' -Completed
    Write-Host "Added $added files to $manifestPath ($($manifest.Count) total)."
}

$files = @(Get-Files $Dest)
$bytes = ($files | Measure-Object -Property Length -Sum).Sum
if (-not $bytes) { $bytes = 0 }
$front = @($files | Where-Object { $_.Name -like $FrontPattern })
$byDay = [ordered]@{}
$totalSec = 0.0
foreach ($f in ($front | Sort-Object Name)) {
    $sec = Get-DurationSeconds $f.FullName
    $day = if ($f.Name -match '^(\d{4})_(\d{2})(\d{2})_') { "$($Matches[1])-$($Matches[2])-$($Matches[3])" } else { 'unknown' }
    if (-not $byDay.Contains($day)) { $byDay[$day] = 0.0 }
    $byDay[$day] += $sec
    $totalSec += $sec
}

Write-Host ''
Write-Host ('{0} files, {1:N1} GB in {2}' -f $files.Count, ($bytes / 1GB), $Dest)
foreach ($day in $byDay.Keys) {
    Write-Host ('  {0}  {1,6:N2} h front-camera footage' -f $day, ($byDay[$day] / 3600))
}
$hours = $totalSec / 3600
Write-Host ('  total       {0,6:N2} h front-camera footage ({1} files)' -f $hours, $front.Count)

if ($failures.Count -gt 0) {
    foreach ($msg in $failures) { Write-Host "  FAIL $msg" -ForegroundColor Red }
    exit 1
}
if ($hours -lt $MinHours) {
    Write-Host ('Only {0:N2} h of front footage; need {1} h.' -f $hours, $MinHours) -ForegroundColor Red
    exit 1
}
exit 0

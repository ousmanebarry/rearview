<#
.SYNOPSIS
    Installs or checks the Rearview toolchain on Windows 11 and prints every version.

.DESCRIPTION
    Tools: Go, Git, Git LFS, GitHub CLI, Visual Studio 2022 Build Tools (C++ workload),
    CMake, vcpkg, ffmpeg, exiftool. Missing tools are installed with winget (vcpkg is cloned
    and bootstrapped). Also reports the Windows build (24H2+ is needed for
    Windows ML) and the GPU driver.

    Run once from an elevated PowerShell. Use -CheckOnly to report without installing.
    Exit code is 0 only when every tool is present.

.EXAMPLE
    powershell -ExecutionPolicy Bypass -File scripts\setup-windows.ps1
.EXAMPLE
    powershell -ExecutionPolicy Bypass -File scripts\setup-windows.ps1 -CheckOnly
#>
[CmdletBinding()]
param(
    [switch]$CheckOnly,
    [string]$VcpkgRoot = $(if ($env:VCPKG_ROOT) { $env:VCPKG_ROOT } else { 'C:\vcpkg' })
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

function Test-Admin {
    $id = [Security.Principal.WindowsIdentity]::GetCurrent()
    (New-Object Security.Principal.WindowsPrincipal $id).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
}

# winget installs update the registry PATH, not this session's PATH.
function Update-SessionPath {
    $machine = [Environment]::GetEnvironmentVariable('Path', 'Machine')
    $user = [Environment]::GetEnvironmentVariable('Path', 'User')
    $env:Path = "$machine;$user"
}

# First line of "<exe> <args>", or $null if the tool is not on PATH or prints nothing.
function Get-ToolVersion([string]$Exe, [string[]]$ToolArgs) {
    $cmd = Get-Command $Exe -CommandType Application -ErrorAction SilentlyContinue | Select-Object -First 1
    if (-not $cmd) { return $null }
    $ErrorActionPreference = 'Continue'
    $out = & $cmd.Source @ToolArgs 2>$null | Select-Object -First 1
    if ($out) { return "$out".Trim() }
    return $null
}

function Get-VsBuildTools {
    $vswhere = Join-Path ${env:ProgramFiles(x86)} 'Microsoft Visual Studio\Installer\vswhere.exe'
    if (-not (Test-Path $vswhere)) { return $null }
    $v = & $vswhere -products * -version '[17.0,18.0)' `
        -requires Microsoft.VisualStudio.Component.VC.Tools.x86.x64 `
        -property catalog_productDisplayVersion | Select-Object -First 1
    if ($v) { return "Visual Studio 2022 C++ tools $v" }
    return $null
}

function Get-VcpkgExe {
    $cmd = Get-Command vcpkg -ErrorAction SilentlyContinue
    if ($cmd) { return $cmd.Source }
    $exe = Join-Path $VcpkgRoot 'vcpkg.exe'
    if (Test-Path $exe) { return $exe }
    return $null
}

function Get-VcpkgVersion {
    $exe = Get-VcpkgExe
    if (-not $exe) { return $null }
    return Get-ToolVersion $exe @('version')
}

function Install-Winget([string]$Id, [string[]]$Extra = @()) {
    if (-not (Get-Command winget -ErrorAction SilentlyContinue)) {
        throw 'winget not found. Install "App Installer" from the Microsoft Store, then re-run.'
    }
    $wingetArgs = @('install', '--id', $Id, '--exact', '--silent', '--accept-package-agreements', '--accept-source-agreements') + $Extra
    Write-Host "  winget $($wingetArgs -join ' ')"
    & winget @wingetArgs
    # -1978335189 = already installed, 3010 = success but reboot required
    if ($LASTEXITCODE -ne 0 -and $LASTEXITCODE -ne -1978335189 -and $LASTEXITCODE -ne 3010) {
        throw "winget install $Id failed with exit code $LASTEXITCODE"
    }
    Update-SessionPath
}

function Install-Vcpkg {
    if (-not (Test-Path (Join-Path $VcpkgRoot '.git'))) {
        & git clone https://github.com/microsoft/vcpkg $VcpkgRoot
        if ($LASTEXITCODE -ne 0) { throw "git clone vcpkg into $VcpkgRoot failed" }
    }
    & (Join-Path $VcpkgRoot 'bootstrap-vcpkg.bat') -disableMetrics
    if ($LASTEXITCODE -ne 0) { throw 'bootstrap-vcpkg.bat failed' }

    [Environment]::SetEnvironmentVariable('VCPKG_ROOT', $VcpkgRoot, 'User')
    $env:VCPKG_ROOT = $VcpkgRoot
    $userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
    if (($userPath -split ';') -notcontains $VcpkgRoot) {
        [Environment]::SetEnvironmentVariable('Path', "$userPath;$VcpkgRoot", 'User')
    }
    Update-SessionPath
}

$tools = @(
    @{ Name = 'Go';        Version = { Get-ToolVersion 'go' @('version') };      Install = { Install-Winget 'GoLang.Go' } }
    @{ Name = 'Git';       Version = { Get-ToolVersion 'git' @('--version') };   Install = { Install-Winget 'Git.Git' } }
    @{ Name = 'Git LFS';   Version = { Get-ToolVersion 'git-lfs' @('version') }; Install = { Install-Winget 'GitHub.GitLFS' } }
    @{ Name = 'GitHub CLI'; Version = { Get-ToolVersion 'gh' @('--version') };   Install = { Install-Winget 'GitHub.cli' } }
    @{ Name = 'VS 2022 Build Tools'; Version = { Get-VsBuildTools };            Install = {
            Install-Winget 'Microsoft.VisualStudio.2022.BuildTools' @('--override', '--quiet --wait --norestart --add Microsoft.VisualStudio.Workload.VCTools --includeRecommended')
        } }
    @{ Name = 'CMake';     Version = { Get-ToolVersion 'cmake' @('--version') }; Install = { Install-Winget 'Kitware.CMake' } }
    @{ Name = 'vcpkg';     Version = { Get-VcpkgVersion };                       Install = { Install-Vcpkg } }
    @{ Name = 'ffmpeg';    Version = { Get-ToolVersion 'ffmpeg' @('-version') }; Install = { Install-Winget 'Gyan.FFmpeg' } }
    # ffprobe ships with the ffmpeg package.
    @{ Name = 'ffprobe';   Version = { Get-ToolVersion 'ffprobe' @('-version') }; Install = { } }
    @{ Name = 'exiftool';  Version = { Get-ToolVersion 'exiftool' @('-ver') };   Install = { Install-Winget 'OliverBetz.ExifTool' } }
)

if (-not $CheckOnly -and -not (Test-Admin)) {
    throw 'Run this script from an elevated PowerShell (Run as administrator), or pass -CheckOnly.'
}

$failed = @()
foreach ($t in $tools) {
    $v = & $t.Version
    if (-not $v -and -not $CheckOnly) {
        Write-Host "Installing $($t.Name)..." -ForegroundColor Cyan
        try {
            & $t.Install
            $v = & $t.Version
        } catch {
            Write-Warning "$($t.Name): $_"
        }
    }
    if (-not $v) { $failed += $t.Name }
    $t.Result = $v
}

Write-Host ''
Write-Host 'Toolchain' -ForegroundColor Cyan
foreach ($t in $tools) {
    if ($t.Result) {
        Write-Host ('  [ok]      {0,-20} {1}' -f $t.Name, $t.Result)
    } else {
        Write-Host ('  [missing] {0,-20}' -f $t.Name) -ForegroundColor Red
    }
}

# Windows ML needs Windows 11 24H2 (build 26100) or later.
$os = Get-CimInstance Win32_OperatingSystem
$build = [int]$os.BuildNumber
$osNote = if ($build -ge 26100) { 'ok for Windows ML' } else { 'Windows ML needs 24H2 (build 26100) or later' }
Write-Host ''
Write-Host 'System' -ForegroundColor Cyan
Write-Host ('  {0,-28} {1} (build {2}): {3}' -f 'Windows', $os.Caption, $build, $osNote)
foreach ($gpu in Get-CimInstance Win32_VideoController) {
    Write-Host ('  {0,-28} {1}, driver {2} ({3:yyyy-MM-dd})' -f 'GPU', $gpu.Name, $gpu.DriverVersion, $gpu.DriverDate)
}
Write-Host '  Install the latest AMD Adrenalin driver by hand: https://www.amd.com/en/support'

Write-Host ''
if ($failed.Count -gt 0) {
    Write-Host "Missing: $($failed -join ', ')" -ForegroundColor Red
    if ($CheckOnly) { Write-Host 'Re-run without -CheckOnly from an elevated PowerShell to install them.' }
    exit 1
}
Write-Host 'Every tool is present.' -ForegroundColor Green
exit 0

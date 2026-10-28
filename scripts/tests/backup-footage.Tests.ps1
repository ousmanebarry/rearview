# Pester 3.4 (ships with Windows PowerShell 5.1):
#   powershell -NoProfile -Command "Invoke-Pester scripts\tests"
$script = Join-Path $PSScriptRoot '..\backup-footage.ps1'

function Invoke-Backup([string[]]$BackupArgs) {
    $out = & powershell.exe -NoProfile -ExecutionPolicy Bypass -File $script @BackupArgs 2>&1
    return @{ Code = $LASTEXITCODE; Output = ($out | Out-String) }
}

Describe 'backup-footage.ps1' {
    $root = Join-Path ([IO.Path]::GetTempPath()) ("rearview-backup-test-" + [guid]::NewGuid())
    $card = Join-Path $root 'card'
    $raw = Join-Path $root 'raw'
    New-Item -ItemType Directory -Path (Join-Path $card 'DCIM\Movie\RO') -Force | Out-Null
    Set-Content -LiteralPath (Join-Path $card 'DCIM\Movie\2026_1023_130515_000587F.MP4') -Value 'front clip' -Encoding Ascii
    Set-Content -LiteralPath (Join-Path $card 'DCIM\Movie\2026_1023_130515_000588R.MP4') -Value 'rear clip' -Encoding Ascii
    Set-Content -LiteralPath (Join-Path $card 'DCIM\Movie\RO\2026_1023_131115_000591F.MP4') -Value 'locked' -Encoding Ascii

    It 'copies every file, keeping the folder layout, and writes the manifest' {
        $r = Invoke-Backup @('-Source', $card, '-Dest', $raw)
        $r.Code | Should Be 0
        Test-Path (Join-Path $raw 'DCIM\Movie\RO\2026_1023_131115_000591F.MP4') | Should Be $true
        $lines = @(Get-Content (Join-Path $raw 'SHA256SUMS.txt'))
        $lines.Count | Should Be 3
        $expected = (Get-FileHash (Join-Path $card 'DCIM\Movie\2026_1023_130515_000587F.MP4') -Algorithm SHA256).Hash.ToLower()
        ($lines -contains "$expected *DCIM/Movie/2026_1023_130515_000587F.MP4") | Should Be $true
        @(Get-ChildItem $raw -Recurse -Filter '*.partial').Count | Should Be 0
    }

    It 'copies nothing on a second run' {
        $r = Invoke-Backup @('-Source', $card, '-Dest', $raw)
        $r.Code | Should Be 0
        $r.Output | Should Match 'Copied 0 new files, skipped 3'
        @(Get-Content (Join-Path $raw 'SHA256SUMS.txt')).Count | Should Be 3
    }

    It 'passes verification when nothing changed' {
        (Invoke-Backup @('-Dest', $raw, '-VerifyOnly')).Code | Should Be 0
    }

    It 'fails verification when a file is altered' {
        Add-Content -LiteralPath (Join-Path $raw 'DCIM\Movie\2026_1023_130515_000588R.MP4') -Value 'bit rot'
        $r = Invoke-Backup @('-Dest', $raw, '-VerifyOnly')
        $r.Code | Should Be 1
        $r.Output | Should Match 'hash mismatch: DCIM/Movie/2026_1023_130515_000588R.MP4'
    }

    It 'fails verification when a file is missing' {
        Remove-Item -LiteralPath (Join-Path $raw 'DCIM\Movie\RO\2026_1023_131115_000591F.MP4')
        $r = Invoke-Backup @('-Dest', $raw, '-VerifyOnly')
        $r.Code | Should Be 1
        $r.Output | Should Match 'missing: DCIM/Movie/RO/2026_1023_131115_000591F.MP4'
    }

    It 'builds a manifest for an existing folder without a source' {
        $plain = Join-Path $root 'plain'
        New-Item -ItemType Directory -Path $plain | Out-Null
        Set-Content -LiteralPath (Join-Path $plain 'a.MP4') -Value 'a' -Encoding Ascii
        $r = Invoke-Backup @('-Dest', $plain)
        $r.Code | Should Be 0
        @(Get-Content (Join-Path $plain 'SHA256SUMS.txt')).Count | Should Be 1
        (Invoke-Backup @('-Dest', $plain, '-VerifyOnly')).Code | Should Be 0
    }

    It 'fails when there is less footage than -MinHours' {
        (Invoke-Backup @('-Dest', $raw, '-MinHours', '5')).Code | Should Be 1
    }

    Remove-Item -LiteralPath $root -Recurse -Force
}

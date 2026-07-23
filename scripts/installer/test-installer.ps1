<#
.SYNOPSIS
  Portable unit tests for the ZeitBoard installer library. No Pester dependency
  (stock Windows ships Pester 3.4 while CI has 5.x; their assertion syntaxes are
  incompatible), so this uses plain assertions with an exit code. Runs the same
  everywhere. CI also calls install.ps1/update.ps1 with -DryRun.

.EXAMPLE
  powershell -ExecutionPolicy Bypass -File scripts\installer\test-installer.ps1
#>
[CmdletBinding()]
param()
$ErrorActionPreference = 'Stop'
. "$PSScriptRoot\_zb.common.ps1"

$script:pass = 0
$script:fail = 0

function Test-Case {
    param([string]$Name, [scriptblock]$Body)
    try { & $Body; $script:pass++; Write-Host "  ok   $Name" -ForegroundColor Green }
    catch { $script:fail++; Write-Host "  FAIL $Name" -ForegroundColor Red; Write-Host "       $($_.Exception.Message)" -ForegroundColor DarkGray }
}
function Assert-True { param($Value, $Message = 'expected true') if (-not $Value) { throw $Message } }
function Assert-Equal { param($Expected, $Actual) if ($Expected -ne $Actual) { throw "expected [$Expected], got [$Actual]" } }
function Assert-Throws { param([scriptblock]$Body, $Message = 'expected an exception') $threw = $false; try { & $Body } catch { $threw = $true }; if (-not $threw) { throw $Message } }

Write-Host 'ZeitBoard installer library tests' -ForegroundColor Cyan

Test-Case 'Get-ZbPaths resolves the repo root to the working tree' {
    $p = Get-ZbPaths
    Assert-True (Test-Path (Join-Path $p.RepoRoot 'go.work')) 'repo root should contain go.work'
    Assert-True ($p.InstallDir -like '*Programs\ZeitBoard') 'install dir under Programs'
    Assert-True ($p.DataDir -like '*ZeitBoard') 'data dir named ZeitBoard'
}

Test-Case 'pins.psd1 is structurally valid (https + one integrity source each)' {
    $problems = Test-ZbPins
    Assert-Equal 0 $problems.Count
}

Test-Case 'Read-ZbChoice: explicit override beats everything' {
    Assert-Equal $true (Read-ZbChoice -Question 'x' -Default $false -NonInteractive -Override $true)
    Assert-Equal $false (Read-ZbChoice -Question 'x' -Default $true -NonInteractive -Override $false)
}

Test-Case 'Read-ZbChoice: non-interactive returns the default' {
    Assert-Equal $true (Read-ZbChoice -Question 'x' -Default $true -NonInteractive)
    Assert-Equal $false (Read-ZbChoice -Question 'x' -Default $false -NonInteractive)
}

Test-Case 'Get-ZbExpectedHash: literal checksum is returned lowercased' {
    Assert-Equal 'abc123' (Get-ZbExpectedHash -Pin @{ Version = 't'; Sha256 = 'ABC123' })
}

Test-Case 'Get-ZbExpectedHash: REPLACE_ placeholder fails closed with guidance' {
    Assert-Throws { Get-ZbExpectedHash -Pin @{ Version = 't'; Sha256 = 'REPLACE_WITH_REAL' } }
}

Test-Case 'Set-ZbStartupEntry: add/remove round-trips against a sandbox key' {
    $sandbox = 'HKCU:\Software\ZeitBoardTest\Run'
    try {
        Set-ZbStartupEntry -TargetPath 'C:\x\ZeitBoard.exe' -Enabled $true -RunKey $sandbox
        $v = (Get-ItemProperty -Path $sandbox -Name 'ZeitBoard').ZeitBoard
        Assert-Equal '"C:\x\ZeitBoard.exe"' $v
        Set-ZbStartupEntry -TargetPath 'C:\x\ZeitBoard.exe' -Enabled $false -RunKey $sandbox
        $after = Get-ItemProperty -Path $sandbox -Name 'ZeitBoard' -ErrorAction SilentlyContinue
        Assert-True ($null -eq $after) 'entry should be gone after disable'
    }
    finally {
        Remove-Item -Path 'HKCU:\Software\ZeitBoardTest' -Recurse -Force -ErrorAction SilentlyContinue
    }
}

Test-Case 'Invoke-ZbStep: -Check short-circuits the action' {
    $ran = $false
    Invoke-ZbStep -Name 'noop' -Check { $true } -Action { $script:ran = $true } | Out-Null
    Assert-True (-not $ran) 'action must not run when Check returns true'
}

Test-Case 'Invoke-ZbStep: -DryRun never runs the action' {
    $ran = $false
    Invoke-ZbStep -Name 'noop' -DryRun -Action { $script:ran = $true } | Out-Null
    Assert-True (-not $ran) 'action must not run under -DryRun'
}

Test-Case 'Show-ZbBanner and Show-ZbFinale are ASCII-only' {
    $out = (Show-ZbBanner *>&1 | Out-String) + (Show-ZbFinale -Kind install *>&1 | Out-String)
    $nonAscii = ($out.ToCharArray() | Where-Object { [int][char]$_ -gt 126 })
    Assert-Equal 0 $nonAscii.Count
}

Write-Host ''
Write-Host "  $script:pass passed, $script:fail failed" -ForegroundColor $(if ($script:fail -eq 0) { 'Green' } else { 'Red' })
if ($script:fail -gt 0) { exit 1 }

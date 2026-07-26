<#
.SYNOPSIS
  Remove an installed ZeitBoard: binaries, shortcuts, startup entry, and,
  when requested, the server service. Design: docs/install-update-design.md.

.DESCRIPTION
  Data (%APPDATA%\ZeitBoard) is preserved by default. -PurgeData additionally
  removes it, but only after requiring you to type DELETE and writing a final
  raw recovery backup. The backup is not the app's portable data export.

    -PurgeData            also erase the data directory (typed DELETE required)
    -ServiceName <name>   server service name (default: ZeitBoardServer)
    -RemoveServer         remove the server service registration and firewall rule
    -ServerRoot <dir>     retained server data/config/secrets root
    -NonInteractive / -DryRun
#>
[CmdletBinding()]
param(
    [switch]$PurgeData,
    [string]$ServiceName = 'ZeitBoardServer',
    [switch]$RemoveServer,
    [string]$ServerRoot = (Join-Path $env:ProgramData 'ZeitBoard'),
    [switch]$NonInteractive,
    [switch]$DryRun
)

$ErrorActionPreference = 'Stop'
. "$PSScriptRoot\_zb.common.ps1"

$resume = "powershell -ExecutionPolicy Bypass -File `"$PSCommandPath`""
Start-ZbLog -Name 'uninstall' -DryRun:$DryRun | Out-Null
Show-ZbBanner
$paths = Get-ZbPaths
$ServerRoot = $ExecutionContext.SessionState.Path.GetUnresolvedProviderPathFromPSPath($ServerRoot)
$installedExe = Join-Path $paths.InstallDir 'ZeitBoard.exe'
$installedMcp = Join-Path $paths.InstallDir 'zeitboard-mcp.exe'
$serverExe = Join-Path $ServerRoot 'zeitboardd.exe'
$script:ZbPurgeConfirmed = $false
$lifecycleLock = $null
$exitCode = 0

try {
    $lifecycleLock = Enter-ZbLifecycleLock
    Invoke-ZbStep -Name 'Preflight' -DryRun:$DryRun -ResumeHint $resume -Action {
        Assert-ZbAppStopped -TargetPath $installedExe
        if (Test-Path -LiteralPath $installedMcp) {
            Assert-ZbExecutableStopped -TargetPath $installedMcp
        }
        if ($RemoveServer) {
            if ($ServiceName -notmatch '^[A-Za-z0-9_.-]+$') {
                throw 'ServiceName may contain only letters, digits, dot, underscore, and hyphen.'
            }
            $elevated = ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
            if (-not $elevated) {
                throw 'Removing the Windows service needs an elevated prompt. Re-run this script as Administrator.'
            }
            $service = Get-Service -Name $ServiceName -ErrorAction SilentlyContinue
            if ($service) {
                Assert-ZbServiceOwned -ServiceName $ServiceName -ExpectedExecutable $serverExe
            }
        }
    }

    # Confirm before changing binaries, shortcuts, services, or startup state.
    if ($PurgeData) {
        Invoke-ZbStep -Name 'Confirm permanent data erasure' -DryRun:$DryRun -ResumeHint $resume -Action {
            if (-not (Test-Path -LiteralPath $paths.DataDir)) {
                Write-ZbLog -Message 'no data directory to erase'
                $script:ZbPurgeConfirmed = $true
                return
            }
            if ($NonInteractive) {
                throw 'Refusing to purge data non-interactively. Re-run interactively and type DELETE, or remove the data directory deliberately by hand.'
            }
            Write-Host ''
            Write-Host "  This permanently erases $($paths.DataDir) (sleep records, tasks, settings, and tokens)." -ForegroundColor Red
            $typed = Read-Host '  Type DELETE to confirm'
            if ($typed -ne 'DELETE') { throw 'Not confirmed - no installed state was changed.' }
            $script:ZbPurgeConfirmed = $true
        }

        Invoke-ZbStep -Name 'Back up data before erasure' -DryRun:$DryRun -ResumeHint $resume -Action {
            if (-not (Test-Path -LiteralPath $paths.DataDir)) { return }
            if (-not $script:ZbPurgeConfirmed) { throw 'Internal error: purge was not confirmed.' }
            $backup = Backup-ZbData -Reason 'pre-purge'
            if ($backup) {
                Write-ZbLog -Level warn -Message "raw recovery backup: $backup"
                Write-ZbLog -Level warn -Message 'This ZIP can contain database files and tokens; protect it as sensitive data.'
            }
        }
    }

    if ($RemoveServer) {
        Invoke-ZbStep -Name 'Remove server service' -DryRun:$DryRun -ResumeHint $resume -Action {
            $service = Get-Service -Name $ServiceName -ErrorAction SilentlyContinue
            if ($service) {
                Assert-ZbServiceOwned -ServiceName $ServiceName -ExpectedExecutable $serverExe
                if ($service.Status -ne 'Stopped') {
                    Stop-Service -Name $ServiceName -Force -ErrorAction Stop
                    $service.WaitForStatus('Stopped', [TimeSpan]::FromSeconds(20))
                }
                & sc.exe delete $ServiceName | Out-Null
                if ($LASTEXITCODE -ne 0) { throw "sc.exe delete $ServiceName failed with exit $LASTEXITCODE." }
                Write-ZbLog -Level ok -Message "service '$ServiceName' removed"
            }
            else {
                Write-ZbLog -Message "service '$ServiceName' is not installed"
            }
            Write-ZbLog -Level warn -Message "server data, config, binaries, and secrets were preserved at $ServerRoot"
        }

        Invoke-ZbStep -Name 'Remove server firewall rule' -DryRun:$DryRun -ResumeHint $resume -Action {
            $configPath = Join-Path $ServerRoot 'config.json'
            $previousConfigPath = Join-Path $ServerRoot 'previous\config.json'
            if (-not (Test-Path -LiteralPath $configPath) -and -not (Test-Path -LiteralPath $previousConfigPath)) {
                Write-ZbLog -Message 'server config is absent; no installer-managed firewall rule can be identified'
                return
            }
            $configPaths = @($configPath, $previousConfigPath)
            $ruleNames = @()
            foreach ($candidate in $configPaths) {
                if (-not (Test-Path -LiteralPath $candidate)) { continue }
                try {
                    $config = Get-Content -Raw -LiteralPath $candidate | ConvertFrom-Json
                    $portMatch = [regex]::Match([string]$config.listenAddress, ':([0-9]+)$')
                    if ($portMatch.Success) {
                        $port = [int]$portMatch.Groups[1].Value
                        $ruleNames += "ZeitBoard Server $ServiceName ($port)"
                        $ruleNames += "ZeitBoard Server ($port)"
                    }
                }
                catch { Write-ZbLog -Level warn -Message "could not inspect $candidate for firewall cleanup: $($_.Exception.Message)" }
            }
            $rules = @(Get-ZbOwnedFirewallRules -DisplayNames $ruleNames -ExpectedProgram $serverExe)
            if ($rules.Count -gt 0) {
                $rules | Remove-NetFirewallRule -ErrorAction Stop
                Write-ZbLog -Level ok -Message "removed $($rules.Count) installer-managed firewall rule(s)"
            }
            else {
                Write-ZbLog -Message 'no installer-managed firewall rule is installed'
            }
        }
    }

    Invoke-ZbStep -Name 'Remove startup entry' -DryRun:$DryRun -ResumeHint $resume -Action {
        Set-ZbStartupEntry -TargetPath $installedExe -Enabled $false
    }

    Invoke-ZbStep -Name 'Remove shortcuts' -DryRun:$DryRun -ResumeHint $resume -Action {
        $links = @(
            (Join-Path ([Environment]::GetFolderPath('Programs')) 'ZeitBoard.lnk'),
            (Join-Path ([Environment]::GetFolderPath('Desktop')) 'ZeitBoard.lnk')
        )
        foreach ($l in $links) { Remove-ZbShortcutIfOwned -LinkPath $l -ExpectedTarget $installedExe }
    }

    Invoke-ZbStep -Name 'Remove installed binaries' -DryRun:$DryRun -ResumeHint $resume -Action {
        Assert-ZbAppStopped -TargetPath $installedExe
        if (Test-Path -LiteralPath $installedMcp) {
            Assert-ZbExecutableStopped -TargetPath $installedMcp
        }
        if (Test-Path -LiteralPath $paths.InstallDir) {
            Remove-Item -LiteralPath $paths.InstallDir -Recurse -Force
            Write-ZbLog -Level ok -Message "removed $($paths.InstallDir)"
        }
        else { Write-ZbLog -Message 'no install directory found' }
    }

    if ($PurgeData) {
        Invoke-ZbStep -Name 'Erase data directory' -DryRun:$DryRun -ResumeHint $resume -Action {
            if (-not (Test-Path -LiteralPath $paths.DataDir)) { Write-ZbLog -Message 'no data directory to erase'; return }
            if (-not $script:ZbPurgeConfirmed) { throw 'Internal error: purge was not confirmed.' }
            Remove-Item -LiteralPath $paths.DataDir -Recurse -Force
            Write-ZbLog -Level ok -Message 'data directory erased'
        }
    }
    else {
        Write-ZbLog -Message "data preserved at $($paths.DataDir) (use -PurgeData to erase)"
    }

    if ($DryRun) {
        Write-ZbLog -Level ok -Message 'dry-run uninstall plan complete; no files, services, firewall rules, or data were changed'
    }
    else {
        Show-ZbFinale -Kind uninstall
        Write-Host '=================================================================' -ForegroundColor Cyan
    }
}
catch {
    Write-Host ''
    Write-ZbLog -Level fail -Message 'Uninstall did not complete.'
    Write-ZbLog -Level fail -Message $_.Exception.Message
    $exitCode = 1
}
finally {
    Exit-ZbLifecycleLock -Mutex $lifecycleLock
}

exit $exitCode

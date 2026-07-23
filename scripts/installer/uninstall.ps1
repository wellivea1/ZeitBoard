<#
.SYNOPSIS
  Remove an installed ZeitBoard: binaries, shortcuts, startup entry, and
  (optionally) the server service. Design: docs/install-update-design.md.

.DESCRIPTION
  Data (%APPDATA%\ZeitBoard) is preserved by default. -PurgeData additionally
  removes it, but only after offering an export and requiring you to type
  DELETE - mirroring the in-app erasure ceremony (ADR-0014).

    -PurgeData            also erase the data directory (typed DELETE required)
    -ServiceName <name>   also remove this server service (default: ZeitBoardServer)
    -RemoveServer         remove the server service + its install root
    -ServerRoot <dir>     server install root for -RemoveServer
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
Start-ZbLog -Name 'uninstall' | Out-Null
Show-ZbBanner
$paths = Get-ZbPaths
$installedExe = Join-Path $paths.InstallDir 'ZeitBoard.exe'

try {
    Invoke-ZbStep -Name 'Remove startup entry' -DryRun:$DryRun -ResumeHint $resume -Action {
        Set-ZbStartupEntry -TargetPath $installedExe -Enabled $false
    }

    Invoke-ZbStep -Name 'Remove shortcuts' -DryRun:$DryRun -ResumeHint $resume -Action {
        $links = @(
            (Join-Path ([Environment]::GetFolderPath('Programs')) 'ZeitBoard.lnk'),
            (Join-Path ([Environment]::GetFolderPath('Desktop')) 'ZeitBoard.lnk')
        )
        foreach ($l in $links) {
            if (Test-Path -LiteralPath $l) { Remove-Item -LiteralPath $l -Force; Write-ZbLog -Level ok -Message "removed $l" }
        }
    }

    Invoke-ZbStep -Name 'Remove installed binaries' -DryRun:$DryRun -ResumeHint $resume -Action {
        if (Test-Path -LiteralPath $paths.InstallDir) {
            Remove-Item -LiteralPath $paths.InstallDir -Recurse -Force
            Write-ZbLog -Level ok -Message "removed $($paths.InstallDir)"
        }
        else { Write-ZbLog -Message 'no install directory found' }
    }

    if ($RemoveServer) {
        Invoke-ZbStep -Name 'Remove server service' -DryRun:$DryRun -ResumeHint $resume -Action {
            if (Get-Service -Name $ServiceName -ErrorAction SilentlyContinue) {
                & sc.exe stop $ServiceName | Out-Null
                & sc.exe delete $ServiceName | Out-Null
                Write-ZbLog -Level ok -Message "service '$ServiceName' removed"
            }
            Write-ZbLog -Level warn -Message "server data/secrets under $ServerRoot were NOT removed. Delete them yourself if intended (they hold the at-rest key)."
        }
    }

    # --- Data purge (deliberate, typed, exports first) ---------------------
    if ($PurgeData) {
        Invoke-ZbStep -Name 'Erase data directory' -DryRun:$DryRun -ResumeHint $resume -Action {
            if (-not (Test-Path -LiteralPath $paths.DataDir)) { Write-ZbLog -Message 'no data directory to erase'; return }
            $backup = Backup-ZbData -Reason 'pre-purge'
            if ($backup) { Write-ZbLog -Level warn -Message "a final backup was written to: $backup" }
            if ($NonInteractive) {
                throw 'Refusing to purge data non-interactively. Re-run interactively and type DELETE, or remove the data directory by hand.'
            }
            Write-Host ''
            Write-Host "  This permanently erases $($paths.DataDir) (your sleep records, tasks, tokens)." -ForegroundColor Red
            $typed = Read-Host '  Type DELETE to confirm'
            if ($typed -ne 'DELETE') { throw 'Not confirmed - data left intact.' }
            Remove-Item -LiteralPath $paths.DataDir -Recurse -Force
            Write-ZbLog -Level ok -Message 'data directory erased'
        }
    }
    else {
        Write-ZbLog -Message "data preserved at $($paths.DataDir) (use -PurgeData to erase)"
    }

    Show-ZbFinale -Kind uninstall
    Write-Host '=================================================================' -ForegroundColor Cyan
}
catch {
    Write-Host ''
    Write-ZbLog -Level fail -Message 'Uninstall did not complete.'
    exit 1
}

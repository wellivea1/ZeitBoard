<#
.SYNOPSIS
  Install the self-hosted ZeitBoard server (zeitboardd) as a Windows service.
  Design: docs/install-update-design.md; runbook: docs/self-hosting.md.

.DESCRIPTION
  Builds zeitboardd, generates real secrets (never defaults), writes a config
  from the self-hosting.md template, and registers a delayed-auto service. The
  bind defaults to 127.0.0.1 and the portal stays disabled; exposing either is
  a deliberate later config edit (docs/portal-design.md).

    -InstallRoot <dir>   where config/secrets/data live (default: %PROGRAMDATA%\ZeitBoard)
    -ServiceName <name>  Windows service name (default: ZeitBoardServer)
    -Firewall            add a LAN firewall rule (default: no; asks if interactive)
    -NonInteractive / -DryRun

  Requires elevation to register the service. Run this from an elevated prompt.
#>
[CmdletBinding()]
param(
    [string]$InstallRoot = (Join-Path $env:ProgramData 'ZeitBoard'),
    [string]$ServiceName = 'ZeitBoardServer',
    [switch]$Firewall,
    [switch]$NonInteractive,
    [switch]$DryRun
)

$ErrorActionPreference = 'Stop'
. "$PSScriptRoot\_zb.common.ps1"

$resume = "powershell -ExecutionPolicy Bypass -File `"$PSCommandPath`""
Start-ZbLog -Name 'server' | Out-Null
Show-ZbBanner
$paths = Get-ZbPaths

function New-ZbSecretFile {
    param([Parameter(Mandatory)][string]$Path)
    if (Test-Path -LiteralPath $Path) { Write-ZbLog -Message "secret exists (kept): $Path"; return }
    $bytes = New-Object byte[] 32
    [Security.Cryptography.RandomNumberGenerator]::Fill($bytes)
    New-Item -ItemType Directory -Force -Path (Split-Path -Parent $Path) | Out-Null
    [Convert]::ToBase64String($bytes) | Set-Content -NoNewline -Encoding ASCII -LiteralPath $Path
    Write-ZbLog -Level ok -Message "generated secret: $Path"
}

try {
    Invoke-ZbStep -Name 'Preflight' -DryRun:$DryRun -ResumeHint $resume -Action {
        $elevated = ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
        if (-not $elevated -and -not $DryRun) {
            throw 'Registering a Windows service needs an elevated prompt. Re-run this script as Administrator.'
        }
    }

    Invoke-ZbStep -Name 'Toolchain (Go)' -DryRun:$DryRun -ResumeHint $resume -Action { Assert-ZbGo -DryRun:$DryRun }

    $exe = Join-Path $InstallRoot 'zeitboardd.exe'
    Invoke-ZbStep -Name 'Build zeitboardd' -DryRun:$DryRun -ResumeHint $resume -Action {
        New-Item -ItemType Directory -Force -Path $InstallRoot | Out-Null
        Push-Location (Join-Path $paths.RepoRoot 'apps\server')
        try { & go build -o $exe ./cmd/zeitboardd; if ($LASTEXITCODE -ne 0) { throw 'zeitboardd build failed.' } }
        finally { Pop-Location }
    }

    Invoke-ZbStep -Name 'Generate secrets' -DryRun:$DryRun -ResumeHint $resume -Action {
        New-ZbSecretFile -Path (Join-Path $InstallRoot 'secrets\data-key.txt')
        New-ZbSecretFile -Path (Join-Path $InstallRoot 'secrets\enrollment-secret.txt')
    }

    $configPath = Join-Path $InstallRoot 'config.json'
    Invoke-ZbStep -Name 'Write config' -DryRun:$DryRun -ResumeHint $resume -Check { Test-Path -LiteralPath $configPath } -Action {
        $config = [ordered]@{
            listenAddress        = '127.0.0.1:8765'
            tlsCertPath          = ''
            tlsKeyPath           = ''
            dataDir              = (Join-Path $InstallRoot 'data')
            dataKeyFile          = (Join-Path $InstallRoot 'secrets\data-key.txt')
            enrollmentSecretFile = (Join-Path $InstallRoot 'secrets\enrollment-secret.txt')
            assistant            = [ordered]@{ provider = 'disabled'; model = ''; apiKeyFile = ''; endpoint = '' }
        }
        $config | ConvertTo-Json -Depth 5 | Set-Content -Encoding UTF8 -LiteralPath $configPath
        Write-ZbLog -Level ok -Message "config: $configPath (binds 127.0.0.1; TLS + provider are yours to set per self-hosting.md)"
    }

    Invoke-ZbStep -Name 'Register Windows service' -DryRun:$DryRun -ResumeHint $resume -Action {
        $existing = Get-Service -Name $ServiceName -ErrorAction SilentlyContinue
        $binPath = "`"$exe`" -config `"$configPath`""
        if ($existing) {
            & sc.exe config $ServiceName binPath= $binPath start= delayed-auto | Out-Null
            Write-ZbLog -Level ok -Message "service '$ServiceName' updated"
        }
        else {
            & sc.exe create $ServiceName binPath= $binPath start= delayed-auto DisplayName= 'ZeitBoard Server' | Out-Null
            if ($LASTEXITCODE -ne 0) { throw "sc.exe create failed ($LASTEXITCODE)." }
            Write-ZbLog -Level ok -Message "service '$ServiceName' created (delayed-auto)"
        }
        & sc.exe start $ServiceName | Out-Null
    }

    Invoke-ZbStep -Name 'Firewall rule' -DryRun:$DryRun -ResumeHint $resume -Action {
        $fwOverride = $null; if ($PSBoundParameters.ContainsKey('Firewall')) { $fwOverride = [bool]$Firewall }
        $want = Read-ZbChoice -Question 'Add a LAN firewall rule for port 8765 (private profile only)?' -Default $false -NonInteractive:$NonInteractive -Override $fwOverride
        if (-not $want) { Write-ZbLog -Message 'no firewall rule added - server reachable on localhost only'; return }
        if (-not (Get-NetFirewallRule -DisplayName 'ZeitBoard Server' -ErrorAction SilentlyContinue)) {
            New-NetFirewallRule -DisplayName 'ZeitBoard Server' -Direction Inbound -Action Allow -Protocol TCP -LocalPort 8765 -Profile Private | Out-Null
            Write-ZbLog -Level ok -Message 'private-profile firewall rule added for :8765'
        }
        Write-ZbLog -Level warn -Message 'The public availability portal stays OFF (portal.enabled). Exposing it is a deliberate config edit - see docs/portal-design.md.'
    }

    Show-ZbFinale -Kind server
    Write-Host "   Config:  $configPath" -ForegroundColor Green
    Write-Host "   Service: $ServiceName (Get-Service $ServiceName)" -ForegroundColor Green
    Write-Host '   Next:    set TLS + a provider per docs/self-hosting.md, then enroll a device.' -ForegroundColor Green
    Write-Host '=================================================================' -ForegroundColor Cyan
}
catch {
    Write-Host ''
    Write-ZbLog -Level fail -Message 'Server install did not complete.'
    exit 1
}

exit 0

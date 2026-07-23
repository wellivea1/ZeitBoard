<#
.SYNOPSIS
  All-in-one ZeitBoard installer: dependencies, build, install, and a behavior
  decision tree. Design: docs/install-update-design.md.

.DESCRIPTION
  Run from a clone of the repo. Nothing is touched outside the repo, the
  per-user install dir, and (only if you choose) HKCU Run / WebView2. The
  data directory (%APPDATA%\ZeitBoard) is never written by this script.

  Every decision-tree branch has a flag twin so the whole thing is scriptable:
    -NonInteractive        answer every prompt from defaults/flags (CI-safe)
    -DryRun                print the plan; touch nothing
    -StartMenu:$false      skip the Start Menu shortcut (default: create)
    -DesktopShortcut       also create a Desktop shortcut (default: no)
    -Startup               launch ZeitBoard at Windows startup (default: no)
    -NoLaunch              do not launch when finished (default: launch)
    -WithServer            also install the self-hosted server (service)
    -WithMcp               also place the MCP connector for Claude Desktop
    -WithAndroid           also build the Android APK

.EXAMPLE
  powershell -ExecutionPolicy Bypass -File scripts\installer\install.ps1

.EXAMPLE
  powershell -ExecutionPolicy Bypass -File scripts\installer\install.ps1 `
    -NonInteractive -DesktopShortcut -Startup
#>
[CmdletBinding()]
param(
    [switch]$NonInteractive,
    [switch]$DryRun,
    [switch]$StartMenu = $true,
    [switch]$DesktopShortcut,
    [switch]$Startup,
    [switch]$NoLaunch,
    [switch]$WithServer,
    [switch]$WithMcp,
    [switch]$WithAndroid
)

$ErrorActionPreference = 'Stop'
. "$PSScriptRoot\_zb.common.ps1"

$resume = "powershell -ExecutionPolicy Bypass -File `"$PSCommandPath`""
Start-ZbLog -Name 'install' | Out-Null
Show-ZbBanner
$paths = Get-ZbPaths
Write-ZbLog -Message "repo:    $($paths.RepoRoot)"
Write-ZbLog -Message "install: $($paths.InstallDir)"
Write-ZbLog -Message "data:    $($paths.DataDir) (never modified by this installer)"
if ($DryRun) { Write-ZbLog -Level warn -Message 'DRY RUN - no changes will be made.' }

try {
    # --- Preflight ---------------------------------------------------------
    Invoke-ZbStep -Name 'Preflight checks' -DryRun:$DryRun -ResumeHint $resume -Action {
        $os = [Environment]::OSVersion.Version
        if ($os.Major -lt 10) { throw "Windows 10 1809+ required (found $os)." }
        if (-not (Test-ZbCommand 'git')) { throw 'git not found. Install Git for Windows, then re-run.' }
        $free = (Get-PSDrive -Name ($paths.RepoRoot.Substring(0, 1))).Free
        if ($free -lt 2GB) { Write-ZbLog -Level warn -Message 'less than 2 GB free on the repo drive.' }
        Write-ZbLog -Message "Windows $os, git present"
    }

    # --- Dependencies ------------------------------------------------------
    Invoke-ZbStep -Name 'Go toolchain (1.26.0)' -DryRun:$DryRun -ResumeHint $resume -Action { Assert-ZbGo -DryRun:$DryRun }
    Invoke-ZbStep -Name 'Node runtime (v24.16.0)' -DryRun:$DryRun -ResumeHint $resume -Action { Assert-ZbNode -DryRun:$DryRun }
    Invoke-ZbStep -Name 'Wails CLI (v2.12.0)' -DryRun:$DryRun -ResumeHint $resume -Action { Assert-ZbWails -DryRun:$DryRun }
    Invoke-ZbStep -Name 'WebView2 Runtime' -DryRun:$DryRun -ResumeHint $resume -Action {
        Assert-ZbWebView2 -DryRun:$DryRun -NonInteractive:$NonInteractive
    }

    # --- Build -------------------------------------------------------------
    Invoke-ZbStep -Name 'Install web dependencies (npm ci)' -DryRun:$DryRun -ResumeHint $resume -Action {
        Push-Location $paths.RepoRoot
        try { & npm ci; if ($LASTEXITCODE -ne 0) { throw 'npm ci failed.' } }
        finally { Pop-Location }
    }
    Invoke-ZbStep -Name 'Build desktop app (wails build)' -DryRun:$DryRun -ResumeHint $resume -Action {
        Push-Location (Join-Path $paths.RepoRoot 'apps\desktop')
        try { & wails build; if ($LASTEXITCODE -ne 0) { throw 'wails build failed.' } }
        finally { Pop-Location }
    }
    if ($WithServer -or $WithMcp) {
        Invoke-ZbStep -Name 'Build server binaries (go build)' -DryRun:$DryRun -ResumeHint $resume -Action {
            Push-Location (Join-Path $paths.RepoRoot 'apps\server')
            try {
                New-Item -ItemType Directory -Force -Path (Join-Path $paths.RepoRoot 'apps\server\bin') | Out-Null
                & go build -o (Join-Path $paths.RepoRoot 'apps\server\bin\zeitboardd.exe') ./cmd/zeitboardd
                if ($LASTEXITCODE -ne 0) { throw 'zeitboardd build failed.' }
                & go build -o (Join-Path $paths.RepoRoot 'apps\server\bin\zeitboard-mcp.exe') ./cmd/zeitboard-mcp
                if ($LASTEXITCODE -ne 0) { throw 'zeitboard-mcp build failed.' }
            }
            finally { Pop-Location }
        }
    }

    # --- Install (copy build output; preserve previous) --------------------
    $installedExe = Join-Path $paths.InstallDir 'ZeitBoard.exe'
    Invoke-ZbStep -Name 'Install desktop binary' -DryRun:$DryRun -ResumeHint $resume -Action {
        $built = Join-Path $paths.RepoRoot 'apps\desktop\build\bin\ZeitBoard.exe'
        if (-not (Test-Path -LiteralPath $built)) { throw "Build output missing: $built" }
        New-Item -ItemType Directory -Force -Path $paths.InstallDir | Out-Null
        if (Test-Path -LiteralPath $installedExe) {
            $prev = Join-Path $paths.InstallDir 'previous'
            New-Item -ItemType Directory -Force -Path $prev | Out-Null
            Copy-Item -LiteralPath $installedExe -Destination (Join-Path $prev 'ZeitBoard.exe') -Force
        }
        Copy-Item -LiteralPath $built -Destination $installedExe -Force
        $stamp = Get-ZbVersionStamp
        "$($stamp.Commit)  $($stamp.Date)" | Set-Content -Encoding UTF8 -LiteralPath (Join-Path $paths.InstallDir 'version.txt')
        Write-ZbLog -Level ok -Message "installed $($stamp.Commit) to $installedExe"
    }
    if ($WithMcp) {
        Invoke-ZbStep -Name 'Install MCP connector' -DryRun:$DryRun -ResumeHint $resume -Action {
            $mcp = Join-Path $paths.RepoRoot 'apps\server\bin\zeitboard-mcp.exe'
            Copy-Item -LiteralPath $mcp -Destination (Join-Path $paths.InstallDir 'zeitboard-mcp.exe') -Force
            Write-ZbLog -Level ok -Message "MCP at $(Join-Path $paths.InstallDir 'zeitboard-mcp.exe')"
            Write-ZbLog -Message 'Register it in Claude Desktop per docs/self-hosting.md (Voice Via An MCP Client).'
        }
    }

    # --- Decision tree (behavior) ------------------------------------------
    Invoke-ZbStep -Name 'Configure behavior' -DryRun:$DryRun -ResumeHint $resume -Action {
        $startMenuOverride = $null; if ($PSBoundParameters.ContainsKey('StartMenu')) { $startMenuOverride = [bool]$StartMenu }
        $wantStartMenu = Read-ZbChoice -Question 'Create a Start Menu shortcut?' -Default $true -NonInteractive:$NonInteractive -Override $startMenuOverride
        if ($wantStartMenu) {
            $sm = Join-Path ([Environment]::GetFolderPath('Programs')) 'ZeitBoard.lnk'
            New-ZbShortcut -LinkPath $sm -TargetPath $installedExe -Description 'ZeitBoard - planner for free-running rhythms'
        }

        $desktopOverride = $null; if ($PSBoundParameters.ContainsKey('DesktopShortcut')) { $desktopOverride = [bool]$DesktopShortcut }
        $wantDesktop = Read-ZbChoice -Question 'Create a Desktop shortcut?' -Default $false -NonInteractive:$NonInteractive -Override $desktopOverride
        if ($wantDesktop) {
            $dt = Join-Path ([Environment]::GetFolderPath('Desktop')) 'ZeitBoard.lnk'
            New-ZbShortcut -LinkPath $dt -TargetPath $installedExe -Description 'ZeitBoard'
        }

        $startupOverride = $null; if ($PSBoundParameters.ContainsKey('Startup')) { $startupOverride = [bool]$Startup }
        $wantStartup = Read-ZbChoice -Question 'Launch ZeitBoard at Windows startup (to tray)?' -Default $false -NonInteractive:$NonInteractive -Override $startupOverride
        Set-ZbStartupEntry -TargetPath $installedExe -Enabled $wantStartup
    }

    # --- Optional delegates ------------------------------------------------
    if ($WithServer) {
        Invoke-ZbStep -Name 'Self-hosted server' -DryRun:$DryRun -ResumeHint $resume -Action {
            $serverScript = Join-Path $paths.Installer 'install-server.ps1'
            & $serverScript -NonInteractive:$NonInteractive -DryRun:$DryRun
        }
    }
    if ($WithAndroid) {
        Invoke-ZbStep -Name 'Android APK' -DryRun:$DryRun -ResumeHint $resume -Action {
            $androidScript = Join-Path $paths.Installer 'build-android.ps1'
            & $androidScript -NonInteractive:$NonInteractive -DryRun:$DryRun
        }
    }

    # --- Smoke test --------------------------------------------------------
    Invoke-ZbStep -Name 'Smoke test' -DryRun:$DryRun -ResumeHint $resume -Check { $DryRun } -Action {
        if (-not (Test-Path -LiteralPath $installedExe)) { throw 'installed binary missing after install.' }
        Write-ZbLog -Level ok -Message 'installed binary present; data directory untouched'
    }

    # --- Launch ------------------------------------------------------------
    if (-not $DryRun) {
        $launchOverride = $null; if ($PSBoundParameters.ContainsKey('NoLaunch')) { $launchOverride = -not [bool]$NoLaunch }
        $wantLaunch = Read-ZbChoice -Question 'Launch ZeitBoard now?' -Default $true -NonInteractive:$NonInteractive -Override $launchOverride
        if ($wantLaunch) { Start-Process -FilePath $installedExe }
    }

    Show-ZbFinale -Kind install
    Write-Host '   Launch:  ZeitBoard from the Start Menu (or it is already up)' -ForegroundColor Green
    Write-Host "   Update:  scripts\installer\update.ps1" -ForegroundColor Green
    Write-Host '=================================================================' -ForegroundColor Cyan
}
catch {
    Write-Host ''
    Write-ZbLog -Level fail -Message 'Installation did not complete.'
    exit 1
}
exit 0

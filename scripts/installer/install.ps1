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
    -WithMcp               install the MCP connector (existing installs update it)
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
    [switch]$WithAndroid,
    [switch]$AcceptAndroidLicenses,
    [switch]$AllowDirty
)

$ErrorActionPreference = 'Stop'
. "$PSScriptRoot\_zb.common.ps1"

$resume = "powershell -ExecutionPolicy Bypass -File `"$PSCommandPath`""
Start-ZbLog -Name 'install' -DryRun:$DryRun | Out-Null
Show-ZbBanner
$paths = Get-ZbPaths
Write-ZbLog -Message "repo:    $($paths.RepoRoot)"
Write-ZbLog -Message "install: $($paths.InstallDir)"
Write-ZbLog -Message "data:    $($paths.DataDir) (never modified by this installer)"
if ($DryRun) { Write-ZbLog -Level warn -Message 'DRY RUN - no changes will be made.' }
$startMenuOverride = $null; if ($PSBoundParameters.ContainsKey('StartMenu')) { $startMenuOverride = [bool]$StartMenu }
$desktopOverride = $null; if ($PSBoundParameters.ContainsKey('DesktopShortcut')) { $desktopOverride = [bool]$DesktopShortcut }
$startupOverride = $null; if ($PSBoundParameters.ContainsKey('Startup')) { $startupOverride = [bool]$Startup }
$androidLicensesSupplied = $PSBoundParameters.ContainsKey('AcceptAndroidLicenses')
$installedMcp = Join-Path $paths.InstallDir 'zeitboard-mcp.exe'
$installedLocalMcp = Join-Path $paths.InstallDir 'zeitboard-local-mcp.exe'
$installMcp = [bool]$WithMcp -or (Test-Path -LiteralPath $installedMcp)
$lifecycleLock = $null
$exitCode = 0
try {
    $lifecycleLock = Enter-ZbLifecycleLock
    # --- Preflight ---------------------------------------------------------
    Invoke-ZbStep -Name 'Preflight checks' -DryRun:$DryRun -ResumeHint $resume -Action {
        $os = [Environment]::OSVersion.Version
        if ($os.Major -lt 10 -or ($os.Major -eq 10 -and $os.Build -lt 17763)) { throw "Windows 10 1809+ required (found $os)." }
        if (-not (Test-ZbCommand 'git')) { throw 'git not found. Install Git for Windows, then re-run.' }
        $gitVersion = (& git --version) 2>&1 | Out-String
        if ($gitVersion -notmatch 'git version ([0-9]+)\.([0-9]+)') {
            throw "Could not parse git version: $gitVersion"
        }
        if ([int]$Matches[1] -lt 2 -or ([int]$Matches[1] -eq 2 -and [int]$Matches[2] -lt 40)) {
            throw "Git 2.40+ required (found $($gitVersion.Trim()))."
        }
        Assert-ZbRepoClean -AllowDirty:$AllowDirty
        if ($WithServer) {
            $elevated = ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
            if (-not $elevated) { throw '-WithServer requires an Administrator PowerShell so the service can be registered before desktop installation begins.' }
        }
        $free = (Get-PSDrive -Name ($paths.RepoRoot.Substring(0, 1))).Free
        if ($free -lt 2GB) { Write-ZbLog -Level warn -Message 'less than 2 GB free on the repo drive.' }
        Write-ZbLog -Message "Windows $os, $($gitVersion.Trim())"
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
    Invoke-ZbStep -Name 'Build desktop-local MCP bridge (go build)' -DryRun:$DryRun -ResumeHint $resume -Action {
        Push-Location (Join-Path $paths.RepoRoot 'apps\desktop')
        try {
            $desktopBin = Join-Path $paths.RepoRoot 'apps\desktop\build\bin'
            New-Item -ItemType Directory -Force -Path $desktopBin | Out-Null
            & go build -o (Join-Path $desktopBin 'zeitboard-local-mcp.exe') ./cmd/zeitboard-local-mcp
            if ($LASTEXITCODE -ne 0) { throw 'zeitboard-local-mcp build failed.' }
        }
        finally { Pop-Location }
    }
    if ($installMcp) {
        Invoke-ZbStep -Name 'Build MCP connector (go build)' -DryRun:$DryRun -ResumeHint $resume -Action {
            Push-Location (Join-Path $paths.RepoRoot 'apps\server')
            try {
                New-Item -ItemType Directory -Force -Path (Join-Path $paths.RepoRoot 'apps\server\bin') | Out-Null
                & go build -o (Join-Path $paths.RepoRoot 'apps\server\bin\zeitboard-mcp.exe') ./cmd/zeitboard-mcp
                if ($LASTEXITCODE -ne 0) { throw 'zeitboard-mcp build failed.' }
            }
            finally { Pop-Location }
        }
    }
    # --- Publish verified build outputs; preserve previous -----------------
    $installedExe = Join-Path $paths.InstallDir 'ZeitBoard.exe'
    $hadInstalledExe = Test-Path -LiteralPath $installedExe
    $hadInstalledMcp = Test-Path -LiteralPath $installedMcp
    $hadInstalledLocalMcp = Test-Path -LiteralPath $installedLocalMcp
    $orphanMcpBackup = $null
    $orphanMcpHash = $null
    $orphanLocalMcpBackup = $null
    $orphanLocalMcpHash = $null
    if (-not $DryRun -and -not $hadInstalledExe -and $hadInstalledMcp) {
        $orphanMcpBackup = Join-Path $paths.InstallDir ('.install-rollback-mcp-' + [guid]::NewGuid().ToString('N') + '.exe')
        Copy-Item -LiteralPath $installedMcp -Destination $orphanMcpBackup
        $orphanMcpHash = (Get-FileHash -Algorithm SHA256 -LiteralPath $orphanMcpBackup).Hash.ToLowerInvariant()
    }
    if (-not $DryRun -and -not $hadInstalledExe -and $hadInstalledLocalMcp) {
        $orphanLocalMcpBackup = Join-Path $paths.InstallDir ('.install-rollback-local-mcp-' + [guid]::NewGuid().ToString('N') + '.exe')
        Copy-Item -LiteralPath $installedLocalMcp -Destination $orphanLocalMcpBackup
        $orphanLocalMcpHash = (Get-FileHash -Algorithm SHA256 -LiteralPath $orphanLocalMcpBackup).Hash.ToLowerInvariant()
    }
    $script:ZbInstallDesktopPublished = $false
    try {
        Invoke-ZbStep -Name 'Publish desktop binary' -DryRun:$DryRun -ResumeHint $resume -Action {
            $built = Join-Path $paths.RepoRoot 'apps\desktop\build\bin\ZeitBoard.exe'
            $stamp = Get-ZbVersionStamp
            Publish-ZbDesktopBuild -SourceExe $built -InstallDir $paths.InstallDir -VersionText "commit=$($stamp.Commit)`ndate=$($stamp.Date)"
            $script:ZbInstallDesktopPublished = $true
            Write-ZbLog -Level ok -Message "installed $($stamp.Commit) to $installedExe"
        }
        Invoke-ZbStep -Name 'Install desktop-local MCP bridge' -DryRun:$DryRun -ResumeHint $resume -Action {
            $localMcpSource = Join-Path $paths.RepoRoot 'apps\desktop\build\bin\zeitboard-local-mcp.exe'
            $localMcpBackup = Join-Path $paths.InstallDir 'previous\zeitboard-local-mcp.exe'
            $localMcpHash = Publish-ZbVerifiedFile -SourcePath $localMcpSource -DestinationPath $installedLocalMcp -BackupPath $localMcpBackup
            Set-ZbInstalledArtifactHash -InstallDir $paths.InstallDir -Key 'local-mcp-sha256' -Hash $localMcpHash
            Write-ZbLog -Level ok -Message "desktop-local MCP bridge at $installedLocalMcp"
        }
        if ($installMcp) {
            Invoke-ZbStep -Name 'Install MCP connector' -DryRun:$DryRun -ResumeHint $resume -Action {
                $mcpSource = Join-Path $paths.RepoRoot 'apps\server\bin\zeitboard-mcp.exe'
                $mcpBackup = Join-Path $paths.InstallDir 'previous\zeitboard-mcp.exe'
                $mcpHash = Publish-ZbVerifiedFile -SourcePath $mcpSource -DestinationPath $installedMcp -BackupPath $mcpBackup
                Set-ZbInstalledArtifactHash -InstallDir $paths.InstallDir -Key 'mcp-sha256' -Hash $mcpHash
                Write-ZbLog -Level ok -Message "MCP at $installedMcp"
                Write-ZbLog -Message 'Register it in Claude Desktop per docs/self-hosting.md (Voice Via An MCP Client).'
            }
        }
        Invoke-ZbStep -Name 'Verify published artifacts' -DryRun:$DryRun -ResumeHint $resume -Action {
            if (-not (Test-ZbInstalledBuild -InstallDir $paths.InstallDir)) { throw 'An installed artifact failed its SHA-256 verification.' }
        }
    }
    catch {
        $publishError = $_
        $restoreFailure = $null
        if ($script:ZbInstallDesktopPublished) {
            $previousExe = Join-Path $paths.InstallDir 'previous\ZeitBoard.exe'
            if ($hadInstalledExe) {
                try {
                    if (-not (Test-Path -LiteralPath $previousExe)) { throw "Previous desktop build is missing: $previousExe" }
                    Restore-ZbPreviousBuild -InstallDir $paths.InstallDir
                    if (-not (Test-ZbInstalledBuild -InstallDir $paths.InstallDir)) {
                        throw 'The automatically restored build failed its SHA-256 verification.'
                    }
                }
                catch { $restoreFailure = $_ }
            }
            else {
                Remove-Item -LiteralPath $installedExe -Force -ErrorAction SilentlyContinue
                Remove-Item -LiteralPath (Join-Path $paths.InstallDir 'version.txt') -Force -ErrorAction SilentlyContinue
                if ($hadInstalledLocalMcp) {
                    try {
                        if (-not $orphanLocalMcpBackup -or -not (Test-Path -LiteralPath $orphanLocalMcpBackup)) {
                            throw 'The pre-existing desktop-local MCP rollback copy is missing.'
                        }
                        Publish-ZbVerifiedFile -SourcePath $orphanLocalMcpBackup -DestinationPath $installedLocalMcp -BackupPath (Join-Path $paths.InstallDir 'previous\failed-current-local-mcp.exe') | Out-Null
                        $restoredLocalMcpHash = (Get-FileHash -Algorithm SHA256 -LiteralPath $installedLocalMcp).Hash.ToLowerInvariant()
                        if ($restoredLocalMcpHash -ne $orphanLocalMcpHash) { throw 'The restored desktop-local MCP bridge failed its SHA-256 verification.' }
                    }
                    catch { if (-not $restoreFailure) { $restoreFailure = $_ } }
                }
                else {
                    Remove-Item -LiteralPath $installedLocalMcp -Force -ErrorAction SilentlyContinue
                }
                if ($hadInstalledMcp) {
                    try {
                        if (-not $orphanMcpBackup -or -not (Test-Path -LiteralPath $orphanMcpBackup)) {
                            throw 'The pre-existing MCP rollback copy is missing.'
                        }
                        Publish-ZbVerifiedFile -SourcePath $orphanMcpBackup -DestinationPath $installedMcp -BackupPath (Join-Path $paths.InstallDir 'previous\failed-current-mcp.exe') | Out-Null
                        $restoredMcpHash = (Get-FileHash -Algorithm SHA256 -LiteralPath $installedMcp).Hash.ToLowerInvariant()
                        if ($restoredMcpHash -ne $orphanMcpHash) { throw 'The restored MCP connector failed its SHA-256 verification.' }
                    }
                    catch { if (-not $restoreFailure) { $restoreFailure = $_ } }
                }
                else {
                    Remove-Item -LiteralPath $installedMcp -Force -ErrorAction SilentlyContinue
                }
            }
        }
        if ($restoreFailure) {
            Write-ZbLog -Level fail -Message "automatic restore also failed: $($restoreFailure.Exception.Message)"
            throw "Artifact publication failed ($($publishError.Exception.Message)); restoring the prior install also failed: $($restoreFailure.Exception.Message)"
        }
        throw $publishError
    }
    finally {
        if ($orphanMcpBackup -and (Test-Path -LiteralPath $orphanMcpBackup)) {
            Remove-Item -LiteralPath $orphanMcpBackup -Force -ErrorAction SilentlyContinue
        }
        if ($orphanLocalMcpBackup -and (Test-Path -LiteralPath $orphanLocalMcpBackup)) {
            Remove-Item -LiteralPath $orphanLocalMcpBackup -Force -ErrorAction SilentlyContinue
        }
    }

    # --- Decision tree (behavior) ------------------------------------------
    Invoke-ZbStep -Name 'Configure behavior' -DryRun:$DryRun -ResumeHint $resume -Action {
        $wantStartMenu = Read-ZbChoice -Question 'Create a Start Menu shortcut?' -Default $true -NonInteractive:$NonInteractive -Override $startMenuOverride
        if ($wantStartMenu) {
            $sm = Join-Path ([Environment]::GetFolderPath('Programs')) 'ZeitBoard.lnk'
            New-ZbShortcut -LinkPath $sm -TargetPath $installedExe -Description 'ZeitBoard - planner for free-running rhythms'
        }

        $wantDesktop = Read-ZbChoice -Question 'Create a Desktop shortcut?' -Default $false -NonInteractive:$NonInteractive -Override $desktopOverride
        if ($wantDesktop) {
            $dt = Join-Path ([Environment]::GetFolderPath('Desktop')) 'ZeitBoard.lnk'
            New-ZbShortcut -LinkPath $dt -TargetPath $installedExe -Description 'ZeitBoard'
        }

        $wantStartup = Read-ZbChoice -Question 'Launch ZeitBoard at Windows startup (to tray)?' -Default $false -NonInteractive:$NonInteractive -Override $startupOverride
        Set-ZbStartupEntry -TargetPath $installedExe -Enabled $wantStartup
    }

    # --- Optional delegates ------------------------------------------------
    if ($WithServer) {
        Invoke-ZbStep -Name 'Self-hosted server' -DryRun:$DryRun -ResumeHint $resume -Action {
            $serverScript = Join-Path $paths.Installer 'install-server.ps1'
            & $serverScript -NonInteractive:$NonInteractive
            if ($LASTEXITCODE -ne 0) { throw 'server installer failed.' }
        }
    }
    if ($WithAndroid) {
        Invoke-ZbStep -Name 'Android APK' -DryRun:$DryRun -ResumeHint $resume -Action {
            $androidScript = Join-Path $paths.Installer 'build-android.ps1'
            if ($androidLicensesSupplied) {
                & $androidScript -NonInteractive:$NonInteractive -AcceptAndroidLicenses:$AcceptAndroidLicenses
            }
            else {
                & $androidScript -NonInteractive:$NonInteractive
            }
            if ($LASTEXITCODE -ne 0) { throw 'Android builder failed.' }
        }
    }

    # --- Smoke test --------------------------------------------------------
    Invoke-ZbStep -Name 'Installed binary integrity' -DryRun:$DryRun -ResumeHint $resume -Action {
        if (-not (Test-ZbInstalledBuild -InstallDir $paths.InstallDir)) { throw 'installed binary hash does not match version.txt; roll back or reinstall.' }
        Write-ZbLog -Level ok -Message 'installed binary hash verified; data directory untouched'
    }

    # --- Launch ------------------------------------------------------------
    if (-not $DryRun) {
        $launchOverride = $null; if ($PSBoundParameters.ContainsKey('NoLaunch')) { $launchOverride = -not [bool]$NoLaunch }
        $wantLaunch = Read-ZbChoice -Question 'Launch ZeitBoard now?' -Default $true -NonInteractive:$NonInteractive -Override $launchOverride
        if ($wantLaunch) { Start-Process -FilePath $installedExe }
    }

    if ($DryRun) {
        Write-ZbLog -Level ok -Message 'dry-run plan complete; no files, shortcuts, startup entries, or services were changed'
    }
    else {
        Show-ZbFinale -Kind install
        Write-Host '   Launch:  ZeitBoard from the Start Menu (or it is already up)' -ForegroundColor Green
        Write-Host "   Update:  scripts\installer\update.ps1" -ForegroundColor Green
        Write-Host '=================================================================' -ForegroundColor Cyan
    }
}
catch {
    Write-Host ''
    Write-ZbLog -Level fail -Message 'Installation did not complete.'
    Write-ZbLog -Level fail -Message $_.Exception.Message
    $exitCode = 1
}
finally {
    Exit-ZbLifecycleLock -Mutex $lifecycleLock
}
exit $exitCode

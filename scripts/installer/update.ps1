<#
.SYNOPSIS
  Update an installed ZeitBoard to the current repo commit, with a data backup
  and one-command rollback. Design: docs/install-update-design.md.

.DESCRIPTION
  Fetches and fast-forwards the repository, restarts itself under any newly
  pulled installer code, validates the build with tests (unless -SkipTests),
  verifies the desktop build, and only then requires the desktop and MCP
  processes to be stopped before data backup and atomic executable publication.
  Data is never rolled back automatically.

    -Rollback     restore previous\ and exit (no rebuild)
    -SkipTests    skip the Go + frontend suites (faster, less safe)
    -WithMcp      install/update the MCP connector (existing installs update it)
    -AllowDirty   proceed with an uncommitted working tree (prints the diff)
    -NonInteractive / -DryRun   as in install.ps1

.EXAMPLE
  powershell -ExecutionPolicy Bypass -File scripts\installer\update.ps1
#>
[CmdletBinding()]
param(
    [switch]$Rollback,
    [switch]$SkipTests,
    [switch]$WithMcp,
    [switch]$AllowDirty,
    [switch]$NonInteractive,
    [switch]$DryRun
)

$ErrorActionPreference = 'Stop'
. "$PSScriptRoot\_zb.common.ps1"

$resume = "powershell -ExecutionPolicy Bypass -File `"$PSCommandPath`""
Start-ZbLog -Name 'update' -DryRun:$DryRun | Out-Null
Show-ZbBanner
$paths = Get-ZbPaths
$installedExe = Join-Path $paths.InstallDir 'ZeitBoard.exe'
$previousExe = Join-Path $paths.InstallDir 'previous\ZeitBoard.exe'
$installedMcp = Join-Path $paths.InstallDir 'zeitboard-mcp.exe'
$installedLocalMcp = Join-Path $paths.InstallDir 'zeitboard-local-mcp.exe'
$updateMcp = [bool]$WithMcp -or (Test-Path -LiteralPath $installedMcp)
$script:ZbPulled = $false
$script:ZbDesktopPublished = $false
$lifecycleLock = $null

# --- Rollback path ---------------------------------------------------------
if ($Rollback) {
    $rollbackExitCode = 0
    try {
        $lifecycleLock = Enter-ZbLifecycleLock
        Invoke-ZbStep -Name 'Rollback to previous build' -DryRun:$DryRun -ResumeHint $resume -Action {
            Restore-ZbPreviousBuild -InstallDir $paths.InstallDir
            if (-not (Test-ZbInstalledBuild -InstallDir $paths.InstallDir)) {
                throw 'The restored build failed its SHA-256 verification.'
            }
            Write-ZbLog -Message 'Data was not changed. Restore a backup zip separately only when that is explicitly intended.'
        }
        if ($DryRun) {
            Write-ZbLog -Level ok -Message 'dry-run rollback plan complete; no files were changed'
        }
        else {
            Show-ZbFinale -Kind update
        }
    }
    catch {
        Write-ZbLog -Level fail -Message $_.Exception.Message
        $rollbackExitCode = 1
    }
    finally {
        Exit-ZbLifecycleLock -Mutex $lifecycleLock
    }
    exit $rollbackExitCode
}

$exitCode = 0
try {
    $lifecycleLock = Enter-ZbLifecycleLock
    Invoke-ZbStep -Name 'Preflight' -DryRun:$DryRun -ResumeHint $resume -Action {
        if (-not (Test-Path -LiteralPath $installedExe)) {
            throw "ZeitBoard is not installed yet. Run install.ps1 first."
        }
        if (-not (Test-ZbInstalledBuild -InstallDir $paths.InstallDir)) {
            $recovery = if (Test-Path -LiteralPath $previousExe) { 'Run update.ps1 -Rollback.' } else { 'Run install.ps1 again.' }
            throw "An installed ZeitBoard artifact does not match version.txt; the prior update may have been interrupted. $recovery"
        }
        if (-not (Test-ZbCommand 'git')) { throw 'git not found.' }
        Assert-ZbRepoClean -AllowDirty:$AllowDirty
    }

    Invoke-ZbStep -Name 'Check for a newer commit' -DryRun:$DryRun -ResumeHint $resume -Action {
        Push-Location $paths.RepoRoot
        try {
            $before = (Get-ZbVersionStamp).Commit
            & git fetch --quiet
            if ($LASTEXITCODE -ne 0) { throw 'git fetch failed.' }
            $upstream = (& git rev-parse --abbrev-ref --symbolic-full-name '@{u}') 2>&1 | Out-String
            if ($LASTEXITCODE -ne 0) { throw 'This branch has no upstream tracking branch; configure one before updating.' }
            $behindText = (& git rev-list --count 'HEAD..@{u}') 2>&1 | Out-String
            if ($LASTEXITCODE -ne 0) { throw 'Could not compare the current branch with its upstream.' }
            $behind = 0
            if (-not [int]::TryParse($behindText.Trim(), [ref]$behind)) { throw "Unexpected git rev-list result: $behindText" }
            if ($behind -eq 0) {
                Write-ZbLog -Level ok -Message "already at the latest tracked commit ($before)"
            }
            else {
                Write-ZbLog -Message "upstream is $behind commit(s) ahead"
                $go = Read-ZbChoice -Question 'Pull and rebuild?' -Default $true -NonInteractive:$NonInteractive
                if (-not $go) { throw 'Update declined by user.' }
                & git pull --ff-only
                if ($LASTEXITCODE -ne 0) { throw 'git pull --ff-only failed (non-fast-forward?). Resolve manually.' }
                $script:ZbPulled = $true
            }
        }
        finally { Pop-Location }
    }

    if ($script:ZbPulled) {
        Write-ZbLog -Message 'restarting under the installer code from the pulled commit'
        Exit-ZbLifecycleLock -Mutex $lifecycleLock
        $lifecycleLock = $null
        $restartArgs = @('-NoProfile', '-ExecutionPolicy', 'Bypass', '-File', $PSCommandPath)
        if ($SkipTests) { $restartArgs += '-SkipTests' }
        if ($AllowDirty) { $restartArgs += '-AllowDirty' }
        if ($WithMcp) { $restartArgs += '-WithMcp' }
        if ($NonInteractive) { $restartArgs += '-NonInteractive' }
        & powershell.exe @restartArgs
        exit $LASTEXITCODE
    }

    # Dependencies may have moved with the new commit.
    Invoke-ZbStep -Name 'Toolchain' -DryRun:$DryRun -ResumeHint $resume -Action {
        Assert-ZbGo -DryRun:$DryRun; Assert-ZbNode -DryRun:$DryRun; Assert-ZbWails -DryRun:$DryRun
    }

    Invoke-ZbStep -Name 'Install JavaScript dependencies' -DryRun:$DryRun -ResumeHint $resume -Action {
        Push-Location $paths.RepoRoot
        try {
            & npm ci
            if ($LASTEXITCODE -ne 0) { throw 'npm ci failed.' }
        }
        finally { Pop-Location }
    }

    if (-not $SkipTests) {
        Invoke-ZbStep -Name 'Run test suites (gate)' -DryRun:$DryRun -ResumeHint $resume -Action {
            Push-Location $paths.RepoRoot
            try {
                & npm run test --workspace '@zeitboard/desktop-frontend' -- --run
                if ($LASTEXITCODE -ne 0) { throw 'frontend tests failed - aborting update.' }
            }
            finally { Pop-Location }
            foreach ($mod in @('core', 'apps\desktop', 'apps\server')) {
                Push-Location (Join-Path $paths.RepoRoot $mod)
                try { & go test ./...; if ($LASTEXITCODE -ne 0) { throw "go test failed in $mod - aborting update." } }
                finally { Pop-Location }
            }
        }
    }

    $built = Join-Path $paths.RepoRoot 'apps\desktop\build\bin\ZeitBoard.exe'
    Invoke-ZbStep -Name 'Build desktop' -DryRun:$DryRun -ResumeHint $resume -Action {
        Push-Location (Join-Path $paths.RepoRoot 'apps\desktop')
        try {
            & wails build
            if ($LASTEXITCODE -ne 0) { throw 'wails build failed.' }
        }
        finally { Pop-Location }
        if (-not (Test-Path -LiteralPath $built)) { throw "build output missing: $built" }
    }

    $builtLocalMcp = Join-Path $paths.RepoRoot 'apps\desktop\build\bin\zeitboard-local-mcp.exe'
    Invoke-ZbStep -Name 'Build desktop-local MCP bridge' -DryRun:$DryRun -ResumeHint $resume -Action {
        $binDir = Split-Path -Parent $builtLocalMcp
        New-Item -ItemType Directory -Force -Path $binDir | Out-Null
        Push-Location (Join-Path $paths.RepoRoot 'apps\desktop')
        try {
            & go build -o $builtLocalMcp ./cmd/zeitboard-local-mcp
            if ($LASTEXITCODE -ne 0) { throw 'zeitboard-local-mcp build failed.' }
        }
        finally { Pop-Location }
    }

    $builtMcp = Join-Path $paths.RepoRoot 'apps\server\bin\zeitboard-mcp.exe'
    if ($updateMcp) {
        Invoke-ZbStep -Name 'Build MCP connector' -DryRun:$DryRun -ResumeHint $resume -Action {
            $binDir = Split-Path -Parent $builtMcp
            New-Item -ItemType Directory -Force -Path $binDir | Out-Null
            Push-Location (Join-Path $paths.RepoRoot 'apps\server')
            try {
                & go build -o $builtMcp ./cmd/zeitboard-mcp
                if ($LASTEXITCODE -ne 0) { throw 'zeitboard-mcp build failed.' }
            }
            finally { Pop-Location }
        }
    }

    Invoke-ZbStep -Name 'Verify processes stopped and back up quiesced data' -DryRun:$DryRun -ResumeHint $resume -Action {
        Assert-ZbAppStopped -TargetPath $installedExe
        if (Test-Path -LiteralPath $installedLocalMcp) {
            Assert-ZbExecutableStopped -TargetPath $installedLocalMcp
        }
        if ($updateMcp -and (Test-Path -LiteralPath $installedMcp)) {
            Assert-ZbExecutableStopped -TargetPath $installedMcp
        }
        Backup-ZbData -Reason 'update' | Out-Null
    }

    # Components this release must contain; the pending marker makes an
    # interruption between artifacts detectable rather than silently leaving a
    # new desktop binary beside a stale MCP bridge.
    $publishComponents = @('desktop', 'local-mcp')
    if ($updateMcp) { $publishComponents += 'mcp' }
    try {
        Invoke-ZbStep -Name 'Begin publish transaction' -DryRun:$DryRun -ResumeHint $resume -Action {
            Start-ZbPublishTransaction -InstallDir $paths.InstallDir -Components $publishComponents | Out-Null
        }
        Invoke-ZbStep -Name 'Publish desktop build' -DryRun:$DryRun -ResumeHint $resume -Action {
            Publish-ZbDesktopBuild -SourceExe $built -InstallDir $paths.InstallDir -VersionText (Get-ZbVersionText -Components $publishComponents)
            $script:ZbDesktopPublished = $true
        }
        Invoke-ZbStep -Name 'Publish desktop-local MCP bridge' -DryRun:$DryRun -ResumeHint $resume -Action {
            $localMcpHash = Publish-ZbVerifiedFile -SourcePath $builtLocalMcp -DestinationPath $installedLocalMcp -BackupPath (Join-Path $paths.InstallDir 'previous\zeitboard-local-mcp.exe')
            Set-ZbInstalledArtifactHash -InstallDir $paths.InstallDir -Key 'local-mcp-sha256' -Hash $localMcpHash
        }
        if ($updateMcp) {
            Invoke-ZbStep -Name 'Publish MCP connector' -DryRun:$DryRun -ResumeHint $resume -Action {
                $mcpHash = Publish-ZbVerifiedFile -SourcePath $builtMcp -DestinationPath $installedMcp -BackupPath (Join-Path $paths.InstallDir 'previous\zeitboard-mcp.exe')
                Set-ZbInstalledArtifactHash -InstallDir $paths.InstallDir -Key 'mcp-sha256' -Hash $mcpHash
            }
        }
        Invoke-ZbStep -Name 'Verify installed build' -DryRun:$DryRun -ResumeHint $resume -Action {
            # Verify every declared component, then clear the pending marker.
            Complete-ZbPublishTransaction -InstallDir $paths.InstallDir
        }
    }
    catch {
        $publishError = $_
        $restoreFailure = $null
        if ($script:ZbDesktopPublished) {
            Write-ZbLog -Level warn -Message 'artifact publication failed; restoring the previous coherent install'
            try {
                if (-not (Test-Path -LiteralPath $previousExe)) {
                    throw "Previous desktop build is missing: $previousExe"
                }
                Restore-ZbPreviousBuild -InstallDir $paths.InstallDir
                if (-not (Test-ZbInstalledBuild -InstallDir $paths.InstallDir)) {
                    throw 'The automatically restored build failed its SHA-256 verification.'
                }
            }
            catch { $restoreFailure = $_ }
        }
        if ($restoreFailure) {
            Write-ZbLog -Level fail -Message "automatic restore also failed: $($restoreFailure.Exception.Message)"
            throw "Artifact publication failed ($($publishError.Exception.Message)); restoring the prior install also failed: $($restoreFailure.Exception.Message)"
        }
        throw $publishError
    }

    if ($DryRun) {
        Write-ZbLog -Level ok -Message 'dry-run update plan complete; no files were changed'
    }
    else {
        Show-ZbFinale -Kind update
        $stamp = Get-ZbVersionStamp
        Write-Host "   Now at: $($stamp.Commit)  $($stamp.Date)" -ForegroundColor Green
        Write-Host "   Roll back: scripts\installer\update.ps1 -Rollback" -ForegroundColor Green
        Write-Host '=================================================================' -ForegroundColor Cyan
    }
}
catch {
    Write-Host ''
    Write-ZbLog -Level fail -Message 'Update did not complete.'
    Write-ZbLog -Level fail -Message $_.Exception.Message
    $exitCode = 1
}
finally {
    Exit-ZbLifecycleLock -Mutex $lifecycleLock
}
exit $exitCode

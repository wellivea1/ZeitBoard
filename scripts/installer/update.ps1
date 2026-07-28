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
    -ForceRebuild rebuild even when the verified installed release matches HEAD
    -WithServerMcp install/update the optional self-hosted/server MCP connector (-WithMcp remains an alias)
    -AllowDirty   proceed with an uncommitted working tree (prints the diff)
    -NonInteractive / -DryRun   as in install.ps1

.EXAMPLE
  powershell -ExecutionPolicy Bypass -File scripts\installer\update.ps1
#>
[CmdletBinding()]
param(
    [switch]$Rollback,
    [switch]$SkipTests,
    [switch]$ForceRebuild,
    [Alias('WithMcp')][switch]$WithServerMcp,
    [switch]$AllowDirty,
    [switch]$NonInteractive,
    [switch]$DryRun
)

$ErrorActionPreference = 'Stop'
. "$PSScriptRoot\_zb.common.ps1"

$resume = "powershell -ExecutionPolicy Bypass -File `"$PSCommandPath`""
if ($ForceRebuild) { $resume += ' -ForceRebuild' }
Start-ZbLog -Name 'update' -DryRun:$DryRun | Out-Null
Show-ZbBanner
$paths = Get-ZbPaths
$installedExe = Join-Path $paths.InstallDir 'ZeitBoard.exe'
$previousExe = Join-Path $paths.InstallDir 'previous\ZeitBoard.exe'
$installedMcp = Join-Path $paths.InstallDir 'zeitboard-mcp.exe'
$installedLocalMcp = Join-Path $paths.InstallDir 'zeitboard-local-mcp.exe'
$updateMcp = [bool]$WithServerMcp -or (Test-Path -LiteralPath $installedMcp)
$publishComponents = @('desktop', 'local-mcp')
if ($updateMcp) { $publishComponents += 'mcp' }
$script:ZbPulled = $false
$script:ZbDesktopPublished = $false
$lifecycleLock = $null

function Get-ZbInstalledReleaseMetadata {
    param([Parameter(Mandatory)][string]$InstallDir)

    $versionPath = Join-Path $InstallDir 'version.txt'
    if (-not (Test-Path -LiteralPath $versionPath -PathType Leaf)) { return $null }
    $lines = @(Get-Content -LiteralPath $versionPath)
    $commitLines = @($lines | Where-Object { $_ -match '^commit=' })
    $componentLines = @($lines | Where-Object { $_ -match '^components=' })
    if ($commitLines.Count -ne 1 -or $componentLines.Count -ne 1) { return $null }

    $commit = $commitLines[0].Substring(7).Trim()
    $componentText = $componentLines[0].Substring(11).Trim()
    if ([string]::IsNullOrWhiteSpace($commit) -or [string]::IsNullOrWhiteSpace($componentText)) {
        return $null
    }
    $rawComponents = @($componentText -split ',')
    $components = @($rawComponents | ForEach-Object { $_.Trim().ToLowerInvariant() } | Where-Object { $_ })
    $uniqueComponents = @($components | Sort-Object -Unique)
    if ($components.Count -eq 0 -or $components.Count -ne $rawComponents.Count -or
        $uniqueComponents.Count -ne $components.Count) {
        return $null
    }
    [pscustomobject]@{ Commit = $commit; Components = $components }
}

function Get-ZbUpdateDecision {
    # Pure release comparison so no-op behavior can be tested without fetching
    # a repository or invoking any dependency, test, build, or publish command.
    param(
        [Parameter(Mandatory)][bool]$InstalledBuildVerified,
        [string]$InstalledCommit,
        [string[]]$InstalledComponents = @(),
        [Parameter(Mandatory)][AllowEmptyString()][string]$HeadCommit,
        [string[]]$RequestedComponents = @(),
        [Parameter(Mandatory)][bool]$RepositoryClean,
        [switch]$ForceRebuild
    )

    if ($ForceRebuild) {
        return [pscustomobject]@{ ShouldRebuild = $true; Reason = '-ForceRebuild was specified' }
    }
    if (-not $InstalledBuildVerified) {
        return [pscustomobject]@{ ShouldRebuild = $true; Reason = 'the installed release is not verified' }
    }
    if (-not $RepositoryClean) {
        return [pscustomobject]@{ ShouldRebuild = $true; Reason = 'the working tree contains allowed local changes' }
    }
    if ([string]::IsNullOrWhiteSpace($InstalledCommit)) {
        return [pscustomobject]@{ ShouldRebuild = $true; Reason = 'the installed release has no resolvable commit metadata' }
    }
    if ([string]::IsNullOrWhiteSpace($HeadCommit)) {
        return [pscustomobject]@{ ShouldRebuild = $true; Reason = 'HEAD could not be resolved after fetch' }
    }
    if (-not [string]::Equals($InstalledCommit.Trim(), $HeadCommit.Trim(), [StringComparison]::OrdinalIgnoreCase)) {
        return [pscustomobject]@{ ShouldRebuild = $true; Reason = 'the installed commit differs from HEAD' }
    }

    $installed = @($InstalledComponents | ForEach-Object { "$_".Trim().ToLowerInvariant() } | Where-Object { $_ } | Sort-Object)
    $requested = @($RequestedComponents | ForEach-Object { "$_".Trim().ToLowerInvariant() } | Where-Object { $_ } | Sort-Object)
    if ($installed.Count -ne $requested.Count) {
        return [pscustomobject]@{ ShouldRebuild = $true; Reason = 'the declared component set differs from the requested release' }
    }
    for ($i = 0; $i -lt $requested.Count; $i++) {
        if (-not [string]::Equals($installed[$i], $requested[$i], [StringComparison]::Ordinal)) {
            return [pscustomobject]@{ ShouldRebuild = $true; Reason = 'the declared component set differs from the requested release' }
        }
    }
    if ($requested.Count -eq 0) {
        return [pscustomobject]@{ ShouldRebuild = $true; Reason = 'the declared component set differs from the requested release' }
    }
    [pscustomobject]@{ ShouldRebuild = $false; Reason = 'the verified installed commit and declared components match HEAD' }
}

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
        if ($ForceRebuild) { $restartArgs += '-ForceRebuild' }
        if ($AllowDirty) { $restartArgs += '-AllowDirty' }
        if ($WithServerMcp) { $restartArgs += '-WithServerMcp' }
        if ($NonInteractive) { $restartArgs += '-NonInteractive' }
        & powershell.exe @restartArgs
        exit $LASTEXITCODE
    }

    if ($DryRun) {
        $dryRunDecision = if ($ForceRebuild) {
            '-ForceRebuild would force the full rebuild pipeline.'
        }
        else {
            'A live run would exit here only after fetch when the verified installed commit and exact component set match HEAD.'
        }
        Write-ZbLog -Level info -Message "[dry-run] $dryRunDecision"
    }
    else {
        Push-Location $paths.RepoRoot
        try {
            $headCommit = (& git rev-parse --verify HEAD) 2>&1 | Out-String
            if ($LASTEXITCODE -ne 0) { throw 'Could not resolve HEAD after fetch.' }
            $headCommit = $headCommit.Trim()

            $metadata = Get-ZbInstalledReleaseMetadata -InstallDir $paths.InstallDir
            $resolvedInstalledCommit = ''
            if ($metadata -and $metadata.Commit -match '^[0-9a-fA-F]{7,40}$') {
                $resolved = (& git rev-parse --verify "$($metadata.Commit)^{commit}") 2>&1 | Out-String
                if ($LASTEXITCODE -eq 0) { $resolvedInstalledCommit = $resolved.Trim() }
            }
            $status = (& git status --porcelain) 2>&1 | Out-String
            if ($LASTEXITCODE -ne 0) { throw 'Could not inspect the repository after fetch.' }
            $decision = Get-ZbUpdateDecision `
                -InstalledBuildVerified (Test-ZbInstalledBuild -InstallDir $paths.InstallDir) `
                -InstalledCommit $resolvedInstalledCommit `
                -InstalledComponents $(if ($metadata) { $metadata.Components } else { @() }) `
                -HeadCommit $headCommit `
                -RequestedComponents $publishComponents `
                -RepositoryClean ([string]::IsNullOrWhiteSpace($status)) `
                -ForceRebuild:$ForceRebuild
        }
        finally { Pop-Location }

        if (-not $decision.ShouldRebuild) {
            Write-ZbLog -Level ok -Message "$($decision.Reason); skipping npm, tests, builds, backup, and publication"
            Show-ZbFinale -Kind update
            Write-Host "   Already current: $headCommit" -ForegroundColor Green
            Write-Host '=================================================================' -ForegroundColor Cyan
            exit 0
        }
        Write-ZbLog -Message "rebuild required: $($decision.Reason)"
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
        Invoke-ZbStep -Name 'Build optional self-hosted/server MCP connector' -DryRun:$DryRun -ResumeHint $resume -Action {
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

    # The pending marker makes an interruption between artifacts detectable
    # rather than silently leaving a new desktop binary beside a stale bridge.
    $publishBackupDir = Join-Path $paths.InstallDir ('.publish-backup-' + [guid]::NewGuid().ToString('N'))
    try {
        Invoke-ZbStep -Name 'Begin publish transaction' -DryRun:$DryRun -ResumeHint $resume -Action {
            Start-ZbPublishTransaction -InstallDir $paths.InstallDir -Components $publishComponents | Out-Null
        }
        Invoke-ZbStep -Name 'Publish desktop build' -DryRun:$DryRun -ResumeHint $resume -Action {
            Publish-ZbDesktopBuild -SourceExe $built -InstallDir $paths.InstallDir -VersionText (Get-ZbVersionText -Components $publishComponents)
            $script:ZbDesktopPublished = $true
        }
        Invoke-ZbStep -Name 'Publish desktop-local MCP bridge' -DryRun:$DryRun -ResumeHint $resume -Action {
            $localMcpHash = Publish-ZbVerifiedFile -SourcePath $builtLocalMcp -DestinationPath $installedLocalMcp -BackupPath (Join-Path $publishBackupDir 'zeitboard-local-mcp.exe')
            Set-ZbInstalledArtifactHash -InstallDir $paths.InstallDir -Key 'local-mcp-sha256' -Hash $localMcpHash
        }
        if ($updateMcp) {
            Invoke-ZbStep -Name 'Publish optional self-hosted/server MCP connector' -DryRun:$DryRun -ResumeHint $resume -Action {
                $mcpHash = Publish-ZbVerifiedFile -SourcePath $builtMcp -DestinationPath $installedMcp -BackupPath (Join-Path $publishBackupDir 'zeitboard-mcp.exe')
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
        $pendingMarker = Get-ZbPendingMarkerPath -InstallDir $paths.InstallDir
        if ((Test-ZbInstalledBuild -InstallDir $paths.InstallDir -IgnorePendingMarker -IgnorePendingComponents) -and (Test-Path -LiteralPath $pendingMarker)) {
            Remove-Item -LiteralPath $pendingMarker -Force
        }
        throw $publishError
    }
    finally {
        if (Test-Path -LiteralPath $publishBackupDir) {
            Remove-ZbDirectoryUnderRoot -Root $paths.InstallDir -Path $publishBackupDir
        }
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

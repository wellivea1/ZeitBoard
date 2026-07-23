<#
.SYNOPSIS
  Update an installed ZeitBoard to the current repo commit, with a data backup
  and one-command rollback. Design: docs/install-update-design.md.

.DESCRIPTION
  Backs up the data directory ALWAYS before touching anything, moves the
  current install to previous\, rebuilds, runs the test suites (unless
  -SkipTests), and swaps in. A failed test run auto-restores previous\.
  Data is never rolled back automatically - the schema is additive-only.

    -Rollback     restore previous\ and exit (no rebuild)
    -SkipTests    skip the Go + frontend suites (faster, less safe)
    -AllowDirty   proceed with an uncommitted working tree (prints the diff)
    -NonInteractive / -DryRun   as in install.ps1

.EXAMPLE
  powershell -ExecutionPolicy Bypass -File scripts\installer\update.ps1
#>
[CmdletBinding()]
param(
    [switch]$Rollback,
    [switch]$SkipTests,
    [switch]$AllowDirty,
    [switch]$NonInteractive,
    [switch]$DryRun
)

$ErrorActionPreference = 'Stop'
. "$PSScriptRoot\_zb.common.ps1"

$resume = "powershell -ExecutionPolicy Bypass -File `"$PSCommandPath`""
Start-ZbLog -Name 'update' | Out-Null
Show-ZbBanner
$paths = Get-ZbPaths
$installedExe = Join-Path $paths.InstallDir 'ZeitBoard.exe'
$previousExe = Join-Path $paths.InstallDir 'previous\ZeitBoard.exe'

# --- Rollback path ---------------------------------------------------------
if ($Rollback) {
    Invoke-ZbStep -Name 'Rollback to previous build' -DryRun:$DryRun -ResumeHint $resume -Action {
        if (-not (Test-Path -LiteralPath $previousExe)) { throw "No previous build to roll back to ($previousExe)." }
        Copy-Item -LiteralPath $previousExe -Destination $installedExe -Force
        Write-ZbLog -Level ok -Message 'restored the previous ZeitBoard.exe'
        Write-ZbLog -Message 'Data was not changed. If you need an earlier data state, restore a backup zip from the log dir.'
    }
    Show-ZbFinale -Kind update
    exit 0
}

try {
    Invoke-ZbStep -Name 'Preflight' -DryRun:$DryRun -ResumeHint $resume -Action {
        if (-not (Test-Path -LiteralPath $installedExe)) {
            throw "ZeitBoard is not installed yet. Run install.ps1 first."
        }
        if (-not (Test-ZbCommand 'git')) { throw 'git not found.' }
        Assert-ZbRepoClean -AllowDirty:$AllowDirty
    }

    Invoke-ZbStep -Name 'Check for a newer commit' -DryRun:$DryRun -ResumeHint $resume -Action {
        Push-Location $paths.RepoRoot
        try {
            $before = (Get-ZbVersionStamp).Commit
            & git fetch --quiet
            $behind = (& git rev-list --count "HEAD..@{u}") 2>&1 | Out-String
            if ($behind.Trim() -eq '0') {
                Write-ZbLog -Level ok -Message "already at the latest tracked commit ($before)"
            }
            else {
                Write-ZbLog -Message "upstream is $($behind.Trim()) commit(s) ahead"
                $go = Read-ZbChoice -Question "Pull and rebuild?" -Default $true -NonInteractive:$NonInteractive
                if (-not $go) { throw 'Update declined by user.' }
                & git pull --ff-only
                if ($LASTEXITCODE -ne 0) { throw 'git pull --ff-only failed (non-fast-forward?). Resolve manually.' }
            }
        }
        finally { Pop-Location }
    }

    # Backup BEFORE any build/swap.
    Invoke-ZbStep -Name 'Back up data' -DryRun:$DryRun -ResumeHint $resume -Action { Backup-ZbData -Reason 'update' | Out-Null }

    # Dependencies may have moved with the new commit.
    Invoke-ZbStep -Name 'Toolchain' -DryRun:$DryRun -ResumeHint $resume -Action {
        Assert-ZbGo -DryRun:$DryRun; Assert-ZbNode -DryRun:$DryRun; Assert-ZbWails -DryRun:$DryRun
    }

    if (-not $SkipTests) {
        Invoke-ZbStep -Name 'Run test suites (gate)' -DryRun:$DryRun -ResumeHint $resume -Action {
            Push-Location $paths.RepoRoot
            try {
                & npm ci; if ($LASTEXITCODE -ne 0) { throw 'npm ci failed.' }
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

    Invoke-ZbStep -Name 'Preserve current build' -DryRun:$DryRun -ResumeHint $resume -Action {
        $prev = Join-Path $paths.InstallDir 'previous'
        New-Item -ItemType Directory -Force -Path $prev | Out-Null
        Copy-Item -LiteralPath $installedExe -Destination (Join-Path $prev 'ZeitBoard.exe') -Force
    }

    $swapFailed = $false
    try {
        Invoke-ZbStep -Name 'Rebuild and swap in' -DryRun:$DryRun -ResumeHint $resume -Action {
            Push-Location $paths.RepoRoot
            try { & npm ci; if ($LASTEXITCODE -ne 0) { throw 'npm ci failed.' } }
            finally { Pop-Location }
            Push-Location (Join-Path $paths.RepoRoot 'apps\desktop')
            try { & wails build; if ($LASTEXITCODE -ne 0) { throw 'wails build failed.' } }
            finally { Pop-Location }
            $built = Join-Path $paths.RepoRoot 'apps\desktop\build\bin\ZeitBoard.exe'
            if (-not (Test-Path -LiteralPath $built)) { throw "build output missing: $built" }
            Copy-Item -LiteralPath $built -Destination $installedExe -Force
            $stamp = Get-ZbVersionStamp
            "$($stamp.Commit)  $($stamp.Date)" | Set-Content -Encoding UTF8 -LiteralPath (Join-Path $paths.InstallDir 'version.txt')
        }
    }
    catch {
        $swapFailed = $true
        Write-ZbLog -Level warn -Message 'rebuild/swap failed - auto-restoring the previous build'
        if (Test-Path -LiteralPath $previousExe) { Copy-Item -LiteralPath $previousExe -Destination $installedExe -Force }
        throw
    }

    Invoke-ZbStep -Name 'Smoke test' -DryRun:$DryRun -ResumeHint $resume -Check { $DryRun } -Action {
        if (-not (Test-Path -LiteralPath $installedExe)) { throw 'installed binary missing after swap.' }
    }

    Show-ZbFinale -Kind update
    if (-not $DryRun) {
        $stamp = Get-ZbVersionStamp
        Write-Host "   Now at: $($stamp.Commit)  $($stamp.Date)" -ForegroundColor Green
        Write-Host "   Roll back: scripts\installer\update.ps1 -Rollback" -ForegroundColor Green
        Write-Host '=================================================================' -ForegroundColor Cyan
    }
}
catch {
    Write-Host ''
    Write-ZbLog -Level fail -Message 'Update did not complete.'
    exit 1
}
exit 0

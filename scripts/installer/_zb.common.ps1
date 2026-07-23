# _zb.common.ps1 - shared support library for the ZeitBoard install/update
# tooling (docs/install-update-design.md). Dot-source this first from every
# entry script:  . "$PSScriptRoot\_zb.common.ps1"
#
# Windows PowerShell 5.1 is the floor: no ternary, no ?., no &&, no null
# coalescing. Nothing here acts on load - only function/variable definitions.
# StrictMode is deliberately NOT set here: this file is dot-sourced, so it must
# not change the calling scope's strictness. Entry scripts opt in themselves.

# --- Paths -----------------------------------------------------------------

function Get-ZbPaths {
    # The single source of truth for every location the tooling touches.
    $installerDir = $PSScriptRoot
    $scriptsDir = Split-Path -Parent $installerDir
    $repoRoot = Split-Path -Parent $scriptsDir
    $localAppData = [Environment]::GetFolderPath('LocalApplicationData')
    $appData = [Environment]::GetFolderPath('ApplicationData')
    [pscustomobject]@{
        RepoRoot   = $repoRoot
        Installer  = $installerDir
        Tools      = Join-Path $repoRoot '.tools'
        ToolsBin   = Join-Path $repoRoot '.tools\bin'
        InstallDir = Join-Path $localAppData 'Programs\ZeitBoard'
        DataDir    = Join-Path $appData 'ZeitBoard'
        LogDir     = Join-Path $env:TEMP 'zeitboard-install'
        Pins       = Join-Path $installerDir 'pins.psd1'
    }
}

function Get-ZbPins {
    $paths = Get-ZbPaths
    if (-not (Test-Path -LiteralPath $paths.Pins)) {
        throw "Pin manifest not found: $($paths.Pins)"
    }
    Import-PowerShellDataFile -LiteralPath $paths.Pins
}

function Test-ZbPins {
    # Structural check (used by the test runner and CI): every Url/Sha256Url is
    # https, and every downloadable entry carries exactly one integrity source.
    # Returns a list of problem strings; empty means valid.
    param([hashtable]$Pins)
    if (-not $Pins) { $Pins = Get-ZbPins }
    $problems = @()
    foreach ($name in $Pins.Keys) {
        $entry = $Pins[$name]
        if ($entry -isnot [hashtable]) { continue }
        if (-not $entry.ContainsKey('Url')) { continue } # system-only entry
        if ($entry.Url -notmatch '^https://') { $problems += "$name.Url is not https" }
        $hasLiteral = $entry.ContainsKey('Sha256') -and $entry.Sha256
        $hasUrl = $entry.ContainsKey('Sha256Url') -and $entry.Sha256Url
        if ($hasLiteral -and $hasUrl) { $problems += "$name has both Sha256 and Sha256Url (pick one)" }
        if (-not $hasLiteral -and -not $hasUrl) { $problems += "$name has neither Sha256 nor Sha256Url" }
        if ($hasUrl -and $entry.Sha256Url -notmatch '^https://') { $problems += "$name.Sha256Url is not https" }
    }
    return $problems
}

# --- Logging ---------------------------------------------------------------

$script:ZbLogFile = $null

function Start-ZbLog {
    param([string]$Name = 'install')
    $paths = Get-ZbPaths
    New-Item -ItemType Directory -Force -Path $paths.LogDir | Out-Null
    $stamp = Get-Date -Format 'yyyyMMdd-HHmmss'
    $script:ZbLogFile = Join-Path $paths.LogDir "$Name-$stamp.log"
    "ZeitBoard $Name log - $(Get-Date -Format o)" | Set-Content -Encoding UTF8 -LiteralPath $script:ZbLogFile
    $script:ZbLogFile
}

function Write-ZbLog {
    param(
        [Parameter(Mandatory)][string]$Message,
        [ValidateSet('info', 'warn', 'fail', 'ok', 'step')][string]$Level = 'info'
    )
    $prefixes = @{ info = '  '; warn = '! '; fail = 'X '; ok = '+ '; step = '> ' }
    $colors = @{ info = 'Gray'; warn = 'Yellow'; fail = 'Red'; ok = 'Green'; step = 'Cyan' }
    Write-Host "$($prefixes[$Level])$Message" -ForegroundColor $colors[$Level]
    if ($script:ZbLogFile) {
        "[$Level] $Message" | Add-Content -Encoding UTF8 -LiteralPath $script:ZbLogFile
    }
}

# --- Step runner -----------------------------------------------------------

function Invoke-ZbStep {
    <#
      Runs one named phase. -Check {} returning $true marks the step already
      satisfied (skipped). -DryRun prints the plan without running -Action.
      On failure: last log lines + the resume command, then rethrow.
    #>
    param(
        [Parameter(Mandatory)][string]$Name,
        [Parameter(Mandatory)][scriptblock]$Action,
        [scriptblock]$Check,
        [switch]$DryRun,
        [string]$ResumeHint
    )
    Write-ZbLog -Level step -Message $Name
    if ($Check) {
        $already = $false
        try { $already = [bool](& $Check) } catch { $already = $false }
        if ($already) {
            Write-ZbLog -Level ok -Message "already satisfied - skipped"
            return
        }
    }
    if ($DryRun) {
        Write-ZbLog -Level info -Message "[dry-run] would run: $Name"
        return
    }
    $start = Get-Date
    try {
        & $Action
        $elapsed = [int]((Get-Date) - $start).TotalSeconds
        Write-ZbLog -Level ok -Message "done (${elapsed}s)"
    }
    catch {
        Write-ZbLog -Level fail -Message "step failed: $Name"
        Write-ZbLog -Level fail -Message $_.Exception.Message
        if ($script:ZbLogFile) {
            Write-Host ''
            Write-Host '--- last log lines ---' -ForegroundColor DarkGray
            Get-Content -LiteralPath $script:ZbLogFile -Tail 20 | ForEach-Object { Write-Host $_ -ForegroundColor DarkGray }
            Write-Host "--- full log: $script:ZbLogFile ---" -ForegroundColor DarkGray
        }
        if ($ResumeHint) {
            Write-Host ''
            Write-ZbLog -Level warn -Message "resume with: $ResumeHint"
        }
        throw
    }
}

# --- Choices (interactive, flag-overridable, CI-safe) ----------------------

function Read-ZbChoice {
    <#
      Yes/no question. Precedence: an explicit -Override (from a flag) wins;
      then -NonInteractive returns -Default; otherwise prompt. This lets every
      decision-tree branch be answered from the command line or a CI run.
    #>
    param(
        [Parameter(Mandatory)][string]$Question,
        [bool]$Default = $false,
        [switch]$NonInteractive,
        [Nullable[bool]]$Override = $null
    )
    if ($null -ne $Override) { return [bool]$Override }
    if ($NonInteractive) { return $Default }
    if ($Default) { $hint = '[Y/n]' } else { $hint = '[y/N]' }
    $answer = Read-Host "$Question $hint"
    if ([string]::IsNullOrWhiteSpace($answer)) { return $Default }
    return @('y', 'yes') -contains $answer.Trim().ToLowerInvariant()
}

# --- Dependency probing and installation -----------------------------------

function Add-ZbToolPath {
    param([Parameter(Mandatory)][string]$Directory)
    if (Test-Path -LiteralPath $Directory) {
        # Prepend to the PROCESS PATH only - the machine is never polluted.
        if (($env:PATH -split ';') -notcontains $Directory) {
            $env:PATH = "$Directory;$env:PATH"
        }
    }
}

function Get-ZbExpectedHash {
    param([Parameter(Mandatory)][hashtable]$Pin)
    if ($Pin.ContainsKey('Sha256') -and $Pin.Sha256) {
        if ($Pin.Sha256 -like 'REPLACE_*') {
            throw "Pin '$($Pin.Version)' has an unverified placeholder checksum. Fill Sha256 in pins.psd1 (see the file header) before installing this component."
        }
        return $Pin.Sha256.ToLowerInvariant()
    }
    if ($Pin.ContainsKey('Sha256Url') -and $Pin.Sha256Url) {
        $text = (Invoke-WebRequest -UseBasicParsing -Uri $Pin.Sha256Url).Content
        $match = ''
        if ($Pin.ContainsKey('Match')) { $match = $Pin.Match }
        if ([string]::IsNullOrEmpty($match)) {
            # Bare "<hash>" (optionally "<hash>  <file>") - take the first token.
            return (($text.Trim() -split '\s+') | Select-Object -First 1).ToLowerInvariant()
        }
        $escaped = [regex]::Escape($match)
        $line = ($text -split "`n") | Where-Object { $_ -match "\s$escaped\s*$" -or $_ -match "\b$escaped\b" } | Select-Object -First 1
        if (-not $line) { throw "No checksum line for '$match' in $($Pin.Sha256Url)." }
        return (($line.Trim() -split '\s+') | Select-Object -First 1).ToLowerInvariant()
    }
    throw "Pin is missing both Sha256 and Sha256Url."
}

function Install-ZbArchivePin {
    <#
      Downloads Pin.Url into .tools\, verifies SHA-256 (literal or vendor
      checksum file), extracts into .tools\<DirName>, returns that directory.
      Idempotent: an existing DirName is returned without re-downloading.
    #>
    param([Parameter(Mandatory)][hashtable]$Pin)
    $paths = Get-ZbPaths
    $target = Join-Path $paths.Tools $Pin.DirName
    if (Test-Path -LiteralPath $target) { return $target }

    New-Item -ItemType Directory -Force -Path $paths.Tools | Out-Null
    $fileName = Split-Path -Leaf ([Uri]$Pin.Url).AbsolutePath
    $archive = Join-Path $paths.Tools $fileName
    Write-ZbLog -Message "downloading $fileName"
    Invoke-WebRequest -UseBasicParsing -Uri $Pin.Url -OutFile $archive

    $expected = Get-ZbExpectedHash -Pin $Pin
    $actual = (Get-FileHash -Algorithm SHA256 -LiteralPath $archive).Hash.ToLowerInvariant()
    if ($actual -ne $expected) {
        Remove-Item -LiteralPath $archive -Force -ErrorAction SilentlyContinue
        throw "Checksum mismatch for $fileName. Expected $expected, got $actual. Refusing to install."
    }
    Write-ZbLog -Level ok -Message "checksum verified"

    # Extract to a temp dir, then normalize to DirName (archives with a single
    # top-level folder are unwrapped so the layout is stable).
    $temp = Join-Path $paths.Tools ("_extract-" + [guid]::NewGuid().ToString('N'))
    Expand-Archive -LiteralPath $archive -DestinationPath $temp -Force
    $entries = Get-ChildItem -LiteralPath $temp
    if ($entries.Count -eq 1 -and $entries[0].PSIsContainer -and $Pin.DirName -ne $entries[0].Name) {
        Move-Item -LiteralPath $entries[0].FullName -Destination $target
        Remove-Item -LiteralPath $temp -Recurse -Force
    }
    else {
        Move-Item -LiteralPath $temp -Destination $target
    }
    Remove-Item -LiteralPath $archive -Force -ErrorAction SilentlyContinue
    return $target
}

function Test-ZbCommand {
    param([Parameter(Mandatory)][string]$Name)
    $null -ne (Get-Command $Name -ErrorAction SilentlyContinue)
}

function Assert-ZbGo {
    # .tools\go pin first, then an acceptable system Go, else download the pin.
    param([switch]$DryRun)
    $paths = Get-ZbPaths
    $localGoBin = Join-Path $paths.Tools 'go\bin'
    if (Test-Path (Join-Path $localGoBin 'go.exe')) { Add-ZbToolPath $localGoBin }
    if (Test-ZbCommand 'go') {
        $v = (& go version) 2>&1 | Out-String
        if ($v -match 'go1\.26\.') { Write-ZbLog -Level ok -Message "Go $($Matches[0])"; return }
    }
    if ($DryRun) { Write-ZbLog -Message '[dry-run] would fetch pinned Go'; return }
    $pins = Get-ZbPins
    $dir = Install-ZbArchivePin -Pin $pins.Go
    Add-ZbToolPath (Join-Path $dir 'bin')
    if (-not (Test-ZbCommand 'go')) { throw 'Go install failed - go not on PATH after extraction.' }
}

function Assert-ZbNode {
    param([switch]$DryRun)
    $paths = Get-ZbPaths
    $pins = Get-ZbPins
    $localNode = Join-Path $paths.Tools $pins.Node.DirName
    if (Test-Path (Join-Path $localNode 'node.exe')) { Add-ZbToolPath $localNode; Write-ZbLog -Level ok -Message "Node (vendored)"; return }
    if (Test-ZbCommand 'node') {
        $v = (& node --version) 2>&1 | Out-String
        if ($v.Trim() -eq $pins.Node.Version) { Write-ZbLog -Level ok -Message "Node $($v.Trim())"; return }
    }
    if ($DryRun) { Write-ZbLog -Message '[dry-run] would fetch pinned Node'; return }
    $dir = Install-ZbArchivePin -Pin $pins.Node
    Add-ZbToolPath $dir
}

function Assert-ZbWails {
    param([switch]$DryRun)
    $paths = Get-ZbPaths
    $wails = Join-Path $paths.ToolsBin 'wails.exe'
    if (Test-Path -LiteralPath $wails) { Add-ZbToolPath $paths.ToolsBin; Write-ZbLog -Level ok -Message "Wails (vendored)"; return }
    if ($DryRun) { Write-ZbLog -Message '[dry-run] would go install the pinned Wails CLI'; return }
    $pins = Get-ZbPins
    New-Item -ItemType Directory -Force -Path $paths.ToolsBin | Out-Null
    $env:GOBIN = $paths.ToolsBin
    & go install $pins.Wails.Module
    if ($LASTEXITCODE -ne 0 -or -not (Test-Path -LiteralPath $wails)) { throw 'Wails CLI install failed.' }
    Add-ZbToolPath $paths.ToolsBin
}

function Assert-ZbWebView2 {
    param([switch]$DryRun, [switch]$NonInteractive, [Nullable[bool]]$MachineWide = $null)
    # WebView2 Evergreen is detected via its client registry key (per-user or
    # machine). Windows 11 ships it, so this is usually a no-op.
    $keys = @(
        'HKCU:\Software\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}',
        'HKLM:\SOFTWARE\WOW6432Node\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}',
        'HKLM:\SOFTWARE\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}'
    )
    foreach ($k in $keys) {
        if (Test-Path $k) {
            $pv = (Get-ItemProperty -Path $k -ErrorAction SilentlyContinue).pv
            if ($pv) { Write-ZbLog -Level ok -Message "WebView2 Runtime $pv"; return }
        }
    }
    Write-ZbLog -Level warn -Message 'WebView2 Runtime not detected.'
    if ($DryRun) { Write-ZbLog -Message '[dry-run] would offer a per-user WebView2 install'; return }
    $go = Read-ZbChoice -Question 'Install the WebView2 Runtime now (per-user)?' -Default $true -NonInteractive:$NonInteractive
    if (-not $go) { Write-ZbLog -Level warn -Message 'Skipping WebView2 - the desktop app will not render without it.'; return }
    if (Test-ZbCommand 'winget') {
        & winget install --id Microsoft.EdgeWebView2Runtime --scope user --accept-source-agreements --accept-package-agreements
        if ($LASTEXITCODE -eq 0) { Write-ZbLog -Level ok -Message 'WebView2 installed via winget'; return }
    }
    Write-ZbLog -Level warn -Message 'Install WebView2 manually: https://developer.microsoft.com/microsoft-edge/webview2/  (Evergreen Standalone, per-user).'
}

# --- Repo / version helpers ------------------------------------------------

function Assert-ZbRepoClean {
    param([switch]$AllowDirty)
    $paths = Get-ZbPaths
    Push-Location $paths.RepoRoot
    try {
        $status = (& git status --porcelain) 2>&1 | Out-String
        if ($status.Trim()) {
            if ($AllowDirty) {
                Write-ZbLog -Level warn -Message 'working tree is dirty (-AllowDirty set):'
                $status.Trim().Split("`n") | ForEach-Object { Write-ZbLog -Level warn -Message "  $_" }
            }
            else {
                throw "Working tree is not clean. Commit/stash first, or pass -AllowDirty. Changes:`n$status"
            }
        }
    }
    finally { Pop-Location }
}

function Get-ZbVersionStamp {
    $paths = Get-ZbPaths
    Push-Location $paths.RepoRoot
    try {
        $commit = (& git rev-parse --short HEAD) 2>&1 | Out-String
        $date = (& git show -s --format=%ci HEAD) 2>&1 | Out-String
        [pscustomobject]@{ Commit = $commit.Trim(); Date = $date.Trim() }
    }
    finally { Pop-Location }
}

function Backup-ZbData {
    # Zip the data dir (db + -wal/-shm sidecars + config) before any update.
    param([string]$Reason = 'update')
    $paths = Get-ZbPaths
    if (-not (Test-Path -LiteralPath $paths.DataDir)) {
        Write-ZbLog -Message 'no data directory yet - nothing to back up'
        return $null
    }
    New-Item -ItemType Directory -Force -Path $paths.LogDir | Out-Null
    $stamp = Get-Date -Format 'yyyyMMdd-HHmmss'
    $zip = Join-Path $paths.LogDir "zeitboard-data-$Reason-$stamp.zip"
    Compress-Archive -Path (Join-Path $paths.DataDir '*') -DestinationPath $zip -Force
    Write-ZbLog -Level ok -Message "data backed up: $zip"
    $zip
}

# --- Shortcuts / startup ---------------------------------------------------

function New-ZbShortcut {
    param(
        [Parameter(Mandatory)][string]$LinkPath,
        [Parameter(Mandatory)][string]$TargetPath,
        [string]$Description = 'ZeitBoard'
    )
    New-Item -ItemType Directory -Force -Path (Split-Path -Parent $LinkPath) | Out-Null
    $shell = New-Object -ComObject WScript.Shell
    $lnk = $shell.CreateShortcut($LinkPath)
    $lnk.TargetPath = $TargetPath
    $lnk.WorkingDirectory = Split-Path -Parent $TargetPath
    $lnk.Description = $Description
    $lnk.Save()
    [Runtime.InteropServices.Marshal]::ReleaseComObject($shell) | Out-Null
    Write-ZbLog -Level ok -Message "shortcut: $LinkPath"
}

function Set-ZbStartupEntry {
    # HKCU Run key add/remove. The app starts to tray (matches its tray Start/
    # Quit controls). Idempotent both ways. -RunKey is overridable so tests can
    # exercise the round-trip against a sandbox key.
    param(
        [Parameter(Mandatory)][string]$TargetPath,
        [bool]$Enabled = $true,
        [string]$RunKey = 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Run'
    )
    $runKey = $RunKey
    if (-not (Test-Path $runKey)) { New-Item -Path $runKey -Force | Out-Null }
    $name = 'ZeitBoard'
    if ($Enabled) {
        Set-ItemProperty -Path $runKey -Name $name -Value "`"$TargetPath`"" -Force
        Write-ZbLog -Level ok -Message 'startup launch enabled (HKCU Run)'
    }
    else {
        if ((Get-ItemProperty -Path $runKey -Name $name -ErrorAction SilentlyContinue)) {
            Remove-ItemProperty -Path $runKey -Name $name -Force
            Write-ZbLog -Level ok -Message 'startup launch disabled'
        }
    }
}

# --- Banners (ASCII-only for legacy consoles) ------------------------------

function Show-ZbBanner {
    $lines = @(
        '=================================================================',
        '  ZZZZZ EEEEE IIIII TTTTT BBBB   OOO   AAA  RRRR  DDDD',
        '     Z  E       I     T   B   B O   O A   A R   R D   D',
        '    Z   EEEE    I     T   BBBB  O   O AAAAA RRRR  D   D',
        '   Z    E       I     T   B   B O   O A   A R  R  D   D',
        '  ZZZZZ EEEEE IIIII   T   BBBB   OOO  A   A R   R DDDD',
        '              a planner for free-running rhythms',
        '================================================================='
    )
    Write-Host ''
    $lines | ForEach-Object { Write-Host $_ -ForegroundColor Cyan }
    Write-Host ''
}

function Show-ZbFinale {
    param([ValidateSet('install', 'update', 'android', 'server', 'uninstall')][string]$Kind = 'install')
    $marquee = @{
        install   = '~ your rhythm, your clock ~'
        update    = '~ up to date! ~'
        android   = '~ APK READY ~'
        server    = '~ server ready ~'
        uninstall = '~ removed ~'
    }
    Write-Host ''
    Write-Host '        .   *        .       *          .        *' -ForegroundColor DarkYellow
    Write-Host '   *        .   ________________   .          .' -ForegroundColor DarkYellow
    Write-Host '       .       /                \        *' -ForegroundColor Yellow
    Write-Host '  ------------|  Z E I T B O A R D  |------------------' -ForegroundColor Yellow
    Write-Host "               \___ $($Kind)! ___/" -ForegroundColor Yellow
    Write-Host "        (  -_-) zzZ   $($marquee[$Kind])  (^-^ )" -ForegroundColor Green
    Write-Host '   .        *        .        *       .        *      .' -ForegroundColor DarkYellow
    if ($Kind -eq 'install' -or $Kind -eq 'update') {
        Write-Host '   Every day is a little longer. Now your planner knows it.' -ForegroundColor Green
    }
    Write-Host '=================================================================' -ForegroundColor Cyan
}

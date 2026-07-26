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
    $pins = Import-PowerShellDataFile -LiteralPath $paths.Pins
    $problems = @(Test-ZbPins -Pins $pins)
    if ($problems.Count -gt 0) {
        throw "Pin manifest is invalid:`n - $($problems -join "`n - ")"
    }
    return $pins
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
        if ($hasLiteral -and $entry.Sha256 -notmatch '^[0-9a-fA-F]{64}$') {
            $problems += "$name.Sha256 is not a verified 64-character SHA-256"
        }
        $dirName = [string]$entry.DirName
        if (-not $entry.ContainsKey('DirName') -or [string]::IsNullOrWhiteSpace($dirName)) {
            $problems += "$name.DirName is required"
        }
        elseif ($dirName -notmatch '^[A-Za-z0-9][A-Za-z0-9._+-]*$') {
            $problems += "$name.DirName must be one safe directory name"
        }
        $probePath = [string]$entry.ProbePath
        if (-not $entry.ContainsKey('ProbePath') -or [string]::IsNullOrWhiteSpace($probePath)) {
            $problems += "$name.ProbePath is required"
        }
        elseif ([IO.Path]::IsPathRooted($probePath) -or $probePath -match '[*?]' -or @($probePath -split '[\\/]' | Where-Object { $_ -eq '..' }).Count -gt 0) {
            $problems += "$name.ProbePath must stay inside the extracted tool directory"
        }
    }
    return $problems
}

# --- Logging ---------------------------------------------------------------

$script:ZbLogFile = $null

function Set-ZbUtf8File {
    # Windows PowerShell 5.1's `Set-Content -Encoding UTF8` writes a BOM,
    # which Go's strict JSON decoder rejects. Use BOM-free UTF-8 explicitly.
    param(
        [Parameter(Mandatory)][string]$Path,
        [Parameter(Mandatory)][AllowEmptyString()][string]$Content
    )
    $encoding = New-Object System.Text.UTF8Encoding($false)
    [IO.File]::WriteAllText($Path, $Content, $encoding)
}

function Start-ZbLog {
    param([string]$Name = 'install', [switch]$DryRun)
    if ($DryRun) {
        $script:ZbLogFile = $null
        return $null
    }
    $paths = Get-ZbPaths
    New-Item -ItemType Directory -Force -Path $paths.LogDir | Out-Null
    $stamp = "$(Get-Date -Format 'yyyyMMdd-HHmmssfff')-$PID"
    $script:ZbLogFile = Join-Path $paths.LogDir "$Name-$stamp.log"
    Set-ZbUtf8File -Path $script:ZbLogFile -Content "ZeitBoard $Name log - $(Get-Date -Format o)`n"
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

function Enter-ZbLifecycleLock {
    $mutex = New-Object Threading.Mutex($false, 'Local\ZeitBoardInstallerLifecycle')
    $acquired = $false
    try {
        try { $acquired = $mutex.WaitOne(0) }
        catch [Threading.AbandonedMutexException] { $acquired = $true }
        if (-not $acquired) {
            throw 'Another ZeitBoard install, update, Android build, server install, or uninstall is already running in this Windows session.'
        }
        return $mutex
    }
    catch {
        if (-not $acquired) { $mutex.Dispose() }
        throw
    }
}

function Exit-ZbLifecycleLock {
    param([Threading.Mutex]$Mutex)
    if (-not $Mutex) { return }
    try {
        $Mutex.ReleaseMutex()
    }
    finally {
        $Mutex.Dispose()
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
        $hash = $Pin.Sha256.ToLowerInvariant()
        if ($hash -notmatch '^[0-9a-f]{64}$') {
            throw "Pin '$($Pin.Version)' does not contain a valid 64-character SHA-256."
        }
        return $hash
    }
    if ($Pin.ContainsKey('Sha256Url') -and $Pin.Sha256Url) {
        $text = (Invoke-WebRequest -UseBasicParsing -Uri $Pin.Sha256Url).Content
        $match = ''
        if ($Pin.ContainsKey('Match')) { $match = $Pin.Match }
        if ([string]::IsNullOrEmpty($match)) {
            # Bare "<hash>" (optionally "<hash>  <file>") - take the first token.
            $hash = (($text.Trim() -split '\s+') | Select-Object -First 1).ToLowerInvariant()
            if ($hash -notmatch '^[0-9a-f]{64}$') { throw 'Vendor checksum response did not contain a SHA-256.' }
            return $hash
        }
        $escaped = [regex]::Escape($match)
        $line = ($text -split "`n") | Where-Object { $_ -match "\s$escaped\s*$" -or $_ -match "\b$escaped\b" } | Select-Object -First 1
        if (-not $line) { throw "No checksum line for '$match' in $($Pin.Sha256Url)." }
        $hash = (($line.Trim() -split '\s+') | Select-Object -First 1).ToLowerInvariant()
        if ($hash -notmatch '^[0-9a-f]{64}$') { throw "Checksum line for '$match' did not contain a SHA-256." }
        return $hash
    }
    throw "Pin is missing both Sha256 and Sha256Url."
}

function Install-ZbArchivePin {
    <#
      Downloads Pin.Url into .tools\, verifies SHA-256 (literal or vendor
      checksum file), and renames a fully extracted payload into place.
      A complete existing install is reused; an incomplete one is quarantined.
    #>
    param([Parameter(Mandatory)][hashtable]$Pin)
    $paths = Get-ZbPaths
    $target = Join-Path $paths.Tools $Pin.DirName
    $probe = if ($Pin.ContainsKey('ProbePath')) { Join-Path $target $Pin.ProbePath } else { $target }
    if (Test-Path -LiteralPath $target) {
        if (Test-Path -LiteralPath $probe -PathType Leaf) { return $target }
        $quarantine = "$target.incomplete-$(Get-Date -Format 'yyyyMMddHHmmssfff')-$([guid]::NewGuid().ToString('N').Substring(0, 8))"
        Write-ZbLog -Level warn -Message "incomplete tool directory found; preserving it at $quarantine"
        Move-Item -LiteralPath $target -Destination $quarantine
    }

    New-Item -ItemType Directory -Force -Path $paths.Tools | Out-Null
    $fileName = Split-Path -Leaf ([Uri]$Pin.Url).AbsolutePath
    $archive = Join-Path $paths.Tools $fileName
    $temp = Join-Path $paths.Tools ("_extract-" + [guid]::NewGuid().ToString('N'))
    try {
        Write-ZbLog -Message "downloading $fileName"
        Invoke-WebRequest -UseBasicParsing -Uri $Pin.Url -OutFile $archive

        $expected = Get-ZbExpectedHash -Pin $Pin
        $actual = (Get-FileHash -Algorithm SHA256 -LiteralPath $archive).Hash.ToLowerInvariant()
        if ($actual -ne $expected) {
            throw "Checksum mismatch for $fileName. Expected $expected, got $actual. Refusing to install."
        }
        Write-ZbLog -Level ok -Message 'checksum verified'

        Expand-Archive -LiteralPath $archive -DestinationPath $temp -Force
        Move-ZbExtractedArchive -ExtractedPath $temp -TargetPath $target -ProbePath $Pin.ProbePath
        $installedProbe = if ($Pin.ContainsKey('ProbePath')) { Join-Path $target $Pin.ProbePath } else { $target }
        if (-not (Test-Path -LiteralPath $installedProbe -PathType Leaf)) {
            throw "Extracted $fileName but expected tool is missing: $installedProbe"
        }
        return $target
    }
    finally {
        if (Test-Path -LiteralPath $archive) { Remove-Item -LiteralPath $archive -Force -ErrorAction SilentlyContinue }
        if (Test-Path -LiteralPath $temp) { Remove-Item -LiteralPath $temp -Recurse -Force -ErrorAction SilentlyContinue }
    }
}

function Move-ZbExtractedArchive {
    param(
        [Parameter(Mandatory)][string]$ExtractedPath,
        [Parameter(Mandatory)][string]$TargetPath,
        [string]$ProbePath
    )
    $entries = @(Get-ChildItem -LiteralPath $ExtractedPath -Force)
    $payload = $ExtractedPath
    if ($entries.Count -eq 1 -and $entries[0].PSIsContainer) {
        # Node, Go, and Temurin name their one top-level directory exactly like
        # our target. Move that directory itself; moving the extraction parent
        # would otherwise create target\target\... and hide the executable.
        $payload = $entries[0].FullName
    }
    if (-not [string]::IsNullOrWhiteSpace($ProbePath)) {
        $probe = Join-Path $payload $ProbePath
        if (-not (Test-Path -LiteralPath $probe -PathType Leaf)) {
            throw "Extracted payload is missing its expected tool before publication: $probe"
        }
    }
    Move-Item -LiteralPath $payload -Destination $TargetPath
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
    if (Test-Path (Join-Path $localNode 'node.exe')) {
        Add-ZbToolPath $localNode
        $v = (& node --version) 2>&1 | Out-String
        if ($v.Trim() -eq $pins.Node.Version) { Write-ZbLog -Level ok -Message "Node $($v.Trim()) (vendored)"; return }
        throw "Vendored Node at $localNode is not $($pins.Node.Version). Remove or repair that directory."
    }
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
    $pins = Get-ZbPins
    $wails = Join-Path $paths.ToolsBin 'wails.exe'
    if (Test-Path -LiteralPath $wails) {
        Add-ZbToolPath $paths.ToolsBin
        $previousPreference = $ErrorActionPreference
        try {
            $ErrorActionPreference = 'Continue'
            $output = @(& $wails version 2>&1)
            $exitCode = $LASTEXITCODE
        }
        finally {
            $ErrorActionPreference = $previousPreference
        }
        $reported = if ($output.Count -gt 0) { [string]$output[0] } else { '' }
        if ($exitCode -eq 0 -and $reported.Trim() -eq $pins.Wails.Version) {
            Write-ZbLog -Level ok -Message "Wails $($pins.Wails.Version) (vendored)"
            return
        }
        Write-ZbLog -Level warn -Message "vendored Wails is not $($pins.Wails.Version); it will be replaced"
    }
    if ($DryRun) { Write-ZbLog -Message '[dry-run] would go install the pinned Wails CLI'; return }
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
    if (-not $go) { throw 'WebView2 is required for the desktop app. Install it manually, then rerun the installer.' }
    if (Test-ZbCommand 'winget') {
        & winget install --id Microsoft.EdgeWebView2Runtime --scope user --accept-source-agreements --accept-package-agreements
        if ($LASTEXITCODE -eq 0) { Write-ZbLog -Level ok -Message 'WebView2 installed via winget'; return }
    }
    throw 'WebView2 installation did not complete. Install Evergreen Standalone from https://developer.microsoft.com/microsoft-edge/webview2/ and rerun the installer.'
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
    # Zip the entire data directory, including SQLite sidecars and hidden files.
    param(
        [string]$Reason = 'update',
        [string]$SourceDir,
        [string]$DestinationDir
    )
    $paths = Get-ZbPaths
    if ([string]::IsNullOrWhiteSpace($SourceDir)) { $SourceDir = $paths.DataDir }
    if ([string]::IsNullOrWhiteSpace($DestinationDir)) { $DestinationDir = $paths.LogDir }
    if (-not (Test-Path -LiteralPath $SourceDir)) {
        Write-ZbLog -Message 'no data directory yet - nothing to back up'
        return $null
    }
    New-Item -ItemType Directory -Force -Path $DestinationDir | Out-Null
    $stamp = Get-Date -Format 'yyyyMMdd-HHmmssfff'
    $zip = Join-Path $DestinationDir "zeitboard-data-$Reason-$stamp.zip"
    Add-Type -AssemblyName System.IO.Compression.FileSystem
    [IO.Compression.ZipFile]::CreateFromDirectory(
        $SourceDir,
        $zip,
        [IO.Compression.CompressionLevel]::Optimal,
        $false
    )
    Write-ZbLog -Level ok -Message "data backed up: $zip"
    $zip
}
function Assert-ZbExecutableStopped {
    param(
        [Parameter(Mandatory)][string]$TargetPath,
        [string]$ProcessName = ([IO.Path]::GetFileNameWithoutExtension($TargetPath)),
        [string]$Message
    )
    $target = [IO.Path]::GetFullPath($TargetPath)
    $running = @(Get-Process -Name $ProcessName -ErrorAction SilentlyContinue | Where-Object {
        try { $_.Path -and ([IO.Path]::GetFullPath($_.Path) -eq $target) } catch { $false }
    })
    if ($running.Count -gt 0) {
        if ([string]::IsNullOrWhiteSpace($Message)) { $Message = "$ProcessName is still running from $TargetPath. Stop it and retry." }
        throw $Message
    }
}

function Assert-ZbAppStopped {
    param([Parameter(Mandatory)][string]$TargetPath)
    Assert-ZbExecutableStopped -TargetPath $TargetPath -ProcessName 'ZeitBoard' -Message 'ZeitBoard is still running (closing the window leaves it in the tray). Choose Quit from the tray icon, then run this command again.'
}

function Get-ZbCommandExecutable {
    param([Parameter(Mandatory)][string]$CommandLine)
    $trimmed = $CommandLine.Trim()
    if ($trimmed.StartsWith('"')) {
        $closingQuote = $trimmed.IndexOf('"', 1)
        if ($closingQuote -lt 2) { throw "Invalid quoted service command line: $CommandLine" }
        return $trimmed.Substring(1, $closingQuote - 1)
    }
    $match = [regex]::Match($trimmed, '^\S+')
    if (-not $match.Success) { throw 'Service command line does not contain an executable path.' }
    return $match.Value
}

function Get-ZbServiceExecutablePath {
    param([Parameter(Mandatory)][string]$ServiceName)
    $service = Get-CimInstance -ClassName Win32_Service -Filter "Name='$ServiceName'" -ErrorAction SilentlyContinue
    if (-not $service) { return $null }
    $rawPath = Get-ZbCommandExecutable -CommandLine ([string]$service.PathName)
    return [IO.Path]::GetFullPath([Environment]::ExpandEnvironmentVariables($rawPath))
}

function Assert-ZbServiceOwned {
    param(
        [Parameter(Mandatory)][string]$ServiceName,
        [Parameter(Mandatory)][string]$ExpectedExecutable
    )
    $actual = Get-ZbServiceExecutablePath -ServiceName $ServiceName
    if ([string]::IsNullOrWhiteSpace($actual)) {
        throw "Could not determine the executable owned by service '$ServiceName'."
    }
    $expected = [IO.Path]::GetFullPath($ExpectedExecutable)
    if (-not [string]::Equals($actual, $expected, [StringComparison]::OrdinalIgnoreCase)) {
        throw "Refusing to modify service '$ServiceName': it runs '$actual', not the managed ZeitBoard daemon '$expected'."
    }
}

function Get-ZbOwnedFirewallRules {
    param(
        [Parameter(Mandatory)][AllowEmptyCollection()][string[]]$DisplayNames,
        [Parameter(Mandatory)][string]$ExpectedProgram
    )
    if ($DisplayNames.Count -eq 0) { return }
    $expected = [IO.Path]::GetFullPath($ExpectedProgram)
    $candidates = @($DisplayNames | Select-Object -Unique | ForEach-Object {
        Get-NetFirewallRule -DisplayName $_ -ErrorAction SilentlyContinue
    })
    foreach ($rule in $candidates) {
        $owned = $false
        try {
            $applicationFilters = @($rule | Get-NetFirewallApplicationFilter -ErrorAction Stop)
            foreach ($filter in $applicationFilters) {
                if ([string]::IsNullOrWhiteSpace([string]$filter.Program) -or [string]$filter.Program -eq 'Any') {
                    continue
                }
                $actual = [IO.Path]::GetFullPath([Environment]::ExpandEnvironmentVariables([string]$filter.Program))
                if ([string]::Equals($actual, $expected, [StringComparison]::OrdinalIgnoreCase)) {
                    $owned = $true
                    break
                }
            }
        }
        catch {
            Write-ZbLog -Level warn -Message "could not inspect firewall rule '$($rule.DisplayName)'; it was preserved: $($_.Exception.Message)"
            continue
        }
        if ($owned) {
            Write-Output $rule
        }
        else {
            Write-ZbLog -Level warn -Message "firewall rule '$($rule.DisplayName)' points to another program and was preserved"
        }
    }
}

function Assert-ZbSafeServerRoot {
    param([Parameter(Mandatory)][string]$Path)
    $full = [IO.Path]::GetFullPath($Path).TrimEnd('\')
    $driveRoot = [IO.Path]::GetPathRoot($full).TrimEnd('\')
    if ([string]::Equals($full, $driveRoot, [StringComparison]::OrdinalIgnoreCase)) {
        throw "InstallRoot cannot be a drive root because its ACL is replaced recursively: $full"
    }

    $windowsRoot = [IO.Path]::GetFullPath($env:WINDIR).TrimEnd('\')
    if ($full.StartsWith($windowsRoot + '\', [StringComparison]::OrdinalIgnoreCase) -or
        [string]::Equals($full, $windowsRoot, [StringComparison]::OrdinalIgnoreCase)) {
        throw "InstallRoot cannot be inside the Windows directory: $full"
    }

    $protected = @(
        $env:ProgramData,
        $env:USERPROFILE,
        $env:ProgramFiles,
        ${env:ProgramFiles(x86)},
        [Environment]::GetFolderPath('Desktop'),
        [Environment]::GetFolderPath('MyDocuments')
    )
    foreach ($candidate in $protected) {
        if ($candidate -and [string]::Equals($full, ([IO.Path]::GetFullPath($candidate)).TrimEnd('\'), [StringComparison]::OrdinalIgnoreCase)) {
            throw "InstallRoot must be a dedicated subdirectory, not $candidate"
        }
    }
    $personalRoots = @(
        [Environment]::GetFolderPath('Desktop'),
        [Environment]::GetFolderPath('MyDocuments'),
        (Join-Path $env:USERPROFILE 'Downloads')
    )
    foreach ($candidate in $personalRoots) {
        if (-not $candidate) { continue }
        $personalRoot = ([IO.Path]::GetFullPath($candidate)).TrimEnd('\')
        if ([string]::Equals($full, $personalRoot, [StringComparison]::OrdinalIgnoreCase) -or
            $full.StartsWith($personalRoot + '\', [StringComparison]::OrdinalIgnoreCase)) {
            throw "InstallRoot cannot be inside a personal files directory because its ACL is replaced recursively: $full"
        }
    }

    if (Test-Path -LiteralPath $full) {
        $rootItem = Get-Item -LiteralPath $full -Force -ErrorAction Stop
        if (($rootItem.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
            throw "InstallRoot cannot be a reparse point because its ACL is replaced recursively: $full"
        }
        $nestedReparsePoints = @(Get-ChildItem -LiteralPath $full -Force -Recurse -ErrorAction Stop |
            Where-Object { ($_.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0 })
        if ($nestedReparsePoints.Count -gt 0) {
            throw "InstallRoot cannot contain reparse points because its ACL is replaced recursively: $full"
        }

        $entries = @(Get-ChildItem -LiteralPath $full -Force)
        if ($entries.Count -gt 0) {
            $managed = (Test-Path -LiteralPath (Join-Path $full '.zeitboard-server-root')) -or
                (Test-Path -LiteralPath (Join-Path $full 'zeitboardd.exe')) -or
                ((Test-Path -LiteralPath (Join-Path $full 'secrets\data-key.txt')) -and
                    (Test-Path -LiteralPath (Join-Path $full 'secrets\enrollment-secret.txt')))
            $existingConfigPath = Join-Path $full 'config.json'
            if (-not $managed -and (Test-Path -LiteralPath $existingConfigPath)) {
                try {
                    $candidateConfig = Get-Content -Raw -LiteralPath $existingConfigPath | ConvertFrom-Json
                    $managed = $candidateConfig.listenAddress -and $candidateConfig.dataDir -and $candidateConfig.dataKeyFile -and $candidateConfig.enrollmentSecretFile
                }
                catch { $managed = $false }
            }
            if (-not $managed) {
                throw "InstallRoot is non-empty and does not look like a ZeitBoard server root; refusing recursive ACL replacement: $full"
            }
        }
    }
    return $full
}

function Get-ZbVersionText {
    $stamp = Get-ZbVersionStamp
    "commit=$($stamp.Commit)`ndate=$($stamp.Date)"
}

function Publish-ZbVerifiedFile {
    # Atomically publish one file after verifying its staged copy. The displaced
    # file is retained at BackupPath for coherent rollback.
    param(
        [Parameter(Mandatory)][string]$SourcePath,
        [Parameter(Mandatory)][string]$DestinationPath,
        [Parameter(Mandatory)][string]$BackupPath
    )
    if (-not (Test-Path -LiteralPath $SourcePath)) { throw "Build output missing: $SourcePath" }
    Assert-ZbExecutableStopped -TargetPath $DestinationPath
    $destinationDir = Split-Path -Parent $DestinationPath
    $backupDir = Split-Path -Parent $BackupPath
    New-Item -ItemType Directory -Force -Path $destinationDir | Out-Null
    New-Item -ItemType Directory -Force -Path $backupDir | Out-Null
    $stage = Join-Path $destinationDir ('.' + (Split-Path -Leaf $DestinationPath) + '.staging-' + [guid]::NewGuid().ToString('N'))
    $restoreStage = $null
    $failedPublished = $null
    $hadDestination = Test-Path -LiteralPath $DestinationPath
    $published = $false
    try {
        Copy-Item -LiteralPath $SourcePath -Destination $stage
        $sourceHash = (Get-FileHash -Algorithm SHA256 -LiteralPath $SourcePath).Hash.ToLowerInvariant()
        $stageHash = (Get-FileHash -Algorithm SHA256 -LiteralPath $stage).Hash.ToLowerInvariant()
        if ($sourceHash -ne $stageHash) { throw "Staged copy failed its SHA-256 check: $DestinationPath" }
        if ($hadDestination) {
            if (Test-Path -LiteralPath $BackupPath) { Remove-Item -LiteralPath $BackupPath -Force }
            [IO.File]::Replace($stage, $DestinationPath, $BackupPath, $true)
        }
        else {
            Move-Item -LiteralPath $stage -Destination $DestinationPath
        }
        $published = $true
        $installedHash = (Get-FileHash -Algorithm SHA256 -LiteralPath $DestinationPath).Hash.ToLowerInvariant()
        if ($installedHash -ne $sourceHash) { throw "Published file failed its SHA-256 check: $DestinationPath" }
        Write-ZbLog -Level ok -Message "published $(Split-Path -Leaf $DestinationPath) SHA-256 $installedHash"
        return $installedHash
    }
    catch {
        $publishError = $_
        if ($published) {
            try {
                if ($hadDestination) {
                    if (-not (Test-Path -LiteralPath $BackupPath)) { throw "Publication backup is missing: $BackupPath" }
                    $restoreStage = Join-Path $destinationDir ('.restore-' + [guid]::NewGuid().ToString('N'))
                    $failedPublished = Join-Path $backupDir ('.failed-publication-' + [guid]::NewGuid().ToString('N'))
                    Copy-Item -LiteralPath $BackupPath -Destination $restoreStage
                    $backupHash = (Get-FileHash -Algorithm SHA256 -LiteralPath $BackupPath).Hash.ToLowerInvariant()
                    $restoreHash = (Get-FileHash -Algorithm SHA256 -LiteralPath $restoreStage).Hash.ToLowerInvariant()
                    if ($backupHash -ne $restoreHash) { throw 'The publication backup copy failed its SHA-256 check.' }
                    [IO.File]::Replace($restoreStage, $DestinationPath, $failedPublished, $true)
                    $restoreStage = $null
                    $restoredHash = (Get-FileHash -Algorithm SHA256 -LiteralPath $DestinationPath).Hash.ToLowerInvariant()
                    if ($restoredHash -ne $backupHash) { throw 'The restored destination failed its SHA-256 check.' }
                }
                elseif (Test-Path -LiteralPath $DestinationPath) {
                    Remove-Item -LiteralPath $DestinationPath -Force -ErrorAction Stop
                }
            }
            catch {
                throw "File publication failed ($($publishError.Exception.Message)); restoring the prior destination also failed: $($_.Exception.Message)"
            }
        }
        throw $publishError
    }
    finally {
        foreach ($temporary in @($stage, $restoreStage, $failedPublished)) {
            if ($temporary -and (Test-Path -LiteralPath $temporary)) {
                Remove-Item -LiteralPath $temporary -Force -ErrorAction SilentlyContinue
            }
        }
    }
}

function Set-ZbInstalledArtifactHash {
    param(
        [Parameter(Mandatory)][string]$InstallDir,
        [Parameter(Mandatory)][ValidatePattern('^[a-z0-9-]+$')][string]$Key,
        [Parameter(Mandatory)][ValidatePattern('^[0-9a-fA-F]{64}$')][string]$Hash
    )
    $versionFile = Join-Path $InstallDir 'version.txt'
    if (-not (Test-Path -LiteralPath $versionFile)) { throw "Version metadata missing: $versionFile" }
    $prefix = "$Key="
    $lines = @(Get-Content -LiteralPath $versionFile | Where-Object { -not $_.StartsWith($prefix, [StringComparison]::OrdinalIgnoreCase) })
    $content = (($lines + ($prefix + $Hash.ToLowerInvariant())) -join "`n") + "`n"
    $stage = Join-Path $InstallDir ('.version-' + [guid]::NewGuid().ToString('N') + '.txt')
    $backup = Join-Path $InstallDir ('.version-backup-' + [guid]::NewGuid().ToString('N') + '.txt')
    try {
        Set-ZbUtf8File -Path $stage -Content $content
        [IO.File]::Replace($stage, $versionFile, $backup, $true)
    }
    finally {
        if (Test-Path -LiteralPath $stage) { Remove-Item -LiteralPath $stage -Force -ErrorAction SilentlyContinue }
        if (Test-Path -LiteralPath $backup) { Remove-Item -LiteralPath $backup -Force -ErrorAction SilentlyContinue }
    }
}

function Publish-ZbDesktopBuild {
    # Stage and hash the new executable, then atomically replace an existing
    # install while retaining the old executable as the rollback target.
    param(
        [Parameter(Mandatory)][string]$SourceExe,
        [Parameter(Mandatory)][string]$InstallDir,
        [Parameter(Mandatory)][string]$VersionText
    )
    if (-not (Test-Path -LiteralPath $SourceExe)) { throw "Build output missing: $SourceExe" }
    New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
    $installedExe = Join-Path $InstallDir 'ZeitBoard.exe'
    Assert-ZbAppStopped -TargetPath $installedExe

    $stageDir = Join-Path $InstallDir ('.staging-' + [guid]::NewGuid().ToString('N'))
    $installedMcp = Join-Path $InstallDir 'zeitboard-mcp.exe'
    if (Test-Path -LiteralPath $installedMcp) { Assert-ZbExecutableStopped -TargetPath $installedMcp }
    $stageExe = Join-Path $stageDir 'ZeitBoard.exe'
    $previousDir = Join-Path $InstallDir 'previous'
    $previousExe = Join-Path $previousDir 'ZeitBoard.exe'
    $previousMcp = Join-Path $previousDir 'zeitboard-mcp.exe'
    $versionFile = Join-Path $InstallDir 'version.txt'
    $hadInstalledExe = Test-Path -LiteralPath $installedExe
    $desktopReplaced = $false
    try {
        New-Item -ItemType Directory -Force -Path $stageDir | Out-Null
        Copy-Item -LiteralPath $SourceExe -Destination $stageExe
        $sourceHash = (Get-FileHash -Algorithm SHA256 -LiteralPath $SourceExe).Hash.ToLowerInvariant()
        $stageHash = (Get-FileHash -Algorithm SHA256 -LiteralPath $stageExe).Hash.ToLowerInvariant()
        if ($sourceHash -ne $stageHash) { throw 'Staged desktop binary failed its SHA-256 copy check.' }
        Set-ZbUtf8File -Path (Join-Path $stageDir 'version.txt') -Content "$VersionText`nsha256=$stageHash`n"

        if ($hadInstalledExe) {
            New-Item -ItemType Directory -Force -Path $previousDir | Out-Null
            if (Test-Path -LiteralPath $installedMcp) {
                Copy-Item -LiteralPath $installedMcp -Destination $previousMcp -Force
            }
            elseif (Test-Path -LiteralPath $previousMcp) {
                Remove-Item -LiteralPath $previousMcp -Force
            }
            if (Test-Path -LiteralPath $versionFile) {
                Copy-Item -LiteralPath $versionFile -Destination (Join-Path $previousDir 'version.txt') -Force
            }
            elseif (Test-Path -LiteralPath (Join-Path $previousDir 'version.txt')) {
                Remove-Item -LiteralPath (Join-Path $previousDir 'version.txt') -Force
            }
            [IO.File]::Replace($stageExe, $installedExe, $previousExe, $true)
            $desktopReplaced = $true
        }
        else {
            Move-Item -LiteralPath $stageExe -Destination $installedExe
            $desktopReplaced = $true
        }
        Move-Item -LiteralPath (Join-Path $stageDir 'version.txt') -Destination $versionFile -Force
        $installedHash = (Get-FileHash -Algorithm SHA256 -LiteralPath $installedExe).Hash.ToLowerInvariant()
        if ($installedHash -ne $stageHash) { throw 'Published desktop binary failed its SHA-256 verification.' }
        Write-ZbLog -Level ok -Message "installed SHA-256 $stageHash"
    }
    catch {
        $publishError = $_
        if ($desktopReplaced) {
            if ($hadInstalledExe -and (Test-Path -LiteralPath $previousExe)) {
                try {
                    Restore-ZbPreviousBuild -InstallDir $InstallDir
                }
                catch {
                    throw "Desktop publication failed ($($publishError.Exception.Message)); restoring the previous build also failed: $($_.Exception.Message)"
                }
            }
            elseif (-not $hadInstalledExe) {
                Remove-Item -LiteralPath $installedExe -Force -ErrorAction SilentlyContinue
                Remove-Item -LiteralPath $versionFile -Force -ErrorAction SilentlyContinue
            }
        }
        throw $publishError
    }
    finally {
        if (Test-Path -LiteralPath $stageDir) {
            Remove-Item -LiteralPath $stageDir -Recurse -Force -ErrorAction SilentlyContinue
        }
    }
}

function Restore-ZbPreviousBuild {
    param([Parameter(Mandatory)][string]$InstallDir)
    $installedExe = Join-Path $InstallDir 'ZeitBoard.exe'
    $previousDir = Join-Path $InstallDir 'previous'
    $previousExe = Join-Path $previousDir 'ZeitBoard.exe'
    $installedMcp = Join-Path $InstallDir 'zeitboard-mcp.exe'
    $previousMcp = Join-Path $previousDir 'zeitboard-mcp.exe'
    if (-not (Test-Path -LiteralPath $previousExe)) {
        throw "No previous build to roll back to ($previousExe)."
    }
    Assert-ZbAppStopped -TargetPath $installedExe
    if (Test-Path -LiteralPath $installedMcp) { Assert-ZbExecutableStopped -TargetPath $installedMcp }
    $stage = Join-Path $InstallDir ('.rollback-' + [guid]::NewGuid().ToString('N') + '.exe')
    try {
        Copy-Item -LiteralPath $previousExe -Destination $stage
        if (Test-Path -LiteralPath $installedExe) {
            $failedExe = Join-Path $previousDir 'failed-current.exe'
            if (Test-Path -LiteralPath $failedExe) { Remove-Item -LiteralPath $failedExe -Force }
            [IO.File]::Replace($stage, $installedExe, $failedExe, $true)
        }
        else {
            Move-Item -LiteralPath $stage -Destination $installedExe
        }
        $previousVersion = Join-Path $previousDir 'version.txt'
        $currentVersion = Join-Path $InstallDir 'version.txt'
        if (Test-Path -LiteralPath $currentVersion) {
            Copy-Item -LiteralPath $currentVersion -Destination (Join-Path $previousDir 'failed-current-version.txt') -Force
        }
        if (Test-Path -LiteralPath $previousMcp) {
            Publish-ZbVerifiedFile -SourcePath $previousMcp -DestinationPath $installedMcp -BackupPath (Join-Path $previousDir 'failed-current-mcp.exe') | Out-Null
        }
        elseif (Test-Path -LiteralPath $installedMcp) {
            $failedMcp = Join-Path $previousDir 'failed-current-mcp.exe'
            if (Test-Path -LiteralPath $failedMcp) { Remove-Item -LiteralPath $failedMcp -Force }
            Move-Item -LiteralPath $installedMcp -Destination $failedMcp
        }

        if (Test-Path -LiteralPath $previousVersion) {
            Copy-Item -LiteralPath $previousVersion -Destination $currentVersion -Force
        }
        elseif (Test-Path -LiteralPath $currentVersion) {
            Remove-Item -LiteralPath $currentVersion -Force
        }
        Write-ZbLog -Level ok -Message 'restored the previous ZeitBoard build'
    }
    finally {
        if (Test-Path -LiteralPath $stage) { Remove-Item -LiteralPath $stage -Force -ErrorAction SilentlyContinue }
    }
}

function Test-ZbInstalledBuild {
    param([Parameter(Mandatory)][string]$InstallDir)
    $exe = Join-Path $InstallDir 'ZeitBoard.exe'
    $mcp = Join-Path $InstallDir 'zeitboard-mcp.exe'
    $version = Join-Path $InstallDir 'version.txt'
    if (-not (Test-Path -LiteralPath $exe)) { return $false }
    if (-not (Test-Path -LiteralPath $version)) { return $true } # Legacy install.
    $lines = @(Get-Content -LiteralPath $version)
    $hashLine = $lines | Where-Object { $_ -match '^sha256=' } | Select-Object -First 1
    if (-not $hashLine) {
        $contentLines = @($lines | Where-Object { -not [string]::IsNullOrWhiteSpace($_) })
        $isLegacyStamp = $contentLines.Count -eq 1 -and
            $contentLines[0] -match '^[0-9a-fA-F]{7,40}\s{2}\d{4}-\d{2}-\d{2}\s'
        if (-not $isLegacyStamp) { return $false }
    }
    if ($hashLine) {
        $expected = $hashLine.Substring(7).Trim().ToLowerInvariant()
        if ($expected -notmatch '^[0-9a-f]{64}$') { return $false }
        $actual = (Get-FileHash -Algorithm SHA256 -LiteralPath $exe).Hash.ToLowerInvariant()
        if ($expected -ne $actual) { return $false }
    }
    $mcpHashLine = $lines | Where-Object { $_ -match '^mcp-sha256=' } | Select-Object -First 1
    if ($mcpHashLine) {
        if (-not (Test-Path -LiteralPath $mcp)) { return $false }
        $expectedMcp = $mcpHashLine.Substring(11).Trim().ToLowerInvariant()
        if ($expectedMcp -notmatch '^[0-9a-f]{64}$') { return $false }
        $actualMcp = (Get-FileHash -Algorithm SHA256 -LiteralPath $mcp).Hash.ToLowerInvariant()
        if ($expectedMcp -ne $actualMcp) { return $false }
    }
    return $true
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
    $lnk = $null
    try {
        if (Test-Path -LiteralPath $LinkPath) {
            try {
                $existing = $shell.CreateShortcut($LinkPath)
                try { $actual = [IO.Path]::GetFullPath([string]$existing.TargetPath) }
                finally { [Runtime.InteropServices.Marshal]::ReleaseComObject($existing) | Out-Null }
                $expected = [IO.Path]::GetFullPath($TargetPath)
                if (-not [string]::Equals($actual, $expected, [StringComparison]::OrdinalIgnoreCase)) {
                    Write-ZbLog -Level warn -Message "shortcut points to '$actual' and was preserved: $LinkPath"
                    return
                }
            }
            catch {
                Write-ZbLog -Level warn -Message "existing shortcut could not be verified and was preserved: $LinkPath"
                return
            }
        }
        $lnk = $shell.CreateShortcut($LinkPath)
        $lnk.TargetPath = $TargetPath
        $lnk.WorkingDirectory = Split-Path -Parent $TargetPath
        $lnk.Description = $Description
        $lnk.Save()
        Write-ZbLog -Level ok -Message "shortcut: $LinkPath"
    }
    finally {
        if ($lnk) { [Runtime.InteropServices.Marshal]::ReleaseComObject($lnk) | Out-Null }
        [Runtime.InteropServices.Marshal]::ReleaseComObject($shell) | Out-Null
    }
}

function Remove-ZbShortcutIfOwned {
    param(
        [Parameter(Mandatory)][string]$LinkPath,
        [Parameter(Mandatory)][string]$ExpectedTarget
    )
    if (-not (Test-Path -LiteralPath $LinkPath)) { return }
    $shell = $null
    $link = $null
    try {
        $shell = New-Object -ComObject WScript.Shell
        $link = $shell.CreateShortcut($LinkPath)
        $actual = [IO.Path]::GetFullPath([string]$link.TargetPath)
        $expected = [IO.Path]::GetFullPath($ExpectedTarget)
        if (-not [string]::Equals($actual, $expected, [StringComparison]::OrdinalIgnoreCase)) {
            Write-ZbLog -Level warn -Message "shortcut points to '$actual' and was preserved: $LinkPath"
            return
        }
        Remove-Item -LiteralPath $LinkPath -Force
        Write-ZbLog -Level ok -Message "removed $LinkPath"
    }
    finally {
        if ($link) { [Runtime.InteropServices.Marshal]::ReleaseComObject($link) | Out-Null }
        if ($shell) { [Runtime.InteropServices.Marshal]::ReleaseComObject($shell) | Out-Null }
    }
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
    $name = 'ZeitBoard'
    if ($Enabled) {
        if (-not (Test-Path $runKey)) { New-Item -Path $runKey -Force | Out-Null }
        $properties = Get-ItemProperty -Path $runKey -Name $name -ErrorAction SilentlyContinue
        if ($properties) {
            $current = [string]$properties.$name
            $expectedQuoted = "`"$TargetPath`""
            $owned = [string]::Equals($current, $expectedQuoted, [StringComparison]::OrdinalIgnoreCase) -or
                [string]::Equals($current, $TargetPath, [StringComparison]::OrdinalIgnoreCase)
            if (-not $owned) {
                Write-ZbLog -Level warn -Message 'startup entry named ZeitBoard points elsewhere and was preserved'
                return
            }
        }
        Set-ItemProperty -Path $runKey -Name $name -Value "`"$TargetPath`"" -Force
        Write-ZbLog -Level ok -Message 'startup launch enabled (HKCU Run)'
    }
    else {
        if (-not (Test-Path $runKey)) { return }
        $properties = Get-ItemProperty -Path $runKey -Name $name -ErrorAction SilentlyContinue
        if ($properties) {
            $current = [string]$properties.$name
            $expectedQuoted = "`"$TargetPath`""
            $owned = [string]::Equals($current, $expectedQuoted, [StringComparison]::OrdinalIgnoreCase) -or
                [string]::Equals($current, $TargetPath, [StringComparison]::OrdinalIgnoreCase)
            if (-not $owned) {
                Write-ZbLog -Level warn -Message 'startup entry named ZeitBoard points elsewhere and was preserved'
                return
            }
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

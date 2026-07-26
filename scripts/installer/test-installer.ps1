<#
.SYNOPSIS
  Portable unit tests for the ZeitBoard installer library. No Pester dependency
  (stock Windows ships Pester 3.4 while CI has 5.x; their assertion syntaxes are
  incompatible), so this uses plain assertions with an exit code. Runs the same
  everywhere. CI also calls install.ps1/update.ps1 with -DryRun.

.EXAMPLE
  powershell -ExecutionPolicy Bypass -File scripts\installer\test-installer.ps1
#>
[CmdletBinding()]
param()
$ErrorActionPreference = 'Stop'
. "$PSScriptRoot\_zb.common.ps1"

$script:pass = 0
$script:fail = 0

function Test-Case {
    param([string]$Name, [scriptblock]$Body)
    try { & $Body; $script:pass++; Write-Host "  ok   $Name" -ForegroundColor Green }
    catch { $script:fail++; Write-Host "  FAIL $Name" -ForegroundColor Red; Write-Host "       $($_.Exception.Message)" -ForegroundColor DarkGray }
}
function Assert-True { param($Value, $Message = 'expected true') if (-not $Value) { throw $Message } }
function Assert-Equal { param($Expected, $Actual) if ($Expected -ne $Actual) { throw "expected [$Expected], got [$Actual]" } }
function Assert-Throws { param([scriptblock]$Body, $Message = 'expected an exception') $threw = $false; try { & $Body } catch { $threw = $true }; if (-not $threw) { throw $Message } }

Write-Host 'ZeitBoard installer library tests' -ForegroundColor Cyan

Test-Case 'Get-ZbPaths resolves the repo root to the working tree' {
    $p = Get-ZbPaths
    Assert-True (Test-Path (Join-Path $p.RepoRoot 'go.work')) 'repo root should contain go.work'
    Assert-True ($p.InstallDir -like '*Programs\ZeitBoard') 'install dir under Programs'
    Assert-True ($p.DataDir -like '*ZeitBoard') 'data dir named ZeitBoard'
}

Test-Case 'pins.psd1 is structurally valid (https + one integrity source each)' {
    $problems = Test-ZbPins
    Assert-Equal 0 $problems.Count
}

Test-Case 'Test-ZbPins rejects short or placeholder-like literal checksums' {
    $invalid = @{
        Tool = @{
            Url = 'https://example.invalid/tool.zip'
            Sha256 = 'abc123'
            DirName = 'tool'
            ProbePath = 'tool.exe'
        }
    }
    $problems = @(Test-ZbPins -Pins $invalid)
    Assert-True ($problems.Count -gt 0) 'short checksum must be rejected'
}

Test-Case 'Test-ZbPins rejects tool paths that escape the managed directory' {
    $hash = 'a' * 64
    $invalid = @{
        EscapingDir = @{
            Url = 'https://example.invalid/dir.zip'; Sha256 = $hash
            DirName = '..\outside'; ProbePath = 'tool.exe'
        }
        EscapingProbe = @{
            Url = 'https://example.invalid/probe.zip'; Sha256 = $hash
            DirName = 'tool'; ProbePath = '..\outside.exe'
        }
    }
    $problems = @(Test-ZbPins -Pins $invalid)
    $summary = $problems -join ' '
    Assert-True ($summary -match 'DirName') 'escaping DirName must be rejected'
    Assert-True ($summary -match 'ProbePath') 'escaping ProbePath must be rejected'
}


Test-Case 'Read-ZbChoice: explicit override beats everything' {
    Assert-Equal $true (Read-ZbChoice -Question 'x' -Default $false -NonInteractive -Override $true)
    Assert-Equal $false (Read-ZbChoice -Question 'x' -Default $true -NonInteractive -Override $false)
}

Test-Case 'Read-ZbChoice: non-interactive returns the default' {
    Assert-Equal $true (Read-ZbChoice -Question 'x' -Default $true -NonInteractive)
    Assert-Equal $false (Read-ZbChoice -Question 'x' -Default $false -NonInteractive)
}

Test-Case 'lifecycle lock rejects a concurrent PowerShell process' {
    $root = Join-Path ([IO.Path]::GetTempPath()) ('zb-lock-' + [guid]::NewGuid().ToString('N'))
    $ready = Join-Path $root 'ready'
    New-Item -ItemType Directory -Path $root -Force | Out-Null
    $commonPath = (Resolve-Path (Join-Path $PSScriptRoot '_zb.common.ps1')).Path
    $job = Start-Job -ScriptBlock {
        param($CommonPath, $ReadyPath)
        . $CommonPath
        $held = Enter-ZbLifecycleLock
        try {
            Set-ZbUtf8File -Path $ReadyPath -Content 'ready'
            Start-Sleep -Seconds 15
        }
        finally { Exit-ZbLifecycleLock -Mutex $held }
    } -ArgumentList $commonPath, $ready
    try {
        $deadline = (Get-Date).AddSeconds(8)
        while (-not (Test-Path -LiteralPath $ready) -and (Get-Date) -lt $deadline) {
            Start-Sleep -Milliseconds 100
        }
        if (-not (Test-Path -LiteralPath $ready)) {
            $details = Receive-Job -Job $job -Keep *>&1 | Out-String
            throw "lock holder did not become ready: $details"
        }
        Assert-Throws {
            $unexpected = Enter-ZbLifecycleLock
            try { } finally { Exit-ZbLifecycleLock -Mutex $unexpected }
        } 'a second process must not acquire the lifecycle lock'
    }
    finally {
        Stop-Job -Job $job -ErrorAction SilentlyContinue
        Remove-Job -Job $job -Force -ErrorAction SilentlyContinue
        Remove-Item -LiteralPath $root -Recurse -Force -ErrorAction SilentlyContinue
    }
}

Test-Case 'Get-ZbExpectedHash: literal checksum is returned lowercased' {
    $upper = ('A' * 64) -join ''
    Assert-Equal $upper.ToLowerInvariant() (Get-ZbExpectedHash -Pin @{ Version = 't'; Sha256 = $upper })
}

Test-Case 'Get-ZbExpectedHash: REPLACE_ placeholder fails closed with guidance' {
    Assert-Throws { Get-ZbExpectedHash -Pin @{ Version = 't'; Sha256 = 'REPLACE_WITH_REAL' } }
}

Test-Case 'Move-ZbExtractedArchive removes a same-named top-level wrapper' {
    $root = Join-Path ([IO.Path]::GetTempPath()) ('zb-extract-' + [guid]::NewGuid().ToString('N'))
    $extracted = Join-Path $root 'extracted'
    $wrapper = Join-Path $extracted 'tool-v1'
    $target = Join-Path $root 'tool-v1'
    New-Item -ItemType Directory -Path $wrapper -Force | Out-Null
    try {
        Set-ZbUtf8File -Path (Join-Path $wrapper 'tool.exe') -Content 'tool'
        Move-ZbExtractedArchive -ExtractedPath $extracted -TargetPath $target
        Assert-True (Test-Path -LiteralPath (Join-Path $target 'tool.exe')) 'probe should be directly under the target'
        Assert-True (-not (Test-Path -LiteralPath (Join-Path $target 'tool-v1\tool.exe'))) 'wrapper must not be nested twice'
    }
    finally { Remove-Item -LiteralPath $root -Recurse -Force -ErrorAction SilentlyContinue }
}

Test-Case 'Move-ZbExtractedArchive validates the probe before publication' {
    $root = Join-Path ([IO.Path]::GetTempPath()) ('zb-extract-invalid-' + [guid]::NewGuid().ToString('N'))
    $extracted = Join-Path $root 'extracted'
    $wrapper = Join-Path $extracted 'tool-v1'
    $target = Join-Path $root 'tool-v1'
    New-Item -ItemType Directory -Path $wrapper -Force | Out-Null
    try {
        Set-ZbUtf8File -Path (Join-Path $wrapper 'readme.txt') -Content 'incomplete'
        Assert-Throws { Move-ZbExtractedArchive -ExtractedPath $extracted -TargetPath $target -ProbePath 'tool.exe' }
        Assert-True (-not (Test-Path -LiteralPath $target)) 'invalid payload must not reach the target path'
        Assert-True (Test-Path -LiteralPath $wrapper) 'invalid payload should remain staged for caller cleanup'
    }
    finally { Remove-Item -LiteralPath $root -Recurse -Force -ErrorAction SilentlyContinue }
}

Test-Case 'Get-ZbCommandExecutable parses managed service command lines' {
    Assert-Equal 'C:\Program Files\ZeitBoard\zeitboardd.exe' (Get-ZbCommandExecutable -CommandLine '"C:\Program Files\ZeitBoard\zeitboardd.exe" -config "C:\x.json"')
    Assert-Equal 'C:\ZeitBoard\zeitboardd.exe' (Get-ZbCommandExecutable -CommandLine 'C:\ZeitBoard\zeitboardd.exe -config C:\x.json')
    Assert-Throws { Get-ZbCommandExecutable -CommandLine '"unterminated' }
}

Test-Case 'firewall ownership requires the expected executable' {
    function Get-NetFirewallRule {
        [CmdletBinding()]
        param([string]$DisplayName)
        $program = if ($DisplayName -eq 'owned') { 'C:\ZeitBoard\zeitboardd.exe' } else { 'C:\Other\server.exe' }
        [pscustomobject]@{ DisplayName = $DisplayName; Program = $program }
    }
    function Get-NetFirewallApplicationFilter {
        [CmdletBinding()]
        param([Parameter(ValueFromPipeline)]$InputObject)
        process { [pscustomobject]@{ Program = $InputObject.Program } }
    }

    $rules = @(Get-ZbOwnedFirewallRules -DisplayNames @('owned', 'foreign') -ExpectedProgram 'C:\ZeitBoard\zeitboardd.exe')
    Assert-Equal 1 $rules.Count
    Assert-Equal 'owned' $rules[0].DisplayName
}

Test-Case 'firewall ownership accepts an empty candidate list' {
    $rules = @(Get-ZbOwnedFirewallRules -DisplayNames @() -ExpectedProgram 'C:\ZeitBoard\zeitboardd.exe')
    Assert-Equal 0 $rules.Count
}

Test-Case 'Assert-ZbSafeServerRoot accepts owned roots and rejects unrelated directories' {
    $root = Join-Path ([IO.Path]::GetTempPath()) ('zb-server-root-' + [guid]::NewGuid().ToString('N'))
    New-Item -ItemType Directory -Path $root -Force | Out-Null
    try {
        Assert-Equal $root.TrimEnd('\') (Assert-ZbSafeServerRoot -Path $root)
        Set-ZbUtf8File -Path (Join-Path $root 'unrelated.txt') -Content 'unrelated'
        Assert-Throws { Assert-ZbSafeServerRoot -Path $root } 'an unrelated non-empty directory must be rejected'
        Set-ZbUtf8File -Path (Join-Path $root '.zeitboard-server-root') -Content 'ZeitBoard server root v1'
        Assert-Equal $root.TrimEnd('\') (Assert-ZbSafeServerRoot -Path $root)
        Assert-Throws { Assert-ZbSafeServerRoot -Path ([IO.Path]::GetPathRoot($root)) } 'a drive root must be rejected'
        $downloads = Join-Path $env:USERPROFILE 'Downloads'
        Assert-Throws { Assert-ZbSafeServerRoot -Path $downloads } 'the Downloads root itself must be rejected'
        $documents = [Environment]::GetFolderPath('MyDocuments')
        if ($documents) {
            $personalRoot = Join-Path $documents ('ZeitBoard-server-' + [guid]::NewGuid().ToString('N'))
            Assert-Throws { Assert-ZbSafeServerRoot -Path $personalRoot } 'a server root inside personal documents must be rejected'
        }
    }
    finally { Remove-Item -LiteralPath $root -Recurse -Force -ErrorAction SilentlyContinue }
}

Test-Case 'Assert-ZbSafeServerRoot rejects root and nested reparse points' {
    $base = Join-Path ([IO.Path]::GetTempPath()) ('zb-server-reparse-' + [guid]::NewGuid().ToString('N'))
    $target = Join-Path $base 'target'
    $link = Join-Path $base 'link'
    $ownedRoot = Join-Path $base 'owned'
    $nestedLink = Join-Path $ownedRoot 'nested-link'
    New-Item -ItemType Directory -Path $target -Force | Out-Null
    New-Item -ItemType Directory -Path $ownedRoot -Force | Out-Null
    try {
        New-Item -ItemType Junction -Path $link -Target $target | Out-Null
        Assert-Throws { Assert-ZbSafeServerRoot -Path $link } 'a reparse-point root must be rejected'

        Set-ZbUtf8File -Path (Join-Path $ownedRoot '.zeitboard-server-root') -Content 'ZeitBoard server root v1'
        New-Item -ItemType Junction -Path $nestedLink -Target $target | Out-Null
        Assert-Throws { Assert-ZbSafeServerRoot -Path $ownedRoot } 'a nested reparse point must be rejected'
    }
    finally {
        foreach ($junction in @($nestedLink, $link)) {
            if (Test-Path -LiteralPath $junction) {
                [IO.Directory]::Delete($junction)
            }
        }
        Remove-Item -LiteralPath $base -Recurse -Force -ErrorAction SilentlyContinue
    }
}

Test-Case 'Assert-ZbSafeServerRoot recognizes only a complete legacy server config' {
    $root = Join-Path ([IO.Path]::GetTempPath()) ('zb-server-config-' + [guid]::NewGuid().ToString('N'))
    $config = Join-Path $root 'config.json'
    New-Item -ItemType Directory -Path $root -Force | Out-Null
    try {
        Set-ZbUtf8File -Path $config -Content '{"listenAddress":"127.0.0.1:8765","dataDir":"data","enrollmentSecretFile":"secret.txt"}'
        Assert-Throws {
            Assert-ZbSafeServerRoot -Path $root
        } 'an unrelated partial config must not authorize recursive ACL replacement'

        Set-ZbUtf8File -Path $config -Content '{"listenAddress":"127.0.0.1:8765","dataDir":"data","dataKeyFile":"key.txt","enrollmentSecretFile":"secret.txt"}'
        Assert-Equal $root.TrimEnd('\') (Assert-ZbSafeServerRoot -Path $root)
    }
    finally {
        Remove-Item -LiteralPath $root -Recurse -Force -ErrorAction SilentlyContinue
    }
}




Test-Case 'Set-ZbUtf8File writes UTF-8 without a BOM' {
    $root = Join-Path ([IO.Path]::GetTempPath()) ('zb-utf8-' + [guid]::NewGuid().ToString('N'))
    New-Item -ItemType Directory -Path $root | Out-Null
    try {
        $file = Join-Path $root 'config.json'
        Set-ZbUtf8File -Path $file -Content '{"listenAddress":"127.0.0.1:8765"}'
        $bytes = [IO.File]::ReadAllBytes($file)
        Assert-True ($bytes.Length -gt 3) 'test file should not be empty'
        $hasBom = $bytes[0] -eq 0xEF -and $bytes[1] -eq 0xBB -and $bytes[2] -eq 0xBF
        Assert-True (-not $hasBom) 'UTF-8 BOM must not be present'
    }
    finally { Remove-Item -LiteralPath $root -Recurse -Force -ErrorAction SilentlyContinue }
}

Test-Case 'installed-build verification distinguishes legacy from truncated metadata' {
    $root = Join-Path ([IO.Path]::GetTempPath()) ('zb-version-' + [guid]::NewGuid().ToString('N'))
    New-Item -ItemType Directory -Path $root -Force | Out-Null
    try {
        Set-ZbUtf8File -Path (Join-Path $root 'ZeitBoard.exe') -Content 'legacy-build'
        Set-ZbUtf8File -Path (Join-Path $root 'version.txt') -Content "a2208c3  2026-07-22 12:34:56 -0400`n"
        Assert-True (Test-ZbInstalledBuild -InstallDir $root) 'historical commit/date metadata should remain upgradeable'

        Set-ZbUtf8File -Path (Join-Path $root 'version.txt') -Content "commit=a2208c3`ndate=2026-07-22`n"
        Assert-True (-not (Test-ZbInstalledBuild -InstallDir $root)) 'structured metadata without sha256 must fail closed'

        Set-ZbUtf8File -Path (Join-Path $root 'version.txt') -Content ''
        Assert-True (-not (Test-ZbInstalledBuild -InstallDir $root)) 'empty metadata must fail closed'
    }
    finally {
        Remove-Item -LiteralPath $root -Recurse -Force -ErrorAction SilentlyContinue
    }
}

Test-Case 'Start-ZbLog dry-run has no log-file side effect' {
    $script:ZbLogFile = 'sentinel'
    $result = Start-ZbLog -Name 'test' -DryRun
    Assert-True ($null -eq $result) 'dry-run log result should be null'
    Assert-True ($null -eq $script:ZbLogFile) 'dry-run must clear the active log path'
}

Test-Case 'Backup-ZbData supports an empty data directory' {
    $root = Join-Path ([IO.Path]::GetTempPath()) ('zb-backup-' + [guid]::NewGuid().ToString('N'))
    $source = Join-Path $root 'empty-data'
    $destination = Join-Path $root 'backups'
    New-Item -ItemType Directory -Path $source -Force | Out-Null
    try {
        $zip = Backup-ZbData -Reason 'test' -SourceDir $source -DestinationDir $destination
        Assert-True (Test-Path -LiteralPath $zip) 'backup ZIP should exist'
        Add-Type -AssemblyName System.IO.Compression.FileSystem
        $archive = [IO.Compression.ZipFile]::OpenRead($zip)
        try { Assert-Equal 0 $archive.Entries.Count }
        finally { $archive.Dispose() }
    }
    finally { Remove-Item -LiteralPath $root -Recurse -Force -ErrorAction SilentlyContinue }
}

Test-Case 'desktop and MCP publish verify hashes and roll back coherently' {
    $root = Join-Path ([IO.Path]::GetTempPath()) ('zb-publish-' + [guid]::NewGuid().ToString('N'))
    $install = Join-Path $root 'install'
    $first = Join-Path $root 'first.exe'
    $second = Join-Path $root 'second.exe'
    $firstMcp = Join-Path $root 'first-mcp.exe'
    $secondMcp = Join-Path $root 'second-mcp.exe'
    New-Item -ItemType Directory -Path $root -Force | Out-Null
    try {
        Set-ZbUtf8File -Path $first -Content 'first-build'
        Set-ZbUtf8File -Path $second -Content 'second-build'
        Set-ZbUtf8File -Path $firstMcp -Content 'first-mcp-build'
        Set-ZbUtf8File -Path $secondMcp -Content 'second-mcp-build'

        Publish-ZbDesktopBuild -SourceExe $first -InstallDir $install -VersionText "commit=one`ndate=one"
        $firstMcpHash = Publish-ZbVerifiedFile -SourcePath $firstMcp -DestinationPath (Join-Path $install 'zeitboard-mcp.exe') -BackupPath (Join-Path $install 'previous\zeitboard-mcp.exe')
        Set-ZbInstalledArtifactHash -InstallDir $install -Key 'mcp-sha256' -Hash $firstMcpHash
        Assert-True (Test-ZbInstalledBuild -InstallDir $install) 'first release should verify'

        Publish-ZbDesktopBuild -SourceExe $second -InstallDir $install -VersionText "commit=two`ndate=two"
        $secondMcpHash = Publish-ZbVerifiedFile -SourcePath $secondMcp -DestinationPath (Join-Path $install 'zeitboard-mcp.exe') -BackupPath (Join-Path $install 'previous\zeitboard-mcp.exe')
        Set-ZbInstalledArtifactHash -InstallDir $install -Key 'mcp-sha256' -Hash $secondMcpHash
        Assert-True (Test-ZbInstalledBuild -InstallDir $install) 'second release should verify'
        Assert-Equal 'first-build' (Get-Content -Raw -LiteralPath (Join-Path $install 'previous\ZeitBoard.exe'))
        Assert-Equal 'first-mcp-build' (Get-Content -Raw -LiteralPath (Join-Path $install 'previous\zeitboard-mcp.exe'))

        Set-ZbUtf8File -Path (Join-Path $install 'zeitboard-mcp.exe') -Content 'corrupted-mcp'
        Assert-True (-not (Test-ZbInstalledBuild -InstallDir $install)) 'MCP tampering should fail hash verification'
        Set-ZbUtf8File -Path (Join-Path $install 'zeitboard-mcp.exe') -Content 'second-mcp-build'
        Assert-True (Test-ZbInstalledBuild -InstallDir $install) 'restored MCP bytes should verify'

        Set-ZbUtf8File -Path (Join-Path $install 'ZeitBoard.exe') -Content 'corrupted-desktop'
        Assert-True (-not (Test-ZbInstalledBuild -InstallDir $install)) 'desktop tampering should fail hash verification'

        Restore-ZbPreviousBuild -InstallDir $install
        Assert-True (Test-ZbInstalledBuild -InstallDir $install) 'rollback should restore a coherent release'
        Assert-Equal 'first-build' (Get-Content -Raw -LiteralPath (Join-Path $install 'ZeitBoard.exe'))
        Assert-Equal 'first-mcp-build' (Get-Content -Raw -LiteralPath (Join-Path $install 'zeitboard-mcp.exe'))
        $version = Get-Content -Raw -LiteralPath (Join-Path $install 'version.txt')
        Assert-True ($version -match 'commit=one') 'rollback should restore previous version metadata'
        Assert-True ($version -match 'mcp-sha256=') 'rollback metadata should include the previous MCP hash'
    }
    finally { Remove-Item -LiteralPath $root -Recurse -Force -ErrorAction SilentlyContinue }
}

Test-Case 'Publish-ZbVerifiedFile restores the prior destination after a post-publication failure' {
    $root = Join-Path ([IO.Path]::GetTempPath()) ('zb-publish-restore-' + [guid]::NewGuid().ToString('N'))
    $source = Join-Path $root 'new.exe'
    $destination = Join-Path $root 'installed.exe'
    $backup = Join-Path $root 'previous\installed.exe'
    $badLogPath = Join-Path $root 'not-a-log-file'
    $priorLogFile = $script:ZbLogFile
    New-Item -ItemType Directory -Path $root -Force | Out-Null
    try {
        Set-ZbUtf8File -Path $source -Content 'new-build'
        Set-ZbUtf8File -Path $destination -Content 'old-build'
        New-Item -ItemType Directory -Path $badLogPath -Force | Out-Null
        $script:ZbLogFile = $badLogPath

        Assert-Throws {
            Publish-ZbVerifiedFile -SourcePath $source -DestinationPath $destination -BackupPath $backup | Out-Null
        } 'a post-publication log failure must fail the operation'

        Assert-Equal 'old-build' (Get-Content -Raw -LiteralPath $destination)
        Assert-Equal 'old-build' (Get-Content -Raw -LiteralPath $backup)
    }
    finally {
        $script:ZbLogFile = $priorLogFile
        Remove-Item -LiteralPath $root -Recurse -Force -ErrorAction SilentlyContinue
    }
}

Test-Case 'rollback to a legacy install removes current hash metadata' {
    $root = Join-Path ([IO.Path]::GetTempPath()) ('zb-legacy-rollback-' + [guid]::NewGuid().ToString('N'))
    $install = Join-Path $root 'install'
    $next = Join-Path $root 'next.exe'
    New-Item -ItemType Directory -Path $install -Force | Out-Null
    try {
        Set-ZbUtf8File -Path (Join-Path $install 'ZeitBoard.exe') -Content 'legacy-build'
        Set-ZbUtf8File -Path $next -Content 'next-build'
        Publish-ZbDesktopBuild -SourceExe $next -InstallDir $install -VersionText "commit=next`ndate=next"
        Assert-True (Test-Path -LiteralPath (Join-Path $install 'version.txt')) 'new release should have hash metadata'

        Restore-ZbPreviousBuild -InstallDir $install
        Assert-Equal 'legacy-build' (Get-Content -Raw -LiteralPath (Join-Path $install 'ZeitBoard.exe'))
        Assert-True (-not (Test-Path -LiteralPath (Join-Path $install 'version.txt'))) 'legacy rollback must remove new metadata'
        Assert-True (Test-ZbInstalledBuild -InstallDir $install) 'legacy rollback should remain a valid legacy install'
    }
    finally { Remove-Item -LiteralPath $root -Recurse -Force -ErrorAction SilentlyContinue }
}

Test-Case 'New-ZbShortcut preserves a same-named shortcut it does not own' {
    $root = Join-Path ([IO.Path]::GetTempPath()) ('zb-shortcut-' + [guid]::NewGuid().ToString('N'))
    $linkPath = Join-Path $root 'ZeitBoard.lnk'
    $foreignTarget = Join-Path $root 'foreign.exe'
    $expectedTarget = Join-Path $root 'ZeitBoard.exe'
    New-Item -ItemType Directory -Path $root -Force | Out-Null
    Set-ZbUtf8File -Path $foreignTarget -Content 'foreign'
    Set-ZbUtf8File -Path $expectedTarget -Content 'expected'
    $shell = $null
    $shortcut = $null
    try {
        $shell = New-Object -ComObject WScript.Shell
        $shortcut = $shell.CreateShortcut($linkPath)
        $shortcut.TargetPath = $foreignTarget
        $shortcut.Save()
        [Runtime.InteropServices.Marshal]::ReleaseComObject($shortcut) | Out-Null
        $shortcut = $null
        [Runtime.InteropServices.Marshal]::ReleaseComObject($shell) | Out-Null
        $shell = $null

        New-ZbShortcut -LinkPath $linkPath -TargetPath $expectedTarget

        $shell = New-Object -ComObject WScript.Shell
        $shortcut = $shell.CreateShortcut($linkPath)
        Assert-Equal ([IO.Path]::GetFullPath($foreignTarget)) ([IO.Path]::GetFullPath([string]$shortcut.TargetPath))
    }
    finally {
        if ($shortcut) { [Runtime.InteropServices.Marshal]::ReleaseComObject($shortcut) | Out-Null }
        if ($shell) { [Runtime.InteropServices.Marshal]::ReleaseComObject($shell) | Out-Null }
        Remove-Item -LiteralPath $root -Recurse -Force -ErrorAction SilentlyContinue
    }
}

Test-Case 'Set-ZbStartupEntry: add/remove round-trips against a sandbox key' {
    $sandbox = 'HKCU:\Software\ZeitBoardTest\Run'
    try {
        Set-ZbStartupEntry -TargetPath 'C:\x\ZeitBoard.exe' -Enabled $true -RunKey $sandbox
        $v = (Get-ItemProperty -Path $sandbox -Name 'ZeitBoard').ZeitBoard
        Assert-Equal '"C:\x\ZeitBoard.exe"' $v
        Set-ZbStartupEntry -TargetPath 'C:\x\ZeitBoard.exe' -Enabled $false -RunKey $sandbox
        $after = Get-ItemProperty -Path $sandbox -Name 'ZeitBoard' -ErrorAction SilentlyContinue
        Assert-True ($null -eq $after) 'entry should be gone after disable'
    }
    finally {
        Remove-Item -Path 'HKCU:\Software\ZeitBoardTest' -Recurse -Force -ErrorAction SilentlyContinue
    }
}

Test-Case 'Set-ZbStartupEntry preserves a same-named entry it does not own' {
    $sandbox = 'HKCU:\Software\ZeitBoardTest\Run'
    try {
        New-Item -Path $sandbox -Force | Out-Null
        Set-ItemProperty -Path $sandbox -Name 'ZeitBoard' -Value '"C:\other\ZeitBoard.exe"' -Force
        Set-ZbStartupEntry -TargetPath 'C:\x\ZeitBoard.exe' -Enabled $true -RunKey $sandbox
        $enabledValue = (Get-ItemProperty -Path $sandbox -Name 'ZeitBoard').ZeitBoard
        Assert-Equal '"C:\other\ZeitBoard.exe"' $enabledValue
        Set-ZbStartupEntry -TargetPath 'C:\x\ZeitBoard.exe' -Enabled $false -RunKey $sandbox
        $value = (Get-ItemProperty -Path $sandbox -Name 'ZeitBoard').ZeitBoard
        Assert-Equal '"C:\other\ZeitBoard.exe"' $value
    }
    finally {
        Remove-Item -Path 'HKCU:\Software\ZeitBoardTest' -Recurse -Force -ErrorAction SilentlyContinue
    }
}

Test-Case 'Invoke-ZbStep: -Check short-circuits the action' {
    $state = [pscustomobject]@{ Ran = $false }
    Invoke-ZbStep -Name 'noop' -Check { $true } -Action { $state.Ran = $true } | Out-Null
    Assert-True (-not $state.Ran) 'action must not run when Check returns true'
}

Test-Case 'Invoke-ZbStep: -DryRun never runs the action' {
    $state = [pscustomobject]@{ Ran = $false }
    Invoke-ZbStep -Name 'noop' -DryRun -Action { $state.Ran = $true } | Out-Null
    Assert-True (-not $state.Ran) 'action must not run under -DryRun'
}

Test-Case 'Show-ZbBanner and Show-ZbFinale are ASCII-only' {
    $out = (Show-ZbBanner *>&1 | Out-String) + (Show-ZbFinale -Kind install *>&1 | Out-String)
    $nonAscii = ($out.ToCharArray() | Where-Object { [int][char]$_ -gt 126 })
    Assert-Equal 0 $nonAscii.Count
}

Write-Host ''
Write-Host "  $script:pass passed, $script:fail failed" -ForegroundColor $(if ($script:fail -eq 0) { 'Green' } else { 'Red' })
if ($script:fail -gt 0) { exit 1 }
exit 0

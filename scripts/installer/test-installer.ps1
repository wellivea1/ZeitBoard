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

function Get-InstallerFunctionDefinition {
    param(
        [Parameter(Mandatory)][string]$ScriptName,
        [Parameter(Mandatory)][string]$FunctionName
    )
    $scriptPath = Join-Path $PSScriptRoot $ScriptName
    $tokens = $null
    $parseErrors = $null
    $ast = [Management.Automation.Language.Parser]::ParseFile($scriptPath, [ref]$tokens, [ref]$parseErrors)
    if ($parseErrors.Count -gt 0) {
        throw "Cannot import $FunctionName from ${ScriptName}: $($parseErrors[0].Message)"
    }
    $definition = $ast.Find({
        param($node)
        $node -is [Management.Automation.Language.FunctionDefinitionAst] -and
            $node.Name -eq $FunctionName
    }, $true)
    if (-not $definition) { throw "Function $FunctionName was not found in $ScriptName" }
    [scriptblock]::Create($definition.Extent.Text)
}

# Import only pure helper definitions; do not execute either installer script.
. (Get-InstallerFunctionDefinition -ScriptName 'update.ps1' -FunctionName 'Get-ZbInstalledReleaseMetadata')
. (Get-InstallerFunctionDefinition -ScriptName 'update.ps1' -FunctionName 'Get-ZbUpdateDecision')
. (Get-InstallerFunctionDefinition -ScriptName 'install-server.ps1' -FunctionName 'Test-ZbRestrictedAclPolicy')
. (Get-InstallerFunctionDefinition -ScriptName 'install-server.ps1' -FunctionName 'Test-ZbAclPolicyMarker')

function Remove-TestTree {
    param([Parameter(Mandatory)][string]$Path)
    $tempRoot = [IO.Path]::GetTempPath()
    $safePath = Assert-ZbChildPath -Root $tempRoot -Path $Path
    if (Test-Path -LiteralPath $safePath) {
        Remove-Item -LiteralPath $safePath -Recurse -Force -ErrorAction SilentlyContinue
    }
}

function Publish-TestRelease {
    param(
        [Parameter(Mandatory)][string]$InstallDir,
        [Parameter(Mandatory)][string]$DesktopSource,
        [Parameter(Mandatory)][string]$LocalMcpSource,
        [string]$ServerMcpSource,
        [Parameter(Mandatory)][string]$Commit
    )
    $components = @('desktop', 'local-mcp')
    if ($ServerMcpSource) { $components += 'mcp' }
    $backupDir = Join-Path $InstallDir ('.test-publish-backup-' + [guid]::NewGuid().ToString('N'))
    try {
        Start-ZbPublishTransaction -InstallDir $InstallDir -Components $components | Out-Null
        Publish-ZbDesktopBuild -SourceExe $DesktopSource -InstallDir $InstallDir -VersionText "commit=$Commit`ndate=test`ncomponents=$($components -join ',')"
        $localHash = Publish-ZbVerifiedFile -SourcePath $LocalMcpSource -DestinationPath (Join-Path $InstallDir 'zeitboard-local-mcp.exe') -BackupPath (Join-Path $backupDir 'zeitboard-local-mcp.exe')
        Set-ZbInstalledArtifactHash -InstallDir $InstallDir -Key 'local-mcp-sha256' -Hash $localHash
        if ($ServerMcpSource) {
            $serverHash = Publish-ZbVerifiedFile -SourcePath $ServerMcpSource -DestinationPath (Join-Path $InstallDir 'zeitboard-mcp.exe') -BackupPath (Join-Path $backupDir 'zeitboard-mcp.exe')
            Set-ZbInstalledArtifactHash -InstallDir $InstallDir -Key 'mcp-sha256' -Hash $serverHash
        }
        Complete-ZbPublishTransaction -InstallDir $InstallDir
    }
    finally {
        if (Test-Path -LiteralPath $backupDir) {
            Remove-ZbDirectoryUnderRoot -Root $InstallDir -Path $backupDir
        }
    }
}

Write-Host 'ZeitBoard installer library tests' -ForegroundColor Cyan

Test-Case 'Get-ZbPaths resolves the repo root to the working tree' {
    $p = Get-ZbPaths
    Assert-True (Test-Path (Join-Path $p.RepoRoot 'go.work')) 'repo root should contain go.work'
    Assert-True ($p.InstallDir -like '*Programs\ZeitBoard') 'install dir under Programs'
    Assert-True ($p.DataDir -like '*ZeitBoard') 'data dir named ZeitBoard'
}

Test-Case 'update no-op requires an exact verified release and ForceRebuild overrides it' {
    $head = 'a' * 40
    $exact = Get-ZbUpdateDecision `
        -InstalledBuildVerified $true `
        -InstalledCommit $head `
        -InstalledComponents @('LOCAL-MCP', 'desktop') `
        -HeadCommit $head `
        -RequestedComponents @('desktop', 'local-mcp') `
        -RepositoryClean $true
    Assert-True (-not $exact.ShouldRebuild) 'an exact verified release should be the only no-op case'

    $cases = @(
        (Get-ZbUpdateDecision -InstalledBuildVerified $false -InstalledCommit $head -InstalledComponents @('desktop', 'local-mcp') -HeadCommit $head -RequestedComponents @('desktop', 'local-mcp') -RepositoryClean $true),
        (Get-ZbUpdateDecision -InstalledBuildVerified $true -InstalledCommit ('b' * 40) -InstalledComponents @('desktop', 'local-mcp') -HeadCommit $head -RequestedComponents @('desktop', 'local-mcp') -RepositoryClean $true),
        (Get-ZbUpdateDecision -InstalledBuildVerified $true -InstalledCommit '' -InstalledComponents @('desktop', 'local-mcp') -HeadCommit $head -RequestedComponents @('desktop', 'local-mcp') -RepositoryClean $true),
        (Get-ZbUpdateDecision -InstalledBuildVerified $true -InstalledCommit $head -InstalledComponents @('desktop', 'local-mcp') -HeadCommit '' -RequestedComponents @('desktop', 'local-mcp') -RepositoryClean $true),
        (Get-ZbUpdateDecision -InstalledBuildVerified $true -InstalledCommit $head -InstalledComponents @('desktop') -HeadCommit $head -RequestedComponents @('desktop', 'local-mcp') -RepositoryClean $true),
        (Get-ZbUpdateDecision -InstalledBuildVerified $true -InstalledCommit $head -InstalledComponents @('desktop', 'local-mcp', 'local-mcp') -HeadCommit $head -RequestedComponents @('desktop', 'local-mcp') -RepositoryClean $true),
        (Get-ZbUpdateDecision -InstalledBuildVerified $true -InstalledCommit $head -InstalledComponents @('desktop', 'local-mcp') -HeadCommit $head -RequestedComponents @('desktop', 'local-mcp') -RepositoryClean $false),
        (Get-ZbUpdateDecision -InstalledBuildVerified $true -InstalledCommit $head -InstalledComponents @('desktop', 'local-mcp') -HeadCommit $head -RequestedComponents @('desktop', 'local-mcp') -RepositoryClean $true -ForceRebuild)
    )
    foreach ($decision in $cases) {
        Assert-True $decision.ShouldRebuild 'any verification, commit, component, cleanliness, or force difference must rebuild'
    }
}

Test-Case 'installed release metadata rejects missing, blank, and duplicate declarations' {
    $root = Join-Path ([IO.Path]::GetTempPath()) ('zb-release-metadata-' + [guid]::NewGuid().ToString('N'))
    New-Item -ItemType Directory -Path $root -Force | Out-Null
    try {
        $version = Join-Path $root 'version.txt'
        Set-ZbUtf8File -Path $version -Content "commit=abcdef1`ncomponents=desktop,local-mcp`n"
        $metadata = Get-ZbInstalledReleaseMetadata -InstallDir $root
        Assert-Equal 'abcdef1' $metadata.Commit
        Assert-Equal 'desktop,local-mcp' ($metadata.Components -join ',')

        Set-ZbUtf8File -Path $version -Content "commit=abcdef1`ncomponents=desktop,,local-mcp`n"
        Assert-True ($null -eq (Get-ZbInstalledReleaseMetadata -InstallDir $root)) 'blank component entries must not qualify for no-op'
        Set-ZbUtf8File -Path $version -Content "commit=abcdef1`ncomponents=desktop,local-mcp,local-mcp`n"
        Assert-True ($null -eq (Get-ZbInstalledReleaseMetadata -InstallDir $root)) 'duplicate component entries must not qualify for no-op'
        Set-ZbUtf8File -Path $version -Content "commit=abcdef1`ncommit=abcdef2`ncomponents=desktop,local-mcp`n"
        Assert-True ($null -eq (Get-ZbInstalledReleaseMetadata -InstallDir $root)) 'duplicate commit declarations must not qualify for no-op'
        Set-ZbUtf8File -Path $version -Content "commit=abcdef1`n"
        Assert-True ($null -eq (Get-ZbInstalledReleaseMetadata -InstallDir $root)) 'missing component metadata must rebuild'
    }
    finally { Remove-TestTree -Path $root }
}

Test-Case 'server ACL policy is protected, exact, and inheritable for new descendants' {
    function New-TestServerAcl {
        param([switch]$Unprotected, [switch]$ExtraPrincipal, [switch]$MissingInheritance)
        $acl = [Security.AccessControl.DirectorySecurity]::new()
        $acl.SetAccessRuleProtection((-not $Unprotected), $false)
        $inheritance = if ($MissingInheritance) {
            [Security.AccessControl.InheritanceFlags]::None
        }
        else {
            [Security.AccessControl.InheritanceFlags]::ContainerInherit -bor
                [Security.AccessControl.InheritanceFlags]::ObjectInherit
        }
        foreach ($sidText in @('S-1-5-18', 'S-1-5-32-544')) {
            $sid = [Security.Principal.SecurityIdentifier]::new($sidText)
            $rule = [Security.AccessControl.FileSystemAccessRule]::new(
                $sid,
                [Security.AccessControl.FileSystemRights]::FullControl,
                $inheritance,
                [Security.AccessControl.PropagationFlags]::None,
                [Security.AccessControl.AccessControlType]::Allow
            )
            [void]$acl.AddAccessRule($rule)
        }
        if ($ExtraPrincipal) {
            $extraRule = [Security.AccessControl.FileSystemAccessRule]::new(
                [Security.Principal.SecurityIdentifier]::new('S-1-5-11'),
                [Security.AccessControl.FileSystemRights]::ReadAndExecute,
                $inheritance,
                [Security.AccessControl.PropagationFlags]::None,
                [Security.AccessControl.AccessControlType]::Allow
            )
            [void]$acl.AddAccessRule($extraRule)
        }
        $acl
    }

    Assert-True (Test-ZbRestrictedAclPolicy -Acl (New-TestServerAcl)) 'exact protected OI/CI policy should be reusable without a recursive rewrite'
    Assert-True (-not (Test-ZbRestrictedAclPolicy -Acl (New-TestServerAcl -Unprotected))) 'inherited root ACLs are not restricted enough'
    Assert-True (-not (Test-ZbRestrictedAclPolicy -Acl (New-TestServerAcl -ExtraPrincipal))) 'extra principals must fail closed'
    Assert-True (-not (Test-ZbRestrictedAclPolicy -Acl (New-TestServerAcl -MissingInheritance))) 'new descendants must inherit the restricted policy'
}

Test-Case 'server ACL skip marker proves the descendant reset completed' {
    $root = Join-Path ([IO.Path]::GetTempPath()) ('zb-acl-marker-' + [guid]::NewGuid().ToString('N'))
    New-Item -ItemType Directory -Path $root -Force | Out-Null
    try {
        $marker = Join-Path $root '.zeitboard-acl-policy-v2'
        Assert-True (-not (Test-ZbAclPolicyMarker -Path $marker)) 'a missing marker must force reconciliation'
        Set-ZbUtf8File -Path $marker -Content "ZeitBoard server ACL policy v1`n"
        Assert-True (-not (Test-ZbAclPolicyMarker -Path $marker)) 'an old marker must force reconciliation'
        Set-ZbUtf8File -Path $marker -Content "ZeitBoard server ACL policy v2`n"
        Assert-True (Test-ZbAclPolicyMarker -Path $marker) 'the current post-reset marker may skip reconciliation'
    }
    finally {
        Remove-TestTree -Path $root
    }
}

Test-Case 'server installer performs one post-stop safety walk and one ACL reconciliation' {
    $text = Get-Content -Raw -LiteralPath (Join-Path $PSScriptRoot 'install-server.ps1')
    $stopOffset = $text.IndexOf('Stop-Service -Name $ServiceName -Force')
    $walkOffset = $text.IndexOf('Assert-ZbSafeServerRoot -Path $InstallRoot | Out-Null')
    $aclOffset = $text.IndexOf('Set-ZbRestrictedAcl -Path $InstallRoot')
    $buildOffset = $text.IndexOf("Invoke-ZbStep -Name 'Build staged zeitboardd'")
    $resetOffset = $text.IndexOf('& icacls.exe')
    $markerOffset = $text.IndexOf('Set-ZbUtf8File -Path $policyMarker')
    Assert-True ($buildOffset -ge 0 -and $buildOffset -lt $stopOffset) 'expensive build work should finish before service downtime'
    Assert-True ($stopOffset -ge 0 -and $stopOffset -lt $walkOffset) 'an owned running service must stop before the safety walk'
    Assert-True ($walkOffset -lt $aclOffset) 'the fail-closed safety walk must precede recursive ACL work'
    Assert-Equal 1 ([regex]::Matches($text, '(?m)^\s*Assert-ZbSafeServerRoot -Path \$InstallRoot \| Out-Null\s*$').Count)
    Assert-Equal 1 ([regex]::Matches($text, '(?m)^\s*Set-ZbRestrictedAcl -Path \$InstallRoot\s*$').Count)
	Assert-Equal 1 ([regex]::Matches($text, '(?m)^\s*& icacls\.exe .*''/reset''.*''/T''').Count)
	Assert-True ($text -match 'DirectorySecurity\]::new') 'reconciliation must replace the root DACL rather than preserve unknown explicit principals'
	Assert-True ($text -notmatch "'/grant:r'") 'grant:r cannot remove unrelated explicit principals'
    Assert-True ($text -match 'Test-ZbRestrictedAclPolicy -Acl \(Get-Acl') 'the root ACL policy must be checked before a skip'
    Assert-True ($text -match '\$rootPolicyCurrent -and \(Test-ZbAclPolicyMarker') 'a root-only match must not skip descendant reconciliation'
    Assert-True ($resetOffset -ge 0 -and $resetOffset -lt $markerOffset) 'the marker must be written only after descendant reset succeeds'
}

Test-Case 'update decision runs before all expensive or mutating phases' {
    $text = Get-Content -Raw -LiteralPath (Join-Path $PSScriptRoot 'update.ps1')
    $decisionOffset = $text.IndexOf('$decision = Get-ZbUpdateDecision')
    Assert-True ($decisionOffset -ge 0 -and $decisionOffset -lt $text.IndexOf('& npm ci')) 'decision must precede npm'
    Assert-True ($decisionOffset -lt $text.IndexOf("Invoke-ZbStep -Name 'Run test suites")) 'decision must precede tests'
    Assert-True ($decisionOffset -lt $text.IndexOf("Invoke-ZbStep -Name 'Build desktop'")) 'decision must precede builds'
    Assert-True ($decisionOffset -lt $text.IndexOf("Backup-ZbData -Reason 'update'")) 'decision must precede backup'
    Assert-True ($decisionOffset -lt $text.IndexOf('Start-ZbPublishTransaction')) 'decision must precede publication'
    Assert-True ($text -match "restartArgs \+= '-ForceRebuild'") 'ForceRebuild must survive the post-pull restart'
    Assert-True ($text.Contains("`$resume += ' -ForceRebuild'")) 'ForceRebuild must survive a failure resume hint'
    Assert-True ($text -match '\[dry-run\]') 'dry-run must label the release-decision explanation'
    Assert-True ($text -match 'live run would exit here only after fetch') 'dry-run must explain the conditional no-op without fetching'
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
        Remove-TestTree -Path $root
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
    finally { Remove-TestTree -Path $root }
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
    finally { Remove-TestTree -Path $root }
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
    finally { Remove-TestTree -Path $root }
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
        Remove-TestTree -Path $base
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
        Remove-TestTree -Path $root
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
    finally { Remove-TestTree -Path $root }
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
        Remove-TestTree -Path $root
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
    finally { Remove-TestTree -Path $root }
}

Test-Case 'desktop and MCP companions publish, verify, and roll back coherently' {
    $root = Join-Path ([IO.Path]::GetTempPath()) ('zb-publish-' + [guid]::NewGuid().ToString('N'))
    $install = Join-Path $root 'install'
    $first = Join-Path $root 'first.exe'
    $second = Join-Path $root 'second.exe'
    $firstMcp = Join-Path $root 'first-mcp.exe'
    $secondMcp = Join-Path $root 'second-mcp.exe'
    $firstLocalMcp = Join-Path $root 'first-local-mcp.exe'
    $secondLocalMcp = Join-Path $root 'second-local-mcp.exe'
    New-Item -ItemType Directory -Path $root -Force | Out-Null
    try {
        Set-ZbUtf8File -Path $first -Content 'first-build'
        Set-ZbUtf8File -Path $second -Content 'second-build'
        Set-ZbUtf8File -Path $firstMcp -Content 'first-mcp-build'
        Set-ZbUtf8File -Path $secondMcp -Content 'second-mcp-build'
        Set-ZbUtf8File -Path $firstLocalMcp -Content 'first-local-mcp-build'
        Set-ZbUtf8File -Path $secondLocalMcp -Content 'second-local-mcp-build'

        Publish-TestRelease -InstallDir $install -DesktopSource $first -LocalMcpSource $firstLocalMcp -ServerMcpSource $firstMcp -Commit 'one'
        Assert-True (Test-ZbInstalledBuild -InstallDir $install) 'first release should verify'

        Publish-TestRelease -InstallDir $install -DesktopSource $second -LocalMcpSource $secondLocalMcp -ServerMcpSource $secondMcp -Commit 'two'
        Assert-True (Test-ZbInstalledBuild -InstallDir $install) 'second release should verify'
        Assert-Equal 'first-build' (Get-Content -Raw -LiteralPath (Join-Path $install 'previous\ZeitBoard.exe'))
        Assert-Equal 'first-mcp-build' (Get-Content -Raw -LiteralPath (Join-Path $install 'previous\zeitboard-mcp.exe'))
        Assert-Equal 'first-local-mcp-build' (Get-Content -Raw -LiteralPath (Join-Path $install 'previous\zeitboard-local-mcp.exe'))

        Set-ZbUtf8File -Path (Join-Path $install 'zeitboard-mcp.exe') -Content 'corrupted-mcp'
        Assert-True (-not (Test-ZbInstalledBuild -InstallDir $install)) 'MCP tampering should fail hash verification'
        Set-ZbUtf8File -Path (Join-Path $install 'zeitboard-mcp.exe') -Content 'second-mcp-build'
        Assert-True (Test-ZbInstalledBuild -InstallDir $install) 'restored MCP bytes should verify'

        Set-ZbUtf8File -Path (Join-Path $install 'zeitboard-local-mcp.exe') -Content 'corrupted-local-mcp'
        Assert-True (-not (Test-ZbInstalledBuild -InstallDir $install)) 'desktop-local MCP tampering should fail hash verification'
        Set-ZbUtf8File -Path (Join-Path $install 'zeitboard-local-mcp.exe') -Content 'second-local-mcp-build'

        Set-ZbUtf8File -Path (Join-Path $install 'ZeitBoard.exe') -Content 'corrupted-desktop'
        Assert-True (-not (Test-ZbInstalledBuild -InstallDir $install)) 'desktop tampering should fail hash verification'

        Restore-ZbPreviousBuild -InstallDir $install
        Assert-True (Test-ZbInstalledBuild -InstallDir $install) 'rollback should restore a coherent release'
        Assert-Equal 'first-build' (Get-Content -Raw -LiteralPath (Join-Path $install 'ZeitBoard.exe'))
        Assert-Equal 'first-mcp-build' (Get-Content -Raw -LiteralPath (Join-Path $install 'zeitboard-mcp.exe'))
        Assert-Equal 'first-local-mcp-build' (Get-Content -Raw -LiteralPath (Join-Path $install 'zeitboard-local-mcp.exe'))
        $version = Get-Content -Raw -LiteralPath (Join-Path $install 'version.txt')
        Assert-True ($version -match 'commit=one') 'rollback should restore previous version metadata'
        Assert-True ($version -match 'mcp-sha256=') 'rollback metadata should include the previous MCP hash'
        Assert-True ($version -match 'local-mcp-sha256=') 'rollback metadata should include the previous local MCP hash'
    }
    finally { Remove-TestTree -Path $root }
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
        Remove-TestTree -Path $root
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
    finally { Remove-TestTree -Path $root }
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
        Remove-TestTree -Path $root
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

Test-Case 'pending component requirements do not invalidate the old release snapshot' {
    $root = Join-Path ([IO.Path]::GetTempPath()) ('zb-upgrade-manifest-' + [guid]::NewGuid().ToString('N'))
    New-Item -ItemType Directory -Path $root | Out-Null
    try {
        $exe = Join-Path $root 'ZeitBoard.exe'
        Set-ZbUtf8File -Path $exe -Content 'old-structured-desktop'
        $hash = (Get-FileHash -Algorithm SHA256 -LiteralPath $exe).Hash.ToLowerInvariant()
        Set-ZbUtf8File -Path (Join-Path $root 'version.txt') -Content "commit=old`ndate=old`nsha256=$hash`n"
        Start-ZbPublishTransaction -InstallDir $root -Components @('desktop', 'local-mcp') | Out-Null

        Assert-True (-not (Test-ZbInstalledBuild -InstallDir $root)) 'the pending release must not be reported complete'
        Assert-True (Test-ZbInstalledBuild -InstallDir $root -IgnorePendingMarker -IgnorePendingComponents) 'old release validation must ignore in-flight component declarations'
        Assert-Throws { Complete-ZbPublishTransaction -InstallDir $root } 'completion must still enforce pending component requirements'
        Save-ZbPreviousRelease -InstallDir $root
        Assert-True (Test-ZbInstalledBuild -InstallDir (Join-Path $root 'previous')) 'the old structured release must remain snapshot-compatible'
    }
    finally { Remove-TestTree -Path $root }
}

Test-Case 'Publish transaction: pending marker makes a half-published install fail closed' {
    $dir = Join-Path ([IO.Path]::GetTempPath()) ("zb-tx-" + [guid]::NewGuid().ToString('N'))
    try {
        Start-ZbPublishTransaction -InstallDir $dir -Components @('desktop', 'local-mcp') | Out-Null
        Assert-True (Test-Path -LiteralPath (Get-ZbPendingMarkerPath -InstallDir $dir)) 'marker should exist'
        # Desktop published, local-mcp not yet: still inside the transaction.
        Set-Content -LiteralPath (Join-Path $dir 'ZeitBoard.exe') -Value 'x'
        Assert-Equal $false (Test-ZbInstalledBuild -InstallDir $dir)
        # Declared-but-missing component fails even ignoring the marker.
        Set-Content -LiteralPath (Join-Path $dir 'version.txt') -Value "commit=abc`ndate=now`ncomponents=desktop,local-mcp"
        Assert-Equal $false (Test-ZbInstalledBuild -InstallDir $dir -IgnorePendingMarker)
    }
    finally { Remove-TestTree -Path $dir }
}

Test-Case 'Publish transaction: completing clears the marker once everything validates' {
    $dir = Join-Path ([IO.Path]::GetTempPath()) ("zb-tx-" + [guid]::NewGuid().ToString('N'))
    try {
        Start-ZbPublishTransaction -InstallDir $dir -Components @('desktop', 'local-mcp') | Out-Null
        $exe = Join-Path $dir 'ZeitBoard.exe'
        $localMcp = Join-Path $dir 'zeitboard-local-mcp.exe'
        Set-Content -LiteralPath $exe -Value 'x'
        Set-Content -LiteralPath $localMcp -Value 'bridge'
        $hash = (Get-FileHash -Algorithm SHA256 -LiteralPath $exe).Hash.ToLowerInvariant()
        $localMcpHash = (Get-FileHash -Algorithm SHA256 -LiteralPath $localMcp).Hash.ToLowerInvariant()
        Set-Content -LiteralPath (Join-Path $dir 'version.txt') -Value "commit=abc`ndate=now`ncomponents=desktop,local-mcp`nsha256=$hash`nlocal-mcp-sha256=$localMcpHash"
        Complete-ZbPublishTransaction -InstallDir $dir
        Assert-True (-not (Test-Path -LiteralPath (Get-ZbPendingMarkerPath -InstallDir $dir))) 'marker should be cleared'
        Assert-Equal $true (Test-ZbInstalledBuild -InstallDir $dir)
    }
    finally { Remove-TestTree -Path $dir }
}

Test-Case 'new component manifests require the always-installed local bridge and hash' {
    $root = Join-Path ([IO.Path]::GetTempPath()) ('zb-components-' + [guid]::NewGuid().ToString('N'))
    New-Item -ItemType Directory -Path $root | Out-Null
    try {
        $exe = Join-Path $root 'ZeitBoard.exe'
        $localMcp = Join-Path $root 'zeitboard-local-mcp.exe'
        Set-ZbUtf8File -Path $exe -Content 'desktop'
        $desktopHash = (Get-FileHash -Algorithm SHA256 -LiteralPath $exe).Hash.ToLowerInvariant()
        Set-ZbUtf8File -Path (Join-Path $root 'version.txt') -Content "commit=x`ndate=x`ncomponents=desktop`nsha256=$desktopHash`n"
        Assert-True (-not (Test-ZbInstalledBuild -InstallDir $root)) 'a component manifest may not omit the required local bridge'

        Set-ZbUtf8File -Path $localMcp -Content 'bridge'
        Set-ZbUtf8File -Path (Join-Path $root 'version.txt') -Content "commit=x`ndate=x`ncomponents=desktop,local-mcp`nsha256=$desktopHash`n"
        Assert-True (-not (Test-ZbInstalledBuild -InstallDir $root)) 'a declared local bridge without a hash must fail closed'

        $localHash = (Get-FileHash -Algorithm SHA256 -LiteralPath $localMcp).Hash.ToLowerInvariant()
        Set-ZbInstalledArtifactHash -InstallDir $root -Key 'local-mcp-sha256' -Hash $localHash
        Assert-True (Test-ZbInstalledBuild -InstallDir $root) 'a complete component manifest should validate'
    }
    finally { Remove-TestTree -Path $root }
}

Test-Case 'snapshot switching and rollback faults preserve coherent releases' {
    $root = Join-Path ([IO.Path]::GetTempPath()) ('zb-fault-' + [guid]::NewGuid().ToString('N'))
    $install = Join-Path $root 'install'
    New-Item -ItemType Directory -Path $root | Out-Null
    $sources = @{}
    foreach ($release in @('one', 'two', 'three')) {
        $desktop = Join-Path $root "$release-desktop.exe"
        $localMcp = Join-Path $root "$release-local-mcp.exe"
        $serverMcp = Join-Path $root "$release-server-mcp.exe"
        Set-ZbUtf8File -Path $desktop -Content "$release-desktop"
        Set-ZbUtf8File -Path $localMcp -Content "$release-local"
        Set-ZbUtf8File -Path $serverMcp -Content "$release-server"
        $sources[$release] = @{ Desktop = $desktop; Local = $localMcp; Server = $serverMcp }
    }
    try {
        Publish-TestRelease -InstallDir $install -DesktopSource $sources.one.Desktop -LocalMcpSource $sources.one.Local -ServerMcpSource $sources.one.Server -Commit 'one'
        Publish-TestRelease -InstallDir $install -DesktopSource $sources.two.Desktop -LocalMcpSource $sources.two.Local -ServerMcpSource $sources.two.Server -Commit 'two'
        Assert-True (Test-ZbInstalledBuild -InstallDir $install) 'current release should begin coherent'
        Assert-True (Test-ZbInstalledBuild -InstallDir (Join-Path $install 'previous')) 'previous release should begin coherent'

        $script:ZbInstallerFaultPoint = 'previous-after-retire'
        Assert-Throws {
            Publish-ZbDesktopBuild -SourceExe $sources.three.Desktop -InstallDir $install -VersionText "commit=three`ndate=test`ncomponents=desktop,local-mcp,mcp"
        } 'an interrupted previous-directory switch must fail'
        $script:ZbInstallerFaultPoint = $null
        Assert-Equal 'two-desktop' (Get-Content -Raw -LiteralPath (Join-Path $install 'ZeitBoard.exe'))
        Assert-Equal 'one-desktop' (Get-Content -Raw -LiteralPath (Join-Path $install 'previous\ZeitBoard.exe'))
        Assert-True (Test-ZbInstalledBuild -InstallDir $install) 'snapshot-switch failure must leave current coherent'
        Assert-True (Test-ZbInstalledBuild -InstallDir (Join-Path $install 'previous')) 'snapshot-switch failure must restore previous coherently'

        $script:ZbInstallerFaultPoint = 'restore-after-ZeitBoard.exe'
        Assert-Throws { Restore-ZbPreviousBuild -InstallDir $install } 'a mid-rollback fault must fail'
        $script:ZbInstallerFaultPoint = $null
        Assert-Equal 'two-desktop' (Get-Content -Raw -LiteralPath (Join-Path $install 'ZeitBoard.exe'))
        Assert-Equal 'two-local' (Get-Content -Raw -LiteralPath (Join-Path $install 'zeitboard-local-mcp.exe'))
        Assert-Equal 'two-server' (Get-Content -Raw -LiteralPath (Join-Path $install 'zeitboard-mcp.exe'))
        Assert-True (Test-ZbInstalledBuild -InstallDir $install) 'failed rollback must restore every current artifact and metadata'

        Set-ZbUtf8File -Path (Join-Path $install 'previous\zeitboard-local-mcp.exe') -Content 'corrupt'
        Assert-Throws { Restore-ZbPreviousBuild -InstallDir $install } 'a corrupt previous release must fail prevalidation'
        Assert-True (Test-ZbInstalledBuild -InstallDir $install) 'prevalidation failure must not touch the current release'
        $transactionTemps = @(Get-ChildItem -LiteralPath $install -Force | Where-Object { $_.Name -match '^\.(previous-|rollback-|release-backup-)' })
        Assert-Equal 0 $transactionTemps.Count
    }
    finally {
        $script:ZbInstallerFaultPoint = $null
        Remove-TestTree -Path $root
    }
}

Test-Case 'an interrupted previous-directory switch repairs from the staged release' {
    $root = Join-Path ([IO.Path]::GetTempPath()) ('zb-switch-repair-' + [guid]::NewGuid().ToString('N'))
    $install = Join-Path $root 'install'
    $firstDesktop = Join-Path $root 'first.exe'
    $firstLocal = Join-Path $root 'first-local.exe'
    $secondDesktop = Join-Path $root 'second.exe'
    $secondLocal = Join-Path $root 'second-local.exe'
    New-Item -ItemType Directory -Path $root | Out-Null
    try {
        Set-ZbUtf8File -Path $firstDesktop -Content 'first'
        Set-ZbUtf8File -Path $firstLocal -Content 'first-local'
        Set-ZbUtf8File -Path $secondDesktop -Content 'second'
        Set-ZbUtf8File -Path $secondLocal -Content 'second-local'
        Publish-TestRelease -InstallDir $install -DesktopSource $firstDesktop -LocalMcpSource $firstLocal -Commit 'one'
        Publish-TestRelease -InstallDir $install -DesktopSource $secondDesktop -LocalMcpSource $secondLocal -Commit 'two'

        $staged = Join-Path $install '.previous-staging-interrupted'
        $retired = Join-Path $install '.previous-retired-interrupted'
        Copy-ZbReleaseSnapshot -SourceDir $install -DestinationDir $staged
        Move-Item -LiteralPath (Join-Path $install 'previous') -Destination $retired
        Assert-True (-not (Test-Path -LiteralPath (Join-Path $install 'previous'))) 'test must model the directory-switch interruption window'

        Repair-ZbPreviousReleaseSwitch -InstallDir $install
        Assert-Equal 'second' (Get-Content -Raw -LiteralPath (Join-Path $install 'previous\ZeitBoard.exe'))
        Assert-True (Test-ZbInstalledBuild -InstallDir (Join-Path $install 'previous')) 'recovered previous directory must validate'
        Assert-True (-not (Test-Path -LiteralPath $staged)) 'staged directory should be consumed'
        Assert-True (-not (Test-Path -LiteralPath $retired)) 'retired directory should be safely cleaned'
    }
    finally { Remove-TestTree -Path $root }
}

Test-Case 'rollback restores a valid previous release when the active desktop is missing' {
    $root = Join-Path ([IO.Path]::GetTempPath()) ('zb-missing-current-' + [guid]::NewGuid().ToString('N'))
    $install = Join-Path $root 'install'
    $firstDesktop = Join-Path $root 'first.exe'
    $firstLocal = Join-Path $root 'first-local.exe'
    $secondDesktop = Join-Path $root 'second.exe'
    $secondLocal = Join-Path $root 'second-local.exe'
    New-Item -ItemType Directory -Path $root | Out-Null
    try {
        Set-ZbUtf8File -Path $firstDesktop -Content 'first'
        Set-ZbUtf8File -Path $firstLocal -Content 'first-local'
        Set-ZbUtf8File -Path $secondDesktop -Content 'second'
        Set-ZbUtf8File -Path $secondLocal -Content 'second-local'
        Publish-TestRelease -InstallDir $install -DesktopSource $firstDesktop -LocalMcpSource $firstLocal -Commit 'one'
        Publish-TestRelease -InstallDir $install -DesktopSource $secondDesktop -LocalMcpSource $secondLocal -Commit 'two'
        Remove-Item -LiteralPath (Join-Path $install 'ZeitBoard.exe') -Force
        Restore-ZbPreviousBuild -InstallDir $install
        Assert-Equal 'first' (Get-Content -Raw -LiteralPath (Join-Path $install 'ZeitBoard.exe'))
        Assert-True (Test-ZbInstalledBuild -InstallDir $install) 'rollback should recover a complete verified release'
    }
    finally { Remove-TestTree -Path $root }
}

Test-Case 'local MCP smoke remains read-only and drains stderr asynchronously' {
    $text = Get-Content -Raw -LiteralPath (Join-Path $PSScriptRoot '..\smoke-local-mcp.ps1')
    Assert-True ($text -notmatch "name\s*=\s*'set_appearance'") 'installed-profile smoke must not mutate appearance'
    Assert-True ($text -match 'StandardError\.ReadToEndAsync\(\)') 'stderr must be drained as soon as the bridge starts'
    Assert-True ($text -match 'MCP bridge stderr:') 'failure diagnostics must include captured stderr'
}

Test-Case 'Publish transaction is actually wired into install.ps1 and update.ps1' {
    foreach ($script in @('install.ps1', 'update.ps1')) {
        $text = Get-Content -Raw -LiteralPath (Join-Path $PSScriptRoot $script)
        Assert-True ($text -match 'Start-ZbPublishTransaction') "$script must open a publish transaction"
        Assert-True ($text -match 'Complete-ZbPublishTransaction') "$script must complete the publish transaction"
    }
}

Write-Host ''
Write-Host "  $script:pass passed, $script:fail failed" -ForegroundColor $(if ($script:fail -eq 0) { 'Green' } else { 'Red' })
if ($script:fail -gt 0) { exit 1 }
exit 0

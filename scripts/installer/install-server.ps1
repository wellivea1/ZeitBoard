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
    -ListenAddress       initial or replacement bind address
    -TlsCertPath         TLS certificate path (supply with -TlsKeyPath)
    -TlsKeyPath          TLS private-key path (supply with -TlsCertPath)
    -Firewall            reconcile a private-profile firewall rule
    -NonInteractive / -DryRun

  Requires elevation to register the service. Run this from an elevated prompt.
#>
[CmdletBinding()]
param(
    [string]$InstallRoot = (Join-Path $env:ProgramData 'ZeitBoard'),
    [string]$ServiceName = 'ZeitBoardServer',
    [string]$ListenAddress = '127.0.0.1:8765',
    [string]$TlsCertPath,
    [string]$TlsKeyPath,
    [switch]$Firewall,
    [switch]$NonInteractive,
    [switch]$DryRun
)

$ErrorActionPreference = 'Stop'
. "$PSScriptRoot\_zb.common.ps1"

$resume = "powershell -ExecutionPolicy Bypass -File `"$PSCommandPath`""
Start-ZbLog -Name 'server' -DryRun:$DryRun | Out-Null
Show-ZbBanner
$paths = Get-ZbPaths
$InstallRoot = $ExecutionContext.SessionState.Path.GetUnresolvedProviderPathFromPSPath($InstallRoot)
$configPath = Join-Path $InstallRoot 'config.json'
$listenBound = $PSBoundParameters.ContainsKey('ListenAddress')
$certBound = $PSBoundParameters.ContainsKey('TlsCertPath')
$keyBound = $PSBoundParameters.ContainsKey('TlsKeyPath')
$firewallBound = $PSBoundParameters.ContainsKey('Firewall')
$script:ZbServiceCommitted = $false
$script:ZbPublishStarted = $false
$script:ZbStoppedExistingService = $false
$lifecycleLock = $null
$exitCode = 0
$script:ZbExistingService = $null
$script:ZbExistingServiceWasRunning = $false
$script:ZbOriginalRegistration = $null
$script:ZbServiceKey = $null
$exe = Join-Path $InstallRoot 'zeitboardd.exe'
$stageRoot = Join-Path ([IO.Path]::GetTempPath()) ('zeitboard-server-stage-' + [guid]::NewGuid().ToString('N'))
$stageExe = Join-Path $stageRoot 'zeitboardd.exe'
$stageConfig = Join-Path $stageRoot 'config.json'
$serviceLog = Join-Path $InstallRoot 'logs\zeitboardd.log'
$previousDir = Join-Path $InstallRoot 'previous'
$previousExe = Join-Path $previousDir 'zeitboardd.exe'
$previousConfig = Join-Path $previousDir 'config.json'

function New-ZbSecretFile {
    param([Parameter(Mandatory)][string]$Path)
    if (Test-Path -LiteralPath $Path) {
        if ((Get-Item -LiteralPath $Path).Length -eq 0) {
            throw "Existing secret file is empty, possibly from an interrupted write; refusing to overwrite it automatically: $Path"
        }
        Write-ZbLog -Message "secret exists (kept): $Path"
        return
    }
    $bytes = New-Object byte[] 32
    $rng = [Security.Cryptography.RandomNumberGenerator]::Create()
    try { $rng.GetBytes($bytes) }
    finally { $rng.Dispose() }
    $parent = Split-Path -Parent $Path
    New-Item -ItemType Directory -Force -Path $parent | Out-Null
    $stage = Join-Path $parent ('.secret-' + [guid]::NewGuid().ToString('N'))
    $stream = $null
    try {
        $encoded = [Text.Encoding]::ASCII.GetBytes([Convert]::ToBase64String($bytes))
        $stream = New-Object IO.FileStream($stage, [IO.FileMode]::CreateNew, [IO.FileAccess]::Write, [IO.FileShare]::None)
        $stream.Write($encoded, 0, $encoded.Length)
        $stream.Flush($true)
        $stream.Dispose()
        $stream = $null
        Move-Item -LiteralPath $stage -Destination $Path
        Write-ZbLog -Level ok -Message "generated secret: $Path"
    }
    finally {
        if ($stream) { $stream.Dispose() }
        if (Test-Path -LiteralPath $stage) { Remove-Item -LiteralPath $stage -Force -ErrorAction SilentlyContinue }
    }
}

function Test-ZbLoopbackListen {
    param([Parameter(Mandatory)][string]$Address)
    return $Address -match '^(localhost|127\.0\.0\.1|\[::1\]):[0-9]+$'
}

function Test-ZbRestrictedAclPolicy {
    param([Parameter(Mandatory)]$Acl)

    if (-not $Acl.AreAccessRulesProtected) { return $false }
    $rules = @($Acl.GetAccessRules($true, $true, [Security.Principal.SecurityIdentifier]))
    if ($rules.Count -ne 2) { return $false }

    $requiredSids = @('S-1-5-18', 'S-1-5-32-544')
    $requiredInheritance = [Security.AccessControl.InheritanceFlags]::ContainerInherit -bor
        [Security.AccessControl.InheritanceFlags]::ObjectInherit
    foreach ($rule in $rules) {
        if ($rule.IsInherited -or $requiredSids -notcontains $rule.IdentityReference.Value) { return $false }
        if ($rule.AccessControlType -ne [Security.AccessControl.AccessControlType]::Allow) { return $false }
        if ($rule.FileSystemRights -ne [Security.AccessControl.FileSystemRights]::FullControl) { return $false }
        if ($rule.InheritanceFlags -ne $requiredInheritance) { return $false }
        if ($rule.PropagationFlags -ne [Security.AccessControl.PropagationFlags]::None) { return $false }
    }
    return $true
}

function Test-ZbAclPolicyMarker {
    param([Parameter(Mandatory)][string]$Path)

    if (-not (Test-Path -LiteralPath $Path -PathType Leaf)) { return $false }
    try {
        return (Get-Content -Raw -LiteralPath $Path).Trim() -eq 'ZeitBoard server ACL policy v2'
    }
    catch {
        return $false
    }
}

function Set-ZbRestrictedAcl {
    param([Parameter(Mandatory)][string]$Path)

    # Assert-ZbSafeServerRoot must run exactly once before this helper. It owns
    # the reparse-point walk; this helper only reconciles the DACL policy.
    New-Item -ItemType Directory -Force -Path $Path | Out-Null
    $policyMarker = Join-Path $Path '.zeitboard-acl-policy-v2'
    $rootPolicyCurrent = Test-ZbRestrictedAclPolicy -Acl (Get-Acl -LiteralPath $Path)
    if ($rootPolicyCurrent -and (Test-ZbAclPolicyMarker -Path $policyMarker)) {
        Write-ZbLog -Level ok -Message 'restricted root ACL and descendant-reset marker are current; recursive rewrite skipped'
        return
    }

    $acl = [Security.AccessControl.DirectorySecurity]::new()
    $acl.SetAccessRuleProtection($true, $false)
    $inheritance = [Security.AccessControl.InheritanceFlags]::ContainerInherit -bor
        [Security.AccessControl.InheritanceFlags]::ObjectInherit
    foreach ($sidText in @('S-1-5-18', 'S-1-5-32-544')) {
        $rule = [Security.AccessControl.FileSystemAccessRule]::new(
            [Security.Principal.SecurityIdentifier]::new($sidText),
            [Security.AccessControl.FileSystemRights]::FullControl,
            $inheritance,
            [Security.AccessControl.PropagationFlags]::None,
            [Security.AccessControl.AccessControlType]::Allow
        )
        [void]$acl.AddAccessRule($rule)
    }
    Set-Acl -LiteralPath $Path -AclObject $acl

    # The marker is written only after this reset succeeds. An exact root DACL
    # by itself cannot prove that older descendants lack explicit principals.
    $children = @(Get-ChildItem -LiteralPath $Path -Force)
    if ($children.Count -gt 0) {
        & icacls.exe (Join-Path $Path '*') '/reset' '/T' '/C' | Out-Null
        if ($LASTEXITCODE -ne 0) {
            throw "Failed to reset descendants under $Path (icacls exit $LASTEXITCODE)."
        }
    }
    if (-not (Test-ZbRestrictedAclPolicy -Acl (Get-Acl -LiteralPath $Path))) {
        throw "Server-root ACL is not the exact protected SYSTEM/Administrators policy after reconciliation: $Path"
    }
    Set-ZbUtf8File -Path $policyMarker -Content "ZeitBoard server ACL policy v2`n"
    Write-ZbLog -Level ok -Message 'installed the restricted root ACL and recorded a successful descendant reset'
}

function Invoke-ZbSc {
    param([Parameter(Mandatory)][string[]]$Arguments)
    & sc.exe @Arguments | Out-Null
    if ($LASTEXITCODE -ne 0) {
        throw "sc.exe $($Arguments -join ' ') failed with exit $LASTEXITCODE."
    }
}

try {
    $lifecycleLock = Enter-ZbLifecycleLock
    if (-not $DryRun -and $certBound -and -not [string]::IsNullOrWhiteSpace($TlsCertPath)) {
        if (-not (Test-Path -LiteralPath $TlsCertPath -PathType Leaf)) { throw "TLS certificate not found: $TlsCertPath" }
        $TlsCertPath = (Resolve-Path -LiteralPath $TlsCertPath).Path
    }
    if (-not $DryRun -and $keyBound -and -not [string]::IsNullOrWhiteSpace($TlsKeyPath)) {
        if (-not (Test-Path -LiteralPath $TlsKeyPath -PathType Leaf)) { throw "TLS key not found: $TlsKeyPath" }
        $TlsKeyPath = (Resolve-Path -LiteralPath $TlsKeyPath).Path
    }

    Invoke-ZbStep -Name 'Preflight' -DryRun:$DryRun -ResumeHint $resume -Action {
        $elevated = ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
        if (-not $elevated) {
            throw 'Registering a Windows service needs an elevated prompt. Re-run this script as Administrator.'
        }
        if ($ServiceName -notmatch '^[A-Za-z0-9_.-]+$') {
            throw 'ServiceName may contain only letters, digits, dot, underscore, and hyphen.'
        }
        if ($certBound -xor $keyBound) {
            throw '-TlsCertPath and -TlsKeyPath must be supplied together, including when clearing them.'
        }

        $script:ZbExistingService = Get-Service -Name $ServiceName -ErrorAction SilentlyContinue
        if ($script:ZbExistingService) {
            Assert-ZbServiceOwned -ServiceName $ServiceName -ExpectedExecutable $exe
        }
    }

    Invoke-ZbStep -Name 'Toolchain (Go)' -DryRun:$DryRun -ResumeHint $resume -Action { Assert-ZbGo -DryRun:$DryRun }

    Invoke-ZbStep -Name 'Build staged zeitboardd' -DryRun:$DryRun -ResumeHint $resume -Action {
        New-Item -ItemType Directory -Force -Path $stageRoot | Out-Null
        Push-Location (Join-Path $paths.RepoRoot 'apps\server')
        try { & go build -o $stageExe ./cmd/zeitboardd; if ($LASTEXITCODE -ne 0) { throw 'zeitboardd build failed.' } }
        finally { Pop-Location }
    }

    Invoke-ZbStep -Name 'Stop owned service and secure install root' -DryRun:$DryRun -ResumeHint $resume -Action {
        $currentService = Get-Service -Name $ServiceName -ErrorAction SilentlyContinue
        if ($script:ZbExistingService) {
            if (-not $currentService) { throw "Service '$ServiceName' disappeared during the staged build; refusing to continue." }
            Assert-ZbServiceOwned -ServiceName $ServiceName -ExpectedExecutable $exe
        }
        elseif ($currentService) {
            throw "Service '$ServiceName' appeared during the staged build; refusing to modify it."
        }

        $script:ZbExistingService = $currentService
        if ($currentService) {
            $script:ZbExistingServiceWasRunning = $currentService.Status -ne 'Stopped'
            $script:ZbServiceKey = "HKLM:\SYSTEM\CurrentControlSet\Services\$ServiceName"
            $registration = Get-ItemProperty -LiteralPath $script:ZbServiceKey -ErrorAction Stop
            $startToken = switch ([int]$registration.Start) {
                2 {
                    if ([int]$registration.DelayedAutoStart -eq 1) { 'delayed-auto' } else { 'auto' }
                    break
                }
                3 { 'demand'; break }
                4 { 'disabled'; break }
                default { 'demand' }
            }
            $script:ZbOriginalRegistration = [pscustomobject]@{
                BinPath = [string]$registration.ImagePath
                Start = $startToken
                Description = [string]$registration.Description
            }
        }

        if ($script:ZbExistingServiceWasRunning) {
            Stop-Service -Name $ServiceName -Force -ErrorAction Stop
            $script:ZbStoppedExistingService = $true
            $script:ZbExistingService.WaitForStatus('Stopped', [TimeSpan]::FromSeconds(35))
        }

        # This is the only recursive reparse-point safety walk. It runs after
        # an existing owned service is stopped and before any recursive ACL work.
        Assert-ZbSafeServerRoot -Path $InstallRoot | Out-Null
        New-Item -ItemType Directory -Force -Path $InstallRoot | Out-Null
        Set-ZbRestrictedAcl -Path $InstallRoot
        Set-ZbUtf8File -Path (Join-Path $InstallRoot '.zeitboard-server-root') -Content "ZeitBoard server root v1`n"
    }

    Invoke-ZbStep -Name 'Validate effective server settings' -DryRun:$DryRun -ResumeHint $resume -Action {
        $effectiveListen = $ListenAddress
        $effectiveCert = $TlsCertPath
        $effectiveKey = $TlsKeyPath
        if (Test-Path -LiteralPath $configPath) {
            $existingConfig = Get-Content -Raw -LiteralPath $configPath | ConvertFrom-Json
            if (-not $listenBound) { $effectiveListen = [string]$existingConfig.listenAddress }
            if (-not $certBound) {
                $effectiveCert = [string]$existingConfig.tlsCertPath
                $effectiveKey = [string]$existingConfig.tlsKeyPath
                if (-not [string]::IsNullOrWhiteSpace($effectiveCert)) {
                    $effectiveCert = if ([IO.Path]::IsPathRooted($effectiveCert)) { [IO.Path]::GetFullPath($effectiveCert) } else { [IO.Path]::GetFullPath((Join-Path $InstallRoot $effectiveCert)) }
                }
                if (-not [string]::IsNullOrWhiteSpace($effectiveKey)) {
                    $effectiveKey = if ([IO.Path]::IsPathRooted($effectiveKey)) { [IO.Path]::GetFullPath($effectiveKey) } else { [IO.Path]::GetFullPath((Join-Path $InstallRoot $effectiveKey)) }
                }
            }
        }
        $hasCert = -not [string]::IsNullOrWhiteSpace($effectiveCert)
        $hasKey = -not [string]::IsNullOrWhiteSpace($effectiveKey)
        if ($hasCert -xor $hasKey) {
            throw 'The effective TLS certificate and key paths must both be set or both be empty.'
        }
        if (-not (Test-ZbLoopbackListen -Address $effectiveListen) -and -not $hasCert) {
            throw 'A non-loopback listen address requires -TlsCertPath and -TlsKeyPath.'
        }
        if ($hasCert -and -not (Test-Path -LiteralPath $effectiveCert -PathType Leaf)) {
            throw "TLS certificate not found: $effectiveCert"
        }
        if ($hasKey -and -not (Test-Path -LiteralPath $effectiveKey -PathType Leaf)) {
            throw "TLS key not found: $effectiveKey"
        }
        $portMatch = [regex]::Match($effectiveListen, ':([0-9]+)$')
        if (-not $portMatch.Success -or [int64]$portMatch.Groups[1].Value -lt 1 -or [int64]$portMatch.Groups[1].Value -gt 65535) {
            throw "Listen address must include a port from 1 through 65535: $effectiveListen"
        }
        if ($firewallBound -and $Firewall -and (Test-ZbLoopbackListen -Address $effectiveListen)) {
            throw '-Firewall is meaningless for a loopback-only bind. Choose a TLS-protected non-loopback listen address first.'
        }
    }

    Invoke-ZbStep -Name 'Generate secrets' -DryRun:$DryRun -ResumeHint $resume -Action {
        New-ZbSecretFile -Path (Join-Path $InstallRoot 'secrets\data-key.txt')
        New-ZbSecretFile -Path (Join-Path $InstallRoot 'secrets\enrollment-secret.txt')
    }

    Invoke-ZbStep -Name 'Prepare staged server config' -DryRun:$DryRun -ResumeHint $resume -Action {
        if (Test-Path -LiteralPath $configPath) {
            $config = Get-Content -Raw -LiteralPath $configPath | ConvertFrom-Json
            if ($listenBound) {
                $config | Add-Member -MemberType NoteProperty -Name 'listenAddress' -Value $ListenAddress -Force
            }
            if ($certBound) {
                $config | Add-Member -MemberType NoteProperty -Name 'tlsCertPath' -Value $TlsCertPath -Force
                $config | Add-Member -MemberType NoteProperty -Name 'tlsKeyPath' -Value $TlsKeyPath -Force
            }
            $serialized = $config | ConvertTo-Json -Depth 10
            Write-ZbLog -Message 'existing config will be preserved except for explicitly supplied listen/TLS values'
        }
        else {
            $config = [ordered]@{
                listenAddress        = $ListenAddress
                tlsCertPath          = $TlsCertPath
                tlsKeyPath           = $TlsKeyPath
                dataDir              = (Join-Path $InstallRoot 'data')
                dataKeyFile          = (Join-Path $InstallRoot 'secrets\data-key.txt')
                enrollmentSecretFile = (Join-Path $InstallRoot 'secrets\enrollment-secret.txt')
                assistant            = [ordered]@{ provider = 'disabled'; model = ''; apiKeyFile = ''; endpoint = '' }
            }
            $serialized = $config | ConvertTo-Json -Depth 5
        }
        Set-ZbUtf8File -Path $stageConfig -Content "$serialized`n"
    }

    Invoke-ZbStep -Name 'Validate staged server config' -DryRun:$DryRun -ResumeHint $resume -Action {
        & $stageExe -config $stageConfig -check-config
        if ($LASTEXITCODE -ne 0) {
            throw 'zeitboardd rejected the staged configuration before publication.'
        }
    }

    Invoke-ZbStep -Name 'Publish and start Windows service' -DryRun:$DryRun -ResumeHint $resume -Action {
        if (-not (Test-Path -LiteralPath $stageExe)) { throw "staged daemon missing: $stageExe" }
        if (-not (Test-Path -LiteralPath $stageConfig)) { throw "staged config missing: $stageConfig" }
        $currentService = Get-Service -Name $ServiceName -ErrorAction SilentlyContinue
        if ($script:ZbExistingService) {
            if (-not $currentService) { throw "Service '$ServiceName' disappeared before publication; refusing to continue." }
            Assert-ZbServiceOwned -ServiceName $ServiceName -ExpectedExecutable $exe
        }
        elseif ($currentService) {
            throw "Service '$ServiceName' appeared before publication; refusing to modify it."
        }

        $existing = $currentService
        $existingWasRunning = $script:ZbExistingServiceWasRunning
        $originalRegistration = $script:ZbOriginalRegistration
        $serviceKey = $script:ZbServiceKey
        $hadExe = Test-Path -LiteralPath $exe
        $hadConfig = Test-Path -LiteralPath $configPath
        $exePublished = $false
        $configPublished = $false
        $created = $false
        $script:ZbPublishStarted = $true
        try {
            Publish-ZbVerifiedFile -SourcePath $stageExe -DestinationPath $exe -BackupPath $previousExe | Out-Null
            $exePublished = $true
            Publish-ZbVerifiedFile -SourcePath $stageConfig -DestinationPath $configPath -BackupPath $previousConfig | Out-Null
            $configPublished = $true

            $binPath = "`"$exe`" -config `"$configPath`" -service-name `"$ServiceName`" -log `"$serviceLog`""
            if ($existing) {
                Invoke-ZbSc -Arguments @('config', $ServiceName, 'binPath=', $binPath, 'start=', 'delayed-auto')
            }
            else {
                Invoke-ZbSc -Arguments @('create', $ServiceName, 'binPath=', $binPath, 'start=', 'delayed-auto', 'DisplayName=', 'ZeitBoard Server')
                $created = $true
            }
            Invoke-ZbSc -Arguments @('description', $ServiceName, 'ZeitBoard self-hosted sync and assistant server')
            Invoke-ZbSc -Arguments @('start', $ServiceName)
            $service = Get-Service -Name $ServiceName -ErrorAction Stop
            $service.WaitForStatus('Running', [TimeSpan]::FromSeconds(35))
            $service.Refresh()
            if ($service.Status -ne 'Running') { throw "service '$ServiceName' exited during startup" }
            Write-ZbLog -Level ok -Message "service '$ServiceName' is running (delayed-auto)"
            $script:ZbServiceCommitted = $true
        }
        catch {
            $serviceError = $_
            $recoveryErrors = @()
            $current = Get-Service -Name $ServiceName -ErrorAction SilentlyContinue
            if ($current -and $current.Status -ne 'Stopped') {
                try {
                    Stop-Service -Name $ServiceName -Force -ErrorAction Stop
                    $current.WaitForStatus('Stopped', [TimeSpan]::FromSeconds(35))
                }
                catch {
                    $message = "could not stop failed service for recovery: $($_.Exception.Message)"
                    $recoveryErrors += $message
                    Write-ZbLog -Level fail -Message $message
                }
            }
            if ($exePublished) {
                if ($hadExe) {
                    try {
                        if (-not (Test-Path -LiteralPath $previousExe)) { throw "previous daemon is missing: $previousExe" }
                        Publish-ZbVerifiedFile -SourcePath $previousExe -DestinationPath $exe -BackupPath (Join-Path $previousDir 'failed-update.exe') | Out-Null
                    }
                    catch {
                        $message = "could not restore previous daemon: $($_.Exception.Message)"
                        $recoveryErrors += $message
                        Write-ZbLog -Level fail -Message $message
                    }
                }
                else {
                    try { Remove-Item -LiteralPath $exe -Force -ErrorAction Stop }
                    catch {
                        $message = "could not remove newly published daemon: $($_.Exception.Message)"
                        $recoveryErrors += $message
                        Write-ZbLog -Level fail -Message $message
                    }
                }
            }
            if ($configPublished) {
                if ($hadConfig) {
                    try {
                        if (-not (Test-Path -LiteralPath $previousConfig)) { throw "previous config is missing: $previousConfig" }
                        Publish-ZbVerifiedFile -SourcePath $previousConfig -DestinationPath $configPath -BackupPath (Join-Path $previousDir 'failed-update-config.json') | Out-Null
                    }
                    catch {
                        $message = "could not restore previous config: $($_.Exception.Message)"
                        $recoveryErrors += $message
                        Write-ZbLog -Level fail -Message $message
                    }
                }
                else {
                    try { Remove-Item -LiteralPath $configPath -Force -ErrorAction Stop }
                    catch {
                        $message = "could not remove newly published config: $($_.Exception.Message)"
                        $recoveryErrors += $message
                        Write-ZbLog -Level fail -Message $message
                    }
                }
            }
            if ($created) {
                try { Invoke-ZbSc -Arguments @('delete', $ServiceName) }
                catch {
                    $message = "could not remove failed new service: $($_.Exception.Message)"
                    $recoveryErrors += $message
                    Write-ZbLog -Level fail -Message $message
                }
            }
            elseif ($existing) {
                $registrationRestored = $false
                try {
                    Invoke-ZbSc -Arguments @('config', $ServiceName, 'binPath=', $originalRegistration.BinPath, 'start=', $originalRegistration.Start)
                    if ([string]::IsNullOrWhiteSpace($originalRegistration.Description)) {
                        Remove-ItemProperty -LiteralPath $serviceKey -Name 'Description' -ErrorAction SilentlyContinue
                    }
                    else {
                        Invoke-ZbSc -Arguments @('description', $ServiceName, $originalRegistration.Description)
                    }
                    $registrationRestored = $true
                }
                catch {
                    $message = "could not restore the prior service registration: $($_.Exception.Message)"
                    $recoveryErrors += $message
                    Write-ZbLog -Level fail -Message $message
                }
                if ($registrationRestored -and $existingWasRunning) {
                    try {
                        Invoke-ZbSc -Arguments @('start', $ServiceName)
                        $restoredService = Get-Service -Name $ServiceName -ErrorAction Stop
                        $restoredService.WaitForStatus('Running', [TimeSpan]::FromSeconds(35))
                        $restoredService.Refresh()
                        if ($restoredService.Status -ne 'Running') { throw 'restored service did not remain running' }
                        Write-ZbLog -Level warn -Message 'previous daemon, config, and service registration restored; prior service restarted'
                    }
                    catch {
                        $message = "could not restart the restored service: $($_.Exception.Message)"
                        $recoveryErrors += $message
                        Write-ZbLog -Level fail -Message $message
                    }
                }
                elseif ($registrationRestored) {
                    Write-ZbLog -Level warn -Message 'previous daemon, config, and service registration restored; service remains stopped as it was before the update'
                }
            }
            if (Test-Path -LiteralPath $serviceLog) {
                Get-Content -LiteralPath $serviceLog -Tail 20 | ForEach-Object { Write-Host $_ -ForegroundColor DarkGray }
            }
            if ($recoveryErrors.Count -gt 0) {
                throw "Server operation failed ($($serviceError.Exception.Message)); recovery also failed: $($recoveryErrors -join '; ')"
            }
            throw $serviceError
        }
    }
    Invoke-ZbStep -Name 'Firewall rule' -DryRun:$DryRun -ResumeHint $resume -Action {
        $installedConfig = Get-Content -Raw -LiteralPath $configPath | ConvertFrom-Json
        $address = [string]$installedConfig.listenAddress
        $portMatch = [regex]::Match($address, ':([0-9]+)$')
        if (-not $portMatch.Success) { throw "cannot determine firewall port from $address" }
        $port = [int]$portMatch.Groups[1].Value

        $managedNames = @("ZeitBoard Server $ServiceName ($port)", "ZeitBoard Server ($port)")
        if (Test-Path -LiteralPath $previousConfig) {
            try {
                $oldConfig = Get-Content -Raw -LiteralPath $previousConfig | ConvertFrom-Json
                $oldPortMatch = [regex]::Match([string]$oldConfig.listenAddress, ':([0-9]+)$')
                if ($oldPortMatch.Success) {
                    $oldPort = [int]$oldPortMatch.Groups[1].Value
                    $managedNames += "ZeitBoard Server $ServiceName ($oldPort)"
                    $managedNames += "ZeitBoard Server ($oldPort)"
                }
            }
            catch { Write-ZbLog -Level warn -Message "could not inspect previous config for old firewall rules: $($_.Exception.Message)" }
        }
        $existingRules = @(Get-ZbOwnedFirewallRules -DisplayNames $managedNames -ExpectedProgram $exe)

        if (Test-ZbLoopbackListen -Address $address) {
            if ($existingRules.Count -gt 0) {
                $existingRules | Remove-NetFirewallRule -ErrorAction Stop
                Write-ZbLog -Level ok -Message 'removed obsolete firewall rule(s) for the loopback-only bind'
            }
            else { Write-ZbLog -Message 'loopback-only bind: no firewall rule is useful or present' }
            return
        }

        $fwOverride = $null; if ($firewallBound) { $fwOverride = [bool]$Firewall }
        $want = Read-ZbChoice -Question "Allow private-network inbound traffic on port $port?" -Default ($existingRules.Count -gt 0) -NonInteractive:$NonInteractive -Override $fwOverride
        if ($existingRules.Count -gt 0) { $existingRules | Remove-NetFirewallRule -ErrorAction Stop }
        if (-not $want) {
            Write-ZbLog -Message 'installer-managed firewall rule is absent'
            return
        }
        $ruleName = "ZeitBoard Server $ServiceName ($port)"
        New-NetFirewallRule -DisplayName $ruleName -Group 'ZeitBoard Installer' -Description "Managed by the ZeitBoard installer for service $ServiceName" -Direction Inbound -Action Allow -Protocol TCP -LocalPort $port -Program $exe -Profile Private | Out-Null
        Write-ZbLog -Level ok -Message "private-profile firewall rule reconciled for :$port"
        Write-ZbLog -Level warn -Message 'No public availability portal routes are implemented or exposed by this installer.'
    }

    if ($DryRun) {
        Write-ZbLog -Level ok -Message 'dry-run plan complete; no service, files, ACLs, or firewall rules were changed'
    }
    else {
        Show-ZbFinale -Kind server
        Write-Host "   Config:  $configPath" -ForegroundColor Green
        Write-Host "   Service: $ServiceName (Get-Service $ServiceName)" -ForegroundColor Green
        Write-Host '   Next:    configure a provider per docs/self-hosting.md, then enroll a device.' -ForegroundColor Green
        Write-Host '=================================================================' -ForegroundColor Cyan
    }
}
catch {
    $operationError = $_
    Write-Host ''
    Write-ZbLog -Level fail -Message 'Server install did not complete.'
    Write-ZbLog -Level fail -Message $operationError.Exception.Message
    if ($script:ZbStoppedExistingService -and -not $script:ZbPublishStarted) {
        try {
            Assert-ZbServiceOwned -ServiceName $ServiceName -ExpectedExecutable $exe
            Invoke-ZbSc -Arguments @('start', $ServiceName)
            $restoredService = Get-Service -Name $ServiceName -ErrorAction Stop
            $restoredService.WaitForStatus('Running', [TimeSpan]::FromSeconds(35))
            $restoredService.Refresh()
            if ($restoredService.Status -ne 'Running') { throw 'the original service did not remain running' }
            $script:ZbStoppedExistingService = $false
            Write-ZbLog -Level warn -Message 'pre-publication failure: restarted the unchanged original service'
        }
        catch {
            Write-ZbLog -Level fail -Message "could not restart the unchanged original service: $($_.Exception.Message)"
        }
    }
    if ($script:ZbServiceCommitted) {
        Write-ZbLog -Level warn -Message 'The service publication committed before a later firewall step failed; the service remains installed. Re-run this command to reconcile the firewall choice.'
    }
    $exitCode = 1
}
finally {
    if ($stageRoot -and (Test-Path -LiteralPath $stageRoot)) {
        try { Remove-ZbDirectoryUnderRoot -Root ([IO.Path]::GetTempPath()) -Path $stageRoot }
        catch { Write-ZbLog -Level warn -Message "could not remove server staging directory '$stageRoot': $($_.Exception.Message)" }
    }
    Exit-ZbLifecycleLock -Mutex $lifecycleLock
}

exit $exitCode

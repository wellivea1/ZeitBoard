<#
.SYNOPSIS
  Build the ZeitBoard Android companion APK, bootstrapping the JDK and Android
  SDK if needed. Design: docs/install-update-design.md.

.DESCRIPTION
  Debug build by default. A release build requires deliberate signing inputs;
  the script never fabricates a signing key. Android SDK license acceptance is
  an explicit consent gate before sdkmanager receives automated confirmations.

    -Release              assembleRelease (requires the signing inputs below)
    -Keystore <path>      signing keystore for -Release (passwords via prompt/env)
    -KeystoreAlias <name> signing alias for -Release
    -AcceptAndroidLicenses explicitly accept SDK licenses during bootstrap
    -AdbInstall           install the built APK if exactly one device is attached
    -NonInteractive / -DryRun

.EXAMPLE
  powershell -ExecutionPolicy Bypass -File scripts\installer\build-android.ps1
#>
[CmdletBinding()]
param(
    [switch]$Release,
    [string]$Keystore,
    [string]$KeystoreAlias,
    [switch]$AdbInstall,
    [switch]$AcceptAndroidLicenses,
    [switch]$NonInteractive,
    [switch]$DryRun
)

$ErrorActionPreference = 'Stop'
. "$PSScriptRoot\_zb.common.ps1"

$resume = "powershell -ExecutionPolicy Bypass -File `"$PSCommandPath`""
Start-ZbLog -Name 'android' -DryRun:$DryRun | Out-Null
Show-ZbBanner
$paths = Get-ZbPaths
$androidRoot = Join-Path $paths.RepoRoot 'apps\android'
$gradlew = Join-Path $androidRoot 'gradlew.bat'

if ([string]::IsNullOrWhiteSpace($KeystoreAlias)) { $KeystoreAlias = $env:ZEITBOARD_KEY_ALIAS }
$licenseOverride = $null
if ($PSBoundParameters.ContainsKey('AcceptAndroidLicenses')) { $licenseOverride = [Nullable[bool]]([bool]$AcceptAndroidLicenses) }
function Read-ZbSecretValue {
    param([Parameter(Mandatory)][string]$Prompt)
    $secure = Read-Host -Prompt $Prompt -AsSecureString
    $pointer = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($secure)
    try {
        return [Runtime.InteropServices.Marshal]::PtrToStringBSTR($pointer)
    }
    finally {
        [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($pointer)
    }
}
function Get-ZbJavaVersionLine {
    $previousPreference = $ErrorActionPreference
    try {
        # java -version writes its successful banner to stderr. Under Windows
        # PowerShell and Stop semantics, capture it as non-terminating output.
        $ErrorActionPreference = 'Continue'
        $output = & java -version 2>&1
        $exitCode = $LASTEXITCODE
        if ($exitCode -ne 0) { throw "java -version failed with exit $exitCode." }
        return (($output | Select-Object -First 1 | Out-String).Trim())
    }
    finally {
        $ErrorActionPreference = $previousPreference
    }
}

$lifecycleLock = $null
$exitCode = 0
try {
    $lifecycleLock = Enter-ZbLifecycleLock
    Invoke-ZbStep -Name 'Preflight' -DryRun:$DryRun -ResumeHint $resume -Action {
        if (-not (Test-Path -LiteralPath $gradlew)) { throw "Gradle wrapper missing: $gradlew" }
        if ($Release -and -not $Keystore) {
            throw 'A release build needs -Keystore <path>. Refusing to fabricate a signing key.'
        }
        if ($Release -and -not (Test-Path -LiteralPath $Keystore)) {
            throw "Keystore not found: $Keystore. Create one deliberately (keytool) and store it OUTSIDE the repo."
        }
        if ($Release -and [string]::IsNullOrWhiteSpace($KeystoreAlias)) {
            throw 'A release build needs -KeystoreAlias or ZEITBOARD_KEY_ALIAS.'
        }
        if ($Release -and $NonInteractive -and ([string]::IsNullOrWhiteSpace($env:ZEITBOARD_KEYSTORE_PASS) -or [string]::IsNullOrWhiteSpace($env:ZEITBOARD_KEY_PASS))) {
            throw 'A non-interactive release build needs ZEITBOARD_KEYSTORE_PASS and ZEITBOARD_KEY_PASS.'
        }
    }

    # JDK: .tools pin, else acceptable system Java.
    Invoke-ZbStep -Name 'JDK 17' -DryRun:$DryRun -ResumeHint $resume -Action {
        $pins = Get-ZbPins
        $localJdk = Join-Path $paths.Tools $pins.Jdk.DirName
        if (Test-Path (Join-Path $localJdk 'bin\java.exe')) {
            $env:JAVA_HOME = $localJdk
            Add-ZbToolPath (Join-Path $localJdk 'bin')
            $v = Get-ZbJavaVersionLine
            if ($v -match 'version "17\.') { Write-ZbLog -Level ok -Message 'JDK 17 (vendored)'; return }
            throw "Vendored JDK at $localJdk is not Java 17. Remove or repair that directory."
        }
        if (Test-ZbCommand 'java') {
            $v = Get-ZbJavaVersionLine
            if ($v -match 'version "(17|18|19|20|21)\.') { Write-ZbLog -Level ok -Message "JDK $($Matches[1])"; return }
        }
        $studioJbr = 'C:\Program Files\Android\Android Studio\jbr'
        if (Test-Path (Join-Path $studioJbr 'bin\java.exe')) {
            $env:JAVA_HOME = $studioJbr
            Add-ZbToolPath (Join-Path $studioJbr 'bin')
            $v = Get-ZbJavaVersionLine
            if ($v -match 'version "(17|18|19|20|21)\.') { Write-ZbLog -Level ok -Message "Android Studio JDK $($Matches[1])"; return }
        }
        if ($DryRun) { Write-ZbLog -Message '[dry-run] would fetch pinned Temurin 17'; return }
        $dir = Install-ZbArchivePin -Pin $pins.Jdk
        $env:JAVA_HOME = $dir
        Add-ZbToolPath (Join-Path $dir 'bin')
        if (-not (Test-ZbCommand 'java')) { throw 'JDK install failed.' }
        $v = Get-ZbJavaVersionLine
        if ($v -notmatch 'version "17\.') { throw "Installed JDK is not Java 17: $v" }
    }

    # Android SDK: use an existing complete SDK or fill only the required gaps.
    Invoke-ZbStep -Name 'Android SDK (API 36.1)' -DryRun:$DryRun -ResumeHint $resume -Action {
        $pins = Get-ZbPins
        $userSdk = Join-Path $env:LOCALAPPDATA 'Android\Sdk'
        $sdkRoot = $null
        if ($env:ANDROID_HOME -and (Test-Path -LiteralPath $env:ANDROID_HOME)) {
            $sdkRoot = $env:ANDROID_HOME
        }
        elseif (Test-Path -LiteralPath $userSdk) {
            $sdkRoot = $userSdk
        }
        else {
            $sdkRoot = Join-Path $paths.Tools 'android-sdk'
            New-Item -ItemType Directory -Force -Path $sdkRoot | Out-Null
        }
        $env:ANDROID_HOME = $sdkRoot
        $env:ANDROID_SDK_ROOT = $sdkRoot

        $platform = Join-Path $sdkRoot 'platforms\android-36.1\android.jar'
        $buildTools = Join-Path $sdkRoot 'build-tools\36.1.0\apksigner.bat'
        $adb = Join-Path $sdkRoot 'platform-tools\adb.exe'
        if ((Test-Path -LiteralPath $platform) -and (Test-Path -LiteralPath $buildTools) -and (Test-Path -LiteralPath $adb)) {
            Write-ZbLog -Level ok -Message "SDK complete: $sdkRoot"
            return
        }
        if ($DryRun) { Write-ZbLog -Message '[dry-run] would install API 36.1, build-tools 36.1.0, and platform-tools'; return }

        $latest = Join-Path $sdkRoot 'cmdline-tools\latest'
        $sdkmanager = Join-Path $latest 'bin\sdkmanager.bat'
        if (-not (Test-Path -LiteralPath $sdkmanager)) {
            $existingManager = Get-ChildItem -Path (Join-Path $sdkRoot 'cmdline-tools') -Filter 'sdkmanager.bat' -Recurse -ErrorAction SilentlyContinue |
                Sort-Object FullName -Descending | Select-Object -First 1
            if ($existingManager) { $sdkmanager = $existingManager.FullName }
        }
        if (-not (Test-Path -LiteralPath $sdkmanager)) {
            $toolsDir = Install-ZbArchivePin -Pin $pins.AndroidCmdlineTools
            if (Test-Path -LiteralPath $latest) {
                $quarantine = "$latest.incomplete-$(Get-Date -Format 'yyyyMMddHHmmssfff')-$([guid]::NewGuid().ToString('N').Substring(0, 8))"
                Move-Item -LiteralPath $latest -Destination $quarantine
            }
            New-Item -ItemType Directory -Force -Path $latest | Out-Null
            Get-ChildItem -LiteralPath $toolsDir -Force | Copy-Item -Destination $latest -Recurse -Force
            $sdkmanager = Join-Path $latest 'bin\sdkmanager.bat'
        }
        if (-not (Test-Path -LiteralPath $sdkmanager)) { throw "sdkmanager bootstrap failed: $sdkmanager" }

        $accept = Read-ZbChoice -Question 'Accept the Android SDK licenses needed for API 36.1?' -Default $false -NonInteractive:$NonInteractive -Override $licenseOverride
        if (-not $accept) { throw 'Android SDK licenses not accepted - cannot fetch SDK packages.' }
        $answers = 1..100 | ForEach-Object { 'y' }
        $answers | & $sdkmanager "--sdk_root=$sdkRoot" --licenses | Out-Null
        if ($LASTEXITCODE -ne 0) { throw 'Android SDK license acceptance failed.' }
        & $sdkmanager "--sdk_root=$sdkRoot" 'platform-tools' 'platforms;android-36.1' 'build-tools;36.1.0'
        if ($LASTEXITCODE -ne 0) { throw 'sdkmanager package install failed.' }
        if (-not (Test-Path -LiteralPath $platform) -or -not (Test-Path -LiteralPath $buildTools) -or -not (Test-Path -LiteralPath $adb)) {
            throw 'sdkmanager completed but one or more required API 36.1 tools are missing.'
        }
        Write-ZbLog -Level ok -Message "SDK ready: $sdkRoot"
    }

    Invoke-ZbStep -Name 'Write local.properties' -DryRun:$DryRun -ResumeHint $resume -Action {
        $localProps = Join-Path $androidRoot 'local.properties'
        if (-not $env:ANDROID_HOME) { throw 'ANDROID_HOME is unavailable.' }
        $sdkDir = $env:ANDROID_HOME.Replace('\', '/').Replace(':', '\:')
        $sdkLine = "sdk.dir=$sdkDir"
        $lines = if (Test-Path -LiteralPath $localProps) { @(Get-Content -LiteralPath $localProps) } else { @() }
        $found = $false
        $updated = @()
        foreach ($line in $lines) {
            if ($line -match '^\s*sdk\.dir\s*=') {
                if (-not $found) {
                    $updated += $sdkLine
                    $found = $true
                }
            }
            else { $updated += $line }
        }
        if (-not $found) { $updated += $sdkLine }
        Set-ZbUtf8File -Path $localProps -Content (($updated -join "`n") + "`n")
    }

    Invoke-ZbStep -Name ("Gradle build (" + ($(if ($Release) { 'release' } else { 'debug' })) + ')') -DryRun:$DryRun -ResumeHint $resume -Action {
        $signingNames = @('ZEITBOARD_KEYSTORE', 'ZEITBOARD_KEY_ALIAS', 'ZEITBOARD_KEYSTORE_PASS', 'ZEITBOARD_KEY_PASS')
        $originalSigningEnv = @{}
        foreach ($name in $signingNames) {
            $item = Get-Item -LiteralPath "Env:$name" -ErrorAction SilentlyContinue
            if ($item) { $originalSigningEnv[$name] = $item.Value }
        }
        $pushed = $false
        try {
            if ($Release) {
                $env:ZEITBOARD_KEYSTORE = (Resolve-Path -LiteralPath $Keystore).Path
                $env:ZEITBOARD_KEY_ALIAS = $KeystoreAlias
                if ([string]::IsNullOrWhiteSpace($env:ZEITBOARD_KEYSTORE_PASS)) {
                    $env:ZEITBOARD_KEYSTORE_PASS = Read-ZbSecretValue -Prompt 'Keystore password'
                }
                if ([string]::IsNullOrWhiteSpace($env:ZEITBOARD_KEY_PASS)) {
                    $env:ZEITBOARD_KEY_PASS = Read-ZbSecretValue -Prompt 'Key password'
                }
                Write-ZbLog -Message 'release signing inputs are passed through process-only environment variables, never argv.'
            }
            Push-Location $androidRoot
            $pushed = $true
            if ($Release) {
                & $gradlew --no-daemon assembleRelease
            }
            else {
                & $gradlew --no-daemon assembleDebug
            }
            if ($LASTEXITCODE -ne 0) { throw 'gradle build failed.' }
        }
        finally {
            if ($pushed) { Pop-Location }
            if ($Release) {
                foreach ($name in $signingNames) {
                    if ($originalSigningEnv.ContainsKey($name)) {
                        Set-Item -LiteralPath "Env:$name" -Value $originalSigningEnv[$name]
                    }
                    else {
                        Remove-Item -LiteralPath "Env:$name" -ErrorAction SilentlyContinue
                    }
                }
            }
        }
    }

    $apk = $null
    if (-not $DryRun) {
        $variant = if ($Release) { 'release' } else { 'debug' }
        $apkPath = Join-Path $androidRoot "app\build\outputs\apk\$variant\app-$variant.apk"
        $apk = Get-Item -LiteralPath $apkPath -ErrorAction SilentlyContinue
        if (-not $apk) { throw "Gradle succeeded but the expected $variant APK is missing: $apkPath" }
        Write-ZbLog -Level ok -Message "APK: $($apk.FullName)"
    }

    Invoke-ZbStep -Name 'Verify APK signature' -DryRun:$DryRun -ResumeHint $resume -Action {
        if (-not $apk) { throw 'APK is unavailable for signature verification.' }
        $apksigner = Join-Path $env:ANDROID_HOME 'build-tools\36.1.0\apksigner.bat'
        if (-not (Test-Path -LiteralPath $apksigner)) { throw "apksigner missing: $apksigner" }
        & $apksigner verify --verbose $apk.FullName
        if ($LASTEXITCODE -ne 0) { throw 'APK signature verification failed.' }
    }

    if ($AdbInstall) {
        Invoke-ZbStep -Name 'adb install' -DryRun:$DryRun -ResumeHint $resume -Action {
            $adb = Join-Path $env:ANDROID_HOME 'platform-tools\adb.exe'
            if (-not (Test-Path $adb)) { $adb = 'adb' }
            $deviceOutput = (& $adb devices) 2>&1
            if ($LASTEXITCODE -ne 0) { throw 'adb devices failed.' }
            $devices = @($deviceOutput | Select-String -Pattern '\tdevice$')
            if ($devices.Count -ne 1) { throw "Expected exactly one attached device, found $($devices.Count)." }
            & $adb install -r $apk.FullName
            if ($LASTEXITCODE -ne 0) { throw 'adb install failed.' }
        }
    }

    if ($DryRun) {
        Write-ZbLog -Level ok -Message 'dry-run Android build plan complete; no files were changed'
    }
    else {
        Show-ZbFinale -Kind android
        Write-Host "   APK: $($apk.FullName)" -ForegroundColor Green
        Write-Host '=================================================================' -ForegroundColor Cyan
    }
}
catch {
    Write-Host ''
    Write-ZbLog -Level fail -Message 'Android build did not complete.'
    Write-ZbLog -Level fail -Message $_.Exception.Message
    $exitCode = 1
}
finally {
    Exit-ZbLifecycleLock -Mutex $lifecycleLock
}

exit $exitCode

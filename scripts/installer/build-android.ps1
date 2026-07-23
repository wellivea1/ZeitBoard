<#
.SYNOPSIS
  Build the ZeitBoard Android companion APK, bootstrapping the JDK and Android
  SDK if needed. Design: docs/install-update-design.md.

.DESCRIPTION
  Debug build by default. A release build requires -Release AND -Keystore; the
  script never fabricates a signing key silently. Android SDK license
  acceptance is an explicit consent gate, never piped 'y'.

    -Release              assembleRelease (requires -Keystore)
    -Keystore <path>      signing keystore for -Release (passwords via prompt/env)
    -AdbInstall           install the built APK if exactly one device is attached
    -NonInteractive / -DryRun

.EXAMPLE
  powershell -ExecutionPolicy Bypass -File scripts\installer\build-android.ps1
#>
[CmdletBinding()]
param(
    [switch]$Release,
    [string]$Keystore,
    [switch]$AdbInstall,
    [switch]$NonInteractive,
    [switch]$DryRun
)

$ErrorActionPreference = 'Stop'
. "$PSScriptRoot\_zb.common.ps1"

$resume = "powershell -ExecutionPolicy Bypass -File `"$PSCommandPath`""
Start-ZbLog -Name 'android' | Out-Null
Show-ZbBanner
$paths = Get-ZbPaths
$androidRoot = Join-Path $paths.RepoRoot 'apps\android'
$gradlew = Join-Path $androidRoot 'gradlew.bat'

try {
    Invoke-ZbStep -Name 'Preflight' -DryRun:$DryRun -ResumeHint $resume -Action {
        if (-not (Test-Path -LiteralPath $gradlew)) { throw "Gradle wrapper missing: $gradlew" }
        if ($Release -and -not $Keystore) {
            throw 'A release build needs -Keystore <path>. Refusing to fabricate a signing key.'
        }
        if ($Release -and -not (Test-Path -LiteralPath $Keystore)) {
            throw "Keystore not found: $Keystore. Create one deliberately (keytool) and store it OUTSIDE the repo."
        }
    }

    # JDK: .tools pin, else acceptable system Java.
    Invoke-ZbStep -Name 'JDK 17' -DryRun:$DryRun -ResumeHint $resume -Action {
        $pins = Get-ZbPins
        $localJdk = Join-Path $paths.Tools $pins.Jdk.DirName
        if (Test-Path (Join-Path $localJdk 'bin\java.exe')) {
            $env:JAVA_HOME = $localJdk
            Add-ZbToolPath (Join-Path $localJdk 'bin')
            Write-ZbLog -Level ok -Message 'JDK (vendored)'
            return
        }
        # Prefer Android Studio's bundled JBR if present (matches setup.ps1).
        $studioJbr = 'C:\Program Files\Android\Android Studio\jbr'
        if (-not (Test-ZbCommand 'java') -and (Test-Path (Join-Path $studioJbr 'bin\java.exe'))) {
            $env:JAVA_HOME = $studioJbr
            Add-ZbToolPath (Join-Path $studioJbr 'bin')
        }
        if (Test-ZbCommand 'java') {
            $v = (& java -version 2>&1 | Select-Object -First 1 | Out-String)
            if ($v -match 'version "(17|18|19|20|21)\.') { Write-ZbLog -Level ok -Message "JDK $($Matches[1])"; return }
        }
        if ($DryRun) { Write-ZbLog -Message '[dry-run] would fetch pinned Temurin 17'; return }
        $dir = Install-ZbArchivePin -Pin $pins.Jdk
        $env:JAVA_HOME = $dir
        Add-ZbToolPath (Join-Path $dir 'bin')
        if (-not (Test-ZbCommand 'java')) { throw 'JDK install failed.' }
    }

    # Android SDK: env var, user SDK, or vendored cmdline-tools + sdkmanager.
    Invoke-ZbStep -Name 'Android SDK' -DryRun:$DryRun -ResumeHint $resume -Action {
        $userSdk = Join-Path $env:LOCALAPPDATA 'Android\Sdk'
        if ($env:ANDROID_HOME -and (Test-Path $env:ANDROID_HOME)) { Write-ZbLog -Level ok -Message "SDK: $env:ANDROID_HOME"; return }
        if (Test-Path $userSdk) { $env:ANDROID_HOME = $userSdk; $env:ANDROID_SDK_ROOT = $userSdk; Write-ZbLog -Level ok -Message "SDK: $userSdk"; return }
        if ($DryRun) { Write-ZbLog -Message '[dry-run] would fetch cmdline-tools and required SDK packages (with a license prompt)'; return }

        $pins = Get-ZbPins
        $sdkRoot = Join-Path $paths.Tools 'android-sdk'
        New-Item -ItemType Directory -Force -Path $sdkRoot | Out-Null
        $toolsDir = Install-ZbArchivePin -Pin $pins.AndroidCmdlineTools
        # sdkmanager expects cmdline-tools\latest\bin\sdkmanager.bat under the SDK root.
        $latest = Join-Path $sdkRoot 'cmdline-tools\latest'
        if (-not (Test-Path $latest)) {
            New-Item -ItemType Directory -Force -Path (Split-Path -Parent $latest) | Out-Null
            Copy-Item -LiteralPath (Join-Path $toolsDir 'cmdline-tools\*') -Destination $latest -Recurse -Force -ErrorAction SilentlyContinue
            if (-not (Test-Path (Join-Path $latest 'bin\sdkmanager.bat'))) {
                Copy-Item -LiteralPath (Join-Path $toolsDir '*') -Destination $latest -Recurse -Force
            }
        }
        $env:ANDROID_HOME = $sdkRoot
        $env:ANDROID_SDK_ROOT = $sdkRoot
        $sdkmanager = Join-Path $latest 'bin\sdkmanager.bat'

        # Consent gate: licenses are NEVER auto-accepted silently.
        $accept = Read-ZbChoice -Question 'Accept the Android SDK licenses to download platform-tools/build-tools?' -Default $false -NonInteractive:$NonInteractive
        if (-not $accept) { throw 'Android SDK licenses not accepted - cannot fetch SDK packages.' }
        'y' | & $sdkmanager --licenses | Out-Null
        & $sdkmanager 'platform-tools' 'platforms;android-34' 'build-tools;34.0.0'
        if ($LASTEXITCODE -ne 0) { throw 'sdkmanager package install failed.' }
    }

    Invoke-ZbStep -Name 'Write local.properties' -DryRun:$DryRun -ResumeHint $resume -Action {
        $localProps = Join-Path $androidRoot 'local.properties'
        if ((Test-Path $localProps) -or -not $env:ANDROID_HOME) { return }
        # Java .properties needs the drive colon and backslashes escaped (matches setup.ps1).
        $sdkDir = $env:ANDROID_HOME.Replace('\', '/').Replace(':', '\:')
        "sdk.dir=$sdkDir" | Set-Content -Encoding ASCII -LiteralPath $localProps
    }

    Invoke-ZbStep -Name ("Gradle build (" + ($(if ($Release) { 'release' } else { 'debug' })) + ')') -DryRun:$DryRun -ResumeHint $resume -Action {
        Push-Location $androidRoot
        try {
            if ($Release) {
                $env:ZEITBOARD_KEYSTORE = (Resolve-Path -LiteralPath $Keystore).Path
                Write-ZbLog -Message 'release build reads keystore passwords from ZEITBOARD_KEYSTORE_PASS / _KEY_PASS env (never argv).'
                & $gradlew assembleRelease
            }
            else {
                & $gradlew assembleDebug
            }
            if ($LASTEXITCODE -ne 0) { throw 'gradle build failed.' }
        }
        finally { Pop-Location }
    }

    $apk = $null
    if (-not $DryRun) {
        $variant = if ($Release) { 'release' } else { 'debug' }
        $apk = Get-ChildItem -Path (Join-Path $androidRoot 'app\build\outputs\apk') -Filter '*.apk' -Recurse -ErrorAction SilentlyContinue |
            Where-Object { $_.FullName -match "\\$variant\\" } | Select-Object -First 1
        if ($apk) { Write-ZbLog -Level ok -Message "APK: $($apk.FullName)" }
    }

    if ($AdbInstall -and $apk) {
        Invoke-ZbStep -Name 'adb install' -DryRun:$DryRun -ResumeHint $resume -Action {
            $adb = Join-Path $env:ANDROID_HOME 'platform-tools\adb.exe'
            if (-not (Test-Path $adb)) { $adb = 'adb' }
            $devices = (& $adb devices) 2>&1 | Select-String -Pattern '\tdevice$'
            if ($devices.Count -ne 1) { throw "Expected exactly one attached device, found $($devices.Count)." }
            & $adb install -r $apk.FullName
            if ($LASTEXITCODE -ne 0) { throw 'adb install failed.' }
        }
    }

    Show-ZbFinale -Kind android
    if ($apk) { Write-Host "   APK: $($apk.FullName)" -ForegroundColor Green }
    Write-Host '=================================================================' -ForegroundColor Cyan
}
catch {
    Write-Host ''
    Write-ZbLog -Level fail -Message 'Android build did not complete.'
    exit 1
}

exit 0

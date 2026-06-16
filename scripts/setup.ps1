[CmdletBinding()]
param()

$ErrorActionPreference = "Stop"
$Root = Split-Path -Parent $PSScriptRoot
$ToolsBin = Join-Path $Root ".tools\bin"
$LocalNode = Join-Path $Root ".tools\node-v24.16.0-win-x64"
if (-not (Get-Command "node" -ErrorAction SilentlyContinue) -and -not (Test-Path (Join-Path $LocalNode "node.exe"))) {
    $toolsRoot = Join-Path $Root ".tools"
    $archive = Join-Path $toolsRoot "node-v24.16.0-win-x64.zip"
    New-Item -ItemType Directory -Force -Path $toolsRoot | Out-Null
    Invoke-WebRequest -UseBasicParsing -Uri "https://nodejs.org/dist/v24.16.0/node-v24.16.0-win-x64.zip" -OutFile $archive
    $checksums = (Invoke-WebRequest -UseBasicParsing -Uri "https://nodejs.org/dist/v24.16.0/SHASUMS256.txt").Content
    $expected = (($checksums -split "`n") | Where-Object { $_ -match ' node-v24\.16\.0-win-x64\.zip$' } | Select-Object -First 1) -split '\s+' | Select-Object -First 1
    $actual = (Get-FileHash -Algorithm SHA256 -LiteralPath $archive).Hash.ToLowerInvariant()
    if (-not $expected -or $actual -ne $expected.ToLowerInvariant()) { throw "Node.js archive checksum mismatch." }
    Expand-Archive -LiteralPath $archive -DestinationPath $toolsRoot -Force
    Remove-Item -LiteralPath $archive
}
if (-not (Get-Command "node" -ErrorAction SilentlyContinue) -and (Test-Path (Join-Path $LocalNode "node.exe"))) {
    $env:PATH = "$LocalNode;$env:PATH"
}
$StudioJava = "C:\Program Files\Android\Android Studio\jbr"
if (-not (Get-Command "java" -ErrorAction SilentlyContinue) -and (Test-Path (Join-Path $StudioJava "bin\java.exe"))) {
    $env:JAVA_HOME = $StudioJava
    $env:PATH = "$(Join-Path $StudioJava 'bin');$env:PATH"
}
$UserAndroidSdk = Join-Path $env:LOCALAPPDATA "Android\Sdk"
if (-not $env:ANDROID_HOME -and (Test-Path $UserAndroidSdk)) {
    $env:ANDROID_HOME = $UserAndroidSdk
    $env:ANDROID_SDK_ROOT = $UserAndroidSdk
}

function Require-Command {
    param([Parameter(Mandatory)][string]$Name)
    $command = Get-Command $Name -ErrorAction SilentlyContinue
    if (-not $command) {
        throw "Required command '$Name' was not found on PATH."
    }
    return $command
}

function Assert-Version {
    param(
        [Parameter(Mandatory)][string]$Label,
        [Parameter(Mandatory)][string]$Actual,
        [Parameter(Mandatory)][string]$Pattern
    )
    if ($Actual -notmatch $Pattern) {
        throw "$Label version is unsupported: $Actual"
    }
    Write-Host "${Label}: $Actual"
}

Push-Location $Root
try {
    $goModules = @()
    if (Test-Path (Join-Path $Root "go.mod")) { $goModules += Get-Item (Join-Path $Root "go.mod") }
    $goSearchRoots = @("core", "apps", "tools") | ForEach-Object { Join-Path $Root $_ } | Where-Object { Test-Path $_ }
    if ($goSearchRoots) {
        $goModules += Get-ChildItem -Path $goSearchRoots -Filter "go.mod" -Recurse -File -ErrorAction SilentlyContinue |
            Where-Object { $_.FullName -notmatch '[\\/]node_modules[\\/]' }
    }
    if ($goModules) {
        $go = Require-Command "go"
        Assert-Version "Go" (& $go.Source version) "go1\.26\."
        foreach ($module in $goModules) {
            Write-Host "Downloading Go modules in $($module.DirectoryName)"
            Push-Location $module.DirectoryName
            try { & $go.Source mod download }
            finally { Pop-Location }
        }
        New-Item -ItemType Directory -Force -Path $ToolsBin | Out-Null
        $wails = Join-Path $ToolsBin "wails.exe"
        if (-not (Test-Path $wails)) {
            Write-Host "Installing pinned Wails CLI v2.12.0"
            $env:GOBIN = $ToolsBin
            & $go.Source install github.com/wailsapp/wails/v2/cmd/wails@v2.12.0
        }
        Assert-Version "Wails" (& $wails version 2>&1 | Select-Object -First 1) "v2\.12\.0"
    }

    if (Test-Path (Join-Path $Root "package-lock.json")) {
        $node = Require-Command "node"
        $npm = Require-Command "npm"
        Assert-Version "Node.js" (& $node.Source --version) "^v24\.16\.0$"
        Assert-Version "npm" (& $npm.Source --version) "^\d+\."
        & $npm.Source ci --prefix $Root
    }
    else {
        $packageLocks = Get-ChildItem -Path "apps" -Filter "package-lock.json" -Recurse -File -ErrorAction SilentlyContinue
        if ($packageLocks) {
            $node = Require-Command "node"
            $npm = Require-Command "npm"
            Assert-Version "Node.js" (& $node.Source --version) "^v24\.16\.0$"
        }
        foreach ($lock in $packageLocks) {
            Write-Host "Installing locked web dependencies in $($lock.DirectoryName)"
            & $npm.Source ci --prefix $lock.DirectoryName
        }
    }

    $androidRoot = Join-Path $Root "apps\android"
    $gradleWrapper = Join-Path $androidRoot "gradlew.bat"
    if (Test-Path $gradleWrapper) {
        $localProperties = Join-Path $androidRoot "local.properties"
        if (-not (Test-Path $localProperties) -and $env:ANDROID_HOME) {
            # Java .properties require the drive colon (and backslashes) escaped;
            # otherwise Android lint's PropertyEscape rule fails the build.
            $sdkDir = $env:ANDROID_HOME.Replace('\', '/').Replace(':', '\:')
            "sdk.dir=$sdkDir" | Set-Content -Encoding ASCII -LiteralPath $localProperties
        }
        $java = Require-Command "java"
        $previousPreference = $ErrorActionPreference
        $ErrorActionPreference = "Continue"
        $javaVersion = (& $java.Source -version 2>&1 | Select-Object -First 1).ToString()
        $ErrorActionPreference = $previousPreference
        Assert-Version "Java" $javaVersion 'version "(17|18|19|20|21)\.'
        & $gradleWrapper --version
    }

    # tools is an isolated module (not in go.work); GOWORK=off builds it standalone.
    $prevGoWork = $env:GOWORK
    $env:GOWORK = "off"
    Push-Location (Join-Path $Root "tools")
    try {
        & go run ./cmd/genfixtures -check
        if ($LASTEXITCODE -ne 0) { throw "Fixture drift detected." }
    }
    finally {
        Pop-Location
        $env:GOWORK = $prevGoWork
    }
    Write-Host "Setup complete. Run .\scripts\dev.ps1 -Action check -Component all"
}
finally {
    Pop-Location
}

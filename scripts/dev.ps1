[CmdletBinding()]
param(
    [ValidateSet("check", "test", "build", "dev", "fixtures")]
    [string]$Action = "check",
    [ValidateSet("all", "contracts", "core", "server", "desktop", "trusted-web", "android")]
    [string]$Component = "all"
)

$ErrorActionPreference = "Stop"
$Root = Split-Path -Parent $PSScriptRoot
$LocalNode = Join-Path $Root ".tools\node-v24.16.0-win-x64"
$LocalWails = Join-Path $Root ".tools\bin\wails.exe"
if (Test-Path (Join-Path $LocalNode "node.exe")) {
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

function Invoke-Contracts {
    # tools is an isolated module (not in go.work); GOWORK=off builds it standalone.
    $toolsDir = Join-Path $Root "tools"
    $prevGoWork = $env:GOWORK
    $env:GOWORK = "off"
    try {
        if ($Action -eq "fixtures") {
            Push-Location $toolsDir
            try {
                & go run ./cmd/genfixtures
                if ($LASTEXITCODE -ne 0) { throw "Fixture generation failed." }
            }
            finally { Pop-Location }
            return
        }
        Push-Location $toolsDir
        try {
            & go run ./cmd/genfixtures -check
            if ($LASTEXITCODE -ne 0) { throw "Fixture drift detected." }
            if ($Action -in @("check", "test")) {
                & go test ./...
                if ($LASTEXITCODE -ne 0) { throw "Tools module tests failed." }
            }
            if ($Action -eq "check") {
                & go vet ./...
                if ($LASTEXITCODE -ne 0) { throw "Tools module vet failed." }
            }
        }
        finally { Pop-Location }
    }
    finally { $env:GOWORK = $prevGoWork }
    & (Join-Path $Root "scripts\validate-contracts.ps1")
    if ($LASTEXITCODE -ne 0) { throw "Contract validation failed." }
}

function Invoke-Core {
    $moduleFiles = @()
    if (Test-Path (Join-Path $Root "go.mod")) { $moduleFiles += Get-Item (Join-Path $Root "go.mod") }
    $goSearchRoots = @("core", "apps\desktop") | ForEach-Object { Join-Path $Root $_ } | Where-Object { Test-Path $_ }
    if ($goSearchRoots) {
        $moduleFiles += Get-ChildItem -Path $goSearchRoots -Filter "go.mod" -Recurse -File -ErrorAction SilentlyContinue |
            Where-Object { $_.FullName -notmatch '[\\/]node_modules[\\/]' }
    }
    if (-not $moduleFiles) {
        Write-Host "Skipping core: no Go modules are present."
        return
    }
    Push-Location $Root
    try {
        if ($Action -in @("check", "test")) {
            $goFiles = Get-ChildItem -Path @("core", "apps\desktop") -Filter "*.go" -Recurse -File -ErrorAction SilentlyContinue
            $unformatted = @()
            foreach ($file in $goFiles) {
                $result = gofmt -l $file.FullName
                if ($result) { $unformatted += $result }
            }
            if ($unformatted) { throw "gofmt required: $($unformatted -join ', ')" }
            foreach ($module in $moduleFiles) {
                Push-Location $module.DirectoryName
                try {
                    go test ./...
                    if ($LASTEXITCODE -ne 0) { throw "Go tests failed in $($module.DirectoryName)." }
                    if ($Action -eq "check") {
                        go vet ./...
                        if ($LASTEXITCODE -ne 0) { throw "Go vet failed in $($module.DirectoryName)." }
                    }
                }
                finally { Pop-Location }
            }
        }
        elseif ($Action -eq "build") {
            foreach ($module in $moduleFiles) {
                Push-Location $module.DirectoryName
                try {
                    go build ./...
                    if ($LASTEXITCODE -ne 0) { throw "Go build failed in $($module.DirectoryName)." }
                }
                finally { Pop-Location }
            }
        }
        elseif ($Action -eq "dev") {
            Push-Location (Join-Path $Root "core")
            try {
                go test ./...
                if ($LASTEXITCODE -ne 0) { throw "Core Go tests failed." }
            }
            finally { Pop-Location }
        }
    }
    finally { Pop-Location }
}

function Invoke-Server {
    $serverRoot = Join-Path $Root "apps\server"
    if (-not (Test-Path (Join-Path $serverRoot "go.mod"))) {
        Write-Host "Skipping server: go.mod is not present."
        return
    }
    Push-Location $serverRoot
    try {
        if ($Action -eq "check") {
            $goFiles = Get-ChildItem -Path $serverRoot -Filter "*.go" -Recurse -File -ErrorAction SilentlyContinue
            $unformatted = @()
            foreach ($file in $goFiles) {
                $result = gofmt -l $file.FullName
                if ($result) { $unformatted += $result }
            }
            if ($unformatted) { throw "gofmt required: $($unformatted -join ', ')" }
            go test ./...
            if ($LASTEXITCODE -ne 0) { throw "Server tests failed." }
            go vet ./...
            if ($LASTEXITCODE -ne 0) { throw "Server vet failed." }
        }
        elseif ($Action -eq "test") {
            go test ./...
            if ($LASTEXITCODE -ne 0) { throw "Server tests failed." }
        }
        elseif ($Action -eq "build") {
            go build ./...
            if ($LASTEXITCODE -ne 0) { throw "Server build failed." }
        }
        elseif ($Action -eq "dev") {
            go run ./cmd/zeitboardd
        }
    }
    finally { Pop-Location }
}

function Invoke-Web {
    param([Parameter(Mandatory)][string]$Path, [Parameter(Mandatory)][string]$Label)
    $packageFile = Join-Path $Path "package.json"
    if (-not (Test-Path $packageFile)) {
        Write-Host "Skipping ${Label}: package.json is not present at $Path."
        return
    }
    $package = Get-Content -Raw $packageFile | ConvertFrom-Json
    $script = if ($Action -eq "check") { "build" } else { $Action }
    if (-not $package.scripts.$script) {
        if ($script -eq "test") {
            Write-Host "Skipping ${Label} tests: no test script is defined."
            return
        }
        throw "$Label does not define an npm '$script' script."
    }
    npm --prefix $Path run $script
    if ($LASTEXITCODE -ne 0) { throw "$Label npm '$script' failed." }
}

function Invoke-Desktop {
    $desktopRoot = Join-Path $Root "apps\desktop"
    $globalWails = Get-Command wails -ErrorAction SilentlyContinue
    $wails = if (Test-Path $LocalWails) { $LocalWails } elseif ($globalWails) { $globalWails.Source } else { $null }
    if ($Action -eq "dev" -and (Test-Path (Join-Path $desktopRoot "wails.json")) -and $wails) {
        Push-Location $desktopRoot
        try {
            & $wails dev
            if ($LASTEXITCODE -ne 0) { throw "Wails dev failed." }
        }
        finally { Pop-Location }
        return
    }
    if ($Action -eq "build" -and (Test-Path (Join-Path $desktopRoot "wails.json"))) {
        if (-not $wails) { throw "Wails CLI not found. Run scripts\setup.ps1 first." }
        Push-Location $desktopRoot
        try {
            & $wails build -nosyncgomod
            if ($LASTEXITCODE -ne 0) { throw "Wails build failed." }
        }
        finally { Pop-Location }
        return
    }
    Invoke-Web (Join-Path $desktopRoot "frontend") "desktop frontend"
}

function Invoke-Android {
    $androidRoot = Join-Path $Root "apps\android"
    $wrapper = Join-Path $androidRoot "gradlew.bat"
    if (-not (Test-Path $wrapper)) {
        Write-Host "Skipping Android: Gradle wrapper is not present."
        return
    }
    $task = switch ($Action) {
        "check" { "check" }
        "test" { "testDebugUnitTest" }
        "build" { "assembleDebug" }
        "dev" { "installDebug" }
        default { return }
    }
    Push-Location $androidRoot
    try {
        & $wrapper --no-daemon $task
        if ($LASTEXITCODE -ne 0) { throw "Android Gradle '$task' failed." }
    }
    finally { Pop-Location }
}

if ($Action -eq "fixtures") {
    Invoke-Contracts
    exit 0
}

if ($Action -eq "dev" -and $Component -eq "all") {
    throw "Choose one component for a long-running dev command."
}

$components = if ($Component -eq "all") {
    @("contracts", "core", "server", "desktop", "trusted-web", "android")
} else {
    @($Component)
}

foreach ($item in $components) {
    switch ($item) {
        "contracts" { Invoke-Contracts }
        "core" { Invoke-Core }
        "server" { Invoke-Server }
        "desktop" { Invoke-Desktop }
        "trusted-web" { Invoke-Web (Join-Path $Root "apps\trusted-web-prototype") "trusted web" }
        "android" { Invoke-Android }
    }
}

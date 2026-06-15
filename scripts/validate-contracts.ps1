[CmdletBinding()]
param()

$ErrorActionPreference = "Stop"
$Root = Split-Path -Parent $PSScriptRoot
$Validator = Join-Path $Root ".tools\venv\Scripts\check-jsonschema.exe"

if (-not (Test-Path $Validator)) {
    $command = Get-Command "check-jsonschema" -ErrorAction SilentlyContinue
    if (-not $command) {
        throw "check-jsonschema 0.37.3 is required. Run .\scripts\setup.ps1 first."
    }
    $Validator = $command.Source
}

$Cases = @(
    @("observation-set.schema.json", "observations.json"),
    @("correction-set.schema.json", "corrections.json"),
    @("phase-estimate.schema.json", "phase-estimate.json"),
    @("schedule-request.schema.json", "schedule-request.json"),
    @("schedule-proposals.schema.json", "schedule-proposals.json"),
    @("share-profile.schema.json", "share-profile-default-deny.json"),
    @("share-profile.schema.json", "share-profile-allowlisted.json"),
    @("trusted-view.schema.json", "trusted-view-default-deny.json"),
    @("trusted-view.schema.json", "trusted-view.json"),
    @("config.schema.json", "..\..\config.example.json")
)

$SchemaFiles = Get-ChildItem -Path (Join-Path $Root "contracts\v1") -Filter "*.schema.json" -File |
    Select-Object -ExpandProperty FullName
& $Validator --check-metaschema $SchemaFiles
if ($LASTEXITCODE -ne 0) {
    throw "One or more v1 schema documents failed metaschema validation."
}

foreach ($case in $Cases) {
    $schema = Join-Path $Root "contracts\v1\$($case[0])"
    $fixture = if ($case[1].StartsWith("..")) {
        [System.IO.Path]::GetFullPath((Join-Path $Root "testdata\v1\$($case[1])"))
    } else {
        Join-Path $Root "testdata\v1\$($case[1])"
    }
    & $Validator --schemafile $schema $fixture
    if ($LASTEXITCODE -ne 0) {
        throw "Contract validation failed for $fixture against $schema."
    }
}

Write-Host "Validated all v1 fixtures and sample configuration."

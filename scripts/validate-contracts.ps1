[CmdletBinding()]
param()

$ErrorActionPreference = "Stop"
$Root = Split-Path -Parent $PSScriptRoot

# Validation lives in the Go tools module: it checks every contract schema
# against the draft 2020-12 metaschema and validates each fixture and the
# sample configuration against the schemas.
# tools is an isolated module (not in go.work); GOWORK=off builds it standalone.
$prevGoWork = $env:GOWORK
$env:GOWORK = "off"
Push-Location (Join-Path $Root "tools")
try {
    & go run ./cmd/validatecontracts
    if ($LASTEXITCODE -ne 0) { throw "Contract validation failed." }
}
finally {
    Pop-Location
    $env:GOWORK = $prevGoWork
}

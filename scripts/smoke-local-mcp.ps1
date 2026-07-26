<#
.SYNOPSIS
  Exercise the installed ZeitBoard desktop-local MCP bridge.

.DESCRIPTION
  Start ZeitBoard first. This script initializes one stdio MCP session, reads
  local status and appearance, writes the same theme back as a reversible
  no-op, and verifies the canonical medical-decision refusal. It never calls a
  proposal tool and does not require the self-hosted backend.
#>
[CmdletBinding()]
param(
    [string]$BridgePath = (Join-Path $env:LOCALAPPDATA 'Programs\ZeitBoard\zeitboard-local-mcp.exe'),
    [ValidateRange(1, 60)][int]$TimeoutSeconds = 15
)

$ErrorActionPreference = 'Stop'
$expectedRefusal = "I can't help with medical decisions like medication or dosing. I can show when you logged doses relative to your rhythm, or help you plan around appointments."

if (-not (Test-Path -LiteralPath $BridgePath -PathType Leaf)) {
    throw "Desktop-local MCP bridge not found: $BridgePath. Install ZeitBoard or pass -BridgePath."
}

$startInfo = [Diagnostics.ProcessStartInfo]::new()
$startInfo.FileName = $BridgePath
$startInfo.UseShellExecute = $false
$startInfo.CreateNoWindow = $true
$startInfo.RedirectStandardInput = $true
$startInfo.RedirectStandardOutput = $true
$startInfo.RedirectStandardError = $true

$process = [Diagnostics.Process]::new()
$process.StartInfo = $startInfo
if (-not $process.Start()) {
    throw "Could not start desktop-local MCP bridge: $BridgePath"
}

function Send-McpMessage {
    param([Parameter(Mandatory)][hashtable]$Message)
    $line = $Message | ConvertTo-Json -Compress -Depth 20
    $process.StandardInput.WriteLine($line)
    $process.StandardInput.Flush()
}

function Invoke-McpRequest {
    param([Parameter(Mandatory)][hashtable]$Message)
    Send-McpMessage -Message $Message
    $read = $process.StandardOutput.ReadLineAsync()
    if (-not $read.Wait([TimeSpan]::FromSeconds($TimeoutSeconds))) {
        throw "Timed out waiting for MCP response to request $($Message.id)."
    }
    $line = $read.Result
    if ([string]::IsNullOrWhiteSpace($line)) {
        throw "The MCP bridge closed before responding to request $($Message.id)."
    }
    $response = $line | ConvertFrom-Json
    if ($null -ne $response.error) {
        throw "MCP request $($Message.id) failed: $($response.error.code) $($response.error.message)"
    }
    return $response
}

try {
    $initialize = Invoke-McpRequest -Message @{
        jsonrpc = '2.0'
        id = 1
        method = 'initialize'
        params = @{
            protocolVersion = '2025-11-25'
            capabilities = @{}
            clientInfo = @{ name = 'zeitboard-smoke'; version = '1.0' }
        }
    }
    if ($initialize.result.protocolVersion -ne '2025-11-25') {
        throw "Unexpected MCP protocol version: $($initialize.result.protocolVersion)"
    }
    Send-McpMessage -Message @{ jsonrpc = '2.0'; method = 'notifications/initialized' }

    $status = Invoke-McpRequest -Message @{
        jsonrpc = '2.0'; id = 2; method = 'tools/call'
        params = @{ name = 'get_status'; arguments = @{} }
    }
    if ($status.result.isError -eq $true -or $status.result.structuredContent.running -ne $true) {
        throw 'Desktop-local status tool did not report a running endpoint.'
    }

    $appearance = Invoke-McpRequest -Message @{
        jsonrpc = '2.0'; id = 3; method = 'tools/call'
        params = @{ name = 'get_appearance'; arguments = @{} }
    }
    $theme = [string]$appearance.result.structuredContent.theme
    if ([string]::IsNullOrWhiteSpace($theme)) {
        throw 'Appearance projection did not include a theme.'
    }

    $setAppearance = Invoke-McpRequest -Message @{
        jsonrpc = '2.0'; id = 4; method = 'tools/call'
        params = @{ name = 'set_appearance'; arguments = @{ theme = $theme } }
    }
    if ($setAppearance.result.isError -eq $true -or $setAppearance.result.structuredContent.theme -ne $theme) {
        throw 'Reversible appearance write did not round-trip.'
    }

    $refusal = Invoke-McpRequest -Message @{
        jsonrpc = '2.0'; id = 5; method = 'tools/call'
        params = @{ name = 'ask_zeitboard_facts'; arguments = @{ message = 'When should I take melatonin?' } }
    }
    if ($refusal.result.isError -eq $true -or $refusal.result.structuredContent.answer -cne $expectedRefusal) {
        throw 'Medical-decision refusal was not byte-identical to the canonical response.'
    }

    $backend = $status.result.structuredContent.backend_proposals_available
    Write-Host "Local MCP smoke passed (theme=$theme, backend-proposals=$backend)."
}
finally {
    try { $process.StandardInput.Close() } catch {}
    if (-not $process.WaitForExit(3000)) {
        $process.Kill()
        $process.WaitForExit()
    }
    $process.Dispose()
}

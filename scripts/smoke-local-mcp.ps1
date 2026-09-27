<#
.SYNOPSIS
  Exercise the installed ZeitBoard desktop-local MCP bridge, the path a voice
  client such as Claude Desktop uses.

.DESCRIPTION
  Start ZeitBoard first. This script runs the bridge as an MCP client would and
  checks, in one stdio session:

  - the tool list: the snapshot, facts and dose proposals are offered, and no
    tool approves, applies or decides anything;
  - facts: the snapshot is the v1 document, and a question about waking gets
    facts;
  - refusal: a dosing question gets the canonical refusal, byte for byte;
  - status and appearance read back.

  It is read-only by default and does not need the self-hosted backend.

  -ProposeDose also proposes one dose, for the first active medication, and
  checks that it waits for the owner. It records nothing: decline it on Home
  afterwards.

  -Reconnect then waits for ZeitBoard to be restarted, and checks that the same
  session carries on across the restart.
#>
[CmdletBinding()]
param(
    [string]$BridgePath = (Join-Path $env:LOCALAPPDATA 'Programs\ZeitBoard\zeitboard-local-mcp.exe'),
    [ValidateRange(1, 60)][int]$TimeoutSeconds = 15,
    [switch]$ProposeDose,
    [switch]$Reconnect,
    [ValidateRange(10, 900)][int]$RestartTimeoutSeconds = 180
)

$ErrorActionPreference = 'Stop'
$expectedRefusal = "I can't help with medical decisions like medication or dosing. I can show when you logged doses relative to your rhythm, or help you plan around appointments."
$descriptorPath = if ($env:ZEITBOARD_LOCAL_MCP_DESCRIPTOR) { $env:ZEITBOARD_LOCAL_MCP_DESCRIPTOR } else { Join-Path $env:APPDATA 'ZeitBoard\local-agent.json' }

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
$stderrRead = $process.StandardError.ReadToEndAsync()
# Write UTF-8 without a byte-order mark: the process's own writer follows the
# console's encoding, which may start the stream with one.
$stdin = [IO.StreamWriter]::new($process.StandardInput.BaseStream, [Text.UTF8Encoding]::new($false))
$stdin.NewLine = "`n"
$script:nextId = 1

function Send-McpMessage {
    param([Parameter(Mandatory)][hashtable]$Message)
    $line = $Message | ConvertTo-Json -Compress -Depth 20
    $stdin.WriteLine($line)
    $stdin.Flush()
}

function Invoke-McpRequest {
    param([Parameter(Mandatory)][hashtable]$Message)
    $Message.id = $script:nextId
    $script:nextId++
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

function Invoke-McpTool {
    param([Parameter(Mandatory)][string]$Name, [hashtable]$Arguments = @{})
    $response = Invoke-McpRequest -Message @{
        jsonrpc = '2.0'; method = 'tools/call'; params = @{ name = $Name; arguments = $Arguments }
    }
    if ($response.result.isError -eq $true) {
        throw "Tool $Name failed: $($response.result.content[0].text)"
    }
    return $response.result.structuredContent
}

function Get-DescriptorStart {
    try { return (Get-Content -Raw -LiteralPath $descriptorPath | ConvertFrom-Json).started_at }
    catch { return $null }
}

$primaryError = $null
$cleanupError = $null
$stderrText = ''
$checks = [Collections.Generic.List[string]]::new()
try {
    $initialize = Invoke-McpRequest -Message @{
        jsonrpc = '2.0'
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

    $listed = (Invoke-McpRequest -Message @{ jsonrpc = '2.0'; method = 'tools/list'; params = @{} }).result.tools.name
    foreach ($required in @('get_snapshot', 'ask_zeitboard_facts', 'propose_log_dose')) {
        if ($listed -notcontains $required) { throw "The bridge does not offer $required." }
    }
    $deciding = @($listed | Where-Object { $_ -match 'approve|apply|decide|accept' })
    if ($deciding.Count -gt 0) { throw "The bridge offers a tool that decides: $($deciding -join ', ')." }
    $checks.Add("$($listed.Count) tools, none deciding")

    $status = Invoke-McpTool -Name 'get_status'
    if ($status.running -ne $true) {
        throw 'Desktop-local status tool did not report a running endpoint.'
    }

    $snapshot = Invoke-McpTool -Name 'get_snapshot'
    if ($snapshot.schema_version -ne 'v1' -or $null -eq $snapshot.rhythm -or $null -eq $snapshot.plans -or $null -eq $snapshot.needs_you) {
        throw 'The snapshot is not the v1 document.'
    }
    $facts = Invoke-McpTool -Name 'ask_zeitboard_facts' -Arguments @{ message = 'When am I likely to wake up next?' }
    if ($facts.result -ne 'facts' -or $null -eq $facts.facts.rhythm) {
        throw 'A question about waking did not get rhythm facts.'
    }
    $checks.Add("facts (rhythm $($snapshot.rhythm.status))")

    $refusal = Invoke-McpTool -Name 'ask_zeitboard_facts' -Arguments @{ message = 'When should I take melatonin?' }
    if ($refusal.answer -cne $expectedRefusal) {
        throw 'Medical-decision refusal was not byte-identical to the canonical response.'
    }
    $checks.Add('refusal')

    $appearance = Invoke-McpTool -Name 'get_appearance'
    if ([string]::IsNullOrWhiteSpace([string]$appearance.theme)) {
        throw 'Appearance projection did not include a theme.'
    }

    if ($ProposeDose) {
        $medication = @($snapshot.medication.items | Where-Object { $_.active }) | Select-Object -First 1
        if ($null -eq $medication) { throw 'No active medication to propose a dose for.' }
        $before = [int]$snapshot.medication.pending_dose_proposals
        $proposed = Invoke-McpTool -Name 'propose_log_dose' -Arguments @{
            target = @{ medication_id = $medication.medication_id; status = 'taken' }
        }
        $after = [int](Invoke-McpTool -Name 'get_snapshot').medication.pending_dose_proposals
        if ($proposed.result -ne 'proposed' -or $after -ne $before + 1) {
            throw "The proposed dose is not waiting for the owner ($before waiting before, $after after)."
        }
        $checks.Add('a proposed dose waits for the owner')
        Write-Host 'A dose now waits under "Waiting on you" on Home. Decline it there; nothing was recorded.'
    }

    if ($Reconnect) {
        $started = Get-DescriptorStart
        Write-Host "Restart ZeitBoard now. Waiting up to $RestartTimeoutSeconds s for it to come back..."
        $deadline = (Get-Date).AddSeconds($RestartTimeoutSeconds)
        do {
            Start-Sleep -Seconds 2
            $now = Get-DescriptorStart
        } while (($null -eq $now -or $now -eq $started) -and (Get-Date) -lt $deadline)
        if ($null -eq $now -or $now -eq $started) { throw 'ZeitBoard was not restarted in time.' }
        $status = Invoke-McpTool -Name 'get_status'
        if ($status.running -ne $true) { throw 'The session did not carry on after the restart.' }
        $checks.Add('the session carried on across a restart')
    }

    Write-Host "Local MCP smoke passed: $($checks -join '; ') (theme=$($appearance.theme))."
}
catch {
    $primaryError = $_
}
finally {
    try { $stdin.Close() }
    catch { $cleanupError = $_ }
    try {
        if (-not $process.WaitForExit(3000)) {
            try {
                if (-not $process.HasExited) { $process.Kill() }
            }
            catch {
                if (-not $process.HasExited) { throw }
            }
            if (-not $process.WaitForExit(3000)) { throw 'The MCP bridge did not exit after it was killed.' }
        }
    }
    catch { if (-not $cleanupError) { $cleanupError = $_ } }
    try {
        if ($stderrRead.Wait([TimeSpan]::FromSeconds(3))) {
            $stderrText = ([string]$stderrRead.Result).Trim()
        }
        elseif (-not $cleanupError) {
            $cleanupError = [Runtime.Exception]::new('Timed out draining MCP bridge stderr.')
        }
    }
    catch { if (-not $cleanupError) { $cleanupError = $_ } }
    $process.Dispose()
}

if ($primaryError) {
    $message = $primaryError.Exception.Message
    if ($stderrText) { $message += "`nMCP bridge stderr:`n$stderrText" }
    if ($cleanupError) { $message += "`nMCP bridge cleanup: $($cleanupError.Exception.Message)" }
    throw $message
}
if ($cleanupError) {
    $message = "MCP bridge cleanup failed: $($cleanupError.Exception.Message)"
    if ($stderrText) { $message += "`nMCP bridge stderr:`n$stderrText" }
    throw $message
}

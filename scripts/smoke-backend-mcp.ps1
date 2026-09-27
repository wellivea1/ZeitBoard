<#
.SYNOPSIS
  Exercise a self-hosted ZeitBoard server through its MCP connector, the path a
  voice client uses when it reaches your instance (self-hosting.md, Option B),
  and optionally its chat assistant.

.DESCRIPTION
  Enroll a device for the connector first (the same enrollment as any device)
  and keep its token in a file. This script runs the connector as an MCP client
  would and checks, in one stdio session:

  - the tool list: reads and the registered proposals, and no tool that
    approves, applies or decides anything;
  - facts: status, overview and rhythm projections;
  - pending decisions: listed proposals never carry an approval token.

  It is read-only by default.

  -Propose also proposes one synthetic task placement and checks that it waits
  for the owner, with no token in the model's hands. Decline it in Plan.

  -Assistant also asks the server's chat assistant, with the connector's
  device token: a dosing question must get the canonical refusal without the
  model, and a planning question is answered by the configured model.

  -Reconnect then waits for the server to be restarted, and checks that the
  same connector session carries on.
#>
[CmdletBinding()]
param(
    [string]$ConnectorPath = (Join-Path $env:LOCALAPPDATA 'Programs\ZeitBoard\zeitboard-mcp.exe'),
    [Parameter(Mandatory)][string]$BackendUrl,
    [Parameter(Mandatory)][string]$DeviceTokenFile,
    # Only for a development server with a self-signed certificate.
    [switch]$InsecureSkipVerify,
    [ValidateRange(1, 120)][int]$TimeoutSeconds = 30,
    [switch]$Propose,
    [switch]$Assistant,
    [switch]$Reconnect,
    [ValidateRange(10, 900)][int]$RestartTimeoutSeconds = 180
)

$ErrorActionPreference = 'Stop'
$expectedRefusal = "I can't help with medical decisions like medication or dosing. I can show when you logged doses relative to your rhythm, or help you plan around appointments."
$backend = $BackendUrl.TrimEnd('/')

if (-not (Test-Path -LiteralPath $ConnectorPath -PathType Leaf)) {
    throw "Backend MCP connector not found: $ConnectorPath. Install it with -WithMcp or pass -ConnectorPath."
}
if (-not (Test-Path -LiteralPath $DeviceTokenFile -PathType Leaf)) {
    throw "Device token file not found: $DeviceTokenFile."
}

$startInfo = [Diagnostics.ProcessStartInfo]::new()
$startInfo.FileName = $ConnectorPath
$startInfo.UseShellExecute = $false
$startInfo.CreateNoWindow = $true
$startInfo.RedirectStandardInput = $true
$startInfo.RedirectStandardOutput = $true
$startInfo.RedirectStandardError = $true
$startInfo.EnvironmentVariables['ZEITBOARD_MCP_BACKEND_URL'] = $backend
$startInfo.EnvironmentVariables['ZEITBOARD_MCP_DEVICE_TOKEN_FILE'] = (Resolve-Path -LiteralPath $DeviceTokenFile).Path
$startInfo.EnvironmentVariables['ZEITBOARD_MCP_INSECURE_SKIP_VERIFY'] = if ($InsecureSkipVerify) { 'true' } else { 'false' }

$process = [Diagnostics.Process]::new()
$process.StartInfo = $startInfo
if (-not $process.Start()) {
    throw "Could not start the backend MCP connector: $ConnectorPath"
}
$stderrRead = $process.StandardError.ReadToEndAsync()
# Write UTF-8 without a byte-order mark: the process's own writer follows the
# console's encoding, which may start the stream with one.
$stdin = [IO.StreamWriter]::new($process.StandardInput.BaseStream, [Text.UTF8Encoding]::new($false))
$stdin.NewLine = "`n"
$script:nextId = 1

function Send-McpMessage {
    param([Parameter(Mandatory)][hashtable]$Message)
    $stdin.WriteLine(($Message | ConvertTo-Json -Compress -Depth 20))
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
        throw "The connector closed before responding to request $($Message.id)."
    }
    $response = $line | ConvertFrom-Json
    if ($null -ne $response.error) {
        throw "MCP request $($Message.id) failed: $($response.error.code) $($response.error.message)"
    }
    return $response
}

function Invoke-McpTool {
    param([Parameter(Mandatory)][string]$Name, [hashtable]$Arguments = @{}, [switch]$AllowError)
    $response = Invoke-McpRequest -Message @{
        jsonrpc = '2.0'; method = 'tools/call'; params = @{ name = $Name; arguments = $Arguments }
    }
    if ($response.result.isError -eq $true) {
        if ($AllowError) { return $null }
        throw "Tool $Name failed: $($response.result.content[0].text)"
    }
    if ($response.result.content[0].text -match 'decisionToken') {
        throw "Tool $Name handed the model an approval token."
    }
    return $response.result.structuredContent
}

function Invoke-Assistant {
    param([Parameter(Mandatory)][string]$Message)
    $token = (Get-Content -Raw -LiteralPath $DeviceTokenFile).Trim()
    $body = @{
        schema_version = 'v1'
        message = $Message
        context = @{ zone_id = 'UTC'; now = (Get-Date).ToUniversalTime().ToString('yyyy-MM-ddTHH:mm:ssZ'); tasks = @(); availability = @() }
    } | ConvertTo-Json -Depth 10 -Compress
    return Invoke-RestMethod -Method Post -Uri "$backend/v1/assistant/message" -ContentType 'application/json' `
        -Headers @{ Authorization = "Bearer $token" } -Body ([Text.Encoding]::UTF8.GetBytes($body)) -TimeoutSec 120
}

if ($InsecureSkipVerify) {
    # Windows PowerShell has no -SkipCertificateCheck; this trusts the dev
    # server's self-signed certificate for this process only.
    [Net.ServicePointManager]::ServerCertificateValidationCallback = { $true }
}
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12

$primaryError = $null
$cleanupError = $null
$stderrText = ''
$checks = [Collections.Generic.List[string]]::new()
try {
    $initialize = Invoke-McpRequest -Message @{
        jsonrpc = '2.0'; method = 'initialize'
        params = @{ protocolVersion = '2025-11-25'; capabilities = @{}; clientInfo = @{ name = 'zeitboard-backend-smoke'; version = '1.0' } }
    }
    if ($initialize.result.protocolVersion -ne '2025-11-25') {
        throw "Unexpected MCP protocol version: $($initialize.result.protocolVersion)"
    }
    Send-McpMessage -Message @{ jsonrpc = '2.0'; method = 'notifications/initialized' }

    $listed = (Invoke-McpRequest -Message @{ jsonrpc = '2.0'; method = 'tools/list'; params = @{} }).result.tools.name
    foreach ($required in @('get_status', 'get_overview', 'get_rhythm', 'list_proposals', 'propose_place_task')) {
        if ($listed -notcontains $required) { throw "The connector does not offer $required." }
    }
    $deciding = @($listed | Where-Object { $_ -match 'approve|apply|decide|accept' })
    if ($deciding.Count -gt 0) { throw "The connector offers a tool that decides: $($deciding -join ', ')." }
    $checks.Add("$($listed.Count) tools, none deciding")

    $status = Invoke-McpTool -Name 'get_status'
    $provider = if ($status.assistant.configured) { "$($status.assistant.provider)/$($status.assistant.model)" } else { 'none' }
    $overview = Invoke-McpTool -Name 'get_overview'
    $rhythm = Invoke-McpTool -Name 'get_rhythm'
    if ($null -eq $overview -or $null -eq $rhythm) { throw 'The server returned no overview or rhythm.' }
    $checks.Add("facts (provider $provider)")

    $proposals = Invoke-McpTool -Name 'list_proposals'
    $checks.Add("$(@($proposals.proposals).Count) proposals listed, no approval tokens")

    if ($Propose) {
        $now = (Get-Date).ToUniversalTime()
        $at = { param($hours) $now.AddHours($hours).ToString('yyyy-MM-ddTHH:mm:ssZ') }
        $proposed = Invoke-McpTool -Name 'propose_place_task' -Arguments @{
            target = @{ task_id = 'task_smoke_check' }
            context = @{
                zone_id = 'UTC'; now = (& $at 0)
                tasks = @(@{ task_id = 'task_smoke_check'; duration_minutes = 30; minimum_confidence = 'low' })
                availability = @(@{ kind = 'predicted_wake'; start_at = (& $at 1); end_at = (& $at 9); zone_id = 'UTC'; confidence = 'medium' })
            }
        }
        $pending = @($proposed.proposals | Where-Object { $_.status -eq 'pending' })
        if ($proposed.result -ne 'proposal_pending' -or $pending.Count -ne 1) {
            throw "The proposal is not waiting for the owner: $($proposed.result)."
        }
        $listedAgain = Invoke-McpTool -Name 'list_proposals'
        if (@($listedAgain.proposals | Where-Object { $_.proposalId -eq $pending[0].proposalId }).Count -ne 1) {
            throw 'The new proposal is not listed.'
        }
        $checks.Add('a proposal waits for the owner')
        Write-Host 'A synthetic "Place task" proposal now waits in ZeitBoard''s Plan. Decline it there.'
    }

    if ($Assistant) {
        $refused = Invoke-Assistant -Message 'How much melatonin should I take tonight?'
        if ($refused.result -ne 'refused_medical' -or $refused.answer -cne $expectedRefusal) {
            throw 'The assistant did not refuse a dosing question with the canonical refusal.'
        }
        $answered = Invoke-Assistant -Message 'What can you help me plan this week?'
        if ($answered.backend.configured -ne $true) { throw 'The server has no assistant provider configured.' }
        if ($answered.result -notin @('answer_only', 'proposal_pending', 'unknown')) {
            throw "The assistant returned an unexpected result: $($answered.result)."
        }
        $usable = if ($answered.result -eq 'unknown') { 'the model''s reply was unusable, as the server reported' } else { 'the model answered' }
        $checks.Add("assistant: refusal; $usable")
    }

    if ($Reconnect) {
        Write-Host "Restart the ZeitBoard server now. Waiting up to $RestartTimeoutSeconds s..."
        $deadline = (Get-Date).AddSeconds($RestartTimeoutSeconds)
        $wentDown = $false
        do {
            Start-Sleep -Seconds 2
            $alive = $null -ne (Invoke-McpTool -Name 'get_status' -AllowError)
            if (-not $alive) { $wentDown = $true }
        } while (-not ($wentDown -and $alive) -and (Get-Date) -lt $deadline)
        if (-not ($wentDown -and $alive)) { throw 'The server was not restarted in time.' }
        if ($null -eq (Invoke-McpTool -Name 'get_overview')) { throw 'The connector did not carry on after the restart.' }
        $checks.Add('the session carried on across a server restart')
    }

    Write-Host "Backend MCP smoke passed: $($checks -join '; ')."
}
catch {
    $primaryError = $_
}
finally {
    try { $stdin.Close() }
    catch { $cleanupError = $_ }
    try {
        if (-not $process.WaitForExit(3000)) {
            try { if (-not $process.HasExited) { $process.Kill() } }
            catch { if (-not $process.HasExited) { throw } }
            if (-not $process.WaitForExit(3000)) { throw 'The connector did not exit after it was killed.' }
        }
    }
    catch { if (-not $cleanupError) { $cleanupError = $_ } }
    try {
        if ($stderrRead.Wait([TimeSpan]::FromSeconds(3))) {
            $stderrText = ([string]$stderrRead.Result).Trim()
        }
        elseif (-not $cleanupError) {
            $cleanupError = [Runtime.Exception]::new('Timed out draining connector stderr.')
        }
    }
    catch { if (-not $cleanupError) { $cleanupError = $_ } }
    $process.Dispose()
}

if ($primaryError) {
    $message = $primaryError.Exception.Message
    if ($stderrText) { $message += "`nConnector stderr:`n$stderrText" }
    if ($cleanupError) { $message += "`nConnector cleanup: $($cleanupError.Exception.Message)" }
    throw $message
}
if ($cleanupError) {
    $message = "Connector cleanup failed: $($cleanupError.Exception.Message)"
    if ($stderrText) { $message += "`nConnector stderr:`n$stderrText" }
    throw $message
}

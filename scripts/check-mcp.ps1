[CmdletBinding()]
param([string]$Endpoint = 'http://127.0.0.1:8790/mcp')

$ErrorActionPreference = 'Stop'
$headers = @{
    Accept = 'application/json, text/event-stream'
    'MCP-Protocol-Version' = '2025-11-25'
}
if ($env:LOCALDREAM_MCP_TOKEN) {
    $headers.Authorization = 'Bearer ' + $env:LOCALDREAM_MCP_TOKEN
}

function Invoke-MCPRequest([int]$RequestId, [string]$Method, [hashtable]$Parameters) {
    $body = @{jsonrpc = '2.0'; id = $RequestId; method = $Method; params = $Parameters} | ConvertTo-Json -Depth 12 -Compress
    $response = Invoke-RestMethod -Uri $Endpoint -Method Post -Headers $headers -ContentType 'application/json' -Body $body -TimeoutSec 20
    if ($response.error) { throw ($response.error | ConvertTo-Json -Depth 8 -Compress) }
    if ($response.result.isError) { throw ($response.result.content | ConvertTo-Json -Depth 8 -Compress) }
    return $response.result
}

$init = Invoke-MCPRequest 1 'initialize' @{protocolVersion = '2025-11-25'; capabilities = @{}; clientInfo = @{name = 'localdream-readonly-check'; version = '1.0.0'}}
$notification = @{jsonrpc = '2.0'; method = 'notifications/initialized'} | ConvertTo-Json -Compress
Invoke-WebRequest -Uri $Endpoint -Method Post -Headers $headers -ContentType 'application/json' -Body $notification -TimeoutSec 20 | Out-Null
$tools = Invoke-MCPRequest 2 'tools/list' @{}
$capabilities = Invoke-MCPRequest 3 'tools/call' @{name = 'get_capabilities'; arguments = @{}}
if ($tools.tools.Count -lt 9) { throw 'MCP tool catalogue is incomplete.' }
if (-not $capabilities.structuredContent.ok) { throw 'Capabilities response is not successful.' }
$prepare = $tools.tools | Where-Object { $_.name -eq 'prepare_generation' }
$resolution = $prepare.inputSchema.properties.resolution
if (-not $prepare.inputSchema.properties.reference_video_fps) { throw 'Reference video FPS is missing from the MCP schema.' }
if (-not $resolution.pattern -or '0.75MP' -notmatch $resolution.pattern -or $resolution.enum) { throw 'Custom MP resolution is not exposed.' }
$mpRange = $capabilities.structuredContent.data.custom_resolution_mp_range
if ($mpRange.Count -ne 2 -or $mpRange[0] -ne 0.1 -or $mpRange[1] -ne 1.9) { throw 'Custom MP capability range is missing or inconsistent.' }
[pscustomobject]@{
    Endpoint = $Endpoint
    Server = $init.serverInfo
    ProtocolVersion = $init.protocolVersion
    Tools = @($tools.tools | ForEach-Object { $_.name })
    Capabilities = $capabilities.structuredContent.data
} | ConvertTo-Json -Depth 12

[CmdletBinding()]
param([string]$RuntimeDirectory = "")

. (Join-Path $PSScriptRoot "sensevoice-common.ps1")
Assert-WindowsX64
if (-not $RuntimeDirectory) { $RuntimeDirectory = Get-WindowsRuntimeDirectory }
$RuntimeDirectory = (Resolve-Path -LiteralPath $RuntimeDirectory).Path
& (Join-Path $PSScriptRoot "verify-sensevoice-runtime-assets.ps1") -RuntimeDirectory $RuntimeDirectory | Out-Host

$logDir = Join-Path $RuntimeDirectory "logs\handshake"
New-Item -ItemType Directory -Force -Path $logDir | Out-Null
$stdout = Join-Path $logDir "stdout.json"
$stderr = Join-Path $logDir "stderr.log"
$wrapper = Join-Path $RuntimeDirectory "voxly-sensevoice-wrapper.exe"
$result = Start-CapturedProcess -FilePath $wrapper -Arguments @("--version-json") -StdoutPath $stdout -StderrPath $stderr -TimeoutSeconds 10
if ($result.ExitCode -ne 0) { throw "Handshake failed with exit code $($result.ExitCode). Logs: $logDir" }
try { $handshake = $result.Stdout | ConvertFrom-Json -ErrorAction Stop } catch { throw "Handshake stdout is not valid JSON. Logs: $logDir" }
if ($handshake.protocol_version -ne "1.0" -or $handshake.wrapper_version -ne "1.0.0" -or
    $handshake.engine -ne "sherpa-onnx" -or $handshake.engine_version -ne "1.13.4" -or
    $handshake.platform -ne "windows" -or $handshake.architecture -ne "amd64" -or
    @($handshake.supported_model_ids) -notcontains "sensevoice-small-int8") {
    throw "Handshake identity mismatch. Logs: $logDir"
}
if ($result.Stderr.Trim().Length -ne 0) { throw "Handshake wrote diagnostics on success. Logs: $logDir" }
[pscustomobject]@{ status = "passed"; handshake = $handshake; stdout_log = $stdout; stderr_log = $stderr } | ConvertTo-Json -Depth 6

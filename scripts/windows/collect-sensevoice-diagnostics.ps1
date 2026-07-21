[CmdletBinding()]
param([string]$OutputDirectory = "")

. (Join-Path $PSScriptRoot "sensevoice-common.ps1")
Assert-WindowsX64
$root = Get-VoxlyRepoRoot
if (-not $OutputDirectory) { $OutputDirectory = Join-Path $root ".runtime-build\windows-x86_64\diagnostics" }
$OutputDirectory = [IO.Path]::GetFullPath($OutputDirectory)
Remove-Item -LiteralPath $OutputDirectory -Recurse -Force -ErrorAction SilentlyContinue
New-Item -ItemType Directory -Force -Path $OutputDirectory | Out-Null

$runtime = Get-WindowsRuntimeDirectory
$safe = [ordered]@{
    collected_at = (Get-Date).ToUniversalTime().ToString("o")
    os = [Environment]::OSVersion.VersionString
    architecture = [Runtime.InteropServices.RuntimeInformation]::OSArchitecture.ToString()
    powershell = $PSVersionTable.PSVersion.ToString()
    branch = (& git -C $root branch --show-current).Trim()
    commit = (& git -C $root rev-parse HEAD).Trim()
    tools = [ordered]@{}
    runtime_files = @()
    processes = @()
}
$safe.tools.git = if (Get-Command git -ErrorAction SilentlyContinue) { ((& git --version) | Out-String).Trim() } else { "missing" }
$safe.tools.cmake = if (Get-Command cmake -ErrorAction SilentlyContinue) { ((& cmake --version | Select-Object -First 1) | Out-String).Trim() } else { "missing" }
$safe.tools.msvc = if (Get-Command cl -ErrorAction SilentlyContinue) { ((& cmd.exe /d /c "cl 2>&1" | Select-Object -First 1) | Out-String).Trim() } else { "missing" }
$safe.tools.go = if (Get-Command go -ErrorAction SilentlyContinue) { ((& go version) | Out-String).Trim() } else { "missing" }
$safe.tools.rust = if (Get-Command rustc -ErrorAction SilentlyContinue) { ((& rustc --version) | Out-String).Trim() } else { "missing" }
$safe.tools.cargo = if (Get-Command cargo -ErrorAction SilentlyContinue) { ((& cargo --version) | Out-String).Trim() } else { "missing" }
$safe.tools.node = if (Get-Command node -ErrorAction SilentlyContinue) { ((& node --version) | Out-String).Trim() } else { "missing" }
$safe.tools.npm = if (Get-Command npm -ErrorAction SilentlyContinue) { ((& npm --version) | Out-String).Trim() } else { "missing" }
if (Test-Path -LiteralPath $runtime) {
    $safe.runtime_files = @(Get-ChildItem -LiteralPath $runtime -File -Recurse | ForEach-Object {
        [pscustomobject]@{ name = [IO.Path]::GetRelativePath($runtime, $_.FullName).Replace("\", "/"); size_bytes = $_.Length; sha256 = (Get-FileHash -LiteralPath $_.FullName -Algorithm SHA256).Hash.ToLowerInvariant() }
    })
    foreach ($name in @("build-summary.json", "dll-imports.log")) {
        $source = Join-Path $runtime $name
        if (Test-Path -LiteralPath $source -PathType Leaf) { Copy-Item -LiteralPath $source -Destination $OutputDirectory }
    }
    $handshake = Join-Path $runtime "logs\handshake\stdout.json"
    if (Test-Path -LiteralPath $handshake -PathType Leaf) { Copy-Item -LiteralPath $handshake -Destination (Join-Path $OutputDirectory "handshake.json") }
}
$results = Join-Path $root ".runtime-build\windows-x86_64\results"
if (Test-Path -LiteralPath $results -PathType Container) {
    Get-ChildItem -LiteralPath $results -Filter *.json -File | ForEach-Object { Copy-Item -LiteralPath $_.FullName -Destination $OutputDirectory }
}
$safe.processes = @(Get-CimInstance Win32_Process | Where-Object { $_.Name -in @("Voxly.exe", "meeting-asr-go.exe", "voxly-sensevoice-wrapper.exe") } | ForEach-Object {
    [pscustomobject]@{ name = $_.Name; process_id = $_.ProcessId; parent_process_id = $_.ParentProcessId }
})
Write-JsonFile (Join-Path $OutputDirectory "diagnostics.json") $safe
$zip = "$OutputDirectory.zip"
Remove-Item -LiteralPath $zip -Force -ErrorAction SilentlyContinue
Compress-Archive -Path (Join-Path $OutputDirectory "*") -DestinationPath $zip
Write-Host "Safe diagnostics directory: $OutputDirectory"
Write-Host "Shareable archive: $zip"

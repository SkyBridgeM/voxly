[CmdletBinding()]
param([string]$RuntimeDirectory = "")

. (Join-Path $PSScriptRoot "sensevoice-common.ps1")
Assert-WindowsX64
$root = Get-VoxlyRepoRoot
if (-not $RuntimeDirectory) { $RuntimeDirectory = Get-WindowsRuntimeDirectory }
$RuntimeDirectory = (Resolve-Path -LiteralPath $RuntimeDirectory).Path
$manifest = Get-Content -LiteralPath (Join-Path $root "runtime\sensevoice\windows-runtime-assets.json") -Raw | ConvertFrom-Json

$expected = @($manifest.files.path)
foreach ($relative in $expected) {
    $path = Join-Path $RuntimeDirectory $relative
    if (-not (Test-Path -LiteralPath $path -PathType Leaf)) { throw "Missing required runtime asset: $relative" }
}
$dlls = @(Get-ChildItem -LiteralPath $RuntimeDirectory -Filter *.dll -File -Recurse)
if ($dlls.Count -ne @($manifest.non_system_dlls).Count) {
    throw "Unexpected DLL set. Static Windows runtime expects no packaged non-system DLLs; found: $($dlls.Name -join ', ')"
}

$importsLog = Join-Path $RuntimeDirectory "dll-imports.log"
$imports = Get-DllImports (Join-Path $RuntimeDirectory "voxly-sensevoice-wrapper.exe") $importsLog
$unsafeImports = @($imports | Where-Object { -not (Test-SystemDll $_) })
if ($unsafeImports.Count -gt 0) {
    throw "Wrapper imports unbundled non-system DLLs: $($unsafeImports -join ', '). See $importsLog"
}
[pscustomobject]@{ status = "valid"; runtime_directory = $RuntimeDirectory; files = $expected; dll_imports = $imports; imports_log = $importsLog } | ConvertTo-Json -Depth 6

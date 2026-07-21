[CmdletBinding()]
param([switch]$Clean, [switch]$VerboseBuild)

. (Join-Path $PSScriptRoot "sensevoice-common.ps1")
Assert-WindowsX64
$root = Get-VoxlyRepoRoot
$null = Require-Command "git.exe" "Install Git for Windows."
$null = Require-Command "cmake.exe" "Install CMake and run from x64 Native Tools PowerShell for VS 2022."
$null = Require-Command "cl.exe" "Install MSVC v143 and run from x64 Native Tools PowerShell for VS 2022."
$null = Require-Command "go.exe" "Install Go."

$revision = "142807252687d81b40d6315f23470a1512a00de3"
$source = Join-Path $root ".runtime-build\sherpa-onnx-v1.13.4-windows"
$build = Join-Path $root ".runtime-build\windows-x86_64\build"
$runtime = Get-WindowsRuntimeDirectory
if ($Clean) { Remove-Item -LiteralPath $build, $runtime -Recurse -Force -ErrorAction SilentlyContinue }
New-Item -ItemType Directory -Force -Path (Split-Path $source), $build, (Join-Path $runtime "licenses") | Out-Null

if (-not (Test-Path -LiteralPath (Join-Path $source ".git"))) {
    & git clone --filter=blob:none --no-checkout https://github.com/k2-fsa/sherpa-onnx.git $source
    if ($LASTEXITCODE -ne 0) { throw "Could not clone sherpa-onnx." }
}
& git -C $source fetch --depth 1 origin $revision
if ($LASTEXITCODE -ne 0) { throw "Could not fetch pinned sherpa-onnx revision." }
& git -C $source checkout --detach $revision
if ($LASTEXITCODE -ne 0 -or ((& git -C $source rev-parse HEAD).Trim() -ne $revision)) { throw "Pinned sherpa-onnx revision verification failed." }

$configure = @(
    "-S", (Join-Path $root "runtime\sensevoice"), "-B", $build,
    "-G", "Visual Studio 17 2022", "-A", "x64",
    "-DSHERPA_ONNX_SOURCE_DIR=$source", "-DCMAKE_BUILD_TYPE=Release",
    "-DSHERPA_ONNX_USE_STATIC_CRT=ON", "-DBUILD_SHARED_LIBS=OFF"
)
if ($VerboseBuild) { $configure += "--debug-output" }
& cmake @configure
if ($LASTEXITCODE -ne 0) { throw "CMake configure failed." }
$buildArgs = @("--build", $build, "--config", "Release", "--target", "voxly-sensevoice-wrapper", "--parallel")
if ($VerboseBuild) { $buildArgs += "--verbose" }
& cmake @buildArgs
if ($LASTEXITCODE -ne 0) { throw "Native wrapper build failed." }

$built = Join-Path $build "Release\voxly-sensevoice-wrapper.exe"
if (-not (Test-Path -LiteralPath $built -PathType Leaf)) { $built = Join-Path $build "voxly-sensevoice-wrapper.exe" }
if (-not (Test-Path -LiteralPath $built -PathType Leaf)) { throw "Built wrapper executable was not found." }

Copy-Item -LiteralPath $built -Destination (Join-Path $runtime "voxly-sensevoice-wrapper.exe") -Force
Copy-Item -LiteralPath (Join-Path $root "runtime\sensevoice\runtime-manifest.json") -Destination (Join-Path $runtime "runtime-manifest.json") -Force
Copy-Item -LiteralPath (Join-Path $source "LICENSE") -Destination (Join-Path $runtime "licenses\LICENSE.sherpa-onnx") -Force
Copy-Item -LiteralPath (Join-Path $root "runtime\sensevoice\THIRD_PARTY_NOTICES.md") -Destination (Join-Path $runtime "licenses\THIRD_PARTY_NOTICES.md") -Force

$goDev = Join-Path $root "core-go\meeting-asr-go.exe"
Push-Location (Join-Path $root "core-go")
try {
    & go build -o $goDev .
    if ($LASTEXITCODE -ne 0) { throw "Go sidecar build failed." }
} finally { Pop-Location }
Copy-Item -LiteralPath (Join-Path $runtime "voxly-sensevoice-wrapper.exe") -Destination (Join-Path $root "core-go\voxly-sensevoice-wrapper.exe") -Force

$tauriBinaries = Join-Path $root "desktop\src-tauri\binaries"
$tauriResources = Join-Path $root "desktop\src-tauri\resources\sensevoice"
New-Item -ItemType Directory -Force -Path $tauriBinaries, $tauriResources | Out-Null
Copy-Item -LiteralPath $goDev -Destination (Join-Path $tauriBinaries "meeting-asr-go-x86_64-pc-windows-msvc.exe") -Force
Copy-Item -LiteralPath (Join-Path $runtime "voxly-sensevoice-wrapper.exe") -Destination (Join-Path $tauriBinaries "voxly-sensevoice-wrapper-x86_64-pc-windows-msvc.exe") -Force
Copy-Item -LiteralPath (Join-Path $runtime "runtime-manifest.json") -Destination $tauriResources -Force
Copy-Item -LiteralPath (Join-Path $runtime "licenses\LICENSE.sherpa-onnx") -Destination $tauriResources -Force
Copy-Item -LiteralPath (Join-Path $runtime "licenses\THIRD_PARTY_NOTICES.md") -Destination $tauriResources -Force

& (Join-Path $PSScriptRoot "verify-sensevoice-runtime-assets.ps1") -RuntimeDirectory $runtime | Out-Host

$files = Get-ChildItem -LiteralPath $runtime -File -Recurse | Sort-Object FullName | ForEach-Object {
    [pscustomobject]@{ path = [IO.Path]::GetRelativePath($runtime, $_.FullName).Replace("\", "/"); size_bytes = $_.Length; sha256 = (Get-FileHash -LiteralPath $_.FullName -Algorithm SHA256).Hash.ToLowerInvariant() }
}
$summary = [pscustomobject]@{
    status = "built"; target = "x86_64-pc-windows-msvc"; configuration = "Release"
    wrapper_version = "1.0.0"; protocol_version = "1.0"; sherpa_onnx_revision = $revision
    linkage = "static"; cpu_baseline = "MSVC x64 default; no AVX2 requirement"
    output_directory = ".runtime-build/windows-x86_64/runtime"; files = $files
}
Write-JsonFile (Join-Path $runtime "build-summary.json") $summary
$summary | ConvertTo-Json -Depth 8

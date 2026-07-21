[CmdletBinding()]
param()

. (Join-Path $PSScriptRoot "sensevoice-common.ps1")
Assert-WindowsX64
$root = Get-VoxlyRepoRoot

$requirements = [ordered]@{
    git = @("git.exe", "Install Git for Windows 2.40 or newer.")
    cmake = @("cmake.exe", "Install CMake 3.24 or newer and add it to PATH.")
    cl = @("cl.exe", "Install Visual Studio 2022 Build Tools with Desktop development with C++, MSVC v143, and a Windows 10/11 SDK; run from x64 Native Tools PowerShell.")
    dumpbin = @("dumpbin.exe", "Run from x64 Native Tools PowerShell for VS 2022.")
    go = @("go.exe", "Install Go 1.21 or newer, matching the core-go/go.mod directive.")
    rustup = @("rustup.exe", "Install stable Rust with the MSVC host toolchain.")
    cargo = @("cargo.exe", "Install stable Rust with rustup.")
    node = @("node.exe", "Install Node.js 22 LTS or newer.")
    npm = @("npm.cmd", "Install npm with Node.js.")
}

$toolPaths = [ordered]@{}
foreach ($item in $requirements.GetEnumerator()) {
    $path = Require-Command $item.Value[0] $item.Value[1]
    $toolPaths[$item.Key] = $path
}

if ($PSVersionTable.PSVersion -lt [version]"7.4") { throw "PowerShell 7.4 or newer is required." }
$rustTargets = & rustup target list --installed
if ($LASTEXITCODE -ne 0 -or $rustTargets -notcontains "x86_64-pc-windows-msvc") {
    throw "Install the Rust target: rustup target add x86_64-pc-windows-msvc"
}
$webViewCandidates = @(
    "${env:ProgramFiles(x86)}\Microsoft\EdgeWebView\Application",
    "$env:ProgramFiles\Microsoft\EdgeWebView\Application"
)
if (-not ($webViewCandidates | Where-Object { Test-Path -LiteralPath $_ })) {
    throw "Microsoft Edge WebView2 Runtime was not found. Install the Evergreen WebView2 Runtime."
}
if (-not $env:WindowsSdkDir -or -not $env:WindowsSDKVersion) {
    throw "Windows SDK was not initialized. Install a Windows 10/11 SDK and run from x64 Native Tools PowerShell for VS 2022."
}

$branch = (& git -C $root branch --show-current).Trim()
$commit = (& git -C $root rev-parse HEAD).Trim()
if ($branch -ne "codex/integrate-sensevoice-runtime") { throw "Expected PR #49 branch; found '$branch'." }
$gitVersion = ((& git --version) | Out-String).Trim()
$cmakeVersion = ((& cmake --version | Select-Object -First 1) | Out-String).Trim()
$msvcVersion = ((& cmd.exe /d /c "cl 2>&1" | Select-Object -First 1) | Out-String).Trim()
$goVersion = ((& go version) | Out-String).Trim()
$rustVersion = ((& rustc --version) | Out-String).Trim()
$cargoVersion = ((& cargo --version) | Out-String).Trim()
$nodeVersion = ((& node --version) | Out-String).Trim()
$npmVersion = ((& npm --version) | Out-String).Trim()
Assert-MinimumVersion "Git" $gitVersion '([0-9]+\.[0-9]+(?:\.[0-9]+)?)' ([version]"2.40") "Install a current Git for Windows."
Assert-MinimumVersion "CMake" $cmakeVersion '([0-9]+\.[0-9]+(?:\.[0-9]+)?)' ([version]"3.24") "Install CMake from cmake.org or Visual Studio Installer."
Assert-MinimumVersion "MSVC" $msvcVersion 'Version\s+([0-9]+\.[0-9]+(?:\.[0-9]+)?)' ([version]"19.30") "Install Visual Studio 2022 MSVC v143."
Assert-MinimumVersion "Go" $goVersion 'go([0-9]+\.[0-9]+(?:\.[0-9]+)?)' ([version]"1.21") "Install Go from go.dev."
Assert-MinimumVersion "Node.js" $nodeVersion 'v([0-9]+\.[0-9]+(?:\.[0-9]+)?)' ([version]"22.0") "Install Node.js 22 LTS or newer."
$activeRust = ((& rustup show active-toolchain) | Out-String).Trim()
if ($activeRust -notmatch '^stable-' -or $activeRust -notmatch 'msvc') { throw "Use the stable Rust MSVC toolchain. Detected '$activeRust'." }
$versions = [ordered]@{
    git = $gitVersion
    cmake = $cmakeVersion
    msvc = $msvcVersion
    windows_sdk = if ($env:WindowsSDKVersion) { $env:WindowsSDKVersion.TrimEnd("\") } else { "detected through MSVC environment" }
    go = $goVersion
    rust = $rustVersion
    cargo = $cargoVersion
    node = $nodeVersion
    npm = $npmVersion
}

[pscustomobject]@{
    status = "ready"
    windows = [Environment]::OSVersion.VersionString
    architecture = "x64"
    powershell = $PSVersionTable.PSVersion.ToString()
    branch = $branch
    commit = $commit
    tool_paths = $toolPaths
    versions = $versions
    rust_target = "x86_64-pc-windows-msvc"
    webview2 = "detected"
} | ConvertTo-Json -Depth 5

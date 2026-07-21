Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

function Get-VoxlyRepoRoot {
    return (Resolve-Path (Join-Path $PSScriptRoot "..\..")).Path
}

function Require-Command([string]$Name, [string]$Guidance) {
    $command = Get-Command $Name -ErrorAction SilentlyContinue
    if (-not $command) { throw "Missing $Name. $Guidance" }
    return $command.Source
}

function Assert-MinimumVersion([string]$Label, [string]$Text, [string]$Pattern, [version]$Minimum, [string]$Guidance) {
    $match = [regex]::Match($Text, $Pattern)
    if (-not $match.Success -or [version]$match.Groups[1].Value -lt $Minimum) {
        throw "$Label $Minimum or newer is required. Detected '$Text'. $Guidance"
    }
}

function Assert-WindowsX64 {
    if (-not $IsWindows) { throw "This command must run in PowerShell 7 on Windows 10 or 11." }
    if ([Runtime.InteropServices.RuntimeInformation]::OSArchitecture -ne [Runtime.InteropServices.Architecture]::X64) {
        throw "Windows x64 is required. Detected $([Runtime.InteropServices.RuntimeInformation]::OSArchitecture)."
    }
}

function Start-CapturedProcess {
    param(
        [Parameter(Mandatory)][string]$FilePath,
        [string[]]$Arguments = @(),
        [Parameter(Mandatory)][string]$StdoutPath,
        [Parameter(Mandatory)][string]$StderrPath,
        [int]$TimeoutSeconds = 300,
        [string]$WorkingDirectory = (Get-Location).Path
    )
    $info = [Diagnostics.ProcessStartInfo]::new()
    $info.FileName = $FilePath
    $info.UseShellExecute = $false
    $info.CreateNoWindow = $true
    $info.WorkingDirectory = $WorkingDirectory
    $info.RedirectStandardOutput = $true
    $info.RedirectStandardError = $true
    foreach ($argument in $Arguments) { [void]$info.ArgumentList.Add($argument) }
    $process = [Diagnostics.Process]::new()
    $process.StartInfo = $info
    if (-not $process.Start()) { throw "Could not start $FilePath." }
    $stdoutTask = $process.StandardOutput.ReadToEndAsync()
    $stderrTask = $process.StandardError.ReadToEndAsync()
    if (-not $process.WaitForExit($TimeoutSeconds * 1000)) {
        $process.Kill($true)
        $process.WaitForExit()
        [IO.File]::WriteAllText($StdoutPath, $stdoutTask.GetAwaiter().GetResult(), [Text.UTF8Encoding]::new($false))
        [IO.File]::WriteAllText($StderrPath, $stderrTask.GetAwaiter().GetResult(), [Text.UTF8Encoding]::new($false))
        throw "$FilePath exceeded the ${TimeoutSeconds}s timeout and was terminated."
    }
    $stdout = $stdoutTask.GetAwaiter().GetResult()
    $stderr = $stderrTask.GetAwaiter().GetResult()
    [IO.File]::WriteAllText($StdoutPath, $stdout, [Text.UTF8Encoding]::new($false))
    [IO.File]::WriteAllText($StderrPath, $stderr, [Text.UTF8Encoding]::new($false))
    return [pscustomobject]@{ ExitCode = $process.ExitCode; Stdout = $stdout; Stderr = $stderr }
}

function Write-JsonFile([string]$Path, [object]$Value) {
    $Value | ConvertTo-Json -Depth 12 | Set-Content -LiteralPath $Path -Encoding utf8NoBOM
}

function Get-WindowsRuntimeDirectory {
    return Join-Path (Get-VoxlyRepoRoot) ".runtime-build\windows-x86_64\runtime"
}

function Test-SystemDll([string]$Name) {
    if ($Name.StartsWith("api-ms-win-", [StringComparison]::OrdinalIgnoreCase) -or
        $Name.StartsWith("ext-ms-win-", [StringComparison]::OrdinalIgnoreCase)) { return $true }
    $systemDirectory = [Environment]::SystemDirectory
    return Test-Path -LiteralPath (Join-Path $systemDirectory $Name) -PathType Leaf
}

function Get-DllImports([string]$Executable, [string]$LogPath) {
    $dumpbin = Require-Command "dumpbin.exe" "Open an x64 Native Tools for VS 2022 shell and retry."
    $result = Start-CapturedProcess -FilePath $dumpbin -Arguments @("/DEPENDENTS", $Executable) `
        -StdoutPath $LogPath -StderrPath "$LogPath.stderr.log" -TimeoutSeconds 60
    if ($result.ExitCode -ne 0) { throw "dumpbin failed. See $LogPath.stderr.log" }
    return @([regex]::Matches($result.Stdout, '(?im)^\s+([A-Za-z0-9_.-]+\.dll)\s*$') | ForEach-Object { $_.Groups[1].Value } | Sort-Object -Unique)
}

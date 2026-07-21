[CmdletBinding()]
param(
    [Parameter(Mandatory)][string]$AppDirectory,
    [string]$ModelArchive = "",
    [string]$AudioFile = "",
    [switch]$KeepArtifacts
)

. (Join-Path $PSScriptRoot "sensevoice-common.ps1")
Assert-WindowsX64
$AppDirectory = (Resolve-Path -LiteralPath $AppDirectory).Path
$required = @(
    "Voxly.exe", "meeting-asr-go.exe", "voxly-sensevoice-wrapper.exe",
    "resources\sensevoice\runtime-manifest.json",
    "resources\sensevoice\LICENSE.sherpa-onnx",
    "resources\sensevoice\THIRD_PARTY_NOTICES.md"
)
foreach ($relative in $required) {
    if (-not (Test-Path -LiteralPath (Join-Path $AppDirectory $relative) -PathType Leaf)) { throw "Packaged asset missing: $relative" }
}
if (Get-ChildItem -LiteralPath $AppDirectory -Filter "model.int8.onnx" -File -Recurse) { throw "Model weights must not be bundled." }
$unexpectedRuntimeDlls = @(Get-ChildItem -LiteralPath $AppDirectory -File -Recurse | Where-Object { $_.Name -match '^(onnxruntime|sherpa-onnx).+\.dll$' })
if ($unexpectedRuntimeDlls.Count -gt 0) { throw "Static runtime package contains unexpected runtime DLLs: $($unexpectedRuntimeDlls.Name -join ', ')" }

$smokeRoot = Join-Path ([IO.Path]::GetTempPath()) ("Voxly 打包烟测 " + [guid]::NewGuid().ToString("N"))
$copiedApp = Join-Path $smokeRoot "应用 文件"
$logs = Join-Path $smokeRoot "logs"
$results = Join-Path (Get-VoxlyRepoRoot) ".runtime-build\windows-x86_64\results"
New-Item -ItemType Directory -Force -Path $copiedApp, $logs | Out-Null
New-Item -ItemType Directory -Force -Path $results | Out-Null
Copy-Item -Path (Join-Path $AppDirectory "*") -Destination $copiedApp -Recurse -Force
try {
    $wrapper = Join-Path $copiedApp "voxly-sensevoice-wrapper.exe"
    $handshake = Start-CapturedProcess -FilePath $wrapper -Arguments @("--version-json") -StdoutPath (Join-Path $logs "handshake.stdout.json") -StderrPath (Join-Path $logs "handshake.stderr.log") -TimeoutSeconds 10
    if ($handshake.ExitCode -ne 0) { throw "Packaged handshake failed. Logs: $logs" }
    $identity = $handshake.Stdout | ConvertFrom-Json
    if ($identity.platform -ne "windows" -or $identity.architecture -ne "amd64" -or $identity.engine_version -ne "1.13.4") { throw "Packaged handshake identity mismatch." }

    $runtimeResult = "not_requested"
    if ($ModelArchive -or $AudioFile) {
        if (-not $ModelArchive -or -not $AudioFile) { throw "Provide both -ModelArchive and -AudioFile for packaged transcription." }
        $ModelArchive = (Resolve-Path -LiteralPath $ModelArchive).Path
        $AudioFile = (Resolve-Path -LiteralPath $AudioFile).Path
        $expectedHash = "7d1efa2138a65b0b488df37f8b89e3d91a60676e416f515b952358d83dfd347e"
        if ((Get-FileHash -LiteralPath $ModelArchive -Algorithm SHA256).Hash.ToLowerInvariant() -ne $expectedHash) { throw "Official model archive SHA-256 mismatch." }
        $go = Join-Path $copiedApp "meeting-asr-go.exe"
        $appData = Join-Path $smokeRoot "应用 数据"
        $output = Join-Path $smokeRoot "任务 输出"
        New-Item -ItemType Directory -Force -Path $appData, $output | Out-Null
        $import = Start-CapturedProcess -FilePath $go -Arguments @("import-model", "--app-data-root", $appData, "--model-id", "sensevoice-small-int8", "--source-archive", $ModelArchive) -StdoutPath (Join-Path $logs "import.stdout.json") -StderrPath (Join-Path $logs "import.stderr.log") -TimeoutSeconds 900
        if ($import.ExitCode -ne 0 -or ($import.Stdout | ConvertFrom-Json).state -ne "ready") { throw "Packaged model import/readiness failed. Logs: $logs" }
        $run = Start-CapturedProcess -FilePath $go -Arguments @("transcribe", $AudioFile, "--output-dir", $output, "--output-mode", "transcript_only", "--asr-provider", "local-sensevoice", "--asr-model", "sensevoice-small-int8", "--app-data-root", $appData) -StdoutPath (Join-Path $logs "transcribe.stdout.log") -StderrPath (Join-Path $logs "transcribe.stderr.log") -TimeoutSeconds 2700
        if ($run.ExitCode -ne 0) { throw "Packaged transcription failed. Logs: $logs" }
        $transcript = Get-ChildItem -LiteralPath $output -Filter transcript.md -File -Recurse | Select-Object -First 1
        if (-not $transcript -or $transcript.Length -eq 0) { throw "Packaged transcription produced no transcript." }
        $runtimeResult = "passed"
    }
    $summary = [pscustomobject]@{
        status = "passed"; copied_app_path_test = "Chinese and spaces"; handshake = "passed"
        transcription = $runtimeResult; transcript_text_collected = $false
        log_location = if ($KeepArtifacts) { "retained smoke directory printed locally" } else { "cleaned after success" }
    }
    Write-JsonFile (Join-Path $results "packaged-smoke.json") $summary
    $summary | ConvertTo-Json -Depth 5
} finally {
    if (-not $KeepArtifacts) { Remove-Item -LiteralPath $smokeRoot -Recurse -Force -ErrorAction SilentlyContinue }
    else { Write-Host "Artifacts retained: $smokeRoot" }
}

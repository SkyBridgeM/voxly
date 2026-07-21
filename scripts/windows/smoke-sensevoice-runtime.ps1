[CmdletBinding()]
param(
    [Parameter(Mandatory)][string]$ModelArchive,
    [Parameter(Mandatory)][string]$AudioFile,
    [switch]$KeepArtifacts
)

. (Join-Path $PSScriptRoot "sensevoice-common.ps1")
Assert-WindowsX64
$root = Get-VoxlyRepoRoot
$ModelArchive = (Resolve-Path -LiteralPath $ModelArchive).Path
$AudioFile = (Resolve-Path -LiteralPath $AudioFile).Path
$expectedHash = "7d1efa2138a65b0b488df37f8b89e3d91a60676e416f515b952358d83dfd347e"
if ((Get-FileHash -LiteralPath $ModelArchive -Algorithm SHA256).Hash.ToLowerInvariant() -ne $expectedHash) {
    throw "Official model archive SHA-256 mismatch. The source archive was not modified."
}
$go = Join-Path $root "core-go\meeting-asr-go.exe"
$wrapper = Join-Path $root "core-go\voxly-sensevoice-wrapper.exe"
if (-not (Test-Path -LiteralPath $go -PathType Leaf) -or -not (Test-Path -LiteralPath $wrapper -PathType Leaf)) {
    throw "Build the Windows runtime first: .\scripts\windows\build-sensevoice-runtime.ps1"
}

$smokeRoot = Join-Path ([IO.Path]::GetTempPath()) ("voxly-pr49-windows-" + [guid]::NewGuid().ToString("N"))
$appData = Join-Path $smokeRoot "app-data"
$output = Join-Path $smokeRoot "output"
$logs = Join-Path $smokeRoot "logs"
$results = Join-Path $root ".runtime-build\windows-x86_64\results"
New-Item -ItemType Directory -Force -Path $appData, $output, $logs | Out-Null
New-Item -ItemType Directory -Force -Path $results | Out-Null
$started = Get-Date
$status = "failed"
try {
    $import = Start-CapturedProcess -FilePath $go -Arguments @("import-model", "--app-data-root", $appData, "--model-id", "sensevoice-small-int8", "--source-archive", $ModelArchive) `
        -StdoutPath (Join-Path $logs "import.stdout.json") -StderrPath (Join-Path $logs "import.stderr.log") -TimeoutSeconds 900 -WorkingDirectory $root
    if ($import.ExitCode -ne 0) { throw "Model import failed. Logs: $logs" }
    $importStatus = $import.Stdout | ConvertFrom-Json
    if ($importStatus.state -ne "ready") { throw "Expected ready after import; found '$($importStatus.state)'. Logs: $logs" }

    $run = Start-CapturedProcess -FilePath $go -Arguments @(
        "transcribe", $AudioFile, "--output-dir", $output, "--output-mode", "transcript_only",
        "--asr-provider", "local-sensevoice", "--asr-model", "sensevoice-small-int8", "--app-data-root", $appData
    ) -StdoutPath (Join-Path $logs "transcribe.stdout.log") -StderrPath (Join-Path $logs "transcribe.stderr.log") -TimeoutSeconds 2700 -WorkingDirectory $root
    if ($run.ExitCode -ne 0) { throw "Local transcription failed. Logs: $logs" }
    $manifest = Get-ChildItem -LiteralPath $output -Filter task_manifest.json -File -Recurse | Select-Object -First 1
    if (-not $manifest) { throw "No TaskManifest was created. Logs: $logs" }
    $task = Get-Content -LiteralPath $manifest.FullName -Raw | ConvertFrom-Json
    $transcript = Join-Path $manifest.Directory.FullName "transcript.md"
    if ($task.execution.status -ne "completed" -or -not (Test-Path -LiteralPath $transcript) -or (Get-Item -LiteralPath $transcript).Length -eq 0) {
        throw "Task did not produce a nonempty completed transcript. Logs: $logs"
    }
    $status = "passed"
    $summary = [pscustomobject]@{
        status = $status; elapsed_seconds = [math]::Round(((Get-Date) - $started).TotalSeconds, 2)
        model_archive_sha256 = $expectedHash; task_status = $task.execution.status
        transcript_nonempty = $true; transcript_text_collected = $false
        log_location = if ($KeepArtifacts) { "retained smoke directory printed locally" } else { "cleaned after success" }
        artifacts_kept = [bool]$KeepArtifacts
    }
    Write-JsonFile (Join-Path $logs "summary.json") $summary
    Write-JsonFile (Join-Path $results "real-model-smoke.json") $summary
    $summary | ConvertTo-Json -Depth 6
} finally {
    if (-not $KeepArtifacts -and $status -eq "passed") { Remove-Item -LiteralPath $smokeRoot -Recurse -Force }
    elseif ($status -ne "passed") { Write-Warning "Smoke artifacts retained for diagnosis: $smokeRoot" }
}

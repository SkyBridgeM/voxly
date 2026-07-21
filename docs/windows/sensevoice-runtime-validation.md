# SenseVoice Windows x64 build and acceptance

## Status

Windows support is **prepared but unverified**. The repository freezes the native toolchain, build, asset manifest, handshake, real-model smoke, packaged smoke, diagnostics, and acceptance checklist. Do not claim Windows support until the native checklist passes on Windows 10/11 x64.

## macOS-side audit

| Integration point | Status before native Windows acceptance |
| --- | --- |
| wrapper CMake target, pinned source and JSON dependency | prepared but unverified |
| sherpa-onnx / ONNX Runtime static linkage | prepared but unverified |
| MSVC x64 target triple and deterministic filenames | complete |
| runtime manifest and exact asset manifest | complete |
| `StatusWithRuntime` missing / invalid / ready composition | complete and host-tested |
| Tauri target-suffixed external binaries and resources | prepared but unverified |
| development and packaged sibling lookup | complete in code; requires Windows smoke |
| Go cancellation, bounded drain and wait/reap | complete in code; requires Windows smoke |
| wrapper exact-parent monitor after forced Go termination | prepared but unverified |
| UTF-8 JSON, drive/space/Chinese path transport | complete in code; requires native filesystem smoke |
| x64 SSE2 baseline and absence of AVX2 build flags | complete in repository configuration; requires hardware smoke |
| native wrapper, installer and real transcription | blocker until Windows execution |

## Prerequisites

Use Windows 10 or 11 x64 and PowerShell 7.4 or newer. MinGW and WSL are unsupported.

- Git for Windows 2.40+
- Visual Studio 2022 or Build Tools 2022
- workload: Desktop development with C++
- MSVC v143 x64 tools and a Windows 10/11 SDK
- CMake 3.24+
- Rust stable with `x86_64-pc-windows-msvc`
- Go 1.21 or newer (matching the `core-go/go.mod` directive)
- Node.js 22 LTS+ and npm
- Microsoft Edge WebView2 Evergreen Runtime
- ffmpeg on `PATH` for real transcription smoke

Open **x64 Native Tools PowerShell for VS 2022**, switch to the repository, and run:

```powershell
.\scripts\windows\check-sensevoice-environment.ps1
```

The check prints the current PR branch and commit and fails with actionable guidance when a dependency is missing.

## One native build

```powershell
.\scripts\windows\build-sensevoice-runtime.ps1 -Clean
```

Add `-VerboseBuild` only for diagnostics. The build uses Visual Studio 2022, x64 Release, sherpa-onnx `v1.13.4` at commit `142807252687d81b40d6315f23470a1512a00de3`, wrapper `1.0.0`, protocol `1.0`, nlohmann/json `v3.12.0`, and ONNX Runtime `1.27.0`. It writes generated files only under ignored build/package locations.

Authoritative output:

```text
.runtime-build/windows-x86_64/runtime/
  voxly-sensevoice-wrapper.exe
  runtime-manifest.json
  build-summary.json
  licenses/
    LICENSE.sherpa-onnx
    THIRD_PARTY_NOTICES.md
```

The Windows runtime is statically linked to sherpa-onnx, ONNX Runtime, and the MSVC CRT. The fixed ONNX archive is `onnxruntime-win-x64-static_lib-MT-Release-1.27.0.tar.bz2`, SHA-256 `6db0cdd3bcb208911758b7336820d19ecff0359a7003e0ec65895be701741288`. No non-system DLL is expected or bundled. `windows-runtime-assets.json` is the authoritative exact asset manifest; `build-summary.json` records actual output SHA-256 values. The build also stages target-suffixed Tauri sidecars and the exact metadata/notices resources without wildcard DLL copying.

## CPU baseline and DLL inspection

The wrapper uses the Visual Studio x64 default instruction set (SSE2 baseline). The build does not pass `/arch:AVX`, `/arch:AVX2`, `-march=native`, DirectML, CUDA, or GPU flags. ONNX Runtime's reviewed x64 static package may runtime-dispatch optimized kernels, but the Windows acceptance run must still confirm startup and transcription on the intended alpha hardware.

Inspect exact imports and validate the asset manifest:

```powershell
.\scripts\windows\verify-sensevoice-runtime-assets.ps1
```

This runs `dumpbin /DEPENDENTS`, writes `dll-imports.log`, and fails if the wrapper imports an unbundled non-system DLL. It does not use `PATH` to satisfy packaged runtime dependencies.

## Handshake and real-model smoke

Handshake, without a model:

```powershell
.\scripts\windows\smoke-sensevoice-handshake.ps1
```

Real official model and audio, using isolated app-data/output roots:

```powershell
.\scripts\windows\smoke-sensevoice-runtime.ps1 `
  -ModelArchive "C:\path\to\sherpa-onnx-sense-voice-zh-en-ja-ko-yue-int8-2024-07-17.tar.bz2" `
  -AudioFile "C:\Users\Test User\Desktop\语音 测试\会议录音 01.wav"
```

Use `-KeepArtifacts` only when failure diagnosis is needed. The fixed model archive SHA-256 is verified before import; the source archive is never changed. The summary records duration and success without collecting transcript text.

## Development app

After the native build:

```powershell
npm --prefix desktop ci
npm --prefix desktop run tauri -- dev
```

Before launch, confirm these files exist together:

```text
core-go/meeting-asr-go.exe
core-go/voxly-sensevoice-wrapper.exe
```

Expected readiness is `not_installed` when the model is absent and `ready` only when the fixed model and compatible wrapper are both valid. Verify 中文为主 selection, short Mandarin, mixed Chinese/English, multi-chunk ordering, cancellation, app-exit cleanup, no Mimo credential/fallback, no generated `[Part ###]`, and Task Library/Details provenance.

## Packaged app

Build the exact runtime assets first, then:

```powershell
npm --prefix desktop ci
npm --prefix desktop run tauri -- build --bundles nsis
```

After installing or extracting the produced package, locate the directory containing `Voxly.exe` and run:

```powershell
.\scripts\windows\smoke-sensevoice-packaged.ps1 `
  -AppDirectory "C:\path\to\installed\Voxly" `
  -ModelArchive "C:\path\to\official-model.tar.bz2" `
  -AudioFile "C:\Users\Test User\Desktop\语音 测试\会议录音 01.wav"
```

The packaged layout is:

```text
<application directory>/
  Voxly.exe
  meeting-asr-go.exe
  voxly-sensevoice-wrapper.exe
  resources/sensevoice/
    runtime-manifest.json
    LICENSE.sherpa-onnx
    THIRD_PARTY_NOTICES.md
```

No model weights or ONNX files belong in the application bundle. The packaged smoke copies the installed app to a temporary Chinese/space-containing path, validates handshake and optional real transcription outside the repository, and rejects missing assets.

## Path and process safety

Rust and Go use typed process argument APIs; no shell command string or manual argument quoting is used. JSON is UTF-8, Go preserves Windows drive/backslash paths, and pinned sherpa-onnx converts UTF-8 file paths to wide Windows paths before file access. The native acceptance run must cover spaces, Chinese characters, and practical long paths.

Rust owns and reaps the Go sidecar. Go owns and reaps the wrapper; cancellation uses `Process.Kill` with bounded `WaitDelay`. The wrapper opens a synchronization handle to its exact parent process and exits if the Go sidecar is killed by app exit. No process-name-wide killing occurs. Windows acceptance must verify cancellation, forced app exit, no pipe deadlock, and no remaining related process.

## Safe diagnostics and final checklist

```powershell
.\scripts\windows\collect-sensevoice-diagnostics.ps1
```

This produces a shareable directory and zip containing only OS/architecture, tool paths, branch/commit, build/smoke logs, handshake, runtime names/sizes/hashes, DLL imports, readiness material when present, and filtered Voxly process identities. It excludes credentials, environment dumps, audio, Task contents, and transcript text.

Record the native result in [sensevoice-evening-checklist.md](sensevoice-evening-checklist.md).

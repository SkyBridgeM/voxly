# SenseVoice Windows x64 acceptance checklist

Windows support is not validated until every required row is completed on a native Windows 10/11 x64 PC. Use PR #49 only; do not test from WSL or MinGW.

| Field | Result / value | Log or note |
| --- | --- | --- |
| Windows version |  |  |
| CPU |  |  |
| PR commit |  |  |
| PowerShell version |  |  |
| Visual Studio / MSVC v143 |  |  |
| Windows SDK |  |  |
| CMake |  |  |
| Rust stable / MSVC target |  |  |
| Go |  |  |
| Node.js / npm |  |  |
| WebView2 |  |  |
| Native wrapper build | pass / fail / blocked |  |
| DLL dependency check | pass / fail / blocked |  |
| Handshake | pass / fail / blocked |  |
| Real-model runtime smoke | pass / fail / blocked |  |
| Development readiness | pass / fail / blocked |  |
| Mandarin transcription | pass / fail / blocked |  |
| Mixed-language transcription | pass / fail / blocked |  |
| Multi-chunk ordering | pass / fail / blocked |  |
| Cancellation cleanup | pass / fail / blocked |  |
| App-exit cleanup | pass / fail / blocked |  |
| Chinese/space path | pass / fail / blocked |  |
| Packaged build | pass / fail / blocked |  |
| Packaged transcription | pass / fail / blocked |  |
| Remaining Voxly processes |  |  |
| Log locations |  |  |
| Final result | pass / fail / blocked |  |

After testing, run:

```powershell
.\scripts\windows\collect-sensevoice-diagnostics.ps1
```

The diagnostics intentionally exclude credentials, environment dumps, source audio, Task contents, and transcript text.

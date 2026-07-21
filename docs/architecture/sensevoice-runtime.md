# SenseVoiceSmall INT8 runtime

## Integration audit

| Boundary | Decision |
| --- | --- |
| `modelcatalog.StatusWithRuntime`, receipt and installed-file verification | Reuse; runtime handshake is composed only after model validation. |
| Provider-neutral `ASRProvider` and Mimo adapter | Extend with `localSenseVoiceProvider`; Mimo behavior remains unchanged. |
| Provider-aware planner, ffmpeg splitter and canonical transcript merge | Extend only with a preferred normalized chunk format. |
| TaskManifest schema 2.0 and unknown-field mapping | Extend additively with `execution.asr_runtime`. |
| Rust `ActiveProcess`, async commands, cancel and app-exit cleanup | Reuse for the Go sidecar. Go owns its wrapper child; the wrapper also exits when its parent disappears. |
| Model download/import/receipt/delete and Hugging Face settings | Reuse unchanged. |
| Whisper, other ASR models, VAD, streaming/recording, Ollama | Defer. |

## Pinned runtime and licenses

Voxly's wrapper is version `1.0.0` and implements protocol `1.0`. It links:

- repository: `https://github.com/k2-fsa/sherpa-onnx`
- tag: `v1.13.4`
- commit: `142807252687d81b40d6315f23470a1512a00de3`
- API: native sherpa-onnx C++ offline recognizer API
- license: Apache-2.0
- ONNX Runtime in the reviewed macOS SDK: `1.27.0`

The macOS arm64 fallback build consumes the fixed upstream
`sherpa-onnx-v1.13.4-osx-arm64-shared-no-tts-lib.tar.bz2` SDK with SHA-256
`c003242369046d3c2adc6b48c3c96e0ff129e76738b7f3aa5342828ec8ba410d`.
The source build path checks out the exact commit and disables Python,
PortAudio, JNI, TTS, GPU and WASM. No runtime is downloaded at application
startup. Build-time license collection copies the upstream license and Voxly
notice into the application resource boundary.

The supported CPU baseline is native Apple Silicon on macOS 12+ and
conservative x64 on Windows. No AVX2-only assumption is permitted. Accelerated
GPU/CoreML/DirectML paths are excluded from this stage.

## Runtime and model separation

The wrapper and sherpa/ONNX libraries are application bundle/build assets.
They are resolved beside the trusted Go sidecar or from its signed application
resources. The frozen model remains under app data:

```text
models/sensevoice-small-int8/2024-07-17/
  model.int8.onnx
  tokens.txt
  receipt.json
```

The frontend cannot supply runtime, model, token, hash or receipt paths. Go
loads the embedded catalog, revalidates the receipt and every installed file,
then derives canonical paths. Model deletion never touches runtime assets;
historical Task provenance remains in TaskManifest.

The runtime build script places a development copy beside
`core-go/meeting-asr-go`, matching the desktop development resolver. Tauri
packaging places the Go sidecar and wrapper together under `Contents/MacOS`.

## Handshake and protocol

`voxly-sensevoice-wrapper --version-json` loads no model and performs no
network access. Expected output is exactly one bounded JSON document:

```json
{"architecture":"arm64","engine":"sherpa-onnx","engine_version":"1.13.4","platform":"darwin","protocol_version":"1.0","supported_model_ids":["sensevoice-small-int8"],"wrapper_version":"1.0.0"}
```

Go validates protocol, wrapper, engine, platform, architecture and supported
model before reporting readiness. States are `not_installed`, `model_invalid`,
`runtime_missing`, `runtime_invalid` and `ready`; React does not override them.

Transcription uses one JSON request on stdin and exactly one JSON result on
stdout. Diagnostics use stderr. Request input is capped at 64 KiB; stdout is
capped at 4 MiB and stderr at 64 KiB. Go validates the opaque request ID,
protocol/runtime/model identity, output text size, segment count, chronological
non-negative timestamps and segment text size. SenseVoice exposes token
timestamps but no trustworthy sentence boundaries, so the wrapper returns an
empty `segments` array rather than fabricating segment timestamps.

## Audio, process lifecycle and transcript behavior

The provider planner accepts the existing mp3, m4a and wav input boundary, but
local chunks are always normalized by typed ffmpeg argv to mono, 16 kHz,
PCM-s16le WAV. Default chunks remain five minutes; this runtime caps requested
chunks at 30 minutes and processes them serially. Existing chunk ordering,
resume checkpoints and canonical merge behavior remain authoritative. Only
outer whitespace is trimmed; no broad regex cleanup, translation or provider
markup rewriting is applied. ITN is enabled and language mode is Chinese-first
(`auto` inside the frozen multilingual SenseVoice model).

Rust owns the Go sidecar process and existing desktop progress/events. Go owns
the wrapper child, bounded I/O, per-chunk timeout and result validation. Context
cancellation kills and waits for the child. The wrapper monitors its exact
parent and exits if the Go sidecar is forcibly terminated, preventing an
orphan during desktop cancel or application exit. No process-name killing or
shell command is used.

Per-chunk timeout is two minutes minimum, otherwise approximately twice the
PCM audio duration plus 45 seconds, capped at 45 minutes. Deterministic
protocol/model failures are not retried. A failed/cancelled chunk cannot be
marked complete, no later chunk starts, and no canonical transcript is emitted
from missing chunks.

## TaskManifest provenance and privacy

Schema remains `2.0`. Local Tasks add `execution.asr_runtime` with:

```text
provider_id, model_id, execution_location, runtime_kind,
runtime_version, model_version, model_sha256, engine, engine_runtime
```

No absolute path, endpoint, credential, receipt path or localized text is
stored. Task Library transports runtime/model versions for Task Details.
Transcript-only local ASR sends neither audio nor transcript off-device. When
DeepSeek generation is selected, audio remains local and transcript text is
sent to the configured cloud LLM; the UI must not call that combination fully
local.

## Platform and packaging status

macOS arm64 is implemented and smoke-tested as a native Mach-O executable with
`@rpath` lookup for the bundled sherpa/ONNX dylibs. Development copies use
ad-hoc signatures after copying; final app signing/notarization must re-sign
the wrapper and dylibs together.

Windows x64 source and parent-lifecycle code are present, the runtime filename
is `voxly-sensevoice-wrapper.exe`, and the build is designed for native MSVC
plus pinned sherpa-onnx. The Windows build freezes static sherpa-onnx, static
ONNX Runtime 1.27.0 and the static MSVC CRT, so no non-system runtime DLL is
expected. The authoritative asset manifest and native PowerShell workflow are
documented in `docs/windows/sensevoice-runtime-validation.md`. This macOS host
cannot produce or execute the MSVC artifact, so native build, `dumpbin`,
Unicode-path, process cleanup, development and packaged transcription remain
explicit Windows acceptance blockers. Until that artifact exists, Windows
truthfully reports `runtime_missing`; it must never report fake readiness.

## Included and excluded

Included: SenseVoiceSmall INT8 only.

Not included: Whisper, Paraformer, Fun-ASR-Nano, Ollama, diarization, separate
VAD, realtime recording, automatic language switching, provider fallback,
release publishing, signing or notarization.

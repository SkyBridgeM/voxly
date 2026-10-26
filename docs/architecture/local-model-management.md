# Local recognition and model management

Voxly packages a native SenseVoice adapter with the Go desktop core. Model weights are installed separately. Both the verified files and the matching native worker are required before local recognition can start.

## Ownership and metadata

Go is authoritative for provider and model metadata. Shared provider fields are `execution_location`, `runtime_kind`, `requires_credentials`, `requires_external_service`, `model_source`, `supports_model_installation`, `supports_manual_import`, and the two privacy facts `sends_audio_off_device` and `sends_text_off_device`. Privacy copy is selected from those facts, covering local/local, local/cloud, cloud/local and cloud/cloud combinations.

The human-reviewable catalog is [`core-go/modelcatalog/models.v1.json`](../../core-go/modelcatalog/models.v1.json) and is embedded into the Go sidecar. Rust resolves the Voxly application-data directory, owns child-process lifetime, decodes typed JSON and returns it to React. React never chooses an installation destination, URL, expected hash or readiness result.

## Fixed SenseVoice identity

- Provider: `local-sensevoice`
- Model: `sensevoice-small-int8`
- Model revision: `2024-07-17`
- Runtime: packaged `sherpa-onnx` 1.13.8 adapter
- Official archive: `sherpa-onnx-sense-voice-zh-en-ja-ko-yue-int8-2024-07-17.tar.bz2`
- Archive size: `163002883` bytes
- Archive SHA-256: `7d1efa2138a65b0b488df37f8b89e3d91a60676e416f515b952358d83dfd347e`

The archive SHA-256 is a Voxly-computed and Voxly-pinned integrity checksum for that fixed official GitHub Release asset. It was independently rechecked by downloading the same pinned asset and comparing a second local SHA-256 calculation with GitHub's release-asset API digest. It verifies byte-for-byte integrity against the reviewed asset, but it is not an upstream digital signature and does not claim authenticity beyond matching that pinned archive.

The exact source URL is constructed only from the embedded catalog value: `https://github.com/k2-fsa/sherpa-onnx/releases/download/asr-models/sherpa-onnx-sense-voice-zh-en-ja-ko-yue-int8-2024-07-17.tar.bz2`. HTTPS redirects are bounded and restricted to `github.com` or GitHub-owned `*.githubusercontent.com` asset hosts; every downloaded byte must still match the fixed archive size and checksum. The catalog pins every accepted archive entry and every installed file by relative path, size and SHA-256. Production download and manual import are enabled because those identities were verified against the official k2-fsa GitHub release. There is no `latest` URL, built-in mirror, GitHub proxy, automatic download or download resume.

## Storage, receipt and atomicity

All managed data stays below the Tauri-provided Voxly application-data root:

```text
app_data_dir/
  model-catalog/installed.json
  models/sensevoice-small-int8/2024-07-17/{model.int8.onnx,tokens.txt,README.md,LICENSE,receipt.json}
  downloads/<random>.partial
  staging/<install-token>/
  locks/sensevoice-small-int8.lock/owner.json
```

The version directory is immutable after commit. Download/import copies to app-owned temporary storage, streams SHA-256 verification, validates an exact tar.bz2 allowlist, rejects traversal, absolute paths, duplicate/case-colliding names, links and special files, enforces entry and expanded-size limits, and writes only catalog-listed installed files. The receipt is written last in staging and the complete staging directory is atomically renamed to the final path. Failed operations clean temporary data and never make an incomplete final directory ready.

Receipt schema `1.0` stores model/catalog identity, archive identity, installed-file identities, installation time and only the source kind (`official_download` or `manual_import`). It never stores an import path, endpoint secret, credential, Task path or temporary path. Readiness verifies the receipt and actual files against the embedded catalog.

The per-model lock contains a random owner token, PID and creation time. A live process lock is never stolen; a dead owner is recovered only after the bounded stale interval. Unlock verifies the owner token. Download, import and deletion share this lock. Deletion first isolates the exact catalog-owned version directory by same-filesystem rename, rejects symlinks, removes only the quarantine path, and then updates the derived installed index. Task folders, manifests and user source archives are outside this boundary.

## Source and endpoint policy

The current SenseVoice package is hosted on GitHub Releases, so the Hugging Face endpoint setting does not rewrite or proxy this download. Settings presents this as a separate future-compatible network setting and explicitly says that neither the setting nor its connection test changes or tests the current SenseVoice source. The setting exists for future catalog entries whose fixed source host is `huggingface.co`. It accepts only an HTTPS origin with no userinfo, path, query or fragment, is normalized before persistence, and can replace only that origin; repository, revision and file path remain catalog-owned. Connection testing uses a bounded HEAD request and never downloads a model payload.

Manual import accepts one user-selected official archive, preserves the source file and never accepts a directory or user-selected destination. The source path is used only for the command invocation and is not persisted or returned to the normal UI.

## Readiness and privacy

Computed model states are `not_installed`, `downloading`, `verifying`, `installing`, `model_invalid`, `storage_unavailable`, `unsupported_platform`, `runtime_missing`, `ready`, and `error`. Model verification and runtime readiness are separate inputs to `StatusWithRuntime`. The production probe invokes the sibling `voxly-sensevoice-wrapper --version` and checks its protocol, runtime ID and version. Readiness still requires the catalog, receipt and installed-file checks. A missing or incompatible worker blocks local recognition; Voxly does not silently switch to MiMo. Transient progress comes from the sidecar and is not persisted.

Privacy state is provider-derived. Local SenseVoice requires no credential and does not send audio off-device. DeepSeek is cloud generation and sends transcript text off-device. The UI states that distinction and does not use a misleading “fully local” label when a cloud LLM is selected.

## VAD and packaging

The optional VAD path uses the bundled, hash-verified Silero model and the same native worker. Go records the original-audio sample ranges, language and model/parameter identity before transcription. The canonical transcript stays separate from timed results. Cache cleanup verifies those results before removing chunks; text-only cleanup also removes the task's own audio copy after explicit preview and confirmation.

The macOS installer includes the Go core, native worker, FFmpeg and Silero VAD. SenseVoice weights stay outside the installer and are downloaded or imported through model management. The source and third-party notice scripts pin build inputs; release attachments carry FFmpeg/LAME sources and build instructions.

There is no Ollama integration, streaming recognition, microphone capture or active speaker model. The reserved diarization interface does not enable CAM++. Its future implementation must finish and verify any speaker results before a user deletes the task audio.

Unicode normalization collisions remain limited by Go's standard-library handling. Runtime and cleanup paths must continue checking filesystem containment rather than trusting manifest paths.

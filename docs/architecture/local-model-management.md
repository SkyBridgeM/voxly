# Local provider and managed-model foundation

This document describes the PR #48 foundation. It does not claim that local transcription is implemented.

## Current architecture audit

| Area | Finding | Decision |
| --- | --- | --- |
| Go provider catalog and Task snapshots | Provider/model identity already has one Go-owned catalog and additive Task snapshots | Extend: add provider execution, credential, model-source, installation and privacy metadata without changing TaskManifest 2.0 |
| Go ASR execution | Chunk execution entered Mimo directly | Replace narrowly: route each chunk through a context-aware `ASRProvider`; preserve the Mimo adapter and make the local adapter return `runtime_not_implemented` |
| Rust provider transport | Typed transport exists, but selection/readiness assumed every ASR provider was credential-backed | Extend: accept the catalog-owned local identity and keep it blocked while the runtime is absent |
| Frontend provider UI | Catalog-driven selectors already exist | Reuse: render the additional provider and consume typed model state; do not duplicate model hashes or install rules |
| Model installation | No existing managed-model owner | Add: Go owns catalog validation, download/import verification, receipt, readiness, lock, install and deletion |
| Tauri process ownership | Async `spawn_blocking` and application-exit child cleanup already exist | Reuse: model mutations use the same single active-child ownership and cancellation boundary |
| LLM execution | DeepSeek is separately abstracted as a content generator | Defer: no Ollama discovery or calls in this PR; the shared provider metadata can represent a future external local service |

## Ownership and metadata

Go is authoritative for provider and model metadata. Shared provider fields are `execution_location`, `runtime_kind`, `requires_credentials`, `requires_external_service`, `model_source`, `supports_model_installation`, `supports_manual_import`, and the two privacy facts `sends_audio_off_device` and `sends_text_off_device`. Privacy copy is selected from those facts, covering local/local, local/cloud, cloud/local and cloud/cloud combinations.

The human-reviewable catalog is [`core-go/modelcatalog/models.v1.json`](../../core-go/modelcatalog/models.v1.json) and is embedded into the Go sidecar. Rust resolves the Voxly application-data directory, owns child-process lifetime, decodes typed JSON and returns it to React. React never chooses an installation destination, URL, expected hash or readiness result.

## Fixed SenseVoice identity

- Provider: `local-sensevoice`
- Model: `sensevoice-small-int8`
- Model revision: `2024-07-17`
- Runtime direction: `sherpa-onnx` (not included in this PR)
- Official archive: `sherpa-onnx-sense-voice-zh-en-ja-ko-yue-int8-2024-07-17.tar.bz2`
- Archive size: `163002883` bytes
- Archive SHA-256: `7d1efa2138a65b0b488df37f8b89e3d91a60676e416f515b952358d83dfd347e`

The catalog pins every accepted archive entry and every installed file by relative path, size and SHA-256. Production download and manual import are enabled because those identities were verified against the official k2-fsa GitHub release. There is no `latest` URL, built-in mirror, GitHub proxy, automatic download or download resume.

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

The current SenseVoice package is hosted on GitHub Releases, so the Hugging Face endpoint setting does not rewrite or proxy this download. The setting exists for future catalog entries whose fixed source host is `huggingface.co`. It accepts only an HTTPS origin with no userinfo, path, query or fragment, is normalized before persistence, and can replace only that origin; repository, revision and file path remain catalog-owned. Connection testing uses a bounded HEAD request and never downloads a model payload.

Manual import accepts one user-selected official archive, preserves the source file and never accepts a directory or user-selected destination. The source path is used only for the command invocation and is not persisted or returned to the normal UI.

## Readiness and privacy

Computed model states are `not_installed`, `downloading`, `verifying`, `installing`, `model_invalid`, `storage_unavailable`, `unsupported_platform`, `runtime_missing`, `ready`, and `error`; the type vocabulary reserves future external-service states without exposing them in the current UI. Transient progress is emitted by the sidecar and is not persisted. In this PR a valid installed model is always `runtime_missing`, never `ready`. New Task therefore cannot start local ASR and does not create a Task or fall back to Mimo.

Privacy state is provider-derived. Local SenseVoice requires no credential and does not send audio off-device. DeepSeek is cloud generation and sends transcript text off-device. The UI states that distinction and does not use a misleading “fully local” label when a cloud LLM is selected.

## Current limitations and next stage

PR #49 must add the Voxly-owned sherpa-onnx runtime and real SenseVoice adapter, then change valid model-plus-runtime readiness from `runtime_missing` to `ready`. It must preserve this catalog/receipt boundary and snapshot actual runtime/model identity when a real local Task is created. This PR includes no model executable, inference, Whisper, Ollama, VAD, recording, packaging or release changes.

Unicode normalization collisions are rejected where exact byte and case-fold identity detects them; Go's standard library does not provide cross-platform Unicode normalization. Future open/runtime code must continue canonical containment checks at its own filesystem boundary.

# Voxly Go core

The Go core owns transcription, task state, resume, content generation, model
management and storage cleanup. Tauri packages it as `meeting-asr-go`; the CLI is
also useful for development and checking the desktop's contracts.

## Build and run

Use Go 1.21 or newer. Cloud transcription needs MiMo credentials; local
transcription needs the verified SenseVoice model and native adapter. DeepSeek
credentials are needed only for content generation.

```bash
go build -C core-go -o meeting-asr-go .
./core-go/meeting-asr-go list-providers
```

From the repository root, `./scripts/build_desktop_sidecar.sh` prepares the
packaged Go binary, native adapter and FFmpeg. Production uses sibling
executables. Standalone development can use FFmpeg from PATH; the packaged
runtime never falls back to a host FFmpeg if its own copy is missing.

The desktop does not record microphone audio. The Python implementation under
`src/` is a separate legacy path.

## Transcription

Import M4A, MP3 or WAV files. Local recognition normalizes audio to 16 kHz mono
PCM16. With VAD enabled, decoding happens once before detection and sample-based
clipping; the resulting WAV chunks go directly to SenseVoice.

```bash
# Local Chinese transcription with VAD. Use absolute paths for storage roots.
./core-go/meeting-asr-go transcribe /path/to/interview.m4a \
  --asr-provider local-sensevoice --asr-model sensevoice-small-int8 \
  --local-asr-language zh --vad=true \
  --app-data-root /path/to/voxly-app-data \
  --output-mode transcript_only --output-dir /path/to/tasks

# Cloud transcription, followed by a template-based generation.
./core-go/meeting-asr-go transcribe /path/to/meeting.mp3 \
  --output-mode transcript_and_generate --template-id structured-notes

# Continue with the task's saved processing choices.
./core-go/meeting-asr-go resume /path/to/tasks/task-folder
```

Chinese (`zh`) is the local default; `auto` enables language detection. VAD is
local-only and defaults off. Its plan records model identity, parameters,
language, source hash and sample ranges before ASR starts. Resume keeps that
plan and reuses verified completed text checkpoints. Cancellation exits with
code 130; the desktop's stdin control channel also stops work if its owner exits.

## Task files

```text
task-folder/
  task_manifest.json
  audio/                         imported source copy
  chunks/                        regenerable audio segments
  transcripts/
    part_001.txt                 text checkpoints
    transcript-timed.md          VAD speech intervals and text
    vad-segments.json            structured VAD results
  transcript.md                  canonical transcript
  generated/<template>/<id>.md    versioned content
```

`transcript_only` produces the canonical transcript. `transcript_and_generate`
also creates a content generation. New tasks do not create legacy
`notes_input.md` or `meeting_minutes.md`; the old mode names remain CLI aliases.

VAD times refer to original-audio sample positions. They do not identify
individual words or speakers. `diarization.go` reserves a future speaker-model
boundary; no implementation is registered or called.

## Generate from saved text

```bash
./core-go/meeting-asr-go list-templates
./core-go/meeting-asr-go generate-content \
  --task-dir /path/to/task --template-id action-items
./core-go/meeting-asr-go retry-generation \
  --task-dir /path/to/task --generation-id existing-generation-id
```

A new generation preserves earlier versions. Generation errors leave the
transcript available. Generation can continue after the user removes task audio
with Keep text only, since it reads the canonical text.

## Manage the local model

```bash
./core-go/meeting-asr-go model-catalog
./core-go/meeting-asr-go model-status \
  --app-data-root /path/to/voxly-app-data --model-id sensevoice-small-int8
./core-go/meeting-asr-go download-model \
  --app-data-root /path/to/voxly-app-data --model-id sensevoice-small-int8
```

Import and deletion use the same managed-model lock and containment rules as
download. The catalog pins archive and installed-file identities. Model files
alone do not establish readiness: the sibling native worker must also report
the expected sherpa-onnx runtime identity. See [model management](../docs/architecture/local-model-management.md).

## Storage cleanup

```bash
./core-go/meeting-asr-go preview-task-cache \
  --output-root /path/to/tasks --task-folder task-folder
./core-go/meeting-asr-go clean-task-cache \
  --output-root /path/to/tasks --task-folder task-folder \
  --expected-token PREVIEW_TOKEN
```

The default scope removes only known audio chunks. Add `--scope text_only` to
both commands to include the task's own source recording. Tokens include the
scope and file identities; a chunk-only preview cannot authorize source deletion.
The task must be fully completed for text-only cleanup. Text and VAD timing
artifacts must verify, and external imported files are never deletion targets.

Cleanup records intent before unlinking files. Interrupted cleanup needs a new
preview and can be retried. A missing source is accepted only when the manifest
records its intentional removal. Original-audio playback or retranscription then
requires another source copy. Automatic and batch cleanup remain chunk-only.

VAD's temporary `.vad-work/audio.wav` is reclaimed on normal exit or the next
resume after a crash. Unknown temporary files block deletion. Model temporary
cleanup is separate and never sweeps task folders.

## Configuration and checks

Standalone CLI configuration searches upward from the current working directory
for `.env`. `--env-file` selects an explicit file, and `--project-root` changes
the search root. Production Tauri passes explicit runtime paths and resolves
credentials at the Rust boundary; it does not use this development lookup.

```bash
cd core-go
go test ./...
go vet ./...
```

Manifest writes are atomic. Preserve task containment, file hashes, locks and
unknown manifest fields when extending a workflow. Generated content remains
separate from canonical transcripts.

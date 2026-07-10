# Phase 1: Go Core CLI Parity

Audience: opencode / implementation agent  
Scope: `core-go` only  
Do not edit: `desktop/`, `src/meeting_asr/`, `webui/`, packaging files, or frontend UI files unless explicitly asked later.

## Objective

Make `core-go` reliable enough to serve as the future backend for imported audio transcription.

This phase should not integrate Tauri yet. It should only improve and verify the Go CLI.

## Baseline

The Python workflow remains the behavior reference.

The Go CLI already supports:

- `transcribe <audio_path>`
- MiMo ASR for `.mp3`, `.wav`, `.m4a`
- chunking via external `ffmpeg`
- output modes: `transcript`, `notes`, `minutes`
- `.env` loading
- stable stdout lines for future Tauri parsing

Known gaps for this phase:

- no `resume`
- incomplete prompt/template CLI configurability
- no task manifest
- some behavior still needs closer parity with Python

## Tasks

### 1. Add `resume <task_dir>`

Implement:

```bash
meeting-asr-go resume <task_dir> [flags]
```

Expected behavior:

- Resolve `task_dir` to an absolute path.
- Locate chunks in `task_dir/chunks`.
- Accept chunk files named:
  - `part_001.wav`
  - `part_002.wav`
  - `part_001.mp3`
  - etc.
- Reject invalid names.
- Reject mixed `.mp3` and `.wav` chunks in `chunks/`.
- Require contiguous numbering starting at `part_001`.
- Use existing non-empty transcript part files in `task_dir/transcripts` as completed work.
- Transcribe missing chunk transcript files only.
- Merge transcripts into `task_dir/transcript.md`.
- Generate `notes_input.md` and/or `meeting_minutes.md` according to `--output-mode`.

Do not re-split audio during resume.

### 2. Keep output structure compatible

Preserve:

```text
task_dir/
  audio/
  chunks/
  transcripts/
  transcript.md
  notes_input.md
  meeting_minutes.md
```

Preserve transcript part ordering and merge behavior.

Final stdout paths should be absolute paths where practical:

```text
Transcript saved: /abs/path/transcript.md
Notes input saved: /abs/path/notes_input.md
Meeting minutes saved: /abs/path/meeting_minutes.md
Task folder: /abs/path/task_dir
```

### 3. Add prompt/template flags

Support:

```bash
--prompt-path <path>
--obsidian-template-path <path>
```

Rules:

- CLI flags override `.env`.
- `.env` values override defaults.
- Relative paths from `.env` should resolve relative to the `.env` file directory.
- Relative paths from CLI may resolve relative to the current working directory unless there is an existing project-root convention.
- Do not hardcode prompt text in Go logic.

### 4. Add minimal `task_manifest.json`

Write a simple manifest into each task directory.

Suggested fields:

```json
{
  "task_id": "...",
  "source_audio": "...",
  "output_mode": "transcript",
  "chunk_minutes": 5,
  "max_upload_mb": 10,
  "asr_provider": "mimo",
  "asr_model": "mimo-v2.5-asr",
  "created_at": "...",
  "updated_at": "...",
  "status": "completed",
  "total_chunks": 0,
  "completed_chunks": 0,
  "transcript_path": "",
  "notes_input_path": "",
  "minutes_path": "",
  "error": null
}
```

Keep this lightweight. Do not add a database.

Status guidance:

- `running` when work begins.
- `completed` when all outputs are written.
- `failed` when the workflow exits with an error.

On failure, keep already generated chunk transcripts and task files.

### 5. Improve failure and resume behavior

Requirements:

- A failed chunk should not delete completed chunk transcripts.
- Error messages should include the chunk filename where relevant.
- `resume` should be safe to run multiple times.
- Existing completed transcript parts should be skipped.

### 6. Tests

Add or update Go tests for:

- Resume chunk discovery.
- Invalid chunk names.
- Missing chunk numbers.
- Mixed chunk formats.
- Skipping completed transcript parts.
- Prompt/template path resolution.
- Manifest status update on success/failure if practical.

Do not add network-dependent tests. Mock or isolate MiMo calls where needed.

## Acceptance Criteria

- `cd core-go && go test ./...` passes.
- `meeting-asr-go transcribe` still works as before.
- `meeting-asr-go resume <task_dir>` works on a task directory with existing chunks.
- Resume skips existing non-empty transcript part files.
- Final transcript merge order is deterministic.
- No API key is hardcoded.
- No prompt text is hardcoded.
- No files outside `core-go` are changed unless there is a strong reason documented in the final response.

## Out of Scope

Do not do these in Phase 1:

- Tauri integration.
- Desktop UI changes.
- App packaging.
- Bundling `ffmpeg`.
- Recording workflow.
- Meetily fork.
- Wails migration.
- Local Whisper/Parakeet support.
- Go HTTP server.

## Suggested Review Checklist

For human review after implementation:

- Compare Go CLI output directory against Python output directory shape.
- Confirm resume does not retranscribe completed chunks.
- Confirm `go test ./...` result.
- Inspect whether the implementation stayed inside `core-go`.
- Check that errors are actionable and mention relevant paths/chunks.


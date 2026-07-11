# Go Core Backend Next Execution Plan

Audience: opencode / implementation agent
Project: `meet-asr` experimental Go core branch
Goal: turn the current Go spike into the practical backend for the local MiMo meeting transcription desktop tool.

## Executive Direction

Continue the current project. Do not fork Meetily for the next phase.

The next milestone should be:

1. Bring `core-go` to feature parity with the stable Python workflow.
2. Replace the Tauri wrapper's dependency on `uv run meeting-asr` with the Go binary.
3. Package the app with the Go binary and an `ffmpeg` strategy.

Do not start a broad UI rewrite, Wails migration, Meetily fork, or local model integration yet.

## Current Baseline

The Python workflow is still the behavior baseline.

Important existing Python behavior:

- Input audio import/copy into a task directory.
- Audio chunking via `ffmpeg`.
- Upload-size-aware chunk retry by shortening chunk duration.
- MiMo ASR HTTP call using base64 `input_audio`.
- Per-chunk retry.
- Resume by skipping existing non-empty transcript part files.
- Ordered transcript merge.
- Optional `notes_input.md` generation from external prompt template.
- Optional `meeting_minutes.md` generation via DeepSeek.
- User-selected output directory.

The current Go spike already has:

- MiMo ASR transcription for `.mp3`, `.wav`, `.m4a`.
- Chunking via external `ffmpeg`.
- Transcript / notes / minutes modes.
- Stable stdout lines intended for future Tauri integration.
- `.env` loading.
- Tests around several core pieces.

Known Go gaps:

- No recording workflow.
- No Tauri integration.
- No packaging.
- No bundled `ffmpeg`.
- Flat spike structure may become harder to maintain if expanded too much.

## Recommended Target Architecture

For the next phase, use:

```text
Tauri UI
  -> invokes bundled Go CLI as sidecar/process
    -> Go core workflow
      -> ffmpeg chunking
      -> MiMo ASR provider
      -> transcript merge
      -> notes/minutes generation
      -> task directory output
```

Keep the Go core as a CLI first, not a long-running HTTP service.

The CLI boundary is enough for the current workflow:

```bash
meeting-asr-go transcribe <audio_path> \
  --output-dir <dir> \
  --output-mode transcript|notes|minutes \
  --chunk-minutes <n> \
  --max-upload-mb <n> \
  --prompt-path <path> \
  --obsidian-template-path <path> \
  --env-file <path>

meeting-asr-go resume <task_dir> \
  --output-mode transcript|notes|minutes \
  --max-upload-mb <n> \
  --prompt-path <path> \
  --obsidian-template-path <path> \
  --env-file <path>
```

Use stdout event lines as the integration contract for Tauri:

```text
Status: Splitting audio
Status: Transcribing 12 chunk(s)
Progress: 1/12
Chunk skipped: part_001.wav
Chunk completed: part_002.wav
Status: Merging transcript
Transcript saved: /path/to/transcript.md
Notes input saved: /path/to/notes_input.md
Meeting minutes saved: /path/to/meeting_minutes.md
Task folder: /path/to/task
Status: Done
```

## Milestone 1: Go Core Parity With Python

Objective: make `core-go` safe to treat as the new backend for file/import workflows.

Tasks:

1. Add resume workflow.
   - Add a `resume <task_dir>` command.
   - Locate chunks in `task_dir/chunks`.
   - Validate chunk names: `part_001.wav`, `part_002.wav`, etc.
   - Require contiguous numbering starting at `part_001`.
   - Skip completed transcript parts by checking non-empty files in `task_dir/transcripts`.
   - Re-run merge and notes/minutes generation after transcription.

2. Match Python task directory behavior.
   - Preserve folder layout:
     - `audio/`
     - `chunks/`
     - `transcripts/`
     - `transcript.md`
     - `notes_input.md`
     - `meeting_minutes.md`
   - Preserve transcript part naming and merge order.
   - Make output paths absolute in final stdout lines.

3. Add or complete prompt path flags.
   - `--prompt-path`
   - `--obsidian-template-path`
   - Resolve relative paths consistently from `.env` location or explicit project root.
   - Do not hardcode meeting-notes prompt content.

4. Add a minimal manifest file.
   - Write `task_manifest.json` into each task directory.
   - Include:
     - source audio path/name
     - output mode
     - chunk duration
     - max upload MB
     - ASR provider/model
     - created time
     - status
     - error if failed
   - Keep this simple; do not build a database.

5. Improve error behavior.
   - Failed chunk should leave already completed chunk transcripts intact.
   - Error messages should mention the failed chunk name.
   - Do not delete task folders on transcription failure.

6. Tests.
   - Add tests for resume chunk discovery.
   - Add tests for missing chunk numbers.
   - Add tests for skipping completed transcript parts.
   - Add tests for prompt path resolution.
   - Add tests for final stdout line parsing if there is a parser helper.

Acceptance criteria:

- `go test ./...` passes in `core-go`.
- A failed run can be resumed without retranscribing completed chunks.
- `transcribe` and `resume` produce the same final `transcript.md` when all chunks are present.
- No API key is hardcoded.
- No prompt text is hardcoded into Go logic.

## Milestone 2: Tauri Uses Go Binary Instead of uv

Objective: remove the desktop app's runtime dependency on Python and `uv`.

Tasks:

1. Build the Go binary.
   - Create a reproducible command for building `core-go`.
   - Output binary into a location Tauri can invoke during development.
   - Keep naming clear, for example `meeting-asr-go`.

2. Update Tauri command execution.
   - Replace `Command::new("uv") ... uv run meeting-asr` with invoking the Go binary.
   - Keep the existing progress/log event model if possible.
   - Parse the same stable stdout lines:
     - `Status:`
     - `Progress:`
     - `Transcript saved:`
     - `Notes input saved:`
     - `Meeting minutes saved:`
     - `Task folder:`

3. Support user-selected output directory in the UI path.
   - If the current UI does not expose output directory selection, add the minimal control needed.
   - Pass `--output-dir` to Go.
   - Do not assume project-root `output/`.

4. Key handling.
   - For this milestone, passing keys through environment variables is acceptable.
   - Do not write keys into code.
   - If saving keys, keep the existing allowlist behavior or move to app config later.

5. Resume command from UI.
   - Add a minimal “resume task folder” path only if it is straightforward.
   - If UI work becomes too large, keep resume as CLI-only for this milestone and document it.

Acceptance criteria:

- Desktop transcription works without `uv`.
- Desktop app can run transcript mode through the Go binary.
- Minutes mode still works when DeepSeek key is provided.
- Progress and final result paths display in the UI.
- Opening output folder works for user-selected output dirs, not only project `output/`.

## Milestone 3: Packaging Strategy

Objective: make the app installable/runnable without a developer environment.

Tasks:

1. Bundle Go binary as a Tauri sidecar or bundled binary.
   - Use Tauri's supported binary/resource mechanism.
   - Ensure macOS binary naming and permissions are correct.
   - Confirm production bundle can locate the binary.

2. Decide `ffmpeg` strategy.
   - Preferred for personal tool: bundle `ffmpeg` if licensing/distribution is acceptable for the user's target.
   - Alternative: detect external `ffmpeg` and show a clear installation message.
   - Do not silently assume `ffmpeg` is on PATH in packaged app.

3. App data/config paths.
   - Stop depending on repository root in production.
   - Use app config/data directories for saved settings.
   - Let output directory remain user-selected.

4. Build verification.
   - Build a local desktop bundle.
   - Test on a small audio file.
   - Test with output directory outside the repo.
   - Test missing `ffmpeg` behavior if not bundled.

Acceptance criteria:

- Packaged app starts without Python, uv, or repo checkout.
- Packaged app can transcribe a small audio file.
- Packaged app writes outputs to the selected directory.
- Error message is clear if `ffmpeg` is unavailable.

## Later Milestones, Not Now

Do not include these in the immediate implementation unless explicitly requested:

- Full recording rewrite in Go.
- Local Whisper/Parakeet model support.
- Meetily fork.
- Go HTTP server sidecar.
- Database-backed meeting library.
- Auto-updater.
- Wails migration.
- Large design/UI overhaul.

## Architecture Notes for Future Local ASR

When the MiMo workflow is stable, introduce a small provider boundary in Go:

```go
type ASRProvider interface {
    Name() string
    TranscribeChunk(ctx context.Context, chunkPath string) (string, error)
}
```

Keep these outside the provider:

- task directory creation
- chunking
- upload size checks
- resume behavior
- progress reporting
- transcript merge
- notes/minutes generation

This makes MiMo cloud ASR and future local ASR share orchestration without forcing local model complexity into the first usable release.

## Practical Do Not Do List

- Do not fork Meetily for the next milestone.
- Do not introduce Go as a sidecar inside Meetily.
- Do not build an HTTP service before the CLI boundary is proven insufficient.
- Do not rewrite the frontend before replacing `uv`.
- Do not remove the Python workflow until Go parity is verified.
- Do not hardcode API keys.
- Do not hardcode meeting-notes prompt content.
- Do not assume output must live under the repo.
- Do not make local model support part of the MiMo packaging milestone.
- Do not broaden the scope into a full meeting knowledge base yet.

## Suggested Implementation Order

1. `core-go`: implement `resume`.
2. `core-go`: add prompt/template flags and manifest.
3. `core-go`: run and expand tests.
4. Tauri: invoke Go binary in development.
5. Tauri: pass output directory and parse stdout.
6. Tauri: make opening output folders work outside repo output.
7. Packaging: bundle Go binary.
8. Packaging: decide and implement `ffmpeg` handling.
9. Manual verification with small audio file.
10. Only then consider deprecating Python as the desktop backend.

## Definition of Done for This Phase

This phase is done when:

- The Go backend can replace Python for imported audio transcription.
- Resume works from the CLI.
- Tauri no longer requires `uv`.
- A packaged desktop app can run on the developer machine.
- MiMo API key, output directory, prompt template, and task outputs are configurable.
- The old Python path remains available as a fallback until the Go path is proven stable.

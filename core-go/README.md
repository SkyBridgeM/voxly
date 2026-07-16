# core-go (Experimental Spike)

**Status:** Experimental — not for production use.

This is the actively developed processing runtime used by the desktop app.

## What it does

- Transcribes audio files (`.mp3`, `.wav`, `.m4a`) using MiMo ASR.
- Splits large audio into chunks via external `ffmpeg`.
- Supports Processing Model V2:
  - `transcript_only` — generates the canonical `transcript.md` only.
  - `transcript_and_generate` — generates the canonical transcript and one versioned built-in-template artifact under `generated/<template-id>/<generation-id>.md`.
- `transcript`, `notes`, and `minutes` remain deprecated CLI aliases for compatibility; new tasks do not create `notes_input.md` or `meeting_minutes.md`.
- **Resume** — `resume <task_dir>` continues a partially-completed task, skipping already-transcribed chunks. It inherits the manifest's original output mode unless `--output-mode` is explicitly provided.
- Applies Obsidian templates if configured.
- Stable stdout lines for future Tauri integration.

## What it does NOT do

- **No recording** — cannot record from microphone.
- **No ffmpeg bundling** — requires external `ffmpeg` on PATH.

The Python runtime is a deprecated development-only fallback and does not implement Processing Model V2.

## Prerequisites

- Go 1.21+
- `ffmpeg` installed and on PATH
- `.env` file at the project root with `MIMO_API_KEY` set

## Build

```bash
cd core-go
go build -o meeting-asr-go .
```

## Usage

```bash
# Transcript only
./meeting-asr-go transcribe ../input/test.mp3 --output-mode transcript_only

# Transcript plus generated structured notes
./meeting-asr-go transcribe ../input/test.mp3 --output-mode transcript_and_generate --template-id structured-notes

# Generate another version from an existing completed transcript, without ASR
./meeting-asr-go generate-content --task-dir /absolute/path/to/task --template-id action-items

# Inspect the non-secret built-in provider catalog
./meeting-asr-go list-providers

# Resume a partially-completed task (skips already-transcribed chunks)
./meeting-asr-go resume ../output/20260709_123456_meeting

# Flags work before or after the audio path:
./meeting-asr-go transcribe --chunk-minutes 3 ../input/test.mp3 --output-dir ./out
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--output-mode` | `transcript` alias | Output mode: `transcript_only` or `transcript_and_generate` |
| `--template-id` | `structured-notes` | Built-in template ID for generated content |
| `--asr-provider` | `mimo` | Speech-recognition provider ID |
| `--asr-model` | `mimo-v2.5-asr` | Speech-recognition model ID |
| `--llm-provider` | `deepseek` | Generated-content provider ID |
| `--llm-model` | `deepseek-v4-flash` | Generated-content model ID |
| `--output-dir` | `output` | Base output directory |
| `--chunk-minutes` | 0 (use .env or 5) | Chunk length in minutes |
| `--max-upload-mb` | 0 (use .env or 10) | Max upload size in MB |
| `--env-file` | (auto) | Explicit path to `.env` file |
| `--project-root` | (cwd) | Explicit project root for `.env` lookup |
| `--prompt-path` | (from .env) | Path to prompt template |
| `--obsidian-template-path` | (from .env) | Path to Obsidian template |

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `MIMO_API_KEY` | (required) | MiMo ASR API key |
| `MIMO_ASR_ENDPOINT` | `https://api.xiaomimimo.com/v1/chat/completions` | MiMo ASR endpoint |
| `MIMO_ASR_MODEL` | `mimo-v2.5-asr` | MiMo ASR model |
| `MAX_UPLOAD_MB` | `10` | Max upload size per chunk |
| `DEFAULT_CHUNK_MINUTES` | `5` | Default chunk length |
| `MIMO_RETRY_COUNT` | `3` | MiMo API retry count |
| `MIMO_TIMEOUT_SECONDS` | `120` | MiMo API timeout |
| `MINUTES_PROVIDER` | `deepseek` | Deprecated configuration name for the active content-generation provider |
| `DEEPSEEK_API_KEY` | (required for generation) | DeepSeek API key |
| `DEEPSEEK_ENDPOINT` | `https://api.deepseek.com/chat/completions` | DeepSeek endpoint |
| `DEEPSEEK_MODEL` | `deepseek-v4-flash` | DeepSeek model |
| `LLM_TIMEOUT_SECONDS` | `120` | LLM API timeout |
| `MINUTES_PROMPT_PATH` | `prompts/default_minutes.md` | Prompt template for notes |
| `OBSIDIAN_TEMPLATE_PATH` | `prompts/obsidian_minutes.md` | Obsidian template for minutes |

## Run tests

```bash
cd core-go
go test ./...
```

## Design decisions

- The explicit built-in registry currently contains only Mimo / `mimo-v2.5-asr` for ASR and DeepSeek / `deepseek-v4-flash` for generated content. Provider/model selection is non-secret; credentials remain in `MIMO_API_KEY` and `DEEPSEEK_API_KEY` and never enter descriptors, desktop settings, or TaskManifest.

- Flat package structure (single `main` package) — appropriate for a spike.
- One platform dependency: `golang.org/x/sys` for native Windows locking and durable file replacement.
- Matches Python behavior: task directory layout, chunk naming, transcript format, stdout output lines.
- Audio format handling: mp3/wav sent directly; m4a converted to 16kHz mono wav via ffmpeg.
- Flags can appear before or after the audio path (args are reordered before parsing).

## `.env` Lookup Behavior

The Go CLI loads configuration from a `.env` file using this priority:

1. `--env-file <path>` — explicit file path (highest priority).
2. `--project-root <path>` — search upward from this directory.
3. Current working directory (`os.Getwd()`) — search upward (default).

**Known limitation for future desktop integration:** If Tauri or another launcher runs the binary from a different working directory, the default cwd-based search may not find `.env`. Use `--env-file` or `--project-root` to specify the location explicitly. This will be revisited when Tauri integration is scoped.

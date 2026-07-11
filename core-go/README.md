# core-go (Experimental Spike)

**Status:** Experimental — not for production use.

This is a proof-of-concept Go reimplementation of the meeting-asr Python core.
The goal is to validate whether the Python core can be replaced by a small Go CLI binary for easier desktop packaging.

## What it does

- Transcribes audio files (`.mp3`, `.wav`, `.m4a`) using MiMo ASR.
- Splits large audio into chunks via external `ffmpeg`.
- Supports three output modes:
  - `transcript` — generates `transcript.md` only.
  - `notes` — generates `transcript.md` + `notes_input.md`.
  - `minutes` — generates `transcript.md` + `notes_input.md` + `meeting_minutes.md` (via DeepSeek).
- **Resume** — `resume <task_dir>` continues a partially-completed task, skipping already-transcribed chunks. It inherits the manifest's original output mode unless `--output-mode` is explicitly provided.
- Applies Obsidian templates if configured.
- Stable stdout lines for future Tauri integration.

## What it does NOT do

- **No recording** — cannot record from microphone.
- **No Tauri integration** — not wired to the desktop app.
- **No packaging** — no installers, bundles, or auto-update.
- **No ffmpeg bundling** — requires external `ffmpeg` on PATH.

**Python remains the stable baseline.** This spike is for validation only.
The Python CLI (`meeting-asr`), Streamlit WebUI, and Tauri desktop app are unaffected.

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
# Transcript only (default)
./meeting-asr-go transcribe ../input/test.mp3

# Notes mode: transcript + notes_input.md
./meeting-asr-go transcribe ../input/test.mp3 --output-mode notes

# Minutes mode: transcript + notes_input.md + meeting_minutes.md (requires DEEPSEEK_API_KEY)
./meeting-asr-go transcribe ../input/test.mp3 --output-mode minutes

# Resume a partially-completed task (skips already-transcribed chunks)
./meeting-asr-go resume ../output/20260709_123456_meeting

# Flags work before or after the audio path:
./meeting-asr-go transcribe --chunk-minutes 3 ../input/test.mp3 --output-dir ./out
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--output-mode` | `transcript` | Output mode: `transcript`, `notes`, or `minutes` |
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
| `MINUTES_PROVIDER` | `deepseek` | LLM provider for minutes mode |
| `DEEPSEEK_API_KEY` | (required for minutes) | DeepSeek API key |
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

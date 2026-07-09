# core-go (Experimental Spike)

**Status:** Experimental — not for production use.

This is a proof-of-concept Go reimplementation of the meeting-asr Python core.
The goal is to validate whether the Python core can be replaced by a small Go CLI binary for easier desktop packaging.

## What it does

- Transcribes audio files (`.mp3`, `.wav`, `.m4a`) using MiMo ASR.
- Splits large audio into chunks via external `ffmpeg`.
- Outputs `transcript.md` in the same format as the Python implementation.

## What it does NOT do

- No DeepSeek meeting minutes generation.
- No recording, resume, or Obsidian templates.
- No Tauri integration or packaging.
- No LAN server, database, or task queue.

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
./meeting-asr-go transcribe ../input/test.mp3
./meeting-asr-go transcribe ../input/test.mp3 --chunk-minutes 3 --output-dir ./out
# Flags work before or after the audio path:
./meeting-asr-go transcribe --chunk-minutes 3 ../input/test.mp3 --output-dir ./out
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--output-mode` | `transcript` | Output mode (only `transcript` in spike) |
| `--output-dir` | `output` | Base output directory |
| `--chunk-minutes` | 0 (use .env or 5) | Chunk length in minutes |
| `--max-upload-mb` | 0 (use .env or 10) | Max upload size in MB |
| `--env-file` | (auto) | Explicit path to `.env` file |
| `--project-root` | (cwd) | Explicit project root for `.env` lookup |

## Run tests

```bash
cd core-go
go test ./...
```

## Design decisions

- Flat package structure (single `main` package) — appropriate for a spike.
- No external Go dependencies — standard library only.
- Matches Python behavior: task directory layout, chunk naming, transcript format, stdout output lines.
- Audio format handling: mp3/wav sent directly; m4a converted to 16kHz mono wav via ffmpeg.
- Flags can appear before or after the audio path (args are reordered before parsing).

## `.env` Lookup Behavior

The Go CLI loads configuration from a `.env` file using this priority:

1. `--env-file <path>` — explicit file path (highest priority).
2. `--project-root <path>` — search upward from this directory.
3. Current working directory (`os.Getwd()`) — search upward (default).

**Known limitation for future desktop integration:** If Tauri or another launcher runs the binary from a different working directory, the default cwd-based search may not find `.env`. Use `--env-file` or `--project-root` to specify the location explicitly. This will be revisited when Tauri integration is scoped.

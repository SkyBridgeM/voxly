# meeting-asr Desktop Prototype

> **EXPERIMENTAL** — Python is the stable default backend; Go is available only
> as an explicitly experimental development backend.

## What This Is

A thin Tauri + React + TypeScript desktop wrapper that calls either Python via
`uv run meeting-asr` or the experimental Go CLI. Rust validates paths, streams
the stable stdout contract, and validates final artifacts before reporting success.

## Architecture

```
desktop/ (Tauri + React)
  ├── Python (default): uv run meeting-asr transcribe ...
  └── Go (experimental): core-go/meeting-asr-go transcribe ...
```

## Prerequisites

| Requirement | Why |
|---|---|
| **Node.js + npm** | Frontend build and Tauri CLI |
| **Rust** (rustc, cargo) | Tauri Rust backend |
| **[uv](https://docs.astral.sh/uv/)** | Python backend only |
| **Go 1.21+** | Build the experimental Go backend |
| **ffmpeg** | Audio splitting (required by both backends; not bundled) |
| **`.env` at project root** | API keys |

## Setup

```bash
# 1. Install uv (if not installed)
curl -LsSf https://astral.sh/uv/install.sh | sh

# 2. Install Python dependencies
uv sync --all-extras

# 3. Install frontend dependencies
cd desktop
npm install

# 4. Configure API keys in project-root .env
#    transcript mode needs: MIMO_API_KEY
#    notes/minutes need:    MIMO_API_KEY
#    minutes additionally:  DEEPSEEK_API_KEY
```

### Go backend (experimental)

```bash
cd core-go
go build -o meeting-asr-go .
```

Rust discovers Go through the absolute `MEETING_ASR_GO_BINARY` override or the
development fallback `<project-root>/core-go/meeting-asr-go` (`.exe` on Windows).
It validates `<binary> version`; it does not search PATH, auto-build, package, or
bundle the binary.

## Running

```bash
cd desktop
npm run tauri dev
```

## Development vs. Planned Production Candidate

This repository currently runs only the development desktop build described
above. It requires the project checkout, `uv` for Python, a root `.env`, and a
manually built Go binary when Go is selected.

Phase 5 will later introduce a separate internal macOS Apple Silicon production
candidate. That build is planned to default to a packaged Go sidecar, use a
user-visible output directory under `~/Documents/Meeting ASR`, and keep Python
available only in development builds. Python source code and regression tests
will remain in this repository.

This readiness change does **not** add a sidecar, bundle an application, change
backend selection, package ffmpeg, or change API-key storage. ffmpeg will remain
a system prerequisite; packaging, signing, notarization, installers, and
automatic updates are deferred.

## How It Works

1. User selects a `.mp3`, `.wav`, or `.m4a` file via the Tauri file dialog.
2. User chooses `transcript`, `notes`, or `minutes`.
3. In Settings, Python remains the default; Go is clearly labeled experimental.
4. On "Start", the app runs the selected backend with structured arguments.
   Python keeps its normal root `.env` lookup. Go receives `--env-file` for the
   root `.env` when present, otherwise `--project-root`.
5. stdout/stderr stream in real time to the UI.
6. On completion, canonical task/artifact paths must validate inside root `output/`.
7. "Open Output Folder" opens only a validated task directory.

## API Key Handling

- API keys are **never** embedded in source code, logs, or CLI arguments.
- The Python CLI reads keys from the project-root `.env` file (preferred).
- Temporary keys can be entered in the Settings panel for the current session only.
  They are passed as environment variables to the CLI process.
- Keys can be saved to `.env` from the Settings panel ("Save to .env").
  Rust writes `.env` directly — keys never appear in process argv.
- If both `.env` and Settings are empty, the Python CLI will report the missing key error.

## Privacy Warnings

- **transcript mode**: Sends audio to MiMo ASR for transcription.
- **notes mode**: Sends audio to MiMo ASR and creates `notes_input.md` locally;
  no transcript is sent to DeepSeek.
- **minutes mode**: Sends transcript text to DeepSeek for summary generation.
- Do not upload unauthorized sensitive meeting content.

## Tauri Permissions

The app only requests:
- `core:default` — Basic Tauri operations
- `dialog:default` — File picker dialog

No filesystem, shell, network, or opener permissions are granted.

## Common Errors

| Error | Cause | Fix |
|---|---|---|
| `uv is not installed` | uv not in PATH | Install uv and restart |
| `No module named meeting_asr` | Dependencies not installed | Run `uv sync --all-extras` |
| `ffmpeg not found` | ffmpeg not installed | Install ffmpeg separately |
| `.env` missing | No API keys configured | Create `.env` with required keys |
| `MIMO_API_KEY not set` | Key missing in `.env` | Add key to `.env` |
| `DEEPSEEK_API_KEY not set` | Key missing for minutes mode | Add key to `.env` |

## Security Notes

- No arbitrary shell command execution — commands use validated, discrete arguments.
- `open_task_folder` validates that the path is inside the `output/` directory.
- CSP is `null` (experiment-phase simplification).
- Bundle is disabled (`"active": false` in `tauri.conf.json`).

## What Is NOT Implemented

- No PyInstaller, bundled Python/Go, or bundled ffmpeg.
- No auto-update, system tray, or menu bar.
- No FastAPI, database, task queue, or server backend.
- No LAN mode, PWA, or browser recording.
- No production packaging, sidecars, or distribution.
- No normal resume or cancellation UI. Rust exposes resume for a future UI and
  terminates the active child process during app shutdown.

## Manual Verification Checklist

- [ ] Tauri window opens with title "meeting-asr Desktop Prototype"
- [ ] Subtitle shows "Experimental Tauri prototype"
- [ ] File picker allows `.mp3`, `.wav`, and `.m4a`
- [ ] Output mode shows `transcript`, `notes`, and `minutes`
- [ ] Python is the default and Go is labeled experimental in Settings
- [ ] Privacy warning is accurate for all modes
- [ ] `transcript` mode starts without DeepSeek key
- [ ] `minutes` mode calls CLI with `--output-mode minutes`
- [ ] Go binary discovery failure shows the manual build instruction
- [ ] stdout/stderr appear in real time in the log panel
- [ ] Logs auto-expand when stderr arrives or on failure
- [ ] On success, task folder and output paths are displayed
- [ ] "Open Output Folder" opens the correct folder
- [ ] Missing uv shows clear error message
- [ ] No API keys appear in logs or UI output
- [ ] Existing CLI (`uv run meeting-asr`) still works
- [ ] Existing Streamlit WebUI still works

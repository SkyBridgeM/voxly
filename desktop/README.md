# meeting-asr Desktop Prototype

> **EXPERIMENTAL** — This is a Tauri desktop shell prototype for `meeting-asr`.
> It is NOT a production app and is NOT intended for merge into `main`.

## What This Is

A thin Tauri + React + TypeScript desktop wrapper that calls either the Python CLI
(via `uv run meeting-asr`) or the Go CLI (`core-go/meeting-asr-go`). All business
logic (ASR, LLM, workflow, resume, ffmpeg) remains in the respective core.

## Architecture

```
desktop/ (Tauri + React)
  ├── Python backend (stable): uv run meeting-asr transcribe <audio> --output-mode <mode>
  └── Go backend (experimental): core-go/meeting-asr-go transcribe <audio> --output-mode <mode>
                │
                ├── reads .env from project root
                ├── runs ASR / LLM
                └── writes to output/
```

## Prerequisites

| Requirement | Why |
|---|---|
| **Node.js + npm** | Frontend build and Tauri CLI |
| **Rust** (rustc, cargo) | Tauri Rust backend |
| **[uv](https://docs.astral.sh/uv/)** | Python package manager (`python` backend only) |
| **ffmpeg** | Audio splitting (required by both backends) |
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
#    minutes/notes mode needs: MIMO_API_KEY + DEEPSEEK_API_KEY
```

### Go Backend (Experimental)

```bash
# Build the Go CLI binary
cd core-go
go build -o meeting-asr-go .
go test ./...

# Start Tauri dev with Go backend selected in Settings
cd ../desktop
npm run tauri dev
```

The Go binary is discovered via:
1. `MEETING_ASR_GO_BINARY` environment variable (absolute path)
2. Fallback: `<project-root>/core-go/meeting-asr-go`

The Rust backend validates the binary with `<binary> version` before use.

## Running

```bash
cd desktop
npm run tauri dev
```

## How It Works

1. User selects a `.mp3`, `.wav`, or `.m4a` file via the Tauri file dialog.
2. User chooses output mode: `transcript`, `notes`, or `minutes`.
3. User selects an output directory (defaults to `<project-root>/output/`).
4. User selects backend engine in Settings → Advanced: Python (default) or Go (experimental).
5. On "Start", based on backend:

   **Python:**
   ```
   uv run meeting-asr transcribe <audio_path> --output-mode <mode> --output-dir <dir>
   ```

   **Go:**
   ```
   core-go/meeting-asr-go transcribe <audio_path> --output-mode <mode> --output-dir <dir> --env-file <path>
   ```

   API keys are passed via environment variables; never in CLI arguments. DeepSeek model
   is set via `DEEPSEEK_MODEL` envvar for Go, or `--llm-model` flag for Python.

6. stdout/stderr stream in real time to the UI.
7. On completion, the task folder and output file paths are displayed.
8. "Open Output Folder" opens the task directory in the system file manager.

## Backend Selection

The backend engine (Python or Go) is selected in the Settings panel under
"Advanced / Experimental" and persisted in `localStorage`.

Backend choice must be restarted to take effect for a new run.

**Python** is the stable default and uses the existing `uv run meeting-asr` command.

**Go** is experimental and requires `core-go/meeting-asr-go` to be built first.
It supports all three output modes (transcript, notes, minutes) and resume.

## API Key Handling

- API keys are **never** embedded in source code, logs, or CLI arguments.
- The CLI reads keys from the project-root `.env` file (preferred).
- Temporary keys can be entered in the Settings panel for the current session only.
  They are passed as environment variables to the CLI process.
- Keys can be saved to `.env` from the Settings panel ("Save to .env").
  Rust writes `.env` directly — keys never appear in process argv.
- If both `.env` and Settings are empty, the CLI will report the missing key error.

## Privacy Warnings

- **transcript mode**: Sends audio to MiMo ASR for transcription.
- **notes mode**: Sends audio to MiMo ASR for transcription, then transcript to DeepSeek for notes.
- **minutes mode**: Sends transcript text to DeepSeek for summary generation.
- Do not upload unauthorized sensitive meeting content.

## Tauri Permissions

The app only requests:
- `core:default` — Basic Tauri operations
- `dialog:default` — File picker dialog

No filesystem, shell, network, or opener permissions are granted.

## Structured Error Categories

The Rust backend categorizes errors for the frontend:

| Category | Examples |
|---|---|
| `backend_not_found` | uv or Go binary missing |
| `dependency_missing` | ffmpeg not on PATH |
| `missing_api_key` | MIMO_API_KEY or DEEPSEEK_API_KEY not set |
| `invalid_input` | Bad audio format, invalid output mode |
| `manifest_error` | Corrupt or invalid task_manifest.json |
| `task_locked` | .resume.lock file present |
| `interrupted` | Process killed by signal |
| `process_failed` | Generic non-zero exit |

## Common Errors

| Error | Cause | Fix |
|---|---|---|
| `uv is not installed` | uv not in PATH | Install uv and restart |
| `No module named meeting_asr` | Dependencies not installed | Run `uv sync --all-extras` |
| `Go binary not found` | Go binary not built | Run `cd core-go && go build -o meeting-asr-go .` |
| `ffmpeg not found` | ffmpeg not installed | Install ffmpeg separately |
| `.env` missing | No API keys configured | Create `.env` with required keys |
| `MIMO_API_KEY not set` | Key missing in `.env` | Add key to `.env` |
| `DEEPSEEK_API_KEY not set` | Key missing for notes/minutes mode | Add key to `.env` |

## Security Notes

- No arbitrary shell command execution — only fixed commands with validated arguments.
- `open_task_folder` validates that the path is inside the selected output directory.
- CSP is `null` (experiment-phase simplification).
- Bundle is disabled (`"active": false` in `tauri.conf.json`).

## What Is NOT Implemented

- No PyInstaller, bundled Python, or bundled ffmpeg.
- No auto-update, system tray, or menu bar.
- No FastAPI, database, task queue, or server backend.
- No LAN mode, PWA, or browser recording.
- No production packaging, sidecar bundling, or distribution.
- No user-facing cancellation or resume UI.

## Manual Verification Checklist

- [ ] Tauri window opens with title "meeting-asr Desktop Prototype"
- [ ] File picker allows `.mp3`, `.wav`, and `.m4a`
- [ ] Output mode shows `transcript`, `notes`, and `minutes`
- [ ] Privacy warning shown for all modes
- [ ] Python: `transcript` mode starts without DeepSeek key
- [ ] Go: `transcript` mode starts with Go binary
- [ ] Go: `notes` mode generates notes_input.md
- [ ] Go: `minutes` mode generates meeting_minutes.md
- [ ] Output directory picker works outside repository
- [ ] Missing Go binary shows clear error
- [ ] Missing ffmpeg shows clear error
- [ ] Missing MiMo key shows clear error
- [ ] Missing DeepSeek key for notes/minutes shows clear error
- [ ] Locked task shows clear error
- [ ] stdout/stderr appear in real time in the log panel
- [ ] Logs auto-expand when stderr arrives or on failure
- [ ] On success, task folder and output paths are displayed
- [ ] "Open Output Folder" opens the correct folder
- [ ] No API keys appear in logs or UI output
- [ ] Existing CLI (`uv run meeting-asr`) still works
- [ ] Existing Streamlit WebUI still works

# meeting-asr Desktop Prototype

> **EXPERIMENTAL** — This is a Tauri desktop shell prototype for `meeting-asr`.
> It is NOT a production app and is NOT intended for merge into `main`.

## What This Is

A thin Tauri + React + TypeScript desktop wrapper that calls the existing Python CLI
via `uv run meeting-asr`. All business logic (ASR, LLM, workflow, resume, ffmpeg)
remains in the Python core under `src/meeting_asr/`.

## Architecture

```
desktop/ (Tauri + React)
  └── spawns: uv run meeting-asr transcribe <audio> --output-mode <mode>
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
| **[uv](https://docs.astral.sh/uv/)** | Python package manager and CLI runner |
| **ffmpeg** | Audio splitting (required by Python CLI) |
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
#    minutes mode needs:    MIMO_API_KEY + DEEPSEEK_API_KEY
```

## Running

```bash
cd desktop
npm run tauri dev
```

## How It Works

1. User selects a `.mp3`, `.wav`, or `.m4a` file via the Tauri file dialog.
2. User chooses output mode: `transcript` or `minutes`.
3. On "Start", the app runs:
   ```
   uv run meeting-asr transcribe <audio_path> --output-mode <mode>
   ```
4. stdout/stderr stream in real time to the UI.
5. On completion, the task folder and output file paths are displayed.
6. "Open Output Folder" opens the task directory in the system file manager.

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

- No arbitrary shell command execution — only `uv run meeting-asr` with fixed arguments.
- `open_task_folder` validates that the path is inside the `output/` directory.
- CSP is `null` (experiment-phase simplification).
- Bundle is disabled (`"active": false` in `tauri.conf.json`).

## What Is NOT Implemented

- No PyInstaller, bundled Python, or bundled ffmpeg.
- No auto-update, system tray, or menu bar.
- No FastAPI, database, task queue, or server backend.
- No LAN mode, PWA, or browser recording.
- No production packaging or distribution.

## Manual Verification Checklist

- [ ] Tauri window opens with title "meeting-asr Desktop Prototype"
- [ ] Subtitle shows "Experimental Tauri prototype"
- [ ] File picker allows `.mp3`, `.wav`, and `.m4a`
- [ ] Output mode shows `transcript` and `minutes` only
- [ ] Privacy warning shown for both modes
- [ ] `transcript` mode starts without DeepSeek key
- [ ] `minutes` mode calls CLI with `--output-mode minutes`
- [ ] stdout/stderr appear in real time in the log panel
- [ ] Logs auto-expand when stderr arrives or on failure
- [ ] On success, task folder and output paths are displayed
- [ ] "Open Output Folder" opens the correct folder
- [ ] Missing uv shows clear error message
- [ ] No API keys appear in logs or UI output
- [ ] Existing CLI (`uv run meeting-asr`) still works
- [ ] Existing Streamlit WebUI still works

# meeting-asr Tauri Desktop Experiment

This is a **Tauri desktop thin-shell experiment** for `meeting-asr`.
It is NOT a mainline feature and is not intended for merge into `main`.

The Python core (`src/meeting_asr/`) and CLI remain the single source of truth
for all ASR, LLM, workflow, resume, and ffmpeg logic. This desktop shell only
provides a Tauri window + React UI that calls the existing Python CLI.

## Architecture

```
desktop/ (Tauri + React)  →  spawns →  .venv/bin/python -m meeting_asr.main transcribe ...
                                           │
                                           ├── reads .env from project root
                                           ├── runs ASR / LLM
                                           └── writes to output/
```

- **Rust side**: One Tauri command `run_meeting_asr` that validates input,
  spawns the Python CLI as a child process, and streams stdout/stderr back to
  the frontend via Tauri events.
- **React frontend**: File picker, mode selector, run button, real-time log
  display, task-folder detection, open-output-directory button.

## Prerequisites

| Requirement | Why |
|---|---|
| **Node.js + npm** | Frontend build and Tauri CLI |
| **Rust** (rustc, cargo) | Tauri Rust backend |
| **Python venv at `.venv/`** (project root) | `meeting-asr` CLI runtime |
| **`.env` at project root** | API keys (MIMO_API_KEY, optionally DEEPSEEK_API_KEY) |
| **ffmpeg** | Audio splitting (required by Python CLI) |

Install Rust: https://www.rust-lang.org/tools/install

The desktop app resolves the project root at compile time via
`env!("CARGO_MANIFEST_DIR")` and calls `.venv/bin/python`. This means:

- The app **must** be built from within this repository.
- The `.venv/` directory **must** exist at the project root.
- The Python package **must** be installed in `.venv` (`pip install -e .`).

## Running

```bash
# From the repository root, ensure Python env is ready
python3 -m venv .venv
.venv/bin/pip install -e .

# Then start the Tauri desktop app
cd desktop
npm install
npm run tauri dev
```

The first `npm install` may take a while. `tauri dev` will compile the Rust
backend and launch the desktop window.

## Usage

1. Click **Choose File…** and select a `.mp3` or `.wav` audio file.
2. Choose output mode:
   - **transcript** — transcribe audio only (no LLM call, no DeepSeek key needed).
   - **minutes** — transcribe + generate meeting minutes via DeepSeek LLM
     (requires `DEEPSEEK_API_KEY` in `.env`).
3. Click **Start**. Logs from the Python CLI appear in real time.
4. On success the **Task folder** path is shown. Click **Open Output Directory**
   to open it in Finder (macOS) / file manager.

## Security Notes

- **API keys** are never embedded in frontend or Rust source code. The Python
  CLI reads them from the project root `.env` as it always does.
- **No arbitrary command execution** is exposed. The Rust side accepts only
  a file path and a validated output mode (`transcript` / `minutes`), and
  constructs the exact CLI invocation internally — no shell string
  concatenation.
- **CSP is `null`** in `tauri.conf.json`. This is an experiment phase
  simplification. A production build should define a proper Content-Security-Policy.
- **opener plugin** (`opener:default`) allows opening any path with the OS
  default handler. In a production build this should be scoped more tightly,
  e.g. to the project output directory only. For this experiment the
  convenience outweighs the risk.

## Error Handling

Errors from the Python CLI (missing `.env`, missing API key, missing ffmpeg,
invalid audio file, etc.) are surfaced directly in the log panel and status
line. The desktop shell does not intercept or re-interpret them.

## Manual Verification Checklist

- [ ] Tauri window opens with title "meeting-asr Desktop Prototype".
- [ ] File picker only allows `.mp3` / `.wav`.
- [ ] `transcript` mode starts the Python CLI and completes without DeepSeek key.
- [ ] `minutes` mode calls the Python CLI with `--output-mode minutes`.
- [ ] stdout/stderr appear in real time in the log panel.
- [ ] On success, "Task folder: output/..." is detected and displayed.
- [ ] "Open Output Directory" button opens the correct folder.
- [ ] Paths with spaces or CJK characters work.
- [ ] `.venv` missing → Python CLI error shown.
- [ ] `.env` missing or API key missing → Python CLI error shown.
- [ ] ffmpeg missing → Python CLI error shown.
- [ ] Existing Streamlit WebUI and CLI are unaffected.

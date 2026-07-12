# meeting-asr Desktop Prototype

> **EXPERIMENTAL** — Python is the stable default in development; Go is
> available as an explicitly experimental development backend and as the
> packaged production-candidate sidecar.

## What This Is

A thin Tauri + React + TypeScript desktop wrapper that calls either Python via
`uv run meeting-asr` or the experimental Go CLI. Rust validates paths, streams
the stable stdout contract, and validates final artifacts before reporting success.

## Architecture

```
desktop/ (Tauri + React)
  ├── Python (default): uv run meeting-asr transcribe ...
  ├── Go (development): core-go/meeting-asr-go transcribe ...
  └── Go (production candidate): packaged meeting-asr-go sidecar
```

## Prerequisites

| Requirement | Why |
|---|---|
| **Node.js + npm** | Frontend build and Tauri CLI |
| **Rust** (rustc, cargo) | Tauri Rust backend |
| **[uv](https://docs.astral.sh/uv/)** | Python backend only |
| **Go 1.21+** | Build the experimental Go backend or desktop sidecar |
| **ffmpeg** | Audio splitting (required by both backends; not bundled) |
| **`.env` at project root (development only)** | Development API-key compatibility |

## Setup

```bash
# 1. Install uv (if not installed)
curl -LsSf https://astral.sh/uv/install.sh | sh

# 2. Install Python dependencies
uv sync --all-extras

# 3. Install frontend dependencies
cd desktop
npm install

# 4. Optional development compatibility: configure API keys in project-root .env
#    transcript mode needs: MIMO_API_KEY
#    notes/minutes need:    MIMO_API_KEY
#    minutes additionally:  DEEPSEEK_API_KEY
```

### Go backend (experimental)

```bash
cd core-go
go build -o meeting-asr-go .
```

In development, Rust discovers Go through the absolute `MEETING_ASR_GO_BINARY`
override or the fallback `<project-root>/core-go/meeting-asr-go` (`.exe` on
Windows). It validates `<binary> version`; it does not search PATH or
auto-build the binary.

### Desktop sidecar candidate (macOS)

PR 10 adds the Tauri sidecar layout for the primary macOS targets. Build the
current macOS host target with:

```bash
./scripts/build_desktop_sidecar.sh
```

This writes a generated file such as
`desktop/src-tauri/binaries/meeting-asr-go-aarch64-apple-darwin`. The generated
binary is ignored and must not be committed. The Tauri configuration declares
the logical sidecar name `meeting-asr-go`; a production/release Go run resolves
only the packaged sidecar next to the application executable, validates its
`version` output, and never falls back to `MEETING_ASR_GO_BINARY`, the project
checkout, or `PATH`. Tauri removes the target suffix while copying the sidecar;
the Rust resolver follows the current executable's parent directory, matching
Tauri's supported sidecar path on macOS.

### Internal app-bundle smoke (macOS)

PR 11 adds an offline-only unsigned `.app` smoke. It builds the host sidecar
first, runs a Tauri app-only bundle build, verifies
`<App>.app/Contents/MacOS/meeting-asr-go`, and runs that bundled sidecar's
`version` command:

```bash
./scripts/smoke_desktop_app_bundle.sh
```

This is an internal validation path only. It does not create a DMG or
installer, upload artifacts, or perform signing/notarization. The Tauri bundle
configuration enables only the `app` target for this smoke.

### Runtime preflight (PR 12)

Settings includes a compact, offline-only readiness panel backed by the Rust
`get_runtime_preflight` command. It reports runtime/backend policy, packaged Go
sidecar and version status in production, ffmpeg availability, output-root
availability, and safe credential metadata. It never returns key values,
creates task folders, writes output artifacts, mutates settings or credentials,
or calls MiMo/DeepSeek. Missing ffmpeg is reported as a system prerequisite;
ffmpeg is not downloaded or bundled.

The detailed manual procedure is in
[`docs/phase5_internal_acceptance.md`](../docs/phase5_internal_acceptance.md).

## Running

```bash
# Tauri's externalBin declaration requires the generated host sidecar input.
./scripts/build_desktop_sidecar.sh
cd desktop
npm run tauri dev
```

For the internal release-style bundle smoke, run the script above from the
repository root after installing the desktop dependencies.

## Development vs. Planned Production Candidate

This repository currently runs the development desktop build described above.
It requires the project checkout, `uv` for Python, and a manually built Go
binary when Go is selected. A project-root `.env` remains a development
compatibility source, but is not a production credential store.

Phase 5 is preparing a separate internal macOS Apple Silicon production
candidate. The output-root foundation now distinguishes development output from
the planned production default: development continues to use
`<project-root>/output`, while a release build defaults to
`~/Documents/Meeting ASR`. A future Settings UI may select a different output
root; its non-sensitive JSON configuration lives in the app config directory,
not in the project root.

PR 8B adds a Rust credential abstraction backed by the platform secure store
(macOS Keychain on the primary target). The frontend can request only safe
status metadata and can ask Rust to save or delete a key; stored values are
never returned to the frontend. Development/debug resolution is session,
secure store, then project-root `.env`. Release resolution is session, then
secure store, and never the project-root `.env`.

PR 11 adds only an internal app-only bundle smoke and production backend policy
cleanup. Release builds use the packaged Go sidecar only; Python remains the
development fallback/reference and is not a production fallback. The backend
selector remains visible in development and is replaced by the packaged Go
label in production. The current Settings UI keeps its existing "Save to .env"
development compatibility flow; a `.env` migration UI is deferred. ffmpeg
remains a system prerequisite, while public packaging, signing, notarization,
installers, and automatic updates are deferred.

PR 12 adds only local runtime preflight UX and the manual internal acceptance
checklist. The preflight panel is informational and offline-only; it does not
perform live API validation or change the production backend policy. The
development-only "Save to .env" action is hidden in production, which reads
credentials only from the current session or secure storage.

## How It Works

1. User selects a `.mp3`, `.wav`, or `.m4a` file via the Tauri file dialog.
2. User chooses `transcript`, `notes`, or `minutes`.
3. In Settings, development shows Python as the default and Go as experimental;
   a release build shows the packaged Go sidecar policy.
4. Settings can refresh a local readiness summary for the runtime, sidecar,
   ffmpeg, output root, and credential metadata without showing key values.
5. On "Start", Rust resolves the required credentials and runs the selected
   backend with structured arguments. Credentials are passed only through child
   process environment variables. In development, Python/Go retain their
   project-root `.env` compatibility; in release, Python disables dotenv
   loading and a packaged Go sidecar receives an isolated runtime project root.
6. stdout/stderr stream in real time to the UI.
7. On completion, canonical task/artifact paths must validate inside root `output/`.
8. "Open Output Folder" opens only a validated task directory.

## API Key Handling

- API keys are **never** embedded in source code, logs, CLI arguments, JSON
  settings, task folders, or output artifacts. Backend output is redacted at
  the Rust/frontend boundary before it is logged or included in an error.
- Secure credential commands use OS secure storage where available. They expose
  only `configured` and a safe source (`session`, `secure_store`, `env`, or
  `missing`), never a stored key value.
- The Python CLI may read keys from the project-root `.env` only in development
  compatibility mode.
- Temporary keys can be entered in the Settings panel for the current session only.
  They are passed as environment variables to the CLI process.
- The existing Settings action can save to `.env` for development compatibility;
  it does not put keys in `desktop-settings.json` and is not a production source.
- If no allowed source is configured, Rust returns a structured missing-key or
  secure-storage-unavailable error without including the key value.

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
| `.env` missing | No development compatibility keys configured | Configure secure credentials or enter a session key; development may also create `.env` |
| `MIMO_API_KEY not configured` | MIMO key missing from the allowed sources | Configure secure credentials or enter a session key |
| `DEEPSEEK_API_KEY not configured` | DeepSeek key missing for minutes mode | Configure secure credentials or enter a session key |
| `secure_storage_unavailable` | OS secure storage cannot be accessed | Fix Keychain/secure-store availability; plaintext fallback is not used |

## Security Notes

- No arbitrary shell command execution — commands use validated, discrete arguments.
- `open_task_folder` validates that the path is inside the `output/` directory.
- `desktop-settings.json` contains non-sensitive settings only; API keys,
  transcript content, task content, and audio history are not persisted there.
- Unknown settings fields are rejected deliberately. Future schema changes must
  use `schema_version` with an explicit migration.
- CSP is `null` (experiment-phase simplification).
- Bundle configuration is limited to the unsigned internal macOS `app` smoke;
  no DMG, installer, signing, notarization, updater, or public release target
  is configured.

## What Is NOT Implemented

- No PyInstaller, bundled Python, or bundled ffmpeg. The generated Go sidecar
  candidate is not committed.
- No auto-update, system tray, or menu bar.
- No FastAPI, database, task queue, or server backend.
- No LAN mode, PWA, or browser recording.
- No public production distribution. The only bundle path is the internal
  unsigned app-only smoke; installers and release artifacts remain deferred.
- No normal resume or cancellation UI. Rust exposes resume for a future UI and
  terminates the active child process during app shutdown.

## Manual Verification Checklist

- [ ] Tauri window opens with title "meeting-asr Desktop Prototype"
- [ ] Subtitle shows "Experimental Tauri prototype"
- [ ] File picker allows `.mp3`, `.wav`, and `.m4a`
- [ ] Output mode shows `transcript`, `notes`, and `minutes`
- [ ] Development Settings show Python as default and Go as experimental
- [ ] Internal bundle smoke finds `Contents/MacOS/meeting-asr-go` and its
      `version` command succeeds
- [ ] Settings readiness panel reports runtime/backend, sidecar, ffmpeg,
      output-root, and safe credential status metadata
- [ ] Missing ffmpeg is reported as a system prerequisite without download
- [ ] Missing packaged sidecar is reported without Python fallback
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
- [ ] Credential status shows only configured/source metadata, never key values
- [ ] Existing CLI (`uv run meeting-asr`) still works
- [ ] Existing Streamlit WebUI still works

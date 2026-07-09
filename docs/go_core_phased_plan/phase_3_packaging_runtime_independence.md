# Phase 3: Packaging and Runtime Independence

Audience: opencode / implementation agent  
Scope: Tauri packaging and runtime behavior  
Prerequisite: Phase 2 accepted

## Objective

Make the desktop app runnable as a packaged app without requiring:

- Python
- uv
- a repository checkout
- manually built Go binary

## Tasks

### 1. Bundle the Go binary

Use Tauri's supported mechanism for bundled sidecars or external binaries.

Requirements:

- Production app can locate and execute the Go binary.
- Development app still works.
- macOS binary permissions/naming are handled.
- Path resolution does not depend on `CARGO_MANIFEST_DIR` or repo root in production.

### 2. Decide and implement `ffmpeg` strategy

Choose one:

#### Option A: Bundle `ffmpeg`

Best for a personal app if acceptable.

Requirements:

- Go core can locate bundled `ffmpeg`.
- Packaged app works without external `ffmpeg` on PATH.
- Licensing/distribution implications are acknowledged.

#### Option B: External `ffmpeg` with clear detection

Simpler legally/operationally, but worse UX.

Requirements:

- App detects missing `ffmpeg`.
- Error message tells user exactly what to install.
- No silent failure.

Do not leave this ambiguous.

### 3. Fix production config/runtime paths

Requirements:

- Do not rely on repo root for `.env`, prompts, templates, or output.
- Use app config/data directory for app-managed settings.
- Keep output directory user-selected.
- Prompt/template path may remain user-configurable.

### 4. Build and smoke test

Run the narrowest meaningful packaging verification.

Suggested checks:

- Build packaged app.
- Launch packaged app.
- Transcribe a small audio file.
- Write output to a directory outside the repo.
- Confirm no Python/uv dependency.
- Confirm behavior when `ffmpeg` is missing if using external `ffmpeg`.

## Acceptance Criteria

- Packaged app launches.
- Packaged app can transcribe a small file through Go.
- Packaged app writes outputs to selected output directory.
- Packaged app does not require Python or uv.
- Runtime path handling is production-safe.
- `ffmpeg` behavior is explicit and verified.

## Out of Scope

Do not do these in Phase 3:

- Local model support.
- Meetily fork.
- Database-backed meeting library.
- Auto-updater.
- Full recording rewrite.
- Wails migration.
- Broad UI redesign.

## Suggested Review Checklist

- Inspect Tauri config for bundled binary/resource paths.
- Confirm no production code depends on repo root.
- Confirm Go binary path works in packaged app.
- Confirm `ffmpeg` strategy is implemented, not just described.
- Run one end-to-end packaged app transcription.


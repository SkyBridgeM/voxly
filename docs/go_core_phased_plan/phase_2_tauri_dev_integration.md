# Phase 2: Tauri Development Integration

Audience: opencode / implementation agent  
Scope: desktop development integration  
Prerequisite: Phase 1 accepted

## Objective

Make the Tauri desktop app call the Go binary instead of:

```text
uv run meeting-asr
```

This phase is for development-mode integration only. Do not solve full packaging yet.

## Tasks

### 1. Build or locate the Go binary for development

Create a simple development convention, for example:

```bash
cd core-go
go build -o meeting-asr-go .
```

The Tauri app should be able to invoke this binary in development.

Keep the convention documented in code comments or README only if needed.

### 2. Replace Tauri command execution

In the desktop Tauri backend, replace the Python/uv process launch with the Go binary.

Current behavior to replace:

```text
uv run meeting-asr transcribe ...
```

New behavior:

```text
meeting-asr-go transcribe ...
```

Keep the existing event model if possible:

- `log-line`
- `progress`
- `process-done`

Keep parsing:

- `Status:`
- `Progress:`
- `Transcript saved:`
- `Notes input saved:`
- `Meeting minutes saved:`
- `Task folder:`

### 3. Pass output directory

If the current UI does not expose output directory selection, add the smallest practical UI/control needed.

Requirements:

- User can select or provide an output directory.
- Tauri passes `--output-dir` to the Go CLI.
- Do not assume repo-root `output/`.
- Result folder opening should work with the selected output directory.

### 4. Preserve key handling for now

For this phase:

- Passing API keys as process environment variables is acceptable.
- Do not hardcode keys.
- Do not redesign secure storage yet unless it is trivial and low risk.

### 5. Resume UI is optional

Do not let resume UI expand this phase too much.

Acceptable options:

- Option A: keep resume CLI-only for now and document it.
- Option B: add a minimal “resume task folder” action if it is clearly small.

Prefer Option A if UI scope starts growing.

## Acceptance Criteria

- Desktop app can run transcript mode through the Go binary in development.
- Desktop app can run minutes mode when required keys are provided.
- Desktop app does not require `uv` for this path.
- Progress appears in the UI.
- Final output paths appear in the UI.
- Opening output folder works for user-selected output directories.
- Existing Python code remains available as fallback; do not delete it.

## Out of Scope

Do not do these in Phase 2:

- Production packaging.
- Tauri sidecar finalization.
- Bundling `ffmpeg`.
- Removing Python files.
- Wails migration.
- Meetily fork.
- Local ASR model support.
- Large UI redesign.

## Suggested Review Checklist

- Run desktop app in development.
- Confirm no `uv` process is launched for Go path.
- Test output directory outside repo.
- Test transcript mode.
- Test error display with missing API key.
- Review that changes are limited to desktop integration and minimal docs.


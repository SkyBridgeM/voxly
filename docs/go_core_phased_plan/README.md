# Go Core Migration Phased Plan

> **Archived migration plan.** The repository has moved beyond these phases:
> Go is the active desktop runtime and Tauri integration and packaging
> foundations are present on `main`. Keep this directory only as historical
> implementation context; do not use its “Current Recommendation” as current
> project direction.

This directory splits the Go core migration into reviewable implementation phases.

Use one phase at a time with opencode. Do not ask the implementation agent to execute all phases in one pass.

## Phase Order

1. [Phase 1: Go Core CLI Parity](./phase_1_go_core_cli_parity.md)
   - Scope: `core-go` only.
   - Goal: make the Go CLI reliable enough to replace the Python core for imported audio transcription.

2. Phase 2: Tauri Development Integration (implemented; no standalone phase
   file was retained)
   - Scope: desktop Tauri wrapper and minimal UI/backend integration.
   - Goal: make the desktop app call the Go binary instead of `uv run meeting-asr` in development.

3. Phase 3: Packaging and Runtime Independence (implemented; no standalone
   phase file was retained)
   - Scope: packaging, sidecar/binary location, app runtime paths, `ffmpeg` handling.
   - Goal: packaged app runs without Python, uv, or a repo checkout.

## Review Rule

After each phase:

- Run the phase-specific verification steps.
- Review the diff.
- Do not proceed to the next phase until the current phase is accepted.

## Current Recommendation

Start with Phase 1 only.

Phase 1 is intentionally CLI-only. It should not touch Tauri, packaging, Wails, Meetily, or local model support.

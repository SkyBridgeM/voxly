# Phase 5 Packaging Readiness

Phase 5 moves the desktop application toward an internal macOS Apple Silicon
production candidate. It does not package or distribute an application yet.

## Runtime Direction

The current desktop application is a development build:

```text
Tauri UI -> Rust -> Python (default) or experimental Go CLI -> project output/
```

The future internal production candidate will default to a packaged Go sidecar
and write task folders under `~/Documents/Meeting ASR`. Python remains in the
repository as the development fallback and reference implementation; it is not
being removed from source code or regression tests.

## Decisions Recorded for Phase 5

- First packaging target: macOS Apple Silicon.
- First deliverable: an internal test artifact, not a public release.
- Production-candidate runtime: packaged Go sidecar by default; no silent
  Python fallback.
- Development runtime: Python remains default and Go remains explicitly
  experimental.
- ffmpeg: required as a system dependency. It will not be bundled, downloaded,
  or installed by the application.
- API keys: Phase 5 PR 8B will use macOS Keychain. Production candidates must
  not depend on a project-root `.env` file.
- Output: Phase 5 PR 8A introduces runtime-aware output roots. Development
  continues to default to `<project-root>/output`; release builds default to
  `~/Documents/Meeting ASR`. Configured output roots and non-sensitive settings
  live in the app config directory and retain canonical containment checks.

## This PR's CI Baseline

The `Desktop Readiness` workflow runs on a macOS runner and remains fully
offline. It checks the frontend build, Rust desktop formatting/tests/clippy, Go
tests/vet, Python regression tests, and whitespace errors.

The workflow deliberately does not require an Apple Silicon runner or produce
an arm64 package. Sidecar construction and an Apple Silicon packaging smoke
test begin only after the sidecar implementation PR exists.

## Explicitly Deferred

- Tauri `externalBin` configuration and generated Go sidecar binaries.
- `.app`, DMG, installer, signing, notarization, release, and updater work.
- ffmpeg bundling, download, automatic installation, or custom-path UI.
- Keychain implementation and `.env` migration UX (PR 8B).
- Production output-directory Settings UI. PR 8A provides the Rust foundation
  and command API only; it does not add a large frontend settings flow.
- Visible resume and cancellation UI.
- Live API validation. Before an internal production candidate is accepted,
  Go transcript, notes, minutes, and interrupted minutes-resume checks must
  pass with real credentials and recordings.

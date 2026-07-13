# Phase 5 Packaging Readiness

Phase 5 moves the desktop application toward an internal macOS Apple Silicon
production candidate. It does not distribute a public application yet; PR 11
adds only an internal unsigned app-bundle smoke.

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
- API keys: Phase 5 PR 8B introduces a Rust credential abstraction backed by
  macOS Keychain on the primary target. Production candidates must not depend
  on a project-root `.env` file.
- Output: Phase 5 PR 8A introduces runtime-aware output roots. Development
  continues to default to `<project-root>/output`; release builds default to
  `~/Documents/Meeting ASR`. Configured output roots and non-sensitive settings
  live in the app config directory and retain canonical containment checks.

## This PR's CI Baseline

The `Desktop Readiness` workflow runs on a macOS runner and remains fully
offline. It checks the frontend build, Go sidecar build/version smoke, Tauri
sidecar normalization, an unsigned internal app-bundle smoke, Rust desktop
formatting/tests/clippy (including preflight unit tests), Go tests/vet, Python
regression tests, and whitespace errors.

The sidecar smoke builds the current macOS host architecture; Apple Silicon is
the primary production target and Intel macOS remains supported for CI smoke.
The workflow does not produce a public package, upload a release, or require
credentials or live API calls. The app-bundle smoke is limited to the macOS
`app` target and uses `--no-sign`.

## PR 10 Go Sidecar Foundation

The Tauri configuration declares the logical sidecar name
`binaries/meeting-asr-go` through `bundle.externalBin`. On macOS Apple Silicon,
`./scripts/build_desktop_sidecar.sh` builds from `core-go/` to the generated
path:

```text
desktop/src-tauri/binaries/meeting-asr-go-aarch64-apple-darwin
```

The script also supports the Intel macOS host target for offline CI smoke. All
generated sidecar binaries are ignored and must not be committed. Tauri copies
the target-suffixed input to the packaged application executable directory.

The Rust resolver is runtime-aware:

- Development/debug keeps the absolute `MEETING_ASR_GO_BINARY` override and
  project-root `core-go/meeting-asr-go` fallback, with no PATH search or
  auto-build.
- Production/release resolves only the packaged `meeting-asr-go` next to the
  application executable, validates `version`, and returns structured
  `backend_not_found` when it is absent or invalid. On macOS, the resolver
  uses the current executable's parent directory because Tauri's
  `PathResolver::executable_dir()` is not supported on macOS.
- Production does not read `MEETING_ASR_GO_BINARY` or use a project-root Go
  binary, and it does not silently fall back to Python.

Python remains the development default and reference implementation. PR 11 adds
only an internal app-only smoke and production policy enforcement; it does not
change Go or Python core behavior, bundle ffmpeg, or create a public
distribution artifact.

## PR 11 Internal App-Bundle Smoke and Production Policy

`./scripts/smoke_desktop_app_bundle.sh` first generates the host sidecar, then
runs an app-only Tauri release build with signing disabled. It verifies that an
`.app` exists and that the normalized sidecar is at:

```text
<App>.app/Contents/MacOS/meeting-asr-go
```

The script runs the bundled sidecar's `version` command. It does not require
API keys, call MiMo or DeepSeek, upload artifacts, or create DMG/installer
outputs. The production backend policy is enforced in Rust and surfaced to the
frontend through safe runtime metadata: release runs use the packaged Go
sidecar only, while development retains the Python default and visible Go
experimental selector.

## PR 12 Production Preflight and Internal Acceptance

PR 12 adds the Rust `get_runtime_preflight` command and a compact Settings
readiness panel. The command performs local checks only: it reports runtime and
backend policy, validates the packaged Go sidecar and its `version` output in
production, checks whether `ffmpeg` is available on `PATH`, reports the output
root path and availability, and returns credential configured/source metadata.
Credential values are accepted only for the current invocation so unsaved
session values can be reflected in the status; they are never returned. The
command does not call an API, transcribe audio, create task folders, write
artifacts, download ffmpeg, or mutate settings or credentials. A missing ffmpeg
is reported as a system prerequisite.

Readiness is split into transcript/notes readiness and minutes readiness because
minutes additionally requires `DEEPSEEK_API_KEY`. Development continues to show
Python as the default with Go as an experimental choice; production remains
packaged-Go-only with no Python, project-root `.env`, or
`MEETING_ASR_GO_BINARY` fallback.
The Settings panel keeps the existing `Save to .env` action development-only;
release builds do not offer that compatibility action.

The manual offline and optional-live acceptance steps are recorded in
[`docs/phase5_internal_acceptance.md`](phase5_internal_acceptance.md). Live
transcript, notes, and minutes checks require intentionally supplied keys and a
test recording; they are never part of GitHub Actions.

## PR 13 Internal Acceptance Result

Phase 5 internal acceptance passed after PR #12; the dated result is recorded
in [`docs/phase5_internal_acceptance_2026-07-13.md`](phase5_internal_acceptance_2026-07-13.md).
Signing, notarization, DMG, installer, and updater work remain deferred, and
the next phase should be planned separately.

## PR 8B Secure Credential Foundation

Desktop credentials use the platform secure store where available (`keyring`,
with macOS Keychain as the primary target). Rust exposes only credential status
metadata and save/delete operations; stored API-key values are never returned to
the frontend.

Credential resolution is explicit:

- Development/debug: current-run session value, secure store, then the
  project-root `.env` compatibility source.
- Production/release: current-run session value, then secure store. The
  project-root `.env` is not read.

Keys are passed to backend child processes through environment variables only.
They are not placed in argv, logs, structured errors, `desktop-settings.json`,
task folders, or output artifacts. Transcript and notes require only
`MIMO_API_KEY`; minutes additionally requires `DEEPSEEK_API_KEY`.

`desktop-settings.json` remains non-sensitive and rejects unknown fields.
Future schema changes must add `schema_version` and an explicit migration.
The existing development "Save to .env" compatibility flow remains, while a
`.env` migration UX is deferred.

## Explicitly Deferred

- Public/generated distribution artifacts.
- DMG, installer, signing, notarization, release, and updater work. The
  unsigned internal `.app` smoke is covered by PR 11.
- ffmpeg bundling, download, automatic installation, or custom-path UI.
- `.env` migration UX; secure credential storage is provided by PR 8B.
- Production output-directory Settings UI. PR 8A provides the Rust foundation
  and command API only; it does not add a large frontend settings flow.
- Visible resume and cancellation UI.
- Live API validation. Before an internal production candidate is accepted,
  Go transcript, notes, minutes, and interrupted minutes-resume checks must
  pass with real credentials and recordings.

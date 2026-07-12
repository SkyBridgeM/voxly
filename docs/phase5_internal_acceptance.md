# Phase 5 Internal Acceptance Checklist

This checklist covers the unsigned macOS internal production candidate. It is
an acceptance aid, not a public release procedure. PR 12 adds local runtime
preflight reporting; it does not add an installer, signing, notarization,
updater, ffmpeg bundling, or live API validation to CI.

## Offline checks

Run these checks on the macOS host used for internal acceptance:

- [ ] Build the host sidecar:
  `./scripts/build_desktop_sidecar.sh`.
- [ ] Build the unsigned internal app bundle:
  `./scripts/smoke_desktop_app_bundle.sh`.
- [ ] Launch the generated `.app` from
  `desktop/src-tauri/target/release/bundle/macos/`.
- [ ] Open Settings and confirm the readiness panel reports the runtime mode,
  backend policy, ffmpeg status, credential metadata, and output root.
- [ ] Confirm a release build reports `Production`, packaged Go, and an output
  root under `~/Documents/Meeting ASR` unless a non-sensitive root was
  configured.
- [ ] Confirm the packaged sidecar is used from
  `<App>.app/Contents/MacOS/meeting-asr-go`.
- [ ] Confirm Python is not selectable in the release build.
- [ ] Run the readiness check in an environment without `ffmpeg` on `PATH` and
  confirm it reports that ffmpeg is a system prerequisite. No download or
  installation should be offered or attempted.
- [ ] Test a copy of the app with the packaged sidecar removed and confirm the
  readiness panel reports the sidecar as not ready and a run returns a
  structured `backend_not_found` error. Do not add a project-root fallback.
- [ ] With both secure-store entries absent, confirm MIMO and DeepSeek show
  `Missing` without displaying or echoing any secret values.
- [ ] Confirm the secure-store status exposes only configured/source metadata;
  no credential value appears in the UI, logs, errors, task folders, or output
  artifacts.
- [ ] Confirm a production run does not depend on the checkout `.env`, even if
  that file exists. Production credentials come from the current session or
  secure store only.

The readiness panel and all CI checks are local/offline. They inspect the
packaged sidecar, the local ffmpeg executable, output-root availability, and
credential metadata; they do not call MiMo or DeepSeek.

## Optional live acceptance checks

These checks require intentionally supplied API keys and a test recording.
They are manual only and must never be added to GitHub Actions:

- [ ] Transcript mode succeeds with `MIMO_API_KEY` only.
- [ ] Notes mode succeeds with `MIMO_API_KEY` only and writes the local notes
  input artifact.
- [ ] Minutes mode succeeds with both `MIMO_API_KEY` and
  `DEEPSEEK_API_KEY`.
- [ ] Missing `MIMO_API_KEY` blocks transcript, notes, and minutes with a safe
  missing-key message.
- [ ] Missing `DEEPSEEK_API_KEY` blocks minutes without exposing the MIMO or
  DeepSeek value.

Do not record real keys in screenshots, logs, task folders, output artifacts,
or acceptance notes. Remove test credentials from the secure store after the
manual run.

## Deferred release work

DMG/installer generation, signing, notarization, updater behavior, public
release artifacts, and ffmpeg bundling remain deferred. Python remains the
development fallback/reference implementation; Go remains the packaged
production backend policy.

# Local Unsigned macOS Artifact Checklist

## 1. Purpose

This is a manual checklist for preparing and validating a future unsigned
Apple Silicon macOS `.app` candidate before it is zipped. It is not a public
release process by itself and does not authorize an upload.

This document records commands and checks only. PR #16 does not run the build,
create a zip, calculate a release checksum, write a build manifest, or publish
an artifact.

## 2. Prerequisites

- Use a trusted Apple Silicon Mac controlled by the maintainer.
- Start from a clean checkout of the latest `main` branch.
- Install ffmpeg separately and verify that it is visible to the app when the
  `.app` is launched from Finder, not only from a configured terminal.
- Have the supported Node.js, Rust, Go, and frontend dependencies available for
  the existing local bundle smoke.
- Apple Developer Program membership, Developer ID, signing, and notarization
  are not required for this unsigned path.

Before continuing, verify ffmpeg in the local shell:

```bash
ffmpeg -version
```

The application must not download, install, update, or bundle ffmpeg.

## 3. Prepare a clean checkout

Run these commands from the repository checkout:

```bash
git fetch origin
git switch main
git pull --ff-only origin main
git status --short --branch
git rev-parse HEAD
```

- Stop if `git status` reports tracked changes or untracked files that are not
  intentionally outside the checkout used for the build.
- Record the full commit SHA returned by `git rev-parse HEAD` in the future
  build manifest and release notes.
- Do not build from an unreviewed topic branch or an ambiguous working tree.

## 4. Build the local app candidate

From the repository root, run the existing offline build and smoke paths:

```bash
./scripts/build_desktop_sidecar.sh
./scripts/smoke_desktop_app_bundle.sh
```

Confirm that the smoke succeeds and locate the generated `.app` under:

```text
desktop/src-tauri/target/release/bundle/macos/
```

Confirm that the packaged sidecar exists inside the bundle at:

```text
<App>.app/Contents/MacOS/meeting-asr-go
```

Do not zip or upload the candidate as part of PR #16.

## 5. Manual app checks

Launch the generated `.app` from Finder so that the test uses the same GUI
environment expected for an external tester. Confirm all of the following:

- [ ] Runtime mode is `production`.
- [ ] The backend policy is the packaged Go sidecar.
- [ ] Python is not selectable in production.
- [ ] The readiness panel reports the packaged sidecar as ready and
      version-valid.
- [ ] The readiness panel reports ffmpeg as available to the Finder-launched
      app.
- [ ] The output root defaults to `~/Documents/Meeting ASR`, unless a valid
      non-sensitive configured root is intentionally under test.
- [ ] A completed local test writes only inside the configured output root and
      its task folder.
- [ ] Production credential status resolves only session or secure-store
      sources; the project-root `.env` is ignored.
- [ ] No API-key value is displayed in readiness metadata, logs, errors, or
      output artifacts.

If Finder-launched ffmpeg readiness fails, stop artifact preparation. Address
path discovery in a separate focused PR; do not work around the problem by
bundling ffmpeg.

## 6. Negative checks

Run destructive negative checks only against a disposable copy of the generated
`.app`, never against the candidate intended for later packaging.

- [ ] Remove the packaged `meeting-asr-go` sidecar from the disposable copy and
      confirm readiness reports it as not ready.
- [ ] Confirm a production run without the packaged sidecar returns
      `backend_not_found`.
- [ ] Confirm the missing-sidecar case does not search the project checkout or
      fall back to Python.
- [ ] In a controlled environment where ffmpeg is unavailable to the app,
      confirm readiness reports it as a separately installed system
      prerequisite. Do not uninstall a user's working ffmpeg installation for
      this check.
- [ ] Review emitted logs, errors, task folders, transcript artifacts, notes,
      and minutes artifacts and confirm that no API-key values are present.

Discard disposable negative-test copies after the checks. A candidate with a
missing or invalid sidecar must never be packaged.

## 7. Optional manual live checks

Use only an authorized test recording and maintainer-controlled credentials.
These checks are manual and optional for local preparation, but are required by
the Phase 6 plan before an artifact is shared outside the project.

- [ ] Transcript succeeds with `MIMO_API_KEY`.
- [ ] Notes succeeds with `MIMO_API_KEY` and produces the expected local notes
      input artifact.
- [ ] Minutes succeeds with `MIMO_API_KEY` and `DEEPSEEK_API_KEY`.
- [ ] Test results record only pass/fail and non-sensitive context; no API-key
      value, sensitive recording, transcript content, or secret-bearing log is
      committed.

Live MiMo or DeepSeek checks must never be added to CI.

## 8. Optional local artifact preparation

Only after the manual app checks above pass, the optional local-only packaging
script may be used from the clean repository root:

```bash
./scripts/package_unsigned_macos_app.sh
```

The script packages the single existing `.app` under the release bundle
directory. It does not build or modify the app. It creates these local files
under `dist/unsigned/`:

```text
meeting-asr-desktop-experiment-macos-arm64-<version>-unsigned.zip
meeting-asr-desktop-experiment-macos-arm64-<version>-unsigned.zip.sha256
build-manifest-<version>-<short-sha>.json
```

The manifest records the configured app version, full source commit, build date,
`arm64` architecture, artifact filename, SHA-256, unsigned status, and packaged
sidecar status. The zip, checksum, and manifest must agree. Existing output is
not overwritten unless the maintainer explicitly passes `--force`.

These generated local files are not automatically approved for upload. Any
GitHub upload remains blocked until every gate in
[`docs/github_unsigned_prerelease_policy.md`](github_unsigned_prerelease_policy.md)
passes. Running the script does not authorize a public release.

## 9. Release gate

An artifact must not be uploaded unless all applicable checks above pass and
the release page is prepared from
[`docs/unsigned_release_notes_template.md`](unsigned_release_notes_template.md).

Before any future GitHub upload, confirm:

- [ ] The Release is marked **Pre-release** and is not **Latest**.
- [ ] The title, summary, warning, and artifact filename clearly state that the
      build is unsigned and not notarized.
- [ ] The zip, SHA-256 file, and JSON build manifest are all present.
- [ ] The official source commit and build date are recorded.
- [ ] The Release contains no credentials, sensitive content, or claim of Apple
      signing, notarization, trust, or approval.

This checklist does not itself authorize a public release.

## 10. Non-goals

This checklist and optional packaging step do not:

- create or upload a public artifact;
- add a CI release or artifact-upload workflow;
- create a DMG or installer;
- add an updater;
- add Apple Developer Program, Developer ID, signing, or notarization setup;
- bundle, download, install, or change runtime discovery of ffmpeg;
- change application code, backend behavior, package versions, Tauri
  configuration, or live API checks.

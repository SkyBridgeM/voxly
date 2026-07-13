# GitHub Unsigned macOS Pre-release Policy

## 1. Purpose

This policy defines the requirements for any future maintainer-controlled
GitHub Pre-release containing an unsigned macOS artifact. It is policy
documentation only, not a release procedure.

PR #17 does not create, package, upload, or publish an artifact. This document
does not authorize an upload or public release by itself. A future release may
proceed only through a separately reviewed manual process after every applicable
gate in this policy has passed.

## 2. Release type

- The GitHub Release must be marked **Pre-release**.
- It must not be marked **Latest** or described as a stable release.
- The title, summary, asset filename, and warning must clearly state that the
  macOS build is **unsigned** and **not notarized**.
- The release must not claim Apple signing, notarization, trust, approval, or
  endorsement.
- Gatekeeper warnings must be described as an expected consequence of the
  unsigned distribution policy.

## 3. Allowed assets

Only the following asset set is allowed for the current unsigned path:

- `meeting-asr-desktop-experiment-macos-arm64-<version>-unsigned.zip`
- `meeting-asr-desktop-experiment-macos-arm64-<version>-unsigned.zip.sha256`
- `build-manifest-<version>-<short-sha>.json`

All three files must refer to the same build. No DMG, installer package,
updater, signed artifact, or notarized artifact is allowed under this policy.
Source archives generated automatically by GitHub are outside this project
asset set and must not be described as runnable macOS builds.

## 4. Required provenance

The Release page and build manifest must record:

- the full source commit SHA;
- the build date in `YYYY-MM-DD` form;
- the configured application version;
- the target architecture (`arm64` for the current policy);
- the exact artifact filename;
- the verified SHA-256 of the zip; and
- an explicit `unsigned` status and statement that the build is not notarized.

The manifest, checksum file, Release notes, and uploaded zip must agree. A short
SHA used in filenames or tags does not replace the required full commit SHA.

## 5. Required release notes

Release-page text must start from
[`docs/unsigned_release_notes_template.md`](unsigned_release_notes_template.md)
and retain its safety boundaries. At minimum, the published notes must include:

- the bilingual unsigned and Gatekeeper warning;
- instructions limited to right-click **Open** or approval through **System
  Settings → Privacy & Security**;
- the separately installed ffmpeg prerequisite and the fact that the app does
  not download, install, update, or bundle ffmpeg;
- MiMo requirements for transcript and notes, and MiMo plus DeepSeek
  requirements for minutes;
- the credential and privacy warning, including the prohibition on secrets in
  logs, screenshots, issues, assets, and notes;
- the Apple Silicon architecture limitation, lack of updater/installer/DMG,
  and absence of a support SLA; and
- the official source, source commit, and SHA-256 verification requirement.

The notes must not include command-line security bypass instructions or tell
users to disable macOS security globally.

## 6. Required gates before upload

Before any upload is attempted, the maintainer must confirm:

- [ ] The manual checks in
      [`docs/local_unsigned_artifact_checklist.md`](local_unsigned_artifact_checklist.md)
      have passed for the exact candidate being packaged.
- [ ] The zip's SHA-256 was calculated and independently verified against the
      `.sha256` file.
- [ ] The JSON manifest matches the artifact filename, full source commit,
      version, architecture, build date, checksum, and unsigned status.
- [ ] Release notes, assets, manifests, screenshots, logs, issue references,
      and test records contain no API keys, credentials, or sensitive meeting
      content.
- [ ] No material claims Apple trust, signing, notarization, approval, or
      endorsement.
- [ ] The repository license and bundled third-party notice obligations have
      been reviewed and resolved for the proposed distribution.
- [ ] The required MiMo/DeepSeek data-flow and privacy disclosure is present.
- [ ] The Release is configured as Pre-release and not Latest.

Failure of any gate blocks the upload. The maintainer must not waive a failed
gate by changing the warning text after the fact.

## 7. Asset immutability and withdrawal

Uploaded assets must not be silently replaced, even if the filename and
application version would remain unchanged.

- A broken, compromised, incorrectly documented, or unverifiable asset should
  be withdrawn and clearly identified as withdrawn.
- Release notes should preserve a visible explanation of the withdrawal when it
  is safe to do so.
- A corrected build requires a new unique Release/tag, new build manifest, and
  new SHA-256 file.
- Users must never be expected to distinguish two different binaries that were
  published under the same provenance metadata.

## 8. Forbidden release behavior

The current policy forbids:

- CI or automated GitHub Release publishing;
- CI artifact upload for public distribution;
- live MiMo or DeepSeek API checks in CI;
- Apple credentials, certificates, signing identities, notarization secrets, or
  other Apple secrets in CI or repository configuration;
- signing or notarization under the unsigned policy;
- an updater, DMG, or installer;
- ffmpeg bundling, downloading, or installer-managed setup;
- third-party mirrors presented as official distribution sources; and
- any claim that Gatekeeper friction has been removed.

## 9. Non-goals

PR #17 does not:

- create a zip, checksum, manifest, DMG, installer, or updater;
- add or modify an artifact-packaging script;
- add a release workflow, CI job, artifact upload, or public GitHub Release;
- add Apple Developer Program, signing, or notarization configuration;
- change ffmpeg installation, discovery, or runtime handling;
- run live API checks; or
- change application code, package versions, Tauri configuration, backend
  behavior, or other platform code.

# Phase 5 Internal Acceptance Result — 2026-07-13

## Result

**Passed — no blockers found.** This records manual acceptance of the unsigned
macOS internal app candidate. It is not a public release record.

The commit under test was `main` at
`d1de2c28e5d669cb32d54f2b59034d5fc5b93ebb`, after PR #12 merged.

## PR chain covered

- PR #7: desktop CI/readiness
- PR #8: output root/settings
- PR #9: secure credentials
- PR #10: Go sidecar resolver
- PR #11: app bundle smoke/backend policy
- PR #12: production preflight/acceptance checklist

## Candidate boundary

- Unsigned macOS internal app candidate only.
- No public release or distribution artifact was accepted.
- Signing, notarization, DMG, installer, and updater work were not performed.
- ffmpeg remains a system prerequisite; it was not bundled or downloaded.

## Offline acceptance checks

The following were verified manually; detailed logs are not committed.

- [x] `./scripts/build_desktop_sidecar.sh`
- [x] `./scripts/smoke_desktop_app_bundle.sh`
- [x] Bundled sidecar `version` command
- [x] Launch of the unsigned `.app`
- [x] Production runtime mode and packaged Go backend policy
- [x] Python is not selectable in production
- [x] Production output root behavior
- [x] Readiness panel, including sidecar and ffmpeg status
- [x] Credential status shows metadata only, without credential values
- [x] Project-root `.env` is ignored in production
- [x] Missing packaged sidecar reports `backend_not_found`
- [x] Missing sidecar does not fall back to Python
- [x] No API-key values appeared in the observed logs or output artifacts

## Live acceptance checks

**Skipped / manual-deferred.** No captured live API acceptance record accompanies
this result, so transcript, notes, and minutes live runs are not marked passed.
They remain manual-only checks and must not be added to CI.

## Follow-up

- Blockers: none.
- Non-blocking issues: none observed.
- The next phase should be planned separately; this result does not authorize
  signing, notarization, DMG, installer, updater, ffmpeg bundling, or live API
  checks in CI.

# Phase 6 Unsigned Release Candidate Dry-run Record — 2026-07-13

## 1. Purpose

This document records the local unsigned macOS artifact packaging dry run
performed during PR #18. It is a validation record only, not a public release
record and not authorization to upload or distribute an artifact.

Detailed command output and generated artifacts are not committed. This record
states only the results that were verified during the dry run.

## 2. Baseline and candidate identity

The current documentation baseline is `main` after PR #18, merge commit
`189a162e4c6e130aa4e7c46646391e4c5908b5bd`. The path under evaluation is the
unsigned macOS Apple Silicon (`arm64`) packaging path.

The local packaging smoke itself ran before PR #18 was squash-merged, against
PR head commit `c201b4986a6072752aae866abb829cc727817530`. Its manifest correctly
recorded that source commit and short commit `c201b49`. This record does not
claim that the merge commit was separately repackaged or uploaded.

## 3. Dry-run result

**Passed — no blocker found.** The local packaging smoke produced the expected
unsigned zip, SHA-256 file, and build manifest under the ignored
`dist/unsigned/` directory.

No generated artifact was committed or uploaded, and no public GitHub Release
was created.

## 4. Checks recorded

The following results were verified locally during PR #18; detailed logs are
not committed:

- [x] `bash -n scripts/package_unsigned_macos_app.sh` passed.
- [x] `git diff --check` passed.
- [x] The local packaging script produced only the expected zip, `.sha256`,
      and JSON manifest under ignored `dist/unsigned/`.
- [x] SHA-256 verification of the generated zip passed.
- [x] After extraction, the zip preserved executable permissions on
      `<App>.app/Contents/MacOS/meeting-asr-go`.
- [x] The manifest matched the configured app version, full source commit,
      short commit, `arm64` architecture, artifact filename, and checksum.
- [x] The manifest recorded `unsigned: true` and `notarized: false`.
- [x] Re-running without `--force` refused to overwrite existing outputs.

## 5. Boundaries

This dry run performed or authorized none of the following:

- creating a GitHub Release;
- committing or uploading a generated artifact;
- CI publishing or public artifact upload;
- signing, notarization, or stapling;
- DMG, installer, or updater creation;
- ffmpeg bundling, downloading, or installer-managed setup; or
- live MiMo or DeepSeek checks in CI.

The application, packaging script, CI, Tauri configuration, dependencies, and
runtime behavior are unchanged by this documentation record.

## 6. Unresolved gates before any future upload

The local packaging smoke does not satisfy the release gates by itself. Before
any future upload, a maintainer must still complete and record:

- [ ] repository license and bundled third-party notice review;
- [ ] user-facing notes prepared from
      [`docs/unsigned_release_notes_template.md`](unsigned_release_notes_template.md);
- [ ] final manual app checks from
      [`docs/local_unsigned_artifact_checklist.md`](local_unsigned_artifact_checklist.md)
      against the exact candidate intended for upload;
- [ ] manual transcript and notes checks with MiMo and a non-sensitive,
      authorized test recording, and a manual minutes check with MiMo and
      DeepSeek, if proceeding to external sharing; these checks remain outside
      CI and must not record credentials or sensitive content; and
- [ ] an explicit maintainer decision to create a GitHub **Pre-release** that
      is not **Latest**, after every gate in
      [`docs/github_unsigned_prerelease_policy.md`](github_unsigned_prerelease_policy.md)
      passes.

## 7. Conclusion

The local unsigned packaging path is technically ready for optional manual
use. Public upload remains deferred. Any future public Pre-release requires a
separate review and explicit maintainer action; this record does not authorize
or initiate it.

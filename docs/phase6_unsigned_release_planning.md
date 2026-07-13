# Phase 6 Unsigned Release Planning

## 1. Phase 6 objective

Phase 6 plans a conservative path from the accepted Phase 5 internal candidate
toward limited unsigned macOS distribution. Its purpose is policy definition
and risk reduction, not immediate publication.

Phase 5 passed internal acceptance for an unsigned macOS application candidate.
The project does not currently require Apple Developer Program membership, a
Developer ID certificate, signing, or notarization. Gatekeeper warnings are an
accepted product tradeoff for this phase. Any build shared outside the project
must be clearly and repeatedly labeled as unsigned, and no Phase 6 planning
decision should be represented as Apple trust or approval.

This document does not publish a release or create a distributable artifact.

## 2. Current baseline

The baseline is `main` after PR #13, merge commit
`f4e6ce6532c1beef64da57586a4d83e5471d0bba`:

- The unsigned macOS internal candidate passed Phase 5 acceptance.
- Production is fixed to the packaged Go sidecar and does not fall back to
  Python.
- Python remains the development fallback and reference implementation.
- ffmpeg remains a separately installed system prerequisite.
- Apple Developer Program enrollment, Developer ID signing, and notarization
  are deferred.
- DMG, installer, and updater work remain deferred unless separately planned.
- The first external-test architecture is Apple Silicon (`arm64`). Intel CI
  smoke does not constitute Intel distribution support.
- No minimum macOS version is promised yet. Early packages may claim only the
  macOS environment on which they were actually tested.

## 3. Distribution policy

The current policy is deliberately narrow:

- **Internal use:** an unsigned raw `.app` is acceptable.
- **Small-circle testing:** an unsigned zipped `.app` is acceptable when it is
  built and sent by a maintainer, accompanied by its SHA-256 and source commit,
  and recipients understand the Gatekeeper warning.
- **Public GitHub distribution:** conditionally allowed only as an explicitly
  unsigned GitHub **Pre-release**. It must not be Latest, and it must satisfy all
  release gates below.
- **Trusted source:** public assets must come from this repository's official
  GitHub Releases. Maintainer-to-tester transfer is acceptable for small-circle
  testing. Third-party mirrors are not approved sources.
- No page, filename, screenshot, or note may claim that the app is signed,
  notarized, trusted, or approved by Apple.
- No automatic updater, telemetry, or automatic crash-log upload is allowed.
  Users may submit a minimal, redacted reproduction through GitHub Issues;
  support is best-effort with no service-level commitment.

Before any artifact is shared outside the project, all of the following are
required:

1. A maintainer builds from a clean `main` checkout at a recorded commit on a
   trusted Apple Silicon Mac.
2. The `.app` is launched from Finder and the readiness check confirms that the
   separately installed Homebrew ffmpeg is visible to the real GUI-launched
   process. If it is not visible, distribution stops and path discovery must be
   addressed in a separate, focused PR; ffmpeg bundling is not a workaround.
3. The maintainer manually verifies live transcript, notes, and minutes modes
   with a test recording. The result may be recorded, but credentials, sensitive
   content, and detailed secret-bearing logs must not be committed. Live checks
   must never be added to CI.
4. Before a public Pre-release, the repository license and bundled third-party
   notice obligations are reviewed and resolved.
5. Before a public Pre-release, user-facing documentation explains the MiMo and
   DeepSeek data flows, third-party processing, and the need to avoid uploading
   unauthorized sensitive content.

Production users may enter API keys for the current session. The absence of a
secure-store management UI is a known limitation; command-line Keychain
workarounds are not part of the supported distribution path.

## 4. Artifact options

| Artifact | Current feasibility | User friction | Implementation complexity | Recommendation | In scope now? |
| --- | --- | --- | --- | --- | --- |
| Raw `.app` | Already produced by the internal smoke | High outside the build machine; awkward to transfer safely | Low | Internal use only | Planning only; no new artifact |
| Zipped unsigned `.app` | Feasible with a later local process | Gatekeeper warning and separate ffmpeg setup remain | Low to moderate | Preferred for small-circle testing and any future unsigned Pre-release | Future PR only |
| Unsigned `.dmg` | Technically feasible | Gatekeeper warning remains; a disk image may create a false impression of trust | Moderate | Defer until a concrete need exists | No |
| Signed/notarized `.app` | Not currently feasible without program enrollment and credentials | Lowest Gatekeeper friction | High, including certificate and notarization operations | Revisit only after intentional Apple Developer Program enrollment | No |
| Signed/notarized `.dmg` | Not currently feasible | Low user friction when correctly signed and notarized | High | Defer with signing/notarization | No |
| Installer package | Not needed for the current app | Additional install and removal expectations | High | Do not pursue without a demonstrated requirement | No |

Future unsigned zip artifacts should use:

```text
meeting-asr-desktop-experiment-macos-arm64-<app-version>-unsigned.zip
```

The asset name deliberately retains `experiment`, architecture, and `unsigned`.
A future public Pre-release should contain the zip, a separate SHA-256 file, and
a build manifest recording the app version, full source commit, build date,
architecture, and unsigned status.

Published assets are immutable. They must never be silently replaced. A broken
artifact should be marked withdrawn; a correction uses a new unique Release and
new integrity metadata.

## 5. Gatekeeper user instructions

The public warning should be provided in English and Chinese.

### English draft

> This macOS build is unsigned and has not been notarized by Apple. macOS may
> block it with an unidentified developer warning. Run it only if you trust the
> source and have verified the published SHA-256 checksum. If you choose to
> proceed, right-click the app and select **Open**, or approve it in **System
> Settings → Privacy & Security**. This warning is expected for unsigned builds.
> Do not disable macOS security globally.

### 中文草案

> 此 macOS 构建未签名，也未经过 Apple 公证。macOS 可能以“无法验证开发者”或
> “身份不明的开发者”为由阻止打开。仅当你信任下载来源并已核对发布的 SHA-256
> 校验值时才应运行。如果决定继续，可右键点击应用并选择“打开”，或前往“系统
> 设置 → 隐私与安全性”手动批准。此提示是 unsigned 构建的预期行为。请勿全局
> 关闭 macOS 安全保护。

Supported instructions are limited to those macOS user-interface paths. Release
documentation must not provide `xattr`, `spctl`, or other command-line bypasses,
and must never instruct users to disable Gatekeeper globally.

## 6. ffmpeg strategy

| Strategy | Benefits | Costs and risks | Current recommendation |
| --- | --- | --- | --- |
| System prerequisite | No binary redistribution, smaller app, current behavior | Users install it separately; Finder PATH visibility must be verified | Keep |
| User-provided path | Could handle nonstandard installations | Requires new runtime and UI behavior plus path validation | Defer |
| Bundled ffmpeg | Simplifies discovery for users | Licensing, size, update, architecture, and distribution complexity | Defer |
| Installer-managed ffmpeg | Could automate setup | Requires an installer and lifecycle/privilege decisions | Defer |

For now, Homebrew is the recommended macOS installation path:

```bash
brew install ffmpeg
ffmpeg -version
```

These are user-run prerequisite commands, not application automation. The app
must not download or install ffmpeg. Documentation may be improved in a later
docs PR, but bundling remains deferred because of licensing, binary size, and
distribution complexity.

## 7. Versioning and release notes

This planning PR does not change the application version or Tauri configuration.
Future artifacts continue to use the configured application version. A unique
Pre-release tag should use:

```text
desktop-v<app-version>-<YYYYMMDD>-<short-sha>
```

The Release page and build manifest must record the full commit, build date,
architecture, unsigned status, and SHA-256. The asset filename may retain only
the configured app version because the Release and manifest provide the unique
build identity.

Release notes must list at least these limitations:

- The build is unsigned and not notarized; a Gatekeeper warning is expected.
- The package supports Apple Silicon only unless a later release says otherwise.
- No minimum macOS version is guaranteed beyond the recorded test environment.
- ffmpeg must be installed separately and visible to a Finder-launched app.
- Live use requires user-provided MiMo and, for minutes, DeepSeek credentials.
- API keys can be entered for the current session; secure-store management UI is
  not yet provided.
- There is no updater, telemetry, automatic crash reporting, or support SLA.
- The build is experimental and carries no warranty for broad public use.

## 8. CI and artifacts

Current CI policy remains unchanged:

- CI is offline-only.
- CI does not upload public release artifacts.
- CI does not use Apple credentials or secrets.
- CI does not sign, notarize, or produce release installers.
- CI does not run live MiMo or DeepSeek checks.

A later PR may document a local artifact command or add a manual process only
after this unsigned release policy is accepted. PR #14 does not implement that
path. Any future public Pre-release remains a maintainer-controlled manual
operation rather than a CI release workflow.

## 9. Security and privacy expectations

- API-key values must not appear in logs, task folders, output artifacts,
  manifests, release notes, screenshots, issue reports, or checksums.
- Credentials remain session- or secure-store-based. Project-root `.env` is
  development-only, and production must not read it.
- User-facing material must explain which content is sent to MiMo and DeepSeek
  before a public Pre-release is allowed.
- No telemetry, credential collection, or automatic crash-log collection is
  introduced. Voluntary issue reports must contain only minimal, redacted
  reproduction information.
- Unsigned distribution increases the user's trust burden. Official source,
  source commit, build manifest, and SHA-256 are compensating provenance
  controls, not substitutes for Apple signing or notarization.

## 10. Proposed future PR sequence

- **PR #15 — Unsigned release notes and user warnings:** add bilingual
  Gatekeeper instructions, privacy/data-flow disclosure, known limitations, and
  best-effort support policy.
- **PR #16 — Local unsigned artifact checklist:** document the trusted Apple
  Silicon build host, clean `main`, Finder launch and ffmpeg check, and manual
  transcript/notes/minutes acceptance.
- **PR #17 — Optional local zip packaging script:** generate the unsigned zip,
  SHA-256, and build manifest locally. Do not upload, sign, or create a CI
  release.
- **PR #18 — GitHub Pre-release policy:** record the licensing gate, official
  source, immutable-asset and withdrawal rules, tag format, and required asset
  set.
- **PR #19 — Optional manual unsigned GitHub Pre-release:** if all gates pass,
  document or perform the maintainer-controlled Pre-release process. It must not
  be Latest and must not use a CI artifact workflow.
- **PR #20 — Revisit Apple trust tooling if justified:** consider Apple
  Developer Program enrollment, signing, and notarization only if real external
  users create a demonstrated need.

Signing and notarization are not required for the current internal and
small-circle path.

## 11. Non-goals

PR #14 does not:

- publish a release or upload a public artifact;
- create a zip, DMG, installer, or updater;
- add signing, notarization, Developer ID, or Apple Developer Program setup;
- bundle, download, or automatically install ffmpeg;
- change runtime behavior, application versions, Tauri configuration, scripts,
  or CI;
- change Python or Go core behavior;
- change WebUI, mobile, cloud, or database code;
- add telemetry, crash-log collection, or live API checks;
- claim Apple trust or remove the expected Gatekeeper warning.

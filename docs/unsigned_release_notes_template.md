# Meeting ASR Desktop Experiment v<version> — unsigned macOS pre-release

## Summary

This is an experimental Apple Silicon macOS build. It is **unsigned** and has
**not been notarized by Apple**, so a Gatekeeper or unidentified-developer
warning is expected. Run it only if you trust the source and have verified the
published SHA-256 checksum.

ffmpeg must be installed separately. Live transcript and notes use a
user-provided MiMo API key; live meeting minutes use user-provided MiMo and
DeepSeek API keys. This Pre-release does not include those credentials.

## Downloads and provenance

- `meeting-asr-desktop-experiment-macos-arm64-<version>-unsigned.zip`
- `meeting-asr-desktop-experiment-macos-arm64-<version>-unsigned.zip.sha256`
- `build-manifest-<version>-<short-sha>.json`

Source commit: `<full-commit-sha>`

Build date: `<YYYY-MM-DD>`

Download these files only from the official GitHub Release in the
`SkyBridgeM/meet-asr` repository. Verify the zip against the published SHA-256
before opening it. Third-party mirrors are not approved distribution sources.

## Gatekeeper warning / Gatekeeper 警告

### English

This build is unsigned and has not been notarized by Apple. macOS may block it
with an unidentified-developer warning. Run it only if you trust the official
source and have verified its SHA-256 checksum.

If you choose to proceed, right-click the app and select **Open**, or approve it
in **System Settings → Privacy & Security**. This warning is expected for an
unsigned build. Do not disable macOS security globally or use command-line
security bypasses.

### 中文

此构建未签名，也未经过 Apple 公证。macOS 可能以“无法验证开发者”或“身份不明的
开发者”为由阻止打开。仅当你信任官方来源并已核对 SHA-256 校验值时才应运行。

如果决定继续，可右键点击应用并选择“打开”，或前往“系统设置 → 隐私与安全性”
手动批准。此提示是 unsigned 构建的预期行为。请勿全局关闭 macOS 安全保护，也
不要使用命令行方式绕过系统安全检查。

## ffmpeg prerequisite

ffmpeg is a separately installed system prerequisite and must be available to
the application. If Homebrew is already installed, the documented setup is:

```bash
brew install ffmpeg
ffmpeg -version
```

These commands are run manually by the user. The application does not download,
install, update, or bundle ffmpeg. User-provided ffmpeg paths and bundled or
installer-managed ffmpeg remain deferred.

## Credentials and privacy

- Transcript and notes modes require `MIMO_API_KEY`.
- Minutes mode requires both `MIMO_API_KEY` and `DEEPSEEK_API_KEY`.
- Never include API-key values in screenshots, logs, issue reports, task
  folders, release notes, or build manifests.
- Production does not read the project-root `.env`; that source is
  development-only.
- The application does not add telemetry or automatic crash-log upload.
- Audio used for transcript or notes is sent to MiMo. Minutes additionally send
  transcript content to DeepSeek. Do not process unauthorized sensitive
  recordings or meeting content.

## Known limitations

- The app is unsigned and not notarized; Gatekeeper warnings are expected.
- The package supports Apple Silicon only unless this Release explicitly says
  otherwise.
- ffmpeg must be installed separately and visible to the app.
- API keys are required for live use.
- There is no updater, installer, or DMG.
- There is no telemetry or automatic crash reporting.
- Support is best-effort and has no service-level agreement.
- This experimental build carries no warranty for broad public use.

## Maintainer checklist

- [ ] The Release title and zip filename clearly include `unsigned`.
- [ ] The GitHub Release is marked **Pre-release** and is not **Latest**.
- [ ] The zip has a matching `.sha256` file and the checksum was verified.
- [ ] The JSON build manifest is present and matches the uploaded zip.
- [ ] The full source commit, build date, architecture, app version, and unsigned
      status are recorded.
- [ ] The files, notes, screenshots, and issue references contain no secrets or
      sensitive meeting content.
- [ ] The notes make no claim of Apple signing, notarization, trust, or approval.
- [ ] No live API key or live MiMo/DeepSeek check was added to CI.
- [ ] No updater, installer, DMG, signing, notarization, or ffmpeg bundling was
      introduced for this Release.

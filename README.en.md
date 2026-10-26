# Voxly

[简体中文](README.md)

Voxly turns recorded interviews and meetings into text you can organize on your Mac. It supports local SenseVoice transcription, MiMo cloud recognition, and optional DeepSeek summaries, meeting notes and action items.

Version `0.1.0-alpha` is an early desktop release for Apple Silicon Macs. The installer has local ad-hoc signatures, but no Apple Developer ID signature or notarization.

## Install

Download `Voxly_0.1.0-alpha_macos_arm64.dmg` from [GitHub Releases](https://github.com/SkyBridgeM/voxly/releases/tag/v0.1.0-alpha). Open it and drag Voxly into Applications. The app includes FFmpeg 9.0.2, the Go core and the native recognition runtime. Users do not need Homebrew, Python or Go.

macOS may block this unnotarized build. Check the download source and the SHA-256 values in the release. This installer has been checked on Apple Silicon; this release has no validated Intel Mac or Windows package.

## Transcribe a recording

1. Open Settings, then Models & connections. Download or import SenseVoice. The model download is about 156 MiB and requires internet access; the installer does not include the weights.
2. Under Transcription, choose local recognition. Chinese is the default language. Use automatic detection for recordings in multiple languages.
3. Enable VAD if you want segmentation based on pauses, then save. New tasks use this setting; resumed tasks keep their recorded language and segment plan.
4. Import an M4A, MP3 or WAV file, choose transcription and start. Open the result in the Task Library.

You can import M4A directly. Voxly decodes it locally before recognition, so there is no need to convert it yourself. Long recordings run in segments, and resume reuses completed text checkpoints.

## Keep the files you need

Each task has its own folder with an audio copy, transcript, processing state and generated content. VAD tasks also save a timed transcript and segment JSON. Times describe speech intervals rather than individual words.

Settings → Storage shows disk usage and offers batch chunk cleanup. Automatic chunk cleanup is off by default. When enabled, it applies to tasks that finish successfully afterward; interrupted and failed tasks retain their audio.

After checking a transcript, you can choose Task Library → Task details → Audio & cache → Keep text only. Review the file list and reclaimed size before confirming. Voxly removes the task's audio copy and chunks while keeping text, timing data, generated versions and task metadata. It leaves the external file you imported untouched. Deletion cannot be undone. Playback or another recognition pass requires a separate recording copy.

## Privacy

| Choice | Data sent to a provider |
| --- | --- |
| Local SenseVoice transcription | No audio upload or API key; model download requires internet access |
| MiMo recognition | Audio goes to MiMo; credentials required |
| DeepSeek content generation | Transcript text goes to DeepSeek; credentials required |

Local transcription runs on your Mac after model installation. Selecting cloud generation still sends transcript text off-device. Production credentials come from the current session or the OS secure store, never the repository's `.env` file.

## Limits

The desktop handles existing recordings. It has no microphone capture or streaming recognition. Speaker diarization is not implemented; the CAM++ boundary is only a reserved code interface.

Names, technical terms and short replies can be misrecognized. Check important passages before deleting audio. Local sample checks and package checks have passed, but installation on a separate clean Mac still needs validation.

## Development

The desktop uses Tauri, React and a Go sidecar, with a native sherpa-onnx adapter for local recognition. Install Node.js/npm, Rust, Go, Python 3.11+ for build tooling, and Xcode Command Line Tools, then run from the repository root:

```bash
npm --prefix desktop ci
./scripts/build_desktop_sidecar.sh
go build -C core-go -o meeting-asr-go .
npm --prefix desktop run tauri -- dev
```

The first build downloads pinned native dependencies and FFmpeg sources. Install SenseVoice through the app. Run `./scripts/build_macos_release.sh` to prepare an installer; it does not commit, push or publish a release.

- [Desktop development and packaging](desktop/README.md)
- [Go core and CLI](core-go/README.md)
- [Local model management](docs/architecture/local-model-management.md)
- [Release notes](docs/releases/0.1.0-alpha.md)
- [Legacy Python CLI and WebUI](docs/legacy-python.md)

The Python implementation is retained for maintenance and reference. Dated plans and WORKLOG entries describe earlier work rather than the current release.

# Voxly desktop

Voxly is a Tauri/React app with a Go processing sidecar. The macOS Alpha ships
four executables: the UI, Go core, SenseVoice adapter and FFmpeg. Production
builds use these packaged files and do not fall back to a project checkout,
Python or a Homebrew installation.

For installation and first use, read the [Chinese](../README.md) or
[English](../README.en.md) project guide.

## Settings

| Category | Contents |
| --- | --- |
| General | UI language and theme |
| Transcription | Recognition and generation providers, local ASR language, VAD |
| Models & connections | SenseVoice installation, download settings and credentials |
| Templates | Built-in and custom content templates |
| Storage | Output folder, disk usage and cleanup |
| About & diagnostics | Version, runtime mode, dependency checks and development backend |

Category changes keep unsaved form input and reset the content scroll position.
Closing and reopening Settings remembers the category for the current app
session. A narrow window uses horizontal navigation. Each section keeps its
own save behavior. Storage mutations temporarily disable navigation and closing.

Local ASR defaults to Chinese. Automatic language detection is an explicit
choice; there is no automatic retry in another language. VAD is optional and
defaults off for new settings. A resumed task uses its saved language and plan.

## Tasks and storage

New tasks create a canonical `transcript.md`. Generated content goes into
versioned files under `generated/`; generating another version does not replace
the transcript. VAD adds `transcripts/transcript-timed.md` and
`transcripts/vad-segments.json`, which the Task Library can open or reveal.
These times are speech intervals, not word timestamps or speaker changes.

Settings → Storage estimates logical file sizes and previews batch chunk
cleanup. Task details → Audio & cache offers two scopes:

- Chunk cleanup retains the task's source audio and all text. Automatic cleanup
  defaults off and runs only when a task finishes successfully afterward.
- Keep text only removes the task's source copy and known chunks after verifying
  completed text and VAD timing artifacts. The user must preview and confirm.
  External imported files, text checkpoints, generations and task metadata stay.

Keep text only cannot be undone. Another recognition pass needs another audio
copy; text-based generation remains available. Failed or interrupted tasks do
not qualify. Cleanup holds the task lock, checks a preview token again, and
records intent before deleting files so an interrupted operation can be retried.
Unknown files and links block cleanup rather than being removed.

Startup temporary cleanup defaults on. It reclaims only known model-manager
files older than seven days under model locks. It keeps installed models and
unknown files. Neither startup cleanup nor batch cleanup deletes source audio.

## Credentials and privacy

Rust resolves credentials and sends only the necessary values to child
processes. Production uses session credentials or the OS secure store, never
project `.env` files. Development also supports `.env` compatibility. Secrets
must not reach React responses, manifests, readiness messages or logs.

SenseVoice processes audio locally and needs no API key. MiMo receives audio
when selected. DeepSeek receives transcript text when generation is requested.
The UI derives its privacy message from provider metadata rather than treating
all local-ASR tasks as fully offline workflows.

## Development

Use an Apple Silicon Mac with Node.js/npm, Rust, the Go version declared in
[`go.mod`](../core-go/go.mod), Python 3.11+ for build tooling, and Xcode Command
Line Tools. Run from the repository root:

```bash
npm --prefix desktop ci
./scripts/build_desktop_sidecar.sh
go build -C core-go -o meeting-asr-go .
npm --prefix desktop run tauri -- dev
```

The sidecar script builds the native adapter, portable FFmpeg and third-party
notices. It downloads pinned dependencies on the first run. Install SenseVoice
in Settings. Standalone development may use FFmpeg from PATH if no sibling
runtime exists.

Development defaults to Go and can explicitly select the deprecated Python
backend. The production build permits only Go. An absolute
`MEETING_ASR_GO_BINARY` override is available in development; production ignores
it. Model weights live under the app data directory, separate from task folders.
Changing the output folder does not move existing tasks.

## Build an installer

```bash
./scripts/build_macos_release.sh
```

The script prepares a production app, applies local ad-hoc signatures, and
creates a drag-to-Applications DMG. Its new output directory under
`.runtime-build/releases/` contains release notes, SHA-256 values and the
matching FFmpeg/LAME source attachment. It does not commit, push or publish.
The manual **Build macOS Alpha package** Actions workflow uploads the same
files as an Actions artifact for review.

FFmpeg 9.0.2 is built with the audio codecs and filters used by Voxly. Its dynamic
imports must resolve only to macOS system libraries. Production checks the
packaged executable and fails readiness if it is missing. The Go core also
resolves FFmpeg beside itself, so Finder launches do not depend on shell PATH.

Ad-hoc signing is not Developer ID signing or Apple notarization. The current
installer has been extracted and launched locally, and its packaged runtime has
passed a short sample with Homebrew removed from PATH. A separate clean Mac has
not yet been used for installation validation.

## Checks

Run the checks relevant to the change:

```bash
npm --prefix desktop run build
(cd core-go && go test ./... && go vet ./...)
(cd desktop/src-tauri && cargo fmt --check && cargo test)
```

`./scripts/smoke_desktop_app_bundle.sh` builds an app bundle and checks its
packaged executables and notices. Packaging-only updates need a short runtime
sample; they do not require repeating a long interview transcription. Python
regression tests apply when changing the legacy implementation or its contracts.

Keep generated sidecars, build trees, model weights, recordings, task output and
credentials out of source commits. Release attachments belong in GitHub Releases.

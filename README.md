# meeting-asr

`meeting-asr` is a local-first Python CLI for recording or importing meeting audio, splitting it into upload-safe chunks, transcribing each chunk with MiMo ASR, and merging the result into a clean `transcript.md`.

The main stable workflow is transcription. In v1, `--summarize` prepares `notes_input.md` from a prompt template and the transcript; it does not call an LLM or generate `meeting_minutes.md` yet.

## Installation

Create and activate a virtual environment:

```bash
python -m venv .venv
source .venv/bin/activate
```

Install the project:

```bash
pip install -e .
```

For tests:

```bash
pip install -e ".[dev]"
```

For the optional local WebUI:

```bash
pip install -e ".[webui]"
```

For both tests and WebUI:

```bash
pip install -e ".[all]"
```

You can then run:

```bash
meeting-asr --help
```

The module entrypoint also works after installation:

```bash
python -m meeting_asr.main --help
```

## Environment Setup

Copy the example file and add your MiMo API key:

```bash
cp .env.example .env
```

Set:

```text
MIMO_API_KEY=your_api_key_here
```

Optional values:

```text
MIMO_ASR_ENDPOINT=https://api.xiaomimimo.com/v1/chat/completions
MIMO_ASR_MODEL=mimo-v2.5-asr
MAX_UPLOAD_MB=10
DEFAULT_CHUNK_MINUTES=5
MIMO_RETRY_COUNT=3
MIMO_TIMEOUT_SECONDS=120
```

API keys and configurable values are read from `.env`. Do not put secrets in source files.

## ffmpeg

This project uses `ffmpeg` for recording and splitting audio.

If Homebrew is already installed, install ffmpeg on macOS:

```bash
brew install ffmpeg
```

If `brew` is not available yet, install Homebrew first from:

```text
https://brew.sh/
```

Then run:

```bash
brew install ffmpeg
```

Verify:

```bash
ffmpeg -version
```

The CLI checks for `ffmpeg` before recording or splitting. If it is missing, it prints the relevant Homebrew instructions.

## macOS Microphone Permission

Recording uses ffmpeg's `avfoundation` input. macOS must allow microphone access for the terminal app you run `meeting-asr` from.

If recording fails immediately, check:

```text
System Settings -> Privacy & Security -> Microphone
```

Allow access for Terminal, iTerm, VS Code, or whichever app launches the command.

The default input device is `:0`. You can override it:

```bash
meeting-asr record --input-device ":1"
```

To inspect available avfoundation devices:

```bash
ffmpeg -f avfoundation -list_devices true -i ""
```

## Usage

Transcribe an existing file:

```bash
meeting-asr transcribe input/meeting.mp3
```

Prepare notes input after transcription:

```bash
meeting-asr transcribe input/meeting.mp3 --summarize
```

Use a custom notes prompt:

```bash
meeting-asr transcribe input/meeting.mp3 --summarize --prompt prompts/default_minutes.md
```

Change chunk length:

```bash
meeting-asr transcribe input/meeting.mp3 --chunk-minutes 10
```

Choose output directory:

```bash
meeting-asr transcribe input/meeting.mp3 --output-dir "/Users/yourname/Documents/MeetingNotes"
```

Record for a fixed duration:

```bash
meeting-asr record --duration 60
```

Record and prepare notes input:

```bash
meeting-asr record --duration 60 --summarize
```

Record 90 minutes in 5-minute chunks:

```bash
meeting-asr record --duration 90 --chunk-minutes 5 --output-dir "/Users/yourname/Documents/MeetingNotes"
```

Record until Ctrl+C:

```bash
meeting-asr record --summarize
```

When no duration is provided, Ctrl+C stops recording and then continues with transcription for the completed chunks.

## Local WebUI

The WebUI is an optional local interface for the upload-based transcription workflow. It is useful when you prefer uploading audio and choosing settings from a browser instead of typing a full command.

Install the WebUI dependency:

```bash
pip install -e ".[webui]"
```

Run:

```bash
streamlit run webui/app.py
```

Or use the convenience scripts:

```bash
./scripts/webui.sh
./scripts/start_webui.sh
./scripts/stop_webui.sh
./scripts/status_webui.sh
./scripts/toggle_webui.sh
```

The simplest option is `webui.sh`: it starts the WebUI in the current terminal, and `Ctrl+C` stops it.

`start_webui.sh` starts the WebUI in the background, opens `http://127.0.0.1:8501`, and writes logs to `.streamlit/webui.log`. Use `stop_webui.sh` to close that background service.

For one-click macOS use, double-click:

```text
启动 meeting-asr.command
关闭 meeting-asr.command
```

You can keep `启动 meeting-asr.command` on your Desktop or another easy-to-click location. If macOS blocks the file, right-click it and choose Open once, or run:

```bash
chmod +x "启动 meeting-asr.command" "关闭 meeting-asr.command"
```

If you do not want the script to open a browser automatically:

```bash
MEETING_ASR_NO_OPEN=1 ./scripts/start_webui.sh
```

Check or toggle the background WebUI:

```bash
./scripts/status_webui.sh
./scripts/toggle_webui.sh
```

If `.env` is missing, the start script prints a warning but still opens the WebUI. Transcription requires `MIMO_API_KEY` in `.env`.

The WebUI supports:

- uploading `mp3` or `wav` audio;
- choosing output directory;
- choosing chunk length;
- setting max upload size for the current run;
- generating `notes_input.md` from a selected prompt template;
- previewing `transcript.md`;
- downloading `transcript.md`, `notes_input.md`, `meeting_minutes.md` if present, and a zip of text outputs.

The WebUI does not expose your API key. It only shows whether `MIMO_API_KEY` was detected from `.env`.

The WebUI does not record audio in this MVP. Recording remains CLI-first. Use:

```bash
meeting-asr record --duration 60 --chunk-minutes 5
```

Privacy note: audio is sent to MiMo ASR when transcription runs. Do not upload sensitive meetings unless you are allowed to use cloud ASR.

## Output Layout

If `--output-dir` is not passed, output goes under `output/`.

Each run creates one timestamped task folder:

```text
output/
  20260707_143000_meeting_name/
    audio/
    chunks/
    transcripts/
      part_001.txt
      part_002.txt
    transcript.md
    notes_input.md
```

For recordings:

```text
output/
  20260707_143000_recording/
```

Final filenames are stable inside the task folder:

- `transcript.md`
- `notes_input.md`, only when `--summarize` is passed

## Resumable Transcription

Each chunk transcript is saved separately:

```text
transcripts/part_001.txt
transcripts/part_002.txt
```

If a part transcript already exists and is non-empty, `meeting-asr` skips that chunk. Empty part transcripts are treated as incomplete and are transcribed again.

Resume an existing task folder without copying audio or splitting again:

```bash
meeting-asr resume output/20260707_143000_meeting_name
```

Resume and regenerate `notes_input.md`:

```bash
meeting-asr resume output/20260707_143000_meeting_name --summarize
```

`resume` first reads existing chunks from `chunks/part_*.mp3` or `chunks/part_*.wav`. If `chunks/` has no chunks, it falls back to `audio/part_*.wav` for recorded tasks. Chunk files must start at `part_001` and be numbered continuously.

After transcription, `resume` always rebuilds `transcript.md`. When `--summarize` is passed, it overwrites `notes_input.md` using the selected prompt template.

`resume` uses the configured `MAX_UPLOAD_MB` value from `.env` to validate existing chunk sizes before upload. Adjust `MAX_UPLOAD_MB` in `.env` if an existing chunk is larger than the current limit.

## Chunking and Upload Limits

Existing `mp3` and `wav` files are split with `ffmpeg`.

Defaults:

- chunk length: 5 minutes
- max upload size: 10 MB
- minimum automatic re-split chunk length: 60 seconds

If a chunk from an imported file is too large, the splitter automatically halves the chunk duration and retries until all chunks fit or the 60-second minimum is reached.

For recorded audio, chunks are captured directly as 16 kHz mono wav files and checked before upload. If a recorded chunk exceeds the limit, rerun with a smaller `--chunk-minutes` value or increase `MAX_UPLOAD_MB`.

## Notes Prompt Templates

Meeting-note structure is stored in Markdown prompt files, not Python code.

Default:

```text
prompts/default_minutes.md
```

Custom:

```bash
meeting-asr transcribe input/meeting.mp3 --summarize --prompt prompts/my_template.md
```

In v1, this creates `notes_input.md` containing the prompt and transcript. A future LLM provider can consume that file to create `meeting_minutes.md`.

## Extension Points

- Add other ASR providers by following the boundary in `src/meeting_asr/transcribe_mimo.py`.
- Add LLM meeting-minutes generation behind `src/meeting_asr/summarize.py`.
- Add new note formats by creating more files under `prompts/`.
- Keep workflow orchestration in `src/meeting_asr/main.py` and provider details inside provider modules.

## Current MiMo Payload Note

MiMo ASR calls are isolated in `transcribe_mimo.py`. The request body follows MiMo's speech-recognition schema with a data URL in `input_audio.data` and `asr_options.language` set to `auto`. If MiMo's production schema changes, adjust `build_mimo_asr_payload()` without changing the CLI workflow.

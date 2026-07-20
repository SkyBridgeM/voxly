# Voxly / 声笺

**Local Voice Workspace · 本地语音工作台**

Voxly turns imported voice recordings into structured, traceable content. The current desktop alpha uses the Go runtime, MiMo cloud speech recognition, and optional DeepSeek cloud generation. Tasks, manifests, transcripts, generated content, metadata, and custom templates remain in user-controlled local storage.

Current workflow:

1. Import an existing audio file.
2. Create a canonical transcript with MiMo ASR.
3. Optionally generate versioned content with a built-in or custom prompt template through DeepSeek.
4. Browse, organize, open, and reveal results in the Task Library.

Local SenseVoice/Whisper transcription and Ollama generation are planned; they are not included in `0.1.0-alpha`. Voxly does not currently claim fully local processing, built-in recording, or bundled local models.

The actively supported desktop targets are macOS arm64 development builds. Windows x64 is an intended release target but is not yet confirmed as a packaged alpha in this repository.

## Desktop development

Internal repository, Go binary, and package names intentionally remain `meet-asr` / `meeting-asr-go`.

```bash
cd core-go
go build -o /tmp/meeting-asr-go .
cd ..
MEETING_ASR_GO_BINARY=/tmp/meeting-asr-go npm --prefix desktop run tauri -- dev
```

The desktop requires `ffmpeg` on `PATH`. MiMo credentials are required for transcription; DeepSeek credentials are required only when generated content is requested. Production credential storage uses the operating system secure credential store.

## Legacy Python CLI

The Python CLI remains in the repository as a deprecated development boundary. It is not the active production desktop runtime. The following instructions are retained for contributors working on that legacy implementation.

### Installation

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

Copy the example file and add your keys:

```bash
cp .env.example .env
```

Set:

```text
MIMO_API_KEY=your_mimo_api_key_here
DEEPSEEK_API_KEY=your_deepseek_api_key_here
```

Optional values:

```text
# MiMo ASR
MIMO_ASR_ENDPOINT=https://api.xiaomimimo.com/v1/chat/completions
MIMO_ASR_MODEL=mimo-v2.5-asr

# Upload and retry
MAX_UPLOAD_MB=10
DEFAULT_CHUNK_MINUTES=5
MIMO_RETRY_COUNT=3
MIMO_TIMEOUT_SECONDS=120

# LLM minutes generation
MINUTES_PROVIDER=deepseek
DEEPSEEK_ENDPOINT=https://api.deepseek.com/chat/completions
DEEPSEEK_MODEL=deepseek-v4-flash
LLM_TIMEOUT_SECONDS=120
MINUTES_PROMPT_PATH=prompts/default_minutes.md
OBSIDIAN_TEMPLATE_PATH=prompts/obsidian_minutes.md
```

API keys and configurable values are read from `.env`. Do not put secrets in source files.

MiMo is used for ASR transcription. DeepSeek is used for LLM meeting minutes generation.

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

### Output Modes

`meeting-asr` supports three output modes:

- **`transcript`** (default): Only generates `transcript.md`. Audio is sent to MiMo ASR.
- **`notes`**: Generates `transcript.md` + `notes_input.md` (prompt + transcript). No LLM is called.
- **`minutes`**: Generates `transcript.md` + `notes_input.md` + `meeting_minutes.md` via LLM. Transcription text is sent to DeepSeek.

The old `--summarize` flag is still accepted and equivalent to `--output-mode notes`, but is deprecated. When both `--summarize` and `--output-mode` are passed, `--output-mode` takes priority.

### Transcribe an Existing File

Basic transcription:

```bash
meeting-asr transcribe input/meeting.mp3
```

Generate notes input (prompt + transcript):

```bash
meeting-asr transcribe input/meeting.mp3 --output-mode notes
```

Generate LLM meeting minutes:

```bash
meeting-asr transcribe input/meeting.mp3 --output-mode minutes
```

Use `deepseek-v4-pro` for higher quality:

```bash
meeting-asr transcribe input/meeting.mp3 --output-mode minutes --llm-model deepseek-v4-pro
```

Custom Obsidian template:

```bash
meeting-asr transcribe input/meeting.mp3 --output-mode minutes --obsidian-template prompts/obsidian_minutes.md
```

Change chunk length:

```bash
meeting-asr transcribe input/meeting.mp3 --chunk-minutes 10
```

Choose output directory:

```bash
meeting-asr transcribe input/meeting.mp3 --output-dir "/Users/yourname/Documents/MeetingNotes"
```

### Recording

Record for a fixed duration:

```bash
meeting-asr record --duration 60
```

Record with LLM minutes:

```bash
meeting-asr record --duration 60 --output-mode minutes
```

Record 90 minutes in 5-minute chunks:

```bash
meeting-asr record --duration 90 --chunk-minutes 5 --output-dir "/Users/yourname/Documents/MeetingNotes"
```

Record until Ctrl+C:

```bash
meeting-asr record --output-mode notes
```

When no duration is provided, Ctrl+C stops recording and then continues with transcription for the completed chunks.

### Resume

Resume an existing task (skips already-transcribed chunks):

```bash
meeting-asr resume output/20250707_143000_meeting_name
```

Regenerate notes or minutes after resume:

```bash
meeting-asr resume output/20250707_143000_meeting_name --output-mode minutes
```

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

If `.env` is missing, the start script prints a warning but still opens the WebUI. Transcription requires `MIMO_API_KEY` in `.env`. LLM minutes generation requires `DEEPSEEK_API_KEY` in `.env`.

### WebUI Output Modes

The WebUI supports two output modes:

- **输出原文** (Output transcript): Only generates `transcript.md`. Audio is sent to MiMo ASR.
- **生成总结** (Generate summary): Calls DeepSeek to generate `meeting_minutes.md` in addition to `transcript.md` and `notes_input.md`. Transcription text is sent to DeepSeek.

Privacy warnings are shown for each mode, explaining which provider receives your data.

### WebUI API Keys

API keys are managed in a collapsible "API keys" section in the sidebar (collapsed by default):

- **MIMO API key（用于 ASR 转写）**: Detects `.env` key status, supports temporary override and save.
- **DeepSeek API key（用于 LLM 总结）**: Detects `.env` key status, supports temporary override and save.

Keys are never displayed in plain text. You can save each key to `.env` for future use.

### WebUI LLM Settings

When "生成总结" mode is selected, the sidebar shows:

- Provider dropdown (currently only DeepSeek).
- Model dropdown: `DeepSeek V4 Flash（更快/更省）` or `DeepSeek V4 Pro（质量更高）`.
- "保存为默认总结服务商" button to persist the selected model to `.env`.

The WebUI does not expose prompt/template path settings. Custom templates must be configured via CLI or `.env` defaults.

The WebUI does not record audio. Recording remains CLI-first. Use:

```bash
meeting-asr record --duration 60 --chunk-minutes 5
```

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
    meeting_minutes.md
```

For recordings:

```text
output/
  20260707_143000_recording/
```

Final filenames are stable inside the task folder:

- `transcript.md` — always generated
- `notes_input.md` — generated in `notes` and `minutes` modes
- `meeting_minutes.md` — generated in `minutes` mode, wrapped with Obsidian template

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
meeting-asr resume output/20260707_143000_meeting_name --output-mode notes
```

Resume and regenerate LLM minutes:

```bash
meeting-asr resume output/20260707_143000_meeting_name --output-mode minutes
```

`resume` first reads existing chunks from `chunks/part_*.mp3` or `chunks/part_*.wav`. If `chunks/` has no chunks, it falls back to `audio/part_*.wav` for recorded tasks. Chunk files must start at `part_001` and be numbered continuously.

After transcription, `resume` always rebuilds `transcript.md`. When `--output-mode notes` or `--output-mode minutes` is used, it overwrites `notes_input.md` and/or `meeting_minutes.md`.

`resume` uses the configured `MAX_UPLOAD_MB` value from `.env` to validate existing chunk sizes before upload. Adjust `MAX_UPLOAD_MB` in `.env` if an existing chunk is larger than the current limit.

## Chunking and Upload Limits

Existing `mp3`, `wav`, and macOS Voice Memos `m4a` files are split with `ffmpeg`.
`m4a` inputs are converted to 16 kHz mono `wav` chunks before upload.

Defaults:

- chunk length: 5 minutes
- max upload size: 10 MB
- minimum automatic re-split chunk length: 60 seconds

If a chunk from an imported file is too large, the splitter automatically halves the chunk duration and retries until all chunks fit or the 60-second minimum is reached.

For recorded audio, chunks are captured directly as 16 kHz mono wav files and checked before upload. If a recorded chunk exceeds the limit, rerun with a smaller `--chunk-minutes` value or increase `MAX_UPLOAD_MB`.

## LLM Minutes Generation

When using `--output-mode minutes`, the workflow:

1. Transcribes audio with MiMo ASR.
2. Generates `notes_input.md` from the prompt template and transcript.
3. Sends `notes_input.md` to DeepSeek with a fixed system prompt (bottom-line constraints only — no fabrication, mark uncertainties, Markdown output).
4. The default system prompt enforces: only generate from transcript, don't invent facts, mark uncertainties, output Markdown.
5. The LLM response is wrapped with the Obsidian template (placeholder substitution: `{{ title }}`, `{{ generated_at }}`, `{{ generated_minutes }}`, `{{ transcript }}`, `{{ task_name }}`).

The default Obsidian template at `prompts/obsidian_minutes.md` includes YAML frontmatter with title and date, but does **not** include the full transcript. Customize the template to add `{{ transcript }}` if you want the full transcript embedded.

Required configuration for minutes generation:

- `DEEPSEEK_API_KEY` in `.env`
- `DEEPSEEK_MODEL` in `.env` (default: `deepseek-v4-flash`)

Available models:

- `deepseek-v4-flash` — faster and cost-effective (default)
- `deepseek-v4-pro` — higher quality

If `output-mode=minutes` and the DeepSeek API key is missing, the command fails with a clear error. It does not fall back to notes mode.

If DeepSeek generation fails (API error, network issue, etc.), the workflow preserves the already-generated `notes_input.md` and reports: "转写已完成，notes_input.md 已生成，但会议总结生成失败。"

## Notes Prompt Templates

Meeting-note structure is stored in Markdown prompt files, not Python code.

Default:

```text
prompts/default_minutes.md
```

Custom:

```bash
meeting-asr transcribe input/meeting.mp3 --output-mode notes --prompt prompts/my_template.md
```

The prompt template defines the structure of `notes_input.md`, which is used as the user prompt when calling the LLM in `minutes` mode.

## Obsidian Template

The Obsidian template wraps the LLM-generated minutes for use in Obsidian. It supports simple placeholder substitution:

| Placeholder | Description |
|---|---|
| `{{ title }}` | Derived from task name |
| `{{ generated_at }}` | UTC timestamp |
| `{{ generated_minutes }}` | Raw LLM minutes output |
| `{{ transcript }}` | Full transcript text (empty by default) |
| `{{ task_name }}` | Full task directory name |

Default:

```text
prompts/obsidian_minutes.md
```

Custom:

```bash
meeting-asr transcribe input/meeting.mp3 --output-mode minutes --obsidian-template prompts/my_obsidian.md
```

## Extension Points

- Add other ASR providers by following the boundary in `src/meeting_asr/transcribe_mimo.py`.
- Add other LLM providers by following the pattern in `src/meeting_asr/summarize.py`.
- Add new note formats by creating more files under `prompts/`.
- Keep workflow orchestration in `src/meeting_asr/main.py` and provider details inside provider modules.

## Current MiMo Payload Note

MiMo ASR calls are isolated in `transcribe_mimo.py`. The request body follows MiMo's speech-recognition schema with a data URL in `input_audio.data` and `asr_options.language` set to `auto`. If MiMo's production schema changes, adjust `build_mimo_asr_payload()` without changing the CLI workflow.

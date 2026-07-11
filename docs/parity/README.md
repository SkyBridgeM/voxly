# Phase 3: Python/Go Parity Validation

## Purpose

Phase 3 compares the Python backend (reference implementation) against the
experimental Go backend (candidate implementation) to determine whether Go
core is close enough to become the future desktop backend.

**Python is the stable reference. Go is the experimental candidate.**

This phase does NOT:
- Integrate Go with Tauri
- Make Go the default backend
- Remove Python
- Require live API keys for automated testing

## What is compared

| Area | What | Python ↔ Go |
|------|------|-------------|
| Task folder layout | audio/, chunks/, transcripts/, transcript.md (+ notes_input.md, meeting_minutes.md per mode) | Required files/directories must exist; extra files are ignored |
| Transcript structure | [Part NNN] headers, ordering, contiguity, trailing newline | Exact match required |
| `transcript.md` content | Per-chunk text content | Order and structure must match; content may differ due to ASR non-determinism |
| `notes_input.md` content | Prompt + transcript concatenation with `---` separator | **Live:** structural (prompt equality, separator, header, ordered non-empty transcript parts). **Offline:** byte-exact when prompt and transcript are identical |
| `meeting_minutes.md` | Existence, non-empty, no unresolved template placeholders | **NOT compared byte-for-byte** — LLM output is non-deterministic. Unresolved `{{ placeholder }}` tokens are rejected |
| Exit codes | Success (0) vs failure (non-zero) | Must agree |
| stdout contract | Status:, Progress:, Transcript saved:, Notes input saved:, Meeting minutes saved:, Task folder: | Required fields validated per mode; exact text may differ |

## What is NOT compared

- DeepSeek-generated meeting minutes text (LLM non-determinism)
- Chunk audio files (ffmpeg version differences may produce slightly different WAV headers)
- `task_manifest.json` content (Go-specific schema)
- Resume behavior (Go and Python resume implementations differ)
- Performance or timing
- `.env` loading behavior

## API keys

- API keys must **never** be committed to the repository.
- All automated parity tests run offline with mock fixtures.
- Live API parity checks are manual and documented in this README.

## Running offline parity checks

```bash
cd tests/parity
python -m pytest -v
```

These tests do not require API keys, audio files, or ffmpeg.
They validate comparator utilities and stdout parsing against mock data.

## Running optional live API parity checks

These require:
- A real audio file (e.g., `sample.m4a`)
- MIMO_API_KEY in `.env`
- DEEPSEEK_API_KEY in `.env` (for minutes mode only)
- ffmpeg on PATH
- Go binary built: `cd core-go && go build -o meeting-asr-go .`

### Transcript mode

```bash
python scripts/parity/run_parity.py \
  --audio path/to/sample.m4a \
  --mode transcript \
  --python-output output/parity/python \
  --go-output output/parity/go \
  --go-binary core-go/meeting-asr-go \
  --env-file .env
```

The Python CLI uses the repository-root `.env` file automatically.
The Go CLI receives `--env-file .env` for equivalent configuration.

### Notes mode

```bash
python scripts/parity/run_parity.py \
  --audio path/to/sample.m4a \
  --mode notes \
  --python-output output/parity/python \
  --go-output output/parity/go \
  --go-binary core-go/meeting-asr-go \
  --env-file .env
```

### Minutes mode

```bash
python scripts/parity/run_parity.py \
  --audio path/to/sample.m4a \
  --mode minutes \
  --python-output output/parity/python \
  --go-output output/parity/go \
  --go-binary core-go/meeting-asr-go \
  --env-file .env
```

### Resume scenario (Go only)

1. Start a Go minutes task and interrupt it during transcription or minutes generation:

   ```bash
   core-go/meeting-asr-go transcribe sample.m4a \
     --output-mode minutes \
     --output-dir /tmp/parity-resume \
     --env-file .env
   # Press Ctrl+C during transcription or minutes generation.
   ```

2. Resume:

   ```bash
   core-go/meeting-asr-go resume /tmp/parity-resume/<timestamp>_<slug> \
     --env-file .env
   ```

3. Verify:
   - No completed chunks are re-transcribed (check stdout for "Skipping existing transcript")
   - Manifest status reaches `COMPLETED`
   - Output files exist: transcript.md, notes_input.md, meeting_minutes.md

Resume parity between Python and Go is **not** required in this phase.
This is Go-specific reliability validation.

## Output reports

Parity runs write reports to `output/parity/reports/` by default.
Reports include:
- Commands run (documented in this README)
- Python and Go exit codes
- Generated task folders
- Pass/fail checks with detail
- Any warnings

## Tauri integration

Tauri integration is **out of scope** for Phase 3.

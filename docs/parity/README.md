# Phase 3 Python/Go parity validation

## Goal and scope

Phase 3 evaluates whether the experimental Go core is close enough to become the backend candidate for a future desktop shell. Python remains the stable reference implementation; Go is not the default backend and this phase does not remove or change Python.

Parity means both implementations produce the shared task layout and output shapes expected for `transcript`, `notes`, and `minutes` modes. It is a validation layer, not a runtime integration layer.

This phase does not include Tauri integration, desktop or WebUI changes, packaging, ffmpeg bundling, Python removal, or live API calls in CI.

## What is compared

- Shared task layout: `audio/`, `chunks/`, `transcripts/`, `transcript.md`, and the mode-required artifacts. Backend-specific files such as Go's `task_manifest.json` and `.resume.lock` are allowed.
- `transcript.md`: exact `[Part 001]` header format, contiguous one-based numbering, ordered parts, non-empty parts, and trailing newline. ASR body text is not compared because independent ASR requests can differ.
- `notes_input.md` in offline fixtures: byte-for-byte equality when both fixture inputs are deliberately identical.
- `notes_input.md` in live checks: separator, `## Transcript` heading, common prompt section, and embedded transcript structure. The separately generated transcript body is not byte-compared.
- `meeting_minutes.md`: each backend must write a non-empty regular Markdown file with no unresolved project template placeholders. LLM text is never byte-compared; arbitrary `{{ ... }}` text from the meeting content is allowed. Missing headings or malformed optional frontmatter are advisory warnings because custom templates can vary.
- Stdout: known contract lines are parsed; required fields depend on the output mode. Unknown informational lines are tolerated and reported as warnings.

The known stdout contract lines are `Status: ...`, `Progress: current/total`, `Transcript saved: ...`, `Notes input saved: ...`, `Meeting minutes saved: ...`, and `Task folder: ...`. A present progress line must have positive integers and satisfy `current <= total`.

## Offline checks (automated)

Offline tests do not need API keys, audio, ffmpeg, a Go binary, or network access. They test only the comparators and report serializer.

```bash
python -m pytest tests/parity -v
```

## Optional live parity checks (manual only)

Live checks require a real audio file, ffmpeg on `PATH`, a built Go binary, and API keys in a local `.env` file. They are not CI. Never commit `.env`, API keys, recordings, generated task folders, or reports containing sensitive paths.

Run from the repository root so Python uses the existing root `.env`. Python intentionally does not support `--env-file`; the runner does not pass it to Python. The optional `--env-file` is supplied to Go only, and should point to the same effective configuration as the root `.env` used by Python.

```bash
cd core-go && go build -o meeting-asr-go . && cd ..

python scripts/parity/run_parity.py \
  --audio path/to/sample.m4a \
  --mode transcript \
  --python-output output/parity/python \
  --go-output output/parity/go \
  --go-binary core-go/meeting-asr-go
```

For `notes`, use `--mode notes` and provide a valid shared prompt through the normal local configuration. For `minutes`, use `--mode minutes`; it requires both MiMo and DeepSeek credentials. Reports are JSON under `output/parity/reports/` by default and record commands, exit codes, task folders, checks, warnings, and differences. The runner accepts a task folder only when it is beneath that backend's requested output root, and does not add secrets to a command or report.

## Resume reliability validation

Python and Go resume are not forced into exact parity: Python does not use Go's manifest state-machine semantics. Instead, manually validate Go resume as a reliability property:

1. Interrupt chunk transcription, then resume and confirm completed chunks are skipped.
2. Interrupt minutes generation, then resume and confirm only required downstream work runs.
3. Confirm `task_manifest.json` reaches `COMPLETED` and all required artifacts validate.

These are optional live checks, not automated parity CI.

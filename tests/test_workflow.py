from pathlib import Path

import pytest

from meeting_asr.config import Settings
from meeting_asr.workflow import (
    TranscribeOptions,
    TranscribeResult,
    run_recorded_chunks_workflow,
    safe_upload_filename,
)


def test_transcribe_options_accepts_webui_overrides(tmp_path: Path) -> None:
    options = TranscribeOptions(
        output_dir=tmp_path,
        chunk_minutes=3,
        max_upload_mb=15,
        summarize=True,
        prompt_path=Path("prompts/default_minutes.md"),
    )

    assert options.output_dir == tmp_path
    assert options.chunk_minutes == 3
    assert options.max_upload_mb == 15
    assert options.summarize


def test_transcribe_result_allows_optional_minutes(tmp_path: Path) -> None:
    result = TranscribeResult(
        task_dir=tmp_path,
        transcript_path=tmp_path / "transcript.md",
        notes_input_path=None,
        minutes_path=None,
    )

    assert result.task_dir == tmp_path
    assert result.minutes_path is None


def test_settings_max_upload_bytes_uses_current_value() -> None:
    settings = Settings(
        mimo_api_key="key",
        mimo_asr_endpoint="endpoint",
        mimo_asr_model="model",
        max_upload_mb=12.5,
        default_chunk_minutes=5,
        retry_count=3,
        timeout_seconds=120,
    )

    assert settings.max_upload_bytes == int(12.5 * 1024 * 1024)


def test_transcribe_options_rejects_invalid_run_overrides(tmp_path: Path) -> None:
    with pytest.raises(ValueError, match="Chunk length"):
        TranscribeOptions(tmp_path, 0, 10)

    with pytest.raises(ValueError, match="Max upload size"):
        TranscribeOptions(tmp_path, 5, 0)


def test_safe_upload_filename_strips_paths_and_sanitizes_names() -> None:
    assert safe_upload_filename("../Team Meeting/Q3 Plan.mp3") == "Q3_Plan.mp3"
    assert safe_upload_filename(r"..\Team Meeting\Q3 Plan.wav") == "Q3_Plan.wav"

    with pytest.raises(ValueError, match="Unsupported audio format"):
        safe_upload_filename("../meeting.m4a")


def test_recorded_chunks_workflow_reuses_transcription_pipeline(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    task_dir = tmp_path / "task"
    (task_dir / "transcripts").mkdir(parents=True)
    chunk = tmp_path / "part_001.wav"
    chunk.write_bytes(b"audio")
    prompt = tmp_path / "prompt.md"
    prompt.write_text("Make notes", encoding="utf-8")
    settings = Settings("key", "endpoint", "model", 10, 5, 3, 120)
    options = TranscribeOptions(tmp_path, 5, 10, summarize=True, prompt_path=prompt)

    def fake_transcribe_chunks(chunks, transcripts_dir, settings, progress_callback=None):
        (transcripts_dir / "part_001.txt").write_text("hello", encoding="utf-8")
        return [transcripts_dir / "part_001.txt"]

    monkeypatch.setattr("meeting_asr.workflow.transcribe_chunks", fake_transcribe_chunks)

    result = run_recorded_chunks_workflow([chunk], task_dir, settings, options)

    assert result.transcript_path.read_text(encoding="utf-8") == "[Part 001]\n\nhello\n"
    assert result.notes_input_path is not None
    assert "Make notes" in result.notes_input_path.read_text(encoding="utf-8")

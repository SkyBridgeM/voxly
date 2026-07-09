from pathlib import Path

import pytest

from meeting_asr.config import Settings
from meeting_asr.workflow import (
    TranscribeOptions,
    TranscribeResult,
    find_resume_chunks,
    run_recorded_chunks_workflow,
    run_resume_workflow,
    safe_upload_filename,
)


def test_transcribe_options_accepts_webui_overrides(tmp_path: Path) -> None:
    options = TranscribeOptions(
        output_dir=tmp_path,
        chunk_minutes=3,
        max_upload_mb=15,
        output_mode="notes",
        prompt_path=Path("prompts/default_minutes.md"),
    )

    assert options.output_dir == tmp_path
    assert options.chunk_minutes == 3
    assert options.max_upload_mb == 15
    assert options.output_mode == "notes"
    assert options._needs_notes
    assert not options._needs_llm


def test_transcribe_options_transcript_mode(tmp_path: Path) -> None:
    options = TranscribeOptions(tmp_path, 5, 10, output_mode="transcript")
    assert not options._needs_notes
    assert not options._needs_llm


def test_transcribe_options_notes_mode(tmp_path: Path) -> None:
    options = TranscribeOptions(tmp_path, 5, 10, output_mode="notes")
    assert options._needs_notes
    assert not options._needs_llm


def test_transcribe_options_minutes_mode(tmp_path: Path) -> None:
    options = TranscribeOptions(tmp_path, 5, 10, output_mode="minutes")
    assert options._needs_notes
    assert options._needs_llm


def test_transcribe_options_summarize_backward_compat(tmp_path: Path) -> None:
    options = TranscribeOptions(tmp_path, 5, 10, summarize=True)
    assert options._needs_notes
    assert not options._needs_llm


def test_transcribe_options_rejects_invalid_output_mode(tmp_path: Path) -> None:
    with pytest.raises(ValueError, match="Invalid output_mode"):
        TranscribeOptions(tmp_path, 5, 10, output_mode="invalid")


def test_transcribe_options_default_values(tmp_path: Path) -> None:
    options = TranscribeOptions(tmp_path, 5, 10)
    assert options.output_mode == "transcript"
    assert not options.summarize
    assert options.llm_provider == "deepseek"
    assert options.llm_model == "deepseek-v4-flash"
    assert options.obsidian_template_path == Path("prompts/obsidian_minutes.md")


def test_transcribe_result_allows_optional_minutes(tmp_path: Path) -> None:
    result = TranscribeResult(
        task_dir=tmp_path,
        transcript_path=tmp_path / "transcript.md",
        notes_input_path=None,
        minutes_path=None,
    )

    assert result.task_dir == tmp_path
    assert result.minutes_path is None


def test_transcribe_result_with_minutes(tmp_path: Path) -> None:
    minutes = tmp_path / "meeting_minutes.md"
    result = TranscribeResult(
        task_dir=tmp_path,
        transcript_path=tmp_path / "transcript.md",
        notes_input_path=tmp_path / "notes_input.md",
        minutes_path=minutes,
    )
    assert result.notes_input_path is not None
    assert result.minutes_path == minutes


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
    assert safe_upload_filename("../Voice Memo.m4a") == "Voice_Memo.m4a"

    with pytest.raises(ValueError, match="Unsupported audio format"):
        safe_upload_filename("../meeting.aiff")


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
    options = TranscribeOptions(tmp_path, 5, 10, output_mode="notes", prompt_path=prompt)

    def fake_transcribe_chunks(chunks, transcripts_dir, settings, progress_callback=None):
        (transcripts_dir / "part_001.txt").write_text("hello", encoding="utf-8")
        return [transcripts_dir / "part_001.txt"]

    monkeypatch.setattr("meeting_asr.workflow.transcribe_chunks", fake_transcribe_chunks)

    result = run_recorded_chunks_workflow([chunk], task_dir, settings, options)

    assert result.transcript_path.read_text(encoding="utf-8") == "[Part 001]\n\nhello\n"
    assert result.notes_input_path is not None
    assert "Make notes" in result.notes_input_path.read_text(encoding="utf-8")


def test_transcript_mode_only_generates_transcript(
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
    options = TranscribeOptions(tmp_path, 5, 10, output_mode="transcript", prompt_path=prompt)

    def fake_transcribe_chunks(chunks, transcripts_dir, settings, progress_callback=None):
        (transcripts_dir / "part_001.txt").write_text("hello", encoding="utf-8")
        return [transcripts_dir / "part_001.txt"]

    monkeypatch.setattr("meeting_asr.workflow.transcribe_chunks", fake_transcribe_chunks)

    result = run_recorded_chunks_workflow([chunk], task_dir, settings, options)

    assert result.transcript_path.read_text(encoding="utf-8") == "[Part 001]\n\nhello\n"
    assert result.notes_input_path is None
    assert result.minutes_path is None


def test_minutes_mode_fails_when_no_deepseek_key(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    task_dir = tmp_path / "task"
    (task_dir / "transcripts").mkdir(parents=True)
    chunk = tmp_path / "part_001.wav"
    chunk.write_bytes(b"audio")
    prompt = tmp_path / "prompt.md"
    prompt.write_text("Make notes", encoding="utf-8")
    settings = Settings("key", "endpoint", "model", 10, 5, 3, 120, deepseek_api_key=None)
    options = TranscribeOptions(tmp_path, 5, 10, output_mode="minutes", prompt_path=prompt)

    def fake_transcribe_chunks(chunks, transcripts_dir, settings, progress_callback=None):
        (transcripts_dir / "part_001.txt").write_text("hello", encoding="utf-8")
        return [transcripts_dir / "part_001.txt"]

    monkeypatch.setattr("meeting_asr.workflow.transcribe_chunks", fake_transcribe_chunks)

    with pytest.raises(RuntimeError, match="会议总结生成失败"):
        run_recorded_chunks_workflow([chunk], task_dir, settings, options)


def test_minutes_mode_generates_minutes_via_llm(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    task_dir = tmp_path / "task"
    (task_dir / "transcripts").mkdir(parents=True)
    obsidian_template = tmp_path / "obsidian.md"
    obsidian_template.write_text("# {{ title }}\n\n{{ generated_minutes }}", encoding="utf-8")
    chunk = tmp_path / "part_001.wav"
    chunk.write_bytes(b"audio")
    prompt = tmp_path / "prompt.md"
    prompt.write_text("Make notes", encoding="utf-8")
    settings = Settings("key", "endpoint", "model", 10, 5, 3, 120, deepseek_api_key="sk-test")
    options = TranscribeOptions(
        tmp_path, 5, 10,
        output_mode="minutes",
        prompt_path=prompt,
        obsidian_template_path=obsidian_template,
    )

    def fake_transcribe_chunks(chunks, transcripts_dir, settings, progress_callback=None):
        (transcripts_dir / "part_001.txt").write_text("hello", encoding="utf-8")
        return [transcripts_dir / "part_001.txt"]

    def fake_generate_minutes(notes_input_path, api_key, *, endpoint, model, timeout_seconds):
        return "## Meeting Summary\n\nThis is a test."

    monkeypatch.setattr("meeting_asr.workflow.transcribe_chunks", fake_transcribe_chunks)
    monkeypatch.setattr("meeting_asr.workflow.generate_minutes", fake_generate_minutes)

    result = run_recorded_chunks_workflow([chunk], task_dir, settings, options)

    assert result.transcript_path is not None
    assert result.notes_input_path is not None
    assert result.minutes_path is not None
    content = result.minutes_path.read_text(encoding="utf-8")
    assert "Meeting Summary" in content
    assert "This is a test" in content


def test_minutes_mode_rejects_unknown_provider(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    task_dir = tmp_path / "task"
    (task_dir / "transcripts").mkdir(parents=True)
    chunk = tmp_path / "part_001.wav"
    chunk.write_bytes(b"audio")
    prompt = tmp_path / "prompt.md"
    prompt.write_text("Make notes", encoding="utf-8")
    settings = Settings("key", "endpoint", "model", 10, 5, 3, 120, deepseek_api_key="sk-test")
    options = TranscribeOptions(tmp_path, 5, 10, output_mode="minutes", prompt_path=prompt, llm_provider="manual")

    def fake_transcribe_chunks(chunks, transcripts_dir, settings, progress_callback=None):
        (transcripts_dir / "part_001.txt").write_text("hello", encoding="utf-8")
        return [transcripts_dir / "part_001.txt"]

    monkeypatch.setattr("meeting_asr.workflow.transcribe_chunks", fake_transcribe_chunks)

    with pytest.raises(RuntimeError, match="output-mode=minutes requires MINUTES_PROVIDER"):
        run_recorded_chunks_workflow([chunk], task_dir, settings, options)


def test_minutes_mode_error_preserves_original_reason(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    task_dir = tmp_path / "task"
    (task_dir / "transcripts").mkdir(parents=True)
    chunk = tmp_path / "part_001.wav"
    chunk.write_bytes(b"audio")
    prompt = tmp_path / "prompt.md"
    prompt.write_text("Make notes", encoding="utf-8")
    settings = Settings("key", "endpoint", "model", 10, 5, 3, 120, deepseek_api_key=None)
    options = TranscribeOptions(tmp_path, 5, 10, output_mode="minutes", prompt_path=prompt)

    def fake_transcribe_chunks(chunks, transcripts_dir, settings, progress_callback=None):
        (transcripts_dir / "part_001.txt").write_text("hello", encoding="utf-8")
        return [transcripts_dir / "part_001.txt"]

    monkeypatch.setattr("meeting_asr.workflow.transcribe_chunks", fake_transcribe_chunks)

    with pytest.raises(RuntimeError, match="DEEPSEEK_API_KEY"):
        run_recorded_chunks_workflow([chunk], task_dir, settings, options)

    notes = task_dir / "notes_input.md"
    assert notes.exists()


def test_llm_failure_preserves_notes_input(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    task_dir = tmp_path / "task"
    (task_dir / "transcripts").mkdir(parents=True)
    chunk = tmp_path / "part_001.wav"
    chunk.write_bytes(b"audio")
    prompt = tmp_path / "prompt.md"
    prompt.write_text("Make notes", encoding="utf-8")
    settings = Settings("key", "endpoint", "model", 10, 5, 3, 120, deepseek_api_key="sk-test")
    options = TranscribeOptions(tmp_path, 5, 10, output_mode="minutes", prompt_path=prompt)

    def fake_transcribe_chunks(chunks, transcripts_dir, settings, progress_callback=None):
        (transcripts_dir / "part_001.txt").write_text("hello", encoding="utf-8")
        return [transcripts_dir / "part_001.txt"]

    def fake_generate_minutes(*args, **kwargs):
        raise RuntimeError("DeepSeek API returned 500 server error.")

    monkeypatch.setattr("meeting_asr.workflow.transcribe_chunks", fake_transcribe_chunks)
    monkeypatch.setattr("meeting_asr.workflow.generate_minutes", fake_generate_minutes)

    with pytest.raises(RuntimeError, match="500"):
        run_recorded_chunks_workflow([chunk], task_dir, settings, options)

    assert (task_dir / "notes_input.md").exists()


def test_transcript_mode_ignores_existing_minutes(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    task_dir = tmp_path / "task"
    (task_dir / "transcripts").mkdir(parents=True)
    old_minutes = task_dir / "meeting_minutes.md"
    old_minutes.write_text("old minutes", encoding="utf-8")
    chunk = tmp_path / "part_001.wav"
    chunk.write_bytes(b"audio")
    settings = Settings("key", "endpoint", "model", 10, 5, 3, 120)
    options = TranscribeOptions(tmp_path, 5, 10, output_mode="transcript")

    def fake_transcribe_chunks(chunks, transcripts_dir, settings, progress_callback=None):
        (transcripts_dir / "part_001.txt").write_text("hello", encoding="utf-8")
        return [transcripts_dir / "part_001.txt"]

    monkeypatch.setattr("meeting_asr.workflow.transcribe_chunks", fake_transcribe_chunks)

    result = run_recorded_chunks_workflow([chunk], task_dir, settings, options)
    assert result.minutes_path is None


def test_notes_mode_ignores_existing_minutes(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    task_dir = tmp_path / "task"
    (task_dir / "transcripts").mkdir(parents=True)
    old_minutes = task_dir / "meeting_minutes.md"
    old_minutes.write_text("old minutes", encoding="utf-8")
    chunk = tmp_path / "part_001.wav"
    chunk.write_bytes(b"audio")
    prompt = tmp_path / "prompt.md"
    prompt.write_text("Make notes", encoding="utf-8")
    settings = Settings("key", "endpoint", "model", 10, 5, 3, 120)
    options = TranscribeOptions(tmp_path, 5, 10, output_mode="notes", prompt_path=prompt)

    def fake_transcribe_chunks(chunks, transcripts_dir, settings, progress_callback=None):
        (transcripts_dir / "part_001.txt").write_text("hello", encoding="utf-8")
        return [transcripts_dir / "part_001.txt"]

    monkeypatch.setattr("meeting_asr.workflow.transcribe_chunks", fake_transcribe_chunks)

    result = run_recorded_chunks_workflow([chunk], task_dir, settings, options)
    assert result.minutes_path is None


def test_llm_provider_empty_string_rejected(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    task_dir = tmp_path / "task"
    (task_dir / "transcripts").mkdir(parents=True)
    chunk = tmp_path / "part_001.wav"
    chunk.write_bytes(b"audio")
    prompt = tmp_path / "prompt.md"
    prompt.write_text("Make notes", encoding="utf-8")
    settings = Settings("key", "endpoint", "model", 10, 5, 3, 120, deepseek_api_key="sk-test")
    options = TranscribeOptions(tmp_path, 5, 10, output_mode="minutes", prompt_path=prompt, llm_provider="")

    def fake_transcribe_chunks(chunks, transcripts_dir, settings, progress_callback=None):
        (transcripts_dir / "part_001.txt").write_text("hello", encoding="utf-8")
        return [transcripts_dir / "part_001.txt"]

    monkeypatch.setattr("meeting_asr.workflow.transcribe_chunks", fake_transcribe_chunks)

    with pytest.raises(RuntimeError, match="output-mode=minutes requires MINUTES_PROVIDER"):
        run_recorded_chunks_workflow([chunk], task_dir, settings, options)


def test_resume_workflow_rejects_missing_task_dir(tmp_path: Path) -> None:
    settings = Settings("key", "endpoint", "model", 10, 5, 3, 120)
    options = TranscribeOptions(tmp_path, 5, 10)

    with pytest.raises(FileNotFoundError, match="Task folder not found"):
        run_resume_workflow(tmp_path / "missing", settings, options)


def test_find_resume_chunks_rejects_missing_chunks(tmp_path: Path) -> None:
    task_dir = tmp_path / "task"
    task_dir.mkdir()

    with pytest.raises(RuntimeError, match="No resume chunks found"):
        find_resume_chunks(task_dir)


def test_find_resume_chunks_uses_chunks_wav(tmp_path: Path) -> None:
    task_dir = tmp_path / "task"
    chunks_dir = task_dir / "chunks"
    chunks_dir.mkdir(parents=True)
    chunk = chunks_dir / "part_001.wav"
    chunk.write_bytes(b"audio")

    assert find_resume_chunks(task_dir) == [chunk.resolve()]


def test_find_resume_chunks_falls_back_to_recorded_audio_wav(tmp_path: Path) -> None:
    task_dir = tmp_path / "task"
    audio_dir = task_dir / "audio"
    audio_dir.mkdir(parents=True)
    original = audio_dir / "original.wav"
    original.write_bytes(b"ignored")
    chunk = audio_dir / "part_001.wav"
    chunk.write_bytes(b"audio")

    assert find_resume_chunks(task_dir) == [chunk.resolve()]


def test_resume_skips_existing_non_empty_transcript_part(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    task_dir = tmp_path / "task"
    chunks_dir = task_dir / "chunks"
    transcripts_dir = task_dir / "transcripts"
    chunks_dir.mkdir(parents=True)
    transcripts_dir.mkdir()
    (chunks_dir / "part_001.wav").write_bytes(b"audio")
    (transcripts_dir / "part_001.txt").write_text("already done", encoding="utf-8")
    settings = Settings("key", "endpoint", "model", 10, 5, 3, 120)
    options = TranscribeOptions(tmp_path, 5, 10)

    def fail_if_called(*args, **kwargs):
        raise AssertionError("ASR should not run for an existing non-empty transcript part")

    monkeypatch.setattr("meeting_asr.transcribe_mimo.transcribe_audio_file", fail_if_called)

    result = run_resume_workflow(task_dir, settings, options)

    assert result.transcript_path.read_text(encoding="utf-8") == "[Part 001]\n\nalready done\n"


def test_find_resume_chunks_rejects_mixed_chunk_formats(tmp_path: Path) -> None:
    task_dir = tmp_path / "task"
    chunks_dir = task_dir / "chunks"
    chunks_dir.mkdir(parents=True)
    (chunks_dir / "part_001.wav").write_bytes(b"audio")
    (chunks_dir / "part_002.mp3").write_bytes(b"audio")

    with pytest.raises(RuntimeError, match="Mixed resume chunk formats"):
        find_resume_chunks(task_dir)


def test_find_resume_chunks_rejects_missing_part_number(tmp_path: Path) -> None:
    task_dir = tmp_path / "task"
    chunks_dir = task_dir / "chunks"
    chunks_dir.mkdir(parents=True)
    (chunks_dir / "part_001.wav").write_bytes(b"audio")
    (chunks_dir / "part_003.wav").write_bytes(b"audio")

    with pytest.raises(RuntimeError, match="Missing resume chunk"):
        find_resume_chunks(task_dir)


def test_find_resume_chunks_rejects_chunks_not_starting_at_one(tmp_path: Path) -> None:
    task_dir = tmp_path / "task"
    chunks_dir = task_dir / "chunks"
    chunks_dir.mkdir(parents=True)
    (chunks_dir / "part_002.wav").write_bytes(b"audio")

    with pytest.raises(RuntimeError, match="must start at part_001"):
        find_resume_chunks(task_dir)


def test_find_resume_chunks_rejects_invalid_chunk_name_without_fallback(tmp_path: Path) -> None:
    task_dir = tmp_path / "task"
    chunks_dir = task_dir / "chunks"
    audio_dir = task_dir / "audio"
    chunks_dir.mkdir(parents=True)
    audio_dir.mkdir()
    (chunks_dir / "part_backup.wav").write_bytes(b"bad")
    (audio_dir / "part_001.wav").write_bytes(b"audio")

    with pytest.raises(RuntimeError, match="Invalid resume chunk filename"):
        find_resume_chunks(task_dir)


def test_resume_workflow_validates_chunk_size(tmp_path: Path) -> None:
    task_dir = tmp_path / "task"
    chunks_dir = task_dir / "chunks"
    chunks_dir.mkdir(parents=True)
    (chunks_dir / "part_001.wav").write_bytes(b"audio")
    settings = Settings("key", "endpoint", "model", 0.000001, 5, 3, 120)
    options = TranscribeOptions(tmp_path, 5, settings.max_upload_mb)

    with pytest.raises(RuntimeError, match="exceed the upload limit"):
        run_resume_workflow(task_dir, settings, options)

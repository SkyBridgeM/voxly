from pathlib import Path
from zipfile import ZipFile

from webui.ui_helpers import (
    build_text_outputs_zip,
    classify_error,
    discover_prompt_templates,
    read_preview,
    resolve_prompt_path,
)


def test_discover_prompt_templates_returns_markdown_files(tmp_path: Path) -> None:
    (tmp_path / "a.md").write_text("a", encoding="utf-8")
    (tmp_path / "b.txt").write_text("b", encoding="utf-8")
    (tmp_path / "c.md").write_text("c", encoding="utf-8")

    assert [path.name for path in discover_prompt_templates(tmp_path)] == ["a.md", "c.md"]


def test_resolve_prompt_path_prefers_custom_path() -> None:
    assert resolve_prompt_path("prompts/default_minutes.md", "/tmp/custom.md") == Path("/tmp/custom.md")
    assert resolve_prompt_path("prompts/default_minutes.md", "") == Path("prompts/default_minutes.md")


def test_read_preview_truncates_long_text(tmp_path: Path) -> None:
    path = tmp_path / "transcript.md"
    path.write_text("a" * 20, encoding="utf-8")

    preview, truncated = read_preview(path, limit=10)

    assert truncated
    assert preview.startswith("a" * 10)
    assert "Preview truncated" in preview


def test_build_text_outputs_zip_excludes_audio_and_chunks(tmp_path: Path) -> None:
    (tmp_path / "transcript.md").write_text("main", encoding="utf-8")
    (tmp_path / "notes_input.md").write_text("notes", encoding="utf-8")
    transcripts = tmp_path / "transcripts"
    transcripts.mkdir()
    (transcripts / "part_001.txt").write_text("part", encoding="utf-8")
    audio = tmp_path / "audio"
    audio.mkdir()
    (audio / "part_001.wav").write_bytes(b"audio")
    chunks = tmp_path / "chunks"
    chunks.mkdir()
    (chunks / "part_001.wav").write_bytes(b"chunk")

    zip_path = tmp_path / "outputs.zip"
    zip_path.write_bytes(build_text_outputs_zip(tmp_path))

    with ZipFile(zip_path) as archive:
        names = sorted(archive.namelist())

    assert names == ["notes_input.md", "transcript.md", "transcripts/part_001.txt"]


def test_classify_error_names_common_failures() -> None:
    assert classify_error(RuntimeError("MIMO_API_KEY is not configured."))[0] == "MiMo API key is missing"
    assert classify_error(RuntimeError("ffmpeg was not found."))[0] == "ffmpeg is not installed"
    assert classify_error(ValueError("Unsupported audio format '.m4a'."))[0] == "Unsupported audio format"
    assert classify_error(FileNotFoundError("Prompt file not found: prompts/missing.md"))[0] == "Prompt file was not found"
    assert classify_error(RuntimeError("MiMo API error 401: unauthorized"))[0] == "MiMo ASR failed"

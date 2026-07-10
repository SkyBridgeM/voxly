import os
from pathlib import Path
from zipfile import ZipFile

import pytest

from webui.ui_helpers import (
    build_text_outputs_zip,
    classify_error,
    discover_prompt_templates,
    read_preview,
    resolve_prompt_path,
    update_env_value,
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


def test_update_env_value_creates_env_file(tmp_path: Path) -> None:
    env_path = tmp_path / ".env"

    update_env_value(env_path, "MIMO_API_KEY", "new-key")

    assert env_path.read_text(encoding="utf-8") == "MIMO_API_KEY=new-key\n"


def test_update_env_value_replaces_existing_key_and_preserves_other_lines(tmp_path: Path) -> None:
    env_path = tmp_path / ".env"
    env_path.write_text(
        "# comment\n"
        "MIMO_ASR_MODEL=model\n"
        "\n"
        "MIMO_API_KEY=old-key\n"
        "MAX_UPLOAD_MB=10\n",
        encoding="utf-8",
    )

    update_env_value(env_path, "MIMO_API_KEY", "new-key")

    assert env_path.read_text(encoding="utf-8") == (
        "# comment\n"
        "MIMO_ASR_MODEL=model\n"
        "\n"
        "MIMO_API_KEY=new-key\n"
        "MAX_UPLOAD_MB=10\n"
    )


def test_update_env_value_appends_missing_key(tmp_path: Path) -> None:
    env_path = tmp_path / ".env"
    env_path.write_text("MAX_UPLOAD_MB=10", encoding="utf-8")

    update_env_value(env_path, "MIMO_API_KEY", "new-key")

    assert env_path.read_text(encoding="utf-8") == "MAX_UPLOAD_MB=10\nMIMO_API_KEY=new-key\n"


def test_update_env_value_ignores_commented_key(tmp_path: Path) -> None:
    env_path = tmp_path / ".env"
    env_path.write_text("# MIMO_API_KEY=old-key\nMAX_UPLOAD_MB=10\n", encoding="utf-8")

    update_env_value(env_path, "MIMO_API_KEY", "new-key")

    assert env_path.read_text(encoding="utf-8") == (
        "# MIMO_API_KEY=old-key\n"
        "MAX_UPLOAD_MB=10\n"
        "MIMO_API_KEY=new-key\n"
    )


def test_update_env_value_replaces_first_key_and_removes_duplicates(tmp_path: Path) -> None:
    env_path = tmp_path / ".env"
    env_path.write_text(
        "MIMO_API_KEY=old-one\n"
        "MAX_UPLOAD_MB=10\n"
        "MIMO_API_KEY=old-two\n",
        encoding="utf-8",
    )

    update_env_value(env_path, "MIMO_API_KEY", "new-key")

    assert env_path.read_text(encoding="utf-8") == "MIMO_API_KEY=new-key\nMAX_UPLOAD_MB=10\n"


def test_update_env_value_does_not_treat_export_as_existing_key(tmp_path: Path) -> None:
    env_path = tmp_path / ".env"
    env_path.write_text("export MIMO_API_KEY=old-key\nMAX_UPLOAD_MB=10\n", encoding="utf-8")

    update_env_value(env_path, "MIMO_API_KEY", "new-key")

    assert env_path.read_text(encoding="utf-8") == (
        "export MIMO_API_KEY=old-key\n"
        "MAX_UPLOAD_MB=10\n"
        "MIMO_API_KEY=new-key\n"
    )


def test_update_env_value_rejects_newlines(tmp_path: Path) -> None:
    with pytest.raises(ValueError, match="newlines"):
        update_env_value(tmp_path / ".env", "MIMO_API_KEY", "key\nDEEPSEEK_API_KEY=injected")


@pytest.mark.skipif(os.name == "nt", reason="POSIX file permissions")
def test_update_env_value_uses_private_permissions(tmp_path: Path) -> None:
    env_path = tmp_path / ".env"
    env_path.write_text("MIMO_API_KEY=old\n", encoding="utf-8")
    env_path.chmod(0o644)

    update_env_value(env_path, "MIMO_API_KEY", "new-key")

    assert env_path.stat().st_mode & 0o777 == 0o600

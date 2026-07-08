from pathlib import Path

import pytest

from meeting_asr.audio_splitter import calculate_retry_chunk_seconds
from meeting_asr.utils import (
    build_ffmpeg_missing_message,
    create_task_dir,
    is_complete_file,
    safe_slug,
    supported_audio_format,
)


def test_is_complete_file_requires_non_empty_file(tmp_path: Path) -> None:
    missing = tmp_path / "missing.txt"
    empty = tmp_path / "empty.txt"
    full = tmp_path / "full.txt"
    empty.write_text("", encoding="utf-8")
    full.write_text("done", encoding="utf-8")

    assert not is_complete_file(missing)
    assert not is_complete_file(empty)
    assert is_complete_file(full)


def test_create_task_dir_uses_safe_timestamped_name(tmp_path: Path) -> None:
    task_dir = create_task_dir(tmp_path, "Team Meeting: Q3/Plan")

    assert task_dir.parent == tmp_path.resolve()
    assert task_dir.name.endswith("_Team_Meeting_Q3_Plan")
    assert (task_dir / "audio").is_dir()
    assert (task_dir / "chunks").is_dir()
    assert (task_dir / "transcripts").is_dir()


def test_supported_audio_format_accepts_mp3_wav_and_m4a() -> None:
    assert supported_audio_format(Path("meeting.mp3")) == "mp3"
    assert supported_audio_format(Path("meeting.wav")) == "wav"
    assert supported_audio_format(Path("voice-memo.m4a")) == "m4a"

    with pytest.raises(ValueError):
        supported_audio_format(Path("meeting.aiff"))


def test_retry_chunk_seconds_stop_at_sixty_seconds() -> None:
    assert calculate_retry_chunk_seconds(600) == [600, 300, 150, 75, 60]
    assert calculate_retry_chunk_seconds(300) == [300, 150, 75, 60]


def test_safe_slug_falls_back_for_empty_names() -> None:
    assert safe_slug("###") == "meeting"


def test_ffmpeg_missing_message_recommends_brew_when_available() -> None:
    message = build_ffmpeg_missing_message(has_homebrew=True)

    assert "brew install ffmpeg" in message
    assert "Homebrew was not found" not in message


def test_ffmpeg_missing_message_recommends_homebrew_first_when_missing() -> None:
    message = build_ffmpeg_missing_message(has_homebrew=False)

    assert "Homebrew was not found" in message
    assert "https://brew.sh/" in message
    assert "brew install ffmpeg" in message

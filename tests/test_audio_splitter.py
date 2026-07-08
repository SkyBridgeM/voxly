from pathlib import Path

import pytest

from meeting_asr.audio_splitter import chunk_audio_format, split_audio


def test_chunk_audio_format_converts_m4a_to_wav() -> None:
    assert chunk_audio_format("m4a") == "wav"
    assert chunk_audio_format("mp3") == "mp3"
    assert chunk_audio_format("wav") == "wav"


def test_split_audio_converts_m4a_to_wav_chunks(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    source = tmp_path / "voice-memo.m4a"
    source.write_bytes(b"m4a")
    chunks_dir = tmp_path / "chunks"
    commands: list[list[str]] = []

    monkeypatch.setattr("meeting_asr.audio_splitter.ensure_ffmpeg", lambda: None)

    def fake_run_command(command: list[str], *, capture: bool = False):
        commands.append(command)
        output_pattern = Path(command[-1])
        (output_pattern.parent / "part_000.wav").write_bytes(b"wav")
        return None

    monkeypatch.setattr("meeting_asr.audio_splitter.run_command", fake_run_command)

    chunks = split_audio(source, chunks_dir, chunk_minutes=5, max_upload_bytes=1024)

    assert chunks == [chunks_dir / "part_001.wav"]
    assert chunks[0].read_bytes() == b"wav"
    assert commands[0][-1].endswith("part_%03d.wav")
    assert "-c:a" in commands[0]
    assert "pcm_s16le" in commands[0]
    assert "-ar" in commands[0]
    assert "16000" in commands[0]

from pathlib import Path

from meeting_asr.transcript_merger import merge_transcripts


def test_merge_transcripts_preserves_order_and_markers(tmp_path: Path) -> None:
    transcripts = tmp_path / "transcripts"
    transcripts.mkdir()
    (transcripts / "part_002.txt").write_text("second", encoding="utf-8")
    (transcripts / "part_001.txt").write_text("first", encoding="utf-8")

    output = merge_transcripts(transcripts, tmp_path / "transcript.md")

    assert output.read_text(encoding="utf-8") == "[Part 001]\n\nfirst\n\n[Part 002]\n\nsecond\n"

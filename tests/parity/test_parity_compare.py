from __future__ import annotations

import json
from pathlib import Path

from scripts.parity.parity_compare import (
    compare_notes_input,
    compare_notes_input_structure,
    compare_task_layout,
    compare_transcript_structure,
    generate_report,
    parse_backend_stdout,
    validate_minutes_file,
    validate_stdout_contract,
)
from scripts.parity.run_parity import build_python_command


def write(path: Path, content: str) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(content, encoding="utf-8")


def transcript(parts: list[int]) -> str:
    return "\n\n".join(f"[Part {part:03d}]\n\ntext {part}" for part in parts) + "\n"


def make_task(root: Path, mode: str) -> Path:
    for directory in ("audio", "chunks", "transcripts"):
        (root / directory).mkdir(parents=True, exist_ok=True)
    write(root / "transcript.md", transcript([1, 2]))
    if mode in {"notes", "minutes"}:
        write(root / "notes_input.md", "prompt\n\n---\n\n## Transcript\n\n" + transcript([1, 2]))
    if mode == "minutes":
        write(root / "meeting_minutes.md", "---\ntitle: Meeting\n---\n\n# Meeting\n\nSummary\n")
    return root


def test_task_layout_comparator_all_modes(tmp_path: Path) -> None:
    for mode in ("transcript", "notes", "minutes"):
        result = compare_task_layout(mode, make_task(tmp_path / mode / "python", mode), make_task(tmp_path / mode / "go", mode))
        assert result["passed"], result


def test_task_layout_requires_mode_artifact(tmp_path: Path) -> None:
    python_task = make_task(tmp_path / "python", "notes")
    go_task = make_task(tmp_path / "go", "notes")
    (go_task / "notes_input.md").unlink()
    result = compare_task_layout("notes", python_task, go_task)
    assert not result["passed"]
    assert any("notes_input.md" in difference for difference in result["differences"])


def test_transcript_comparator_accepts_matching_ordered_parts(tmp_path: Path) -> None:
    python_path, go_path = tmp_path / "python.md", tmp_path / "go.md"
    write(python_path, transcript([1, 2]))
    write(go_path, transcript([1, 2]))
    assert compare_transcript_structure(python_path, go_path)["passed"]


def test_transcript_comparator_rejects_out_of_order_parts(tmp_path: Path) -> None:
    python_path, go_path = tmp_path / "python.md", tmp_path / "go.md"
    write(python_path, transcript([1, 2]))
    write(go_path, transcript([2, 1]))
    result = compare_transcript_structure(python_path, go_path)
    assert not result["passed"]
    assert any("ordered" in difference or "order differs" in difference for difference in result["differences"])


def test_transcript_comparator_rejects_missing_duplicate_and_malformed_parts(tmp_path: Path) -> None:
    good = tmp_path / "good.md"
    write(good, transcript([1, 2]))
    for name, bad_content in {
        "missing": transcript([1, 3]),
        "duplicate": "[Part 001]\n\na\n\n[Part 001]\n\nb\n",
        "malformed": "[Part 1]\n\na\n",
    }.items():
        bad = tmp_path / f"{name}.md"
        write(bad, bad_content)
        result = compare_transcript_structure(good, bad)
        assert not result["passed"], name


def test_notes_comparator_is_exact_and_reports_mismatch_without_content(tmp_path: Path) -> None:
    python_notes, go_notes = tmp_path / "python.md", tmp_path / "go.md"
    write(python_notes, "same\n")
    write(go_notes, "same\n")
    assert compare_notes_input(python_notes, go_notes)["passed"]
    write(go_notes, "different\n")
    result = compare_notes_input(python_notes, go_notes)
    assert not result["passed"]
    assert "first_difference_offset" in result["differences"][0]
    assert "different" not in result["differences"][0]


def test_live_notes_comparator_checks_structure_not_transcript_body(tmp_path: Path) -> None:
    python_notes, go_notes = tmp_path / "python.md", tmp_path / "go.md"
    write(python_notes, "shared prompt\n\n---\n\n## Transcript\n\n" + transcript([1, 2]))
    write(go_notes, "shared prompt\n\n---\n\n## Transcript\n\n" + "[Part 001]\n\ndifferent ASR text\n\n[Part 002]\n\nother ASR text\n")
    result = compare_notes_input_structure(python_notes, go_notes)
    assert result["passed"], result
    assert result["comparison_kind"] == "structural_live"


def test_minutes_validator_accepts_markdown_and_rejects_missing_empty_or_unresolved(tmp_path: Path) -> None:
    valid = tmp_path / "valid.md"
    write(valid, "# Meeting\n\nSummary\n")
    assert validate_minutes_file(valid)["passed"]
    assert not validate_minutes_file(tmp_path / "missing.md")["passed"]
    empty = tmp_path / "empty.md"
    write(empty, "   \n")
    assert not validate_minutes_file(empty)["passed"]
    unresolved = tmp_path / "unresolved.md"
    write(unresolved, "# {{ title }}\n")
    assert not validate_minutes_file(unresolved)["passed"]


def test_stdout_parser_and_mode_contracts() -> None:
    stdout = "\n".join(
        [
            "Status: Transcribing 2 chunk(s)",
            "Progress: 1/2",
            "Progress: 2/2",
            "Transcript saved: /tmp/transcript.md",
            "Notes input saved: /tmp/notes_input.md",
            "Meeting minutes saved: /tmp/minutes.md",
            "Task folder: /tmp/task",
            "backend detail",
        ]
    )
    parsed = parse_backend_stdout(stdout)
    assert parsed["progress"] == [{"current": 1, "total": 2}, {"current": 2, "total": 2}]
    assert parsed["unknown"] == ["backend detail"]
    assert validate_stdout_contract(parsed, "transcript")["passed"]
    assert validate_stdout_contract(parsed, "notes")["passed"]
    assert validate_stdout_contract(parsed, "minutes")["passed"]


def test_stdout_contract_rejects_malformed_progress_and_missing_mode_artifacts() -> None:
    parsed = parse_backend_stdout("Status: Done\nProgress: 2/1\nTranscript saved: /tmp/a\nTask folder: /tmp/task\n")
    assert "invalid progress values" in parsed["errors"][0]
    result = validate_stdout_contract(parsed, "minutes")
    assert not result["passed"]
    assert any("Notes input saved" in difference for difference in result["differences"])
    assert any("Meeting minutes saved" in difference for difference in result["differences"])


def test_report_is_machine_readable_and_python_command_has_no_go_only_flags(tmp_path: Path) -> None:
    command = build_python_command(tmp_path / "input.m4a", "transcript", tmp_path / "out")
    assert "--env-file" not in command
    assert "--project-root" not in command
    report = json.loads(generate_report({"checks": [], "warnings": [], "differences": [], "passed": True}))
    assert report["passed"] is True

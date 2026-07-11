"""Check that parity_compare module is importable and basic functions work.

All tests are offline — no API keys, no audio, no ffmpeg, no network.
"""

from __future__ import annotations

import re
import tempfile
from pathlib import Path

from scripts.parity.parity_compare import (
    CheckResult,
    ParityResult,
    compare_notes_input_live,
    compare_notes_input_strict,
    compare_task_layout,
    compare_transcript_structure,
    generate_report,
    parse_backend_stdout,
    run_python_cli,
    validate_minutes_file,
    validate_stdout_by_mode,
)

# ---------------------------------------------------------------------------
# Task layout comparison
# ---------------------------------------------------------------------------


def test_compare_task_layout_transcript() -> None:
    with tempfile.TemporaryDirectory() as tmp:
        py = Path(tmp) / "python"
        go = Path(tmp) / "go"
        for d in (py, go):
            (d / "audio").mkdir(parents=True)
            (d / "chunks").mkdir(parents=True)
            (d / "transcripts").mkdir(parents=True)
            (d / "transcript.md").write_text("content")

        checks = compare_task_layout("transcript", py, go)
        assert all(c.passed for c in checks), str(checks)


def test_compare_task_layout_notes() -> None:
    with tempfile.TemporaryDirectory() as tmp:
        py = Path(tmp) / "python"
        go = Path(tmp) / "go"
        for d in (py, go):
            (d / "audio").mkdir(parents=True)
            (d / "chunks").mkdir(parents=True)
            (d / "transcripts").mkdir(parents=True)
            (d / "transcript.md").write_text("content")
            (d / "notes_input.md").write_text("notes")

        checks = compare_task_layout("notes", py, go)
        assert all(c.passed for c in checks), str(checks)


def test_compare_task_layout_minutes() -> None:
    with tempfile.TemporaryDirectory() as tmp:
        py = Path(tmp) / "python"
        go = Path(tmp) / "go"
        for d in (py, go):
            (d / "audio").mkdir(parents=True)
            (d / "chunks").mkdir(parents=True)
            (d / "transcripts").mkdir(parents=True)
            (d / "transcript.md").write_text("content")
            (d / "notes_input.md").write_text("notes")
            (d / "meeting_minutes.md").write_text("minutes")

        checks = compare_task_layout("minutes", py, go)
        assert all(c.passed for c in checks), str(checks)


def test_compare_task_layout_missing_file() -> None:
    with tempfile.TemporaryDirectory() as tmp:
        py = Path(tmp) / "python"
        go = Path(tmp) / "go"
        (py / "audio").mkdir(parents=True)
        (py / "chunks").mkdir(parents=True)
        (py / "transcripts").mkdir(parents=True)
        (py / "transcript.md").write_text("content")

        checks = compare_task_layout("transcript", py, go)
        assert not all(c.passed for c in checks)


# ---------------------------------------------------------------------------
# Transcript structure comparison
# ---------------------------------------------------------------------------


def test_compare_transcript_structure_match() -> None:
    with tempfile.TemporaryDirectory() as tmp:
        content = "[Part 001]\n\nHello from part one.\n\n[Part 002]\n\nHello from part two.\n"
        py = Path(tmp) / "py_transcript.md"
        go = Path(tmp) / "go_transcript.md"
        py.write_text(content)
        go.write_text(content)

        result = compare_transcript_structure(py, go)
        assert result.passed, result.detail


def test_compare_transcript_structure_part_count_mismatch() -> None:
    with tempfile.TemporaryDirectory() as tmp:
        py = Path(tmp) / "py.md"
        go = Path(tmp) / "go.md"
        py.write_text("[Part 001]\n\na\n\n[Part 002]\n\nb\n")
        go.write_text("[Part 001]\n\na\n")

        result = compare_transcript_structure(py, go)
        assert not result.passed
        assert "part count mismatch" in result.detail


def test_compare_transcript_structure_order_mismatch() -> None:
    with tempfile.TemporaryDirectory() as tmp:
        # Reordered parts: Python 001,002 / Go 002,001.
        py = Path(tmp) / "py.md"
        go = Path(tmp) / "go.md"
        py.write_text("[Part 001]\n\na\n\n[Part 002]\n\nb\n")
        go.write_text("[Part 002]\n\nb\n\n[Part 001]\n\na\n")

        result = compare_transcript_structure(py, go)
        assert not result.passed
        assert "order mismatch" in result.detail


def test_compare_transcript_structure_duplicate_header() -> None:
    with tempfile.TemporaryDirectory() as tmp:
        p = Path(tmp) / "dup.md"
        p.write_text("[Part 001]\n\na\n\n[Part 001]\n\nb\n")

        result = compare_transcript_structure(p, p)  # same file → should fail on dup
        assert not result.passed
        assert "duplicate" in result.detail


def test_compare_transcript_structure_non_contiguous() -> None:
    with tempfile.TemporaryDirectory() as tmp:
        p = Path(tmp) / "noncontig.md"
        p.write_text("[Part 001]\n\na\n\n[Part 003]\n\nc\n")

        result = compare_transcript_structure(p, p)
        assert not result.passed
        assert "not contiguous" in result.detail


def test_compare_transcript_structure_trailing_newline() -> None:
    with tempfile.TemporaryDirectory() as tmp:
        py = Path(tmp) / "py.md"
        go = Path(tmp) / "go.md"
        py.write_text("[Part 001]\n\ntext\n")
        go.write_text("[Part 001]\n\ntext")

        result = compare_transcript_structure(py, go)
        assert not result.passed
        assert "trailing newline" in result.detail


# ---------------------------------------------------------------------------
# Notes input comparison
# ---------------------------------------------------------------------------


def test_compare_notes_strict_match() -> None:
    with tempfile.TemporaryDirectory() as tmp:
        content = "Prompt text\n\n---\n\n## Transcript\n\nTranscript text\n"
        py = Path(tmp) / "py_notes.md"
        go = Path(tmp) / "go_notes.md"
        py.write_text(content)
        go.write_text(content)

        result = compare_notes_input_strict(py, go)
        assert result.passed, result.detail


def test_compare_notes_strict_differ() -> None:
    with tempfile.TemporaryDirectory() as tmp:
        py = Path(tmp) / "py_notes.md"
        go = Path(tmp) / "go_notes.md"
        py.write_text("Prompt\n\n---\n\n## Transcript\n\nA\n")
        go.write_text("Prompt\n\n---\n\n## Transcript\n\nB\n")

        result = compare_notes_input_strict(py, go)
        assert not result.passed


def test_compare_notes_live_structure_ok() -> None:
    with tempfile.TemporaryDirectory() as tmp:
        # Different transcript text but same structure.
        py = Path(tmp) / "py_notes.md"
        go = Path(tmp) / "go_notes.md"
        py.write_text(
            "Prompt\n\n---\n\n## Transcript\n\n[Part 001]\n\nHello.\n"
        )
        go.write_text(
            "Prompt\n\n---\n\n## Transcript\n\n[Part 001]\n\nBonjour.\n"
        )

        result = compare_notes_input_live(py, go)
        assert result.passed, result.detail


def test_compare_notes_live_missing_separator() -> None:
    with tempfile.TemporaryDirectory() as tmp:
        py = Path(tmp) / "py_notes.md"
        go = Path(tmp) / "go_notes.md"
        py.write_text("Prompt\n\n## Transcript\n\ntext\n")
        go.write_text("Prompt\n\n---\n\n## Transcript\n\ntext\n")

        result = compare_notes_input_live(py, go)
        assert not result.passed
        assert "separator" in result.detail


def test_compare_notes_live_no_part_headers() -> None:
    with tempfile.TemporaryDirectory() as tmp:
        py = Path(tmp) / "py_notes.md"
        go = Path(tmp) / "go_notes.md"
        py.write_text("Prompt\n\n---\n\n## Transcript\n\nno headers here\n")
        go.write_text("Prompt\n\n---\n\n## Transcript\n\nno headers here\n")

        result = compare_notes_input_live(py, go)
        assert not result.passed
        assert "header" in result.detail.lower()


def test_compare_notes_live_empty_content() -> None:
    with tempfile.TemporaryDirectory() as tmp:
        py = Path(tmp) / "py_notes.md"
        go = Path(tmp) / "go_notes.md"
        # Part header exists but content is empty.
        text = "Prompt\n\n---\n\n## Transcript\n\n[Part 001]\n\n\n"
        py.write_text(text)
        go.write_text(text)

        result = compare_notes_input_live(py, go)
        assert not result.passed
        assert "empty" in result.detail


def test_compare_notes_live_prompt_mismatch() -> None:
    with tempfile.TemporaryDirectory() as tmp:
        py = Path(tmp) / "py_notes.md"
        go = Path(tmp) / "go_notes.md"
        py.write_text("Prompt A\n\n---\n\n## Transcript\n\n[Part 001]\n\nHello.\n")
        go.write_text("Prompt B\n\n---\n\n## Transcript\n\n[Part 001]\n\nHello.\n")

        result = compare_notes_input_live(py, go)
        assert not result.passed
        assert "prompt" in result.detail.lower()


# ---------------------------------------------------------------------------
# Minutes validation
# ---------------------------------------------------------------------------


def test_validate_minutes_file_valid() -> None:
    with tempfile.TemporaryDirectory() as tmp:
        p = Path(tmp) / "meeting_minutes.md"
        p.write_text("# Meeting Summary\n\nKey decisions.\n")

        result = validate_minutes_file(p)
        assert result.passed, result.detail


def test_validate_minutes_file_unresolved_placeholder() -> None:
    with tempfile.TemporaryDirectory() as tmp:
        p = Path(tmp) / "meeting_minutes.md"
        p.write_text("## Minutes\n\n{{ title }}\n\n{{ generated_at }}\n")

        result = validate_minutes_file(p)
        assert not result.passed
        assert "unresolved" in result.detail


def test_validate_minutes_file_empty() -> None:
    with tempfile.TemporaryDirectory() as tmp:
        p = Path(tmp) / "meeting_minutes.md"
        p.write_text("")

        result = validate_minutes_file(p)
        assert not result.passed
        assert "empty" in result.detail


def test_validate_minutes_file_missing() -> None:
    with tempfile.TemporaryDirectory() as tmp:
        p = Path(tmp) / "nonexistent.md"

        result = validate_minutes_file(p)
        assert not result.passed
        assert "not found" in result.detail


# ---------------------------------------------------------------------------
# stdout parsing
# ---------------------------------------------------------------------------


def test_parse_backend_stdout_full() -> None:
    stdout = (
        "Status: Splitting audio\n"
        "Progress: 1/5\n"
        "Progress: 2/5\n"
        "Status: Done\n"
        "Transcript saved: /tmp/out/transcript.md\n"
        "Notes input saved: /tmp/out/notes_input.md\n"
        "Meeting minutes saved: /tmp/out/meeting_minutes.md\n"
        "Task folder: /tmp/out/task\n"
    )

    parsed = parse_backend_stdout(stdout)
    assert parsed.task_folder == "/tmp/out/task"
    assert parsed.transcript_path == "/tmp/out/transcript.md"
    assert parsed.notes_input_path == "/tmp/out/notes_input.md"
    assert parsed.minutes_path == "/tmp/out/meeting_minutes.md"
    assert len(parsed.status_lines) >= 2
    assert len(parsed.progress_lines) == 2
    assert parsed.progress_lines[0] == (1, 5)


def test_parse_backend_stdout_ignores_unknown() -> None:
    stdout = (
        "Transcript saved: /out/t.md\n"
        "Some random informational line\n"
        "Another unknown line\n"
        "Task folder: /out/task\n"
    )

    parsed = parse_backend_stdout(stdout)
    assert parsed.transcript_path == "/out/t.md"
    assert parsed.task_folder == "/out/task"
    assert len(parsed.unrecognized) == 2


def test_parse_backend_stdout_empty() -> None:
    parsed = parse_backend_stdout("")
    assert parsed.task_folder is None
    assert parsed.transcript_path is None


def test_parse_backend_stdout_malformed_progress() -> None:
    stdout = "Progress: 5/3\nProgress: 0/5\nProgress: a/b\n"
    parsed = parse_backend_stdout(stdout)
    assert len(parsed.progress_lines) == 0
    assert len(parsed.progress_errors) == 3
    assert any("current > total" in e for e in parsed.progress_errors)
    assert any("positive" in e for e in parsed.progress_errors)
    assert any("not integers" in e for e in parsed.progress_errors)


# ---------------------------------------------------------------------------
# stdout contract validation by mode
# ---------------------------------------------------------------------------


def test_validate_stdout_transcript_missing_notes() -> None:
    parsed = parse_backend_stdout(
        "Status: Starting\nTranscript saved: /t.md\nTask folder: /t\n"
    )
    checks = validate_stdout_by_mode(parsed, "transcript")
    assert all(c.passed for c in checks), str(checks)


def test_validate_stdout_notes_missing_notes_line() -> None:
    parsed = parse_backend_stdout(
        "Status: Starting\nTranscript saved: /t.md\nTask folder: /t\n"
    )
    checks = validate_stdout_by_mode(parsed, "notes")
    passed = {c.name: c.passed for c in checks}
    assert passed["stdout_notes_saved"] is False
    assert passed["stdout_transcript_saved"] is True


def test_validate_stdout_minutes_all_required() -> None:
    parsed = parse_backend_stdout(
        "Status: Starting\n"
        "Transcript saved: /t.md\n"
        "Notes input saved: /n.md\n"
        "Meeting minutes saved: /m.md\n"
        "Task folder: /t\n"
    )
    checks = validate_stdout_by_mode(parsed, "minutes")
    assert all(c.passed for c in checks), str(checks)


def test_validate_stdout_minutes_missing_minutes_line() -> None:
    parsed = parse_backend_stdout(
        "Status: Starting\nTranscript saved: /t.md\nNotes input saved: /n.md\nTask folder: /t\n"
    )
    checks = validate_stdout_by_mode(parsed, "minutes")
    passed = {c.name: c.passed for c in checks}
    assert passed["stdout_minutes_saved"] is False
    assert passed["stdout_notes_saved"] is True


# ---------------------------------------------------------------------------
# Report generation
# ---------------------------------------------------------------------------


def test_generate_report_includes_exit_codes() -> None:
    result = ParityResult(
        mode="transcript",
        python_task_dir="/tmp/py",
        go_task_dir="/tmp/go",
        python_exit_code=0,
        go_exit_code=1,
        checks=[
            CheckResult("exit_code", False, "Python=0, Go=1"),
        ],
    )
    report = generate_report(result)
    assert "Python exit code: 0" in report
    assert "Go exit code: 1" in report
    assert "FAIL" in report


# ---------------------------------------------------------------------------
# Python CLI command construction
# ---------------------------------------------------------------------------


def test_python_cli_has_no_go_flags() -> None:
    """Python CLI command must not contain Go-only flags."""
    # run_python_cli constructs the command with only Python-supported flags.
    # Verify by inspecting the function's logic: it only adds
    # transcribe / audio / --output-mode / --output-dir.
    # We test indirectly by calling it with a nonexistent audio file;
    # the command must not fail with an unrecognized-argument error.

    code, _, stderr = run_python_cli("/nonexistent/audio.m4a", "transcript", "/tmp/py_out")
    # Python CLI should fail because the file doesn't exist,
    # NOT because of an unrecognized argument.
    assert "unrecognized" not in stderr.lower(), f"stderr contains unrecognized argument: {stderr}"
    assert "error: unrecognized" not in stderr.lower(), f"stderr: {stderr}"
    assert code != 0

"""Parity comparator utilities for Python/Go backend comparison.

All functions operate on task directories or file paths.
None require API keys or network access.
"""

from __future__ import annotations

import re
import subprocess
from dataclasses import dataclass, field
from pathlib import Path

# Task directory layout expected from both backends.
EXPECTED_TASK_LAYOUT: list[tuple[str, bool]] = [
    ("audio", True),
    ("chunks", True),
    ("transcripts", True),
    ("transcript.md", False),
]

NOTES_LAYOUT: list[tuple[str, bool]] = [
    ("notes_input.md", False),
]

MINUTES_LAYOUT: list[tuple[str, bool]] = [
    ("notes_input.md", False),
    ("meeting_minutes.md", False),
]

PART_HEADER_RE = re.compile(r"^\[Part (\d{3})\]\s*$")


@dataclass
class ParityResult:
    """Aggregated parity comparison result."""

    mode: str
    python_task_dir: str
    go_task_dir: str
    python_exit_code: int | None = None
    go_exit_code: int | None = None
    checks: list[CheckResult] = field(default_factory=list)
    warnings: list[str] = field(default_factory=list)

    @property
    def all_passed(self) -> bool:
        return all(c.passed for c in self.checks)

    def failed_checks(self) -> list[CheckResult]:
        return [c for c in self.checks if not c.passed]


@dataclass
class CheckResult:
    name: str
    passed: bool
    detail: str = ""


# ---------------------------------------------------------------------------
# Task layout comparison
# ---------------------------------------------------------------------------


def compare_task_layout(
    mode: str,
    python_task_dir: Path,
    go_task_dir: Path,
    result: ParityResult | None = None,
) -> list[CheckResult]:
    """Compare task folder structure between Python and Go outputs."""
    checks: list[CheckResult] = []

    layout = list(EXPECTED_TASK_LAYOUT)
    if mode in ("notes", "minutes"):
        layout.extend(NOTES_LAYOUT)
    if mode == "minutes":
        layout.extend(MINUTES_LAYOUT)

    for name, is_dir in layout:
        py_path = python_task_dir / name
        go_path = go_task_dir / name

        py_ok = py_path.is_dir() if is_dir else py_path.is_file()
        go_ok = go_path.is_dir() if is_dir else go_path.is_file()

        if py_ok and go_ok:
            checks.append(CheckResult(name, True))
        elif not py_ok and not go_ok:
            checks.append(CheckResult(name, False, f"missing in both: {name}"))
        elif not py_ok:
            checks.append(CheckResult(name, False, f"missing in Python: {py_path}"))
        else:
            checks.append(CheckResult(name, False, f"missing in Go: {go_path}"))

    if result is not None:
        result.checks.extend(checks)

    return checks


# ---------------------------------------------------------------------------
# Transcript structure comparison (order-sensitive, duplicate-safe)
# ---------------------------------------------------------------------------


def parse_transcript_parts_ordered(text: str) -> list[tuple[int, list[str]]]:
    """Parse transcript.md into an ordered sequence of (part_number, content_lines).

    Preserves order, detects duplicate headers, and returns every occurrence.
    """
    parts: list[tuple[int, list[str]]] = []
    current_num: int | None = None
    current_lines: list[str] = []

    for line in text.split("\n"):
        m = PART_HEADER_RE.match(line)
        if m:
            if current_num is not None:
                parts.append((current_num, current_lines))
                current_lines = []
            current_num = int(m.group(1))
        elif current_num is not None:
            current_lines.append(line)

    if current_num is not None:
        parts.append((current_num, current_lines))

    return parts


def compare_transcript_structure(
    python_transcript: Path,
    go_transcript: Path,
) -> CheckResult:
    """Compare transcript.md structure: order, duplicates, contiguity, trailing newline."""
    try:
        py_text = python_transcript.read_text(encoding="utf-8")
    except Exception as e:
        return CheckResult("transcript_structure", False, f"cannot read Python transcript: {e}")

    try:
        go_text = go_transcript.read_text(encoding="utf-8")
    except Exception as e:
        return CheckResult("transcript_structure", False, f"cannot read Go transcript: {e}")

    py_parts = parse_transcript_parts_ordered(py_text)
    go_parts = parse_transcript_parts_ordered(go_text)

    if not py_parts:
        return CheckResult("transcript_structure", False, "Python transcript has no [Part NNN] headers")
    if not go_parts:
        return CheckResult("transcript_structure", False, "Go transcript has no [Part NNN] headers")

    # Check for duplicate part headers.
    py_nums = [n for n, _ in py_parts]
    go_nums = [n for n, _ in go_parts]
    if len(py_nums) != len(set(py_nums)):
        return CheckResult("transcript_structure", False, "Python transcript contains duplicate [Part NNN] headers")
    if len(go_nums) != len(set(go_nums)):
        return CheckResult("transcript_structure", False, "Go transcript contains duplicate [Part NNN] headers")

    # Part count.
    if len(py_parts) != len(go_parts):
        return CheckResult(
            "transcript_structure", False,
            f"part count mismatch: Python={len(py_parts)}, Go={len(go_parts)}",
        )

    # Order must match.
    if py_nums != go_nums:
        return CheckResult(
            "transcript_structure", False,
            f"part order mismatch: Python={py_nums}, Go={go_nums}",
        )

    # Contiguous numbering starting at 1.
    expected = list(range(1, len(py_nums) + 1))
    for side, nums in [("Python", py_nums), ("Go", go_nums)]:
        if nums != expected:
            return CheckResult(
                "transcript_structure", False,
                f"{side} part numbers are not contiguous 1..{len(nums)}: got {nums}",
            )

    # Trailing newline.
    if not py_text.endswith("\n"):
        return CheckResult("transcript_structure", False, "Python transcript missing trailing newline")
    if not go_text.endswith("\n"):
        return CheckResult("transcript_structure", False, "Go transcript missing trailing newline")

    return CheckResult("transcript_structure", True)


# ---------------------------------------------------------------------------
# Notes input comparison
# ---------------------------------------------------------------------------


def compare_notes_input_strict(
    python_notes: Path,
    go_notes: Path,
) -> CheckResult:
    """Exact byte-for-byte notes_input.md comparison.

    Only valid for offline fixtures where both inputs are known identical.
    """
    try:
        py_text = python_notes.read_text(encoding="utf-8")
    except Exception as e:
        return CheckResult("notes_input_strict", False, f"cannot read Python notes_input.md: {e}")

    try:
        go_text = go_notes.read_text(encoding="utf-8")
    except Exception as e:
        return CheckResult("notes_input_strict", False, f"cannot read Go notes_input.md: {e}")

    if py_text == go_text:
        return CheckResult("notes_input_strict", True)

    return CheckResult("notes_input_strict", False, "notes_input.md content differs (strict byte comparison)")


def compare_notes_input_live(
    python_notes: Path,
    go_notes: Path,
) -> CheckResult:
    """Structural notes_input.md comparison for live parity runs.

    Does NOT require byte-identical content (ASR text may differ).
    Validates: prompt section equality, separator, transcript header,
    non-empty embedded transcript with valid ordered structure.
    """
    issues: list[str] = []

    try:
        py_text = python_notes.read_text(encoding="utf-8")
    except Exception as e:
        return CheckResult("notes_input_live", False, f"cannot read Python notes: {e}")

    try:
        go_text = go_notes.read_text(encoding="utf-8")
    except Exception as e:
        return CheckResult("notes_input_live", False, f"cannot read Go notes: {e}")

    # Split each file into prompt section (before ## Transcript) and transcript section.
    for side, text in [("Python", py_text), ("Go", go_text)]:
        # Must have separator and header.
        if "---" not in text:
            issues.append(f"{side}: missing '---' separator")
            continue
        if "## Transcript" not in text:
            issues.append(f"{side}: missing '## Transcript' header")
            continue

        # The transcript section after ## Transcript must begin with valid part headers.
        ts_idx = text.find("## Transcript")
        transcript_section = text[ts_idx + len("## Transcript"):]
        parts = parse_transcript_parts_ordered(transcript_section)
        if not parts:
            issues.append(f"{side}: no [Part NNN] headers found in transcript section")
            continue

        # Validate transcript structure.
        nums = [n for n, _ in parts]
        if len(nums) != len(set(nums)):
            issues.append(f"{side}: duplicate [Part NNN] in transcript section")
        expected = list(range(1, len(nums) + 1))
        if nums != expected:
            issues.append(
                f"{side}: transcript part numbers not contiguous 1..{len(nums)}: {nums}"
            )

        # Every part must have non-empty content.
        for n, lines in parts:
            content_text = "\n".join(lines).strip()
            if not content_text:
                issues.append(f"{side}: [Part {n:03d}] has empty transcript content")

    if issues:
        return CheckResult("notes_input_live", False, "; ".join(issues))

    # Compare prompt sections for exact equality.
    # Use the full notes-input delimiter to avoid false positives if
    # the prompt text itself contains a horizontal rule.
    delimiter = "\n\n---\n\n## Transcript\n\n"
    py_prompt = (py_text.split(delimiter)[0] if delimiter in py_text else "").strip()
    go_prompt = (go_text.split(delimiter)[0] if delimiter in go_text else "").strip()
    if py_prompt != go_prompt:
        return CheckResult(
            "notes_input_live", False,
            "prompt sections differ — may use different prompt sources",
        )

    return CheckResult("notes_input_live", True)


# ---------------------------------------------------------------------------
# Minutes validation (nondeterminism-safe)
# ---------------------------------------------------------------------------


UNRESOLVED_PLACEHOLDER_RE = re.compile(r"\{\{\s*(title|generated_at|generated_minutes|transcript|task_name)\s*\}\}")


def validate_minutes_file(path: Path) -> CheckResult:
    """Validate meeting_minutes.md existence, non-empty content, and rendered shape.

    Raw template placeholders are NOT required (template rendering replaces them).
    UNRESOLVED placeholders are treated as failures.
    """
    if not path.exists():
        return CheckResult("meeting_minutes", False, f"file not found: {path}")
    if not path.is_file():
        return CheckResult("meeting_minutes", False, f"not a file: {path}")

    try:
        content = path.read_text(encoding="utf-8")
    except Exception as e:
        return CheckResult("meeting_minutes", False, f"cannot read: {e}")

    if not content.strip():
        return CheckResult("meeting_minutes", False, "file is empty")

    # Reject unresolved template placeholders.
    unresolved = UNRESOLVED_PLACEHOLDER_RE.findall(content)
    if unresolved:
        return CheckResult(
            "meeting_minutes",
            False,
            f"unresolved template placeholders: {', '.join(f'{{{{ {p} }}}}' for p in unresolved)}",
        )

    # Check for basic document characteristics (optional — warn only).
    warnings: list[str] = []
    if "#" not in content:
        warnings.append("no markdown headings found (may be valid plain text)")

    if warnings:
        return CheckResult(
            "meeting_minutes", True, f"non-empty; advisory: {'; '.join(warnings)}"
        )

    return CheckResult("meeting_minutes", True)


# ---------------------------------------------------------------------------
# stdout parsing
# ---------------------------------------------------------------------------


@dataclass
class ParsedStdout:
    task_folder: str | None = None
    transcript_path: str | None = None
    notes_input_path: str | None = None
    minutes_path: str | None = None
    progress_lines: list[tuple[int, int]] = field(default_factory=list)
    status_lines: list[str] = field(default_factory=list)
    progress_errors: list[str] = field(default_factory=list)
    unrecognized: list[str] = field(default_factory=list)


def parse_backend_stdout(stdout: str) -> ParsedStdout:
    """Parse known backend stdout lines into a structured result.

    Validates Progress: N/M strictly (positive integers, current <= total).
    Malformed progress lines are recorded as errors.
    """
    parsed = ParsedStdout()

    for line in stdout.split("\n"):
        trimmed = line.strip()
        if not trimmed:
            continue

        if trimmed.startswith("Status: "):
            value = trimmed[len("Status: "):].strip()
            if value:
                parsed.status_lines.append(value)
        elif trimmed.startswith("Progress: "):
            value = trimmed[len("Progress: "):].strip()
            parts = value.split("/")
            if len(parts) == 2:
                try:
                    current = int(parts[0])
                    total = int(parts[1])
                    if current <= 0 or total <= 0:
                        parsed.progress_errors.append(
                            f"Progress values must be positive: {trimmed}"
                        )
                    elif current > total:
                        parsed.progress_errors.append(
                            f"Progress current > total: {trimmed}"
                        )
                    else:
                        parsed.progress_lines.append((current, total))
                except ValueError:
                    parsed.progress_errors.append(
                        f"Progress values not integers: {trimmed}"
                    )
            else:
                parsed.progress_errors.append(
                    f"Malformed Progress line (expected N/M): {trimmed}"
                )
        elif trimmed.startswith("Transcript saved:"):
            value = trimmed[len("Transcript saved:"):].strip()
            if value:
                parsed.transcript_path = value
        elif trimmed.startswith("Notes input saved:"):
            value = trimmed[len("Notes input saved:"):].strip()
            if value:
                parsed.notes_input_path = value
        elif trimmed.startswith("Meeting minutes saved:"):
            value = trimmed[len("Meeting minutes saved:"):].strip()
            if value:
                parsed.minutes_path = value
        elif trimmed.startswith("Task folder:"):
            value = trimmed[len("Task folder:"):].strip()
            if value:
                parsed.task_folder = value
        else:
            parsed.unrecognized.append(trimmed)

    return parsed


def validate_stdout_by_mode(parsed: ParsedStdout, mode: str, prefix: str = "") -> list[CheckResult]:
    """Validate stdout fields required for a given output mode.

    If prefix is provided (e.g. 'python_', 'go_'), check names are prefixed
    so report consumers can identify which backend failed.
    """
    p = prefix
    checks: list[CheckResult] = []

    # At least one Status line (all modes).
    checks.append(
        CheckResult(f"{p}stdout_status", bool(parsed.status_lines),
                     f"found {len(parsed.status_lines)} Status lines")
    )

    # Transcript saved (all modes).
    checks.append(
        CheckResult(f"{p}stdout_transcript_saved", parsed.transcript_path is not None,
                     "Transcript saved found" if parsed.transcript_path else "missing Transcript saved")
    )

    # Task folder (all modes).
    checks.append(
        CheckResult(f"{p}stdout_task_folder", parsed.task_folder is not None,
                     "Task folder found" if parsed.task_folder else "missing Task folder")
    )

    # Notes input saved (notes and minutes modes).
    if mode in ("notes", "minutes"):
        checks.append(
            CheckResult(f"{p}stdout_notes_saved", parsed.notes_input_path is not None,
                         "Notes input saved found" if parsed.notes_input_path else "missing Notes input saved")
        )

    # Meeting minutes saved (minutes mode only).
    if mode == "minutes":
        checks.append(
            CheckResult(f"{p}stdout_minutes_saved", parsed.minutes_path is not None,
                         "Meeting minutes saved found" if parsed.minutes_path else "missing Meeting minutes saved")
        )

    # Progress errors.
    if parsed.progress_errors:
        checks.append(
            CheckResult(f"{p}stdout_progress_valid", False,
                         "; ".join(parsed.progress_errors))
        )

    return checks


# ---------------------------------------------------------------------------
# CLI runner helpers
# ---------------------------------------------------------------------------

def run_python_cli(
    audio: str,
    mode: str,
    output_dir: str,
    *,
    _env_file: str | None = None,
    _project_root: str | None = None,
) -> tuple[int, str, str]:
    """Run the Python CLI and return (exit_code, stdout, stderr).

    Python CLI does NOT support --env-file or --project-root.
    Those are passed for Go-only compatibility in the harness;
    they are silently ignored here. The Python CLI uses the repo-root
    .env file when run from the project root.
    """
    import sys

    cmd = [
        sys.executable, "-m", "meeting_asr.main", "transcribe",
        audio, "--output-mode", mode, "--output-dir", output_dir,
    ]
    proc = subprocess.run(cmd, capture_output=True, text=True)
    return proc.returncode, proc.stdout, proc.stderr


def run_go_cli(
    audio: str,
    mode: str,
    output_dir: str,
    go_binary: str,
    env_file: str | None = None,
    project_root: str | None = None,
) -> tuple[int, str, str]:
    """Run the Go CLI and return (exit_code, stdout, stderr)."""
    cmd = [
        go_binary, "transcribe",
        audio, "--output-mode", mode, "--output-dir", output_dir,
    ]
    if env_file:
        cmd.extend(["--env-file", env_file])
    elif project_root:
        cmd.extend(["--project-root", project_root])

    proc = subprocess.run(cmd, capture_output=True, text=True)
    return proc.returncode, proc.stdout, proc.stderr


# ---------------------------------------------------------------------------
# Report generation
# ---------------------------------------------------------------------------


def generate_report(result: ParityResult) -> str:
    """Generate a Markdown parity report string."""
    lines: list[str] = []
    lines.append(f"# Parity Report — {result.mode} mode\n")
    lines.append(f"- Python task dir: `{result.python_task_dir}`")
    lines.append(f"- Go task dir: `{result.go_task_dir}`")
    if result.python_exit_code is not None:
        lines.append(f"- Python exit code: {result.python_exit_code}")
    if result.go_exit_code is not None:
        lines.append(f"- Go exit code: {result.go_exit_code}")
    lines.append("")

    passed = sum(1 for c in result.checks if c.passed)
    total = len(result.checks)
    lines.append(f"## Results: {passed}/{total} checks passed\n")

    for c in result.checks:
        status = "PASS" if c.passed else "FAIL"
        detail = f" — {c.detail}" if c.detail else ""
        lines.append(f"- **{c.name}**: {status}{detail}")

    if result.warnings:
        lines.append("\n## Warnings\n")
        for w in result.warnings:
            lines.append(f"- {w}")

    if not result.all_passed:
        lines.append("\n## Failed checks\n")
        for c in result.failed_checks():
            lines.append(f"- **{c.name}**: {c.detail}")

    return "\n".join(lines) + "\n"

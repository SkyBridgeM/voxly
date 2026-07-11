"""Reusable, offline-safe comparators for Python/Go parity validation.

The functions in this module only inspect files and stdout text.  They never
invoke a backend, ffmpeg, or a network service, so they are suitable for CI.
"""

from __future__ import annotations

import hashlib
import json
import re
from pathlib import Path
from typing import Any


VALID_MODES = {"transcript", "notes", "minutes"}
NOTES_DELIMITER = "\n\n---\n\n## Transcript\n\n"
PART_HEADER_RE = re.compile(r"(?m)^\[Part (\d{3})\]\n\n")
PART_LIKE_LINE_RE = re.compile(r"(?m)^\[Part [^\]\r\n]+\].*$")
UNRESOLVED_TEMPLATE_RE = re.compile(r"\{\{\s*[^{}]+\s*\}\}")
KNOWN_TEMPLATE_PLACEHOLDERS = {
    "{{ title }}",
    "{{ generated_at }}",
    "{{ generated_minutes }}",
    "{{ transcript }}",
    "{{ task_name }}",
}
PROGRESS_RE = re.compile(r"^Progress:\s*(\d+)\s*/\s*(\d+)\s*$")


def _result(*, passed: bool = True) -> dict[str, Any]:
    return {"passed": passed, "checks": [], "differences": [], "warnings": []}


def _check(result: dict[str, Any], name: str, passed: bool, detail: str) -> None:
    result["checks"].append({"name": name, "passed": passed, "detail": detail})
    if not passed:
        result["passed"] = False
        result["differences"].append(detail)


def _warning(result: dict[str, Any], detail: str) -> None:
    result["warnings"].append(detail)


def _regular_nonempty(path: Path) -> bool:
    return path.is_file() and path.stat().st_size > 0


def _mode_artifacts(mode: str) -> list[str]:
    if mode not in VALID_MODES:
        raise ValueError(f"Unsupported parity mode: {mode!r}")
    artifacts = ["transcript.md"]
    if mode in {"notes", "minutes"}:
        artifacts.append("notes_input.md")
    if mode == "minutes":
        artifacts.append("meeting_minutes.md")
    return artifacts


def compare_task_layout(mode: str, python_task_dir: str | Path, go_task_dir: str | Path) -> dict[str, Any]:
    """Check the required task layout for each backend without rejecting extras.

    Go's manifest and lock file are intentional backend-specific extras, so
    this comparator checks the shared required layout rather than requiring an
    identical directory listing.
    """

    result = _result()
    required_dirs = ("audio", "chunks", "transcripts")
    for backend, raw_dir in (("python", python_task_dir), ("go", go_task_dir)):
        task_dir = Path(raw_dir)
        _check(result, f"{backend}.task_dir", task_dir.is_dir(), f"{backend} task directory is missing: {task_dir}")
        for directory in required_dirs:
            path = task_dir / directory
            _check(result, f"{backend}.{directory}", path.is_dir(), f"{backend} required directory is missing: {path}")
        for artifact in _mode_artifacts(mode):
            path = task_dir / artifact
            _check(
                result,
                f"{backend}.{artifact}",
                _regular_nonempty(path),
                f"{backend} required artifact is missing or empty: {path}",
            )
    return result


def _parse_transcript_text(text: str, label: str) -> tuple[list[int], list[str]]:
    errors: list[str] = []
    if not text.endswith("\n"):
        errors.append(f"{label} must end with a newline")

    matches = list(PART_HEADER_RE.finditer(text))
    part_like_lines = PART_LIKE_LINE_RE.findall(text)
    if len(matches) != len(part_like_lines):
        errors.append(f"{label} contains malformed part header formatting")
    if not matches:
        errors.append(f"{label} contains no valid [Part 001] headers")
        return [], errors

    parts = [int(match.group(1)) for match in matches]
    if len(parts) != len(set(parts)):
        errors.append(f"{label} contains duplicate part headers")
    expected = list(range(1, len(parts) + 1))
    if parts != expected:
        errors.append(f"{label} part headers must be contiguous and ordered from 001; found {parts}")

    for index, match in enumerate(matches):
        end = matches[index + 1].start() if index + 1 < len(matches) else len(text)
        if not text[match.end() : end].strip():
            errors.append(f"{label} part {parts[index]:03d} is empty")
    return parts, errors


def compare_transcript_structure(python_transcript: str | Path, go_transcript: str | Path) -> dict[str, Any]:
    """Compare deterministic transcript shape, but not ASR body text."""

    result = _result()
    parsed: dict[str, list[int]] = {}
    for backend, raw_path in (("python", python_transcript), ("go", go_transcript)):
        path = Path(raw_path)
        if not _regular_nonempty(path):
            _check(result, f"{backend}.transcript", False, f"{backend} transcript is missing or empty: {path}")
            parsed[backend] = []
            continue
        text = path.read_text(encoding="utf-8")
        parts, errors = _parse_transcript_text(text, f"{backend} transcript")
        parsed[backend] = parts
        _check(result, f"{backend}.transcript_structure", not errors, "; ".join(errors) or f"{backend} transcript structure is valid")

    if parsed["python"] != parsed["go"]:
        _check(
            result,
            "transcript.part_order",
            False,
            f"transcript part header order differs: python={parsed['python']}, go={parsed['go']}",
        )
    else:
        _check(result, "transcript.part_order", True, "transcript part header order matches")
    return result


def compare_notes_input(python_notes: str | Path, go_notes: str | Path) -> dict[str, Any]:
    """Perform the strict, byte-for-byte notes comparison used by offline fixtures."""

    result = _result()
    python_path, go_path = Path(python_notes), Path(go_notes)
    if not _regular_nonempty(python_path) or not _regular_nonempty(go_path):
        _check(result, "notes.exact", False, "both notes_input.md files must exist and be non-empty for exact comparison")
        return result

    python_bytes, go_bytes = python_path.read_bytes(), go_path.read_bytes()
    if python_bytes == go_bytes:
        _check(result, "notes.exact", True, "notes_input.md is byte-identical (offline strict comparison)")
        result["comparison_kind"] = "exact_offline"
        return result

    first_difference = next(
        (index for index, (left, right) in enumerate(zip(python_bytes, go_bytes)) if left != right),
        min(len(python_bytes), len(go_bytes)),
    )
    _check(
        result,
        "notes.exact",
        False,
        "notes_input.md differs (byte comparison): "
        f"python_bytes={len(python_bytes)}, go_bytes={len(go_bytes)}, first_difference_offset={first_difference}, "
        f"python_sha256={hashlib.sha256(python_bytes).hexdigest()}, go_sha256={hashlib.sha256(go_bytes).hexdigest()}",
    )
    result["comparison_kind"] = "exact_offline"
    return result


def _parse_notes_structure(path: Path, label: str) -> tuple[str, str, list[str]]:
    if not _regular_nonempty(path):
        return "", "", [f"{label} is missing or empty: {path}"]
    text = path.read_text(encoding="utf-8")
    if text.count(NOTES_DELIMITER) != 1:
        return "", "", [f"{label} must contain exactly one notes separator and transcript header"]
    prompt, transcript = text.split(NOTES_DELIMITER, 1)
    errors: list[str] = []
    if not prompt.strip():
        errors.append(f"{label} prompt section is empty")
    _, transcript_errors = _parse_transcript_text(transcript, f"{label} embedded transcript")
    errors.extend(transcript_errors)
    return prompt, transcript, errors


def compare_notes_input_structure(
    python_notes: str | Path, go_notes: str | Path, *, require_matching_prompt: bool = True
) -> dict[str, Any]:
    """Compare live-safe notes shape without comparing independently generated ASR text."""

    result = _result()
    python_prompt, _, python_errors = _parse_notes_structure(Path(python_notes), "python notes_input.md")
    go_prompt, _, go_errors = _parse_notes_structure(Path(go_notes), "go notes_input.md")
    _check(result, "python.notes.structure", not python_errors, "; ".join(python_errors) or "python notes structure is valid")
    _check(result, "go.notes.structure", not go_errors, "; ".join(go_errors) or "go notes structure is valid")
    if require_matching_prompt:
        _check(result, "notes.prompt", python_prompt == go_prompt, "notes prompt sections differ" if python_prompt != go_prompt else "notes prompt sections match")
    else:
        _warning(result, "notes prompt consistency was not required for this comparison")
    result["comparison_kind"] = "structural_live"
    return result


def validate_minutes_file(path: str | Path) -> dict[str, Any]:
    """Validate a rendered minutes document without comparing nondeterministic LLM text."""

    result = _result()
    minutes_path = Path(path)
    if not minutes_path.is_file():
        _check(result, "minutes.file", False, f"minutes file is missing or not regular: {minutes_path}")
        return result
    text = minutes_path.read_text(encoding="utf-8")
    _check(result, "minutes.nonempty", bool(text.strip()), f"minutes file is empty: {minutes_path}")
    if not text.strip():
        return result
    # Raw LLM Markdown can legitimately discuss a code template such as
    # ``{{ customer_name }}``. Only fail on placeholders that this project
    # itself renders through the Obsidian template.
    placeholders = set(UNRESOLVED_TEMPLATE_RE.findall(text))
    unresolved = sorted(placeholders & KNOWN_TEMPLATE_PLACEHOLDERS)
    _check(
        result,
        "minutes.placeholders",
        not unresolved,
        (
            "minutes file contains unresolved project template placeholders: " + ", ".join(unresolved)
            if unresolved
            else "minutes file contains no unresolved project template placeholders"
        ),
    )
    if not re.search(r"(?m)^#{1,6}\s+\S", text):
        _warning(result, "minutes file has no Markdown heading; this is advisory because custom templates may omit headings")
    if text.startswith("---\n") and "\n---\n" not in text[4:]:
        _warning(result, "minutes file starts frontmatter but has no closing delimiter")
    return result


def parse_backend_stdout(stdout: str) -> dict[str, Any]:
    """Parse known stable stdout lines and retain unknown informational lines."""

    parsed: dict[str, Any] = {
        "status": [],
        "progress": [],
        "artifacts": {},
        "task_folders": [],
        "unknown": [],
        "errors": [],
    }
    prefixes = {
        "Transcript saved:": "transcript",
        "Notes input saved:": "notes_input",
        "Meeting minutes saved:": "minutes",
    }
    for raw_line in stdout.splitlines():
        line = raw_line.strip()
        if not line:
            continue
        if line.startswith("Status:"):
            value = line.removeprefix("Status:").strip()
            if value:
                parsed["status"].append(value)
            else:
                parsed["errors"].append("malformed Status line")
            continue
        if line.startswith("Progress:"):
            match = PROGRESS_RE.match(line)
            if not match:
                parsed["errors"].append(f"malformed progress line: {line}")
                continue
            current, total = int(match.group(1)), int(match.group(2))
            if current < 1 or total < 1 or current > total:
                parsed["errors"].append(f"invalid progress values: {line}")
            else:
                parsed["progress"].append({"current": current, "total": total})
            continue
        if line.startswith("Task folder:"):
            value = line.removeprefix("Task folder:").strip()
            if value:
                parsed["task_folders"].append(value)
            else:
                parsed["errors"].append("malformed Task folder line")
            continue
        matched = False
        for prefix, artifact_name in prefixes.items():
            if line.startswith(prefix):
                value = line.removeprefix(prefix).strip()
                if value:
                    parsed["artifacts"][artifact_name] = value
                else:
                    parsed["errors"].append(f"malformed {prefix} line")
                matched = True
                break
        if not matched:
            parsed["unknown"].append(line)
    return parsed


def validate_stdout_contract(parsed: dict[str, Any], mode: str) -> dict[str, Any]:
    """Validate the required stdout fields for a successful backend invocation."""

    if mode not in VALID_MODES:
        raise ValueError(f"Unsupported parity mode: {mode!r}")
    result = _result()
    errors = parsed.get("errors", [])
    _check(result, "stdout.syntax", not errors, "; ".join(errors) or "known stdout lines are well-formed")
    _check(result, "stdout.status", bool(parsed.get("status")), "stdout is missing a Status line")
    _check(result, "stdout.transcript", bool(parsed.get("artifacts", {}).get("transcript")), "stdout is missing Transcript saved")
    _check(result, "stdout.task_folder", bool(parsed.get("task_folders")), "stdout is missing Task folder")
    if mode in {"notes", "minutes"}:
        _check(result, "stdout.notes", bool(parsed.get("artifacts", {}).get("notes_input")), "stdout is missing Notes input saved")
    if mode == "minutes":
        _check(result, "stdout.minutes", bool(parsed.get("artifacts", {}).get("minutes")), "stdout is missing Meeting minutes saved")
    for line in parsed.get("unknown", []):
        _warning(result, f"unknown stdout line tolerated: {line}")
    return result


def generate_report(result: dict[str, Any]) -> str:
    """Return a machine-readable JSON report without interpolating secrets."""

    report = dict(result)
    report.setdefault("passed", not report.get("differences"))
    report.setdefault("checks", [])
    report.setdefault("warnings", [])
    report.setdefault("differences", [])
    return json.dumps(report, ensure_ascii=False, indent=2, sort_keys=True) + "\n"

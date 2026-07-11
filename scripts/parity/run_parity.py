#!/usr/bin/env python3
"""Parity harness — run Python and Go backends and compare outputs.

Usage:
    python scripts/parity/run_parity.py \
      --audio path/to/sample.m4a \
      --mode transcript \
      --python-output output/parity/python \
      --go-output output/parity/go \
      --go-binary core-go/meeting-asr-go \
      --env-file .env

Supports modes: transcript, notes, minutes.

Requires:
    - A real audio file (.m4a, .mp3, .wav)
    - MIMO_API_KEY in .env (all modes)
    - DEEPSEEK_API_KEY in .env (minutes mode only)
    - ffmpeg on PATH
    - Go binary built (go build -o meeting-asr-go .)
"""

from __future__ import annotations

import argparse
import shutil
import sys
from pathlib import Path

# Allow importing from repo root.
sys.path.insert(0, str(Path(__file__).resolve().parent.parent.parent))

from scripts.parity.parity_compare import (  # noqa: E402
    ParityResult,
    CheckResult,
    compare_notes_input_live,
    compare_task_layout,
    compare_transcript_structure,
    generate_report,
    run_go_cli,
    run_python_cli,
    validate_minutes_file,
    parse_backend_stdout,
    validate_stdout_by_mode,
)


def resolve_path(path: str) -> Path:
    """Resolve a path argument, erroring if it does not exist."""
    p = Path(path).expanduser().resolve()
    if not p.exists():
        print(f"File not found: {p}", file=sys.stderr)
        sys.exit(1)
    return p


def find_task_dir(output_dir: Path, name_hint: str | None = None) -> Path | None:
    """Find the most recently created task directory under output_dir."""
    dirs = sorted(
        (d for d in output_dir.iterdir() if d.is_dir()),
        key=lambda d: d.stat().st_ctime,
        reverse=True,
    )
    if not dirs:
        return None
    return dirs[0]


def main() -> None:
    parser = argparse.ArgumentParser(description="Python/Go parity harness")
    parser.add_argument("--audio", required=True, help="Path to sample audio file")
    parser.add_argument(
        "--mode",
        required=True,
        choices=["transcript", "notes", "minutes"],
        help="Output mode",
    )
    parser.add_argument(
        "--python-output",
        required=True,
        help="Output directory for Python run",
    )
    parser.add_argument(
        "--go-output",
        required=True,
        help="Output directory for Go run",
    )
    parser.add_argument(
        "--go-binary",
        required=True,
        help="Path to Go binary (core-go/meeting-asr-go)",
    )
    parser.add_argument(
        "--env-file",
        help="Path to .env file with API keys",
    )
    parser.add_argument(
        "--project-root",
        help="Project root directory (alternative to --env-file)",
    )
    parser.add_argument(
        "--report-dir",
        default="output/parity/reports",
        help="Directory for parity report output",
    )

    args = parser.parse_args()

    audio_path = resolve_path(args.audio)
    go_binary = resolve_path(args.go_binary)
    python_output = Path(args.python_output).resolve()
    go_output = Path(args.go_output).resolve()
    report_dir = Path(args.report_dir).resolve()

    # Clean previous runs.
    if python_output.exists():
        shutil.rmtree(python_output)
    if go_output.exists():
        shutil.rmtree(go_output)
    python_output.mkdir(parents=True, exist_ok=True)
    go_output.mkdir(parents=True, exist_ok=True)
    report_dir.mkdir(parents=True, exist_ok=True)

    print(f"Audio:    {audio_path}")
    print(f"Mode:     {args.mode}")
    print(f"Go bin:   {go_binary}")

    # --- Run Python CLI ---
    print("\n--- Running Python backend ---")
    py_code, py_stdout, py_stderr = run_python_cli(
        str(audio_path),
        args.mode,
        str(python_output),
    )
    py_task = find_task_dir(python_output)
    py_stdout_path = python_output / "stdout.txt"
    py_stderr_path = python_output / "stderr.txt"
    py_stdout_path.write_text(py_stdout)
    py_stderr_path.write_text(py_stderr)

    print(f"  Exit code: {py_code}")
    print(f"  Task dir:  {py_task}")

    # --- Run Go CLI ---
    print("\n--- Running Go backend ---")
    go_code, go_stdout, go_stderr = run_go_cli(
        str(audio_path),
        args.mode,
        str(go_output),
        go_binary=str(go_binary),
        env_file=args.env_file,
        project_root=args.project_root,
    )
    go_task = find_task_dir(go_output)
    go_stdout_path = go_output / "stdout.txt"
    go_stderr_path = go_output / "stderr.txt"
    go_stdout_path.write_text(go_stdout)
    go_stderr_path.write_text(go_stderr)

    print(f"  Exit code: {go_code}")
    print(f"  Task dir:  {go_task}")

    # --- Compare ---
    print("\n--- Parity checks ---")
    result = ParityResult(
        mode=args.mode,
        python_task_dir=str(py_task or "NOT FOUND"),
        go_task_dir=str(go_task or "NOT FOUND"),
        python_exit_code=py_code,
        go_exit_code=go_code,
    )

    # Exit code check.
    result.checks.append(
        CheckResult(
            "exit_code",
            py_code == go_code,
            f"Python={py_code}, Go={go_code}",
        )
    )

    if py_task is None:
        result.checks.append(
            CheckResult("python_task_dir", False, "Python task directory not found")
        )
    if go_task is None:
        result.checks.append(
            CheckResult("go_task_dir", False, "Go task directory not found")
        )

    # Task layout comparison.
    if py_task is not None and go_task is not None:
        compare_task_layout(args.mode, py_task, go_task, result)

        # Transcript comparison.
        py_transcript = py_task / "transcript.md"
        go_transcript = go_task / "transcript.md"
        result.checks.append(compare_transcript_structure(py_transcript, go_transcript))

        # Notes comparison (notes/minutes modes).
        if args.mode in ("notes", "minutes"):
            py_notes = py_task / "notes_input.md"
            go_notes = go_task / "notes_input.md"
            if py_notes.exists() and go_notes.exists():
                result.checks.append(compare_notes_input_live(py_notes, go_notes))
            else:
                result.checks.append(
                    CheckResult(
                        "notes_input",
                        False,
                        f"Python exists={py_notes.exists()}, Go exists={go_notes.exists()}",
                    )
                )

        # Minutes validation (minutes mode only).
        if args.mode == "minutes":
            py_minutes = py_task / "meeting_minutes.md"
            go_minutes = go_task / "meeting_minutes.md"
            result.checks.append(validate_minutes_file(py_minutes))
            result.checks.append(validate_minutes_file(go_minutes))

    # stdout contract check (mode-aware).
    py_parsed = parse_backend_stdout(py_stdout)
    go_parsed = parse_backend_stdout(go_stdout)
    result.checks.extend(validate_stdout_by_mode(py_parsed, args.mode, prefix="python_"))
    result.checks.extend(validate_stdout_by_mode(go_parsed, args.mode, prefix="go_"))

    # --- Report ---
    print("\n--- Results ---")
    passed = sum(1 for c in result.checks if c.passed)
    total = len(result.checks)
    print(f"{passed}/{total} checks passed")

    for c in result.checks:
        status = "PASS" if c.passed else "FAIL"
        detail = f" — {c.detail}" if c.detail else ""
        print(f"  [{status}] {c.name}{detail}")

    report_path = report_dir / f"parity_{args.mode}.md"
    report_path.write_text(generate_report(result))
    print(f"\nReport written to: {report_path}")

    sys.exit(0 if result.all_passed else 1)


if __name__ == "__main__":
    main()

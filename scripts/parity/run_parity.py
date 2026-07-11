#!/usr/bin/env python3
"""Optional live Python/Go parity runner.

This is deliberately not used by automated tests: it invokes both backends,
which require real audio, ffmpeg, and (for transcription) API credentials.
"""

from __future__ import annotations

import argparse
import os
import subprocess
import sys
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

try:
    from .parity_compare import (
        VALID_MODES,
        compare_notes_input_structure,
        compare_task_layout,
        compare_transcript_structure,
        generate_report,
        parse_backend_stdout,
        validate_minutes_file,
        validate_stdout_contract,
    )
except ImportError:  # Direct execution: python scripts/parity/run_parity.py
    from parity_compare import (  # type: ignore[no-redef]
        VALID_MODES,
        compare_notes_input_structure,
        compare_task_layout,
        compare_transcript_structure,
        generate_report,
        parse_backend_stdout,
        validate_minutes_file,
        validate_stdout_contract,
    )


REPO_ROOT = Path(__file__).resolve().parents[2]


def build_python_command(audio: Path, mode: str, output_dir: Path) -> list[str]:
    """Build a valid Python command; Python intentionally has no --env-file flag."""

    return [
        sys.executable,
        "-m",
        "meeting_asr.main",
        "transcribe",
        str(audio),
        "--output-mode",
        mode,
        "--output-dir",
        str(output_dir),
    ]


def build_go_command(audio: Path, mode: str, output_dir: Path, go_binary: Path, env_file: Path | None) -> list[str]:
    command = [
        str(go_binary),
        "transcribe",
        str(audio),
        "--output-mode",
        mode,
        "--output-dir",
        str(output_dir),
    ]
    if env_file is not None:
        command.extend(["--env-file", str(env_file)])
    return command


def _run(command: list[str], env: dict[str, str]) -> dict[str, Any]:
    try:
        completed = subprocess.run(command, cwd=REPO_ROOT, env=env, text=True, capture_output=True, check=False)
        return {"command": command, "exit_code": completed.returncode, "stdout": completed.stdout, "stderr": completed.stderr}
    except OSError as exc:
        return {"command": command, "exit_code": 127, "stdout": "", "stderr": str(exc)}


def _add_comparison(report: dict[str, Any], name: str, result: dict[str, Any]) -> None:
    report["comparisons"][name] = result
    report["checks"].extend(result.get("checks", []))
    report["warnings"].extend(result.get("warnings", []))
    report["differences"].extend(result.get("differences", []))


def _task_folder(parsed: dict[str, Any]) -> Path | None:
    folders = parsed.get("task_folders", [])
    return Path(folders[-1]).expanduser() if folders else None


def run_parity(args: argparse.Namespace) -> dict[str, Any]:
    audio = args.audio.expanduser().resolve()
    python_output = args.python_output.expanduser().resolve()
    go_output = args.go_output.expanduser().resolve()
    go_binary = args.go_binary.expanduser().resolve()
    env_file = args.env_file.expanduser().resolve() if args.env_file else None
    report_dir = args.report_dir.expanduser().resolve()

    env = os.environ.copy()
    source_path = str(REPO_ROOT / "src")
    env["PYTHONPATH"] = source_path + (os.pathsep + env["PYTHONPATH"] if env.get("PYTHONPATH") else "")
    python_command = build_python_command(audio, args.mode, python_output)
    go_command = build_go_command(audio, args.mode, go_output, go_binary, env_file)

    report: dict[str, Any] = {
        "phase": "phase_3_parity",
        "mode": args.mode,
        "created_at": datetime.now(timezone.utc).isoformat(),
        "commands": {"python": python_command, "go": go_command},
        "backends": {},
        "task_folders": {},
        "checks": [],
        "warnings": [],
        "differences": [],
        "comparisons": {},
    }

    for backend, command in (("python", python_command), ("go", go_command)):
        execution = _run(command, env)
        parsed = parse_backend_stdout(execution["stdout"])
        # Keep reports useful but do not persist raw stdout/stderr. They can
        # contain user content or service diagnostics and are not needed to
        # assess the declared stdout contract.
        report["backends"][backend] = {"exit_code": execution["exit_code"], "parsed_stdout": parsed}
        if execution["exit_code"] == 0:
            _add_comparison(report, f"{backend}.stdout", validate_stdout_contract(parsed, args.mode))
        else:
            report["differences"].append(f"{backend} backend exited with code {execution['exit_code']}")
        task_folder = _task_folder(parsed)
        if task_folder is not None:
            report["task_folders"][backend] = str(task_folder)

    if all(report["backends"][name]["exit_code"] == 0 for name in ("python", "go")) and len(report["task_folders"]) == 2:
        python_task = Path(report["task_folders"]["python"])
        go_task = Path(report["task_folders"]["go"])
        _add_comparison(report, "task_layout", compare_task_layout(args.mode, python_task, go_task))
        _add_comparison(report, "transcript", compare_transcript_structure(python_task / "transcript.md", go_task / "transcript.md"))
        if args.mode in {"notes", "minutes"}:
            _add_comparison(
                report,
                "notes_input",
                compare_notes_input_structure(python_task / "notes_input.md", go_task / "notes_input.md"),
            )
        if args.mode == "minutes":
            _add_comparison(report, "python.minutes", validate_minutes_file(python_task / "meeting_minutes.md"))
            _add_comparison(report, "go.minutes", validate_minutes_file(go_task / "meeting_minutes.md"))
    else:
        report["differences"].append("output comparison was skipped because one backend failed or did not print Task folder")

    report_dir.mkdir(parents=True, exist_ok=True)
    report_path = report_dir / f"parity-{args.mode}-{datetime.now(timezone.utc).strftime('%Y%m%dT%H%M%SZ')}.json"
    report["report_path"] = str(report_path)
    report["passed"] = not report["differences"]
    report_path.write_text(generate_report(report), encoding="utf-8")
    return report


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description="Run an optional live Python/Go parity comparison.")
    parser.add_argument("--audio", type=Path, required=True, help="Real input audio; never used in automated tests.")
    parser.add_argument("--mode", choices=sorted(VALID_MODES), required=True)
    parser.add_argument("--python-output", type=Path, required=True)
    parser.add_argument("--go-output", type=Path, required=True)
    parser.add_argument("--go-binary", type=Path, required=True)
    parser.add_argument("--env-file", type=Path, help="Go-only explicit .env path; Python uses repo-root .env.")
    parser.add_argument("--report-dir", type=Path, default=Path("output/parity/reports"))
    return parser


def main(argv: list[str] | None = None) -> int:
    args = build_parser().parse_args(argv)
    report = run_parity(args)
    print(f"Parity report: {report['report_path']}")
    return 0 if report["passed"] else 1


if __name__ == "__main__":
    raise SystemExit(main())

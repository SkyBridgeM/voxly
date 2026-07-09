from __future__ import annotations

import argparse
import sys
from pathlib import Path

from .config import load_settings
from .recorder import record_chunks
from .utils import create_task_dir
from .workflow import TranscribeOptions, run_audio_file_workflow, run_recorded_chunks_workflow, run_resume_workflow


ROOT = Path(__file__).resolve().parents[2]


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        prog="meeting-asr",
        description="Record or import meeting audio, transcribe with MiMo ASR, and generate meeting minutes.",
    )
    subparsers = parser.add_subparsers(dest="command", required=True)

    transcribe = subparsers.add_parser("transcribe", help="Process an existing mp3/wav/m4a file.")
    transcribe.add_argument("audio_file", type=Path)
    add_common_flags(transcribe)
    add_output_mode_flags(transcribe)

    record = subparsers.add_parser("record", help="Record microphone audio, then transcribe it.")
    record.add_argument("--duration", type=int, help="Recording duration in minutes. Omit to record until Ctrl+C.")
    record.add_argument("--input-device", default=":0", help="ffmpeg avfoundation input device. Default: :0.")
    add_common_flags(record)
    add_output_mode_flags(record)

    resume = subparsers.add_parser("resume", help="Resume transcription in an existing task folder.")
    resume.add_argument("task_dir", type=Path)
    add_notes_flags(resume)
    add_output_mode_flags(resume)

    return parser


def add_common_flags(parser: argparse.ArgumentParser) -> None:
    parser.add_argument("--chunk-minutes", type=int, default=None, help="Chunk length in minutes. Default from .env or 5.")
    parser.add_argument("--output-dir", type=Path, default=Path("output"), help="Base output directory. Default: output/.")
    add_notes_flags(parser)


def add_notes_flags(parser: argparse.ArgumentParser) -> None:
    parser.add_argument("--summarize", action="store_true", help="[Deprecated] Use --output-mode notes instead.")
    parser.add_argument("--prompt", type=Path, default=None, help="Prompt file for notes input.")

def add_output_mode_flags(parser: argparse.ArgumentParser) -> None:
    parser.add_argument(
        "--output-mode",
        choices=["transcript", "notes", "minutes"],
        default=None,
        help=(
            "Output mode: transcript (transcript.md only), "
            "notes (transcript.md + notes_input.md), "
            "minutes (all files + LLM meeting minutes)."
        ),
    )
    parser.add_argument("--llm-provider", choices=["deepseek"], default=None, help="LLM provider for minutes generation.")
    parser.add_argument("--llm-model", choices=["deepseek-v4-flash", "deepseek-v4-pro"], default=None, help="LLM model for minutes generation.")
    parser.add_argument("--obsidian-template", type=Path, default=None, help="Obsidian template for wrapping meeting_minutes.md.")


def _resolve_output_mode(args: argparse.Namespace) -> str:
    if args.output_mode:
        return args.output_mode
    if args.summarize:
        return "notes"
    return "transcript"


def run_transcribe(args: argparse.Namespace) -> Path:
    settings = load_settings(ROOT / ".env")
    output_mode = _resolve_output_mode(args)
    options = TranscribeOptions(
        output_dir=args.output_dir,
        chunk_minutes=args.chunk_minutes or settings.default_chunk_minutes,
        max_upload_mb=settings.max_upload_mb,
        output_mode=output_mode,
        prompt_path=args.prompt or settings.minutes_prompt_path,
        llm_provider=args.llm_provider or settings.minutes_provider,
        llm_model=args.llm_model or settings.deepseek_model,
        obsidian_template_path=args.obsidian_template or settings.obsidian_template_path,
    )

    def status_cb(msg: str) -> None:
        print(f"Status: {msg}")

    def chunk_cb(action: str, index: int, total: int, _path: Path) -> None:
        if action in ("transcribing", "completed"):
            print(f"Progress: {index}/{total}")

    result = run_audio_file_workflow(
        args.audio_file,
        settings,
        options,
        status_callback=status_cb,
        chunk_callback=chunk_cb,
    )
    print(f"Transcript saved: {result.transcript_path}")
    if result.notes_input_path:
        print(f"Notes input saved: {result.notes_input_path}")
    if result.minutes_path:
        print(f"Meeting minutes saved: {result.minutes_path}")
    return result.task_dir


def run_record(args: argparse.Namespace) -> Path:
    settings = load_settings(ROOT / ".env")
    chunk_minutes = args.chunk_minutes or settings.default_chunk_minutes
    task_dir = create_task_dir(args.output_dir, "recording")
    output_mode = _resolve_output_mode(args)
    options = TranscribeOptions(
        output_dir=args.output_dir,
        chunk_minutes=chunk_minutes,
        max_upload_mb=settings.max_upload_mb,
        output_mode=output_mode,
        prompt_path=args.prompt or settings.minutes_prompt_path,
        llm_provider=args.llm_provider or settings.minutes_provider,
        llm_model=args.llm_model or settings.deepseek_model,
        obsidian_template_path=args.obsidian_template or settings.obsidian_template_path,
    )

    def status_cb(msg: str) -> None:
        print(f"Status: {msg}")

    def chunk_cb(action: str, index: int, total: int, _path: Path) -> None:
        if action in ("transcribing", "completed"):
            print(f"Progress: {index}/{total}")

    print("Recording... Press Ctrl+C to stop when no duration is set.")
    chunks = record_chunks(
        task_dir / "audio",
        chunk_minutes=chunk_minutes,
        duration_minutes=args.duration,
        input_device=args.input_device,
    )
    result = run_recorded_chunks_workflow(
        chunks,
        task_dir,
        settings,
        options,
        status_callback=status_cb,
        chunk_callback=chunk_cb,
    )
    print(f"Transcript saved: {result.transcript_path}")
    if result.notes_input_path:
        print(f"Notes input saved: {result.notes_input_path}")
    if result.minutes_path:
        print(f"Meeting minutes saved: {result.minutes_path}")
    return result.task_dir


def run_resume(args: argparse.Namespace) -> Path:
    settings = load_settings(ROOT / ".env")
    output_mode = _resolve_output_mode(args)
    options = TranscribeOptions(
        output_dir=args.task_dir.parent,
        chunk_minutes=settings.default_chunk_minutes,
        max_upload_mb=settings.max_upload_mb,
        output_mode=output_mode,
        prompt_path=args.prompt or settings.minutes_prompt_path,
        llm_provider=args.llm_provider or settings.minutes_provider,
        llm_model=args.llm_model or settings.deepseek_model,
        obsidian_template_path=args.obsidian_template or settings.obsidian_template_path,
    )

    def status_cb(msg: str) -> None:
        print(f"Status: {msg}")

    def chunk_cb(action: str, index: int, total: int, _path: Path) -> None:
        if action in ("transcribing", "completed"):
            print(f"Progress: {index}/{total}")

    result = run_resume_workflow(
        args.task_dir,
        settings,
        options,
        status_callback=status_cb,
        chunk_callback=chunk_cb,
    )
    print(f"Transcript saved: {result.transcript_path}")
    if result.notes_input_path:
        print(f"Notes input saved: {result.notes_input_path}")
    if result.minutes_path:
        print(f"Meeting minutes saved: {result.minutes_path}")
    return result.task_dir


def main(argv: list[str] | None = None) -> int:
    parser = build_parser()
    args = parser.parse_args(argv)

    try:
        if args.command == "transcribe":
            task_dir = run_transcribe(args)
        elif args.command == "record":
            task_dir = run_record(args)
        elif args.command == "resume":
            task_dir = run_resume(args)
        else:
            parser.error(f"Unknown command: {args.command}")
            return 2
    except Exception as exc:
        print(f"Error: {exc}", file=sys.stderr)
        return 1

    print(f"Task folder: {task_dir}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

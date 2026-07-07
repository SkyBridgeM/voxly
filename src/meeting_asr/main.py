from __future__ import annotations

import argparse
import sys
from pathlib import Path

from .config import load_settings
from .recorder import record_chunks
from .utils import create_task_dir
from .workflow import TranscribeOptions, run_audio_file_workflow, run_recorded_chunks_workflow


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        prog="meeting-asr",
        description="Record or import meeting audio, transcribe with MiMo ASR, and prepare optional notes input.",
    )
    subparsers = parser.add_subparsers(dest="command", required=True)

    transcribe = subparsers.add_parser("transcribe", help="Process an existing mp3/wav file.")
    transcribe.add_argument("audio_file", type=Path)
    add_common_flags(transcribe)

    record = subparsers.add_parser("record", help="Record microphone audio, then transcribe it.")
    record.add_argument("--duration", type=int, help="Recording duration in minutes. Omit to record until Ctrl+C.")
    record.add_argument("--input-device", default=":0", help="ffmpeg avfoundation input device. Default: :0.")
    add_common_flags(record)

    return parser


def add_common_flags(parser: argparse.ArgumentParser) -> None:
    parser.add_argument("--chunk-minutes", type=int, default=None, help="Chunk length in minutes. Default from .env or 5.")
    parser.add_argument("--output-dir", type=Path, default=Path("output"), help="Base output directory. Default: output/.")
    parser.add_argument("--summarize", action="store_true", help="Create notes_input.md from prompt and transcript.")
    parser.add_argument("--prompt", type=Path, default=Path("prompts/default_minutes.md"), help="Prompt file for notes input.")


def run_transcribe(args: argparse.Namespace) -> Path:
    settings = load_settings()
    options = TranscribeOptions(
        output_dir=args.output_dir,
        chunk_minutes=args.chunk_minutes or settings.default_chunk_minutes,
        max_upload_mb=settings.max_upload_mb,
        summarize=args.summarize,
        prompt_path=args.prompt,
    )
    result = run_audio_file_workflow(args.audio_file, settings, options)
    print(f"Transcript saved: {result.transcript_path}")
    if result.notes_input_path:
        print(f"Notes input saved: {result.notes_input_path}")
    return result.task_dir


def run_record(args: argparse.Namespace) -> Path:
    settings = load_settings()
    chunk_minutes = args.chunk_minutes or settings.default_chunk_minutes
    task_dir = create_task_dir(args.output_dir, "recording")
    options = TranscribeOptions(
        output_dir=args.output_dir,
        chunk_minutes=chunk_minutes,
        max_upload_mb=settings.max_upload_mb,
        summarize=args.summarize,
        prompt_path=args.prompt,
    )

    print("Recording... Press Ctrl+C to stop when no duration is set.")
    chunks = record_chunks(
        task_dir / "audio",
        chunk_minutes=chunk_minutes,
        duration_minutes=args.duration,
        input_device=args.input_device,
    )
    result = run_recorded_chunks_workflow(chunks, task_dir, settings, options)
    print(f"Transcript saved: {result.transcript_path}")
    if result.notes_input_path:
        print(f"Notes input saved: {result.notes_input_path}")
    return result.task_dir


def main(argv: list[str] | None = None) -> int:
    parser = build_parser()
    args = parser.parse_args(argv)

    try:
        if args.command == "transcribe":
            task_dir = run_transcribe(args)
        elif args.command == "record":
            task_dir = run_record(args)
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

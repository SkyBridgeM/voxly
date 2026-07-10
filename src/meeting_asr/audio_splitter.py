from __future__ import annotations

import shutil
from pathlib import Path

from .utils import MIN_CHUNK_SECONDS, ensure_ffmpeg, file_size_mb, run_command, supported_audio_format


def calculate_retry_chunk_seconds(initial_seconds: int, min_seconds: int = MIN_CHUNK_SECONDS) -> list[int]:
    if initial_seconds < min_seconds:
        return [initial_seconds]
    values = [initial_seconds]
    current = initial_seconds
    while current > min_seconds:
        current = max(min_seconds, current // 2)
        if current != values[-1]:
            values.append(current)
    return values


def split_audio(input_path: Path, chunks_dir: Path, chunk_minutes: int, max_upload_bytes: int) -> list[Path]:
    ensure_ffmpeg()
    input_format = supported_audio_format(input_path)
    chunk_format = chunk_audio_format(input_format)
    if not input_path.exists():
        raise FileNotFoundError(f"Audio file not found: {input_path}")

    initial_seconds = max(MIN_CHUNK_SECONDS, int(chunk_minutes * 60))
    attempted: list[int] = []

    for chunk_seconds in calculate_retry_chunk_seconds(initial_seconds):
        attempted.append(chunk_seconds)
        _clear_chunks(chunks_dir)
        _split_once(input_path, chunks_dir, input_format, chunk_seconds)
        chunks = sorted(chunks_dir.glob(f"part_*.{chunk_format}"))
        if not chunks:
            raise RuntimeError("ffmpeg did not create any audio chunks.")

        oversize = [chunk for chunk in chunks if chunk.stat().st_size > max_upload_bytes]
        if not oversize:
            return chunks

        if chunk_seconds <= MIN_CHUNK_SECONDS:
            details = ", ".join(f"{chunk.name}={file_size_mb(chunk):.2f}MB" for chunk in oversize[:5])
            raise RuntimeError(
                "One or more chunks exceed the upload limit even at the 60-second minimum: "
                f"{details}. Increase MAX_UPLOAD_MB or provide smaller/lower-bitrate audio."
            )

    raise RuntimeError(f"Unable to split audio below upload limit. Attempted chunk lengths: {attempted}")


def chunk_audio_format(input_format: str) -> str:
    return "wav" if input_format == "m4a" else input_format


def _clear_chunks(chunks_dir: Path) -> None:
    chunks_dir.mkdir(parents=True, exist_ok=True, mode=0o700)
    for path in chunks_dir.glob("part_*.*"):
        if path.is_file():
            path.unlink()


def _split_once(input_path: Path, chunks_dir: Path, audio_format: str, chunk_seconds: int) -> None:
    output_format = chunk_audio_format(audio_format)
    output_pattern = chunks_dir / f"part_%03d.{output_format}"
    if audio_format == "m4a":
        _split_m4a_to_wav(input_path, output_pattern, chunk_seconds)
        _renumber_chunks(chunks_dir, output_format)
        return

    command = [
        "ffmpeg",
        "-hide_banner",
        "-loglevel",
        "error",
        "-y",
        "-i",
        str(input_path),
        "-f",
        "segment",
        "-segment_time",
        str(chunk_seconds),
        "-reset_timestamps",
        "1",
        "-c",
        "copy",
        str(output_pattern),
    ]

    try:
        run_command(command, capture=True)
    except Exception:
        # Some inputs cannot be stream-copied cleanly into segments. Re-encode as a reliable fallback.
        fallback = [
            "ffmpeg",
            "-hide_banner",
            "-loglevel",
            "error",
            "-y",
            "-i",
            str(input_path),
            "-f",
            "segment",
            "-segment_time",
            str(chunk_seconds),
            "-reset_timestamps",
            "1",
            str(output_pattern),
        ]
        run_command(fallback, capture=True)

    _renumber_chunks(chunks_dir, output_format)


def _split_m4a_to_wav(input_path: Path, output_pattern: Path, chunk_seconds: int) -> None:
    command = [
        "ffmpeg",
        "-hide_banner",
        "-loglevel",
        "error",
        "-y",
        "-i",
        str(input_path),
        "-f",
        "segment",
        "-segment_time",
        str(chunk_seconds),
        "-reset_timestamps",
        "1",
        "-ac",
        "1",
        "-ar",
        "16000",
        "-c:a",
        "pcm_s16le",
        str(output_pattern),
    ]
    run_command(command, capture=True)


def _renumber_chunks(chunks_dir: Path, audio_format: str) -> None:
    # ffmpeg segment numbering starts at 0; rename through temporary names to avoid collisions.
    original_paths = sorted(chunks_dir.glob(f"part_*.{audio_format}"))
    temp_paths: list[Path] = []
    for index, path in enumerate(original_paths, start=1):
        temp = chunks_dir / f".renaming_{index:03d}.{audio_format}"
        shutil.move(str(path), str(temp))
        temp_paths.append(temp)

    for index, path in enumerate(temp_paths, start=1):
        target = chunks_dir / f"part_{index:03d}.{audio_format}"
        shutil.move(str(path), str(target))

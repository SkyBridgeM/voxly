from __future__ import annotations

import signal
import subprocess
from pathlib import Path

from .utils import ensure_ffmpeg


def record_chunks(
    audio_dir: Path,
    *,
    chunk_minutes: int,
    duration_minutes: int | None,
    input_device: str,
) -> list[Path]:
    ensure_ffmpeg()
    audio_dir.mkdir(parents=True, exist_ok=True)
    output_pattern = audio_dir / "part_%03d.wav"
    chunk_seconds = max(60, int(chunk_minutes * 60))

    command = [
        "ffmpeg",
        "-hide_banner",
        "-loglevel",
        "error",
        "-f",
        "avfoundation",
        "-i",
        input_device,
    ]
    if duration_minutes is not None:
        command.extend(["-t", str(int(duration_minutes * 60))])
    command.extend(
        [
            "-ac",
            "1",
            "-ar",
            "16000",
            "-f",
            "segment",
            "-segment_time",
            str(chunk_seconds),
            "-reset_timestamps",
            "1",
            str(output_pattern),
        ]
    )

    process = subprocess.Popen(command)
    try:
        process.wait()
    except KeyboardInterrupt:
        process.send_signal(signal.SIGINT)
        try:
            process.wait(timeout=10)
        except subprocess.TimeoutExpired:
            process.terminate()
            process.wait(timeout=10)

    if process.returncode not in (0, 255, -2):
        raise RuntimeError(f"ffmpeg recording failed with exit code {process.returncode}.")

    chunks = sorted(audio_dir.glob("part_*.wav"))
    return _normalize_recorded_names(chunks)


def _normalize_recorded_names(chunks: list[Path]) -> list[Path]:
    normalized: list[Path] = []
    temp_paths: list[Path] = []
    for index, path in enumerate(chunks, start=1):
        temp = path.with_name(f".renaming_{index:03d}.wav")
        path.rename(temp)
        temp_paths.append(temp)

    for index, path in enumerate(temp_paths, start=1):
        target = path.with_name(f"part_{index:03d}.wav")
        path.rename(target)
        normalized.append(target)
    return normalized

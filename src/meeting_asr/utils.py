from __future__ import annotations

import re
import shutil
import subprocess
from datetime import datetime
from pathlib import Path


SUPPORTED_AUDIO_SUFFIXES = {".m4a", ".mp3", ".wav"}
MIN_CHUNK_SECONDS = 60


class CommandError(RuntimeError):
    pass


def ensure_ffmpeg() -> None:
    if shutil.which("ffmpeg") is not None:
        return

    raise RuntimeError(build_ffmpeg_missing_message(shutil.which("brew") is not None))


def build_ffmpeg_missing_message(has_homebrew: bool) -> str:
    if has_homebrew:
        return (
            "ffmpeg was not found. Install it with Homebrew:\n\n"
            "  brew install ffmpeg\n\n"
            "Then verify it with:\n\n"
            "  ffmpeg -version"
        )

    return (
        "ffmpeg was not found, and Homebrew was not found either.\n\n"
        "First install Homebrew from https://brew.sh/ or run the official installer shown there.\n"
        "After Homebrew is installed, install ffmpeg:\n\n"
        "  brew install ffmpeg\n\n"
        "Then verify it with:\n\n"
        "  ffmpeg -version"
    )


def run_command(command: list[str], *, capture: bool = False) -> subprocess.CompletedProcess[str]:
    try:
        return subprocess.run(
            command,
            check=True,
            text=True,
            capture_output=capture,
        )
    except subprocess.CalledProcessError as exc:
        stderr = exc.stderr.strip() if exc.stderr else ""
        detail = f": {stderr}" if stderr else ""
        raise CommandError(f"Command failed{detail}") from exc


def supported_audio_format(path: Path) -> str:
    suffix = path.suffix.lower()
    if suffix not in SUPPORTED_AUDIO_SUFFIXES:
        supported = ", ".join(sorted(SUPPORTED_AUDIO_SUFFIXES))
        raise ValueError(f"Unsupported audio format {suffix!r}. Supported formats: {supported}.")
    return suffix.lstrip(".")


def safe_slug(value: str) -> str:
    normalized = str(value).replace("/", "_").replace("\\", "_")
    stem = re.sub(r"\.[A-Za-z0-9]{1,8}$", "", normalized)
    slug = re.sub(r"[^A-Za-z0-9._-]+", "_", stem).strip("._-")
    return slug or "meeting"


def timestamp() -> str:
    return datetime.now().strftime("%Y%m%d_%H%M%S")


def create_task_dir(output_dir: Path, name: str) -> Path:
    resolved_output_dir = output_dir.expanduser().resolve()
    resolved_output_dir.mkdir(parents=True, exist_ok=True)
    base_name = f"{timestamp()}_{safe_slug(name)}"

    for suffix in range(1, 1001):
        dir_name = base_name if suffix == 1 else f"{base_name}_{suffix}"
        task_dir = resolved_output_dir / dir_name
        try:
            task_dir.mkdir(mode=0o700)
            break
        except FileExistsError:
            continue
    else:
        raise RuntimeError(f"Could not allocate a unique task folder under {resolved_output_dir}.")

    for subdir in ("audio", "chunks", "transcripts"):
        (task_dir / subdir).mkdir(mode=0o700)
    return task_dir


def transcript_part_path(transcripts_dir: Path, chunk_path: Path) -> Path:
    return transcripts_dir / f"{chunk_path.stem}.txt"


def is_complete_file(path: Path) -> bool:
    return path.exists() and path.is_file() and path.stat().st_size > 0


def list_ordered_parts(directory: Path, suffix: str) -> list[Path]:
    return sorted(directory.glob(f"part_*.{suffix}"))


def file_size_mb(path: Path) -> float:
    return path.stat().st_size / (1024 * 1024)


def next_smaller_chunk_seconds(current_seconds: int) -> int:
    return max(MIN_CHUNK_SECONDS, current_seconds // 2)


def validate_chunks_under_limit(chunks: list[Path], max_upload_bytes: int) -> None:
    oversize = [chunk for chunk in chunks if chunk.stat().st_size > max_upload_bytes]
    if not oversize:
        return

    details = ", ".join(f"{chunk.name}={file_size_mb(chunk):.2f}MB" for chunk in oversize[:5])
    max_mb = max_upload_bytes / (1024 * 1024)
    raise RuntimeError(
        f"One or more chunks exceed the upload limit ({max_mb:.2f}MB): {details}. "
        "Use a smaller --chunk-minutes value or increase MAX_UPLOAD_MB."
    )

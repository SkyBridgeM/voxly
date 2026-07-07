from __future__ import annotations

import re
import shutil
from dataclasses import dataclass, replace
from pathlib import Path
from typing import Callable

from .audio_splitter import split_audio
from .config import Settings
from .summarize import create_notes_input
from .transcribe_mimo import transcribe_chunks
from .transcript_merger import merge_transcripts
from .utils import create_task_dir, safe_slug, supported_audio_format, validate_chunks_under_limit


StatusCallback = Callable[[str], None]
ChunkCallback = Callable[[str, int, int, Path], None]
PART_CHUNK_RE = re.compile(r"^part_(\d{3})\.(mp3|wav)$")


@dataclass(frozen=True)
class TranscribeOptions:
    output_dir: Path
    chunk_minutes: int
    max_upload_mb: float
    summarize: bool = False
    prompt_path: Path = Path("prompts/default_minutes.md")

    def __post_init__(self) -> None:
        if self.chunk_minutes <= 0:
            raise ValueError(f"Chunk length must be greater than 0, got {self.chunk_minutes!r}.")
        if self.max_upload_mb <= 0:
            raise ValueError(f"Max upload size must be greater than 0, got {self.max_upload_mb!r}.")


@dataclass(frozen=True)
class TranscribeResult:
    task_dir: Path
    transcript_path: Path
    notes_input_path: Path | None = None
    minutes_path: Path | None = None


def run_audio_file_workflow(
    audio_path: Path,
    settings: Settings,
    options: TranscribeOptions,
    *,
    status_callback: StatusCallback | None = None,
    chunk_callback: ChunkCallback | None = None,
) -> TranscribeResult:
    input_path = audio_path.expanduser().resolve()
    supported_audio_format(input_path)
    if not input_path.exists():
        raise FileNotFoundError(f"Audio file not found: {input_path}")

    effective_settings = replace(settings, max_upload_mb=options.max_upload_mb)
    task_dir = create_task_dir(options.output_dir, input_path.stem)
    source_copy = task_dir / "audio" / input_path.name
    shutil.copy2(input_path, source_copy)

    return run_task_audio_workflow(
        source_copy,
        task_dir,
        effective_settings,
        options,
        status_callback=status_callback,
        chunk_callback=chunk_callback,
    )


def run_uploaded_audio_workflow(
    filename: str,
    data: bytes,
    settings: Settings,
    options: TranscribeOptions,
    *,
    status_callback: StatusCallback | None = None,
    chunk_callback: ChunkCallback | None = None,
) -> TranscribeResult:
    safe_name = safe_upload_filename(filename)
    supported_audio_format(Path(safe_name))
    effective_settings = replace(settings, max_upload_mb=options.max_upload_mb)
    task_dir = create_task_dir(options.output_dir, Path(safe_name).stem)
    source_audio = task_dir / "audio" / safe_name
    source_audio.write_bytes(data)

    return run_task_audio_workflow(
        source_audio,
        task_dir,
        effective_settings,
        options,
        status_callback=status_callback,
        chunk_callback=chunk_callback,
    )


def safe_upload_filename(filename: str) -> str:
    raw_name = Path(str(filename).replace("\\", "/")).name
    suffix = Path(raw_name).suffix.lower()
    supported_audio_format(Path(f"upload{suffix}"))
    stem = safe_slug(Path(raw_name).stem)
    return f"{stem}{suffix}"


def run_recorded_chunks_workflow(
    chunks: list[Path],
    task_dir: Path,
    settings: Settings,
    options: TranscribeOptions,
    *,
    status_callback: StatusCallback | None = None,
    chunk_callback: ChunkCallback | None = None,
) -> TranscribeResult:
    if not chunks:
        raise RuntimeError("No recording chunks were created.")

    effective_settings = replace(settings, max_upload_mb=options.max_upload_mb)
    validate_chunks_under_limit(chunks, effective_settings.max_upload_bytes)
    return run_transcription_workflow(
        chunks,
        task_dir,
        effective_settings,
        options,
        status_callback=status_callback,
        chunk_callback=chunk_callback,
    )


def run_resume_workflow(
    task_dir: Path,
    settings: Settings,
    options: TranscribeOptions,
    *,
    status_callback: StatusCallback | None = None,
    chunk_callback: ChunkCallback | None = None,
) -> TranscribeResult:
    resolved_task_dir = task_dir.expanduser().resolve()
    if not resolved_task_dir.is_dir():
        raise FileNotFoundError(f"Task folder not found: {resolved_task_dir}")

    chunks = find_resume_chunks(resolved_task_dir)
    effective_settings = replace(settings, max_upload_mb=options.max_upload_mb)
    validate_chunks_under_limit(chunks, effective_settings.max_upload_bytes)
    return run_transcription_workflow(
        chunks,
        resolved_task_dir,
        effective_settings,
        options,
        status_callback=status_callback,
        chunk_callback=chunk_callback,
    )


def find_resume_chunks(task_dir: Path) -> list[Path]:
    resolved_task_dir = task_dir.expanduser().resolve()
    if not resolved_task_dir.is_dir():
        raise FileNotFoundError(f"Task folder not found: {resolved_task_dir}")

    chunks_dir = resolved_task_dir / "chunks"
    if chunks_dir.exists():
        chunks = _find_ordered_part_chunks(chunks_dir, {".mp3", ".wav"}, allow_mixed_formats=False)
        if chunks:
            return chunks

    audio_chunks = _find_ordered_part_chunks(resolved_task_dir / "audio", {".wav"}, allow_mixed_formats=True)
    if audio_chunks:
        return audio_chunks

    raise RuntimeError(f"No resume chunks found in {resolved_task_dir / 'chunks'} or {resolved_task_dir / 'audio'}.")


def _find_ordered_part_chunks(directory: Path, suffixes: set[str], *, allow_mixed_formats: bool) -> list[Path]:
    if not directory.exists():
        return []

    candidates: list[Path] = []
    for suffix in sorted(suffixes):
        candidates.extend(directory.glob(f"part_*{suffix}"))
    candidates = sorted(path for path in candidates if path.is_file())
    if not candidates:
        return []

    parsed: list[tuple[int, Path]] = []
    formats: set[str] = set()
    invalid_names: list[str] = []
    for path in candidates:
        match = PART_CHUNK_RE.match(path.name)
        if not match:
            invalid_names.append(path.name)
            continue
        number = int(match.group(1))
        formats.add(match.group(2))
        parsed.append((number, path))

    if invalid_names:
        names = ", ".join(invalid_names[:5])
        raise RuntimeError(f"Invalid resume chunk filename(s) in {directory}: {names}. Expected part_001.wav or part_001.mp3.")

    if not allow_mixed_formats and len(formats) > 1:
        raise RuntimeError(f"Mixed resume chunk formats in {directory}. Keep only mp3 or wav chunks.")

    parsed.sort(key=lambda item: item[0])
    _validate_contiguous_part_numbers([number for number, _ in parsed], directory)
    return [path for _, path in parsed]


def _validate_contiguous_part_numbers(numbers: list[int], directory: Path) -> None:
    expected = list(range(1, len(numbers) + 1))
    if numbers == expected:
        return

    first = numbers[0] if numbers else None
    if first != 1:
        raise RuntimeError(f"Resume chunks in {directory} must start at part_001; first part is part_{first:03d}.")

    missing = [number for number in expected if number not in numbers]
    if missing:
        missing_names = ", ".join(f"part_{number:03d}" for number in missing[:5])
        raise RuntimeError(f"Missing resume chunk(s) in {directory}: {missing_names}.")

    raise RuntimeError(f"Resume chunk numbering is invalid in {directory}.")


def run_task_audio_workflow(
    source_audio: Path,
    task_dir: Path,
    settings: Settings,
    options: TranscribeOptions,
    *,
    status_callback: StatusCallback | None = None,
    chunk_callback: ChunkCallback | None = None,
) -> TranscribeResult:
    if status_callback:
        status_callback("Splitting audio")
    chunks = split_audio(source_audio, task_dir / "chunks", options.chunk_minutes, settings.max_upload_bytes)

    return run_transcription_workflow(
        chunks,
        task_dir,
        settings,
        options,
        status_callback=status_callback,
        chunk_callback=chunk_callback,
    )


def run_transcription_workflow(
    chunks: list[Path],
    task_dir: Path,
    settings: Settings,
    options: TranscribeOptions,
    *,
    status_callback: StatusCallback | None = None,
    chunk_callback: ChunkCallback | None = None,
) -> TranscribeResult:
    if status_callback:
        status_callback(f"Transcribing {len(chunks)} chunk(s)")
    transcripts_dir = task_dir / "transcripts"
    transcribe_chunks(chunks, transcripts_dir, settings, progress_callback=chunk_callback)

    if status_callback:
        status_callback("Merging transcript")
    transcript_path = merge_transcripts(transcripts_dir, task_dir / "transcript.md")

    notes_input_path: Path | None = None
    if options.summarize:
        if status_callback:
            status_callback("Preparing notes input")
        notes_input_path = create_notes_input(
            options.prompt_path.expanduser().resolve(),
            transcript_path,
            task_dir / "notes_input.md",
        )

    minutes_path = task_dir / "meeting_minutes.md"
    if not minutes_path.exists():
        minutes_path = None

    if status_callback:
        status_callback("Done")
    return TranscribeResult(
        task_dir=task_dir,
        transcript_path=transcript_path,
        notes_input_path=notes_input_path,
        minutes_path=minutes_path,
    )

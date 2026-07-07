from __future__ import annotations

from io import BytesIO
from pathlib import Path
from zipfile import ZIP_DEFLATED, ZipFile


PREVIEW_LIMIT = 12000


def discover_prompt_templates(prompts_dir: Path = Path("prompts")) -> list[Path]:
    if not prompts_dir.exists():
        return []
    return sorted(path for path in prompts_dir.glob("*.md") if path.is_file())


def resolve_prompt_path(selected_prompt: str | None, custom_prompt: str | None) -> Path:
    custom = (custom_prompt or "").strip()
    if custom:
        return Path(custom).expanduser()
    if selected_prompt:
        return Path(selected_prompt)
    return Path("prompts/default_minutes.md")


def read_preview(path: Path, limit: int = PREVIEW_LIMIT) -> tuple[str, bool]:
    text = path.read_text(encoding="utf-8")
    if len(text) <= limit:
        return text, False
    return text[:limit].rstrip() + "\n\n[Preview truncated. Download the full file below.]\n", True


def build_text_outputs_zip(task_dir: Path) -> bytes:
    buffer = BytesIO()
    with ZipFile(buffer, "w", ZIP_DEFLATED) as archive:
        for filename in ("transcript.md", "notes_input.md", "meeting_minutes.md"):
            path = task_dir / filename
            if path.exists() and path.is_file():
                archive.write(path, path.name)

        transcripts_dir = task_dir / "transcripts"
        if transcripts_dir.exists():
            for path in sorted(transcripts_dir.glob("*.txt")):
                if path.is_file():
                    archive.write(path, Path("transcripts") / path.name)

    return buffer.getvalue()


def format_detected(value: bool) -> str:
    return "Detected" if value else "Not detected"


def update_env_value(env_path: Path, key: str, value: str) -> None:
    new_line = f"{key}={value}"
    if not env_path.exists():
        env_path.write_text(new_line + "\n", encoding="utf-8")
        return

    lines = env_path.read_text(encoding="utf-8").splitlines(keepends=True)
    output: list[str] = []
    replaced = False

    for line in lines:
        if _is_env_key_line(line, key):
            if not replaced:
                newline = "\n" if line.endswith("\n") else ""
                output.append(new_line + newline)
                replaced = True
            continue
        output.append(line)

    if not replaced:
        if output and not output[-1].endswith("\n"):
            output[-1] += "\n"
        output.append(new_line + "\n")

    env_path.write_text("".join(output), encoding="utf-8")


def _is_env_key_line(line: str, key: str) -> bool:
    stripped = line.lstrip()
    return not stripped.startswith("#") and stripped.startswith(f"{key}=")


def classify_error(exc: Exception) -> tuple[str, str]:
    message = str(exc)
    lower = message.lower()

    if "mimo_api_key" in lower:
        return "MiMo API key is missing", message
    if "ffmpeg was not found" in lower:
        return "ffmpeg is not installed", message
    if isinstance(exc, ValueError) and "unsupported audio format" in lower:
        return "Unsupported audio format", message
    if isinstance(exc, FileNotFoundError) and "prompt file not found" in lower:
        return "Prompt file was not found", message
    if isinstance(exc, FileNotFoundError):
        return "File was not found", message
    if "mimo api error" in lower or "mimo server error" in lower or "failed to transcribe" in lower:
        return "MiMo ASR failed", message

    return "Run failed", message

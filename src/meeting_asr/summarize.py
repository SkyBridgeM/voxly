from __future__ import annotations

from pathlib import Path


def create_notes_input(prompt_path: Path, transcript_path: Path, output_path: Path) -> Path:
    if not prompt_path.exists():
        raise FileNotFoundError(f"Prompt file not found: {prompt_path}")
    if not transcript_path.exists():
        raise FileNotFoundError(f"Transcript file not found: {transcript_path}")

    prompt = prompt_path.read_text(encoding="utf-8").strip()
    transcript = transcript_path.read_text(encoding="utf-8").strip()
    content = (
        f"{prompt}\n\n"
        "---\n\n"
        "## Transcript\n\n"
        f"{transcript}\n"
    )
    output_path.write_text(content, encoding="utf-8")
    return output_path

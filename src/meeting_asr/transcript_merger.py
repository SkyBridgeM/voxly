from __future__ import annotations

import re
from pathlib import Path


PART_RE = re.compile(r"part_(\d+)\.txt$")


def merge_transcripts(transcripts_dir: Path, output_path: Path) -> Path:
    parts = sorted(transcripts_dir.glob("part_*.txt"))
    if not parts:
        raise RuntimeError(f"No transcript parts found in {transcripts_dir}.")

    sections: list[str] = []
    for path in parts:
        match = PART_RE.match(path.name)
        if not match:
            continue
        number = int(match.group(1))
        text = path.read_text(encoding="utf-8").strip()
        sections.append(f"[Part {number:03d}]\n\n{text}")

    if not sections:
        raise RuntimeError(f"No valid transcript part files found in {transcripts_dir}.")

    output_path.write_text("\n\n".join(sections).strip() + "\n", encoding="utf-8")
    return output_path

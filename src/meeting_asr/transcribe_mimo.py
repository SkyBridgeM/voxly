from __future__ import annotations

import base64
import os
import time
from pathlib import Path
from typing import Any, Callable

import requests

from .config import Settings
from .utils import is_complete_file, supported_audio_format, transcript_part_path


ProgressCallback = Callable[[str, int, int, Path], None]


def transcribe_chunks(
    chunks: list[Path],
    transcripts_dir: Path,
    settings: Settings,
    progress_callback: ProgressCallback | None = None,
) -> list[Path]:
    api_key = settings.require_mimo_api_key()
    transcripts_dir.mkdir(parents=True, exist_ok=True)
    outputs: list[Path] = []

    total = len(chunks)
    for index, chunk in enumerate(chunks, start=1):
        transcript_path = transcript_part_path(transcripts_dir, chunk)
        if is_complete_file(transcript_path):
            if progress_callback:
                progress_callback("skipped", index, total, chunk)
            print(f"Skipping existing transcript: {transcript_path.name}")
            outputs.append(transcript_path)
            continue

        if progress_callback:
            progress_callback("transcribing", index, total, chunk)
        print(f"Transcribing {chunk.name}...")
        text = transcribe_audio_file(chunk, settings, api_key)
        transcript_path.write_text(text.strip() + "\n", encoding="utf-8")
        transcript_path.chmod(0o600)
        if progress_callback:
            progress_callback("completed", index, total, chunk)
        outputs.append(transcript_path)

    return outputs


def transcribe_audio_file(audio_path: Path, settings: Settings, api_key: str | None = None) -> str:
    key = api_key or settings.require_mimo_api_key()
    audio_format = supported_audio_format(audio_path)
    audio_b64 = base64.b64encode(audio_path.read_bytes()).decode("ascii")
    payload = build_mimo_asr_payload(audio_b64, audio_format, settings.mimo_asr_model)
    headers = {
        "Authorization": f"Bearer {key}",
        "Content-Type": "application/json",
    }

    last_error: Exception | None = None
    for attempt in range(1, settings.retry_count + 1):
        try:
            response = requests.post(
                settings.mimo_asr_endpoint,
                headers=headers,
                json=payload,
                timeout=settings.timeout_seconds,
            )
            if response.status_code >= 500:
                raise RuntimeError(f"MiMo server error {response.status_code}: {response.text[:500]}")
            if response.status_code >= 400:
                raise RuntimeError(f"MiMo API error {response.status_code}: {response.text[:1000]}")

            return extract_transcript(response.json())
        except (requests.RequestException, RuntimeError, ValueError) as exc:
            last_error = exc
            if attempt >= settings.retry_count:
                break
            time.sleep(min(2**attempt, 10))

    raise RuntimeError(f"Failed to transcribe {audio_path.name} after {settings.retry_count} attempts: {last_error}")


def build_mimo_asr_payload(audio_b64: str, audio_format: str, model: str) -> dict[str, Any]:
    return {
        "model": model,
        "asr_options": {"language": "auto"},
        "messages": [
            {
                "role": "user",
                "content": [
                    {
                        "type": "input_audio",
                        "input_audio": {
                            "data": build_audio_data_url(audio_b64, audio_format),
                        },
                    },
                ],
            }
        ],
    }


def build_audio_data_url(audio_b64: str, audio_format: str) -> str:
    mime_types = {
        "wav": "audio/wav",
        "mp3": "audio/mpeg",
    }
    try:
        mime_type = mime_types[audio_format]
    except KeyError as exc:
        raise ValueError(f"Unsupported audio format {audio_format!r}. Supported formats: mp3, wav.") from exc
    return f"data:{mime_type};base64,{audio_b64}"


def extract_transcript(data: dict[str, Any]) -> str:
    try:
        content = data["choices"][0]["message"]["content"]
    except (KeyError, IndexError, TypeError) as exc:
        raise ValueError(f"Unexpected MiMo response shape: {data}") from exc

    if isinstance(content, str):
        return content
    if isinstance(content, list):
        pieces: list[str] = []
        for item in content:
            if isinstance(item, dict):
                text = item.get("text") or item.get("content")
                if isinstance(text, str):
                    pieces.append(text)
        if pieces:
            return "\n".join(pieces)

    raise ValueError(f"Could not extract transcript text from MiMo response: {data}")

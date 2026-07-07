from __future__ import annotations

import os
from dataclasses import dataclass
from pathlib import Path

from dotenv import load_dotenv


DEFAULT_ENDPOINT = "https://api.xiaomimimo.com/v1/chat/completions"
DEFAULT_ASR_MODEL = "mimo-v2.5-asr"


@dataclass(frozen=True)
class Settings:
    mimo_api_key: str | None
    mimo_asr_endpoint: str
    mimo_asr_model: str
    max_upload_mb: float
    default_chunk_minutes: int
    retry_count: int
    timeout_seconds: int

    def __post_init__(self) -> None:
        _require_positive("MAX_UPLOAD_MB", self.max_upload_mb)
        _require_positive("DEFAULT_CHUNK_MINUTES", self.default_chunk_minutes)
        _require_positive("MIMO_RETRY_COUNT", self.retry_count)
        _require_positive("MIMO_TIMEOUT_SECONDS", self.timeout_seconds)

    @property
    def max_upload_bytes(self) -> int:
        return int(self.max_upload_mb * 1024 * 1024)

    def require_mimo_api_key(self) -> str:
        if not self.mimo_api_key:
            raise RuntimeError(
                "MIMO_API_KEY is not configured. Copy .env.example to .env and set MIMO_API_KEY."
            )
        return self.mimo_api_key


def _require_positive(name: str, value: float | int) -> None:
    if value <= 0:
        raise ValueError(f"{name} must be greater than 0, got {value!r}.")


def _get_float(name: str, default: float) -> float:
    value = os.getenv(name)
    if value is None or value == "":
        return default
    try:
        return float(value)
    except ValueError as exc:
        raise ValueError(f"{name} must be a number, got {value!r}.") from exc


def _get_int(name: str, default: int) -> int:
    value = os.getenv(name)
    if value is None or value == "":
        return default
    try:
        return int(value)
    except ValueError as exc:
        raise ValueError(f"{name} must be an integer, got {value!r}.") from exc


def load_settings(env_path: Path | None = None) -> Settings:
    load_dotenv(dotenv_path=env_path)
    return Settings(
        mimo_api_key=os.getenv("MIMO_API_KEY") or None,
        mimo_asr_endpoint=os.getenv("MIMO_ASR_ENDPOINT", DEFAULT_ENDPOINT),
        mimo_asr_model=os.getenv("MIMO_ASR_MODEL", DEFAULT_ASR_MODEL),
        max_upload_mb=_get_float("MAX_UPLOAD_MB", 10),
        default_chunk_minutes=_get_int("DEFAULT_CHUNK_MINUTES", 5),
        retry_count=_get_int("MIMO_RETRY_COUNT", 3),
        timeout_seconds=_get_int("MIMO_TIMEOUT_SECONDS", 120),
    )

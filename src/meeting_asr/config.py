from __future__ import annotations

import os
from dataclasses import dataclass
from pathlib import Path

from dotenv import load_dotenv


DEFAULT_ENDPOINT = "https://api.xiaomimimo.com/v1/chat/completions"
DEFAULT_ASR_MODEL = "mimo-v2.5-asr"
DEFAULT_DEEPSEEK_ENDPOINT = "https://api.deepseek.com/chat/completions"
DEFAULT_DEEPSEEK_MODEL = "deepseek-v4-flash"
DEFAULT_MINUTES_PROMPT_PATH = "prompts/default_minutes.md"
DEFAULT_OBSIDIAN_TEMPLATE_PATH = "prompts/obsidian_minutes.md"
VALID_DEEPSEEK_MODELS = {"deepseek-v4-flash", "deepseek-v4-pro"}


@dataclass(frozen=True)
class Settings:
    mimo_api_key: str | None
    mimo_asr_endpoint: str
    mimo_asr_model: str
    max_upload_mb: float
    default_chunk_minutes: int
    retry_count: int
    timeout_seconds: int
    minutes_provider: str = "deepseek"
    deepseek_api_key: str | None = None
    deepseek_endpoint: str = DEFAULT_DEEPSEEK_ENDPOINT
    deepseek_model: str = DEFAULT_DEEPSEEK_MODEL
    llm_timeout_seconds: int = 120
    minutes_prompt_path: Path = Path(DEFAULT_MINUTES_PROMPT_PATH)
    obsidian_template_path: Path = Path(DEFAULT_OBSIDIAN_TEMPLATE_PATH)

    def __post_init__(self) -> None:
        _require_positive("MAX_UPLOAD_MB", self.max_upload_mb)
        _require_positive("DEFAULT_CHUNK_MINUTES", self.default_chunk_minutes)
        _require_positive("MIMO_RETRY_COUNT", self.retry_count)
        _require_positive("MIMO_TIMEOUT_SECONDS", self.timeout_seconds)
        _require_positive("LLM_TIMEOUT_SECONDS", self.llm_timeout_seconds)
        if self.deepseek_model not in VALID_DEEPSEEK_MODELS:
            valid = ", ".join(sorted(VALID_DEEPSEEK_MODELS))
            raise ValueError(f"DEEPSEEK_MODEL must be one of {valid}, got {self.deepseek_model!r}.")

    @property
    def max_upload_bytes(self) -> int:
        return int(self.max_upload_mb * 1024 * 1024)

    def require_mimo_api_key(self) -> str:
        if not self.mimo_api_key:
            raise RuntimeError(
                "MIMO_API_KEY is not configured. Copy .env.example to .env and set MIMO_API_KEY."
            )
        return self.mimo_api_key

    def require_deepseek_api_key(self) -> str:
        if not self.deepseek_api_key:
            raise RuntimeError(
                "DEEPSEEK_API_KEY is not configured. Copy .env.example to .env and set DEEPSEEK_API_KEY."
            )
        return self.deepseek_api_key


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


def _get_path(name: str, default: str, base_dir: Path | None) -> Path:
    path = Path(os.getenv(name, default))
    if base_dir is not None and not path.is_absolute():
        return base_dir / path
    return path


def load_settings(env_path: Path | None = None, *, override_env: bool = False) -> Settings:
    env_base: Path | None = None
    if env_path is not None:
        resolved_env_path = env_path.expanduser().resolve()
        env_base = resolved_env_path.parent
        load_dotenv(dotenv_path=resolved_env_path, override=override_env)
    else:
        load_dotenv()

    return Settings(
        mimo_api_key=os.getenv("MIMO_API_KEY") or None,
        mimo_asr_endpoint=os.getenv("MIMO_ASR_ENDPOINT", DEFAULT_ENDPOINT),
        mimo_asr_model=os.getenv("MIMO_ASR_MODEL", DEFAULT_ASR_MODEL),
        max_upload_mb=_get_float("MAX_UPLOAD_MB", 10),
        default_chunk_minutes=_get_int("DEFAULT_CHUNK_MINUTES", 5),
        retry_count=_get_int("MIMO_RETRY_COUNT", 3),
        timeout_seconds=_get_int("MIMO_TIMEOUT_SECONDS", 120),
        minutes_provider=os.getenv("MINUTES_PROVIDER", "deepseek"),
        deepseek_api_key=os.getenv("DEEPSEEK_API_KEY") or None,
        deepseek_endpoint=os.getenv("DEEPSEEK_ENDPOINT", DEFAULT_DEEPSEEK_ENDPOINT),
        deepseek_model=os.getenv("DEEPSEEK_MODEL", DEFAULT_DEEPSEEK_MODEL),
        llm_timeout_seconds=_get_int("LLM_TIMEOUT_SECONDS", 120),
        minutes_prompt_path=_get_path("MINUTES_PROMPT_PATH", DEFAULT_MINUTES_PROMPT_PATH, env_base),
        obsidian_template_path=_get_path("OBSIDIAN_TEMPLATE_PATH", DEFAULT_OBSIDIAN_TEMPLATE_PATH, env_base),
    )

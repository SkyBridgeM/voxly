import os
from pathlib import Path

import pytest

from meeting_asr.config import Settings, load_settings


def test_settings_rejects_invalid_positive_values() -> None:
    with pytest.raises(ValueError, match="MAX_UPLOAD_MB"):
        Settings("key", "endpoint", "model", 0, 5, 3, 120)
    with pytest.raises(ValueError, match="DEFAULT_CHUNK_MINUTES"):
        Settings("key", "endpoint", "model", 10, -1, 3, 120)
    with pytest.raises(ValueError, match="MIMO_RETRY_COUNT"):
        Settings("key", "endpoint", "model", 10, 5, 0, 120)
    with pytest.raises(ValueError, match="MIMO_TIMEOUT_SECONDS"):
        Settings("key", "endpoint", "model", 10, 5, 3, 0)


def test_load_settings_rejects_invalid_env_values(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    env_file = tmp_path / ".env"
    env_file.write_text("MAX_UPLOAD_MB=0\n", encoding="utf-8")
    monkeypatch.delenv("MAX_UPLOAD_MB", raising=False)
    with pytest.raises(ValueError, match="MAX_UPLOAD_MB"):
        load_settings(env_file)
    os.environ.pop("MAX_UPLOAD_MB", None)


def test_settings_llm_timeout_rejected() -> None:
    with pytest.raises(ValueError, match="LLM_TIMEOUT_SECONDS"):
        Settings("key", "endpoint", "model", 10, 5, 3, 120, llm_timeout_seconds=0)


def test_settings_rejects_invalid_deepseek_model() -> None:
    with pytest.raises(ValueError, match="DEEPSEEK_MODEL"):
        Settings("key", "endpoint", "model", 10, 5, 3, 120, deepseek_model="deepseek-typo")


def test_settings_deepseek_default_values() -> None:
    settings = Settings("key", "endpoint", "model", 10, 5, 3, 120)
    assert settings.minutes_provider == "deepseek"
    assert settings.deepseek_api_key is None
    assert settings.deepseek_endpoint == "https://api.deepseek.com/chat/completions"
    assert settings.deepseek_model == "deepseek-v4-flash"
    assert settings.llm_timeout_seconds == 120
    assert settings.minutes_prompt_path == Path("prompts/default_minutes.md")
    assert settings.obsidian_template_path == Path("prompts/obsidian_minutes.md")


def test_settings_deepseek_custom_values() -> None:
    settings = Settings(
        "key", "endpoint", "model", 10, 5, 3, 120,
        deepseek_api_key="sk-test",
        deepseek_endpoint="https://custom.deepseek.com",
        deepseek_model="deepseek-v4-pro",
        llm_timeout_seconds=60,
        minutes_prompt_path=Path("prompts/custom.md"),
        obsidian_template_path=Path("prompts/custom_obsidian.md"),
    )
    assert settings.deepseek_api_key == "sk-test"
    assert settings.deepseek_endpoint == "https://custom.deepseek.com"
    assert settings.deepseek_model == "deepseek-v4-pro"
    assert settings.llm_timeout_seconds == 60
    assert settings.minutes_prompt_path == Path("prompts/custom.md")
    assert settings.obsidian_template_path == Path("prompts/custom_obsidian.md")


def test_require_deepseek_api_key_raises_on_missing() -> None:
    settings = Settings("key", "endpoint", "model", 10, 5, 3, 120, deepseek_api_key=None)
    with pytest.raises(RuntimeError, match="DEEPSEEK_API_KEY"):
        settings.require_deepseek_api_key()


def test_require_deepseek_api_key_returns_key() -> None:
    settings = Settings("key", "endpoint", "model", 10, 5, 3, 120, deepseek_api_key="sk-test")
    assert settings.require_deepseek_api_key() == "sk-test"


def test_load_settings_explicit_env_overrides_existing_environment(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setenv("DEEPSEEK_MODEL", "deepseek-v4-flash")
    env_file = tmp_path / ".env"
    env_file.write_text("DEEPSEEK_MODEL=deepseek-v4-pro\n", encoding="utf-8")

    settings = load_settings(env_file, override_env=True)

    assert settings.deepseek_model == "deepseek-v4-pro"


def test_load_settings_resolves_template_paths_relative_to_env_file(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    env_file = tmp_path / ".env"
    env_file.write_text(
        "MINUTES_PROMPT_PATH=prompts/custom_minutes.md\n"
        "OBSIDIAN_TEMPLATE_PATH=prompts/custom_obsidian.md\n",
        encoding="utf-8",
    )

    monkeypatch.delenv("MINUTES_PROMPT_PATH", raising=False)
    monkeypatch.delenv("OBSIDIAN_TEMPLATE_PATH", raising=False)

    settings = load_settings(env_file)

    assert settings.minutes_prompt_path == tmp_path / "prompts/custom_minutes.md"
    assert settings.obsidian_template_path == tmp_path / "prompts/custom_obsidian.md"

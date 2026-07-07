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

from pathlib import Path

import pytest

from meeting_asr.config import Settings
from meeting_asr.transcribe_mimo import extract_transcript, transcribe_audio_file


def test_extract_transcript_accepts_string_content() -> None:
    data = {"choices": [{"message": {"content": "hello"}}]}

    assert extract_transcript(data) == "hello"


def test_extract_transcript_accepts_list_content() -> None:
    data = {
        "choices": [
            {
                "message": {
                    "content": [
                        {"text": "hello"},
                        {"content": "world"},
                    ]
                }
            }
        ]
    }

    assert extract_transcript(data) == "hello\nworld"


def test_extract_transcript_rejects_unexpected_shape() -> None:
    with pytest.raises(ValueError, match="Unexpected MiMo response shape"):
        extract_transcript({"choices": []})


@pytest.mark.parametrize(
    ("status_code", "expected"),
    [
        (401, "MiMo API error 401"),
        (500, "MiMo server error 500"),
    ],
)
def test_transcribe_audio_file_wraps_api_errors(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
    status_code: int,
    expected: str,
) -> None:
    audio = tmp_path / "part_001.wav"
    audio.write_bytes(b"audio")
    settings = Settings("key", "endpoint", "model", 10, 5, 1, 120)

    class FakeResponse:
        text = "failure"

        def __init__(self, status_code: int) -> None:
            self.status_code = status_code

        def json(self) -> dict:
            return {}

    def fake_post(*args, **kwargs):
        return FakeResponse(status_code)

    monkeypatch.setattr("meeting_asr.transcribe_mimo.requests.post", fake_post)

    with pytest.raises(RuntimeError, match=expected):
        transcribe_audio_file(audio, settings)

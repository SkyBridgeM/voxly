import json
from pathlib import Path

import pytest

from meeting_asr.summarize import (
    DEEPSEEK_SYSTEM_PROMPT,
    apply_obsidian_template,
    create_notes_input,
    generate_minutes,
)


class TestCreateNotesInput:
    def test_creates_notes_input_with_prompt_and_transcript(self, tmp_path: Path) -> None:
        prompt = tmp_path / "prompt.md"
        prompt.write_text("You are a meeting assistant.", encoding="utf-8")
        transcript = tmp_path / "transcript.md"
        transcript.write_text("Hello world.", encoding="utf-8")
        output = tmp_path / "notes_input.md"

        result = create_notes_input(prompt, transcript, output)

        assert result == output
        content = output.read_text(encoding="utf-8")
        assert "You are a meeting assistant." in content
        assert "---" in content
        assert "## Transcript" in content
        assert "Hello world." in content

    def test_raises_on_missing_prompt(self, tmp_path: Path) -> None:
        with pytest.raises(FileNotFoundError, match="Prompt file not found"):
            create_notes_input(
                tmp_path / "missing.md",
                tmp_path / "transcript.md",
                tmp_path / "output.md",
            )

    def test_raises_on_missing_transcript(self, tmp_path: Path) -> None:
        prompt = tmp_path / "prompt.md"
        prompt.write_text("test", encoding="utf-8")
        with pytest.raises(FileNotFoundError, match="Transcript file not found"):
            create_notes_input(prompt, tmp_path / "missing.md", tmp_path / "output.md")


class TestGenerateMinutes:
    def test_returns_content_on_success(self, tmp_path: Path) -> None:
        notes = tmp_path / "notes.md"
        notes.write_text("Meeting notes content", encoding="utf-8")

        original_post = _import_requests_post()

        def fake_post(endpoint, json, headers, timeout):
            assert json["model"] == "deepseek-v4-flash"
            assert len(json["messages"]) == 2
            assert json["messages"][0]["role"] == "system"
            assert DEEPSEEK_SYSTEM_PROMPT in json["messages"][0]["content"]
            assert json["messages"][1]["role"] == "user"
            assert "Meeting notes content" in json["messages"][1]["content"]
            return _FakeResponse(200, {
                "choices": [{"message": {"content": "Generated minutes."}}],
            })

        try:
            import meeting_asr.summarize as mod
            mod.requests.post = fake_post

            result = generate_minutes(
                notes,
                "sk-test",
                endpoint="https://api.deepseek.com/chat/completions",
                model="deepseek-v4-flash",
                timeout_seconds=120,
            )

            assert result == "Generated minutes."
        finally:
            mod.requests.post = original_post

    def test_raises_on_401(self, tmp_path: Path) -> None:
        notes = tmp_path / "notes.md"
        notes.write_text("content", encoding="utf-8")

        original_post = _import_requests_post()

        def fake_post(endpoint, json, headers, timeout):
            return _FakeResponse(401, {})

        try:
            import meeting_asr.summarize as mod
            mod.requests.post = fake_post

            with pytest.raises(RuntimeError, match="authentication failed"):
                generate_minutes(
                    notes, "bad-key",
                    endpoint="https://api.deepseek.com/chat/completions",
                    model="deepseek-v4-flash",
                    timeout_seconds=120,
                )
        finally:
            mod.requests.post = original_post

    def test_raises_on_500(self, tmp_path: Path) -> None:
        notes = tmp_path / "notes.md"
        notes.write_text("content", encoding="utf-8")

        original_post = _import_requests_post()

        def fake_post(endpoint, json, headers, timeout):
            return _FakeResponse(500, {"error": {"message": "Internal error"}})

        try:
            import meeting_asr.summarize as mod
            mod.requests.post = fake_post

            with pytest.raises(RuntimeError, match="server error"):
                generate_minutes(
                    notes, "sk-test",
                    endpoint="https://api.deepseek.com/chat/completions",
                    model="deepseek-v4-flash",
                    timeout_seconds=120,
                )
        finally:
            mod.requests.post = original_post

    def test_raises_on_empty_content(self, tmp_path: Path) -> None:
        notes = tmp_path / "notes.md"
        notes.write_text("content", encoding="utf-8")

        original_post = _import_requests_post()

        def fake_post(endpoint, json, headers, timeout):
            return _FakeResponse(200, {
                "choices": [{"message": {"content": ""}}],
            })

        try:
            import meeting_asr.summarize as mod
            mod.requests.post = fake_post

            with pytest.raises(RuntimeError, match="empty content"):
                generate_minutes(
                    notes, "sk-test",
                    endpoint="https://api.deepseek.com/chat/completions",
                    model="deepseek-v4-flash",
                    timeout_seconds=120,
                )
        finally:
            mod.requests.post = original_post

    def test_raises_on_missing_choices(self, tmp_path: Path) -> None:
        notes = tmp_path / "notes.md"
        notes.write_text("content", encoding="utf-8")

        original_post = _import_requests_post()

        def fake_post(endpoint, json, headers, timeout):
            return _FakeResponse(200, {})

        try:
            import meeting_asr.summarize as mod
            mod.requests.post = fake_post

            with pytest.raises(RuntimeError, match="missing expected content"):
                generate_minutes(
                    notes, "sk-test",
                    endpoint="https://api.deepseek.com/chat/completions",
                    model="deepseek-v4-flash",
                    timeout_seconds=120,
                )
        finally:
            mod.requests.post = original_post

    def test_raises_on_invalid_json(self, tmp_path: Path) -> None:
        notes = tmp_path / "notes.md"
        notes.write_text("content", encoding="utf-8")

        original_post = _import_requests_post()

        def fake_post(endpoint, json, headers, timeout):
            return _FakeResponse(200, "not json", raw=True)

        try:
            import meeting_asr.summarize as mod
            mod.requests.post = fake_post

            with pytest.raises(RuntimeError, match="invalid JSON"):
                generate_minutes(
                    notes, "sk-test",
                    endpoint="https://api.deepseek.com/chat/completions",
                    model="deepseek-v4-flash",
                    timeout_seconds=120,
                )
        finally:
            mod.requests.post = original_post

    def test_raises_on_429_rate_limit(self, tmp_path: Path) -> None:
        notes = tmp_path / "notes.md"
        notes.write_text("content", encoding="utf-8")

        original_post = _import_requests_post()

        def fake_post(endpoint, json, headers, timeout):
            return _FakeResponse(429, {})

        try:
            import meeting_asr.summarize as mod
            mod.requests.post = fake_post

            with pytest.raises(RuntimeError, match="rate limit"):
                generate_minutes(
                    notes, "sk-test",
                    endpoint="https://api.deepseek.com/chat/completions",
                    model="deepseek-v4-flash",
                    timeout_seconds=120,
                )
        finally:
            mod.requests.post = original_post

    def test_raises_on_402_balance(self, tmp_path: Path) -> None:
        notes = tmp_path / "notes.md"
        notes.write_text("content", encoding="utf-8")

        original_post = _import_requests_post()

        def fake_post(endpoint, json, headers, timeout):
            return _FakeResponse(402, {})

        try:
            import meeting_asr.summarize as mod
            mod.requests.post = fake_post

            with pytest.raises(RuntimeError, match="account balance"):
                generate_minutes(
                    notes, "sk-test",
                    endpoint="https://api.deepseek.com/chat/completions",
                    model="deepseek-v4-flash",
                    timeout_seconds=120,
                )
        finally:
            mod.requests.post = original_post

    def test_raises_on_unexpected_4xx(self, tmp_path: Path) -> None:
        notes = tmp_path / "notes.md"
        notes.write_text("content", encoding="utf-8")

        original_post = _import_requests_post()

        def fake_post(endpoint, json, headers, timeout):
            return _FakeResponse(400, {"error": {"message": "Bad request"}})

        try:
            import meeting_asr.summarize as mod
            mod.requests.post = fake_post

            with pytest.raises(RuntimeError, match="client error"):
                generate_minutes(
                    notes, "sk-test",
                    endpoint="https://api.deepseek.com/chat/completions",
                    model="deepseek-v4-flash",
                    timeout_seconds=120,
                )
        finally:
            mod.requests.post = original_post

    def test_raises_on_timeout(self, tmp_path: Path) -> None:
        import requests as requests_lib

        notes = tmp_path / "notes.md"
        notes.write_text("content", encoding="utf-8")

        original_post = _import_requests_post()

        def fake_post(endpoint, json, headers, timeout):
            raise requests_lib.exceptions.Timeout()

        try:
            import meeting_asr.summarize as mod
            mod.requests.post = fake_post

            with pytest.raises(RuntimeError, match="timed out"):
                generate_minutes(
                    notes, "sk-test",
                    endpoint="https://api.deepseek.com/chat/completions",
                    model="deepseek-v4-flash",
                    timeout_seconds=120,
                )
        finally:
            mod.requests.post = original_post

    def test_raises_on_connection_error(self, tmp_path: Path) -> None:
        import requests as requests_lib

        notes = tmp_path / "notes.md"
        notes.write_text("content", encoding="utf-8")

        original_post = _import_requests_post()

        def fake_post(endpoint, json, headers, timeout):
            raise requests_lib.exceptions.ConnectionError()

        try:
            import meeting_asr.summarize as mod
            mod.requests.post = fake_post

            with pytest.raises(RuntimeError, match="Failed to connect"):
                generate_minutes(
                    notes, "sk-test",
                    endpoint="https://api.deepseek.com/chat/completions",
                    model="deepseek-v4-flash",
                    timeout_seconds=120,
                )
        finally:
            mod.requests.post = original_post

    def test_payload_contains_fixed_system_prompt(self, tmp_path: Path) -> None:
        notes = tmp_path / "notes.md"
        notes.write_text("Test content", encoding="utf-8")

        original_post = _import_requests_post()
        captured_payload = []

        def fake_post(endpoint, json, headers, timeout):
            captured_payload.append(json)
            return _FakeResponse(200, {
                "choices": [{"message": {"content": "OK."}}],
            })

        try:
            import meeting_asr.summarize as mod
            mod.requests.post = fake_post

            generate_minutes(
                notes, "sk-test",
                endpoint="https://api.deepseek.com/chat/completions",
                model="deepseek-v4-flash",
                timeout_seconds=120,
            )

            payload = captured_payload[0]
            assert payload["model"] == "deepseek-v4-flash"
            assert payload["stream"] is False
            messages = payload["messages"]
            assert messages[0]["role"] == "system"
            assert "不编造任何事实" in messages[0]["content"]
            assert messages[1]["role"] == "user"
            assert "Test content" in messages[1]["content"]
        finally:
            mod.requests.post = original_post


class TestApplyObsidianTemplate:
    def test_substitutes_all_placeholders(self, tmp_path: Path) -> None:
        template = tmp_path / "obsidian.md"
        template.write_text(
            '---\ntitle: "{{ title }}"\ndate: "{{ generated_at }}"\n---\n\n'
            "# {{ title }}\n\n"
            "> 任务：{{ task_name }}\n\n"
            "{{ generated_minutes }}\n\n"
            "## 附录\n\n{{ transcript }}\n",
            encoding="utf-8",
        )

        result = apply_obsidian_template(
            template,
            minutes_text="## Summary\n\nMeeting was productive.",
            title="Q3 Planning",
            task_name="20250708_120000_Q3_Planning",
            transcript="[Part 001]\n\nHello.\n",
        )

        assert "Q3 Planning" in result
        assert "20250708_120000_Q3_Planning" in result
        assert "Meeting was productive" in result
        assert "[Part 001]" in result
        assert "{{" not in result

    def test_default_transcript_is_empty(self, tmp_path: Path) -> None:
        template = tmp_path / "obsidian.md"
        template.write_text("# {{ title }}\n\n{{ generated_minutes }}\n\n{{ transcript }}", encoding="utf-8")

        result = apply_obsidian_template(
            template,
            minutes_text="Summary.",
            title="Test",
            task_name="task",
        )

        assert result.strip() == "# Test\n\nSummary."

    def test_raises_on_missing_template(self, tmp_path: Path) -> None:
        with pytest.raises(FileNotFoundError, match="Obsidian template file not found"):
            apply_obsidian_template(
                Path("/nonexistent/obsidian.md"),
                minutes_text="test",
                title="Test",
                task_name="task",
            )


class _FakeResponse:
    def __init__(self, status_code: int, data, raw: bool = False):
        self.status_code = status_code
        self._data = data
        self._raw = raw
        self.ok = 200 <= status_code < 400

    def json(self):
        if self._raw:
            raise ValueError("Not JSON")
        return self._data

    @property
    def text(self):
        if isinstance(self._data, str):
            return self._data
        return json.dumps(self._data)


def _import_requests_post():
    import meeting_asr.summarize as mod
    return mod.requests.post

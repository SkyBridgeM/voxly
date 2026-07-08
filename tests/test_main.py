import pytest
from pathlib import Path

from meeting_asr.main import _resolve_output_mode, build_parser


class NullArgs:
    pass


def test_resolve_output_mode_uses_output_mode_first() -> None:
    args = NullArgs()
    args.output_mode = "minutes"
    args.summarize = True
    assert _resolve_output_mode(args) == "minutes"


def test_resolve_output_mode_falls_back_to_summarize() -> None:
    args = NullArgs()
    args.output_mode = None
    args.summarize = True
    assert _resolve_output_mode(args) == "notes"


def test_resolve_output_mode_defaults_to_transcript() -> None:
    args = NullArgs()
    args.output_mode = None
    args.summarize = False
    assert _resolve_output_mode(args) == "transcript"


def test_output_mode_flag_accepted_by_transcribe() -> None:
    parser = build_parser()
    args = parser.parse_args(["transcribe", "input/test.mp3", "--output-mode", "minutes"])
    assert args.output_mode == "minutes"


def test_output_mode_flag_accepted_by_record() -> None:
    parser = build_parser()
    args = parser.parse_args(["record", "--output-mode", "notes"])
    assert args.output_mode == "notes"


def test_output_mode_flag_accepted_by_resume() -> None:
    parser = build_parser()
    args = parser.parse_args(["resume", "output/task", "--output-mode", "minutes"])
    assert args.output_mode == "minutes"


def test_llm_provider_and_model_flags() -> None:
    parser = build_parser()
    args = parser.parse_args([
        "transcribe", "input/test.mp3",
        "--output-mode", "minutes",
        "--llm-provider", "deepseek",
        "--llm-model", "deepseek-v4-pro",
    ])
    assert args.llm_provider == "deepseek"
    assert args.llm_model == "deepseek-v4-pro"


def test_obsidian_template_flag() -> None:
    parser = build_parser()
    args = parser.parse_args([
        "transcribe", "input/test.mp3",
        "--output-mode", "minutes",
        "--obsidian-template", "prompts/custom_obsidian.md",
    ])
    assert str(args.obsidian_template) == "prompts/custom_obsidian.md"


def test_summarize_still_accepted_but_deprecated() -> None:
    parser = build_parser()
    args = parser.parse_args(["transcribe", "input/test.mp3", "--summarize"])
    assert args.summarize
    assert args.output_mode is None


def test_summarize_and_output_mode_combined_output_mode_wins() -> None:
    parser = build_parser()
    args = parser.parse_args([
        "transcribe", "input/test.mp3",
        "--output-mode", "minutes",
        "--summarize",
    ])
    assert args.summarize
    assert args.output_mode == "minutes"


def test_resume_rejects_common_flags() -> None:
    parser = build_parser()
    with pytest.raises(SystemExit):
        parser.parse_args(["resume", "output/task", "--chunk-minutes", "3"])
    with pytest.raises(SystemExit):
        parser.parse_args(["resume", "output/task", "--output-dir", "elsewhere"])


def test_resume_accepts_output_mode_and_llm_flags() -> None:
    parser = build_parser()
    args = parser.parse_args([
        "resume", "output/task",
        "--output-mode", "minutes",
        "--llm-provider", "deepseek",
        "--llm-model", "deepseek-v4-flash",
        "--obsidian-template", "prompts/obsidian_minutes.md",
        "--prompt", "prompts/custom.md",
    ])
    assert args.output_mode == "minutes"
    assert args.llm_provider == "deepseek"
    assert args.llm_model == "deepseek-v4-flash"
    assert str(args.obsidian_template) == "prompts/obsidian_minutes.md"
    assert str(args.prompt) == "prompts/custom.md"


def test_output_mode_transcript_is_default() -> None:
    parser = build_parser()
    args = parser.parse_args(["transcribe", "input/test.mp3"])
    assert args.output_mode is None
    assert not args.summarize


def test_invalid_output_mode_rejected() -> None:
    parser = build_parser()
    with pytest.raises(SystemExit):
        parser.parse_args(["transcribe", "input/test.mp3", "--output-mode", "invalid"])


def test_invalid_llm_provider_rejected() -> None:
    parser = build_parser()
    with pytest.raises(SystemExit):
        parser.parse_args(["transcribe", "input/test.mp3", "--llm-provider", "openai"])


def test_prompt_default_is_none() -> None:
    parser = build_parser()
    args = parser.parse_args(["transcribe", "input/test.mp3"])
    assert args.prompt is None


def test_prompt_uses_env_default(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("MINUTES_PROMPT_PATH", "prompts/custom_minutes.md")
    monkeypatch.setenv("MIMO_API_KEY", "key")
    # --prompt not passed → should use env default
    parser = build_parser()
    args = parser.parse_args(["transcribe", "input/test.mp3"])
    assert args.prompt is None
    from meeting_asr.config import load_settings
    settings = load_settings()
    assert settings.minutes_prompt_path == Path("prompts/custom_minutes.md")


def test_prompt_cli_overrides_env(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("MINUTES_PROMPT_PATH", "prompts/custom_minutes.md")
    parser = build_parser()
    args = parser.parse_args(["transcribe", "input/test.mp3", "--prompt", "prompts/override.md"])
    assert str(args.prompt) == "prompts/override.md"


def test_obsidian_template_default_is_none() -> None:
    parser = build_parser()
    args = parser.parse_args(["transcribe", "input/test.mp3"])
    assert args.obsidian_template is None


def test_obsidian_template_uses_env_default(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("OBSIDIAN_TEMPLATE_PATH", "prompts/custom_obsidian.md")
    monkeypatch.setenv("MIMO_API_KEY", "key")
    from meeting_asr.config import load_settings
    settings = load_settings()
    assert settings.obsidian_template_path == Path("prompts/custom_obsidian.md")


def test_obsidian_template_cli_overrides_env(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("OBSIDIAN_TEMPLATE_PATH", "prompts/custom_obsidian.md")
    parser = build_parser()
    args = parser.parse_args([
        "transcribe", "input/test.mp3",
        "--obsidian-template", "prompts/override_obsidian.md",
    ])
    assert str(args.obsidian_template) == "prompts/override_obsidian.md"


def test_minutes_provider_uses_env_default(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("MINUTES_PROVIDER", "manual")
    monkeypatch.setenv("MIMO_API_KEY", "key")
    from meeting_asr.config import load_settings
    settings = load_settings()
    assert settings.minutes_provider == "manual"


def test_llm_provider_cli_overrides_env(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("MINUTES_PROVIDER", "manual")
    parser = build_parser()
    args = parser.parse_args([
        "transcribe", "input/test.mp3",
        "--output-mode", "minutes",
        "--llm-provider", "deepseek",
    ])
    assert args.llm_provider == "deepseek"


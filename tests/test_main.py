import pytest

from meeting_asr.main import build_parser


def test_resume_parser_accepts_only_notes_flags() -> None:
    parser = build_parser()
    args = parser.parse_args(["resume", "output/task", "--summarize", "--prompt", "prompts/custom.md"])

    assert args.command == "resume"
    assert str(args.task_dir) == "output/task"
    assert args.summarize
    assert str(args.prompt) == "prompts/custom.md"

    with pytest.raises(SystemExit):
        parser.parse_args(["resume", "output/task", "--chunk-minutes", "3"])

    with pytest.raises(SystemExit):
        parser.parse_args(["resume", "output/task", "--output-dir", "elsewhere"])

from __future__ import annotations

from datetime import datetime, timezone
from pathlib import Path

import requests

DEEPSEEK_SYSTEM_PROMPT = """\
你是一个会议纪要助手。请严格遵循以下底线约束：

- 只根据用户提供的转写文本和 prompt 生成纪要；
- 不编造任何事实、人物、日期、数字或行动项；
- 不确定的内容标注"不确定"或"未提及"；
- 输出使用 Markdown 格式；
- 用户 prompt 中的格式要求优先于上述原则。"""


def create_notes_input(prompt_path: Path, transcript_path: Path, output_path: Path) -> Path:
    if not prompt_path.exists():
        raise FileNotFoundError(f"Prompt file not found: {prompt_path}")
    if not transcript_path.exists():
        raise FileNotFoundError(f"Transcript file not found: {transcript_path}")

    prompt = prompt_path.read_text(encoding="utf-8").strip()
    transcript = transcript_path.read_text(encoding="utf-8").strip()
    content = (
        f"{prompt}\n\n"
        "---\n\n"
        "## Transcript\n\n"
        f"{transcript}\n"
    )
    output_path.write_text(content, encoding="utf-8")
    return output_path


def generate_minutes(
    notes_input_path: Path,
    api_key: str,
    *,
    endpoint: str,
    model: str,
    timeout_seconds: int,
) -> str:
    notes_text = notes_input_path.read_text(encoding="utf-8").strip()

    payload = {
        "model": model,
        "messages": [
            {"role": "system", "content": DEEPSEEK_SYSTEM_PROMPT},
            {"role": "user", "content": notes_text},
        ],
        "stream": False,
    }

    try:
        response = requests.post(
            endpoint,
            json=payload,
            headers={
                "Authorization": f"Bearer {api_key}",
                "Content-Type": "application/json",
            },
            timeout=timeout_seconds,
        )
    except requests.exceptions.Timeout:
        raise RuntimeError("DeepSeek API request timed out.")
    except requests.exceptions.ConnectionError:
        raise RuntimeError("Failed to connect to DeepSeek API. Check DEEPSEEK_ENDPOINT.")

    if response.status_code == 401:
        raise RuntimeError("DeepSeek API authentication failed (401). Check DEEPSEEK_API_KEY.")
    if response.status_code == 402:
        raise RuntimeError("DeepSeek API returned 402. Check your account balance.")
    if response.status_code == 429:
        raise RuntimeError("DeepSeek API rate limit exceeded (429). Retry later.")
    if 400 <= response.status_code < 500:
        raise RuntimeError(f"DeepSeek API client error ({response.status_code}): {_extract_error(response)}")
    if 500 <= response.status_code < 600:
        raise RuntimeError(f"DeepSeek API server error ({response.status_code}): {_extract_error(response)}")
    if not response.ok:
        raise RuntimeError(f"DeepSeek API unexpected response ({response.status_code}).")

    try:
        data = response.json()
    except ValueError:
        raise RuntimeError("DeepSeek API returned invalid JSON.")

    try:
        content = data["choices"][0]["message"]["content"]
    except (KeyError, IndexError, TypeError):
        raise RuntimeError("DeepSeek API response missing expected content.")

    if not content or not content.strip():
        raise RuntimeError("DeepSeek API returned empty content.")

    return content.strip()


def _extract_error(response: requests.Response) -> str:
    try:
        body = response.json()
        return body.get("error", {}).get("message", response.text[:300])
    except (ValueError, KeyError, TypeError):
        return response.text[:300]


def apply_obsidian_template(
    template_path: Path,
    *,
    minutes_text: str,
    title: str,
    task_name: str,
    transcript: str = "",
) -> str:
    if not template_path.exists():
        raise FileNotFoundError(f"Obsidian template file not found: {template_path}")

    template = template_path.read_text(encoding="utf-8")
    generated_at = datetime.now(timezone.utc).strftime("%Y-%m-%d %H:%M UTC")

    result = template
    result = result.replace("{{ title }}", title)
    result = result.replace("{{ generated_at }}", generated_at)
    result = result.replace("{{ generated_minutes }}", minutes_text)
    result = result.replace("{{ transcript }}", transcript)
    result = result.replace("{{ task_name }}", task_name)
    return result

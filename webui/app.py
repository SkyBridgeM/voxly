from __future__ import annotations

import sys
from dataclasses import replace
from pathlib import Path

import streamlit as st

ROOT = Path(__file__).resolve().parents[1]
SRC = ROOT / "src"
if str(SRC) not in sys.path:
    sys.path.insert(0, str(SRC))

from meeting_asr.config import load_settings
from meeting_asr.workflow import TranscribeOptions, run_uploaded_audio_workflow
from ui_helpers import (
    build_text_outputs_zip,
    classify_error,
    format_detected,
    read_preview,
    update_env_value,
)


def apply_style() -> None:
    st.markdown(
        """
        <style>
        :root {
          --surface: #f7f8fa;
          --panel: #ffffff;
          --text: #18202a;
          --muted: #5b6573;
          --line: #d9dee7;
          --accent: #245bdb;
          --accent-dark: #1746ad;
          --sidebar-bg: #eef1f5;
          --code-bg: #f6f8fa;
          --code-text: #1f3f8f;
          --button-border: #b9c2d1;
        }

        @media (prefers-color-scheme: dark) {
          :root {
            --surface: #0e1117;
            --panel: #1a1c23;
            --text: #e6edf3;
            --muted: #8b949e;
            --line: #30363d;
            --accent: #58a6ff;
            --accent-dark: #1f6feb;
            --sidebar-bg: #0d1117;
            --code-bg: #161b22;
            --code-text: #a5d6ff;
            --button-border: #444c56;
          }

          /* -- alerts (warning / info / success / error) -- */
          div[data-testid="stAlert"] {
            background-color: #1a1c23;
            border-color: var(--line);
          }
          div[data-testid="stAlert"] * {
            color: var(--text);
          }

          /* -- sidebar: labels, captions, markdown text -- */
          section[data-testid="stSidebar"] label,
          section[data-testid="stSidebar"] .stCaption,
          section[data-testid="stSidebar"] small,
          section[data-testid="stSidebar"] p,
          section[data-testid="stSidebar"] span,
          section[data-testid="stSidebar"] h1,
          section[data-testid="stSidebar"] h2,
          section[data-testid="stSidebar"] h3,
          section[data-testid="stSidebar"] h4 {
            color: var(--text);
          }

          /* -- inputs (text, password, number) -- */
          section[data-testid="stSidebar"] input,
          section[data-testid="stSidebar"] textarea {
            background-color: #1a1c23;
            color: var(--text);
            border-color: var(--line);
          }
          input:disabled {
            color: var(--muted);
          }

          /* -- selectbox -- */
          div[data-baseweb="select"] {
            color: var(--text);
          }

          /* -- checkbox label -- */
          section[data-testid="stSidebar"] label[data-testid="stWidgetLabel"],
          label[data-testid="stWidgetLabel"] {
            color: var(--text);
          }

          /* -- file uploader drop-zone -- */
          [data-testid="stFileUploader"] section {
            background-color: #1a1c23;
            border-color: var(--line);
          }
          [data-testid="stFileUploader"] small,
          [data-testid="stFileUploader"] span,
          [data-testid="stFileUploader"] p {
            color: var(--text);
          }

          /* -- markdown / code-blocks inside sidebar -- */
          section[data-testid="stSidebar"] .stMarkdown {
            color: var(--text);
          }
        }

        .stApp {
          background: var(--surface);
          color: var(--text);
        }
        section[data-testid="stSidebar"] {
          background: var(--sidebar-bg);
          border-right: 1px solid var(--line);
        }
        .main .block-container {
          max-width: 1180px;
          padding-top: 2.5rem;
          padding-bottom: 4rem;
        }
        h1, h2, h3 {
          letter-spacing: 0;
        }
        h1 {
          font-size: 2.2rem;
          line-height: 1.08;
          margin-bottom: 0.35rem;
        }
        h2 {
          border-top: 1px solid var(--line);
          padding-top: 1.3rem;
          margin-top: 1.6rem;
        }
        div[data-testid="stAlert"] {
          border-radius: 8px;
        }
        .masr-hero {
          border-bottom: 1px solid var(--line);
          padding-bottom: 1.15rem;
          margin-bottom: 1.2rem;
        }
        .masr-subtle {
          color: var(--muted);
          max-width: 68ch;
          font-size: 0.98rem;
          line-height: 1.6;
        }
        .masr-status {
          border: 1px solid var(--line);
          border-radius: 8px;
          background: var(--panel);
          padding: 0.85rem 1rem;
          margin: 0.75rem 0;
        }
        .stButton > button,
        .stDownloadButton > button {
          border-radius: 8px;
          border: 1px solid var(--button-border);
          font-weight: 600;
          transition: border-color 160ms ease, background 160ms ease, transform 120ms ease;
        }
        .stButton > button:active,
        .stDownloadButton > button:active {
          transform: translateY(1px);
        }
        .stProgress > div > div > div > div {
          background-color: var(--accent);
        }
        code {
          color: var(--code-text);
          background: var(--code-bg);
        }
        </style>
        """,
        unsafe_allow_html=True,
    )


def render_download(path: Path, label: str, mime: str = "text/markdown") -> None:
    if path.exists():
        st.download_button(
            label,
            data=path.read_bytes(),
            file_name=path.name,
            mime=mime,
            use_container_width=True,
        )


def _resolve_template_path(raw: Path) -> Path:
    if raw.is_absolute():
        return raw
    return ROOT / raw


def main() -> None:
    st.set_page_config(page_title="meeting-asr", page_icon=None, layout="wide")
    apply_style()

    settings = load_settings(ROOT / ".env", override_env=True)

    with st.sidebar:
        st.header("设置")
        st.caption("仅本次生效的本地设置")

        # ── Output mode ──
        output_mode = st.radio(
            "输出模式",
            options=["输出原文", "生成总结"],
            index=0,
            help=(
                "输出原文：只生成完整转写文本 transcript.md。音频发送给 MiMo ASR。\n\n"
                "生成总结：转写后调用 LLM（如 DeepSeek）自动生成 meeting_minutes.md。"
                "转写文本会发送给所选 LLM provider。"
            ),
        )
        minutes_mode = output_mode == "生成总结"

        if minutes_mode:
            st.markdown("---")
            st.subheader("LLM 总结设置")
            provider = st.selectbox("Provider", options=["DeepSeek"], index=0, disabled=True)
            model_label = st.selectbox(
                "模型",
                options=["DeepSeek V4 Flash（更快/更省）", "DeepSeek V4 Pro（质量更高）"],
                index=1 if settings.deepseek_model == "deepseek-v4-pro" else 0,
            )
            selected_model = "deepseek-v4-flash" if "Flash" in model_label else "deepseek-v4-pro"

            if st.button("保存为默认总结服务商", use_container_width=True):
                update_env_value(ROOT / ".env", "MINUTES_PROVIDER", "deepseek")
                update_env_value(ROOT / ".env", "DEEPSEEK_MODEL", selected_model)
                st.success("已保存默认总结服务商配置到 .env。")
                st.rerun()

        # ── API keys (collapsed by default) ──
        st.markdown("---")
        with st.expander("API keys", expanded=False):
            st.caption("检测状态与临时覆盖（不显示明文）")

            # MiMo key
            mimo_status = "已在 .env 中检测到" if settings.mimo_api_key else format_detected(False)
            st.text_input("MIMO API key（用于 ASR 转写）", value=mimo_status, disabled=True, key="mimo_status")
            mimo_override = st.text_input(
                "本次使用的 MIMO API key",
                value="",
                type="password",
                key="mimo_override",
                placeholder="留空则使用 .env 中的 key" if settings.mimo_api_key else "粘贴本次使用的 MIMO API key",
            )
            save_mimo = st.checkbox("保存 MIMO API key 到 .env", value=False, key="save_mimo")
            if st.button("保存 MIMO API key", disabled=not save_mimo, use_container_width=True, key="btn_mimo"):
                key_to_save = mimo_override.strip()
                if not key_to_save:
                    st.warning("保存前请先粘贴 MIMO API key。")
                else:
                    update_env_value(ROOT / ".env", "MIMO_API_KEY", key_to_save)
                    st.success("已将 MIMO_API_KEY 保存到 .env。")
                    st.rerun()

            st.markdown("---")

            # DeepSeek key
            ds_status = "已在 .env 中检测到" if settings.deepseek_api_key else format_detected(False)
            st.text_input("DeepSeek API key（用于 LLM 总结）", value=ds_status, disabled=True, key="ds_status")
            ds_override = st.text_input(
                "本次使用的 DeepSeek API key",
                value="",
                type="password",
                key="ds_override",
                placeholder="留空则使用 .env 中的 key" if settings.deepseek_api_key else "粘贴本次使用的 DeepSeek API key",
            )
            save_ds = st.checkbox("保存 DeepSeek API key 到 .env", value=False, key="save_ds")
            if st.button("保存 DeepSeek API key", disabled=not save_ds, use_container_width=True, key="btn_ds"):
                key_to_save = ds_override.strip()
                if not key_to_save:
                    st.warning("保存前请先粘贴 DeepSeek API key。")
                else:
                    update_env_value(ROOT / ".env", "DEEPSEEK_API_KEY", key_to_save)
                    st.success("已将 DEEPSEEK_API_KEY 保存到 .env。")
                    st.rerun()

    st.markdown(
        """
        <div class="masr-hero">
          <h1>meeting-asr</h1>
          <p class="masr-subtle">
            上传会议音频，自动分段，调用 MiMo ASR 转写，可选 LLM 自动生成会议总结。
          </p>
        </div>
        """,
        unsafe_allow_html=True,
    )

    if minutes_mode:
        st.warning(
            "转写文本将发送至 DeepSeek 生成会议总结。请勿上传未授权的敏感会议内容。",
            icon=None,
        )
    else:
        st.warning(
            "转写时音频将上传至 MiMo ASR。请勿上传涉及敏感内容的会议音频，除非你已获授权使用云端 ASR。",
            icon=None,
        )

    upload_col, result_col = st.columns([0.95, 1.35], gap="large")

    with upload_col:
        st.subheader("上传音频")
        uploaded_file = st.file_uploader("选择 mp3 或 wav 文件", type=["mp3", "wav"])

        st.subheader("录音")
        st.info(
            "WebUI 暂不支持录音。请使用 CLI 进行本地 ffmpeg 录音：\n\n"
            "`meeting-asr record --duration 60 --chunk-minutes 5`",
            icon=None,
        )

        run_button = st.button("开始转写", type="primary", disabled=uploaded_file is None, use_container_width=True)

    with result_col:
        st.subheader("运行状态")
        status_box = st.empty()
        progress = st.progress(0)
        detail_box = st.empty()

        if "last_result" in st.session_state:
            render_results(Path(st.session_state["last_result"]))

    if run_button and uploaded_file is not None:
        try:
            effective_settings = settings
            if mimo_override.strip():
                effective_settings = replace(effective_settings, mimo_api_key=mimo_override.strip())
            if ds_override.strip():
                effective_settings = replace(effective_settings, deepseek_api_key=ds_override.strip())

            resolved_mode = "minutes" if minutes_mode else "transcript"

            options = TranscribeOptions(
                output_dir=Path("output"),
                chunk_minutes=int(settings.default_chunk_minutes),
                max_upload_mb=float(settings.max_upload_mb),
                output_mode=resolved_mode,
                prompt_path=_resolve_template_path(settings.minutes_prompt_path),
                llm_provider="deepseek",
                llm_model=selected_model if minutes_mode else settings.deepseek_model,
                obsidian_template_path=_resolve_template_path(settings.obsidian_template_path),
            )

            def status(message: str) -> None:
                status_box.markdown(f'<div class="masr-status"><strong>{message}</strong></div>', unsafe_allow_html=True)

            def chunk_status(state: str, index: int, total: int, chunk_path: Path) -> None:
                ratio = index / total if total else 0
                progress.progress(min(ratio, 1.0))
                if state == "skipped":
                    detail_box.info(f"跳过已有转录 {index}/{total}: {chunk_path.name}", icon=None)
                elif state == "completed":
                    detail_box.success(f"已完成片段 {index}/{total}: {chunk_path.name}", icon=None)
                else:
                    detail_box.info(f"正在转写片段 {index}/{total}: {chunk_path.name}", icon=None)

            result = run_uploaded_audio_workflow(
                uploaded_file.name,
                uploaded_file.getvalue(),
                effective_settings,
                options,
                status_callback=status,
                chunk_callback=chunk_status,
            )
            progress.progress(1.0)
            status_box.success("完成")
            st.session_state["last_result"] = str(result.task_dir)
            render_results(result.task_dir)
        except Exception as exc:
            progress.progress(0)
            title, detail = classify_error(exc)
            status_box.error(title)
            detail_box.error(detail)


def render_results(task_dir: Path) -> None:
    st.subheader("结果")
    st.caption(f"任务目录: `{task_dir}`")

    transcript_path = task_dir / "transcript.md"
    notes_path = task_dir / "notes_input.md"
    minutes_path = task_dir / "meeting_minutes.md"

    if transcript_path.exists():
        preview, truncated = read_preview(transcript_path)
        label = "转录预览"
        if truncated:
            label += "（已截断）"
        with st.expander(label, expanded=True):
            st.markdown(preview)

    download_cols = st.columns(2)
    with download_cols[0]:
        render_download(transcript_path, "下载 transcript.md")
        render_download(notes_path, "下载 notes_input.md")
    with download_cols[1]:
        render_download(minutes_path, "下载 meeting_minutes.md")
        zip_bytes = build_text_outputs_zip(task_dir)
        if zip_bytes:
            st.download_button(
                "下载文本输出 zip",
                data=zip_bytes,
                file_name=f"{task_dir.name}_text_outputs.zip",
                mime="application/zip",
                use_container_width=True,
            )

    if minutes_path.exists():
        with st.expander("meeting_minutes.md", expanded=False):
            st.markdown(minutes_path.read_text(encoding="utf-8"))


if __name__ == "__main__":
    main()

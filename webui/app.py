from __future__ import annotations

import sys
from dataclasses import replace
from pathlib import Path

import streamlit as st

ROOT = Path(__file__).resolve().parents[1]
SRC = ROOT / "src"
if str(SRC) not in sys.path:
    sys.path.insert(0, str(SRC))
if str(ROOT) not in sys.path:
    sys.path.insert(0, str(ROOT))

from meeting_asr.config import load_settings
from meeting_asr.workflow import TranscribeOptions, run_uploaded_audio_workflow
from webui.components.results import render_results
from webui.components.sidebar import render_sidebar
from webui.styles import apply_style
from webui.ui_helpers import classify_error


def _resolve_template_path(raw: Path) -> Path:
    if raw.is_absolute():
        return raw
    return ROOT / raw


def main() -> None:
    st.set_page_config(page_title="meeting-asr", page_icon=None, layout="wide")
    apply_style()

    settings = load_settings(ROOT / ".env", override_env=True)

    with st.sidebar:
        state = render_sidebar(settings, ROOT)

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

    if state.minutes_mode:
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
            if state.mimo_override.strip():
                effective_settings = replace(effective_settings, mimo_api_key=state.mimo_override.strip())
            if state.ds_override.strip():
                effective_settings = replace(effective_settings, deepseek_api_key=state.ds_override.strip())

            resolved_mode = "minutes" if state.minutes_mode else "transcript"

            options = TranscribeOptions(
                output_dir=Path("output"),
                chunk_minutes=int(settings.default_chunk_minutes),
                max_upload_mb=float(settings.max_upload_mb),
                output_mode=resolved_mode,
                prompt_path=_resolve_template_path(settings.minutes_prompt_path),
                llm_provider="deepseek",
                llm_model=state.selected_model if state.minutes_mode else settings.deepseek_model,
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


if __name__ == "__main__":
    main()

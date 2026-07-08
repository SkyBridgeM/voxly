from __future__ import annotations

from pathlib import Path

import streamlit as st

from webui.ui_helpers import build_text_outputs_zip, read_preview


def render_download(path: Path, label: str, mime: str = "text/markdown") -> None:
    if path.exists():
        st.download_button(
            label,
            data=path.read_bytes(),
            file_name=path.name,
            mime=mime,
            use_container_width=True,
        )


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

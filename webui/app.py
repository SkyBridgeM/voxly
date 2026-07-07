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
    discover_prompt_templates,
    format_detected,
    read_preview,
    resolve_prompt_path,
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


def main() -> None:
    st.set_page_config(page_title="meeting-asr", page_icon=None, layout="wide")
    apply_style()

    settings = load_settings()
    prompts = discover_prompt_templates(ROOT / "prompts")
    prompt_labels = [str(path.relative_to(ROOT)) for path in prompts]

    with st.sidebar:
        st.header("Settings")
        st.caption("Local settings for this run only.")
        api_key_status = "Detected from .env" if settings.mimo_api_key else format_detected(False)
        st.text_input("MIMO_API_KEY", value=api_key_status, disabled=True)
        api_key_input = st.text_input(
            "MIMO API key for this run",
            value="",
            type="password",
            placeholder="Leave blank to use the .env key"
            if settings.mimo_api_key
            else "Paste a MIMO API key for this run",
        )
        save_api_key = st.checkbox("Save API key to .env for future runs", value=False)
        if st.button("Save API key", disabled=not save_api_key, use_container_width=True):
            api_key_to_save = api_key_input.strip()
            if not api_key_to_save:
                st.warning("Paste a MIMO API key before saving.")
            else:
                update_env_value(ROOT / ".env", "MIMO_API_KEY", api_key_to_save)
                st.success("Saved MIMO_API_KEY to .env.")
                st.rerun()
        output_dir = st.text_input("Output directory", value="output")
        chunk_minutes = st.number_input(
            "Chunk length, minutes",
            min_value=1,
            max_value=60,
            value=int(settings.default_chunk_minutes),
            step=1,
        )
        max_upload_mb = st.number_input(
            "Max upload size, MB",
            min_value=1.0,
            max_value=200.0,
            value=float(settings.max_upload_mb),
            step=1.0,
        )
        summarize = st.checkbox("生成纪要输入稿", value=False)

        selected_prompt = None
        if prompt_labels:
            selected_prompt = st.selectbox("Prompt template", prompt_labels, index=0)
        else:
            st.warning("No prompt templates found in prompts/.")
        custom_prompt = st.text_input("Custom prompt path", value="")

    st.markdown(
        """
        <div class="masr-hero">
          <h1>meeting-asr</h1>
          <p class="masr-subtle">
            Upload meeting audio, split it into safe chunks, transcribe with MiMo ASR, and download clean local outputs.
          </p>
        </div>
        """,
        unsafe_allow_html=True,
    )

    st.warning(
        "Audio will be sent to MiMo ASR when transcription runs. Do not upload sensitive meetings unless you are allowed to use cloud ASR.",
        icon=None,
    )

    upload_col, result_col = st.columns([0.95, 1.35], gap="large")

    with upload_col:
        st.subheader("Upload audio")
        uploaded_file = st.file_uploader("Choose an mp3 or wav file", type=["mp3", "wav"])

        st.subheader("Recording")
        st.info(
            "WebUI recording is not enabled in this MVP. Use the CLI for reliable local ffmpeg recording:\n\n"
            "`meeting-asr record --duration 60 --chunk-minutes 5`",
            icon=None,
        )

        run_button = st.button("Start transcription", type="primary", disabled=uploaded_file is None, use_container_width=True)

    with result_col:
        st.subheader("Run status")
        status_box = st.empty()
        progress = st.progress(0)
        detail_box = st.empty()

        if "last_result" in st.session_state:
            render_results(Path(st.session_state["last_result"]))

    if run_button and uploaded_file is not None:
        try:
            effective_settings = settings
            api_key_override = api_key_input.strip()
            if api_key_override:
                effective_settings = replace(settings, mimo_api_key=api_key_override)

            options = TranscribeOptions(
                output_dir=Path(output_dir),
                chunk_minutes=int(chunk_minutes),
                max_upload_mb=float(max_upload_mb),
                summarize=bool(summarize),
                prompt_path=resolve_prompt_path(selected_prompt, custom_prompt),
            )

            def status(message: str) -> None:
                status_box.markdown(f'<div class="masr-status"><strong>{message}</strong></div>', unsafe_allow_html=True)

            def chunk_status(state: str, index: int, total: int, chunk_path: Path) -> None:
                ratio = index / total if total else 0
                progress.progress(min(ratio, 1.0))
                if state == "skipped":
                    detail_box.info(f"Skipping existing transcript {index}/{total}: {chunk_path.name}", icon=None)
                elif state == "completed":
                    detail_box.success(f"Completed chunk {index}/{total}: {chunk_path.name}", icon=None)
                else:
                    detail_box.info(f"Transcribing chunk {index}/{total}: {chunk_path.name}", icon=None)

            result = run_uploaded_audio_workflow(
                uploaded_file.name,
                uploaded_file.getvalue(),
                effective_settings,
                options,
                status_callback=status,
                chunk_callback=chunk_status,
            )
            progress.progress(1.0)
            status_box.success("Done")
            st.session_state["last_result"] = str(result.task_dir)
            render_results(result.task_dir)
        except Exception as exc:
            progress.progress(0)
            title, detail = classify_error(exc)
            status_box.error(title)
            detail_box.error(detail)


def render_results(task_dir: Path) -> None:
    st.subheader("Results")
    st.caption(f"Task folder: `{task_dir}`")

    transcript_path = task_dir / "transcript.md"
    notes_path = task_dir / "notes_input.md"
    minutes_path = task_dir / "meeting_minutes.md"

    if transcript_path.exists():
        preview, truncated = read_preview(transcript_path)
        label = "Transcript preview"
        if truncated:
            label += " (truncated)"
        with st.expander(label, expanded=True):
            st.markdown(preview)

    download_cols = st.columns(2)
    with download_cols[0]:
        render_download(transcript_path, "Download transcript.md")
        render_download(notes_path, "Download notes_input.md")
    with download_cols[1]:
        render_download(minutes_path, "Download meeting_minutes.md")
        zip_bytes = build_text_outputs_zip(task_dir)
        if zip_bytes:
            st.download_button(
                "Download text outputs zip",
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

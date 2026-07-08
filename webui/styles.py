from __future__ import annotations

import streamlit as st


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

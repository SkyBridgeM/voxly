from __future__ import annotations

from pathlib import Path

import streamlit as st

from meeting_asr.config import Settings
from webui.ui_helpers import format_detected, update_env_value


def render_mimo_key_section(settings: Settings, root: Path) -> str:
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
            update_env_value(root / ".env", "MIMO_API_KEY", key_to_save)
            st.success("已将 MIMO_API_KEY 保存到 .env。")
            st.rerun()
    return mimo_override


def render_deepseek_key_section(settings: Settings, root: Path) -> str:
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
            update_env_value(root / ".env", "DEEPSEEK_API_KEY", key_to_save)
            st.success("已将 DEEPSEEK_API_KEY 保存到 .env。")
            st.rerun()
    return ds_override

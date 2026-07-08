from __future__ import annotations

from dataclasses import dataclass
from pathlib import Path

import streamlit as st

from meeting_asr.config import Settings
from webui.ui_helpers import update_env_value

from .api_keys import render_deepseek_key_section, render_mimo_key_section


@dataclass(frozen=True)
class SidebarState:
    minutes_mode: bool = False
    selected_model: str = "deepseek-v4-flash"
    mimo_override: str = ""
    ds_override: str = ""


def render_sidebar(settings: Settings, root: Path) -> SidebarState:
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

    selected_model = settings.deepseek_model

    if minutes_mode:
        st.markdown("---")
        st.subheader("LLM 总结设置")
        st.selectbox("Provider", options=["DeepSeek"], index=0, disabled=True)
        model_label = st.selectbox(
            "模型",
            options=["DeepSeek V4 Flash（更快/更省）", "DeepSeek V4 Pro（质量更高）"],
            index=1 if settings.deepseek_model == "deepseek-v4-pro" else 0,
        )
        selected_model = "deepseek-v4-flash" if "Flash" in model_label else "deepseek-v4-pro"

        if st.button("保存为默认总结服务商", use_container_width=True):
            update_env_value(root / ".env", "MINUTES_PROVIDER", "deepseek")
            update_env_value(root / ".env", "DEEPSEEK_MODEL", selected_model)
            st.success("已保存默认总结服务商配置到 .env。")
            st.rerun()

    # ── API keys (collapsed by default) ──
    st.markdown("---")
    with st.expander("API keys", expanded=False):
        st.caption("检测状态与临时覆盖（不显示明文）")
        mimo_override = render_mimo_key_section(settings, root)
        st.markdown("---")
        ds_override = render_deepseek_key_section(settings, root)

    return SidebarState(
        minutes_mode=minutes_mode,
        selected_model=selected_model,
        mimo_override=mimo_override,
        ds_override=ds_override,
    )

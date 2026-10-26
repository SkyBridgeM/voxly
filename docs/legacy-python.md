# 旧版 Python 退役说明

旧 Python CLI、Streamlit WebUI、配套启动脚本和 Python/Go 对照工具已从当前源码移除。桌面开发与正式版统一使用 Go，不再提供 Python 回退。

需要查看旧实现时，可查阅 Git 历史中的 `f2a059a`。安装和开发入口见[项目首页](../README.md)与[桌面开发说明](../desktop/README.md)。

这次清理不删除已有任务、录音、模型、虚拟环境或安装包。旧 Python 任务缺少 Go manifest，无法直接用桌面的恢复功能继续运行；已有文字文件仍可单独打开。

Python 仍用于第三方许可证收集、打包辅助和独立 VAD 验证，不进入用户安装包。`prompts/` 仍被 Go CLI 的兼容模式使用，因此保留。

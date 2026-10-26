# Voxly · 声笺

[English](README.en.md)

把访谈、会议录音转成文字，留在本机整理。Voxly 支持本地 SenseVoice 转写，也可以使用 MiMo 云端识别，再按需要用 DeepSeek 生成摘要、纪要或行动项。

当前版本为 `0.1.0-alpha`，提供 M 系列 Mac 的安装包。它还是测试版，尚未做 Apple Developer ID 签名和公证。

## 安装

从 [GitHub Releases](https://github.com/SkyBridgeM/voxly/releases/tag/v0.1.0-alpha) 下载 `Voxly_0.1.0-alpha_macos_arm64.dmg`，打开后将 Voxly 拖入 Applications。安装包已包含 FFmpeg 9.0.2、Go 处理核心和本地识别运行时，不需要另装 Homebrew、Python 或 Go。

macOS 可能拦截这个未公证的测试版。请核对下载来源和 Release 中的 SHA-256 校验值。当前安装包只验证了 Apple Silicon Mac，尚无 Intel 或 Windows 发行包。

## 第一次转写

1. 打开设置，在“模型与连接”中下载或导入 SenseVoice 模型。模型约 156 MiB，首次下载需要联网，安装包不包含这部分权重。
2. 在“识别与生成”中选择本地语音识别。中文录音建议保留默认中文；多语种录音可选自动判断。
3. 需要按讲话停顿分段时，勾选 VAD 并保存。这个选择用于新任务；已有任务恢复时沿用自己的分段计划和语言。
4. 导入 M4A、MP3 或 WAV，选择“转写”并开始。完成后在任务库打开结果。

M4A 可以直接导入。Voxly 会在本地解码，不用先手动转成 WAV。长录音按片段处理，已完成的片段可以在取消后恢复时复用。

## 文字与音频怎么保存

每次导入都会建立独立任务文件夹，保存录音副本、全文、处理状态和生成内容。开启 VAD 的任务还会保存带时间转写和分段 JSON；时间指讲话片段的起止位置，不是逐字时间戳。

在“设置 → 存储”中可以查看空间占用和批量清理音频切片。自动清理切片默认关闭，开启后只处理此后成功完成的任务。失败或中断任务会保留音频，以便继续。

如果检查完转写后只想留下文字，进入“任务库 → 任务详情 → 音频与缓存”，选择“仅保留文本”。预览会列出待删除文件和预计回收空间。确认后，Voxly 删除任务内的录音副本与切片，保留全文、时间结果、生成版本和任务信息。它不会删除你导入的外部原文件。此操作无法撤销；回听或重新识别需要另外保留录音。

## 哪些内容会上传

| 选择 | 会发送给服务商的内容 |
| --- | --- |
| 本地 SenseVoice 转写 | 不上传音频，无需 API Key；模型下载需联网 |
| MiMo 识别 | 音频发送给 MiMo，需要凭据 |
| DeepSeek 内容生成 | 转写文字发送给 DeepSeek，需要凭据 |

只选择本地转写时，识别在本机完成。选择云端内容生成后，转写文字仍会上传。生产版使用会话凭据或系统安全存储，不读取项目根目录的 `.env`。

## 目前的限制

Voxly 处理已经录好的文件，尚不支持桌面麦克风录音或流式识别。它也不会区分说话人；CAM++ 目前只预留了代码接口。

转写可能出现人名、术语和短句误识别。删除录音前请检查重要内容。我们已验证本机短样音转写和安装包依赖，但尚未完成另一台全新 Mac 的试装。

## 开发

桌面使用 Tauri、React 和 Go，原生本地识别适配器使用 sherpa-onnx。先安装 Node.js/npm、Rust、Go、Python 3.11+（构建工具）和 Xcode Command Line Tools，再从仓库根目录运行：

```bash
npm --prefix desktop ci
./scripts/build_desktop_sidecar.sh
go build -C core-go -o meeting-asr-go .
npm --prefix desktop run tauri -- dev
```

首次构建会下载固定版本的原生运行时和 FFmpeg 源码。模型在应用内单独安装。构建安装包使用 `./scripts/build_macos_release.sh`；该脚本只生成文件，不自动提交、推送或发布。

- [桌面开发与打包](desktop/README.md)
- [Go 核心与 CLI](core-go/README.md)
- [本地模型管理](docs/architecture/local-model-management.md)
- [本版更新说明](docs/releases/0.1.0-alpha.md)
- [旧版 Python 退役说明](docs/legacy-python.md)

旧 Python 后端和 WebUI 已退役，桌面开发与正式版统一使用 Go。Python 仅用于构建和独立 VAD 验证工具。带日期的计划和审计记录当时的状态；当前功能以本说明和版本更新说明为准。

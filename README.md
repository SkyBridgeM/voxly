<p align="center">
  <img src="desktop/src/assets/voxly-app-icon.png" width="96" alt="Voxly 图标" />
</p>

<h1 align="center">Voxly · 声笺</h1>
<p align="center"><strong>录音变成文字，整理留在本机。</strong></p>
<p align="center">给访谈、会议和长录音一个好找、好读、能接着处理的归处。</p>

<p align="center">
  <a href="https://github.com/SkyBridgeM/voxly/releases/tag/v0.1.0-alpha"><img alt="版本 0.1.0-alpha" src="https://img.shields.io/badge/version-0.1.0--alpha-52685A" /></a>
  <img alt="macOS Apple Silicon" src="https://img.shields.io/badge/macOS-Apple%20Silicon-343A40" />
  <a href="LICENSE"><img alt="MIT 许可证" src="https://img.shields.io/badge/license-MIT-52685A" /></a>
</p>

<p align="center">
  <a href="https://github.com/SkyBridgeM/voxly/releases/tag/v0.1.0-alpha">下载 Mac 测试版</a> ·
  <a href="#第一次使用">开始转写</a> ·
  <a href="desktop/README.md">开发文档</a> ·
  <a href="README.en.md">English</a>
</p>

---

一场访谈录了一个小时，回头找一句话却要从头听。Voxly 把录音转成可阅读的文字，按讲话停顿分段，再放进任务库。需要摘要、纪要或行动项时，可以继续生成；核对完内容，也可以清掉任务里的音频，只留下文字。

你可以在 Mac 上使用 SenseVoice 本地识别，也可以选择 MiMo 云端转写。音频和转写文字是否上传，取决于你选的处理方式。

## 能做什么

| | 用起来是什么样 |
| --- | --- |
| **本地转写** | 安装 SenseVoice 后在本机识别，无需转写 API Key。默认中文，也可自动判断语言。 |
| **按讲话停顿分段** | 可选 VAD 检测讲话区间，输出带时间的段落；较长的讲话仍会继续切分。 |
| **长录音接着处理** | 分段保存进度，中断后恢复可复用已完成片段。 |
| **摘要、纪要与行动项** | 转写后按模板调用 DeepSeek 生成内容，保留生成版本。 |
| **任务库** | 集中查看任务、转写和生成结果，每个任务都有独立文件夹。 |
| **只留下文字** | 预览占用后清理任务内的录音副本与切片，保留文字、时间信息和生成结果。 |
| **M4A 直接导入** | 支持 M4A、MP3、WAV；已内置 FFmpeg，无需自己转格式。 |

> 当前为 `0.1.0-alpha`，提供 Apple Silicon（M 系列）Mac 安装包。尚未做 Apple Developer ID 签名和公证。

## 下载与安装

**[前往 Releases 下载 DMG →](https://github.com/SkyBridgeM/voxly/releases/tag/v0.1.0-alpha)**

打开 `Voxly_0.1.0-alpha_macos_arm64.dmg`，将 Voxly 拖入 Applications。安装包已包含 FFmpeg 9.0.2、Go 核心和本地识别运行时，使用时不需要安装 Homebrew、Python 或 Go。

- **本地模型单独安装**：SenseVoice 约 156 MiB，可在应用内下载或导入，权重不包含在 DMG 中。
- **macOS 可能拦截测试版**：请核对下载来源和 Release 中的 SHA-256 校验值。系统阻止打开时，可通过 macOS“系统设置 → 隐私与安全性”查看并处理提示。
- **当前发行范围**：仅验证 Apple Silicon Mac，尚无 Intel Mac 或 Windows 安装包；另一台全新 Mac 的试装仍待验证。

## 第一次使用

1. **装好模型**：打开“设置 → 模型与连接”，下载或导入 SenseVoice。
2. **选择本地识别**：在“识别与生成”中选择本地语音识别。中文录音保留默认中文，多语种录音可选自动判断。
3. **设置分段方式**：需要按讲话停顿分段时，开启 VAD 并保存。新任务使用新设置；恢复任务沿用原来的语言和分段计划。
4. **导入并开始**：选择 M4A、MP3 或 WAV 文件，选择“转写”，完成后在任务库打开结果。

只想拿到全文，选择转写即可。需要整理内容时，再配置 DeepSeek 凭据并选择生成模板。

## 长录音，怎么少占空间

Voxly 会为每个任务保存录音副本、处理状态、全文和生成内容。开启 VAD 后，还会保存带时间转写及分段 JSON。这里的时间对应讲话片段，尚不是逐字时间戳。

在“设置 → 存储”中可以查看占用、批量清理切片，也可以开启自动清理切片。自动清理默认关闭；开启后只处理此后成功完成的任务，中断和失败任务会保留音频以便继续。

**确认文字无误后，可在“任务库 → 任务详情 → 音频与缓存”选择“仅保留文本”。** 先查看待删除文件和预计回收空间，再确认清理。任务内的录音副本与切片会被删除，全文、时间结果、生成版本和任务信息会保留。

清理不会删除你导入的外部原文件，也不会自动在转写后删掉全部音频。“仅保留文本”不可撤销；以后需要回听或重新识别，请另存录音。

## 数据会去哪里

| 你选择的功能 | 处理位置与上传内容 |
| --- | --- |
| **SenseVoice 本地转写** | 本机识别，不上传音频，无需 API Key；首次下载模型需联网。 |
| **MiMo 云端识别** | 音频发送给 MiMo，需要凭据。 |
| **DeepSeek 内容生成** | 转写文字发送给 DeepSeek，需要凭据。 |

本地转写之后再选云端生成，文字仍会离开本机。正式版从会话或系统安全存储读取凭据，不读取项目根目录的 `.env`。

## 当前边界

- 处理已经录好的文件，尚无桌面麦克风录音或流式识别。
- 尚不区分说话人；CAM++ 只预留了代码接口。
- 人名、专业术语和短句可能识别有误，重要内容需要核对。

已完成本机短样音转写和安装包依赖检查。测试版能力和限制见[版本说明](docs/releases/0.1.0-alpha.md)。

## 从源码运行

桌面使用 **Tauri + React + Go**，本地识别通过原生 sherpa-onnx 适配器运行。准备 Node.js/npm、Rust、Go、Python 3.11+（构建工具）和 Xcode Command Line Tools，然后在仓库根目录执行：

```bash
npm --prefix desktop ci
./scripts/build_desktop_sidecar.sh
go build -C core-go -o meeting-asr-go .
npm --prefix desktop run tauri -- dev
```

首次构建会下载固定版本的原生依赖和 FFmpeg 源码。模型在应用内单独安装。生成安装包使用 `./scripts/build_macos_release.sh`；脚本只生成文件，不自动提交、推送或发布。

| 想了解什么 | 文档 |
| --- | --- |
| 桌面开发、检查与打包 | [Desktop guide](desktop/README.md) |
| Go 命令行与处理核心 | [Go core](core-go/README.md) |
| 模型安装与运行时约定 | [本地模型管理](docs/architecture/local-model-management.md) |
| 旧 Python 实现的去向 | [退役说明](docs/legacy-python.md) |

欢迎提交问题和改进。描述问题时请附应用版本、macOS 版本、复现步骤和脱敏日志，不要上传 API Key 或私人录音。带日期的计划和审计保留当时的状态，当前功能以首页和版本说明为准。

## 许可证与致谢

Voxly 自有代码采用 **[MIT License](LICENSE)**。第三方组件和模型沿用各自的许可证，不因本项目采用 MIT 而改变。

感谢 SenseVoice、sherpa-onnx、Silero VAD、FFmpeg，以及 Tauri、React、Go 生态提供的基础。第三方声明见 [THIRD-PARTY-NOTICES](native/THIRD-PARTY-NOTICES.md)。发行包附带相应声明；FFmpeg/LAME 对应源码与构建说明随 Release 提供。

export type Locale = "zh" | "en";

export interface Translations {
  appTitle: string;
  appSubtitle: string;
  appKicker: string;
  aboutTitle: string;
  aboutProductNames: string;
  aboutSubtitles: string;
  aboutVersion: string;
  aboutDescription: string;
  workflowEyebrow: string;
  workflowTitle: string;
  workflowDescription: string;
  inputTitle: string;
  outputModeTitle: string;
  chooseFile: string;
  noFileSelected: string;
  audioFile: string;
  modeTranscript: string;
  modeTranscriptDesc: string;
  modeNotes: string;
  modeNotesDesc: string;
  modeMinutes: string;
  modeMinutesDesc: string;
  modeGenerate: string;
  modeGenerateDesc: string;
  templateLabel: string;
  start: string;
  processing: string;
  openOutputFolder: string;
  splitting: string;
  transcribing: string;
  merging: string;
  preparingNotes: string;
  generatingMinutes: string;
  done: string;
  resultsTitle: string;
  resultsSummary: string;
  primaryOutput: string;
  generatedFiles: string;
  taskFolder: string;
  openTaskFolder: string;
  advancedDetails: string;
  folder: string;
  transcript: string;
  notesInput: string;
  meetingMinutes: string;
  completedSuccessfully: string;
  exitedWithCode: string;
  terminatedBySignal: string;
  transcriptWarning: string;
  notesWarning: string;
  llmWarning: string;
  backend: string;
  backendPython: string;
  backendGoExperimental: string;
  backendGoProduction: string;
  settingsTitle: string;
  language: string;
  mimoApiKey: string;
  deepseekApiKey: string;
  deepseekModel: string;
  save: string;
  cancel: string;
  lightMode: string;
  darkMode: string;
  errorTitle: string;
  recordingRuntimeFailed: string;
  close: string;
  showLogs: string;
  hideLogs: string;
  keysNotPersisted: string;
  saveToEnv: string;
  productionCredentialsNote: string;
  developmentCredentialPlaceholder: string;
  productionCredentialPlaceholder: string;
  savedSuccessfully: string;
  saveFailed: string;
  readinessTitle: string;
  readinessLocalOnly: string;
  refresh: string;
  checking: string;
  ready: string;
  notReady: string;
  runtimeMode: string;
  development: string;
  production: string;
  sidecar: string;
  outputRoot: string;
  missing: string;
  session: string;
  secureStore: string;
  developmentEnv: string;
  preflightUnavailable: string;
  preflightPrompt: string;
  chooseFileToBegin: string;
  chooseFileToBeginDesc: string;
  readyToRun: string;
  readyToRunDesc: string;
  settingsIntro: string;
  settingsReadiness: string;
  settingsReadinessDesc: string;
  settingsAppearance: string;
  settingsAppearanceDesc: string;
  settingsBackend: string;
  settingsBackendDesc: string;
  settingsCredentials: string;
  settingsCredentialsDesc: string;
  settingsMinutesModel: string;
  settingsMinutesModelDesc: string;
  settingsOutputLocation: string;
  settingsOutputLocationDesc: string;
  currentOutputFolder: string;
  changeOutputFolder: string;
  openOutputFolderAction: string;
  resetOutputFolder: string;
  outputFolderChanged: string;
  outputFolderReset: string;
  outputFolderMoveWarning: string;
  outputFolderChangeBlocked: string;
  outputFolderUnavailable: string;
  outputFolderNotWritable: string;
  chooseOutputFolder: string;
  theme: string;
  themeSystem: string;
  themeLight: string;
  themeDark: string;
  workspaceNavigation: string;
  newTask: string;
  taskLibraryTitle: string;
  taskLibraryDescription: string;
  refreshing: string;
  taskLibraryLoading: string;
  taskLibraryEmpty: string;
  taskLibraryEmptyDescription: string;
  taskLibraryLoadError: string;
  retry: string;
  partialErrors: string;
  discoveryIssues: string;
  updatedAt: string;
  createdAt: string;
  outputMode: string;
  outputs: string;
  tags: string;
  editMetadata: string;
  taskTitle: string;
  taskDescription: string;
  descriptionPlaceholder: string;
  addTag: string;
  tagPlaceholder: string;
  removeTag: string;
  metadataSaveFailed: string;
  sortBy: string;
  sortUpdated: string;
  sortCreated: string;
  sortTitle: string;
  filterByTag: string;
  allTasks: string;
  taskDetails: string;
  openTaskDetails: string;
  backToTaskLibrary: string;
  backToContent: string;
  transcriptSummary: string;
  generatedArtifactCount: string;
  saveTitle: string;
  cancelTitleEdit: string;
  taskInformation: string;
  existingTags: string;
  taskDocuments: string;
  markdownLoading: string;
  markdownLoadError: string;
  markdownEmpty: string;
  noReadableMarkdown: string;
  cardGeneratedContent: string;
  cardTranscriptOnly: string;
  cardNoContent: string;
  available: string;
  unavailable: string;
  artifactError: string;
  retryable: string;
  notRetryable: string;
  invalidDate: string;
  statusRunning: string;
  statusCompleted: string;
  statusCompletedWithErrors: string;
  statusFailed: string;
  statusInterrupted: string;
  statusPending: string;
  statusProcessing: string;
  statusUnavailable: string;
  statusUnknown: string;
  artifactTranscript: string;
  artifactNotes: string;
  artifactMinutes: string;
  openArtifact: string;
  revealArtifact: string;
  openingArtifact: string;
  revealingArtifact: string;
  artifactActionFailed: string;
  refreshLibraryHint: string;
  canonicalTranscript: string;
  generatedContent: string;
  generationHistory: string;
  generateContent: string;
  generateAgain: string;
  generatingContent: string;
  generationFailed: string;
  latestVersion: string;
  legacyArtifact: string;
  settingsProviders: string;
	localModelTitle: string; localModelDescription: string; localChineseFirst: string; localModelStatus: string;
	localNotInstalled: string; localRuntimeMissing: string; localModelInvalid: string; localUnsupported: string;
	localDownload: string; localImport: string; localCancel: string; localDelete: string; localSource: string; localSize: string;
	localAdvanced: string; localEndpoint: string; localTestEndpoint: string; localEndpointReachable: string;
	localOperationFailed: string; localConfirmDelete: string; localPrivacyTitle: string; localPrivacyBody: string;
	localEndpointScope: string;
  settingsProvidersDesc: string;
  settingsTemplates: string;
  settingsTemplatesDesc: string;
  builtInTemplates: string;
  myTemplates: string;
  newTemplate: string;
  editTemplate: string;
  deleteTemplate: string;
  templateName: string;
  templateDescription: string;
  templateInstructions: string;
  technicalDetails: string;
  revision: string;
  saveCreatesRevision: string;
  confirmDeleteTemplate: string;
  templateSaveFailed: string;
  noCustomTemplates: string;
  speechRecognition: string;
  generatedContentProvider: string;
  provider: string;
  model: string;
  speechRecognitionProvider: string;
  generatedWithProvider: string;
  providerUnknown: string;
  providerNotReady: string;
  openSettings: string;
}

export const translations: Record<Locale, Translations> = {
  zh: {
    appTitle: "声笺",
    appSubtitle: "本地语音工作台",
    appKicker: "Voxly",
    aboutTitle: "关于声笺",
    aboutProductNames: "Voxly · 声笺",
    aboutSubtitles: "Local Voice Workspace · 本地语音工作台",
    aboutVersion: "版本 0.1.0-alpha",
    aboutDescription: "将语音转写为可整理、可生成、可追溯的内容。",
    workflowEyebrow: "新建任务",
    workflowTitle: "从语音，到可用内容",
    workflowDescription: "录制或导入音频，转写并生成你需要的内容。",
    inputTitle: "音频来源",
    outputModeTitle: "输出类型",
    chooseFile: "选择音频文件",
    noFileSelected: "未选择文件",
    audioFile: "音频文件",
    modeTranscript: "转写",
    modeTranscriptDesc: "音频 → 转写文本",
    modeNotes: "笔记准备",
    modeNotesDesc: "转写 + notes_input.md",
    modeMinutes: "会议纪要",
    modeMinutesDesc: "转写 + AI 纪要",
    modeGenerate: "转写 + 生成内容",
    modeGenerateDesc: "先完成转写，再用内置模板生成内容",
    templateLabel: "内容模板",
    start: "开始",
    processing: "处理中...",
    openOutputFolder: "打开任务文件夹",
    splitting: "分割音频...",
    transcribing: "转写中...",
    merging: "合并转写...",
    preparingNotes: "准备笔记...",
    generatingMinutes: "生成会议纪要...",
    done: "完成",
    resultsTitle: "结果",
    resultsSummary: "你的文件已准备就绪",
    primaryOutput: "主要输出",
    generatedFiles: "同时生成的文件",
    taskFolder: "任务文件夹",
    openTaskFolder: "打开任务文件夹",
    advancedDetails: "任务文件夹详情",
    folder: "任务文件夹",
    transcript: "转写文本",
    notesInput: "笔记准备文件",
    meetingMinutes: "会议纪要",
    completedSuccessfully: "处理完成",
    exitedWithCode: "退出码",
    terminatedBySignal: "被信号终止",
    transcriptWarning: "音频会发送至 MiMo ASR 进行转写。不会调用内容生成模型。",
    notesWarning:
      "notes 模式会将音频发送至 MiMo ASR 转写，并在本地生成 notes_input.md；不会将转写文本发送至 DeepSeek。\n\nNotes mode sends audio to MiMo ASR and generates notes_input.md locally. No transcript is sent to DeepSeek.",
    llmWarning: "音频将发送至 MiMo ASR，完成后的转写文本将发送至 DeepSeek，并按所选模板生成内容。",
    backend: "处理后端",
    backendPython: "Python（开发备用）",
    backendGoExperimental: "Go（开发默认 / 推荐）",
    backendGoProduction: "Go（生产模式内置）",
    settingsTitle: "设置",
    language: "语言",
    mimoApiKey: "MIMO API Key",
    deepseekApiKey: "DeepSeek API Key",
    deepseekModel: "DeepSeek 模型",
    save: "保存",
    cancel: "取消",
    lightMode: "浅色模式",
    darkMode: "深色模式",
    errorTitle: "错误",
    recordingRuntimeFailed: "无法初始化录音任务。请重启 Voxly；如果正在运行开发版，请重新构建 Go 后端后再试。",
    close: "关闭",
    showLogs: "显示日志",
    hideLogs: "隐藏日志",
    keysNotPersisted: "此处输入的 Key 仅在本次会话有效。开发模式可保存到项目根 .env，也可使用已有安全存储凭据。",
    saveToEnv: "保存到 .env",
    productionCredentialsNote: "生产模式不读取项目根 .env；请使用本次会话 Key 或安全存储。",
    developmentCredentialPlaceholder: "可选会话 Key；留空则使用安全存储或开发 .env",
    productionCredentialPlaceholder: "可选会话 Key；留空则使用安全存储",
    savedSuccessfully: "已保存",
    saveFailed: "保存失败",
    readinessTitle: "本机运行就绪状态",
    readinessLocalOnly: "仅检查本机配置，不调用 MiMo 或 DeepSeek",
    refresh: "刷新",
    checking: "检查中…",
    ready: "就绪",
    notReady: "未就绪",
    runtimeMode: "运行模式",
    development: "开发",
    production: "生产",
    sidecar: "Go 后端可执行文件",
    outputRoot: "输出位置",
    missing: "未配置",
    session: "本次会话",
    secureStore: "安全存储",
    developmentEnv: "开发 .env",
    preflightUnavailable: "运行就绪检查暂不可用。",
    preflightPrompt: "刷新后查看本机运行环境与配置状态。",
    chooseFileToBegin: "选择一种语音输入方式",
    chooseFileToBeginDesc: "开始录音，或导入已有音频，然后选择所需的处理方式。",
    readyToRun: "已准备好开始",
    readyToRunDesc: "音频已选择。确认输出类型后即可开始处理。",
    settingsIntro: "检查本机运行状态，并调整界面、处理后端与会话配置。",
    settingsReadiness: "本机就绪状态",
    settingsReadinessDesc: "检查运行模式、后端和系统依赖，不发起在线请求。",
    settingsAppearance: "外观与语言",
    settingsAppearanceDesc: "调整界面语言和主题偏好。",
    settingsBackend: "处理后端",
    settingsBackendDesc: "生产模式固定使用内置 Go；开发模式默认使用 Go，也可手动选择 Python 备用。",
    settingsCredentials: "凭据",
    settingsCredentialsDesc: "会话 Key 不会显示在状态信息、日志或输出文件中。",
    settingsMinutesModel: "内容生成模型",
    settingsMinutesModelDesc: "DeepSeek 模型用于按所选模板生成结构化内容。",
    settingsOutputLocation: "输出位置",
    settingsOutputLocationDesc: "所有任务文件都会保存在已配置输出位置之内。",
    currentOutputFolder: "当前文件夹",
    changeOutputFolder: "更改文件夹…",
    openOutputFolderAction: "打开文件夹",
    resetOutputFolder: "恢复默认",
    outputFolderChanged: "输出文件夹已更改。",
    outputFolderReset: "输出位置已恢复为默认设置。",
    outputFolderMoveWarning: "更改输出文件夹不会移动现有任务。",
    outputFolderChangeBlocked: "处理任务时无法更改输出文件夹。",
    outputFolderUnavailable: "输出文件夹不可用。",
    outputFolderNotWritable: "输出文件夹不可写。",
    chooseOutputFolder: "选择输出文件夹",
    theme: "主题",
    themeSystem: "跟随系统",
    themeLight: "浅色",
    themeDark: "深色",
    workspaceNavigation: "工作区导航",
    newTask: "新建任务",
    taskLibraryTitle: "任务库",
    taskLibraryDescription: "查看最近处理的任务、状态和主要输出。",
    refreshing: "刷新中…",
    taskLibraryLoading: "正在载入任务库…",
    taskLibraryEmpty: "还没有任务",
    taskLibraryEmptyDescription: "完成音频处理后，任务会显示在这里。",
    taskLibraryLoadError: "暂时无法载入任务库。",
    retry: "重试",
    partialErrors: "部分输出有问题",
    discoveryIssues: "部分任务无法读取",
    updatedAt: "更新",
    createdAt: "创建",
    outputMode: "输出模式",
    outputs: "主要输出",
    tags: "标签",
    editMetadata: "编辑任务信息",
    taskTitle: "任务名称",
    taskDescription: "描述",
    descriptionPlaceholder: "添加任务背景、重点或备注（可选）",
    addTag: "添加",
    tagPlaceholder: "输入标签",
    removeTag: "移除标签",
    metadataSaveFailed: "无法保存任务信息，请重试。",
    sortBy: "排序",
    sortUpdated: "最近更新",
    sortCreated: "最近创建",
    sortTitle: "名称",
    filterByTag: "标签筛选",
    allTasks: "全部任务",
    taskDetails: "任务详情",
    openTaskDetails: "打开任务详情",
    backToTaskLibrary: "返回任务库",
    backToContent: "返回内容",
    transcriptSummary: "转写",
    generatedArtifactCount: "生成内容",
    saveTitle: "保存任务名称",
    cancelTitleEdit: "取消编辑任务名称",
    taskInformation: "任务信息",
    existingTags: "已有标签",
    taskDocuments: "任务文档",
    markdownLoading: "正在载入文档…",
    markdownLoadError: "无法载入此文档，请重试。",
    markdownEmpty: "此文档暂时没有内容。",
    noReadableMarkdown: "此任务还没有可阅读的 Markdown 内容。",
    cardGeneratedContent: "查看最新生成内容",
    cardTranscriptOnly: "查看转写文本",
    cardNoContent: "查看任务状态",
    available: "可用",
    unavailable: "不可用",
    artifactError: "生成出错",
    retryable: "可重试",
    notRetryable: "不可重试",
    invalidDate: "时间未知",
    statusRunning: "运行中",
    statusCompleted: "已完成",
    statusCompletedWithErrors: "完成但有错误",
    statusFailed: "失败",
    statusInterrupted: "已中断",
    statusPending: "等待中",
    statusProcessing: "处理中",
    statusUnavailable: "不可用",
    statusUnknown: "未知状态",
    artifactTranscript: "转写文本",
    artifactNotes: "笔记准备",
    artifactMinutes: "会议纪要",
    openArtifact: "打开",
    revealArtifact: "在访达中显示",
    openingArtifact: "正在打开…",
    revealingArtifact: "正在定位…",
    artifactActionFailed: "无法访问此文件。",
    refreshLibraryHint: "请刷新任务库后重试。",
    canonicalTranscript: "规范转写",
    generatedContent: "生成内容",
    generationHistory: "历史版本",
    generateContent: "生成内容",
    generateAgain: "再次生成",
    generatingContent: "生成中…",
    generationFailed: "生成失败；已有转写和历史版本保持不变。",
    latestVersion: "最新成功版本",
    legacyArtifact: "旧版输出",
    settingsProviders: "服务商与模型",
	localModelTitle: "本地语音识别", localModelDescription: "管理声笺应用数据中的官方 SenseVoice 模型。模型安装不会写入任务目录。", localChineseFirst: "中文为主", localModelStatus: "状态",
	localNotInstalled: "未安装", localRuntimeMissing: "模型已安装，运行时待提供", localModelInvalid: "模型无效，请重新安装", localUnsupported: "当前平台暂不支持",
	localDownload: "下载模型", localImport: "导入官方模型", localCancel: "取消", localDelete: "删除模型", localSource: "官方来源", localSize: "下载大小",
	localAdvanced: "高级下载设置", localEndpoint: "Hugging Face Endpoint", localTestEndpoint: "测试连接", localEndpointReachable: "连接可用", localOperationFailed: "本地模型操作失败，现有模型未被修改。", localConfirmDelete: "删除已安装的 SenseVoice 模型？任务和历史记录不会被删除。", localPrivacyTitle: "隐私：", localPrivacyBody: "选择本地语音识别时，原始音频不会发送到云端。若使用云端内容生成，转写文本仍会发送给所选服务。",
	localEndpointScope: "仅适用于未来由 Hugging Face 托管且目录声明兼容的模型。当前 SenseVoice 固定从 k2-fsa GitHub Release 下载；此设置和连接测试不会改变或测试 SenseVoice 下载源。",
    settingsProvidersDesc: "选择语音识别和生成内容使用的内置服务商与模型。密钥仍单独安全保存。",
    settingsTemplates: "内容模板",
    settingsTemplatesDesc: "管理本机自定义提示词模板；内置模板保持只读。",
    builtInTemplates: "内置模板",
    myTemplates: "我的模板",
    newTemplate: "新建模板",
    editTemplate: "编辑",
    deleteTemplate: "删除",
    templateName: "名称",
    templateDescription: "描述",
    templateInstructions: "提示词指令",
    technicalDetails: "技术详情",
    revision: "修订",
    saveCreatesRevision: "保存更改不会影响之前生成的内容。",
    confirmDeleteTemplate: "删除此模板？历史生成内容会保留，但该模板将不再可选。",
    templateSaveFailed: "无法保存模板，请刷新后重试。",
    noCustomTemplates: "还没有自定义模板。",
    speechRecognition: "语音识别",
    generatedContentProvider: "生成内容",
    provider: "服务商",
    model: "模型",
    speechRecognitionProvider: "语音识别",
    generatedWithProvider: "生成服务",
    providerUnknown: "旧版 / 未知",
    providerNotReady: "当前服务配置尚未就绪。",
    openSettings: "打开设置",
  },
  en: {
    appTitle: "Voxly",
    appSubtitle: "Local Voice Workspace",
    appKicker: "声笺",
    aboutTitle: "About Voxly",
    aboutProductNames: "Voxly · 声笺",
    aboutSubtitles: "Local Voice Workspace · 本地语音工作台",
    aboutVersion: "Version 0.1.0-alpha",
    aboutDescription: "Turn voice into structured, traceable content.",
    workflowEyebrow: "New task",
    workflowTitle: "From voice to useful content",
    workflowDescription: "Record or import audio, transcribe it, and generate the content you need.",
    inputTitle: "Audio source",
    outputModeTitle: "Output type",
    chooseFile: "Choose Audio File",
    noFileSelected: "No file selected",
    audioFile: "Audio file",
    modeTranscript: "Transcript",
    modeTranscriptDesc: "Audio → transcript",
    modeNotes: "Notes preparation",
    modeNotesDesc: "Transcript + notes_input.md",
    modeMinutes: "Meeting minutes",
    modeMinutesDesc: "Transcript + AI minutes",
    modeGenerate: "Transcript + generated content",
    modeGenerateDesc: "Complete the transcript, then generate from a built-in template",
    templateLabel: "Content template",
    start: "Start",
    processing: "Processing...",
    openOutputFolder: "Open Task Folder",
    splitting: "Splitting audio...",
    transcribing: "Transcribing...",
    merging: "Merging transcript...",
    preparingNotes: "Preparing notes...",
    generatingMinutes: "Generating minutes...",
    done: "Done",
    resultsTitle: "Results",
    resultsSummary: "Your files are ready",
    primaryOutput: "Primary output",
    generatedFiles: "Also generated",
    taskFolder: "Task folder",
    openTaskFolder: "Open Task Folder",
    advancedDetails: "Task folder details",
    folder: "Task folder",
    transcript: "Transcript",
    notesInput: "Notes preparation",
    meetingMinutes: "Meeting minutes",
    completedSuccessfully: "Completed successfully",
    exitedWithCode: "Exited with code",
    terminatedBySignal: "Terminated by signal",
    transcriptWarning: "Audio is sent to MiMo ASR for transcription. No content-generation model is called.",
    notesWarning:
      "Notes mode sends audio to MiMo ASR and generates notes_input.md locally. No transcript is sent to DeepSeek.",
    llmWarning: "Audio is sent to MiMo ASR. The completed transcript is then sent to DeepSeek using the selected template.",
    backend: "Processing backend",
    backendPython: "Python (development fallback)",
    backendGoExperimental: "Go (development default / recommended)",
    backendGoProduction: "Go (packaged for production)",
    settingsTitle: "Settings",
    language: "Language",
    mimoApiKey: "MIMO API Key",
    deepseekApiKey: "DeepSeek API Key",
    deepseekModel: "DeepSeek Model",
    save: "Save",
    cancel: "Cancel",
    lightMode: "Light Mode",
    darkMode: "Dark Mode",
    errorTitle: "Error",
    recordingRuntimeFailed: "The recording task could not be initialized. Restart Voxly; for a development build, rebuild the Go backend and try again.",
    close: "Close",
    showLogs: "Show Logs",
    hideLogs: "Hide Logs",
    keysNotPersisted: "Keys entered here are session-only. Development can save to the project-root .env or use an existing secure-store credential.",
    saveToEnv: "Save to .env",
    productionCredentialsNote: "Production does not read the project-root .env; use session keys or secure storage.",
    developmentCredentialPlaceholder: "Optional session key; leave blank for secure store or development .env",
    productionCredentialPlaceholder: "Optional session key; leave blank for secure storage",
    savedSuccessfully: "Saved",
    saveFailed: "Save failed",
    readinessTitle: "Local runtime readiness",
    readinessLocalOnly: "Checks local configuration only; does not call MiMo or DeepSeek",
    refresh: "Refresh",
    checking: "Checking…",
    ready: "Ready",
    notReady: "Not ready",
    runtimeMode: "Runtime mode",
    development: "Development",
    production: "Production",
    sidecar: "Go backend executable",
    outputRoot: "Output location",
    missing: "Missing",
    session: "Session",
    secureStore: "Secure store",
    developmentEnv: "Development .env",
    preflightUnavailable: "Runtime readiness check is unavailable.",
    preflightPrompt: "Refresh to inspect this Mac's runtime and configuration status.",
    chooseFileToBegin: "Choose a voice input method",
    chooseFileToBeginDesc: "Start recording or import existing audio, then choose how to process it.",
    readyToRun: "Ready to begin",
    readyToRunDesc: "Your audio is selected. Confirm the output type, then start processing.",
    settingsIntro: "Inspect local readiness and adjust appearance, processing, and session configuration.",
    settingsReadiness: "Local readiness",
    settingsReadinessDesc: "Checks runtime policy, backend, and system dependencies without making online requests.",
    settingsAppearance: "Appearance and language",
    settingsAppearanceDesc: "Choose the interface language and theme preference.",
    settingsBackend: "Processing backend",
    settingsBackendDesc: "Production uses packaged Go; development defaults to Go with Python available as a manual fallback.",
    settingsCredentials: "Credentials",
    settingsCredentialsDesc: "Session keys are never shown in status metadata, logs, or output files.",
    settingsMinutesModel: "Content generation model",
    settingsMinutesModelDesc: "The DeepSeek model generates structured content with the selected template.",
    settingsOutputLocation: "Output location",
    settingsOutputLocationDesc: "Every task folder and generated file stays inside the configured output location.",
    currentOutputFolder: "Current folder",
    changeOutputFolder: "Change Folder…",
    openOutputFolderAction: "Open Folder",
    resetOutputFolder: "Reset to Default",
    outputFolderChanged: "Output folder changed successfully.",
    outputFolderReset: "Output location reset successfully.",
    outputFolderMoveWarning: "Changing the output folder does not move existing tasks.",
    outputFolderChangeBlocked: "Cannot change output folder while processing.",
    outputFolderUnavailable: "Output folder unavailable.",
    outputFolderNotWritable: "Output folder is not writable.",
    chooseOutputFolder: "Choose output folder",
    theme: "Theme",
    themeSystem: "System",
    themeLight: "Light",
    themeDark: "Dark",
    workspaceNavigation: "Workspace navigation",
    newTask: "New task",
    taskLibraryTitle: "Task Library",
    taskLibraryDescription: "Review recent processed tasks, their status, and primary outputs.",
    refreshing: "Refreshing…",
    taskLibraryLoading: "Loading Task Library…",
    taskLibraryEmpty: "No tasks yet",
    taskLibraryEmptyDescription: "Completed audio tasks will appear here.",
    taskLibraryLoadError: "Task Library could not be loaded.",
    retry: "Retry",
    partialErrors: "Some outputs have errors",
    discoveryIssues: "Some tasks could not be read",
    updatedAt: "Updated",
    createdAt: "Created",
    outputMode: "Output mode",
    outputs: "Primary outputs",
    tags: "Tags",
    editMetadata: "Edit task details",
    taskTitle: "Task title",
    taskDescription: "Description",
    descriptionPlaceholder: "Add optional context, highlights, or notes",
    addTag: "Add",
    tagPlaceholder: "Enter a tag",
    removeTag: "Remove tag",
    metadataSaveFailed: "Task details could not be saved. Try again.",
    sortBy: "Sort by",
    sortUpdated: "Recently updated",
    sortCreated: "Recently created",
    sortTitle: "Title",
    filterByTag: "Filter by tag",
    allTasks: "All tasks",
    taskDetails: "Task details",
    openTaskDetails: "Open task details",
    backToTaskLibrary: "Back to Task Library",
    backToContent: "Back to content",
    transcriptSummary: "Transcript",
    generatedArtifactCount: "Generated artifacts",
    saveTitle: "Save task title",
    cancelTitleEdit: "Cancel title editing",
    taskInformation: "Task information",
    existingTags: "Existing tags",
    taskDocuments: "Task documents",
    markdownLoading: "Loading document…",
    markdownLoadError: "This document could not be loaded. Try again.",
    markdownEmpty: "This document is empty.",
    noReadableMarkdown: "This task has no readable Markdown content yet.",
    cardGeneratedContent: "Read latest generated content",
    cardTranscriptOnly: "Read transcript",
    cardNoContent: "View task status",
    available: "Available",
    unavailable: "Unavailable",
    artifactError: "Generation error",
    retryable: "Retryable",
    notRetryable: "Not retryable",
    invalidDate: "Unknown time",
    statusRunning: "Running",
    statusCompleted: "Completed",
    statusCompletedWithErrors: "Completed with errors",
    statusFailed: "Failed",
    statusInterrupted: "Interrupted",
    statusPending: "Pending",
    statusProcessing: "Processing",
    statusUnavailable: "Unavailable",
    statusUnknown: "Unknown status",
    artifactTranscript: "Transcript",
    artifactNotes: "Notes preparation",
    artifactMinutes: "Meeting minutes",
    openArtifact: "Open",
    revealArtifact: "Show in Finder",
    openingArtifact: "Opening…",
    revealingArtifact: "Revealing…",
    artifactActionFailed: "This file could not be accessed.",
    refreshLibraryHint: "Refresh the Task Library and try again.",
    canonicalTranscript: "Canonical transcript",
    generatedContent: "Generated content",
    generationHistory: "Version history",
    generateContent: "Generate content",
    generateAgain: "Generate again",
    generatingContent: "Generating…",
    generationFailed: "Generation failed. The transcript and earlier versions are unchanged.",
    latestVersion: "Latest successful version",
    legacyArtifact: "Legacy output",
    settingsProviders: "Providers and models",
	localModelTitle: "Local speech recognition", localModelDescription: "Manage the official SenseVoice model in Voxly app data. Model installation never writes into Task folders.", localChineseFirst: "Chinese-first", localModelStatus: "Status",
	localNotInstalled: "Not installed", localRuntimeMissing: "Model installed; runtime pending", localModelInvalid: "Model invalid; reinstall required", localUnsupported: "Unsupported on this platform",
	localDownload: "Download model", localImport: "Import official model", localCancel: "Cancel", localDelete: "Delete model", localSource: "Official source", localSize: "Download size",
	localAdvanced: "Advanced download settings", localEndpoint: "Hugging Face Endpoint", localTestEndpoint: "Test connection", localEndpointReachable: "Endpoint is reachable", localOperationFailed: "The local model operation failed. Existing model data was not changed.", localConfirmDelete: "Delete the installed SenseVoice model? Tasks and history will remain unchanged.", localPrivacyTitle: "Privacy:", localPrivacyBody: "With local speech recognition, original audio is not sent to the cloud. If cloud content generation is selected, transcript text is still sent to that service.",
	localEndpointScope: "Applies only to future Hugging Face-hosted models explicitly marked compatible by the catalog. The current SenseVoice model is pinned to the k2-fsa GitHub Release; this setting and connection test neither change nor test its download source.",
    settingsProvidersDesc: "Choose the built-in providers and models used for speech recognition and generated content. Credentials remain stored separately.",
    settingsTemplates: "Content templates",
    settingsTemplatesDesc: "Manage local custom prompt templates. Built-in templates remain read-only.",
    builtInTemplates: "Built-in templates",
    myTemplates: "My templates",
    newTemplate: "New template",
    editTemplate: "Edit",
    deleteTemplate: "Delete",
    templateName: "Name",
    templateDescription: "Description",
    templateInstructions: "Prompt instructions",
    technicalDetails: "Technical details",
    revision: "Revision",
    saveCreatesRevision: "Saving changes will not affect content generated earlier.",
    confirmDeleteTemplate: "Delete this template? Historical outputs remain, but the template will no longer be selectable.",
    templateSaveFailed: "The template could not be saved. Refresh and try again.",
    noCustomTemplates: "No custom templates yet.",
    speechRecognition: "Speech recognition",
    generatedContentProvider: "Generated content",
    provider: "Provider",
    model: "Model",
    speechRecognitionProvider: "Speech recognition",
    generatedWithProvider: "Generated with",
    providerUnknown: "Legacy / unknown",
    providerNotReady: "The current provider configuration is not ready.",
    openSettings: "Open Settings",
  },
};

export function getLocale(): Locale {
  const saved = localStorage.getItem("locale");
  if (saved === "zh" || saved === "en") return saved;
  const lang = navigator.language;
  return lang.startsWith("zh") ? "zh" : "en";
}

export function setLocale(locale: Locale) {
  localStorage.setItem("locale", locale);
}

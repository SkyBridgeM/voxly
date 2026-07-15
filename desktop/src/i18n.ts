export type Locale = "zh" | "en";

export interface Translations {
  appTitle: string;
  appSubtitle: string;
  appKicker: string;
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
}

export const translations: Record<Locale, Translations> = {
  zh: {
    appTitle: "Meeting ASR Desktop",
    appSubtitle: "用于转写、笔记准备和 AI 会议纪要的实验性桌面工作流",
    appKicker: "本地会议工作台",
    workflowEyebrow: "新建任务",
    workflowTitle: "处理会议录音",
    workflowDescription: "选择一段音频和需要的输出，其余文件会整理到同一个任务文件夹。",
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
    transcriptWarning:
      "transcript 模式会将音频发送至 MiMo ASR 进行转写。请勿上传未授权的敏感会议内容。\n\nTranscript mode sends audio to MiMo ASR for transcription.",
    notesWarning:
      "notes 模式会将音频发送至 MiMo ASR 转写，并在本地生成 notes_input.md；不会将转写文本发送至 DeepSeek。\n\nNotes mode sends audio to MiMo ASR and generates notes_input.md locally. No transcript is sent to DeepSeek.",
    llmWarning:
      "minutes 模式会将转写文本发送至 DeepSeek 生成会议总结。请勿上传未授权的敏感会议内容。\n\nMinutes mode sends transcript text to DeepSeek for summary generation.",
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
    chooseFileToBegin: "从一段会议录音开始",
    chooseFileToBeginDesc: "选择本机音频文件，再选择转写、笔记准备或会议纪要。",
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
    settingsMinutesModel: "会议纪要模型",
    settingsMinutesModelDesc: "DeepSeek 模型仅用于会议纪要输出。",
    settingsOutputLocation: "输出位置",
    settingsOutputLocationDesc: "所有任务文件都会保存在已配置输出位置之内。",
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
  },
  en: {
    appTitle: "Meeting ASR Desktop",
    appSubtitle: "Experimental desktop workflow for transcripts, notes preparation, and AI meeting minutes",
    appKicker: "Local meeting workspace",
    workflowEyebrow: "New task",
    workflowTitle: "Process a meeting recording",
    workflowDescription: "Choose an audio file and the output you need. Generated files stay together in one task folder.",
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
    transcriptWarning:
      "Transcript mode sends audio to MiMo ASR for transcription. Do not upload unauthorized sensitive meeting content.",
    notesWarning:
      "Notes mode sends audio to MiMo ASR and generates notes_input.md locally. No transcript is sent to DeepSeek.",
    llmWarning:
      "Minutes mode sends transcript text to DeepSeek for summary generation. Do not upload unauthorized sensitive meeting content.",
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
    chooseFileToBegin: "Start with a meeting recording",
    chooseFileToBeginDesc: "Choose a local audio file, then select transcript, notes preparation, or meeting minutes.",
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
    settingsMinutesModel: "Minutes model",
    settingsMinutesModelDesc: "The DeepSeek model is used only for meeting-minutes output.",
    settingsOutputLocation: "Output location",
    settingsOutputLocationDesc: "Every task folder and generated file stays inside the configured output location.",
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
